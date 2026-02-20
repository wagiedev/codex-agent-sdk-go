// Package main demonstrates context compaction monitoring using the PreCompact hook.
//
// Context compaction occurs when the conversation history exceeds limits (95% by default)
// or when manually triggered with /compact. The PreCompact hook allows you to monitor
// and potentially customize compaction behavior.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync/atomic"
	"time"

	codexsdk "github.com/wagiedev/codex-agent-sdk-go"
)

var compactionFired atomic.Bool

// displayMessage displays message content in a clean format.
func displayMessage(msg codexsdk.Message) {
	switch m := msg.(type) {
	case *codexsdk.AssistantMessage:
		for _, block := range m.Content {
			if textBlock, ok := block.(*codexsdk.TextBlock); ok {
				text := textBlock.Text
				if len(text) > 150 {
					text = text[:150] + "..."
				}

				fmt.Printf("Codex: %s\n", text)
			}
		}

	case *codexsdk.ResultMessage:
		if m.TotalCostUSD != nil {
			fmt.Printf("Cost: $%.6f\n", *m.TotalCostUSD)
		}
	}
}

func main() {
	fmt.Println("Compaction Hook Example")
	fmt.Println("Demonstrating PreCompact hook with manual /compact trigger")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client := codexsdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	defer client.Close()

	timeout := 5.0

	// PreCompact hook to monitor compaction events
	compactionHook := func(
		ctx context.Context,
		input codexsdk.HookInput,
		toolUseID *string,
		hookCtx *codexsdk.HookContext,
	) (codexsdk.HookJSONOutput, error) {
		preCompact, ok := input.(*codexsdk.PreCompactHookInput)
		if !ok {
			continueFlag := true

			return &codexsdk.SyncHookJSONOutput{Continue: &continueFlag}, nil
		}

		fmt.Println()
		fmt.Println(strings.Repeat("*", 60))
		fmt.Println("[COMPACTION EVENT FIRED]")
		fmt.Printf("  Trigger: %s\n", preCompact.Trigger)
		fmt.Printf("  Session ID: %s\n", preCompact.SessionID)

		if preCompact.CustomInstructions != nil {
			fmt.Printf("  Custom Instructions: %s\n", *preCompact.CustomInstructions)
		}

		if preCompact.TranscriptPath != "" {
			fmt.Printf("  Transcript: %s\n", preCompact.TranscriptPath)
		}

		fmt.Println(strings.Repeat("*", 60))
		fmt.Println()

		compactionFired.Store(true)

		continueFlag := true

		return &codexsdk.SyncHookJSONOutput{Continue: &continueFlag}, nil
	}

	if err := client.Start(ctx,
		codexsdk.WithLogger(logger),
		codexsdk.WithHooks(map[codexsdk.HookEvent][]*codexsdk.HookMatcher{
			codexsdk.HookEventPreCompact: {{
				Hooks:   []codexsdk.HookCallback{compactionHook},
				Timeout: &timeout,
			}},
		}),
	); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)

		return
	}

	// Step 1: Build up some context first
	fmt.Println("Step 1: Building up context...")
	fmt.Println(strings.Repeat("-", 50))

	buildContextPrompt := "Write a detailed paragraph about the history of computers, " +
		"including key milestones like ENIAC, personal computers, and smartphones."

	if err := client.Query(ctx, buildContextPrompt); err != nil {
		fmt.Printf("Failed to send query: %v\n", err)

		return
	}

	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			fmt.Printf("Error receiving response: %v\n", err)

			return
		}

		displayMessage(msg)
	}

	// Step 2: Trigger manual compaction with /compact
	fmt.Println()
	fmt.Println("Step 2: Triggering manual compaction with /compact...")
	fmt.Println(strings.Repeat("-", 50))

	// Send /compact command - this triggers the PreCompact hook
	if err := client.Query(ctx, "/compact Preserve the computing history discussion."); err != nil {
		fmt.Printf("Failed to send /compact: %v\n", err)

		return
	}

	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			fmt.Printf("Error receiving response: %v\n", err)

			return
		}

		displayMessage(msg)
	}

	// Step 3: Verify hook fired
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))

	if compactionFired.Load() {
		fmt.Println("SUCCESS: PreCompact hook intercepted the compaction event!")
		fmt.Println()
		fmt.Println("The hook received:")
		fmt.Println("  - Trigger type (manual/auto)")
		fmt.Println("  - Session ID")
		fmt.Println("  - Custom instructions (if provided)")
		fmt.Println("  - Transcript path")
	} else {
		fmt.Println("NOTE: Compaction hook did not fire.")
		fmt.Println("This may happen if context was too small to compact.")
	}
}
