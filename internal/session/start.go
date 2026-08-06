package session

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"time"
)

func Start(profile string, environmentVariable string, duration time.Duration, secret []byte) (Status, error) {
	if _, err := StatusNow(); err == nil {
		if err := Revoke(); err != nil {
			return Status{}, fmt.Errorf("replace active lease: %w", err)
		}
	} else {
		removeStaleSessionFiles()
	}

	sessionPaths := currentPaths()
	if err := ensurePrivateSessionDirectory(sessionPaths); err != nil {
		return Status{}, err
	}
	executable, err := os.Executable()
	if err != nil {
		return Status{}, fmt.Errorf("locate key-session executable: %w", err)
	}
	expiresAt := time.Now().Add(duration)
	stdinReader, stdinWriter, err := os.Pipe()
	if err != nil {
		return Status{}, fmt.Errorf("create broker input pipe: %w", err)
	}

	broker := exec.Command(
		executable,
		"_broker",
		"--profile", profile,
		"--env", environmentVariable,
		"--expires-at", strconv.FormatInt(expiresAt.Unix(), 10),
	)
	broker.Stdin = stdinReader
	broker.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := broker.Start(); err != nil {
		stdinReader.Close()
		stdinWriter.Close()
		return Status{}, fmt.Errorf("start broker: %w", err)
	}
	stdinReader.Close()
	if _, err := stdinWriter.Write(secret); err != nil {
		stdinWriter.Close()
		return Status{}, fmt.Errorf("send secret to broker: %w", err)
	}
	if err := stdinWriter.Close(); err != nil {
		return Status{}, fmt.Errorf("close broker input: %w", err)
	}
	if err := broker.Process.Release(); err != nil {
		return Status{}, fmt.Errorf("release broker process: %w", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		status, err := StatusNow()
		if err == nil {
			return status, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return Status{}, fmt.Errorf("broker failed to start")
}
