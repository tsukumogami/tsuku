package llm

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestNewServerLifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	addonPath := "/nonexistent/addon"

	lifecycle := NewServerLifecycle(socketPath, addonPath)
	require.NotNil(t, lifecycle)
	require.Equal(t, socketPath+".lock", lifecycle.LockPath())
}

func TestServerLifecycle_IsRunning_NoLockFile(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	lifecycle := NewServerLifecycle(socketPath, "")
	require.False(t, lifecycle.IsRunning(), "should return false when no lock file exists")
}

func TestServerLifecycle_IsRunning_LockFileNotHeld(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	lockPath := socketPath + ".lock"

	// Create the lock file but don't hold a lock on it
	f, err := os.Create(lockPath)
	require.NoError(t, err)
	f.Close()

	lifecycle := NewServerLifecycle(socketPath, "")
	require.False(t, lifecycle.IsRunning(), "should return false when lock file exists but isn't held")
}

func TestServerLifecycle_IsRunning_LockFileHeld(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	lockPath := socketPath + ".lock"

	// Create and hold an exclusive lock on the lock file
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer f.Close()

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	require.NoError(t, err)
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	lifecycle := NewServerLifecycle(socketPath, "")
	require.True(t, lifecycle.IsRunning(), "should return true when lock is held")
}

func TestServerLifecycle_EnsureRunning_AddonNotInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	addonPath := filepath.Join(tmpDir, "nonexistent", "addon")

	lifecycle := NewServerLifecycle(socketPath, addonPath)
	ctx := context.Background()

	err := lifecycle.EnsureRunning(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "not installed")
}

func TestServerLifecycle_EnsureRunning_CleansUpStaleSocket(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create a stale socket file (no daemon holding the lock)
	err := os.WriteFile(socketPath, []byte("stale"), 0600)
	require.NoError(t, err)

	// Verify the file exists
	_, err = os.Stat(socketPath)
	require.NoError(t, err, "stale socket should exist before test")

	// Create lifecycle with no addon (to trigger the cleanup path)
	lifecycle := NewServerLifecycle(socketPath, "")

	ctx := context.Background()
	err = lifecycle.EnsureRunning(ctx)
	// Should fail because addon path is not configured, but socket should be cleaned up
	require.Error(t, err)
	require.Contains(t, err.Error(), "addon path not configured")

	// Socket should have been cleaned up
	_, err = os.Stat(socketPath)
	require.True(t, os.IsNotExist(err), "stale socket should be removed")
}

func TestServerLifecycle_Stop_WhenNotRunning(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	lifecycle := NewServerLifecycle(socketPath, "")
	ctx := context.Background()

	// Stop should not error when nothing is running
	err := lifecycle.Stop(ctx)
	require.NoError(t, err)
}

func TestServerLifecycle_LockPath(t *testing.T) {
	tests := []struct {
		name       string
		socketPath string
		expected   string
	}{
		{
			name:       "standard path",
			socketPath: "/home/user/.tsuku/llm.sock",
			expected:   "/home/user/.tsuku/llm.sock.lock",
		},
		{
			name:       "tmp path",
			socketPath: "/tmp/test.sock",
			expected:   "/tmp/test.sock.lock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			lifecycle := NewServerLifecycle(tt.socketPath, "")
			require.Equal(t, tt.expected, lifecycle.LockPath())
		})
	}
}

func TestServerLifecycle_EnsureRunning_ReturnsIfAlreadyRunning(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	lockPath := socketPath + ".lock"

	// Create and hold an exclusive lock to simulate running daemon
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer lockFile.Close()

	err = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_EX)
	require.NoError(t, err)
	defer func() { _ = syscall.Flock(int(lockFile.Fd()), syscall.LOCK_UN) }()

	// Create a socket file that we can connect to (well, sort of)
	// In reality this will timeout because there's no real server
	lifecycle := NewServerLifecycle(socketPath, "")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// This should return an error because we can't connect, but not because
	// it tried to start the addon (since the lock is held)
	err = lifecycle.EnsureRunning(ctx)
	require.Error(t, err)
	// The error should be a timeout or context deadline, not "addon not installed"
	require.NotContains(t, err.Error(), "not installed")
}

