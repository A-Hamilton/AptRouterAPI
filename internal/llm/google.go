package llm

import (
	"context"
	"fmt"
	"io"
	"iter"
	"log/slog"

	"github.com/pkoukk/tiktoken-go"
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
	content := []*genai.Content{
		{
			Parts: []*genai.Part{
				{Text: prompt},
			},
		},
	}

	slog.Info("Google client: Making API call", "model", c.modelID, "gemini_model", geminiModel)

	// Generate content
	result, err := client.Models.GenerateContent(ctx, geminiModel, content, nil)
	if err != nil {
		slog.Error("Google client: API call failed", "error", err, "model", c.modelID)
		return nil, &ProviderError{
			Provider:  "google",
			ModelID:   c.modelID,
			Message:   fmt.Sprintf("API call failed: %v", err),
			Retryable: true,
		}
	}

	slog.Info("Google client: API call successful", "model", c.modelID, "candidates_count", len(result.Candidates))

	// Extract response text
	responseText := ""
	if len(result.Candidates) > 0 && len(result.Candidates[0].Content.Parts) > 0 {
		responseText = result.Candidates[0].Content.Parts[0].Text
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

	// Count tokens
	inputTokens, err := c.CountTokens(prompt)
	if err != nil {
		slog.Warn("Failed to count input tokens", "error", err)
		inputTokens = 0
	}

	outputTokens, err := c.CountTokens(responseText)
	if err != nil {
		slog.Warn("Failed to count output tokens", "error", err)
		outputTokens = 0
	}

	return &GenerateResponse{
		Text:         responseText,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		Usage:        nil,
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

// CountTokens counts tokens using tiktoken
func (c *GoogleClient) CountTokens(text string) (int, error) {
	// Get encoding for the model - Google uses a different encoding
	// For now, use cl100k_base as an approximation
	encoding, err := tiktoken.GetEncoding("cl100k_base")
	if err != nil {
		return 0, fmt.Errorf("failed to get encoding: %w", err)
	}

	tokens := encoding.Encode(text, nil, nil)
	return len(tokens), nil
}
