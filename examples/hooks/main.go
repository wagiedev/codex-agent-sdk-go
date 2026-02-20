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

const (
	bashToolName  = "Bash"
	writeToolName = "Write"
)

// displayMessage standardizes message display function.
func displayMessage(msg codexsdk.Message) {
	switch m := msg.(type) {
	case *codexsdk.AssistantMessage:
		for _, block := range m.Content {
			if textBlock, ok := block.(*codexsdk.TextBlock); ok {
				fmt.Printf("Codex: %s\n", textBlock.Text)
			}
		}

	case *codexsdk.ResultMessage:
		fmt.Println("Result ended")
	}
}

// examplePreToolUse demonstrates blocking commands using PreToolUse hook.
func examplePreToolUse() {
	fmt.Println("=== PreToolUse Example ===")
	fmt.Println("This example demonstrates PreToolUse configuration and reports observed callback activity.")
	fmt.Println("Note: some runtimes may emit zero hook callbacks; zero is valid observed behavior here.")
	fmt.Println()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client := codexsdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	defer client.Close()

	bashTool := bashToolName
	timeout := 5.0

	var (
		preToolCalls          int32
		patternBlockDecisions int32
	)

	// Hook to check bash commands
	checkBashCommand := func(
		ctx context.Context,
		input codexsdk.HookInput,
		toolUseID *string,
		hookCtx *codexsdk.HookContext,
	) (codexsdk.HookJSONOutput, error) {
		preToolInput, ok := input.(*codexsdk.PreToolUseHookInput)
		if !ok {
			continueFlag := true

			return &codexsdk.SyncHookJSONOutput{Continue: &continueFlag}, nil
		}

		atomic.AddInt32(&preToolCalls, 1)

		if preToolInput.ToolName != bashToolName {
			continueFlag := true

			return &codexsdk.SyncHookJSONOutput{Continue: &continueFlag}, nil
		}

		command, _ := preToolInput.ToolInput["command"].(string)
		blockPatterns := []string{"foo.sh"}

		for _, pattern := range blockPatterns {
			if strings.Contains(command, pattern) {
				atomic.AddInt32(&patternBlockDecisions, 1)
				fmt.Printf("[HOOK] Blocked command: %s\n", command)

				return &codexsdk.SyncHookJSONOutput{
					HookSpecificOutput: &codexsdk.PreToolUseHookSpecificOutput{
						HookEventName:            "PreToolUse",
						PermissionDecision:       new("deny"),
						PermissionDecisionReason: new("Command contains invalid pattern: " + pattern),
					},
				}, nil
			}
		}

		continueFlag := true

		return &codexsdk.SyncHookJSONOutput{Continue: &continueFlag}, nil
	}

	if err := client.Start(ctx,
		codexsdk.WithLogger(logger),
		codexsdk.WithAllowedTools(bashToolName),
		codexsdk.WithPermissionMode("bypassPermissions"),
		codexsdk.WithHooks(map[codexsdk.HookEvent][]*codexsdk.HookMatcher{
			codexsdk.HookEventPreToolUse: {{
				Matcher: &bashTool,
				Hooks:   []codexsdk.HookCallback{checkBashCommand},
				Timeout: &timeout,
			}},
		}),
	); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)

		return
	}

	// Test 1: Command with forbidden pattern (deny decision attempted)
	fmt.Println("Test 1: Trying a command with a deny-pattern (hook may or may not enforce denial end-to-end)...")
	fmt.Println("User: Run the bash command: ./foo.sh --help")

	if err := client.Query(ctx, "Run the bash command: ./foo.sh --help"); err != nil {
		fmt.Printf("Failed to send query: %v\n", err)

		return
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	fmt.Println("\n" + strings.Repeat("=", 50) + "\n")

	// Test 2: Safe command path
	fmt.Println("Test 2: Trying a command that should pass the hook policy...")
	fmt.Println("User: Run the bash command: echo 'Hello from hooks example!'")

	if err := client.Query(ctx, "Run the bash command: echo 'Hello from hooks example!'"); err != nil {
		fmt.Printf("Failed to send query: %v\n", err)

		return
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	printHookObservation("Observed PreToolUse callbacks", atomic.LoadInt32(&preToolCalls))
	fmt.Printf("Observed deny decisions from hook: %d\n", atomic.LoadInt32(&patternBlockDecisions))
	fmt.Println()
}

// exampleUserPromptSubmit demonstrates adding context at user prompt submit.
func exampleUserPromptSubmit() {
	fmt.Println("=== UserPromptSubmit Example ===")
	fmt.Println("This example demonstrates UserPromptSubmit configuration and reports callback count.")
	fmt.Println("Note: some runtimes may emit zero hook callbacks; zero is valid observed behavior here.")
	fmt.Println()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client := codexsdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	defer client.Close()

	timeout := 5.0

	var submitCalls int32

	// Hook to add custom instructions at session start
	addCustomInstructions := func(
		ctx context.Context,
		input codexsdk.HookInput,
		toolUseID *string,
		hookCtx *codexsdk.HookContext,
	) (codexsdk.HookJSONOutput, error) {
		atomic.AddInt32(&submitCalls, 1)

		return &codexsdk.SyncHookJSONOutput{
			HookSpecificOutput: &codexsdk.UserPromptSubmitHookSpecificOutput{
				HookEventName:     "UserPromptSubmit",
				AdditionalContext: new("My favorite color is hot pink"),
			},
		}, nil
	}

	if err := client.Start(ctx,
		codexsdk.WithLogger(logger),
		codexsdk.WithHooks(map[codexsdk.HookEvent][]*codexsdk.HookMatcher{
			codexsdk.HookEventUserPromptSubmit: {{
				Hooks:   []codexsdk.HookCallback{addCustomInstructions},
				Timeout: &timeout,
			}},
		}),
	); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)

		return
	}

	fmt.Println("User: What's my favorite color?")

	if err := client.Query(ctx, "What's my favorite color?"); err != nil {
		fmt.Printf("Failed to send query: %v\n", err)

		return
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	printHookObservation("Observed UserPromptSubmit callbacks", atomic.LoadInt32(&submitCalls))
	fmt.Println()
}

