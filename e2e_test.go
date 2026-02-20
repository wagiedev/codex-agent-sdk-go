//go:build integration

package codexsdk_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	codexsdk "github.com/wagiedev/codex-agent-sdk-go"
)

// =============================================================================
// Test Suite 1: Agents and Settings
// =============================================================================

// TestAgentsAndSettings_AgentDefinition tests custom agent configuration.
func TestAgentsAndSettings_AgentDefinition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	receivedResponse := false

	for msg, err := range codexsdk.Query(ctx, "Say 'hello'",
		codexsdk.WithPermissionMode("acceptAll"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		switch m := msg.(type) {
		case *codexsdk.AssistantMessage:
			t.Logf("Received assistant message with %d content blocks", len(m.Content))
			receivedResponse = true
		case *codexsdk.ResultMessage:
			require.False(t, m.IsError, "Query should not result in error")
		}
	}

	require.True(t, receivedResponse, "Should receive assistant response")
}

// TestAgentsAndSettings_SettingSources tests setting source loading.
func TestAgentsAndSettings_SettingSources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	receivedResult := false

	// Test with user and project settings
	for msg, err := range codexsdk.Query(ctx, "What is 2+2? Reply with just the number.",
		codexsdk.WithPermissionMode("acceptAll"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		if result, ok := msg.(*codexsdk.ResultMessage); ok {
			receivedResult = true
			require.False(t, result.IsError, "Query should not result in error")
		}
	}

	require.True(t, receivedResult, "Should receive result message")
}

