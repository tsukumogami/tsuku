package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
)

// ClaudeProvider implements the Provider interface for Claude/Anthropic models.
type ClaudeProvider struct {
	client anthropic.Client
	model  anthropic.Model
}

// NewClaudeProvider creates a new Claude provider using ANTHROPIC_API_KEY from environment.
// Returns an error if the API key is not set.
func NewClaudeProvider() (*ClaudeProvider, error) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY environment variable not set")
	}

	return &ClaudeProvider{
		client: anthropic.NewClient(option.WithAPIKey(apiKey)),
		model:  anthropic.Model(Model),
	}, nil
}

// Name returns the provider identifier.
func (p *ClaudeProvider) Name() string {
	return "claude"
}

// Complete sends messages to Claude and returns a single response.
// Tool calls in the response must be handled by the caller.
func (p *ClaudeProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// Convert common types to Anthropic format
	anthropicMessages := toAnthropicMessages(req.Messages)
	anthropicTools := toAnthropicTools(req.Tools)

	maxTokens := int64(req.MaxTokens)
	if maxTokens == 0 {
		maxTokens = 4096 // Default max tokens
	}

	params := anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: maxTokens,
		Messages:  anthropicMessages,
	}

	if req.SystemPrompt != "" {
		params.System = []anthropic.TextBlockParam{
			{Text: req.SystemPrompt},
		}
	}

	if len(anthropicTools) > 0 {
		params.Tools = anthropicTools
	}

	resp, err := p.client.Messages.New(ctx, params)
	if err != nil {
		return nil, fmt.Errorf("anthropic API call failed: %w", err)
	}

	return fromAnthropicResponse(resp), nil
}

// toAnthropicMessages converts common Messages to Anthropic format.
func toAnthropicMessages(msgs []Message) []anthropic.MessageParam {
	result := make([]anthropic.MessageParam, 0, len(msgs))

	for _, msg := range msgs {
		switch msg.Role {
		case RoleUser:
			if msg.ToolResult != nil {
				// User message with tool result
				result = append(result, anthropic.NewUserMessage(
					anthropic.NewToolResultBlock(
						msg.ToolResult.CallID,
						msg.ToolResult.Content,
						msg.ToolResult.IsError,
					),
				))
			} else {
				// Regular user message
				result = append(result, anthropic.NewUserMessage(
					anthropic.NewTextBlock(msg.Content),
				))
			}
		case RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				// Assistant message with tool calls
				blocks := make([]anthropic.ContentBlockParamUnion, 0, len(msg.ToolCalls)+1)
				if msg.Content != "" {
					blocks = append(blocks, anthropic.NewTextBlock(msg.Content))
				}
				for _, tc := range msg.ToolCalls {
					// Input field expects a map/dictionary, not JSON bytes
					blocks = append(blocks, anthropic.ContentBlockParamUnion{
						OfToolUse: &anthropic.ToolUseBlockParam{
							ID:    tc.ID,
							Name:  tc.Name,
							Input: tc.Arguments,
						},
					})
				}
				result = append(result, anthropic.NewAssistantMessage(blocks...))
			} else {
				// Regular assistant message
				result = append(result, anthropic.NewAssistantMessage(
					anthropic.NewTextBlock(msg.Content),
				))
			}
		}
	}

	return result
}

// toAnthropicTools converts common ToolDefs to Anthropic format.
func toAnthropicTools(tools []ToolDef) []anthropic.ToolUnionParam {
	if len(tools) == 0 {
		return nil
	}

	result := make([]anthropic.ToolUnionParam, 0, len(tools))
	for _, tool := range tools {
		// Extract required fields from Parameters if present
		var required []string
		if reqVal, ok := tool.Parameters["required"]; ok {
			if reqSlice, ok := reqVal.([]string); ok {
				required = reqSlice
			} else if reqSlice, ok := reqVal.([]any); ok {
				for _, r := range reqSlice {
					if s, ok := r.(string); ok {
						required = append(required, s)
					}
				}
			}
		}

		// Extract properties from Parameters
		properties := tool.Parameters
		if props, ok := tool.Parameters["properties"].(map[string]any); ok {
			properties = props
		}

		result = append(result, anthropic.ToolUnionParam{
			OfTool: &anthropic.ToolParam{
				Name:        tool.Name,
				Description: anthropic.String(tool.Description),
				InputSchema: anthropic.ToolInputSchemaParam{
					Type:       "object",
					Properties: properties,
					Required:   required,
				},
			},
		})
	}

	return result
}

// fromAnthropicResponse converts an Anthropic response to common format.
func fromAnthropicResponse(resp *anthropic.Message) *CompletionResponse {
	result := &CompletionResponse{
		StopReason: string(resp.StopReason),
		Usage: Usage{
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
		},
	}

	for _, block := range resp.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			result.Content += variant.Text
		case anthropic.ToolUseBlock:
			// Parse arguments from JSON
			var args map[string]any
			_ = json.Unmarshal(variant.Input, &args)

			result.ToolCalls = append(result.ToolCalls, ToolCall{
				ID:        variant.ID,
				Name:      variant.Name,
				Arguments: args,
			})
		}
	}

	return result
}
