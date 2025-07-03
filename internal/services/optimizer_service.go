package services

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	"github.com/apt-router/api/internal/data"
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
	// New fields for actual API token counts
	Gemma3InputTokens    int `json:"gemma3_input_tokens,omitempty"`     // Actual input tokens from Gemma 3 API response
	UserModelInputTokens int `json:"user_model_input_tokens,omitempty"` // Actual input tokens from user's model API response
}

// Optimizer handles token optimization using a lightweight model
type Optimizer struct {
	client data.LLMClient
	model  string
}

// NewOptimizer creates a new optimizer instance
func NewOptimizer(model string, apiKey string) (*Optimizer, error) {
	// Use Google's Gemini Flash model for optimization (lightweight and efficient)
	client, err := data.NewGoogleClient(model, apiKey)
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

	// Apply rule-based optimizations first (no API call)
	ruleOptimized := o.applyRuleBasedOptimizations(originalPrompt)
	if ruleOptimized != originalPrompt {
		result.OptimizedText = ruleOptimized
		result.WasOptimized = true
		result.OptimizationType = "rule_based"
		// Use rough estimation for rule-based optimization (no API call)
		result.OriginalTokens = len(originalPrompt) / 4
		result.OptimizedTokens = len(ruleOptimized) / 4
		result.TokensSaved = result.OriginalTokens - result.OptimizedTokens
		if result.OriginalTokens > 0 {
			result.SavingsPercent = float64(result.TokensSaved) / float64(result.OriginalTokens) * 100
		}
		return result, nil
	}

	// Use AI-based optimization - ONLY ONE API CALL to Gemma 3
	optimizationPrompt := o.buildPromptOptimizationPrompt(originalPrompt)
	params := map[string]interface{}{
		"prompt":      optimizationPrompt,
		"max_tokens":  300,
		"temperature": 0.1,
	}

	resp, err := o.client.GenerateWithParams(ctx, params)
	if err != nil {
		slog.Warn("AI prompt optimization failed, using original prompt", "error", err)
		result.FallbackReason = "ai_optimization_failed"
		result.OptimizedText = originalPrompt
		result.WasOptimized = false
		result.OriginalTokens = len(originalPrompt) / 4
		result.OptimizedTokens = result.OriginalTokens
		result.TokensSaved = 0
		result.SavingsPercent = 0
		return result, nil
	}

	optimizedPrompt := strings.TrimSpace(resp.Text)
	optimizedPrompt = o.cleanOptimizedResponse(optimizedPrompt)

	if optimizedPrompt != "" && optimizedPrompt != originalPrompt {
		result.OptimizedText = optimizedPrompt
		result.WasOptimized = true
		result.OptimizationType = "ai_based"
		result.OptimizedPrompt = optimizedPrompt

		// Use rough estimation for AI-based optimization (no additional API call)
		result.OriginalTokens = len(originalPrompt) / 4
		result.OptimizedTokens = len(optimizedPrompt) / 4
		result.TokensSaved = result.OriginalTokens - result.OptimizedTokens
		if result.OriginalTokens > 0 {
			result.SavingsPercent = float64(result.TokensSaved) / float64(result.OriginalTokens) * 100
		}

		slog.Info("Prompt optimization completed with ONE API call to Gemma 3",
			"original_tokens", result.OriginalTokens,
			"optimized_tokens", result.OptimizedTokens,
			"tokens_saved", result.TokensSaved,
			"savings_percent", fmt.Sprintf("%.1f%%", result.SavingsPercent),
			"optimization_type", result.OptimizationType,
			"api_calls", "1")
	} else {
		result.OptimizedText = originalPrompt
		result.WasOptimized = false
		result.OriginalTokens = len(originalPrompt) / 4
		result.OptimizedTokens = result.OriginalTokens
		result.TokensSaved = 0
		result.SavingsPercent = 0
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

	// Apply rule-based optimizations first (no API call)
	ruleOptimized := o.applyRuleBasedOptimizations(originalResponse)
	if ruleOptimized != originalResponse {
		result.OptimizedText = ruleOptimized
		result.WasOptimized = true
		result.OptimizationType = "rule_based"
		// Use rough estimation for rule-based optimization (no API call)
		result.OriginalTokens = len(originalResponse) / 4
		result.OptimizedTokens = len(ruleOptimized) / 4
		result.TokensSaved = result.OriginalTokens - result.OptimizedTokens
		if result.OriginalTokens > 0 {
			result.SavingsPercent = float64(result.TokensSaved) / float64(result.OriginalTokens) * 100
		}
		return result, nil
	}

	// Use AI-based optimization - ONLY ONE API CALL to Gemma 3
	optimizationPrompt := o.buildResponseOptimizationPromptWithEstimate(originalResponse)

	params := map[string]interface{}{
		"prompt":      optimizationPrompt,
		"max_tokens":  800,
		"temperature": 0.1, // Very low temperature for consistent optimization
	}

	resp, err := o.client.GenerateWithParams(ctx, params)
	if err != nil {
		slog.Warn("AI response optimization failed, using original response", "error", err)
		result.FallbackReason = "ai_optimization_failed"
		result.OptimizedText = originalResponse
		result.WasOptimized = false
		result.OriginalTokens = len(originalResponse) / 4
		result.OptimizedTokens = result.OriginalTokens
		result.TokensSaved = 0
		result.SavingsPercent = 0
		return result, nil
	}

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
		result.OptimizationType = "ai_based_with_estimate"
		result.OptimizedResponse = optimizedResponse

		// Use rough estimation for AI-based optimization (no additional API call)
		result.OriginalTokens = len(originalResponse) / 4
		result.OptimizedTokens = len(parsedText) / 4
		result.TokensSaved = result.OriginalTokens - result.OptimizedTokens
		if result.OriginalTokens > 0 {
			result.SavingsPercent = float64(result.TokensSaved) / float64(result.OriginalTokens) * 100
		}

		// If we got a model estimate, use it as additional info
		if tokensSavedEstimate > 0 {
			result.FallbackReason = fmt.Sprintf("model_estimate_%d", tokensSavedEstimate)
		}

		slog.Info("Response optimization completed with ONE API call to Gemma 3",
			"original_tokens", result.OriginalTokens,
			"optimized_tokens", result.OptimizedTokens,
			"tokens_saved", result.TokensSaved,
			"savings_percent", fmt.Sprintf("%.1f%%", result.SavingsPercent),
			"optimization_type", result.OptimizationType,
			"model_estimate", tokensSavedEstimate,
			"api_calls", "1")
	} else {
		result.OptimizedText = originalResponse
		result.WasOptimized = false
		result.OriginalTokens = len(originalResponse) / 4
		result.OptimizedTokens = result.OriginalTokens
		result.TokensSaved = 0
		result.SavingsPercent = 0
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

	// Apply rule-based optimizations first (no API call)
	ruleOptimized := o.applyRuleBasedOptimizations(originalPrompt)
	if ruleOptimized != originalPrompt {
		result.OptimizedText = ruleOptimized
		result.WasOptimized = true
		result.OptimizationType = "rule_based"
		// Use rough estimation for rule-based optimization (no API call)
		result.OriginalTokens = len(originalPrompt) / 4
		result.OptimizedTokens = len(ruleOptimized) / 4
		result.TokensSaved = result.OriginalTokens - result.OptimizedTokens
		if result.OriginalTokens > 0 {
			result.SavingsPercent = float64(result.TokensSaved) / float64(result.OriginalTokens) * 100
		}
		return result, nil
	}

	// Use AI-based optimization - ONLY ONE API CALL to Gemma 3
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
		slog.Warn("AI prompt optimization failed, using original prompt", "error", err)
		result.FallbackReason = "ai_optimization_failed"
		result.OptimizedText = originalPrompt
		result.WasOptimized = false
		result.OriginalTokens = len(originalPrompt) / 4
		result.OptimizedTokens = result.OriginalTokens
		result.TokensSaved = 0
		result.SavingsPercent = 0
		return result, nil
	}

	optimizedPrompt := strings.TrimSpace(resp.Text)
	// Clean up the response - remove quotes and extra formatting
	optimizedPrompt = o.cleanOptimizedResponse(optimizedPrompt)

	if optimizedPrompt != "" && optimizedPrompt != originalPrompt {
		result.OptimizedText = optimizedPrompt
		result.WasOptimized = true
		result.OptimizationType = "ai_based"
		result.OptimizedPrompt = optimizedPrompt

		// Use rough estimation for AI-based optimization (no additional API call)
		result.OriginalTokens = len(originalPrompt) / 4
		result.OptimizedTokens = len(optimizedPrompt) / 4
		result.TokensSaved = result.OriginalTokens - result.OptimizedTokens
		if result.OriginalTokens > 0 {
			result.SavingsPercent = float64(result.TokensSaved) / float64(result.OriginalTokens) * 100
		}

		slog.Info("Prompt optimization completed with ONE API call to Gemma 3",
			"original_tokens", result.OriginalTokens,
			"optimized_tokens", result.OptimizedTokens,
			"tokens_saved", result.TokensSaved,
			"savings_percent", fmt.Sprintf("%.1f%%", result.SavingsPercent),
			"optimization_type", result.OptimizationType,
			"mode", mode,
			"api_calls", "1")
	} else {
		result.OptimizedText = originalPrompt
		result.WasOptimized = false
		result.OriginalTokens = len(originalPrompt) / 4
		result.OptimizedTokens = result.OriginalTokens
		result.TokensSaved = 0
		result.SavingsPercent = 0
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

	// Apply rule-based optimizations first (no API call)
	ruleOptimized := o.applyRuleBasedOptimizations(originalResponse)
	if ruleOptimized != originalResponse {
		result.OptimizedText = ruleOptimized
		result.WasOptimized = true
		result.OptimizationType = "rule_based"
		// Use rough estimation for rule-based optimization (no API call)
		result.OriginalTokens = len(originalResponse) / 4
		result.OptimizedTokens = len(ruleOptimized) / 4
		result.TokensSaved = result.OriginalTokens - result.OptimizedTokens
		if result.OriginalTokens > 0 {
			result.SavingsPercent = float64(result.TokensSaved) / float64(result.OriginalTokens) * 100
		}
		return result, nil
	}

	// Use AI-based optimization - ONLY ONE API CALL to Gemma 3
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
		slog.Warn("AI response optimization failed, using original response", "error", err)
		result.FallbackReason = "ai_optimization_failed"
		result.OptimizedText = originalResponse
		result.WasOptimized = false
		result.OriginalTokens = len(originalResponse) / 4
		result.OptimizedTokens = result.OriginalTokens
		result.TokensSaved = 0
		result.SavingsPercent = 0
		return result, nil
	}

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
		result.OptimizationType = "ai_based_with_estimate"
		result.OptimizedResponse = optimizedResponse

		// Use rough estimation for AI-based optimization (no additional API call)
		result.OriginalTokens = len(originalResponse) / 4
		result.OptimizedTokens = len(parsedText) / 4
		result.TokensSaved = result.OriginalTokens - result.OptimizedTokens
		if result.OriginalTokens > 0 {
			result.SavingsPercent = float64(result.TokensSaved) / float64(result.OriginalTokens) * 100
		}

		// If we got a model estimate, use it as additional info
		if tokensSavedEstimate > 0 {
			result.FallbackReason = fmt.Sprintf("model_estimate_%d", tokensSavedEstimate)
		}

		slog.Info("Response optimization completed with ONE API call to Gemma 3",
			"original_tokens", result.OriginalTokens,
			"optimized_tokens", result.OptimizedTokens,
			"tokens_saved", result.TokensSaved,
			"savings_percent", fmt.Sprintf("%.1f%%", result.SavingsPercent),
			"optimization_type", result.OptimizationType,
			"mode", mode,
			"model_estimate", tokensSavedEstimate,
			"api_calls", "1")
	} else {
		result.OptimizedText = originalResponse
		result.WasOptimized = false
		result.OriginalTokens = len(originalResponse) / 4
		result.OptimizedTokens = result.OriginalTokens
		result.TokensSaved = 0
		result.SavingsPercent = 0
	}

	return result, nil
}

// buildPromptOptimizationPromptContext creates a context-preserving prompt for input optimization
func (o *Optimizer) buildPromptOptimizationPromptContext(originalPrompt string) string {
	return fmt.Sprintf(`Optimize for tokens. Keep context.

"%s"

Optimized:`, originalPrompt)
}

// buildPromptOptimizationPromptEfficiency creates an aggressive prompt for input optimization
func (o *Optimizer) buildPromptOptimizationPromptEfficiency(originalPrompt string) string {
	return fmt.Sprintf(`Minimize tokens. Core info only.

"%s"

Optimized:`, originalPrompt)
}

// buildResponseOptimizationPromptContext creates a context-preserving prompt for output optimization
func (o *Optimizer) buildResponseOptimizationPromptContext(originalResponse string) string {
	return fmt.Sprintf(`Rewrite efficiently. Append [tokens_saved]=<number>.

%s`, originalResponse)
}

// buildResponseOptimizationPromptEfficiency creates an aggressive prompt for output optimization
func (o *Optimizer) buildResponseOptimizationPromptEfficiency(originalResponse string) string {
	return fmt.Sprintf(`Minimize tokens. Append [tokens_saved]=<number>.

%s`, originalResponse)
}
