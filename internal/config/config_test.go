package config

import (
	"path/filepath"
	"testing"
)

func TestStoreRoundTrip(t *testing.T) {
	store := Store{Path: filepath.Join(t.TempDir(), "nested", "config.json")}
	expected := Config{
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
	profile := loaded.Profiles["production-read-only"]
	if profile.DefaultLeaseSeconds != 3600 {
		t.Fatalf("lease = %d", profile.DefaultLeaseSeconds)
	}
	if profile.EnvironmentVariable != "MONGODB_URI" {
		t.Fatalf("environment variable = %q", profile.EnvironmentVariable)
	}
}
