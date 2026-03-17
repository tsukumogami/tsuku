package install

import (
	"os"
	"testing"

	"github.com/tsukumogami/tsuku/internal/testutil"
)

func TestListWithOptions_MultiVersion(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)
	sm := NewStateManager(cfg)

	// Create tool directories for multiple versions
	toolDir1 := cfg.ToolDir("nodejs", "18.20.0")
	toolDir2 := cfg.ToolDir("nodejs", "20.10.0")
	toolDir3 := cfg.ToolDir("liberica-jdk", "17.0.12")
	toolDir4 := cfg.ToolDir("liberica-jdk", "21.0.5")

	for _, dir := range []string{toolDir1, toolDir2, toolDir3, toolDir4} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create tool dir: %v", err)
		}
	}

	// Add state entries with multi-version support
	err := sm.UpdateTool("nodejs", func(ts *ToolState) {
		ts.ActiveVersion = "20.10.0"
		ts.Versions = map[string]VersionState{
			"18.20.0": {Requested: "18", Binaries: []string{"node", "npm"}},
			"20.10.0": {Requested: "20", Binaries: []string{"node", "npm"}},
		}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	err = sm.UpdateTool("liberica-jdk", func(ts *ToolState) {
		ts.ActiveVersion = "21.0.5"
		ts.Versions = map[string]VersionState{
			"17.0.12": {Requested: "17", Binaries: []string{"java"}},
			"21.0.5":  {Requested: "21", Binaries: []string{"java"}},
		}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// List all versions
	tools, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	// Should have 4 entries (2 versions each for 2 tools)
	if len(tools) != 4 {
		t.Errorf("List() returned %d tools, want 4", len(tools))
	}

	// Verify sorted order: by tool name, then by version
	expectedOrder := []struct {
		name     string
		version  string
		isActive bool
	}{
		{"liberica-jdk", "17.0.12", false},
		{"liberica-jdk", "21.0.5", true},
		{"nodejs", "18.20.0", false},
		{"nodejs", "20.10.0", true},
	}

	for i, expected := range expectedOrder {
		if i >= len(tools) {
			break
		}
		if tools[i].Name != expected.name {
			t.Errorf("tools[%d].Name = %q, want %q", i, tools[i].Name, expected.name)
		}
		if tools[i].Version != expected.version {
			t.Errorf("tools[%d].Version = %q, want %q", i, tools[i].Version, expected.version)
		}
		if tools[i].IsActive != expected.isActive {
			t.Errorf("tools[%d].IsActive = %v, want %v", i, tools[i].IsActive, expected.isActive)
		}
	}
}

func TestListWithOptions_StaleStateEntries(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)
	sm := NewStateManager(cfg)

	// Create only one version directory
	toolDir := cfg.ToolDir("kubectl", "1.29.0")
	if err := os.MkdirAll(toolDir, 0755); err != nil {
		t.Fatalf("failed to create tool dir: %v", err)
	}

	// Add state entry with versions that don't exist on disk
	err := sm.UpdateTool("kubectl", func(ts *ToolState) {
		ts.ActiveVersion = "1.29.0"
		ts.Versions = map[string]VersionState{
			"1.28.0": {Requested: "1.28", Binaries: []string{"kubectl"}}, // Stale - no directory
			"1.29.0": {Requested: "1.29", Binaries: []string{"kubectl"}}, // Valid
			"1.30.0": {Requested: "1.30", Binaries: []string{"kubectl"}}, // Stale - no directory
		}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// List should only return the version with existing directory
	tools, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(tools) != 1 {
		t.Errorf("List() returned %d tools, want 1 (stale entries filtered)", len(tools))
	}

	if len(tools) > 0 && tools[0].Version != "1.29.0" {
		t.Errorf("tools[0].Version = %q, want %q", tools[0].Version, "1.29.0")
	}
}

func TestListWithOptions_EmptyVersionsMap(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)
	sm := NewStateManager(cfg)

	// Add state entry with empty Versions map (shouldn't happen, but be defensive)
	err := sm.UpdateTool("broken", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{} // Empty
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// List should return empty (no versions to list)
	tools, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(tools) != 0 {
		t.Errorf("List() returned %d tools, want 0 (empty versions map)", len(tools))
	}
}

func TestListWithOptions_SourceField(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)
	sm := NewStateManager(cfg)

	// Create tool directories
	for _, dir := range []string{
		cfg.ToolDir("ripgrep", "14.1.1"),
		cfg.ToolDir("my-tool", "1.0.0"),
		cfg.ToolDir("local-tool", "2.0.0"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create tool dir: %v", err)
		}
	}

	// Central source tool
	err := sm.UpdateTool("ripgrep", func(ts *ToolState) {
		ts.ActiveVersion = "14.1.1"
		ts.Source = "central"
		ts.Versions = map[string]VersionState{
			"14.1.1": {Requested: "", Binaries: []string{"rg"}},
		}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Distributed source tool
	err = sm.UpdateTool("my-tool", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Source = "alice/tools"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Requested: "", Binaries: []string{"my-tool"}},
		}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Local source tool
	err = sm.UpdateTool("local-tool", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.Source = "local"
		ts.Versions = map[string]VersionState{
			"2.0.0": {Requested: "", Binaries: []string{"local-tool"}},
		}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	tools, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(tools) != 3 {
		t.Fatalf("List() returned %d tools, want 3", len(tools))
	}

	// Verify sources (tools are sorted by name)
	expected := []struct {
		name   string
		source string
	}{
		{"local-tool", "local"},
		{"my-tool", "alice/tools"},
		{"ripgrep", "central"},
	}

	for i, exp := range expected {
		if tools[i].Name != exp.name {
			t.Errorf("tools[%d].Name = %q, want %q", i, tools[i].Name, exp.name)
		}
		if tools[i].Source != exp.source {
			t.Errorf("tools[%d].Source = %q, want %q", i, tools[i].Source, exp.source)
		}
	}
}

func TestListWithOptions_MigratedSource(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)
	sm := NewStateManager(cfg)

	if err := os.MkdirAll(cfg.ToolDir("old-tool", "1.0.0"), 0755); err != nil {
		t.Fatalf("failed to create tool dir: %v", err)
	}

	// Tool without Source field (pre-migration state) gets migrated to "central"
	err := sm.UpdateTool("old-tool", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Requested: "", Binaries: []string{"old-tool"}},
		}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	tools, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}

	if len(tools) != 1 {
		t.Fatalf("List() returned %d tools, want 1", len(tools))
	}

	// Source should be "central" after lazy migration
	if tools[0].Source != "central" {
		t.Errorf("tools[0].Source = %q, want %q", tools[0].Source, "central")
	}
}

func TestListWithOptions_HiddenToolFiltering(t *testing.T) {
	cfg, cleanup := testutil.NewTestConfig(t)
	defer cleanup()

	mgr := New(cfg)
	sm := NewStateManager(cfg)

	// Create directories
	if err := os.MkdirAll(cfg.ToolDir("visible", "1.0.0"), 0755); err != nil {
		t.Fatalf("failed to create tool dir: %v", err)
	}
	if err := os.MkdirAll(cfg.ToolDir("hidden", "2.0.0"), 0755); err != nil {
		t.Fatalf("failed to create tool dir: %v", err)
	}

	// Add visible tool
	err := sm.UpdateTool("visible", func(ts *ToolState) {
		ts.ActiveVersion = "1.0.0"
		ts.Versions = map[string]VersionState{
			"1.0.0": {Requested: "", Binaries: []string{"visible"}},
		}
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// Add hidden tool
	err = sm.UpdateTool("hidden", func(ts *ToolState) {
		ts.ActiveVersion = "2.0.0"
		ts.Versions = map[string]VersionState{
			"2.0.0": {Requested: "", Binaries: []string{"hidden"}},
		}
		ts.IsHidden = true
	})
	if err != nil {
		t.Fatalf("UpdateTool() error = %v", err)
	}

	// List() should exclude hidden
	tools, err := mgr.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(tools) != 1 {
		t.Errorf("List() returned %d tools, want 1 (hidden excluded)", len(tools))
	}

	// ListAll() should include hidden
	allTools, err := mgr.ListAll()
	if err != nil {
		t.Fatalf("ListAll() error = %v", err)
	}
	if len(allTools) != 2 {
		t.Errorf("ListAll() returned %d tools, want 2 (hidden included)", len(allTools))
	}
}
