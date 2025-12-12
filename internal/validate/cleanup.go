package validate

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/log"
)

// ContainerLabelPrefix is the label prefix used to identify tsuku validation containers.
const ContainerLabelPrefix = "tsuku-validate"

// TempDirPrefix is the prefix for temporary directories created during validation.
const TempDirPrefix = "tsuku-validate-"

// DefaultTempDirMaxAge is the maximum age for temp directories before cleanup.
const DefaultTempDirMaxAge = 1 * time.Hour

// Cleaner handles cleanup of orphaned validation artifacts.
// It removes:
// - Exited/dead containers with the tsuku-validate label
// - Temporary directories older than MaxTempDirAge
type Cleaner struct {
	detector      *RuntimeDetector
	lockManager   *LockManager
	tempDir       string
	maxTempDirAge time.Duration
	logger        log.Logger

	// For testing: allow overriding command execution
	execCommand func(ctx context.Context, name string, args ...string) ([]byte, error)
}

// CleanerOption configures a Cleaner.
type CleanerOption func(*Cleaner)

// WithLogger sets a logger for cleanup operations.
func WithLogger(logger log.Logger) CleanerOption {
	return func(c *Cleaner) {
		c.logger = logger
	}
}

// WithTempDir sets the temp directory to scan for orphaned directories.
func WithTempDir(dir string) CleanerOption {
	return func(c *Cleaner) {
		c.tempDir = dir
	}
}

// WithMaxTempDirAge sets the maximum age for temp directories.
func WithMaxTempDirAge(age time.Duration) CleanerOption {
	return func(c *Cleaner) {
		c.maxTempDirAge = age
	}
}

