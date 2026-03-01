// Package codexsdk provides a Go SDK for interacting with the Codex CLI agent.
//
// This SDK enables Go applications to programmatically communicate with the
// Codex CLI tool. It supports both one-shot queries and interactive multi-turn
// conversations.
//
// # Basic Usage
//
// For simple, one-shot queries, use the Query function:
//
//	ctx := context.Background()
//	for msg, err := range codexsdk.Query(ctx, "What is 2+2?",
//	    codexsdk.WithPermissionMode("acceptEdits"),
//	) {
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//
//	    switch m := msg.(type) {
//	    case *codexsdk.AssistantMessage:
//	        for _, block := range m.Content {
//	            if text, ok := block.(*codexsdk.TextBlock); ok {
//	                fmt.Println(text.Text)
//	            }
//	        }
//	    case *codexsdk.ResultMessage:
//	        if m.Usage != nil {
//	            fmt.Printf("Tokens: %d in / %d out\n", m.Usage.InputTokens, m.Usage.OutputTokens)
//	        }
//	    }
//	}
//
// # Interactive Sessions
//
// For multi-turn conversations, use NewClient or the WithClient helper:
//
//	// Using WithClient for automatic lifecycle management
//	err := codexsdk.WithClient(ctx, func(c codexsdk.Client) error {
//	    if err := c.Query(ctx, "Hello Codex"); err != nil {
//	        return err
//	    }
//	    for msg, err := range c.ReceiveResponse(ctx) {
//	        if err != nil {
//	            return err
//	        }
//	        // process message...
//	    }
//	    return nil
//	},
//	    codexsdk.WithLogger(slog.Default()),
//	    codexsdk.WithPermissionMode("acceptEdits"),
//	)
//
//	// Or using NewClient directly for more control
//	client := codexsdk.NewClient()
//	defer func() {
//	    if err := client.Close(); err != nil {
//	        log.Printf("failed to close client: %v", err)
//	    }
//	}()
//
//	err := client.Start(ctx,
//	    codexsdk.WithLogger(slog.Default()),
//	    codexsdk.WithPermissionMode("acceptEdits"),
//	)
//
// # Streaming Deltas
//
// By default, only completed AssistantMessage and ResultMessage are emitted.
// To receive token-by-token streaming deltas as StreamEvent messages, enable
// WithIncludePartialMessages:
//
//	for msg, err := range codexsdk.Query(ctx, "Hello",
//	    codexsdk.WithIncludePartialMessages(true),
//	) {
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    if se, ok := msg.(*codexsdk.StreamEvent); ok {
//	        // se.Event contains content_block_delta / text_delta data
//	    }
//	}
//
// # Logging
//
// For detailed operation tracking, use WithLogger:
//
//	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
//	messages, err := codexsdk.Query(ctx, "Hello Codex",
//	    codexsdk.WithLogger(logger),
//	)
//
// # Error Handling
//
// The SDK provides typed errors for different failure scenarios:
//
//	messages, err := codexsdk.Query(ctx, prompt, codexsdk.WithPermissionMode("acceptEdits"))
//	if err != nil {
//	    if cliErr, ok := errors.AsType[*codexsdk.CLINotFoundError](err); ok {
//	        log.Fatalf("Codex CLI not installed, searched: %v", cliErr.SearchedPaths)
//	    }
//	    if procErr, ok := errors.AsType[*codexsdk.ProcessError](err); ok {
//	        log.Fatalf("CLI process failed with exit code %d: %s", procErr.ExitCode, procErr.Stderr)
//	    }
//	    log.Fatal(err)
//	}
//
// # SDK Tools
//
// Register custom tools that the agent can call back into your Go code using
// NewTool and WithSDKTools. Tools are sent as dynamicTools and dispatched via
// the item/tool/call RPC:
//
//	add := codexsdk.NewTool("add", "Add two numbers",
//	    map[string]any{
//	        "type": "object",
//	        "properties": map[string]any{
//	            "a": map[string]any{"type": "number"},
//	            "b": map[string]any{"type": "number"},
//	        },
//	        "required": []string{"a", "b"},
//	    },
//	    func(_ context.Context, input map[string]any) (map[string]any, error) {
//	        a, _ := input["a"].(float64)
//	        b, _ := input["b"].(float64)
//	        return map[string]any{"result": a + b}, nil
//	    },
//	)
//
//	for msg, err := range codexsdk.Query(ctx, "Add 5 and 3",
//	    codexsdk.WithSDKTools(add),
//	    codexsdk.WithPermissionMode("bypassPermissions"),
//	) {
//	    // ...
//	}
//
// # Requirements
//
// This SDK requires the Codex CLI to be installed and available in your system PATH.
// You can specify a custom CLI path using the WithCliPath option.
package codexsdk
