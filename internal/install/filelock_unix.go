//go:build !windows

package install

import (
	"fmt"
	"syscall"
)

// lockShared acquires a shared (read) lock using flock(2).
func (fl *FileLock) lockShared() error {
	if err := syscall.Flock(int(fl.file.Fd()), syscall.LOCK_SH); err != nil {
		return fmt.Errorf("failed to acquire shared lock: %w", err)
	}
	return nil
}

// lockExclusive acquires an exclusive (write) lock using flock(2).
func (fl *FileLock) lockExclusive() error {
	if err := syscall.Flock(int(fl.file.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}
	return nil
}

// unlock releases the flock.
func (fl *FileLock) unlock() error {
	if err := syscall.Flock(int(fl.file.Fd()), syscall.LOCK_UN); err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	return nil
}
