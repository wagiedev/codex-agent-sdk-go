package protocol

import (
	"context"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/wagiedev/codex-agent-sdk-go/internal/config"
	"github.com/wagiedev/codex-agent-sdk-go/internal/hook"
	"github.com/wagiedev/codex-agent-sdk-go/internal/mcp"
	"github.com/wagiedev/codex-agent-sdk-go/internal/permission"
)

const (
	// defaultInitializeTimeout is the default timeout for initialize control requests.
	defaultInitializeTimeout = 60 * time.Second
)

// Session encapsulates protocol handling logic for hooks, MCP servers, and callbacks.
type Session struct {
	log        *slog.Logger
	controller *Controller
	options    *config.Options

	hookCallbacks   map[string]hook.Callback
	nextCallbackID  int
	hookCallbacksMu sync.RWMutex

	sdkMcpServers map[string]mcp.ServerInstance

	initMu               sync.RWMutex
	initializationResult map[string]any
}

// NewSession creates a new Session for protocol handling.
func NewSession(
	log *slog.Logger,
	controller *Controller,
	options *config.Options,
) *Session {
	return &Session{
		log:           log.With("component", "session"),
		controller:    controller,
		options:       options,
		hookCallbacks: make(map[string]hook.Callback, 16),
		sdkMcpServers: make(map[string]mcp.ServerInstance, 4),
	}
}

// RegisterHandlers registers protocol handlers for hooks, MCP, and tool permissions.
func (s *Session) RegisterHandlers() {
	s.controller.RegisterHandler("hook_callback", s.HandleHookCallback)
	s.controller.RegisterHandler("hooks_callback", s.HandleHookCallback)
	s.controller.RegisterHandler("mcp_message", s.HandleMCPMessage)
	s.controller.RegisterHandler("can_use_tool", s.HandleCanUseTool)
	s.controller.RegisterHandler("item_commandExecution/requestApproval", s.HandleCanUseTool)
	s.controller.RegisterHandler("item_commandExecution_requestApproval", s.HandleCanUseTool)
}

// RegisterMCPServers extracts and registers SDK MCP servers from options.
func (s *Session) RegisterMCPServers() {
	if s.options == nil || s.options.MCPServers == nil {
		return
	}

	for serverKey, serverConfig := range s.options.MCPServers {
		if serverConfig == nil {
			continue
		}

		sdkConfig, ok := serverConfig.(*mcp.SdkServerConfig)
		if !ok {
			continue
		}

		if sdkConfig.Instance == nil {
			continue
		}

		server, ok := sdkConfig.Instance.(mcp.ServerInstance)
		if !ok {
			continue
		}

		s.sdkMcpServers[serverKey] = server
		s.log.Debug("registered SDK MCP server", slog.String("server", serverKey))
	}
}

// Initialize sends the initialization control request to the CLI.
func (s *Session) Initialize(ctx context.Context) error {
	s.log.DebugContext(ctx, "sending initialize request")

	hooksConfig := make(map[string]any, 8)

	if s.options != nil && s.options.Hooks != nil {
		s.hookCallbacksMu.Lock()

		for event, matchers := range s.options.Hooks {
			eventMatchers := make([]map[string]any, 0, len(matchers))

			for _, m := range matchers {
				callbackIDs := make([]string, 0, len(m.Hooks))

				for _, hookFn := range m.Hooks {
					callbackID := fmt.Sprintf("hook_%d", s.nextCallbackID)
					s.nextCallbackID++
					s.hookCallbacks[callbackID] = hookFn
					callbackIDs = append(callbackIDs, callbackID)
				}

				matcherConfig := map[string]any{
					"matcher":         m.Matcher,
					"hookCallbackIds": callbackIDs,
				}

				if m.Timeout != nil {
					matcherConfig["timeout"] = *m.Timeout
				}

				eventMatchers = append(eventMatchers, matcherConfig)
			}

			hooksConfig[string(event)] = eventMatchers
		}

		s.hookCallbacksMu.Unlock()
	}

	payload := s.buildInitializePayload(hooksConfig)

	timeout := s.getInitializeTimeout()

	resp, err := s.controller.SendRequest(ctx, "initialize", payload, timeout)
	if err != nil {
		return fmt.Errorf("initialize: %w", err)
	}

	s.initMu.Lock()
	s.initializationResult = resp.Payload()
	s.initMu.Unlock()

	return nil
}

