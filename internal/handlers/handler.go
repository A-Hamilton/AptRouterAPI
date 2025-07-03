package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/apt-router/api/internal/config"
	"github.com/apt-router/api/internal/firebase"
	"github.com/apt-router/api/internal/pricing"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
)

// Handler handles all API requests
type Handler struct {
	config            *config.Config
	firebaseService   *firebase.Service
	cache             *cache.Cache
	pricingService    *pricing.Service
	generationService *GenerationService
}

// NewHandler creates a new API handler
func NewHandler(
	cfg *config.Config,
	firebaseService *firebase.Service,
	cache *cache.Cache,
	pricingService *pricing.Service,
) *Handler {
	generationService := NewGenerationService(cfg, firebaseService, cache, pricingService)

	return &Handler{
		config:            cfg,
		firebaseService:   firebaseService,
		cache:             cache,
		pricingService:    pricingService,
		generationService: generationService,
	}
}

// HealthCheck handles the health check endpoint
func (h *Handler) HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status":  "healthy",
		"service": "apt-router-api",
		"version": "1.0.0",
	})
}

// AuthMiddleware authenticates API key requests and sets up request context
func (h *Handler) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := h.getRequestID(c)
		logger := h.getLogger(c)

		// Extract API key from Authorization header
		authHeader := c.GetHeader("Authorization")
		var apiKey string

		if authHeader != "" {
			// Handle "Bearer <token>" format
			if strings.HasPrefix(authHeader, "Bearer ") {
				apiKey = strings.TrimPrefix(authHeader, "Bearer ")
			} else {
				// Handle direct API key format
				apiKey = authHeader
			}
		}

		// For development/testing, accept any API key and create a mock context
		// In production, this would validate the API key against Firebase
		if apiKey == "" {
			logger.Warn("No API key provided, using mock key for development")
			apiKey = "mock-api-key-for-development"
		}

		// Hash the API key for logging (don't log the actual key)
		keyHash := h.hashAPIKey(apiKey)
		logger.Info("API key authentication", "key_hash", keyHash[:8]+"...")

		// Get user from Firebase (for development, use mock user)
		var user *firebase.User
		var err error

		if apiKey == "mock-api-key-for-development" {
			// Create mock user for development
			user = &firebase.User{
				ID:            "mock-user-id",
				Email:         "dev@example.com",
				Balance:       100.0,
				TierID:        "tier-1",
				IsActive:      true,
				CustomPricing: false,
			}
		} else {
			// Get real user from Firebase
			user, err = h.firebaseService.GetUserByAPIKey(c.Request.Context(), keyHash)
			if err != nil {
				logger.Error("Failed to get user by API key", "error", err)
				c.JSON(http.StatusUnauthorized, gin.H{
					"error": "Invalid API key",
				})
				c.Abort()
				return
			}
		}

		// Get cached user data for performance
		cachedUser, err := h.getUserFromCache(c.Request.Context(), user.ID)
		if err != nil {
			logger.Error("Failed to get cached user data", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to load user data",
			})
			c.Abort()
			return
		}

		// Get pricing tier from cache
		tier, err := h.getPricingTierFromCache(c.Request.Context(), cachedUser.TierID)
		if err != nil {
			logger.Error("Failed to get pricing tier", "error", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to load pricing information",
			})
			c.Abort()
			return
		}

		// Create request context with cached user data
		requestCtx := &RequestContext{
			RequestID: requestID,
			UserID:    user.ID,
			APIKeyID:  keyHash,
			PricingTier: pricing.PricingTier{
				ID:                  tier.ID,
				TierName:            tier.TierName,
				MinMonthlySpend:     tier.MinMonthlySpend,
				InputMarkupPercent:  tier.InputMarkupPercent,
				OutputMarkupPercent: tier.OutputMarkupPercent,
				IsActive:            tier.IsActive,
				IsCustom:            tier.IsCustom,
				CustomModelPricing:  tier.CustomModelPricing,
			},
			Logger:     logger,
			CachedUser: cachedUser,
		}

		// Store request context in Gin context
		c.Set(string(requestContextGinKey), requestCtx)

		// Continue to next middleware/handler
		c.Next()
	}
}

// JWTAuthMiddleware authenticates JWT requests
func (h *Handler) JWTAuthMiddleware() gin.HandlerFunc {
	// TODO: Implement JWT authentication with Firebase Auth
	return func(c *gin.Context) {
		c.Next()
	}
}

