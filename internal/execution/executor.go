package execution

import (
	"bytes"
	"context"
	"errors"
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

// ExecuteWithSecret runs one child process with an in-memory secret. It is the
// daemon-facing execution seam; callers never receive the secret itself.
func ExecuteWithSecret(parentContext context.Context, secret []byte, environmentVariable string, arguments []string, workingDirectory string, timeout time.Duration) CommandResult {
	return runCommand(parentContext, secret, environmentVariable, arguments, workingDirectory, timeout)
}

const maximumCapturedStreamBytes = 1024 * 1024

type boundedBuffer struct {
	contents  bytes.Buffer
	remaining int
	truncated bool
}

func newBoundedBuffer(maximumBytes int) *boundedBuffer {
	return &boundedBuffer{remaining: maximumBytes}
}

func (buffer *boundedBuffer) Write(contents []byte) (int, error) {
	writtenToCaller := len(contents)
	if len(contents) > buffer.remaining {
		contents = contents[:buffer.remaining]
		buffer.truncated = true
	}
	_, _ = buffer.contents.Write(contents)
	buffer.remaining -= len(contents)
	return writtenToCaller, nil
}

func (buffer *boundedBuffer) WriteString(contents string) (int, error) {
	return buffer.Write([]byte(contents))
}

func (buffer *boundedBuffer) String() string {
	contents := buffer.contents.String()
	if buffer.truncated {
		contents += "\n<output truncated by key-session>\n"
	}
	return contents
}

func runCommand(parentContext context.Context, secret []byte, environmentVariable string, arguments []string, workingDirectory string, timeout time.Duration) CommandResult {
	if timeout < time.Second || timeout > 30*time.Minute {
		timeout = 5 * time.Minute
	}
	commandContext, cancel := context.WithTimeout(parentContext, timeout)
	defer cancel()

	if len(arguments) == 0 {
		return CommandResult{ExitCode: 2, Stderr: "exec requires a program after --"}
	}
	command := exec.CommandContext(commandContext, arguments[0], arguments[1:]...)
	command.Dir = workingDirectory
	command.Env = withEnvironment(minimalEnvironment(os.Environ()), environmentVariable, string(secret))
	standardOutput := newBoundedBuffer(maximumCapturedStreamBytes)
	standardError := newBoundedBuffer(maximumCapturedStreamBytes)
	command.Stdout = standardOutput
	command.Stderr = standardError

	err := command.Run()
	exitCode := 0
	if err != nil {
		if commandContext.Err() == context.DeadlineExceeded {
			return CommandResult{ExitCode: 124, Stderr: fmt.Sprintf("command timed out after %s", timeout)}
		}
		var exitError *exec.ExitError
		if errors.As(err, &exitError) {
			exitCode = exitError.ExitCode()
		} else {
			exitCode = 1
			_, _ = standardError.WriteString(err.Error())
		}
	}

	sensitiveValues := valuesToRedact(string(secret))
	return CommandResult{
		ExitCode: exitCode,
		Stdout:   redact(standardOutput.String(), sensitiveValues),
		Stderr:   redact(standardError.String(), sensitiveValues),
	}
}

var inheritedEnvironmentNames = map[string]struct{}{
	"COLORTERM": {},
	"HOME":      {},
	"LANG":      {},
	"LC_ALL":    {},
	"LC_CTYPE":  {},
	"LOGNAME":   {},
	"NO_COLOR":  {},
	"PATH":      {},
	"SHELL":     {},
	"TEMP":      {},
	"TERM":      {},
	"TMP":       {},
	"TMPDIR":    {},
	"TZ":        {},
	"USER":      {},
}

// minimalEnvironment keeps process basics needed by normal command-line tools
// without forwarding arbitrary credentials from the daemon's launch context.
func minimalEnvironment(environment []string) []string {
	minimal := make([]string, 0, len(inheritedEnvironmentNames))
	hasPath := false
	for _, variable := range environment {
		name, _, found := strings.Cut(variable, "=")
		if !found {
			continue
		}
		if _, allowed := inheritedEnvironmentNames[name]; allowed {
			minimal = append(minimal, variable)
			if name == "PATH" {
				hasPath = true
			}
		}
	}
	if !hasPath {
		minimal = append(minimal, "PATH=/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin")
	}
	return minimal
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