// buildInitializePayload builds thread/start initialization payload from options.
func (s *Session) buildInitializePayload(hooksConfig map[string]any) map[string]any {
	payload := map[string]any{
		"hooks": hooksConfig,
	}

	if s.options == nil {
		return payload
	}

	if s.options.Model != "" {
		payload["model"] = s.options.Model
	}

	if s.options.Cwd != "" {
		payload["cwd"] = s.options.Cwd
	}

	if s.options.SystemPromptPreset != nil {
		payload["systemPromptPreset"] = s.options.SystemPromptPreset
	} else if s.options.SystemPrompt != "" {
		payload["systemPrompt"] = s.options.SystemPrompt
	}

	if s.options.ContinueConversation {
		payload["continueConversation"] = true
	}

	if s.options.Resume != "" {
		payload["resume"] = s.options.Resume
	}

	if s.options.ForkSession {
		payload["forkSession"] = true
	}

	if s.options.Effort != nil {
		payload["reasoningEffort"] = string(*s.options.Effort)
	}

	sandboxMode := s.options.Sandbox
	if sandboxMode == "" {
		sandboxMode = mapPermissionToSandbox(s.options.PermissionMode)
	}

	if sandboxMode != "" {
		payload["sandbox"] = sandboxMode
	}

	if approvalPolicy := mapPermissionToApprovalPolicy(s.options.PermissionMode); approvalPolicy != "" {
		payload["approvalPolicy"] = approvalPolicy
	}

	if len(s.options.AllowedTools) > 0 {
		payload["allowedTools"] = s.options.AllowedTools
	}

	if len(s.options.DisallowedTools) > 0 {
		payload["disallowedTools"] = s.options.DisallowedTools
	}

	switch t := s.options.Tools.(type) {
	case config.ToolsList:
		payload["tools"] = t
	case *config.ToolsPreset:
		payload["tools"] = t
	}

	if len(s.options.AddDirs) > 0 {
		payload["addDirs"] = s.options.AddDirs
	}

	if len(s.options.Config) > 0 {
		cfg := make(map[string]any, len(s.options.Config))
		for k, v := range s.options.Config {
			cfg[k] = v
		}

		payload["config"] = cfg
	}

	if s.options.OutputSchema != "" {
		payload["outputSchema"] = s.options.OutputSchema
	} else if schema := extractOutputSchema(s.options.OutputFormat); schema != nil {
		payload["outputSchema"] = schema
	}

	if s.options.PermissionPromptToolName != "" {
		payload["permissionPromptToolName"] = s.options.PermissionPromptToolName
	}

	return payload
}

func extractOutputSchema(outputFormat map[string]any) map[string]any {
	if outputFormat == nil {
		return nil
	}

	formatType, _ := outputFormat["type"].(string)
	if formatType == "json_schema" {
		if schema, ok := outputFormat["schema"].(map[string]any); ok {
			return schema
		}

		return nil
	}

	if _, ok := outputFormat["properties"]; ok {
		return outputFormat
	}

	return nil
}

func mapPermissionToSandbox(permMode string) string {
	switch permMode {
	case "acceptEdits":
		return "workspace-write"
	case "bypassPermissions", "acceptAll":
		return "danger-full-access"
	default:
		return ""
	}
}

func mapPermissionToApprovalPolicy(permMode string) string {
	switch permMode {
	case "bypassPermissions", "acceptAll":
		return "never"
	case "default", "acceptEdits", "":
		return "on-request"
	default:
		return ""
	}
}

