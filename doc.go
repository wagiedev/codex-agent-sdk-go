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
//	messages, err := codexsdk.Query(ctx, "What is 2+2?",
//	    codexsdk.WithPermissionMode("acceptEdits"),
//	)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	for msg := range messages {
//	    switch m := msg.(type) {
//	    case *codexsdk.AssistantMessage:
//	        for _, block := range m.Content {
//	            if text, ok := block.(*codexsdk.TextBlock); ok {
//	                fmt.Println(text.Text)
//	            }
//	        }
//	    case *codexsdk.ResultMessage:
//	        fmt.Printf("Completed in %dms\n", m.DurationMs)
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
//	defer client.Close(ctx)
//
//	err := client.Start(ctx,
//	    codexsdk.WithLogger(slog.Default()),
//	    codexsdk.WithPermissionMode("acceptEdits"),
//	)
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
// # Requirements
//
// This SDK requires the Codex CLI to be installed and available in your system PATH.
// You can specify a custom CLI path using the WithCliPath option.
package codexsdk
