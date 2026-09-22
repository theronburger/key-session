package daemon

import (
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/theronburger/key-session/internal/config"
	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
)

func TestProfileRemovalRequiresApprovalBeforeChangingState(t *testing.T) {
	for _, approved := range []bool{false, true} {
		name := "cancelled"
		if approved {
			name = "approved"
		}
		t.Run(name, func(t *testing.T) {
			leaseSecret := []byte("leased-secret")
			editorSecret := []byte("editor-secret")
			consumer := &consumerSession{leases: map[string]*activeLease{
				"lease": {metadata: contractv2.Lease{Profile: "example"}, secret: leaseSecret},
			}}
			service := &Service{
				config:    config.Store{Path: t.TempDir() + "/config.json"},
				audit:     auditStore{path: t.TempDir() + "/audit.json"},
				consumers: map[[sha256.Size]byte]*consumerSession{sha256.Sum256([]byte("consumer")): consumer},
				management: map[string]*profileManagementSession{
					"editor": {profile: "example", secret: editorSecret, expiresAt: time.Now().Add(time.Minute)},
					"other":  {profile: "other", secret: []byte("other-secret"), expiresAt: time.Now().Add(time.Minute)},
				},
			}
			if err := service.config.Save(config.Config{Profiles: map[string]config.Profile{
				"example": {EnvironmentVariable: "TEST_SECRET", DefaultLeaseSeconds: 3600},
				"other":   {EnvironmentVariable: "OTHER_SECRET", DefaultLeaseSeconds: 3600},
			}}); err != nil {
				t.Fatal(err)
			}
			cancelled := errors.New("authentication cancelled")
			calls := 0
			err := service.deleteProfile("example", func(profile string) error {
				calls++
				if profile != "example" || len(consumer.leases) != 1 || len(service.management) != 2 {
					t.Fatal("profile state changed before authentication")
				}
				if !approved {
					return cancelled
				}
				return nil
			})
			if calls != 1 || (approved && err != nil) || (!approved && !errors.Is(err, cancelled)) {
				t.Fatalf("approval calls = %d, error = %v", calls, err)
			}
			configuration, err := service.config.Load()
			if err != nil {
				t.Fatal(err)
			}
			_, profileExists := configuration.Profiles["example"]
			_, leaseExists := consumer.leases["lease"]
			_, editorExists := service.management["editor"]
			if profileExists == approved || leaseExists == approved || editorExists == approved {
				t.Fatal("profile, lease, and editing session must survive cancellation and be removed after approval")
			}
			if _, exists := configuration.Profiles["other"]; !exists || service.management["other"] == nil {
				t.Fatal("unrelated profile changed")
			}
			if approved {
				for _, value := range append(leaseSecret, editorSecret...) {
					if value != 0 {
						t.Fatal("removed profile retained secret bytes")
					}
				}
			} else if string(leaseSecret) != "leased-secret" || string(editorSecret) != "editor-secret" {
				t.Fatal("cancellation changed secret bytes")
			}
		})
	}
}

func TestManagementCapabilityIsSingleUseAndClearsSecret(t *testing.T) {
	secret := []byte("not-a-real-secret")
	service := &Service{management: map[string]*profileManagementSession{
		"capability": {profile: "example", secret: secret, expiresAt: time.Now().Add(time.Minute)},
	}}

	if err := service.consumeManagementSession("example", "capability"); err != nil {
		t.Fatal(err)
	}
	for index, value := range secret {
		if value != 0 {
			t.Fatalf("secret byte %d was not cleared", index)
		}
	}
	if err := service.consumeManagementSession("example", "capability"); err == nil {
		t.Fatal("reused management capability was accepted")
	}
}

func TestExpiredManagementCapabilityIsRejected(t *testing.T) {
	service := &Service{management: map[string]*profileManagementSession{
		"expired": {profile: "example", secret: []byte("not-a-real-secret"), expiresAt: time.Now().Add(-time.Second)},
	}}

	if err := service.consumeManagementSession("example", "expired"); err == nil {
		t.Fatal("expired management capability was accepted")
	}
	if len(service.management) != 0 {
		t.Fatal("expired management capability was not removed")
	}
}
