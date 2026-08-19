package buildinfo

import "testing"

func TestShortCommit(t *testing.T) {
	if got := ShortCommit("1234567890abcdef"); got != "1234567890ab" {
		t.Fatalf("ShortCommit() = %q", got)
	}
	if got := ShortCommit("unknown"); got != "unknown" {
		t.Fatalf("ShortCommit() = %q", got)
	}
}
