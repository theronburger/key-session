package daemon

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
)

func TestHTTPBoundaryRequiresAuthenticationAndRejectsOrigins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	service, err := NewService("ksd_test", "0.4.0-test", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	server := &Server{service: service, token: "test-token", instanceID: "ksd_test", version: "0.4.0-test", startedAt: time.Now()}

	unauthorized := httptest.NewRecorder()
	server.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/v2/status", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", unauthorized.Code)
	}

	originRequest := httptest.NewRequest(http.MethodGet, "/v2/status", nil)
	originRequest.Header.Set("Authorization", "Bearer test-token")
	originRequest.Header.Set("Origin", "https://example.invalid")
	origin := httptest.NewRecorder()
	server.ServeHTTP(origin, originRequest)
	if origin.Code != http.StatusForbidden {
		t.Fatalf("origin status = %d", origin.Code)
	}

	authorizedRequest := httptest.NewRequest(http.MethodGet, "/v2/status", nil)
	authorizedRequest.Header.Set("Authorization", "Bearer test-token")
	authorized := httptest.NewRecorder()
	server.ServeHTTP(authorized, authorizedRequest)
	if authorized.Code != http.StatusOK {
		t.Fatalf("authorized status = %d: %s", authorized.Code, authorized.Body.String())
	}
	if strings.Contains(authorized.Body.String(), "test-token") {
		t.Fatal("response exposed bearer token")
	}
	if cacheControl := authorized.Header().Get("Cache-Control"); cacheControl != "no-store" {
		t.Fatalf("Cache-Control = %q", cacheControl)
	}
}

func TestRuntimeDescriptorIsOwnerOnly(t *testing.T) {
	path := t.TempDir() + "/runtime/endpoint.json"
	descriptor := testDescriptor()
	if err := publishDescriptor(path, descriptor); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %04o", info.Mode().Perm())
	}
}

func TestAcquireLockRecoversFromOldLockWithReusedPID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "daemon.lock")
	if err := os.WriteFile(path, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Close() }()
}

func testDescriptor() contractv2.RuntimeDescriptor {
	return contractv2.RuntimeDescriptor{
		SchemaVersion:    contractv2.SchemaVersion,
		Endpoint:         "http://127.0.0.1:43123",
		Token:            "test-token",
		DaemonInstanceID: "ksd_test",
		DaemonVersion:    "0.4.0-test",
		PID:              42,
		GeneratedAt:      time.Now(),
	}
}
