//go:build integration

package llm

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/tsukumogami/tsuku/internal/llm/proto"
)

// These tests require the tsuku-llm binary to be built and available.
// Run with: go test -tags=integration ./internal/llm/...
//
// To build the addon first:
//   cd tsuku-llm && cargo build --release
//
// Set TSUKU_LLM_BINARY to the path of the binary if not in tsuku-llm/target/release/.

// modelSourceURL is the URL where models are downloaded from (HuggingFace Hub).
// Tests that require model inference skip if this URL is unreachable.
const modelSourceURL = "https://huggingface.co/Qwen/Qwen2.5-0.5B-Instruct-GGUF/resolve/main/qwen2.5-0.5b-instruct-q4_k_m.gguf"

// skipIfModelCDNUnavailable skips the test if the model source is not reachable.
// This allows tests to pass in environments where HuggingFace is blocked or
// network access is restricted.
func skipIfModelCDNUnavailable(t *testing.T) {
	t.Helper()

	client := &http.Client{Timeout: 10 * time.Second}
	// Use HEAD request to check accessibility without downloading the file
	req, err := http.NewRequest("HEAD", modelSourceURL, nil)
	if err != nil {
		t.Skipf("Model source unavailable (request error): %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Skipf("Model source unavailable (network error): %v", err)
	}
	defer resp.Body.Close()
	// Drain body to allow connection reuse
	_, _ = io.Copy(io.Discard, resp.Body)

	// 4xx/5xx responses indicate source is down or model is not available
	if resp.StatusCode >= 400 {
		t.Skipf("Model source unavailable (HTTP %d)", resp.StatusCode)
	}
}

// grpcDial connects to the daemon's Unix socket.
func grpcDial(ctx context.Context, socketPath string) (*grpc.ClientConn, error) {
	return grpc.NewClient(
		"unix://"+socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}

// testMessage is a simple message struct for testing.
type testMessage struct {
	role    string
	content string
}

// testResponse is a simple response struct for testing.
type testResponse struct {
	content    string
	stopReason string
}

// inferenceClient wraps the proto client for easier testing.
type inferenceClient struct {
	client pb.InferenceServiceClient
}

func newInferenceClient(conn *grpc.ClientConn) *inferenceClient {
	return &inferenceClient{client: pb.NewInferenceServiceClient(conn)}
}

func (c *inferenceClient) complete(ctx context.Context, systemPrompt string, messages []testMessage, maxTokens int) (*testResponse, error) {
	req := &pb.CompletionRequest{
		SystemPrompt: systemPrompt,
		MaxTokens:    int32(maxTokens),
	}
	for _, msg := range messages {
		role := pb.Role_ROLE_USER
		if msg.role == "assistant" {
			role = pb.Role_ROLE_ASSISTANT
		}
		req.Messages = append(req.Messages, &pb.Message{
			Role:    role,
			Content: msg.content,
		})
	}

	resp, err := c.client.Complete(ctx, req)
	if err != nil {
		return nil, err
	}
	return &testResponse{
		content:    resp.Content,
		stopReason: resp.StopReason,
	}, nil
}

func getAddonBinary(t *testing.T) string {
	t.Helper()

	// Check for explicit path via env var
	if path := os.Getenv("TSUKU_LLM_BINARY"); path != "" {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		t.Skipf("TSUKU_LLM_BINARY set to %s but file not found", path)
	}

	// Try default build location
	workspaceRoot := findWorkspaceRoot(t)
	defaultPath := filepath.Join(workspaceRoot, "tsuku-llm", "target", "release", "tsuku-llm")
	if _, err := os.Stat(defaultPath); err == nil {
		return defaultPath
	}

	// Try debug build
	debugPath := filepath.Join(workspaceRoot, "tsuku-llm", "target", "debug", "tsuku-llm")
	if _, err := os.Stat(debugPath); err == nil {
		return debugPath
	}

	t.Skip("tsuku-llm binary not found. Build with: cd tsuku-llm && cargo build --release")
	return ""
}

func findWorkspaceRoot(t *testing.T) string {
	t.Helper()

	// Start from current working directory and find go.mod
	cwd, err := os.Getwd()
	require.NoError(t, err)

	dir := cwd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("Could not find workspace root (go.mod)")
		}
		dir = parent
	}
}

// startDaemon starts the tsuku-llm daemon with the given TSUKU_HOME.
func startDaemon(t *testing.T, tsukuHome string, timeout time.Duration) *exec.Cmd {
	t.Helper()

	binary := getAddonBinary(t)
	cmd := exec.Command(binary, "serve", "--idle-timeout", timeout.String())
	cmd.Env = append(os.Environ(), "TSUKU_HOME="+tsukuHome)
	cmd.Stderr = os.Stderr

	err := cmd.Start()
	require.NoError(t, err, "failed to start daemon")

	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	})

	return cmd
}

