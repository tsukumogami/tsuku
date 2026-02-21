package llm

import (
	"context"
	"encoding/json"
	"errors"
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

// localMaxTokens caps the MaxTokens for local inference to keep latency
// reasonable on CPU (~15 tok/s).
const localMaxTokens = 2048

// LocalProvider implements the Provider interface using the local tsuku-llm addon.
// It communicates with the addon over gRPC via Unix domain sockets.
type LocalProvider struct {
	addonManager *addon.AddonManager
	lifecycle    *ServerLifecycle
	conn         *grpc.ClientConn
	client       pb.InferenceServiceClient

	// prompter handles user confirmation before downloads.
	prompter addon.Prompter

	// modelPrompted tracks whether we already prompted for model download
	// this session, to avoid re-prompting on subsequent Complete calls.
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
	// Create addon manager without an installer -- the LocalProvider is used
	// as a fallback LLM provider and does not drive installation itself.
	// The caller (typically the install command) handles installation.
	addonManager := addon.NewAddonManager("", nil, "")
	lifecycle := NewServerLifecycle(socketPath, "")
	lifecycle.SetIdleTimeout(idleTimeout)

	return &LocalProvider{
		addonManager: addonManager,
		lifecycle:    lifecycle,
	}
}

// NewLocalProviderWithInstaller creates a local provider wired with an Installer
// so it can install the addon via the recipe system when needed.
func NewLocalProviderWithInstaller(installer addon.Installer, backendOverride string, idleTimeout time.Duration) *LocalProvider {
	socketPath := SocketPath()
	addonManager := addon.NewAddonManager("", installer, backendOverride)
	lifecycle := NewServerLifecycle(socketPath, "")
	lifecycle.SetIdleTimeout(idleTimeout)

	return &LocalProvider{
		addonManager: addonManager,
		lifecycle:    lifecycle,
	}
}

// SetPrompter sets the prompter used for download confirmation.
// It also configures the prompter on the addon manager.
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
// If the user declines a download prompt, it returns a helpful error about cloud providers.
func (p *LocalProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// Ensure addon is installed via recipe system
	addonPath, err := p.addonManager.EnsureAddon(ctx)
	if err != nil {
		if errors.Is(err, addon.ErrDownloadDeclined) {
			return nil, fmt.Errorf("local LLM addon download declined; configure ANTHROPIC_API_KEY or GOOGLE_API_KEY for cloud inference instead")
		}
		return nil, fmt.Errorf("failed to ensure addon: %w", err)
	}

	// Update lifecycle with the resolved binary path
	p.lifecycle.addonPath = addonPath

	// Ensure the addon server is running
	if err := p.lifecycle.EnsureRunning(ctx); err != nil {
		return nil, fmt.Errorf("local LLM addon not available: %w", err)
	}

	// Ensure we have a connection
	if err := p.ensureConnection(ctx); err != nil {
		return nil, fmt.Errorf("failed to connect to local LLM addon: %w", err)
	}

	// Check model readiness and prompt for model download if needed
	if err := p.ensureModelReady(ctx); err != nil {
		if errors.Is(err, addon.ErrDownloadDeclined) {
			return nil, fmt.Errorf("model download declined; configure ANTHROPIC_API_KEY or GOOGLE_API_KEY for cloud inference instead")
		}
		return nil, err
	}

	// Cap MaxTokens for local inference to keep latency reasonable
	if req.MaxTokens > localMaxTokens || req.MaxTokens == 0 {
		req.MaxTokens = localMaxTokens
	}

	// Show spinner during inference
	spinner := progress.NewSpinner(os.Stderr)
	spinner.Start("Generating...")

	resp, err := p.sendRequest(ctx, req)
	if err != nil {
		spinner.StopWithMessage("Generation failed.")
		return nil, err
	}

	spinner.Stop()
	return resp, nil
}

// invalidateConnection closes and nils the cached gRPC connection and client
// so that subsequent calls trigger reconnection via ensureConnection instead
// of reusing a dead connection.
func (p *LocalProvider) invalidateConnection() {
	p.client = nil
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}
}

// sendRequest converts the request to proto format, sends it over gRPC,
// and invalidates the cached connection on error so subsequent calls
// trigger reconnection via ensureConnection.
func (p *LocalProvider) sendRequest(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	pbReq := toProtoRequest(req)

	pbResp, err := p.client.Complete(ctx, pbReq)
	if err != nil {
		p.invalidateConnection()
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

// ensureModelReady checks if the addon has a model loaded and prompts for
// download if necessary. The addon server reports model status via GetStatus.
// If the model is not yet downloaded and a prompter is configured, the user
// is asked to confirm the download.
func (p *LocalProvider) ensureModelReady(ctx context.Context) error {
	status, err := p.client.GetStatus(ctx, &pb.StatusRequest{})
	if err != nil {
		// If GetStatus fails, the server may still be starting up.
		// Don't block inference on this -- let the Complete RPC handle it.
		return nil
	}

	// If the server reports ready with a model loaded, nothing to do
	if status.Ready && status.ModelName != "" {
		return nil
	}

	// Model not yet loaded -- prompt for download if we haven't already
	if p.prompter != nil && !p.modelPrompted {
		p.modelPrompted = true

		modelSize := status.ModelSizeBytes
		description := "LLM model"
		if status.ModelName != "" {
			description = fmt.Sprintf("LLM model (%s)", status.ModelName)
		}

		ok, err := p.prompter.ConfirmDownload(ctx, description, modelSize)
		if err != nil {
			return err
		}
		if !ok {
			return addon.ErrDownloadDeclined
		}
	}

	return nil
}

// TriggerModelDownload sends a lightweight Complete request to the addon server
// to trigger model download and loading. The addon downloads the model on first
// inference call, so a trivial request forces this to happen. After the call
// completes, the model is downloaded, loaded, and verified.
func (p *LocalProvider) TriggerModelDownload(ctx context.Context) error {
	if err := p.ensureConnection(ctx); err != nil {
		return fmt.Errorf("failed to connect to addon: %w", err)
	}

	// Send a minimal completion request. The addon will download and load
	// the model before responding. This is the only way to trigger model
	// download since the gRPC API has no dedicated DownloadModel RPC.
	req := &pb.CompletionRequest{
		SystemPrompt: "Respond with OK.",
		Messages: []*pb.Message{
			{Role: pb.Role_ROLE_USER, Content: "OK"},
		},
		MaxTokens: 4,
	}

	_, err := p.client.Complete(ctx, req)
	if err != nil {
		p.invalidateConnection()
		return fmt.Errorf("model download failed: %w", err)
	}
	return nil
}

// Shutdown sends a shutdown request to the addon.
func (p *LocalProvider) Shutdown(ctx context.Context, graceful bool) error {
	if p.client == nil {
		return nil
	}
	_, err := p.client.Shutdown(ctx, &pb.ShutdownRequest{Graceful: graceful})
	if err != nil {
		p.invalidateConnection()
		return err
	}
	return nil
}

// GetStatus retrieves the addon's current status.
func (p *LocalProvider) GetStatus(ctx context.Context) (*pb.StatusResponse, error) {
	if err := p.ensureConnection(ctx); err != nil {
		return nil, err
	}
	resp, err := p.client.GetStatus(ctx, &pb.StatusRequest{})
	if err != nil {
		p.invalidateConnection()
		return nil, err
	}
	return resp, nil
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
