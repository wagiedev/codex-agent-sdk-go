package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	codexsdk "github.com/wagiedev/codex-agent-sdk-go"
)

func displayMessage(msg codexsdk.Message) float64 {
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
	case *codexsdk.ResultMessage:
		fmt.Printf("Status: %s\n", m.Subtype)

		if m.TotalCostUSD != nil {
			fmt.Printf("Total cost: $%.6f\n", *m.TotalCostUSD)

			return *m.TotalCostUSD
		}
	}

	return 0
}

func runSingleQuery(title, prompt string) {
	fmt.Printf("=== %s ===\n", title)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	client := codexsdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	defer func() {
		if err := client.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close client: %v\n", err)
		}
	}()

	if err := client.Start(ctx, codexsdk.WithLogger(logger)); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)

		return
	}

	if err := client.Query(ctx, prompt); err != nil {
		fmt.Printf("Failed to send query: %v\n", err)

		return
	}

	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			fmt.Printf("Failed to receive response: %v\n", err)

			return
		}

		displayMessage(msg)
	}

	fmt.Println()
}

func runSoftBudgetExample() {
	fmt.Println("=== Soft Budget Guard Example (Client-Side) ===")
	fmt.Println("This demo enforces a local budget by stopping additional turns once cumulative cost exceeds the target.")
	fmt.Println()

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	client := codexsdk.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	defer func() {
		if err := client.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "failed to close client: %v\n", err)
		}
	}()

	if err := client.Start(ctx,
		codexsdk.WithLogger(logger),
		codexsdk.WithPermissionMode("bypassPermissions"),
	); err != nil {
		fmt.Printf("Failed to connect: %v\n", err)

		return
	}

	softBudget := 0.0001
	prompts := []string{
		"Read README.md and summarize it in 5 bullet points.",
		"Now summarize the previous summary in 2 bullet points.",
		"Now give a one-paragraph abstract of the repository.",
	}

	cumulative := 0.0
	costSignals := 0

	for i, prompt := range prompts {
		fmt.Printf("Soft-budget query %d: %s\n", i+1, prompt)

		if err := client.Query(ctx, prompt); err != nil {
			fmt.Printf("Failed to send query: %v\n", err)

			return
		}

		for msg, err := range client.ReceiveResponse(ctx) {
			if err != nil {
				fmt.Printf("Failed to receive response: %v\n", err)

				return
			}

			turnCost := displayMessage(msg)
			if turnCost > 0 {
				costSignals++
				cumulative += turnCost
			}
		}

		fmt.Printf("Cumulative observed cost: $%.6f\n", cumulative)

		if cumulative >= softBudget {
			fmt.Printf("Soft budget reached ($%.6f >= $%.6f); stopping further queries.\n", cumulative, softBudget)

			break
		}

		fmt.Println()
	}

	if costSignals == 0 {
		fmt.Println("[INFO] No cost values were returned in result messages for this run.")
		fmt.Println("[INFO] Soft budget guard cannot trigger without reported TotalCostUSD.")
	}

	fmt.Println()
}

func main() {
	fmt.Println("Budget Management Examples")
	fmt.Println()
	fmt.Println("This example demonstrates budget-aware execution using result-message cost tracking.")
	fmt.Println()

	runSingleQuery("Baseline Query", "What is 2 + 2?")
	runSoftBudgetExample()

	fmt.Println("Note: This is a client-side soft budget strategy.")
	fmt.Println("The SDK currently does not expose a built-in hard max budget option.")
}
