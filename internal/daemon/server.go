package daemon

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/theronburger/key-session/internal/buildinfo"
	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
)

const maximumRequestBytes = 2 * 1024 * 1024

type Server struct {
	service    *Service
	token      string
	instanceID string
	version    string
	startedAt  time.Time
}

func Run(parent context.Context) error {
	paths, err := DefaultRuntimePaths()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.Directory, 0o700); err != nil {
		return err
	}
	lock, err := acquireLock(paths.Lock)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Close(); _ = os.Remove(paths.Lock) }()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("listen on daemon loopback: %w", err)
	}
	defer func() { _ = listener.Close() }()
	token, err := randomToken()
	if err != nil {
		return err
	}
	instanceID, err := randomID()
	if err != nil {
		return err
	}
	startedAt := time.Now()
	version := buildinfo.Current().Version
	service, err := NewService(instanceID, version, startedAt)
	if err != nil {
		return err
	}
	defer service.Close()
	server := &Server{service: service, token: token, instanceID: instanceID, version: version, startedAt: startedAt}
	httpServer := &http.Server{
		Handler:           server,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    16 << 10,
	}
	endpoint := "http://" + listener.Addr().String()
	if err := publishDescriptor(paths.Descriptor, contractv2.RuntimeDescriptor{
		SchemaVersion: contractv2.SchemaVersion, Endpoint: endpoint, Token: token,
		DaemonInstanceID: instanceID, DaemonVersion: version, PID: os.Getpid(), GeneratedAt: time.Now(),
	}); err != nil {
		return err
	}
	defer func() { _ = os.Remove(paths.Descriptor) }()

	runContext, stop := signal.NotifyContext(parent, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	finished := make(chan error, 1)
	go func() {
		err := httpServer.Serve(listener)
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		finished <- err
	}()
	select {
	case err := <-finished:
		return err
	case <-runContext.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return httpServer.Shutdown(shutdownContext)
	}
}

func acquireLock(path string) (*os.File, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err == nil {
		_, _ = fmt.Fprintf(file, "%d\n", os.Getpid())
		return file, nil
	}
	if !os.IsExist(err) {
		return nil, err
	}
	lockInfo, _ := os.Lstat(path)
	payload, readErr := os.ReadFile(path)
	if readErr == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(payload)))
		if parseErr == nil && pid > 0 && syscall.Kill(pid, 0) == nil {
			// A brand-new lock may precede descriptor publication by a few
			// milliseconds. An older lock must prove the daemon identity through
			// its authenticated endpoint so PID reuse cannot wedge startup.
			if (lockInfo != nil && time.Since(lockInfo.ModTime()) < 5*time.Second) || descriptorConfirmsDaemon(filepath.Join(filepath.Dir(path), "endpoint.json"), pid) {
				return nil, fmt.Errorf("key-session daemon is already running as pid %d", pid)
			}
		}
	}
	if err := os.Remove(path); err != nil {
		return nil, fmt.Errorf("remove stale daemon lock: %w", err)
	}
	return acquireLock(path)
}

func descriptorConfirmsDaemon(path string, pid int) bool {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return false
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	var descriptor contractv2.RuntimeDescriptor
	if json.Unmarshal(payload, &descriptor) != nil || descriptor.PID != pid || descriptor.Token == "" || descriptor.DaemonInstanceID == "" {
		return false
	}
	endpoint, err := url.Parse(descriptor.Endpoint)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.Path != "" {
		return false
	}
	request, err := http.NewRequest(http.MethodGet, descriptor.Endpoint+"/handshake", nil)
	if err != nil {
		return false
	}
	request.Header.Set("Authorization", "Bearer "+descriptor.Token)
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil, DialContext: (&net.Dialer{Timeout: 300 * time.Millisecond}).DialContext},
		Timeout:   500 * time.Millisecond,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode != http.StatusOK {
		return false
	}
	var handshake contractv2.Handshake
	if json.NewDecoder(io.LimitReader(response.Body, 64*1024)).Decode(&handshake) != nil {
		return false
	}
	return handshake.SchemaVersion == contractv2.SchemaVersion && handshake.DaemonInstanceID == descriptor.DaemonInstanceID
}

