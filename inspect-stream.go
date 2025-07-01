package main

import (
	"context"
	"fmt"
	"os"
	"reflect"

	anthropic "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

func main() {
	fmt.Println("=== Anthropic Stream Type Inspection ===")

	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		fmt.Println("❌ ANTHROPIC_API_KEY environment variable not set")
		return
	}

	// Create Anthropic client
	client := anthropic.NewClient(option.WithAPIKey(apiKey))

	// Create streaming request
	stream := client.Messages.NewStreaming(context.Background(), anthropic.MessageNewParams{
		MaxTokens: int64(100),
		Messages: []anthropic.MessageParam{{
			Content: []anthropic.ContentBlockParamUnion{{
				OfText: &anthropic.TextBlockParam{Text: "Hello"},
			}},
			Role: anthropic.MessageParamRoleUser,
		}},
		Model:       anthropic.ModelClaude3_7SonnetLatest,
		Temperature: anthropic.Float(0.7),
	})

	// Inspect stream type
	fmt.Printf("Stream type: %T\n", stream)
	fmt.Printf("Stream value: %+v\n", stream)

	// Use reflection to inspect the stream
	val := reflect.ValueOf(stream)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}
	fmt.Printf("Stream kind: %s\n", val.Kind())
	fmt.Printf("Stream type: %s\n", val.Type())

	// List all methods
	typ := val.Type()
	fmt.Printf("Methods:\n")
	for i := 0; i < typ.NumMethod(); i++ {
		method := typ.Method(i)
		fmt.Printf("  %s\n", method.Name)
	}

	// List all fields
	fmt.Printf("Fields:\n")
	for i := 0; i < val.NumField(); i++ {
		field := val.Field(i)
		fieldType := val.Type().Field(i)
		fmt.Printf("  %s: %s = %v\n", fieldType.Name, field.Type(), field.Interface())
	}

	// Try to call Next() to see what happens
	fmt.Printf("\n--- Testing Next() ---\n")
	if nextMethod := val.MethodByName("Next"); nextMethod.IsValid() {
		fmt.Println("Next() method found")
		results := nextMethod.Call(nil)
		fmt.Printf("Next() returned: %v\n", results)
	} else {
		fmt.Println("Next() method not found")
	}

	// Try to call Event() to see what happens
	fmt.Printf("\n--- Testing Event() ---\n")
	if eventMethod := val.MethodByName("Event"); eventMethod.IsValid() {
		fmt.Println("Event() method found")
		results := eventMethod.Call(nil)
		fmt.Printf("Event() returned: %v\n", results)
	} else {
		fmt.Println("Event() method not found")
	}
}
