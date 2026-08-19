package execution

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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
	result := runCommand(context.Background(), secret, "TEST_SECRET", []string{"secret-consumer", "safe-argument"}, temporaryDirectory, 5*time.Second)
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

func TestBoundedBufferTruncatesWithoutFailingWriter(t *testing.T) {
	buffer := newBoundedBuffer(4)
	written, err := buffer.Write([]byte("abcdef"))
	if err != nil {
		t.Fatal(err)
	}
	if written != 6 {
		t.Fatalf("Write() = %d, want 6", written)
	}
	if got := buffer.String(); got != "abcd\n<output truncated by key-session>\n" {
		t.Fatalf("String() = %q", got)
	}
}

func TestMinimalEnvironmentDropsUnrelatedCredentials(t *testing.T) {
	environment := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/Users/example",
		"LANG=en_US.UTF-8",
		"AWS_SECRET_ACCESS_KEY=do-not-forward",
		"KEY_SESSION_CONSUMER_TOKEN=do-not-forward",
		"MALFORMED",
	}
	got := minimalEnvironment(environment)
	joined := strings.Join(got, "\n")
	for _, expected := range []string{"PATH=/usr/bin:/bin", "HOME=/Users/example", "LANG=en_US.UTF-8"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("minimal environment omitted %q: %q", expected, got)
		}
	}
	for _, forbidden := range []string{"AWS_SECRET_ACCESS_KEY", "KEY_SESSION_CONSUMER_TOKEN", "MALFORMED"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("minimal environment retained %q: %q", forbidden, got)
		}
	}
}

func TestMinimalEnvironmentProvidesExecutablePath(t *testing.T) {
	got := strings.Join(minimalEnvironment([]string{"HOME=/Users/example"}), "\n")
	if !strings.Contains(got, "PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin") {
		t.Fatalf("minimal environment omitted executable path: %q", got)
	}
}

func TestWithEnvironmentReplacesConfiguredVariable(t *testing.T) {
	got := withEnvironment([]string{"PATH=/usr/bin", "TEST_SECRET=old"}, "TEST_SECRET", "new")
	if joined := strings.Join(got, "\n"); strings.Contains(joined, "TEST_SECRET=old") || !strings.Contains(joined, "TEST_SECRET=new") {
		t.Fatalf("withEnvironment() = %q", got)
	}
}
