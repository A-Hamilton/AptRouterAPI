package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/apt-router/api/internal/api"
	"github.com/apt-router/api/internal/config"
	"github.com/apt-router/api/internal/pricing"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	supabase "github.com/supabase-community/supabase-go"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Initialize logger
	logger := initLogger(cfg)
	slog.SetDefault(logger)
	slog.Info("Starting AptRouter API", "version", "1.0.0", "env", cfg.Server.Env)

	// Create root context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Supabase client
	supabaseClient, err := supabase.NewClient(cfg.Supabase.URL, cfg.Supabase.ServiceRoleKey, nil)
	if err != nil {
		slog.Error("Failed to initialize Supabase client", "error", err)
		os.Exit(1)
	}

	// Initialize memory cache with optimized settings
	memoryCache := cache.New(cfg.Cache.DefaultExpiration, cfg.Cache.CleanupInterval)

	// Initialize pricing service and pre-cache data with timeout
	pricingService := pricing.NewService(supabaseClient)
	pricingCtx, pricingCancel := context.WithTimeout(ctx, 60*time.Second)
	defer pricingCancel()

	if err := pricingService.PreCacheData(pricingCtx); err != nil {
		slog.Error("Failed to pre-cache pricing data", "error", err)
		os.Exit(1)
	}

	// Set Gin mode based on environment
	if cfg.IsProduction() {
		gin.SetMode(gin.ReleaseMode)
	}

	// Initialize router with optimized settings
	router := gin.New()
	router.Use(gin.Recovery())

	// Initialize API handlers
	apiHandler := api.NewHandler(cfg, supabaseClient, memoryCache, pricingService)

	// Add request logging middleware
	router.Use(apiHandler.RequestLogger())

	// Register routes
	registerRoutes(router, apiHandler)

	// Create HTTP server with optimized settings
	server := &http.Server{
		Addr:         ":" + cfg.GetPort(),
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
		// Performance optimizations
		MaxHeaderBytes: 1 << 20, // 1MB
	}

	// Start server in a goroutine
	go func() {
		slog.Info("Starting HTTP server", "port", cfg.GetPort())
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Failed to start server", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("Shutting down server...")

	// Create a deadline for server shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	// Attempt graceful shutdown
	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("Server forced to shutdown", "error", err)
	}

	slog.Info("Server exited")
}

// initLogger initializes the structured logger based on configuration
func initLogger(cfg *config.Config) *slog.Logger {
	var level slog.Level
	switch cfg.Logging.Level {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	var handler slog.Handler
	if cfg.Logging.Format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	} else {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: level,
		})
	}

	return slog.New(handler)
}

// registerRoutes registers all API routes
func registerRoutes(router *gin.Engine, handler *api.Handler) {
	// Health check endpoint
	router.GET("/healthz", handler.HealthCheck)

	// API v1 routes
	v1 := router.Group("/v1")
	{
		// Public endpoints (require API key authentication)
		generate := v1.Group("/generate")
		generate.Use(handler.AuthMiddleware())
		{
			generate.POST("", handler.Generate)
			generate.POST("/stream", handler.GenerateStream)
		}

		// User management endpoints (require JWT authentication)
		user := v1.Group("/user")
		user.Use(handler.JWTAuthMiddleware())
		{
			user.GET("/profile", handler.GetProfile)
			user.GET("/balance", handler.GetBalance)
			user.GET("/usage", handler.GetUsage)
		}

		// API key management endpoints (require JWT authentication)
		keys := v1.Group("/keys")
		keys.Use(handler.JWTAuthMiddleware())
		{
			keys.POST("", handler.CreateAPIKey)
			keys.GET("", handler.ListAPIKeys)
			keys.DELETE(":key_id", handler.RevokeAPIKey)
		}
	}
}
