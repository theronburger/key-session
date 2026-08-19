package agentconnections

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
	"golang.org/x/sys/unix"
)

const (
	ServerName       = "key-session"
	SkillName        = "using-keys"
	StateConnected   = "connected"
	StateMissing     = "missing"
	StateNeedsRepair = "needs_repair"
	StateUnavailable = "unavailable"
	StateRefused     = "refused"
	maximumFileBytes = 2 * 1024 * 1024
)

type Paths struct {
	HelperExecutable string
	SkillSource      string
	CodexExecutable  string
	CodexConfig      string
	CodexConfigRoot  string
	CodexSkill       string
	ClaudeExecutable string
	ClaudeConfig     string
	ClaudeSkill      string
}

func StandardPaths() (Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Paths{}, err
	}
	helper, err := os.Executable()
	if err != nil {
		return Paths{}, err
	}
	helper, err = filepath.EvalSymlinks(helper)
	if err != nil {
		return Paths{}, err
	}
	resources := filepath.Join(filepath.Dir(filepath.Dir(helper)), "Resources")
	codexRoot := filepath.Join(home, ".codex")
	claudeRoot := home
	claudeSkillRoot := filepath.Join(home, ".claude")
	return Paths{
		HelperExecutable: helper,
		SkillSource:      filepath.Join(resources, "skills", SkillName),
		CodexExecutable: findExecutable([]string{
			"/Applications/ChatGPT.app/Contents/Resources/codex",
			filepath.Join(home, ".local", "bin", "codex"),
			"/opt/homebrew/bin/codex",
			"/usr/local/bin/codex",
		}),
		CodexConfig:     filepath.Join(codexRoot, "config.toml"),
		CodexConfigRoot: codexRoot,
		CodexSkill:      filepath.Join(codexRoot, "skills", SkillName),
		ClaudeExecutable: findExecutable([]string{
			filepath.Join(home, ".local", "bin", "claude"),
			"/opt/homebrew/bin/claude",
			"/usr/local/bin/claude",
		}),
		ClaudeConfig: filepath.Join(claudeRoot, ".claude.json"),
		ClaudeSkill:  filepath.Join(claudeSkillRoot, "skills", SkillName),
	}, nil
}

type commandResult struct {
	ExitCode int
	Stdout   []byte
}

type commandRunner interface {
	Run(context.Context, string, []string, map[string]string) (commandResult, error)
}

type exactRunner struct{}

func (exactRunner) Run(ctx context.Context, executable string, arguments []string, overrides map[string]string) (commandResult, error) {
	command := exec.CommandContext(ctx, executable, arguments...)
	command.Env = commandEnvironment(executable, overrides)
	command.Stdin = nil
	var stdout bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = io.Discard
	err := command.Run()
	if err == nil {
		return commandResult{ExitCode: 0, Stdout: stdout.Bytes()}, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return commandResult{ExitCode: exitError.ExitCode(), Stdout: stdout.Bytes()}, nil
	}
	return commandResult{}, err
}

type Manager struct {
	paths  Paths
	runner commandRunner
	mu     sync.Mutex
}

func New() (*Manager, error) {
	paths, err := StandardPaths()
	if err != nil {
		return nil, err
	}
	return &Manager{paths: paths, runner: exactRunner{}}, nil
}

func newWithRunner(paths Paths, runner commandRunner) *Manager {
	return &Manager{paths: paths, runner: runner}
}

func (manager *Manager) Inspect(ctx context.Context) contractv2.AgentConnectionsReport {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	return contractv2.AgentConnectionsReport{Connections: []contractv2.AgentConnection{
		manager.inspectCodex(ctx),
		manager.inspectClaude(),
	}}
}

