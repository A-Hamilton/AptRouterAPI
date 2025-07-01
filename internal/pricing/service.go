package pricing

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	supabase "github.com/supabase-community/supabase-go"
)

// Service handles pricing tier calculations and model configurations with optimized caching
type Service struct {
	client       *supabase.Client
	pricingTiers map[string]PricingTier
	modelConfigs map[string]ModelConfig
	mu           sync.RWMutex
	lastRefresh  time.Time
	cacheTTL     time.Duration
}

// PricingTier represents a pricing tier configuration
type PricingTier struct {
	ID                             int     `json:"id"`
	TierName                       string  `json:"tier_name"`
	MinMonthlyRequests             int     `json:"min_monthly_requests"`
	InputSavingsRateUSDPerMillion  float64 `json:"input_savings_rate_usd_per_million"`
	OutputSavingsRateUSDPerMillion float64 `json:"output_savings_rate_usd_per_million"`
}

// ModelConfig represents a model configuration
type ModelConfig struct {
	ID                    int     `json:"id"`
	ModelID               string  `json:"model_id"`
	Provider              string  `json:"provider"`
	InputPricePerMillion  float64 `json:"input_price_per_million"`
	OutputPricePerMillion float64 `json:"output_price_per_million"`
	ContextWindowSize     int     `json:"context_window_size"`
}

// NewService creates a new pricing service with optimized caching
func NewService(client *supabase.Client) *Service {
	return &Service{
		client:       client,
		pricingTiers: make(map[string]PricingTier, 10), // Pre-allocate with expected capacity
		modelConfigs: make(map[string]ModelConfig, 20), // Pre-allocate with expected capacity
		cacheTTL:     1 * time.Hour,                    // Cache for 1 hour
	}
}

// PreCacheData loads pricing tiers and model configs into memory with timeout
func (s *Service) PreCacheData(ctx context.Context) error {
	slog.Info("Pre-caching pricing data...")

	// Load pricing tiers
	if err := s.loadPricingTiers(ctx); err != nil {
		return fmt.Errorf("failed to load pricing tiers: %w", err)
	}

	// Load model configs
	if err := s.loadModelConfigs(ctx); err != nil {
		return fmt.Errorf("failed to load model configs: %w", err)
	}

	s.lastRefresh = time.Now()
	slog.Info("Pricing data pre-cached successfully",
		"pricing_tiers", len(s.pricingTiers),
		"model_configs", len(s.modelConfigs),
		"cache_ttl", s.cacheTTL)
	return nil
}

// loadPricingTiers loads pricing tiers from the database
func (s *Service) loadPricingTiers(ctx context.Context) error {
	// TODO: Implement proper Supabase query
	// For now, use default pricing tiers
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use a more efficient map initialization
	s.pricingTiers = map[string]PricingTier{
		"free": {
			ID: 1, TierName: "free", MinMonthlyRequests: 0,
			InputSavingsRateUSDPerMillion: 0.50, OutputSavingsRateUSDPerMillion: 1.50,
		},
		"starter": {
			ID: 2, TierName: "starter", MinMonthlyRequests: 1000,
			InputSavingsRateUSDPerMillion: 0.40, OutputSavingsRateUSDPerMillion: 1.20,
		},
		"pro": {
			ID: 3, TierName: "pro", MinMonthlyRequests: 10000,
			InputSavingsRateUSDPerMillion: 0.30, OutputSavingsRateUSDPerMillion: 0.90,
		},
		"enterprise": {
			ID: 4, TierName: "enterprise", MinMonthlyRequests: 100000,
			InputSavingsRateUSDPerMillion: 0.20, OutputSavingsRateUSDPerMillion: 0.60,
		},
	}

	return nil
}

