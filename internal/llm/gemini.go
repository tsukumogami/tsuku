package llm

import (
	"context"
	"fmt"
	"os"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// GeminiModel is the Gemini model used for recipe generation.
const GeminiModel = "gemini-2.0-flash"

// GeminiProvider implements Provider using the Google AI API.
type GeminiProvider struct {
	client *genai.Client
	model  string
}

// NewGeminiProvider creates a provider using GOOGLE_API_KEY (or GEMINI_API_KEY).
// Returns an error if the API key is not set.
func NewGeminiProvider(ctx context.Context) (*GeminiProvider, error) {
	apiKey := os.Getenv("GOOGLE_API_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("GEMINI_API_KEY")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("GOOGLE_API_KEY (or GEMINI_API_KEY) environment variable not set")
	}

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create Gemini client: %w", err)
	}

	return &GeminiProvider{
		client: client,
		model:  GeminiModel,
	}, nil
}

// Name returns the provider identifier.
func (p *GeminiProvider) Name() string {
	return "gemini"
}

// Complete sends messages to Gemini and returns a single response.
func (p *GeminiProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	model := p.client.GenerativeModel(p.model)

	// Set system instruction
	if req.SystemPrompt != "" {
		model.SystemInstruction = &genai.Content{
			Parts: []genai.Part{genai.Text(req.SystemPrompt)},
		}
	}

	// Set max tokens
	if req.MaxTokens > 0 {
		model.MaxOutputTokens = int32Ptr(int32(req.MaxTokens))
	}

	// Convert tools to Gemini function declarations
	if len(req.Tools) > 0 {
		model.Tools = []*genai.Tool{
			{FunctionDeclarations: p.convertTools(req.Tools)},
		}
	}

	// Convert messages to Gemini content
	contents := p.convertMessages(req.Messages)

	// Make API call
	resp, err := model.GenerateContent(ctx, contents...)
	if err != nil {
		return nil, fmt.Errorf("gemini API call failed: %w", err)
	}

	return p.convertResponse(resp), nil
}

// Close releases the Gemini client resources.
func (p *GeminiProvider) Close() error {
	return p.client.Close()
}

// convertTools converts ToolDef slice to Gemini FunctionDeclaration slice.
func (p *GeminiProvider) convertTools(tools []ToolDef) []*genai.FunctionDeclaration {
	declarations := make([]*genai.FunctionDeclaration, len(tools))
	for i, tool := range tools {
		declarations[i] = &genai.FunctionDeclaration{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  convertSchemaToGemini(tool.Parameters),
		}
	}
	return declarations
}

// convertSchemaToGemini converts a JSON Schema map to Gemini Schema.
func convertSchemaToGemini(params map[string]any) *genai.Schema {
	if params == nil {
		return nil
	}

	schema := &genai.Schema{}

	// Get type
	if t, ok := params["type"].(string); ok {
		switch t {
		case "object":
			schema.Type = genai.TypeObject
		case "array":
			schema.Type = genai.TypeArray
		case "string":
			schema.Type = genai.TypeString
		case "number":
			schema.Type = genai.TypeNumber
		case "integer":
			schema.Type = genai.TypeInteger
		case "boolean":
			schema.Type = genai.TypeBoolean
		}
	}

	// Get description
	if desc, ok := params["description"].(string); ok {
		schema.Description = desc
	}

	// Get properties (for objects)
	if props, ok := params["properties"].(map[string]interface{}); ok {
		schema.Properties = make(map[string]*genai.Schema)
		for name, prop := range props {
			if propMap, ok := prop.(map[string]interface{}); ok {
				schema.Properties[name] = convertSchemaToGemini(propMap)
			}
		}
	}

	// Get required fields
	if required, ok := params["required"].([]string); ok {
		schema.Required = required
	} else if required, ok := params["required"].([]interface{}); ok {
		schema.Required = make([]string, len(required))
		for i, r := range required {
			if s, ok := r.(string); ok {
				schema.Required[i] = s
			}
		}
	}

	// Get items (for arrays)
	if items, ok := params["items"].(map[string]interface{}); ok {
		schema.Items = convertSchemaToGemini(items)
	}

	return schema
}

// convertMessages converts Message slice to Gemini Content slice.
func (p *GeminiProvider) convertMessages(messages []Message) []genai.Part {
	var parts []genai.Part

	for _, msg := range messages {
		switch msg.Role {
		case RoleUser:
			if msg.ToolResult != nil {
				// Tool result message
				parts = append(parts, genai.FunctionResponse{
					Name:     msg.ToolResult.CallID,
					Response: map[string]any{"result": msg.ToolResult.Content},
				})
			} else {
				// Regular user message
				parts = append(parts, genai.Text(msg.Content))
			}
		case RoleAssistant:
			if len(msg.ToolCalls) > 0 {
				// Assistant made tool calls
				for _, tc := range msg.ToolCalls {
					parts = append(parts, genai.FunctionCall{
						Name: tc.Name,
						Args: tc.Arguments,
					})
				}
			} else if msg.Content != "" {
				parts = append(parts, genai.Text(msg.Content))
			}
		}
	}

	return parts
}

// convertResponse converts Gemini response to CompletionResponse.
func (p *GeminiProvider) convertResponse(resp *genai.GenerateContentResponse) *CompletionResponse {
	result := &CompletionResponse{}

	if resp == nil || len(resp.Candidates) == 0 {
		return result
	}

	candidate := resp.Candidates[0]

	// Extract content and tool calls from parts
	if candidate.Content != nil {
		for _, part := range candidate.Content.Parts {
			switch v := part.(type) {
			case genai.Text:
				result.Content += string(v)
			case genai.FunctionCall:
				result.ToolCalls = append(result.ToolCalls, ToolCall{
					ID:        v.Name, // Gemini uses function name as ID
					Name:      v.Name,
					Arguments: v.Args,
				})
			}
		}
	}

	// Set stop reason
	switch candidate.FinishReason {
	case genai.FinishReasonStop:
		if len(result.ToolCalls) > 0 {
			result.StopReason = "tool_use"
		} else {
			result.StopReason = "end_turn"
		}
	case genai.FinishReasonMaxTokens:
		result.StopReason = "max_tokens"
	default:
		result.StopReason = "end_turn"
	}

	// Extract token usage
	if resp.UsageMetadata != nil {
		result.Usage = Usage{
			InputTokens:  int(resp.UsageMetadata.PromptTokenCount),
			OutputTokens: int(resp.UsageMetadata.CandidatesTokenCount),
		}
	}

	return result
}

// int32Ptr returns a pointer to the given int32 value.
func int32Ptr(v int32) *int32 {
	return &v
}