// GenerateRequest represents a text generation request from HTTP
type GenerateRequest struct {
	Model       string                 `json:"model" binding:"required"`
	Prompt      string                 `json:"prompt" binding:"required"`
	MaxTokens   *int                   `json:"max_tokens,omitempty"`
	Temperature *float64               `json:"temperature,omitempty"`
	TopP        *float64               `json:"top_p,omitempty"`
	Stream      *bool                  `json:"stream,omitempty"`
	Extra       map[string]interface{} `json:"extra,omitempty"`
	// BYOK fields
	OpenAIAPIKey    string `json:"openai_api_key,omitempty"`
	AnthropicAPIKey string `json:"anthropic_api_key,omitempty"`
	GoogleAPIKey    string `json:"google_api_key,omitempty"`
	// Optimization mode: "context" (default) or "efficiency"
	OptimizationMode string `json:"optimization_mode,omitempty"`
}

// GenerateResponse represents a text generation response for HTTP
type GenerateResponse struct {
	ID           string                 `json:"id"`
	Text         string                 `json:"text"`
	Model        string                 `json:"model"`
	Provider     string                 `json:"provider"`
	Usage        *UsageInfo             `json:"usage,omitempty"`
	FinishReason string                 `json:"finish_reason,omitempty"`
	CreatedAt    int64                  `json:"created_at"`
	Metadata     map[string]interface{} `json:"metadata,omitempty"`
}

// UsageInfo contains token usage information for HTTP responses
type UsageInfo struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
	TotalTokens  int `json:"total_tokens"`
}

// Generate handles the main generation endpoint
func (h *Handler) Generate(c *gin.Context) {
	startTime := time.Now()

	// Get request context
	requestCtx, exists := h.getRequestContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Request context not found",
		})
		return
	}

	// Parse request
	var req GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format: " + err.Error(),
		})
		return
	}

	// Convert HTTP request to service request
	serviceReq := &GenerationRequest{
		Model:            req.Model,
		Prompt:           req.Prompt,
		MaxTokens:        h.getIntValue(req.MaxTokens, 1000),
		Temperature:      h.getFloatValue(req.Temperature, 0.7),
		TopP:             h.getFloatValue(req.TopP, 1.0),
		Stream:           h.getBoolValue(req.Stream, false),
		Extra:            req.Extra,
		OpenAIAPIKey:     req.OpenAIAPIKey,
		AnthropicAPIKey:  req.AnthropicAPIKey,
		GoogleAPIKey:     req.GoogleAPIKey,
		OptimizationMode: req.OptimizationMode,
	}

	// Call service layer
	result, err := h.generationService.Generate(c.Request.Context(), serviceReq, requestCtx)
	if err != nil {
		requestCtx.Logger.Error("Generation failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Calculate cost with percentage-based pricing
	totalCost, markupAmount, err := h.pricingService.CalculateCost(
		c.Request.Context(),
		requestCtx.UserID,
		req.Model,
		result.Response.Usage.InputTokens,
		result.Response.Usage.OutputTokens,
	)
	if err != nil {
		requestCtx.Logger.Error("Failed to calculate cost", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to calculate cost",
		})
		return
	}

	// Check user balance
	balance, err := h.firebaseService.GetUserBalance(c.Request.Context(), requestCtx.UserID)
	if err != nil {
		requestCtx.Logger.Error("Failed to get user balance", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to check balance",
		})
		return
	}

	if balance < totalCost {
		c.JSON(http.StatusPaymentRequired, gin.H{
			"error": fmt.Sprintf("Insufficient balance: %.6f required, %.6f available", totalCost, balance),
		})
		return
	}

	// Convert service response to HTTP response
	httpResp := &GenerateResponse{
		ID:           result.Response.ID,
		Text:         result.Response.Text,
		Model:        result.Response.Model,
		Provider:     result.Response.Provider,
		FinishReason: result.Response.FinishReason,
		CreatedAt:    result.Response.CreatedAt,
		Metadata:     result.Response.Metadata,
	}

	// Convert usage info
	if result.Response.Usage != nil {
		httpResp.Usage = &UsageInfo{
			InputTokens:  result.Response.Usage.InputTokens,
			OutputTokens: result.Response.Usage.OutputTokens,
			TotalTokens:  result.Response.Usage.InputTokens + result.Response.Usage.OutputTokens,
		}
	}

	// Add cost information to metadata
	if httpResp.Metadata == nil {
		httpResp.Metadata = make(map[string]interface{})
	}
	httpResp.Metadata["total_cost"] = totalCost
	httpResp.Metadata["markup_amount"] = markupAmount
	httpResp.Metadata["base_cost"] = totalCost - markupAmount

	// Log the request for audit purposes
	err = h.logRequest(c.Request.Context(), requestCtx, serviceReq, result, totalCost, markupAmount, startTime, time.Now(), false)
	if err != nil {
		requestCtx.Logger.Error("Failed to log request", "error", err)
		// Don't fail the request, just log the error
	}

	// Charge the user
	err = h.firebaseService.UpdateUserBalance(c.Request.Context(), requestCtx.UserID, -totalCost)
	if err != nil {
		requestCtx.Logger.Error("Failed to charge user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to process payment",
		})
		return
	}

	c.JSON(http.StatusOK, httpResp)
}

