package pricing

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/apt-router/api/internal/firebase"
)

// Service handles pricing calculations and model configurations
type Service struct {
	firebaseService *firebase.Service
	modelConfigs    map[string]ModelConfig
	mu              sync.RWMutex
	lastRefresh     time.Time
	cacheTTL        time.Duration
}

// ModelConfig represents pricing configuration for a model
type ModelConfig struct {
	ID                    string  `firestore:"id"`
	ModelID               string  `firestore:"model_id"`
	Provider              string  `firestore:"provider"`
	InputPricePerMillion  float64 `firestore:"input_price_per_million"`
	OutputPricePerMillion float64 `firestore:"output_price_per_million"`
	ContextWindowSize     int     `firestore:"context_window_size"`
	IsActive              bool    `firestore:"is_active"`
}

// PricingTier represents a pricing tier (for backward compatibility)
type PricingTier struct {
	ID                  string                  `firestore:"id"`
	TierName            string                  `firestore:"tier_name"`
	MinMonthlySpend     float64                 `firestore:"min_monthly_spend"`
	InputMarkupPercent  float64                 `firestore:"input_markup_percent"`
	OutputMarkupPercent float64                 `firestore:"output_markup_percent"`
	IsActive            bool                    `firestore:"is_active"`
	IsCustom            bool                    `firestore:"is_custom"`
	CustomModelPricing  map[string]ModelPricing `firestore:"custom_model_pricing,omitempty"`
}

// ModelPricing represents custom pricing for specific models
type ModelPricing struct {
	ModelID               string  `firestore:"model_id"`
	Provider              string  `firestore:"provider"`
	InputPricePerMillion  float64 `firestore:"input_price_per_million"`
	OutputPricePerMillion float64 `firestore:"output_price_per_million"`
}

// NewService creates a new pricing service
func NewService(firebaseService *firebase.Service) *Service {
	return &Service{
		firebaseService: firebaseService,
		modelConfigs:    make(map[string]ModelConfig),
		cacheTTL:        5 * time.Minute,
	}
}

// PreCacheData pre-caches model configurations
func (s *Service) PreCacheData(ctx context.Context) error {
	slog.Info("Pre-caching model configurations and pricing tiers")

	// Try to load model configurations from Firestore first
	if err := s.loadModelConfigsFromFirestore(ctx); err != nil {
		slog.Warn("Failed to load model configurations from Firestore, falling back to defaults", "error", err)
		// Only load defaults if Firestore fails
		s.loadDefaultModelConfigs()
	} else {
		slog.Info("Successfully loaded model configurations from Firestore")
	}

	// Try to load pricing tiers from Firestore
	if err := s.loadPricingTiersFromFirestore(ctx); err != nil {
		slog.Warn("Failed to load pricing tiers from Firestore, using on-demand loading", "error", err)
	}

	// Set last refresh time
	s.mu.Lock()
	s.lastRefresh = time.Now()
	s.mu.Unlock()

	slog.Info("Model configurations and pricing tiers pre-cached successfully",
		"model_count", len(s.modelConfigs))
	return nil
}

