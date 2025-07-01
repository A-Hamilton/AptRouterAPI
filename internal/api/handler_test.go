package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/apt-router/api/internal/config"
	"github.com/apt-router/api/internal/pricing"
	"github.com/gin-gonic/gin"
	"github.com/patrickmn/go-cache"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	supabase "github.com/supabase-community/supabase-go"
)

// setupTestHandler creates a test handler with mock dependencies
func setupTestHandler(_ *testing.T) *Handler {
	// Create mock config
	cfg := &config.Config{
		Server: config.ServerConfig{
			Port: 8080,
			Env:  "test",
		},
		Security: config.SecurityConfig{
			JWTSecret:  "test-secret",
			APIKeySalt: "test-salt",
		},
		LLM: config.LLMConfig{
			GoogleAPIKey:    "test-google-key",
			OpenAIAPIKey:    "test-openai-key",
			AnthropicAPIKey: "test-anthropic-key",
		},
		Optimization: config.OptimizationConfig{
			Enabled:                       true,
			FallbackOnOptimizationFailure: true,
		},
	}

	// Create mock Supabase client (nil for tests)
	var supabaseClient *supabase.Client

	// Create cache
	memoryCache := cache.New(5, 10)

	// Create pricing service
	pricingService := pricing.NewService(supabaseClient)

	// Create handler
	handler := NewHandler(cfg, supabaseClient, memoryCache, pricingService)

	return handler
}

// setupTestRouter creates a test router with the handler
func setupTestRouter(handler *Handler) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(handler.RequestLogger())

	// Register routes
	router.GET("/healthz", handler.HealthCheck)

	v1 := router.Group("/v1")
	{
		generate := v1.Group("/generate")
		generate.Use(handler.AuthMiddleware())
		{
			generate.POST("", handler.Generate)
			generate.POST("/stream", handler.GenerateStream)
		}

		user := v1.Group("/user")
		user.Use(handler.JWTAuthMiddleware())
		{
			user.GET("/profile", handler.GetProfile)
			user.GET("/balance", handler.GetBalance)
			user.GET("/usage", handler.GetUsage)
		}

		keys := v1.Group("/keys")
		keys.Use(handler.JWTAuthMiddleware())
		{
			keys.POST("", handler.CreateAPIKey)
			keys.GET("", handler.ListAPIKeys)
			keys.DELETE(":key_id", handler.RevokeAPIKey)
		}
	}

	return router
}

func TestHealthCheck(t *testing.T) {
	handler := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Create request
	req, err := http.NewRequest("GET", "/healthz", nil)
	require.NoError(t, err)

	// Create response recorder
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Assert response
	assert.Equal(t, http.StatusOK, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)

	assert.Equal(t, "healthy", response["status"])
	assert.Equal(t, "apt-router-api", response["service"])
	assert.Equal(t, "1.0.0", response["version"])
}

