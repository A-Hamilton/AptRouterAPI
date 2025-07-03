package data

import (
	"context"
	"fmt"
	"io"
	"iter"
	"log/slog"

	"google.golang.org/genai"
)

// GoogleClient implements LLMClient for Google
type GoogleClient struct {
	modelID string
	apiKey  string
}

// NewGoogleClient creates a new Google client
func NewGoogleClient(modelID, apiKey string) (LLMClient, error) {
	return &GoogleClient{
		modelID: modelID,
		apiKey:  apiKey,
	}, nil
}

// GenerateWithParams generates text using Google's API
func (c *GoogleClient) GenerateWithParams(ctx context.Context, params map[string]interface{}) (*GenerateResponse, error) {
	slog.Info("Google client: Starting real API call", "model", c.modelID, "api_key_length", len(c.apiKey))

	prompt, ok := params["prompt"].(string)
	if !ok {
		return nil, &ProviderError{
			Provider:  "google",
			ModelID:   c.modelID,
			Message:   "prompt parameter is required and must be a string",
			Retryable: false,
		}
	}

	// Note: maxTokens is not used in the current Gemini API implementation
	// but kept for future use with generation config
	_ = params["max_tokens"]

	slog.Info("Google client: Creating client and making API call", "model", c.modelID, "prompt_length", len(prompt))

	// Create Google Gemini client
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		slog.Error("Google client: Failed to create client", "error", err, "model", c.modelID)
		return nil, &ProviderError{
			Provider:  "google",
			ModelID:   c.modelID,
			Message:   fmt.Sprintf("failed to create client: %v", err),
			Retryable: false,
		}
	}

	// Map model ID to Gemini model
	geminiModel := "gemini-2.0-flash"
	switch c.modelID {
	case "gemini-1.5-pro":
		geminiModel = "gemini-1.5-pro"
	case "gemini-1.5-flash":
		geminiModel = "gemini-1.5-flash"
	case "gemini-2.0-flash":
		geminiModel = "gemini-2.0-flash"
	default:
		geminiModel = "gemini-2.0-flash"
	}

	// Create content with text
	content := []*genai.Content{{
		Parts: []*genai.Part{{Text: prompt}},
	}}

	// Call the Gemini API
	resp, err := client.Models.GenerateContent(ctx, geminiModel, content, nil)
	if err != nil {
		// Try to extract status code and error code from error if possible
		statusCode := 0
		errCode := ""
		msg := err.Error()
		if apiErr, ok := err.(*genai.APIError); ok {
			// apiErr.Code is the HTTP status code (int), apiErr.Message is the error message
			statusCode = apiErr.Code
			if statusCode < 100 || statusCode > 599 {
				statusCode = 0 // unknown or not an HTTP status code
			}
			errCode = fmt.Sprintf("%d", apiErr.Code)
			if apiErr.Message != "" {
				msg = apiErr.Message
			}
		}
		// Determine retryability
		retryable := true
		if statusCode == 401 || statusCode == 402 || statusCode == 403 || statusCode == 404 || statusCode == 429 {
			retryable = false
		} else if statusCode >= 500 && statusCode < 600 {
			retryable = true
		}
		return nil, &ProviderError{
			Provider:   "google",
			ModelID:    c.modelID,
			StatusCode: statusCode,
			ErrorCode:  errCode,
			Message:    msg,
			Retryable:  retryable,
		}
	}

	slog.Info("Google client: API call successful", "model", c.modelID, "candidates_count", len(resp.Candidates))

	// Extract response text
	responseText := ""
	if len(resp.Candidates) > 0 && len(resp.Candidates[0].Content.Parts) > 0 {
		responseText = resp.Candidates[0].Content.Parts[0].Text
	}

	if responseText == "" {
		slog.Error("Google client: No content returned", "model", c.modelID)
		return nil, &ProviderError{
			Provider:  "google",
			ModelID:   c.modelID,
			Message:   "no content returned from API",
			Retryable: false,
		}
	}

	slog.Info("Google client: Response received", "model", c.modelID, "response_length", len(responseText))

	// Use actual token usage from provider response if available
	var inputTokens, outputTokens int
	if resp.UsageMetadata != nil && (resp.UsageMetadata.PromptTokenCount > 0 || resp.UsageMetadata.CandidatesTokenCount > 0) {
		// Use actual token counts from Google response
		inputTokens = int(resp.UsageMetadata.PromptTokenCount)
		outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
		slog.Info("Google client: Using actual token usage from provider",
			"input_tokens", inputTokens, "output_tokens", outputTokens)
	} else {
		// CRITICAL: No fallback to tokenizer estimates - we must use real API usage data
		slog.Warn("Google client: No usage data provided by API - cannot calculate accurate token counts",
			"input_tokens", 0, "output_tokens", 0,
			"note", "Using real API usage data only, no estimators allowed")
		inputTokens = 0
		outputTokens = 0
	}

	return &GenerateResponse{
		Text:         responseText,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Usage: &UsageInfo{
			PromptTokens:     inputTokens,
			CompletionTokens: outputTokens,
			TotalTokens:      inputTokens + outputTokens,
		},
		FinishReason: "STOP",
		ModelID:      c.modelID,
		Provider:     "google",
	}, nil
}

