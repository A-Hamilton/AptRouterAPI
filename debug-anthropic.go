package main

import (
	"context"
	"fmt"
	"os"

	"github.com/apt-router/api/internal/llm"
)

func main() {
	fmt.Println("=== Anthropic Claude Debug Test ===")

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ ANTHROPIC_API_KEY environment variable not set")
		return
	}

	// Create Anthropic client
	client, err := llm.NewAnthropicClient("claude-3-haiku-20240307", apiKey)
	if err != nil {
		fmt.Printf("❌ Failed to create Anthropic client: %v\n", err)
		return
	}

	// Test non-streaming first
	fmt.Println("\n--- Testing Non-Streaming ---")
	ctx := context.Background()
	params := map[string]interface{}{
		"prompt": "Tell me a short story about a robot",
	}

	resp, err := client.GenerateWithParams(ctx, params)
	if err != nil {
		fmt.Printf("❌ Non-streaming failed: %v\n", err)
	} else {
		fmt.Printf("✅ Non-streaming success: %d chars\n", len(resp.Text))
		previewLen := 100
		if len(resp.Text) < previewLen {
			previewLen = len(resp.Text)
		}
		fmt.Printf("Text: %s\n", resp.Text[:previewLen])
	}

	// Test streaming
	fmt.Println("\n--- Testing Streaming ---")
	streamResp, err := client.GenerateStream(ctx, params)
	if err != nil {
		fmt.Printf("❌ Streaming failed: %v\n", err)
		return
	}

	fmt.Println("✅ Stream created successfully")

	// Read from stream
	buffer := make([]byte, 1024)
	totalBytes := 0
	chunkCount := 0

	for {
		n, err := streamResp.Stream.Read(buffer)
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			fmt.Printf("❌ Stream read error: %v\n", err)
			break
		}

		if n > 0 {
			chunkCount++
			totalBytes += n
			chunk := string(buffer[:n])
			fmt.Printf("Chunk %d (%d bytes): %s\n", chunkCount, n, chunk)
		}
	}

	fmt.Printf("\n📊 Streaming Summary:\n")
	fmt.Printf("Total Chunks: %d\n", chunkCount)
	fmt.Printf("Total Bytes: %d\n", totalBytes)

	if chunkCount == 0 {
		fmt.Println("❌ No content received from stream")
	} else {
		fmt.Println("✅ Streaming working correctly")
	}
}
