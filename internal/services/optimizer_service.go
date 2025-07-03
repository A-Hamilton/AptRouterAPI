package llm

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/pkoukk/tiktoken-go"
)

// OptimizationResult holds detailed information about optimization
type OptimizationResult struct {
	OriginalText      string  `json:"original_text"`
	OptimizedText     string  `json:"optimized_text"`
	OriginalTokens    int     `json:"original_tokens"`
	OptimizedTokens   int     `json:"optimized_tokens"`
	TokensSaved       int     `json:"tokens_saved"`
	SavingsPercent    float64 `json:"savings_percent"`
	OptimizationType  string  `json:"optimization_type"`
	WasOptimized      bool    `json:"was_optimized"`
	FallbackReason    string  `json:"fallback_reason,omitempty"`
	OptimizedPrompt   string  `json:"optimized_prompt,omitempty"`
	OptimizedResponse string  `json:"optimized_response,omitempty"`
}

// Optimizer handles token optimization using a lightweight model
type Optimizer struct {
	client LLMClient
	model  string
}

// NewOptimizer creates a new optimizer instance
func NewOptimizer(model string, apiKey string) (*Optimizer, error) {
	// Use Google's Gemini Flash model for optimization (lightweight and efficient)
	client, err := NewGoogleClient(model, apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create optimizer client: %w", err)
	}

	return &Optimizer{
		client: client,
		model:  model,
	}, nil
}

// OptimizePrompt optimizes a user prompt for token efficiency
func (o *Optimizer) OptimizePrompt(ctx context.Context, originalPrompt string) (*OptimizationResult, error) {
	result := &OptimizationResult{
		OriginalText:     originalPrompt,
		OptimizationType: "prompt",
		WasOptimized:     false,
	}

	// Count original tokens
	originalTokens, err := o.CountTokens(originalPrompt)
	if err != nil {
		slog.Warn("Failed to count original tokens", "error", err)
		originalTokens = len(originalPrompt) / 4 // Rough estimate
	}
	result.OriginalTokens = originalTokens

	// Apply rule-based optimizations first
	ruleOptimized := o.applyRuleBasedOptimizations(originalPrompt)
	if ruleOptimized != originalPrompt {
		result.OptimizedText = ruleOptimized
		result.WasOptimized = true
		result.OptimizationType = "rule_based"
	} else {
		// Use AI-based optimization
		optimizationPrompt := o.buildPromptOptimizationPrompt(originalPrompt)

		params := map[string]interface{}{
			"prompt":      optimizationPrompt,
			"max_tokens":  300,
			"temperature": 0.1, // Very low temperature for consistent optimization
		}

		resp, err := o.client.GenerateWithParams(ctx, params)
		if err != nil {
			slog.Warn("AI prompt optimization failed, using rule-based", "error", err)
			result.FallbackReason = "ai_optimization_failed"
			result.OptimizedText = ruleOptimized
			result.WasOptimized = ruleOptimized != originalPrompt
			if result.WasOptimized {
				result.OptimizationType = "rule_based_fallback"
			}
		} else {
			optimizedPrompt := strings.TrimSpace(resp.Text)
			// Clean up the response - remove quotes and extra formatting
			optimizedPrompt = o.cleanOptimizedResponse(optimizedPrompt)

			if optimizedPrompt != "" && optimizedPrompt != originalPrompt {
				result.OptimizedText = optimizedPrompt
				result.WasOptimized = true
				result.OptimizationType = "ai_based"
				result.OptimizedPrompt = optimizedPrompt
			} else {
				result.OptimizedText = ruleOptimized
				result.WasOptimized = ruleOptimized != originalPrompt
				if result.WasOptimized {
					result.OptimizationType = "rule_based_fallback"
				}
			}
		}
	}

	// Count optimized tokens
	optimizedTokens, err := o.CountTokens(result.OptimizedText)
	if err != nil {
		slog.Warn("Failed to count optimized tokens", "error", err)
		optimizedTokens = len(result.OptimizedText) / 4 // Rough estimate
	}
	result.OptimizedTokens = optimizedTokens

	// Calculate savings
	result.TokensSaved = originalTokens - optimizedTokens
	if originalTokens > 0 {
		result.SavingsPercent = float64(result.TokensSaved) / float64(originalTokens) * 100
	}

	if result.WasOptimized {
		slog.Info("Prompt optimization completed",
			"original_tokens", originalTokens,
			"optimized_tokens", optimizedTokens,
			"tokens_saved", result.TokensSaved,
			"savings_percent", fmt.Sprintf("%.1f%%", result.SavingsPercent),
			"optimization_type", result.OptimizationType)
	}

	return result, nil
}

