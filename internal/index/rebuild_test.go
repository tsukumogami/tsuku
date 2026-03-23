package index

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

// --- stub implementations for Registry and StateReader ---

// stubRegistry implements Registry for tests.
type stubRegistry struct {
	recipes map[string][]byte
	listErr error
	getErr  map[string]error
}

func (s *stubRegistry) ListCached() ([]string, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	names := make([]string, 0, len(s.recipes))
	for name := range s.recipes {
		names = append(names, name)
	}
	return names, nil
}

func (s *stubRegistry) GetCached(name string) ([]byte, error) {
	if s.getErr != nil {
		if err, ok := s.getErr[name]; ok {
			return nil, err
		}
	}
	data, ok := s.recipes[name]
	if !ok {
		return nil, fmt.Errorf("recipe %q not found", name)
	}
	return data, nil
}

// stubState implements StateReader for tests.
type stubState struct {
	tools   map[string]ToolInfo
	loadErr error
}

func (s *stubState) AllTools() (map[string]ToolInfo, error) {
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return s.tools, nil
}

// --- helpers ---

// minimalRecipeTOML returns a minimal recipe TOML that declares one binary.
func minimalRecipeTOML(binaryPath string) []byte {
	return []byte(fmt.Sprintf(`
[metadata]
name = "test"

[[steps]]
action = "install_binaries"
binaries = [%q]

[verify]
command = "test --version"
`, binaryPath))
}

// openTestIndex opens a fresh index in a temp dir.
func openTestIndex(t *testing.T) BinaryIndex {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "binary-index.db")
	idx, err := Open(dbPath)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	t.Cleanup(func() { _ = idx.Close() })
	return idx
}

// countRows returns the number of rows in the binaries table.
func countRows(t *testing.T, idx BinaryIndex) int {
	t.Helper()
	si := idx.(*sqliteBinaryIndex)
	var n int
	if err := si.db.QueryRow(`SELECT COUNT(*) FROM binaries`).Scan(&n); err != nil {
		t.Fatalf("count binaries rows: %v", err)
	}
	return n
}

// queryInstalledFlag returns the installed flag (0 or 1) for the given recipe.
// It fails the test if the recipe has no rows.
func queryInstalledFlag(t *testing.T, idx BinaryIndex, recipe string) int {
	t.Helper()
	si := idx.(*sqliteBinaryIndex)
	var installed int
	err := si.db.QueryRow(`SELECT installed FROM binaries WHERE recipe = ? LIMIT 1`, recipe).Scan(&installed)
	if err == sql.ErrNoRows {
		t.Fatalf("no rows in binaries for recipe %q", recipe)
	}
	if err != nil {
		t.Fatalf("query installed flag for %q: %v", recipe, err)
	}
	return installed
}

// querySourceFlag returns the source for the given recipe.
func querySourceFlag(t *testing.T, idx BinaryIndex, recipe string) string {
	t.Helper()
	si := idx.(*sqliteBinaryIndex)
	var source string
	err := si.db.QueryRow(`SELECT source FROM binaries WHERE recipe = ? LIMIT 1`, recipe).Scan(&source)
	if err == sql.ErrNoRows {
		t.Fatalf("no rows in binaries for recipe %q", recipe)
	}
	if err != nil {
		t.Fatalf("query source for %q: %v", recipe, err)
	}
	return source
}

// queryBuiltAt reads the built_at key from the meta table.
func queryBuiltAt(t *testing.T, idx BinaryIndex) string {
	t.Helper()
	si := idx.(*sqliteBinaryIndex)
	var v string
	err := si.db.QueryRow(`SELECT value FROM meta WHERE key = 'built_at'`).Scan(&v)
	if err == sql.ErrNoRows {
		t.Fatal("built_at key not found in meta table")
	}
	if err != nil {
		t.Fatalf("query built_at: %v", err)
	}
	return v
}

// --- tests ---

