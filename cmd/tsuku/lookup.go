package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
)

// lookupBinaryCommand opens the binary index and looks up the given command,
// returning all matching recipes. It is network-free: it reads only the local
// SQLite index and must not transmit command names or results externally.
//
// Returns (nil, ErrIndexNotBuilt) if the index has never been populated.
// Returns results and a StaleIndexWarning if the index may be out of date;
// callers should print the warning but continue using the results.
// Returns (nil, error) for other failures (corrupt index, I/O errors, etc.).
func lookupBinaryCommand(ctx context.Context, cfg *config.Config, command string) ([]index.BinaryMatch, error) {
	dbPath := cfg.BinaryIndexPath()

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil, index.ErrIndexNotBuilt
	}

	idx, err := index.Open(dbPath, cfg.RegistryDir)
	if err != nil {
		return nil, fmt.Errorf("open binary index: %w", err)
	}
	defer func() { _ = idx.Close() }()

	matches, err := idx.Lookup(ctx, command)
	if err != nil {
		if errors.Is(err, index.ErrIndexNotBuilt) {
			return nil, index.ErrIndexNotBuilt
		}
		// StaleIndexWarning: results are still valid; return them with the warning.
		var stale index.StaleIndexWarning
		if errors.As(err, &stale) {
			return matches, err
		}
		return nil, fmt.Errorf("look up command: %w", err)
	}

	return matches, nil
}