// OptimizeResponse optimizes a model response for token efficiency
func (o *Optimizer) OptimizeResponse(ctx context.Context, originalResponse string) (*OptimizationResult, error) {
	result := &OptimizationResult{
		OriginalText:     originalResponse,
		OptimizationType: "response",
		WasOptimized:     false,
	}

	// Count original tokens
	originalTokens, err := o.CountTokens(originalResponse)
	if err != nil {
		slog.Warn("Failed to count original tokens", "error", err)
		originalTokens = len(originalResponse) / 4 // Rough estimate
	}
	result.OriginalTokens = originalTokens

	// Apply rule-based optimizations first
	ruleOptimized := o.applyRuleBasedOptimizations(originalResponse)
	if ruleOptimized != originalResponse {
		result.OptimizedText = ruleOptimized
		result.WasOptimized = true
		result.OptimizationType = "rule_based"
	} else {
		// Use AI-based optimization with token savings estimation
		optimizationPrompt := o.buildResponseOptimizationPromptWithEstimate(originalResponse)

		params := map[string]interface{}{
			"prompt":      optimizationPrompt,
			"max_tokens":  800,
			"temperature": 0.1, // Very low temperature for consistent optimization
		}

		resp, err := o.client.GenerateWithParams(ctx, params)
		if err != nil {
			slog.Warn("AI response optimization failed, using rule-based", "error", err)
			result.FallbackReason = "ai_optimization_failed"
			result.OptimizedText = ruleOptimized
			result.WasOptimized = ruleOptimized != originalResponse
			if result.WasOptimized {
				result.OptimizationType = "rule_based_fallback"
			}
		} else {
			optimizedResponse := strings.TrimSpace(resp.Text)
			// Clean up the response - remove quotes and extra formatting
			optimizedResponse = o.cleanOptimizedResponse(optimizedResponse)

			// Parse [tokens_saved]=... at the end if present
			tokensSavedEstimate := 0
			parsedText := optimizedResponse
			if idx := strings.LastIndex(optimizedResponse, "[tokens_saved]="); idx != -1 {
				endIdx := idx + len("[tokens_saved]=")
				numStr := ""
				for i := endIdx; i < len(optimizedResponse); i++ {
					if optimizedResponse[i] >= '0' && optimizedResponse[i] <= '9' {
						numStr += string(optimizedResponse[i])
					} else {
						break
					}
				}
				if numStr != "" {
					tokensSavedEstimate, _ = strconv.Atoi(numStr)
				}
				parsedText = strings.TrimSpace(optimizedResponse[:idx])
			}

			if parsedText != "" && parsedText != originalResponse {
				result.OptimizedText = parsedText
				result.WasOptimized = true
				result.OptimizationType = "ai_based"
				// Attach the model's estimate
				result.FallbackReason = "model_estimate"
				// Add to result as OutputTokensSavedEstimate
				result.TokensSaved = tokensSavedEstimate // This is the model's estimate
				result.OptimizedTokens = 0               // Not known, but can be counted below
				result.SavingsPercent = 0                // Not known, but can be counted below
				result.OptimizationType = "ai_based_with_estimate"
				result.OptimizedResponse = optimizedResponse
			} else {
				result.OptimizedText = ruleOptimized
				result.WasOptimized = ruleOptimized != originalResponse
				if result.WasOptimized {
					result.OptimizationType = "rule_based_fallback"
				}
			}
		}
	}

	// Count optimized tokens
	optimizedTokens, err := o.CountTokens(result.OptimizedText)
	if err != nil {
		slog.Warn("Failed to count optimized tokens", "error", err)
		optimizedTokens = len(result.OptimizedText) / 4 // Rough estimate
	}
	result.OptimizedTokens = optimizedTokens

	// Calculate savings
	result.TokensSaved = originalTokens - optimizedTokens
	if originalTokens > 0 {
		result.SavingsPercent = float64(result.TokensSaved) / float64(originalTokens) * 100
	}

	if result.WasOptimized {
		slog.Info("Response optimization completed",
			"original_tokens", originalTokens,
			"optimized_tokens", optimizedTokens,
			"tokens_saved", result.TokensSaved,
			"savings_percent", fmt.Sprintf("%.1f%%", result.SavingsPercent),
			"optimization_type", result.OptimizationType)
	}

	return result, nil
}

