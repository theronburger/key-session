package session

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"time"
)

const maximumMessageBytes = 16 * 1024 * 1024

type request struct {
	Action           string   `json:"action"`
	Arguments        []string `json:"arguments,omitempty"`
	WorkingDirectory string   `json:"working_directory,omitempty"`
	TimeoutSeconds   int      `json:"timeout_seconds,omitempty"`
}

type response struct {
	Active              bool   `json:"active,omitempty"`
	Profile             string `json:"profile,omitempty"`
	EnvironmentVariable string `json:"environment_variable,omitempty"`
	ExpiresAt           int64  `json:"expires_at,omitempty"`
	Revoked             bool   `json:"revoked,omitempty"`
	ExitCode            int    `json:"exit_code,omitempty"`
	Stdout              string `json:"stdout,omitempty"`
	Stderr              string `json:"stderr,omitempty"`
	Error               string `json:"error,omitempty"`
}

type Status struct {
	Profile             string
	EnvironmentVariable string
	ExpiresAt           time.Time
}

func writeMessage(connection net.Conn, value any) error {
	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("encode broker message: %w", err)
	}
	if len(encoded) > maximumMessageBytes {
		return fmt.Errorf("broker message exceeds %d bytes", maximumMessageBytes)
	}
	if err := binary.Write(connection, binary.BigEndian, uint32(len(encoded))); err != nil {
		return fmt.Errorf("write broker message size: %w", err)
	}
	if _, err := connection.Write(encoded); err != nil {
		return fmt.Errorf("write broker message: %w", err)
	}
	return nil
}

func readMessage(connection net.Conn, value any) error {
	var messageSize uint32
	if err := binary.Read(connection, binary.BigEndian, &messageSize); err != nil {
		return fmt.Errorf("read broker message size: %w", err)
	}
	if messageSize > maximumMessageBytes {
		return fmt.Errorf("broker message exceeds %d bytes", maximumMessageBytes)
	}
	encoded := make([]byte, messageSize)
	if _, err := io.ReadFull(connection, encoded); err != nil {
		return fmt.Errorf("read broker message: %w", err)
	}
	if err := json.Unmarshal(encoded, value); err != nil {
		return fmt.Errorf("decode broker message: %w", err)
	}
	return nil
}
