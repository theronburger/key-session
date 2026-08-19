package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/theronburger/key-session/internal/apiclient"
	contractv2 "github.com/theronburger/key-session/internal/contract/v2"
)

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type mcpResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *mcpError       `json:"error,omitempty"`
}

type mcpError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func newMCPCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "mcp",
		Short: "Serve agent-safe Key Session tools over MCP stdio",
		Args:  cobra.NoArgs,
		RunE:  func(_ *cobra.Command, _ []string) error { return serveMCP() },
	}
}

func serveMCP() error {
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	encoder := json.NewEncoder(os.Stdout)
	for scanner.Scan() {
		var request mcpRequest
		if err := json.Unmarshal(scanner.Bytes(), &request); err != nil {
			_ = encoder.Encode(mcpResponse{JSONRPC: "2.0", Error: &mcpError{Code: -32700, Message: "Parse error"}})
			continue
		}
		if request.Method == "notifications/initialized" {
			continue
		}
		response := handleMCPRequest(request)
		if len(request.ID) > 0 {
			_ = encoder.Encode(response)
		}
	}
	return scanner.Err()
}

func handleMCPRequest(request mcpRequest) mcpResponse {
	response := mcpResponse{JSONRPC: "2.0", ID: request.ID}
	switch request.Method {
	case "initialize":
		response.Result = map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
			"serverInfo":      map[string]string{"name": "key-session", "version": "2"},
		}
	case "ping":
		response.Result = map[string]any{}
	case "tools/list":
		response.Result = map[string]any{"tools": mcpTools()}
	case "tools/call":
		result, err := callMCPTool(request.Params)
		if err != nil {
			response.Result = map[string]any{"isError": true, "content": []map[string]string{{"type": "text", "text": err.Error()}}}
		} else {
			response.Result = map[string]any{"content": []map[string]string{{"type": "text", "text": result}}}
		}
	default:
		response.Error = &mcpError{Code: -32601, Message: "Method not found"}
	}
	return response
}

func mcpTools() []map[string]any {
	return []map[string]any{
		{"name": "list_key_profiles", "description": "List Key Session profile metadata without reading secrets.", "inputSchema": objectSchema(map[string]any{}, []string{})},
		{"name": "get_key_session_status", "description": "Show only the leases owned by this agent task's consumer capability.", "inputSchema": objectSchema(map[string]any{
			"consumer_token": map[string]string{"type": "string", "description": "Ephemeral capability returned by the first approved request in this agent task."},
		}, []string{"consumer_token"})},
		{"name": "request_key_session", "description": "Approve a consumer-scoped Keychain lease. Omit consumer_token only for the task's first request; the response then returns a capability to retain in task context.", "inputSchema": objectSchema(map[string]any{
			"profile":                   map[string]string{"type": "string", "description": "Profile name; omit to use the default."},
			"consumer_token":            map[string]string{"type": "string", "description": "Existing capability for this agent task; omit to create one."},
			"consumer_label":            map[string]string{"type": "string", "description": "Agent and task label; required only when creating a consumer."},
			"consumer_duration_seconds": map[string]any{"type": "integer", "minimum": 3600, "maximum": 604800, "description": "New consumer lifetime; defaults to 24 hours."},
			"reason":                    map[string]string{"type": "string", "description": "Specific purpose for this lease."},
			"duration_seconds":          map[string]any{"type": "integer", "minimum": 60, "maximum": 86400},
		}, []string{"reason"})},
		{"name": "revoke_key_session", "description": "Revoke one owned lease, or omit lease_id to end this consumer and all its leases.", "inputSchema": objectSchema(map[string]any{
			"consumer_token": map[string]string{"type": "string", "description": "This agent task's consumer capability."},
			"lease_id":       map[string]string{"type": "string", "description": "Owned lease to revoke; omit to end the entire consumer."},
		}, []string{"consumer_token"})},
		{"name": "exec_with_key_session", "description": "Run one exact argv through an explicitly selected consumer-owned lease.", "inputSchema": objectSchema(map[string]any{
			"consumer_token":    map[string]string{"type": "string", "description": "This agent task's consumer capability."},
			"lease_id":          map[string]string{"type": "string", "description": "Exact lease returned by request_key_session."},
			"arguments":         map[string]any{"type": "array", "items": map[string]string{"type": "string"}, "minItems": 1},
			"working_directory": map[string]string{"type": "string"},
			"timeout_seconds":   map[string]any{"type": "integer", "minimum": 1, "maximum": 1800},
		}, []string{"consumer_token", "lease_id", "arguments", "working_directory"})},
	}
}

func objectSchema(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func callMCPTool(raw json.RawMessage) (string, error) {
	var call struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &call); err != nil {
		return "", fmt.Errorf("invalid tool call: %w", err)
	}
	callContext, cancel := context.WithTimeout(context.Background(), 35*time.Minute)
	defer cancel()
	client, err := apiclient.Connect(callContext)
	if err != nil {
		return "", err
	}
	encode := func(value any) (string, error) {
		payload, err := json.MarshalIndent(value, "", "  ")
		return string(payload), err
	}
	switch call.Name {
	case "list_key_profiles":
		snapshot, err := client.Snapshot(callContext)
		if err != nil {
			return "", err
		}
		return encode(map[string]any{"profiles": snapshot.Profiles})
	case "get_key_session_status":
		var arguments contractv2.ConsumerRequest
		if err := json.Unmarshal(call.Arguments, &arguments); err != nil {
			return "", err
		}
		status, err := client.ConsumerStatus(callContext, arguments.ConsumerToken)
		if err != nil {
			return "", err
		}
		arguments.ConsumerToken = ""
		return encode(map[string]any{"active": len(status.Consumer.Leases) > 0, "consumer": status.Consumer})
	case "request_key_session":
		var arguments contractv2.GrantRequest
		if err := json.Unmarshal(call.Arguments, &arguments); err != nil {
			return "", err
		}
		grant, err := client.Grant(callContext, arguments)
		if err != nil {
			return "", err
		}
		arguments.ConsumerToken = ""
		return encode(grant)
	case "revoke_key_session":
		var arguments contractv2.RevokeRequest
		if err := json.Unmarshal(call.Arguments, &arguments); err != nil {
			return "", err
		}
		revoked, err := client.Revoke(callContext, arguments)
		if err != nil {
			return "", err
		}
		arguments.ConsumerToken = ""
		return encode(map[string]bool{"revoked": revoked})
	case "exec_with_key_session":
		var arguments contractv2.ExecRequest
		if err := json.Unmarshal(call.Arguments, &arguments); err != nil {
			return "", err
		}
		if arguments.TimeoutSeconds == 0 {
			arguments.TimeoutSeconds = 300
		}
		result, err := client.Execute(callContext, arguments)
		if err != nil {
			return "", err
		}
		return encode(result)
	default:
		return "", fmt.Errorf("unknown tool %q", call.Name)
	}
}
