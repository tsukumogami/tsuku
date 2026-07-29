package updates

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/notices"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/userconfig"
	"github.com/tsukumogami/tsuku/internal/version"
)

// testRecipeLoader is a mock recipe loader for testing.
type testRecipeLoader struct {
	recipes map[string]*recipe.Recipe
	err     error

	// calls counts LoadRecipe invocations per tool name, for asserting how
	// many times a tool was actually checked.
	calls map[string]int
}

func (l *testRecipeLoader) LoadRecipe(_ context.Context, toolName string, _ *install.State, _ *config.Config) (*recipe.Recipe, error) {
	if l.calls == nil {
		l.calls = make(map[string]int)
	}
	l.calls[toolName]++

	if l.err != nil {
		return nil, l.err
	}
	r, ok := l.recipes[toolName]
	if !ok {
		return nil, fmt.Errorf("recipe not found for %s", toolName)
	}
	return r, nil
}

func TestCheckToolRecipeLoadFailure(t *testing.T) {
	tool := install.InstalledTool{
		Name:    "test-tool",
		Version: "1.0.0",
	}

	loader := &testRecipeLoader{
		recipes: map[string]*recipe.Recipe{},
	}

	state := &install.State{
		Installed: map[string]install.ToolState{
			"test-tool": {
				ActiveVersion: "1.0.0",
				Versions: map[string]install.VersionState{
					"1.0.0": {Requested: "1"},
				},
			},
		},
	}

	cfg := &config.Config{
		HomeDir: t.TempDir(),
	}
	res := version.New()
	factory := version.NewProviderFactory()

	ctx := context.Background()
	entry := checkTool(ctx, tool, "1", state, cfg, loader, res, factory)

	if entry.Tool != "test-tool" {
		t.Errorf("Tool = %q, want %q", entry.Tool, "test-tool")
	}
	if entry.ActiveVersion != "1.0.0" {
		t.Errorf("ActiveVersion = %q, want %q", entry.ActiveVersion, "1.0.0")
	}
	if entry.Requested != "1" {
		t.Errorf("Requested = %q, want %q", entry.Requested, "1")
	}
	if entry.Error == "" {
		t.Error("expected error in entry when recipe not found")
	}
}

func TestCheckToolRecipeError(t *testing.T) {
	tool := install.InstalledTool{
		Name:    "bad-tool",
		Version: "1.0.0",
	}

	loader := &testRecipeLoader{
		err: fmt.Errorf("recipe not found"),
	}

	state := &install.State{
		Installed: map[string]install.ToolState{},
	}

	cfg := &config.Config{HomeDir: t.TempDir()}
	res := version.New()
	factory := version.NewProviderFactory()

	entry := checkTool(context.Background(), tool, "", state, cfg, loader, res, factory)

	if entry.Error == "" {
		t.Error("expected error in entry for recipe failure")
	}
	if entry.Tool != "bad-tool" {
		t.Errorf("Tool = %q, want %q", entry.Tool, "bad-tool")
	}
}

func TestRunUpdateCheckDoubleCheck(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Touch sentinel to make cache fresh
	if err := TouchSentinel(cacheDir); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{HomeDir: dir}
	userCfg := userconfig.DefaultConfig()
	loader := &testRecipeLoader{recipes: map[string]*recipe.Recipe{}}

	// Should return immediately due to fresh sentinel (double-check)
	err := RunUpdateCheck(context.Background(), cfg, userCfg, loader, nil)
	if err != nil {
		t.Fatalf("RunUpdateCheck should succeed with fresh sentinel: %v", err)
	}
}

func TestRunUpdateCheckWritesEntries(t *testing.T) {
	dir := t.TempDir()
	cacheDir := filepath.Join(dir, "cache", "updates")

	// Create state and tools directories
	stateDir := filepath.Join(dir, "tools")
	if err := os.MkdirAll(stateDir, 0755); err != nil {
		t.Fatal(err)
	}

	// Write a minimal state.json
	stateFile := filepath.Join(dir, "state.json")
	if err := os.WriteFile(stateFile, []byte(`{"installed":{}}`), 0644); err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{
		HomeDir:  dir,
		ToolsDir: stateDir,
	}
	userCfg := userconfig.DefaultConfig()
	loader := &testRecipeLoader{recipes: map[string]*recipe.Recipe{}}

	err := RunUpdateCheck(context.Background(), cfg, userCfg, loader, nil)
	if err != nil {
		t.Fatalf("RunUpdateCheck: %v", err)
	}

	// Sentinel should exist after run
	sentinelPath := filepath.Join(cacheDir, SentinelFile)
	if _, err := os.Stat(sentinelPath); err != nil {
		t.Error("sentinel should exist after RunUpdateCheck")
	}

	// Cache should no longer be stale
	if IsCheckStale(cacheDir, 24*time.Hour) {
		t.Error("cache should be fresh after RunUpdateCheck")
	}
}

