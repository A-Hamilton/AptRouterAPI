package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the AptRouter API
type Config struct {
	Server       ServerConfig       `mapstructure:"server"`
	Supabase     SupabaseConfig     `mapstructure:"supabase"`
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

// SupabaseConfig holds Supabase-related configuration
type SupabaseConfig struct {
	URL            string `mapstructure:"url"`
	ServiceRoleKey string `mapstructure:"service_role_key"`
	AnonKey        string `mapstructure:"anon_key"`
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

// LoggingConfig holds logging-related configuration
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	RequestsPerMinute int `mapstructure:"requests_per_minute"`
	Burst             int `mapstructure:"burst"`
}

// CostConfig holds cost management configuration
type CostConfig struct {
	MaxCostPerRequestUSD  float64 `mapstructure:"max_cost_per_request_usd"`
	DefaultUserBalanceUSD float64 `mapstructure:"default_user_balance_usd"`
}

// OptimizationConfig holds optimization-related configuration
type OptimizationConfig struct {
	Enabled                       bool `mapstructure:"enabled"`
	FallbackOnOptimizationFailure bool `mapstructure:"fallback_on_optimization_failure"`
}

// LoadConfig loads configuration from environment variables and .env file
func LoadConfig() (*Config, error) {
	viper.SetConfigName(".env")
	viper.SetConfigType("env")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("../config")
	viper.AddConfigPath("../../config")

	// Set default values
	setDefaults()

	// Read environment variables
	viper.AutomaticEnv()

	// Read .env file if it exists
	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	var config Config
	if err := viper.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	// Validate required fields
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("config validation failed: %w", err)
	}

	return &config, nil
}

// setDefaults sets default values for configuration
func setDefaults() {
	// Server defaults
	viper.SetDefault("PORT", "8080")
	viper.SetDefault("ENV", "development")

	// Cache defaults
	viper.SetDefault("CACHE_DEFAULT_EXPIRATION", "5m")
	viper.SetDefault("CACHE_CLEANUP_INTERVAL", "10m")

	// Logging defaults
	viper.SetDefault("LOG_LEVEL", "info")
	viper.SetDefault("LOG_FORMAT", "json")

	// Rate limiting defaults
	viper.SetDefault("RATE_LIMIT_REQUESTS_PER_MINUTE", "100")
	viper.SetDefault("RATE_LIMIT_BURST", "20")

	// Cost management defaults
	viper.SetDefault("MAX_COST_PER_REQUEST_USD", "10.00")
	viper.SetDefault("DEFAULT_USER_BALANCE_USD", "100.00")

	// Optimization defaults
	viper.SetDefault("OPTIMIZATION_ENABLED", "true")
	viper.SetDefault("FALLBACK_ON_OPTIMIZATION_FAILURE", "true")

	// Map environment variables to config structure
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
}

// validateConfig validates that all required configuration fields are present
func validateConfig(config *Config) error {
	if config.Supabase.URL == "" {
		return fmt.Errorf("SUPABASE_URL is required")
	}
	if config.Supabase.ServiceRoleKey == "" {
		return fmt.Errorf("SUPABASE_SERVICE_ROLE_KEY is required")
	}
	if config.Supabase.AnonKey == "" {
		return fmt.Errorf("SUPABASE_ANON_KEY is required")
	}
	if config.Security.JWTSecret == "" {
		return fmt.Errorf("JWT_SECRET is required")
	}
	if config.Security.APIKeySalt == "" {
		return fmt.Errorf("API_KEY_SALT is required")
	}
	if config.LLM.GoogleAPIKey == "" {
		return fmt.Errorf("GOOGLE_API_KEY is required")
	}
	if config.LLM.OpenAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is required")
	}
	if config.LLM.AnthropicAPIKey == "" {
		return fmt.Errorf("ANTHROPIC_API_KEY is required")
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
