package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
)

// binaryCommandLookup is the boundary every command-to-recipe lookup in this
// package goes through. It exists so a test can put a known index behind
// `tsuku run` and see which recipe the wiring in cmd_run.go actually reaches
// -- until now that wiring was only exercisable by whatever index happened to
// be on the machine, so it had no test at all.
//
// It is a seam for the lookup, not a source of matches. A test replaces it
// with internal/indexfixture's Lookup; a test that instead returns a
// hand-written []index.BinaryMatch is building a multi-provider case outside
// the fixture, which TestMultiProviderCasesUseTheFixture rejects.
var binaryCommandLookup = lookupBinaryCommand

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