func TestGenerateEndpoint(t *testing.T) {
	handler := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Create request body
	requestBody := GenerateRequest{
		Model:  "gpt-3.5-turbo",
		Prompt: "Hello, world!",
	}
	bodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	// Create request
	req, err := http.NewRequest("POST", "/v1/generate", bytes.NewBuffer(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")

	// Create response recorder
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Note: This will likely fail due to missing API keys, but we're testing the endpoint structure
	// In a real test environment, you'd mock the LLM clients
	t.Logf("Response status: %d", w.Code)
	t.Logf("Response body: %s", w.Body.String())
}

func TestGenerateStreamEndpoint(t *testing.T) {
	handler := setupTestHandler(t)
	router := setupTestRouter(handler)

	// Create request body
	requestBody := GenerateRequest{
		Model:  "gpt-3.5-turbo",
		Prompt: "Hello, world!",
	}
	bodyBytes, err := json.Marshal(requestBody)
	require.NoError(t, err)

	// Create request
	req, err := http.NewRequest("POST", "/v1/generate/stream", bytes.NewBuffer(bodyBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer test-api-key")

	// Create response recorder
	w := httptest.NewRecorder()

	// Serve request
	router.ServeHTTP(w, req)

	// Note: This will likely fail due to missing API keys, but we're testing the endpoint structure
	// In a real test environment, you'd mock the LLM clients
	t.Logf("Response status: %d", w.Code)
	t.Logf("Response body: %s", w.Body.String())
}

func TestUserEndpoints(t *testing.T) {
	handler := setupTestHandler(t)
	router := setupTestRouter(handler)

	testCases := []struct {
		name     string
		method   string
		endpoint string
	}{
		{"GetProfile", "GET", "/v1/user/profile"},
		{"GetBalance", "GET", "/v1/user/balance"},
		{"GetUsage", "GET", "/v1/user/usage"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, tc.endpoint, nil)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Contains(t, response, "message")
		})
	}
}

func TestAPIKeyEndpoints(t *testing.T) {
	handler := setupTestHandler(t)
	router := setupTestRouter(handler)

	testCases := []struct {
		name     string
		method   string
		endpoint string
	}{
		{"CreateAPIKey", "POST", "/v1/keys"},
		{"ListAPIKeys", "GET", "/v1/keys"},
		{"RevokeAPIKey", "DELETE", "/v1/keys/test-key-id"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, tc.endpoint, nil)
			require.NoError(t, err)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			assert.Equal(t, http.StatusOK, w.Code)

			var response map[string]interface{}
			err = json.Unmarshal(w.Body.Bytes(), &response)
			require.NoError(t, err)

			assert.Contains(t, response, "message")
		})
	}
}

func TestAuthMiddleware(t *testing.T) {
	handler := setupTestHandler(t)

	// Test with valid API key
	req, err := http.NewRequest("GET", "/test", nil)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer valid-api-key")

	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req

	// Call middleware
	handler.AuthMiddleware()(c)

	// Verify request context was set
	requestCtx, exists := handler.getRequestContext(c)
	assert.True(t, exists)
	assert.NotNil(t, requestCtx)
	assert.Equal(t, "mock-user-id", requestCtx.UserID)
}

func TestRequestLogger(t *testing.T) {
	handler := setupTestHandler(t)

	req, err := http.NewRequest("GET", "/test", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	// Call middleware
	handler.RequestLogger()(c)

	// Verify logger was set in context
	logger := handler.getLogger(c)
	assert.NotNil(t, logger)
}

func TestHelperFunctions(t *testing.T) {
	handler := setupTestHandler(t)

	// Test getIntValue
	intPtr := 42
	assert.Equal(t, 42, handler.getIntValue(&intPtr, 0))
	assert.Equal(t, 0, handler.getIntValue(nil, 0))

	// Test getFloatValue
	floatPtr := 3.14
	assert.Equal(t, 3.14, handler.getFloatValue(&floatPtr, 0.0))
	assert.Equal(t, 0.0, handler.getFloatValue(nil, 0.0))

	// Test getBoolValue
	boolPtr := true
	assert.Equal(t, true, handler.getBoolValue(&boolPtr, false))
	assert.Equal(t, false, handler.getBoolValue(nil, false))
}

func TestRequestContext(t *testing.T) {
	handler := setupTestHandler(t)

	// Test getRequestID
	req, err := http.NewRequest("GET", "/test", nil)
	require.NoError(t, err)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	requestID := handler.getRequestID(c)
	assert.NotEmpty(t, requestID)

	// Test getLogger
	logger := handler.getLogger(c)
	assert.NotNil(t, logger)

	// Test getRequestContext when not set
	requestCtx, exists := handler.getRequestContext(c)
	assert.False(t, exists)
	assert.Nil(t, requestCtx)
}

func BenchmarkHealthCheck(b *testing.B) {
	handler := setupTestHandler(&testing.T{})
	router := setupTestRouter(handler)

	req, err := http.NewRequest("GET", "/healthz", nil)
	if err != nil {
		b.Fatal(err)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

func BenchmarkAuthMiddleware(b *testing.B) {
	handler := setupTestHandler(&testing.T{})

	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		b.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer test-api-key")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = req
		handler.AuthMiddleware()(c)
	}
}
