package llm

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"reflect"

	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"
	"github.com/pkoukk/tiktoken-go"
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
		slog.Error("OpenAI client: API call failed", "error", err, "model", c.modelID)
		return nil, &ProviderError{
			Provider:  "openai",
			ModelID:   c.modelID,
			Message:   fmt.Sprintf("API call failed: %v", err),
			Retryable: true,
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
		Usage:        nil, // Optionally map resp.Usage
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
	stream := client.Chat.Completions.NewStreaming(ctx, openai.ChatCompletionNewParams{
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.UserMessage(prompt),
		},
		Model:       openai.ChatModel(c.modelID),
		MaxTokens:   openai.Int(int64(maxTokens)),
		Temperature: openai.Float(temperature),
	})

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
	// Type assert to openai.ChatCompletionChunk (not pointer)
	chunk, ok := currentResult[0].Interface().(openai.ChatCompletionChunk)
	if !ok {
		return 0, nil
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

// CountTokens counts tokens using tiktoken
func (c *OpenAIClient) CountTokens(text string) (int, error) {
	// Get encoding for the model
	encoding, err := tiktoken.GetEncoding("cl100k_base") // OpenAI uses cl100k_base
	if err != nil {
		return 0, fmt.Errorf("failed to get encoding: %w", err)
	}

	tokens := encoding.Encode(text, nil, nil)
	return len(tokens), nil
}