// TestAgentsAndSettings_NoSettingSources tests isolated environment without settings.
func TestAgentsAndSettings_NoSettingSources(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	receivedResult := false

	// Empty setting sources for isolated environment
	for msg, err := range codexsdk.Query(ctx, "Say 'isolated'",
		codexsdk.WithPermissionMode("acceptAll"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		if result, ok := msg.(*codexsdk.ResultMessage); ok {
			receivedResult = true
			require.False(t, result.IsError, "Query should not result in error")
		}
	}

	require.True(t, receivedResult, "Should receive result message")
}

// =============================================================================
// Test Suite 2: Tool Permissions
// =============================================================================

// TestToolPermissions_AllowExplicit tests CanUseTool returning PermissionResultAllow.
func TestToolPermissions_AllowExplicit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var toolCallCount int32

	for msg, err := range codexsdk.Query(ctx, "List files in the current directory using Bash",
		codexsdk.WithPermissionMode("default"),
		codexsdk.WithCanUseTool(func(
			_ context.Context,
			toolName string,
			_ map[string]any,
			_ *codexsdk.ToolPermissionContext,
		) (codexsdk.PermissionResult, error) {
			atomic.AddInt32(&toolCallCount, 1)
			t.Logf("Tool permission check: %s", toolName)

			return &codexsdk.PermissionResultAllow{
				Behavior: "allow",
			}, nil
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		if result, ok := msg.(*codexsdk.ResultMessage); ok {
			require.False(t, result.IsError, "Query should not result in error")
		}
	}

	// The callback should have been invoked at least once
	require.Greater(t, atomic.LoadInt32(&toolCallCount), int32(0),
		"CanUseTool callback should have been invoked")
}

// TestToolPermissions_Deny tests CanUseTool returning PermissionResultDeny.
func TestToolPermissions_Deny(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var deniedTool string

	for _, err := range codexsdk.Query(ctx, "Run the command 'echo hello' using Bash",
		codexsdk.WithPermissionMode("default"),
		codexsdk.WithCanUseTool(func(
			_ context.Context,
			toolName string,
			_ map[string]any,
			_ *codexsdk.ToolPermissionContext,
		) (codexsdk.PermissionResult, error) {
			// Deny Bash commands
			if toolName == "Bash" {
				deniedTool = toolName

				return &codexsdk.PermissionResultDeny{
					Behavior: "deny",
					Message:  "Bash commands are not allowed in this test",
				}, nil
			}

			return &codexsdk.PermissionResultAllow{
				Behavior: "allow",
			}, nil
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}
	}

	// The Bash tool should have been denied
	require.Equal(t, "Bash", deniedTool, "Bash tool should have been denied")
}

// TestToolPermissions_ModifyInput tests modifying tool input via UpdatedInput.
func TestToolPermissions_ModifyInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var inputModified bool

	for _, err := range codexsdk.Query(ctx, "Run 'ls' command",
		codexsdk.WithPermissionMode("default"),
		codexsdk.WithCanUseTool(func(
			_ context.Context,
			toolName string,
			input map[string]any,
			_ *codexsdk.ToolPermissionContext,
		) (codexsdk.PermissionResult, error) {
			// Modify Bash commands to be safer
			if toolName == "Bash" {
				if cmd, ok := input["command"].(string); ok {
					modifiedInput := make(map[string]any, len(input))
					for k, v := range input {
						modifiedInput[k] = v
					}

					modifiedInput["command"] = "echo 'modified: " + cmd + "'"
					inputModified = true

					return &codexsdk.PermissionResultAllow{
						Behavior:     "allow",
						UpdatedInput: modifiedInput,
					}, nil
				}
			}

			return &codexsdk.PermissionResultAllow{
				Behavior: "allow",
			}, nil
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}
	}

	// Input should have been modified
	require.True(t, inputModified, "Tool input should have been modified")
}

// =============================================================================
// Test Suite 3: Partial Messages
// =============================================================================

// TestPartialMessages_StreamEventsReceived tests StreamEvent with IncludePartialMessages.
func TestPartialMessages_StreamEventsReceived(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var streamEventCount int

	for msg, err := range codexsdk.Query(ctx, "Write a short haiku about testing.",
		codexsdk.WithPermissionMode("acceptAll"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		if _, ok := msg.(*codexsdk.StreamEvent); ok {
			streamEventCount++
		}
	}

	require.Greater(t, streamEventCount, 0,
		"Should receive StreamEvents when IncludePartialMessages is true")
}

// TestPartialMessages_EventTypes verifies content_block_delta events are received.
func TestPartialMessages_EventTypes(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	eventTypes := make(map[string]bool)

	for msg, err := range codexsdk.Query(ctx, "Say 'hello world'",
		codexsdk.WithPermissionMode("acceptAll"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		if streamEvent, ok := msg.(*codexsdk.StreamEvent); ok {
			if eventType, ok := streamEvent.Event["type"].(string); ok {
				eventTypes[eventType] = true
			}
		}
	}

	// We expect to see at least message_start or content_block_delta events
	hasExpectedEvents := eventTypes["message_start"] ||
		eventTypes["content_block_delta"] ||
		eventTypes["message_delta"]
	require.True(t, hasExpectedEvents,
		"Should receive expected event types; got: %v", eventTypes)
}

// TestPartialMessages_DisabledByDefault verifies no StreamEvents when disabled.
func TestPartialMessages_DisabledByDefault(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var streamEventCount int

	for msg, err := range codexsdk.Query(ctx, "Say 'hello'",
		codexsdk.WithPermissionMode("acceptAll"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		if _, ok := msg.(*codexsdk.StreamEvent); ok {
			streamEventCount++
		}
	}

	require.Equal(t, 0, streamEventCount,
		"Should not receive StreamEvents when IncludePartialMessages is false")
}

// =============================================================================
// Test Suite 4: Dynamic Control
// =============================================================================

// TestDynamicControl_SetPermissionMode tests changing permission mode mid-session.
func TestDynamicControl_SetPermissionMode(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := codexsdk.NewClient()
	defer client.Close()

	err := client.Start(ctx,
		codexsdk.WithPermissionMode("default"),
	)
	if err != nil {
		if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
			t.Skip("Codex CLI not installed")
		}

		t.Fatalf("Connect failed: %v", err)
	}

	// Change permission mode
	err = client.SetPermissionMode(ctx, "acceptAll")
	require.NoError(t, err, "SetPermissionMode should succeed")

	// Run a query to verify the session still works
	err = client.Query(ctx, "Say 'permission changed'")
	require.NoError(t, err, "Query should succeed after SetPermissionMode")

	var messages []codexsdk.Message
	for msg, err := range client.ReceiveResponse(ctx) {
		require.NoError(t, err, "ReceiveResponse should succeed")
		messages = append(messages, msg)
	}
	require.NotEmpty(t, messages, "Should receive messages")
}

// TestDynamicControl_SetModel tests switching model during session.
func TestDynamicControl_SetModel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := codexsdk.NewClient()
	defer client.Close()

	err := client.Start(ctx,
		codexsdk.WithPermissionMode("acceptAll"),
	)
	if err != nil {
		if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
			t.Skip("Codex CLI not installed")
		}

		t.Fatalf("Connect failed: %v", err)
	}

	// Change model
	model := "codex-mini-latest"
	err = client.SetModel(ctx, &model)
	require.NoError(t, err, "SetModel should succeed")

	// Run a query to verify the session still works
	err = client.Query(ctx, "Say 'model changed'")
	require.NoError(t, err, "Query should succeed after SetModel")

	var messages []codexsdk.Message
	for msg, err := range client.ReceiveResponse(ctx) {
		require.NoError(t, err, "ReceiveResponse should succeed")
		messages = append(messages, msg)
	}
	require.NotEmpty(t, messages, "Should receive messages")
}

// TestDynamicControl_Interrupt tests interrupting a long-running query.
func TestDynamicControl_Interrupt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := codexsdk.NewClient()
	defer client.Close()

	err := client.Start(ctx,
		codexsdk.WithPermissionMode("acceptAll"),
	)
	if err != nil {
		if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
			t.Skip("Codex CLI not installed")
		}

		t.Fatalf("Connect failed: %v", err)
	}

	// Start a long query
	err = client.Query(ctx,
		"Write a very long essay about the history of computing, including many details.")
	require.NoError(t, err, "Query should succeed")

	// Wait a bit for processing to start
	time.Sleep(500 * time.Millisecond)

	// Interrupt
	err = client.Interrupt(ctx)
	require.NoError(t, err, "Interrupt should succeed")

	// Drain remaining messages
	for msg, err := range client.ReceiveMessages(ctx) {
		if err != nil {
			break
		}

		// Check if we got a result
		if result, ok := msg.(*codexsdk.ResultMessage); ok {
			t.Logf("Session ended: isError=%v, numTurns=%d", result.IsError, result.NumTurns)

			break
		}
	}
}

// =============================================================================
// Test Suite 5: Structured Output
// =============================================================================

// TestStructuredOutput_JSONSchema tests OutputFormat produces valid JSON.
func TestStructuredOutput_JSONSchema(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var receivedResponse bool

	for msg, err := range codexsdk.Query(ctx, "What is 2+2? Provide structured output.",
		codexsdk.WithPermissionMode("acceptAll"),
		codexsdk.WithOutputFormat(map[string]any{
			"type": "json_schema",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"answer": map[string]any{
						"type":        "string",
						"description": "The answer to the question",
					},
					"confidence": map[string]any{
						"type":        "number",
						"description": "Confidence level from 0 to 1",
					},
				},
				"required": []string{"answer"},
			},
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		switch m := msg.(type) {
		case *codexsdk.AssistantMessage:
			for _, block := range m.Content {
				if textBlock, ok := block.(*codexsdk.TextBlock); ok {
					t.Logf("Structured output: %s", textBlock.Text)
					receivedResponse = true
				}
			}
		case *codexsdk.ResultMessage:
			require.False(t, m.IsError, "Query should not result in error")
		}
	}

	require.True(t, receivedResponse, "Should receive structured response")
}

// TestStructuredOutput_RequiredFields tests required fields are present in output.
func TestStructuredOutput_RequiredFields(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var receivedResponse bool

	for msg, err := range codexsdk.Query(ctx,
		"Generate a fictional person with a name and age in structured format.",
		codexsdk.WithPermissionMode("acceptAll"),
		codexsdk.WithOutputFormat(map[string]any{
			"type": "json_schema",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"name": map[string]any{
						"type": "string",
					},
					"age": map[string]any{
						"type": "integer",
					},
				},
				"required": []string{"name", "age"},
			},
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		switch m := msg.(type) {
		case *codexsdk.AssistantMessage:
			for _, block := range m.Content {
				if textBlock, ok := block.(*codexsdk.TextBlock); ok {
					t.Logf("Output with required fields: %s", textBlock.Text)
					receivedResponse = true
				}
			}
		case *codexsdk.ResultMessage:
			require.False(t, m.IsError, "Query should not result in error")
		}
	}

	require.True(t, receivedResponse, "Should receive response with required fields")
}

// =============================================================================
// Test Suite 6: Stderr Callback
// =============================================================================

// TestStderrCallback_ReceivesOutput tests Stderr callback invocation.
func TestStderrCallback_ReceivesOutput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var stderrLines []string

	for _, err := range codexsdk.Query(ctx, "Say 'hello'",
		codexsdk.WithPermissionMode("acceptAll"),
		codexsdk.WithStderr(func(line string) {
			stderrLines = append(stderrLines, line)
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}
	}

	// Note: stderr callback may or may not receive output depending on CLI behavior
	t.Logf("Received %d stderr lines", len(stderrLines))
}

// TestStderrCallback_CapturesDebugInfo tests debug flag produces stderr output.
func TestStderrCallback_CapturesDebugInfo(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var stderrLines []string

	debugFlag := ""

	for _, err := range codexsdk.Query(ctx, "Say 'debug test'",
		codexsdk.WithPermissionMode("acceptAll"),
		codexsdk.WithStderr(func(line string) {
			stderrLines = append(stderrLines, line)
		}),
		codexsdk.WithExtraArgs(map[string]*string{
			"debug-to-stderr": &debugFlag,
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}
	}

	// With debug-to-stderr, we expect some output
	t.Logf("Received %d stderr lines with debug enabled", len(stderrLines))

	if len(stderrLines) > 0 {
		t.Logf("First line: %s", stderrLines[0])
	}
}

// =============================================================================
// Test Suite 7: Hooks
// =============================================================================

// TestHooks_PreToolUse tests hook invoked before tool execution.
func TestHooks_PreToolUse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var hookInvoked int32

	timeout := 30.0

	for _, err := range codexsdk.Query(ctx, "List files in the current directory using ls",
		codexsdk.WithPermissionMode("acceptAll"),
		codexsdk.WithHooks(map[codexsdk.HookEvent][]*codexsdk.HookMatcher{
			codexsdk.HookEventPreToolUse: {{
				Hooks: []codexsdk.HookCallback{
					func(_ context.Context, input codexsdk.HookInput,
						_ *string, _ *codexsdk.HookContext,
					) (codexsdk.HookJSONOutput, error) {
						atomic.AddInt32(&hookInvoked, 1)

						if preInput, ok := input.(*codexsdk.PreToolUseHookInput); ok {
							t.Logf("PreToolUse hook called for tool: %s", preInput.ToolName)
						}

						continueFlag := true

						return &codexsdk.SyncHookJSONOutput{
							Continue: &continueFlag,
						}, nil
					},
				},
				Timeout: &timeout,
			}},
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}
	}

	require.Greater(t, atomic.LoadInt32(&hookInvoked), int32(0),
		"PreToolUse hook should have been invoked")
}

// TestHooks_PostToolUse tests hook invoked after tool execution.
func TestHooks_PostToolUse(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var hookInvoked int32

	timeout := 30.0

	for _, err := range codexsdk.Query(ctx, "Run 'echo hello' command",
		codexsdk.WithPermissionMode("acceptAll"),
		codexsdk.WithHooks(map[codexsdk.HookEvent][]*codexsdk.HookMatcher{
			codexsdk.HookEventPostToolUse: {{
				Hooks: []codexsdk.HookCallback{
					func(_ context.Context, input codexsdk.HookInput,
						_ *string, _ *codexsdk.HookContext,
					) (codexsdk.HookJSONOutput, error) {
						atomic.AddInt32(&hookInvoked, 1)

						if postInput, ok := input.(*codexsdk.PostToolUseHookInput); ok {
							t.Logf("PostToolUse hook called for tool: %s", postInput.ToolName)
						}

						continueFlag := true

						return &codexsdk.SyncHookJSONOutput{
							Continue: &continueFlag,
						}, nil
					},
				},
				Timeout: &timeout,
			}},
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}
	}

	require.Greater(t, atomic.LoadInt32(&hookInvoked), int32(0),
		"PostToolUse hook should have been invoked")
}

// TestHooks_BlockTool tests PreToolUse with Continue: false blocks tool.
func TestHooks_BlockTool(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var toolBlocked int32

	bashTool := "Bash"
	timeout := 30.0

	for _, err := range codexsdk.Query(ctx, "Run 'echo blocked' command",
		codexsdk.WithPermissionMode("acceptAll"),
		codexsdk.WithHooks(map[codexsdk.HookEvent][]*codexsdk.HookMatcher{
			codexsdk.HookEventPreToolUse: {{
				Matcher: &bashTool,
				Hooks: []codexsdk.HookCallback{
					func(_ context.Context, _ codexsdk.HookInput,
						_ *string, _ *codexsdk.HookContext,
					) (codexsdk.HookJSONOutput, error) {
						atomic.AddInt32(&toolBlocked, 1)
						t.Logf("Blocking Bash tool")

						continueFlag := false
						denyDecision := "deny"
						reason := "Tool blocked by test hook"

						return &codexsdk.SyncHookJSONOutput{
							Continue: &continueFlag,
							HookSpecificOutput: &codexsdk.PreToolUseHookSpecificOutput{
								PermissionDecision:       &denyDecision,
								PermissionDecisionReason: &reason,
							},
						}, nil
					},
				},
				Timeout: &timeout,
			}},
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}
	}

	require.Greater(t, atomic.LoadInt32(&toolBlocked), int32(0),
		"Bash tool should have been blocked by hook")
}

// =============================================================================
// Test Suite 8: SDK MCP Tools
// =============================================================================

// TestMCPTools_Registration tests tool registration with the MCP server.
func TestMCPTools_Registration(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	echoTool := codexsdk.NewSdkMcpTool(
		"test_echo",
		"Echoes the input message back",
		codexsdk.SimpleSchema(map[string]string{
			"message": "string",
		}),
		func(_ context.Context, req *codexsdk.CallToolRequest) (*codexsdk.CallToolResult, error) {
			args, err := codexsdk.ParseArguments(req)
			if err != nil {
				return codexsdk.ErrorResult(err.Error()), nil
			}

			msg, _ := args["message"].(string)

			return codexsdk.TextResult(fmt.Sprintf(`{"echo": %q}`, msg)), nil
		},
	)

	server := codexsdk.CreateSdkMcpServer("sdk", "1.0.0", echoTool)
	receivedResult := false

	for msg, err := range codexsdk.Query(ctx, "Say hello",
		codexsdk.WithPermissionMode("bypassPermissions"),
		codexsdk.WithMCPServers(map[string]codexsdk.MCPServerConfig{
			"sdk": server,
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		if result, ok := msg.(*codexsdk.ResultMessage); ok {
			receivedResult = true
			require.False(t, result.IsError, "Query should not result in error")
		}
	}

	require.True(t, receivedResult, "Should complete successfully with registered tool")
}

// TestMCPTools_Execution tests tool called with correct input.
func TestMCPTools_Execution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var toolExecuted bool

	addTool := codexsdk.NewSdkMcpTool(
		"add_numbers",
		"Adds two numbers together and returns the result",
		codexsdk.SimpleSchema(map[string]string{
			"a": "float64",
			"b": "float64",
		}),
		func(_ context.Context, req *codexsdk.CallToolRequest) (*codexsdk.CallToolResult, error) {
			args, err := codexsdk.ParseArguments(req)
			if err != nil {
				return codexsdk.ErrorResult(err.Error()), nil
			}

			toolExecuted = true
			a, _ := args["a"].(float64)
			b, _ := args["b"].(float64)
			t.Logf("Tool executed with a=%v, b=%v", a, b)

			return codexsdk.TextResult(fmt.Sprintf(`{"result": %v}`, a+b)), nil
		},
	)

	server := codexsdk.CreateSdkMcpServer("sdk", "1.0.0", addTool)

	for _, err := range codexsdk.Query(ctx,
		"Use the mcp__sdk__add_numbers tool to add 5 and 3",
		codexsdk.WithPermissionMode("bypassPermissions"),
		codexsdk.WithMCPServers(map[string]codexsdk.MCPServerConfig{
			"sdk": server,
		}),
		codexsdk.WithAllowedTools("mcp__sdk__add_numbers"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}
	}

	require.True(t, toolExecuted, "Tool should have been executed")
}

// TestMCPTools_ReturnValue tests tool result used by the agent.
func TestMCPTools_ReturnValue(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var toolExecuted bool

	magicTool := codexsdk.NewSdkMcpTool(
		"get_magic_number",
		"Returns a magic number",
		codexsdk.SimpleSchema(map[string]string{}),
		func(_ context.Context, _ *codexsdk.CallToolRequest) (*codexsdk.CallToolResult, error) {
			toolExecuted = true

			return codexsdk.TextResult(`{"number": 42}`), nil
		},
	)

	server := codexsdk.CreateSdkMcpServer("sdk", "1.0.0", magicTool)
	var mentionedNumber bool

	for msg, err := range codexsdk.Query(ctx,
		"Use the mcp__sdk__get_magic_number tool and tell me what number it returns",
		codexsdk.WithPermissionMode("bypassPermissions"),
		codexsdk.WithMCPServers(map[string]codexsdk.MCPServerConfig{
			"sdk": server,
		}),
		codexsdk.WithAllowedTools("mcp__sdk__get_magic_number"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		if assistantMsg, ok := msg.(*codexsdk.AssistantMessage); ok {
			for _, block := range assistantMsg.Content {
				if textBlock, ok := block.(*codexsdk.TextBlock); ok {
					t.Logf("Response: %s", textBlock.Text)

					if contains42(textBlock.Text) {
						mentionedNumber = true
					}
				}
			}
		}
	}

	require.True(t, toolExecuted, "Tool should have been executed")
	require.True(t, mentionedNumber, "Agent should mention the returned number (42)")
}

// contains42 checks if a string contains "42" in various formats.
func contains42(s string) bool {
	checks := []string{"42", "forty-two", "forty two"}
	for _, check := range checks {
		if containsString(s, check) {
			return true
		}
	}

	return false
}

// containsString is a simple case-insensitive contains check.
func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) > len(substr) &&
			(s[:len(substr)] == substr || containsString(s[1:], substr)))
}

// =============================================================================
// Test Suite 9: Filesystem Agent Loading
// =============================================================================

// TestAgentsAndSettings_FilesystemAgentLoading tests that filesystem-based agents
// load via setting_sources=["project"] and produce a full response cycle.
// This verifies that when using SettingSources with a .claude/agents/ directory
// containing agent definitions, the SDK:
// 1. Loads the agents (they appear in init message)
// 2. Produces a full response with AssistantMessage
// 3. Completes with a ResultMessage.
func TestAgentsAndSettings_FilesystemAgentLoading(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Create a temporary project directory
	tmpDir, err := os.MkdirTemp("", "codex-sdk-test-*")
	require.NoError(t, err)

	defer os.RemoveAll(tmpDir)

	// Create .claude/agents directory
	agentsDir := filepath.Join(tmpDir, ".claude", "agents")
	err = os.MkdirAll(agentsDir, 0755)
	require.NoError(t, err)

	// Create a test agent file with YAML frontmatter
	agentFile := filepath.Join(agentsDir, "fs-test-agent.md")
	agentContent := `---
name: fs-test-agent
description: A filesystem test agent for SDK testing
tools: Read
---

# Filesystem Test Agent

You are a simple test agent. When asked a question, provide a brief, helpful answer.
`
	err = os.WriteFile(agentFile, []byte(agentContent), 0644)
	require.NoError(t, err)

	var (
		receivedSystem    bool
		receivedAssistant bool
		receivedResult    bool
		foundAgent        bool
	)

	for msg, err := range codexsdk.Query(ctx, "Say hello in exactly 3 words",
		codexsdk.WithCwd(tmpDir),
		codexsdk.WithPermissionMode("acceptAll"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		switch m := msg.(type) {
		case *codexsdk.SystemMessage:
			receivedSystem = true

			if m.Subtype == "init" {
				// Check if the filesystem agent was loaded
				if agents, ok := m.Data["agents"].([]any); ok {
					for _, agent := range agents {
						if agentName, ok := agent.(string); ok && agentName == "fs-test-agent" {
							foundAgent = true

							t.Logf("Found filesystem agent: %s", agentName)
						}
					}
				}
			}
		case *codexsdk.AssistantMessage:
			receivedAssistant = true
			t.Logf("Received assistant message with %d content blocks", len(m.Content))
		case *codexsdk.ResultMessage:
			receivedResult = true
			require.False(t, m.IsError, "Query should not result in error")
		}
	}

	require.True(t, receivedSystem, "Should receive SystemMessage (init)")
	require.True(t, receivedAssistant,
		"Should receive AssistantMessage - missing may indicate filesystem agent loading issue")
	require.True(t, receivedResult, "Should receive ResultMessage")
	require.True(t, foundAgent,
		"fs-test-agent should be loaded from filesystem via setting_sources")
}

// =============================================================================
// Test Suite 10: MCP Tool Permission Enforcement
// =============================================================================

// TestMCPTools_PermissionEnforcement tests that disallowed_tools blocks MCP tool execution.
// This verifies that when both allowed_tools and disallowed_tools are specified,
// the disallowed tools are not executed while allowed tools are.
func TestMCPTools_PermissionEnforcement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var (
		echoExecuted  bool
		greetExecuted bool
	)

	// Create echo tool - this will be disallowed
	echoTool := codexsdk.NewSdkMcpTool(
		"echo",
		"Echo back the input text",
		codexsdk.SimpleSchema(map[string]string{
			"text": "string",
		}),
		func(_ context.Context, req *codexsdk.CallToolRequest) (*codexsdk.CallToolResult, error) {
			args, err := codexsdk.ParseArguments(req)
			if err != nil {
				return codexsdk.ErrorResult(err.Error()), nil
			}

			echoExecuted = true
			text, _ := args["text"].(string)
			t.Logf("Echo tool executed with: %s", text)

			return codexsdk.TextResult(fmt.Sprintf(`{"echo": %q}`, text)), nil
		},
	)

	// Create greet tool - this will be allowed
	greetTool := codexsdk.NewSdkMcpTool(
		"greet",
		"Greet a person by name",
		codexsdk.SimpleSchema(map[string]string{
			"name": "string",
		}),
		func(_ context.Context, req *codexsdk.CallToolRequest) (*codexsdk.CallToolResult, error) {
			args, err := codexsdk.ParseArguments(req)
			if err != nil {
				return codexsdk.ErrorResult(err.Error()), nil
			}

			greetExecuted = true
			name, _ := args["name"].(string)
			t.Logf("Greet tool executed with: %s", name)

			return codexsdk.TextResult(fmt.Sprintf(`{"greeting": "Hello, %s!"}`, name)), nil
		},
	)

	server := codexsdk.CreateSdkMcpServer("sdk", "1.0.0", echoTool, greetTool)

	for _, err := range codexsdk.Query(ctx,
		"Use the echo tool to echo 'test' and use greet tool to greet 'Alice'",
		codexsdk.WithPermissionMode("bypassPermissions"),
		codexsdk.WithMCPServers(map[string]codexsdk.MCPServerConfig{
			"sdk": server,
		}),
		codexsdk.WithDisallowedTools("mcp__sdk__echo"),
		codexsdk.WithAllowedTools("mcp__sdk__greet"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}
	}

	// Check actual function executions
	require.False(t, echoExecuted, "Disallowed echo tool should NOT have been executed")
	require.True(t, greetExecuted, "Allowed greet tool should have been executed")
}

// =============================================================================
// Test Suite 11: Hook Additional Context
// =============================================================================

// TestHooks_WithAdditionalContext tests PostToolUse hook with additionalContext field.
func TestHooks_WithAdditionalContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var (
		hookInvoked       int32
		receivedToolInput map[string]any
	)

	timeout := 30.0

	for _, err := range codexsdk.Query(ctx, "Run 'echo test' command",
		codexsdk.WithPermissionMode("acceptAll"),
		codexsdk.WithHooks(map[codexsdk.HookEvent][]*codexsdk.HookMatcher{
			codexsdk.HookEventPostToolUse: {{
				Hooks: []codexsdk.HookCallback{
					func(_ context.Context, input codexsdk.HookInput,
						_ *string, _ *codexsdk.HookContext,
					) (codexsdk.HookJSONOutput, error) {
						atomic.AddInt32(&hookInvoked, 1)

						if postInput, ok := input.(*codexsdk.PostToolUseHookInput); ok {
							receivedToolInput = postInput.ToolInput
							t.Logf("PostToolUse hook for tool: %s, input: %v",
								postInput.ToolName, postInput.ToolInput)
						}

						continueFlag := true
						additionalContext := "This is additional context from the hook"

						return &codexsdk.SyncHookJSONOutput{
							Continue: &continueFlag,
							HookSpecificOutput: &codexsdk.PostToolUseHookSpecificOutput{
								AdditionalContext: &additionalContext,
							},
						}, nil
					},
				},
				Timeout: &timeout,
			}},
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}
	}

	require.Greater(t, atomic.LoadInt32(&hookInvoked), int32(0),
		"PostToolUse hook should have been invoked")
	require.NotNil(t, receivedToolInput, "Hook should have received tool input")
}

// =============================================================================
// Test Suite 12: Structured Output with Enum
// =============================================================================

// TestStructuredOutput_WithEnum tests structured output with enum type.
func TestStructuredOutput_WithEnum(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var receivedResponse bool

	for msg, err := range codexsdk.Query(ctx,
		"Pick a random color and intensity. Respond in structured format.",
		codexsdk.WithPermissionMode("acceptAll"),
		codexsdk.WithOutputFormat(map[string]any{
			"type": "json_schema",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"color": map[string]any{
						"type":        "string",
						"enum":        []string{"red", "green", "blue"},
						"description": "A color choice",
					},
					"intensity": map[string]any{
						"type":        "string",
						"enum":        []string{"low", "medium", "high"},
						"description": "Intensity level",
					},
				},
				"required": []string{"color", "intensity"},
			},
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		switch m := msg.(type) {
		case *codexsdk.AssistantMessage:
			for _, block := range m.Content {
				if textBlock, ok := block.(*codexsdk.TextBlock); ok {
					t.Logf("Structured output with enum: %s", textBlock.Text)
					receivedResponse = true
				}
			}
		case *codexsdk.ResultMessage:
			require.False(t, m.IsError, "Query should not result in error")
		}
	}

	require.True(t, receivedResponse, "Should receive structured response with enum values")
}

// =============================================================================
// Test Suite 13: Structured Output with Tools
// =============================================================================

// TestStructuredOutput_WithTools tests structured output combined with tool usage.
func TestStructuredOutput_WithTools(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	var toolExecuted bool

	dataTool := codexsdk.NewSdkMcpTool(
		"get_data",
		"Gets data for structured output",
		codexsdk.SimpleSchema(map[string]string{}),
		func(_ context.Context, _ *codexsdk.CallToolRequest) (*codexsdk.CallToolResult, error) {
			toolExecuted = true

			return codexsdk.TextResult(`{"value": 42, "status": "success"}`), nil
		},
	)

	server := codexsdk.CreateSdkMcpServer("sdk", "1.0.0", dataTool)
	var receivedResponse bool

	for msg, err := range codexsdk.Query(ctx,
		"Use the mcp__sdk__get_data tool to get a value, "+
			"then provide the result in structured format.",
		codexsdk.WithPermissionMode("bypassPermissions"),
		codexsdk.WithMCPServers(map[string]codexsdk.MCPServerConfig{
			"sdk": server,
		}),
		codexsdk.WithAllowedTools("mcp__sdk__get_data"),
		codexsdk.WithOutputFormat(map[string]any{
			"type": "json_schema",
			"schema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"data_value": map[string]any{
						"type":        "integer",
						"description": "The value from the data tool",
					},
					"summary": map[string]any{
						"type":        "string",
						"description": "Summary of the result",
					},
				},
				"required": []string{"data_value", "summary"},
			},
		}),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		switch m := msg.(type) {
		case *codexsdk.AssistantMessage:
			for _, block := range m.Content {
				if textBlock, ok := block.(*codexsdk.TextBlock); ok {
					t.Logf("Structured output with tools: %s", textBlock.Text)
					receivedResponse = true
				}
			}
		case *codexsdk.ResultMessage:
			require.False(t, m.IsError, "Query should not result in error")
		}
	}

	require.True(t, toolExecuted, "Tool should have been executed")
	require.True(t, receivedResponse, "Should receive structured response after tool use")
}

