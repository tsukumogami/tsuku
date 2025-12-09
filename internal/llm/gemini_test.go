package llm

import (
	"context"
	"os"
	"testing"

	"github.com/google/generative-ai-go/genai"
)

func TestGeminiProviderName(t *testing.T) {
	// Skip if no API key (can't create real provider)
	if os.Getenv("GOOGLE_API_KEY") == "" && os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY/GEMINI_API_KEY not set, skipping provider name test")
	}

	ctx := context.Background()
	provider, err := NewGeminiProvider(ctx)
	if err != nil {
		t.Fatalf("NewGeminiProvider failed: %v", err)
	}
	defer provider.Close()

	if got := provider.Name(); got != "gemini" {
		t.Errorf("Name() = %q, want %q", got, "gemini")
	}
}

func TestNewGeminiProviderMissingAPIKey(t *testing.T) {
	// Save and clear the API key
	originalKey := os.Getenv("GOOGLE_API_KEY")
	os.Unsetenv("GOOGLE_API_KEY")
	defer os.Setenv("GOOGLE_API_KEY", originalKey)

	ctx := context.Background()
	_, err := NewGeminiProvider(ctx)
	if err == nil {
		t.Error("NewGeminiProvider should fail when GOOGLE_API_KEY is not set")
	}
}

func TestConvertSchemaToGemini(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		wantType genai.Type
	}{
		{
			name:     "object type",
			input:    map[string]any{"type": "object"},
			wantType: genai.TypeObject,
		},
		{
			name:     "array type",
			input:    map[string]any{"type": "array"},
			wantType: genai.TypeArray,
		},
		{
			name:     "string type",
			input:    map[string]any{"type": "string"},
			wantType: genai.TypeString,
		},
		{
			name:     "number type",
			input:    map[string]any{"type": "number"},
			wantType: genai.TypeNumber,
		},
		{
			name:     "integer type",
			input:    map[string]any{"type": "integer"},
			wantType: genai.TypeInteger,
		},
		{
			name:     "boolean type",
			input:    map[string]any{"type": "boolean"},
			wantType: genai.TypeBoolean,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			schema := convertSchemaToGemini(tt.input)
			if schema.Type != tt.wantType {
				t.Errorf("Type = %v, want %v", schema.Type, tt.wantType)
			}
		})
	}
}

func TestConvertSchemaToGeminiWithProperties(t *testing.T) {
	input := map[string]any{
		"type": "object",
		"properties": map[string]interface{}{
			"name": map[string]interface{}{
				"type":        "string",
				"description": "The name of the item",
			},
			"count": map[string]interface{}{
				"type": "integer",
			},
		},
		"required": []string{"name"},
	}

	schema := convertSchemaToGemini(input)

	if schema.Type != genai.TypeObject {
		t.Errorf("Type = %v, want %v", schema.Type, genai.TypeObject)
	}

	if len(schema.Properties) != 2 {
		t.Errorf("len(Properties) = %d, want 2", len(schema.Properties))
	}

	if schema.Properties["name"] == nil {
		t.Error("Properties[\"name\"] is nil")
	} else {
		if schema.Properties["name"].Type != genai.TypeString {
			t.Errorf("Properties[\"name\"].Type = %v, want %v", schema.Properties["name"].Type, genai.TypeString)
		}
		if schema.Properties["name"].Description != "The name of the item" {
			t.Errorf("Properties[\"name\"].Description = %q, want %q", schema.Properties["name"].Description, "The name of the item")
		}
	}

	if len(schema.Required) != 1 || schema.Required[0] != "name" {
		t.Errorf("Required = %v, want [\"name\"]", schema.Required)
	}
}

func TestConvertSchemaToGeminiWithArrayItems(t *testing.T) {
	input := map[string]any{
		"type": "array",
		"items": map[string]interface{}{
			"type": "string",
		},
	}

	schema := convertSchemaToGemini(input)

	if schema.Type != genai.TypeArray {
		t.Errorf("Type = %v, want %v", schema.Type, genai.TypeArray)
	}

	if schema.Items == nil {
		t.Error("Items is nil")
	} else if schema.Items.Type != genai.TypeString {
		t.Errorf("Items.Type = %v, want %v", schema.Items.Type, genai.TypeString)
	}
}

func TestConvertSchemaToGeminiNil(t *testing.T) {
	schema := convertSchemaToGemini(nil)
	if schema != nil {
		t.Errorf("convertSchemaToGemini(nil) = %v, want nil", schema)
	}
}

func TestConvertSchemaToGeminiRequiredAsInterfaceSlice(t *testing.T) {
	// This tests when required comes as []interface{} instead of []string
	input := map[string]any{
		"type":     "object",
		"required": []interface{}{"field1", "field2"},
	}

	schema := convertSchemaToGemini(input)

	if len(schema.Required) != 2 {
		t.Errorf("len(Required) = %d, want 2", len(schema.Required))
	}
	if schema.Required[0] != "field1" || schema.Required[1] != "field2" {
		t.Errorf("Required = %v, want [\"field1\", \"field2\"]", schema.Required)
	}
}

// Integration test - requires GOOGLE_API_KEY or GEMINI_API_KEY
func TestGeminiProviderComplete(t *testing.T) {
	if os.Getenv("GOOGLE_API_KEY") == "" && os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY/GEMINI_API_KEY not set, skipping integration test")
	}

	ctx := context.Background()
	provider, err := NewGeminiProvider(ctx)
	if err != nil {
		t.Fatalf("NewGeminiProvider failed: %v", err)
	}
	defer provider.Close()

	req := &CompletionRequest{
		SystemPrompt: "You are a helpful assistant. Respond briefly.",
		Messages: []Message{
			{Role: RoleUser, Content: "Say hello"},
		},
		MaxTokens: 100,
	}

	resp, err := provider.Complete(ctx, req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if resp.Content == "" {
		t.Error("Response content is empty")
	}

	if resp.StopReason == "" {
		t.Error("StopReason is empty")
	}
}

// Integration test with tool use - requires GOOGLE_API_KEY or GEMINI_API_KEY
func TestGeminiProviderCompleteWithTools(t *testing.T) {
	if os.Getenv("GOOGLE_API_KEY") == "" && os.Getenv("GEMINI_API_KEY") == "" {
		t.Skip("GOOGLE_API_KEY/GEMINI_API_KEY not set, skipping integration test")
	}

	ctx := context.Background()
	provider, err := NewGeminiProvider(ctx)
	if err != nil {
		t.Fatalf("NewGeminiProvider failed: %v", err)
	}
	defer provider.Close()

	req := &CompletionRequest{
		SystemPrompt: "You must use the get_weather tool to answer questions about weather. Do not answer without calling the tool first.",
		Messages: []Message{
			{Role: RoleUser, Content: "What's the weather in Tokyo?"},
		},
		Tools: []ToolDef{
			{
				Name:        "get_weather",
				Description: "Get the current weather for a location",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]interface{}{
						"location": map[string]interface{}{
							"type":        "string",
							"description": "The city name",
						},
					},
					"required": []string{"location"},
				},
			},
		},
		MaxTokens: 200,
	}

	resp, err := provider.Complete(ctx, req)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}

	if len(resp.ToolCalls) == 0 {
		t.Error("Expected tool calls but got none")
	} else {
		tc := resp.ToolCalls[0]
		if tc.Name != "get_weather" {
			t.Errorf("ToolCall.Name = %q, want %q", tc.Name, "get_weather")
		}
		if tc.Arguments["location"] == nil {
			t.Error("ToolCall.Arguments[\"location\"] is nil")
		}
	}
}
