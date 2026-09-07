package index

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// openTestIndexWithRegistry opens a fresh index with the given registry dir.
func openTestIndexWithRegistry(t *testing.T, registryDir string) BinaryIndex {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "binary-index.db")
	idx, err := Open(dbPath, registryDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

// buildIndexWithRows populates an index with the given rows via Rebuild.
func buildIndexWithRows(t *testing.T, idx BinaryIndex, recipes map[string][]byte, tools map[string]ToolInfo) {
	t.Helper()
	ctx := context.Background()
	reg := &stubRegistry{recipes: recipes}
	state := &stubState{tools: tools}
	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
}

// The ordering contract -- installed providers first, then recipe name
// ascending -- is exercised in lookup_ordering_test.go against the shared
// fixture index. It cannot live here: an in-package test file cannot import
// internal/indexfixture, which imports this package.

// TestLookup_NotFound verifies that a command with no matching rows returns an
// empty slice and nil error (never an error for "not found").
func TestLookup_NotFound(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	recipes := map[string][]byte{
		"jq": minimalRecipeTOML("bin/jq"),
	}
	buildIndexWithRows(t, idx, recipes, map[string]ToolInfo{})

	matches, err := idx.Lookup(ctx, "nonexistent-command-xyz")
	if err != nil {
		t.Fatalf("Lookup() error = %v, want nil for not-found", err)
	}
	if len(matches) != 0 {
		t.Errorf("Lookup() returned %d matches, want 0 for not-found", len(matches))
	}
}

// TestLookup_ErrIndexNotBuilt verifies that Lookup returns ErrIndexNotBuilt
// when the index has never been populated (meta table has no built_at row).
func TestLookup_ErrIndexNotBuilt(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	// Do not call Rebuild — the index is fresh with no built_at.
	_, err := idx.Lookup(ctx, "jq")
	if !errors.Is(err, ErrIndexNotBuilt) {
		t.Errorf("Lookup() error = %v, want ErrIndexNotBuilt", err)
	}
}

// TestLookup_StaleWarning verifies that Lookup returns results AND a
// StaleIndexWarning when the registry directory is newer than built_at.
// The results are not withheld due to staleness.
func TestLookup_StaleWarning(t *testing.T) {
	// Create a registry dir that will be used for staleness detection.
	registryDir := t.TempDir()

	idx := openTestIndexWithRegistry(t, registryDir)
	ctx := context.Background()

	recipes := map[string][]byte{
		"jq": minimalRecipeTOML("bin/jq"),
	}
	buildIndexWithRows(t, idx, recipes, map[string]ToolInfo{})

	// Force the index to appear stale by setting built_at to a time in the past
	// and then touching the registry dir to ensure its mtime is newer.
	si := idx.(*sqliteBinaryIndex)
	pastTime := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if _, err := si.db.Exec(
		`INSERT OR REPLACE INTO meta (key, value) VALUES ('built_at', ?)`,
		pastTime,
	); err != nil {
		t.Fatalf("set built_at to past: %v", err)
	}

	// Touch a file in the registry dir to ensure its mtime is after built_at.
	touchPath := filepath.Join(registryDir, "jq.toml")
	if err := os.WriteFile(touchPath, []byte("x"), 0600); err != nil {
		t.Fatalf("touch registry file: %v", err)
	}
	// Also update the dir mtime itself.
	now := time.Now()
	if err := os.Chtimes(registryDir, now, now); err != nil {
		t.Fatalf("chtimes registry dir: %v", err)
	}

	matches, err := idx.Lookup(ctx, "jq")

	// Results must be present even though the index is stale.
	if len(matches) == 0 {
		t.Error("Lookup() returned no matches, want at least one (staleness should not suppress results)")
	}

	// The error must be a StaleIndexWarning.
	var warning StaleIndexWarning
	if !errors.As(err, &warning) {
		t.Errorf("Lookup() error = %v, want StaleIndexWarning", err)
	}
}

// TestCheckStaleness_Stale verifies that CheckStaleness returns (true, nil)
// when the registry directory mtime is newer than built_at.
func TestCheckStaleness_Stale(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	// Rebuild to set built_at.
	buildIndexWithRows(t, idx, map[string][]byte{
		"jq": minimalRecipeTOML("bin/jq"),
	}, map[string]ToolInfo{})

	si := idx.(*sqliteBinaryIndex)

	// Set built_at to a time in the past.
	pastTime := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if _, err := si.db.ExecContext(ctx,
		`INSERT OR REPLACE INTO meta (key, value) VALUES ('built_at', ?)`,
		pastTime,
	); err != nil {
		t.Fatalf("set built_at to past: %v", err)
	}

	// Create a registry dir with a mtime that is clearly in the future relative
	// to built_at.
	registryDir := t.TempDir()
	now := time.Now()
	if err := os.Chtimes(registryDir, now, now); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	stale, err := CheckStaleness(ctx, si.db, registryDir)
	if err != nil {
		t.Fatalf("CheckStaleness() error = %v", err)
	}
	if !stale {
		t.Errorf("CheckStaleness() = false, want true (registry is newer than built_at)")
	}
}

// TestCheckStaleness_Current verifies that CheckStaleness returns (false, nil)
// when built_at is newer than the registry directory mtime.
func TestCheckStaleness_Current(t *testing.T) {
	registryDir := t.TempDir()

	// Set the registry dir mtime to a time well in the past.
	pastMtime := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(registryDir, pastMtime, pastMtime); err != nil {
		t.Fatalf("chtimes: %v", err)
	}

	idx := openTestIndex(t)

	// Rebuild so built_at is set to approximately now (which is after pastMtime).
	buildIndexWithRows(t, idx, map[string][]byte{
		"jq": minimalRecipeTOML("bin/jq"),
	}, map[string]ToolInfo{})

	si := idx.(*sqliteBinaryIndex)

	stale, err := CheckStaleness(context.Background(), si.db, registryDir)
	if err != nil {
		t.Fatalf("CheckStaleness() error = %v, want nil", err)
	}
	if stale {
		t.Errorf("CheckStaleness() = true, want false (built_at is newer than registry mtime)")
	}
}
