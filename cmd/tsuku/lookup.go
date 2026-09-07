package main

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/index"
)

// binaryCommandLookup is the lookup the `tsuku run` path goes through, and
// only that path -- newRunWiring is its one production caller. It exists so a
// test can put a known index behind that path and see which recipe the wiring
// reaches, which until now was only exercisable through whatever index
// happened to be on the machine.
//
// `tsuku which` and `tsuku suggest` call lookupBinaryCommand directly and are
// deliberately left alone: neither joins the index to the project
// configuration, so neither has the composition this variable exists to make
// testable. Swapping this variable does not redirect them, and a test that
// needs it to should route them through here first rather than assume it.
//
// It is a seam for the lookup, not a source of matches. A test replaces it
// with internal/indexfixture's Lookup; a test that instead returns a
// hand-written multi-provider []index.BinaryMatch literal is building a case
// outside the fixture, which TestMultiProviderCasesUseTheFixture rejects.
// That check reads composite literals of two or more elements and nothing
// else, so a slice assembled some other way passes it -- see the package
// comment in internal/indexfixture, which is about what to build rather than
// about what the check will catch.
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