// getIntValue safely extracts int value from pointer
func (h *Handler) getIntValue(ptr *int, defaultValue int) int {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

// getFloatValue safely extracts float64 value from pointer
func (h *Handler) getFloatValue(ptr *float64, defaultValue float64) float64 {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

// getBoolValue safely extracts bool value from pointer
func (h *Handler) getBoolValue(ptr *bool, defaultValue bool) bool {
	if ptr != nil {
		return *ptr
	}
	return defaultValue
}

// logRequest logs the generation request to Firebase for audit purposes
func (h *Handler) logRequest(ctx context.Context, requestCtx *RequestContext, req *GenerationRequest, result *GenerationResult, totalCost, markupAmount float64, startTime, endTime time.Time, streaming bool) error {
	// Create request log
	log := &firebase.RequestLog{
		ID:                 requestCtx.RequestID,
		UserID:             requestCtx.UserID,
		APIKeyID:           requestCtx.APIKeyID,
		RequestID:          requestCtx.RequestID,
		ModelID:            req.Model,
		Provider:           result.Response.Provider,
		InputTokens:        result.Response.Usage.InputTokens,
		OutputTokens:       result.Response.Usage.OutputTokens,
		TotalTokens:        result.Response.Usage.InputTokens + result.Response.Usage.OutputTokens,
		BaseCost:           totalCost - markupAmount,
		MarkupAmount:       markupAmount,
		TotalCost:          totalCost,
		TierID:             requestCtx.PricingTier.ID,
		MarkupPercent:      requestCtx.PricingTier.InputMarkupPercent, // Use input markup as representative
		WasOptimized:       result.WasOptimized,
		OptimizationStatus: result.OptimizationStatus,
		TokensSaved:        0, // Will be calculated if optimization occurred
		SavingsAmount:      0, // Will be calculated if optimization occurred
		Streaming:          streaming,
		RequestTimestamp:   startTime,
		ResponseTimestamp:  endTime,
		DurationMs:         endTime.Sub(startTime).Milliseconds(),
		Status:             "success",
		IPAddress:          "", // TODO: Extract from request
		UserAgent:          "", // TODO: Extract from request
		Metadata:           result.Response.Metadata,
	}

	// Calculate tokens saved if optimization occurred
	if result.PromptOptimizationResult != nil {
		log.TokensSaved = result.PromptOptimizationResult.TokensSaved
		log.SavingsAmount = float64(result.PromptOptimizationResult.TokensSaved) * (requestCtx.PricingTier.InputMarkupPercent / 100) / 1000000
	}

	// Log to Firebase
	return h.firebaseService.LogRequest(ctx, log)
}

// GenerateStream handles the streaming generation endpoint
func (h *Handler) GenerateStream(c *gin.Context) {
	startTime := time.Now()

	// Get request context
	requestCtx, exists := h.getRequestContext(c)
	if !exists {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Request context not found",
		})
		return
	}

	// Parse request
	var req GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format: " + err.Error(),
		})
		return
	}

	// Convert HTTP request to service request
	serviceReq := &GenerationRequest{
		Model:            req.Model,
		Prompt:           req.Prompt,
		MaxTokens:        h.getIntValue(req.MaxTokens, 1000),
		Temperature:      h.getFloatValue(req.Temperature, 0.7),
		TopP:             h.getFloatValue(req.TopP, 1.0),
		Stream:           true, // Force streaming for this endpoint
		Extra:            req.Extra,
		OpenAIAPIKey:     req.OpenAIAPIKey,
		AnthropicAPIKey:  req.AnthropicAPIKey,
		GoogleAPIKey:     req.GoogleAPIKey,
		OptimizationMode: req.OptimizationMode,
	}

	// Call service layer for streaming
	streamResp, err := h.generationService.GenerateStream(c.Request.Context(), serviceReq, requestCtx)
	if err != nil {
		requestCtx.Logger.Error("Streaming generation failed", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Set headers for streaming (all available before streaming)
	if streamResp.Metadata != nil {
		if inputTokens, ok := streamResp.Metadata["input_tokens"]; ok {
			c.Header("X-Input-Tokens", inputTokens)
		}
		if wasOptimized, ok := streamResp.Metadata["was_optimized"]; ok {
			c.Header("X-Was-Optimized", wasOptimized)
		}
		if optimizationStatus, ok := streamResp.Metadata["optimization_status"]; ok {
			c.Header("X-Optimization-Status", optimizationStatus)
		}
	}

	// Set up streaming response headers
	c.Header("Content-Type", "text/plain; charset=utf-8")
	c.Header("Transfer-Encoding", "chunked")
	c.Writer.WriteHeader(http.StatusOK)
	c.Writer.Flush()

	// Stream the response in real-time
	buffer := make([]byte, 1024)
	var fullResponse strings.Builder

	for {
		n, err := streamResp.Stream.Read(buffer)
		if n > 0 {
			chunk := string(buffer[:n])

			// Accumulate the full response for processing
			fullResponse.WriteString(chunk)

			// Send this chunk immediately to the user
			c.Writer.Write(buffer[:n])
			c.Writer.Flush() // Force immediate transmission
		}
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			requestCtx.Logger.Error("Stream read error", "error", err)
			break
		}
	}

	// Close the stream (this will trigger usage logging in EnhancedStreamReader)
	streamResp.Stream.Close()

	// Get the full response content
	fullContent := fullResponse.String()

	// Parse saved_tokens estimate from the response if present
	savedTokensEstimate := 0
	cleanedContent := fullContent
	if idx := strings.LastIndex(fullContent, "saved_tokens="); idx != -1 {
		startIdx := idx + len("saved_tokens=")
		endIdx := startIdx
		for endIdx < len(fullContent) && fullContent[endIdx] >= '0' && fullContent[endIdx] <= '9' {
			endIdx++
		}
		if endIdx > startIdx {
			if estimate, err := strconv.Atoi(fullContent[startIdx:endIdx]); err == nil {
				savedTokensEstimate = estimate
				requestCtx.Logger.Info("Parsed saved_tokens estimate from response", "estimate", savedTokensEstimate)
			}
		}
		// Remove the saved_tokens pattern from the content
		cleanedContent = strings.TrimSpace(fullContent[:idx])

		// Send a backspace sequence to remove the saved_tokens pattern from the user's view
		// This is a bit hacky but works for terminal output
		backspaceCount := len(fullContent) - len(cleanedContent)
		if backspaceCount > 0 {
			backspaceSequence := strings.Repeat("\b \b", backspaceCount)
			c.Writer.Write([]byte(backspaceSequence))
			c.Writer.Flush()
		}
	}

	// After streaming, set output token and optimization headers if possible
	if enhanced, ok := streamResp.Stream.(*EnhancedStreamReader); ok {
		requestCtx.Logger.Info("Setting optimization headers",
			"output_tokens", enhanced.outputTokens,
			"has_optimization_result", enhanced.promptOptimizationResult != nil)

		c.Header("X-Output-Tokens", fmt.Sprintf("%d", enhanced.outputTokens))
		if enhanced.promptOptimizationResult != nil {
			requestCtx.Logger.Info("Setting optimization result headers",
				"original_tokens", enhanced.promptOptimizationResult.OriginalTokens,
				"tokens_saved", enhanced.promptOptimizationResult.TokensSaved)
			c.Header("X-Input-Tokens-Original", fmt.Sprintf("%d", enhanced.promptOptimizationResult.OriginalTokens))
			c.Header("X-Input-Tokens-Saved", fmt.Sprintf("%d", enhanced.promptOptimizationResult.TokensSaved))
		} else {
			// Set default values if no optimization result
			c.Header("X-Input-Tokens-Original", fmt.Sprintf("%d", enhanced.inputTokens))
			c.Header("X-Input-Tokens-Saved", "0")
		}

		// Calculate total cost using the generation service
		totalCost := h.generationService.calculateCost(enhanced.inputTokens, enhanced.outputTokens, enhanced.modelConfig, requestCtx.PricingTier)

		// Create metadata for logging and headers (not sent to user)
		metadata := map[string]interface{}{
			"input_tokens":        enhanced.inputTokens,
			"output_tokens":       enhanced.outputTokens,
			"total_tokens":        enhanced.inputTokens + enhanced.outputTokens,
			"cost":                totalCost,
			"was_optimized":       enhanced.wasOptimized,
			"optimization_status": enhanced.optimizationStatus,
			"fallback_reason":     enhanced.fallbackReason,
		}

		if enhanced.promptOptimizationResult != nil {
			metadata["input_tokens_original"] = enhanced.promptOptimizationResult.OriginalTokens
			metadata["tokens_saved"] = enhanced.promptOptimizationResult.TokensSaved
		} else {
			metadata["input_tokens_original"] = enhanced.inputTokens
			metadata["tokens_saved"] = 0
		}

		// Add output optimization estimate if available
		if savedTokensEstimate > 0 {
			metadata["output_tokens_saved_estimate"] = savedTokensEstimate
		}

		// Log the metadata instead of sending it to user
		requestCtx.Logger.Info("Streaming metadata", "metadata", metadata)

		// Set additional headers with metadata info
		c.Header("X-Total-Tokens", fmt.Sprintf("%d", enhanced.inputTokens+enhanced.outputTokens))
		c.Header("X-Cost", fmt.Sprintf("%.6f", totalCost))
		c.Header("X-Was-Optimized", fmt.Sprintf("%t", enhanced.wasOptimized))
		c.Header("X-Optimization-Status", enhanced.optimizationStatus)
		if enhanced.fallbackReason != "" {
			c.Header("X-Fallback-Reason", enhanced.fallbackReason)
		}
		if savedTokensEstimate > 0 {
			c.Header("X-Output-Tokens-Saved-Estimate", fmt.Sprintf("%d", savedTokensEstimate))
		}

		// Log the streaming request completion
		requestCtx.Logger.Info("Streaming request completed",
			"user_id", requestCtx.UserID,
			"model", req.Model,
			"streaming", true,
			"duration_ms", time.Since(startTime).Milliseconds(),
		)
	}
}

