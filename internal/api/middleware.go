package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"time"

	"github.com/apt-router/api/internal/pricing"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ContextKey types to avoid collisions with built-in string keys
type contextKey string

const (
	loggerKey         contextKey = "logger"
	requestIDKey      contextKey = "request_id"
	requestContextKey contextKey = "requestContext"
	userCacheKey      contextKey = "userCache"
)

// RequestContext contains request-scoped data
type RequestContext struct {
	RequestID   string
	UserID      string
	APIKeyID    string
	PricingTier pricing.PricingTier
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

// RequestLogger middleware generates a unique request_id and injects a request-scoped logger
func (h *Handler) RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := uuid.New().String()

		// Create request-scoped logger with pre-allocated fields
		logger := slog.With(
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"user_agent", c.Request.UserAgent(),
			"remote_addr", c.ClientIP(),
		)

		// Store logger in context
		ctx := context.WithValue(c.Request.Context(), loggerKey, logger)
		c.Request = c.Request.WithContext(ctx)

		// Store request ID in context
		ctx = context.WithValue(ctx, requestIDKey, requestID)
		c.Request = c.Request.WithContext(ctx)

		// Process request
		c.Next()

		// Log request completion with performance metrics
		duration := time.Since(start)
		logger.Info("Request completed",
			"status", c.Writer.Status(),
			"duration_ms", duration.Milliseconds(),
			"bytes", c.Writer.Size(),
		)
	}
}

// APIKeyData represents an API key from the database
type APIKeyData struct {
	ID      string `json:"id"`
	UserID  string `json:"user_id"`
	KeyHash string `json:"key_hash"`
	Name    string `json:"name"`
	Status  string `json:"status"`
}

// UserProfile represents a user profile from the database
type UserProfile struct {
	ID      string  `json:"id"`
	Email   string  `json:"email"`
	Balance float64 `json:"balance"`
}

// hashAPIKey hashes the API key using SHA-256 with salt
func (h *Handler) hashAPIKey(apiKey string) string {
	// Add salt to the API key before hashing
	saltedKey := apiKey + h.config.Security.APIKeySalt
	hash := sha256.Sum256([]byte(saltedKey))
	return hex.EncodeToString(hash[:])
}

// lookupAPIKey looks up an API key by its hash
func (h *Handler) lookupAPIKey(ctx context.Context, keyHash string) (*APIKeyData, error) {
	// TODO: Implement database lookup using Supabase
	// For now, return a mock API key for development
	if keyHash == "mock_hash_for_development" {
		return &APIKeyData{
			ID:      "mock-api-key-id",
			UserID:  "mock-user-id",
			KeyHash: keyHash,
			Name:    "Development API Key",
			Status:  "active",
		}, nil
	}
	return nil, nil
}

// getUserProfile gets a user profile by user ID
func (h *Handler) getUserProfile(ctx context.Context, userID string) (*UserProfile, error) {
	// TODO: Implement database lookup using Supabase
	// For now, return a mock user profile for development
	return &UserProfile{
		ID:      userID,
		Email:   "user@example.com",
		Balance: 100.00,
	}, nil
}

// getRequestID gets the request ID from the context
func (h *Handler) getRequestID(c *gin.Context) string {
	if requestID, exists := c.Request.Context().Value(requestIDKey).(string); exists {
		return requestID
	}
	return uuid.New().String()
}

// getLogger gets the logger from the context
func (h *Handler) getLogger(c *gin.Context) *slog.Logger {
	if logger, exists := c.Request.Context().Value(loggerKey).(*slog.Logger); exists {
		return logger
	}
	return slog.Default()
}

// GinContextKey types to avoid collisions with built-in string keys
type ginContextKey string

const (
	requestContextGinKey ginContextKey = "requestContext"
)

// getRequestContext gets the request context from Gin context
func (h *Handler) getRequestContext(c *gin.Context) (*RequestContext, bool) {
	if ctx, exists := c.Get(string(requestContextGinKey)); exists {
		if requestCtx, ok := ctx.(*RequestContext); ok {
			return requestCtx, true
		}
	}
	return nil, false
}