// TestRunUpdateCheckSkipsInactiveVersionDirectories guards against a
// regression to the mgr.List()-returns-one-row-per-version-directory bug:
// a tool with several old versions still on disk (pending garbage
// collection) must only be checked once, for its active version -- not once
// per retained directory. Each check makes real version-resolution network
// calls, so redundant checks needlessly multiply GitHub API usage and can
// starve other tools of their share of an unauthenticated rate limit.
func TestRunUpdateCheckSkipsInactiveVersionDirectories(t *testing.T) {
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, "tools")
	cfg := &config.Config{HomeDir: dir, ToolsDir: toolsDir}

	for _, v := range []string{"1.0.0", "2.0.0"} {
		if err := os.MkdirAll(cfg.ToolDir("multi", v), 0755); err != nil {
			t.Fatal(err)
		}
	}

	stateJSON := `{"installed":{"multi":{"active_version":"2.0.0","versions":{
		"1.0.0":{"requested":""},
		"2.0.0":{"requested":""}
	}}}}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(stateJSON), 0644); err != nil {
		t.Fatal(err)
	}

	userCfg := userconfig.DefaultConfig()
	loader := &testRecipeLoader{recipes: map[string]*recipe.Recipe{}}

	if err := RunUpdateCheck(context.Background(), cfg, userCfg, loader, nil); err != nil {
		t.Fatalf("RunUpdateCheck: %v", err)
	}

	if got := loader.calls["multi"]; got != 1 {
		t.Errorf("LoadRecipe called %d times for tool %q, want 1 (only the active version should be checked)", got, "multi")
	}
}

// TestRunUpdateCheckSurfacesPersistentCheckFailure guards the observability
// gap that let koto silently stop being considered for auto-update: a check
// error alone (e.g. a rate limit) is invisible -- IsPendingEntry excludes it
// from auto-apply and nothing else looks at the check-cache directory. A run
// of consecutive failures must surface via the standard notices mechanism so
// a stuck tool doesn't go unnoticed indefinitely.
func TestRunUpdateCheckSurfacesPersistentCheckFailure(t *testing.T) {
	dir := t.TempDir()
	toolsDir := filepath.Join(dir, "tools")
	cfg := &config.Config{HomeDir: dir, ToolsDir: toolsDir}

	if err := os.MkdirAll(cfg.ToolDir("flaky", "1.0.0"), 0755); err != nil {
		t.Fatal(err)
	}
	stateJSON := `{"installed":{"flaky":{"active_version":"1.0.0","versions":{"1.0.0":{"requested":""}}}}}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(stateJSON), 0644); err != nil {
		t.Fatal(err)
	}

	userCfg := userconfig.DefaultConfig()
	noticesDir := notices.NoticesDir(cfg.HomeDir)
	cacheDir := CacheDir(cfg.HomeDir)

	// Recipe load always fails: simulates a check that can never resolve
	// (e.g. GitHub rate limit) rather than a one-off blip. This needs no
	// network access -- checkTool returns before creating a version provider.
	failingLoader := &testRecipeLoader{err: fmt.Errorf("resolve within pin: rate limit exceeded")}

	for i := 1; i <= checkFailureNoticeThreshold; i++ {
		_ = os.Remove(filepath.Join(cacheDir, SentinelFile))
		if err := RunUpdateCheck(context.Background(), cfg, userCfg, failingLoader, nil); err != nil {
			t.Fatalf("RunUpdateCheck (attempt %d): %v", i, err)
		}

		n, err := notices.ReadNotice(noticesDir, "flaky")
		if err != nil {
			t.Fatalf("ReadNotice (attempt %d): %v", i, err)
		}
		if i < checkFailureNoticeThreshold {
			if n != nil {
				t.Errorf("attempt %d: expected no notice before threshold, got %+v", i, n)
			}
			continue
		}
		if n == nil {
			t.Fatalf("attempt %d: expected a KindCheckFailure notice at threshold, got none", i)
		}
		if n.Kind != notices.KindCheckFailure {
			t.Errorf("Kind = %q, want %q", n.Kind, notices.KindCheckFailure)
		}
		if n.ConsecutiveFailures != checkFailureNoticeThreshold {
			t.Errorf("ConsecutiveFailures = %d, want %d", n.ConsecutiveFailures, checkFailureNoticeThreshold)
		}
	}
}

