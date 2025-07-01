# Firestore Setup Guide for AptRouter API

This guide will help you set up Firestore with the new structure for the AptRouter API.

## Overview

The new Firestore structure includes 5 collections:
1. **users** - User profiles and balances
2. **api_keys** - Hashed API keys for authentication
3. **request_logs** - API usage logs for audit and analytics
4. **model_configurations** - LLM model pricing and configuration
5. **pricing_tiers** - User pricing tiers with percentage markups

## Prerequisites

1. **Firebase Project**: You need a Firebase project with Firestore enabled
2. **Service Account Key**: Download your Firebase service account key
3. **Go Environment**: Make sure Go is installed and configured

## Step 1: Firebase Project Setup

1. Go to [Firebase Console](https://console.firebase.google.com/)
2. Create a new project or select existing project
3. Enable Firestore Database
4. Set up security rules (see below)

## Step 2: Download Service Account Key

1. In Firebase Console, go to **Project Settings** > **Service Accounts**
2. Click **Generate New Private Key**
3. Save the JSON file as `firestore-credentials.json` in your project root
4. **Important**: Add this file to `.gitignore` to keep it secure

## Step 3: Update Environment Variables

Create or update your `.env` file:

```env
# === AptRouter API Environment Variables ===

# --- Server Configuration ---
PORT=8080
ENV=development

# --- Firebase Configuration ---
FIREBASE_PROJECT_ID=your-project-id
FIREBASE_SERVICE_ACCOUNT_PATH=firestore-credentials.json

# --- Memory Cache Configuration ---
CACHE_DEFAULT_EXPIRATION=5m
CACHE_CLEANUP_INTERVAL=10m

# --- LLM Provider API Keys ---
GOOGLE_API_KEY=your-google-api-key
OPENAI_API_KEY=your-openai-api-key
ANTHROPIC_API_KEY=your-anthropic-api-key

# --- Logging ---
LOGGING_LEVEL=info
LOGGING_FORMAT=json

# --- Rate Limiting ---
RATE_LIMIT_REQUESTS_PER_MINUTE=100
RATE_LIMIT_BURST=20

# --- Optimization Settings ---
OPTIMIZATION_ENABLED=true
OPTIMIZATION_FALLBACK_ON_OPTIMIZATION_FAILURE=true
```

## Step 4: Set Up Firestore Security Rules

In Firebase Console > Firestore Database > Rules, add these security rules:

```javascript
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    // Users can only read their own data
    match /users/{userId} {
      allow read, write: if request.auth != null && request.auth.uid == userId;
    }
    
    // API keys - users can only read their own keys
    match /api_keys/{keyId} {
      allow read: if request.auth != null && 
        resource.data.user_id == request.auth.uid;
      allow write: if false; // Only system can write
    }
    
    // Pricing tiers are public for reading
    match /pricing_tiers/{tierId} {
      allow read: if true;
      allow write: if false; // Only admin can modify
    }
    
    // Model configurations are public for reading
    match /model_configurations/{modelId} {
      allow read: if true;
      allow write: if false; // Only admin can modify
    }
    
    // Request logs - users can only read their own
    match /request_logs/{logId} {
      allow read: if request.auth != null && 
        resource.data.user_id == request.auth.uid;
      allow write: if false; // Only system can write
    }
  }
}
```

## Step 5: Populate Mock Data

Run the mock data setup script:

```powershell
# Windows PowerShell
.\setup-mock-data.ps1

# Or manually
go run cmd/mock-data/main.go
```

This will create:
- **Test User**: `test-user-1` with $100 balance
- **Test API Key**: `test-api-key-hash` 
- **Test Pricing Tier**: `tier-1` with 10% markup
- **Sample Model**: `gpt-4o` configuration
- **Sample Request Log**: Test request data

## Step 6: Test the Setup

Start the API server:

```bash
go run cmd/api/main.go
```

Test with the mock API key:

```bash
curl -X POST http://localhost:8080/v1/generate \
  -H "Authorization: test-api-key-hash" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-4o",
    "prompt": "Hello, world!",
    "max_tokens": 100
  }'
```

## Firestore Collections Structure

### 1. users Collection
```json
{
  "id": "test-user-1",
  "email": "testuser@example.com",
  "balance": 100.0,
  "tier_id": "tier-1",
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z",
  "is_active": true,
  "custom_pricing": false
}
```

### 2. api_keys Collection
```json
{
  "id": "test-api-key-hash",
  "user_id": "test-user-1",
  "key_hash": "test-api-key-hash",
  "name": "Test Key",
  "status": "active",
  "created_at": "2024-01-01T00:00:00Z",
  "last_used": "2024-01-01T00:00:00Z"
}
```

### 3. request_logs Collection
```json
{
  "id": "test-request-1",
  "user_id": "test-user-1",
  "api_key_id": "test-api-key-hash",
  "request_id": "test-request-1",
  "model_id": "gpt-4o",
  "provider": "openai",
  "input_tokens": 100,
  "output_tokens": 50,
  "total_tokens": 150,
  "base_cost": 0.01,
  "markup_amount": 0.001,
  "total_cost": 0.011,
  "tier_id": "tier-1",
  "markup_percent": 10.0,
  "was_optimized": true,
  "optimization_status": "success",
  "tokens_saved": 10,
  "savings_amount": 0.0005,
  "streaming": false,
  "request_timestamp": "2024-01-01T00:00:00Z",
  "response_timestamp": "2024-01-01T00:01:00Z",
  "duration_ms": 1000,
  "status": "success",
  "ip_address": "127.0.0.1",
  "user_agent": "curl/7.68.0",
  "metadata": {"test": true}
}
```

### 4. model_configurations Collection
```json
{
  "id": "gpt-4o",
  "model_id": "gpt-4o",
  "provider": "openai",
  "input_price_per_million": 5.0,
  "output_price_per_million": 15.0,
  "context_length": 128000,
  "is_active": true,
  "created_at": "2024-01-01T00:00:00Z"
}
```

### 5. pricing_tiers Collection
```json
{
  "id": "tier-1",
  "name": "Free Tier",
  "min_monthly_spend": 0,
  "input_markup_percent": 10.0,
  "output_markup_percent": 10.0,
  "is_active": true,
  "is_custom": false,
  "custom_model_pricing": {}
}
```

## Pricing Model

The new pricing model works as follows:

### Base Cost
- You pay the model provider (e.g., $5 per 1M input tokens for GPT-4o)
- This is your cost to provide the service

### Your Fee (Percentage Markup)
- **Tier 1**: 10% markup (new users, low volume)
- **Tier 2**: 7% markup (medium volume users)
- **Tier 3**: 5% markup (high volume users)
- **Custom Tier**: Negotiated rates for enterprise customers

### Example
- User requests 1M tokens using GPT-4o
- Base cost: $5 (you pay to OpenAI)
- Your markup: $0.50 (10% for Tier 1)
- User pays: $5.50
- Your profit: $0.50

## API Key Management

### Frontend Generation
The frontend should generate API keys using a secure method:

```javascript
// Example: Generate API key hash
const crypto = require('crypto');
const apiKey = crypto.randomBytes(32).toString('hex');
const keyHash = crypto.createHash('sha256').update(apiKey).digest('hex');

// Store in Firestore
await firestore.collection('api_keys').doc(keyHash).set({
  id: keyHash,
  user_id: userId,
  key_hash: keyHash,
  name: 'My API Key',
  status: 'active',
  created_at: new Date()
});
```

### Backend Validation
The backend validates API keys by:
1. Hashing the provided API key
2. Looking up the hash in Firestore
3. Checking if the key is active
4. Loading the associated user profile

## Testing

### Test Data
After running the mock data script, you can test with:
- **API Key**: `test-api-key-hash`
- **User ID**: `test-user-1`
- **Pricing Tier**: `tier-1`

### Test Commands
```bash
# Test API key validation
curl -X POST http://localhost:8080/v1/generate \
  -H "Authorization: test-api-key-hash" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4o", "prompt": "Hello", "max_tokens": 10}'

# Test streaming
curl -X POST http://localhost:8080/v1/generate/stream \
  -H "Authorization: test-api-key-hash" \
  -H "Content-Type: application/json" \
  -d '{"model": "gpt-4o", "prompt": "Hello", "max_tokens": 10}'
```

## Troubleshooting

### Common Issues

1. **Service Account Key Not Found**
   ```
   Error: failed to initialize Firebase app
   ```
   Solution: Ensure `firestore-credentials.json` exists in project root

2. **Permission Denied**
   ```
   Error: API key not found
   ```
   Solution: Check Firestore security rules and ensure API key exists

3. **Model Not Found**
   ```
   Error: model config not found for model ID: gpt-4o
   ```
   Solution: Run the mock data script to populate model configurations

4. **User Balance Issues**
   ```
   Error: insufficient balance
   ```
   Solution: Check user balance in Firestore or add funds

### Debug Mode

Enable debug logging in your `.env`:
```env
LOGGING_LEVEL=debug
```

## Next Steps

1. **Production Deployment**: Update security rules for production
2. **User Management**: Implement user registration and authentication
3. **Billing Integration**: Add payment processing for balance top-ups
4. **Analytics**: Build dashboards using request_logs data
5. **Monitoring**: Set up alerts for high usage or errors

## Support

For issues with this setup:
1. Check Firebase Console for errors
2. Verify service account key permissions
3. Test with the provided mock data
4. Review Firestore security rules
5. Check environment variables 