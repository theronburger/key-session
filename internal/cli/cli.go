package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/theronburger/key-session/internal/apiclient"
	"github.com/theronburger/key-session/internal/buildinfo"
	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
	"github.com/theronburger/key-session/internal/daemon"
)

const (
	defaultLease              = time.Hour
	minimumLease              = time.Minute
	maximumLease              = 24 * time.Hour
	maximumConsumerCharacters = 80
	maximumReasonCharacters   = 240
	consumerTokenEnvironment  = "KEY_SESSION_CONSUMER_TOKEN"
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
	err := root.Execute()
	if err == nil {
		checkForUpdatesAutomatically(arguments)
	}
	return err
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "key-session",
		Short:         "Grant time-limited access to Keychain secrets",
		Long:          "key-session stores named secrets in macOS Keychain and grants them to local programs through expiring, user-approved leases.",
		Version:       buildinfo.Current().Version,
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
		newUpdateCommand(),
		newDoctorCommand(),
		newDaemonCommand(),
		newMCPCommand(),
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
	var consumer string
	var consumerDuration time.Duration
	var reason string
	command := &cobra.Command{
		Use:     "grant [profile]",
		Short:   "Approve a consumer-scoped expiring lease",
		Long:    "Create or reuse an ephemeral consumer capability, authenticate with Touch ID, and grant only that consumer a profile lease.",
		Example: "  key-session grant production-read-only --consumer \"Codex: jira-mcp-relay\" --reason \"Verify DEED-123 records\" --duration 15m",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, arguments []string) error {
			profileName := ""
			if len(arguments) == 1 {
				profileName = arguments[0]
			}
			return grantProfile(profileName, consumer, reason, duration, consumerDuration)
		},
	}
	command.Flags().DurationVar(&duration, "duration", 0, "override the profile's lease duration")
	command.Flags().StringVar(&consumer, "consumer", "", "agent task or thread receiving a new consumer capability")
	command.Flags().DurationVar(&consumerDuration, "consumer-duration", 0, "new consumer lifetime (default 24h; 1h to 7d)")
	command.Flags().StringVar(&reason, "reason", "", "specific purpose for this lease (required)")
	_ = command.MarkFlagRequired("reason")
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
	var leaseID string
	command := &cobra.Command{
		Use:     "exec -- <program> [arguments...]",
		Short:   "Run a program through one owned lease",
		Long:    "Run one exact program with the selected consumer-owned lease. The secret is injected in memory and redacted from captured output.",
		Example: "  KEY_SESSION_CONSUMER_TOKEN=... key-session exec --lease lease_... -- api-client fetch /resource",
		Args:    cobra.MinimumNArgs(1),
		RunE: func(command *cobra.Command, arguments []string) error {
			if command.ArgsLenAtDash() < 0 {
				return fmt.Errorf("place -- before the child program; example: key-session exec --lease lease_... -- my-command")
			}
			return executeCommand(arguments, timeout, leaseID)
		},
	}
	command.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "child command timeout (1s to 30m)")
	command.Flags().StringVar(&leaseID, "lease", "", "exact approved lease ID (required)")
	_ = command.MarkFlagRequired("lease")
	return command
}

func newRevokeCommand() *cobra.Command {
	var leaseID string
	command := &cobra.Command{
		Use:     "revoke",
		Short:   "End one owned lease or the entire consumer",
		Example: "  KEY_SESSION_CONSUMER_TOKEN=... key-session revoke --lease lease_...\n  KEY_SESSION_CONSUMER_TOKEN=... key-session revoke",
		Args:    cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return revokeLease(leaseID)
		},
	}
	command.Flags().StringVar(&leaseID, "lease", "", "revoke one lease; omit to end the consumer and all its leases")
	return command
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
		Short: "Open the human profile-removal flow",
		Long:  "Profile removal is completed in Key Session.app through its Keychain-authorized human management flow.",
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

