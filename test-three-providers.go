package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type GenerateRequest struct {
	Prompt string `json:"prompt"`
	Model  string `json:"model"`
	Stream bool   `json:"stream"`
}

type TestCase struct {
	Name     string
	Provider string
	Model    string
	Prompt   string
}

func main() {
	fmt.Println("=== AptRouter API - Three Providers Test ===")
	fmt.Println("Testing all three providers with fast/cheap models")
	fmt.Println()

	baseURL := "http://localhost:8080"
	authHeader := "Bearer test-key"

	testCases := []TestCase{
		{
			Name:     "OpenAI GPT-3.5 Turbo - Short Story",
			Provider: "openai",
			Model:    "gpt-3.5-turbo",
			Prompt:   "Write a 3-sentence story about a robot learning to paint",
		},
		{
			Name:     "Google Gemini 1.5 Flash - Creative Writing",
			Provider: "google",
			Model:    "gemini-1.5-flash",
			Prompt:   "Describe a futuristic city where humans and AI coexist peacefully",
		},
		{
			Name:     "Anthropic Claude 3 Haiku - Analysis",
			Provider: "anthropic",
			Model:    "claude-3-haiku-20240307",
			Prompt:   "Analyze the impact of artificial intelligence on modern education",
		},
		{
			Name:     "OpenAI GPT-3.5 Turbo - Code Explanation",
			Provider: "openai",
			Model:    "gpt-3.5-turbo",
			Prompt:   "Explain how to implement a binary search algorithm in Python",
		},
		{
			Name:     "Google Gemini 1.5 Flash - Technical Question",
			Provider: "google",
			Model:    "gemini-1.5-flash",
			Prompt:   "What are the key differences between REST and GraphQL APIs?",
		},
		{
			Name:     "Anthropic Claude 3 Haiku - Creative Task",
			Provider: "anthropic",
			Model:    "claude-3-haiku-20240307",
			Prompt:   "Write a haiku about the changing seasons",
		},
	}

	// Test health check first
	if !testHealthCheck(baseURL) {
		fmt.Println("❌ Server is not running. Please start the server first.")
		return
	}

	successCount := 0
	totalCount := len(testCases)

	for i, testCase := range testCases {
		fmt.Printf("\n--- Test %d/%d: %s ---\n", i+1, totalCount, testCase.Name)
		fmt.Printf("Provider: %s\n", testCase.Provider)
		fmt.Printf("Model: %s\n", testCase.Model)
		fmt.Printf("Prompt: %s\n", testCase.Prompt)
		fmt.Println("Response:")

		if testStreamingEndpoint(baseURL, authHeader, testCase) {
			successCount++
		}

		fmt.Println(strings.Repeat("=", 80))

		// Small delay between tests
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Printf("\n=== Test Summary ===\n")
	fmt.Printf("Total Tests: %d\n", totalCount)
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Failed: %d\n", totalCount-successCount)
	fmt.Printf("Success Rate: %.1f%%\n", float64(successCount)/float64(totalCount)*100)

	fmt.Println("\n🎉 Three providers test completed!")
}

func testHealthCheck(baseURL string) bool {
	fmt.Println("--- Health Check ---")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"/healthz", nil)
	if err != nil {
		fmt.Printf("❌ Health check error: %v\n", err)
		return false
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Health check failed: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode == 200 {
		fmt.Println("✅ Server is healthy")
		return true
	} else {
		fmt.Printf("❌ Server health check failed: status %d\n", resp.StatusCode)
		return false
	}
}

func testStreamingEndpoint(baseURL, authHeader string, testCase TestCase) bool {
	startTime := time.Now()

	reqBody := GenerateRequest{
		Prompt: testCase.Prompt,
		Model:  testCase.Model,
		Stream: true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Printf("❌ JSON marshal error: %v\n", err)
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", baseURL+"/v1/generate/stream", bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Printf("❌ Request creation error: %v\n", err)
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", authHeader)

	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ Request failed: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		fmt.Printf("❌ HTTP error: status %d\n", resp.StatusCode)
		return false
	}

	// Analyze streaming response
	chunkCount := 0
	totalBytes := 0
	firstChunkTime := time.Time{}
	lastChunkTime := time.Time{}
	var fullResponse strings.Builder

	buffer := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("❌ Read error: %v\n", err)
			return false
		}

		if n > 0 {
			chunkCount++
			totalBytes += n
			chunk := string(buffer[:n])
			fullResponse.WriteString(chunk)

			if firstChunkTime.IsZero() {
				firstChunkTime = time.Now()
			}
			lastChunkTime = time.Now()

			// Print chunk (truncated for readability)
			if len(chunk) > 100 {
				fmt.Printf("Chunk %d (%d bytes): %s...\n", chunkCount, n, chunk[:100])
			} else {
				fmt.Printf("Chunk %d (%d bytes): %s\n", chunkCount, n, chunk)
			}
		}
	}

	endTime := time.Now()
	duration := endTime.Sub(startTime)

	// Calculate metrics
	var timeToFirstChunk time.Duration
	if !firstChunkTime.IsZero() {
		timeToFirstChunk = firstChunkTime.Sub(startTime)
	}

	var streamingDuration time.Duration
	if !lastChunkTime.IsZero() && !firstChunkTime.IsZero() {
		streamingDuration = lastChunkTime.Sub(firstChunkTime)
	}

	avgChunkSize := 0
	if chunkCount > 0 {
		avgChunkSize = totalBytes / chunkCount
	}

	// Print analysis
	fmt.Printf("\n📊 Streaming Analysis:\n")
	fmt.Printf("  Total Duration: %v\n", duration)
	fmt.Printf("  Time to First Chunk: %v\n", timeToFirstChunk)
	fmt.Printf("  Streaming Duration: %v\n", streamingDuration)
	fmt.Printf("  Total Chunks: %d\n", chunkCount)
	fmt.Printf("  Total Bytes: %d\n", totalBytes)
	fmt.Printf("  Average Chunk Size: %d bytes\n", avgChunkSize)
	fmt.Printf("  Chunks per Second: %.2f\n", float64(chunkCount)/duration.Seconds())
	fmt.Printf("  Bytes per Second: %.2f\n", float64(totalBytes)/duration.Seconds())

	// Content analysis
	fullText := fullResponse.String()
	fmt.Printf("  Total Response Length: %d characters\n", len(fullText))
	fmt.Printf("  Word Count: %d\n", len(strings.Fields(fullText)))

	// Success criteria
	success := chunkCount > 0 && totalBytes > 0 && len(fullText) > 0
	if success {
		fmt.Printf("✅ SUCCESS: %s\n", testCase.Name)
	} else {
		fmt.Printf("❌ FAILED: %s (no content received)\n", testCase.Name)
	}

	return success
}