// getInitializeTimeout returns the initialize timeout.
func (s *Session) getInitializeTimeout() time.Duration {
	if s.options != nil && s.options.InitializeTimeout != nil {
		return *s.options.InitializeTimeout
	}

	if timeoutStr := os.Getenv("CODEX_CODE_STREAM_CLOSE_TIMEOUT"); timeoutStr != "" {
		if timeoutSec, err := strconv.Atoi(timeoutStr); err == nil && timeoutSec > 0 {
			return time.Duration(timeoutSec) * time.Second
		}
	}

	return defaultInitializeTimeout
}

// NeedsInitialization returns true if the session has callbacks that require initialization.
func (s *Session) NeedsInitialization() bool {
	if s.options == nil {
		return false
	}

	return len(s.options.Hooks) > 0 ||
		s.options.CanUseTool != nil ||
		len(s.sdkMcpServers) > 0
}

// GetInitializationResult returns a copy of the server initialization info.
func (s *Session) GetInitializationResult() map[string]any {
	s.initMu.RLock()
	defer s.initMu.RUnlock()

	if s.initializationResult == nil {
		return nil
	}

	return maps.Clone(s.initializationResult)
}

// GetSDKMCPServerNames returns the names of all registered SDK MCP servers.
func (s *Session) GetSDKMCPServerNames() []string {
	names := make([]string, 0, len(s.sdkMcpServers))
	for name := range s.sdkMcpServers {
		names = append(names, name)
	}

	return names
}

// HandleHookCallback handles hook_callback control requests from the CLI.
func (s *Session) HandleHookCallback(
	ctx context.Context,
	req *ControlRequest,
) (map[string]any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	callbackID, _ := req.Request["callback_id"].(string)
	inputData, _ := req.Request["input"].(map[string]any)

	var toolUseID *string
	if toolUseIDStr, ok := req.Request["tool_use_id"].(string); ok && toolUseIDStr != "" {
		toolUseID = &toolUseIDStr
	}

	s.hookCallbacksMu.RLock()
	callback, exists := s.hookCallbacks[callbackID]
	s.hookCallbacksMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("unknown callback_id: %s", callbackID)
	}

	hookInput, err := s.parseHookInput(inputData)
	if err != nil {
		return nil, fmt.Errorf("parse hook input: %w", err)
	}

	hookCtx := &hook.Context{}

	output, err := callback(ctx, hookInput, toolUseID, hookCtx)
	if err != nil {
		return nil, fmt.Errorf("hook callback error: %w", err)
	}

	return s.convertHookOutput(output)
}

