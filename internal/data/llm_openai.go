package data

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"reflect"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
)

// OpenAIClient implements LLMClient for OpenAI
type OpenAIClient struct {
	modelID string
	apiKey  string
}

// NewOpenAIClient creates a new OpenAI client
func NewOpenAIClient(modelID, apiKey string) (LLMClient, error) {
	return &OpenAIClient{
		modelID: modelID,
		apiKey:  apiKey,
	}, nil
}

// GenerateWithParams generates text using OpenAI's API
func (c *OpenAIClient) GenerateWithParams(ctx context.Context, params map[string]interface{}) (*GenerateResponse, error) {
	slog.Info("OpenAI client: Starting real API call", "model", c.modelID, "api_key_length", len(c.apiKey))

	prompt, ok := params["prompt"].(string)
	if !ok {
		return nil, &ProviderError{
			Provider:  "openai",
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

	slog.Info("OpenAI client: Creating client and making API call", "model", c.modelID, "prompt_length", len(prompt))

	client := openai.NewClient(option.WithAPIKey(c.apiKey))
	resp, err := client.Chat.Completions.New(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       openai.ChatModel(c.modelID),
		MaxTokens:   openai.Int(int64(maxTokens)),
		Temperature: openai.Float(temperature),
	})
	if err != nil {
		// Try to extract structured error info
		var apiErr *openai.Error
		statusCode := 0
		errCode := ""
		msg := err.Error()
		if errors.As(err, &apiErr) {
			statusCode = apiErr.StatusCode
			errCode = apiErr.Code
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
			Provider:   "openai",
			ModelID:    c.modelID,
			StatusCode: statusCode,
			ErrorCode:  errCode,
			Message:    msg,
			Retryable:  retryable,
		}
	}

	slog.Info("OpenAI client: API call successful", "model", c.modelID, "choices_count", len(resp.Choices))

	if len(resp.Choices) == 0 {
		return nil, &ProviderError{
			Provider:  "openai",
			ModelID:   c.modelID,
			Message:   "no choices returned from API",
			Retryable: false,
		}
	}

	responseText := resp.Choices[0].Message.Content
	slog.Info("OpenAI client: Response received", "model", c.modelID, "response_length", len(responseText))

	// Use actual token usage from provider response if available
	var inputTokens, outputTokens int
	if resp.Usage.PromptTokens > 0 || resp.Usage.CompletionTokens > 0 {
		// Use actual token counts from OpenAI response
		inputTokens = int(resp.Usage.PromptTokens)
		outputTokens = int(resp.Usage.CompletionTokens)
		slog.Info("OpenAI client: Using actual token usage from provider",
			"input_tokens", inputTokens, "output_tokens", outputTokens)
	} else {
		// CRITICAL: No fallback to tokenizer estimates - we must use real API usage data
		slog.Warn("OpenAI client: No usage data provided by API - cannot calculate accurate token counts",
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
		FinishReason: string(resp.Choices[0].FinishReason),
		ModelID:      c.modelID,
		Provider:     "openai",
	}, nil
}

// GenerateStream generates text with streaming response using OpenAI's API
func (c *OpenAIClient) GenerateStream(ctx context.Context, params map[string]interface{}) (*StreamResponse, error) {
	slog.Info("OpenAI client: Starting streaming API call", "model", c.modelID, "api_key_length", len(c.apiKey))

	prompt, ok := params["prompt"].(string)
	if !ok {
		return nil, &ProviderError{
			Provider:  "openai",
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

	slog.Info("OpenAI client: Creating streaming client", "model", c.modelID, "prompt_length", len(prompt))

	client := openai.NewClient(option.WithAPIKey(c.apiKey))

	// Check if include_usage is requested
	includeUsage := false
	if includeUsageParam, ok := params["include_usage"].(bool); ok {
		includeUsage = includeUsageParam
	}

	streamParams := openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       openai.ChatModel(c.modelID),
		MaxTokens:   openai.Int(int64(maxTokens)),
		Temperature: openai.Float(temperature),
	}

	// Add stream options if include_usage is requested
	if includeUsage {
		streamParams.StreamOptions = openai.ChatCompletionStreamOptionsParam{
			IncludeUsage: openai.Bool(true),
		}
	}

	stream := client.Chat.Completions.NewStreaming(ctx, streamParams)

	streamReader := &OpenAIStreamReader{
		stream:  stream,
		modelID: c.modelID,
	}

	return &StreamResponse{
		Stream: streamReader,
		Metadata: map[string]string{
			"provider": "openai",
			"model_id": c.modelID,
		},
	}, nil
}

// OpenAIStreamReader wraps the OpenAI stream to implement io.ReadCloser
type OpenAIStreamReader struct {
	stream  interface{}
	modelID string
	buffer  []byte
	pos     int
	closed  bool
	// Usage tracking
	inputTokens  int
	outputTokens int
	usageFound   bool
}

func (r *OpenAIStreamReader) Read(p []byte) (n int, err error) {
	if r.closed {
		return 0, io.EOF
	}

	// If buffer has data, return it
	if r.pos < len(r.buffer) {
		n = copy(p, r.buffer[r.pos:])
		r.pos += n
		if r.pos >= len(r.buffer) {
			r.buffer = nil
			r.pos = 0
		}
		return n, nil
	}

	// Use reflection to call Next() and Current()
	streamValue := reflect.ValueOf(r.stream)
	// Call Next()
	nextMethod := streamValue.MethodByName("Next")
	if !nextMethod.IsValid() {
		r.closed = true
		return 0, io.EOF
	}
	nextResult := nextMethod.Call(nil)
	if len(nextResult) == 0 || !nextResult[0].Bool() {
		// Check for error
		errMethod := streamValue.MethodByName("Err")
		if errMethod.IsValid() {
			errResult := errMethod.Call(nil)
			if len(errResult) > 0 && !errResult[0].IsNil() {
				r.closed = true
				return 0, errResult[0].Interface().(error)
			}
		}
		r.closed = true
		return 0, io.EOF
	}
	// Call Current()
	currentMethod := streamValue.MethodByName("Current")
	if !currentMethod.IsValid() {
		r.closed = true
		return 0, io.EOF
	}
	currentResult := currentMethod.Call(nil)
	if len(currentResult) == 0 {
		r.closed = true
		return 0, io.EOF
	}

	// Type assert to openai.ChatCompletionChunk
	chunk, ok := currentResult[0].Interface().(openai.ChatCompletionChunk)
	if !ok {
		return 0, nil
	}

	// Check for usage information in the final chunk
	// According to OpenAI docs, when include_usage=true, the final chunk has usage data
	if !r.usageFound && (chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0) {
		r.inputTokens = int(chunk.Usage.PromptTokens)
		r.outputTokens = int(chunk.Usage.CompletionTokens)
		r.usageFound = true
		slog.Info("OpenAI streaming: Captured usage from final chunk",
			"input_tokens", r.inputTokens, "output_tokens", r.outputTokens)
	}

	var content string
	if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
		content = chunk.Choices[0].Delta.Content
	}
	if content != "" {
		r.buffer = []byte(content)
		r.pos = 0
		n = copy(p, r.buffer)
		r.pos += n
		if r.pos >= len(r.buffer) {
			r.buffer = nil
			r.pos = 0
		}
		return n, nil
	}
	return 0, nil
}

func (r *OpenAIStreamReader) Close() error {
	r.closed = true
	return nil
}

// GetUsage returns the captured usage information
func (r *OpenAIStreamReader) GetUsage() (int, int) {
	return r.inputTokens, r.outputTokens
}
