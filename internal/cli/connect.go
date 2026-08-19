package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/theronburger/key-session/internal/apiclient"
	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
)

func newConnectCommand() *cobra.Command {
	var outputJSON bool
	command := &cobra.Command{
		Use:   "connect [codex|claude]",
		Short: "Install the skill and connect detected coding agents",
		Long:  "Ask the daemon to install the bundled using-keys skill and register the Key Session MCP server through each detected agent's own CLI. Omit the host to configure every detected agent.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(_ *cobra.Command, arguments []string) error {
			host := ""
			if len(arguments) == 1 {
				host = arguments[0]
				if host != "codex" && host != "claude" {
					return fmt.Errorf("unknown agent %q; use codex or claude", host)
				}
			}
			return connectAgents(host, outputJSON)
		},
	}
	command.Flags().BoolVar(&outputJSON, "json", false, "emit machine-readable JSON")
	return command
}

func connectAgents(host string, outputJSON bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	client, err := apiclient.Connect(ctx)
	if err != nil {
		return err
	}
	report, err := client.RepairAgentConnections(ctx, host)
	if err != nil {
		return err
	}
	if outputJSON {
		return json.NewEncoder(os.Stdout).Encode(report)
	}
	printAgentConnections(report)
	return nil
}

func printAgentConnections(report contractv2.AgentConnectionsReport) {
	for _, connection := range report.Connections {
		fmt.Printf("%-12s %-14s MCP: %-12s Skill: %-12s %s\n",
			connection.DisplayName, connection.State, connection.MCPState, connection.SkillState, connection.Detail)
	}
}
