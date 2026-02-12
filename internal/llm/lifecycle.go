package llm

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/tsukumogami/tsuku/internal/llm/proto"
)

// ServerLifecycle manages the lifecycle of the tsuku-llm addon server.
// It uses a lock file protocol to reliably detect whether the daemon is running.
type ServerLifecycle struct {
	mu sync.Mutex

	socketPath string
	lockPath   string
	addonPath  string

	process *os.Process
	lockFd  *os.File // holds the lock file when we started the server
}

// NewServerLifecycle creates a new lifecycle manager.
// socketPath is the Unix domain socket path (e.g., $TSUKU_HOME/llm.sock).
// addonPath is the path to the tsuku-llm binary.
func NewServerLifecycle(socketPath, addonPath string) *ServerLifecycle {
	return &ServerLifecycle{
		socketPath: socketPath,
		lockPath:   socketPath + ".lock",
		addonPath:  addonPath,
	}
}

// LockPath returns the path to the lock file.
func (s *ServerLifecycle) LockPath() string {
	return s.lockPath
}

// IsRunning checks if the addon server is running by attempting to acquire
// a non-blocking exclusive lock on the lock file.
// If the lock can be acquired, the daemon is not running.
// If the lock fails (EWOULDBLOCK), the daemon is running.
func (s *ServerLifecycle) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.isRunningLocked()
}

// isRunningLocked checks daemon state without holding the mutex.
func (s *ServerLifecycle) isRunningLocked() bool {
	// Try to acquire the lock file
	fd, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		// Can't open lock file - assume not running
		return false
	}
	defer fd.Close()

	// Try non-blocking exclusive lock
	err = syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Lock failed - daemon is holding it
		return true
	}

	// We got the lock - daemon is not running
	// Release the lock immediately (we're just checking)
	_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
	return false
}

// EnsureRunning starts the addon server if it's not already running.
// It uses the lock file protocol to reliably detect running state.
// If the socket file exists but no daemon holds the lock, it cleans up
// the stale socket before starting.
func (s *ServerLifecycle) EnsureRunning(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if addon binary exists
	if s.addonPath != "" {
		if _, err := os.Stat(s.addonPath); os.IsNotExist(err) {
			return fmt.Errorf("tsuku-llm addon not installed at %s", s.addonPath)
		}
	}

	// Try to acquire the lock file
	fd, err := os.OpenFile(s.lockPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("failed to open lock file: %w", err)
	}

	// Try non-blocking exclusive lock
	err = syscall.Flock(int(fd.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Lock failed - daemon is already running
		fd.Close()
		return s.waitForReady(ctx)
	}

	// We got the lock - no daemon is running
	// Clean up stale socket if it exists
	if _, err := os.Stat(s.socketPath); err == nil {
		if err := os.Remove(s.socketPath); err != nil {
			_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
			fd.Close()
			return fmt.Errorf("failed to remove stale socket: %w", err)
		}
	}

	// Start the addon server
	if s.addonPath == "" {
		_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
		fd.Close()
		return fmt.Errorf("addon path not configured")
	}

	cmd := exec.CommandContext(ctx, s.addonPath)
	cmd.Env = os.Environ()
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		_ = syscall.Flock(int(fd.Fd()), syscall.LOCK_UN)
		fd.Close()
		return fmt.Errorf("failed to start addon: %w", err)
	}

	s.process = cmd.Process
	s.lockFd = fd // Keep the lock file open; addon will acquire its own lock

	// Monitor process in background
	go func() {
		_ = cmd.Wait()
		s.mu.Lock()
		s.process = nil
		if s.lockFd != nil {
			_ = syscall.Flock(int(s.lockFd.Fd()), syscall.LOCK_UN)
			s.lockFd.Close()
			s.lockFd = nil
		}
		s.mu.Unlock()
	}()

	// Wait for the server to become ready
	return s.waitForReady(ctx)
}

// waitForReady polls until the addon server is ready or context expires.
func (s *ServerLifecycle) waitForReady(ctx context.Context) error {
	timeout := 10 * time.Second
	deadline := time.Now().Add(timeout)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if time.Now().After(deadline) {
				return fmt.Errorf("timeout waiting for addon to start")
			}

			// Try to connect to the socket
			conn, err := net.DialTimeout("unix", s.socketPath, 100*time.Millisecond)
			if err == nil {
				_ = conn.Close()
				return nil
			}
		}
	}
}

// Stop sends a graceful shutdown request to the addon server.
// It first tries to shut down via gRPC, then falls back to SIGTERM.
func (s *ServerLifecycle) Stop(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.isRunningLocked() {
		return nil
	}

	// Try graceful shutdown via gRPC
	if err := s.shutdownViaGRPC(ctx); err == nil {
		// Give it a moment to clean up
		time.Sleep(500 * time.Millisecond)
		if !s.isRunningLocked() {
			return nil
		}
	}

	// Fall back to SIGTERM
	if s.process != nil {
		if err := s.process.Signal(syscall.SIGTERM); err != nil {
			// Process might have already exited
			if !isProcessDone(err) {
				return fmt.Errorf("failed to send SIGTERM: %w", err)
			}
		}

		// Wait briefly for shutdown
		time.Sleep(500 * time.Millisecond)
	}

	// Clean up lock file if we still hold it
	if s.lockFd != nil {
		_ = syscall.Flock(int(s.lockFd.Fd()), syscall.LOCK_UN)
		s.lockFd.Close()
		s.lockFd = nil
	}

	return nil
}

// shutdownViaGRPC attempts graceful shutdown via gRPC Shutdown RPC.
func (s *ServerLifecycle) shutdownViaGRPC(ctx context.Context) error {
	conn, err := grpc.NewClient(
		"unix://"+s.socketPath,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	client := pb.NewInferenceServiceClient(conn)
	_, err = client.Shutdown(ctx, &pb.ShutdownRequest{Graceful: true})
	return err
}

// isProcessDone checks if an error indicates the process has already finished.
func isProcessDone(err error) bool {
	if err == nil {
		return false
	}
	// On Unix, sending a signal to a finished process returns this error
	return err.Error() == "os: process already finished"
}
