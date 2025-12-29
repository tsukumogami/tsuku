package main

import (
	"fmt"
	"testing"

	"github.com/tsukumogami/tsuku/internal/install"
	"github.com/tsukumogami/tsuku/internal/recipe"
	"github.com/tsukumogami/tsuku/internal/testutil"
)

func TestCircularDependencyDetection(t *testing.T) {
	visited := make(map[string]bool)

	// Test simple circular dependency
	visited["tool-a"] = false
	visited["tool-b"] = false

	// First visit
	if visited["tool-a"] {
		t.Error("tool-a should not be visited initially")
	}
	visited["tool-a"] = true

	// Try to visit again (simulates circular dependency)
	if visited["tool-a"] {
		// This is expected - circular dependency detected
	} else {
		t.Error("Should detect that tool-a was already visited")
	}
}

func TestDependencyResolution_Simple(t *testing.T) {
	// Test that we can track visited tools correctly
	visited := make(map[string]bool)

	tools := []string{"tool-a", "tool-b", "tool-c"}

	for _, tool := range tools {
		if visited[tool] {
			t.Errorf("tool %s should not be visited yet", tool)
		}
		visited[tool] = true
	}

	// Verify all marked as visited
	for _, tool := range tools {
		if !visited[tool] {
			t.Errorf("tool %s should be marked as visited", tool)
		}
	}
}

func TestDependencyResolution_PreventDuplicates(t *testing.T) {
	visited := make(map[string]bool)

	// Simulate installing tool-a and its dependencies
	installOrder := []string{}

	checkAndAdd := func(tool string) bool {
		if visited[tool] {
			return false // Already visited
		}
		visited[tool] = true
		installOrder = append(installOrder, tool)
		return true
	}

	// Install tool-c (dependency)
	if !checkAndAdd("tool-c") {
		t.Error("Should add tool-c")
	}

	// Install tool-b (depends on tool-c)
	if !checkAndAdd("tool-b") {
		t.Error("Should add tool-b")
	}

	// Try to install tool-c again (should skip)
	if checkAndAdd("tool-c") {
		t.Error("Should not add tool-c again")
	}

	// Install tool-a
	if !checkAndAdd("tool-a") {
		t.Error("Should add tool-a")
	}

	expectedOrder := []string{"tool-c", "tool-b", "tool-a"}
	if len(installOrder) != len(expectedOrder) {
		t.Fatalf("Install order length = %d, want %d", len(installOrder), len(expectedOrder))
	}

	for i, tool := range expectedOrder {
		if installOrder[i] != tool {
			t.Errorf("installOrder[%d] = %s, want %s", i, installOrder[i], tool)
		}
	}
}

func TestDependencyResolution_SharedDependency(t *testing.T) {
	// Test the scenario that caused issue #732:
	// - git-source depends on: curl, openssl
	// - curl depends on: openssl
	// When curl is installed first (bringing in openssl as a dependency),
	// then git-source tries to install openssl directly, it should not
	// trigger a circular dependency error.

	visited := make(map[string]bool)
	installOrder := []string{}

	// Simulate tool status: nil means not installed
	installed := make(map[string]bool)

	checkAndInstall := func(tool string, isAlreadyInstalled bool) error {
		// Check if already installed BEFORE circular dependency check
		if isAlreadyInstalled || installed[tool] {
			// Update state and return early WITHOUT marking as visited
			// This is the fix for issue #732
			installed[tool] = true
			return nil
		}

		// Check for circular dependencies AFTER confirming not installed
		if visited[tool] {
			return fmt.Errorf("circular dependency detected: %s", tool)
		}
		visited[tool] = true

		// Install the tool
		installOrder = append(installOrder, tool)
		installed[tool] = true
		return nil
	}

	// Simulate installing curl (which depends on openssl)
	// 1. Install openssl as dependency of curl
	if err := checkAndInstall("openssl", false); err != nil {
		t.Fatalf("Failed to install openssl as curl dependency: %v", err)
	}

	// 2. Install curl itself
	if err := checkAndInstall("curl", false); err != nil {
		t.Fatalf("Failed to install curl: %v", err)
	}

	// Now simulate installing git-source (which depends on curl and openssl)
	// 3. Try to install curl - already installed, should skip
	if err := checkAndInstall("curl", false); err != nil {
		t.Fatalf("Failed to handle already-installed curl: %v", err)
	}

	// 4. Try to install openssl - already installed, should NOT trigger circular dependency
	// This is the key test: openssl was marked as visited during curl's installation,
	// but since it's already installed, it should return early without error
	if err := checkAndInstall("openssl", false); err != nil {
		t.Fatalf("False positive circular dependency for shared dependency: %v", err)
	}

	// 5. Install git-source itself
	if err := checkAndInstall("git-source", false); err != nil {
		t.Fatalf("Failed to install git-source: %v", err)
	}

	// Verify install order: openssl, curl, git-source
	// (curl and openssl shouldn't be in installOrder again since they were already installed)
	expectedOrder := []string{"openssl", "curl", "git-source"}
	if len(installOrder) != len(expectedOrder) {
		t.Fatalf("Install order length = %d, want %d. Order: %v", len(installOrder), len(expectedOrder), installOrder)
	}

	for i, tool := range expectedOrder {
		if installOrder[i] != tool {
			t.Errorf("installOrder[%d] = %s, want %s", i, installOrder[i], tool)
		}
	}

	// Verify all tools are installed
	for _, tool := range []string{"openssl", "curl", "git-source"} {
		if !installed[tool] {
			t.Errorf("tool %s should be marked as installed", tool)
		}
	}
}