// loadModelConfigs loads model configurations from the database
func (s *Service) loadModelConfigs(ctx context.Context) error {
	// TODO: Implement proper Supabase query
	// For now, use default model configs with actual available models
	s.mu.Lock()
	defer s.mu.Unlock()

	// Use a more efficient map initialization with actual available models
	s.modelConfigs = map[string]ModelConfig{
		// OpenAI models - using actual available constants
		"gpt-4o": {
			ID: 1, ModelID: "gpt-4o", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4o-mini": {
			ID: 2, ModelID: "gpt-4o-mini", Provider: "openai",
			InputPricePerMillion: 0.15, OutputPricePerMillion: 0.60, ContextWindowSize: 128000,
		},
		"gpt-3.5-turbo": {
			ID: 3, ModelID: "gpt-3.5-turbo", Provider: "openai",
			InputPricePerMillion: 0.50, OutputPricePerMillion: 1.50, ContextWindowSize: 16385,
		},
		"gpt-4o-latest": {
			ID: 4, ModelID: "gpt-4o-latest", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4o-2024-05-13": {
			ID: 5, ModelID: "gpt-4o-2024-05-13", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4o-2024-08-06": {
			ID: 6, ModelID: "gpt-4o-2024-08-06", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4o-2024-11-20": {
			ID: 7, ModelID: "gpt-4o-2024-11-20", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4o-mini-2024-07-18": {
			ID: 9, ModelID: "gpt-4o-mini-2024-07-18", Provider: "openai",
			InputPricePerMillion: 0.15, OutputPricePerMillion: 0.60, ContextWindowSize: 128000,
		},
		"gpt-3.5-turbo-0125": {
			ID: 11, ModelID: "gpt-3.5-turbo-0125", Provider: "openai",
			InputPricePerMillion: 0.50, OutputPricePerMillion: 1.50, ContextWindowSize: 16385,
		},
		"gpt-3.5-turbo-0301": {
			ID: 12, ModelID: "gpt-3.5-turbo-0301", Provider: "openai",
			InputPricePerMillion: 0.50, OutputPricePerMillion: 1.50, ContextWindowSize: 16385,
		},
		"gpt-3.5-turbo-0613": {
			ID: 13, ModelID: "gpt-3.5-turbo-0613", Provider: "openai",
			InputPricePerMillion: 0.50, OutputPricePerMillion: 1.50, ContextWindowSize: 16385,
		},
		"gpt-3.5-turbo-1106": {
			ID: 14, ModelID: "gpt-3.5-turbo-1106", Provider: "openai",
			InputPricePerMillion: 0.50, OutputPricePerMillion: 1.50, ContextWindowSize: 16385,
		},
		"gpt-3.5-turbo-16k": {
			ID: 15, ModelID: "gpt-3.5-turbo-16k", Provider: "openai",
			InputPricePerMillion: 0.50, OutputPricePerMillion: 1.50, ContextWindowSize: 16385,
		},
		"gpt-3.5-turbo-16k-0613": {
			ID: 16, ModelID: "gpt-3.5-turbo-16k-0613", Provider: "openai",
			InputPricePerMillion: 0.50, OutputPricePerMillion: 1.50, ContextWindowSize: 16385,
		},
		"gpt-4": {
			ID: 17, ModelID: "gpt-4", Provider: "openai",
			InputPricePerMillion: 30.00, OutputPricePerMillion: 60.00, ContextWindowSize: 8192,
		},
		"gpt-4-turbo": {
			ID: 18, ModelID: "gpt-4-turbo", Provider: "openai",
			InputPricePerMillion: 10.00, OutputPricePerMillion: 30.00, ContextWindowSize: 128000,
		},
		"gpt-4-turbo-2024-04-09": {
			ID: 19, ModelID: "gpt-4-turbo-2024-04-09", Provider: "openai",
			InputPricePerMillion: 10.00, OutputPricePerMillion: 30.00, ContextWindowSize: 128000,
		},
		"gpt-4-turbo-preview": {
			ID: 20, ModelID: "gpt-4-turbo-preview", Provider: "openai",
			InputPricePerMillion: 10.00, OutputPricePerMillion: 30.00, ContextWindowSize: 128000,
		},
		"gpt-4-vision-preview": {
			ID: 21, ModelID: "gpt-4-vision-preview", Provider: "openai",
			InputPricePerMillion: 10.00, OutputPricePerMillion: 30.00, ContextWindowSize: 128000,
		},
		"gpt-4-0125-preview": {
			ID: 22, ModelID: "gpt-4-0125-preview", Provider: "openai",
			InputPricePerMillion: 10.00, OutputPricePerMillion: 30.00, ContextWindowSize: 128000,
		},
		"gpt-4-0314": {
			ID: 23, ModelID: "gpt-4-0314", Provider: "openai",
			InputPricePerMillion: 30.00, OutputPricePerMillion: 60.00, ContextWindowSize: 8192,
		},
		"gpt-4-0613": {
			ID: 24, ModelID: "gpt-4-0613", Provider: "openai",
			InputPricePerMillion: 30.00, OutputPricePerMillion: 60.00, ContextWindowSize: 8192,
		},
		"gpt-4-1": {
			ID: 25, ModelID: "gpt-4.1", Provider: "openai",
			InputPricePerMillion: 30.00, OutputPricePerMillion: 60.00, ContextWindowSize: 8192,
		},
		"gpt-4-1106-preview": {
			ID: 26, ModelID: "gpt-4-1106-preview", Provider: "openai",
			InputPricePerMillion: 10.00, OutputPricePerMillion: 30.00, ContextWindowSize: 128000,
		},
		"gpt-4-1-mini": {
			ID: 27, ModelID: "gpt-4.1-mini", Provider: "openai",
			InputPricePerMillion: 15.00, OutputPricePerMillion: 45.00, ContextWindowSize: 128000,
		},
		"gpt-4-1-mini-2025-04-14": {
			ID: 28, ModelID: "gpt-4.1-mini-2025-04-14", Provider: "openai",
			InputPricePerMillion: 15.00, OutputPricePerMillion: 45.00, ContextWindowSize: 128000,
		},
		"gpt-4-1-nano": {
			ID: 29, ModelID: "gpt-4.1-nano", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4-1-nano-2025-04-14": {
			ID: 30, ModelID: "gpt-4.1-nano-2025-04-14", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4-1-2025-04-14": {
			ID: 31, ModelID: "gpt-4.1-2025-04-14", Provider: "openai",
			InputPricePerMillion: 30.00, OutputPricePerMillion: 60.00, ContextWindowSize: 8192,
		},
		"gpt-4-32k": {
			ID: 32, ModelID: "gpt-4-32k", Provider: "openai",
			InputPricePerMillion: 60.00, OutputPricePerMillion: 120.00, ContextWindowSize: 32768,
		},
		"gpt-4-32k-0314": {
			ID: 33, ModelID: "gpt-4-32k-0314", Provider: "openai",
			InputPricePerMillion: 60.00, OutputPricePerMillion: 120.00, ContextWindowSize: 32768,
		},
		"gpt-4-32k-0613": {
			ID: 34, ModelID: "gpt-4-32k-0613", Provider: "openai",
			InputPricePerMillion: 60.00, OutputPricePerMillion: 120.00, ContextWindowSize: 32768,
		},
		"gpt-4o-audio-preview": {
			ID: 35, ModelID: "gpt-4o-audio-preview", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4o-audio-preview-2024-10-01": {
			ID: 36, ModelID: "gpt-4o-audio-preview-2024-10-01", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4o-audio-preview-2024-12-17": {
			ID: 37, ModelID: "gpt-4o-audio-preview-2024-12-17", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4o-audio-preview-2025-06-03": {
			ID: 38, ModelID: "gpt-4o-audio-preview-2025-06-03", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4o-mini-audio-preview": {
			ID: 39, ModelID: "gpt-4o-mini-audio-preview", Provider: "openai",
			InputPricePerMillion: 0.15, OutputPricePerMillion: 0.60, ContextWindowSize: 128000,
		},
		"gpt-4o-mini-audio-preview-2024-12-17": {
			ID: 40, ModelID: "gpt-4o-mini-audio-preview-2024-12-17", Provider: "openai",
			InputPricePerMillion: 0.15, OutputPricePerMillion: 0.60, ContextWindowSize: 128000,
		},
		"gpt-4o-mini-search-preview": {
			ID: 41, ModelID: "gpt-4o-mini-search-preview", Provider: "openai",
			InputPricePerMillion: 0.15, OutputPricePerMillion: 0.60, ContextWindowSize: 128000,
		},
		"gpt-4o-mini-search-preview-2025-03-11": {
			ID: 42, ModelID: "gpt-4o-mini-search-preview-2025-03-11", Provider: "openai",
			InputPricePerMillion: 0.15, OutputPricePerMillion: 0.60, ContextWindowSize: 128000,
		},
		"gpt-4o-search-preview": {
			ID: 43, ModelID: "gpt-4o-search-preview", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"gpt-4o-search-preview-2025-03-11": {
			ID: 44, ModelID: "gpt-4o-search-preview-2025-03-11", Provider: "openai",
			InputPricePerMillion: 5.00, OutputPricePerMillion: 15.00, ContextWindowSize: 128000,
		},
		"o1": {
			ID: 45, ModelID: "o1", Provider: "openai",
			InputPricePerMillion: 15.00, OutputPricePerMillion: 60.00, ContextWindowSize: 32768,
		},
		"o1-mini": {
			ID: 46, ModelID: "o1-mini", Provider: "openai",
			InputPricePerMillion: 3.00, OutputPricePerMillion: 15.00, ContextWindowSize: 32768,
		},
		"o1-mini-2024-09-12": {
			ID: 47, ModelID: "o1-mini-2024-09-12", Provider: "openai",
			InputPricePerMillion: 3.00, OutputPricePerMillion: 15.00, ContextWindowSize: 32768,
		},
		"o1-preview": {
			ID: 48, ModelID: "o1-preview", Provider: "openai",
			InputPricePerMillion: 15.00, OutputPricePerMillion: 60.00, ContextWindowSize: 32768,
		},
		"o1-preview-2024-09-12": {
			ID: 49, ModelID: "o1-preview-2024-09-12", Provider: "openai",
			InputPricePerMillion: 15.00, OutputPricePerMillion: 60.00, ContextWindowSize: 32768,
		},
		"o1-2024-12-17": {
			ID: 50, ModelID: "o1-2024-12-17", Provider: "openai",
			InputPricePerMillion: 15.00, OutputPricePerMillion: 60.00, ContextWindowSize: 32768,
		},
		"o3": {
			ID: 51, ModelID: "o3", Provider: "openai",
			InputPricePerMillion: 15.00, OutputPricePerMillion: 60.00, ContextWindowSize: 32768,
		},
		"o3-mini": {
			ID: 52, ModelID: "o3-mini", Provider: "openai",
			InputPricePerMillion: 3.00, OutputPricePerMillion: 15.00, ContextWindowSize: 32768,
		},
		"o3-mini-2025-01-31": {
			ID: 53, ModelID: "o3-mini-2025-01-31", Provider: "openai",
			InputPricePerMillion: 3.00, OutputPricePerMillion: 15.00, ContextWindowSize: 32768,
		},
		"o3-2025-04-16": {
			ID: 54, ModelID: "o3-2025-04-16", Provider: "openai",
			InputPricePerMillion: 15.00, OutputPricePerMillion: 60.00, ContextWindowSize: 32768,
		},
		"o4-mini": {
			ID: 55, ModelID: "o4-mini", Provider: "openai",
			InputPricePerMillion: 3.00, OutputPricePerMillion: 15.00, ContextWindowSize: 32768,
		},
		"o4-mini-2025-04-16": {
			ID: 56, ModelID: "o4-mini-2025-04-16", Provider: "openai",
			InputPricePerMillion: 3.00, OutputPricePerMillion: 15.00, ContextWindowSize: 32768,
		},
		"codex-mini-latest": {
			ID: 57, ModelID: "codex-mini-latest", Provider: "openai",
			InputPricePerMillion: 0.50, OutputPricePerMillion: 1.50, ContextWindowSize: 16385,
		},

		// Anthropic models - using actual available models
		"claude-3-5-sonnet-20241022": {
			ID: 58, ModelID: "claude-3-5-sonnet-20241022", Provider: "anthropic",
			InputPricePerMillion: 3.00, OutputPricePerMillion: 15.00, ContextWindowSize: 200000,
		},
		"claude-3-5-haiku-20241022": {
			ID: 59, ModelID: "claude-3-5-haiku-20241022", Provider: "anthropic",
			InputPricePerMillion: 0.25, OutputPricePerMillion: 1.25, ContextWindowSize: 200000,
		},
		"claude-3-haiku-20240307": {
			ID: 60, ModelID: "claude-3-haiku-20240307", Provider: "anthropic",
			InputPricePerMillion: 0.25, OutputPricePerMillion: 1.25, ContextWindowSize: 200000,
		},
		"claude-3-opus-20240229": {
			ID: 61, ModelID: "claude-3-opus-20240229", Provider: "anthropic",
			InputPricePerMillion: 15.00, OutputPricePerMillion: 75.00, ContextWindowSize: 200000,
		},
		"claude-3-sonnet-20240229": {
			ID: 62, ModelID: "claude-3-sonnet-20240229", Provider: "anthropic",
			InputPricePerMillion: 3.00, OutputPricePerMillion: 15.00, ContextWindowSize: 200000,
		},

		// Google Gemini models - using actual available models
		"gemini-1.5-pro": {
			ID: 64, ModelID: "gemini-1.5-pro", Provider: "google",
			InputPricePerMillion: 3.50, OutputPricePerMillion: 10.50, ContextWindowSize: 1000000,
		},
		"gemini-1.5-flash": {
			ID: 65, ModelID: "gemini-1.5-flash", Provider: "google",
			InputPricePerMillion: 0.075, OutputPricePerMillion: 0.30, ContextWindowSize: 1000000,
		},
		"gemini-2.0-flash": {
			ID: 66, ModelID: "gemini-2.0-flash", Provider: "google",
			InputPricePerMillion: 0.075, OutputPricePerMillion: 0.30, ContextWindowSize: 1000000,
		},
	}

	return nil
}

// GetPricingTier returns the pricing tier for a given user based on their monthly request count
// Optimized with read locks for better concurrency
func (s *Service) GetPricingTier(ctx context.Context, userID string) (PricingTier, error) {
	// Check if cache needs refresh
	if s.shouldRefreshCache() {
		if err := s.RefreshCache(ctx); err != nil {
			slog.Warn("Failed to refresh cache, using stale data", "error", err)
		}
	}

	// TODO: Implement proper RPC call to get user monthly requests
	// For now, return the free tier
	monthlyRequests := 0

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find the appropriate tier with optimized search
	var bestTier PricingTier
	var maxRequests int = -1

	for _, tier := range s.pricingTiers {
		if tier.MinMonthlyRequests <= monthlyRequests && tier.MinMonthlyRequests > maxRequests {
			bestTier = tier
			maxRequests = tier.MinMonthlyRequests
		}
	}

	// If no tier found, return the free tier
	if maxRequests == -1 {
		if freeTier, exists := s.pricingTiers["free"]; exists {
			return freeTier, nil
		}
		return PricingTier{}, fmt.Errorf("no pricing tier found and no free tier available")
	}

	return bestTier, nil
}

// GetModelConfig returns the model configuration for a given model ID
// Optimized with read locks for better concurrency
func (s *Service) GetModelConfig(modelID string) (ModelConfig, error) {
	// Check if cache needs refresh
	if s.shouldRefreshCache() {
		// Use background context for refresh to avoid blocking the request
		go func() {
			if err := s.RefreshCache(context.Background()); err != nil {
				slog.Warn("Failed to refresh cache in background", "error", err)
			}
		}()
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	config, exists := s.modelConfigs[modelID]
	if !exists {
		return ModelConfig{}, fmt.Errorf("model config not found for model ID: %s", modelID)
	}

	return config, nil
}

// CalculateCost calculates the cost for a request based on input and output tokens
// Optimized to avoid unnecessary allocations
func (s *Service) CalculateCost(modelID string, inputTokens, outputTokens int) (float64, error) {
	config, err := s.GetModelConfig(modelID)
	if err != nil {
		return 0, err
	}

	// Use more efficient floating point operations
	inputCost := float64(inputTokens) * config.InputPricePerMillion / 1000000
	outputCost := float64(outputTokens) * config.OutputPricePerMillion / 1000000

	return inputCost + outputCost, nil
}

// CalculateSavingsFee calculates the fee charged for token savings
// Optimized to avoid unnecessary allocations
func (s *Service) CalculateSavingsFee(tier PricingTier, inputTokensSaved, outputTokensSaved int) float64 {
	inputSavingsFee := float64(inputTokensSaved) * tier.InputSavingsRateUSDPerMillion / 1000000
	outputSavingsFee := float64(outputTokensSaved) * tier.OutputSavingsRateUSDPerMillion / 1000000

	return inputSavingsFee + outputSavingsFee
}

// RefreshCache refreshes the cached data
func (s *Service) RefreshCache(ctx context.Context) error {
	slog.Info("Refreshing pricing cache...")

	// Load pricing tiers
	if err := s.loadPricingTiers(ctx); err != nil {
		return fmt.Errorf("failed to load pricing tiers: %w", err)
	}

	// Load model configs
	if err := s.loadModelConfigs(ctx); err != nil {
		return fmt.Errorf("failed to load model configs: %w", err)
	}

	s.lastRefresh = time.Now()
	slog.Info("Pricing cache refreshed successfully",
		"pricing_tiers", len(s.pricingTiers),
		"model_configs", len(s.modelConfigs))
	return nil
}

// shouldRefreshCache checks if the cache should be refreshed based on TTL
func (s *Service) shouldRefreshCache() bool {
	return time.Since(s.lastRefresh) > s.cacheTTL
}

// GetCacheStats returns cache statistics for monitoring
func (s *Service) GetCacheStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"pricing_tiers_count": len(s.pricingTiers),
		"model_configs_count": len(s.modelConfigs),
		"last_refresh":        s.lastRefresh,
		"cache_ttl":           s.cacheTTL,
		"should_refresh":      s.shouldRefreshCache(),
	}
}
