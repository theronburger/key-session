package session

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"time"
)

type CommandResult struct {
	ExitCode int
	Stdout   string
	Stderr   string
}

func runCommand(parentContext context.Context, secret []byte, environmentVariable string, message request) CommandResult {
	timeout := time.Duration(message.TimeoutSeconds) * time.Second
	if timeout < time.Second || timeout > 30*time.Minute {
		timeout = 5 * time.Minute
	}
	commandContext, cancel := context.WithTimeout(parentContext, timeout)
	defer cancel()

	if len(message.Arguments) == 0 {
		return CommandResult{ExitCode: 2, Stderr: "exec requires a program after --"}
	}
	command := exec.CommandContext(commandContext, message.Arguments[0], message.Arguments[1:]...)
	command.Dir = message.WorkingDirectory
	command.Env = withEnvironment(os.Environ(), environmentVariable, string(secret))
	var standardOutput bytes.Buffer
	var standardError bytes.Buffer
	command.Stdout = &standardOutput
	command.Stderr = &standardError

	err := command.Run()
	exitCode := 0
	if err != nil {
		if commandContext.Err() == context.DeadlineExceeded {
			return CommandResult{ExitCode: 124, Stderr: fmt.Sprintf("command timed out after %s", timeout)}
		}
		if exitError, ok := err.(*exec.ExitError); ok {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
			standardError.WriteString(err.Error())
		}
	}

	sensitiveValues := valuesToRedact(string(secret))
	return CommandResult{
		ExitCode: exitCode,
		Stdout:   redact(standardOutput.String(), sensitiveValues),
		Stderr:   redact(standardError.String(), sensitiveValues),
	}
}

func withEnvironment(environment []string, name string, value string) []string {
	prefix := name + "="
	updated := make([]string, 0, len(environment)+1)
	for _, variable := range environment {
		if !strings.HasPrefix(variable, prefix) {
			updated = append(updated, variable)
		}
	}
	return append(updated, prefix+value)
}

func valuesToRedact(secret string) []string {
	values := []string{secret}
	parsed, err := url.Parse(secret)
	if err == nil && parsed.User != nil {
		if password, found := parsed.User.Password(); found && password != "" {
			values = append(values, password)
			if decoded, err := url.QueryUnescape(password); err == nil && decoded != password {
				values = append(values, decoded)
			}
		}
	}
	return values
}

func redact(text string, sensitiveValues []string) string {
	redacted := text
	for _, sensitiveValue := range sensitiveValues {
		if sensitiveValue != "" {
			redacted = strings.ReplaceAll(redacted, sensitiveValue, "<redacted>")
		}
	}
	return redacted
}