func (manager *Manager) Repair(_ context.Context, host string) contractv2.AgentConnectionsReport {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	host = strings.TrimSpace(host)
	if host == "" {
		return contractv2.AgentConnectionsReport{Connections: []contractv2.AgentConnection{
			manager.repairCodex(ctx),
			manager.repairClaude(ctx),
		}}
	}
	connections := []contractv2.AgentConnection{manager.inspectCodex(ctx), manager.inspectClaude()}
	for index := range connections {
		if connections[index].Host != host {
			continue
		}
		if host == "codex" {
			connections[index] = manager.repairCodex(ctx)
		} else {
			connections[index] = manager.repairClaude(ctx)
		}
		return contractv2.AgentConnectionsReport{Connections: connections}
	}
	return contractv2.AgentConnectionsReport{Connections: connections}
}

func (manager *Manager) inspectCodex(ctx context.Context) contractv2.AgentConnection {
	if manager.paths.CodexExecutable == "" {
		return unavailable("codex", "Codex", "Codex is not installed in a known location.")
	}
	mcpState, detail := manager.inspectCodexMCP(ctx)
	skillState, skillDetail := inspectSkill(manager.paths.SkillSource, manager.paths.CodexSkill)
	return combined("codex", "Codex", mcpState, skillState, detail, skillDetail)
}

func (manager *Manager) inspectClaude() contractv2.AgentConnection {
	if manager.paths.ClaudeExecutable == "" {
		return unavailable("claude", "Claude Code", "Claude Code is not installed in a known location.")
	}
	mcpState, detail := manager.inspectClaudeMCP()
	skillState, skillDetail := inspectSkill(manager.paths.SkillSource, manager.paths.ClaudeSkill)
	return combined("claude", "Claude Code", mcpState, skillState, detail, skillDetail)
}

func (manager *Manager) inspectCodexMCP(ctx context.Context) (string, string) {
	if !isExecutable(manager.paths.HelperExecutable) {
		return StateUnavailable, "The installed Key Session helper is unavailable."
	}
	if _, err := readPrivateFile(manager.paths.CodexConfig); err != nil {
		return StateRefused, "Codex configuration must be an owner-controlled regular file that is not group- or world-writable."
	}
	commandContext, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := manager.runner.Run(commandContext, manager.paths.CodexExecutable, []string{"mcp", "list", "--json"}, map[string]string{
		"CODEX_HOME": manager.paths.CodexConfigRoot,
	})
	if err != nil || result.ExitCode != 0 {
		return StateRefused, "Codex MCP configuration could not be inspected safely."
	}
	var servers []codexServer
	if json.Unmarshal(result.Stdout, &servers) != nil {
		return StateRefused, "Codex returned an unsupported MCP configuration."
	}
	for _, server := range servers {
		if server.Name != ServerName {
			continue
		}
		if server.Enabled && server.Transport.Type == "stdio" &&
			server.Transport.Command == manager.paths.HelperExecutable &&
			equalStrings(server.Transport.Arguments, []string{"mcp"}) &&
			len(server.Transport.EnvironmentVariables) == 0 && server.Transport.CWD == "" {
			return StateConnected, "MCP uses the installed helper with no stored credential."
		}
		return StateNeedsRepair, "The MCP entry does not use the exact installed helper."
	}
	return StateMissing, "MCP is not connected."
}

func (manager *Manager) inspectClaudeMCP() (string, string) {
	if !isExecutable(manager.paths.HelperExecutable) {
		return StateUnavailable, "The installed Key Session helper is unavailable."
	}
	payload, err := readPrivateFile(manager.paths.ClaudeConfig)
	if err != nil {
		return StateRefused, "Claude Code configuration must be an owner-controlled regular file that is not group- or world-writable."
	}
	if payload == nil {
		return StateMissing, "MCP is not connected."
	}
	var root map[string]any
	if json.Unmarshal(payload, &root) != nil {
		return StateRefused, "Claude Code configuration is not valid JSON."
	}
	serversValue, found := root["mcpServers"]
	if !found {
		return StateMissing, "MCP is not connected."
	}
	servers, ok := serversValue.(map[string]any)
	if !ok {
		return StateRefused, "Claude Code uses an unsupported MCP configuration shape."
	}
	serverValue, found := servers[ServerName]
	if !found {
		return StateMissing, "MCP is not connected."
	}
	server, ok := serverValue.(map[string]any)
	if !ok {
		return StateNeedsRepair, "The MCP entry has an unsupported shape."
	}
	arguments, _ := stringSlice(server["args"])
	environmentEmpty := true
	if environment, exists := server["env"]; exists && environment != nil {
		values, valid := environment.(map[string]any)
		environmentEmpty = valid && len(values) == 0
	}
	if server["type"] == "stdio" && server["command"] == manager.paths.HelperExecutable &&
		equalStrings(arguments, []string{"mcp"}) && environmentEmpty {
		return StateConnected, "MCP uses the installed helper with no stored credential."
	}
	return StateNeedsRepair, "The MCP entry does not use the exact installed helper."
}

