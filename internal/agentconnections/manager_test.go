package agentconnections

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type recordingRunner struct {
	mu       sync.Mutex
	commands [][]string
	run      func(string, []string, map[string]string) (commandResult, error)
}

func (runner *recordingRunner) Run(_ context.Context, executable string, arguments []string, environment map[string]string) (commandResult, error) {
	runner.mu.Lock()
	runner.commands = append(runner.commands, append([]string{executable}, arguments...))
	runner.mu.Unlock()
	return runner.run(executable, arguments, environment)
}

func TestRepairAllInstallsSkillsAndUsesAgentCLIs(t *testing.T) {
	root := t.TempDir()
	helper := testExecutable(t, filepath.Join(root, "helper", "key-session"))
	codex := testExecutable(t, filepath.Join(root, "bin", "codex"))
	claude := testExecutable(t, filepath.Join(root, "bin", "claude"))
	skillSource := filepath.Join(root, "bundle", "using-keys")
	if err := os.MkdirAll(skillSource, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillSource, "SKILL.md"), []byte("---\nname: using-keys\n---\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	codexConfig := testPrivateFile(t, filepath.Join(root, "codex", "config.toml"), []byte("model = \"keep\"\n"))
	claudeConfig := testPrivateFile(t, filepath.Join(root, "claude", ".claude.json"), []byte("{\"foreign\":true}\n"))
	paths := Paths{
		HelperExecutable: helper,
		SkillSource:      skillSource,
		CodexExecutable:  codex,
		CodexConfig:      codexConfig,
		CodexSkill:       filepath.Join(root, "codex", "skills", SkillName),
		ClaudeExecutable: claude,
		ClaudeConfig:     claudeConfig,
		ClaudeSkill:      filepath.Join(root, "claude", "skills", SkillName),
	}
	codexConnected := false
	runner := &recordingRunner{}
	runner.run = func(executable string, arguments []string, environment map[string]string) (commandResult, error) {
		if executable == codex && equalStrings(arguments, []string{"mcp", "list", "--json"}) {
			servers := []map[string]any{}
			if codexConnected {
				servers = append(servers, map[string]any{
					"name": ServerName, "enabled": true,
					"transport": map[string]any{"type": "stdio", "command": helper, "args": []string{"mcp"}, "env_vars": []string{}, "cwd": nil},
				})
			}
			payload, _ := json.Marshal(servers)
			return commandResult{Stdout: payload}, nil
		}
		if executable == codex && equalStrings(arguments, []string{"mcp", "add", ServerName, "--", helper, "mcp"}) {
			codexConnected = true
			return commandResult{}, nil
		}
		if executable == claude && equalStrings(arguments, []string{"mcp", "add", "--scope", "user", ServerName, "--", helper, "mcp"}) {
			if _, found := environment["CLAUDE_CONFIG_DIR"]; found {
				t.Fatal("default Claude invocation overrides CLAUDE_CONFIG_DIR")
			}
			payload := []byte(`{"foreign":true,"mcpServers":{"key-session":{"type":"stdio","command":"` + helper + `","args":["mcp"],"env":{}}}}`)
			if err := os.WriteFile(claudeConfig, payload, 0o600); err != nil {
				t.Fatal(err)
			}
			return commandResult{}, nil
		}
		t.Fatalf("unexpected command: %s %v", executable, arguments)
		return commandResult{ExitCode: 1}, nil
	}

	report := newWithRunner(paths, runner).Repair(context.Background(), "")
	for _, connection := range report.Connections {
		if connection.State != StateConnected || connection.MCPState != StateConnected || connection.SkillState != StateConnected {
			t.Fatalf("%s connection = %+v", connection.Host, connection)
		}
	}
	if _, err := os.Stat(filepath.Join(paths.CodexSkill, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.ClaudeSkill, "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if payload, err := os.ReadFile(codexConfig); err != nil || string(payload) != "model = \"keep\"\n" {
		t.Fatalf("Codex foreign config changed: %q, %v", payload, err)
	}
}

func TestUnsafeSkillDestinationIsRefused(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source")
	if err := os.Mkdir(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "foreign")
	if err := os.WriteFile(target, []byte("leave me"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "skills", SkillName)
	if err := os.Mkdir(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, destination); err != nil {
		t.Fatal(err)
	}
	if err := installSkill(source, destination); err == nil {
		t.Fatal("symlinked destination was replaced")
	}
	payload, err := os.ReadFile(target)
	if err != nil || string(payload) != "leave me" {
		t.Fatalf("symlink target changed: %q, %v", payload, err)
	}
}

func TestPrivateConfigAcceptsReadableButNotWritablePeerModes(t *testing.T) {
	path := testPrivateFile(t, filepath.Join(t.TempDir(), "config.toml"), []byte("model = \"keep\"\n"))
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateFile(path); err != nil {
		t.Fatalf("0644 owner-controlled config was refused: %v", err)
	}
	if err := os.Chmod(path, 0o664); err != nil {
		t.Fatal(err)
	}
	if _, err := readPrivateFile(path); err == nil {
		t.Fatal("group-writable config was accepted")
	}
}

func TestCommandEnvironmentUsesSanitizedExecutablePath(t *testing.T) {
	environment := commandEnvironment("/opt/homebrew/bin/claude", nil)
	joined := strings.Join(environment, "\n")
	if !strings.Contains(joined, "PATH=/opt/homebrew/bin:/opt/homebrew/bin:/usr/local/bin:/usr/bin:/bin:/usr/sbin:/sbin") {
		t.Fatalf("sanitized PATH missing: %s", joined)
	}
	if strings.Contains(joined, "SSH_AUTH_SOCK=") {
		t.Fatal("unrelated parent environment escaped the allowlist")
	}
}

func TestCodexRepairRestoresOnlyWhenConfigurationIsUnchanged(t *testing.T) {
	root := t.TempDir()
	config := testPrivateFile(t, filepath.Join(root, "config.toml"), []byte("original\n"))
	helper := testExecutable(t, filepath.Join(root, "key-session"))
	paths := Paths{HelperExecutable: helper, CodexExecutable: testExecutable(t, filepath.Join(root, "codex")), CodexConfig: config}
	runner := &recordingRunner{}
	runner.run = func(_ string, arguments []string, _ map[string]string) (commandResult, error) {
		switch {
		case equalStrings(arguments, []string{"mcp", "remove", ServerName}):
			if err := os.WriteFile(config, []byte("removed\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return commandResult{}, nil
		case equalStrings(arguments, []string{"mcp", "add", ServerName, "--", helper, "mcp"}):
			if err := os.WriteFile(config, []byte("concurrent\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			return commandResult{ExitCode: 1}, nil
		default:
			t.Fatalf("unexpected arguments: %v", arguments)
			return commandResult{ExitCode: 1}, nil
		}
	}

	err := newWithRunner(paths, runner).repairCodexMCP(context.Background(), StateNeedsRepair)
	if err == nil {
		t.Fatal("failed repair returned no error")
	}
	payload, readErr := os.ReadFile(config)
	if readErr != nil || string(payload) != "concurrent\n" {
		t.Fatalf("concurrent configuration was overwritten: %q, %v", payload, readErr)
	}
}

func testExecutable(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("binary"), 0o700); err != nil {
		t.Fatal(err)
	}
	return path
}

func testPrivateFile(t *testing.T, path string, payload []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
