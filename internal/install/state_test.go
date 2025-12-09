package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/testutil"
)

func newTestStateManager(t *testing.T) (*StateManager, func()) {
	t.Helper()
	cfg, cleanup := testutil.NewTestConfig(t)
	sm := NewStateManager(cfg)
	return sm, cleanup
}

func TestStateManager_LoadMissing(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	state, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if state.Installed == nil {
		t.Fatal("Installed map is nil")
	}

	if len(state.Installed) != 0 {
		t.Errorf("Installed tools count = %d, want 0", len(state.Installed))
	}
}

func TestStateManager_SaveAndLoad(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Create test state using new multi-version format
	state := &State{
		Installed: map[string]ToolState{
			"kubectl": {
				ActiveVersion: "1.29.0",
				Versions: map[string]VersionState{
					"1.29.0": {Requested: "", Binaries: []string{"kubectl"}},
				},
				IsExplicit: true,
				RequiredBy: []string{},
			},
		},
	}

	// Save
	if err := sm.Save(state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load
	loaded, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify
	kubectl, ok := loaded.Installed["kubectl"]
	if !ok {
		t.Fatal("kubectl not found in loaded state")
	}

	if kubectl.ActiveVersion != "1.29.0" {
		t.Errorf("ActiveVersion = %s, want 1.29.0", kubectl.ActiveVersion)
	}

	if !kubectl.IsExplicit {
		t.Error("IsExplicit = false, want true")
	}

	if len(kubectl.RequiredBy) != 0 {
		t.Errorf("RequiredBy length = %d, want 0", len(kubectl.RequiredBy))
	}
}

func TestStateManager_LoadCorrupted(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	sm := NewStateManager(cfg)

	// Write corrupted JSON
	corruptedPath := filepath.Join(cfg.HomeDir, "state.json")
	if err := os.WriteFile(corruptedPath, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("failed to write corrupted state: %v", err)
	}

	// Load should fail
	_, err := sm.Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestStateManager_UpdateTool_NewTool(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	err := sm.UpdateTool("kubectl", func(ts *ToolState) {
		ts.ActiveVersion = "1.29.0"
		ts.Versions = map[string]VersionState{
			"1.29.0": {Requested: "", Binaries: []string{"kubectl"}},
		}
		ts.IsExplicit = true
	})

	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Verify
	state, _ := sm.Load()
	kubectl, ok := state.Installed["kubectl"]
	if !ok {
		t.Fatal("kubectl not found in state")
	}

	if kubectl.ActiveVersion != "1.29.0" {
		t.Errorf("ActiveVersion = %s, want 1.29.0", kubectl.ActiveVersion)
	}

	if !kubectl.IsExplicit {
		t.Error("IsExplicit = false, want true")
	}
}

func TestStateManager_UpdateTool_ExistingTool(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Add initial tool
	err := sm.UpdateTool("kubectl", func(ts *ToolState) {
		ts.ActiveVersion = "1.28.0"
		ts.Versions = map[string]VersionState{
			"1.28.0": {Requested: "", Binaries: []string{"kubectl"}},
		}
		ts.IsExplicit = false
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Update to new version (add new version, change active)
	err = sm.UpdateTool("kubectl", func(ts *ToolState) {
		ts.ActiveVersion = "1.29.0"
		ts.Versions["1.29.0"] = VersionState{Requested: "", Binaries: []string{"kubectl"}}
		ts.IsExplicit = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Verify
	state, _ := sm.Load()
	kubectl := state.Installed["kubectl"]

	if kubectl.ActiveVersion != "1.29.0" {
		t.Errorf("ActiveVersion = %s, want 1.29.0", kubectl.ActiveVersion)
	}

	if !kubectl.IsExplicit {
		t.Error("IsExplicit = false, want true")
	}
}

func TestStateManager_RemoveTool(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Add tool
	err := sm.UpdateTool("kubectl", func(ts *ToolState) {
		ts.ActiveVersion = "1.29.0"
		ts.Versions = map[string]VersionState{
			"1.29.0": {Requested: "", Binaries: []string{"kubectl"}},
		}
		ts.IsExplicit = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Remove tool
	err = sm.RemoveTool("kubectl")
	if err != nil {
		t.Fatalf("RemoveTool() error = %v", err)
	}

	// Verify
	state, _ := sm.Load()
	if _, ok := state.Installed["kubectl"]; ok {
		t.Error("kubectl should not be in state after removal")
	}
}

func TestStateManager_RemoveTool_NotExists(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Try to remove non-existent tool (should not error)
	err := sm.RemoveTool("nonexistent")
	if err != nil {
		t.Errorf("RemoveTool() error = %v, want nil", err)
	}
}

func TestStateManager_AddRequiredBy(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Add dependency tool
	err := sm.UpdateTool("tool-b", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Requested: "", Binaries: []string{"tool-b"}},
		}
		ts.IsExplicit = false
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Add dependent
	err = sm.AddRequiredBy("tool-b", "tool-a")
	if err != nil {
		t.Fatalf("AddRequiredBy() error = %v", err)
	}

	// Verify
	state, _ := sm.Load()
	toolB := state.Installed["tool-b"]

	if len(toolB.RequiredBy) != 1 {
		t.Fatalf("RequiredBy length = %d, want 1", len(toolB.RequiredBy))
	}

	if toolB.RequiredBy[0] != "tool-a" {
		t.Errorf("RequiredBy[0] = %s, want tool-a", toolB.RequiredBy[0])
	}
}

func TestStateManager_AddRequiredBy_Duplicate(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Add dependency tool
	err := sm.UpdateTool("tool-b", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Requested: "", Binaries: []string{"tool-b"}},
		}
		ts.IsExplicit = false
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Add dependent twice
	err = sm.AddRequiredBy("tool-b", "tool-a")
	if err != nil {
		t.Fatalf("AddRequiredBy() error = %v", err)
	}

	err = sm.AddRequiredBy("tool-b", "tool-a")
	if err != nil {
		t.Fatalf("AddRequiredBy() error = %v", err)
	}

	// Verify only one entry
	state, _ := sm.Load()
	toolB := state.Installed["tool-b"]

	if len(toolB.RequiredBy) != 1 {
		t.Errorf("RequiredBy length = %d, want 1 (duplicates should be ignored)", len(toolB.RequiredBy))
	}
}

func TestStateManager_RemoveRequiredBy(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Setup tool with dependencies
	err := sm.UpdateTool("tool-b", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Requested: "", Binaries: []string{"tool-b"}},
		}
		ts.IsExplicit = false
		ts.RequiredBy = []string{"tool-a", "tool-c"}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Remove one dependent
	err = sm.RemoveRequiredBy("tool-b", "tool-a")
	if err != nil {
		t.Fatalf("RemoveRequiredBy() error = %v", err)
	}

	// Verify
	state, _ := sm.Load()
	toolB := state.Installed["tool-b"]

	if len(toolB.RequiredBy) != 1 {
		t.Fatalf("RequiredBy length = %d, want 1", len(toolB.RequiredBy))
	}

	if toolB.RequiredBy[0] != "tool-c" {
		t.Errorf("RequiredBy[0] = %s, want tool-c", toolB.RequiredBy[0])
	}
}

func TestStateManager_RemoveRequiredBy_NotExists(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Setup tool
	err := sm.UpdateTool("tool-b", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Requested: "", Binaries: []string{"tool-b"}},
		}
		ts.RequiredBy = []string{"tool-a"}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Remove non-existent dependent
	err = sm.RemoveRequiredBy("tool-b", "tool-c")
	if err != nil {
		t.Fatalf("RemoveRequiredBy() error = %v", err)
	}

	// Verify original list unchanged
	state, _ := sm.Load()
	toolB := state.Installed["tool-b"]

	if len(toolB.RequiredBy) != 1 {
		t.Errorf("RequiredBy length = %d, want 1", len(toolB.RequiredBy))
	}
}

func TestStateManager_IsOrphan(t *testing.T) {
	tests := []struct {
		name      string
		toolState ToolState
		want      bool
	}{
		{
			name: "explicit tool is not orphan",
			toolState: ToolState{
				IsExplicit: true,
				RequiredBy: []string{},
			},
			want: false,
		},
		{
			name: "dependency with no dependents is orphan",
			toolState: ToolState{
				IsExplicit: false,
				RequiredBy: []string{},
			},
			want: true,
		},
		{
			name: "dependency with dependents is not orphan",
			toolState: ToolState{
				IsExplicit: false,
				RequiredBy: []string{"tool-a"},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isOrphan := !tt.toolState.IsExplicit && len(tt.toolState.RequiredBy) == 0
			if isOrphan != tt.want {
				t.Errorf("isOrphan = %v, want %v", isOrphan, tt.want)
			}
		})
	}
}

func TestHiddenInstallOptions(t *testing.T) {
	opts := HiddenInstallOptions()

	if opts.CreateSymlinks != false {
		t.Error("CreateSymlinks should be false for hidden install")
	}

	if opts.IsHidden != true {
		t.Error("IsHidden should be true for hidden install")
	}
}

func TestIsHidden_NotInstalled(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	isHidden, err := IsHidden(cfg, "nonexistent-tool")
	if err != nil {
		t.Fatalf("IsHidden() error = %v", err)
	}

	if isHidden {
		t.Error("non-existent tool should not be marked as hidden")
	}
}

func TestIsHidden_NotHidden(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	sm := NewStateManager(cfg)

	// Add a non-hidden tool
	err := sm.UpdateTool("kubectl", func(ts *ToolState) {
		ts.ActiveVersion = "1.29.0"
		ts.Versions = map[string]VersionState{
			"1.29.0": {Requested: "", Binaries: []string{"kubectl"}},
		}
		ts.IsExplicit = true
		ts.IsHidden = false
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	isHidden, err := IsHidden(cfg, "kubectl")
	if err != nil {
		t.Fatalf("IsHidden() error = %v", err)
	}

	if isHidden {
		t.Error("visible tool should not be marked as hidden")
	}
}

func TestIsHidden_Hidden(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	sm := NewStateManager(cfg)

	// Add a hidden tool
	err := sm.UpdateTool("python", func(ts *ToolState) {
		ts.ActiveVersion = "3.12.0"
		ts.Versions = map[string]VersionState{
			"3.12.0": {Requested: "", Binaries: []string{"python"}},
		}
		ts.IsExplicit = false
		ts.IsHidden = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	isHidden, err := IsHidden(cfg, "python")
	if err != nil {
		t.Fatalf("IsHidden() error = %v", err)
	}

	if !isHidden {
		t.Error("hidden tool should be marked as hidden")
	}
}

func TestDefaultInstallOptions(t *testing.T) {
	opts := DefaultInstallOptions()

	if opts.CreateSymlinks != true {
		t.Error("CreateSymlinks should be true by default")
	}

	if opts.IsHidden != false {
		t.Error("IsHidden should be false by default")
	}
}

func TestManager_ListEmptyToolsDir(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// List should return empty when tools dir doesn't exist
	tools, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(tools) != 0 {
		t.Errorf("List() returned %d tools, want 0", len(tools))
	}
}

func TestManager_ListAllEmptyToolsDir(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// ListAll should return empty when tools dir doesn't exist
	tools, err := mgr.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}

	if len(tools) != 0 {
		t.Errorf("ListAll() returned %d tools, want 0", len(tools))
	}
}

func TestManager_New(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	if mgr == nil {
		t.Fatal("New() returned nil")
	}

	if mgr.config != cfg {
		t.Error("Manager config not set correctly")
	}
}

func TestManager_ListWithOptions_WithTools(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create some tool directories
	toolDir1 := filepath.Join(cfg.ToolsDir, "kubectl-1.29.0")
	toolDir2 := filepath.Join(cfg.ToolsDir, "jq-1.7")
	toolDir3 := filepath.Join(cfg.ToolsDir, "hidden-tool-1.0.0")

	if err := os.MkdirAll(toolDir1, 0755); err != nil {
		t.Fatalf("failed to create tool dir: %v", err)
	}
	if err := os.MkdirAll(toolDir2, 0755); err != nil {
		t.Fatalf("failed to create tool dir: %v", err)
	}
	if err := os.MkdirAll(toolDir3, 0755); err != nil {
		t.Fatalf("failed to create tool dir: %v", err)
	}

	// Mark hidden-tool as hidden in state
	sm := NewStateManager(cfg)
	err := sm.UpdateTool("hidden-tool", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Requested: "", Binaries: []string{"hidden-tool"}},
		}
		ts.IsHidden = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// List excluding hidden
	tools, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Should have 2 visible tools
	if len(tools) != 2 {
		t.Errorf("List() returned %d tools, want 2", len(tools))
	}

	// ListAll should include hidden
	allTools, err := mgr.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}

	if len(allTools) != 3 {
		t.Errorf("ListAll() returned %d tools, want 3", len(allTools))
	}
}

func TestManager_ListWithOptions_InvalidFormat(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create directories with invalid format (no hyphen)
	invalidDir := filepath.Join(cfg.ToolsDir, "nohyphen")
	if err := os.MkdirAll(invalidDir, 0755); err != nil {
		t.Fatalf("failed to create invalid dir: %v", err)
	}

	// Create a "current" directory (should be skipped)
	currentDir := filepath.Join(cfg.ToolsDir, "current")
	if err := os.MkdirAll(currentDir, 0755); err != nil {
		t.Fatalf("failed to create current dir: %v", err)
	}

	// Create file (not directory, should be skipped)
	filePath := filepath.Join(cfg.ToolsDir, "somefile")
	if err := os.WriteFile(filePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}

	// List should return empty (all invalid entries skipped)
	tools, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(tools) != 0 {
		t.Errorf("List() returned %d tools, want 0 (invalid entries)", len(tools))
	}
}

func TestManager_GetState(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Add a tool to state
	sm := NewStateManager(cfg)
	err := sm.UpdateTool("kubectl", func(ts *ToolState) {
		ts.Version = "1.29.0"
		ts.IsExplicit = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// GetState returns the StateManager
	stateManager := mgr.GetState()
	if stateManager == nil {
		t.Fatal("GetState() returned nil")
	}

	// Load the state using the StateManager
	state, err := stateManager.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	kubectl, exists := state.Installed["kubectl"]
	if !exists {
		t.Error("kubectl not found in state")
	}

	// After migration, both Version (for backward compat) and ActiveVersion should be set
	if kubectl.ActiveVersion != "1.29.0" {
		t.Errorf("kubectl ActiveVersion = %q, want %q", kubectl.ActiveVersion, "1.29.0")
	}
	if kubectl.Version != "1.29.0" {
		t.Errorf("kubectl Version = %q, want %q (preserved for backward compat)", kubectl.Version, "1.29.0")
	}
}

func TestManager_ListWithOptions_StateLoadError(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)

	// Create tools directory with a valid tool
	toolDir := filepath.Join(cfg.ToolsDir, "kubectl-1.29.0")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatalf("failed to create tool dir: %v", err)
	}

	// Write corrupted state.json to trigger load error
	statePath := filepath.Join(cfg.HomeDir, "state.json")
	if err := os.WriteFile(statePath, []byte("{invalid json}"), 0644); err != nil {
		t.Fatalf("failed to write corrupted state: %v", err)
	}

	// ListWithOptions should return error when state load fails
	_, err := mgr.ListWithOptions(false)
	if err == nil {
		t.Error("ListWithOptions() should fail when state load fails")
	}
}

func TestStateManager_statePath(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	sm := NewStateManager(cfg)

	// Verify statePath returns expected path
	expected := filepath.Join(cfg.HomeDir, "state.json")
	if sm.statePath() != expected {
		t.Errorf("statePath() = %q, want %q", sm.statePath(), expected)
	}
}

// Library state tests

func TestStateManager_LoadMissing_InitializesLibs(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	state, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v, want nil", err)
	}

	if state.Libs == nil {
		t.Fatal("Libs map is nil, should be initialized")
	}

	if len(state.Libs) != 0 {
		t.Errorf("Libs count = %d, want 0", len(state.Libs))
	}
}

func TestStateManager_BackwardCompatibility_NoLibsSection(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	sm := NewStateManager(cfg)

	// Write state.json without libs section (old format)
	oldState := `{"installed":{"kubectl":{"version":"1.29.0","is_explicit":true,"required_by":[]}}}`
	statePath := filepath.Join(cfg.HomeDir, "state.json")
	if err := os.WriteFile(statePath, []byte(oldState), 0644); err != nil {
		t.Fatalf("failed to write old state: %v", err)
	}

	// Load should succeed and initialize Libs
	state, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if state.Libs == nil {
		t.Fatal("Libs map is nil after loading old format")
	}

	if len(state.Libs) != 0 {
		t.Errorf("Libs count = %d, want 0", len(state.Libs))
	}

	// Verify installed tools still work
	if _, ok := state.Installed["kubectl"]; !ok {
		t.Error("kubectl not found in installed tools")
	}
}

func TestStateManager_AddLibraryUsedBy(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	err := sm.AddLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")
	if err != nil {
		t.Fatalf("AddLibraryUsedBy() error = %v", err)
	}

	// Verify
	state, _ := sm.Load()
	libState := state.Libs["libyaml"]["0.2.5"]

	if len(libState.UsedBy) != 1 {
		t.Fatalf("UsedBy length = %d, want 1", len(libState.UsedBy))
	}

	if libState.UsedBy[0] != "ruby-3.4.0" {
		t.Errorf("UsedBy[0] = %s, want ruby-3.4.0", libState.UsedBy[0])
	}
}

func TestStateManager_AddLibraryUsedBy_Multiple(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Add multiple tools using same library
	err := sm.AddLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")
	if err != nil {
		t.Fatalf("AddLibraryUsedBy() error = %v", err)
	}

	err = sm.AddLibraryUsedBy("libyaml", "0.2.5", "python-3.12")
	if err != nil {
		t.Fatalf("AddLibraryUsedBy() error = %v", err)
	}

	// Verify
	state, _ := sm.Load()
	libState := state.Libs["libyaml"]["0.2.5"]

	if len(libState.UsedBy) != 2 {
		t.Fatalf("UsedBy length = %d, want 2", len(libState.UsedBy))
	}
}

func TestStateManager_AddLibraryUsedBy_Duplicate(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Add same tool twice
	_ = sm.AddLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")
	_ = sm.AddLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")

	// Verify only one entry
	state, _ := sm.Load()
	libState := state.Libs["libyaml"]["0.2.5"]

	if len(libState.UsedBy) != 1 {
		t.Errorf("UsedBy length = %d, want 1 (duplicates should be ignored)", len(libState.UsedBy))
	}
}

func TestStateManager_RemoveLibraryUsedBy(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Setup
	_ = sm.AddLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")
	_ = sm.AddLibraryUsedBy("libyaml", "0.2.5", "python-3.12")

	// Remove one
	err := sm.RemoveLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")
	if err != nil {
		t.Fatalf("RemoveLibraryUsedBy() error = %v", err)
	}

	// Verify
	state, _ := sm.Load()
	libState := state.Libs["libyaml"]["0.2.5"]

	if len(libState.UsedBy) != 1 {
		t.Fatalf("UsedBy length = %d, want 1", len(libState.UsedBy))
	}

	if libState.UsedBy[0] != "python-3.12" {
		t.Errorf("UsedBy[0] = %s, want python-3.12", libState.UsedBy[0])
	}
}

func TestStateManager_RemoveLibraryVersion(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Setup
	_ = sm.AddLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")
	_ = sm.AddLibraryUsedBy("libyaml", "0.2.4", "python-3.11")

	// Remove one version
	err := sm.RemoveLibraryVersion("libyaml", "0.2.5")
	if err != nil {
		t.Fatalf("RemoveLibraryVersion() error = %v", err)
	}

	// Verify version removed
	state, _ := sm.Load()
	if _, exists := state.Libs["libyaml"]["0.2.5"]; exists {
		t.Error("libyaml 0.2.5 should be removed")
	}

	// Other version should still exist
	if _, exists := state.Libs["libyaml"]["0.2.4"]; !exists {
		t.Error("libyaml 0.2.4 should still exist")
	}
}

func TestStateManager_RemoveLibraryVersion_CleansEmptyLib(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Setup single version
	_ = sm.AddLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")

	// Remove only version
	err := sm.RemoveLibraryVersion("libyaml", "0.2.5")
	if err != nil {
		t.Fatalf("RemoveLibraryVersion() error = %v", err)
	}

	// Verify library entry is cleaned up
	state, _ := sm.Load()
	if _, exists := state.Libs["libyaml"]; exists {
		t.Error("libyaml entry should be removed when empty")
	}
}

func TestStateManager_GetLibraryState(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Setup
	_ = sm.AddLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")

	// Get existing
	libState, err := sm.GetLibraryState("libyaml", "0.2.5")
	if err != nil {
		t.Fatalf("GetLibraryState() error = %v", err)
	}

	if libState == nil {
		t.Fatal("GetLibraryState() returned nil for existing library")
	}

	if len(libState.UsedBy) != 1 {
		t.Errorf("UsedBy length = %d, want 1", len(libState.UsedBy))
	}
}

func TestStateManager_GetLibraryState_NotFound(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Get non-existent
	libState, err := sm.GetLibraryState("nonexistent", "1.0.0")
	if err != nil {
		t.Fatalf("GetLibraryState() error = %v", err)
	}

	if libState != nil {
		t.Error("GetLibraryState() should return nil for non-existent library")
	}
}

func TestStateManager_MultipleLibraryVersions(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Setup multiple versions of same library
	_ = sm.AddLibraryUsedBy("libyaml", "0.2.5", "ruby-3.4.0")
	_ = sm.AddLibraryUsedBy("libyaml", "0.2.4", "ruby-3.3.0")
	_ = sm.AddLibraryUsedBy("openssl", "3.0.0", "python-3.12")

	// Verify
	state, _ := sm.Load()

	if len(state.Libs) != 2 {
		t.Errorf("Libs count = %d, want 2", len(state.Libs))
	}

	if len(state.Libs["libyaml"]) != 2 {
		t.Errorf("libyaml versions count = %d, want 2", len(state.Libs["libyaml"]))
	}

	if len(state.Libs["openssl"]) != 1 {
		t.Errorf("openssl versions count = %d, want 1", len(state.Libs["openssl"]))
	}
}

func TestStateManager_SaveAndLoad_WithLibs(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Create state with both tools and libs
	state := &State{
		Installed: map[string]ToolState{
			"ruby": {
				ActiveVersion: "3.4.0",
				Versions: map[string]VersionState{
					"3.4.0": {Requested: "", Binaries: []string{"ruby"}},
				},
				IsExplicit: true,
				RequiredBy: []string{},
			},
		},
		Libs: map[string]map[string]LibraryVersionState{
			"libyaml": {
				"0.2.5": {UsedBy: []string{"ruby-3.4.0"}},
			},
		},
	}

	// Save
	if err := sm.Save(state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load
	loaded, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify tools
	if _, ok := loaded.Installed["ruby"]; !ok {
		t.Error("ruby not found in loaded state")
	}

	// Verify libs
	libState := loaded.Libs["libyaml"]["0.2.5"]
	if len(libState.UsedBy) != 1 {
		t.Errorf("UsedBy length = %d, want 1", len(libState.UsedBy))
	}
	if libState.UsedBy[0] != "ruby-3.4.0" {
		t.Errorf("UsedBy[0] = %s, want ruby-3.4.0", libState.UsedBy[0])
	}
}

func TestStateManager_SaveAndLoad_WithDependencies(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Create test state with dependencies
	state := &State{
		Installed: map[string]ToolState{
			"turbo": {
				ActiveVersion: "1.10.0",
				Versions: map[string]VersionState{
					"1.10.0": {Requested: "", Binaries: []string{"turbo"}},
				},
				IsExplicit:          true,
				RequiredBy:          []string{},
				InstallDependencies: []string{"nodejs"},
				RuntimeDependencies: []string{"nodejs"},
			},
			"esbuild": {
				ActiveVersion: "0.19.0",
				Versions: map[string]VersionState{
					"0.19.0": {Requested: "", Binaries: []string{"esbuild"}},
				},
				IsExplicit:          true,
				RequiredBy:          []string{},
				InstallDependencies: []string{"nodejs"},
				RuntimeDependencies: []string{}, // compiled binary, no runtime deps
			},
		},
		Libs: make(map[string]map[string]LibraryVersionState),
	}

	// Save
	if err := sm.Save(state); err != nil {
		t.Fatalf("Save() error = %v", err)
	}

	// Load
	loaded, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify turbo
	turbo, ok := loaded.Installed["turbo"]
	if !ok {
		t.Fatal("turbo not found in loaded state")
	}
	if len(turbo.InstallDependencies) != 1 || turbo.InstallDependencies[0] != "nodejs" {
		t.Errorf("turbo.InstallDependencies = %v, want [nodejs]", turbo.InstallDependencies)
	}
	if len(turbo.RuntimeDependencies) != 1 || turbo.RuntimeDependencies[0] != "nodejs" {
		t.Errorf("turbo.RuntimeDependencies = %v, want [nodejs]", turbo.RuntimeDependencies)
	}

	// Verify esbuild
	esbuild, ok := loaded.Installed["esbuild"]
	if !ok {
		t.Fatal("esbuild not found in loaded state")
	}
	if len(esbuild.InstallDependencies) != 1 || esbuild.InstallDependencies[0] != "nodejs" {
		t.Errorf("esbuild.InstallDependencies = %v, want [nodejs]", esbuild.InstallDependencies)
	}
	if len(esbuild.RuntimeDependencies) != 0 {
		t.Errorf("esbuild.RuntimeDependencies = %v, want []", esbuild.RuntimeDependencies)
	}
}

func TestStateManager_BackwardCompatibility_NoDependencyFields(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	sm := NewStateManager(cfg)

	// Write state file without dependency fields (simulates old state.json)
	oldStateJSON := `{
  "installed": {
    "kubectl": {
      "version": "1.29.0",
      "is_explicit": true,
      "required_by": []
    }
  }
}`
	statePath := filepath.Join(cfg.HomeDir, "state.json")
	if err := os.WriteFile(statePath, []byte(oldStateJSON), 0644); err != nil {
		t.Fatalf("failed to write old state: %v", err)
	}

	// Load should succeed
	loaded, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// Verify tool exists
	kubectl, ok := loaded.Installed["kubectl"]
	if !ok {
		t.Fatal("kubectl not found in loaded state")
	}

	// Dependency fields should be nil (empty), not cause errors
	if len(kubectl.InstallDependencies) != 0 {
		t.Errorf("InstallDependencies = %v, want nil or empty", kubectl.InstallDependencies)
	}
	if len(kubectl.RuntimeDependencies) != 0 {
		t.Errorf("RuntimeDependencies = %v, want nil or empty", kubectl.RuntimeDependencies)
	}
}

func TestStateManager_UpdateTool_WithDependencies(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Update tool with dependencies
	err := sm.UpdateTool("turbo", func(ts *ToolState) {
		ts.ActiveVersion = "1.10.0"
		ts.Versions = map[string]VersionState{
			"1.10.0": {Requested: "", Binaries: []string{"turbo"}},
		}
		ts.IsExplicit = true
		ts.InstallDependencies = []string{"nodejs"}
		ts.RuntimeDependencies = []string{"nodejs"}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Load and verify
	state, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	turbo := state.Installed["turbo"]
	if len(turbo.InstallDependencies) != 1 || turbo.InstallDependencies[0] != "nodejs" {
		t.Errorf("InstallDependencies = %v, want [nodejs]", turbo.InstallDependencies)
	}
	if len(turbo.RuntimeDependencies) != 1 || turbo.RuntimeDependencies[0] != "nodejs" {
		t.Errorf("RuntimeDependencies = %v, want [nodejs]", turbo.RuntimeDependencies)
	}
}

// Multi-version state tests

func TestStateManager_MigrateSingleVersionToMultiVersion(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	sm := NewStateManager(cfg)

	// Write state.json in old format (single version field)
	oldStateJSON := `{
  "installed": {
    "kubectl": {
      "version": "1.29.0",
      "is_explicit": true,
      "required_by": [],
      "binaries": ["kubectl"]
    }
  }
}`
	statePath := filepath.Join(cfg.HomeDir, "state.json")
	if err := os.WriteFile(statePath, []byte(oldStateJSON), 0644); err != nil {
		t.Fatalf("failed to write old state: %v", err)
	}

	// Load should migrate to new format
	state, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	kubectl, ok := state.Installed["kubectl"]
	if !ok {
		t.Fatal("kubectl not found in loaded state")
	}

	// Verify migration: Version preserved for backward compat, ActiveVersion set
	if kubectl.Version != "1.29.0" {
		t.Errorf("Version = %q, want 1.29.0 (preserved for backward compat)", kubectl.Version)
	}
	if kubectl.ActiveVersion != "1.29.0" {
		t.Errorf("ActiveVersion = %q, want 1.29.0", kubectl.ActiveVersion)
	}

	// Verify Versions map created
	if len(kubectl.Versions) != 1 {
		t.Fatalf("Versions count = %d, want 1", len(kubectl.Versions))
	}
	vs, exists := kubectl.Versions["1.29.0"]
	if !exists {
		t.Fatal("version 1.29.0 not found in Versions map")
	}
	if len(vs.Binaries) != 1 || vs.Binaries[0] != "kubectl" {
		t.Errorf("VersionState.Binaries = %v, want [kubectl]", vs.Binaries)
	}

	// Verify other fields preserved
	if !kubectl.IsExplicit {
		t.Error("IsExplicit = false, want true")
	}
}

func TestStateManager_NoMigrationForNewFormat(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	sm := NewStateManager(cfg)

	// Write state.json in new format (already migrated)
	newStateJSON := `{
  "installed": {
    "kubectl": {
      "active_version": "1.29.0",
      "versions": {
        "1.29.0": {"requested": "@lts", "binaries": ["kubectl"]}
      },
      "is_explicit": true,
      "required_by": []
    }
  }
}`
	statePath := filepath.Join(cfg.HomeDir, "state.json")
	if err := os.WriteFile(statePath, []byte(newStateJSON), 0644); err != nil {
		t.Fatalf("failed to write new state: %v", err)
	}

	// Load should NOT modify already-migrated state
	state, err := sm.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	kubectl := state.Installed["kubectl"]

	// ActiveVersion should remain as-is
	if kubectl.ActiveVersion != "1.29.0" {
		t.Errorf("ActiveVersion = %q, want 1.29.0", kubectl.ActiveVersion)
	}

	// Versions map should preserve original data
	vs := kubectl.Versions["1.29.0"]
	if vs.Requested != "@lts" {
		t.Errorf("Requested = %q, want @lts", vs.Requested)
	}
}

func TestValidateVersionString_Valid(t *testing.T) {
	validVersions := []string{
		"1.0.0",
		"17.0.12",
		"21.0.5+9-LTS",
		"3.12.0-rc1",
		"v1.2.3",
	}

	for _, v := range validVersions {
		t.Run(v, func(t *testing.T) {
			err := ValidateVersionString(v)
			if err != nil {
				t.Errorf("ValidateVersionString(%q) = %v, want nil", v, err)
			}
		})
	}
}

func TestValidateVersionString_PathTraversal(t *testing.T) {
	tests := []struct {
		version string
		desc    string
	}{
		{"../etc/passwd", "path traversal with .."},
		{"1.0.0/../2.0.0", "embedded path traversal"},
		{"foo/bar", "forward slash"},
		{"foo\\bar", "backslash"},
		{"..\\windows\\system32", "backslash traversal"},
	}

	for _, tt := range tests {
		t.Run(tt.desc, func(t *testing.T) {
			err := ValidateVersionString(tt.version)
			if err == nil {
				t.Errorf("ValidateVersionString(%q) = nil, want error", tt.version)
			}
		})
	}
}

func TestStateManager_MultipleVersionsPerTool(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Install first version
	err := sm.UpdateTool("liberica-jdk", func(ts *ToolState) {
		ts.ActiveVersion = "17.0.12"
		ts.Versions = map[string]VersionState{
			"17.0.12": {Requested: "17", Binaries: []string{"java", "javac"}},
		}
		ts.IsExplicit = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Install second version (add to versions map, change active)
	err = sm.UpdateTool("liberica-jdk", func(ts *ToolState) {
		ts.ActiveVersion = "21.0.5"
		ts.Versions["21.0.5"] = VersionState{Requested: "@lts", Binaries: []string{"java", "javac"}}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Verify both versions exist
	state, _ := sm.Load()
	jdk := state.Installed["liberica-jdk"]

	if jdk.ActiveVersion != "21.0.5" {
		t.Errorf("ActiveVersion = %q, want 21.0.5", jdk.ActiveVersion)
	}

	if len(jdk.Versions) != 2 {
		t.Fatalf("Versions count = %d, want 2", len(jdk.Versions))
	}

	// Check both versions have their metadata
	v17 := jdk.Versions["17.0.12"]
	if v17.Requested != "17" {
		t.Errorf("v17.Requested = %q, want 17", v17.Requested)
	}

	v21 := jdk.Versions["21.0.5"]
	if v21.Requested != "@lts" {
		t.Errorf("v21.Requested = %q, want @lts", v21.Requested)
	}
}

func TestStateManager_GetToolState(t *testing.T) {
	sm, cleanup := newTestStateManager(t)
	defer cleanup()

	// Add a tool
	err := sm.UpdateTool("kubectl", func(ts *ToolState) {
		ts.ActiveVersion = "1.29.0"
		ts.Versions = map[string]VersionState{
			"1.29.0": {Requested: "", Binaries: []string{"kubectl"}},
		}
		ts.IsExplicit = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Get existing tool
	toolState, err := sm.GetToolState("kubectl")
	if err != nil {
		t.Fatalf("GetToolState() error = %v", err)
	}
	switch {
	case toolState == nil:
		t.Fatal("GetToolState() returned nil for existing tool")
	case toolState.ActiveVersion != "1.29.0":
		t.Errorf("ActiveVersion = %q, want 1.29.0", toolState.ActiveVersion)
	}

	// Get non-existent tool
	nonExistentState, err := sm.GetToolState("nonexistent")
	if err != nil {
		t.Fatalf("GetToolState() error = %v", err)
	}
	if nonExistentState != nil {
		t.Error("GetToolState() should return nil for non-existent tool")
	}
}
