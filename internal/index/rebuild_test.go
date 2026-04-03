package index

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// --- stub implementations for Registry and StateReader ---

// stubRegistry implements Registry for tests.
//
// Cache and fetch paths are independent:
//   - recipes/getErr control GetCached (local cache)
//   - fetchRecipes/fetchErr control FetchRecipe (remote fetch)
//
// This matches the real Registry contract where GetCached and FetchRecipe
// are separate operations with separate failure modes. Tests for Issue 2's
// bounded-fetch path must use fetchRecipes and fetchErr, not recipes/getErr.
type stubRegistry struct {
	mu sync.Mutex
	// cache path
	recipes map[string][]byte
	listErr error
	getErr  map[string]error
	// fetch path (independent from cache)
	fetchRecipes map[string][]byte
	fetchErr     map[string]error
}

func (s *stubRegistry) ListCached() ([]string, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
	s.mu.Lock()
	defer s.mu.Unlock()
	data, ok := s.recipes[name]
	if !ok {
		return nil, nil // cache miss: matches *registry.Registry.GetCached contract
	}
	return data, nil
}

func (s *stubRegistry) ListAll(_ context.Context) ([]string, error) {
	return s.ListCached()
}

// FetchRecipe simulates a remote fetch. It uses the fetchRecipes and fetchErr
// maps, which are independent from the local cache (recipes/getErr).
func (s *stubRegistry) FetchRecipe(_ context.Context, name string) ([]byte, error) {
	if s.fetchErr != nil {
		if err, ok := s.fetchErr[name]; ok {
			return nil, err
		}
	}
	if s.fetchRecipes != nil {
		if data, ok := s.fetchRecipes[name]; ok {
			return data, nil
		}
	}
	return nil, fmt.Errorf("fetch: recipe %q not found in stub", name)
}