// parseHookInput converts the input map to the appropriate HookInput type.
func (s *Session) parseHookInput(inputData map[string]any) (hook.Input, error) {
	if inputData == nil {
		return nil, fmt.Errorf("input data is nil")
	}

	hookEventName, _ := inputData["hook_event_name"].(string)
	sessionID, _ := inputData["session_id"].(string)
	transcriptPath, _ := inputData["transcript_path"].(string)
	cwd, _ := inputData["cwd"].(string)

	var permissionMode *string
	if pm, ok := inputData["permission_mode"].(string); ok {
		permissionMode = &pm
	}

	baseInput := hook.BaseInput{
		SessionID:      sessionID,
		TranscriptPath: transcriptPath,
		Cwd:            cwd,
		PermissionMode: permissionMode,
	}

	switch hookEventName {
	case string(hook.EventPreToolUse):
		toolName, _ := inputData["tool_name"].(string)
		toolInput, _ := inputData["tool_input"].(map[string]any)
		toolUseID, _ := inputData["tool_use_id"].(string)

		return &hook.PreToolUseInput{
			BaseInput:     baseInput,
			HookEventName: hookEventName,
			ToolName:      toolName,
			ToolInput:     toolInput,
			ToolUseID:     toolUseID,
		}, nil

	case string(hook.EventPostToolUse):
		toolName, _ := inputData["tool_name"].(string)
		toolInput, _ := inputData["tool_input"].(map[string]any)
		toolUseID, _ := inputData["tool_use_id"].(string)
		toolResponse := inputData["tool_response"]

		return &hook.PostToolUseInput{
			BaseInput:     baseInput,
			HookEventName: hookEventName,
			ToolName:      toolName,
			ToolInput:     toolInput,
			ToolUseID:     toolUseID,
			ToolResponse:  toolResponse,
		}, nil

	case string(hook.EventUserPromptSubmit):
		prompt, _ := inputData["prompt"].(string)

		return &hook.UserPromptSubmitInput{
			BaseInput:     baseInput,
			HookEventName: hookEventName,
			Prompt:        prompt,
		}, nil

	case string(hook.EventStop):
		stopHookActive, _ := inputData["stop_hook_active"].(bool)

		return &hook.StopInput{
			BaseInput:      baseInput,
			HookEventName:  hookEventName,
			StopHookActive: stopHookActive,
		}, nil

	case string(hook.EventSubagentStop):
		stopHookActive, _ := inputData["stop_hook_active"].(bool)
		agentID, _ := inputData["agent_id"].(string)
		agentTranscriptPath, _ := inputData["agent_transcript_path"].(string)
		agentType, _ := inputData["agent_type"].(string)

		return &hook.SubagentStopInput{
			BaseInput:           baseInput,
			HookEventName:       hookEventName,
			StopHookActive:      stopHookActive,
			AgentID:             agentID,
			AgentTranscriptPath: agentTranscriptPath,
			AgentType:           agentType,
		}, nil

	case string(hook.EventPreCompact):
		trigger, _ := inputData["trigger"].(string)

		var customInstructions *string
		if ci, ok := inputData["custom_instructions"].(string); ok && ci != "" {
			customInstructions = &ci
		}

		return &hook.PreCompactInput{
			BaseInput:          baseInput,
			HookEventName:      hookEventName,
			Trigger:            trigger,
			CustomInstructions: customInstructions,
		}, nil

	case string(hook.EventPostToolUseFailure):
		toolName, _ := inputData["tool_name"].(string)
		toolInput, _ := inputData["tool_input"].(map[string]any)
		toolUseID, _ := inputData["tool_use_id"].(string)
		toolError, _ := inputData["error"].(string)

		var isInterrupt *bool
		if v, ok := inputData["is_interrupt"].(bool); ok {
			isInterrupt = &v
		}

		return &hook.PostToolUseFailureInput{
			BaseInput:     baseInput,
			HookEventName: hookEventName,
			ToolName:      toolName,
			ToolInput:     toolInput,
			ToolUseID:     toolUseID,
			Error:         toolError,
			IsInterrupt:   isInterrupt,
		}, nil

	case string(hook.EventNotification):
		msg, _ := inputData["message"].(string)
		notificationType, _ := inputData["notification_type"].(string)

		var title *string
		if t, ok := inputData["title"].(string); ok && t != "" {
			title = &t
		}

		return &hook.NotificationInput{
			BaseInput:        baseInput,
			HookEventName:    hookEventName,
			Message:          msg,
			Title:            title,
			NotificationType: notificationType,
		}, nil

	case string(hook.EventSubagentStart):
		agentID, _ := inputData["agent_id"].(string)
		agentType, _ := inputData["agent_type"].(string)

		return &hook.SubagentStartInput{
			BaseInput:     baseInput,
			HookEventName: hookEventName,
			AgentID:       agentID,
			AgentType:     agentType,
		}, nil

	case string(hook.EventPermissionRequest):
		toolName, _ := inputData["tool_name"].(string)
		toolInput, _ := inputData["tool_input"].(map[string]any)

		var permissionSuggestions []any
		if ps, ok := inputData["permission_suggestions"].([]any); ok {
			permissionSuggestions = ps
		}

		return &hook.PermissionRequestInput{
			BaseInput:             baseInput,
			HookEventName:         hookEventName,
			ToolName:              toolName,
			ToolInput:             toolInput,
			PermissionSuggestions: permissionSuggestions,
		}, nil

	default:
		// Unknown event type - return a generic stop input as fallback
		return &hook.StopInput{
			BaseInput:      baseInput,
			HookEventName:  hookEventName,
			StopHookActive: false,
		}, nil
	}
}