// isDaemonReady checks if the daemon is ready by attempting to connect to the socket.
func isDaemonReady(tsukuHome string) bool {
	socketPath := filepath.Join(tsukuHome, "llm.sock")
	conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

// isDaemonRunning checks if the daemon process is still running by checking the lock file.
func isDaemonRunning(tsukuHome string) bool {
	lockPath := filepath.Join(tsukuHome, "llm.sock.lock")
	// Don't create the file - just try to open it for reading
	f, err := os.OpenFile(lockPath, os.O_RDWR, 0600)
	if err != nil {
		// File doesn't exist or can't be opened - daemon not running
		return false
	}
	defer f.Close()

	// Try non-blocking lock
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Lock held - daemon is running
		return true
	}

	// We got the lock - daemon is not running
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	return false
}

func TestIntegration_LockFilePreventsduplicates(t *testing.T) {
	tsukuHome := t.TempDir()

	// Start first daemon
	daemon1 := startDaemon(t, tsukuHome, 5*time.Minute)

	// Wait for it to be ready
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon1 should become ready")

	// Try to start second daemon - it should fail to bind
	binary := getAddonBinary(t)
	cmd2 := exec.Command(binary, "serve", "--idle-timeout", "5m")
	cmd2.Env = append(os.Environ(), "TSUKU_HOME="+tsukuHome)

	err := cmd2.Start()
	if err == nil {
		// Started successfully, but it should exit quickly due to socket conflict
		done := make(chan error, 1)
		go func() { done <- cmd2.Wait() }()

		select {
		case err := <-done:
			// Second daemon should have exited with error
			require.Error(t, err, "second daemon should fail when socket already in use")
		case <-time.After(3 * time.Second):
			_ = cmd2.Process.Kill()
			t.Fatal("second daemon should have exited but is still running")
		}
	}

	// First daemon should still be running
	require.True(t, isDaemonReady(tsukuHome), "first daemon should still be running")

	// Clean up first daemon
	_ = daemon1.Process.Signal(syscall.SIGTERM)
}

func TestIntegration_StaleSocketCleanup(t *testing.T) {
	tsukuHome := t.TempDir()

	// Create orphaned socket file (no lock held)
	socketPath := filepath.Join(tsukuHome, "llm.sock")
	require.NoError(t, os.WriteFile(socketPath, []byte("stale"), 0600))
	require.FileExists(t, socketPath)

	// Start daemon - should clean up stale socket and start successfully
	daemon := startDaemon(t, tsukuHome, 5*time.Minute)

	// Should become ready
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should start after cleaning stale socket")

	// Clean up
	_ = daemon.Process.Signal(syscall.SIGTERM)
}

func TestIntegration_ShortTimeoutTriggersShutdown(t *testing.T) {
	skipIfModelCDNUnavailable(t)

	tsukuHome := t.TempDir()

	// Start daemon with short 2s timeout
	daemon := startDaemon(t, tsukuHome, 2*time.Second)

	// Wait for it to be ready
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should become ready")

	// Wait for idle timeout (2s + buffer)
	time.Sleep(3 * time.Second)

	// Verify daemon stopped
	require.Eventually(t, func() bool {
		return !isDaemonRunning(tsukuHome)
	}, 5*time.Second, 100*time.Millisecond, "daemon should stop after idle timeout")

	// Verify socket cleaned up
	socketPath := filepath.Join(tsukuHome, "llm.sock")
	_, err := os.Stat(socketPath)
	require.True(t, os.IsNotExist(err), "socket should be cleaned up after idle timeout")

	// Wait for process to exit
	err = daemon.Wait()
	require.NoError(t, err, "daemon should exit cleanly")
}

