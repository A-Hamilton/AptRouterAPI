package firebase

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"
	"google.golang.org/api/option"
)

// Service handles Firebase operations
type Service struct {
	app        *firebase.App
	authClient *auth.Client
	dbClient   *firestore.Client
	config     *FirebaseConfig
}

// FirebaseConfig holds Firebase configuration
type FirebaseConfig struct {
	ProjectID          string
	ServiceAccountPath string
	UseCLIAuth         bool
}

// User represents a user in the system
type User struct {
	ID            string    `firestore:"id"`
	Email         string    `firestore:"email"`
	Balance       float64   `firestore:"balance"`
	TierID        string    `firestore:"tier_id"`
	CreatedAt     time.Time `firestore:"created_at"`
	UpdatedAt     time.Time `firestore:"updated_at"`
	IsActive      bool      `firestore:"is_active"`
	CustomPricing bool      `firestore:"custom_pricing"`
}

// PricingTier represents a pricing tier
type PricingTier struct {
	ID                  string                  `firestore:"id"`
	Name                string                  `firestore:"name"`
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

// APIKey represents an API key
type APIKey struct {
	ID        string    `firestore:"id"`
	UserID    string    `firestore:"user_id"`
	KeyHash   string    `firestore:"key_hash"`
	Name      string    `firestore:"name"`
	Status    string    `firestore:"status"`
	CreatedAt time.Time `firestore:"created_at"`
	LastUsed  time.Time `firestore:"last_used,omitempty"`
}

// RequestLog represents a logged request for audit purposes
type RequestLog struct {
	ID                 string                 `firestore:"id"`
	UserID             string                 `firestore:"user_id"`
	APIKeyID           string                 `firestore:"api_key_id"`
	RequestID          string                 `firestore:"request_id"`
	ModelID            string                 `firestore:"model_id"`
	Provider           string                 `firestore:"provider"`
	InputTokens        int                    `firestore:"input_tokens"`
	OutputTokens       int                    `firestore:"output_tokens"`
	TotalTokens        int                    `firestore:"total_tokens"`
	BaseCost           float64                `firestore:"base_cost"`
	MarkupAmount       float64                `firestore:"markup_amount"`
	TotalCost          float64                `firestore:"total_cost"`
	TierID             string                 `firestore:"tier_id"`
	MarkupPercent      float64                `firestore:"markup_percent"`
	WasOptimized       bool                   `firestore:"was_optimized"`
	OptimizationStatus string                 `firestore:"optimization_status"`
	TokensSaved        int                    `firestore:"tokens_saved"`
	SavingsAmount      float64                `firestore:"savings_amount"`
	Streaming          bool                   `firestore:"streaming"`
	RequestTimestamp   time.Time              `firestore:"request_timestamp"`
	ResponseTimestamp  time.Time              `firestore:"response_timestamp"`
	DurationMs         int64                  `firestore:"duration_ms"`
	Status             string                 `firestore:"status"`
	Error              string                 `firestore:"error,omitempty"`
	Metadata           map[string]interface{} `firestore:"metadata,omitempty"`
	IPAddress          string                 `firestore:"ip_address"`
	UserAgent          string                 `firestore:"user_agent"`
}

// NewService creates a new Firebase service
func NewService(config *FirebaseConfig) (*Service, error) {
	var opts []option.ClientOption

	if config.UseCLIAuth {
		// Use Firebase CLI authentication (recommended for development)
		slog.Info("Using Firebase CLI authentication")
		// No additional options needed - Firebase CLI handles auth automatically
	} else if config.ServiceAccountPath != "" {
		// Use service account key file (fallback for production)
		slog.Info("Using service account key authentication", "path", config.ServiceAccountPath)
		opts = append(opts, option.WithCredentialsFile(config.ServiceAccountPath))
	} else {
		// Use Application Default Credentials (ADC)
		slog.Info("Using Application Default Credentials")
		// No additional options needed - ADC will be used automatically
	}

	// Initialize Firebase app
	app, err := firebase.NewApp(context.Background(), &firebase.Config{
		ProjectID: config.ProjectID,
	}, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firebase app: %w", err)
	}

	// Initialize Auth client
	authClient, err := app.Auth(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Auth client: %w", err)
	}

	// Initialize Firestore client
	dbClient, err := app.Firestore(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to initialize Firestore client: %w", err)
	}

	return &Service{
		app:        app,
		authClient: authClient,
		dbClient:   dbClient,
		config:     config,
	}, nil
}

// Close closes the Firebase connections
func (s *Service) Close() error {
	if s.dbClient != nil {
		return s.dbClient.Close()
	}
	return nil
}

// DB returns the Firestore client
func (s *Service) DB() *firestore.Client {
	return s.dbClient
}

// GetUserByAPIKey gets a user by API key hash
func (s *Service) GetUserByAPIKey(ctx context.Context, keyHash string) (*User, error) {
	// Query API keys collection
	iter := s.dbClient.Collection("api_keys").Where("key_hash", "==", keyHash).Where("status", "==", "active").Limit(1).Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err != nil {
		return nil, fmt.Errorf("API key not found: %w", err)
	}

	var apiKey APIKey
	if err := doc.DataTo(&apiKey); err != nil {
		return nil, fmt.Errorf("failed to parse API key: %w", err)
	}

	// Get user by ID
	return s.GetUserByID(ctx, apiKey.UserID)
}

// GetUserByID gets a user by ID
func (s *Service) GetUserByID(ctx context.Context, userID string) (*User, error) {
	doc, err := s.dbClient.Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	var user User
	if err := doc.DataTo(&user); err != nil {
		return nil, fmt.Errorf("failed to parse user: %w", err)
	}

	return &user, nil
}

// GetPricingTier gets a pricing tier by ID
func (s *Service) GetPricingTier(ctx context.Context, tierID string) (*PricingTier, error) {
	doc, err := s.dbClient.Collection("pricing_tiers").Doc(tierID).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("pricing tier not found: %w", err)
	}

	var tier PricingTier
	if err := doc.DataTo(&tier); err != nil {
		return nil, fmt.Errorf("failed to parse pricing tier: %w", err)
	}

	return &tier, nil
}

