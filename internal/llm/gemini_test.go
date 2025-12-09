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
	defer func() { _ = provider.Close() }()

	if got := provider.Name(); got != "gemini" {
		t.Errorf("Name() = %q, want %q", got, "gemini")
	}
}

func TestNewGeminiProviderMissingAPIKey(t *testing.T) {
	// Save and clear both API keys
	originalGoogleKey := os.Getenv("GOOGLE_API_KEY")
	originalGeminiKey := os.Getenv("GEMINI_API_KEY")
	_ = os.Unsetenv("GOOGLE_API_KEY")
	_ = os.Unsetenv("GEMINI_API_KEY")
	defer func() {
		_ = os.Setenv("GOOGLE_API_KEY", originalGoogleKey)
		_ = os.Setenv("GEMINI_API_KEY", originalGeminiKey)
	}()

	ctx := context.Background()
	_, err := NewGeminiProvider(ctx)
	if err == nil {
		t.Error("NewGeminiProvider should fail when neither GOOGLE_API_KEY nor GEMINI_API_KEY is set")
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

func TestInt32Ptr(t *testing.T) {
	val := int32(42)
	ptr := int32Ptr(val)
	if ptr == nil {
		t.Fatal("int32Ptr returned nil")
	}
	if *ptr != val {
		t.Errorf("*int32Ptr(42) = %d, want %d", *ptr, val)
	}
}

func TestConvertTools(t *testing.T) {
	// Create a zero-value provider for testing conversion methods
	p := &GeminiProvider{}

	tools := []ToolDef{
		{
			Name:        "get_weather",
			Description: "Get the current weather",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]interface{}{
					"location": map[string]interface{}{
						"type":        "string",
						"description": "City name",
					},
				},
				"required": []string{"location"},
			},
		},
		{
			Name:        "search",
			Description: "Search for information",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]interface{}{
					"query": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	}

	declarations := p.convertTools(tools)

	if len(declarations) != 2 {
		t.Fatalf("len(declarations) = %d, want 2", len(declarations))
	}

	// Check first tool
	if declarations[0].Name != "get_weather" {
		t.Errorf("declarations[0].Name = %q, want %q", declarations[0].Name, "get_weather")
	}
	if declarations[0].Description != "Get the current weather" {
		t.Errorf("declarations[0].Description = %q, want %q", declarations[0].Description, "Get the current weather")
	}
	if declarations[0].Parameters == nil {
		t.Error("declarations[0].Parameters is nil")
	}

	// Check second tool
	if declarations[1].Name != "search" {
		t.Errorf("declarations[1].Name = %q, want %q", declarations[1].Name, "search")
	}
}

func TestConvertToolsEmpty(t *testing.T) {
	p := &GeminiProvider{}
	declarations := p.convertTools([]ToolDef{})
	if len(declarations) != 0 {
		t.Errorf("len(declarations) = %d, want 0", len(declarations))
	}
}

func TestConvertMessages(t *testing.T) {
	p := &GeminiProvider{}

	messages := []Message{
		{Role: RoleUser, Content: "Hello"},
		{Role: RoleAssistant, Content: "Hi there!"},
		{Role: RoleUser, Content: "How are you?"},
	}

	parts := p.convertMessages(messages)

	if len(parts) != 3 {
		t.Fatalf("len(parts) = %d, want 3", len(parts))
	}

	// Check first message (user text)
	if text, ok := parts[0].(genai.Text); !ok {
		t.Errorf("parts[0] is not genai.Text, got %T", parts[0])
	} else if string(text) != "Hello" {
		t.Errorf("parts[0] = %q, want %q", string(text), "Hello")
	}

	// Check second message (assistant text)
	if text, ok := parts[1].(genai.Text); !ok {
		t.Errorf("parts[1] is not genai.Text, got %T", parts[1])
	} else if string(text) != "Hi there!" {
		t.Errorf("parts[1] = %q, want %q", string(text), "Hi there!")
	}
}

func TestConvertMessagesWithToolResult(t *testing.T) {
	p := &GeminiProvider{}

	messages := []Message{
		{
			Role: RoleUser,
			ToolResult: &ToolResult{
				CallID:  "get_weather",
				Content: "Sunny, 25°C",
			},
		},
	}

	parts := p.convertMessages(messages)

	if len(parts) != 1 {
		t.Fatalf("len(parts) = %d, want 1", len(parts))
	}

	funcResp, ok := parts[0].(genai.FunctionResponse)
	if !ok {
		t.Fatalf("parts[0] is not genai.FunctionResponse, got %T", parts[0])
	}

	if funcResp.Name != "get_weather" {
		t.Errorf("FunctionResponse.Name = %q, want %q", funcResp.Name, "get_weather")
	}

	result, ok := funcResp.Response["result"].(string)
	if !ok {
		t.Fatalf("FunctionResponse.Response[\"result\"] is not string")
	}
	if result != "Sunny, 25°C" {
		t.Errorf("FunctionResponse.Response[\"result\"] = %q, want %q", result, "Sunny, 25°C")
	}
}

func TestConvertMessagesWithToolCalls(t *testing.T) {
	p := &GeminiProvider{}

	messages := []Message{
		{
			Role: RoleAssistant,
			ToolCalls: []ToolCall{
				{
					ID:        "call1",
					Name:      "get_weather",
					Arguments: map[string]any{"location": "Tokyo"},
				},
				{
					ID:        "call2",
					Name:      "search",
					Arguments: map[string]any{"query": "weather forecast"},
				},
			},
		},
	}

	parts := p.convertMessages(messages)

	if len(parts) != 2 {
		t.Fatalf("len(parts) = %d, want 2", len(parts))
	}

	// Check first function call
	funcCall1, ok := parts[0].(genai.FunctionCall)
	if !ok {
		t.Fatalf("parts[0] is not genai.FunctionCall, got %T", parts[0])
	}
	if funcCall1.Name != "get_weather" {
		t.Errorf("parts[0].Name = %q, want %q", funcCall1.Name, "get_weather")
	}
	if funcCall1.Args["location"] != "Tokyo" {
		t.Errorf("parts[0].Args[\"location\"] = %v, want %q", funcCall1.Args["location"], "Tokyo")
	}

	// Check second function call
	funcCall2, ok := parts[1].(genai.FunctionCall)
	if !ok {
		t.Fatalf("parts[1] is not genai.FunctionCall, got %T", parts[1])
	}
	if funcCall2.Name != "search" {
		t.Errorf("parts[1].Name = %q, want %q", funcCall2.Name, "search")
	}
}

func TestConvertMessagesAssistantEmptyContent(t *testing.T) {
	p := &GeminiProvider{}

	// Assistant message with neither content nor tool calls should produce no parts
	messages := []Message{
		{Role: RoleAssistant, Content: ""},
	}

	parts := p.convertMessages(messages)
	if len(parts) != 0 {
		t.Errorf("len(parts) = %d, want 0 for empty assistant message", len(parts))
	}
}

func TestConvertResponse(t *testing.T) {
	p := &GeminiProvider{}

	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("Hello, world!"),
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
		UsageMetadata: &genai.UsageMetadata{
			PromptTokenCount:     10,
			CandidatesTokenCount: 5,
		},
	}

	result := p.convertResponse(resp)

	if result.Content != "Hello, world!" {
		t.Errorf("Content = %q, want %q", result.Content, "Hello, world!")
	}
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "end_turn")
	}
	if result.Usage.InputTokens != 10 {
		t.Errorf("Usage.InputTokens = %d, want %d", result.Usage.InputTokens, 10)
	}
	if result.Usage.OutputTokens != 5 {
		t.Errorf("Usage.OutputTokens = %d, want %d", result.Usage.OutputTokens, 5)
	}
}