func (manager *Manager) repairCodex(ctx context.Context) contractv2.AgentConnection {
	before := manager.inspectCodex(ctx)
	if !before.CanRepair {
		return before
	}
	if err := installSkill(manager.paths.SkillSource, manager.paths.CodexSkill); err != nil {
		return refused("codex", "Codex", "The using-keys skill could not be installed safely.")
	}
	if before.MCPState != StateConnected {
		if err := manager.repairCodexMCP(ctx, before.MCPState); err != nil {
			return refused("codex", "Codex", err.Error())
		}
	}
	return manager.inspectCodex(ctx)
}

func (manager *Manager) repairClaude(ctx context.Context) contractv2.AgentConnection {
	before := manager.inspectClaude()
	if !before.CanRepair {
		return before
	}
	if err := installSkill(manager.paths.SkillSource, manager.paths.ClaudeSkill); err != nil {
		return refused("claude", "Claude Code", "The using-keys skill could not be installed safely.")
	}
	if before.MCPState != StateConnected {
		if err := manager.repairClaudeMCP(ctx, before.MCPState); err != nil {
			return refused("claude", "Claude Code", err.Error())
		}
	}
	return manager.inspectClaude()
}

func (manager *Manager) repairCodexMCP(ctx context.Context, state string) error {
	original, err := readPrivateFile(manager.paths.CodexConfig)
	if err != nil {
		return errors.New("Codex configuration is not safe to repair")
	}
	current, err := readPrivateFile(manager.paths.CodexConfig)
	if err != nil || !bytes.Equal(current, original) {
		return errors.New("Codex configuration changed during repair; no changes were made")
	}
	environment := map[string]string{"CODEX_HOME": manager.paths.CodexConfigRoot}
	rollbackExpected := original
	if state == StateNeedsRepair {
		if err := manager.runMutation(ctx, manager.paths.CodexExecutable, []string{"mcp", "remove", ServerName}, environment); err != nil {
			return errors.New("Codex refused to remove the outdated MCP entry")
		}
		rollbackExpected, err = readPrivateFile(manager.paths.CodexConfig)
		if err != nil {
			return errors.New("Codex configuration became unsafe after removing the outdated MCP entry")
		}
	}
	if err := manager.runMutation(ctx, manager.paths.CodexExecutable, []string{
		"mcp", "add", ServerName, "--", manager.paths.HelperExecutable, "mcp",
	}, environment); err != nil {
		if restoreErr := restorePrivateFile(manager.paths.CodexConfig, rollbackExpected, original); restoreErr != nil {
			return errors.New("Codex refused the MCP connection; a concurrent configuration change was left untouched")
		}
		return errors.New("Codex refused the MCP connection; its prior configuration was restored")
	}
	return nil
}

