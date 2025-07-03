package util

import (
	"fmt"
	"strings"
	"sync"

	"github.com/pkoukk/tiktoken-go"
)

// TokenCounter provides token counting functionality with optimized caching
type TokenCounter struct {
	encodings map[string]*tiktoken.Tiktoken
	mu        sync.RWMutex
}

// NewTokenCounter creates a new token counter with thread-safe caching
func NewTokenCounter() *TokenCounter {
	return &TokenCounter{
		encodings: make(map[string]*tiktoken.Tiktoken),
	}
}

// Count counts tokens in the given text using the specified encoding
func (tc *TokenCounter) Count(text, encodingName string) (int, error) {
	encoding, err := tc.getEncoding(encodingName)
	if err != nil {
		return 0, err
	}

	// Use a pre-allocated slice for better performance
	tokens := encoding.Encode(text, nil, nil)
	return len(tokens), nil
}

// CountForModel counts tokens using the appropriate encoding for the given model
func (tc *TokenCounter) CountForModel(text, modelID string) (int, error) {
	encodingName := tc.getEncodingForModel(modelID)
	return tc.Count(text, encodingName)
}

// getEncoding returns the encoding for the given name, caching it for performance
func (tc *TokenCounter) getEncoding(encodingName string) (*tiktoken.Tiktoken, error) {
	// Try to get from cache first (read lock)
	tc.mu.RLock()
	if encoding, exists := tc.encodings[encodingName]; exists {
		tc.mu.RUnlock()
		return encoding, nil
	}
	tc.mu.RUnlock()

	// Not in cache, acquire write lock and create
	tc.mu.Lock()
	defer tc.mu.Unlock()

	// Double-check after acquiring write lock
	if encoding, exists := tc.encodings[encodingName]; exists {
		return encoding, nil
	}

	encoding, err := tiktoken.GetEncoding(encodingName)
	if err != nil {
		return nil, fmt.Errorf("failed to get encoding %s: %w", encodingName, err)
	}

	tc.encodings[encodingName] = encoding
	return encoding, nil
}

// getEncodingForModel returns the appropriate encoding name for the given model
func (tc *TokenCounter) getEncodingForModel(modelID string) string {
	// Convert to lowercase once for efficiency
	modelID = strings.ToLower(modelID)

	switch {
	case strings.HasPrefix(modelID, "gpt-4"), strings.HasPrefix(modelID, "gpt-3.5"):
		return "cl100k_base" // OpenAI models
	case strings.HasPrefix(modelID, "claude"):
		return "cl100k_base" // Anthropic models
	case strings.HasPrefix(modelID, "gemini"):
		return "cl100k_base" // Google models (approximation)
	default:
		return "cl100k_base" // Default to OpenAI encoding
	}
}

// EstimateTokens estimates tokens based on character count (fallback method)
// Optimized to avoid unnecessary allocations
func (tc *TokenCounter) EstimateTokens(text string) int {
	// Rough estimation: 1 token ≈ 4 characters for English text
	// This is a conservative estimate
	// Use bit shift for division by 4 (more efficient)
	return len(text) >> 2
}

// CountTokens is a convenience function for quick token counting
// Uses a global instance for better performance
var globalTokenCounter = NewTokenCounter()

func CountTokens(text, modelID string) (int, error) {
	return globalTokenCounter.CountForModel(text, modelID)
}

// EstimateTokensFromChars is a convenience function for character-based estimation
func EstimateTokensFromChars(text string) int {
	return globalTokenCounter.EstimateTokens(text)
}

// PreloadEncodings preloads common encodings to avoid cold start delays
func (tc *TokenCounter) PreloadEncodings() error {
	commonEncodings := []string{"cl100k_base", "p50k_base", "r50k_base"}

	for _, encoding := range commonEncodings {
		if _, err := tc.getEncoding(encoding); err != nil {
			return fmt.Errorf("failed to preload encoding %s: %w", encoding, err)
		}
	}

	return nil
}

// ClearCache clears the encoding cache (useful for memory management)
func (tc *TokenCounter) ClearCache() {
	tc.mu.Lock()
	defer tc.mu.Unlock()
	tc.encodings = make(map[string]*tiktoken.Tiktoken)
}
