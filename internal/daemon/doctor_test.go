package daemon

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrivatePathCheckRejectsLoosePermissionsAndSymlinks(t *testing.T) {
	directory := t.TempDir()
	privateFile := filepath.Join(directory, "private.json")
	if err := os.WriteFile(privateFile, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if check := privatePathCheck("Private", privateFile, false, 0o600, false, ""); check.Status != "pass" {
		t.Fatalf("private file check = %+v", check)
	}
	if err := os.Chmod(privateFile, 0o644); err != nil {
		t.Fatal(err)
	}
	if check := privatePathCheck("Private", privateFile, false, 0o600, false, ""); check.Status != "fail" {
		t.Fatalf("loose file check = %+v", check)
	}

	symlink := filepath.Join(directory, "link.json")
	if err := os.Symlink(privateFile, symlink); err != nil {
		t.Fatal(err)
	}
	if check := privatePathCheck("Private", symlink, false, 0o600, false, ""); check.Status != "fail" {
		t.Fatalf("symlink check = %+v", check)
	}
}

func TestPrivatePathCheckAllowsOptionalMissingFile(t *testing.T) {
	check := privatePathCheck("Optional", filepath.Join(t.TempDir(), "missing"), false, 0o600, true, "Not created yet")
	if check.Status != "info" || check.Detail != "Not created yet" {
		t.Fatalf("missing file check = %+v", check)
	}
}
