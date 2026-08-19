package daemon

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
)

type RuntimePaths struct {
	Directory  string
	Descriptor string
	Lock       string
}

func DefaultRuntimePaths() (RuntimePaths, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return RuntimePaths{}, fmt.Errorf("locate application support: %w", err)
	}
	directory := filepath.Join(root, "key-session", "runtime")
	return RuntimePaths{
		Directory:  directory,
		Descriptor: filepath.Join(directory, "endpoint.json"),
		Lock:       filepath.Join(directory, "daemon.lock"),
	}, nil
}

func randomToken() (string, error) {
	return randomCapability("")
}

func randomCapability(prefix string) (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate capability: %w", err)
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}

func randomID() (string, error) {
	return randomIdentifier("ksd_")
}

func randomIdentifier(prefix string) (string, error) {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(value), nil
}

func publishDescriptor(path string, descriptor contractv2.RuntimeDescriptor) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create daemon runtime directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("protect daemon runtime directory: %w", err)
	}
	payload, err := json.MarshalIndent(descriptor, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(path), ".endpoint-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}
