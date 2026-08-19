//go:build darwin

package keychain

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestProtectedKeychainRoundTripSetup(t *testing.T) {
	if os.Getenv("KEY_SESSION_KEYCHAIN_INTEGRATION") != "1" {
		t.Skip("set KEY_SESSION_KEYCHAIN_INTEGRATION=1 to exercise the macOS Keychain")
	}

	account := fmt.Sprintf("integration-test-%d-%d", os.Getpid(), time.Now().UnixNano())
	if err := Store(account, []byte("non-secret-integration-value")); err != nil {
		t.Fatalf("store user-presence-protected item: %v", err)
	}
	t.Cleanup(func() {
		if err := Delete(account); err != nil {
			t.Errorf("delete user-presence-protected item: %v", err)
		}
	})
}
