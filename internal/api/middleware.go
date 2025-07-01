package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/apt-router/api/internal/pricing"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// RequestContext contains request-scoped data
type RequestContext struct {
	RequestID   string
	UserID      string
	APIKeyID    string
	PricingTier pricing.PricingTier
	Logger      *slog.Logger
}

// RequestLogger middleware generates a unique request_id and injects a request-scoped logger
func (h *Handler) RequestLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		requestID := uuid.New().String()

		// Create request-scoped logger
		logger := slog.With(
			"request_id", requestID,
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"user_agent", c.Request.UserAgent(),
			"remote_addr", c.ClientIP(),
		)

		// Store logger in context
		ctx := context.WithValue(c.Request.Context(), "logger", logger)
		c.Request = c.Request.WithContext(ctx)

		// Store request ID in context
		ctx = context.WithValue(ctx, "request_id", requestID)
		c.Request = c.Request.WithContext(ctx)

		// Process request
		c.Next()

		// Log request completion
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

// hashAPIKey hashes the API key using SHA-256
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
	if requestID, exists := c.Request.Context().Value("request_id").(string); exists {
		return requestID
	}
	return uuid.New().String()
}

// getLogger gets the logger from the context
func (h *Handler) getLogger(c *gin.Context) *slog.Logger {
	if logger, exists := c.Request.Context().Value("logger").(*slog.Logger); exists {
		return logger
	}
	return slog.Default()
}

// getRequestContext gets the request context from Gin context
func (h *Handler) getRequestContext(c *gin.Context) (*RequestContext, bool) {
	if ctx, exists := c.Get("request_context"); exists {
		if requestCtx, ok := ctx.(*RequestContext); ok {
			return requestCtx, true
		}
	}
	return nil, false
}
