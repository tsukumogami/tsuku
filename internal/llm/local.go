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

	"github.com/tsukumogami/tsuku/internal/llm/addon"
	pb "github.com/tsukumogami/tsuku/internal/llm/proto"
)

// LocalProvider implements the Provider interface using the local tsuku-llm addon.
// It communicates with the addon over gRPC via Unix domain sockets.
type LocalProvider struct {
	addonManager *addon.AddonManager
	lifecycle    *ServerLifecycle
	conn         *grpc.ClientConn
	client       pb.InferenceServiceClient
}

// NewLocalProvider creates a new local provider with default idle timeout.
// The provider uses ServerLifecycle to ensure the addon is running before making requests.
func NewLocalProvider() *LocalProvider {
	return NewLocalProviderWithTimeout(DefaultIdleTimeout)
}

// NewLocalProviderWithTimeout creates a new local provider with the specified idle timeout.
// The idle timeout is passed to the addon server when starting it.
func NewLocalProviderWithTimeout(idleTimeout time.Duration) *LocalProvider {
	socketPath := SocketPath()
	addonManager := addon.NewAddonManager()
	lifecycle := NewServerLifecycleWithManager(socketPath, addonManager)
	lifecycle.SetIdleTimeout(idleTimeout)

	return &LocalProvider{
		addonManager: addonManager,
		lifecycle:    lifecycle,
	}
}

// Name returns the provider identifier.
func (p *LocalProvider) Name() string {
	return "local"
}

// Complete sends a completion request to the local addon.
// It ensures the addon is downloaded, verified, and running before sending the request.
func (p *LocalProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// Ensure addon is downloaded and verified
	if _, err := p.addonManager.EnsureAddon(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure addon: %w", err)
	}

	// Ensure the addon server is running (includes pre-execution verification)
	if err := p.lifecycle.EnsureRunning(ctx); err != nil {
		return nil, fmt.Errorf("local LLM addon not available: %w", err)
	}

	// Ensure we have a connection
	if err := p.ensureConnection(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to local LLM addon: %w", err)
	}

	// Convert and send request
	return p.sendRequest(ctx, req)
}

// sendRequest converts the request to proto format, sends it over gRPC,
// and invalidates the cached connection on error so subsequent calls
// trigger reconnection via ensureConnection.
func (p *LocalProvider) sendRequest(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	pbReq := toProtoRequest(req)

	pbResp, err := p.client.Complete(ctx, pbReq)
	if err != nil {
		// Invalidate the cached connection so subsequent calls trigger
		// reconnection via ensureConnection instead of reusing a dead connection.
		p.client = nil
		if p.conn != nil {
			_ = p.conn.Close()
			p.conn = nil
		}
		return nil, fmt.Errorf("local LLM completion failed: %w", err)
	}

	return fromProtoResponse(pbResp), nil
}

// ensureConnection establishes the gRPC connection if not already connected.
func (p *LocalProvider) ensureConnection(ctx context.Context) error {
	if p.client != nil {
		return nil
	}

	socketPath := SocketPath()

	conn, err := grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}

	p.conn = conn
	p.client = pb.NewInferenceServiceClient(conn)
	return nil
}

// Close releases the gRPC connection and stops the server if we started it.
func (p *LocalProvider) Close() error {
	if p.conn != nil {
		err := p.conn.Close()
		p.conn = nil
		p.client = nil
		return err
	}
	return nil
}

// Shutdown sends a shutdown request to the addon.
func (p *LocalProvider) Shutdown(ctx context.Context, graceful bool) error {
	if p.client == nil {
		return nil
	}
	_, err := p.client.Shutdown(ctx, &pb.ShutdownRequest{Graceful: graceful})
	return err
}

// GetStatus retrieves the addon's current status.
func (p *LocalProvider) GetStatus(ctx context.Context) (*pb.StatusResponse, error) {
	if err := p.ensureConnection(ctx); err != nil {
		return nil, err
	}
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

// LockPath returns the path to the lock file for daemon state detection.
func LockPath() string {
	return SocketPath() + ".lock"
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
