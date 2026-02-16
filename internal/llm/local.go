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
	"github.com/tsukumogami/tsuku/internal/progress"
)

// LocalProvider implements the Provider interface using the local tsuku-llm addon.
// It communicates with the addon over gRPC via Unix domain sockets.
type LocalProvider struct {
	addonManager *addon.AddonManager
	lifecycle    *ServerLifecycle
	conn         *grpc.ClientConn
	client       pb.InferenceServiceClient

	// prompter handles user confirmation before downloads.
	// If nil, downloads proceed without prompting.
	prompter addon.Prompter

	// modelPrompted tracks whether we've already prompted for the model download
	// in this session, to avoid re-prompting on each Complete call.
	modelPrompted bool
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

// SetPrompter sets the prompter for download confirmations.
// The prompter is propagated to the AddonManager for addon downloads
// and used directly for model download prompts.
func (p *LocalProvider) SetPrompter(prompter addon.Prompter) {
	p.prompter = prompter
	p.addonManager.SetPrompter(prompter)
}

// Name returns the provider identifier.
func (p *LocalProvider) Name() string {
	return "local"
}

// Complete sends a completion request to the local addon.
// It ensures the addon is downloaded, verified, and running before sending the request.
// Shows a spinner during inference when running in a terminal.
func (p *LocalProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// Ensure addon is downloaded and verified
	if _, err := p.addonManager.EnsureAddon(ctx); err != nil {
		if err == addon.ErrDownloadDeclined {
			return nil, fmt.Errorf("addon download declined: configure a cloud provider (ANTHROPIC_API_KEY or GOOGLE_API_KEY) as an alternative")
		}
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

	// Check model status and prompt for model download if needed
	if err := p.ensureModelReady(ctx); err != nil {
		if err == addon.ErrDownloadDeclined {
			return nil, fmt.Errorf("model download declined: configure a cloud provider (ANTHROPIC_API_KEY or GOOGLE_API_KEY) as an alternative")
		}
		return nil, err
	}

	// Convert to proto format
	pbReq := toProtoRequest(req)

	// Show spinner during inference
	spinner := progress.NewSpinner(os.Stderr)
	spinner.Start("Generating...")
	defer spinner.Stop()

	pbResp, err := p.client.Complete(ctx, pbReq)
	if err != nil {
		spinner.StopWithMessage("Generation failed.")
		return nil, fmt.Errorf("local LLM completion failed: %w", err)
	}

	spinner.Stop()
	return fromProtoResponse(pbResp), nil
}

// ensureModelReady checks if the model is downloaded and prompts the user if needed.
// The model is managed by the addon, but we prompt from the Go side for UX consistency.
func (p *LocalProvider) ensureModelReady(ctx context.Context) error {
	if p.modelPrompted {
		return nil // Already prompted this session
	}

	status, err := p.client.GetStatus(ctx, &pb.StatusRequest{})
	if err != nil {
		// Can't get status -- the addon may still be loading. Let it proceed;
		// the addon handles model download internally if needed.
		return nil
	}

	// If model is already loaded, no prompt needed
	if status.Ready && status.ModelName != "" {
		p.modelPrompted = true
		return nil
	}

	// Model not ready -- it may need to be downloaded.
	// Only prompt if we have a prompter and the model size is known.
	if p.prompter != nil && status.ModelSizeBytes > 0 && status.ModelName != "" {
		modelDesc := fmt.Sprintf("LLM model %s", status.ModelName)
		approved, err := p.prompter.ConfirmDownload(ctx, modelDesc, status.ModelSizeBytes)
		if err != nil {
			return fmt.Errorf("model download prompt failed: %w", err)
		}
		if !approved {
			return addon.ErrDownloadDeclined
		}
	}

	p.modelPrompted = true
	return nil
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
