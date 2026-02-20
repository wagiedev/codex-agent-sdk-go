package codexsdk

import (
	"log/slog"
	"time"

	"github.com/wagiedev/codex-agent-sdk-go/internal/config"
)

// Option configures CodexAgentOptions using the functional options pattern.
type Option func(*CodexAgentOptions)

// AgentOption is an alias for Option for backward compatibility.
//
// Deprecated: Use Option instead.
type AgentOption = Option

// applyAgentOptions applies functional options to a CodexAgentOptions struct.
func applyAgentOptions(opts []Option) *CodexAgentOptions {
	options := &CodexAgentOptions{}
	for _, opt := range opts {
		opt(options)
	}

	return options
}

// ===== Basic Configuration =====

// WithLogger sets the logger for debug output.
// If not set, logging is disabled (silent operation).
func WithLogger(logger *slog.Logger) Option {
	return func(o *CodexAgentOptions) {
		o.Logger = logger
	}
}

// WithAgentLogger is an alias for WithLogger.
//
// Deprecated: Use WithLogger instead.
var WithAgentLogger = WithLogger

// WithSystemPrompt sets the system message to send to the agent.
func WithSystemPrompt(prompt string) Option {
	return func(o *CodexAgentOptions) {
		o.SystemPrompt = prompt
	}
}

// WithSystemPromptPreset sets a preset system prompt configuration.
// If set, this takes precedence over WithSystemPrompt.
func WithSystemPromptPreset(preset *SystemPromptPreset) Option {
	return func(o *CodexAgentOptions) {
		o.SystemPromptPreset = preset
	}
}

// WithModel specifies which model to use.
func WithModel(model string) Option {
	return func(o *CodexAgentOptions) {
		o.Model = model
	}
}

// WithPermissionMode controls how permissions are handled.
// For Codex, maps to sandbox modes: "default" -> full-auto,
// "acceptEdits" -> workspace-write, "bypassPermissions" -> danger-full-access.
func WithPermissionMode(mode string) Option {
	return func(o *CodexAgentOptions) {
		o.PermissionMode = mode
	}
}

// WithCwd sets the working directory for the CLI process.
func WithCwd(cwd string) Option {
	return func(o *CodexAgentOptions) {
		o.Cwd = cwd
	}
}

// WithCliPath sets the explicit path to the codex CLI binary.
// If not set, the CLI will be searched in PATH.
func WithCliPath(path string) Option {
	return func(o *CodexAgentOptions) {
		o.CliPath = path
	}
}

// WithEnv provides additional environment variables for the CLI process.
func WithEnv(env map[string]string) Option {
	return func(o *CodexAgentOptions) {
		o.Env = env
	}
}

// ===== Hooks =====

// WithHooks configures event hooks for tool interception.
// Hooks are registered via protocol session and dispatched when Codex CLI sends
// hooks/callback requests.
func WithHooks(hooks map[HookEvent][]*HookMatcher) Option {
	return func(o *CodexAgentOptions) {
		o.Hooks = hooks
	}
}

// ===== Token/Budget =====

// WithEffort sets the thinking effort level.
// Passed to CLI via initialization; support depends on Codex CLI version.
func WithEffort(effort config.Effort) Option {
	return func(o *CodexAgentOptions) {
		o.Effort = &effort
	}
}

// ===== MCP =====

// WithMCPServers configures external MCP servers to connect to.
// Map key is the server name, value is the server configuration.
func WithMCPServers(servers map[string]MCPServerConfig) Option {
	return func(o *CodexAgentOptions) {
		o.MCPServers = servers
	}
}

// ===== Tools =====

// WithTools specifies which tools are available.
// Accepts ToolsList (tool names) or *ToolsPreset.
func WithTools(tools config.ToolsConfig) Option {
	return func(o *CodexAgentOptions) {
		o.Tools = tools
	}
}

// WithAllowedTools sets pre-approved tools that can be used without prompting.
func WithAllowedTools(tools ...string) Option {
	return func(o *CodexAgentOptions) {
		o.AllowedTools = tools
	}
}

// WithDisallowedTools sets tools that are explicitly blocked.
func WithDisallowedTools(tools ...string) Option {
	return func(o *CodexAgentOptions) {
		o.DisallowedTools = tools
	}
}

