//go:build windows

package shellenv

import (
	"fmt"
	"os"

	"golang.org/x/sys/windows"
)

// acquireFileLock opens or creates the lock file and acquires an exclusive
// lock using LockFileEx. Returns an unlock function that releases the lock
// and closes the file.
func acquireFileLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file %s: %w", path, err)
	}

	var overlapped windows.Overlapped
	err = windows.LockFileEx(
		windows.Handle(f.Fd()),
		0x00000002, // LOCKFILE_EXCLUSIVE_LOCK
		0,
		1,
		0,
		&overlapped,
	)
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("acquiring exclusive lock: %w", err)
	}

	unlock := func() {
		var ov windows.Overlapped
		_ = windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &ov)
		f.Close()
	}
	return unlock, nil
}
