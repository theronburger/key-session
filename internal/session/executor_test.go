package session

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCommandKeepsSecretOutOfArgumentsAndRedactsOutput(t *testing.T) {
	temporaryDirectory := t.TempDir()
	fakeProgram := filepath.Join(temporaryDirectory, "secret-consumer")
	script := `#!/bin/sh
case "$*" in
  *dummy-password*) echo "secret appeared in argv" >&2; exit 91 ;;
esac
echo "command-ok"
pwd
echo "$TEST_SECRET" >&2
`
	if err := os.WriteFile(fakeProgram, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", temporaryDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))

	secret := []byte("https://reader:dummy-password@example.invalid/value")
	result := runCommand(context.Background(), secret, "TEST_SECRET", request{
		Arguments:        []string{"secret-consumer", "safe-argument"},
		WorkingDirectory: temporaryDirectory,
		TimeoutSeconds:   5,
	})
	if result.ExitCode != 0 {
		t.Fatalf("exit code = %d, stderr = %q", result.ExitCode, result.Stderr)
	}
	if !strings.Contains(result.Stdout, "command-ok") {
		t.Fatalf("stdout = %q", result.Stdout)
	}
	if !strings.Contains(result.Stdout, temporaryDirectory) {
		t.Fatalf("command did not run in %q: %q", temporaryDirectory, result.Stdout)
	}
	if strings.Contains(result.Stderr, "dummy-password") || !strings.Contains(result.Stderr, "<redacted>") {
		t.Fatalf("stderr was not redacted: %q", result.Stderr)
	}
}