// examplePostToolUse demonstrates reviewing tool output with reason and systemMessage.
func examplePostToolUse() {
	fmt.Println("=== PostToolUse Example ===")
	fmt.Println("This example demonstrates PostToolUse configuration and reports callback activity.")
	fmt.Println("Note: some runtimes may emit zero hook callbacks; zero is valid observed behavior here.")
	fmt.Println()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client := codexsdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	defer client.Close()

	bashTool := bashToolName
	timeout := 5.0

	var (
		postToolCalls      int32
		errorResponsesSeen int32
	)

	// Hook to review tool output
	reviewToolOutput := func(
		ctx context.Context,
		input codexsdk.HookInput,
		toolUseID *string,
		hookCtx *codexsdk.HookContext,
	) (codexsdk.HookJSONOutput, error) {
		postToolInput, ok := input.(*codexsdk.PostToolUseHookInput)
		if !ok {
			continueFlag := true

			return &codexsdk.SyncHookJSONOutput{Continue: &continueFlag}, nil
		}

		atomic.AddInt32(&postToolCalls, 1)

		toolResponse := fmt.Sprintf("%v", postToolInput.ToolResponse)

		// If the tool produced an error, add helpful context
		if strings.Contains(strings.ToLower(toolResponse), "error") {
			atomic.AddInt32(&errorResponsesSeen, 1)

			return &codexsdk.SyncHookJSONOutput{
				SystemMessage: new("The command produced an error. You may want to try a different approach."),
				Reason:        new("Tool execution failed - consider checking the command syntax"),
				HookSpecificOutput: &codexsdk.PostToolUseHookSpecificOutput{
					HookEventName: "PostToolUse",
				},
			}, nil
		}

		continueFlag := true

		return &codexsdk.SyncHookJSONOutput{Continue: &continueFlag}, nil
	}

	if err := client.Start(ctx,
		codexsdk.WithLogger(logger),
		codexsdk.WithAllowedTools(bashToolName),
		codexsdk.WithPermissionMode("bypassPermissions"),
		codexsdk.WithHooks(map[codexsdk.HookEvent][]*codexsdk.HookMatcher{
			codexsdk.HookEventPostToolUse: {{
				Matcher: &bashTool,
				Hooks:   []codexsdk.HookCallback{reviewToolOutput},
				Timeout: &timeout,
			}},
		}),
	); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)

		return
	}

	fmt.Println("User: Run a command that will produce an error: ls /nonexistent_directory")

	if err := client.Query(ctx, "Run this command: ls /nonexistent_directory"); err != nil {
		fmt.Printf("Failed to send query: %v\n", err)

		return
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	printHookObservation("Observed PostToolUse callbacks", atomic.LoadInt32(&postToolCalls))
	fmt.Printf("Observed PostToolUse error annotations: %d\n", atomic.LoadInt32(&errorResponsesSeen))
	fmt.Println()
}

