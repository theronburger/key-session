package apiclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
	"github.com/theronburger/key-session/internal/daemon"
)

type Client struct {
	descriptor contractv2.RuntimeDescriptor
	http       *http.Client
}

func Connect(context context.Context) (*Client, error) {
	client, err := discover()
	if err == nil {
		if handshakeErr := client.Handshake(context); handshakeErr == nil {
			return client, nil
		}
	}
	if err := startDaemon(); err != nil {
		return nil, err
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		client, err = discover()
		if err == nil {
			if handshakeErr := client.Handshake(context); handshakeErr == nil {
				return client, nil
			}
		}
		time.Sleep(75 * time.Millisecond)
	}
	return nil, errors.New("key-session daemon did not become ready")
}

func discover() (*Client, error) {
	paths, err := daemon.DefaultRuntimePaths()
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(paths.Descriptor)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm() != 0o600 {
		return nil, errors.New("daemon endpoint descriptor is not an owner-only regular file")
	}
	payload, err := os.ReadFile(paths.Descriptor)
	if err != nil {
		return nil, err
	}
	var descriptor contractv2.RuntimeDescriptor
	if err := json.Unmarshal(payload, &descriptor); err != nil {
		return nil, err
	}
	endpoint, err := url.Parse(descriptor.Endpoint)
	if err != nil || endpoint.Scheme != "http" || endpoint.Hostname() != "127.0.0.1" || endpoint.Port() == "" || endpoint.Path != "" {
		return nil, errors.New("daemon endpoint is not a canonical loopback origin")
	}
	if descriptor.SchemaVersion != contractv2.SchemaVersion || descriptor.Token == "" || descriptor.PID <= 0 {
		return nil, errors.New("daemon endpoint descriptor is incompatible")
	}
	transport := &http.Transport{
		Proxy:              nil,
		DialContext:        (&net.Dialer{Timeout: 3 * time.Second}).DialContext,
		DisableCompression: true,
	}
	return &Client{descriptor: descriptor, http: &http.Client{
		Transport: transport, Timeout: 35 * time.Minute,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func startDaemon() error {
	applicationSupport, err := os.UserConfigDir()
	if err != nil {
		return fmt.Errorf("locate key-session helper: %w", err)
	}
	executable := filepath.Join(applicationSupport, "key-session", "Key Session Helper.app", "Contents", "MacOS", "KeySessionDaemon")
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("key-session helper is not installed; open Key Session.app once, then retry")
	}
	launchAgent := filepath.Join(filepath.Dir(applicationSupport), "LaunchAgents", "com.theronburger.key-session.daemon.plist")
	launchAgentInfo, err := os.Lstat(launchAgent)
	if err != nil || !launchAgentInfo.Mode().IsRegular() || launchAgentInfo.Mode().Perm() != 0o600 {
		return errors.New("key-session launch agent is not installed correctly; open Key Session.app and run Connection Doctor")
	}
	target := fmt.Sprintf("gui/%d/com.theronburger.key-session.daemon", os.Getuid())
	if runLaunchctl("kickstart", target) == nil {
		return nil
	}
	domain := fmt.Sprintf("gui/%d", os.Getuid())
	if err := runLaunchctl("bootstrap", domain, launchAgent); err != nil {
		return fmt.Errorf("register key-session launch agent: %w", err)
	}
	return nil
}

const safeExecutablePath = "/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin"

func daemonEnvironment(environment []string) []string {
	allowed := map[string]struct{}{"HOME": {}, "TMPDIR": {}, "USER": {}, "LOGNAME": {}, "LANG": {}, "LC_ALL": {}, "LC_CTYPE": {}}
	clean := make([]string, 0, len(allowed)+1)
	for _, variable := range environment {
		name, _, found := strings.Cut(variable, "=")
		if found {
			if _, ok := allowed[name]; ok {
				clean = append(clean, variable)
			}
		}
	}
	return append(clean, "PATH="+safeExecutablePath)
}

func runLaunchctl(arguments ...string) error {
	command := exec.Command("/bin/launchctl", arguments...)
	command.Env = daemonEnvironment(os.Environ())
	command.Stdin = nil
	command.Stdout = nil
	command.Stderr = nil
	return command.Run()
}

func (client *Client) Handshake(context context.Context) error {
	var handshake contractv2.Handshake
	if err := client.get(context, "/handshake", &handshake); err != nil {
		return err
	}
	if handshake.SchemaVersion != contractv2.SchemaVersion || handshake.DaemonInstanceID != client.descriptor.DaemonInstanceID {
		return errors.New("daemon handshake does not match endpoint descriptor")
	}
	return nil
}

func (client *Client) Snapshot(context context.Context) (contractv2.Snapshot, error) {
	var value contractv2.Snapshot
	err := client.get(context, "/v2/status", &value)
	return value, err
}

func (client *Client) ConsumerStatus(context context.Context, token string) (contractv2.ConsumerStatusResponse, error) {
	var value contractv2.ConsumerStatusResponse
	err := client.send(context, http.MethodPost, "/v2/consumers/status", contractv2.ConsumerRequest{ConsumerToken: token}, &value)
	return value, err
}

func (client *Client) Grant(context context.Context, request contractv2.GrantRequest) (contractv2.GrantResponse, error) {
	var value contractv2.GrantResponse
	err := client.send(context, http.MethodPost, "/v2/leases", request, &value)
	return value, err
}

func (client *Client) Revoke(context context.Context, request contractv2.RevokeRequest) (bool, error) {
	var value struct {
		Revoked bool `json:"revoked"`
	}
	err := client.send(context, http.MethodPost, "/v2/leases/revoke", request, &value)
	return value.Revoked, err
}

func (client *Client) AdminRevoke(context context.Context, request contractv2.AdminRevokeRequest) (bool, error) {
	var value struct {
		Revoked bool `json:"revoked"`
	}
	err := client.send(context, http.MethodPost, "/v2/admin/revoke", request, &value)
	return value.Revoked, err
}

func (client *Client) Execute(context context.Context, request contractv2.ExecRequest) (contractv2.ExecResponse, error) {
	var value contractv2.ExecResponse
	err := client.send(context, http.MethodPost, "/v2/exec", request, &value)
	return value, err
}

func (client *Client) StoreProfile(context context.Context, request contractv2.ProfileRequest) error {
	return client.send(context, http.MethodPost, "/v2/profiles", request, &struct{}{})
}

func (client *Client) Doctor(context context.Context) (contractv2.DoctorReport, error) {
	var value contractv2.DoctorReport
	err := client.get(context, "/v2/doctor", &value)
	return value, err
}

func (client *Client) AgentConnections(context context.Context) (contractv2.AgentConnectionsReport, error) {
	var value contractv2.AgentConnectionsReport
	err := client.get(context, "/v2/agent-connections", &value)
	return value, err
}

func (client *Client) RepairAgentConnections(context context.Context, host string) (contractv2.AgentConnectionsReport, error) {
	var value contractv2.AgentConnectionsReport
	err := client.send(
		context,
		http.MethodPost,
		"/v2/agent-connections/repair",
		contractv2.AgentConnectionRepairRequest{Host: host},
		&value,
	)
	return value, err
}

func (client *Client) get(context context.Context, path string, target any) error {
	return client.send(context, http.MethodGet, path, nil, target)
}

func (client *Client) send(context context.Context, method, path string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(payload)
	}
	request, err := http.NewRequestWithContext(context, method, client.descriptor.Endpoint+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("Authorization", "Bearer "+client.descriptor.Token)
	request.Header.Set("Accept", "application/json")
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.http.Do(request)
	if err != nil {
		return fmt.Errorf("contact key-session daemon: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		var envelope contractv2.ErrorEnvelope
		if json.NewDecoder(io.LimitReader(response.Body, 256*1024)).Decode(&envelope) == nil && envelope.Error.Message != "" {
			return errors.New(envelope.Error.Message)
		}
		return fmt.Errorf("daemon returned HTTP %d", response.StatusCode)
	}
	if target == nil {
		return nil
	}
	return json.NewDecoder(io.LimitReader(response.Body, maximumResponseBytes)).Decode(target)
}

const maximumResponseBytes = 4 * 1024 * 1024