// loadDefaultModelConfigs loads the default model configurations
func (s *Service) loadDefaultModelConfigs() {
	s.mu.Lock()
	defer s.mu.Unlock()

	// OpenAI Text Models (latest, text-only)
	s.modelConfigs["gpt-4.1-2025-04-14"] = ModelConfig{
		ID:                    "101",
		ModelID:               "gpt-4.1-2025-04-14",
		Provider:              "openai",
		InputPricePerMillion:  2.00,
		OutputPricePerMillion: 8.00,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["gpt-4.1-mini-2025-04-14"] = ModelConfig{
		ID:                    "102",
		ModelID:               "gpt-4.1-mini-2025-04-14",
		Provider:              "openai",
		InputPricePerMillion:  0.40,
		OutputPricePerMillion: 1.60,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["gpt-4.1-nano-2025-04-14"] = ModelConfig{
		ID:                    "103",
		ModelID:               "gpt-4.1-nano-2025-04-14",
		Provider:              "openai",
		InputPricePerMillion:  0.10,
		OutputPricePerMillion: 0.40,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["gpt-4.5-preview-2025-02-27"] = ModelConfig{
		ID:                    "104",
		ModelID:               "gpt-4.5-preview-2025-02-27",
		Provider:              "openai",
		InputPricePerMillion:  75.00,
		OutputPricePerMillion: 150.00,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["gpt-4o-2024-08-06"] = ModelConfig{
		ID:                    "105",
		ModelID:               "gpt-4o-2024-08-06",
		Provider:              "openai",
		InputPricePerMillion:  2.50,
		OutputPricePerMillion: 10.00,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["gpt-4o-2024-11-20"] = ModelConfig{
		ID:                    "106",
		ModelID:               "gpt-4o-2024-11-20",
		Provider:              "openai",
		InputPricePerMillion:  2.50,
		OutputPricePerMillion: 10.00,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["gpt-4o-2024-05-13"] = ModelConfig{
		ID:                    "107",
		ModelID:               "gpt-4o-2024-05-13",
		Provider:              "openai",
		InputPricePerMillion:  5.00,
		OutputPricePerMillion: 15.00,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["gpt-4o-mini-2024-07-18"] = ModelConfig{
		ID:                    "108",
		ModelID:               "gpt-4o-mini-2024-07-18",
		Provider:              "openai",
		InputPricePerMillion:  0.15,
		OutputPricePerMillion: 0.60,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["o1-2024-12-17"] = ModelConfig{
		ID:                    "109",
		ModelID:               "o1-2024-12-17",
		Provider:              "openai",
		InputPricePerMillion:  15.00,
		OutputPricePerMillion: 60.00,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["o3-2025-04-16"] = ModelConfig{
		ID:                    "110",
		ModelID:               "o3-2025-04-16",
		Provider:              "openai",
		InputPricePerMillion:  2.00,
		OutputPricePerMillion: 8.00,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["o3-mini-2025-01-31"] = ModelConfig{
		ID:                    "111",
		ModelID:               "o3-mini-2025-01-31",
		Provider:              "openai",
		InputPricePerMillion:  1.10,
		OutputPricePerMillion: 4.40,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["o1-mini-2024-09-12"] = ModelConfig{
		ID:                    "112",
		ModelID:               "o1-mini-2024-09-12",
		Provider:              "openai",
		InputPricePerMillion:  1.10,
		OutputPricePerMillion: 4.40,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	s.modelConfigs["codex-mini-latest"] = ModelConfig{
		ID:                    "113",
		ModelID:               "codex-mini-latest",
		Provider:              "openai",
		InputPricePerMillion:  1.50,
		OutputPricePerMillion: 6.00,
		ContextWindowSize:     128000,
		IsActive:              true,
	}

	// Google Gemini Models (Text Only)
	s.modelConfigs["gemini-2.5-pro"] = ModelConfig{
		ID:                    "6",
		ModelID:               "gemini-2.5-pro",
		Provider:              "google",
		InputPricePerMillion:  1.25,
		OutputPricePerMillion: 10.00,
		ContextWindowSize:     1000000,
		IsActive:              true,
	}

	s.modelConfigs["gemini-2.5-flash"] = ModelConfig{
		ID:                    "7",
		ModelID:               "gemini-2.5-flash",
		Provider:              "google",
		InputPricePerMillion:  0.30,
		OutputPricePerMillion: 2.50,
		ContextWindowSize:     1000000,
		IsActive:              true,
	}

	s.modelConfigs["gemini-2.5-flash-lite-preview-06-17"] = ModelConfig{
		ID:                    "8",
		ModelID:               "gemini-2.5-flash-lite-preview-06-17",
		Provider:              "google",
		InputPricePerMillion:  0.10,
		OutputPricePerMillion: 0.40,
		ContextWindowSize:     1000000,
		IsActive:              true,
	}

	s.modelConfigs["gemini-2.0-flash"] = ModelConfig{
		ID:                    "9",
		ModelID:               "gemini-2.0-flash",
		Provider:              "google",
		InputPricePerMillion:  0.075,
		OutputPricePerMillion: 0.30,
		ContextWindowSize:     1048576,
		IsActive:              true,
	}

	s.modelConfigs["gemini-2.0-flash-lite"] = ModelConfig{
		ID:                    "10",
		ModelID:               "gemini-2.0-flash-lite",
		Provider:              "google",
		InputPricePerMillion:  0.05,
		OutputPricePerMillion: 0.20,
		ContextWindowSize:     1048576,
		IsActive:              true,
	}

	s.modelConfigs["gemini-1.5-flash"] = ModelConfig{
		ID:                    "11",
		ModelID:               "gemini-1.5-flash",
		Provider:              "google",
		InputPricePerMillion:  0.075,
		OutputPricePerMillion: 0.30,
		ContextWindowSize:     1048576,
		IsActive:              true,
	}

	s.modelConfigs["gemini-1.5-flash-8b"] = ModelConfig{
		ID:                    "12",
		ModelID:               "gemini-1.5-flash-8b",
		Provider:              "google",
		InputPricePerMillion:  0.05,
		OutputPricePerMillion: 0.20,
		ContextWindowSize:     1048576,
		IsActive:              true,
	}

	s.modelConfigs["gemini-1.5-pro"] = ModelConfig{
		ID:                    "13",
		ModelID:               "gemini-1.5-pro",
		Provider:              "google",
		InputPricePerMillion:  3.50,
		OutputPricePerMillion: 10.50,
		ContextWindowSize:     1048576,
		IsActive:              true,
	}

	// Anthropic Claude Models
	s.modelConfigs["claude-opus-4-20250514"] = ModelConfig{
		ID:                    "14",
		ModelID:               "claude-opus-4-20250514",
		Provider:              "anthropic",
		InputPricePerMillion:  15.00,
		OutputPricePerMillion: 75.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-sonnet-4-20250514"] = ModelConfig{
		ID:                    "15",
		ModelID:               "claude-sonnet-4-20250514",
		Provider:              "anthropic",
		InputPricePerMillion:  3.00,
		OutputPricePerMillion: 15.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-3-7-sonnet-20250219"] = ModelConfig{
		ID:                    "16",
		ModelID:               "claude-3-7-sonnet-20250219",
		Provider:              "anthropic",
		InputPricePerMillion:  3.00,
		OutputPricePerMillion: 15.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-3-5-sonnet-20241022"] = ModelConfig{
		ID:                    "17",
		ModelID:               "claude-3-5-sonnet-20241022",
		Provider:              "anthropic",
		InputPricePerMillion:  3.00,
		OutputPricePerMillion: 15.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-3-5-sonnet-20240620"] = ModelConfig{
		ID:                    "18",
		ModelID:               "claude-3-5-sonnet-20240620",
		Provider:              "anthropic",
		InputPricePerMillion:  3.00,
		OutputPricePerMillion: 15.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-3-5-haiku-20241022"] = ModelConfig{
		ID:                    "19",
		ModelID:               "claude-3-5-haiku-20241022",
		Provider:              "anthropic",
		InputPricePerMillion:  0.80,
		OutputPricePerMillion: 4.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-3-opus-20240229"] = ModelConfig{
		ID:                    "20",
		ModelID:               "claude-3-opus-20240229",
		Provider:              "anthropic",
		InputPricePerMillion:  15.00,
		OutputPricePerMillion: 75.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-3-haiku-20240307"] = ModelConfig{
		ID:                    "21",
		ModelID:               "claude-3-haiku-20240307",
		Provider:              "anthropic",
		InputPricePerMillion:  0.25,
		OutputPricePerMillion: 1.25,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	// Anthropic Model Aliases (for convenience)
	s.modelConfigs["claude-opus-4-0"] = ModelConfig{
		ID:                    "22",
		ModelID:               "claude-opus-4-0", // Alias for claude-opus-4-20250514
		Provider:              "anthropic",
		InputPricePerMillion:  15.00,
		OutputPricePerMillion: 75.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-sonnet-4-0"] = ModelConfig{
		ID:                    "23",
		ModelID:               "claude-sonnet-4-0", // Alias for claude-sonnet-4-20250514
		Provider:              "anthropic",
		InputPricePerMillion:  3.00,
		OutputPricePerMillion: 15.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-3-7-sonnet-latest"] = ModelConfig{
		ID:                    "24",
		ModelID:               "claude-3-7-sonnet-latest", // Alias for claude-3-7-sonnet-20250219
		Provider:              "anthropic",
		InputPricePerMillion:  3.00,
		OutputPricePerMillion: 15.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-3-5-sonnet-latest"] = ModelConfig{
		ID:                    "25",
		ModelID:               "claude-3-5-sonnet-latest", // Alias for claude-3-5-sonnet-20241022
		Provider:              "anthropic",
		InputPricePerMillion:  3.00,
		OutputPricePerMillion: 15.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}

	s.modelConfigs["claude-3-5-haiku-latest"] = ModelConfig{
		ID:                    "26",
		ModelID:               "claude-3-5-haiku-latest", // Alias for claude-3-5-haiku-20241022
		Provider:              "anthropic",
		InputPricePerMillion:  0.80,
		OutputPricePerMillion: 4.00,
		ContextWindowSize:     200000,
		IsActive:              true,
	}
}

// GetModelConfig gets the configuration for a specific model
func (s *Service) GetModelConfig(modelID string) (ModelConfig, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	config, exists := s.modelConfigs[modelID]
	if !exists {
		return ModelConfig{}, fmt.Errorf("model config not found for model ID: %s", modelID)
	}

	if !config.IsActive {
		return ModelConfig{}, fmt.Errorf("model is not active: %s", modelID)
	}

	return config, nil
}

// GetPricingTier gets a pricing tier by ID (for backward compatibility)
func (s *Service) GetPricingTier(ctx context.Context, userID string) (PricingTier, error) {
	// Get user from Firebase
	user, err := s.firebaseService.GetUserByID(ctx, userID)
	if err != nil {
		return PricingTier{}, fmt.Errorf("failed to get user: %w", err)
	}

	// Get pricing tier from Firebase
	tier, err := s.firebaseService.GetPricingTier(ctx, user.TierID)
	if err != nil {
		// Fallback to default tier
		tier, err = s.firebaseService.GetDefaultPricingTier(ctx)
		if err != nil {
			return PricingTier{}, fmt.Errorf("failed to get pricing tier: %w", err)
		}
	}

	// Convert Firebase ModelPricing to pricing ModelPricing
	customModelPricing := make(map[string]ModelPricing)
	for modelID, modelPricing := range tier.CustomModelPricing {
		customModelPricing[modelID] = ModelPricing{
			ModelID:               modelPricing.ModelID,
			Provider:              modelPricing.Provider,
			InputPricePerMillion:  modelPricing.InputPricePerMillion,
			OutputPricePerMillion: modelPricing.OutputPricePerMillion,
		}
	}

	// Convert to PricingTier format
	return PricingTier{
		ID:                  tier.ID,
		TierName:            tier.Name,
		MinMonthlySpend:     tier.MinMonthlySpend,
		InputMarkupPercent:  tier.InputMarkupPercent,
		OutputMarkupPercent: tier.OutputMarkupPercent,
		IsActive:            tier.IsActive,
		IsCustom:            tier.IsCustom,
		CustomModelPricing:  customModelPricing,
	}, nil
}

// CalculateCost calculates the cost for a request with percentage-based markup
func (s *Service) CalculateCost(ctx context.Context, userID, modelID string, inputTokens, outputTokens int) (float64, float64, error) {
	// Get user from Firebase
	user, err := s.firebaseService.GetUserByID(ctx, userID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get user: %w", err)
	}

	// Get model configuration
	modelConfig, err := s.GetModelConfig(modelID)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get model config: %w", err)
	}

	// Calculate cost with Firebase service
	totalCost, markupAmount, err := s.firebaseService.CalculateCost(
		ctx,
		user,
		modelID,
		modelConfig.Provider,
		inputTokens,
		outputTokens,
		modelConfig.InputPricePerMillion,
		modelConfig.OutputPricePerMillion,
	)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to calculate cost: %w", err)
	}

	return totalCost, markupAmount, nil
}

// CalculateSavingsFee calculates the savings fee based on tokens saved
func (s *Service) CalculateSavingsFee(tier PricingTier, inputTokensSaved, outputTokensSaved int) float64 {
	// Calculate savings based on the tier's markup percentages
	inputSavings := float64(inputTokensSaved) * (tier.InputMarkupPercent / 100) / 1000000
	outputSavings := float64(outputTokensSaved) * (tier.OutputMarkupPercent / 100) / 1000000

	return inputSavings + outputSavings
}

// RefreshCache refreshes the cached data
func (s *Service) RefreshCache(ctx context.Context) error {
	slog.Info("Refreshing pricing cache")

	// Try to reload model configurations from Firestore first
	if err := s.loadModelConfigsFromFirestore(ctx); err != nil {
		slog.Warn("Failed to refresh model configurations from Firestore, falling back to defaults", "error", err)
		// Only load defaults if Firestore fails
		s.loadDefaultModelConfigs()
	} else {
		slog.Info("Successfully refreshed model configurations from Firestore")
	}

	// Update last refresh time
	s.mu.Lock()
	s.lastRefresh = time.Now()
	s.mu.Unlock()

	slog.Info("Pricing cache refreshed successfully", "model_count", len(s.modelConfigs))
	return nil
}

// shouldRefreshCache checks if the cache should be refreshed
func (s *Service) shouldRefreshCache() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return time.Since(s.lastRefresh) > s.cacheTTL
}

// GetCacheStats returns cache statistics
func (s *Service) GetCacheStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"model_configs_count": len(s.modelConfigs),
		"last_refresh":        s.lastRefresh,
		"cache_ttl":           s.cacheTTL,
		"should_refresh":      s.shouldRefreshCache(),
	}
}

// LoadDefaultModelConfigs loads the default model configurations (for testing)
func (s *Service) LoadDefaultModelConfigs() {
	s.loadDefaultModelConfigs()
}

// loadModelConfigsFromFirestore loads model configurations from Firestore
func (s *Service) loadModelConfigsFromFirestore(ctx context.Context) error {
	iter := s.firebaseService.DB().Collection("model_configurations").Documents(ctx)
	defer iter.Stop()

	s.mu.Lock()
	defer s.mu.Unlock()

	count := 0
	for {
		doc, err := iter.Next()
		if err != nil {
			break // End of iteration
		}

		slog.Debug("Processing model configuration document", "doc_id", doc.Ref.ID)

		var modelConfig ModelConfig
		if err := doc.DataTo(&modelConfig); err != nil {
			slog.Warn("Failed to parse model configuration", "doc_id", doc.Ref.ID, "error", err)
			continue
		}

		slog.Debug("Successfully parsed model configuration", "model_id", modelConfig.ModelID, "provider", modelConfig.Provider)

		// Use the document ID as the key
		s.modelConfigs[modelConfig.ModelID] = modelConfig
		count++
	}

	slog.Info("Loaded model configurations from Firestore", "count", count, "total_loaded", len(s.modelConfigs))
	return nil
}

// loadPricingTiersFromFirestore loads pricing tiers from Firestore
func (s *Service) loadPricingTiersFromFirestore(ctx context.Context) error {
	// This method is a placeholder for future implementation
	// Currently, pricing tiers are loaded on-demand from Firebase
	slog.Info("Pricing tiers will be loaded on-demand from Firestore")
	return nil
}
