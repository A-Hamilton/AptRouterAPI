# Firebase Setup Guide

## Overview

This guide will help you set up Firebase for the AptRouter API with the new percentage-based pricing model using Firebase CLI authentication.

## Pricing Model

The new pricing model works as follows:

### Base Cost
- You pay the model provider (e.g., $10 per 1M tokens for Gemini 2.5 Pro)
- This is your cost to provide the service

### Your Fee (Percentage Markup)
- **Tier 1**: 10% markup (new users, low volume)
- **Tier 2**: 7% markup (medium volume users)
- **Tier 3**: 5% markup (high volume users)
- **Custom Tier**: Negotiated rates for enterprise customers

### Example
- User requests 1M tokens using Gemini 2.5 Pro
- Base cost: $10 (you pay to Google)
- Your markup: $1 (10% for Tier 1)
- User pays: $11
- Your profit: $1

## Firebase Setup Steps

### 1. Install Firebase CLI

```bash
npm install -g firebase-tools
```

### 2. Login to Firebase

```bash
firebase login
```

This will open a browser window for authentication. Use your Google account that has access to the Firebase project.

### 3. Initialize Firebase in Your Project

```bash
firebase init
```

During initialization:
1. **Select your project**: Choose `aptrouter-44552` (or create a new one)
2. **Select services**: Choose:
   - Firestore Database
   - Authentication (optional, for future use)
3. **Configure Firestore**: Use default settings
4. **Configure Authentication**: Use default settings

This will create:
- `.firebaserc` - Project configuration
- `firebase.json` - Firebase configuration
- `firestore.rules` - Security rules
- `firestore.indexes.json` - Database indexes

### 4. Set Up Environment Variables

Create a `.env` file with the following variables:

```bash
# Firebase Configuration
FIREBASE_PROJECT_ID=aptrouter-44552
FIREBASE_USE_CLI_AUTH=true

# Other configurations...
```

### 5. Set Up Firestore Database

1. Go to Firestore Database in Firebase Console
2. Create database in production mode
3. Set up the following collections:

#### Users Collection
```json
{
  "id": "user123",
  "email": "user@example.com",
  "balance": 100.0,
  "tier_id": "tier-1",
  "is_active": true,
  "custom_pricing": false,
  "created_at": "2024-01-01T00:00:00Z",
  "updated_at": "2024-01-01T00:00:00Z"
}
```

#### Pricing Tiers Collection
```json
{
  "id": "tier-1",
  "name": "Free Tier",
  "min_monthly_spend": 0,
  "input_markup_percent": 10.0,
  "output_markup_percent": 10.0,
  "is_active": true,
  "is_custom": false,
  "custom_model_pricing": {},
  "created_at": "2024-01-01T00:00:00Z"
}
```

```json
{
  "id": "tier-2",
  "name": "Growth Tier",
  "min_monthly_spend": 100,
  "input_markup_percent": 7.0,
  "output_markup_percent": 7.0,
  "is_active": true,
  "is_custom": false,
  "custom_model_pricing": {},
  "created_at": "2024-01-01T00:00:00Z"
}
```

```json
{
  "id": "tier-3",
  "name": "Enterprise Tier",
  "min_monthly_spend": 1000,
  "input_markup_percent": 5.0,
  "output_markup_percent": 5.0,
  "is_active": true,
  "is_custom": false,
  "custom_model_pricing": {},
  "created_at": "2024-01-01T00:00:00Z"
}
```

#### Model Configurations Collection
```json
{
  "id": "gemini-2.5-pro",
  "model_id": "gemini-2.5-pro",
  "provider": "google",
  "input_price_per_million": 10.0,
  "output_price_per_million": 30.0,
  "context_length": 1000000,
  "is_active": true,
  "created_at": "2024-01-01T00:00:00Z"
}
```

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

```json
{
  "id": "claude-3-5-sonnet",
  "model_id": "claude-3-5-sonnet",
  "provider": "anthropic",
  "input_price_per_million": 3.0,
  "output_price_per_million": 15.0,
  "context_length": 200000,
  "is_active": true,
  "created_at": "2024-01-01T00:00:00Z"
}
```

