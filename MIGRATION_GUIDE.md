# Migration Guide: Hardcoded to Database-Driven Model Configurations

## Overview

This guide helps you migrate from hardcoded model configurations to a fully database-driven approach using Firestore. This change provides dynamic model management without code deployments.

## Current State

### Before Migration
- **Model Configurations**: Hardcoded in `internal/pricing/service.go`
- **Pricing Tiers**: Already database-driven (Firestore)
- **Fallback**: Hardcoded defaults always loaded first

### After Migration
- **Model Configurations**: Database-driven (Firestore) with hardcoded fallback
- **Pricing Tiers**: Database-driven (Firestore)
- **Fallback**: Hardcoded defaults only when Firestore fails

## Benefits of Migration

1. **Dynamic Updates**: Change model pricing without deployments
2. **A/B Testing**: Enable/disable models dynamically
3. **Cost Management**: Update prices in real-time
4. **Operational Flexibility**: Add new models without code changes
5. **Consistency**: Both model configs and pricing tiers in same place

## Migration Steps

### Step 1: Run the Migration Script

The updated `cmd/mock-data/main.go` now includes all hardcoded model configurations:

```bash
# Run the migration script
go run cmd/mock-data/main.go
```

This will populate Firestore with:
- **26 Model Configurations** (all current hardcoded models)
- **1 Test User** with $100 balance
- **1 Test API Key** for testing
- **1 Test Request Log** for reference
- **1 Pricing Tier** (Free Tier with 10% markup)

### Step 2: Verify Migration

Check Firestore Console to confirm:
1. `model_configurations` collection has 26 documents
2. All model IDs match the hardcoded configurations
3. Pricing and context windows are correct

### Step 3: Test the System

Start the server and test with the migration script:

```bash
# Start server
go run cmd/api/main.go

# In another terminal, test with the mock data
curl -X POST http://localhost:8080/v1/generate \
  -H "Authorization: test-api-key-hash" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o-2024-05-13",
    "prompt": "Hello, world!",
    "max_tokens": 100
  }'
```

### Step 4: Monitor Logs

Check server logs for:
```
✅ Successfully loaded model configurations from Firestore
📊 Model configurations and pricing tiers pre-cached successfully model_count=26
```

## Model Configuration Schema

Each model configuration in Firestore has this structure:

```json
{
  "id": "101",
  "model_id": "gpt-4.1-2025-04-14",
  "provider": "openai",
  "input_price_per_million": 2.00,
  "output_price_per_million": 8.00,
  "context_window_size": 128000,
  "is_active": true,
  "created_at": "2024-01-01T00:00:00Z"
}
```

### Fields Explained

- **id**: Unique identifier (numeric string)
- **model_id**: The actual model identifier used in API calls
- **provider**: LLM provider ("openai", "google", "anthropic")
- **input_price_per_million**: Cost per 1M input tokens
- **output_price_per_million**: Cost per 1M output tokens
- **context_window_size**: Maximum context window in tokens
- **is_active**: Whether the model is available for use
- **created_at**: Timestamp when the configuration was created

## Included Models

### OpenAI Models (13 models)
- `gpt-4.1-2025-04-14` - Latest GPT-4.1 model
- `gpt-4.1-mini-2025-04-14` - Mini version
- `gpt-4.1-nano-2025-04-14` - Nano version
- `gpt-4.5-preview-2025-02-27` - Preview model
- `gpt-4o-2024-08-06` - GPT-4o model
- `gpt-4o-2024-11-20` - Updated GPT-4o
- `gpt-4o-2024-05-13` - Earlier GPT-4o
- `gpt-4o-mini-2024-07-18` - GPT-4o mini
- `o1-2024-12-17` - O1 model
- `o3-2025-04-16` - O3 model
- `o3-mini-2025-01-31` - O3 mini
- `o1-mini-2024-09-12` - O1 mini
- `codex-mini-latest` - Codex model

### Google Gemini Models (15 models)
- `gemini-2.5-pro` - Latest Gemini Pro
- `gemini-2.5-flash` - Fast Gemini model
- `gemini-2.5-flash-lite-preview-06-17` - Lite preview
- `gemini-2.0-flash` - Gemini 2.0 Flash
- `gemini-2.0-flash-lite` - Gemini 2.0 Flash Lite
- `gemini-1.5-flash` - Gemini 1.5 Flash
- `gemini-1.5-flash-8b` - 8B parameter version
- `gemini-1.5-flash-1b` - 1B parameter version
- `gemini-1.5-pro` - Gemini 1.5 Pro
- `gemini-1.5-pro-1m` - 1M context version
- `gemini-1.5-pro-latest` - Latest 1.5 Pro
- `gemini-1.5-flash-latest` - Latest 1.5 Flash
- `gemini-1.0-pro` - Gemini 1.0 Pro
- `gemini-1.0-pro-001` - Specific version
- `gemini-1.0-pro-latest` - Latest 1.0 Pro

