package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// TestRequest represents a test request
type TestRequest struct {
	Model  string `json:"model"`
	Prompt string `json:"prompt"`
}

// TestResponse represents a test response
type TestResponse struct {
	ID           string                 `json:"id"`
	Text         string                 `json:"text"`
	Model        string                 `json:"model"`
	Provider     string                 `json:"provider"`
	Usage        *UsageInfo             `json:"usage,omitempty"`
	FinishReason string                 `json:"finish_reason,omitempty"`
	CreatedAt    int64                  `json:"created_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// UsageInfo contains token usage information
type UsageInfo struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// TestResult represents the result of a test
type TestResult struct {
	Provider     string
	Model        string
	Endpoint     string
	Success      bool
	Error        string
	ResponseTime time.Duration
	Response     string
	Usage        *UsageInfo
	Metadata     map[string]interface{}
}

// TestConfig holds test configuration
type TestConfig struct {
	BaseURL    string
	APIKey     string
	Models     map[string]string // provider -> model
	TestPrompt string
	Timeout    time.Duration
}

func main() {
	// Configuration
	config := TestConfig{
		BaseURL: "http://localhost:8080",
		APIKey:  "test-api-key",
		Models: map[string]string{
			"openai":    "gpt-3.5-turbo",
			"google":    "gemini-2.0-flash",
			"anthropic": "claude-3-haiku-20240307",
		},
		TestPrompt: "Write a short, friendly greeting in exactly 2 sentences.",
		Timeout:    30 * time.Second,
	}

	// Override with environment variables
	if baseURL := os.Getenv("API_BASE_URL"); baseURL != "" {
		config.BaseURL = baseURL
	}
	if apiKey := os.Getenv("API_KEY"); apiKey != "" {
		config.APIKey = apiKey
	}

	fmt.Printf("🚀 Starting E2E Tests for AptRouter API\n")
	fmt.Printf("Base URL: %s\n", config.BaseURL)
	fmt.Printf("Test Prompt: %s\n", config.TestPrompt)
	fmt.Printf("Timeout: %s\n\n", config.Timeout)

	// Test results
	var results []TestResult

	// Test non-streaming endpoints
	fmt.Println("📝 Testing Non-Streaming Endpoints")
	fmt.Println("==================================")
	for provider, model := range config.Models {
		fmt.Printf("\nTesting %s with model %s...\n", provider, model)

		// Test non-streaming
		result := testNonStreaming(config, provider, model)
		results = append(results, result)

		if result.Success {
			fmt.Printf("✅ %s non-streaming: SUCCESS (%.2fs)\n", provider, result.ResponseTime.Seconds())
			fmt.Printf("   Response: %s\n", truncateString(result.Response, 100))
			if result.Usage != nil {
				fmt.Printf("   Usage: %d input, %d output tokens\n", result.Usage.InputTokens, result.Usage.OutputTokens)
			}
		} else {
			fmt.Printf("❌ %s non-streaming: FAILED - %s\n", provider, result.Error)
		}

		// Small delay between requests
		time.Sleep(1 * time.Second)
	}

	// Test streaming endpoints
	fmt.Println("\n\n🌊 Testing Streaming Endpoints")
	fmt.Println("===============================")
	for provider, model := range config.Models {
		fmt.Printf("\nTesting %s with model %s...\n", provider, model)

		// Test streaming
		result := testStreaming(config, provider, model)
		results = append(results, result)

		if result.Success {
			fmt.Printf("✅ %s streaming: SUCCESS (%.2fs)\n", provider, result.ResponseTime.Seconds())
			fmt.Printf("   Response: %s\n", truncateString(result.Response, 100))
			if result.Usage != nil {
				fmt.Printf("   Usage: %d input, %d output tokens\n", result.Usage.InputTokens, result.Usage.OutputTokens)
			}
		} else {
			fmt.Printf("❌ %s streaming: FAILED - %s\n", provider, result.Error)
		}

		// Small delay between requests
		time.Sleep(1 * time.Second)
	}

	// Print summary
	printSummary(results)
}

func testNonStreaming(config TestConfig, provider, model string) TestResult {
	start := time.Now()

	// Create request
	reqBody := TestRequest{
		Model:  model,
		Prompt: config.TestPrompt,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return TestResult{
			Provider: provider,
			Model:    model,
			Endpoint: "non-streaming",
			Success:  false,
			Error:    fmt.Sprintf("Failed to marshal request: %v", err),
		}
	}

	// Create HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", config.BaseURL+"/v1/generate", bytes.NewBuffer(jsonBody))
	if err != nil {
		return TestResult{
			Provider: provider,
			Model:    model,
			Endpoint: "non-streaming",
			Success:  false,
			Error:    fmt.Sprintf("Failed to create request: %v", err),
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.APIKey)

	// Make request
	client := &http.Client{Timeout: config.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return TestResult{
			Provider:     provider,
			Model:        model,
			Endpoint:     "non-streaming",
			Success:      false,
			Error:        fmt.Sprintf("Request failed: %v", err),
			ResponseTime: time.Since(start),
		}
	}
	defer resp.Body.Close()

	// Read response
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return TestResult{
			Provider:     provider,
			Model:        model,
			Endpoint:     "non-streaming",
			Success:      false,
			Error:        fmt.Sprintf("Failed to read response: %v", err),
			ResponseTime: time.Since(start),
		}
	}

	// Check status code
	if resp.StatusCode != http.StatusOK {
		return TestResult{
			Provider:     provider,
			Model:        model,
			Endpoint:     "non-streaming",
			Success:      false,
			Error:        fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
			ResponseTime: time.Since(start),
		}
	}

	// Parse response
	var testResp TestResponse
	if err := json.Unmarshal(body, &testResp); err != nil {
		return TestResult{
			Provider:     provider,
			Model:        model,
			Endpoint:     "non-streaming",
			Success:      false,
			Error:        fmt.Sprintf("Failed to parse response: %v", err),
			ResponseTime: time.Since(start),
		}
	}

	return TestResult{
		Provider:     provider,
		Model:        model,
		Endpoint:     "non-streaming",
		Success:      true,
		ResponseTime: time.Since(start),
		Response:     testResp.Text,
		Usage:        testResp.Usage,
		Metadata:     testResp.Metadata,
	}
}

func testStreaming(config TestConfig, provider, model string) TestResult {
	start := time.Now()

	// Create request
	reqBody := TestRequest{
		Model:  model,
		Prompt: config.TestPrompt,
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return TestResult{
			Provider: provider,
			Model:    model,
			Endpoint: "streaming",
			Success:  false,
			Error:    fmt.Sprintf("Failed to marshal request: %v", err),
		}
	}

	// Create HTTP request
	ctx, cancel := context.WithTimeout(context.Background(), config.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "POST", config.BaseURL+"/v1/generate/stream", bytes.NewBuffer(jsonBody))
	if err != nil {
		return TestResult{
			Provider: provider,
			Model:    model,
			Endpoint: "streaming",
			Success:  false,
			Error:    fmt.Sprintf("Failed to create request: %v", err),
		}
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.APIKey)

	// Make request
	client := &http.Client{Timeout: config.Timeout}
	resp, err := client.Do(req)
	if err != nil {
		return TestResult{
			Provider:     provider,
			Model:        model,
			Endpoint:     "streaming",
			Success:      false,
			Error:        fmt.Sprintf("Request failed: %v", err),
			ResponseTime: time.Since(start),
		}
	}
	defer resp.Body.Close()

	// Check status code
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return TestResult{
			Provider:     provider,
			Model:        model,
			Endpoint:     "streaming",
			Success:      false,
			Error:        fmt.Sprintf("HTTP %d: %s", resp.StatusCode, string(body)),
			ResponseTime: time.Since(start),
		}
	}

	// Read streaming response
	var responseBuilder strings.Builder
	buffer := make([]byte, 1024)

	for {
		n, err := resp.Body.Read(buffer)
		if n > 0 {
			responseBuilder.Write(buffer[:n])
		}
		if err != nil {
			if err == io.EOF {
				break
			}
			return TestResult{
				Provider:     provider,
				Model:        model,
				Endpoint:     "streaming",
				Success:      false,
				Error:        fmt.Sprintf("Failed to read stream: %v", err),
				ResponseTime: time.Since(start),
			}
		}
	}

	response := responseBuilder.String()

	// Check if we got any content
	if response == "" {
		return TestResult{
			Provider:     provider,
			Model:        model,
			Endpoint:     "streaming",
			Success:      false,
			Error:        "No content received from stream",
			ResponseTime: time.Since(start),
		}
	}

	return TestResult{
		Provider:     provider,
		Model:        model,
		Endpoint:     "streaming",
		Success:      true,
		ResponseTime: time.Since(start),
		Response:     response,
	}
}

func printSummary(results []TestResult) {
	fmt.Println("\n\n📊 Test Summary")
	fmt.Println("===============")

	successCount := 0
	totalCount := len(results)

	for _, result := range results {
		if result.Success {
			successCount++
		}
	}

	fmt.Printf("Total Tests: %d\n", totalCount)
	fmt.Printf("Successful: %d\n", successCount)
	fmt.Printf("Failed: %d\n", totalCount-successCount)
	fmt.Printf("Success Rate: %.1f%%\n", float64(successCount)/float64(totalCount)*100)

	fmt.Println("\nDetailed Results:")
	fmt.Println("=================")

	for _, result := range results {
		status := "✅ PASS"
		if !result.Success {
			status = "❌ FAIL"
		}
		fmt.Printf("%s %s %s (%s) - %.2fs\n",
			status,
			result.Provider,
			result.Endpoint,
			result.Model,
			result.ResponseTime.Seconds())

		if !result.Success {
			fmt.Printf("   Error: %s\n", result.Error)
		}
	}

	if successCount == totalCount {
		fmt.Println("\n🎉 All tests passed! The API is working correctly.")
	} else {
		fmt.Printf("\n⚠️  %d tests failed. Please check the errors above.\n", totalCount-successCount)
		os.Exit(1)
	}
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