// GetDefaultPricingTier gets the default pricing tier
func (s *Service) GetDefaultPricingTier(ctx context.Context) (*PricingTier, error) {
	iter := s.dbClient.Collection("pricing_tiers").Where("is_active", "==", true).Where("is_custom", "==", false).OrderBy("min_monthly_spend", firestore.Asc).Limit(1).Documents(ctx)
	defer iter.Stop()

	doc, err := iter.Next()
	if err != nil {
		return nil, fmt.Errorf("default pricing tier not found: %w", err)
	}

	var tier PricingTier
	if err := doc.DataTo(&tier); err != nil {
		return nil, fmt.Errorf("failed to parse pricing tier: %w", err)
	}

	return &tier, nil
}

// CalculateCost calculates the cost with markup based on tier
func (s *Service) CalculateCost(ctx context.Context, user *User, modelID, provider string, inputTokens, outputTokens int, baseInputPrice, baseOutputPrice float64) (float64, float64, error) {
	// Get user's pricing tier
	tier, err := s.GetPricingTier(ctx, user.TierID)
	if err != nil {
		// Fallback to default tier
		tier, err = s.GetDefaultPricingTier(ctx)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get pricing tier: %w", err)
		}
	}

	// Check for custom model pricing
	inputPrice := baseInputPrice
	outputPrice := baseOutputPrice

	if user.CustomPricing && tier.IsCustom && tier.CustomModelPricing != nil {
		if modelPricing, exists := tier.CustomModelPricing[modelID]; exists {
			inputPrice = modelPricing.InputPricePerMillion
			outputPrice = modelPricing.OutputPricePerMillion
		}
	}

	// Calculate base cost
	inputCost := (float64(inputTokens) / 1000000) * inputPrice
	outputCost := (float64(outputTokens) / 1000000) * outputPrice
	baseCost := inputCost + outputCost

	// Calculate markup
	inputMarkup := inputCost * (tier.InputMarkupPercent / 100)
	outputMarkup := outputCost * (tier.OutputMarkupPercent / 100)
	totalMarkup := inputMarkup + outputMarkup

	totalCost := baseCost + totalMarkup

	return totalCost, totalMarkup, nil
}

// LogRequest logs a request for audit purposes
func (s *Service) LogRequest(ctx context.Context, log *RequestLog) error {
	// Set timestamps if not provided
	if log.RequestTimestamp.IsZero() {
		log.RequestTimestamp = time.Now()
	}
	if log.ResponseTimestamp.IsZero() {
		log.ResponseTimestamp = time.Now()
	}

	// Calculate duration
	log.DurationMs = log.ResponseTimestamp.Sub(log.RequestTimestamp).Milliseconds()

	// Add to Firestore
	_, err := s.dbClient.Collection("request_logs").Doc(log.ID).Set(ctx, log)
	if err != nil {
		return fmt.Errorf("failed to log request: %w", err)
	}

	slog.Info("Request logged",
		"request_id", log.RequestID,
		"user_id", log.UserID,
		"model", log.ModelID,
		"total_cost", log.TotalCost,
		"duration_ms", log.DurationMs,
	)

	return nil
}

