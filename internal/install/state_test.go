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

	// Create test state
	state := &State{
		Installed: map[string]ToolState{
			"kubectl": {
				Version:    "1.29.0",
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

	if kubectl.Version != "1.29.0" {
		t.Errorf("Version = %s, want 1.29.0", kubectl.Version)
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
		ts.Version = "1.29.0"
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

	if kubectl.Version != "1.29.0" {
		t.Errorf("Version = %s, want 1.29.0", kubectl.Version)
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
		ts.Version = "1.28.0"
		ts.IsExplicit = false
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Update to new version
	err = sm.UpdateTool("kubectl", func(ts *ToolState) {
		ts.Version = "1.29.0"
		ts.IsExplicit = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Verify
	state, _ := sm.Load()
	kubectl := state.Installed["kubectl"]

	if kubectl.Version != "1.29.0" {
		t.Errorf("Version = %s, want 1.29.0", kubectl.Version)
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
		ts.Version = "1.29.0"
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
		ts.Version = "1.0.0"
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
		ts.Version = "1.0.0"
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
		ts.Version = "1.0.0"
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
		ts.Version = "1.0.0"
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
		ts.Version = "1.29.0"
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
		ts.Version = "3.12.0"
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
		ts.Version = "1.0.0"
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

	if kubectl.Version != "1.29.0" {
		t.Errorf("kubectl version = %q, want %q", kubectl.Version, "1.29.0")
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
