package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/tsukumogami/tsuku/internal/llm/proto"
)

// LocalProvider implements the Provider interface using the local tsuku-llm addon.
// It communicates with the addon over gRPC via Unix domain sockets.
type LocalProvider struct {
	conn   *grpc.ClientConn
	client pb.InferenceServiceClient
}

// NewLocalProvider creates a new local provider by connecting to the tsuku-llm addon.
// The addon must be running and listening on the Unix socket at $TSUKU_HOME/llm.sock.
func NewLocalProvider(_ context.Context) (*LocalProvider, error) {
	socketPath := SocketPath()

	// Check if socket exists
	if _, err := os.Stat(socketPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("local LLM addon not running (socket not found: %s)", socketPath)
	}

	// Create gRPC client (connection is lazy)
	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to local LLM addon: %w", err)
	}

	client := pb.NewInferenceServiceClient(conn)

	return &LocalProvider{
		conn:   conn,
		client: client,
	}, nil
}

// Name returns the provider identifier.
func (p *LocalProvider) Name() string {
	return "local"
}

// Complete sends a completion request to the local addon.
func (p *LocalProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// Convert to proto format
	pbReq := toProtoRequest(req)

	pbResp, err := p.client.Complete(ctx, pbReq)
	if err != nil {
		return nil, fmt.Errorf("local LLM completion failed: %w", err)
	}

	return fromProtoResponse(pbResp), nil
}

// Close releases the gRPC connection.
func (p *LocalProvider) Close() error {
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Shutdown sends a shutdown request to the addon.
func (p *LocalProvider) Shutdown(ctx context.Context, graceful bool) error {
	_, err := p.client.Shutdown(ctx, &pb.ShutdownRequest{Graceful: graceful})
	return err
}

// GetStatus retrieves the addon's current status.
func (p *LocalProvider) GetStatus(ctx context.Context) (*pb.StatusResponse, error) {
	return p.client.GetStatus(ctx, &pb.StatusRequest{})
}

// SocketPath returns the path to the Unix domain socket.
func SocketPath() string {
	home := os.Getenv("TSUKU_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "/tmp/tsuku-llm.sock" // Fallback
		}
		home = filepath.Join(userHome, ".tsuku")
	}
	return filepath.Join(home, "llm.sock")
}

// IsAddonRunning checks if the addon is running by attempting to connect.
func IsAddonRunning() bool {
	socketPath := SocketPath()
	conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// toProtoRequest converts a CompletionRequest to proto format.
func toProtoRequest(req *CompletionRequest) *pb.CompletionRequest {
	pbReq := &pb.CompletionRequest{
		SystemPrompt: req.SystemPrompt,
		MaxTokens:    int32(req.MaxTokens),
	}

	// Convert messages
	for _, msg := range req.Messages {
		pbMsg := &pb.Message{
			Role:    toProtoRole(msg.Role),
			Content: msg.Content,
		}

		// Convert tool calls
		for _, tc := range msg.ToolCalls {
			argsJSON, err := json.Marshal(tc.Arguments)
			if err != nil {
				argsJSON = []byte("{}")
			}
			pbMsg.ToolCalls = append(pbMsg.ToolCalls, &pb.ToolCall{
				Id:            tc.ID,
				Name:          tc.Name,
				ArgumentsJson: string(argsJSON),
			})
		}

		// Convert tool result
		if msg.ToolResult != nil {
			pbMsg.ToolResult = &pb.ToolResult{
				ToolCallId: msg.ToolResult.CallID,
				Content:    msg.ToolResult.Content,
				IsError:    msg.ToolResult.IsError,
			}
		}

		pbReq.Messages = append(pbReq.Messages, pbMsg)
	}

	// Convert tools
	for _, tool := range req.Tools {
		paramsJSON, err := json.Marshal(tool.Parameters)
		if err != nil {
			paramsJSON = []byte("{}")
		}
		pbReq.Tools = append(pbReq.Tools, &pb.ToolDef{
			Name:             tool.Name,
			Description:      tool.Description,
			ParametersSchema: string(paramsJSON),
		})
	}

	return pbReq
}

// toProtoRole converts a Role to proto format.
func toProtoRole(role Role) pb.Role {
	switch role {
	case RoleUser:
		return pb.Role_ROLE_USER
	case RoleAssistant:
		return pb.Role_ROLE_ASSISTANT
	default:
		return pb.Role_ROLE_UNSPECIFIED
	}
}

// fromProtoResponse converts a proto response to CompletionResponse.
func fromProtoResponse(resp *pb.CompletionResponse) *CompletionResponse {
	result := &CompletionResponse{
		Content:    resp.Content,
		StopReason: resp.StopReason,
	}

	// Convert usage
	if resp.Usage != nil {
		result.Usage = Usage{
			InputTokens:  int(resp.Usage.InputTokens),
			OutputTokens: int(resp.Usage.OutputTokens),
		}
	}

	// Convert tool calls
	for _, tc := range resp.ToolCalls {
		args := make(map[string]any)
		if err := json.Unmarshal([]byte(tc.ArgumentsJson), &args); err != nil {
			args = make(map[string]any)
		}
		result.ToolCalls = append(result.ToolCalls, ToolCall{
			ID:        tc.Id,
			Name:      tc.Name,
			Arguments: args,
		})
	}

	return result
}
