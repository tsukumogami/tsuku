package install

import (
	"fmt"
	"os"
)

// FileLock provides advisory file locking for cross-process synchronization.
// The lock is automatically released when the file is closed or the process exits.
type FileLock struct {
	path string
	file *os.File
}

// NewFileLock creates a new FileLock for the given path.
// The lock file will be created if it doesn't exist.
func NewFileLock(path string) *FileLock {
	return &FileLock{path: path}
}

// LockShared acquires a shared (read) lock on the file.
// Multiple processes can hold shared locks simultaneously.
// Blocks until the lock is acquired.
func (fl *FileLock) LockShared() error {
	if err := fl.openFile(); err != nil {
		return err
	}
	return fl.lockShared()
}

// LockExclusive acquires an exclusive (write) lock on the file.
// Only one process can hold an exclusive lock, and it blocks shared locks.
// Blocks until the lock is acquired.
func (fl *FileLock) LockExclusive() error {
	if err := fl.openFile(); err != nil {
		return err
	}
	return fl.lockExclusive()
}

// Unlock releases the lock and closes the file.
func (fl *FileLock) Unlock() error {
	if fl.file == nil {
		return nil
	}
	if err := fl.unlock(); err != nil {
		fl.file.Close()
		fl.file = nil
		return err
	}
	err := fl.file.Close()
	fl.file = nil
	return err
}

// openFile opens or creates the lock file.
func (fl *FileLock) openFile() error {
	if fl.file != nil {
		return nil // Already open
	}
	f, err := os.OpenFile(fl.path, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return fmt.Errorf("failed to open lock file %s: %w", fl.path, err)
	}
	fl.file = f
	return nil
}