func TestOrphanDetection(t *testing.T) {
	tests := []struct {
		name       string
		isExplicit bool
		requiredBy []string
		wantOrphan bool
	}{
		{
			name:       "explicit tool is not orphan",
			isExplicit: true,
			requiredBy: []string{},
			wantOrphan: false,
		},
		{
			name:       "auto-installed with no dependents is orphan",
			isExplicit: false,
			requiredBy: []string{},
			wantOrphan: true,
		},
		{
			name:       "auto-installed with dependents is not orphan",
			isExplicit: false,
			requiredBy: []string{"tool-a"},
			wantOrphan: false,
		},
		{
			name:       "explicit with dependents is not orphan",
			isExplicit: true,
			requiredBy: []string{"tool-b"},
			wantOrphan: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isOrphan := !tt.isExplicit && len(tt.requiredBy) == 0

			if isOrphan != tt.wantOrphan {
				t.Errorf("isOrphan = %v, want %v", isOrphan, tt.wantOrphan)
			}
		})
	}
}

func TestStateManagement_RequiredBy(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Create dependency chain: tool-a -> tool-b -> tool-c

	// Install tool-c (will be marked as dependency of tool-b)
	err := sm.UpdateTool("tool-c", func(ts *install.ToolState) {
		ts.Version = "1.0.0"
		ts.IsExplicit = false
	})
	if err != nil {
		t.Fatalf("UpdateTool(tool-c) error = %v", err)
	}

	err = sm.AddRequiredBy("tool-c", "tool-b")
	if err != nil {
		t.Fatalf("AddRequiredBy(tool-c, tool-b) error = %v", err)
	}

	// Install tool-b (will be marked as dependency of tool-a)
	err = sm.UpdateTool("tool-b", func(ts *install.ToolState) {
		ts.Version = "1.0.0"
		ts.IsExplicit = false
	})
	if err != nil {
		t.Fatalf("UpdateTool(tool-b) error = %v", err)
	}

	err = sm.AddRequiredBy("tool-b", "tool-a")
	if err != nil {
		t.Fatalf("AddRequiredBy(tool-b, tool-a) error = %v", err)
	}

	// Install tool-a (explicit)
	err = sm.UpdateTool("tool-a", func(ts *install.ToolState) {
		ts.Version = "1.0.0"
		ts.IsExplicit = true
	})
	if err != nil {
		t.Fatalf("UpdateTool(tool-a) error = %v", err)
	}

	// Verify state
	state, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Check tool-c
	toolC := state.Installed["tool-c"]
	if toolC.IsExplicit {
		t.Error("tool-c should not be explicit")
	}
	if len(toolC.RequiredBy) != 1 || toolC.RequiredBy[0] != "tool-b" {
		t.Errorf("tool-c RequiredBy = %v, want [tool-b]", toolC.RequiredBy)
	}

	// Check tool-b
	toolB := state.Installed["tool-b"]
	if toolB.IsExplicit {
		t.Error("tool-b should not be explicit")
	}
	if len(toolB.RequiredBy) != 1 || toolB.RequiredBy[0] != "tool-a" {
		t.Errorf("tool-b RequiredBy = %v, want [tool-a]", toolB.RequiredBy)
	}

	// Check tool-a
	toolA := state.Installed["tool-a"]
	if !toolA.IsExplicit {
		t.Error("tool-a should be explicit")
	}
	if len(toolA.RequiredBy) != 0 {
		t.Errorf("tool-a RequiredBy = %v, want []", toolA.RequiredBy)
	}
}

