// Package llmpb contains proto message types for the LLM service.
// This is a hand-written stub. Replace with protoc-generated code when available.
//
//go:generate protoc --go_out=. --go-grpc_out=. ../../proto/llm.proto
package llmpb

import (
	"context"

	"google.golang.org/grpc"
)

// Role indicates who authored a message.
type Role int32

const (
	Role_ROLE_UNSPECIFIED Role = 0
	Role_ROLE_USER        Role = 1
	Role_ROLE_ASSISTANT   Role = 2
	Role_ROLE_TOOL        Role = 3
)

// CompletionRequest contains the input for an inference request.
type CompletionRequest struct {
	SystemPrompt string
	Messages     []*Message
	Tools        []*ToolDef
	MaxTokens    int32
	JsonSchema   string
}

// CompletionResponse contains the model's output.
type CompletionResponse struct {
	Content    string
	ToolCalls  []*ToolCall
	StopReason string
	Usage      *Usage
}

// Message represents a single turn in the conversation.
type Message struct {
	Role       Role
	Content    string
	ToolCalls  []*ToolCall
	ToolResult *ToolResult
}

// ToolDef defines a tool available to the model.
type ToolDef struct {
	Name             string
	Description      string
	ParametersSchema string
}

// ToolCall represents a request from the model to invoke a tool.
type ToolCall struct {
	Id            string
	Name          string
	ArgumentsJson string
}

// ToolResult contains the output from a tool invocation.
type ToolResult struct {
	ToolCallId string
	Content    string
	IsError    bool
}

// Usage tracks token consumption.
type Usage struct {
	InputTokens  int32
	OutputTokens int32
}

// ShutdownRequest signals the server to terminate.
type ShutdownRequest struct {
	Graceful bool
}

// ShutdownResponse acknowledges shutdown initiation.
type ShutdownResponse struct {
	Accepted bool
}

// StatusRequest queries the server's current state.
type StatusRequest struct{}

// StatusResponse provides server health and model information.
type StatusResponse struct {
	Ready              bool
	ModelName          string
	ModelSizeBytes     int64
	Backend            string
	AvailableVramBytes int64
}

// InferenceServiceClient is the client API for InferenceService.
type InferenceServiceClient interface {
	Complete(ctx context.Context, in *CompletionRequest, opts ...grpc.CallOption) (*CompletionResponse, error)
	Shutdown(ctx context.Context, in *ShutdownRequest, opts ...grpc.CallOption) (*ShutdownResponse, error)
	GetStatus(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error)
}

type inferenceServiceClient struct {
	cc grpc.ClientConnInterface
}

// NewInferenceServiceClient creates a new InferenceService client.
func NewInferenceServiceClient(cc grpc.ClientConnInterface) InferenceServiceClient {
	return &inferenceServiceClient{cc}
}

func (c *inferenceServiceClient) Complete(ctx context.Context, in *CompletionRequest, opts ...grpc.CallOption) (*CompletionResponse, error) {
	out := new(CompletionResponse)
	err := c.cc.Invoke(ctx, "/tsuku.llm.v1.InferenceService/Complete", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *inferenceServiceClient) Shutdown(ctx context.Context, in *ShutdownRequest, opts ...grpc.CallOption) (*ShutdownResponse, error) {
	out := new(ShutdownResponse)
	err := c.cc.Invoke(ctx, "/tsuku.llm.v1.InferenceService/Shutdown", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *inferenceServiceClient) GetStatus(ctx context.Context, in *StatusRequest, opts ...grpc.CallOption) (*StatusResponse, error) {
	out := new(StatusResponse)
	err := c.cc.Invoke(ctx, "/tsuku.llm.v1.InferenceService/GetStatus", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
