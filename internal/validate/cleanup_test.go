package validate

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/log"
)

// mockLogger records log messages for testing.
type mockLogger struct {
	messages []string
}

func (m *mockLogger) Debug(msg string, args ...any) {
	m.messages = append(m.messages, msg)
}

func (m *mockLogger) Info(msg string, args ...any) {
	m.messages = append(m.messages, msg)
}

func (m *mockLogger) Warn(msg string, args ...any) {
	m.messages = append(m.messages, msg)
}

func (m *mockLogger) Error(msg string, args ...any) {
	m.messages = append(m.messages, msg)
}

func (m *mockLogger) With(args ...any) log.Logger {
	return m
}

func TestCleaner_CleanupTempDirs(t *testing.T) {
	// Create a temp directory for testing
	testDir := t.TempDir()

	// Create some tsuku-validate directories
	oldDir := filepath.Join(testDir, "tsuku-validate-old")
	newDir := filepath.Join(testDir, "tsuku-validate-new")
	otherDir := filepath.Join(testDir, "other-dir")

	if err := os.Mkdir(oldDir, 0755); err != nil {
		t.Fatalf("failed to create old dir: %v", err)
	}
	if err := os.Mkdir(newDir, 0755); err != nil {
		t.Fatalf("failed to create new dir: %v", err)
	}
	if err := os.Mkdir(otherDir, 0755); err != nil {
		t.Fatalf("failed to create other dir: %v", err)
	}

	// Set the old directory's modification time to 2 hours ago
	oldTime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(oldDir, oldTime, oldTime); err != nil {
		t.Fatalf("failed to set old dir time: %v", err)
	}

	// Create cleaner with test temp dir
	logger := &mockLogger{}
	cleaner := NewCleaner(nil, nil,
		WithTempDir(testDir),
		WithMaxTempDirAge(1*time.Hour),
		WithLogger(logger),
	)

	// Run cleanup
	ctx := context.Background()
	result := cleaner.CleanupWithResult(ctx)

	// Verify old directory was removed
	if _, err := os.Stat(oldDir); !os.IsNotExist(err) {
		t.Error("old directory should have been removed")
	}

	// Verify new directory still exists
	if _, err := os.Stat(newDir); err != nil {
		t.Error("new directory should still exist")
	}

	// Verify other directory still exists (not our prefix)
	if _, err := os.Stat(otherDir); err != nil {
		t.Error("other directory should still exist")
	}

	// Verify result
	if len(result.TempDirectories) != 1 {
		t.Errorf("expected 1 temp directory removed, got %d", len(result.TempDirectories))
	}
	if len(result.TempDirectories) > 0 && result.TempDirectories[0] != oldDir {
		t.Errorf("expected %s to be removed, got %s", oldDir, result.TempDirectories[0])
	}
}

func TestCleaner_CleanupTempDirs_NoOldDirs(t *testing.T) {
	testDir := t.TempDir()

	// Create only new directories
	newDir := filepath.Join(testDir, "tsuku-validate-new")
	if err := os.Mkdir(newDir, 0755); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}

	cleaner := NewCleaner(nil, nil,
		WithTempDir(testDir),
		WithMaxTempDirAge(1*time.Hour),
	)

	ctx := context.Background()
	result := cleaner.CleanupWithResult(ctx)

	// Verify no directories were removed
	if len(result.TempDirectories) != 0 {
		t.Errorf("expected 0 temp directories removed, got %d", len(result.TempDirectories))
	}

	// Verify directory still exists
	if _, err := os.Stat(newDir); err != nil {
		t.Error("new directory should still exist")
	}
}

func TestCleaner_CleanupTempDirs_EmptyDir(t *testing.T) {
	testDir := t.TempDir()

	cleaner := NewCleaner(nil, nil, WithTempDir(testDir))

	ctx := context.Background()
	result := cleaner.CleanupWithResult(ctx)

	// Should complete without errors
	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}
}

func TestCleaner_CleanupContainers_NoRuntime(t *testing.T) {
	// Create a detector that finds no runtime
	detector := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			return nil, errors.New("not found")
		},
	}

	logger := &mockLogger{}
	cleaner := NewCleaner(detector, nil, WithLogger(logger))

	ctx := context.Background()
	cleaner.Cleanup(ctx)

	// Should log that no runtime is available
	found := false
	for _, msg := range logger.messages {
		if msg == "no container runtime available for cleanup" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected log message about no runtime available")
	}
}

func TestCleaner_CleanupContainers_WithMockedRuntime(t *testing.T) {
	// Create a detector that finds podman
	detector := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			if name == "podman" {
				return "/usr/bin/podman", nil
			}
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if name == "/usr/bin/podman" && len(args) >= 1 && args[0] == "info" {
				return []byte("true\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
	}

	// Track executed commands
	var executedCommands [][]string
	mockExec := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		cmd := append([]string{name}, args...)
		executedCommands = append(executedCommands, cmd)

		// Return mock container list
		if len(args) >= 2 && args[0] == "ps" {
			return []byte("abc123\ndef456\n"), nil
		}

		// Return success for rm commands
		if len(args) >= 1 && args[0] == "rm" {
			return []byte(""), nil
		}

		return []byte(""), nil
	}

	logger := &mockLogger{}
	cleaner := NewCleaner(detector, nil, WithLogger(logger))
	cleaner.execCommand = mockExec

	ctx := context.Background()
	result := cleaner.CleanupWithResult(ctx)

	// Verify containers were "removed"
	if len(result.OrphanedContainers) != 2 {
		t.Errorf("expected 2 containers removed, got %d", len(result.OrphanedContainers))
	}

	// Verify rm commands were executed
	rmCount := 0
	for _, cmd := range executedCommands {
		if len(cmd) >= 2 && cmd[1] == "rm" {
			rmCount++
		}
	}
	if rmCount != 2 {
		t.Errorf("expected 2 rm commands, got %d", rmCount)
	}
}

