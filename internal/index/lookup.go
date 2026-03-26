package index

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// Lookup returns all recipes that provide command, ranked by preference:
// installed recipes first, then lexicographic by recipe name. Returns an empty
// slice and nil error when the command is not found. Returns ErrIndexNotBuilt
// when the index has never been populated (no built_at key in the meta table).
// When the index is stale, Lookup returns results and a StaleIndexWarning error.
// Use errors.As to inspect the warning: var w index.StaleIndexWarning; errors.As(err, &w)
func (idx *sqliteBinaryIndex) Lookup(ctx context.Context, command string) ([]BinaryMatch, error) {
	// Check whether the index has ever been built by reading built_at from meta.
	var builtAtRaw string
	err := idx.db.QueryRowContext(ctx,
		`SELECT value FROM meta WHERE key = 'built_at'`,
	).Scan(&builtAtRaw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrIndexNotBuilt
	}
	if err != nil {
		return nil, fmt.Errorf("index lookup: read built_at: %w", err)
	}

	// Query matching rows ordered by preference: installed DESC, recipe ASC.
	rows, err := idx.db.QueryContext(ctx,
		`SELECT command, recipe, binary_path, installed, source
		 FROM binaries
		 WHERE command = ?
		 ORDER BY installed DESC, recipe ASC`,
		command,
	)
	if err != nil {
		return nil, fmt.Errorf("index lookup: query: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var matches []BinaryMatch
	for rows.Next() {
		var m BinaryMatch
		var installedInt int
		if err := rows.Scan(&m.Command, &m.Recipe, &m.BinaryPath, &installedInt, &m.Source); err != nil {
			return nil, fmt.Errorf("index lookup: scan row: %w", err)
		}
		m.Installed = installedInt == 1
		matches = append(matches, m)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("index lookup: iterate rows: %w", err)
	}

	// Non-blocking staleness check. If stale, return results with the warning.
	if idx.registryDir != "" {
		stale, indexBuiltAt, registryAt, staleErr := checkStalenessDetails(ctx, idx.db, idx.registryDir)
		if staleErr == nil && stale {
			return matches, StaleIndexWarning{
				BuiltAt:    indexBuiltAt,
				RegistryAt: registryAt,
			}
		}
	}

	return matches, nil
}