// GenerateStream generates text with streaming response using Google's API
func (c *GoogleClient) GenerateStream(ctx context.Context, params map[string]interface{}) (*StreamResponse, error) {
	slog.Info("Google client: Starting streaming API call", "model", c.modelID, "api_key_length", len(c.apiKey))

	prompt, ok := params["prompt"].(string)
	if !ok {
		return nil, &ProviderError{
			Provider:  "google",
			ModelID:   c.modelID,
			Message:   "prompt parameter is required and must be a string",
			Retryable: false,
		}
	}

	slog.Info("Google client: Creating streaming client", "model", c.modelID, "prompt_length", len(prompt))

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey:  c.apiKey,
		Backend: genai.BackendGeminiAPI,
	})
	if err != nil {
		slog.Error("Google client: Failed to create client", "error", err, "model", c.modelID)
		return nil, &ProviderError{
			Provider:  "google",
			ModelID:   c.modelID,
			Message:   fmt.Sprintf("failed to create client: %v", err),
			Retryable: false,
		}
	}

	// Use gemini-2.0-flash as the model
	geminiModel := "gemini-2.0-flash"

	slog.Info("Google client: Using Gemini model", "input_model", c.modelID, "gemini_model", geminiModel)

	content := []*genai.Content{{
		Parts: []*genai.Part{{Text: prompt}},
	}}

	stream := client.Models.GenerateContentStream(ctx, geminiModel, content, nil)

	streamReader := &GoogleStreamReader{
		stream: stream,
	}

	return &StreamResponse{
		Stream: streamReader,
		Metadata: map[string]string{
			"provider": "google",
			"model_id": c.modelID,
		},
	}, nil
}

// GoogleStreamReader is a stream reader for Google Gemini
// Uses the iter.Seq2[*genai.GenerateContentResponse, error] type
type GoogleStreamReader struct {
	stream iter.Seq2[*genai.GenerateContentResponse, error]
	buffer []byte
	pos    int
	closed bool
	// Channel to receive items from the iterator
	items chan *genai.GenerateContentResponse
	// Channel to receive errors from the iterator
	errors chan error
	// Channel to signal when iteration is done
	done chan struct{}
	// Current error if any
	currentError error
	// Usage tracking
	inputTokens  int
	outputTokens int
	usageFound   bool
}

