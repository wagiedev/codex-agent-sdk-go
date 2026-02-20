package protocol

import (
	"context"
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wagiedev/codex-agent-sdk-go/internal/config"
	"github.com/wagiedev/codex-agent-sdk-go/internal/mcp"
)

// TestSession_NeedsInitialization_Empty tests that NeedsInitialization returns false
// when no CanUseTool or MCP servers are configured.
func TestSession_NeedsInitialization_Empty(t *testing.T) {
	log := slog.Default()

	session := &Session{
		log:             log,
		options:         &config.Options{},
		sdkMcpServers:   make(map[string]mcp.ServerInstance, 4),
		sdkDynamicTools: make(map[string]*config.DynamicTool, 4),
	}

	require.False(t, session.NeedsInitialization(),
		"Expected NeedsInitialization() to return false with empty options")
}

func TestSession_NeedsInitialization_AdvancedOptionsAlone(t *testing.T) {
	log := slog.Default()

	session := &Session{
		log: log,
		options: &config.Options{
			Resume:               "thread_123",
			ContinueConversation: true,
			OutputFormat:         map[string]any{"type": "json_schema", "schema": map[string]any{"type": "object"}},
		},
		sdkMcpServers:   make(map[string]mcp.ServerInstance, 4),
		sdkDynamicTools: make(map[string]*config.DynamicTool, 4),
	}

	require.False(t, session.NeedsInitialization(),
		"Expected NeedsInitialization() to remain false without callbacks/MCP")
}

func TestSession_NeedsInitialization_WithDynamicTools(t *testing.T) {
	log := slog.Default()

	session := &Session{
		log:           log,
		options:       &config.Options{},
		sdkMcpServers: make(map[string]mcp.ServerInstance, 4),
		sdkDynamicTools: map[string]*config.DynamicTool{
			"add": {Name: "add", Description: "Add numbers"},
		},
	}

	require.True(t, session.NeedsInitialization(),
		"Expected NeedsInitialization() to return true with dynamic tools")
}

func TestSession_RegisterDynamicTools(t *testing.T) {
	log := slog.Default()

	tools := []*config.DynamicTool{
		{
			Name:        "add",
			Description: "Add two numbers",
			InputSchema: map[string]any{"type": "object"},
			Handler: func(_ context.Context, _ map[string]any) (map[string]any, error) {
				return map[string]any{"result": 42}, nil
			},
		},
		{
			Name:        "multiply",
			Description: "Multiply two numbers",
			Handler: func(_ context.Context, _ map[string]any) (map[string]any, error) {
				return map[string]any{"result": 6}, nil
			},
		},
	}

	session := NewSession(log, nil, &config.Options{SDKTools: tools})
	session.RegisterDynamicTools()

	require.Len(t, session.sdkDynamicTools, 2)
	require.NotNil(t, session.sdkDynamicTools["add"])
	require.NotNil(t, session.sdkDynamicTools["multiply"])
	require.Equal(t, "Add two numbers", session.sdkDynamicTools["add"].Description)
}

