package protocol

import (
	"log/slog"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/wagiedev/codex-agent-sdk-go/internal/config"
	"github.com/wagiedev/codex-agent-sdk-go/internal/hook"
	"github.com/wagiedev/codex-agent-sdk-go/internal/mcp"
)

// TestSession_NeedsInitialization_Empty tests that NeedsInitialization returns false
// when no hooks, agents, CanUseTool, or MCP servers are configured.
func TestSession_NeedsInitialization_Empty(t *testing.T) {
	log := slog.Default()

	session := &Session{
		log:           log,
		options:       &config.Options{},
		hookCallbacks: make(map[string]hook.Callback, 16),
		sdkMcpServers: make(map[string]mcp.ServerInstance, 4),
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
		hookCallbacks: make(map[string]hook.Callback, 16),
		sdkMcpServers: make(map[string]mcp.ServerInstance, 4),
	}

	require.False(t, session.NeedsInitialization(),
		"Expected NeedsInitialization() to remain false without hooks/callbacks/MCP/agents")
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
		hookCallbacks: make(map[string]hook.Callback, 16),
		sdkMcpServers: make(map[string]mcp.ServerInstance, 4),
	}

	payload := session.buildInitializePayload(map[string]any{})

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

	// Create a session without a controller (we'll manipulate the field directly)
	session := &Session{
		log:           log,
		hookCallbacks: make(map[string]hook.Callback, 16),
		sdkMcpServers: make(map[string]mcp.ServerInstance, 4),
	}

	const iterations = 1000

	var wg sync.WaitGroup

	// Writer goroutine: simulates what Initialize() does (with mutex protection)

	wg.Go(func() {
		for i := range iterations {
			// This simulates what Initialize() does at line 141-143 (with mutex)
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
			// This calls the actual GetInitializationResult() which uses mutex
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
		log:           log,
		hookCallbacks: make(map[string]hook.Callback, 16),
		sdkMcpServers: make(map[string]mcp.ServerInstance, 4),
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
