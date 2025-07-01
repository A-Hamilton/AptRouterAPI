package main

import (
	"context"
	"log"
	"time"

	"github.com/apt-router/api/internal/firebase"
)

func main() {
	ctx := context.Background()

	// Update this path to your service account key
	fbConfig := &firebase.FirebaseConfig{
		ProjectID:          "aptrouter-44552",
		ServiceAccountPath: "firestore-credentials.json",
		UseCLIAuth:         false,
	}

	service, err := firebase.NewService(fbConfig)
	if err != nil {
		log.Fatalf("Failed to initialize Firebase: %v", err)
	}
	defer service.Close()

	// 1. users
	user := &firebase.User{
		ID:            "test-user-1",
		Email:         "testuser@example.com",
		Balance:       100.0,
		TierID:        "tier-1",
		CreatedAt:     time.Now(),
		UpdatedAt:     time.Now(),
		IsActive:      true,
		CustomPricing: false,
	}
	_, err = service.DB().Collection("users").Doc(user.ID).Set(ctx, user)
	if err != nil {
		log.Fatalf("Failed to write user: %v", err)
	}

	// 2. api_keys
	apiKey := &firebase.APIKey{
		ID:        "8ae0e0d38a722d95f0b2580207dff8c69225e575d1cdd8da6d98c46a0c0c7336",
		UserID:    user.ID,
		KeyHash:   "8ae0e0d38a722d95f0b2580207dff8c69225e575d1cdd8da6d98c46a0c0c7336",
		Name:      "Test Key",
		Status:    "active",
		CreatedAt: time.Now(),
		LastUsed:  time.Now(),
	}
	_, err = service.DB().Collection("api_keys").Doc(apiKey.ID).Set(ctx, apiKey)
	if err != nil {
		log.Fatalf("Failed to write api_key: %v", err)
	}

	// 3. request_logs
	requestLog := &firebase.RequestLog{
		ID:                 "test-request-1",
		UserID:             user.ID,
		APIKeyID:           "8ae0e0d38a722d95f0b2580207dff8c69225e575d1cdd8da6d98c46a0c0c7336",
		RequestID:          "test-request-1",
		ModelID:            "gpt-4o",
		Provider:           "openai",
		InputTokens:        100,
		OutputTokens:       50,
		TotalTokens:        150,
		BaseCost:           0.01,
		MarkupAmount:       0.001,
		TotalCost:          0.011,
		TierID:             "tier-1",
		MarkupPercent:      10.0,
		WasOptimized:       true,
		OptimizationStatus: "success",
		TokensSaved:        10,
		SavingsAmount:      0.0005,
		Streaming:          false,
		RequestTimestamp:   time.Now().Add(-1 * time.Minute),
		ResponseTimestamp:  time.Now(),
		DurationMs:         1000,
		Status:             "success",
		IPAddress:          "127.0.0.1",
		UserAgent:          "curl/7.68.0",
		Metadata:           map[string]interface{}{"test": true},
	}
	_, err = service.DB().Collection("request_logs").Doc(requestLog.ID).Set(ctx, requestLog)
	if err != nil {
		log.Fatalf("Failed to write request_log: %v", err)
	}

	// 4. model_configurations - All models from hardcoded config
	// Define a struct matching the Firestore tags
	type ModelConfig struct {
		ID                    string    `firestore:"id"`
		ModelID               string    `firestore:"model_id"`
		Provider              string    `firestore:"provider"`
		InputPricePerMillion  float64   `firestore:"input_price_per_million"`
		OutputPricePerMillion float64   `firestore:"output_price_per_million"`
		ContextWindowSize     int       `firestore:"context_window_size"`
		IsActive              bool      `firestore:"is_active"`
		CreatedAt             time.Time `firestore:"created_at"`
	}

	modelConfigs := []ModelConfig{
		// OpenAI Text Models
		{
			ID:                    "101",
			ModelID:               "gpt-4.1-2025-04-14",
			Provider:              "openai",
			InputPricePerMillion:  2.00,
			OutputPricePerMillion: 8.00,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "102",
			ModelID:               "gpt-4.1-mini-2025-04-14",
			Provider:              "openai",
			InputPricePerMillion:  0.40,
			OutputPricePerMillion: 1.60,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "103",
			ModelID:               "gpt-4.1-nano-2025-04-14",
			Provider:              "openai",
			InputPricePerMillion:  0.10,
			OutputPricePerMillion: 0.40,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "104",
			ModelID:               "gpt-4.5-preview-2025-02-27",
			Provider:              "openai",
			InputPricePerMillion:  75.00,
			OutputPricePerMillion: 150.00,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "105",
			ModelID:               "gpt-4o-2024-08-06",
			Provider:              "openai",
			InputPricePerMillion:  2.50,
			OutputPricePerMillion: 10.00,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "106",
			ModelID:               "gpt-4o-2024-11-20",
			Provider:              "openai",
			InputPricePerMillion:  2.50,
			OutputPricePerMillion: 10.00,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "107",
			ModelID:               "gpt-4o-2024-05-13",
			Provider:              "openai",
			InputPricePerMillion:  5.00,
			OutputPricePerMillion: 15.00,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "108",
			ModelID:               "gpt-4o-mini-2024-07-18",
			Provider:              "openai",
			InputPricePerMillion:  0.15,
			OutputPricePerMillion: 0.60,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "109",
			ModelID:               "o1-2024-12-17",
			Provider:              "openai",
			InputPricePerMillion:  15.00,
			OutputPricePerMillion: 60.00,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "110",
			ModelID:               "o3-2025-04-16",
			Provider:              "openai",
			InputPricePerMillion:  2.00,
			OutputPricePerMillion: 8.00,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "111",
			ModelID:               "o3-mini-2025-01-31",
			Provider:              "openai",
			InputPricePerMillion:  1.10,
			OutputPricePerMillion: 4.40,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "112",
			ModelID:               "o1-mini-2024-09-12",
			Provider:              "openai",
			InputPricePerMillion:  1.10,
			OutputPricePerMillion: 4.40,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "113",
			ModelID:               "codex-mini-latest",
			Provider:              "openai",
			InputPricePerMillion:  1.50,
			OutputPricePerMillion: 6.00,
			ContextWindowSize:     128000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		// Google Gemini Models
		{
			ID:                    "6",
			ModelID:               "gemini-2.5-pro",
			Provider:              "google",
			InputPricePerMillion:  1.25,
			OutputPricePerMillion: 10.00,
			ContextWindowSize:     1000000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "7",
			ModelID:               "gemini-2.5-flash",
			Provider:              "google",
			InputPricePerMillion:  0.30,
			OutputPricePerMillion: 2.50,
			ContextWindowSize:     1000000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "8",
			ModelID:               "gemini-2.5-flash-lite-preview-06-17",
			Provider:              "google",
			InputPricePerMillion:  0.10,
			OutputPricePerMillion: 0.40,
			ContextWindowSize:     1000000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "9",
			ModelID:               "gemini-2.0-flash",
			Provider:              "google",
			InputPricePerMillion:  0.075,
			OutputPricePerMillion: 0.30,
			ContextWindowSize:     1048576,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "10",
			ModelID:               "gemini-2.0-flash-lite",
			Provider:              "google",
			InputPricePerMillion:  0.05,
			OutputPricePerMillion: 0.20,
			ContextWindowSize:     1048576,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "11",
			ModelID:               "gemini-1.5-flash",
			Provider:              "google",
			InputPricePerMillion:  0.075,
			OutputPricePerMillion: 0.30,
			ContextWindowSize:     1048576,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "12",
			ModelID:               "gemini-1.5-flash-8b",
			Provider:              "google",
			InputPricePerMillion:  0.075,
			OutputPricePerMillion: 0.30,
			ContextWindowSize:     1048576,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "13",
			ModelID:               "gemini-1.5-flash-1b",
			Provider:              "google",
			InputPricePerMillion:  0.075,
			OutputPricePerMillion: 0.30,
			ContextWindowSize:     1048576,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "14",
			ModelID:               "gemini-1.5-pro",
			Provider:              "google",
			InputPricePerMillion:  3.50,
			OutputPricePerMillion: 10.50,
			ContextWindowSize:     1048576,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "15",
			ModelID:               "gemini-1.5-pro-1m",
			Provider:              "google",
			InputPricePerMillion:  3.50,
			OutputPricePerMillion: 10.50,
			ContextWindowSize:     1048576,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "16",
			ModelID:               "gemini-1.5-pro-latest",
			Provider:              "google",
			InputPricePerMillion:  3.50,
			OutputPricePerMillion: 10.50,
			ContextWindowSize:     1048576,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "17",
			ModelID:               "gemini-1.5-flash-latest",
			Provider:              "google",
			InputPricePerMillion:  0.075,
			OutputPricePerMillion: 0.30,
			ContextWindowSize:     1048576,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "18",
			ModelID:               "gemini-1.0-pro",
			Provider:              "google",
			InputPricePerMillion:  1.50,
			OutputPricePerMillion: 4.50,
			ContextWindowSize:     30720,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "19",
			ModelID:               "gemini-1.0-pro-001",
			Provider:              "google",
			InputPricePerMillion:  1.50,
			OutputPricePerMillion: 4.50,
			ContextWindowSize:     30720,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "20",
			ModelID:               "gemini-1.0-pro-latest",
			Provider:              "google",
			InputPricePerMillion:  1.50,
			OutputPricePerMillion: 4.50,
			ContextWindowSize:     30720,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		// Anthropic Claude Models
		{
			ID:                    "21",
			ModelID:               "claude-3-5-sonnet-20241022",
			Provider:              "anthropic",
			InputPricePerMillion:  3.00,
			OutputPricePerMillion: 15.00,
			ContextWindowSize:     200000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "22",
			ModelID:               "claude-3-5-haiku-20241022",
			Provider:              "anthropic",
			InputPricePerMillion:  0.80,
			OutputPricePerMillion: 4.00,
			ContextWindowSize:     200000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "23",
			ModelID:               "claude-3-5-opus-20241022",
			Provider:              "anthropic",
			InputPricePerMillion:  15.00,
			OutputPricePerMillion: 75.00,
			ContextWindowSize:     200000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "24",
			ModelID:               "claude-3-opus-20240229",
			Provider:              "anthropic",
			InputPricePerMillion:  15.00,
			OutputPricePerMillion: 75.00,
			ContextWindowSize:     200000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "25",
			ModelID:               "claude-3-5-sonnet-latest",
			Provider:              "anthropic",
			InputPricePerMillion:  3.00,
			OutputPricePerMillion: 15.00,
			ContextWindowSize:     200000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
		{
			ID:                    "26",
			ModelID:               "claude-3-5-haiku-latest",
			Provider:              "anthropic",
			InputPricePerMillion:  0.80,
			OutputPricePerMillion: 4.00,
			ContextWindowSize:     200000,
			IsActive:              true,
			CreatedAt:             time.Now(),
		},
	}

	// Write all model configurations
	for _, config := range modelConfigs {
		_, err = service.DB().Collection("model_configurations").Doc(config.ModelID).Set(ctx, config)
		if err != nil {
			log.Fatalf("Failed to write model_configuration for %s: %v", config.ModelID, err)
		}
		log.Printf("Written model configuration: %s", config.ModelID)
	}

	// 5. pricing_tiers
	pricingTier := &firebase.PricingTier{
		ID:                  "tier-1",
		Name:                "Free Tier",
		MinMonthlySpend:     0,
		InputMarkupPercent:  10.0,
		OutputMarkupPercent: 10.0,
		IsActive:            true,
		IsCustom:            false,
		CustomModelPricing:  map[string]firebase.ModelPricing{},
	}
	_, err = service.DB().Collection("pricing_tiers").Doc(pricingTier.ID).Set(ctx, pricingTier)
	if err != nil {
		log.Fatalf("Failed to write pricing_tier: %v", err)
	}

	log.Println("✅ All mock data written successfully!")
	log.Printf("📊 Summary:")
	log.Printf("   - Users: 1")
	log.Printf("   - API Keys: 1")
	log.Printf("   - Request Logs: 1")
	log.Printf("   - Model Configurations: %d", len(modelConfigs))
	log.Printf("   - Pricing Tiers: 1")
}
