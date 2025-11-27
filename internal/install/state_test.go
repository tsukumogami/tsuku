package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsuku-dev/tsuku/internal/testutil"
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
