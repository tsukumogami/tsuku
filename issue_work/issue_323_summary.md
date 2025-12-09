# Issue 323 Summary

## Changes Made

### New Files
- `internal/llm/provider.go` - Provider interface and types

### Types Defined

| Type | Purpose |
|------|---------|
| `Provider` | Interface with `Name()` and `Complete()` methods |
| `CompletionRequest` | Input for single LLM turn |
| `CompletionResponse` | Output from single LLM turn |
| `Message` | Conversation message with role, content, tool calls |
| `Role` | Enum for user/assistant |
| `ToolCall` | LLM request to invoke a tool |
| `ToolResult` | Output from tool execution |
| `ToolDef` | Tool definition with JSON Schema parameters |

### Design Decisions

1. **Single-turn interface**: Providers handle one request/response. Multi-turn loops belong in the builder layer.

2. **Reused `Usage` type**: The existing `Usage` type from `cost.go` already has `InputTokens` and `OutputTokens`, so no changes needed there.

3. **Generic `map[string]any` for arguments/parameters**: Allows flexibility for different tool schemas without strong typing.

## Verification

- Build: Pass
- Tests: 19/19 packages pass
- gofmt: Clean
- go vet: Clean

## Acceptance Criteria

- [x] `Provider` interface defined with `Name()` and `Complete()` methods
- [x] `CompletionRequest` and `CompletionResponse` types defined
- [x] `Message`, `ToolCall`, `ToolResult`, `ToolDef` types defined
- [x] `Usage` type includes input/output token counts (already exists in cost.go)
- [x] All types have documentation comments

## Ready for PR
