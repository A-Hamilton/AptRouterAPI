package services

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/apt-router/api/internal/data"
	"github.com/apt-router/api/internal/utils"
	"github.com/google/uuid"
	"github.com/patrickmn/go-cache"
)

// RequestContext contains request-scoped data (shared with handlers)
type RequestContext struct {
	RequestID   string
	UserID      string
	APIKeyID    string
	PricingTier PricingTier
	Logger      *slog.Logger
	// Cached user data for performance
	CachedUser *CachedUserData
}

// CachedUserData contains frequently accessed user information
type CachedUserData struct {
	ID            string    `json:"id"`
	Email         string    `json:"email"`
	Balance       float64   `json:"balance"`
	TierID        string    `json:"tier_id"`
	IsActive      bool      `json:"is_active"`
	CustomPricing bool      `json:"custom_pricing"`
	LastUpdated   time.Time `json:"last_updated"`
}

// GenerationService handles the business logic for text generation
type GenerationService struct {
	config          *utils.Config
	firebaseService *data.Service
	cache           *cache.Cache
	pricingService  *PricingService
	optimizer       *Optimizer
}

// NewGenerationService creates a new generation service
func NewGenerationService(
	cfg *utils.Config,
	firebaseService *data.Service,
	cache *cache.Cache,
	pricingService *PricingService,
) *GenerationService {
	// Initialize optimizer with Gemma model
	optimizer, err := NewOptimizer("gemma-3-27b-it", cfg.LLM.GoogleAPIKey)
	if err != nil {
		slog.Error("Failed to initialize optimizer", "error", err)
		// Continue without optimizer if it fails
		optimizer = nil
	}

	return &GenerationService{
		config:          cfg,
		firebaseService: firebaseService,
		cache:           cache,
		pricingService:  pricingService,
		optimizer:       optimizer,
	}
}

// GenerationRequest represents a text generation request
type GenerationRequest struct {
	Model            string                 `json:"model"`
	Prompt           string                 `json:"prompt"`
	MaxTokens        int                    `json:"max_tokens"`
	Temperature      float64                `json:"temperature"`
	TopP             float64                `json:"top_p"`
	Stream           bool                   `json:"stream"`
	Extra            map[string]interface{} `json:"extra,omitempty"`
	OpenAIAPIKey     string                 `json:"openai_api_key,omitempty"`
	AnthropicAPIKey  string                 `json:"anthropic_api_key,omitempty"`
	GoogleAPIKey     string                 `json:"google_api_key,omitempty"`
	OptimizationMode string                 `json:"optimization_mode,omitempty"`
}

