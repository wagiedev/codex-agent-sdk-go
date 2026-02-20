package protocol

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/wagiedev/codex-agent-sdk-go/internal/config"
	"github.com/wagiedev/codex-agent-sdk-go/internal/mcp"
	"github.com/wagiedev/codex-agent-sdk-go/internal/permission"
)

const (
	// defaultInitializeTimeout is the default timeout for initialize control requests.
	defaultInitializeTimeout = 60 * time.Second
)

// Session encapsulates protocol handling logic for MCP servers and callbacks.
type Session struct {
	log        *slog.Logger
	controller *Controller
	options    *config.Options

	sdkMcpServers   map[string]mcp.ServerInstance
	sdkDynamicTools map[string]*config.DynamicTool

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
		log:             log.With("component", "session"),
		controller:      controller,
		options:         options,
		sdkMcpServers:   make(map[string]mcp.ServerInstance, 4),
		sdkDynamicTools: make(map[string]*config.DynamicTool, 4),
	}
}

// RegisterHandlers registers protocol handlers for MCP tool calls and
// command approval requests.
func (s *Session) RegisterHandlers() {
	s.controller.RegisterHandler("item_tool/call", s.HandleDynamicToolCall)
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

// RegisterDynamicTools indexes SDK dynamic tools by name for dispatch.
func (s *Session) RegisterDynamicTools() {
	if s.options == nil || len(s.options.SDKTools) == 0 {
		return
	}

	for _, tool := range s.options.SDKTools {
		s.sdkDynamicTools[tool.Name] = tool
		s.log.Debug("registered dynamic tool", slog.String("tool", tool.Name))
	}
}

// Initialize sends the initialization control request to the CLI.
func (s *Session) Initialize(ctx context.Context) error {
	s.log.DebugContext(ctx, "sending initialize request")

	payload := s.buildInitializePayload()

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
func (s *Session) buildInitializePayload() map[string]any {
	payload := make(map[string]any, 16)

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

	if servers := serializeMCPServers(s.options.MCPServers); len(servers) > 0 {
		payload["mcpServers"] = servers
	}

	if dynamicTools := serializeDynamicTools(s.options.SDKTools); len(dynamicTools) > 0 {
		payload["dynamicTools"] = dynamicTools
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

// serializeDynamicTools converts SDK dynamic tools into the flat array format
// expected by the Codex CLI dynamicTools API.
func serializeDynamicTools(tools []*config.DynamicTool) []map[string]any {
	if len(tools) == 0 {
		return nil
	}

	result := make([]map[string]any, 0, len(tools))

	for _, tool := range tools {
		entry := map[string]any{
			"name":        tool.Name,
			"description": tool.Description,
		}

		if tool.InputSchema != nil {
			entry["inputSchema"] = tool.InputSchema
		}

		result = append(result, entry)
	}

	return result
}

// serializeMCPServers converts MCP server configs into a map suitable for the
// initialize payload. SDK servers are serialized as {"type":"sdk"} so the CLI
// routes tool calls back through the control protocol.
func serializeMCPServers(servers map[string]mcp.ServerConfig) map[string]any {
	if len(servers) == 0 {
		return nil
	}

	result := make(map[string]any, len(servers))

	for name, serverCfg := range servers {
		if serverCfg == nil {
			continue
		}

		switch cfg := serverCfg.(type) {
		case *mcp.StdioServerConfig:
			entry := map[string]any{
				"type":    string(cfg.GetType()),
				"command": cfg.Command,
			}

			if len(cfg.Args) > 0 {
				entry["args"] = cfg.Args
			}

			if len(cfg.Env) > 0 {
				entry["env"] = cfg.Env
			}

			result[name] = entry
		case *mcp.SSEServerConfig:
			entry := map[string]any{
				"type": string(cfg.Type),
				"url":  cfg.URL,
			}

			if len(cfg.Headers) > 0 {
				entry["headers"] = cfg.Headers
			}

			result[name] = entry
		case *mcp.HTTPServerConfig:
			entry := map[string]any{
				"type": string(cfg.Type),
				"url":  cfg.URL,
			}

			if len(cfg.Headers) > 0 {
				entry["headers"] = cfg.Headers
			}

			result[name] = entry
		case *mcp.SdkServerConfig:
			entry := map[string]any{
				"type": string(cfg.Type),
			}

			if instance, ok := cfg.Instance.(mcp.ServerInstance); ok {
				entry["tools"] = instance.ListTools()
			}

			result[name] = entry
		}
	}

	return result
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
	case "askAll":
		return "untrusted"
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

	if timeoutStr := os.Getenv("CODEX_INITIALIZE_TIMEOUT"); timeoutStr != "" {
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

	return s.options.CanUseTool != nil ||
		len(s.sdkMcpServers) > 0 ||
		len(s.sdkDynamicTools) > 0
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

// HandleDynamicToolCall handles item/tool/call requests from the CLI for
// SDK-registered dynamic tools and MCP server tools.
func (s *Session) HandleDynamicToolCall(
	ctx context.Context,
	req *ControlRequest,
) (map[string]any, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}

	toolFullName, _ := req.Request["tool"].(string)
	arguments, _ := req.Request["arguments"].(map[string]any)

	// Try plain name lookup in dynamic tools first.
	if tool, ok := s.sdkDynamicTools[toolFullName]; ok {
		return s.executeDynamicTool(ctx, tool, arguments)
	}

	// Fall back to MCP server lookup for mcp__<server>__<tool> names.
	serverName, toolName, err := parseMCPToolName(toolFullName)
	if err != nil {
		//nolint:nilerr // Error is encoded in the protocol response
		return map[string]any{
			"success": false,
			"contentItems": []map[string]any{{
				"type": "inputText",
				"text": fmt.Sprintf("unknown tool: %s", toolFullName),
			}},
		}, nil
	}

	server, exists := s.sdkMcpServers[serverName]
	if !exists {
		return map[string]any{
			"success": false,
			"contentItems": []map[string]any{{
				"type": "inputText",
				"text": fmt.Sprintf("SDK MCP server not found: %s", serverName),
			}},
		}, nil
	}

	result, callErr := server.CallTool(ctx, toolName, arguments)
	if callErr != nil {
		//nolint:nilerr // Error is encoded in the protocol response
		return map[string]any{
			"success": false,
			"contentItems": []map[string]any{{
				"type": "inputText",
				"text": callErr.Error(),
			}},
		}, nil
	}

	isError, _ := result["is_error"].(bool)

	contentItems := convertMCPContentToItems(result)

	return map[string]any{
		"success":      !isError,
		"contentItems": contentItems,
	}, nil
}

// executeDynamicTool calls a dynamic tool handler and formats the result
// as the protocol response.
func (s *Session) executeDynamicTool(
	ctx context.Context,
	tool *config.DynamicTool,
	arguments map[string]any,
) (map[string]any, error) {
	result, err := tool.Handler(ctx, arguments)
	if err != nil {
		//nolint:nilerr // Error is encoded in the protocol response
		return map[string]any{
			"success": false,
			"contentItems": []map[string]any{{
				"type": "inputText",
				"text": err.Error(),
			}},
		}, nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return map[string]any{
			"success": false,
			"contentItems": []map[string]any{{
				"type": "inputText",
				"text": fmt.Sprintf("failed to marshal tool result: %v", err),
			}},
		}, nil
	}

	return map[string]any{
		"success": true,
		"contentItems": []map[string]any{{
			"type": "inputText",
			"text": string(data),
		}},
	}, nil
}

// parseMCPToolName splits "mcp__<server>__<tool>" into server and tool names.
func parseMCPToolName(fullName string) (serverName, toolName string, err error) {
	const prefix = "mcp__"
	if len(fullName) <= len(prefix) || fullName[:len(prefix)] != prefix {
		return "", "", fmt.Errorf("invalid MCP tool name format: %s", fullName)
	}

	rest := fullName[len(prefix):]
	idx := 0

	for i := 0; i+1 < len(rest); i++ {
		if rest[i] == '_' && rest[i+1] == '_' {
			idx = i

			break
		}
	}

	if idx == 0 {
		return "", "", fmt.Errorf("invalid MCP tool name format (missing server/tool separator): %s", fullName)
	}

	return rest[:idx], rest[idx+2:], nil
}

// convertMCPContentToItems converts MCP result content entries to the
// DynamicToolCallResponse contentItems format.
func convertMCPContentToItems(result map[string]any) []map[string]any {
	content, ok := result["content"].([]map[string]any)
	if !ok {
		// Try []any (common from JSON unmarshalling).
		if contentAny, ok := result["content"].([]any); ok {
			items := make([]map[string]any, 0, len(contentAny))
			for _, entry := range contentAny {
				if entryMap, ok := entry.(map[string]any); ok {
					text, _ := entryMap["text"].(string)
					items = append(items, map[string]any{
						"type": "inputText",
						"text": text,
					})
				}
			}

			return items
		}

		return []map[string]any{}
	}

	items := make([]map[string]any, 0, len(content))
	for _, entry := range content {
		text, _ := entry["text"].(string)
		items = append(items, map[string]any{
			"type": "inputText",
			"text": text,
		})
	}

	return items
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
			"decision": "accept",
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

	switch decision.(type) {
	case *permission.ResultAllow:
		return map[string]any{
			"decision": "accept",
		}, nil

	case *permission.ResultDeny:
		return map[string]any{
			"decision": "decline",
		}, nil

	default:
		return nil, fmt.Errorf(
			"tool permission callback must return *ResultAllow or *ResultDeny, got %T",
			decision,
		)
	}
}
