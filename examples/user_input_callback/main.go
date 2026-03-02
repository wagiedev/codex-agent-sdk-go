package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	codexsdk "github.com/wagiedev/codex-agent-sdk-go"
)

// displayMessage standardizes message display across examples.
func displayMessage(msg codexsdk.Message) {
	switch m := msg.(type) {
	case *codexsdk.UserMessage:
		for _, block := range m.Content.Blocks() {
			if textBlock, ok := block.(*codexsdk.TextBlock); ok {
				fmt.Printf("User: %s\n", textBlock.Text)
			}
		}

	case *codexsdk.AssistantMessage:
		for _, block := range m.Content {
			if textBlock, ok := block.(*codexsdk.TextBlock); ok {
				fmt.Printf("Codex: %s\n", textBlock.Text)
			}
		}

	case *codexsdk.SystemMessage:
		// Ignore system messages in display

	case *codexsdk.ResultMessage:
		fmt.Println("Task completed!")

		if m.Usage != nil {
			fmt.Printf("   Tokens: %d in / %d out\n", m.Usage.InputTokens, m.Usage.OutputTokens)
		}
	}
}

// myUserInputCallback handles user input requests from the agent.
// It auto-selects the first option for multiple-choice questions
// or returns a canned answer for free-text questions.
func myUserInputCallback(
	_ context.Context,
	req *codexsdk.UserInputRequest,
) (*codexsdk.UserInputResponse, error) {
	answers := make(map[string]*codexsdk.UserInputAnswer, len(req.Questions))

	for _, q := range req.Questions {
		fmt.Printf("\n--- User Input Request ---\n")
		fmt.Printf("   Question: %s\n", q.Question)

		if q.Header != "" {
			fmt.Printf("   Header:   %s\n", q.Header)
		}

		if len(q.Options) > 0 {
			fmt.Printf("   Options:\n")

			for i, opt := range q.Options {
				marker := "  "
				if i == 0 {
					marker = ">>"
				}

				fmt.Printf("     %s %s - %s\n", marker, opt.Label, opt.Description)
			}

			// Auto-select the first option.
			fmt.Printf("   Auto-selecting: %s\n", q.Options[0].Label)

			answers[q.ID] = &codexsdk.UserInputAnswer{
				Answers: []string{q.Options[0].Label},
			}
		} else {
			// Free text — return a canned answer.
			canned := "Automated SDK response"

			fmt.Printf("   Free text — answering: %q\n", canned)

			answers[q.ID] = &codexsdk.UserInputAnswer{
				Answers: []string{canned},
			}
		}
	}

	return &codexsdk.UserInputResponse{Answers: answers}, nil
}

// myPermissionCallback auto-allows all tool usage.
func myPermissionCallback(
	_ context.Context,
	toolName string,
	_ map[string]any,
	_ *codexsdk.ToolPermissionContext,
) (codexsdk.PermissionResult, error) {
	fmt.Printf("   Auto-allowing tool: %s\n", toolName)

	return &codexsdk.PermissionResultAllow{Behavior: "allow"}, nil
}

func main() {
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("User Input Callback Example")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("\nThis example demonstrates how to:")
	fmt.Println("1. Handle requestUserInput from the agent")
	fmt.Println("2. Auto-select options or provide free-text answers")
	fmt.Println("3. Use plan mode with user input callbacks")
	fmt.Println(strings.Repeat("=", 60))

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	client := codexsdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	defer func() {
		if err := client.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close client: %v\n", err)
		}
	}()

	if err := client.Start(ctx,
		codexsdk.WithLogger(logger),
		codexsdk.WithOnUserInput(myUserInputCallback),
		codexsdk.WithCanUseTool(myPermissionCallback),
		codexsdk.WithPermissionMode("plan"),
		codexsdk.WithCwd("."),
	); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)

		return
	}

	fmt.Println("\nSending query to Codex...")

	queryText := `I'm testing the request_user_input tool. Please use the request_user_input tool ` +
		`to ask me to select between options: Go, Rust, Python. ` +
		`Then tell me which one I selected.`

	if err := client.Query(ctx, queryText); err != nil {
		fmt.Printf("Failed to send query: %v\n", err)

		return
	}

	fmt.Println("\nReceiving response...")

	messageCount := 0

	for msg, err := range client.ReceiveMessages(ctx) {
		if err != nil {
			break
		}

		messageCount++

		displayMessage(msg)

		if _, ok := msg.(*codexsdk.ResultMessage); ok {
			fmt.Printf("   Messages processed: %d\n", messageCount)

			break
		}
	}
}
