package cli

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"github.com/theronburger/key-session/internal/apiclient"
	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
	"github.com/theronburger/key-session/internal/daemon"
)

func newSSHCommand() *cobra.Command {
	root := &cobra.Command{Use: "ssh", Short: "Manage lease-gated SSH signing"}
	var duration time.Duration
	setup := &cobra.Command{
		Use: "setup <profile>", Short: "Generate an SSH identity in Keychain; print only its public key",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, arguments []string) error {
			if err := validateProfileName(arguments[0]); err != nil {
				return err
			}
			if err := validateLease(duration); err != nil {
				return err
			}
			client, err := apiclient.Connect(context.Background())
			if err != nil {
				return err
			}
			profile, err := client.CreateSSHProfile(context.Background(), contractv2.SSHProfileRequest{
				Name: arguments[0], DefaultLeaseSeconds: int64(duration.Seconds()),
			})
			if err != nil {
				return err
			}
			fmt.Println(profile.PublicKey + " key-session:" + profile.Name)
			return nil
		},
	}
	setup.Flags().DurationVar(&duration, "duration", time.Hour, "default SSH lease duration (1m to 24h)")
	public := &cobra.Command{
		Use: "public-key <profile>", Short: "Print an existing SSH profile's public key without unlocking it",
		Args: cobra.ExactArgs(1),
		RunE: func(_ *cobra.Command, arguments []string) error {
			client, err := apiclient.Connect(context.Background())
			if err != nil {
				return err
			}
			snapshot, err := client.Snapshot(context.Background())
			if err != nil {
				return err
			}
			for _, profile := range snapshot.Profiles {
				if profile.Name == arguments[0] && profile.Kind == "ssh" {
					fmt.Println(profile.PublicKey + " key-session:" + profile.Name)
					return nil
				}
			}
			return fmt.Errorf("SSH profile %q is not configured", arguments[0])
		},
	}
	socket := &cobra.Command{
		Use: "socket", Short: "Print the OpenSSH IdentityAgent socket path", Args: cobra.NoArgs,
		RunE: func(_ *cobra.Command, _ []string) error {
			paths, err := daemon.DefaultRuntimePaths()
			if err != nil {
				return err
			}
			fmt.Println(filepath.Join(paths.Directory, "ssh-agent.sock"))
			return nil
		},
	}
	root.AddCommand(setup, public, socket)
	return root
}
