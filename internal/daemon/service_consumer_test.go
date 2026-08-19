package daemon

import (
	"context"
	"crypto/sha256"
	"strings"
	"testing"
	"time"

	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
)

func TestConsumerCapabilityScopesStatusExecutionAndRevocation(t *testing.T) {
	now := time.Now()
	codexSecret := []byte("codex-only-secret")
	claudeSecret := []byte("claude-only-secret")
	codexToken := "ksc_codex"
	claudeToken := "ksc_claude"
	service := &Service{
		consumers: map[[sha256.Size]byte]*consumerSession{
			sha256.Sum256([]byte(codexToken)): {
				metadata: contractv2.Consumer{ID: "consumer_codex", Label: "Codex task", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
				leases: map[string]*activeLease{
					"lease_codex": {metadata: contractv2.Lease{ID: "lease_codex", ConsumerID: "consumer_codex", ConsumerLabel: "Codex task", Profile: "prod", EnvironmentVariable: "TEST_SECRET", GrantedAt: now, ExpiresAt: now.Add(time.Hour)}, secret: codexSecret},
				},
			},
			sha256.Sum256([]byte(claudeToken)): {
				metadata: contractv2.Consumer{ID: "consumer_claude", Label: "Claude task", CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
				leases: map[string]*activeLease{
					"lease_claude": {metadata: contractv2.Lease{ID: "lease_claude", ConsumerID: "consumer_claude", ConsumerLabel: "Claude task", Profile: "prod", EnvironmentVariable: "TEST_SECRET", GrantedAt: now, ExpiresAt: now.Add(time.Hour)}, secret: claudeSecret},
				},
			},
		},
		management: map[string]*profileManagementSession{},
		audit:      auditStore{path: t.TempDir() + "/audit.json"},
	}

	status, err := service.ConsumerStatus(codexToken)
	if err != nil {
		t.Fatal(err)
	}
	if len(status.Consumer.Leases) != 1 || status.Consumer.Leases[0].ID != "lease_codex" {
		t.Fatalf("Codex status exposed the wrong leases: %#v", status.Consumer.Leases)
	}

	_, err = service.Execute(context.Background(), contractv2.ExecRequest{
		ConsumerToken: claudeToken, LeaseID: "lease_codex", Arguments: []string{"true"}, TimeoutSeconds: 5,
	})
	if err == nil || !strings.Contains(err.Error(), "belongs to another consumer") {
		t.Fatalf("cross-consumer exec error = %v", err)
	}
	if service.Revoke(contractv2.RevokeRequest{ConsumerToken: claudeToken, LeaseID: "lease_codex"}) {
		t.Fatal("Claude revoked Codex's lease")
	}
	if !service.Revoke(contractv2.RevokeRequest{ConsumerToken: codexToken, LeaseID: "lease_codex"}) {
		t.Fatal("Codex could not revoke its own lease")
	}
	for index, value := range codexSecret {
		if value != 0 {
			t.Fatalf("revoked secret byte %d was not cleared", index)
		}
	}
	claudeStatus, err := service.ConsumerStatus(claudeToken)
	if err != nil || len(claudeStatus.Consumer.Leases) != 1 {
		t.Fatalf("Claude's independent lease was disturbed: status=%#v error=%v", claudeStatus, err)
	}
}

func TestConsumerExpiryClearsAllOwnedLeases(t *testing.T) {
	secret := []byte("temporary-secret")
	token := "ksc_expired"
	service := &Service{
		consumers: map[[sha256.Size]byte]*consumerSession{
			sha256.Sum256([]byte(token)): {
				metadata: contractv2.Consumer{ID: "consumer_expired", Label: "Expired task", CreatedAt: time.Now().Add(-2 * time.Hour), ExpiresAt: time.Now().Add(-time.Minute)},
				leases: map[string]*activeLease{
					"lease_expired": {metadata: contractv2.Lease{ID: "lease_expired", Profile: "prod", ExpiresAt: time.Now().Add(time.Hour)}, secret: secret},
				},
			},
		},
		management: map[string]*profileManagementSession{},
		audit:      auditStore{path: t.TempDir() + "/audit.json"},
	}

	if _, err := service.ConsumerStatus(token); err == nil {
		t.Fatal("expired consumer capability was accepted")
	}
	for index, value := range secret {
		if value != 0 {
			t.Fatalf("expired secret byte %d was not cleared", index)
		}
	}
}