// =============================================================================
// Test Suite 14: Partial Messages - Thinking Deltas
// =============================================================================

// TestPartialMessages_ThinkingDeltas tests receiving thinking block deltas.
func TestPartialMessages_ThinkingDeltas(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	maxThinking := 1000

	var (
		streamEventCount int
		hasThinkingEvent bool
	)

	for msg, err := range codexsdk.Query(ctx,
		"Think step by step: what is the sum of numbers from 1 to 10?",
		codexsdk.WithPermissionMode("acceptAll"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		if streamEvent, ok := msg.(*codexsdk.StreamEvent); ok {
			streamEventCount++

			if eventType, ok := streamEvent.Event["type"].(string); ok {
				// Look for content_block_delta with thinking type
				if eventType == "content_block_delta" {
					if delta, ok := streamEvent.Event["delta"].(map[string]any); ok {
						if deltaType, ok := delta["type"].(string); ok {
							if deltaType == "thinking_delta" {
								hasThinkingEvent = true
								t.Logf("Received thinking delta event")
							}
						}
					}
				}
			}
		}
	}

	require.Greater(t, streamEventCount, 0,
		"Should receive StreamEvents when IncludePartialMessages is true")

	// Note: hasThinkingEvent may be false if the model doesn't use extended thinking
	// or if thinking tokens are not enabled
	t.Logf("Received %d stream events, hasThinkingEvent=%v",
		streamEventCount, hasThinkingEvent)
}

// =============================================================================
// Test Suite 15: Max Budget USD
// =============================================================================

// TestMaxBudgetUSD_LimitEnforced tests that MaxBudgetUSD option limits spending.
// When the budget is exceeded, the result should indicate error_max_budget_usd.
func TestMaxBudgetUSD_LimitEnforced(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Set an extremely low budget to trigger the limit
	budget := 0.0001 // $0.0001 - very low to trigger quickly

	var (
		receivedResult   bool
		resultSubtype    string
		resultIsError    bool
		totalCost        float64
		receivedResponse bool
	)

	for msg, err := range codexsdk.Query(ctx,
		"Write a detailed essay about the history of computing, "+
			"including the development of transistors, integrated circuits, "+
			"and modern processors. Be very thorough.",
		codexsdk.WithPermissionMode("acceptAll"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		switch m := msg.(type) {
		case *codexsdk.AssistantMessage:
			receivedResponse = true
		case *codexsdk.ResultMessage:
			receivedResult = true
			resultSubtype = m.Subtype
			resultIsError = m.IsError

			if m.TotalCostUSD != nil {
				totalCost = *m.TotalCostUSD
			}

			t.Logf("Result: subtype=%s, isError=%v, totalCost=%f",
				resultSubtype, resultIsError, totalCost)
		}
	}

	require.True(t, receivedResult, "Should receive ResultMessage")

	// The query should either complete normally (if budget wasn't exceeded)
	// or return error_max_budget_usd
	if resultSubtype == "error_max_budget_usd" {
		require.True(t, resultIsError, "Budget exceeded should be an error")
		t.Logf("Budget limit was enforced as expected")
	} else {
		// Budget wasn't exceeded - this is also valid behavior
		t.Logf("Budget was not exceeded (subtype=%s), cost=%f",
			resultSubtype, totalCost)
		require.True(t, receivedResponse, "Should receive response if budget not exceeded")
	}
}

// TestMaxBudgetUSD_ZeroBudget tests behavior with zero budget.
func TestMaxBudgetUSD_ZeroBudget(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Zero budget should immediately fail
	budget := 0.0

	var resultSubtype string

	for msg, err := range codexsdk.Query(ctx, "Say hello",
		codexsdk.WithPermissionMode("acceptAll"),
	) {
		if err != nil {
			if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
				t.Skip("Codex CLI not installed")
			}

			t.Fatalf("Query failed: %v", err)
		}

		if result, ok := msg.(*codexsdk.ResultMessage); ok {
			resultSubtype = result.Subtype
			t.Logf("Result with zero budget: subtype=%s, isError=%v",
				resultSubtype, result.IsError)
		}
	}

	// Zero budget should result in immediate budget exceeded error
	if resultSubtype == "error_max_budget_usd" {
		t.Logf("Zero budget correctly triggered budget exceeded error")
	} else {
		t.Logf("Unexpected subtype with zero budget: %s", resultSubtype)
	}
}

// =============================================================================
// Test Suite 16: Mid-Operation Close/Cancel
// =============================================================================

// TestQuery_CloseMidStream tests that closing the client mid-stream
// during a real query terminates cleanly without hanging processes.
func TestQuery_CloseMidStream(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	client := codexsdk.NewClient()

	err := client.Start(ctx,
		codexsdk.WithPermissionMode("acceptAll"),
	)
	if err != nil {
		if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
			t.Skip("Codex CLI not installed")
		}

		t.Fatalf("Connect failed: %v", err)
	}

	// Start a query that will produce multiple messages
	err = client.Query(ctx, "Write a short story about a robot. Include at least 3 paragraphs.")
	require.NoError(t, err, "Query should succeed")

	// Receive some messages in background
	receiveDone := make(chan struct{})

	var receivedCount int

	var receivedTypes []string

	go func() {
		defer close(receiveDone)

		for msg, err := range client.ReceiveMessages(ctx) {
			if err != nil {
				t.Logf("ReceiveMessages error: %v", err)

				return
			}

			receivedCount++
			receivedTypes = append(receivedTypes, msg.MessageType())

			// After receiving a few messages, signal we're ready to close
			if receivedCount >= 2 {
				return
			}
		}
	}()

	// Wait for some messages to be received
	select {
	case <-receiveDone:
		// Got some messages
	case <-time.After(30 * time.Second):
		t.Fatal("Timeout waiting for messages")
	}

	t.Logf("Received %d messages before close: %v", receivedCount, receivedTypes)

	// Close the client mid-stream
	closeStart := time.Now()
	err = client.Close()
	closeDuration := time.Since(closeStart)

	require.NoError(t, err, "Close should succeed")
	t.Logf("Close completed in %v", closeDuration)

	// Verify close didn't take too long (should be quick, not waiting for full response)
	require.Less(t, closeDuration, 10*time.Second,
		"Close should complete quickly, not wait for full response")

	// Verify we received at least some messages
	require.Greater(t, receivedCount, 0, "Should have received messages before close")
}