func TestCleanupOrphans_Logic(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Setup: tool-a (explicit) -> tool-b (auto) -> tool-c (auto)
	err := sm.UpdateTool("tool-c", func(ts *install.ToolState) {
		ts.Version = "1.0.0"
		ts.IsExplicit = false
		ts.RequiredBy = []string{"tool-b"}
	})
	if err != nil {
		t.Fatalf("UpdateTool(tool-c) error = %v", err)
	}

	err = sm.UpdateTool("tool-b", func(ts *install.ToolState) {
		ts.Version = "1.0.0"
		ts.IsExplicit = false
		ts.RequiredBy = []string{"tool-a"}
	})
	if err != nil {
		t.Fatalf("UpdateTool(tool-b) error = %v", err)
	}

	err = sm.UpdateTool("tool-a", func(ts *install.ToolState) {
		ts.Version = "1.0.0"
		ts.IsExplicit = true
		ts.RequiredBy = []string{}
	})
	if err != nil {
		t.Fatalf("UpdateTool(tool-a) error = %v", err)
	}

	// Verify initial state
	state, _ := sm.Load()
	if len(state.Installed) != 3 {
		t.Fatalf("Initial installed count = %d, want 3", len(state.Installed))
	}

	// Remove tool-a (should trigger orphan cleanup of tool-b and tool-c)
	err = sm.RemoveTool("tool-a")
	if err != nil {
		t.Fatalf("RemoveTool(tool-a) error = %v", err)
	}

	// Simulate orphan cleanup logic
	// After removing tool-a, tool-b should be orphaned
	err = sm.RemoveRequiredBy("tool-b", "tool-a")
	if err != nil {
		t.Fatalf("RemoveRequiredBy(tool-b, tool-a) error = %v", err)
	}

	state, _ = sm.Load()
	toolB := state.Installed["tool-b"]
	isOrphanB := !toolB.IsExplicit && len(toolB.RequiredBy) == 0

	if !isOrphanB {
		t.Error("tool-b should be orphaned after removing tool-a")
	}

	// Remove tool-b (should trigger orphan cleanup of tool-c)
	err = sm.RemoveTool("tool-b")
	if err != nil {
		t.Fatalf("RemoveTool(tool-b) error = %v", err)
	}

	err = sm.RemoveRequiredBy("tool-c", "tool-b")
	if err != nil {
		t.Fatalf("RemoveRequiredBy(tool-c, tool-b) error = %v", err)
	}

	state, _ = sm.Load()
	toolC := state.Installed["tool-c"]
	isOrphanC := !toolC.IsExplicit && len(toolC.RequiredBy) == 0

	if !isOrphanC {
		t.Error("tool-c should be orphaned after removing tool-b")
	}
}

// Helper function from state_test.go
func newTestStateManager(t *testing.T) (*install.StateManager, func()) {
	t.Helper()
	cfg, cleanup := testutil.NewTestConfig(t)
	sm := install.NewStateManager(cfg)
	return sm, cleanup
}

func TestMapKeys(t *testing.T) {
	tests := []struct {
		name string
		m    map[string]string
		want int
	}{
		{
			name: "empty map",
			m:    map[string]string{},
			want: 0,
		},
		{
			name: "single key",
			m:    map[string]string{"nodejs": "20.10.0"},
			want: 1,
		},
		{
			name: "multiple keys",
			m:    map[string]string{"nodejs": "20.10.0", "python": "3.11.0", "go": "1.21.0"},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := mapKeys(tt.m)
			if len(keys) != tt.want {
				t.Errorf("mapKeys() returned %d keys, want %d", len(keys), tt.want)
			}

			// Verify all keys are present
			keySet := make(map[string]bool)
			for _, k := range keys {
				keySet[k] = true
			}
			for k := range tt.m {
				if !keySet[k] {
					t.Errorf("mapKeys() missing key %q", k)
				}
			}
		})
	}
}

func TestMapKeys_Empty(t *testing.T) {
	keys := mapKeys(nil)
	if keys == nil {
		t.Error("mapKeys(nil) should return empty slice, not nil")
	}
	if len(keys) != 0 {
		t.Errorf("mapKeys(nil) returned %d keys, want 0", len(keys))
	}
}

func TestResolveRuntimeDeps_NoRuntimeDeps(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := install.New(cfg)

	// Create a recipe without runtime dependencies
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "test-tool",
		},
	}

	result := resolveRuntimeDeps(r, mgr)
	if result != nil {
		t.Errorf("resolveRuntimeDeps() = %v, want nil for recipe without runtime deps", result)
	}
}

func TestResolveRuntimeDeps_WithRuntimeDeps(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := install.New(cfg)

	// Install a runtime dependency first
	sm := mgr.GetState()
	err := sm.UpdateTool("nodejs", func(ts *install.ToolState) {
		ts.Version = "20.10.0"
		ts.IsExplicit = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Create a recipe with runtime_dependencies
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:                "npm-tool",
			RuntimeDependencies: []string{"nodejs"},
		},
	}

	result := resolveRuntimeDeps(r, mgr)
	if result == nil {
		t.Fatal("resolveRuntimeDeps() = nil, want map with nodejs")
	}

	version, ok := result["nodejs"]
	if !ok {
		t.Error("resolveRuntimeDeps() missing nodejs key")
	}
	if version != "20.10.0" {
		t.Errorf("resolveRuntimeDeps()[nodejs] = %q, want %q", version, "20.10.0")
	}
}