// applyRuleBasedOptimizations applies rule-based token optimizations
func (o *Optimizer) applyRuleBasedOptimizations(text string) string {
	optimized := text

	// Remove unnecessary whitespace
	optimized = regexp.MustCompile(`\s+`).ReplaceAllString(optimized, " ")
	optimized = strings.TrimSpace(optimized)

	// Remove redundant punctuation
	optimized = regexp.MustCompile(`[.!?]+`).ReplaceAllString(optimized, ".")
	optimized = regexp.MustCompile(`[,;]+`).ReplaceAllString(optimized, ",")

	// Remove unnecessary words and phrases
	unnecessaryWords := []string{
		"please", "kindly", "if you could", "would you mind",
		"i would like to", "i want to", "i need to",
		"in order to", "so that", "in such a way that",
		"very", "really", "quite", "rather",
		"basically", "essentially", "fundamentally",
		"as a matter of fact", "in fact", "actually",
		"you know", "i mean", "like", "sort of", "kind of",
	}

	for _, word := range unnecessaryWords {
		pattern := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(word) + `\b`)
		optimized = pattern.ReplaceAllString(optimized, "")
	}

	// Remove redundant phrases
	redundantPhrases := map[string]string{
		"in the event that":            "if",
		"at this point in time":        "now",
		"due to the fact that":         "because",
		"in spite of the fact that":    "although",
		"with regard to":               "regarding",
		"with respect to":              "regarding",
		"in terms of":                  "regarding",
		"as far as .* is concerned":    "regarding",
		"it is important to note that": "",
		"it should be noted that":      "",
		"it is worth mentioning that":  "",
	}

	for phrase, replacement := range redundantPhrases {
		pattern := regexp.MustCompile(`(?i)` + phrase)
		optimized = pattern.ReplaceAllString(optimized, replacement)
	}

	// Clean up multiple spaces again
	optimized = regexp.MustCompile(`\s+`).ReplaceAllString(optimized, " ")
	optimized = strings.TrimSpace(optimized)

	return optimized
}

// buildPromptOptimizationPrompt creates an optimized prompt for prompt optimization
func (o *Optimizer) buildPromptOptimizationPrompt(originalPrompt string) string {
	return fmt.Sprintf(`Optimize for token efficiency while preserving meaning.

"%s"

Optimized:`, originalPrompt)
}

// buildResponseOptimizationPromptWithEstimate builds a prompt for the model to optimize and estimate tokens saved
func (o *Optimizer) buildResponseOptimizationPromptWithEstimate(originalResponse string) string {
	return fmt.Sprintf(`Rewrite for token efficiency. Append [tokens_saved]=<number> at end.

%s`, originalResponse)
}

// cleanOptimizedResponse cleans up the AI-generated optimization response
func (o *Optimizer) cleanOptimizedResponse(response string) string {
	// Remove quotes if the response is wrapped in them
	response = strings.Trim(response, `"'`)

	// Remove common AI response prefixes
	prefixes := []string{
		"Optimized prompt:", "Optimized response:", "Optimized version:",
		"Here's the optimized version:", "The optimized version is:",
	}

	for _, prefix := range prefixes {
		if strings.HasPrefix(response, prefix) {
			response = strings.TrimPrefix(response, prefix)
			response = strings.TrimSpace(response)
		}
	}

	return response
}

// ShouldOptimize determines if optimization should be attempted based on input length
func (o *Optimizer) ShouldOptimize(input string, threshold int) bool {
	return len(input) > threshold
}

// CountTokens counts tokens using the same method as other clients
func (o *Optimizer) CountTokens(text string) (int, error) {
	// Use the same token counting method as other clients
	encoding, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return 0, fmt.Errorf("failed to get encoding: %w", err)
	}

	tokens := encoding.Encode(text, nil, nil)
	return len(tokens), nil
}

// CalculateOptimizationSavings calculates the token savings from optimization
func (o *Optimizer) CalculateOptimizationSavings(originalTokens, optimizedTokens int) (int, float64) {
	if originalTokens == 0 {
		return 0, 0
	}

	savings := originalTokens - optimizedTokens
	savingsPercent := float64(savings) / float64(originalTokens) * 100

	return savings, savingsPercent
}

// OptimizePromptWithMode optimizes a user prompt for token efficiency with a given mode
func (o *Optimizer) OptimizePromptWithMode(ctx context.Context, originalPrompt string, mode string) (*OptimizationResult, error) {
	if mode != "efficiency" {
		mode = "context"
	}
	return o.optimizePromptWithMode(ctx, originalPrompt, mode)
}

