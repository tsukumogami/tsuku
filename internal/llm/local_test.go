package llm

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"github.com/tsukumogami/tsuku/internal/llm/addon"
	pb "github.com/tsukumogami/tsuku/internal/llm/proto"
)

func TestSocketPath(t *testing.T) {
	t.Run("uses TSUKU_HOME when set", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		path := SocketPath()
		require.Equal(t, filepath.Join(tmpDir, "llm.sock"), path)
	})

	t.Run("defaults to ~/.tsuku when TSUKU_HOME not set", func(t *testing.T) {
		t.Setenv("TSUKU_HOME", "")

		homeDir, err := os.UserHomeDir()
		require.NoError(t, err)

		path := SocketPath()
		require.Equal(t, filepath.Join(homeDir, ".tsuku", "llm.sock"), path)
	})
}

func TestLockPath(t *testing.T) {
	t.Run("uses socket path with .lock suffix", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		path := LockPath()
		require.Equal(t, filepath.Join(tmpDir, "llm.sock.lock"), path)
	})
}

func TestIsAddonRunning(t *testing.T) {
	t.Run("returns false when socket does not exist", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		require.False(t, IsAddonRunning())
	})
}

func TestNewLocalProvider(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpDir)

	provider := NewLocalProvider()
	require.NotNil(t, provider)
	require.Equal(t, "local", provider.Name())
}

func TestToProtoRole(t *testing.T) {
	tests := []struct {
		name     string
		role     Role
		expected int32
	}{
		{"user role", RoleUser, 1},
		{"assistant role", RoleAssistant, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toProtoRole(tt.role)
			require.Equal(t, tt.expected, int32(result))
		})
	}
}

// TestLocalProviderIntegration tests end-to-end communication with the tsuku-llm addon.
// Run with: go test -v -run TestLocalProviderIntegration ./internal/llm/
// Requires tsuku-llm to be running.
func TestLocalProviderIntegration(t *testing.T) {
	if !IsAddonRunning() {
		t.Skip("tsuku-llm addon not running, skipping integration test")
	}

	ctx := context.Background()

	provider := NewLocalProvider()
	require.NotNil(t, provider)
	defer func() { _ = provider.Close() }()

	t.Run("GetStatus", func(t *testing.T) {
		status, err := provider.GetStatus(ctx)
		require.NoError(t, err, "GetStatus failed")
		require.NotNil(t, status)
		t.Logf("Addon status: ready=%v, model=%s, backend=%s", status.Ready, status.ModelName, status.Backend)
	})

	t.Run("Complete", func(t *testing.T) {
		req := &CompletionRequest{
			SystemPrompt: "You are a helpful assistant.",
			Messages: []Message{
				{Role: RoleUser, Content: "Say hello"},
			},
			MaxTokens: 100,
		}

		resp, err := provider.Complete(ctx, req)
		require.NoError(t, err, "Complete failed")
		require.NotNil(t, resp)
		t.Logf("Response: content=%q, stop_reason=%s", resp.Content, resp.StopReason)
		t.Logf("Usage: input=%d, output=%d", resp.Usage.InputTokens, resp.Usage.OutputTokens)
	})
}

// mockInferenceServer is a mock implementation of the gRPC InferenceService for testing.
type mockInferenceServer struct {
	pb.UnimplementedInferenceServiceServer
	completeResponse *pb.CompletionResponse
	statusResponse   *pb.StatusResponse
}

func (m *mockInferenceServer) Complete(ctx context.Context, req *pb.CompletionRequest) (*pb.CompletionResponse, error) {
	if m.completeResponse != nil {
		return m.completeResponse, nil
	}
	return &pb.CompletionResponse{
		Content:    "Hello from mock server!",
		StopReason: "end_turn",
		Usage: &pb.Usage{
			InputTokens:  10,
			OutputTokens: 5,
		},
	}, nil
}

func (m *mockInferenceServer) GetStatus(ctx context.Context, req *pb.StatusRequest) (*pb.StatusResponse, error) {
	if m.statusResponse != nil {
		return m.statusResponse, nil
	}
	return &pb.StatusResponse{
		Ready:     true,
		ModelName: "test-model",
		Backend:   "cpu",
	}, nil
}

func (m *mockInferenceServer) Shutdown(ctx context.Context, req *pb.ShutdownRequest) (*pb.ShutdownResponse, error) {
	return &pb.ShutdownResponse{Accepted: true}, nil
}

// TestLocalProviderWithMockServer tests LocalProvider with an in-memory mock gRPC server.
// This validates the end-to-end flow without requiring the actual tsuku-llm addon.
func TestLocalProviderWithMockServer(t *testing.T) {
	// Create a buffer-based listener for in-process gRPC
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	// Create and start the mock server
	mockServer := &mockInferenceServer{}
	grpcServer := grpc.NewServer()
	pb.RegisterInferenceServiceServer(grpcServer, mockServer)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("mock server error: %v", err)
		}
	}()
	defer grpcServer.Stop()

	// Create a client connection using the bufconn dialer
	ctx := context.Background()
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	defer func() { _ = conn.Close() }()

	// Create a LocalProvider-like client using the mock connection
	client := pb.NewInferenceServiceClient(conn)

	t.Run("Complete", func(t *testing.T) {
		req := &pb.CompletionRequest{
			SystemPrompt: "You are a helpful assistant.",
			Messages: []*pb.Message{
				{Role: pb.Role_ROLE_USER, Content: "Say hello"},
			},
			MaxTokens: 100,
		}

		resp, err := client.Complete(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.Equal(t, "Hello from mock server!", resp.Content)
		require.Equal(t, "end_turn", resp.StopReason)
		require.NotNil(t, resp.Usage)
		require.Equal(t, int32(10), resp.Usage.InputTokens)
		require.Equal(t, int32(5), resp.Usage.OutputTokens)
	})

	t.Run("GetStatus", func(t *testing.T) {
		resp, err := client.GetStatus(ctx, &pb.StatusRequest{})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.True(t, resp.Ready)
		require.Equal(t, "test-model", resp.ModelName)
		require.Equal(t, "cpu", resp.Backend)
	})

	t.Run("Shutdown", func(t *testing.T) {
		resp, err := client.Shutdown(ctx, &pb.ShutdownRequest{Graceful: true})
		require.NoError(t, err)
		require.NotNil(t, resp)
		require.True(t, resp.Accepted)
	})
}