func TestIntegration_SIGTERMTriggersGracefulShutdown(t *testing.T) {
	skipIfModelCDNUnavailable(t)

	tsukuHome := t.TempDir()

	// Start daemon
	daemon := startDaemon(t, tsukuHome, 5*time.Minute)

	// Wait for it to be ready
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should become ready")

	// Send SIGTERM
	require.NoError(t, daemon.Process.Signal(syscall.SIGTERM))

	// Verify graceful shutdown
	require.Eventually(t, func() bool {
		return !isDaemonRunning(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should stop after SIGTERM")

	// Verify files cleaned up
	socketPath := filepath.Join(tsukuHome, "llm.sock")
	lockPath := filepath.Join(tsukuHome, "llm.sock.lock")

	_, err := os.Stat(socketPath)
	require.True(t, os.IsNotExist(err), "socket should be cleaned up after SIGTERM")

	_, err = os.Stat(lockPath)
	require.True(t, os.IsNotExist(err), "lock file should be cleaned up after SIGTERM")

	// Wait for process exit
	err = daemon.Wait()
	require.NoError(t, err, "daemon should exit cleanly after SIGTERM")
}

func TestIntegration_MultipleSIGTERMIsSafe(t *testing.T) {
	skipIfModelCDNUnavailable(t)

	tsukuHome := t.TempDir()

	// Start daemon
	daemon := startDaemon(t, tsukuHome, 5*time.Minute)

	// Wait for ready
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should become ready")

	// Send multiple SIGTERMs quickly
	_ = daemon.Process.Signal(syscall.SIGTERM)
	time.Sleep(50 * time.Millisecond)
	_ = daemon.Process.Signal(syscall.SIGTERM)
	time.Sleep(50 * time.Millisecond)
	_ = daemon.Process.Signal(syscall.SIGTERM)

	// Should still shutdown gracefully
	require.Eventually(t, func() bool {
		return !isDaemonRunning(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should stop after multiple SIGTERMs")

	// Should exit cleanly
	err := daemon.Wait()
	require.NoError(t, err, "daemon should exit cleanly after multiple SIGTERMs")
}

// TestIntegration_gRPCGetStatus tests that the Go client can call GetStatus on the daemon.
func TestIntegration_gRPCGetStatus(t *testing.T) {
	skipIfModelCDNUnavailable(t)

	tsukuHome := t.TempDir()
	os.Setenv("TSUKU_HOME", tsukuHome)
	defer os.Unsetenv("TSUKU_HOME")

	// Start daemon
	daemon := startDaemon(t, tsukuHome, 5*time.Minute)

	// Wait for ready
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should become ready")

	// Create provider and get status
	provider := NewLocalProvider()
	defer provider.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	status, err := provider.GetStatus(ctx)
	require.NoError(t, err, "GetStatus should succeed")
	require.NotNil(t, status, "status should not be nil")
	require.True(t, status.Ready, "daemon should report ready")
	require.NotEmpty(t, status.ModelName, "model name should be set")

	// Clean up
	_ = daemon.Process.Signal(syscall.SIGTERM)
}

// TestIntegration_gRPCComplete tests that the Go client can call Complete on the daemon.
// This test uses the proto client directly since the daemon is already running
// (bypasses the addon download/verification that would happen in production).
func TestIntegration_gRPCComplete(t *testing.T) {
	skipIfModelCDNUnavailable(t)

	tsukuHome := t.TempDir()
	os.Setenv("TSUKU_HOME", tsukuHome)
	defer os.Unsetenv("TSUKU_HOME")

	// Start daemon
	daemon := startDaemon(t, tsukuHome, 5*time.Minute)

	// Wait for ready
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should become ready")

	// Connect directly to the daemon via gRPC (bypass addon manager)
	socketPath := filepath.Join(tsukuHome, "llm.sock")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	conn, err := grpcDial(ctx, socketPath)
	require.NoError(t, err, "should connect to daemon")
	defer conn.Close()

	client := newInferenceClient(conn)

	// Send a completion request using proto types directly
	resp, err := client.complete(ctx, "You are a helpful assistant.", []testMessage{
		{role: "user", content: "Hello, what is 2+2?"},
	}, 100)
	require.NoError(t, err, "Complete should succeed")
	require.NotEmpty(t, resp.content, "response content should not be empty")
	require.NotEmpty(t, resp.stopReason, "stop reason should be set")

	// Clean up
	_ = daemon.Process.Signal(syscall.SIGTERM)
}

// TestIntegration_gRPCShutdown tests that the Go client can request daemon shutdown via gRPC.
func TestIntegration_gRPCShutdown(t *testing.T) {
	skipIfModelCDNUnavailable(t)

	tsukuHome := t.TempDir()
	os.Setenv("TSUKU_HOME", tsukuHome)
	defer os.Unsetenv("TSUKU_HOME")

	// Start daemon
	daemon := startDaemon(t, tsukuHome, 5*time.Minute)

	// Wait for ready
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should become ready")

	// Create provider
	provider := NewLocalProvider()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Ensure connection is established
	_, err := provider.GetStatus(ctx)
	require.NoError(t, err, "GetStatus should succeed")

	// Request graceful shutdown via gRPC
	err = provider.Shutdown(ctx, true)
	require.NoError(t, err, "Shutdown request should succeed")

	// Close client connection
	provider.Close()

	// Daemon should stop
	require.Eventually(t, func() bool {
		return !isDaemonRunning(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should stop after gRPC shutdown request")

	// Verify files cleaned up
	socketPath := filepath.Join(tsukuHome, "llm.sock")
	_, err = os.Stat(socketPath)
	require.True(t, os.IsNotExist(err), "socket should be cleaned up after shutdown")

	// Process should have exited
	err = daemon.Wait()
	require.NoError(t, err, "daemon should exit cleanly")
}
