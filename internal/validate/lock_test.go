package validate

import (
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLockManager_NewLockManager(t *testing.T) {
	tempDir := t.TempDir()
	lockDir := filepath.Join(tempDir, "locks")

	m, err := NewLockManager(lockDir)
	if err != nil {
		t.Fatalf("NewLockManager() error = %v", err)
	}

	// Verify directory was created
	info, err := os.Stat(m.lockDir)
	if err != nil {
		t.Fatalf("lock directory not created: %v", err)
	}
	if !info.IsDir() {
		t.Error("lock path is not a directory")
	}
}

func TestLockManager_AcquireRelease(t *testing.T) {
	tempDir := t.TempDir()
	m, err := NewLockManager(filepath.Join(tempDir, "locks"))
	if err != nil {
		t.Fatalf("NewLockManager() error = %v", err)
	}

	containerID := "test-container-123"

	// Acquire lock
	lock, err := m.Acquire(containerID, false)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	// Verify container ID
	if lock.ContainerID() != containerID {
		t.Errorf("ContainerID() = %q, want %q", lock.ContainerID(), containerID)
	}

	// Verify lock file exists with metadata
	lockPath := m.lockPath(containerID)
	metadata, err := m.readLockMetadata(lockPath)
	if err != nil {
		t.Fatalf("readLockMetadata() error = %v", err)
	}
	if metadata.ContainerID != containerID {
		t.Errorf("metadata.ContainerID = %q, want %q", metadata.ContainerID, containerID)
	}
	if metadata.PID != os.Getpid() {
		t.Errorf("metadata.PID = %d, want %d", metadata.PID, os.Getpid())
	}

	// Release lock
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	// Verify lock file was removed
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Error("lock file should be removed after Release()")
	}
}

func TestLockManager_AcquireBlocking(t *testing.T) {
	tempDir := t.TempDir()
	m, err := NewLockManager(filepath.Join(tempDir, "locks"))
	if err != nil {
		t.Fatalf("NewLockManager() error = %v", err)
	}

	containerID := "test-container-blocking"

	// Acquire first lock
	lock1, err := m.Acquire(containerID, false)
	if err != nil {
		t.Fatalf("Acquire() first error = %v", err)
	}
	defer func() { _ = lock1.Release() }()

	// Try to acquire same lock non-blocking - should fail
	_, err = m.Acquire(containerID, false)
	if err != ErrLockBusy {
		t.Errorf("Acquire() non-blocking error = %v, want ErrLockBusy", err)
	}
}

func TestLockManager_AcquireBlockingWaits(t *testing.T) {
	tempDir := t.TempDir()
	m, err := NewLockManager(filepath.Join(tempDir, "locks"))
	if err != nil {
		t.Fatalf("NewLockManager() error = %v", err)
	}

	containerID := "test-container-wait"

	// Acquire first lock
	lock1, err := m.Acquire(containerID, false)
	if err != nil {
		t.Fatalf("Acquire() first error = %v", err)
	}

	var lock2Acquired atomic.Bool
	var wg sync.WaitGroup
	wg.Add(1)

	// Start goroutine that waits for lock
	go func() {
		defer wg.Done()
		lock2, err := m.Acquire(containerID, true) // blocking
		if err != nil {
			t.Errorf("Acquire() blocking error = %v", err)
			return
		}
		lock2Acquired.Store(true)
		_ = lock2.Release()
	}()

	// Give goroutine time to start waiting
	time.Sleep(50 * time.Millisecond)

	// Lock should not be acquired yet
	if lock2Acquired.Load() {
		t.Error("lock2 should not be acquired while lock1 is held")
	}

	// Release first lock
	if err := lock1.Release(); err != nil {
		t.Fatalf("Release() error = %v", err)
	}

	// Wait for goroutine to complete
	wg.Wait()

	// Now lock2 should have been acquired
	if !lock2Acquired.Load() {
		t.Error("lock2 should have been acquired after lock1 was released")
	}
}

