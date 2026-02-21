//go:build integration

package llm

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// TestIntegration_ActivityResetsIdleTimeout verifies that making gRPC calls
// resets the idle timeout, preventing premature shutdown.
func TestIntegration_ActivityResetsIdleTimeout(t *testing.T) {
	skipIfModelCDNUnavailable(t)

	tsukuHome := setupTsukuHome(t)
	os.Setenv("TSUKU_HOME", tsukuHome)
	defer os.Unsetenv("TSUKU_HOME")

	// Start daemon with 30s timeout (allows time for model loading)
	daemon := startDaemon(t, tsukuHome, 30*time.Second)

	// Wait for socket to be available
	require.Eventually(t, func() bool {
		return isDaemonReady(tsukuHome)
	}, 10*time.Second, 100*time.Millisecond, "daemon socket should become ready")

	// Create provider and wait for it to be fully ready (model loaded)
	provider := NewLocalProvider()
	defer provider.Close()

	t.Log("Waiting for daemon to be fully ready (model loading)...")
	require.Eventually(t, func() bool {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		status, err := provider.GetStatus(ctx)
		return err == nil && status != nil && status.Ready
	}, 3*time.Minute, 1*time.Second, "daemon should be fully ready (gRPC accepting, model loaded)")

	t.Log("Daemon fully ready, starting activity test...")

	// Wait 20 seconds (less than 30s timeout)
	t.Log("Waiting 20 seconds...")
	time.Sleep(20 * time.Second)

	// Make another GetStatus call - this should reset the timeout
	ctx2, cancel2 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel2()
	status2, err := provider.GetStatus(ctx2)
	require.NoError(t, err, "second GetStatus should succeed (daemon should still be alive)")
	require.True(t, status2.Ready, "daemon should still be ready")
	t.Log("Second GetStatus succeeded (at 20s mark), timeout should be reset")

	// Wait another 20 seconds (total 40s from first activity, but only 20s since last activity)
	t.Log("Waiting 20 more seconds...")
	time.Sleep(20 * time.Second)

	// Daemon should still be alive because timeout was reset at 20s mark
	ctx3, cancel3 := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel3()
	status3, err := provider.GetStatus(ctx3)
	require.NoError(t, err, "third GetStatus should succeed (timeout was reset)")
	require.True(t, status3.Ready, "daemon should still be ready after activity reset")
	t.Log("SUCCESS: Third GetStatus succeeded at 40s mark (activity reset works)")

	// Verify daemon is still running
	require.True(t, isDaemonRunning(tsukuHome), "daemon should still be running")

	// Clean up
	_ = daemon.Process.Kill()
}
