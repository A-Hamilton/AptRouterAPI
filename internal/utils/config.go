package utils

import (
	"fmt"
	"strconv"
	"time"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
)

// Config holds all configuration for the application
type Config struct {
	Server       ServerConfig       `mapstructure:"server"`
	Firebase     FirebaseConfig     `mapstructure:"firebase"`
	Cache        CacheConfig        `mapstructure:"cache"`
	LLM          LLMConfig          `mapstructure:"llm"`
	Security     SecurityConfig     `mapstructure:"security"`
	Logging      LoggingConfig      `mapstructure:"logging"`
	RateLimit    RateLimitConfig    `mapstructure:"rate_limit"`
	Cost         CostConfig         `mapstructure:"cost"`
	Optimization OptimizationConfig `mapstructure:"optimization"`
}

// ServerConfig holds server-related configuration
type ServerConfig struct {
	Port int    `mapstructure:"port"`
	Env  string `mapstructure:"env"`
}

// FirebaseConfig holds Firebase configuration
type FirebaseConfig struct {
	ProjectID          string `mapstructure:"project_id"`
	ServiceAccountPath string `mapstructure:"service_account_path"`
	WebAPIKey          string `mapstructure:"web_api_key"`
	AuthDomain         string `mapstructure:"auth_domain"`
	StorageBucket      string `mapstructure:"storage_bucket"`
	MessagingSenderID  string `mapstructure:"messaging_sender_id"`
	AppID              string `mapstructure:"app_id"`
	MeasurementID      string `mapstructure:"measurement_id"`
	UseCLIAuth         bool   `mapstructure:"use_cli_auth"`
}

// CacheConfig holds cache-related configuration
type CacheConfig struct {
	DefaultExpiration time.Duration `mapstructure:"default_expiration"`
	CleanupInterval   time.Duration `mapstructure:"cleanup_interval"`
}

// LLMConfig holds LLM provider API keys
type LLMConfig struct {
	GoogleAPIKey    string `mapstructure:"google_api_key"`
	OpenAIAPIKey    string `mapstructure:"openai_api_key"`
	AnthropicAPIKey string `mapstructure:"anthropic_api_key"`
}

// SecurityConfig holds security-related configuration
type SecurityConfig struct {
	JWTSecret  string `mapstructure:"jwt_secret"`
	APIKeySalt string `mapstructure:"api_key_salt"`
}

// LoggingConfig holds logging configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int `mapstructure:"requests_per_minute"`
	Burst             int `mapstructure:"burst"`
}

// CostConfig holds cost-related configuration
type CostConfig struct {
	MaxCostPerRequestUSD  float64 `mapstructure:"max_cost_per_request_usd"`
	DefaultUserBalanceUSD float64 `mapstructure:"default_user_balance_usd"`
}

// OptimizationConfig holds optimization configuration
type OptimizationConfig struct {
	Enabled                       bool `mapstructure:"enabled"`
	FallbackOnOptimizationFailure bool `mapstructure:"fallback_on_optimization_failure"`
}