// exampleDecisionFields demonstrates using permissionDecision allow/deny.
func exampleDecisionFields() {
	fmt.Println("=== Permission Decision Example ===")
	fmt.Println("This example configures allow/deny permission decisions and reports observed hook calls.")
	fmt.Println("Note: some runtimes may emit zero hook callbacks; zero is valid observed behavior here.")
	fmt.Println()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client := codexsdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	defer client.Close()

	writeTool := writeToolName
	timeout := 5.0

	var (
		decisionCalls  int32
		denyDecisions  int32
		allowDecisions int32
	)

	// Hook with strict approval logic
	strictApprovalHook := func(
		ctx context.Context,
		input codexsdk.HookInput,
		toolUseID *string,
		hookCtx *codexsdk.HookContext,
	) (codexsdk.HookJSONOutput, error) {
		preToolInput, ok := input.(*codexsdk.PreToolUseHookInput)
		if !ok {
			continueFlag := true

			return &codexsdk.SyncHookJSONOutput{Continue: &continueFlag}, nil
		}

		atomic.AddInt32(&decisionCalls, 1)

		// Block any Write operations to specific files
		if preToolInput.ToolName == writeToolName {
			filePath, _ := preToolInput.ToolInput["file_path"].(string)

			if strings.Contains(strings.ToLower(filePath), "important") {
				atomic.AddInt32(&denyDecisions, 1)
				fmt.Printf("[HOOK] Blocked Write to: %s\n", filePath)

				return &codexsdk.SyncHookJSONOutput{
					Reason:        new("Writes to files containing 'important' in the name are not allowed for safety"),
					SystemMessage: new("Write operation blocked by security policy"),
					HookSpecificOutput: &codexsdk.PreToolUseHookSpecificOutput{
						HookEventName:            "PreToolUse",
						PermissionDecision:       new("deny"),
						PermissionDecisionReason: new("Security policy blocks writes to important files"),
					},
				}, nil
			}
		}

		// Allow everything else explicitly
		atomic.AddInt32(&allowDecisions, 1)

		return &codexsdk.SyncHookJSONOutput{
			Reason: new("Tool use approved after security review"),
			HookSpecificOutput: &codexsdk.PreToolUseHookSpecificOutput{
				HookEventName:            "PreToolUse",
				PermissionDecision:       new("allow"),
				PermissionDecisionReason: new("Tool passed security checks"),
			},
		}, nil
	}

	if err := client.Start(ctx,
		codexsdk.WithLogger(logger),
		codexsdk.WithTools(codexsdk.ToolsList{writeToolName}),
		codexsdk.WithPermissionMode("bypassPermissions"),
		codexsdk.WithHooks(map[codexsdk.HookEvent][]*codexsdk.HookMatcher{
			codexsdk.HookEventPreToolUse: {{
				Matcher: &writeTool,
				Hooks:   []codexsdk.HookCallback{strictApprovalHook},
				Timeout: &timeout,
			}},
		}),
	); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)

		return
	}

	// Test 1: Try to write to a file with "important" in the name
	fmt.Println("Test 1: Trying to write to important_config.txt (hook should emit deny decision)...")
	fmt.Println("User: Write 'test' to important_config.txt")

	if err := client.Query(ctx, "Write the text 'test data' to a file called important_config.txt"); err != nil {
		fmt.Printf("Failed to send query: %v\n", err)

		return
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	fmt.Println("\n" + strings.Repeat("=", 50) + "\n")

	// Test 2: Write to a regular file
	fmt.Println("Test 2: Trying to write to regular_file.txt (hook should emit allow decision)...")
	fmt.Println("User: Write 'test' to regular_file.txt")

	if err := client.Query(ctx, "Write the text 'test data' to a file called regular_file.txt"); err != nil {
		fmt.Printf("Failed to send query: %v\n", err)

		return
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	decisionCount := atomic.LoadInt32(&decisionCalls)
	printHookObservation("Observed PermissionDecision callbacks", decisionCount)
	fmt.Printf("Observed decision breakdown: allow=%d deny=%d\n",
		atomic.LoadInt32(&allowDecisions),
		atomic.LoadInt32(&denyDecisions),
	)
	fmt.Println()
}

// exampleContinueControl demonstrates using continue=false to stop execution on errors.
func exampleContinueControl() {
	fmt.Println("=== Continue/Stop Control Example ===")
	fmt.Println("This example demonstrates continue/stop hook configuration and reports observed signals.")
	fmt.Println("Note: some runtimes may emit zero hook callbacks; zero is valid observed behavior here.")
	fmt.Println()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client := codexsdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	defer client.Close()

	bashTool := bashToolName
	timeout := 5.0

	var (
		postToolCalls int32
		stopSignals   int32
	)

	// Hook to stop on critical errors
	stopOnErrorHook := func(
		ctx context.Context,
		input codexsdk.HookInput,
		toolUseID *string,
		hookCtx *codexsdk.HookContext,
	) (codexsdk.HookJSONOutput, error) {
		postToolInput, ok := input.(*codexsdk.PostToolUseHookInput)
		if !ok {
			continueFlag := true

			return &codexsdk.SyncHookJSONOutput{Continue: &continueFlag}, nil
		}

		atomic.AddInt32(&postToolCalls, 1)

		toolResponse := fmt.Sprintf("%v", postToolInput.ToolResponse)

		// Stop execution if we see a critical error
		if strings.Contains(strings.ToLower(toolResponse), "critical") {
			atomic.AddInt32(&stopSignals, 1)
			fmt.Println("[HOOK] Critical error detected - stopping execution")

			continueFlag := false

			return &codexsdk.SyncHookJSONOutput{
				Continue:      &continueFlag,
				StopReason:    new("Critical error detected in tool output - execution halted for safety"),
				SystemMessage: new("Execution stopped due to critical error"),
			}, nil
		}

		continueFlag := true

		return &codexsdk.SyncHookJSONOutput{Continue: &continueFlag}, nil
	}

	if err := client.Start(ctx,
		codexsdk.WithLogger(logger),
		codexsdk.WithAllowedTools(bashToolName),
		codexsdk.WithPermissionMode("bypassPermissions"),
		codexsdk.WithHooks(map[codexsdk.HookEvent][]*codexsdk.HookMatcher{
			codexsdk.HookEventPostToolUse: {{
				Matcher: &bashTool,
				Hooks:   []codexsdk.HookCallback{stopOnErrorHook},
				Timeout: &timeout,
			}},
		}),
	); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)

		return
	}

	fmt.Println("User: Run a command that outputs 'CRITICAL ERROR'")

	if err := client.Query(ctx, "Run this bash command: echo 'CRITICAL ERROR: system failure'"); err != nil {
		fmt.Printf("Failed to send query: %v\n", err)

		return
	}

	for msg := range client.ReceiveResponse(ctx) {
		displayMessage(msg)
	}

	printHookObservation("Observed ContinueControl callbacks", atomic.LoadInt32(&postToolCalls))
	fmt.Printf("Observed continue=false stop signals: %d\n", atomic.LoadInt32(&stopSignals))
	fmt.Println()
}

