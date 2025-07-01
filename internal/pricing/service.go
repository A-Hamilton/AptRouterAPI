package pricing

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	supabase "github.com/supabase-community/supabase-go"
)

// Service handles pricing tier calculations and model configurations
type Service struct {
	client       *supabase.Client
	pricingTiers map[string]PricingTier
	modelConfigs map[string]ModelConfig
	mu           sync.RWMutex
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

// NewService creates a new pricing service
func NewService(client *supabase.Client) *Service {
	return &Service{
		client:       client,
		pricingTiers: make(map[string]PricingTier),
		modelConfigs: make(map[string]ModelConfig),
	}
}

// PreCacheData loads pricing tiers and model configs into memory
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

	slog.Info("Pricing data pre-cached successfully",
		"pricing_tiers", len(s.pricingTiers),
		"model_configs", len(s.modelConfigs))
	return nil
}

// loadPricingTiers loads pricing tiers from the database
func (s *Service) loadPricingTiers(ctx context.Context) error {
	// TODO: Implement proper Supabase query
	// For now, use default pricing tiers
	s.mu.Lock()
	defer s.mu.Unlock()

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
	// For now, use default model configs
	s.mu.Lock()
	defer s.mu.Unlock()

	s.modelConfigs = map[string]ModelConfig{
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
		"claude-3-5-sonnet-20241022": {
			ID: 4, ModelID: "claude-3-5-sonnet-20241022", Provider: "anthropic",
			InputPricePerMillion: 3.00, OutputPricePerMillion: 15.00, ContextWindowSize: 200000,
		},
		"claude-3-5-haiku-20241022": {
			ID: 5, ModelID: "claude-3-5-haiku-20241022", Provider: "anthropic",
			InputPricePerMillion: 0.25, OutputPricePerMillion: 1.25, ContextWindowSize: 200000,
		},
		"gemini-1.5-pro": {
			ID: 6, ModelID: "gemini-1.5-pro", Provider: "google",
			InputPricePerMillion: 3.50, OutputPricePerMillion: 10.50, ContextWindowSize: 1000000,
		},
		"gemini-1.5-flash": {
			ID: 7, ModelID: "gemini-1.5-flash", Provider: "google",
			InputPricePerMillion: 0.075, OutputPricePerMillion: 0.30, ContextWindowSize: 1000000,
		},
	}

	return nil
}

// GetPricingTier returns the pricing tier for a given user based on their monthly request count
func (s *Service) GetPricingTier(ctx context.Context, userID string) (PricingTier, error) {
	// TODO: Implement proper RPC call to get user monthly requests
	// For now, return the free tier
	monthlyRequests := 0

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Find the appropriate tier
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
func (s *Service) GetModelConfig(modelID string) (ModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	config, exists := s.modelConfigs[modelID]
	if !exists {
		return ModelConfig{}, fmt.Errorf("model config not found for model ID: %s", modelID)
	}

	return config, nil
}

// CalculateCost calculates the cost for a request based on input and output tokens
func (s *Service) CalculateCost(modelID string, inputTokens, outputTokens int) (float64, error) {
	config, err := s.GetModelConfig(modelID)
	if err != nil {
		return 0, err
	}

	inputCost := (float64(inputTokens) / 1000000) * config.InputPricePerMillion
	outputCost := (float64(outputTokens) / 1000000) * config.OutputPricePerMillion

	return inputCost + outputCost, nil
}

// CalculateSavingsFee calculates the fee charged for token savings
func (s *Service) CalculateSavingsFee(tier PricingTier, inputTokensSaved, outputTokensSaved int) float64 {
	inputSavingsFee := (float64(inputTokensSaved) / 1000000) * tier.InputSavingsRateUSDPerMillion
	outputSavingsFee := (float64(outputTokensSaved) / 1000000) * tier.OutputSavingsRateUSDPerMillion

	return inputSavingsFee + outputSavingsFee
}

// RefreshCache refreshes the cached data from the database
func (s *Service) RefreshCache(ctx context.Context) error {
	return s.PreCacheData(ctx)
}
