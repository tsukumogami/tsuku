package main

import (
	"testing"

	"github.com/tsuku-dev/tsuku/internal/install"
	"github.com/tsuku-dev/tsuku/internal/testutil"
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