// TestRecordConsecutiveCheckFailures exercises the escalate/recover state
// machine directly with synthetic entries, since driving a real successful
// check through RunUpdateCheck requires live version-resolution network
// access that unit tests can't rely on.
func TestRecordConsecutiveCheckFailures(t *testing.T) {
	dir := t.TempDir()
	cacheDir := CacheDir(dir)
	noticesDir := notices.NoticesDir(dir)

	// Fewer failures than the threshold: no notice yet, but the count persists.
	for i := 1; i < checkFailureNoticeThreshold; i++ {
		entry := &UpdateCheckEntry{Tool: "flaky", Error: "boom", CheckedAt: time.Now()}
		recordConsecutiveCheckFailures(cacheDir, noticesDir, entry)
		if entry.ConsecutiveCheckFailures != i {
			t.Fatalf("attempt %d: ConsecutiveCheckFailures = %d, want %d", i, entry.ConsecutiveCheckFailures, i)
		}
		if n, _ := notices.ReadNotice(noticesDir, "flaky"); n != nil {
			t.Fatalf("attempt %d: expected no notice below threshold, got %+v", i, n)
		}
		if err := WriteEntry(cacheDir, entry); err != nil {
			t.Fatalf("WriteEntry: %v", err)
		}
	}

	// The threshold-th failure escalates to a notice.
	entry := &UpdateCheckEntry{Tool: "flaky", Error: "boom", CheckedAt: time.Now()}
	recordConsecutiveCheckFailures(cacheDir, noticesDir, entry)
	if entry.ConsecutiveCheckFailures != checkFailureNoticeThreshold {
		t.Fatalf("ConsecutiveCheckFailures = %d, want %d", entry.ConsecutiveCheckFailures, checkFailureNoticeThreshold)
	}
	n, err := notices.ReadNotice(noticesDir, "flaky")
	if err != nil {
		t.Fatalf("ReadNotice: %v", err)
	}
	if n == nil || n.Kind != notices.KindCheckFailure {
		t.Fatalf("expected a KindCheckFailure notice at threshold, got %+v", n)
	}
	if err := WriteEntry(cacheDir, entry); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	// A successful check clears the standing notice and resets the counter.
	recovered := &UpdateCheckEntry{Tool: "flaky", Error: "", CheckedAt: time.Now()}
	recordConsecutiveCheckFailures(cacheDir, noticesDir, recovered)
	if recovered.ConsecutiveCheckFailures != 0 {
		t.Errorf("ConsecutiveCheckFailures = %d, want 0 after recovery", recovered.ConsecutiveCheckFailures)
	}
	if n, err := notices.ReadNotice(noticesDir, "flaky"); err != nil {
		t.Fatalf("ReadNotice: %v", err)
	} else if n != nil {
		t.Errorf("expected the check-failure notice to be cleared after a successful check, got %+v", n)
	}
}

// TestRecordConsecutiveCheckFailuresPreservesUnrelatedNotice guards against
// an over-eager clear: recovering from a check failure must not delete a
// different, still-pending notice for the same tool (e.g. an apply failure
// awaiting the user's review via `tsuku notices`).
func TestRecordConsecutiveCheckFailuresPreservesUnrelatedNotice(t *testing.T) {
	dir := t.TempDir()
	cacheDir := CacheDir(dir)
	noticesDir := notices.NoticesDir(dir)

	applyFailure := &notices.Notice{
		Tool:  "flaky",
		Error: "install failed: checksum mismatch",
		Kind:  notices.KindUpdateResult,
		Shown: false,
	}
	if err := notices.WriteNotice(noticesDir, applyFailure); err != nil {
		t.Fatalf("WriteNotice: %v", err)
	}

	entry := &UpdateCheckEntry{Tool: "flaky", Error: "", CheckedAt: time.Now()}
	recordConsecutiveCheckFailures(cacheDir, noticesDir, entry)

	n, err := notices.ReadNotice(noticesDir, "flaky")
	if err != nil {
		t.Fatalf("ReadNotice: %v", err)
	}
	if n == nil || n.Kind != notices.KindUpdateResult || n.Error != applyFailure.Error {
		t.Errorf("expected the unrelated apply-failure notice to survive, got %+v", n)
	}
}
