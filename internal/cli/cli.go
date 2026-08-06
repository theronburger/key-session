package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/theronburger/key-session/internal/config"
	"github.com/theronburger/key-session/internal/keychain"
	"github.com/theronburger/key-session/internal/session"
)

const (
	defaultLease = time.Hour
	minimumLease = time.Minute
	maximumLease = 24 * time.Hour
	version      = "0.1.1"
)

var (
	profileNamePattern         = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	environmentVariablePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

type ExitError struct {
	Code int
}

func (err ExitError) Error() string {
	return fmt.Sprintf("child command exited with status %d", err.Code)
}

func Run(arguments []string) error {
	root := newRootCommand()
	root.SetArgs(arguments)
	return root.Execute()
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "key-session",
		Short:         "Grant time-limited access to Keychain secrets",
		Long:          "key-session stores named secrets in macOS Keychain and grants them to local programs through expiring, user-approved leases.",
		Version:       version,
		SilenceErrors: true,
		SilenceUsage:  true,
		Args:          cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			return command.Help()
		},
	}
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "Show help for key-session or a command",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(command *cobra.Command, arguments []string) error {
			if len(arguments) == 0 {
				return command.Root().Help()
			}
			target, _, err := command.Root().Find(arguments)
			if err != nil {
				return err
			}
			return target.Help()
		},
	})
	root.AddCommand(
		newSetupCommand(),
		newGrantCommand(),
		newStatusCommand(),
		newExecCommand(),
		newRevokeCommand(),
		newProfilesCommand(),
		newRemoveProfileCommand(),
		newVersionCommand(),
		newBrokerCommand(),
	)
	return root
}

func newSetupCommand() *cobra.Command {
	var environmentVariable string
	var duration time.Duration
	command := &cobra.Command{
		Use:     "setup <profile>",
		Short:   "Store a secret and make its profile the default",
		Long:    "Prompt for a secret in the current terminal, store it in the macOS login Keychain with approval required for access, and save only non-secret profile metadata on disk.",
		Example: "  key-session setup production-read-only --env MONGODB_URI --duration 1h\n  key-session setup github-read-only --env GITHUB_TOKEN --duration 30m",
		Args:    cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, arguments []string) error {
			return setupProfile(arguments[0], environmentVariable, duration)
		},
	}
	command.Flags().StringVar(&environmentVariable, "env", "", "environment variable provided to child programs (required)")
	command.Flags().DurationVar(&duration, "duration", defaultLease, "default lease duration (1m to 24h)")
	_ = command.MarkFlagRequired("env")
	return command
}

func newGrantCommand() *cobra.Command {
	var duration time.Duration
	command := &cobra.Command{
		Use:     "grant [profile]",
		Short:   "Approve and start an expiring lease",
		Long:    "Read a configured secret from macOS Keychain after user authentication and hold it in a private local broker until the lease expires or is revoked.",
		Example: "  key-session grant\n  key-session grant production-read-only --duration 15m",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, arguments []string) error {
			profileName := ""
			if len(arguments) == 1 {
				profileName = arguments[0]
			}
			return grantProfile(profileName, duration)
		},
	}
	command.Flags().DurationVar(&duration, "duration", 0, "override the profile's lease duration")
	return command
}

func newStatusCommand() *cobra.Command {
	var outputJSON bool
	command := &cobra.Command{
		Use:     "status",
		Short:   "Show the active profile and remaining lease",
		Example: "  key-session status\n  key-session status --json",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return showStatus(outputJSON)
		},
	}
	command.Flags().BoolVar(&outputJSON, "json", false, "emit machine-readable JSON")
	return command
}