func printHookObservation(label string, count int32) {
	fmt.Printf("%s: %d\n", label, count)

	if count == 0 {
		fmt.Println("Hook callbacks were not emitted in this runtime; this is expected in some app-server builds.")
	}
}

func main() {
	fmt.Println("Starting Codex SDK Hooks Examples...")
	fmt.Println(strings.Repeat("=", 50))
	fmt.Println()

	examples := map[string]func(){
		"PreToolUse":       examplePreToolUse,
		"UserPromptSubmit": exampleUserPromptSubmit,
		"PostToolUse":      examplePostToolUse,
		"DecisionFields":   exampleDecisionFields,
		"ContinueControl":  exampleContinueControl,
	}

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run main.go <example_name>")
		fmt.Println("\nAvailable examples:")
		fmt.Println("  all - Run all examples")

		for name := range examples {
			fmt.Printf("  %s\n", name)
		}

		fmt.Println("\nExample descriptions:")
		fmt.Println("  PreToolUse       - Observe pre-tool callback activity and deny decisions")
		fmt.Println("  UserPromptSubmit - Observe user-prompt submit callback activity")
		fmt.Println("  PostToolUse      - Observe post-tool callback activity")
		fmt.Println("  DecisionFields   - Emit allow/deny permission decisions from hooks")
		fmt.Println("  ContinueControl  - Emit continue/stop signals from post-tool hooks")

		return
	}

	exampleName := os.Args[1]

	if exampleName == "all" {
		exampleOrder := []string{
			"PreToolUse", "UserPromptSubmit", "PostToolUse",
			"DecisionFields", "ContinueControl",
		}

		for _, name := range exampleOrder {
			if fn, ok := examples[name]; ok {
				fn()
				fmt.Println(strings.Repeat("-", 50))
				fmt.Println()
			}
		}
	} else if fn, ok := examples[exampleName]; ok {
		fn()
	} else {
		fmt.Printf("Error: Unknown example '%s'\n", exampleName)
		fmt.Println("\nAvailable examples:")
		fmt.Println("  all - Run all examples")

		for name := range examples {
			fmt.Printf("  %s\n", name)
		}

		os.Exit(1)
	}
}
