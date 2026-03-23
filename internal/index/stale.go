package index

import (
	"context"
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
func CheckStaleness(ctx context.Context, db *sql.DB, registryDir string) (bool, error) {
	stale, _, _, err := checkStalenessDetails(ctx, db, registryDir)
	return stale, err
}

// checkStalenessDetails is the internal implementation of staleness detection.
// It returns the stale flag, the parsed built_at time, the registry directory
// mtime, and any error. Both time values are zero on error. Lookup uses this
// directly to avoid a second stat call when populating StaleIndexWarning.
func checkStalenessDetails(ctx context.Context, db *sql.DB, registryDir string) (stale bool, builtAt time.Time, registryAt time.Time, err error) {
	var builtAtRaw string
	err = db.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = 'built_at'`).Scan(&builtAtRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return false, time.Time{}, time.Time{}, ErrIndexNotBuilt
	}
	if err != nil {
		return false, time.Time{}, time.Time{}, fmt.Errorf("index stale check: read built_at: %w", err)
	}

	builtAt, err = time.Parse(time.RFC3339, builtAtRaw)
	if err != nil {
		return false, time.Time{}, time.Time{}, fmt.Errorf("index stale check: parse built_at %q: %w", builtAtRaw, err)
	}

	registryAt, err = registryDirMtime(registryDir)
	if err != nil {
		return false, time.Time{}, time.Time{}, fmt.Errorf("index stale check: stat registry dir: %w", err)
	}

	return registryAt.After(builtAt), builtAt, registryAt, nil
}

// registryDirMtime returns the modification time of registryDir.
func registryDirMtime(registryDir string) (time.Time, error) {
	info, err := os.Stat(registryDir)
	if err != nil {
		return time.Time{}, err
	}
	return info.ModTime(), nil
}