// TestLocalProviderSetPrompter verifies prompter propagation to AddonManager.
func TestLocalProviderSetPrompter(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpDir)

	provider := NewLocalProvider()
	require.NotNil(t, provider)

	// Set auto-approve prompter
	prompter := addon.NewAutoApprovePrompter()
	provider.SetPrompter(prompter)

	// Verify prompter was set
	require.Equal(t, prompter, provider.prompter)
}

// TestEnsureModelReady verifies model download prompt behavior.
func TestEnsureModelReady(t *testing.T) {
	// Create a buffer-based listener for in-process gRPC
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	t.Run("skips prompt when model is ready", func(t *testing.T) {
		mockServer := &mockInferenceServer{
			statusResponse: &pb.StatusResponse{
				Ready:     true,
				ModelName: "qwen2.5-1.5b-q4",
			},
		}
		grpcServer := grpc.NewServer()
		pb.RegisterInferenceServiceServer(grpcServer, mockServer)
		go func() { _ = grpcServer.Serve(lis) }()
		defer grpcServer.Stop()

		conn, err := grpc.NewClient(
			"passthrough://bufnet",
			grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
				return lis.Dial()
			}),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
		)
		require.NoError(t, err)
		defer conn.Close()

		prompted := false
		provider := &LocalProvider{
			client: pb.NewInferenceServiceClient(conn),
			conn:   conn,
			prompter: &testLocalPrompter{
				approve:  true,
				onPrompt: func(_ string, _ int64) { prompted = true },
			},
		}

		err = provider.ensureModelReady(context.Background())
		require.NoError(t, err)
		require.False(t, prompted, "should not prompt when model is ready")
		require.True(t, provider.modelPrompted, "modelPrompted flag should be set")
	})

	t.Run("does not re-prompt after first check", func(t *testing.T) {
		tmpDir := t.TempDir()
		t.Setenv("TSUKU_HOME", tmpDir)

		provider := &LocalProvider{modelPrompted: true}
		err := provider.ensureModelReady(context.Background())
		require.NoError(t, err)
	})
}

// testLocalPrompter is a mock Prompter for LocalProvider tests.
type testLocalPrompter struct {
	approve  bool
	onPrompt func(description string, size int64)
}

func (p *testLocalPrompter) ConfirmDownload(_ context.Context, description string, size int64) (bool, error) {
	if p.onPrompt != nil {
		p.onPrompt(description, size)
	}
	return p.approve, nil
}

// TestErrDownloadDeclinedMessage verifies the user-facing error message.
func TestErrDownloadDeclinedMessage(t *testing.T) {
	require.Contains(t, addon.ErrDownloadDeclined.Error(), "download declined")
}

// TestFromProtoResponse verifies the proto-to-Go conversion.
func TestFromProtoResponse(t *testing.T) {
	pbResp := &pb.CompletionResponse{
		Content:    "Test content",
		StopReason: "end_turn",
		Usage: &pb.Usage{
			InputTokens:  100,
			OutputTokens: 50,
		},
		ToolCalls: []*pb.ToolCall{
			{
				Id:            "call_123",
				Name:          "test_tool",
				ArgumentsJson: `{"arg1": "value1"}`,
			},
		},
	}

	resp := fromProtoResponse(pbResp)

	require.Equal(t, "Test content", resp.Content)
	require.Equal(t, "end_turn", resp.StopReason)
	require.Equal(t, 100, resp.Usage.InputTokens)
	require.Equal(t, 50, resp.Usage.OutputTokens)
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, "call_123", resp.ToolCalls[0].ID)
	require.Equal(t, "test_tool", resp.ToolCalls[0].Name)
	require.Equal(t, "value1", resp.ToolCalls[0].Arguments["arg1"])
}

// TestToProtoRequest verifies the Go-to-proto conversion.
func TestToProtoRequest(t *testing.T) {
	req := &CompletionRequest{
		SystemPrompt: "System prompt",
		Messages: []Message{
			{Role: RoleUser, Content: "User message"},
			{Role: RoleAssistant, Content: "Assistant message"},
		},
		Tools: []ToolDef{
			{
				Name:        "test_tool",
				Description: "A test tool",
				Parameters:  map[string]any{"type": "object"},
			},
		},
		MaxTokens: 100,
	}

	pbReq := toProtoRequest(req)

	require.Equal(t, "System prompt", pbReq.SystemPrompt)
	require.Len(t, pbReq.Messages, 2)
	require.Equal(t, pb.Role_ROLE_USER, pbReq.Messages[0].Role)
	require.Equal(t, "User message", pbReq.Messages[0].Content)
	require.Equal(t, pb.Role_ROLE_ASSISTANT, pbReq.Messages[1].Role)
	require.Len(t, pbReq.Tools, 1)
	require.Equal(t, "test_tool", pbReq.Tools[0].Name)
	require.Equal(t, int32(100), pbReq.MaxTokens)
}