func TestLockManager_DifferentContainers(t *testing.T) {
	tempDir := t.TempDir()
	m, err := NewLockManager(filepath.Join(tempDir, "locks"))
	if err != nil {
		t.Fatalf("NewLockManager() error = %v", err)
	}

	// Acquire locks for different containers - should succeed
	lock1, err := m.Acquire("container-1", false)
	if err != nil {
		t.Fatalf("Acquire(container-1) error = %v", err)
	}
	defer func() { _ = lock1.Release() }()

	lock2, err := m.Acquire("container-2", false)
	if err != nil {
		t.Fatalf("Acquire(container-2) error = %v", err)
	}
	defer func() { _ = lock2.Release() }()

	// Both locks should be held simultaneously
	if lock1.ContainerID() != "container-1" {
		t.Errorf("lock1.ContainerID() = %q, want %q", lock1.ContainerID(), "container-1")
	}
	if lock2.ContainerID() != "container-2" {
		t.Errorf("lock2.ContainerID() = %q, want %q", lock2.ContainerID(), "container-2")
	}
}

func TestLockManager_ListLocks(t *testing.T) {
	tempDir := t.TempDir()
	m, err := NewLockManager(filepath.Join(tempDir, "locks"))
	if err != nil {
		t.Fatalf("NewLockManager() error = %v", err)
	}

	// Initially empty
	locks, err := m.ListLocks()
	if err != nil {
		t.Fatalf("ListLocks() error = %v", err)
	}
	if len(locks) != 0 {
		t.Errorf("ListLocks() returned %d locks, want 0", len(locks))
	}

	// Acquire two locks
	lock1, err := m.Acquire("container-a", false)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer func() { _ = lock1.Release() }()

	lock2, err := m.Acquire("container-b", false)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	defer func() { _ = lock2.Release() }()

	// List should show two locks
	locks, err = m.ListLocks()
	if err != nil {
		t.Fatalf("ListLocks() error = %v", err)
	}
	if len(locks) != 2 {
		t.Errorf("ListLocks() returned %d locks, want 2", len(locks))
	}

	// Verify both containers are listed
	foundA, foundB := false, false
	for _, l := range locks {
		if l.ContainerID == "container-a" {
			foundA = true
		}
		if l.ContainerID == "container-b" {
			foundB = true
		}
	}
	if !foundA {
		t.Error("ListLocks() missing container-a")
	}
	if !foundB {
		t.Error("ListLocks() missing container-b")
	}
}

func TestLock_ReleaseIdempotent(t *testing.T) {
	tempDir := t.TempDir()
	m, err := NewLockManager(filepath.Join(tempDir, "locks"))
	if err != nil {
		t.Fatalf("NewLockManager() error = %v", err)
	}

	lock, err := m.Acquire("test-container", false)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}

	// First release
	if err := lock.Release(); err != nil {
		t.Fatalf("Release() first error = %v", err)
	}

	// Second release should be safe (no-op)
	if err := lock.Release(); err != nil {
		t.Errorf("Release() second error = %v, want nil", err)
	}
}

func TestLockManager_ConcurrentAcquire(t *testing.T) {
	tempDir := t.TempDir()
	m, err := NewLockManager(filepath.Join(tempDir, "locks"))
	if err != nil {
		t.Fatalf("NewLockManager() error = %v", err)
	}

	containerID := "contested-container"
	numGoroutines := 10
	numIterations := 5

	var successCount atomic.Int32
	var wg sync.WaitGroup

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < numIterations; j++ {
				lock, err := m.Acquire(containerID, true) // blocking
				if err != nil {
					t.Errorf("Acquire() error = %v", err)
					return
				}
				successCount.Add(1)
				// Small delay to simulate work
				time.Sleep(time.Millisecond)
				if err := lock.Release(); err != nil {
					t.Errorf("Release() error = %v", err)
					return
				}
			}
		}()
	}

	wg.Wait()

	expected := int32(numGoroutines * numIterations)
	if successCount.Load() != expected {
		t.Errorf("successCount = %d, want %d", successCount.Load(), expected)
	}
}

func TestLockManager_ListLocksEmptyDir(t *testing.T) {
	tempDir := t.TempDir()
	lockDir := filepath.Join(tempDir, "nonexistent", "locks")

	// Create manager but directory doesn't exist yet
	m := &LockManager{lockDir: lockDir}

	// ListLocks on non-existent directory should return empty, not error
	locks, err := m.ListLocks()
	if err != nil {
		t.Fatalf("ListLocks() error = %v", err)
	}
	if len(locks) != 0 {
		t.Errorf("ListLocks() returned %d locks, want 0", len(locks))
	}
}
