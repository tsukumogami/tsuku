package validate

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// LockManager manages lock files for parallel container validation.
// Lock files prevent interference between concurrent tsuku instances.
type LockManager struct {
	lockDir string
}

// LockMetadata contains information about the lock holder for debugging.
type LockMetadata struct {
	ContainerID string    `json:"container_id"`
	PID         int       `json:"pid"`
	AcquiredAt  time.Time `json:"acquired_at"`
}

// Lock represents an acquired lock on a container.
type Lock struct {
	file     *os.File
	path     string
	metadata LockMetadata
}

// NewLockManager creates a LockManager with the given lock directory.
// The directory is created if it doesn't exist.
func NewLockManager(lockDir string) (*LockManager, error) {
	if err := os.MkdirAll(lockDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create lock directory: %w", err)
	}
	return &LockManager{lockDir: lockDir}, nil
}

// Acquire attempts to acquire an exclusive lock for the given container ID.
// If blocking is true, waits until the lock is available.
// If blocking is false, returns ErrLockBusy immediately if the lock is held.
func (m *LockManager) Acquire(containerID string, blocking bool) (*Lock, error) {
	lockPath := m.lockPath(containerID)

	// Create or open the lock file
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open lock file: %w", err)
	}

	// Determine flock flags
	flockFlags := syscall.LOCK_EX
	if !blocking {
		flockFlags |= syscall.LOCK_NB
	}

	// Acquire exclusive lock
	if err := syscall.Flock(int(file.Fd()), flockFlags); err != nil {
		file.Close()
		if err == syscall.EWOULDBLOCK {
			return nil, ErrLockBusy
		}
		return nil, fmt.Errorf("failed to acquire lock: %w", err)
	}

	// Write metadata to lock file
	metadata := LockMetadata{
		ContainerID: containerID,
		PID:         os.Getpid(),
		AcquiredAt:  time.Now(),
	}

	// Truncate and write new metadata
	if err := file.Truncate(0); err != nil {
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()
		return nil, fmt.Errorf("failed to truncate lock file: %w", err)
	}

	if _, err := file.Seek(0, 0); err != nil {
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()
		return nil, fmt.Errorf("failed to seek lock file: %w", err)
	}

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(metadata); err != nil {
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()
		return nil, fmt.Errorf("failed to write lock metadata: %w", err)
	}

	return &Lock{
		file:     file,
		path:     lockPath,
		metadata: metadata,
	}, nil
}

// Release releases the lock and removes the lock file.
func (l *Lock) Release() error {
	if l.file == nil {
		return nil
	}

	// Unlock and close file
	err := syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
	closeErr := l.file.Close()
	l.file = nil

	// Remove lock file
	removeErr := os.Remove(l.path)

	// Return first error encountered
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	if closeErr != nil {
		return fmt.Errorf("failed to close lock file: %w", closeErr)
	}
	if removeErr != nil && !os.IsNotExist(removeErr) {
		return fmt.Errorf("failed to remove lock file: %w", removeErr)
	}

	return nil
}

// ContainerID returns the container ID this lock is for.
func (l *Lock) ContainerID() string {
	return l.metadata.ContainerID
}

// ListLocks returns metadata for all lock files in the lock directory.
// This is useful for cleanup operations to identify potentially orphaned locks.
func (m *LockManager) ListLocks() ([]LockMetadata, error) {
	entries, err := os.ReadDir(m.lockDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read lock directory: %w", err)
	}

	var locks []LockMetadata
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) != ".lock" {
			continue
		}

		metadata, err := m.readLockMetadata(filepath.Join(m.lockDir, entry.Name()))
		if err != nil {
			// Skip unreadable lock files
			continue
		}
		locks = append(locks, metadata)
	}

	return locks, nil
}

// TryCleanupStale attempts to remove lock files for processes that no longer exist.
// Returns the list of container IDs that were cleaned up.
func (m *LockManager) TryCleanupStale() ([]string, error) {
	locks, err := m.ListLocks()
	if err != nil {
		return nil, err
	}

	var cleaned []string
	for _, lock := range locks {
		// Check if the process is still running
		if isProcessRunning(lock.PID) {
			continue
		}

		// Process is dead, try to clean up the lock file
		lockPath := m.lockPath(lock.ContainerID)

		// Try to acquire the lock non-blocking to verify it's truly orphaned
		file, err := os.OpenFile(lockPath, os.O_RDWR, 0644)
		if err != nil {
			continue
		}

		if err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			file.Close()
			continue // Still locked by something
		}

		// Successfully acquired - this was orphaned
		syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
		file.Close()

		if err := os.Remove(lockPath); err == nil {
			cleaned = append(cleaned, lock.ContainerID)
		}
	}

	return cleaned, nil
}

// lockPath returns the path to the lock file for a container ID.
func (m *LockManager) lockPath(containerID string) string {
	return filepath.Join(m.lockDir, containerID+".lock")
}

// readLockMetadata reads the metadata from a lock file.
func (m *LockManager) readLockMetadata(path string) (LockMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return LockMetadata{}, err
	}

	var metadata LockMetadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return LockMetadata{}, err
	}

	return metadata, nil
}

// isProcessRunning checks if a process with the given PID is still running.
func isProcessRunning(pid int) bool {
	// On Unix, sending signal 0 checks if process exists without affecting it
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// Signal 0 checks existence without sending a real signal
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// ErrLockBusy is returned when a non-blocking lock acquisition fails
// because the lock is already held by another process.
var ErrLockBusy = fmt.Errorf("lock is busy")