func newExecCommand() *cobra.Command {
	var timeout time.Duration
	command := &cobra.Command{
		Use:     "exec -- <program> [arguments...]",
		Short:   "Run a program with the active secret injected",
		Long:    "Ask the lease broker to run one program with the active profile's secret in its configured environment variable. The secret is redacted from captured output.",
		Example: "  key-session exec -- api-client fetch /resource\n  key-session exec --timeout 2m -- mongosh --nodb --quiet --eval 'let uri=process.env.MONGODB_URI; delete process.env.MONGODB_URI; db=connect(uri); uri=undefined; db.runCommand({ping: 1})'",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, arguments []string) error {
			if command.ArgsLenAtDash() < 0 {
				return fmt.Errorf("place -- before the child program; example: key-session exec -- my-command")
			}
			return executeCommand(arguments, timeout)
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "child command timeout (1s to 30m)")
	return command
}

func newRevokeCommand() *cobra.Command {
	return &cobra.Command{
		Use:     "revoke",
		Short:   "End the active lease immediately",
		Example: "  key-session revoke",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return revokeLease()
		},
	}
}

func newProfilesCommand() *cobra.Command {
	var outputJSON bool
	command := &cobra.Command{
		Use:     "profiles",
		Short:   "List configured profiles without reading secrets",
		Example: "  key-session profiles\n  key-session profiles --json",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return listProfiles(outputJSON)
		},
	}
	command.Flags().BoolVar(&outputJSON, "json", false, "emit machine-readable JSON")
	return command
}

func newRemoveProfileCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "remove-profile [profile]",
		Short: "Delete a profile from Keychain and local configuration",
		Long:  "Delete the selected profile. When omitted, the default profile is removed. This also revokes any active lease.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, arguments []string) error {
			profileName := ""
			if len(arguments) == 1 {
				profileName = arguments[0]
			}
			return removeProfile(profileName)
		},
	}
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the key-session version",
		Args:  cobra.NoArgs,
		Run: func(_ *cobra.Command, _ []string) {
			fmt.Println(version)
		},
	}
}

func newBrokerCommand() *cobra.Command {
	var profileName string
	var environmentVariable string
	var expiresAt int64
	command := &cobra.Command{
		Use:    "_broker",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return runBroker(profileName, environmentVariable, expiresAt)
		},
	}
	command.Flags().StringVar(&profileName, "profile", "", "profile")
	command.Flags().StringVar(&environmentVariable, "env", "", "environment variable")
	command.Flags().Int64Var(&expiresAt, "expires-at", 0, "expiration time")
	return command
}

func setupProfile(profileName string, environmentVariable string, duration time.Duration) error {
	if err := validateProfileName(profileName); err != nil {
		return err
	}
	if err := validateEnvironmentVariable(environmentVariable); err != nil {
		return err
	}
	if err := validateLease(duration); err != nil {
		return err
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return fmt.Errorf("setup requires a normal interactive terminal; run it yourself, not through an agent pipe")
	}

	fmt.Fprintf(os.Stderr, "Paste secret for profile %q: ", profileName)
	secret, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return fmt.Errorf("read secret: %w", err)
	}
	defer clearBytes(secret)
	if len(secret) == 0 {
		return fmt.Errorf("secret was empty; run setup again and paste the value at the hidden prompt")
	}
	if bytes.IndexByte(secret, 0) >= 0 {
		return fmt.Errorf("environment-variable secrets cannot contain a NUL byte")
	}
	if err := keychain.Store(profileName, secret); err != nil {
		return fmt.Errorf("store profile %q: %w", profileName, err)
	}

	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	configuration, err := store.Load()
	if err != nil {
		return err
	}
	configuration.Profiles[profileName] = config.Profile{
		EnvironmentVariable: environmentVariable,
		DefaultLeaseSeconds: int64(duration.Seconds()),
	}
	configuration.DefaultProfile = profileName
	if err := store.Save(configuration); err != nil {
		return err
	}
	fmt.Printf("Profile %q saved as the default with a %s lease.\n", profileName, friendlyDuration(duration))
	return nil
}

