package session

import (
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

var ErrInactive = errors.New("key-session lease is inactive or expired; run 'key-session grant'")

func StatusNow() (Status, error) {
	reply, err := sendRequest(request{Action: "status"}, 5*time.Second)
	if err != nil || !reply.Active {
		return Status{}, ErrInactive
	}
	expiresAt := time.Unix(reply.ExpiresAt, 0)
	if time.Now().After(expiresAt) {
		return Status{}, ErrInactive
	}
	return Status{
		Profile:             reply.Profile,
		EnvironmentVariable: reply.EnvironmentVariable,
		ExpiresAt:           expiresAt,
	}, nil
}

func Revoke() error {
	if _, err := StatusNow(); err != nil {
		removeStaleSessionFiles()
		return ErrInactive
	}
	reply, err := sendRequest(request{Action: "revoke"}, 5*time.Second)
	if err != nil {
		return err
	}
	if !reply.Revoked {
		return fmt.Errorf("broker did not confirm revocation")
	}
	return nil
}

func Execute(arguments []string, timeout time.Duration) (CommandResult, error) {
	if _, err := StatusNow(); err != nil {
		return CommandResult{}, ErrInactive
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return CommandResult{}, fmt.Errorf("determine command working directory: %w", err)
	}
	reply, err := sendRequest(request{
		Action:           "exec",
		Arguments:        arguments,
		WorkingDirectory: workingDirectory,
		TimeoutSeconds:   int(timeout.Seconds()),
	}, timeout+5*time.Second)
	if err != nil {
		return CommandResult{}, err
	}
	if reply.Error != "" {
		return CommandResult{}, errors.New(reply.Error)
	}
	return CommandResult{ExitCode: reply.ExitCode, Stdout: reply.Stdout, Stderr: reply.Stderr}, nil
}

func sendRequest(message request, timeout time.Duration) (response, error) {
	sessionPaths := currentPaths()
	connection, err := net.DialTimeout("unix", sessionPaths.Socket, timeout)
	if err != nil {
		return response{}, ErrInactive
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(timeout))
	if err := writeMessage(connection, message); err != nil {
		return response{}, err
	}
	var reply response
	if err := readMessage(connection, &reply); err != nil {
		return response{}, err
	}
	return reply, nil
}

func removeStaleSessionFiles() {
	sessionPaths := currentPaths()
	_ = os.Remove(sessionPaths.Socket)
	_ = os.Remove(sessionPaths.PID)
}