// NewCleaner creates a new Cleaner with the given dependencies.
func NewCleaner(detector *RuntimeDetector, lockManager *LockManager, opts ...CleanerOption) *Cleaner {
	c := &Cleaner{
		detector:      detector,
		lockManager:   lockManager,
		tempDir:       os.TempDir(),
		maxTempDirAge: DefaultTempDirMaxAge,
		logger:        log.NewNoop(),
		execCommand:   defaultExecCommand,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// defaultExecCommand executes a command and returns its output.
func defaultExecCommand(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.CombinedOutput()
}

// Cleanup performs all cleanup operations.
// This method is safe to call on startup - it never blocks and errors are logged
// but not returned (best-effort cleanup).
func (c *Cleaner) Cleanup(ctx context.Context) {
	// Clean up stale lock files first
	c.cleanupStaleLocks()

	// Clean up orphaned containers
	c.cleanupContainers(ctx)

	// Clean up old temp directories
	c.cleanupTempDirs()
}

// cleanupStaleLocks removes lock files for processes that no longer exist.
func (c *Cleaner) cleanupStaleLocks() {
	if c.lockManager == nil {
		return
	}

	cleaned, err := c.lockManager.TryCleanupStale()
	if err != nil {
		c.logger.Debug("failed to cleanup stale locks", "error", err)
		return
	}

	for _, containerID := range cleaned {
		c.logger.Debug("removed stale lock", "container_id", containerID)
	}
}

// cleanupContainers removes exited/dead containers with the tsuku-validate label.
func (c *Cleaner) cleanupContainers(ctx context.Context) {
	if c.detector == nil {
		return
	}

	runtime, err := c.detector.Detect(ctx)
	if err != nil {
		// No runtime available - nothing to clean up
		c.logger.Debug("no container runtime available for cleanup", "error", err)
		return
	}

	// List containers with our label
	containers, err := c.listOrphanedContainers(ctx, runtime.Name())
	if err != nil {
		c.logger.Debug("failed to list containers", "error", err)
		return
	}

	for _, containerID := range containers {
		// Try to acquire lock for this container (non-blocking)
		// If we can't acquire it, someone else is using it
		if c.lockManager != nil {
			lock, err := c.lockManager.Acquire(containerID, false)
			if err != nil {
				c.logger.Debug("container is locked, skipping", "container_id", containerID)
				continue
			}
			// Release lock after cleanup
			defer func(l *Lock) { _ = l.Release() }(lock)
		}

		// Remove the container
		if err := c.removeContainer(ctx, runtime.Name(), containerID); err != nil {
			c.logger.Debug("failed to remove container", "container_id", containerID, "error", err)
			continue
		}

		c.logger.Debug("removed orphaned container", "container_id", containerID)
	}
}

// listOrphanedContainers returns container IDs that are exited/dead with our label.
func (c *Cleaner) listOrphanedContainers(ctx context.Context, runtimeName string) ([]string, error) {
	// List containers with our label that are in exited or dead state
	// Format: just the container ID
	args := []string{
		"ps", "-a",
		"--filter", fmt.Sprintf("label=%s", ContainerLabelPrefix),
		"--filter", "status=exited",
		"--format", "{{.ID}}",
	}

	output, err := c.execCommand(ctx, runtimeName, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}

	// Also check for dead containers
	deadArgs := []string{
		"ps", "-a",
		"--filter", fmt.Sprintf("label=%s", ContainerLabelPrefix),
		"--filter", "status=dead",
		"--format", "{{.ID}}",
	}

	deadOutput, err := c.execCommand(ctx, runtimeName, deadArgs...)
	if err != nil {
		// Log but don't fail - exited containers were already found
		c.logger.Debug("failed to list dead containers", "error", err)
	}

	// Combine and dedupe results
	var containerIDs []string
	seen := make(map[string]bool)

	for _, line := range strings.Split(string(output), "\n") {
		id := strings.TrimSpace(line)
		if id != "" && !seen[id] {
			containerIDs = append(containerIDs, id)
			seen[id] = true
		}
	}

	for _, line := range strings.Split(string(deadOutput), "\n") {
		id := strings.TrimSpace(line)
		if id != "" && !seen[id] {
			containerIDs = append(containerIDs, id)
			seen[id] = true
		}
	}

	return containerIDs, nil
}

// removeContainer removes a container by ID.
func (c *Cleaner) removeContainer(ctx context.Context, runtimeName, containerID string) error {
	args := []string{"rm", "-f", containerID}
	_, err := c.execCommand(ctx, runtimeName, args...)
	return err
}

// cleanupTempDirs removes temporary directories older than MaxTempDirAge.
func (c *Cleaner) cleanupTempDirs() {
	entries, err := os.ReadDir(c.tempDir)
	if err != nil {
		c.logger.Debug("failed to read temp directory", "path", c.tempDir, "error", err)
		return
	}

	cutoff := time.Now().Add(-c.maxTempDirAge)

	for _, entry := range entries {
		// Only look at directories with our prefix
		if !entry.IsDir() {
			continue
		}
		if !strings.HasPrefix(entry.Name(), TempDirPrefix) {
			continue
		}

		path := filepath.Join(c.tempDir, entry.Name())

		// Check modification time
		info, err := entry.Info()
		if err != nil {
			c.logger.Debug("failed to stat temp directory", "path", path, "error", err)
			continue
		}

		if info.ModTime().After(cutoff) {
			// Directory is too new, skip it
			continue
		}

		// Remove the old directory
		if err := os.RemoveAll(path); err != nil {
			c.logger.Debug("failed to remove temp directory", "path", path, "error", err)
			continue
		}

		c.logger.Debug("removed old temp directory", "path", path, "age", time.Since(info.ModTime()))
	}
}

// CleanupResult contains information about what was cleaned up.
// This is useful for testing and monitoring.
type CleanupResult struct {
	StaleLocks         []string
	OrphanedContainers []string
	TempDirectories    []string
	Errors             []error
}

// CleanupWithResult performs cleanup and returns detailed results.
// Unlike Cleanup(), this returns errors and what was cleaned.
func (c *Cleaner) CleanupWithResult(ctx context.Context) *CleanupResult {
	result := &CleanupResult{}

	// Clean up stale lock files
	if c.lockManager != nil {
		cleaned, err := c.lockManager.TryCleanupStale()
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("stale lock cleanup: %w", err))
		} else {
			result.StaleLocks = cleaned
		}
	}

	// Clean up orphaned containers
	if c.detector != nil {
		runtime, err := c.detector.Detect(ctx)
		if err == nil {
			containers, err := c.listOrphanedContainers(ctx, runtime.Name())
			if err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("list containers: %w", err))
			} else {
				for _, containerID := range containers {
					// Try to acquire lock (non-blocking)
					var lock *Lock
					if c.lockManager != nil {
						lock, err = c.lockManager.Acquire(containerID, false)
						if err != nil {
							continue // Container is locked
						}
					}

					if err := c.removeContainer(ctx, runtime.Name(), containerID); err != nil {
						result.Errors = append(result.Errors, fmt.Errorf("remove container %s: %w", containerID, err))
					} else {
						result.OrphanedContainers = append(result.OrphanedContainers, containerID)
					}

					if lock != nil {
						_ = lock.Release()
					}
				}
			}
		}
	}

	// Clean up old temp directories
	entries, err := os.ReadDir(c.tempDir)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Errorf("read temp directory: %w", err))
	} else {
		cutoff := time.Now().Add(-c.maxTempDirAge)
		for _, entry := range entries {
			if !entry.IsDir() || !strings.HasPrefix(entry.Name(), TempDirPrefix) {
				continue
			}

			path := filepath.Join(c.tempDir, entry.Name())
			info, err := entry.Info()
			if err != nil {
				continue
			}

			if info.ModTime().After(cutoff) {
				continue
			}

			if err := os.RemoveAll(path); err != nil {
				result.Errors = append(result.Errors, fmt.Errorf("remove temp dir %s: %w", path, err))
			} else {
				result.TempDirectories = append(result.TempDirectories, path)
			}
		}
	}

	return result
}

// Discard is an io.Writer that discards all written data.
// It's used when no logger is configured.
var Discard io.Writer = io.Discard