func grantProfile(requestedProfile string, durationOverride time.Duration) error {
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	configuration, err := store.Load()
	if err != nil {
		return err
	}
	profileName, profile, err := configuration.Resolve(requestedProfile)
	if err != nil {
		return err
	}
	duration := durationOverride
	if duration == 0 {
		duration = time.Duration(profile.DefaultLeaseSeconds) * time.Second
	}
	if err := validateLease(duration); err != nil {
		return err
	}

	secret, err := keychain.Read(profileName)
	if err != nil {
		return fmt.Errorf("read profile %q: %w", profileName, err)
	}
	defer clearBytes(secret)
	status, err := session.Start(profileName, profile.EnvironmentVariable, duration, secret)
	if err != nil {
		return err
	}
	fmt.Printf("Profile %q granted as %s for %s.\n", status.Profile, status.EnvironmentVariable, friendlyDuration(time.Until(status.ExpiresAt)))
	return nil
}

func showStatus(outputJSON bool) error {
	status, err := session.StatusNow()
	if errors.Is(err, session.ErrInactive) {
		if outputJSON {
			return printJSON(map[string]any{"active": false})
		}
		fmt.Println("Key session: inactive")
		return nil
	}
	if err != nil {
		return err
	}
	if outputJSON {
		return printJSON(map[string]any{
			"active":               true,
			"profile":              status.Profile,
			"environment_variable": status.EnvironmentVariable,
			"expires_at":           status.ExpiresAt.Format(time.RFC3339),
			"remaining_seconds":    max(0, int(time.Until(status.ExpiresAt).Seconds())),
		})
	}
	fmt.Printf("Key session: active (%s as %s, expires in %s)\n", status.Profile, status.EnvironmentVariable, friendlyDuration(time.Until(status.ExpiresAt)))
	return nil
}

func executeCommand(arguments []string, timeout time.Duration) error {
	if timeout < time.Second || timeout > 30*time.Minute {
		return fmt.Errorf("exec timeout must be between 1s and 30m")
	}
	result, err := session.Execute(arguments, timeout)
	if err != nil {
		return err
	}
	if result.Stdout != "" {
		fmt.Print(result.Stdout)
		if !strings.HasSuffix(result.Stdout, "\n") {
			fmt.Println()
		}
	}
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
		if !strings.HasSuffix(result.Stderr, "\n") {
			fmt.Fprintln(os.Stderr)
		}
	}
	if result.ExitCode != 0 {
		return ExitError{Code: result.ExitCode}
	}
	return nil
}

func revokeLease() error {
	if err := session.Revoke(); errors.Is(err, session.ErrInactive) {
		fmt.Println("Key session was already inactive.")
		return nil
	} else if err != nil {
		return err
	}
	fmt.Println("Key session revoked.")
	return nil
}

func listProfiles(outputJSON bool) error {
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	configuration, err := store.Load()
	if err != nil {
		return err
	}
	if outputJSON {
		type profileOutput struct {
			Name                string `json:"name"`
			Default             bool   `json:"default"`
			EnvironmentVariable string `json:"environment_variable"`
			LeaseSeconds        int64  `json:"lease_seconds"`
		}
		profiles := make([]profileOutput, 0, len(configuration.Profiles))
		for _, profileName := range configuration.SortedProfileNames() {
			profile := configuration.Profiles[profileName]
			profiles = append(profiles, profileOutput{
				Name:                profileName,
				Default:             profileName == configuration.DefaultProfile,
				EnvironmentVariable: profile.EnvironmentVariable,
				LeaseSeconds:        profile.DefaultLeaseSeconds,
			})
		}
		return printJSON(map[string]any{"profiles": profiles})
	}
	for _, profileName := range configuration.SortedProfileNames() {
		profile := configuration.Profiles[profileName]
		marker := " "
		if profileName == configuration.DefaultProfile {
			marker = "*"
		}
		fmt.Printf("%s %s -> %s (%s)\n", marker, profileName, profile.EnvironmentVariable, friendlyDuration(time.Duration(profile.DefaultLeaseSeconds)*time.Second))
	}
	return nil
}