// convertHookOutput converts a HookJSONOutput to a response map.
func (s *Session) convertHookOutput(output hook.JSONOutput) (map[string]any, error) {
	if output == nil {
		return map[string]any{"continue": true}, nil
	}

	switch o := output.(type) {
	case *hook.SyncJSONOutput:
		result := make(map[string]any, 8)

		if o.Continue != nil {
			result["continue"] = *o.Continue
		} else {
			result["continue"] = true
		}

		if o.SuppressOutput != nil {
			result["suppressOutput"] = *o.SuppressOutput
		}

		if o.StopReason != nil {
			result["stopReason"] = *o.StopReason
		}

		if o.Decision != nil {
			result["decision"] = *o.Decision
		}

		if o.SystemMessage != nil {
			result["systemMessage"] = *o.SystemMessage
		}

		if o.Reason != nil {
			result["reason"] = *o.Reason
		}

		if o.HookSpecificOutput != nil {
			result["hookSpecificOutput"] = o.HookSpecificOutput
		}

		return result, nil

	case *hook.AsyncJSONOutput:
		return map[string]any{
			"async":        o.Async,
			"asyncTimeout": o.AsyncTimeout,
		}, nil

	default:
		return map[string]any{"continue": true}, nil
	}
}

// HandleMCPMessage handles unified mcp_message control requests from the CLI.
func (s *Session) HandleMCPMessage(
	ctx context.Context,
	req *ControlRequest,
) (map[string]any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	serverName, _ := req.Request["server_name"].(string)
	message, _ := req.Request["message"].(map[string]any)

	if message == nil {
		return nil, fmt.Errorf("missing message field in mcp_message request")
	}

	// Extract JSONRPC fields
	jsonrpcVersion, _ := message["jsonrpc"].(string)
	method, _ := message["method"].(string)

	// Get message ID (can be string or number)
	var msgID any
	if id, ok := message["id"].(float64); ok {
		msgID = int(id)
	} else if id, ok := message["id"].(string); ok {
		msgID = id
	} else {
		msgID = message["id"]
	}

	server, exists := s.sdkMcpServers[serverName]
	if !exists {
		return s.mcpErrorResponse(msgID, -32600, fmt.Sprintf("MCP server not found: %s", serverName)), nil
	}

	// Route based on method
	params, _ := message["params"].(map[string]any)

	switch method {
	case "initialize":
		return s.handleMCPInitialize(msgID, jsonrpcVersion, server)

	case "notifications/initialized":
		// No-op acknowledgment - return empty success
		return map[string]any{
			"mcp_response": map[string]any{
				"jsonrpc": "2.0",
				"id":      msgID,
				"result":  map[string]any{},
			},
		}, nil

	case "tools/list":
		return s.handleMCPToolsList(msgID, server)

	case "tools/call":
		return s.handleMCPToolsCall(ctx, msgID, params, server)

	default:
		return s.mcpErrorResponse(msgID, -32601, fmt.Sprintf("Method not found: %s", method)), nil
	}
}

// handleMCPInitialize handles the initialize method.
func (s *Session) handleMCPInitialize(
	msgID any,
	_ string,
	server mcp.ServerInstance,
) (map[string]any, error) {
	var serverInfo map[string]any

	var capabilities map[string]any

	if infoProvider, ok := server.(interface{ ServerInfo() map[string]any }); ok {
		serverInfo = infoProvider.ServerInfo()
	} else {
		serverInfo = map[string]any{
			"name":    "sdk-server",
			"version": "1.0.0",
		}
	}

	if capsProvider, ok := server.(interface{ Capabilities() map[string]any }); ok {
		capabilities = capsProvider.Capabilities()
	} else {
		capabilities = map[string]any{
			"tools": map[string]any{},
		}
	}

	return map[string]any{
		"mcp_response": map[string]any{
			"jsonrpc": "2.0",
			"id":      msgID,
			"result": map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    capabilities,
				"serverInfo":      serverInfo,
			},
		},
	}, nil
}