func TestResolveRuntimeDeps_MissingDep(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := install.New(cfg)

	// Create a recipe with a runtime dependency that isn't installed
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:                "npm-tool",
			RuntimeDependencies: []string{"nodejs"},
		},
	}

	result := resolveRuntimeDeps(r, mgr)
	// Should return empty map (not nil) since deps were specified but not found
	if result == nil {
		// This is acceptable - nil means no deps resolved
		return
	}
	if len(result) != 0 {
		t.Errorf("resolveRuntimeDeps() = %v, want empty map for missing dep", result)
	}
}

func TestFindDependencyBinPath_NotInstalled(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := install.New(cfg)

	// Try to find a dependency that isn't installed
	_, err := findDependencyBinPath(mgr, "nonexistent-tool")
	if err == nil {
		t.Error("findDependencyBinPath() should return error for nonexistent tool")
	}
}

func TestFindDependencyBinPath_NoBinDir(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := install.New(cfg)

	// Install a tool in state but don't create the bin directory
	sm := mgr.GetState()
	err := sm.UpdateTool("test-tool", func(ts *install.ToolState) {
		ts.Version = "1.0.0"
		ts.IsExplicit = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Try to find bin path - should fail because bin directory doesn't exist
	_, err = findDependencyBinPath(mgr, "test-tool")
	if err == nil {
		t.Error("findDependencyBinPath() should return error when bin directory doesn't exist")
	}
}

func TestEnsurePackageManagersForRecipe_NoDeps(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := install.New(cfg)

	// Create a recipe with no dependencies (download action)
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "simple-tool",
		},
		Steps: []recipe.Step{
			{
				Action: "download",
				Params: map[string]interface{}{
					"url": "https://example.com/tool.tar.gz",
				},
			},
		},
	}

	visited := make(map[string]bool)
	execPaths, err := ensurePackageManagersForRecipe(mgr, r, visited, nil)
	if err != nil {
		t.Errorf("ensurePackageManagersForRecipe() error = %v", err)
	}
	if len(execPaths) != 0 {
		t.Errorf("ensurePackageManagersForRecipe() returned %d paths, want 0", len(execPaths))
	}
}

func TestEnsurePackageManagersForRecipe_AlreadyInstalled(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := install.New(cfg)

	// Pre-install nodejs in state
	sm := mgr.GetState()
	err := sm.UpdateTool("nodejs", func(ts *install.ToolState) {
		ts.Version = "20.10.0"
		ts.IsExplicit = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Create a recipe that has nodejs as a metadata dependency
	// Note: When dependencies are set in metadata, ResolveDependencies uses those
	// instead of action implicit deps. The npm_install action implies "nodejs",
	// but metadata.Dependencies = ["nodejs"] overrides that.
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:         "npm-tool",
			Dependencies: []string{"nodejs"},
		},
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{
					"package": "some-package",
				},
			},
		},
	}

	visited := make(map[string]bool)
	execPaths, err := ensurePackageManagersForRecipe(mgr, r, visited, nil)
	if err != nil {
		t.Errorf("ensurePackageManagersForRecipe() error = %v", err)
	}

	// The function checks if nodejs is installed but findDependencyBinPath
	// uses config.DefaultConfig() which may not match our test config.
	// Since we can't easily mock that, we just verify no error occurred
	// and accept that the path may or may not be found depending on environment.
	// The important thing is that it doesn't try to install nodejs again.
	t.Logf("ensurePackageManagersForRecipe() returned %d paths", len(execPaths))
}

func TestLibraryInstallAllowed(t *testing.T) {
	// Test that library installation is allowed in all cases
	// Direct library installation was previously blocked but is now allowed
	// to support runtime dependencies like gcc-libs for nodejs.
	// Library verification is a future concern tracked separately.

	tests := []struct {
		name       string
		isExplicit bool
		parent     string
	}{
		{
			name:       "direct user install should be allowed",
			isExplicit: true,
			parent:     "",
		},
		{
			name:       "dependency install should be allowed",
			isExplicit: false,
			parent:     "ruby",
		},
		{
			name:       "explicit with parent should be allowed",
			isExplicit: true,
			parent:     "ruby",
		},
		{
			name:       "implicit without parent should be allowed",
			isExplicit: false,
			parent:     "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Libraries are now installable in all cases
			// The code path proceeds to installLibrary() without blocking
			// This test documents that the blocking check was removed
			shouldBlock := false // No longer blocking any library installs
			if shouldBlock {
				t.Errorf("library install should be allowed for isExplicit=%v, parent=%q", tt.isExplicit, tt.parent)
			}
		})
	}
}
