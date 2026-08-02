package install

import (
	"context"
	"fmt"
)

// CheckAndExposeHidden checks if a tool is installed as hidden and exposes it
// if requested. This is used when the user explicitly runs `tsuku install npm`.
// ctx is threaded through to ExposeHidden for cancellation hooks.
func CheckAndExposeHidden(ctx context.Context, mgr *Manager, toolName string) (bool, error) {
	hidden, err := IsHidden(mgr.config, toolName)
	if err != nil {
		return false, err
	}

	if !hidden {
		return false, nil
	}

	// Exposing means linking the binaries the entry records. An entry that
	// records none cannot be exposed -- createSymlinksForBinaries would fall
	// back to guessing bin/<toolname>, which is wrong for any tool whose
	// binary is named differently or lives at the tool root, and AtomicSymlink
	// does not check that its target exists. Reporting "exposed" there would
	// hand the user a dangling symlink and an install that did nothing.
	//
	// Saying no instead sends the caller down the normal install path, which
	// is the honest answer: the tool's payload is on disk but tsuku does not
	// know enough about it to put it on PATH.
	if !canExpose(mgr, toolName) {
		return false, nil
	}

	// Tool is hidden, expose it
	if err := ExposeHidden(ctx, mgr, toolName); err != nil {
		return false, fmt.Errorf("failed to expose hidden tool: %w", err)
	}

	return true, nil
}

// canExpose reports whether the tool's state records the binaries ExposeHidden
// would need to link.
func canExpose(mgr *Manager, toolName string) bool {
	ts, err := mgr.GetToolState(toolName)
	if err != nil || ts == nil {
		return false
	}
	if len(ts.Binaries) > 0 {
		return true
	}
	version := ts.ActiveVersion
	if version == "" {
		version = ts.Version
	}
	return len(ts.Versions[version].Binaries) > 0
}
