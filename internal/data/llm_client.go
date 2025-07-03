package data

import (
	"context"
	"fmt"
	"io"
)

// LLMClient interface defines the contract for all LLM provider clients
type LLMClient interface {
	// GenerateWithParams generates text using the specified parameters
	GenerateWithParams(ctx context.Context, params map[string]interface{}) (*GenerateResponse, error)

	// GenerateStream generates text with streaming response
	GenerateStream(ctx context.Context, params map[string]interface{}) (*StreamResponse, error)
}

// ProviderError represents errors from LLM providers with additional context
type ProviderError struct {
	Provider   string `json:"provider"`
	ModelID    string `json:"model_id"`
	StatusCode int    `json:"status_code,omitempty"`
	ErrorCode  string `json:"error_code,omitempty"`
	Message    string `json:"message"`
	Retryable  bool   `json:"retryable"`
}

// Error implements the error interface
func (e *ProviderError) Error() string {
	return fmt.Sprintf("[%s:%s] %s", e.Provider, e.ModelID, e.Message)
}

// GenerateResponse represents the response from an LLM generation request
type GenerateResponse struct {
	Text         string            `json:"text"`
	InputTokens  int               `json:"input_tokens"`
	OutputTokens int               `json:"output_tokens"`
	Usage        *UsageInfo        `json:"usage,omitempty"`
	FinishReason string            `json:"finish_reason,omitempty"`
	ModelID      string            `json:"model_id"`
	Provider     string            `json:"provider"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// UsageInfo contains token usage information
type UsageInfo struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// GenerateParams represents the parameters for text generation
type GenerateParams struct {
	Model       string                 `json:"model"`
	Prompt      string                 `json:"prompt"`
	MaxTokens   int                    `json:"max_tokens,omitempty"`
	Temperature float64                `json:"temperature,omitempty"`
	TopP        float64                `json:"top_p,omitempty"`
	Stream      bool                   `json:"stream,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
}

// StreamResponse represents a streaming response from an LLM
type StreamResponse struct {
	Stream   io.ReadCloser     `json:"-"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// StreamChunk represents a single chunk in a streaming response
type StreamChunk struct {
	Content      string            `json:"content"`
	IsComplete   bool              `json:"is_complete"`
	FinishReason string            `json:"finish_reason,omitempty"`
	Usage        *UsageInfo        `json:"usage,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// NewClientForModel creates a specific provider client instance using the provided API key
func NewClientForModel(modelID, provider, apiKey string) (LLMClient, error) {
	switch provider {
	case "openai":
		return NewOpenAIClient(modelID, apiKey)
	case "anthropic":
		return NewAnthropicClient(modelID, apiKey)
	case "google":
		return NewGoogleClient(modelID, apiKey)
	default:
		return nil, fmt.Errorf("unsupported provider: %s", provider)
	}
}

// IsRetryableError checks if an error is retryable
func IsRetryableError(err error) bool {
	if providerErr, ok := err.(*ProviderError); ok {
		return providerErr.Retryable
	}
	return false
}

// GetProviderFromModelID extracts the provider from a model ID
func GetProviderFromModelID(modelID string) string {
	// Simple mapping - in production, this could be more sophisticated
	switch {
	case len(modelID) >= 3 && modelID[:3] == "gpt":
		return "openai"
	case len(modelID) >= 7 && modelID[:7] == "claude":
		return "anthropic"
	case len(modelID) >= 6 && modelID[:6] == "gemini":
		return "google"
	default:
		return "unknown"
	}
}