func (o *Optimizer) optimizePromptWithMode(ctx context.Context, originalPrompt string, mode string) (*OptimizationResult, error) {
	result := &OptimizationResult{
		OriginalText:     originalPrompt,
		OptimizationType: "prompt",
		WasOptimized:     false,
	}

	// Count original tokens
	originalTokens, err := o.CountTokens(originalPrompt)
	if err != nil {
		slog.Warn("Failed to count original tokens", "error", err)
		originalTokens = len(originalPrompt) / 4 // Rough estimate
	}
	result.OriginalTokens = originalTokens

	// Apply rule-based optimizations first
	ruleOptimized := o.applyRuleBasedOptimizations(originalPrompt)
	if ruleOptimized != originalPrompt {
		result.OptimizedText = ruleOptimized
		result.WasOptimized = true
		result.OptimizationType = "rule_based"
	} else {
		// Use AI-based optimization
		var optimizationPrompt string
		if mode == "efficiency" {
			optimizationPrompt = o.buildPromptOptimizationPromptEfficiency(originalPrompt)
		} else {
			optimizationPrompt = o.buildPromptOptimizationPromptContext(originalPrompt)
		}

		params := map[string]interface{}{
			"prompt":      optimizationPrompt,
			"max_tokens":  300,
			"temperature": 0.1, // Very low temperature for consistent optimization
		}

		resp, err := o.client.GenerateWithParams(ctx, params)
		if err != nil {
			slog.Warn("AI prompt optimization failed, using rule-based", "error", err)
			result.FallbackReason = "ai_optimization_failed"
			result.OptimizedText = ruleOptimized
			result.WasOptimized = ruleOptimized != originalPrompt
			if result.WasOptimized {
				result.OptimizationType = "rule_based_fallback"
			}
		} else {
			optimizedPrompt := strings.TrimSpace(resp.Text)
			// Clean up the response - remove quotes and extra formatting
			optimizedPrompt = o.cleanOptimizedResponse(optimizedPrompt)

			if optimizedPrompt != "" && optimizedPrompt != originalPrompt {
				result.OptimizedText = optimizedPrompt
				result.WasOptimized = true
				result.OptimizationType = "ai_based"
				result.OptimizedPrompt = optimizedPrompt
			} else {
				result.OptimizedText = ruleOptimized
				result.WasOptimized = ruleOptimized != originalPrompt
				if result.WasOptimized {
					result.OptimizationType = "rule_based_fallback"
				}
			}
		}
	}

	// Count optimized tokens
	optimizedTokens, err := o.CountTokens(result.OptimizedText)
	if err != nil {
		slog.Warn("Failed to count optimized tokens", "error", err)
		optimizedTokens = len(result.OptimizedText) / 4 // Rough estimate
	}
	result.OptimizedTokens = optimizedTokens

	// Calculate savings
	result.TokensSaved = originalTokens - optimizedTokens
	if originalTokens > 0 {
		result.SavingsPercent = float64(result.TokensSaved) / float64(originalTokens) * 100
	}

	if result.WasOptimized {
		slog.Info("Prompt optimization completed",
			"original_tokens", originalTokens,
			"optimized_tokens", optimizedTokens,
			"tokens_saved", result.TokensSaved,
			"savings_percent", fmt.Sprintf("%.1f%%", result.SavingsPercent),
			"optimization_type", result.OptimizationType)
	}

	return result, nil
}

// OptimizeResponseWithMode optimizes a model response for token efficiency with a given mode
func (o *Optimizer) OptimizeResponseWithMode(ctx context.Context, originalResponse string, mode string) (*OptimizationResult, error) {
	if mode != "efficiency" {
		mode = "context"
	}
	return o.optimizeResponseWithMode(ctx, originalResponse, mode)
}

