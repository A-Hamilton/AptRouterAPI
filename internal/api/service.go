package api

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"github.com/apt-router/api/internal/config"
	"github.com/apt-router/api/internal/llm"
	"github.com/apt-router/api/internal/pricing"
	"github.com/patrickmn/go-cache"
	supabase "github.com/supabase-community/supabase-go"
)

// GenerationService handles the business logic for text generation
type GenerationService struct {
	config         *config.Config
	supabaseClient *supabase.Client
	cache          *cache.Cache
	pricingService *pricing.Service
	optimizer      *llm.Optimizer
}

// NewGenerationService creates a new generation service
func NewGenerationService(
	cfg *config.Config,
	supabaseClient *supabase.Client,
	cache *cache.Cache,
	pricingService *pricing.Service,
) *GenerationService {
	// Initialize optimizer with Gemma model
	optimizer, err := llm.NewOptimizer("gemini-1.5-flash", cfg.LLM.GoogleAPIKey)
	if err != nil {
		slog.Error("Failed to initialize optimizer", "error", err)
		// Continue without optimizer if it fails
		optimizer = nil
	}

	return &GenerationService{
		config:         cfg,
		supabaseClient: supabaseClient,
		cache:          cache,
		pricingService: pricingService,
		optimizer:      optimizer,
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
	PromptOptimizationResult   *llm.OptimizationResult
	ResponseOptimizationResult *llm.OptimizationResult
}

// EnhancedStreamReader wraps the original stream to track tokens and usage
type EnhancedStreamReader struct {
	originalStream           io.ReadCloser
	modelConfig              pricing.ModelConfig
	requestCtx               *RequestContext
	inputTokens              int
	outputTokens             int
	accumulatedContent       strings.Builder
	wasOptimized             bool
	optimizationStatus       string
	fallbackReason           string
	promptOptimizationResult *llm.OptimizationResult
	closed                   bool
	usageLogged              bool
}

func (r *EnhancedStreamReader) Read(p []byte) (n int, err error) {
	if r.closed {
		return 0, io.EOF
	}

	// Read from original stream
	n, err = r.originalStream.Read(p)
	if n > 0 {
		// Accumulate content for token counting
		r.accumulatedContent.Write(p[:n])
	}

	// If stream ended, calculate usage and log
	if err == io.EOF && !r.usageLogged {
		r.logUsage()
		r.usageLogged = true
	}

	return n, err
}

func (r *EnhancedStreamReader) Close() error {
	r.closed = true

	// Log usage if not already logged
	if !r.usageLogged {
		r.logUsage()
	}

	return r.originalStream.Close()
}

func (r *EnhancedStreamReader) logUsage() {
	// Count output tokens from accumulated content
	outputContent := r.accumulatedContent.String()

	// Create a temporary client to count tokens
	tempClient, err := llm.NewClientForModel(r.modelConfig.ModelID, r.modelConfig.Provider, "")
	if err != nil {
		r.requestCtx.Logger.Warn("Failed to create temp client for token counting", "error", err)
		r.outputTokens = 0
	} else {
		r.outputTokens, err = tempClient.CountTokens(outputContent)
		if err != nil {
			r.requestCtx.Logger.Warn("Failed to count output tokens", "error", err)
			r.outputTokens = 0
		}
	}

	// Calculate cost
	cost := r.calculateCost(r.inputTokens, r.outputTokens)

	// Log the streaming request with usage information
	r.requestCtx.Logger.Info("Streaming request completed with usage",
		"user_id", r.requestCtx.UserID,
		"model", r.modelConfig.ModelID,
		"streaming", true,
		"input_tokens", r.inputTokens,
		"output_tokens", r.outputTokens,
		"total_tokens", r.inputTokens+r.outputTokens,
		"cost", cost,
		"was_optimized", r.wasOptimized,
		"optimization_status", r.optimizationStatus,
		"fallback_reason", r.fallbackReason,
	)

	// Log request to database (simplified for streaming)
	r.logStreamingRequest(cost)

	// Charge the user
	r.chargeUser(cost)
}

func (r *EnhancedStreamReader) calculateCost(inputTokens, outputTokens int) float64 {
	// Calculate base cost
	inputCost := float64(inputTokens) * r.modelConfig.InputPricePerMillion / 1000000
	outputCost := float64(outputTokens) * r.modelConfig.OutputPricePerMillion / 1000000
	baseCost := inputCost + outputCost

	// Apply pricing tier discounts
	inputSavings := float64(inputTokens) * r.requestCtx.PricingTier.InputSavingsRateUSDPerMillion / 1000000
	outputSavings := float64(outputTokens) * r.requestCtx.PricingTier.OutputSavingsRateUSDPerMillion / 1000000
	totalSavings := inputSavings + outputSavings

	finalCost := baseCost - totalSavings
	if finalCost < 0 {
		finalCost = 0
	}

	return finalCost
}

func (r *EnhancedStreamReader) logStreamingRequest(cost float64) {
	// TODO: Implement database logging using Supabase
	// For now, just log to console
	r.requestCtx.Logger.Info("Streaming request logged",
		"user_id", r.requestCtx.UserID,
		"model", r.modelConfig.ModelID,
		"input_tokens", r.inputTokens,
		"output_tokens", r.outputTokens,
		"cost", cost,
		"was_optimized", r.wasOptimized,
		"optimization_status", r.optimizationStatus,
		"fallback_reason", r.fallbackReason,
	)
}

func (r *EnhancedStreamReader) chargeUser(cost float64) {
	// TODO: Implement database charging using Supabase
	// For now, just log to console
	r.requestCtx.Logger.Info("User charged for streaming",
		"user_id", r.requestCtx.UserID,
		"cost", cost,
	)
}

// Generate handles the main generation logic
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

	// Check if streaming is requested
	if req.Stream {
		return nil, fmt.Errorf("streaming generation not yet implemented")
	}

	// Handle non-streaming generation
	return s.handleNonStreamingGeneration(ctx, req, modelConfig, requestCtx)
}

