# E2E Testing for AptRouter API

This document describes how to run comprehensive end-to-end tests for the AptRouter API.

## Overview

The e2e testing suite tests all three LLM providers (OpenAI, Google Gemini, Anthropic) with cheap/fast models for both non-streaming and streaming endpoints.

## Test Models

The tests use the following cost-effective models:

- **OpenAI**: `gpt-3.5-turbo` - Fast and affordable GPT model
- **Google**: `gemini-2.0-flash` - Fast and efficient Gemini model  
- **Anthropic**: `claude-3-haiku-20240307` - Fast and cost-effective Claude model

## Prerequisites

1. **Go 1.24+** installed and in PATH
2. **API Keys** configured in environment variables:
   - `OPENAI_API_KEY`
   - `GOOGLE_API_KEY` 
   - `ANTHROPIC_API_KEY`
3. **Server running** on `http://localhost:8080` (or configure `API_BASE_URL`)

## Running Tests

### Option 1: PowerShell Script (Recommended)

```powershell
# Run tests against running server
.\run-e2e-tests.ps1

# Run tests and start/stop server automatically
.\run-e2e-tests.ps1 -StartServer -StopServer

# Run tests against different server
.\run-e2e-tests.ps1 -ApiBaseUrl "http://localhost:9000"
```

### Option 2: Direct Go Command

```bash
# Run tests directly
go run test-e2e.go

# With custom environment variables
API_BASE_URL="http://localhost:9000" API_KEY="custom-key" go run test-e2e.go
```

## Test Coverage

The e2e tests cover:

### Non-Streaming Endpoints
- ✅ `/v1/generate` with all three providers
- ✅ Response parsing and validation
- ✅ Token usage tracking
- ✅ Cost calculation

### Streaming Endpoints  
- ✅ `/v1/generate/stream` with all three providers
- ✅ Real-time streaming response handling
- ✅ Stream completion detection
- ✅ Response content validation

### Performance Metrics
- ✅ Response time measurement
- ✅ Success/failure tracking
- ✅ Detailed error reporting
- ✅ Test summary with statistics

## Test Results

When all tests pass, you'll see output like:

```
🚀 Starting E2E Tests for AptRouter API
Base URL: http://localhost:8080
Test Prompt: Write a short, friendly greeting in exactly 2 sentences.
Timeout: 30s

📝 Testing Non-Streaming Endpoints
==================================
✅ anthropic non-streaming: SUCCESS (1.19s)
✅ openai non-streaming: SUCCESS (2.02s)  
✅ google non-streaming: SUCCESS (0.65s)

🌊 Testing Streaming Endpoints
===============================
✅ openai streaming: SUCCESS (2.08s)
✅ google streaming: SUCCESS (0.52s)
✅ anthropic streaming: SUCCESS (0.77s)

📊 Test Summary
===============
Total Tests: 6
Successful: 6
Failed: 0
Success Rate: 100.0%

🎉 All tests passed! The API is working correctly.
```

## Configuration

### Environment Variables

- `API_BASE_URL`: API server URL (default: `http://localhost:8080`)
- `API_KEY`: API key for authentication (default: `test-api-key`)

### Test Parameters

- **Test Prompt**: "Write a short, friendly greeting in exactly 2 sentences."
- **Timeout**: 30 seconds per request
- **Delay**: 1 second between requests to avoid rate limiting

## Troubleshooting

### Common Issues

1. **Server not running**
   ```
   ❌ API server is not running at http://localhost:8080
   ```
   Solution: Start the server with `go run cmd/api/main.go`

2. **Missing API keys**
   ```
   ❌ Some E2E tests failed
   Error: no API key provided for provider: openai
   ```
   Solution: Set the required environment variables

3. **Model not found**
   ```
   ❌ Some E2E tests failed  
   Error: model config not found for model ID: gpt-3.5-turbo
   ```
   Solution: Check that models are configured in `internal/pricing/service.go`

### Debug Mode

For detailed debugging, you can modify the test prompt or add more verbose logging in `test-e2e.go`.

## Continuous Integration

The e2e tests can be integrated into CI/CD pipelines:

```yaml
# Example GitHub Actions step
- name: Run E2E Tests
  run: |
    go run cmd/api/main.go &
    sleep 10
    go run test-e2e.go
  env:
    OPENAI_API_KEY: ${{ secrets.OPENAI_API_KEY }}
    GOOGLE_API_KEY: ${{ secrets.GOOGLE_API_KEY }}
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
```

## Performance Benchmarks

Typical response times for the test models:

| Provider | Model | Non-Streaming | Streaming |
|----------|-------|---------------|-----------|
| OpenAI | gpt-3.5-turbo | ~2.0s | ~2.1s |
| Google | gemini-2.0-flash | ~0.7s | ~0.5s |
| Anthropic | claude-3-haiku | ~1.2s | ~0.8s |

*Note: Response times may vary based on network conditions and API availability.* 