func newDaemonCommand() *cobra.Command {
	return &cobra.Command{
		Use:    "_daemon",
		Hidden: true,
		Args:   cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return daemon.Run(context.Background())
		},
	}
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
	client, err := apiclient.Connect(context.Background())
	if err != nil {
		return err
	}
	if err := client.StoreProfile(context.Background(), contractv2.ProfileRequest{
		Name: profileName, EnvironmentVariable: environmentVariable,
		DefaultLeaseSeconds: int64(duration.Seconds()), Secret: string(secret),
	}); err != nil {
		return err
	}
	if err := client.SetDefaultProfile(context.Background(), profileName); err != nil {
		return err
	}
	fmt.Printf("Profile %q saved as the default with a %s lease.\n", profileName, friendlyDuration(duration))
	return nil
}

func grantProfile(requestedProfile, consumerLabel, reason string, durationOverride, consumerDuration time.Duration) error {
	reason = strings.TrimSpace(reason)
	consumerLabel = strings.TrimSpace(consumerLabel)
	if err := validatePromptField("reason", reason, maximumReasonCharacters); err != nil {
		return err
	}
	if durationOverride != 0 {
		if err := validateLease(durationOverride); err != nil {
			return err
		}
	}
	consumerToken := strings.TrimSpace(os.Getenv(consumerTokenEnvironment))
	if consumerToken == "" {
		if err := validatePromptField("consumer", consumerLabel, maximumConsumerCharacters); err != nil {
			return fmt.Errorf("%w when %s is not set", err, consumerTokenEnvironment)
		}
		if consumerDuration != 0 && (consumerDuration < time.Hour || consumerDuration > 7*24*time.Hour) {
			return fmt.Errorf("--consumer-duration must be between 1h and 7d")
		}
	} else if consumerLabel != "" || consumerDuration != 0 {
		return fmt.Errorf("--consumer and --consumer-duration create a new consumer; omit them while %s is set", consumerTokenEnvironment)
	}
	client, err := apiclient.Connect(context.Background())
	if err != nil {
		return err
	}
	grant, err := client.Grant(context.Background(), contractv2.GrantRequest{
		Profile: requestedProfile, ConsumerToken: consumerToken, ConsumerLabel: consumerLabel,
		ConsumerDurationSeconds: int64(consumerDuration.Seconds()), Reason: reason,
		DurationSeconds: int64(durationOverride.Seconds()),
	})
	if err != nil {
		return err
	}
	if grant.ConsumerToken != "" {
		fmt.Printf("Consumer %q created until %s.\n", grant.Consumer.Label, grant.Consumer.ExpiresAt.Format(time.RFC3339))
		fmt.Printf("Consumer capability: %s\n", grant.ConsumerToken)
	}
	fmt.Printf("Lease %s grants profile %q as %s for %s.\n", grant.Lease.ID, grant.Lease.Profile, grant.Lease.EnvironmentVariable, friendlyDuration(time.Until(grant.Lease.ExpiresAt)))
	return nil
}

func showStatus(outputJSON bool) error {
	client, err := apiclient.Connect(context.Background())
	if err != nil {
		return err
	}
	consumerToken := strings.TrimSpace(os.Getenv(consumerTokenEnvironment))
	if consumerToken != "" {
		status, err := client.ConsumerStatus(context.Background(), consumerToken)
		if err != nil {
			return err
		}
		if outputJSON {
			return printJSON(map[string]any{"active": len(status.Consumer.Leases) > 0, "consumer": status.Consumer})
		}
		fmt.Printf("Consumer %q: %d active lease(s), expires in %s.\n", status.Consumer.Label, len(status.Consumer.Leases), friendlyDuration(time.Until(status.Consumer.ExpiresAt)))
		for _, lease := range status.Consumer.Leases {
			fmt.Printf("  %s: %s as %s, %s remaining — %s\n", lease.ID, lease.Profile, lease.EnvironmentVariable, friendlyDuration(time.Until(lease.ExpiresAt)), lease.Reason)
		}
		return nil
	}
	snapshot, err := client.Snapshot(context.Background())
	if err != nil {
		return err
	}
	if outputJSON {
		return printJSON(map[string]any{"consumers": snapshot.Consumers})
	}
	leaseCount := 0
	for _, consumer := range snapshot.Consumers {
		leaseCount += len(consumer.Leases)
	}
	if len(snapshot.Consumers) == 0 {
		fmt.Println("Key Session: no active consumers.")
		return nil
	}
	fmt.Printf("Key Session: %d consumer(s), %d active lease(s). Set %s to inspect one consumer.\n", len(snapshot.Consumers), leaseCount, consumerTokenEnvironment)
	for _, consumer := range snapshot.Consumers {
		fmt.Printf("  %s: %s, %d lease(s), expires in %s\n", consumer.ID, consumer.Label, len(consumer.Leases), friendlyDuration(time.Until(consumer.ExpiresAt)))
	}
	return nil
}