// LoadConfig loads configuration from environment variables and config files
func LoadConfig() (*Config, error) {
	// Load .env file if it exists
	if err := gotenv.Load(); err != nil {
		// .env file is optional, so we don't return an error
	}

	// Bind environment variables
	bindEnvVars()

	// Set defaults
	setDefaults()

	// Create config instance
	config := &Config{}

	// Unmarshal configuration
	if err := viper.Unmarshal(config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate configuration
	if err := validateConfig(config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return config, nil
}

// bindEnvVars binds environment variables to viper
func bindEnvVars() {
	// Server
	viper.BindEnv("server.port", "PORT")
	viper.BindEnv("server.env", "ENV")

	// Firebase
	viper.BindEnv("firebase.project_id", "FIREBASE_PROJECT_ID")
	viper.BindEnv("firebase.service_account_path", "FIREBASE_SERVICE_ACCOUNT_PATH")
	viper.BindEnv("firebase.web_api_key", "FIREBASE_WEB_API_KEY")
	viper.BindEnv("firebase.auth_domain", "FIREBASE_AUTH_DOMAIN")
	viper.BindEnv("firebase.storage_bucket", "FIREBASE_STORAGE_BUCKET")
	viper.BindEnv("firebase.messaging_sender_id", "FIREBASE_MESSAGING_SENDER_ID")
	viper.BindEnv("firebase.app_id", "FIREBASE_APP_ID")
	viper.BindEnv("firebase.measurement_id", "FIREBASE_MEASUREMENT_ID")
	viper.BindEnv("firebase.use_cli_auth", "FIREBASE_USE_CLI_AUTH")

	// LLM API Keys
	viper.BindEnv("llm.google_api_key", "GOOGLE_API_KEY")
	viper.BindEnv("llm.openai_api_key", "OPENAI_API_KEY")
	viper.BindEnv("llm.anthropic_api_key", "ANTHROPIC_API_KEY")

	// Security
	viper.BindEnv("security.jwt_secret", "JWT_SECRET")
	viper.BindEnv("security.api_key_salt", "API_KEY_SALT")

	// Logging
	viper.BindEnv("logging.level", "LOG_LEVEL")
	viper.BindEnv("logging.format", "LOG_FORMAT")

	// Rate Limiting
	viper.BindEnv("rate_limit.requests_per_minute", "RATE_LIMIT_REQUESTS_PER_MINUTE")
	viper.BindEnv("rate_limit.burst", "RATE_LIMIT_BURST")

	// Cost
	viper.BindEnv("cost.max_cost_per_request_usd", "MAX_COST_PER_REQUEST_USD")
	viper.BindEnv("cost.default_user_balance_usd", "DEFAULT_USER_BALANCE_USD")

	// Optimization
	viper.BindEnv("optimization.enabled", "OPTIMIZATION_ENABLED")
	viper.BindEnv("optimization.fallback_on_optimization_failure", "OPTIMIZATION_FALLBACK_ON_FAILURE")
}

// setDefaults sets default values for configuration
func setDefaults() {
	// Server defaults
	viper.SetDefault("server.port", 8080)
	viper.SetDefault("server.env", "development")

	// Firebase defaults (will be overridden by environment variables)
	viper.SetDefault("firebase.project_id", "aptrouter-44552")

	// Cache defaults
	viper.SetDefault("cache.default_expiration", 5*time.Minute)
	viper.SetDefault("cache.cleanup_interval", 10*time.Minute)

	// Security defaults
	viper.SetDefault("security.jwt_secret", "your-jwt-secret-change-in-production")
	viper.SetDefault("security.api_key_salt", "your-api-key-salt-change-in-production")

	// Logging defaults
	viper.SetDefault("logging.level", "info")
	viper.SetDefault("logging.format", "json")

	// Rate limiting defaults
	viper.SetDefault("rate_limit.requests_per_minute", 60)
	viper.SetDefault("rate_limit.burst", 10)

	// Cost defaults
	viper.SetDefault("cost.max_cost_per_request_usd", 10.0)
	viper.SetDefault("cost.default_user_balance_usd", 100.0)

	// Optimization defaults
	viper.SetDefault("optimization.enabled", true)
	viper.SetDefault("optimization.fallback_on_optimization_failure", true)
}

// validateConfig validates the configuration
func validateConfig(config *Config) error {
	// Validate server port
	if config.Server.Port <= 0 || config.Server.Port > 65535 {
		return fmt.Errorf("invalid server port: %d", config.Server.Port)
	}

	// Validate Firebase configuration
	if config.Firebase.ProjectID == "" {
		return fmt.Errorf("firebase project ID is required")
	}

	// Validate required API keys (at least one should be present)
	if config.LLM.GoogleAPIKey == "" && config.LLM.OpenAIAPIKey == "" && config.LLM.AnthropicAPIKey == "" {
		return fmt.Errorf("at least one LLM API key is required")
	}

	// Validate security configuration
	if config.Security.JWTSecret == "" {
		return fmt.Errorf("JWT secret is required")
	}

	if config.Security.APIKeySalt == "" {
		return fmt.Errorf("API key salt is required")
	}

	// Validate cost configuration
	if config.Cost.MaxCostPerRequestUSD <= 0 {
		return fmt.Errorf("max cost per request must be positive")
	}

	if config.Cost.DefaultUserBalanceUSD <= 0 {
		return fmt.Errorf("default user balance must be positive")
	}

	return nil
}

// GetPort returns the server port as a string
func (c *Config) GetPort() string {
	return strconv.Itoa(c.Server.Port)
}

// IsDevelopment returns true if the environment is development
func (c *Config) IsDevelopment() bool {
	return c.Server.Env == "development"
}

// IsProduction returns true if the environment is production
func (c *Config) IsProduction() bool {
	return c.Server.Env == "production"
}
