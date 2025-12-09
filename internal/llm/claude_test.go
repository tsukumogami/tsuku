package llm

import (
	"os"
	"testing"
)

func TestNewClaudeProvider_NoAPIKey(t *testing.T) {
	cleanup := setTestAPIKey(t, "")
	defer cleanup()

	_, err := NewClaudeProvider()
	if err == nil {
		t.Error("expected error when ANTHROPIC_API_KEY is not set")
	}
}

func TestNewClaudeProvider_WithAPIKey(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	provider, err := NewClaudeProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if provider == nil {
		t.Error("expected non-nil provider")
	}
}

func TestClaudeProvider_Name(t *testing.T) {
	cleanup := setTestAPIKey(t, "test-key")
	defer cleanup()

	provider, err := NewClaudeProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := provider.Name(); got != "claude" {
		t.Errorf("Name() = %q, want %q", got, "claude")
	}
}

func TestToAnthropicMessages_UserMessage(t *testing.T) {
	msgs := []Message{
		{Role: RoleUser, Content: "Hello"},
	}

	result := toAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
}

func TestToAnthropicMessages_AssistantMessage(t *testing.T) {
	msgs := []Message{
		{Role: RoleAssistant, Content: "Hello back"},
	}

	result := toAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
}

func TestToAnthropicMessages_ToolCall(t *testing.T) {
	msgs := []Message{
		{
			Role:    RoleAssistant,
			Content: "",
			ToolCalls: []ToolCall{
				{
					ID:   "call-123",
					Name: "fetch_file",
					Arguments: map[string]any{
						"path": "README.md",
					},
				},
			},
		},
	}

	result := toAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
}

func TestToAnthropicMessages_ToolResult(t *testing.T) {
	msgs := []Message{
		{
			Role: RoleUser,
			ToolResult: &ToolResult{
				CallID:  "call-123",
				Content: "File contents here",
				IsError: false,
			},
		},
	}

	result := toAnthropicMessages(msgs)
	if len(result) != 1 {
		t.Fatalf("expected 1 message, got %d", len(result))
	}
}

func TestToAnthropicTools(t *testing.T) {
	tools := []ToolDef{
		{
			Name:        "test_tool",
			Description: "A test tool",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"arg1": map[string]any{
						"type":        "string",
						"description": "First argument",
					},
				},
				"required": []string{"arg1"},
			},
		},
	}

	result := toAnthropicTools(tools)
	if len(result) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(result))
	}
}

func TestToAnthropicTools_Empty(t *testing.T) {
	result := toAnthropicTools(nil)
	if result != nil {
		t.Errorf("expected nil for empty tools, got %v", result)
	}
}

func TestBuildToolDefs(t *testing.T) {
	tools := buildToolDefs()

	// Should have 3 tools: fetch_file, inspect_archive, extract_pattern
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}

	expectedNames := []string{ToolFetchFile, ToolInspectArchive, ToolExtractPattern}
	for i, expected := range expectedNames {
		if tools[i].Name != expected {
			t.Errorf("tool[%d].Name = %q, want %q", i, tools[i].Name, expected)
		}
		if tools[i].Description == "" {
			t.Errorf("tool[%d].Description is empty", i)
		}
		if tools[i].Parameters == nil {
			t.Errorf("tool[%d].Parameters is nil", i)
		}
	}
}

// TestClaudeProvider_Complete_Integration is skipped unless ANTHROPIC_API_KEY is set.
// This test makes real API calls and incurs costs.
func TestClaudeProvider_Complete_Integration(t *testing.T) {
	if os.Getenv("ANTHROPIC_API_KEY") == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping integration test")
	}

	provider, err := NewClaudeProvider()
	if err != nil {
		t.Fatalf("NewClaudeProvider error: %v", err)
	}

	req := &CompletionRequest{
		SystemPrompt: "You are a helpful assistant. Respond briefly.",
		Messages: []Message{
			{Role: RoleUser, Content: "Say hello"},
		},
		MaxTokens: 100,
	}

	resp, err := provider.Complete(t.Context(), req)
	if err != nil {
		t.Fatalf("Complete error: %v", err)
	}

	if resp == nil {
		t.Fatal("expected non-nil response")
	}

	if resp.Content == "" {
		t.Error("expected non-empty content")
	}

	if resp.Usage.InputTokens == 0 {
		t.Error("expected non-zero input tokens")
	}

	if resp.Usage.OutputTokens == 0 {
		t.Error("expected non-zero output tokens")
	}

	t.Logf("Response: %s", resp.Content)
	t.Logf("Usage: %s", resp.Usage.String())
}
