//go:build windows

package install

import (
	"fmt"

	"golang.org/x/sys/windows"
)

const (
	// lockfileExclusiveLock is the flag for exclusive lock
	lockfileExclusiveLock = 0x00000002
)

// lockShared acquires a shared (read) lock using LockFileEx.
func (fl *FileLock) lockShared() error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(fl.file.Fd()),
		0, // Shared lock (no LOCKFILE_EXCLUSIVE_LOCK flag)
		0,
		1,
		0,
		&overlapped,
	)
	if err != nil {
		return fmt.Errorf("failed to acquire shared lock: %w", err)
	}
	return nil
}

// lockExclusive acquires an exclusive (write) lock using LockFileEx.
func (fl *FileLock) lockExclusive() error {
	var overlapped windows.Overlapped
	err := windows.LockFileEx(
		windows.Handle(fl.file.Fd()),
		lockfileExclusiveLock,
		0,
		1,
		0,
		&overlapped,
	)
	if err != nil {
		return fmt.Errorf("failed to acquire exclusive lock: %w", err)
	}
	return nil
}

// unlock releases the lock using UnlockFileEx.
func (fl *FileLock) unlock() error {
	var overlapped windows.Overlapped
	err := windows.UnlockFileEx(
		windows.Handle(fl.file.Fd()),
		0,
		1,
		0,
		&overlapped,
	)
	if err != nil {
		return fmt.Errorf("failed to release lock: %w", err)
	}
	return nil
}
