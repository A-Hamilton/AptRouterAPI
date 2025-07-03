package handlers

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/apt-router/api/internal/data"
	"github.com/apt-router/api/internal/services"
	"github.com/apt-router/api/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
)

// Handler handles all API requests
type Handler struct {
	config            *utils.Config
	firebaseService   *data.Service
	cache             *cache.Cache
	pricingService    *services.PricingService
	generationService *services.GenerationService
}

// NewHandler creates a new API handler
func NewHandler(
	cfg *utils.Config,
	firebaseService *data.Service,
	cache *cache.Cache,
	pricingService *services.PricingService,
) *Handler {
	generationService := services.NewGenerationService(cfg, firebaseService, cache, pricingService)

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
		var user *data.User
		var err error

		if apiKey == "mock-api-key-for-development" {
			// Create mock user for development
			user = &data.User{
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
			PricingTier: services.PricingTier{
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
	requestCtx.Logger.Info("Handler entered", "request_id", requestCtx.RequestID, "timestamp", time.Now().Format(time.RFC3339Nano))

	// Parse request
	var req GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format: " + err.Error(),
		})
		return
	}

	// Convert HTTP request to service request
	serviceReq := &services.GenerationRequest{
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
	result, err := h.generationService.Generate(c.Request.Context(), serviceReq, &services.RequestContext{
		RequestID:   requestCtx.RequestID,
		UserID:      requestCtx.UserID,
		APIKeyID:    requestCtx.APIKeyID,
		PricingTier: requestCtx.PricingTier,
		Logger:      requestCtx.Logger,
		CachedUser:  convertCachedUserData(requestCtx.CachedUser),
	})
	if err != nil {
		requestCtx.Logger.Error("Generation failed", "error", err, "model", req.Model, "provider", "openai")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Generation failed: %v", err),
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
func (h *Handler) logRequest(ctx context.Context, requestCtx *RequestContext, req *services.GenerationRequest, result *services.GenerationResult, totalCost, markupAmount float64, startTime, endTime time.Time, streaming bool) error {
	// Create request log
	log := &data.RequestLog{
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
	requestCtx.Logger.Info("Streaming handler entered", "request_id", requestCtx.RequestID, "timestamp", time.Now().Format(time.RFC3339Nano))

	// Parse request
	var req GenerateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request format: " + err.Error(),
		})
		return
	}

	// Convert HTTP request to service request
	serviceReq := &services.GenerationRequest{
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

	// Set up streaming response headers immediately
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Request-ID", requestCtx.RequestID)

	// Call service layer for streaming
	streamResp, err := h.generationService.GenerateStream(c.Request.Context(), serviceReq, &services.RequestContext{
		RequestID:   requestCtx.RequestID,
		UserID:      requestCtx.UserID,
		APIKeyID:    requestCtx.APIKeyID,
		PricingTier: requestCtx.PricingTier,
		Logger:      requestCtx.Logger,
		CachedUser:  convertCachedUserData(requestCtx.CachedUser),
	})
	if err != nil {
		requestCtx.Logger.Error("Streaming generation failed", "error", err)
		// Don't try to write to the response if the stream failed to start
		// just return since the connection might be closed.
		return
	}

	// Use c.Stream for a more robust streaming implementation
	c.Stream(func(w io.Writer) bool {
		buf := make([]byte, 1024)
		n, err := streamResp.Stream.Read(buf)
		if n > 0 {
			// SSE format: data: <json-payload>\n\n
			data := fmt.Sprintf("data: %s\n\n", string(buf[:n]))
			if _, writeErr := w.Write([]byte(data)); writeErr != nil {
				requestCtx.Logger.Error("Failed to write chunk to stream", "error", writeErr)
				return false // Stop streaming
			}
		}

		if err != nil {
			if err != io.EOF {
				requestCtx.Logger.Error("Streaming: Read error from source", "error", err)
			} else {
				requestCtx.Logger.Info("Streaming: EOF reached from source")
			}
			// Stop streaming on any error, including EOF
			return false
		}

		return true // Continue streaming
	})

	requestCtx.Logger.Info("Streaming request completed", "request_id", requestCtx.RequestID, "duration_ms", time.Since(startTime).Milliseconds())
	// Note: Full request logging (with token counts, cost, etc.) is more complex for streams.
	// This would typically be handled by the generation service after the stream is fully consumed.
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

// convertCachedUserData converts handlers.CachedUserData to services.CachedUserData
func convertCachedUserData(cachedUser *CachedUserData) *services.CachedUserData {
	if cachedUser == nil {
		return nil
	}
	return &services.CachedUserData{
		ID:            cachedUser.ID,
		Email:         cachedUser.Email,
		Balance:       cachedUser.Balance,
		TierID:        cachedUser.TierID,
		IsActive:      cachedUser.IsActive,
		CustomPricing: cachedUser.CustomPricing,
		LastUpdated:   cachedUser.LastUpdated,
	}
}