// TestRebuild_RowCounts verifies that Rebuild inserts one row per binary per
// recipe.
func TestRebuild_RowCounts(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &stubRegistry{
		recipes: map[string][]byte{
			"jq":  minimalRecipeTOML("bin/jq"),
			"rg":  minimalRecipeTOML("bin/rg"),
			"bat": minimalRecipeTOML("bin/bat"),
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	got := countRows(t, idx)
	if got != 3 {
		t.Errorf("row count = %d, want 3", got)
	}
}

// TestRebuild_InstalledFlag verifies that the installed flag is 1 for recipes
// with a non-empty ActiveVersion in the state and 0 for uninstalled recipes.
func TestRebuild_InstalledFlag(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &stubRegistry{
		recipes: map[string][]byte{
			"jq": minimalRecipeTOML("bin/jq"),
			"rg": minimalRecipeTOML("bin/rg"),
		},
	}
	state := &stubState{
		tools: map[string]ToolInfo{
			"jq": {ActiveVersion: "1.7.1"},
			// rg is not installed (no ActiveVersion)
		},
	}

	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if got := queryInstalledFlag(t, idx, "jq"); got != 1 {
		t.Errorf("jq installed flag = %d, want 1", got)
	}
	if got := queryInstalledFlag(t, idx, "rg"); got != 0 {
		t.Errorf("rg installed flag = %d, want 0", got)
	}
}

// TestRebuild_RegistrySource verifies that registry recipes get source = 'registry'.
func TestRebuild_RegistrySource(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &stubRegistry{
		recipes: map[string][]byte{
			"jq": minimalRecipeTOML("bin/jq"),
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if got := querySourceFlag(t, idx, "jq"); got != "registry" {
		t.Errorf("jq source = %q, want %q", got, "registry")
	}
}

// TestRebuild_InstalledSource verifies that tools with a local/distributed
// source (not in the registry) get source = 'installed'.
func TestRebuild_InstalledSource(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	// Empty registry — no cached recipes.
	reg := &stubRegistry{recipes: map[string][]byte{}}
	state := &stubState{
		tools: map[string]ToolInfo{
			"mytool": {
				ActiveVersion: "2.0.0",
				Source:        "local",
				Versions: map[string]VersionInfo{
					"2.0.0": {Binaries: []string{"bin/mytool"}},
				},
			},
		},
	}

	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	if got := querySourceFlag(t, idx, "mytool"); got != "installed" {
		t.Errorf("mytool source = %q, want %q", got, "installed")
	}
	if got := queryInstalledFlag(t, idx, "mytool"); got != 1 {
		t.Errorf("mytool installed flag = %d, want 1", got)
	}
}

// TestRebuild_Idempotent verifies that calling Rebuild twice produces the same
// number of rows.
func TestRebuild_Idempotent(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &stubRegistry{
		recipes: map[string][]byte{
			"jq":  minimalRecipeTOML("bin/jq"),
			"rg":  minimalRecipeTOML("bin/rg"),
			"bat": minimalRecipeTOML("bin/bat"),
		},
	}
	state := &stubState{
		tools: map[string]ToolInfo{
			"jq": {ActiveVersion: "1.7.1"},
		},
	}

	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("first Rebuild() error = %v", err)
	}
	firstCount := countRows(t, idx)

	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("second Rebuild() error = %v", err)
	}
	secondCount := countRows(t, idx)

	if firstCount != secondCount {
		t.Errorf("row count changed across rebuild calls: %d → %d", firstCount, secondCount)
	}

	// Installed flags should also be stable.
	if got := queryInstalledFlag(t, idx, "jq"); got != 1 {
		t.Errorf("jq installed flag after second rebuild = %d, want 1", got)
	}
}

// TestRebuild_MetaBuiltAt verifies that Rebuild sets a built_at key in the
// meta table containing a valid RFC3339 timestamp.
func TestRebuild_MetaBuiltAt(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &stubRegistry{
		recipes: map[string][]byte{
			"jq": minimalRecipeTOML("bin/jq"),
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	before := time.Now().UTC().Truncate(time.Second)
	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}
	after := time.Now().UTC().Add(time.Second)

	raw := queryBuiltAt(t, idx)
	parsed, err := time.Parse(time.RFC3339, raw)
	if err != nil {
		t.Fatalf("built_at value %q is not valid RFC3339: %v", raw, err)
	}
	if parsed.Before(before) || parsed.After(after) {
		t.Errorf("built_at %v is outside expected range [%v, %v]", parsed, before, after)
	}
}

// TestRebuild_Transactional verifies that a mid-run failure leaves no partial
// writes: the binaries table should remain empty (as it was before Rebuild).
func TestRebuild_Transactional(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	// The registry will fail when asked to retrieve "bad-recipe".
	reg := &stubRegistry{
		recipes: map[string][]byte{
			"jq":         minimalRecipeTOML("bin/jq"),
			"bad-recipe": minimalRecipeTOML("bin/bad"),
		},
		getErr: map[string]error{
			"bad-recipe": fmt.Errorf("simulated network error"),
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	err := idx.Rebuild(ctx, reg, state)
	if err == nil {
		t.Fatal("Rebuild() should have returned an error when a recipe fetch fails")
	}

	// The binaries table must be empty — no partial writes committed.
	got := countRows(t, idx)
	if got != 0 {
		t.Errorf("binaries table has %d rows after failed rebuild, want 0", got)
	}
}

// TestRebuild_MalformedRecipeSkipped verifies that a malformed TOML recipe is
// skipped with a warning rather than causing Rebuild to fail. Valid recipes
// processed before or after the malformed one must still appear in the index.
func TestRebuild_MalformedRecipeSkipped(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &stubRegistry{
		recipes: map[string][]byte{
			"jq":     minimalRecipeTOML("bin/jq"),
			"broken": []byte(`this is not valid toml ===`),
			"bat":    minimalRecipeTOML("bin/bat"),
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("Rebuild() error = %v, want nil (malformed recipe should be skipped)", err)
	}

	got := countRows(t, idx)
	if got != 2 {
		t.Errorf("row count = %d, want 2 (only valid recipes inserted)", got)
	}
}

// TestRebuild_MultipleBinariesPerRecipe verifies that recipes with multiple
// binaries produce one row per binary.
func TestRebuild_MultipleBinariesPerRecipe(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	// Recipe that exposes two binaries.
	toml := []byte(`
[metadata]
name = "multi"

[[steps]]
action = "install_binaries"
binaries = ["bin/foo", "bin/bar"]

[verify]
command = "foo --version"
`)
	reg := &stubRegistry{
		recipes: map[string][]byte{"multi": toml},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	got := countRows(t, idx)
	if got != 2 {
		t.Errorf("row count = %d, want 2 (one per binary)", got)
	}
}