func TestIsProcessDone(t *testing.T) {
	// Test with nil error
	require.False(t, isProcessDone(nil))

	// Test with "process already finished" error
	err := &os.PathError{Op: "signal", Path: "", Err: os.ErrProcessDone}
	// This won't match our simple string comparison, but let's test the function
	require.False(t, isProcessDone(err))
}

func TestGetIdleTimeout(t *testing.T) {
	t.Run("returns default when env not set", func(t *testing.T) {
		t.Setenv(IdleTimeoutEnvVar, "")
		require.Equal(t, DefaultIdleTimeout, GetIdleTimeout())
	})

	t.Run("parses valid duration from env", func(t *testing.T) {
		t.Setenv(IdleTimeoutEnvVar, "10s")
		require.Equal(t, 10*time.Second, GetIdleTimeout())
	})

	t.Run("parses minutes from env", func(t *testing.T) {
		t.Setenv(IdleTimeoutEnvVar, "2m")
		require.Equal(t, 2*time.Minute, GetIdleTimeout())
	})

	t.Run("returns default on invalid duration", func(t *testing.T) {
		t.Setenv(IdleTimeoutEnvVar, "invalid")
		require.Equal(t, DefaultIdleTimeout, GetIdleTimeout())
	})

	t.Run("returns default on empty string", func(t *testing.T) {
		t.Setenv(IdleTimeoutEnvVar, "")
		require.Equal(t, DefaultIdleTimeout, GetIdleTimeout())
	})
}

func TestServerLifecycle_IdleTimeout(t *testing.T) {
	t.Run("uses default timeout", func(t *testing.T) {
		t.Setenv(IdleTimeoutEnvVar, "")
		lifecycle := NewServerLifecycle("/tmp/test.sock", "")
		require.Equal(t, DefaultIdleTimeout, lifecycle.IdleTimeout())
	})

	t.Run("uses env var timeout", func(t *testing.T) {
		t.Setenv(IdleTimeoutEnvVar, "30s")
		lifecycle := NewServerLifecycle("/tmp/test.sock", "")
		require.Equal(t, 30*time.Second, lifecycle.IdleTimeout())
	})

	t.Run("SetIdleTimeout overrides", func(t *testing.T) {
		lifecycle := NewServerLifecycle("/tmp/test.sock", "")
		lifecycle.SetIdleTimeout(2 * time.Second)
		require.Equal(t, 2*time.Second, lifecycle.IdleTimeout())
	})
}

func TestServerLifecycleWithManager_IdleTimeout(t *testing.T) {
	t.Run("uses default timeout", func(t *testing.T) {
		t.Setenv(IdleTimeoutEnvVar, "")
		t.Setenv("TSUKU_HOME", t.TempDir())
		lifecycle := NewServerLifecycleWithManager("/tmp/test.sock", nil)
		require.Equal(t, DefaultIdleTimeout, lifecycle.IdleTimeout())
	})

	t.Run("uses env var timeout", func(t *testing.T) {
		t.Setenv(IdleTimeoutEnvVar, "1m")
		t.Setenv("TSUKU_HOME", t.TempDir())
		lifecycle := NewServerLifecycleWithManager("/tmp/test.sock", nil)
		require.Equal(t, 1*time.Minute, lifecycle.IdleTimeout())
	})
}

// Tests for lock file behavior

