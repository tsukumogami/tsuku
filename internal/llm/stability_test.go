//go:build integration

package llm

import (
	"context"
	"fmt"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestSequentialInference starts the daemon once and sends multiple inference
// requests sequentially, verifying that the server handles sustained workloads
// without degradation or connection issues.
//
// This test uses low-level gRPC calls (grpcDial + inferenceClient) rather than
// the provider.Complete() API so we can verify the raw server behavior without
// any reconnection or restart logic from LocalProvider masking failures.
func TestSequentialInference(t *testing.T) {
	skipIfModelCDNUnavailable(t)

	tsukuHome := setupTsukuHome(t)
	t.Setenv("TSUKU_HOME", tsukuHome)

	// Start daemon with a generous idle timeout so it stays alive for all requests.
	daemon := startDaemon(t, tsukuHome, 10*time.Minute)

	// Wait for socket to be available.
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon socket should become ready")

	// Wait for the model to finish loading by polling GetStatus.
	provider := NewLocalProvider()
	defer provider.Close()

	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		status, err := provider.GetStatus(ctx)
		return err == nil && status != nil && status.Ready
	}, 3*time.Minute, 1*time.Second, "daemon should be fully ready (model loaded)")

	// Define a set of distinct prompts to exercise the server across requests.
	prompts := []struct {
		system  string
		message string
	}{
		{"You are a helpful assistant.", "What is 2+2?"},
		{"You are a concise assistant.", "Name a color."},
		{"You answer in one sentence.", "What is the capital of France?"},
		{"You are a math tutor.", "What is 10 divided by 2?"},
		{"You are a geography expert.", "Name a continent."},
	}

	socketPath := filepath.Join(tsukuHome, "llm.sock")

	for i, p := range prompts {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)

		conn, err := grpcDial(ctx, socketPath)
		require.NoError(t, err, "request %d: should connect to daemon", i+1)

		client := newInferenceClient(conn)
		resp, err := client.complete(ctx, p.system, []testMessage{
			{role: "user", content: p.message},
		}, 100)
		require.NoError(t, err, "request %d: Complete should succeed", i+1)
		require.NotEmpty(t, resp.content, "request %d: response content should not be empty", i+1)

		t.Logf("request %d: stop_reason=%s, response_length=%d", i+1, resp.stopReason, len(resp.content))

		_ = conn.Close()
		cancel()
	}

	// Gracefully shut down the daemon.
	_ = daemon.Process.Signal(syscall.SIGTERM)
}

// TestCrashRecovery verifies that LocalProvider recovers after the daemon
// crashes unexpectedly. After a SIGKILL, EnsureRunning detects the dead
// daemon (lock file is no longer held), cleans up the stale socket, starts
// a fresh daemon, and waits for it to become ready. Combined with gRPC's
// transparent reconnection to the new Unix socket, recovery is seamless:
// the first Complete call after the crash succeeds.
//
// This test uses provider.Complete() (the high-level API) rather than raw gRPC
// because we need to exercise the full reconnection and daemon-restart path
// inside LocalProvider -- that's the behavior under test.
func TestCrashRecovery(t *testing.T) {
	skipIfModelCDNUnavailable(t)

	tsukuHome := setupTsukuHome(t)
	t.Setenv("TSUKU_HOME", tsukuHome)

	// Start daemon for the first time.
	daemon := startDaemon(t, tsukuHome, 10*time.Minute)

	// Wait for socket.
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon socket should become ready")

	// Use NewLocalProvider so the full EnsureRunning / reconnect path is exercised.
	provider := NewLocalProvider()
	defer provider.Close()

	// Wait for the model to be fully loaded.
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		status, err := provider.GetStatus(ctx)
		return err == nil && status != nil && status.Ready
	}, 3*time.Minute, 1*time.Second, "daemon should be fully ready (model loaded)")

	// --- Baseline: send one successful inference request ---
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	baselineResp, err := provider.Complete(ctx, &CompletionRequest{
		SystemPrompt: "You are a helpful assistant.",
		Messages:     []Message{{Role: RoleUser, Content: "Say hello."}},
		MaxTokens:    50,
	})
	cancel()
	require.NoError(t, err, "baseline Complete should succeed")
	require.NotEmpty(t, baselineResp.Content, "baseline response should not be empty")
	t.Logf("baseline response: %s", baselineResp.Content)

	// --- Simulate crash: SIGKILL the daemon ---
	t.Log("sending SIGKILL to daemon")
	err = daemon.Process.Signal(syscall.SIGKILL)
	require.NoError(t, err, "SIGKILL should succeed")
	_ = daemon.Wait() // reap the zombie

	// Wait until the daemon is confirmed dead.
	require.Eventually(t, func() bool {
		return !isDaemonRunning(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon should be dead after SIGKILL")

	// --- Post-crash call: EnsureRunning restarts the daemon seamlessly ---
	// EnsureRunning detects the dead daemon (lock is free), starts a new one,
	// and waits for socket readiness. gRPC transparently reconnects to the
	// new Unix socket, so the first Complete after crash should succeed.
	// Give enough time for the addon to start and load the model.
	var recoveryResp *CompletionResponse
	require.Eventually(t, func() bool {
		reqCtx, reqCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer reqCancel()
		resp, callErr := provider.Complete(reqCtx, &CompletionRequest{
			SystemPrompt: "You are a helpful assistant.",
			Messages:     []Message{{Role: RoleUser, Content: "What is 1+1?"}},
			MaxTokens:    50,
		})
		if callErr != nil {
			t.Logf("recovery attempt: %v", callErr)
			return false
		}
		recoveryResp = resp
		return true
	}, 5*time.Minute, 5*time.Second, "provider should recover and return a successful response")

	require.NotNil(t, recoveryResp, "recovery response should not be nil")
	require.NotEmpty(t, recoveryResp.Content, "recovery response should not be empty")
	t.Logf("recovery response: %s", fmt.Sprintf("%.100s", recoveryResp.Content))
}