func TestCleaner_CleanupContainers_LockedContainer(t *testing.T) {
	// Create a temp dir for lock manager
	testDir := t.TempDir()
	lockDir := filepath.Join(testDir, "locks")
	lockManager, err := NewLockManager(lockDir)
	if err != nil {
		t.Fatalf("failed to create lock manager: %v", err)
	}

	// Acquire a lock for one container
	lock, err := lockManager.Acquire("locked-container", false)
	if err != nil {
		t.Fatalf("failed to acquire lock: %v", err)
	}
	defer func() { _ = lock.Release() }()

	// Create detector
	detector := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			if name == "podman" {
				return "/usr/bin/podman", nil
			}
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if name == "/usr/bin/podman" && len(args) >= 1 && args[0] == "info" {
				return []byte("true\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
	}

	// Mock exec to return both locked and unlocked containers
	mockExec := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		if len(args) >= 2 && args[0] == "ps" {
			return []byte("locked-container\nunlocked-container\n"), nil
		}
		if len(args) >= 1 && args[0] == "rm" {
			return []byte(""), nil
		}
		return []byte(""), nil
	}

	logger := &mockLogger{}
	cleaner := NewCleaner(detector, lockManager, WithLogger(logger))
	cleaner.execCommand = mockExec

	ctx := context.Background()

	// Use Cleanup() to test logging path
	cleaner.Cleanup(ctx)

	// Verify log message about locked container
	found := false
	for _, msg := range logger.messages {
		if msg == "container is locked, skipping" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected log message about locked container being skipped")
	}

	// Also verify with CleanupWithResult that only unlocked container is removed
	// Reset lock state first
	_ = lock.Release()
	lock, err = lockManager.Acquire("locked-container", false)
	if err != nil {
		t.Fatalf("failed to reacquire lock: %v", err)
	}

	result := cleaner.CleanupWithResult(ctx)

	// Only the unlocked container should be removed
	if len(result.OrphanedContainers) != 1 {
		t.Errorf("expected 1 container removed (unlocked), got %d", len(result.OrphanedContainers))
	}
	if len(result.OrphanedContainers) > 0 && result.OrphanedContainers[0] != "unlocked-container" {
		t.Errorf("expected unlocked-container to be removed, got %s", result.OrphanedContainers[0])
	}
}

func TestCleaner_CleanupStaleLocks(t *testing.T) {
	testDir := t.TempDir()
	lockDir := filepath.Join(testDir, "locks")
	lockManager, err := NewLockManager(lockDir)
	if err != nil {
		t.Fatalf("failed to create lock manager: %v", err)
	}

	logger := &mockLogger{}
	// Use testDir as temp dir to avoid cleaning up leftover container dirs
	// from previous runs that may have permission issues
	cleaner := NewCleaner(nil, lockManager, WithLogger(logger), WithTempDir(testDir))

	// Just verify it doesn't crash with empty lock dir
	ctx := context.Background()
	result := cleaner.CleanupWithResult(ctx)

	if len(result.Errors) != 0 {
		t.Errorf("expected no errors, got %v", result.Errors)
	}
}

func TestCleaner_Cleanup_BestEffort(t *testing.T) {
	// Test that Cleanup() doesn't panic or block even with nil dependencies
	cleaner := NewCleaner(nil, nil)

	ctx := context.Background()

	// Should complete without panicking
	cleaner.Cleanup(ctx)
}

func TestCleanerOptions(t *testing.T) {
	testDir := t.TempDir()
	logger := &mockLogger{}

	cleaner := NewCleaner(nil, nil,
		WithTempDir(testDir),
		WithMaxTempDirAge(2*time.Hour),
		WithLogger(logger),
	)

	if cleaner.tempDir != testDir {
		t.Errorf("expected temp dir %s, got %s", testDir, cleaner.tempDir)
	}
	if cleaner.maxTempDirAge != 2*time.Hour {
		t.Errorf("expected max age 2h, got %v", cleaner.maxTempDirAge)
	}
}

func TestCleaner_ListOrphanedContainers_CommandError(t *testing.T) {
	detector := &RuntimeDetector{
		lookPath: func(name string) (string, error) {
			if name == "podman" {
				return "/usr/bin/podman", nil
			}
			return "", errors.New("not found")
		},
		cmdRun: func(ctx context.Context, name string, args ...string) ([]byte, error) {
			if name == "/usr/bin/podman" && len(args) >= 1 && args[0] == "info" {
				return []byte("true\n"), nil
			}
			return nil, errors.New("unexpected command")
		},
	}

	mockExec := func(ctx context.Context, name string, args ...string) ([]byte, error) {
		return nil, errors.New("command failed")
	}

	logger := &mockLogger{}
	cleaner := NewCleaner(detector, nil, WithLogger(logger))
	cleaner.execCommand = mockExec

	ctx := context.Background()
	result := cleaner.CleanupWithResult(ctx)

	// Should have an error from listing containers
	if len(result.Errors) == 0 {
		t.Error("expected error from failed container listing")
	}
}
