package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type StreamingRequest struct {
	Prompt      string  `json:"prompt"`
	Model       string  `json:"model"`
	MaxTokens   int     `json:"max_tokens"`
	Temperature float64 `json:"temperature"`
	Stream      bool    `json:"stream"`
}

type StreamingTestCase struct {
	Provider string
	Model    string
	Prompt   string
}

func mainStreamingTest() {
	fmt.Println("🚀 AptRouter API - All Providers Streaming Test")
	fmt.Println("==============================================")

	baseURL := "http://localhost:8080"
	apiKey := "test-key-123"

	// Test cases for each provider
	testCases := []StreamingTestCase{
		{
			Provider: "OpenAI",
			Model:    "gpt-3.5-turbo",
			Prompt:   "Write a haiku about artificial intelligence.",
		},
		{
			Provider: "Google",
			Model:    "gemini-2.0-flash",
			Prompt:   "Explain blockchain technology in one sentence.",
		},
		{
			Provider: "Anthropic",
			Model:    "claude-3-haiku-20240307",
			Prompt:   "Describe the future of renewable energy in two sentences.",
		},
	}

	// Health check first
	fmt.Println("\n🔍 Step 1: Health Check")
	if err := healthCheck(baseURL); err != nil {
		fmt.Printf("❌ Health check failed: %v\n", err)
		return
	}
	fmt.Println("✅ Health check passed")

	// Test each provider
	successCount := 0
	totalTests := len(testCases)

	for i, testCase := range testCases {
		fmt.Printf("\n🧪 Test %d/%d: %s with %s\n", i+1, totalTests, testCase.Provider, testCase.Model)
		fmt.Printf("Prompt: %s\n", testCase.Prompt)

		if testStreaming(baseURL, apiKey, testCase) {
			successCount++
		}
	}

	// Summary
	fmt.Printf("\n📊 Test Summary\n")
	fmt.Printf("==============\n")
	fmt.Printf("Total tests: %d\n", totalTests)
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Failed: %d\n", totalTests-successCount)
	fmt.Printf("Success rate: %.1f%%\n", float64(successCount)/float64(totalTests)*100)

	if successCount == totalTests {
		fmt.Println("\n🎉 All streaming tests passed! All providers are working correctly.")
	} else {
		fmt.Println("\n⚠️  Some tests failed. Check the output above for details.")
	}

	fmt.Println("\n✨ Test completed!")
}

func main() {
	mainStreamingTest()
}

func healthCheck(baseURL string) error {
	resp, err := http.Get(baseURL + "/healthz")
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check returned status %d", resp.StatusCode)
	}

	var health struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&health); err != nil {
		return err
	}

	if health.Status != "healthy" {
		return fmt.Errorf("health check returned status %s", health.Status)
	}

	return nil
}

func testStreaming(baseURL, apiKey string, testCase StreamingTestCase) bool {
	reqBody := StreamingRequest{
		Prompt:      testCase.Prompt,
		Model:       testCase.Model,
		MaxTokens:   200,
		Temperature: 0.7,
		Stream:      true,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		fmt.Printf("❌ Failed to marshal request: %v\n", err)
		return false
	}

	req, err := http.NewRequest("POST", baseURL+"/v1/generate/stream", bytes.NewBuffer(jsonBody))
	if err != nil {
		fmt.Printf("❌ Failed to create request: %v\n", err)
		return false
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	startTime := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Printf("❌ Request failed: %v\n", err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("❌ Request returned status %d: %s\n", resp.StatusCode, string(body))
		return false
	}

	// Read streaming response
	fmt.Printf("📡 Streaming response from %s:\n", testCase.Provider)
	fmt.Printf("   ")

	chunkCount := 0
	totalBytes := 0
	buffer := make([]byte, 1024)

	for {
		n, err := resp.Body.Read(buffer)
		if err != nil {
			if err == io.EOF {
				break
			}
			fmt.Printf("\n❌ Error reading stream: %v\n", err)
			return false
		}

		if n > 0 {
			chunk := buffer[:n]
			fmt.Print(string(chunk))
			chunkCount++
			totalBytes += n
		}
	}

	duration := time.Since(startTime)
	fmt.Printf("\n\n📊 Streaming Metrics:\n")
	fmt.Printf("   Duration: %v\n", duration)
	fmt.Printf("   Chunks received: %d\n", chunkCount)
	fmt.Printf("   Total bytes: %d\n", totalBytes)
	fmt.Printf("   Average chunk size: %.1f bytes\n", float64(totalBytes)/float64(chunkCount))

	if chunkCount > 0 && totalBytes > 0 {
		fmt.Printf("✅ %s streaming successful!\n", testCase.Provider)
		return true
	} else {
		fmt.Printf("❌ %s streaming failed - no content received\n", testCase.Provider)
		return false
	}
}
