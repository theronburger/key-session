package session

import (
	"fmt"
	"os"
	"path/filepath"
)

type paths struct {
	Directory string
	Socket    string
	PID       string
}

func currentPaths() paths {
	directory := filepath.Join(os.TempDir(), fmt.Sprintf("key-session-%d", os.Getuid()))
	return paths{
		Directory: directory,
		Socket:    filepath.Join(directory, "broker.sock"),
		PID:       filepath.Join(directory, "broker.pid"),
	}
}

func ensurePrivateSessionDirectory(sessionPaths paths) error {
	info, err := os.Lstat(sessionPaths.Directory)
	if err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing symlinked session directory: %s", sessionPaths.Directory)
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("inspect session directory: %w", err)
	}
	if err := os.MkdirAll(sessionPaths.Directory, 0o700); err != nil {
		return fmt.Errorf("create session directory: %w", err)
	}
	return os.Chmod(sessionPaths.Directory, 0o700)
}