// getUserFromCache retrieves user data from cache or loads from Firebase
func (h *Handler) getUserFromCache(ctx context.Context, userID string) (*CachedUserData, error) {
	cacheKey := fmt.Sprintf("user:%s", userID)

	// Try to get from cache first
	if cached, found := h.cache.Get(cacheKey); found {
		if userData, ok := cached.(*CachedUserData); ok {
			// Check if cache is still valid (5 minutes)
			if time.Since(userData.LastUpdated) < 5*time.Minute {
				return userData, nil
			}
		}
	}

	// Cache miss or expired, load from Firebase
	user, err := h.firebaseService.GetUserByID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user from Firebase: %w", err)
	}

	// Create cached user data
	cachedUser := &CachedUserData{
		ID:            user.ID,
		Email:         user.Email,
		Balance:       user.Balance,
		TierID:        user.TierID,
		IsActive:      user.IsActive,
		CustomPricing: user.CustomPricing,
		LastUpdated:   time.Now(),
	}

	// Store in cache for 5 minutes
	h.cache.Set(cacheKey, cachedUser, 5*time.Minute)

	return cachedUser, nil
}

// checkUserBalance performs a quick balance check before processing expensive operations
func (h *Handler) checkUserBalance(ctx context.Context, userID string, estimatedCost float64) (bool, float64, error) {
	// Get user from cache
	cachedUser, err := h.getUserFromCache(ctx, userID)
	if err != nil {
		return false, 0, fmt.Errorf("failed to get user data: %w", err)
	}

	// Check if user is active
	if !cachedUser.IsActive {
		return false, cachedUser.Balance, fmt.Errorf("user account is inactive")
	}

	// Allow negative balance (graceful handling)
	// Users can go into negative balance and it will be deducted from next purchase
	return true, cachedUser.Balance, nil
}

// updateUserBalance updates user balance in both cache and Firebase
func (h *Handler) updateUserBalance(ctx context.Context, userID string, amount float64) error {
	// Update in Firebase first
	err := h.firebaseService.UpdateUserBalance(ctx, userID, amount)
	if err != nil {
		return fmt.Errorf("failed to update user balance in Firebase: %w", err)
	}

	// Invalidate cache to force refresh on next request
	cacheKey := fmt.Sprintf("user:%s", userID)
	h.cache.Delete(cacheKey)

	return nil
}

// getPricingTierFromCache retrieves pricing tier from cache or loads from Firebase
func (h *Handler) getPricingTierFromCache(ctx context.Context, tierID string) (*pricing.PricingTier, error) {
	cacheKey := fmt.Sprintf("tier:%s", tierID)

	// Try to get from cache first
	if cached, found := h.cache.Get(cacheKey); found {
		if tier, ok := cached.(*pricing.PricingTier); ok {
			return tier, nil
		}
	}

	// Cache miss, load from Firebase
	firebaseTier, err := h.firebaseService.GetPricingTier(ctx, tierID)
	if err != nil {
		// Fallback to default tier
		firebaseTier, err = h.firebaseService.GetDefaultPricingTier(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to get pricing tier: %w", err)
		}
	}

	// Convert Firebase ModelPricing to pricing.ModelPricing
	customModelPricing := make(map[string]pricing.ModelPricing)
	for modelID, modelPricing := range firebaseTier.CustomModelPricing {
		customModelPricing[modelID] = pricing.ModelPricing{
			ModelID:               modelPricing.ModelID,
			Provider:              modelPricing.Provider,
			InputPricePerMillion:  modelPricing.InputPricePerMillion,
			OutputPricePerMillion: modelPricing.OutputPricePerMillion,
		}
	}

	// Create pricing tier
	tier := &pricing.PricingTier{
		ID:                  firebaseTier.ID,
		TierName:            firebaseTier.Name,
		MinMonthlySpend:     firebaseTier.MinMonthlySpend,
		InputMarkupPercent:  firebaseTier.InputMarkupPercent,
		OutputMarkupPercent: firebaseTier.OutputMarkupPercent,
		IsActive:            firebaseTier.IsActive,
		IsCustom:            firebaseTier.IsCustom,
		CustomModelPricing:  customModelPricing,
	}

	// Store in cache for 10 minutes (pricing tiers change less frequently)
	h.cache.Set(cacheKey, tier, 10*time.Minute)

	return tier, nil
}
