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
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/userconfig"
	"github.com/tsukumogami/tsuku/internal/version"
)

// testRecipeLoader is a mock recipe loader for testing.
type testRecipeLoader struct {
	recipes map[string]*recipe.Recipe
	err     error
}

func (l *testRecipeLoader) LoadRecipe(_ context.Context, toolName string, _ *install.State, _ *config.Config) (*recipe.Recipe, error) {
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
	err := RunUpdateCheck(context.Background(), cfg, userCfg, loader)
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

	err := RunUpdateCheck(context.Background(), cfg, userCfg, loader)
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