func (manager *Manager) repairClaudeMCP(ctx context.Context, state string) error {
	original, err := readPrivateFile(manager.paths.ClaudeConfig)
	if err != nil {
		return errors.New("Claude Code configuration is not safe to repair")
	}
	current, err := readPrivateFile(manager.paths.ClaudeConfig)
	if err != nil || !bytes.Equal(current, original) {
		return errors.New("Claude Code configuration changed during repair; no changes were made")
	}
	rollbackExpected := original
	environment := map[string]string{}
	if state == StateNeedsRepair {
		if err := manager.runMutation(ctx, manager.paths.ClaudeExecutable, []string{
			"mcp", "remove", ServerName, "--scope", "user",
		}, environment); err != nil {
			return errors.New("Claude Code refused to remove the outdated MCP entry")
		}
		rollbackExpected, err = readPrivateFile(manager.paths.ClaudeConfig)
		if err != nil {
			return errors.New("Claude Code configuration became unsafe after removing the outdated MCP entry")
		}
	}
	if err := manager.runMutation(ctx, manager.paths.ClaudeExecutable, []string{
		"mcp", "add", "--scope", "user", ServerName, "--", manager.paths.HelperExecutable, "mcp",
	}, environment); err != nil {
		if restoreErr := restorePrivateFile(manager.paths.ClaudeConfig, rollbackExpected, original); restoreErr != nil {
			return errors.New("Claude Code refused the MCP connection; a concurrent configuration change was left untouched")
		}
		return errors.New("Claude Code refused the MCP connection; its prior configuration was restored")
	}
	return nil
}

func (manager *Manager) runMutation(ctx context.Context, executable string, arguments []string, environment map[string]string) error {
	commandContext, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := manager.runner.Run(commandContext, executable, arguments, environment)
	if err != nil {
		return err
	}
	if result.ExitCode != 0 {
		return fmt.Errorf("agent command exited with status %d", result.ExitCode)
	}
	return nil
}

type codexServer struct {
	Name      string         `json:"name"`
	Enabled   bool           `json:"enabled"`
	Transport codexTransport `json:"transport"`
}

type codexTransport struct {
	Type                 string   `json:"type"`
	Command              string   `json:"command"`
	Arguments            []string `json:"args"`
	EnvironmentVariables []string `json:"env_vars"`
	CWD                  string   `json:"cwd"`
}

func combined(host, name, mcpState, skillState, mcpDetail, skillDetail string) contractv2.AgentConnection {
	state := StateConnected
	if mcpState == StateRefused || skillState == StateRefused {
		state = StateRefused
	} else if mcpState == StateUnavailable || skillState == StateUnavailable {
		state = StateUnavailable
	} else if mcpState == StateNeedsRepair || skillState == StateNeedsRepair {
		state = StateNeedsRepair
	} else if mcpState == StateMissing || skillState == StateMissing {
		state = StateMissing
	}
	detail := mcpDetail + " " + skillDetail
	return contractv2.AgentConnection{
		Host: host, DisplayName: name, State: state, MCPState: mcpState, SkillState: skillState,
		Detail: strings.TrimSpace(detail), CanRepair: state == StateMissing || state == StateNeedsRepair,
	}
}

func unavailable(host, name, detail string) contractv2.AgentConnection {
	return contractv2.AgentConnection{Host: host, DisplayName: name, State: StateUnavailable,
		MCPState: StateUnavailable, SkillState: StateUnavailable, Detail: detail}
}

func refused(host, name, detail string) contractv2.AgentConnection {
	return contractv2.AgentConnection{Host: host, DisplayName: name, State: StateRefused,
		MCPState: StateRefused, SkillState: StateRefused, Detail: detail}
}

func inspectSkill(source, destination string) (string, string) {
	if !isDirectory(source) {
		return StateUnavailable, "The bundled using-keys skill is unavailable."
	}
	if !pathExists(destination) {
		return StateMissing, "The using-keys skill is not installed."
	}
	sourceHash, sourceErr := treeHash(source)
	destinationHash, destinationErr := treeHash(destination)
	if sourceErr != nil || destinationErr != nil {
		return StateRefused, "The using-keys skill directory is unsafe."
	}
	if sourceHash != destinationHash {
		return StateNeedsRepair, "The using-keys skill is out of date."
	}
	return StateConnected, "The using-keys skill is current."
}