func TestServerLifecycle_LockFilePreventsSecondInstance(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	lockPath := socketPath + ".lock"

	// First lifecycle acquires the lock
	f1, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer f1.Close()

	err = syscall.Flock(int(f1.Fd()), syscall.LOCK_EX)
	require.NoError(t, err)

	// Second attempt to acquire should fail with LOCK_NB
	f2, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer f2.Close()

	err = syscall.Flock(int(f2.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	require.Error(t, err, "second lock attempt should fail when first holds lock")
}

func TestServerLifecycle_LockReleasedOnClose(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	lockPath := socketPath + ".lock"

	// Acquire lock
	f1, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	err = syscall.Flock(int(f1.Fd()), syscall.LOCK_EX)
	require.NoError(t, err)

	// Close file (releases lock)
	f1.Close()

	// Now second attempt should succeed
	f2, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer f2.Close()

	err = syscall.Flock(int(f2.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	require.NoError(t, err, "lock should be available after first holder closes")
}

func TestServerLifecycle_IsRunning_ReturnsFalseAfterLockRelease(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	lockPath := socketPath + ".lock"

	lifecycle := NewServerLifecycle(socketPath, "")

	// Acquire and hold lock
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	require.NoError(t, err)

	// Should be running
	require.True(t, lifecycle.IsRunning())

	// Release lock
	_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
	f.Close()

	// Should no longer be running
	require.False(t, lifecycle.IsRunning())
}

func TestServerLifecycle_StaleSocketRemovedWhenLockAvailable(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")

	// Create stale socket file
	require.NoError(t, os.WriteFile(socketPath, []byte("stale data"), 0600))
	require.FileExists(t, socketPath)

	lifecycle := NewServerLifecycle(socketPath, "")
	ctx := context.Background()

	// Try to ensure running - will fail because no addon, but should clean up stale socket
	err := lifecycle.EnsureRunning(ctx)
	require.Error(t, err)

	// Stale socket should be removed
	_, err = os.Stat(socketPath)
	require.True(t, os.IsNotExist(err), "stale socket should be removed")
}

func TestServerLifecycle_StaleSocketNotRemovedWhenLockHeld(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	lockPath := socketPath + ".lock"

	// Create socket file
	require.NoError(t, os.WriteFile(socketPath, []byte("data"), 0600))

	// Hold the lock (simulating running daemon)
	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0600)
	require.NoError(t, err)
	defer f.Close()
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
	require.NoError(t, err)
	defer func() { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }()

	lifecycle := NewServerLifecycle(socketPath, "")
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Try to ensure running - should not remove socket because lock is held
	_ = lifecycle.EnsureRunning(ctx)

	// Socket should still exist
	require.FileExists(t, socketPath, "socket should not be removed when lock is held")
}

// TestServerLifecycle_EnsureRunning_ReleasesLockForChild verifies that
// EnsureRunning releases the parent's flock before starting the child process.
// This is a regression test for the lock handoff bug where the parent held the
// lock while starting the addon, preventing the child from acquiring it.
func TestServerLifecycle_EnsureRunning_ReleasesLockForChild(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "test.sock")
	lockPath := socketPath + ".lock"

	// Create a mock addon script that:
	// 1. Tries to acquire the lock (fails = parent still holds it = bug)
	// 2. Creates a Unix socket so waitForReady succeeds
	// 3. Sleeps briefly then exits
	mockAddon := filepath.Join(tmpDir, "mock-addon")
	script := fmt.Sprintf(`#!/bin/bash
# Try to acquire the lock non-blocking
exec 200>"%s"
if ! flock -n 200; then
    echo "LOCK_HELD_BY_PARENT" >&2
    exit 1
fi

# Create Unix socket using Python (widely available)
python3 -c "
import socket, time, os, sys
s = socket.socket(socket.AF_UNIX, socket.SOCK_STREAM)
s.bind('%s')
s.listen(1)
time.sleep(3)
s.close()
" &

# Wait for socket to appear
for i in $(seq 1 20); do
    [ -S "%s" ] && break
    sleep 0.05
done

# Keep script alive while socket exists
wait
`, lockPath, socketPath, socketPath)

	require.NoError(t, os.WriteFile(mockAddon, []byte(script), 0755))

	lifecycle := NewServerLifecycle(socketPath, mockAddon)
	lifecycle.SetIdleTimeout(1 * time.Minute)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := lifecycle.EnsureRunning(ctx)
	require.NoError(t, err, "EnsureRunning should succeed when child can acquire lock")

	// Verify the socket is reachable
	conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
	require.NoError(t, err, "should be able to connect to socket")
	_ = conn.Close()

	// Clean up: stop the addon
	_ = lifecycle.Stop(context.Background())
}
