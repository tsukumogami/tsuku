package llm

import "context"

// Provider defines the interface for single-turn LLM completion.
// Multi-turn conversation loops live in the builder layer, not here.
// Each provider implementation (Claude, Gemini) converts between these
// common types and their SDK-specific formats.
type Provider interface {
	// Name returns the provider identifier (e.g., "claude", "gemini").
	Name() string

	// Complete sends messages to the LLM and returns a single response.
	// Tool calls in the response must be handled by the caller.
	// The provider is stateless - callers manage conversation history.
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
}

// CompletionRequest contains input for a single LLM turn.
type CompletionRequest struct {
	// SystemPrompt provides context and instructions for the LLM.
	SystemPrompt string

	// Messages contains the conversation history.
	// Must include at least one user message.
	Messages []Message

	// Tools defines the functions the LLM can call.
	// Providers convert these to native formats (Claude tool_use, Gemini functionCall).
	Tools []ToolDef

	// MaxTokens limits the response length.
	// If zero, providers use their default limits.
	MaxTokens int
}

// CompletionResponse contains the LLM's response for a single turn.
type CompletionResponse struct {
	// Content is the text response from the LLM.
	// May be empty if the response only contains tool calls.
	Content string

	// ToolCalls contains any tools the LLM wants to invoke.
	// Empty if the LLM responded with text only.
	ToolCalls []ToolCall

	// StopReason indicates why the LLM stopped generating.
	// Common values: "end_turn", "tool_use", "max_tokens".
	StopReason string

	// Usage tracks token consumption for this turn.
	Usage Usage
}

// Message represents a single message in a conversation.
type Message struct {
	// Role identifies the message sender.
	Role Role

	// Content is the text content of the message.
	// For assistant messages with tool calls, this may be empty.
	Content string

	// ToolCalls contains tools the assistant wants to invoke.
	// Only present in assistant messages.
	ToolCalls []ToolCall

	// ToolResult contains the result of a tool execution.
	// Only present in user messages responding to tool calls.
	ToolResult *ToolResult
}

// Role identifies the sender of a message in a conversation.
type Role string

const (
	// RoleUser indicates a message from the user or application.
	RoleUser Role = "user"

	// RoleAssistant indicates a message from the LLM.
	RoleAssistant Role = "assistant"
)

// ToolCall represents an LLM's request to invoke a tool.
type ToolCall struct {
	// ID uniquely identifies this tool call for correlation with results.
	// Claude and Gemini both provide call IDs in their responses.
	ID string

	// Name is the tool to invoke, matching a ToolDef.Name.
	Name string

	// Arguments contains the parsed arguments for the tool.
	// The structure matches the JSON Schema defined in ToolDef.Parameters.
	Arguments map[string]any
}

// ToolResult contains the output from executing a tool.
type ToolResult struct {
	// CallID correlates this result to a ToolCall.ID.
	CallID string

	// Content is the tool's output, typically as text or JSON.
	Content string

	// IsError indicates whether the tool execution failed.
	// Providers may format error results differently.
	IsError bool
}

// ToolDef defines a tool that the LLM can call.
// Providers convert these to their native format:
// - Claude: ToolParam with tool_use
// - Gemini: FunctionDeclaration with functionCall
type ToolDef struct {
	// Name identifies the tool. Must be unique within a request.
	Name string

	// Description explains what the tool does.
	// Good descriptions help the LLM use tools correctly.
	Description string

	// Parameters defines the tool's input using JSON Schema.
	// Use map[string]any to represent arbitrary JSON Schema objects.
	Parameters map[string]any
}
