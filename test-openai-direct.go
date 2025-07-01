package main

import (
	"context"
	"fmt"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

func main() {
	fmt.Println("🚀 Testing OpenAI Streaming Directly")
	fmt.Println("====================================")

	client := openai.NewClient(option.WithAPIKey("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"))

	stream := client.Chat.Completions.NewStreaming(context.Background(), openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage("Write a short poem about the ocean."),
		},
		Model: openai.ChatModelGPT4o,
	})

	fmt.Println("📝 Streaming response:")
	fmt.Println("=====================")

	chunkCount := 0
	for stream.Next() {
		chunk := stream.Current()
		chunkCount++

		fmt.Printf("[Chunk %d] Type: %T\n", chunkCount, chunk)
		fmt.Printf("Chunk content: %+v\n", chunk)
		fmt.Println("---")
	}

	if stream.Err() != nil {
		fmt.Printf("❌ Stream error: %v\n", stream.Err())
		return
	}

	fmt.Printf("🏁 Stream completed! Total chunks: %d\n", chunkCount)
}
