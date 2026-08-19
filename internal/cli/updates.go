package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/theronburger/key-session/internal/buildinfo"
	"github.com/theronburger/key-session/internal/updatecheck"
)

func newVersionCommand() *cobra.Command {
	var outputJSON bool
	command := &cobra.Command{
		Use:   "version",
		Short: "Print version and build information",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			information := buildinfo.Current()
			if outputJSON {
				return printJSON(information)
			}
			fmt.Printf("key-session %s (%s, %s/%s, built %s)\n", information.Version, buildinfo.ShortCommit(information.Commit), information.Platform, information.Architecture, information.BuildDate)
			return nil
		},
	}
	command.Flags().BoolVar(&outputJSON, "json", false, "emit machine-readable JSON")
	return command
}

func newUpdateCommand() *cobra.Command {
	var outputJSON bool
	var force bool
	command := &cobra.Command{
		Use:   "update",
		Short: "Check GitHub for a newer release",
		Long:  "Check the official GitHub releases feed. This command reports availability but never replaces the executable automatically.",
		Args:  cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			return checkForUpdates(outputJSON, force)
		},
	}
	command.Flags().BoolVar(&outputJSON, "json", false, "emit machine-readable JSON")
	command.Flags().BoolVar(&force, "force", false, "ignore the 24-hour update cache")
	return command
}

func checkForUpdates(outputJSON bool, force bool) error {
	checker, err := updatecheck.Default()
	if err != nil {
		return err
	}
	checkContext, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	result, err := checker.Check(checkContext, buildinfo.Current().Version, force)
	if err != nil {
		return err
	}
	if outputJSON {
		return printJSON(result)
	}
	if result.UpdateAvailable {
		fmt.Printf("Update available: %s (installed: %s)\n%s\n", result.LatestVersion, result.CurrentVersion, result.ReleaseURL)
		return nil
	}
	fmt.Printf("key-session %s is up to date.\n", result.CurrentVersion)
	return nil
}

func checkForUpdatesAutomatically(arguments []string) {
	if !automaticUpdateCheckAllowed(arguments, term.IsTerminal(int(os.Stderr.Fd()))) {
		return
	}
	checker, err := updatecheck.Default()
	if err != nil {
		return
	}
	checkContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := checker.Check(checkContext, buildinfo.Current().Version, false)
	if err == nil && result.UpdateAvailable {
		fmt.Fprintf(os.Stderr, "\nkey-session %s is available; run 'key-session update' for details.\n", result.LatestVersion)
	}
}

func automaticUpdateCheckAllowed(arguments []string, standardErrorIsTerminal bool) bool {
	if !standardErrorIsTerminal || os.Getenv("CI") != "" || os.Getenv("KEY_SESSION_NO_UPDATE_CHECK") != "" {
		return false
	}
	for _, argument := range arguments {
		if argument == "--json" {
			return false
		}
	}
	if len(arguments) == 0 {
		return true
	}
	switch arguments[0] {
	case "_daemon", "mcp", "exec", "grant", "setup", "update", "connect":
		return false
	default:
		return true
	}
}