// GenerateStream handles streaming generation logic with optimization and token tracking
func (s *GenerationService) GenerateStream(ctx context.Context, req *GenerationRequest, requestCtx *RequestContext) (*llm.StreamResponse, error) {
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

	// Track optimization metrics
	var (
		optimizedPrompt    = req.Prompt
		wasOptimized       = false
		optimizationStatus = "not_attempted"
		fallbackReason     = ""
	)

	// Default optimization mode
	if req.OptimizationMode != "efficiency" {
		req.OptimizationMode = "context"
	}

	// Step 1: Optimize input prompt if optimization is enabled and prompt is long enough
	var promptOptimizationResult *llm.OptimizationResult

	if s.optimizer != nil && s.config.Optimization.Enabled && s.optimizer.ShouldOptimize(req.Prompt, 50) {
		requestCtx.Logger.Info("Attempting prompt optimization", "original_length", len(req.Prompt), "mode", req.OptimizationMode)

		optimizationResult, err := s.optimizer.OptimizePromptWithMode(ctx, req.Prompt, req.OptimizationMode)
		if err != nil {
			requestCtx.Logger.Warn("Prompt optimization failed for streaming, using original", "error", err)
			optimizationStatus = "failed"
			fallbackReason = "optimization_error"
		} else {
			promptOptimizationResult = optimizationResult // Always set, even if not optimized
			requestCtx.Logger.Info("Prompt optimization result", "original_prompt", req.Prompt, "optimized_prompt", optimizationResult.OptimizedText)
			if optimizationResult.WasOptimized {
				optimizedPrompt = optimizationResult.OptimizedText
				// Add output optimization instructions to the optimized prompt
				optimizedPrompt += "\n\nIMPORTANT: Make your response token-efficient while preserving all essential information. At the end of your response, include your estimate of tokens saved in this format: saved_tokens={your_estimate_number}"
				wasOptimized = true
				optimizationStatus = "success"
				requestCtx.Logger.Info("Prompt optimization successful for streaming",
					"original_tokens", optimizationResult.OriginalTokens,
					"optimized_tokens", optimizationResult.OptimizedTokens,
					"tokens_saved", optimizationResult.TokensSaved,
					"savings_percent", fmt.Sprintf("%.1f%%", optimizationResult.SavingsPercent),
					"optimization_type", optimizationResult.OptimizationType)
			}
		}
	}

	// Create LLM client
	client, err := s.createLLMClient(modelConfig, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Count input tokens (use optimized prompt if available)
	inputTokens, err := client.CountTokens(optimizedPrompt)
	if err != nil {
		requestCtx.Logger.Warn("Failed to count input tokens for streaming", "error", err)
		inputTokens = 0
	}

	// Prepare parameters for LLM (use optimized prompt)
	params := map[string]interface{}{
		"prompt":      optimizedPrompt,
		"max_tokens":  req.MaxTokens,
		"temperature": req.Temperature,
		"top_p":       req.TopP,
	}
	if req.Extra != nil {
		for k, v := range req.Extra {
			params[k] = v
		}
	}

	// Generate streaming response
	streamResp, err := client.GenerateStream(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("LLM streaming generation failed: %w", err)
	}

	// Create an enhanced stream reader that tracks tokens and usage
	enhancedStream := &EnhancedStreamReader{
		originalStream:           streamResp.Stream,
		modelConfig:              modelConfig,
		requestCtx:               requestCtx,
		inputTokens:              inputTokens,
		wasOptimized:             wasOptimized,
		optimizationStatus:       optimizationStatus,
		fallbackReason:           fallbackReason,
		promptOptimizationResult: promptOptimizationResult,
	}

	// Return enhanced stream response
	return &llm.StreamResponse{
		Stream: enhancedStream,
		Metadata: map[string]string{
			"provider":            modelConfig.Provider,
			"model_id":            req.Model,
			"input_tokens":        fmt.Sprintf("%d", inputTokens),
			"was_optimized":       fmt.Sprintf("%t", wasOptimized),
			"optimization_status": optimizationStatus,
		},
	}, nil
}

// handleNonStreamingGeneration handles non-streaming text generation
func (s *GenerationService) handleNonStreamingGeneration(ctx context.Context, req *GenerationRequest, modelConfig pricing.ModelConfig, requestCtx *RequestContext) (*GenerationResult, error) {
	// Track optimization metrics
	var (
		optimizedPrompt    = req.Prompt
		wasOptimized       = false
		optimizationStatus = "not_attempted"
		fallbackReason     = ""
	)

	// Default optimization mode
	if req.OptimizationMode != "efficiency" {
		req.OptimizationMode = "context"
	}

	// Step 1: Optimize input prompt if optimization is enabled and prompt is long enough
	var promptOptimizationResult *llm.OptimizationResult

	if s.optimizer != nil && s.config.Optimization.Enabled && s.optimizer.ShouldOptimize(req.Prompt, 50) {
		requestCtx.Logger.Info("Attempting prompt optimization", "original_length", len(req.Prompt), "mode", req.OptimizationMode)

		optimizationResult, err := s.optimizer.OptimizePromptWithMode(ctx, req.Prompt, req.OptimizationMode)
		if err != nil {
			requestCtx.Logger.Warn("Prompt optimization failed, using original", "error", err)
			optimizationStatus = "failed"
			fallbackReason = "optimization_error"
		} else if optimizationResult.WasOptimized {
			optimizedPrompt = optimizationResult.OptimizedText
			promptOptimizationResult = optimizationResult
			wasOptimized = true
			optimizationStatus = "success"
			requestCtx.Logger.Info("Prompt optimization successful",
				"original_tokens", optimizationResult.OriginalTokens,
				"optimized_tokens", optimizationResult.OptimizedTokens,
				"tokens_saved", optimizationResult.TokensSaved,
				"savings_percent", fmt.Sprintf("%.1f%%", optimizationResult.SavingsPercent),
				"optimization_type", optimizationResult.OptimizationType)
		}
	}

	// Create LLM client
	client, err := s.createLLMClient(modelConfig, req)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Prepare parameters for LLM (use optimized prompt)
	params := map[string]interface{}{
		"prompt":      optimizedPrompt,
		"max_tokens":  req.MaxTokens,
		"temperature": req.Temperature,
		"top_p":       req.TopP,
	}
	if req.Extra != nil {
		for k, v := range req.Extra {
			params[k] = v
		}
	}

	// Generate text
	llmResp, err := client.GenerateWithParams(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("LLM generation failed: %w", err)
	}

	// Step 2: Optimize response if optimization is enabled and response is long enough
	if s.optimizer != nil && s.config.Optimization.Enabled && s.optimizer.ShouldOptimize(llmResp.Text, 100) {
		requestCtx.Logger.Info("Attempting response optimization", "original_length", len(llmResp.Text), "mode", req.OptimizationMode)

		optimizationResult, err := s.optimizer.OptimizeResponseWithMode(ctx, llmResp.Text, req.OptimizationMode)
		if err != nil {
			requestCtx.Logger.Warn("Response optimization failed, using original", "error", err)
			if optimizationStatus == "success" {
				optimizationStatus = "partial_success"
			} else {
				optimizationStatus = "failed"
			}
			fallbackReason = "response_optimization_error"
		} else if optimizationResult.WasOptimized {
			llmResp.Text = optimizationResult.OptimizedText
			// Use the optimized token count from the result
			llmResp.OutputTokens = optimizationResult.OptimizedTokens
			requestCtx.Logger.Info("Response optimization successful",
				"original_tokens", optimizationResult.OriginalTokens,
				"optimized_tokens", optimizationResult.OptimizedTokens,
				"tokens_saved", optimizationResult.TokensSaved,
				"savings_percent", fmt.Sprintf("%.1f%%", optimizationResult.SavingsPercent),
				"optimization_type", optimizationResult.OptimizationType)
		}
	}

	// Calculate cost
	cost := s.calculateCost(llmResp.InputTokens, llmResp.OutputTokens, modelConfig, requestCtx.PricingTier)

	// Build response
	response := &GenerationResponse{
		ID:       requestCtx.RequestID,
		Text:     llmResp.Text,
		Model:    req.Model,
		Provider: modelConfig.Provider,
		Usage: &ServiceUsageInfo{
			InputTokens:  llmResp.InputTokens,
			OutputTokens: llmResp.OutputTokens,
			TotalTokens:  llmResp.InputTokens + llmResp.OutputTokens,
		},
		FinishReason: llmResp.FinishReason,
		CreatedAt:    time.Now().Unix(),
		Metadata: map[string]interface{}{
			"cost":                cost,
			"fallback_reason":     fallbackReason,
			"optimization_status": optimizationStatus,
			"was_optimized":       wasOptimized,
		},
	}

	// Add optimization details if optimization occurred
	if promptOptimizationResult != nil {
		response.Metadata["input_tokens_saved"] = promptOptimizationResult.TokensSaved
		response.Metadata["input_savings_percent"] = promptOptimizationResult.SavingsPercent
		response.Metadata["input_optimization_type"] = promptOptimizationResult.OptimizationType
		// Keep legacy fields for backward compatibility
		response.Metadata["tokens_saved"] = promptOptimizationResult.TokensSaved
		response.Metadata["savings_percent"] = promptOptimizationResult.SavingsPercent
		response.Metadata["optimization_type"] = promptOptimizationResult.OptimizationType
		if promptOptimizationResult.OptimizedPrompt != "" {
			response.Metadata["optimized_prompt"] = promptOptimizationResult.OptimizedPrompt
		}
	}

	// Calculate total token savings
	totalInputTokensSaved := 0
	if promptOptimizationResult != nil {
		totalInputTokensSaved = promptOptimizationResult.TokensSaved
	}
	totalTokensSaved := totalInputTokensSaved

	if totalTokensSaved > 0 {
		response.Metadata["total_tokens_saved"] = totalTokensSaved
		response.Metadata["total_input_tokens_saved"] = totalInputTokensSaved
		response.Metadata["total_output_tokens_saved"] = 0
	}

	return &GenerationResult{
		Response:                   response,
		WasOptimized:               wasOptimized,
		OptimizationStatus:         optimizationStatus,
		FallbackReason:             fallbackReason,
		PromptOptimizationResult:   promptOptimizationResult,
		ResponseOptimizationResult: nil,
	}, nil
}

// createLLMClient creates an LLM client for the given model configuration
func (s *GenerationService) createLLMClient(modelConfig pricing.ModelConfig, req *GenerationRequest) (llm.LLMClient, error) {
	// Use user-provided API key if present, otherwise fallback to server key
	var apiKey string
	var keySource string

	switch modelConfig.Provider {
	case "openai":
		if req.OpenAIAPIKey != "" {
			apiKey = req.OpenAIAPIKey
			keySource = "user_provided"
		} else {
			apiKey = s.config.LLM.OpenAIAPIKey
			keySource = "server_config"
		}
	case "anthropic":
		if req.AnthropicAPIKey != "" {
			apiKey = req.AnthropicAPIKey
			keySource = "user_provided"
		} else {
			apiKey = s.config.LLM.AnthropicAPIKey
			keySource = "server_config"
		}
	case "google":
		if req.GoogleAPIKey != "" {
			apiKey = req.GoogleAPIKey
			keySource = "user_provided"
		} else {
			apiKey = s.config.LLM.GoogleAPIKey
			keySource = "server_config"
		}
	default:
		return nil, fmt.Errorf("unsupported provider: %s", modelConfig.Provider)
	}

	if apiKey == "" {
		return nil, fmt.Errorf("no API key provided for provider: %s", modelConfig.Provider)
	}

	slog.Info("Creating LLM client",
		"provider", modelConfig.Provider,
		"model", modelConfig.ModelID,
		"key_source", keySource,
		"api_key_length", len(apiKey))

	client, err := llm.NewClientForModel(modelConfig.ModelID, modelConfig.Provider, apiKey)
	if err != nil {
		slog.Error("Failed to create LLM client", "error", err, "provider", modelConfig.Provider, "model", modelConfig.ModelID)
		return nil, err
	}

	slog.Info("LLM client created successfully", "provider", modelConfig.Provider, "model", modelConfig.ModelID)
	return client, nil
}

// calculateCost calculates the cost for the given token usage
func (s *GenerationService) calculateCost(inputTokens, outputTokens int, modelConfig pricing.ModelConfig, pricingTier pricing.PricingTier) float64 {
	// Convert tokens to millions and apply pricing
	inputCost := (float64(inputTokens) / 1000000) * modelConfig.InputPricePerMillion
	outputCost := (float64(outputTokens) / 1000000) * modelConfig.OutputPricePerMillion

	// Apply tier-based savings (lower tier = higher savings rate)
	inputSavings := inputCost * (pricingTier.InputSavingsRateUSDPerMillion / 100)
	outputSavings := outputCost * (pricingTier.OutputSavingsRateUSDPerMillion / 100)

	return inputCost + outputCost - inputSavings - outputSavings
}