func removeProfile(requestedProfile string) error {
	store, err := config.DefaultStore()
	if err != nil {
		return err
	}
	configuration, err := store.Load()
	if err != nil {
		return err
	}
	profileName, _, err := configuration.Resolve(requestedProfile)
	if err != nil {
		return err
	}
	_ = session.Revoke()
	if err := keychain.Delete(profileName); err != nil {
		return fmt.Errorf("delete profile %q: %w", profileName, err)
	}
	delete(configuration.Profiles, profileName)
	if configuration.DefaultProfile == profileName {
		configuration.DefaultProfile = ""
		remaining := configuration.SortedProfileNames()
		if len(remaining) > 0 {
			configuration.DefaultProfile = remaining[0]
		}
	}
	if err := store.Save(configuration); err != nil {
		return err
	}
	fmt.Printf("Profile %q removed from Keychain and local configuration.\n", profileName)
	return nil
}

func runBroker(profileName string, environmentVariable string, expiresAt int64) error {
	if err := validateProfileName(profileName); err != nil {
		return err
	}
	if err := validateEnvironmentVariable(environmentVariable); err != nil {
		return err
	}
	expiration := time.Unix(expiresAt, 0)
	if !expiration.After(time.Now()) {
		return fmt.Errorf("broker expiration must be in the future")
	}
	secret, err := io.ReadAll(io.LimitReader(os.Stdin, 1024*1024))
	if err != nil {
		return fmt.Errorf("read broker secret: %w", err)
	}
	if len(secret) == 0 {
		return fmt.Errorf("broker secret is empty")
	}
	context, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return session.ServeBroker(context, session.BrokerOptions{
		Profile:             profileName,
		EnvironmentVariable: environmentVariable,
		ExpiresAt:           expiration,
		Secret:              secret,
	})
}

func validateProfileName(profileName string) error {
	if !profileNamePattern.MatchString(profileName) {
		return fmt.Errorf("profile names must be 1-64 characters using letters, numbers, dots, underscores, or hyphens")
	}
	return nil
}

func validateEnvironmentVariable(name string) error {
	if !environmentVariablePattern.MatchString(name) {
		return fmt.Errorf("--env must be a valid environment variable name such as MONGODB_URI")
	}
	upperName := strings.ToUpper(name)
	dangerousNames := map[string]bool{
		"PATH": true, "IFS": true, "BASH_ENV": true, "ENV": true,
		"PROMPT_COMMAND": true, "PYTHONPATH": true, "PYTHONHOME": true,
		"NODE_OPTIONS": true, "RUBYOPT": true,
	}
	if dangerousNames[upperName] || strings.HasPrefix(upperName, "LD_") || strings.HasPrefix(upperName, "DYLD_") {
		return fmt.Errorf("refusing dangerous environment variable %q; choose a data variable such as API_TOKEN", name)
	}
	return nil
}

func validateLease(duration time.Duration) error {
	if duration < minimumLease || duration > maximumLease {
		return fmt.Errorf("lease duration must be between 1m and 24h")
	}
	return nil
}

func friendlyDuration(duration time.Duration) string {
	if duration <= 0 {
		return "expired"
	}
	duration = duration.Round(time.Second)
	hours := int(duration / time.Hour)
	minutes := int(duration%time.Hour) / int(time.Minute)
	seconds := int(duration%time.Minute) / int(time.Second)
	parts := make([]string, 0, 2)
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%dh", hours))
	}
	if minutes > 0 {
		parts = append(parts, fmt.Sprintf("%dm", minutes))
	}
	if seconds > 0 && hours == 0 {
		parts = append(parts, fmt.Sprintf("%ds", seconds))
	}
	if len(parts) == 0 {
		return "expired"
	}
	return strings.Join(parts, " ")
}

func clearBytes(contents []byte) {
	for index := range contents {
		contents[index] = 0
	}
}

func printJSON(value any) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}
