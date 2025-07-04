package data

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	ssestream "github.com/anthropics/anthropic-sdk-go/packages/ssestream"
)

// AnthropicClient implements LLMClient for Anthropic
type AnthropicClient struct {
	modelID string
	apiKey  string
}

// NewAnthropicClient creates a new Anthropic client
func NewAnthropicClient(modelID, apiKey string) (LLMClient, error) {
	return &AnthropicClient{
		modelID: modelID,
		apiKey:  apiKey,
	}, nil
}

// GenerateWithParams generates text using Anthropic's API
func (c *AnthropicClient) GenerateWithParams(ctx context.Context, params map[string]interface{}) (*GenerateResponse, error) {
	slog.Info("Anthropic client: Starting real API call", "model", c.modelID, "api_key_length", len(c.apiKey))

	prompt, ok := params["prompt"].(string)
	if !ok {
		return nil, &ProviderError{
			Provider:  "anthropic",
			ModelID:   c.modelID,
			Message:   "prompt parameter is required and must be a string",
			Retryable: false,
		}
	}

	maxTokens := 1000
	if mt, ok := params["max_tokens"].(int); ok {
		maxTokens = mt
	}
	temperature := 0.7
	if temp, ok := params["temperature"].(float64); ok {
		temperature = temp
	}

	slog.Info("Anthropic client: Creating client and making API call", "model", c.modelID, "prompt_length", len(prompt))

	// Create Anthropic client
	client := anthropic.NewClient(option.WithAPIKey(c.apiKey))

	// Map model ID to Anthropic model - use actual model IDs
	anthropicModel := anthropic.Model(c.modelID)
	switch c.modelID {
	case "claude-3-5-sonnet-20241022":
		anthropicModel = anthropic.Model("claude-3-5-sonnet-20241022")
	case "claude-3-5-haiku-20241022":
		anthropicModel = anthropic.Model("claude-3-5-haiku-20241022")
	case "claude-3-haiku-20240307":
		anthropicModel = anthropic.Model("claude-3-haiku-20240307")
	case "claude-3-opus-20240229":
		anthropicModel = anthropic.Model("claude-3-opus-20240229")
	case "claude-3-sonnet-20240229":
		anthropicModel = anthropic.Model("claude-3-sonnet-20240229")
	default:
		// Use the model ID directly if no specific mapping
		anthropicModel = anthropic.Model(c.modelID)
	}

	slog.Info("Anthropic client: Making API call", "model", c.modelID, "anthropic_model", anthropicModel)

	// Make API call
	resp, err := client.Messages.New(ctx, anthropic.MessageNewParams{
		MaxTokens: int64(maxTokens),
		Messages: []anthropic.MessageParam{{
			Content: []anthropic.ContentBlockParamUnion{{
				OfText: &anthropic.TextBlockParam{Text: prompt},
			}},
			Role: anthropic.MessageParamRoleUser,
		}},
		Model:       anthropicModel,
		Temperature: anthropic.Float(temperature),
	})
	if err != nil {
		slog.Error("Anthropic client: API call failed", "error", err, "model", c.modelID)
		return nil, &ProviderError{
			Provider:  "anthropic",
			ModelID:   c.modelID,
			Message:   fmt.Sprintf("API call failed: %v", err),
			Retryable: true,
		}
	}

	slog.Info("Anthropic client: API call successful", "model", c.modelID, "content_blocks", len(resp.Content))

	// Extract response text
	responseText := ""
	for _, content := range resp.Content {
		if content.Type == "text" {
			responseText += content.Text
		}
	}

	if responseText == "" {
		slog.Error("Anthropic client: No content returned", "model", c.modelID)
		return nil, &ProviderError{
			Provider:  "anthropic",
			ModelID:   c.modelID,
			Message:   "no content returned from API",
			Retryable: false,
		}
	}

	slog.Info("Anthropic client: Response received", "model", c.modelID, "response_length", len(responseText))

	// Use actual token usage from provider response if available
	var inputTokens, outputTokens int
	if resp.Usage.InputTokens > 0 || resp.Usage.OutputTokens > 0 {
		// Use actual token counts from Anthropic response
		inputTokens = int(resp.Usage.InputTokens)
		outputTokens = int(resp.Usage.OutputTokens)
		slog.Info("Anthropic client: Using actual token usage from provider",
			"input_tokens", inputTokens, "output_tokens", outputTokens)
	} else {
		// CRITICAL: No fallback to tokenizer estimates - we must use real API usage data
		slog.Warn("Anthropic client: No usage data provided by API - cannot calculate accurate token counts",
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
		FinishReason: "end_turn",
		ModelID:      c.modelID,
		Provider:     "anthropic",
	}, nil
}

// GenerateStream generates text with streaming response using Anthropic's API
func (c *AnthropicClient) GenerateStream(ctx context.Context, params map[string]interface{}) (*StreamResponse, error) {
	slog.Info("Anthropic client: Starting streaming API call", "model", c.modelID, "api_key_length", len(c.apiKey))

	prompt, ok := params["prompt"].(string)
	if !ok {
		return nil, &ProviderError{
			Provider:  "anthropic",
			ModelID:   c.modelID,
			Message:   "prompt parameter is required and must be a string",
			Retryable: false,
		}
	}

	slog.Info("Anthropic client: Creating streaming client", "model", c.modelID, "prompt_length", len(prompt))

	client := anthropic.NewClient(option.WithAPIKey(c.apiKey))

	// Map model ID to Anthropic model - use actual model IDs
	anthropicModel := anthropic.Model(c.modelID)
	switch c.modelID {
	case "claude-3-5-sonnet-20241022":
		anthropicModel = anthropic.Model("claude-3-5-sonnet-20241022")
	case "claude-3-5-haiku-20241022":
		anthropicModel = anthropic.Model("claude-3-5-haiku-20241022")
	case "claude-3-haiku-20240307":
		anthropicModel = anthropic.Model("claude-3-haiku-20240307")
	case "claude-3-opus-20240229":
		anthropicModel = anthropic.Model("claude-3-opus-20240229")
	case "claude-3-sonnet-20240229":
		anthropicModel = anthropic.Model("claude-3-sonnet-20240229")
	default:
		// Use the model ID directly if no specific mapping
		anthropicModel = anthropic.Model(c.modelID)
	}

	stream := client.Messages.NewStreaming(ctx, anthropic.MessageNewParams{
		MaxTokens: int64(1000),
		Messages: []anthropic.MessageParam{{
			Content: []anthropic.ContentBlockParamUnion{{
				OfText: &anthropic.TextBlockParam{Text: prompt},
			}},
			Role: anthropic.MessageParamRoleUser,
		}},
		Model:       anthropicModel,
		Temperature: anthropic.Float(0.7),
	})

	streamReader := &AnthropicStreamReader{
		stream: stream,
	}

	return &StreamResponse{
		Stream: streamReader,
		Metadata: map[string]string{
			"provider": "anthropic",
			"model_id": c.modelID,
		},
	}, nil
}

// AnthropicStreamReader is a stream reader for Anthropic Claude
// Uses the concrete ssestream.Stream[anthropic.MessageStreamEventUnion]
type AnthropicStreamReader struct {
	stream *ssestream.Stream[anthropic.MessageStreamEventUnion]
	buffer []byte
	pos    int
	closed bool
	// Usage tracking
	inputTokens  int
	outputTokens int
	usageFound   bool
}

func (r *AnthropicStreamReader) Read(p []byte) (n int, err error) {
	slog.Info("AnthropicStreamReader: Read called", "closed", r.closed, "pos", r.pos, "buffer_len", len(r.buffer))
	if r.closed {
		return 0, io.EOF
	}

	if r.pos < len(r.buffer) {
		n = copy(p, r.buffer[r.pos:])
		r.pos += n
		if r.pos >= len(r.buffer) {
			r.buffer = nil
			r.pos = 0
		}
		slog.Info("AnthropicStreamReader: Returning buffered content", "n", n)
		return n, nil
	}

	if !r.stream.Next() {
		if r.stream.Err() != nil {
			r.closed = true
			slog.Error("AnthropicStreamReader: Stream error", "error", r.stream.Err())
			return 0, r.stream.Err()
		}
		r.closed = true
		slog.Debug("AnthropicStreamReader: Stream ended")
		return 0, io.EOF
	}

	event := r.stream.Current()
	slog.Info("AnthropicStreamReader: Event received", "event_type", fmt.Sprintf("%T", event))

	// Check for usage information in the final chunk
	// Anthropic provides usage data in the final message event
	if !r.usageFound {
		// Use reflection to check for usage data in the event
		eventValue := reflect.ValueOf(event)
		if eventValue.Kind() == reflect.Ptr {
			eventValue = eventValue.Elem()
		}

		// Look for usage fields
		if usageField := eventValue.FieldByName("Usage"); usageField.IsValid() {
			if inputField := usageField.FieldByName("InputTokens"); inputField.IsValid() && inputField.Kind() == reflect.Int64 {
				r.inputTokens = int(inputField.Int())
			}
			if outputField := usageField.FieldByName("OutputTokens"); outputField.IsValid() && outputField.Kind() == reflect.Int64 {
				r.outputTokens = int(outputField.Int())
			}
			if r.inputTokens > 0 || r.outputTokens > 0 {
				r.usageFound = true
				slog.Info("Anthropic streaming: Captured usage from final chunk",
					"input_tokens", r.inputTokens, "output_tokens", r.outputTokens)
			}
		}
	}

	var content string

	// Try to extract content using reflection and type assertions
	if textEvent, ok := any(event).(interface{ GetText() string }); ok {
		content = textEvent.GetText()
		slog.Info("AnthropicStreamReader: Content extracted via GetText", "content", content)
	}

	// If no content found, try to extract from the event structure
	if content == "" {
		// Use reflection to explore the event structure
		eventValue := reflect.ValueOf(event)
		if eventValue.Kind() == reflect.Ptr {
			eventValue = eventValue.Elem()
		}

		// Look for common fields that might contain text
		if deltaField := eventValue.FieldByName("Delta"); deltaField.IsValid() {
			if textField := deltaField.FieldByName("Text"); textField.IsValid() && textField.Kind() == reflect.String {
				content = textField.String()
				slog.Info("AnthropicStreamReader: Content extracted from Delta.Text", "content", content)
			}
		}

		if contentField := eventValue.FieldByName("Content"); contentField.IsValid() {
			if textField := contentField.FieldByName("Text"); textField.IsValid() && textField.Kind() == reflect.String {
				content = textField.String()
				slog.Info("AnthropicStreamReader: Content extracted from Content.Text", "content", content)
			}
		}
	}

	if content == "" {
		slog.Debug("AnthropicStreamReader: No text content found in event", "event_type", fmt.Sprintf("%T", event))
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

	slog.Info("AnthropicStreamReader: Returning new content", "n", n, "content", content)
	return n, nil
}

func (r *AnthropicStreamReader) Close() error {
	r.closed = true
	return nil
}

// GetUsage returns the captured usage information
func (r *AnthropicStreamReader) GetUsage() (int, int) {
	return r.inputTokens, r.outputTokens
}
