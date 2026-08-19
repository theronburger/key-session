package daemon

import (
	"testing"
	"time"
)

func TestGrantApprovalMessageFitsSingleAuthenticationSheet(t *testing.T) {
	message := grantApprovalMessage(
		"Codex · Key Session flow demo",
		"mongodb-prod",
		"Show the human approval flow only",
		5*time.Minute,
	)
	want := "Consumer: Codex · Key Session flow demo\n\nProfile: mongodb-prod\nLease: 5m\n\nReason: Show the human approval flow only"
	if message != want {
		t.Fatalf("message = %q, want %q", message, want)
	}
}