func TestSession_BuildInitializePayload_DynamicTools(t *testing.T) {
	log := slog.Default()

	session := &Session{
		log: log,
		options: &config.Options{
			SDKTools: []*config.DynamicTool{
				{
					Name:        "add",
					Description: "Add two numbers",
					InputSchema: map[string]any{
						"type": "object",
						"properties": map[string]any{
							"a": map[string]any{"type": "number"},
							"b": map[string]any{"type": "number"},
						},
						"required": []string{"a", "b"},
					},
				},
				{
					Name:        "greet",
					Description: "Say hello",
				},
			},
		},
		sdkMcpServers:   make(map[string]mcp.ServerInstance, 4),
		sdkDynamicTools: make(map[string]*config.DynamicTool, 4),
	}

	payload := session.buildInitializePayload()

	dynamicTools, ok := payload["dynamicTools"].([]map[string]any)
	require.True(t, ok, "dynamicTools should be []map[string]any")
	require.Len(t, dynamicTools, 2)

	require.Equal(t, "add", dynamicTools[0]["name"])
	require.Equal(t, "Add two numbers", dynamicTools[0]["description"])
	require.NotNil(t, dynamicTools[0]["inputSchema"])

	inputSchema, ok := dynamicTools[0]["inputSchema"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", inputSchema["type"])

	require.Equal(t, "greet", dynamicTools[1]["name"])
	require.Equal(t, "Say hello", dynamicTools[1]["description"])
	require.Nil(t, dynamicTools[1]["inputSchema"])
}

func TestSession_HandleDynamicToolCall_PlainName(t *testing.T) {
	log := slog.Default()

	var calledWith map[string]any

	session := NewSession(log, nil, &config.Options{})
	session.sdkDynamicTools["add"] = &config.DynamicTool{
		Name: "add",
		Handler: func(_ context.Context, input map[string]any) (map[string]any, error) {
			calledWith = input

			return map[string]any{"result": 42}, nil
		},
	}

	resp, err := session.HandleDynamicToolCall(context.Background(), &ControlRequest{
		Request: map[string]any{
			"tool":      "add",
			"arguments": map[string]any{"a": float64(5), "b": float64(3)},
		},
	})

	require.NoError(t, err)
	require.Equal(t, true, resp["success"])
	require.NotNil(t, calledWith)
	require.Equal(t, float64(5), calledWith["a"])

	items, ok := resp["contentItems"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Contains(t, items[0]["text"], "42")
}

func TestSession_HandleDynamicToolCall_FallbackMCP(t *testing.T) {
	log := slog.Default()

	session := NewSession(log, nil, &config.Options{})
	// No dynamic tools registered, but we have an MCP server
	// The MCP fallback should try to parse the name and fail
	// since no MCP server is registered.

	resp, err := session.HandleDynamicToolCall(context.Background(), &ControlRequest{
		Request: map[string]any{
			"tool":      "mcp__sdk__calc",
			"arguments": map[string]any{},
		},
	})

	require.NoError(t, err)
	require.Equal(t, false, resp["success"])

	items, ok := resp["contentItems"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Contains(t, items[0]["text"], "SDK MCP server not found")
}

func TestSession_HandleDynamicToolCall_UnknownTool(t *testing.T) {
	log := slog.Default()

	session := NewSession(log, nil, &config.Options{})

	resp, err := session.HandleDynamicToolCall(context.Background(), &ControlRequest{
		Request: map[string]any{
			"tool":      "nonexistent",
			"arguments": map[string]any{},
		},
	})

	require.NoError(t, err)
	require.Equal(t, false, resp["success"])

	items, ok := resp["contentItems"].([]map[string]any)
	require.True(t, ok)
	require.Len(t, items, 1)
	require.Contains(t, items[0]["text"], "unknown tool")
}

func TestSession_BuildInitializePayload_IncludesAdvancedFields(t *testing.T) {
	log := slog.Default()

	session := &Session{
		log: log,
		options: &config.Options{
			Model:                "gpt-5",
			Cwd:                  "/tmp/project",
			ContinueConversation: true,
			Resume:               "thread_abc",
			ForkSession:          true,
			AddDirs:              []string{"/tmp/extra"},
			OutputFormat: map[string]any{
				"type": "json_schema",
				"schema": map[string]any{
					"type": "object",
					"properties": map[string]any{
						"answer": map[string]any{"type": "string"},
					},
				},
			},
		},
		sdkMcpServers:   make(map[string]mcp.ServerInstance, 4),
		sdkDynamicTools: make(map[string]*config.DynamicTool, 4),
	}

	payload := session.buildInitializePayload()

	require.Equal(t, "gpt-5", payload["model"])
	require.Equal(t, "/tmp/project", payload["cwd"])
	require.Equal(t, true, payload["continueConversation"])
	require.Equal(t, "thread_abc", payload["resume"])
	require.Equal(t, true, payload["forkSession"])
	require.Equal(t, []string{"/tmp/extra"}, payload["addDirs"])

	outputSchema, ok := payload["outputSchema"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "object", outputSchema["type"])
}

// TestSession_InitializationResult_DataRace tests for data race between
// writing initializationResult and reading it via GetInitializationResult().
// Run with: go test -race -run TestSession_InitializationResult_DataRace.
func TestSession_InitializationResult_DataRace(t *testing.T) {
	log := slog.Default()

	session := &Session{
		log:             log,
		sdkMcpServers:   make(map[string]mcp.ServerInstance, 4),
		sdkDynamicTools: make(map[string]*config.DynamicTool, 4),
	}

	const iterations = 1000

	var wg sync.WaitGroup

	// Writer goroutine: simulates what Initialize() does (with mutex protection)

	wg.Go(func() {
		for i := range iterations {
			session.initMu.Lock()
			session.initializationResult = map[string]any{
				"iteration": i,
				"data":      "test",
			}
			session.initMu.Unlock()
		}
	})

	// Reader goroutine: simulates concurrent GetInitializationResult() calls

	wg.Go(func() {
		for range iterations {
			result := session.GetInitializationResult()

			// Access the map to ensure the race detector catches any issues
			if result != nil {
				_ = len(result)
			}
		}
	})

	wg.Wait()
}

// TestSession_InitializationResult_ConcurrentReadWrite tests the race between
// a single write and multiple concurrent reads.
// Run with: go test -race -run TestSession_InitializationResult_ConcurrentReadWrite.
func TestSession_InitializationResult_ConcurrentReadWrite(t *testing.T) {
	log := slog.Default()

	session := &Session{
		log:             log,
		sdkMcpServers:   make(map[string]mcp.ServerInstance, 4),
		sdkDynamicTools: make(map[string]*config.DynamicTool, 4),
	}

	const (
		readers    = 10
		iterations = 1000
	)

	var wg sync.WaitGroup

	// Single writer (simulates Initialize with mutex protection)

	wg.Go(func() {
		for i := range iterations {
			session.initMu.Lock()
			session.initializationResult = map[string]any{
				"version": "1.0.0",
				"count":   i,
			}
			session.initMu.Unlock()
		}
	})

	// Multiple readers using GetInitializationResult()
	for range readers {
		wg.Go(func() {
			for range iterations {
				result := session.GetInitializationResult()
				if result != nil {
					// Access map contents - safe because we received a copy
					_ = result["version"]
					_ = result["count"]
				}
			}
		})
	}

	wg.Wait()
}
