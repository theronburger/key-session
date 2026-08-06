package session

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"time"
)

type BrokerOptions struct {
	Profile             string
	EnvironmentVariable string
	ExpiresAt           time.Time
	Secret              []byte
}

func ServeBroker(context context.Context, options BrokerOptions) error {
	sessionPaths := currentPaths()
	if err := ensurePrivateSessionDirectory(sessionPaths); err != nil {
		return err
	}
	_ = os.Remove(sessionPaths.Socket)

	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: sessionPaths.Socket, Net: "unix"})
	if err != nil {
		return fmt.Errorf("listen on broker socket: %w", err)
	}
	defer listener.Close()
	defer os.Remove(sessionPaths.Socket)
	defer os.Remove(sessionPaths.PID)
	defer clearBytes(options.Secret)
	if err := os.Chmod(sessionPaths.Socket, 0o600); err != nil {
		return fmt.Errorf("protect broker socket: %w", err)
	}
	if err := os.WriteFile(sessionPaths.PID, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0o600); err != nil {
		return fmt.Errorf("write broker pid: %w", err)
	}

	for time.Now().Before(options.ExpiresAt) {
		if err := listener.SetDeadline(time.Now().Add(time.Second)); err != nil {
			return fmt.Errorf("set broker deadline: %w", err)
		}
		connection, err := listener.AcceptUnix()
		if err != nil {
			var networkError net.Error
			if errors.As(err, &networkError) && networkError.Timeout() {
				select {
				case <-context.Done():
					return nil
				default:
					continue
				}
			}
			return fmt.Errorf("accept broker connection: %w", err)
		}
		shouldStop := handleConnection(context, connection, options)
		connection.Close()
		if shouldStop {
			return nil
		}
	}
	return nil
}

func handleConnection(context context.Context, connection net.Conn, options BrokerOptions) bool {
	var message request
	if err := readMessage(connection, &message); err != nil {
		_ = writeMessage(connection, response{Error: err.Error(), ExitCode: 1})
		return false
	}
	if time.Now().After(options.ExpiresAt) {
		_ = writeMessage(connection, response{Error: ErrInactive.Error(), ExitCode: 1})
		return true
	}

	switch message.Action {
	case "status":
		_ = writeMessage(connection, response{
			Active:              true,
			Profile:             options.Profile,
			EnvironmentVariable: options.EnvironmentVariable,
			ExpiresAt:           options.ExpiresAt.Unix(),
		})
		return false
	case "exec":
		if len(message.Arguments) == 0 {
			_ = writeMessage(connection, response{Error: "exec requires a program after --", ExitCode: 2})
			return false
		}
		result := runCommand(context, options.Secret, options.EnvironmentVariable, message)
		_ = writeMessage(connection, response{
			ExitCode: result.ExitCode,
			Stdout:   result.Stdout,
			Stderr:   result.Stderr,
		})
		return false
	case "revoke":
		_ = writeMessage(connection, response{Revoked: true})
		return true
	default:
		_ = writeMessage(connection, response{Error: "unknown broker action", ExitCode: 2})
		return false
	}
}

func clearBytes(contents []byte) {
	for index := range contents {
		contents[index] = 0
	}
}