func (o *Optimizer) optimizeResponseWithMode(ctx context.Context, originalResponse string, mode string) (*OptimizationResult, error) {
	result := &OptimizationResult{
		OriginalText:     originalResponse,
		OptimizationType: "response",
		WasOptimized:     false,
	}

	// Count original tokens
	originalTokens, err := o.CountTokens(originalResponse)
	if err != nil {
		slog.Warn("Failed to count original tokens", "error", err)
		originalTokens = len(originalResponse) / 4 // Rough estimate
	}
	result.OriginalTokens = originalTokens

	// Apply rule-based optimizations first
	ruleOptimized := o.applyRuleBasedOptimizations(originalResponse)
	if ruleOptimized != originalResponse {
		result.OptimizedText = ruleOptimized
		result.WasOptimized = true
		result.OptimizationType = "rule_based"
	} else {
		// Use AI-based optimization with token savings estimation
		var optimizationPrompt string
		if mode == "efficiency" {
			optimizationPrompt = o.buildResponseOptimizationPromptEfficiency(originalResponse)
		} else {
			optimizationPrompt = o.buildResponseOptimizationPromptContext(originalResponse)
		}

		params := map[string]interface{}{
			"prompt":      optimizationPrompt,
			"max_tokens":  800,
			"temperature": 0.1, // Very low temperature for consistent optimization
		}

		resp, err := o.client.GenerateWithParams(ctx, params)
		if err != nil {
			slog.Warn("AI response optimization failed, using rule-based", "error", err)
			result.FallbackReason = "ai_optimization_failed"
			result.OptimizedText = ruleOptimized
			result.WasOptimized = ruleOptimized != originalResponse
			if result.WasOptimized {
				result.OptimizationType = "rule_based_fallback"
			}
		} else {
			optimizedResponse := strings.TrimSpace(resp.Text)
			// Clean up the response - remove quotes and extra formatting
			optimizedResponse = o.cleanOptimizedResponse(optimizedResponse)

			// Parse [tokens_saved]=... at the end if present
			tokensSavedEstimate := 0
			parsedText := optimizedResponse
			if idx := strings.LastIndex(optimizedResponse, "[tokens_saved]="); idx != -1 {
				endIdx := idx + len("[tokens_saved]=")
				numStr := ""
				for i := endIdx; i < len(optimizedResponse); i++ {
					if optimizedResponse[i] >= '0' && optimizedResponse[i] <= '9' {
						numStr += string(optimizedResponse[i])
					} else {
						break
					}
				}
				if numStr != "" {
					tokensSavedEstimate, _ = strconv.Atoi(numStr)
				}
				parsedText = strings.TrimSpace(optimizedResponse[:idx])
			}

			if parsedText != "" && parsedText != originalResponse {
				result.OptimizedText = parsedText
				result.WasOptimized = true
				result.OptimizationType = "ai_based"
				// Attach the model's estimate
				result.FallbackReason = "model_estimate"
				// Add to result as OutputTokensSavedEstimate
				result.TokensSaved = tokensSavedEstimate // This is the model's estimate
				result.OptimizedTokens = 0               // Not known, but can be counted below
				result.SavingsPercent = 0                // Not known, but can be counted below
				result.OptimizationType = "ai_based_with_estimate"
				result.OptimizedResponse = optimizedResponse
			} else {
				result.OptimizedText = ruleOptimized
				result.WasOptimized = ruleOptimized != originalResponse
				if result.WasOptimized {
					result.OptimizationType = "rule_based_fallback"
				}
			}
		}
	}

	// Count optimized tokens
	optimizedTokens, err := o.CountTokens(result.OptimizedText)
	if err != nil {
		slog.Warn("Failed to count optimized tokens", "error", err)
		optimizedTokens = len(result.OptimizedText) / 4 // Rough estimate
	}
	result.OptimizedTokens = optimizedTokens

	// Calculate savings
	result.TokensSaved = originalTokens - optimizedTokens
	if originalTokens > 0 {
		result.SavingsPercent = float64(result.TokensSaved) / float64(originalTokens) * 100
	}

	if result.WasOptimized {
		slog.Info("Response optimization completed",
			"original_tokens", originalTokens,
			"optimized_tokens", optimizedTokens,
			"tokens_saved", result.TokensSaved,
			"savings_percent", fmt.Sprintf("%.1f%%", result.SavingsPercent),
			"optimization_type", result.OptimizationType)
	}

	return result, nil
}

// buildPromptOptimizationPromptContext creates a context-preserving prompt for input optimization
func (o *Optimizer) buildPromptOptimizationPromptContext(originalPrompt string) string {
	return fmt.Sprintf(`Optimize this prompt for token efficiency while preserving context and clarity. Remove unnecessary words but keep essential information.

"%s"

Optimized:`, originalPrompt)
}

// buildPromptOptimizationPromptEfficiency creates an aggressive prompt for input optimization
func (o *Optimizer) buildPromptOptimizationPromptEfficiency(originalPrompt string) string {
	return fmt.Sprintf(`Aggressively minimize tokens. Keep only core information. Remove all non-essential context.

"%s"

Optimized:`, originalPrompt)
}

// buildResponseOptimizationPromptContext creates a context-preserving prompt for output optimization
func (o *Optimizer) buildResponseOptimizationPromptContext(originalResponse string) string {
	return fmt.Sprintf(`Rewrite for token efficiency while preserving context. Append [tokens_saved]=<number> at end.

%s`, originalResponse)
}

// buildResponseOptimizationPromptEfficiency creates an aggressive prompt for output optimization
func (o *Optimizer) buildResponseOptimizationPromptEfficiency(originalResponse string) string {
	return fmt.Sprintf(`Aggressively minimize tokens. Keep only core info. Append [tokens_saved]=<number> at end.

%s`, originalResponse)
}
