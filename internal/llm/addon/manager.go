// Package addon manages the tsuku-llm addon lifecycle.
package addon

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/tsukumogami/tsuku/internal/llm"
)

// Manager handles the lifecycle of the tsuku-llm addon.
type Manager struct {
	mu      sync.Mutex
	process *os.Process
	started bool
}

// NewManager creates a new addon manager.
func NewManager() *Manager {
	return &Manager{}
}

// EnsureRunning starts the addon if it's not already running.
// Returns a LocalProvider connected to the addon.
func (m *Manager) EnsureRunning(ctx context.Context) (*llm.LocalProvider, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already running
	if llm.IsAddonRunning() {
		return llm.NewLocalProvider(ctx)
	}

	// Check if addon is installed
	addonPath := AddonPath()
	if _, err := os.Stat(addonPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("tsuku-llm addon not installed at %s", addonPath)
	}

	// Start the addon
	if err := m.startAddon(ctx, addonPath); err != nil {
		return nil, fmt.Errorf("failed to start addon: %w", err)
	}

	// Wait for the addon to become ready
	if err := m.waitForReady(ctx); err != nil {
		return nil, fmt.Errorf("addon failed to become ready: %w", err)
	}

	return llm.NewLocalProvider(ctx)
}

// startAddon starts the tsuku-llm process.
func (m *Manager) startAddon(ctx context.Context, addonPath string) error {
	cmd := exec.CommandContext(ctx, addonPath)

	// Inherit environment
	cmd.Env = os.Environ()

	// Redirect stderr for debugging
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return err
	}

	m.process = cmd.Process
	m.started = true

	// Monitor the process in background
	go func() {
		_ = cmd.Wait()
		m.mu.Lock()
		m.started = false
		m.process = nil
		m.mu.Unlock()
	}()

	return nil
}

// waitForReady polls until the addon is ready or timeout.
func (m *Manager) waitForReady(ctx context.Context) error {
	timeout := time.After(10 * time.Second)
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timeout:
			return fmt.Errorf("timeout waiting for addon to start")
		case <-ticker.C:
			if llm.IsAddonRunning() {
				return nil
			}
		}
	}
}

// Shutdown stops the addon if it's running.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started || m.process == nil {
		return nil
	}

	// Try graceful shutdown via gRPC first
	if llm.IsAddonRunning() {
		provider, err := llm.NewLocalProvider(ctx)
		if err == nil {
			_ = provider.Shutdown(ctx, true)
			_ = provider.Close()

			// Wait briefly for graceful shutdown
			time.Sleep(500 * time.Millisecond)
		}
	}

	// Force kill if still running
	if m.process != nil {
		_ = m.process.Kill()
	}

	m.started = false
	m.process = nil
	return nil
}

// IsRunning returns whether the addon is currently running.
func (m *Manager) IsRunning() bool {
	return llm.IsAddonRunning()
}

// AddonPath returns the path to the tsuku-llm binary.
func AddonPath() string {
	home := os.Getenv("TSUKU_HOME")
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return ""
		}
		home = filepath.Join(userHome, ".tsuku")
	}

	binName := "tsuku-llm"
	if runtime.GOOS == "windows" {
		binName = "tsuku-llm.exe"
	}

	return filepath.Join(home, "tools", "tsuku-llm", binName)
}

// IsInstalled checks if the addon is installed.
func IsInstalled() bool {
	_, err := os.Stat(AddonPath())
	return err == nil
}
