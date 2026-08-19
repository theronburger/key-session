package daemon

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
)

const maximumAuditEvents = 200

type auditStore struct {
	path string
}

func defaultAuditStore() (auditStore, error) {
	root, err := os.UserConfigDir()
	if err != nil {
		return auditStore{}, err
	}
	return auditStore{path: filepath.Join(root, "key-session", "audit.json")}, nil
}

func (store auditStore) load() ([]contractv2.Event, error) {
	payload, err := os.ReadFile(store.path)
	if os.IsNotExist(err) {
		return []contractv2.Event{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read audit journal: %w", err)
	}
	var events []contractv2.Event
	if err := json.Unmarshal(payload, &events); err != nil {
		return nil, fmt.Errorf("parse audit journal: %w", err)
	}
	if events == nil {
		events = []contractv2.Event{}
	}
	return events, nil
}

func (store auditStore) save(events []contractv2.Event) error {
	if len(events) > maximumAuditEvents {
		events = events[len(events)-maximumAuditEvents:]
	}
	if err := os.MkdirAll(filepath.Dir(store.path), 0o700); err != nil {
		return err
	}
	payload, err := json.MarshalIndent(events, "", "  ")
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(store.path), ".audit-*")
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
	return os.Rename(temporaryPath, store.path)
}