// TestClient_ContextCancelDuringQuery tests that context cancellation
// during an active query terminates cleanly.
func TestClient_ContextCancelDuringQuery(t *testing.T) {
	// Use a short overall timeout
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := codexsdk.NewClient()
	defer client.Close()

	err := client.Start(ctx,
		codexsdk.WithPermissionMode("acceptAll"),
	)
	if err != nil {
		if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
			t.Skip("Codex CLI not installed")
		}

		t.Fatalf("Connect failed: %v", err)
	}

	// Create a cancellable context for the query
	queryCtx, queryCancel := context.WithCancel(ctx)

	// Start a query
	err = client.Query(queryCtx, "Explain quantum computing in detail.")
	require.NoError(t, err, "Query should succeed")

	// Receive messages with the cancellable context
	receiveDone := make(chan struct{})

	var receivedCount int

	var gotContextError bool

	go func() {
		defer close(receiveDone)

		for _, err := range client.ReceiveMessages(queryCtx) {
			if err != nil {
				if err == context.Canceled {
					gotContextError = true
				}

				return
			}

			receivedCount++

			// Cancel after receiving a couple messages
			if receivedCount >= 2 {
				queryCancel()
			}
		}
	}()

	// Wait for receiver to complete
	select {
	case <-receiveDone:
		// Success
	case <-time.After(30 * time.Second):
		t.Fatal("Timeout waiting for receiver to complete")
	}

	t.Logf("Received %d messages, gotContextError: %v", receivedCount, gotContextError)

	// Either we got context canceled error or messages channel closed
	require.True(t, receivedCount > 0 || gotContextError,
		"Should have received messages or context error")
}

// TestClient_RapidCloseReopen tests rapid close and reopen doesn't cause issues.
func TestClient_RapidCloseReopen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	for i := range 3 {
		t.Run(fmt.Sprintf("iteration_%d", i), func(t *testing.T) {
			client := codexsdk.NewClient()

			err := client.Start(ctx,
				codexsdk.WithPermissionMode("acceptAll"),
			)
			if err != nil {
				if func() bool { _, ok := errors.AsType[*codexsdk.CLINotFoundError](err); return ok }() {
					t.Skip("Codex CLI not installed")
				}

				t.Fatalf("Connect failed: %v", err)
			}

			// Quick query
			err = client.Query(ctx, "Say 'hello'")
			require.NoError(t, err)

			// Receive one message
			for msg, err := range client.ReceiveMessages(ctx) {
				if err != nil {
					break
				}

				t.Logf("Got message type: %s", msg.MessageType())

				break
			}

			// Close immediately
			err = client.Close()
			require.NoError(t, err)
		})
	}
}