func installSkill(source, destination string) error {
	if !isDirectory(source) {
		return errors.New("skill source is unavailable")
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	if err := validateOwnedDirectory(parent); err != nil {
		return err
	}
	temporary, err := os.MkdirTemp(parent, ".key-session-skill-")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	staged := filepath.Join(temporary, SkillName)
	if err := copyTree(source, staged); err != nil {
		return err
	}
	if !pathExists(destination) {
		return os.Rename(staged, destination)
	}
	if _, err := treeHash(destination); err != nil {
		return err
	}
	backup := filepath.Join(parent, ".key-session-skill-backup-"+fmt.Sprint(time.Now().UnixNano()))
	if err := os.Rename(destination, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, destination); err != nil {
		_ = os.Rename(backup, destination)
		return err
	}
	return os.RemoveAll(backup)
}

func copyTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("skill contains a symbolic link")
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		if !info.Mode().IsRegular() || info.Size() > maximumFileBytes {
			return errors.New("skill contains an unsupported file")
		}
		payload, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, payload, 0o600)
	})
}

func treeHash(root string) (string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errors.New("symbolic links are not supported")
		}
		if entry.IsDir() {
			return nil
		}
		if !info.Mode().IsRegular() || info.Size() > maximumFileBytes {
			return errors.New("unsupported file")
		}
		files = append(files, path)
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	hash := sha256.New()
	for _, path := range files {
		relative, _ := filepath.Rel(root, path)
		_, _ = io.WriteString(hash, relative+"\x00")
		payload, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		_, _ = hash.Write(payload)
		_, _ = hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func readPrivateFile(path string) ([]byte, error) {
	descriptor, err := unix.Open(path, unix.O_RDONLY|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(descriptor), path)
	if file == nil {
		_ = unix.Close(descriptor)
		return nil, errors.New("open private file")
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.Mode().IsRegular() || info.Mode().Perm()&0o022 != 0 || stat.Uid != uint32(os.Geteuid()) || info.Size() > maximumFileBytes {
		return nil, errors.New("unsafe private file")
	}
	payload, err := io.ReadAll(io.LimitReader(file, maximumFileBytes+1))
	if err != nil {
		return nil, err
	}
	if len(payload) > maximumFileBytes {
		return nil, errors.New("unsafe private file")
	}
	return payload, nil
}

func restorePrivateFile(path string, expected, payload []byte) error {
	current, err := readPrivateFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, expected) {
		return errors.New("private file changed concurrently")
	}
	if payload == nil {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	parent := filepath.Dir(path)
	if err := validateOwnedDirectory(parent); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(parent, ".key-session-config-")
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
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryPath, path)
}

func validateOwnedDirectory(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || !info.IsDir() || info.Mode()&0o022 != 0 || stat.Uid != uint32(os.Geteuid()) {
		return errors.New("directory is not owner-controlled")
	}
	return nil
}

func commandEnvironment(executable string, overrides map[string]string) []string {
	allowed := map[string]bool{"HOME": true, "USER": true, "LOGNAME": true, "TMPDIR": true, "LANG": true, "LC_ALL": true, "LC_CTYPE": true}
	environment := make([]string, 0, len(allowed)+len(overrides)+1)
	for _, variable := range os.Environ() {
		name, _, found := strings.Cut(variable, "=")
		if found && allowed[name] {
			environment = append(environment, variable)
		}
	}
	sanitizedPath := strings.Join([]string{
		filepath.Dir(executable), "/opt/homebrew/bin", "/usr/local/bin", "/usr/bin", "/bin", "/usr/sbin", "/sbin",
	}, ":")
	environment = append(environment, "PATH="+sanitizedPath)
	for name, value := range overrides {
		environment = append(environment, name+"="+value)
	}
	return environment
}

func findExecutable(candidates []string) string {
	for _, candidate := range candidates {
		resolved, err := filepath.EvalSymlinks(candidate)
		if err == nil && isExecutable(resolved) {
			return filepath.Clean(candidate)
		}
	}
	return ""
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0
}

func isDirectory(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func stringSlice(value any) ([]string, bool) {
	items, ok := value.([]any)
	if !ok {
		return nil, false
	}
	stringsValue := make([]string, len(items))
	for index, item := range items {
		text, ok := item.(string)
		if !ok {
			return nil, false
		}
		stringsValue[index] = text
	}
	return stringsValue, true
}