func executeCommand(arguments []string, timeout time.Duration, leaseID string) error {
	if timeout < time.Second || timeout > 30*time.Minute {
		return fmt.Errorf("exec timeout must be between 1s and 30m")
	}
	client, err := apiclient.Connect(context.Background())
	if err != nil {
		return err
	}
	workingDirectory, err := os.Getwd()
	if err != nil {
		return err
	}
	consumerToken, err := requireConsumerToken()
	if err != nil {
		return err
	}
	result, err := client.Execute(context.Background(), contractv2.ExecRequest{
		ConsumerToken: consumerToken, LeaseID: strings.TrimSpace(leaseID),
		Arguments: arguments, WorkingDirectory: workingDirectory, TimeoutSeconds: int(timeout.Seconds()),
	})
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

func revokeLease(leaseID string) error {
	consumerToken, err := requireConsumerToken()
	if err != nil {
		return err
	}
	client, err := apiclient.Connect(context.Background())
	if err != nil {
		return err
	}
	revoked, err := client.Revoke(context.Background(), contractv2.RevokeRequest{ConsumerToken: consumerToken, LeaseID: strings.TrimSpace(leaseID)})
	if err != nil {
		return err
	}
	if !revoked {
		fmt.Println("The lease or consumer was already inactive.")
		return nil
	}
	if leaseID == "" {
		fmt.Println("Consumer session and all of its leases revoked.")
	} else {
		fmt.Printf("Lease %s revoked.\n", leaseID)
	}
	return nil
}

func listProfiles(outputJSON bool) error {
	client, err := apiclient.Connect(context.Background())
	if err != nil {
		return err
	}
	snapshot, err := client.Snapshot(context.Background())
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
		profiles := make([]profileOutput, 0, len(snapshot.Profiles))
		for _, profile := range snapshot.Profiles {
			profiles = append(profiles, profileOutput{
				Name:                profile.Name,
				Default:             profile.IsDefault,
				EnvironmentVariable: profile.EnvironmentVariable,
				LeaseSeconds:        profile.DefaultLeaseSeconds,
			})
		}
		return printJSON(map[string]any{"profiles": profiles})
	}
	for _, profile := range snapshot.Profiles {
		marker := " "
		if profile.IsDefault {
			marker = "*"
		}
		fmt.Printf("%s %s -> %s (%s)\n", marker, profile.Name, profile.EnvironmentVariable, friendlyDuration(time.Duration(profile.DefaultLeaseSeconds)*time.Second))
	}
	return nil
}

func removeProfile(requestedProfile string) error {
	profileName := strings.TrimSpace(requestedProfile)
	if profileName == "" {
		profileName = "the profile"
	} else {
		profileName = fmt.Sprintf("profile %q", profileName)
	}
	return fmt.Errorf("remove %s from Key Session.app; the app requires Keychain approval before deletion", profileName)
}

func validatePromptField(name string, value string, maximumCharacters int) error {
	if value == "" {
		return fmt.Errorf("--%s is required", name)
	}
	if len([]rune(value)) > maximumCharacters {
		return fmt.Errorf("--%s must be at most %d characters", name, maximumCharacters)
	}
	for _, character := range value {
		if character < ' ' || character == 0x7f {
			return fmt.Errorf("--%s cannot contain control characters", name)
		}
	}
	return nil
}

func requireConsumerToken() (string, error) {
	token := strings.TrimSpace(os.Getenv(consumerTokenEnvironment))
	if token == "" {
		return "", fmt.Errorf("%s is required; retain the capability returned by grant only for this agent task", consumerTokenEnvironment)
	}
	return token, nil
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