func TestConvertResponseWithToolCalls(t *testing.T) {
	p := &GeminiProvider{}

	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.FunctionCall{
							Name: "get_weather",
							Args: map[string]any{"location": "Tokyo"},
						},
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}

	result := p.convertResponse(resp)

	if len(result.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(result.ToolCalls))
	}

	tc := result.ToolCalls[0]
	if tc.Name != "get_weather" {
		t.Errorf("ToolCall.Name = %q, want %q", tc.Name, "get_weather")
	}
	if tc.ID != "get_weather" {
		t.Errorf("ToolCall.ID = %q, want %q (Gemini uses name as ID)", tc.ID, "get_weather")
	}
	if tc.Arguments["location"] != "Tokyo" {
		t.Errorf("ToolCall.Arguments[\"location\"] = %v, want %q", tc.Arguments["location"], "Tokyo")
	}
	if result.StopReason != "tool_use" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "tool_use")
	}
}

func TestConvertResponseMaxTokens(t *testing.T) {
	p := &GeminiProvider{}

	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("Partial response..."),
					},
				},
				FinishReason: genai.FinishReasonMaxTokens,
			},
		},
	}

	result := p.convertResponse(resp)

	if result.StopReason != "max_tokens" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "max_tokens")
	}
}

func TestConvertResponseNilResponse(t *testing.T) {
	p := &GeminiProvider{}

	result := p.convertResponse(nil)

	if result.Content != "" {
		t.Errorf("Content = %q, want empty", result.Content)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("len(ToolCalls) = %d, want 0", len(result.ToolCalls))
	}
}