func (r *GoogleStreamReader) Read(p []byte) (n int, err error) {
	if r.closed {
		return 0, io.EOF
	}

	// Initialize channels on first read
	if r.items == nil {
		r.items = make(chan *genai.GenerateContentResponse, 1)
		r.errors = make(chan error, 1)
		r.done = make(chan struct{})

		// Start the iterator in a goroutine
		go func() {
			defer close(r.done)
			r.stream(func(resp *genai.GenerateContentResponse, err error) bool {
				if err != nil {
					select {
					case r.errors <- err:
					case <-r.done:
					}
					return false
				}
				select {
				case r.items <- resp:
				case <-r.done:
				}
				return true
			})
		}()
	}

	// Check for current error
	if r.currentError != nil {
		err := r.currentError
		r.currentError = nil
		return 0, err
	}

	if r.pos < len(r.buffer) {
		n = copy(p, r.buffer[r.pos:])
		r.pos += n
		if r.pos >= len(r.buffer) {
			r.buffer = nil
			r.pos = 0
		}
		return n, nil
	}

	// Get next item from the iterator
	select {
	case resp := <-r.items:
		if resp == nil {
			r.closed = true
			slog.Debug("GoogleStreamReader: Stream ended")
			return 0, io.EOF
		}

		// Check for usage information in the response
		if !r.usageFound && resp.UsageMetadata != nil {
			r.inputTokens = int(resp.UsageMetadata.PromptTokenCount)
			r.outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
			if r.inputTokens > 0 || r.outputTokens > 0 {
				r.usageFound = true
				slog.Info("Google streaming: Captured usage from response",
					"input_tokens", r.inputTokens, "output_tokens", r.outputTokens)
			}
		}

		// Also check if this is the final chunk (no more content) and we haven't found usage yet
		if !r.usageFound && (len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0) {
			// This might be the final chunk with usage data
			if resp.UsageMetadata != nil {
				r.inputTokens = int(resp.UsageMetadata.PromptTokenCount)
				r.outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
				if r.inputTokens > 0 || r.outputTokens > 0 {
					r.usageFound = true
					slog.Info("Google streaming: Captured usage from final chunk",
						"input_tokens", r.inputTokens, "output_tokens", r.outputTokens)
				}
			}
		}

		// Log the response structure for debugging
		if resp.UsageMetadata != nil {
			slog.Debug("Google streaming: Found UsageMetadata",
				"prompt_token_count", resp.UsageMetadata.PromptTokenCount,
				"candidates_token_count", resp.UsageMetadata.CandidatesTokenCount,
				"total_token_count", resp.UsageMetadata.TotalTokenCount)
		} else {
			slog.Debug("Google streaming: No UsageMetadata in response")
		}

		// Always check for usage metadata in every response - Google Gemini provides it
		if resp.UsageMetadata != nil {
			// Update usage data if we haven't found it yet or if this response has more complete data
			if !r.usageFound || (resp.UsageMetadata.PromptTokenCount > 0 && resp.UsageMetadata.CandidatesTokenCount > 0) {
				r.inputTokens = int(resp.UsageMetadata.PromptTokenCount)
				r.outputTokens = int(resp.UsageMetadata.CandidatesTokenCount)
				r.usageFound = true
				slog.Info("Google streaming: Updated usage from response",
					"input_tokens", r.inputTokens, "output_tokens", r.outputTokens)
			}
		}

		if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
			slog.Debug("GoogleStreamReader: No candidates or content parts in response")
			return 0, nil
		}

		content := resp.Candidates[0].Content.Parts[0].Text
		if content == "" {
			slog.Debug("GoogleStreamReader: Empty content text")
			return 0, nil
		}

		r.buffer = []byte(content)
		r.pos = 0

		n = copy(p, r.buffer)
		r.pos += n
		if r.pos >= len(r.buffer) {
			r.buffer = nil
			r.pos = 0
		}

		return n, nil

	case err := <-r.errors:
		r.closed = true
		slog.Error("GoogleStreamReader: Stream error", "error", err)
		return 0, err

	case <-r.done:
		r.closed = true
		slog.Debug("GoogleStreamReader: Stream ended")
		return 0, io.EOF
	}
}

func (r *GoogleStreamReader) Close() error {
	r.closed = true
	if r.done != nil {
		select {
		case <-r.done:
			// Channel already closed, do nothing
		default:
			close(r.done)
		}
	}
	return nil
}

// GetUsage returns the captured usage information
func (r *GoogleStreamReader) GetUsage() (int, int) {
	return r.inputTokens, r.outputTokens
}