### Anthropic Claude Models (6 models)
- `claude-3-5-sonnet-20241022` - Claude 3.5 Sonnet
- `claude-3-5-haiku-20241022` - Claude 3.5 Haiku
- `claude-3-5-opus-20241022` - Claude 3.5 Opus
- `claude-3-opus-20240229` - Claude 3 Opus
- `claude-3-5-sonnet-latest` - Latest Sonnet
- `claude-3-5-haiku-latest` - Latest Haiku

## Managing Models

### Adding New Models

1. **Via Firestore Console**:
   ```json
   {
     "id": "27",
     "model_id": "new-model-id",
     "provider": "openai",
     "input_price_per_million": 1.00,
     "output_price_per_million": 4.00,
     "context_window_size": 128000,
     "is_active": true,
     "created_at": "2024-01-01T00:00:00Z"
   }
   ```

2. **Via API** (future enhancement):
   ```bash
   curl -X POST /admin/models \
     -H "Authorization: admin-key" \
     -d '{"model_id": "new-model", "provider": "openai", ...}'
   ```

### Updating Model Pricing

1. **Via Firestore Console**: Edit the document directly
2. **Via Code**: Update the document programmatically
3. **Automatic Refresh**: Server will pick up changes on next cache refresh

### Disabling Models

Set `is_active: false` in the model configuration document.

## Cache Management

### Cache Refresh

The system automatically refreshes the cache:
- **On startup**: Loads all configurations
- **Periodic refresh**: Every 10 minutes (configurable)
- **Manual refresh**: Via admin endpoint (future)

### Cache Statistics

Monitor cache performance:
```bash
curl -X GET /admin/cache/stats \
  -H "Authorization: admin-key"
```

Response:
```json
{
  "model_configs_count": 26,
  "last_refresh": "2024-01-01T12:00:00Z",
  "cache_ttl": "10m",
  "should_refresh": false
}
```

## Fallback Strategy

### When Firestore Fails

1. **Startup**: Falls back to hardcoded defaults
2. **Runtime**: Continues using cached data
3. **Refresh**: Attempts to reload from Firestore

### Hardcoded Defaults

Hardcoded defaults remain as emergency fallback:
- Ensures system availability
- Provides baseline functionality
- Can be updated via code deployment

## Monitoring and Alerts

### Key Metrics to Monitor

1. **Cache Hit Rate**: Should be >95%
2. **Firestore Read Latency**: Should be <100ms
3. **Model Configuration Count**: Should match expected count
4. **Cache Refresh Success Rate**: Should be >99%

### Alerts to Set Up

1. **Cache Miss**: When hardcoded defaults are used
2. **Firestore Errors**: When database operations fail
3. **Model Not Found**: When requested model doesn't exist
4. **Cache Refresh Failure**: When refresh operations fail

## Rollback Plan

### If Migration Fails

1. **Immediate Rollback**: Revert code changes
2. **Data Rollback**: Delete Firestore collections
3. **Fallback**: System continues with hardcoded defaults

### Rollback Commands

```bash
# Revert code changes
git revert <commit-hash>

# Clear Firestore data (if needed)
# Use Firestore Console to delete collections
```

## Production Deployment

### Pre-Deployment Checklist

- [ ] Migration script tested in staging
- [ ] All model configurations verified
- [ ] Cache refresh working correctly
- [ ] Fallback mechanism tested
- [ ] Monitoring alerts configured
- [ ] Rollback plan documented

### Deployment Steps

1. **Deploy Code**: Push updated code
2. **Run Migration**: Execute migration script
3. **Verify Data**: Check Firestore collections
4. **Test System**: Verify all models working
5. **Monitor**: Watch for any issues

### Post-Deployment Verification

1. **Check Logs**: Ensure Firestore loading successful
2. **Test Models**: Verify all models accessible
3. **Monitor Performance**: Check cache hit rates
4. **Verify Pricing**: Confirm costs calculated correctly

## Troubleshooting

### Common Issues

1. **Models Not Found**
   - Check Firestore for model configurations
   - Verify `is_active: true`
   - Check cache refresh logs

2. **Pricing Errors**
   - Verify price fields in Firestore
   - Check pricing tier configuration
   - Review cost calculation logic

3. **Cache Issues**
   - Check cache refresh logs
   - Verify Firestore connectivity
   - Review cache TTL settings

### Debug Commands

```bash
# Check cache status
curl -X GET /admin/cache/stats

# Force cache refresh
curl -X POST /admin/cache/refresh

# Test specific model
curl -X POST /v1/generate \
  -H "Authorization: test-api-key-hash" \
  -d '{"model": "gpt-4o", "prompt": "test"}'
```

## Future Enhancements

### Planned Features

1. **Admin API**: REST endpoints for model management
2. **Bulk Operations**: Import/export model configurations
3. **Versioning**: Track model configuration changes
4. **A/B Testing**: Dynamic model allocation
5. **Analytics**: Model usage and performance tracking

### API Endpoints (Future)

```bash
# List all models
GET /admin/models

# Add new model
POST /admin/models

# Update model
PUT /admin/models/{model_id}

# Delete model
DELETE /admin/models/{model_id}

# Bulk import
POST /admin/models/bulk
```

## Conclusion

This migration provides a solid foundation for dynamic model management while maintaining system reliability through fallback mechanisms. The database-driven approach enables operational flexibility and real-time updates without code deployments. 