// GetProfile handles getting user profile
func (h *Handler) GetProfile(c *gin.Context) {
	// TODO: Implement get profile logic with Firebase
	c.JSON(http.StatusOK, gin.H{
		"message": "GetProfile endpoint - not implemented yet",
	})
}

// GetBalance handles getting user balance
func (h *Handler) GetBalance(c *gin.Context) {
	// TODO: Implement get balance logic with Firebase
	c.JSON(http.StatusOK, gin.H{
		"message": "GetBalance endpoint - not implemented yet",
	})
}

// GetUsage handles getting user usage
func (h *Handler) GetUsage(c *gin.Context) {
	// TODO: Implement get usage logic with Firebase
	c.JSON(http.StatusOK, gin.H{
		"message": "GetUsage endpoint - not implemented yet",
	})
}

// CreateAPIKey handles creating new API keys
func (h *Handler) CreateAPIKey(c *gin.Context) {
	// TODO: Implement create API key logic with Firebase
	c.JSON(http.StatusOK, gin.H{
		"message": "CreateAPIKey endpoint - not implemented yet",
	})
}

// ListAPIKeys handles listing user's API keys
func (h *Handler) ListAPIKeys(c *gin.Context) {
	// TODO: Implement list API keys logic with Firebase
	c.JSON(http.StatusOK, gin.H{
		"message": "ListAPIKeys endpoint - not implemented yet",
	})
}

// RevokeAPIKey handles revoking API keys
func (h *Handler) RevokeAPIKey(c *gin.Context) {
	// TODO: Implement revoke API key logic with Firebase
	c.JSON(http.StatusOK, gin.H{
		"message": "RevokeAPIKey endpoint - not implemented yet",
	})
}