// UpdateUserBalance updates a user's balance
func (s *Service) UpdateUserBalance(ctx context.Context, userID string, amount float64) error {
	// Use a transaction to ensure atomicity
	err := s.dbClient.RunTransaction(ctx, func(ctx context.Context, tx *firestore.Transaction) error {
		userRef := s.dbClient.Collection("users").Doc(userID)

		// Get current user
		doc, err := tx.Get(userRef)
		if err != nil {
			return fmt.Errorf("failed to get user: %w", err)
		}

		var user User
		if err := doc.DataTo(&user); err != nil {
			return fmt.Errorf("failed to parse user: %w", err)
		}

		// Update balance
		user.Balance += amount
		user.UpdatedAt = time.Now()

		// Ensure balance doesn't go negative
		if user.Balance < 0 {
			return fmt.Errorf("insufficient balance: current balance %.2f, attempted charge %.2f", user.Balance-amount, -amount)
		}

		// Update user
		return tx.Set(userRef, user)
	})

	if err != nil {
		return fmt.Errorf("failed to update user balance: %w", err)
	}

	slog.Info("User balance updated",
		"user_id", userID,
		"amount", amount,
	)

	return nil
}

// GetUserBalance gets a user's current balance
func (s *Service) GetUserBalance(ctx context.Context, userID string) (float64, error) {
	user, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return 0, err
	}

	return user.Balance, nil
}

// GetUserUsage gets a user's usage statistics
func (s *Service) GetUserUsage(ctx context.Context, userID string, startDate, endDate time.Time) (map[string]interface{}, error) {
	// Query request logs for the user in the date range
	iter := s.dbClient.Collection("request_logs").
		Where("user_id", "==", userID).
		Where("request_timestamp", ">=", startDate).
		Where("request_timestamp", "<=", endDate).
		Documents(ctx)
	defer iter.Stop()

	var totalCost float64
	var totalTokens int
	var totalRequests int
	var totalTokensSaved int
	var totalSavings float64

	for {
		doc, err := iter.Next()
		if err != nil {
			break // End of iteration
		}

		var log RequestLog
		if err := doc.DataTo(&log); err != nil {
			continue // Skip malformed logs
		}

		totalCost += log.TotalCost
		totalTokens += log.TotalTokens
		totalRequests++
		totalTokensSaved += log.TokensSaved
		totalSavings += log.SavingsAmount
	}

	return map[string]interface{}{
		"total_cost":         totalCost,
		"total_tokens":       totalTokens,
		"total_requests":     totalRequests,
		"total_tokens_saved": totalTokensSaved,
		"total_savings":      totalSavings,
		"start_date":         startDate,
		"end_date":           endDate,
	}, nil
}

// CreateAPIKey creates a new API key for a user
func (s *Service) CreateAPIKey(ctx context.Context, userID, keyHash, name string) (*APIKey, error) {
	apiKey := &APIKey{
		ID:        keyHash, // Use hash as ID for simplicity
		UserID:    userID,
		KeyHash:   keyHash,
		Name:      name,
		Status:    "active",
		CreatedAt: time.Now(),
	}

	_, err := s.dbClient.Collection("api_keys").Doc(apiKey.ID).Set(ctx, apiKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create API key: %w", err)
	}

	return apiKey, nil
}

// ListAPIKeys lists all API keys for a user
func (s *Service) ListAPIKeys(ctx context.Context, userID string) ([]*APIKey, error) {
	iter := s.dbClient.Collection("api_keys").Where("user_id", "==", userID).Documents(ctx)
	defer iter.Stop()

	var apiKeys []*APIKey
	for {
		doc, err := iter.Next()
		if err != nil {
			break
		}

		var apiKey APIKey
		if err := doc.DataTo(&apiKey); err != nil {
			continue
		}

		apiKeys = append(apiKeys, &apiKey)
	}

	return apiKeys, nil
}

// RevokeAPIKey revokes an API key
func (s *Service) RevokeAPIKey(ctx context.Context, keyID, userID string) error {
	// Verify the key belongs to the user
	doc, err := s.dbClient.Collection("api_keys").Doc(keyID).Get(ctx)
	if err != nil {
		return fmt.Errorf("API key not found: %w", err)
	}

	var apiKey APIKey
	if err := doc.DataTo(&apiKey); err != nil {
		return fmt.Errorf("failed to parse API key: %w", err)
	}

	if apiKey.UserID != userID {
		return fmt.Errorf("unauthorized: API key does not belong to user")
	}

	// Update status to revoked
	apiKey.Status = "revoked"
	_, err = s.dbClient.Collection("api_keys").Doc(keyID).Set(ctx, apiKey)
	if err != nil {
		return fmt.Errorf("failed to revoke API key: %w", err)
	}

	return nil
}
