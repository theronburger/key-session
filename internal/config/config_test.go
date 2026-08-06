package config

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "nested", "config.json")}
	expected := Config{
		DefaultProfile: "production-read-only",
		Profiles: map[string]Profile{
			"production-read-only": {EnvironmentVariable: "MONGODB_URI", DefaultLeaseSeconds: 3600},
		},
	}
	if err := store.Save(expected); err != nil {
		t.Fatal(err)
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.DefaultProfile != expected.DefaultProfile {
		t.Fatalf("default profile = %q", loaded.DefaultProfile)
	}
	if loaded.Profiles[expected.DefaultProfile].DefaultLeaseSeconds != 3600 {
		t.Fatalf("lease = %d", loaded.Profiles[expected.DefaultProfile].DefaultLeaseSeconds)
	}
	if loaded.Profiles[expected.DefaultProfile].EnvironmentVariable != "MONGODB_URI" {
		t.Fatalf("environment variable = %q", loaded.Profiles[expected.DefaultProfile].EnvironmentVariable)
	}
}
