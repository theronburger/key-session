package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Profile struct {
	EnvironmentVariable string `json:"environment_variable"`
	DefaultLeaseSeconds int64  `json:"default_lease_seconds"`
}

type Config struct {
	DefaultProfile string             `json:"default_profile,omitempty"`
	Profiles       map[string]Profile `json:"profiles"`
}

type Store struct {
	Path string
}

func DefaultStore() (Store, error) {
	configRoot, err := os.UserConfigDir()
	if err != nil {
		return Store{}, fmt.Errorf("locate user config directory: %w", err)
	}
	return Store{Path: filepath.Join(configRoot, "key-session", "config.json")}, nil
}

func (store Store) Load() (Config, error) {
	contents, err := os.ReadFile(store.Path)
	if os.IsNotExist(err) {
		return Config{Profiles: map[string]Profile{}}, nil
	}
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}

	var loaded Config
	if err := json.Unmarshal(contents, &loaded); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if loaded.Profiles == nil {
		loaded.Profiles = map[string]Profile{}
	}
	return loaded, nil
}

func (store Store) Save(configuration Config) error {
	directory := filepath.Dir(store.Path)
	if err := ensurePrivateDirectory(directory); err != nil {
		return err
	}

	contents, err := json.MarshalIndent(configuration, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	contents = append(contents, '\n')

	temporaryFile, err := os.CreateTemp(directory, "config-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary config: %w", err)
	}
	temporaryPath := temporaryFile.Name()
	defer os.Remove(temporaryPath)

	if err := temporaryFile.Chmod(0o600); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("protect temporary config: %w", err)
	}
	if _, err := temporaryFile.Write(contents); err != nil {
		temporaryFile.Close()
		return fmt.Errorf("write temporary config: %w", err)
	}
	if err := temporaryFile.Close(); err != nil {
		return fmt.Errorf("close temporary config: %w", err)
	}
	if err := os.Rename(temporaryPath, store.Path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	return os.Chmod(store.Path, 0o600)
}

func (configuration Config) Resolve(requestedProfile string) (string, Profile, error) {
	profileName := requestedProfile
	if profileName == "" {
		profileName = configuration.DefaultProfile
	}
	if profileName == "" {
		return "", Profile{}, fmt.Errorf("no default profile; run 'key-session setup <profile> --env <NAME>' first")
	}
	profile, found := configuration.Profiles[profileName]
	if !found {
		return "", Profile{}, fmt.Errorf("profile %q is not configured", profileName)
	}
	return profileName, profile, nil
}

func (configuration Config) SortedProfileNames() []string {
	names := make([]string, 0, len(configuration.Profiles))
	for name := range configuration.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func ensurePrivateDirectory(directory string) error {
	info, err := os.Lstat(directory)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlinked config directory: %s", directory)
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect config directory: %w", err)
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	return os.Chmod(directory, 0o700)
}
