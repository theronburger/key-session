package cli

import (
	"strings"
	"testing"
)

func TestValidatePromptField(t *testing.T) {
	if err := validatePromptField("consumer", "Codex: jira-mcp-relay", maximumConsumerCharacters); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name  string
		value string
		want  string
	}{
		{name: "missing", want: "--consumer is required"},
		{name: "control character", value: "Codex\nspoofed", want: "--consumer cannot contain control characters"},
		{name: "long", value: strings.Repeat("a", maximumConsumerCharacters+1), want: "--consumer must be at most 80 characters"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validatePromptField("consumer", test.value, maximumConsumerCharacters)
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}
		})
	}
}

func TestRequireConsumerToken(t *testing.T) {
	t.Setenv(consumerTokenEnvironment, "  ksc_example  ")
	token, err := requireConsumerToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "ksc_example" {
		t.Fatalf("token = %q", token)
	}
}

func TestAutomaticUpdateCheckAllowed(t *testing.T) {
	t.Setenv("CI", "")
	t.Setenv("KEY_SESSION_NO_UPDATE_CHECK", "")
	if !automaticUpdateCheckAllowed([]string{"status"}, true) {
		t.Fatal("interactive status should permit an update check")
	}
	for _, arguments := range [][]string{
		{"status", "--json"},
		{"exec", "--", "true"},
		{"grant", "example"},
		{"update"},
	} {
		if automaticUpdateCheckAllowed(arguments, true) {
			t.Fatalf("arguments %q unexpectedly permitted an update check", arguments)
		}
	}
	if automaticUpdateCheckAllowed([]string{"status"}, false) {
		t.Fatal("non-interactive status unexpectedly permitted an update check")
	}
}

func TestFindApplicationBundle(t *testing.T) {
	path := "/Users/example/Applications/Key Session.app/Contents/MacOS/key-session"
	if got := findApplicationBundle(path); got != "/Users/example/Applications/Key Session.app" {
		t.Fatalf("findApplicationBundle() = %q", got)
	}
	if got := findApplicationBundle("/usr/local/bin/key-session"); got != "" {
		t.Fatalf("findApplicationBundle() = %q", got)
	}
}
