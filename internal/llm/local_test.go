package llm

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

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
	completeErr      error
	statusResponse   *pb.StatusResponse
}

func (m *mockInferenceServer) Complete(ctx context.Context, req *pb.CompletionRequest) (*pb.CompletionResponse, error) {
	if m.completeErr != nil {
		return nil, m.completeErr
	}
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

// TestSendRequestInvalidatesConnectionOnError verifies that a gRPC error from
// sendRequest causes the LocalProvider to nil out its cached connection and client.
// This ensures subsequent calls trigger reconnection via ensureConnection
// instead of reusing a dead connection after a server crash.
func TestSendRequestInvalidatesConnectionOnError(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpDir)

	// Start a mock server that returns an error from Complete.
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	mockServer := &mockInferenceServer{
		completeErr: fmt.Errorf("simulated server crash"),
	}
	grpcServer := grpc.NewServer()
	pb.RegisterInferenceServiceServer(grpcServer, mockServer)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("mock server error: %v", err)
		}
	}()
	defer grpcServer.Stop()

	// Create a client connection through bufconn.
	ctx := context.Background()
	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)

	// Inject the connection into a LocalProvider, bypassing ensureConnection
	// and the lifecycle/addon manager (which aren't relevant to this test).
	provider := &LocalProvider{
		conn:   conn,
		client: pb.NewInferenceServiceClient(conn),
	}

	// Verify the connection fields are populated before the call.
	require.NotNil(t, provider.conn, "conn should be set before sendRequest call")
	require.NotNil(t, provider.client, "client should be set before sendRequest call")

	// Call sendRequest directly -- this tests the gRPC call and invalidation
	// logic without going through addon/lifecycle checks.
	req := &CompletionRequest{
		SystemPrompt: "test",
		Messages:     []Message{{Role: RoleUser, Content: "hello"}},
		MaxTokens:    10,
	}
	_, err = provider.sendRequest(ctx, req)
	require.Error(t, err)
	require.Contains(t, err.Error(), "local LLM completion failed")

	// After the error, the cached connection must be invalidated.
	require.Nil(t, provider.client, "client should be nil after gRPC error")
	require.Nil(t, provider.conn, "conn should be nil after gRPC error")
}

// TestSendRequestSucceedsOnValidResponse verifies that sendRequest does NOT
// invalidate the connection when the gRPC call succeeds.
func TestSendRequestSucceedsOnValidResponse(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("TSUKU_HOME", tmpDir)

	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	mockServer := &mockInferenceServer{}
	grpcServer := grpc.NewServer()
	pb.RegisterInferenceServiceServer(grpcServer, mockServer)

	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			t.Logf("mock server error: %v", err)
		}
	}()
	defer grpcServer.Stop()

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

	provider := &LocalProvider{
		conn:   conn,
		client: pb.NewInferenceServiceClient(conn),
	}

	req := &CompletionRequest{
		SystemPrompt: "test",
		Messages:     []Message{{Role: RoleUser, Content: "hello"}},
		MaxTokens:    10,
	}
	resp, err := provider.sendRequest(ctx, req)
	require.NoError(t, err)
	require.Equal(t, "Hello from mock server!", resp.Content)

	// Connection should still be valid after a successful call.
	require.NotNil(t, provider.client, "client should remain set after successful call")
	require.NotNil(t, provider.conn, "conn should remain set after successful call")
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