// handleMCPToolsList handles the tools/list method.
func (s *Session) handleMCPToolsList(
	msgID any,
	server mcp.ServerInstance,
) (map[string]any, error) {
	tools := server.ListTools()

	return map[string]any{
		"mcp_response": map[string]any{
			"jsonrpc": "2.0",
			"id":      msgID,
			"result": map[string]any{
				"tools": tools,
			},
		},
	}, nil
}

// handleMCPToolsCall handles the tools/call method.
func (s *Session) handleMCPToolsCall(
	ctx context.Context,
	msgID any,
	params map[string]any,
	server mcp.ServerInstance,
) (map[string]any, error) {
	if params == nil {
		return s.mcpErrorResponse(msgID, -32602, "Missing params for tools/call"), nil
	}

	toolName, _ := params["name"].(string)
	arguments, _ := params["arguments"].(map[string]any)

	if toolName == "" {
		return s.mcpErrorResponse(msgID, -32602, "Missing tool name in params"), nil
	}

	result, err := server.CallTool(ctx, toolName, arguments)
	if err != nil {
		return s.mcpErrorResponse(msgID, -32603, err.Error()), nil
	}

	return map[string]any{
		"mcp_response": map[string]any{
			"jsonrpc": "2.0",
			"id":      msgID,
			"result":  result,
		},
	}, nil
}

// mcpErrorResponse creates a JSONRPC error response.
func (s *Session) mcpErrorResponse(msgID any, code int, message string) map[string]any {
	return map[string]any{
		"mcp_response": map[string]any{
			"jsonrpc": "2.0",
			"id":      msgID,
			"error": map[string]any{
				"code":    code,
				"message": message,
			},
		},
	}
}

// HandleCanUseTool is called by CLI before tool use.
func (s *Session) HandleCanUseTool(
	ctx context.Context,
	req *ControlRequest,
) (map[string]any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	toolName, _ := req.Request["tool_name"].(string)
	input, _ := req.Request["input"].(map[string]any)

	// Compatibility path for newer app-server approval requests.
	if toolName == "" {
		if command, ok := req.Request["command"].(string); ok && command != "" {
			toolName = "Bash"

			if input == nil {
				input = map[string]any{
					"command": command,
				}
				if cwd, ok := req.Request["cwd"].(string); ok && cwd != "" {
					input["cwd"] = cwd
				}
			}
		}
	}

	if s.options == nil || s.options.CanUseTool == nil {
		return map[string]any{
			"behavior": "allow",
			"decision": "allow",
		}, nil
	}

	var suggestions []*permission.Update
	if suggestionsData, ok := req.Request["suggestions"].([]any); ok {
		suggestions = make([]*permission.Update, 0, len(suggestionsData))

		for _, sg := range suggestionsData {
			if suggestionMap, ok := sg.(map[string]any); ok {
				update := &permission.Update{}
				if t, ok := suggestionMap["type"].(string); ok {
					update.Type = permission.UpdateType(t)
				}

				suggestions = append(suggestions, update)
			}
		}
	}

	permCtx := &permission.Context{
		Suggestions: suggestions,
	}

	decision, err := s.options.CanUseTool(ctx, toolName, input, permCtx)
	if err != nil {
		return nil, err
	}

	switch d := decision.(type) {
	case *permission.ResultAllow:
		result := map[string]any{
			"behavior": "allow",
			"decision": "allow",
		}

		if d.UpdatedInput != nil {
			result["updatedInput"] = d.UpdatedInput
		}

		if d.UpdatedPermissions != nil {
			updates := make([]map[string]any, len(d.UpdatedPermissions))
			for i, u := range d.UpdatedPermissions {
				updates[i] = u.ToDict()
			}

			result["updatedPermissions"] = updates
		}

		return result, nil

	case *permission.ResultDeny:
		result := map[string]any{
			"behavior": "deny",
			"decision": "deny",
			"message":  d.Message,
		}

		if d.Interrupt {
			result["interrupt"] = true
		}

		return result, nil

	default:
		return nil, fmt.Errorf(
			"tool permission callback must return *ResultAllow or *ResultDeny, got %T",
			decision,
		)
	}
}