// GenerationResponse represents a text generation response
type GenerationResponse struct {
	ID           string                 `json:"id"`
	Text         string                 `json:"text"`
	Model        string                 `json:"model"`
	Provider     string                 `json:"provider"`
	Usage        *ServiceUsageInfo      `json:"usage,omitempty"`
	FinishReason string                 `json:"finish_reason,omitempty"`
	CreatedAt    int64                  `json:"created_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// ServiceUsageInfo contains token usage information for the service layer
type ServiceUsageInfo struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// GenerationResult contains the result of a generation request
type GenerationResult struct {
	Response                   *GenerationResponse
	WasOptimized               bool
	OptimizationStatus         string
	FallbackReason             string
	PromptOptimizationResult   *OptimizationResult
	ResponseOptimizationResult *OptimizationResult
}

// EnhancedStreamReader wraps the original stream to track tokens and usage
type EnhancedStreamReader struct {
	OriginalStream           io.ReadCloser
	ModelConfig              ModelConfig
	RequestCtx               *RequestContext
	InputTokens              int
	OutputTokens             int
	AccumulatedContent       strings.Builder
	WasOptimized             bool
	OptimizationStatus       string
	FallbackReason           string
	PromptOptimizationResult *OptimizationResult
	Closed                   bool
	UsageLogged              bool
	GenerationService        *GenerationService
	StartTime                time.Time
	// Token savings tracking
	InputTokensSaved  int
	OutputTokensSaved int
	TotalTokensSaved  int
}

func (r *EnhancedStreamReader) Read(p []byte) (n int, err error) {
	if r.Closed {
		return 0, io.EOF
	}

	// Read from original stream
	n, err = r.OriginalStream.Read(p)
	if n > 0 {
		// Accumulate content for token counting
		r.AccumulatedContent.Write(p[:n])
	}

	// If stream ended, mark for logging but don't log yet
	if err == io.EOF && !r.UsageLogged {
		r.UsageLogged = true
		// Don't call logUsage() here - defer it to Close()
	}

	return n, err
}

// Flush ensures all buffered data is written
func (r *EnhancedStreamReader) Flush() error {
	// If the underlying stream has a Flush method, call it
	if flusher, ok := r.OriginalStream.(interface{ Flush() error }); ok {
		return flusher.Flush()
	}
	return nil
}

func (r *EnhancedStreamReader) Close() error {
	r.Closed = true

	// Try to flush any remaining data before closing
	if err := r.Flush(); err != nil {
		r.RequestCtx.Logger.Warn("Failed to flush stream", "error", err)
	}

	// Log usage if not already logged - this ensures stream completes first
	if !r.UsageLogged {
		r.logUsage()
	}

	return r.OriginalStream.Close()
}

func (r *EnhancedStreamReader) logUsage() {
	// Try to get usage information from the streaming response
	if usageReader, ok := r.OriginalStream.(interface{ GetUsage() (int, int) }); ok {
		inputTokens, outputTokens := usageReader.GetUsage()
		if inputTokens > 0 || outputTokens > 0 {
			r.InputTokens = inputTokens
			r.OutputTokens = outputTokens
			r.RequestCtx.Logger.Info("EnhancedStreamReader: Using usage from streaming response",
				"input_tokens", inputTokens, "output_tokens", outputTokens)
		}
	}

	// If no usage from streaming, count output tokens from accumulated content
	if r.OutputTokens == 0 {
		outputTokens := 0 // Token estimation removed; only real API usage data is used
		r.OutputTokens = outputTokens
	}

	// If no input tokens from streaming, use tokenizer estimate
	if r.InputTokens == 0 {
		// For now, we'll use the tokenizer estimate for input tokens
		// This could be improved by capturing the original prompt and counting it
		r.RequestCtx.Logger.Info("EnhancedStreamReader: No input tokens from streaming, using tokenizer estimate")
	}

	// Calculate actual input token savings using real usage data from streaming response
	if r.PromptOptimizationResult != nil && r.PromptOptimizationResult.WasOptimized {
		// Get actual input tokens from the streaming response
		userModelInputTokens := r.InputTokens

		// CRITICAL: We can only calculate savings if we have real usage data
		// If we don't have real usage data, we cannot claim any savings
		if userModelInputTokens == 0 {
			r.RequestCtx.Logger.Warn("Cannot calculate input token savings - no real usage data available",
				"input_tokens", userModelInputTokens,
				"note", "Using real API usage data only, no estimators allowed")
			r.InputTokensSaved = 0
			r.TotalTokensSaved = r.OutputTokensSaved
			return
		}

		// Use real Gemma 3 API usage data for original tokens
		gemma3InputTokens := r.PromptOptimizationResult.Gemma3InputTokens
		if gemma3InputTokens == 0 {
			// Fallback to the original token count if no real Gemma 3 usage data
			gemma3InputTokens = r.PromptOptimizationResult.OriginalTokens
			r.RequestCtx.Logger.Warn("No real Gemma 3 usage data, using fallback token count",
				"gemma3_input_tokens", gemma3InputTokens,
				"note", "This may not be accurate - real API usage data preferred")
		}

		actualInputTokensSaved := gemma3InputTokens - userModelInputTokens
		if actualInputTokensSaved < 0 {
			actualInputTokensSaved = 0 // Don't show negative savings
		}

		// Update the input tokens saved with actual usage data
		r.InputTokensSaved = actualInputTokensSaved
		r.TotalTokensSaved = actualInputTokensSaved + r.OutputTokensSaved

		r.RequestCtx.Logger.Info("Updated input tokens saved with real API usage data",
			"gemma3_input_tokens", gemma3InputTokens,
			"user_model_input_tokens", userModelInputTokens,
			"input_tokens_saved", actualInputTokensSaved,
			"usage_source", "real_api_responses",
			"comparison_note", "Real Gemma3 usage vs actual user model usage")
	}

	// For output token savings, we need to use AI estimation since we only generate one response
	// Extract AI estimation of output tokens saved from the content
	outputTokensSaved := 0
	if strings.Contains(r.AccumulatedContent.String(), "tokens_saved=") {
		// Find the marker and extract the estimate
		startIdx := strings.Index(r.AccumulatedContent.String(), "tokens_saved=")
		if startIdx != -1 {
			startIdx += len("tokens_saved=")
			endIdx := startIdx
			// Find the end of the number
			for endIdx < len(r.AccumulatedContent.String()) && r.AccumulatedContent.String()[endIdx] >= '0' && r.AccumulatedContent.String()[endIdx] <= '9' {
				endIdx++
			}
			if endIdx > startIdx {
				if estimate, parseErr := strconv.Atoi(r.AccumulatedContent.String()[startIdx:endIdx]); parseErr == nil {
					outputTokensSaved = estimate
					r.RequestCtx.Logger.Info("Extracted AI estimation of output tokens saved", "estimate", outputTokensSaved)
				}
			}
		}
	}

	r.OutputTokensSaved = outputTokensSaved
	r.TotalTokensSaved = r.InputTokensSaved + outputTokensSaved

	r.RequestCtx.Logger.Info("Output token savings calculation",
		"actual_output_tokens", r.OutputTokens,
		"ai_estimated_output_tokens_saved", outputTokensSaved,
		"note", "Using AI estimation for output savings since only one response is generated")

	// Calculate actual cost using provider token counts
	actualCost := r.calculateActualCost(r.InputTokens, r.OutputTokens)

	// Log the streaming request completion with comprehensive token data
	r.RequestCtx.Logger.Info("Streaming request completed with usage",
		"user_id", r.RequestCtx.UserID,
		"model", r.ModelConfig.ModelID,
		"streaming", true,
		"input_tokens", r.InputTokens,
		"output_tokens", r.OutputTokens,
		"total_tokens", r.InputTokens+r.OutputTokens,
		"actual_cost", actualCost,
		"was_optimized", r.WasOptimized,
		"optimization_status", r.OptimizationStatus,
		"fallback_reason", r.FallbackReason,
		"input_tokens_saved", r.InputTokensSaved,
		"output_tokens_saved", r.OutputTokensSaved,
		"total_tokens_saved", r.TotalTokensSaved)

	// Log the request to Firebase
	r.logStreamingRequest(actualCost)

	// Charge the user
	r.chargeUser(actualCost)

	// Mark as logged
	r.UsageLogged = true

	// Add debug logs to output tokens saved parsing
	if strings.Contains(r.AccumulatedContent.String(), "tokens_saved=") {
		r.RequestCtx.Logger.Info("Streaming: Found tokens_saved marker in stream")
	}
	r.RequestCtx.Logger.Info("Streaming: Parsed output_tokens_saved", "output_tokens_saved", r.OutputTokensSaved)
	r.RequestCtx.Logger.Info("Streaming: Final input/output tokens saved", "input_tokens_saved", r.InputTokensSaved, "output_tokens_saved", r.OutputTokensSaved)
}

func (r *EnhancedStreamReader) calculateActualCost(inputTokens, outputTokens int) float64 {
	// Calculate base cost
	inputCost := float64(inputTokens) * r.ModelConfig.InputPricePerMillion / 1000000
	outputCost := float64(outputTokens) * r.ModelConfig.OutputPricePerMillion / 1000000
	baseCost := inputCost + outputCost

	// Apply pricing tier markups (percentage-based)
	inputMarkup := inputCost * (r.RequestCtx.PricingTier.InputMarkupPercent / 100)
	outputMarkup := outputCost * (r.RequestCtx.PricingTier.OutputMarkupPercent / 100)
	totalMarkup := inputMarkup + outputMarkup

	finalCost := baseCost + totalMarkup

	return finalCost
}

func (r *EnhancedStreamReader) logStreamingRequest(cost float64) {
	// Create request log
	log := &data.RequestLog{
		ID:                 r.RequestCtx.RequestID,
		UserID:             r.RequestCtx.UserID,
		APIKeyID:           r.RequestCtx.APIKeyID,
		RequestID:          r.RequestCtx.RequestID,
		ModelID:            r.ModelConfig.ModelID,
		Provider:           r.ModelConfig.Provider,
		InputTokens:        r.InputTokens,
		OutputTokens:       r.OutputTokens,
		TotalTokens:        r.InputTokens + r.OutputTokens,
		BaseCost:           cost / (1 + (r.RequestCtx.PricingTier.InputMarkupPercent+r.RequestCtx.PricingTier.OutputMarkupPercent)/100),
		MarkupAmount:       cost - (cost / (1 + (r.RequestCtx.PricingTier.InputMarkupPercent+r.RequestCtx.PricingTier.OutputMarkupPercent)/100)),
		TotalCost:          cost,
		TierID:             r.RequestCtx.PricingTier.ID,
		MarkupPercent:      (r.RequestCtx.PricingTier.InputMarkupPercent + r.RequestCtx.PricingTier.OutputMarkupPercent) / 2,
		WasOptimized:       r.WasOptimized,
		OptimizationStatus: r.OptimizationStatus,
		TokensSaved:        r.getTokensSaved(),
		SavingsAmount:      r.getSavingsAmount(),
		Streaming:          true,
		RequestTimestamp:   r.StartTime,
		ResponseTimestamp:  time.Now(),
		DurationMs:         time.Since(r.StartTime).Milliseconds(),
		Status:             "success",
		IPAddress:          "127.0.0.1", // Will be set by middleware
		UserAgent:          "streaming-client",
		Metadata: map[string]interface{}{
			"fallback_reason":     r.FallbackReason,
			"input_tokens_saved":  r.InputTokensSaved,
			"output_tokens_saved": r.OutputTokensSaved,
			"total_tokens_saved":  r.TotalTokensSaved,
		},
	}

	// Log to Firebase
	if err := r.GenerationService.firebaseService.LogRequest(context.Background(), log); err != nil {
		r.RequestCtx.Logger.Error("Failed to log streaming request", "error", err)
	}
}

func (r *EnhancedStreamReader) chargeUser(cost float64) {
	// Update user balance (allows negative balance)
	if err := r.GenerationService.firebaseService.UpdateUserBalance(context.Background(), r.RequestCtx.UserID, -cost); err != nil {
		r.RequestCtx.Logger.Error("Failed to update user balance", "error", err)
	}
}

// getTokensSaved calculates the total tokens saved from optimization
func (r *EnhancedStreamReader) getTokensSaved() int {
	if r.PromptOptimizationResult != nil && r.PromptOptimizationResult.WasOptimized {
		return r.PromptOptimizationResult.TokensSaved
	}
	return 0
}

// getSavingsAmount calculates the monetary savings from optimization
func (r *EnhancedStreamReader) getSavingsAmount() float64 {
	tokensSaved := r.getTokensSaved()
	if tokensSaved > 0 {
		// Calculate savings based on input token cost (since optimization affects input tokens)
		inputCostPerToken := r.ModelConfig.InputPricePerMillion / 1000000
		return float64(tokensSaved) * inputCostPerToken
	}
	return 0
}

// Generate handles the main generation logic with optimized billing
func (s *GenerationService) Generate(ctx context.Context, req *GenerationRequest, requestCtx *RequestContext) (*GenerationResult, error) {
	// Validate model
	if req.Model == "" {
		return nil, fmt.Errorf("model is required")
	}

	// Get model configuration
	modelConfig, err := s.pricingService.GetModelConfig(req.Model)
	if err != nil {
		return nil, fmt.Errorf("invalid model %s: %w", req.Model, err)
	}

	// Set defaults
	if req.MaxTokens == 0 {
		req.MaxTokens = 1000
	}
	if req.Temperature == 0 {
		req.Temperature = 0.7
	}
	if req.TopP == 0 {
		req.TopP = 1.0
	}

	// Pre-flight balance check (quick cache check before expensive operations)
	estimatedInputTokens := len(req.Prompt) / 4 // Rough estimate
	estimatedOutputTokens := req.MaxTokens
	estimatedCost := s.calculateEstimatedCost(estimatedInputTokens, estimatedOutputTokens, modelConfig, requestCtx.PricingTier)

	canProceed, currentBalance, err := s.checkUserBalance(ctx, requestCtx.UserID)
	if err != nil {
		return nil, fmt.Errorf("balance check failed: %w", err)
	}

	if !canProceed {
		return nil, fmt.Errorf("user account is inactive")
	}

	requestCtx.Logger.Info("Pre-flight balance check passed",
		"user_id", requestCtx.UserID,
		"current_balance", currentBalance,
		"estimated_cost", estimatedCost,
		"estimated_input_tokens", estimatedInputTokens,
		"estimated_output_tokens", estimatedOutputTokens,
	)

	// Check if streaming is requested
	if req.Stream {
		return nil, fmt.Errorf("streaming generation not yet implemented")
	}

	// Step 1: Optimize input prompt if optimization is enabled and prompt is long enough
	var promptOptimizationResult *OptimizationResult

	if s.optimizer != nil && s.config.Optimization.Enabled && s.optimizer.ShouldOptimize(req.Prompt, 50) {
		// Try to optimize the prompt
		optimizationResult, err := s.optimizer.OptimizePromptWithMode(ctx, req.Prompt, req.OptimizationMode)
		if err != nil {
			if s.config.Optimization.FallbackOnOptimizationFailure {
				requestCtx.Logger.Warn("Prompt optimization failed, using original prompt", "error", err)
				promptOptimizationResult = &OptimizationResult{
					OriginalText:     req.Prompt,
					OptimizedText:    req.Prompt,
					OriginalTokens:   0,
					OptimizedTokens:  0,
					TokensSaved:      0,
					SavingsPercent:   0,
					OptimizationType: "none",
					WasOptimized:     false,
					FallbackReason:   "optimization_failed",
				}
			} else {
				return nil, fmt.Errorf("prompt optimization failed: %w", err)
			}
		} else {
			promptOptimizationResult = optimizationResult
			if optimizationResult.WasOptimized {
				req.Prompt = optimizationResult.OptimizedText
				requestCtx.Logger.Info("Prompt optimized successfully",
					"original_tokens", optimizationResult.OriginalTokens,
					"optimized_tokens", optimizationResult.OptimizedTokens,
					"tokens_saved", optimizationResult.TokensSaved,
					"savings_percent", fmt.Sprintf("%.1f%%", optimizationResult.SavingsPercent))
			}
		}
	}

	// Add response optimization prompt to get AI estimate of output tokens saved
	if promptOptimizationResult != nil && promptOptimizationResult.WasOptimized {
		responseOptimizationPrompt := "\n\nIMPORTANT: Be concise and efficient. After your response, append exactly: tokens_saved=<number> where <number> is your estimate of how many tokens you saved by being concise compared to a verbose response."
		req.Prompt += responseOptimizationPrompt
		requestCtx.Logger.Info("Added response optimization prompt for AI estimation", "prompt_length", len(req.Prompt))
	}

	// Handle non-streaming generation
	result, err := s.handleNonStreamingGeneration(ctx, req, modelConfig, requestCtx)
	if err != nil {
		return nil, err
	}

	// Add optimization information to the result
	if promptOptimizationResult != nil {
		result.WasOptimized = promptOptimizationResult.WasOptimized
		result.OptimizationStatus = "success"
		result.FallbackReason = promptOptimizationResult.FallbackReason
		result.PromptOptimizationResult = promptOptimizationResult
	}
	return result, nil
}

// GenerateStream generates text with streaming response
func (s *GenerationService) GenerateStream(ctx context.Context, req *GenerationRequest, requestCtx *RequestContext) (*data.StreamResponse, error) {
	// Get model configuration
	modelConfig, err := s.pricingService.GetModelConfig(req.Model)
	if err != nil {
		return nil, fmt.Errorf("model config not found for model ID: %s", req.Model)
	}

	// Step 1: Quick optimization check - only optimize if prompt is very long and optimization is enabled
	var promptOptimizationResult *OptimizationResult
	originalPrompt := req.Prompt

	if s.optimizer != nil && s.config.Optimization.Enabled && s.optimizer.ShouldOptimize(req.Prompt, 100) { // Increased threshold
		// Create a quick optimization context with shorter timeout
		optCtx, optCancel := context.WithTimeout(ctx, 30*time.Second)

		// Try to optimize the prompt with a quick timeout
		optimizationResult, err := s.optimizer.OptimizePromptWithMode(optCtx, req.Prompt, req.OptimizationMode)
		optCancel() // Cancel immediately after optimization attempt

		if err != nil {
			if s.config.Optimization.FallbackOnOptimizationFailure {
				requestCtx.Logger.Warn("Prompt optimization failed, using original prompt", "error", err)
				promptOptimizationResult = &OptimizationResult{
					OriginalText:     req.Prompt,
					OptimizedText:    req.Prompt,
					OriginalTokens:   0,
					OptimizedTokens:  0,
					TokensSaved:      0,
					SavingsPercent:   0,
					OptimizationType: "none",
					WasOptimized:     false,
					FallbackReason:   "optimization_failed",
				}
			} else {
				return nil, fmt.Errorf("prompt optimization failed: %w", err)
			}
		} else {
			promptOptimizationResult = optimizationResult
			if optimizationResult.WasOptimized {
				req.Prompt = optimizationResult.OptimizedText
				requestCtx.Logger.Info("Prompt optimized successfully",
					"original_tokens", optimizationResult.OriginalTokens,
					"optimized_tokens", optimizationResult.OptimizedTokens,
					"tokens_saved", optimizationResult.TokensSaved,
					"savings_percent", fmt.Sprintf("%.1f%%", optimizationResult.SavingsPercent))
			}
		}
	} else {
		// No optimization needed or disabled
		promptOptimizationResult = &OptimizationResult{
			OriginalText:     req.Prompt,
			OptimizedText:    req.Prompt,
			OriginalTokens:   0,
			OptimizedTokens:  0,
			TokensSaved:      0,
			SavingsPercent:   0,
			OptimizationType: "none",
			WasOptimized:     false,
			FallbackReason:   "not_needed",
		}
	}

	// Add response optimization prompt only if optimization was actually used
	if promptOptimizationResult != nil && promptOptimizationResult.WasOptimized {
		responseOptimizationPrompt := "\n\nIMPORTANT: Be concise and efficient. After your response, append exactly: tokens_saved=<number> where <number> is your estimate of how many tokens you saved by being concise compared to a verbose response."
		req.Prompt += responseOptimizationPrompt
		requestCtx.Logger.Info("Added response optimization prompt for AI estimation", "prompt_length", len(req.Prompt))
	}

	// Step 2: Create LLM client
	client, err := s.createLLMClient(modelConfig, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Step 3: Prepare generation parameters with include_usage for streaming
	params := map[string]interface{}{
		"model":         req.Model,
		"prompt":        req.Prompt,
		"max_tokens":    req.MaxTokens,
		"temperature":   req.Temperature,
		"top_p":         req.TopP,
		"stream":        true,
		"include_usage": true, // Add this to get usage information in streaming
	}

	// Add any extra parameters
	for key, value := range req.Extra {
		params[key] = value
	}

	// Step 4: Generate streaming response with timeout
	streamCtx, streamCancel := context.WithTimeout(ctx, 8*time.Minute)
	defer streamCancel()

	streamResp, err := client.GenerateStream(streamCtx, params)
	if err != nil {
		return nil, fmt.Errorf("streaming generation failed: %w", err)
	}

	// Step 5: Wrap the stream with enhanced tracking
	enhancedStream := &EnhancedStreamReader{
		OriginalStream:           streamResp.Stream,
		ModelConfig:              modelConfig,
		RequestCtx:               requestCtx,
		InputTokens:              0, // Will be set from streaming usage data in logUsage()
		OutputTokens:             0, // Will be calculated from stream content
		AccumulatedContent:       strings.Builder{},
		WasOptimized:             promptOptimizationResult != nil && promptOptimizationResult.WasOptimized,
		OptimizationStatus:       "success",
		FallbackReason:           "",
		PromptOptimizationResult: promptOptimizationResult,
		Closed:                   false,
		UsageLogged:              false,
		GenerationService:        s,
		StartTime:                time.Now(),
		// Token savings tracking
		InputTokensSaved:  0, // Will be set by real-time marker detection
		OutputTokensSaved: 0, // Will be set by real-time marker detection
		TotalTokensSaved:  0, // Will be updated when output savings are detected
	}

	// If optimization was used, set the fallback reason
	if promptOptimizationResult != nil && promptOptimizationResult.FallbackReason != "" {
		enhancedStream.FallbackReason = promptOptimizationResult.FallbackReason
	}

	// Add metadata about optimization
	metadata := make(map[string]string)
	if streamResp.Metadata != nil {
		for k, v := range streamResp.Metadata {
			metadata[k] = v
		}
	}

	// Add optimization metadata
	metadata["was_optimized"] = fmt.Sprintf("%v", promptOptimizationResult.WasOptimized)
	metadata["optimization_type"] = promptOptimizationResult.OptimizationType
	metadata["original_prompt_length"] = fmt.Sprintf("%d", len(originalPrompt))
	metadata["optimized_prompt_length"] = fmt.Sprintf("%d", len(req.Prompt))

	// Return enhanced stream response
	return &data.StreamResponse{
		Stream:   enhancedStream,
		Metadata: metadata,
	}, nil
}

// handleNonStreamingGeneration handles non-streaming text generation
func (s *GenerationService) handleNonStreamingGeneration(ctx context.Context, req *GenerationRequest, modelConfig ModelConfig, requestCtx *RequestContext) (*GenerationResult, error) {
	// Step 1: Optimize input prompt if optimization is enabled and prompt is long enough
	var promptOptimizationResult *OptimizationResult

	if s.optimizer != nil && s.config.Optimization.Enabled && s.optimizer.ShouldOptimize(req.Prompt, 50) {
		// Try to optimize the prompt
		optimizationResult, err := s.optimizer.OptimizePromptWithMode(ctx, req.Prompt, req.OptimizationMode)
		if err != nil {
			if s.config.Optimization.FallbackOnOptimizationFailure {
				requestCtx.Logger.Warn("Prompt optimization failed, using original prompt", "error", err)
				promptOptimizationResult = &OptimizationResult{
					OriginalText:     req.Prompt,
					OptimizedText:    req.Prompt,
					OriginalTokens:   0,
					OptimizedTokens:  0,
					TokensSaved:      0,
					SavingsPercent:   0,
					OptimizationType: "none",
					WasOptimized:     false,
					FallbackReason:   "optimization_failed",
				}
			} else {
				return nil, fmt.Errorf("prompt optimization failed: %w", err)
			}
		} else {
			promptOptimizationResult = optimizationResult
			if optimizationResult.WasOptimized {
				req.Prompt = optimizationResult.OptimizedText
				requestCtx.Logger.Info("Prompt optimized successfully",
					"original_tokens", optimizationResult.OriginalTokens,
					"optimized_tokens", optimizationResult.OptimizedTokens,
					"tokens_saved", optimizationResult.TokensSaved,
					"savings_percent", fmt.Sprintf("%.1f%%", optimizationResult.SavingsPercent))
			}
		}
	}

	// Add response optimization prompt to get AI estimate of output tokens saved
	if promptOptimizationResult != nil && promptOptimizationResult.WasOptimized {
		responseOptimizationPrompt := "\n\nIMPORTANT: Be concise and efficient. After your response, append exactly: tokens_saved=<number> where <number> is your estimate of how many tokens you saved by being concise compared to a verbose response."
		req.Prompt += responseOptimizationPrompt
		requestCtx.Logger.Info("Added response optimization prompt for AI estimation", "prompt_length", len(req.Prompt))
	}

	// Step 2: Create LLM client
	client, err := s.createLLMClient(modelConfig, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Step 3: Prepare generation parameters
	params := map[string]interface{}{
		"model":       req.Model,
		"prompt":      req.Prompt,
		"max_tokens":  req.MaxTokens,
		"temperature": req.Temperature,
		"top_p":       req.TopP,
		"stream":      false,
	}

	// Add any extra parameters
	for key, value := range req.Extra {
		params[key] = value
	}

	// Step 4: Generate response
	resp, err := client.GenerateWithParams(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("generation failed: %w", err)
	}

	// Step 5: Use actual input tokens from response usage
	inputTokensSaved := 0
	outputTokensSaved := 0

	if promptOptimizationResult != nil && promptOptimizationResult.WasOptimized {
		userModelInputTokens := 0
		if resp.Usage != nil {
			userModelInputTokens = resp.Usage.PromptTokens
		}

		// CRITICAL: We can only calculate savings if we have real usage data
		if userModelInputTokens == 0 {
			requestCtx.Logger.Warn("Cannot calculate input token savings - no real usage data available",
				"input_tokens", userModelInputTokens,
				"note", "Using real API usage data only, no estimators allowed")
		} else {
			// Use real Gemma 3 API usage data for original tokens
			gemma3InputTokens := promptOptimizationResult.Gemma3InputTokens
			if gemma3InputTokens == 0 {
				// Fallback to the original token count if no real Gemma 3 usage data
				gemma3InputTokens = promptOptimizationResult.OriginalTokens
				requestCtx.Logger.Warn("No real Gemma 3 usage data, using fallback token count",
					"gemma3_input_tokens", gemma3InputTokens,
					"note", "This may not be accurate - real API usage data preferred")
			}

			inputTokensSaved = gemma3InputTokens - userModelInputTokens
			if inputTokensSaved < 0 {
				inputTokensSaved = 0
			}
			requestCtx.Logger.Info("Calculated input tokens saved using real API usage data",
				"gemma3_input_tokens", gemma3InputTokens,
				"user_model_input_tokens", userModelInputTokens,
				"input_tokens_saved", inputTokensSaved,
				"usage_source", "real_api_responses",
				"comparison_note", "Real Gemma3 usage vs actual user model usage")
		}
	}

	// Extract AI estimation of output tokens saved from the response
	if strings.Contains(resp.Text, "tokens_saved=") {
		// Find the marker and extract the estimate
		startIdx := strings.Index(resp.Text, "tokens_saved=")
		if startIdx != -1 {
			startIdx += len("tokens_saved=")
			endIdx := startIdx
			// Find the end of the number
			for endIdx < len(resp.Text) && resp.Text[endIdx] >= '0' && resp.Text[endIdx] <= '9' {
				endIdx++
			}
			if endIdx > startIdx {
				if estimate, parseErr := strconv.Atoi(resp.Text[startIdx:endIdx]); parseErr == nil {
					outputTokensSaved = estimate
					requestCtx.Logger.Info("Extracted AI estimation of output tokens saved", "estimate", outputTokensSaved)
				}
			}
		}
	}

	// Calculate total tokens saved
	totalTokensSaved := inputTokensSaved + outputTokensSaved

	// Step 6: Create result with comprehensive token savings
	result := &GenerationResult{
		Response: &GenerationResponse{
			ID:           uuid.New().String(),
			Text:         resp.Text,
			Model:        resp.ModelID,
			Provider:     resp.Provider,
			FinishReason: resp.FinishReason,
			CreatedAt:    time.Now().Unix(),
			Metadata:     convertMetadata(resp.Metadata),
		},
		WasOptimized:             promptOptimizationResult != nil && promptOptimizationResult.WasOptimized,
		OptimizationStatus:       "success",
		FallbackReason:           "",
		PromptOptimizationResult: promptOptimizationResult,
	}

	// Add usage information
	if resp.Usage != nil {
		result.Response.Usage = &ServiceUsageInfo{
			InputTokens:  resp.Usage.PromptTokens,
			OutputTokens: resp.Usage.CompletionTokens,
			TotalTokens:  resp.Usage.TotalTokens,
		}
	}

	// Add comprehensive token savings to metadata
	if result.Response.Metadata == nil {
		result.Response.Metadata = make(map[string]interface{})
	}
	result.Response.Metadata["was_optimized"] = promptOptimizationResult != nil && promptOptimizationResult.WasOptimized
	result.Response.Metadata["optimization_status"] = "success"
	result.Response.Metadata["input_tokens_saved"] = inputTokensSaved
	result.Response.Metadata["output_tokens_saved"] = outputTokensSaved
	result.Response.Metadata["total_tokens_saved"] = totalTokensSaved

	if promptOptimizationResult != nil && promptOptimizationResult.FallbackReason != "" {
		result.Response.Metadata["fallback_reason"] = promptOptimizationResult.FallbackReason
		result.FallbackReason = promptOptimizationResult.FallbackReason
	}

	// Debug logs for token savings (no re-parsing)
	requestCtx.Logger.Info("Non-streaming: Final input/output tokens saved", "input_tokens_saved", inputTokensSaved, "output_tokens_saved", outputTokensSaved)

	requestCtx.Logger.Info("Returning response with metadata", "metadata", result.Response.Metadata)
	return result, nil
}

// convertMetadata converts map[string]string to map[string]interface{}
func convertMetadata(metadata map[string]string) map[string]interface{} {
	result := make(map[string]interface{})
	for k, v := range metadata {
		result[k] = v
	}
	return result
}

// createLLMClient creates an LLM client for the specified model
func (s *GenerationService) createLLMClient(modelConfig ModelConfig, req *GenerationRequest) (data.LLMClient, error) {
	// Determine which API key to use based on provider and request
	var apiKey string

	switch modelConfig.Provider {
	case "openai":
		if req.OpenAIAPIKey != "" {
			apiKey = req.OpenAIAPIKey
		} else {
			apiKey = s.config.LLM.OpenAIAPIKey
		}
	case "anthropic":
		if req.AnthropicAPIKey != "" {
			apiKey = req.AnthropicAPIKey
		} else {
			apiKey = s.config.LLM.AnthropicAPIKey
		}
	case "google":
		if req.GoogleAPIKey != "" {
			apiKey = req.GoogleAPIKey
		} else {
			apiKey = s.config.LLM.GoogleAPIKey
		}
	default:
		return nil, fmt.Errorf("unsupported provider: %s", modelConfig.Provider)
	}

	if apiKey == "" {
		return nil, fmt.Errorf("no API key provided for provider: %s", modelConfig.Provider)
	}

	// Create client using the factory function
	return data.NewClientForModel(modelConfig.ModelID, modelConfig.Provider, apiKey)
}

// CalculateCost calculates the cost for a request
func (s *GenerationService) CalculateCost(inputTokens, outputTokens int, modelConfig ModelConfig, pricingTier PricingTier) float64 {
	// Calculate base cost
	inputCost := float64(inputTokens) * modelConfig.InputPricePerMillion / 1000000
	outputCost := float64(outputTokens) * modelConfig.OutputPricePerMillion / 1000000
	baseCost := inputCost + outputCost

	// Apply pricing tier markups (percentage-based)
	inputMarkup := inputCost * (pricingTier.InputMarkupPercent / 100)
	outputMarkup := outputCost * (pricingTier.OutputMarkupPercent / 100)
	totalMarkup := inputMarkup + outputMarkup

	return baseCost + totalMarkup
}

// calculateEstimatedCost calculates an estimated cost for a request
func (s *GenerationService) calculateEstimatedCost(inputTokens, outputTokens int, modelConfig ModelConfig, pricingTier PricingTier) float64 {
	// Use the same calculation as actual cost for now
	// In the future, this could include additional factors like optimization savings
	return s.CalculateCost(inputTokens, outputTokens, modelConfig, pricingTier)
}

// checkUserBalance checks the user's balance
func (s *GenerationService) checkUserBalance(ctx context.Context, userID string) (bool, float64, error) {
	// Get user from cache
	cacheKey := fmt.Sprintf("user:%s", userID)

	var cachedUser *CachedUserData
	if cached, found := s.cache.Get(cacheKey); found {
		if userData, ok := cached.(*CachedUserData); ok {
			// Check if cache is still valid (5 minutes)
			if time.Since(userData.LastUpdated) < 5*time.Minute {
				cachedUser = userData
			}
		}
	}

	// Cache miss or expired, load from Firebase
	if cachedUser == nil {
		user, err := s.firebaseService.GetUserByID(ctx, userID)
		if err != nil {
			return false, 0, fmt.Errorf("failed to get user from Firebase: %w", err)
		}

		cachedUser = &CachedUserData{
			ID:            user.ID,
			Email:         user.Email,
			Balance:       user.Balance,
			TierID:        user.TierID,
			IsActive:      user.IsActive,
			CustomPricing: user.CustomPricing,
			LastUpdated:   time.Now(),
		}

		// Store in cache for 5 minutes
		s.cache.Set(cacheKey, cachedUser, 5*time.Minute)
	}

	// Check if user is active
	if !cachedUser.IsActive {
		return false, cachedUser.Balance, fmt.Errorf("user account is inactive")
	}

	// Allow negative balance (graceful handling)
	// Users can go into negative balance and it will be deducted from next purchase
	return true, cachedUser.Balance, nil
}
