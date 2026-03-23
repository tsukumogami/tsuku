package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

// CheckStaleness reports whether the binary index is out of date relative to
// the registry directory. It returns (true, nil) when registryDir has been
// modified more recently than the built_at timestamp stored in the meta table,
// (false, nil) when the index is current, and (false, err) on any read failure.
//
// A missing built_at key (index never built) is treated as a read failure and
// returns (false, ErrIndexNotBuilt).
func CheckStaleness(db *sql.DB, registryDir string) (bool, error) {
	var builtAtRaw string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = 'built_at'`).Scan(&builtAtRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, ErrIndexNotBuilt
	}
	if err != nil {
		return false, fmt.Errorf("index stale check: read built_at: %w", err)
	}

	builtAt, err := time.Parse(time.RFC3339, builtAtRaw)
	if err != nil {
		return false, fmt.Errorf("index stale check: parse built_at %q: %w", builtAtRaw, err)
	}

	registryMtime, err := registryDirMtime(registryDir)
	if err != nil {
		return false, fmt.Errorf("index stale check: stat registry dir: %w", err)
	}

	return registryMtime.After(builtAt), nil
}

// registryDirMtime returns the modification time of registryDir.
func registryDirMtime(registryDir string) (time.Time, error) {
	info, err := os.Stat(registryDir)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