func (server *Server) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	response.Header().Set("Cache-Control", "no-store")
	response.Header().Set("X-Content-Type-Options", "nosniff")
	response.Header().Set("Content-Type", "application/json")
	if request.Header.Get("Origin") != "" {
		writeError(response, http.StatusForbidden, "HOSTILE_ORIGIN", "Browser-origin requests are not allowed", false)
		return
	}
	if !server.authorized(request) {
		writeError(response, http.StatusUnauthorized, "UNAUTHORIZED", "Authentication is required", false)
		return
	}
	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/handshake":
		writeJSON(response, http.StatusOK, contractv2.Handshake{
			SchemaVersion: contractv2.SchemaVersion, DaemonInstanceID: server.instanceID,
			DaemonVersion: server.version, SupportedSchemaVersions: []int{contractv2.SchemaVersion},
		})
	case request.Method == http.MethodGet && request.URL.Path == "/v2/status":
		snapshot, err := server.service.Snapshot()
		if err != nil {
			writeServiceError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, snapshot)
	case request.Method == http.MethodPost && request.URL.Path == "/v2/consumers/status":
		var body contractv2.ConsumerRequest
		if !decodeBody(response, request, &body) {
			return
		}
		status, err := server.service.ConsumerStatus(body.ConsumerToken)
		if err != nil {
			writeServiceError(response, err)
			return
		}
		body.ConsumerToken = ""
		writeJSON(response, http.StatusOK, status)
	case request.Method == http.MethodPost && request.URL.Path == "/v2/leases":
		var body contractv2.GrantRequest
		if !decodeBody(response, request, &body) {
			return
		}
		grant, err := server.service.Grant(body)
		if err != nil {
			writeServiceError(response, err)
			return
		}
		body.ConsumerToken = ""
		writeJSON(response, http.StatusOK, grant)
	case request.Method == http.MethodPost && request.URL.Path == "/v2/leases/revoke":
		var body contractv2.RevokeRequest
		if !decodeBody(response, request, &body) {
			return
		}
		revoked := server.service.Revoke(body)
		body.ConsumerToken = ""
		writeJSON(response, http.StatusOK, map[string]bool{"revoked": revoked})
	case request.Method == http.MethodPost && request.URL.Path == "/v2/admin/revoke":
		var body contractv2.AdminRevokeRequest
		if !decodeBody(response, request, &body) {
			return
		}
		writeJSON(response, http.StatusOK, map[string]bool{"revoked": server.service.AdminRevoke(body)})
	case request.Method == http.MethodPost && request.URL.Path == "/v2/exec":
		var body contractv2.ExecRequest
		if !decodeBody(response, request, &body) {
			return
		}
		result, err := server.service.Execute(request.Context(), body)
		body.ConsumerToken = ""
		if err != nil {
			writeServiceError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, result)
	case request.Method == http.MethodPost && request.URL.Path == "/v2/profiles":
		var body contractv2.ProfileRequest
		if !decodeBody(response, request, &body) {
			return
		}
		if err := server.service.StoreProfile(body); err != nil {
			writeServiceError(response, err)
			return
		}
		body.Secret = ""
		writeJSON(response, http.StatusOK, map[string]bool{"saved": true})
	case request.Method == http.MethodPost && request.URL.Path == "/v2/profiles/default":
		var body contractv2.DefaultProfileRequest
		if !decodeBody(response, request, &body) {
			return
		}
		if err := server.service.SetDefaultProfile(body.Name); err != nil {
			writeServiceError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]bool{"updated": true})
	case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/management/end"):
		name := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/v2/profiles/"), "/management/end")
		if name == "" || strings.Contains(name, "/") {
			writeError(response, http.StatusNotFound, "NOT_FOUND", "Route not found", false)
			return
		}
		var body contractv2.ProfileManagementRequest
		if !decodeBody(response, request, &body) {
			return
		}
		writeJSON(response, http.StatusOK, map[string]bool{"ended": server.service.EndProfileManagement(name, body.ManagementToken)})
	case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/management"):
		name := strings.TrimSuffix(strings.TrimPrefix(request.URL.Path, "/v2/profiles/"), "/management")
		if name == "" || strings.Contains(name, "/") {
			writeError(response, http.StatusNotFound, "NOT_FOUND", "Route not found", false)
			return
		}
		management, err := server.service.BeginProfileManagement(name)
		if err != nil {
			writeServiceError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, management)
	case request.Method == http.MethodPut && strings.HasPrefix(request.URL.Path, "/v2/profiles/"):
		name := strings.TrimPrefix(request.URL.Path, "/v2/profiles/")
		if name == "" || strings.Contains(name, "/") {
			writeError(response, http.StatusNotFound, "NOT_FOUND", "Route not found", false)
			return
		}
		var body contractv2.ProfileUpdateRequest
		if !decodeBody(response, request, &body) {
			return
		}
		if err := server.service.UpdateProfile(name, body); err != nil {
			writeServiceError(response, err)
			return
		}
		body.Secret = ""
		body.ManagementToken = ""
		writeJSON(response, http.StatusOK, map[string]bool{"updated": true})
	case request.Method == http.MethodDelete && strings.HasPrefix(request.URL.Path, "/v2/profiles/"):
		name := strings.TrimPrefix(request.URL.Path, "/v2/profiles/")
		if name == "" || strings.Contains(name, "/") {
			writeError(response, http.StatusNotFound, "NOT_FOUND", "Route not found", false)
			return
		}
		var body contractv2.ProfileManagementRequest
		if !decodeBody(response, request, &body) {
			return
		}
		if err := server.service.DeleteProfile(name, body.ManagementToken); err != nil {
			writeServiceError(response, err)
			return
		}
		writeJSON(response, http.StatusOK, map[string]bool{"deleted": true})
	case request.Method == http.MethodGet && request.URL.Path == "/v2/doctor":
		writeJSON(response, http.StatusOK, server.service.Doctor())
	default:
		writeError(response, http.StatusNotFound, "NOT_FOUND", "Route not found", false)
	}
}

func (server *Server) authorized(request *http.Request) bool {
	header := request.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return false
	}
	provided := sha256.Sum256([]byte(strings.TrimPrefix(header, "Bearer ")))
	expected := sha256.Sum256([]byte(server.token))
	return subtle.ConstantTimeCompare(provided[:], expected[:]) == 1
}

func decodeBody(response http.ResponseWriter, request *http.Request, target any) bool {
	request.Body = http.MaxBytesReader(response, request.Body, maximumRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		writeError(response, http.StatusBadRequest, "INVALID_REQUEST", "Request body is invalid", false)
		return false
	}
	return true
}

func writeServiceError(response http.ResponseWriter, err error) {
	writeError(response, http.StatusBadRequest, "REQUEST_FAILED", err.Error(), false)
}

func writeError(response http.ResponseWriter, status int, code, message string, retryable bool) {
	writeJSON(response, status, contractv2.ErrorEnvelope{
		SchemaVersion: contractv2.SchemaVersion,
		Error:         contractv2.ContractError{Code: code, Message: message, Retryable: retryable},
	})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