func (s *stubRegistry) CacheRecipe(_ string, _ []byte) error {
	return nil
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

// openTestIndex opens a fresh index in a temp dir with no registry dir
// (staleness detection disabled).
func openTestIndex(t *testing.T) BinaryIndex {
	t.Helper()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "binary-index.db")
	idx, err := Open(dbPath, "")
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

// TestRebuild_EmptyCacheWithManifest verifies that Rebuild produces index rows
// for all recipes when the local cache is empty but fetchRecipes is populated
// (simulating a manifest-driven cold-start where recipes must be fetched).
func TestRebuild_EmptyCacheWithManifest(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &stubRegistry{
		// Empty local cache — all GetCached calls return nil, nil.
		recipes: map[string][]byte{},
		// Remote fetch provides all three recipes.
		fetchRecipes: map[string][]byte{
			"jq":  minimalRecipeTOML("bin/jq"),
			"rg":  minimalRecipeTOML("bin/rg"),
			"bat": minimalRecipeTOML("bin/bat"),
		},
	}
	// Override ListAll to return names from fetchRecipes (simulates manifest).
	// We do this by populating a separate field and overriding ListCached via
	// a custom stub that uses fetchRecipes keys as the manifest.
	manifestReg := &manifestStubRegistry{stubRegistry: reg}
	state := &stubState{tools: map[string]ToolInfo{}}

	if err := idx.Rebuild(ctx, manifestReg, state); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	got := countRows(t, idx)
	if got != 3 {
		t.Errorf("row count = %d, want 3", got)
	}
}

// manifestStubRegistry wraps stubRegistry and overrides ListAll to return keys
// from fetchRecipes rather than recipes (simulating manifest enumeration).
type manifestStubRegistry struct {
	*stubRegistry
}

func (m *manifestStubRegistry) ListAll(_ context.Context) ([]string, error) {
	names := make([]string, 0, len(m.fetchRecipes))
	for name := range m.fetchRecipes {
		names = append(names, name)
	}
	return names, nil
}

// TestRebuild_PartialFetchError verifies that when FetchRecipe fails for one
// recipe, Rebuild still indexes the remaining recipes and returns nil.
func TestRebuild_PartialFetchError(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &manifestStubRegistry{
		stubRegistry: &stubRegistry{
			recipes: map[string][]byte{}, // empty cache
			fetchRecipes: map[string][]byte{
				"jq":  minimalRecipeTOML("bin/jq"),
				"rg":  minimalRecipeTOML("bin/rg"),
				"bat": minimalRecipeTOML("bin/bat"),
			},
			fetchErr: map[string]error{
				"rg": fmt.Errorf("network timeout"),
			},
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	err := idx.Rebuild(ctx, reg, state)
	if err != nil {
		t.Fatalf("Rebuild() error = %v, want nil (fetch errors should be skipped)", err)
	}

	got := countRows(t, idx)
	if got != 2 {
		t.Errorf("row count = %d, want 2 (jq and bat indexed, rg skipped)", got)
	}
}

// TestRebuild_WarmCacheNeverFetches verifies that when all recipes are in the
// local cache, FetchRecipe is never called. This is tested by setting fetchErr
// for all recipes: if FetchRecipe were called, Rebuild would skip those recipes
// and produce 0 rows instead of 3.
func TestRebuild_WarmCacheNeverFetches(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &stubRegistry{
		// All recipes in cache.
		recipes: map[string][]byte{
			"jq":  minimalRecipeTOML("bin/jq"),
			"rg":  minimalRecipeTOML("bin/rg"),
			"bat": minimalRecipeTOML("bin/bat"),
		},
		// If FetchRecipe is called for any recipe, it returns an error.
		fetchErr: map[string]error{
			"jq":  fmt.Errorf("should not be called"),
			"rg":  fmt.Errorf("should not be called"),
			"bat": fmt.Errorf("should not be called"),
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("Rebuild() error = %v", err)
	}

	got := countRows(t, idx)
	if got != 3 {
		t.Errorf("row count = %d, want 3 (warm cache should serve all recipes without fetching)", got)
	}
}

// cacheTrackingRegistry wraps stubRegistry and records CacheRecipe calls.
type cacheTrackingRegistry struct {
	*stubRegistry
	mu     sync.Mutex
	cached map[string][]byte
}

func (c *cacheTrackingRegistry) CacheRecipe(name string, data []byte) error {
	// Protect c.cached (concurrent CacheRecipe calls from the goroutine pool).
	c.mu.Lock()
	if c.cached == nil {
		c.cached = make(map[string][]byte)
	}
	c.cached[name] = data
	c.mu.Unlock()

	// Protect stubRegistry.recipes against concurrent GetCached reads.
	c.stubRegistry.mu.Lock()
	if c.stubRegistry.recipes == nil {
		c.stubRegistry.recipes = make(map[string][]byte)
	}
	c.stubRegistry.recipes[name] = data
	c.stubRegistry.mu.Unlock()

	return nil
}

func (c *cacheTrackingRegistry) ListAll(ctx context.Context) ([]string, error) {
	// Return names from fetchRecipes (manifest) for first call, then from
	// recipes (cache) for subsequent calls — but since the underlying
	// stubRegistry.ListAll just calls ListCached (from recipes), and we
	// populate recipes in CacheRecipe, this works naturally after the first run.
	names := make([]string, 0)
	seen := make(map[string]bool)
	for name := range c.fetchRecipes {
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	for name := range c.recipes {
		if !seen[name] {
			names = append(names, name)
			seen[name] = true
		}
	}
	return names, nil
}

// TestRebuild_CacheRecipeAfterFetch verifies that CacheRecipe is called after
// a successful FetchRecipe, and that a subsequent Rebuild uses only the cache
// (no more FetchRecipe calls).
func TestRebuild_CacheRecipeAfterFetch(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &cacheTrackingRegistry{
		stubRegistry: &stubRegistry{
			recipes: map[string][]byte{}, // empty cache initially
			fetchRecipes: map[string][]byte{
				"jq": minimalRecipeTOML("bin/jq"),
				"rg": minimalRecipeTOML("bin/rg"),
			},
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	// First Rebuild: cache is empty, recipes are fetched and cached.
	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("first Rebuild() error = %v", err)
	}

	if len(reg.cached) != 2 {
		t.Errorf("CacheRecipe called %d times after first Rebuild, want 2", len(reg.cached))
	}

	// Reset tracking, then set fetchErr to verify FetchRecipe is not called again.
	reg.cached = nil
	reg.stubRegistry.fetchErr = map[string]error{
		"jq": fmt.Errorf("should not be called on second rebuild"),
		"rg": fmt.Errorf("should not be called on second rebuild"),
	}

	// Second Rebuild: cache is warm, no fetching should occur.
	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("second Rebuild() error = %v", err)
	}

	if len(reg.cached) != 0 {
		t.Errorf("CacheRecipe called %d times on second Rebuild, want 0 (cache should be warm)", len(reg.cached))
	}

	got := countRows(t, idx)
	if got != 2 {
		t.Errorf("row count after second Rebuild = %d, want 2", got)
	}
}

// TestRebuild_DBUnavailable verifies that Rebuild returns an error when the
// underlying database is unavailable (e.g., closed connection).
func TestRebuild_DBUnavailable(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	reg := &stubRegistry{
		recipes: map[string][]byte{
			"jq": minimalRecipeTOML("bin/jq"),
			"rg": minimalRecipeTOML("bin/rg"),
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	// Close the underlying DB to simulate write errors during insert.
	si := idx.(*sqliteBinaryIndex)
	if err := si.db.Close(); err != nil {
		t.Fatalf("failed to close DB: %v", err)
	}

	err := idx.Rebuild(ctx, reg, state)
	if err == nil {
		t.Fatal("Rebuild() returned nil, want error (DB is closed)")
	}
}

// TestRebuild_DBWriteError verifies that a DB write error during the insert
// phase rolls back all inserts atomically. It installs a SQLite trigger that
// calls RAISE(ABORT) after the first successful INSERT, which aborts the
// entire transaction (including the preceding DELETE). After the error, the
// binaries table must contain 0 rows — no partial commit.
func TestRebuild_DBWriteError(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()
	si := idx.(*sqliteBinaryIndex)

	// Install a trigger that aborts after the first INSERT into binaries.
	// RAISE(ABORT) rolls back the enclosing transaction, undoing both the
	// DELETE and any partial INSERTs that preceded the failure.
	_, err := si.db.ExecContext(ctx, `
		CREATE TRIGGER fail_after_first AFTER INSERT ON binaries
		WHEN (SELECT COUNT(*) FROM binaries) >= 1
		BEGIN SELECT RAISE(ABORT, 'injected test failure'); END
	`)
	if err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	reg := &stubRegistry{
		recipes: map[string][]byte{
			"jq": minimalRecipeTOML("bin/jq"),
			"rg": minimalRecipeTOML("bin/rg"),
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	err = idx.Rebuild(ctx, reg, state)
	if err == nil {
		t.Fatal("Rebuild() returned nil, want error (trigger should abort the transaction)")
	}

	// The transaction must have rolled back atomically: 0 rows committed.
	got := countRows(t, idx)
	if got != 0 {
		t.Errorf("row count = %d, want 0 (all inserts must roll back on error)", got)
	}
}

// TestRebuild_PathTraversalRejected verifies that recipe names containing '/',
// '..', or null bytes are rejected with a warning and skipped before any URL
// or path construction occurs.
func TestRebuild_PathTraversalRejected(t *testing.T) {
	idx := openTestIndex(t)
	ctx := context.Background()

	invalidNames := []string{
		"../etc/passwd",
		"../../secret",
		"recipes/evil",
		"null\x00byte",
	}

	validRecipe := minimalRecipeTOML("bin/jq")

	// Provide invalid names via a custom ListAll; valid recipe "jq" should be indexed.
	reg := &customListAllRegistry{
		names: append(invalidNames, "jq"),
		stubRegistry: &stubRegistry{
			recipes: map[string][]byte{
				"jq": validRecipe,
			},
		},
	}
	state := &stubState{tools: map[string]ToolInfo{}}

	if err := idx.Rebuild(ctx, reg, state); err != nil {
		t.Fatalf("Rebuild() error = %v, want nil", err)
	}

	// Only jq should be indexed; all invalid names skipped.
	got := countRows(t, idx)
	if got != 1 {
		t.Errorf("row count = %d, want 1 (only jq; invalid names rejected)", got)
	}
}

// customListAllRegistry overrides ListAll to return a fixed set of names.
type customListAllRegistry struct {
	*stubRegistry
	names []string
}

func (c *customListAllRegistry) ListAll(_ context.Context) ([]string, error) {
	return c.names, nil
}
