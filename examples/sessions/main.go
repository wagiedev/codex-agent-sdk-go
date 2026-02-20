package main

import (
	"context"
	"fmt"
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

	case *codexsdk.ResultMessage:
		fmt.Println("Result ended")

		if m.TotalCostUSD != nil {
			fmt.Printf("Cost: $%.4f\n", *m.TotalCostUSD)
		}
	}
}

func queryAndDisplay(ctx context.Context, client codexsdk.Client, prompt string) *codexsdk.ResultMessage {
	if err := client.Query(ctx, prompt); err != nil {
		fmt.Printf("Query failed: %v\n", err)

		return nil
	}

	var result *codexsdk.ResultMessage

	for msg, err := range client.ReceiveResponse(ctx) {
		if err != nil {
			fmt.Printf("Receive failed: %v\n", err)

			return nil
		}

		displayMessage(msg)

		if r, ok := msg.(*codexsdk.ResultMessage); ok {
			result = r
		}
	}

	return result
}

// continueConversationExample demonstrates multi-turn conversation in one live client session.
func continueConversationExample() {
	fmt.Println("=== Continue Conversation Example ===")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := codexsdk.NewClient()
	defer client.Close()

	if err := client.Start(ctx); err != nil {
		fmt.Printf("Start failed: %v\n", err)

		return
	}

	fmt.Println("\n--- First query: Establish context ---")
	queryAndDisplay(ctx, client, "Remember: my favorite color is blue")

	fmt.Println("\n--- Second query: Verify memory ---")
	queryAndDisplay(ctx, client, "What is my favorite color?")

	fmt.Println()
}

// resumeSessionExample demonstrates session ID capture from ResultMessage.
func resumeSessionExample() {
	fmt.Println("=== Resume Session Example ===")

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	client := codexsdk.NewClient()
	defer client.Close()

	if err := client.Start(ctx); err != nil {
		fmt.Printf("Start failed: %v\n", err)

		return
	}

	fmt.Println("\n--- First query: Establish context and capture session id ---")

	result := queryAndDisplay(ctx, client, "Remember: x = 42")
	if result != nil && result.SessionID != "" {
		fmt.Printf("Captured Session ID: %s\n", result.SessionID)
	} else {
		fmt.Println("Session ID not present in result message")
	}

	fmt.Println("\n--- Second query: Verify memory in same session ---")
	queryAndDisplay(ctx, client, "What is x?")

	fmt.Println()
}

// forkSessionExample demonstrates independent conversation contexts via two clients.
func forkSessionExample() {
	fmt.Println("=== Fork Session Example ===")

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	originalClient := codexsdk.NewClient()
	defer originalClient.Close()

	if err := originalClient.Start(ctx); err != nil {
		fmt.Printf("Original start failed: %v\n", err)

		return
	}

	fmt.Println("\n--- Original session: set Python ---")
	queryAndDisplay(ctx, originalClient, "Remember: the project language is Python")

	forkClient := codexsdk.NewClient()
	defer forkClient.Close()

	if err := forkClient.Start(ctx); err != nil {
		fmt.Printf("Fork start failed: %v\n", err)

		return
	}

	fmt.Println("\n--- Fork-like independent session: set Rust ---")
	queryAndDisplay(ctx, forkClient, "Remember: the project language is Rust")

	fmt.Println("\n--- Verify original session still says Python ---")
	queryAndDisplay(ctx, originalClient, "What is the project language?")

	fmt.Println("\n--- Verify fork session says Rust ---")
	queryAndDisplay(ctx, forkClient, "What is the project language?")

	fmt.Println()
}

func main() {
	fmt.Println("Session Examples")
	fmt.Println()

	continueConversationExample()
	resumeSessionExample()
	forkSessionExample()
}