// WithCanUseTool sets a callback for permission checking before each tool use.
// Permission callback invoked when CLI sends can_use_tool requests via protocol.
func WithCanUseTool(callback ToolPermissionCallback) Option {
	return func(o *CodexAgentOptions) {
		o.CanUseTool = callback
	}
}

// ===== Session =====

// WithContinueConversation indicates whether to continue an existing conversation.
func WithContinueConversation(cont bool) Option {
	return func(o *CodexAgentOptions) {
		o.ContinueConversation = cont
	}
}

// WithResume sets a session ID to resume from.
func WithResume(sessionID string) Option {
	return func(o *CodexAgentOptions) {
		o.Resume = sessionID
	}
}

// WithForkSession indicates whether to fork the resumed session to a new ID.
func WithForkSession(fork bool) Option {
	return func(o *CodexAgentOptions) {
		o.ForkSession = fork
	}
}

// ===== Advanced =====

// WithPermissionPromptToolName specifies the tool name to use for permission prompts.
func WithPermissionPromptToolName(name string) Option {
	return func(o *CodexAgentOptions) {
		o.PermissionPromptToolName = name
	}
}

// WithAddDirs adds additional directories to make accessible.
func WithAddDirs(dirs ...string) Option {
	return func(o *CodexAgentOptions) {
		o.AddDirs = dirs
	}
}

// WithExtraArgs provides arbitrary CLI flags to pass to the CLI.
// If the value is nil, the flag is passed without a value (boolean flag).
func WithExtraArgs(args map[string]*string) Option {
	return func(o *CodexAgentOptions) {
		o.ExtraArgs = args
	}
}

// WithStderr sets a callback function for handling stderr output.
func WithStderr(handler func(string)) Option {
	return func(o *CodexAgentOptions) {
		o.Stderr = handler
	}
}

// WithOutputFormat specifies a JSON schema for structured output.
//
// The canonical format uses a wrapper object:
//
//	codexsdk.WithOutputFormat(map[string]any{
//	    "type": "json_schema",
//	    "schema": map[string]any{
//	        "type":       "object",
//	        "properties": map[string]any{...},
//	        "required":   []string{...},
//	    },
//	})
//
// Raw JSON schemas (without the wrapper) are also accepted and auto-wrapped:
//
//	codexsdk.WithOutputFormat(map[string]any{
//	    "type":       "object",
//	    "properties": map[string]any{...},
//	    "required":   []string{...},
//	})
func WithOutputFormat(format map[string]any) Option {
	return func(o *CodexAgentOptions) {
		o.OutputFormat = format
	}
}

// WithInitializeTimeout sets the timeout for the initialize control request.
func WithInitializeTimeout(timeout time.Duration) Option {
	return func(o *CodexAgentOptions) {
		o.InitializeTimeout = &timeout
	}
}

// WithTransport injects a custom transport implementation.
// The transport must implement the Transport interface.
func WithTransport(transport config.Transport) Option {
	return func(o *CodexAgentOptions) {
		o.Transport = transport
	}
}

// ===== Codex-Native Options =====

// WithSandbox sets the Codex sandbox mode directly.
// Valid values: "read-only", "workspace-write", "danger-full-access".
func WithSandbox(sandbox string) Option {
	return func(o *CodexAgentOptions) {
		o.Sandbox = sandbox
	}
}

// WithImages provides file paths for image inputs (passed via -i flags).
func WithImages(images ...string) Option {
	return func(o *CodexAgentOptions) {
		o.Images = images
	}
}

// WithConfig provides key=value pairs for Codex CLI configuration (passed via -c flags).
func WithConfig(cfg map[string]string) Option {
	return func(o *CodexAgentOptions) {
		o.Config = cfg
	}
}

// WithOutputSchema sets the --output-schema flag for structured Codex output.
func WithOutputSchema(schema string) Option {
	return func(o *CodexAgentOptions) {
		o.OutputSchema = schema
	}
}

// WithSkipVersionCheck disables CLI version validation during discovery.
func WithSkipVersionCheck(skip bool) Option {
	return func(o *CodexAgentOptions) {
		o.SkipVersionCheck = skip
	}
}