#### Request Logs Collection
```json
{
  "id": "req_123",
  "user_id": "user123",
  "api_key_id": "key_hash",
  "request_id": "req_123",
  "model_id": "gemini-2.5-pro",
  "provider": "google",
  "input_tokens": 1000,
  "output_tokens": 500,
  "total_tokens": 1500,
  "base_cost": 0.015,
  "markup_amount": 0.0015,
  "total_cost": 0.0165,
  "tier_id": "tier-1",
  "markup_percent": 10.0,
  "was_optimized": true,
  "optimization_status": "success",
  "tokens_saved": 200,
  "savings_amount": 0.002,
  "streaming": false,
  "request_timestamp": "2024-01-01T00:00:00Z",
  "response_timestamp": "2024-01-01T00:00:01Z",
  "duration_ms": 1000,
  "status": "success",
  "ip_address": "192.168.1.1",
  "user_agent": "curl/7.68.0",
  "metadata": {}
}
```

### 6. Set Up Security Rules

In Firestore Database > Rules, add the following security rules:

```javascript
rules_version = '2';
service cloud.firestore {
  match /databases/{database}/documents {
    // Users can only read their own data
    match /users/{userId} {
      allow read, write: if request.auth != null && request.auth.uid == userId;
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

## Firebase CLI Authentication Benefits

### Security
- **No service account keys**: No need to manage sensitive JSON files
- **Automatic token refresh**: Firebase CLI handles authentication automatically
- **Local development**: Works seamlessly in development environments

### Ease of Use
- **Simple setup**: Just run `firebase login` and `firebase init`
- **No key management**: No need to download, store, or rotate service account keys
- **Team collaboration**: Multiple developers can use the same project easily

### Production Deployment
For production deployment, you can still use service account keys if needed:

```bash
# Set environment variable to use service account key
FIREBASE_USE_CLI_AUTH=false
FIREBASE_SERVICE_ACCOUNT_PATH=/path/to/service-account.json
```

## Testing the Setup

1. Start the server:
```bash
go run cmd/api/main.go
```

2. Test with a mock API key:
```bash
curl -X POST http://localhost:8080/v1/generate \
  -H "Authorization: mock-api-key-for-development" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gemini-2.5-pro",
    "prompt": "Hello, world!",
    "max_tokens": 100
  }'
```

3. Check Firestore for the logged request

## Monitoring and Analytics

### Key Metrics to Track
- **Revenue per user**: Track how much each user spends
- **Tier progression**: Monitor users moving between tiers
- **Model usage**: Which models are most popular
- **Optimization effectiveness**: How much tokens are saved
- **Error rates**: Monitor failed requests

### Firebase Analytics
- Set up Firebase Analytics for web dashboard
- Create custom events for API usage
- Build dashboards for business metrics

## Security Considerations

1. **Firebase CLI Security**: Keep your Firebase CLI login secure
2. **Rate Limiting**: Implement per-user rate limits
3. **Balance Checks**: Always verify user balance before processing
4. **Audit Logging**: All requests are logged for security
5. **Tier Validation**: Verify pricing tier access
6. **Input Validation**: Sanitize all user inputs

## Cost Optimization

### For Users
- Higher tiers get lower percentage markups
- Optimization reduces token usage
- Bulk usage discounts available

### For You
- Monitor model provider costs
- Negotiate better rates with high volume
- Implement caching to reduce API calls
- Use optimization to reduce token usage

## Next Steps

1. Complete Firebase CLI setup with `firebase init`
2. Set up Firestore collections
3. Test the API with mock data
4. Implement user registration and authentication
5. Set up monitoring and analytics
6. Deploy to production

## Support

For issues with Firebase setup:
1. Check Firebase Console for errors
2. Verify Firebase CLI authentication with `firebase projects:list`
3. Test with Firebase CLI commands
4. Review Firestore security rules
5. Check environment variables 