func TestConvertResponseEmptyCandidates(t *testing.T) {
	p := &GeminiProvider{}

	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{},
	}

	result := p.convertResponse(resp)

	if result.Content != "" {
		t.Errorf("Content = %q, want empty", result.Content)
	}
}

func TestConvertResponseNilContent(t *testing.T) {
	p := &GeminiProvider{}

	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content:      nil,
				FinishReason: genai.FinishReasonStop,
			},
		},
	}

	result := p.convertResponse(resp)

	if result.Content != "" {
		t.Errorf("Content = %q, want empty", result.Content)
	}
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "end_turn")
	}
}

func TestConvertResponseDefaultFinishReason(t *testing.T) {
	p := &GeminiProvider{}

	// Test with an unhandled finish reason (e.g., Safety, Other)
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("Response"),
					},
				},
				FinishReason: genai.FinishReasonSafety, // Not explicitly handled
			},
		},
	}

	result := p.convertResponse(resp)

	// Default case should still return "end_turn"
	if result.StopReason != "end_turn" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "end_turn")
	}
}

func TestConvertResponseNoUsageMetadata(t *testing.T) {
	p := &GeminiProvider{}

	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("Response"),
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
		UsageMetadata: nil, // No usage metadata
	}

	result := p.convertResponse(resp)

	// Should have zero usage when metadata is nil
	if result.Usage.InputTokens != 0 {
		t.Errorf("Usage.InputTokens = %d, want 0", result.Usage.InputTokens)
	}
	if result.Usage.OutputTokens != 0 {
		t.Errorf("Usage.OutputTokens = %d, want 0", result.Usage.OutputTokens)
	}
}

func TestConvertResponseMixedParts(t *testing.T) {
	p := &GeminiProvider{}

	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []genai.Part{
						genai.Text("Let me check the weather. "),
						genai.FunctionCall{
							Name: "get_weather",
							Args: map[string]any{"location": "Paris"},
						},
						genai.Text("Here's what I found."),
					},
				},
				FinishReason: genai.FinishReasonStop,
			},
		},
	}

	result := p.convertResponse(resp)

	// Content should be concatenated
	expectedContent := "Let me check the weather. Here's what I found."
	if result.Content != expectedContent {
		t.Errorf("Content = %q, want %q", result.Content, expectedContent)
	}

	// Should have one tool call
	if len(result.ToolCalls) != 1 {
		t.Fatalf("len(ToolCalls) = %d, want 1", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Name != "get_weather" {
		t.Errorf("ToolCall.Name = %q, want %q", result.ToolCalls[0].Name, "get_weather")
	}

	// Should indicate tool use since there are tool calls
	if result.StopReason != "tool_use" {
		t.Errorf("StopReason = %q, want %q", result.StopReason, "tool_use")
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
	defer func() { _ = provider.Close() }()

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
	defer func() { _ = provider.Close() }()

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
