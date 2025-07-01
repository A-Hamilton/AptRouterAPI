package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
	"github.com/subosito/gotenv"
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
	// Load .env file if present (ignore errors if file doesn't exist)
	_ = gotenv.Load()

	// Set default values first
	setDefaults()

	// Configure Viper to read environment variables
	viper.AutomaticEnv()

	// Map environment variables to config structure
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))

	// Bind environment variables to config fields
	bindEnvVars()

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

// bindEnvVars binds environment variables to config fields
func bindEnvVars() {
	// Server config
	viper.BindEnv("server.port", "PORT")
	viper.BindEnv("server.env", "ENV")

	// Supabase config
	viper.BindEnv("supabase.url", "SUPABASE_URL")
	viper.BindEnv("supabase.service_role_key", "SUPABASE_SERVICE_ROLE_KEY")
	viper.BindEnv("supabase.anon_key", "SUPABASE_ANON_KEY")

	// Cache config
	viper.BindEnv("cache.default_expiration", "CACHE_DEFAULT_EXPIRATION")
	viper.BindEnv("cache.cleanup_interval", "CACHE_CLEANUP_INTERVAL")

	// LLM config
	viper.BindEnv("llm.google_api_key", "GOOGLE_API_KEY")
	viper.BindEnv("llm.openai_api_key", "OPENAI_API_KEY")
	viper.BindEnv("llm.anthropic_api_key", "ANTHROPIC_API_KEY")

	// Security config
	viper.BindEnv("security.jwt_secret", "JWT_SECRET")
	viper.BindEnv("security.api_key_salt", "API_KEY_SALT")

	// Logging config
	viper.BindEnv("logging.level", "LOG_LEVEL")
	viper.BindEnv("logging.format", "LOG_FORMAT")

	// Rate limit config
	viper.BindEnv("rate_limit.requests_per_minute", "RATE_LIMIT_REQUESTS_PER_MINUTE")
	viper.BindEnv("rate_limit.burst", "RATE_LIMIT_BURST")

	// Cost config
	viper.BindEnv("cost.max_cost_per_request_usd", "MAX_COST_PER_REQUEST_USD")
	viper.BindEnv("cost.default_user_balance_usd", "DEFAULT_USER_BALANCE_USD")

	// Optimization config
	viper.BindEnv("optimization.enabled", "OPTIMIZATION_ENABLED")
	viper.BindEnv("optimization.fallback_on_optimization_failure", "FALLBACK_ON_OPTIMIZATION_FAILURE")
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
}

// validateConfig validates that all required configuration fields are present
func validateConfig(config *Config) error {
	requiredFields := []struct {
		name  string
		value string
	}{
		{"SUPABASE_URL", config.Supabase.URL},
		{"SUPABASE_SERVICE_ROLE_KEY", config.Supabase.ServiceRoleKey},
		{"SUPABASE_ANON_KEY", config.Supabase.AnonKey},
		{"JWT_SECRET", config.Security.JWTSecret},
		{"API_KEY_SALT", config.Security.APIKeySalt},
		{"GOOGLE_API_KEY", config.LLM.GoogleAPIKey},
		{"OPENAI_API_KEY", config.LLM.OpenAIAPIKey},
		{"ANTHROPIC_API_KEY", config.LLM.AnthropicAPIKey},
	}

	for _, field := range requiredFields {
		if field.value == "" {
			return fmt.Errorf("%s is required", field.name)
		}
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
