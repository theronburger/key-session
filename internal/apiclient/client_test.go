package apiclient

import (
	"strings"
	"testing"
)

func TestDaemonEnvironmentDropsLaunchingShellCredentials(t *testing.T) {
	got := daemonEnvironment([]string{
		"HOME=/Users/example",
		"TMPDIR=/tmp/example",
		"AWS_SECRET_ACCESS_KEY=do-not-forward",
		"KEY_SESSION_CONSUMER_TOKEN=do-not-forward",
	})
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "HOME=/Users/example") || !strings.Contains(joined, "TMPDIR=/tmp/example") {
		t.Fatalf("daemon environment omitted process basics: %q", got)
	}
	if !strings.Contains(joined, "PATH="+safeExecutablePath) {
		t.Fatalf("daemon environment omitted the safe executable path: %q", got)
	}
	if strings.Contains(joined, "AWS_SECRET_ACCESS_KEY") || strings.Contains(joined, "KEY_SESSION_CONSUMER_TOKEN") {
		t.Fatalf("daemon environment retained credentials: %q", got)
	}
}
