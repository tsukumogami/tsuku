//go:build !windows

package shellenv

import (
	"fmt"
	"os"
	"syscall"
)

// acquireFileLock opens or creates the lock file and acquires an exclusive
// advisory lock using flock(2). Returns an unlock function that releases
// the lock and closes the file.
func acquireFileLock(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("opening lock file %s: %w", path, err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		return nil, fmt.Errorf("acquiring exclusive lock: %w", err)
	}

	unlock := func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}
	return unlock, nil
}
