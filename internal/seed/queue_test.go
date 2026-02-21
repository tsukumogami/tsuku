package seed

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/batch"
)

func TestMerge_deduplicates(t *testing.T) {
	q := &PriorityQueue{
		SchemaVersion: 1,
		Packages: []Package{
			{ID: "homebrew:jq", Source: "homebrew", Name: "jq", Tier: 1, Status: "success"},
			{ID: "homebrew:fd", Source: "homebrew", Name: "fd", Tier: 1, Status: "pending"},
		},
	}

	newPkgs := []Package{
		{ID: "homebrew:jq", Source: "homebrew", Name: "jq", Tier: 1, Status: "pending"},   // duplicate
		{ID: "homebrew:bat", Source: "homebrew", Name: "bat", Tier: 1, Status: "pending"}, // new
		{ID: "homebrew:fd", Source: "homebrew", Name: "fd", Tier: 1, Status: "pending"},   // duplicate
		{ID: "homebrew:fzf", Source: "homebrew", Name: "fzf", Tier: 2, Status: "pending"}, // new
	}

	added := q.Merge(newPkgs)
	if added != 2 {
		t.Errorf("expected 2 added, got %d", added)
	}
	if len(q.Packages) != 4 {
		t.Errorf("expected 4 total, got %d", len(q.Packages))
	}

	// Verify existing entry status preserved (jq was "success", not overwritten)
	for _, p := range q.Packages {
		if p.ID == "homebrew:jq" && p.Status != "success" {
			t.Errorf("jq status should be preserved as 'success', got %q", p.Status)
		}
	}
}

func TestLoadSave_roundtrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "queue.json")

	q := &PriorityQueue{
		SchemaVersion: 1,
		Tiers: map[string]string{
			"1": "Critical",
			"2": "Popular",
			"3": "Standard",
		},
		Packages: []Package{
			{ID: "homebrew:jq", Source: "homebrew", Name: "jq", Tier: 1, Status: "pending", AddedAt: "2025-01-01T00:00:00Z"},
		},
	}

	if err := q.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}

	loaded, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if len(loaded.Packages) != 1 {
		t.Errorf("expected 1 package, got %d", len(loaded.Packages))
	}
	if loaded.Packages[0].ID != "homebrew:jq" {
		t.Errorf("expected homebrew:jq, got %s", loaded.Packages[0].ID)
	}
}

func TestLoad_missingFile(t *testing.T) {
	q, err := Load("/nonexistent/path.json")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got: %v", err)
	}
	if len(q.Packages) != 0 {
		t.Errorf("expected empty queue, got %d packages", len(q.Packages))
	}
}

func TestAssignTier_Homebrew(t *testing.T) {
	tests := []struct {
		formula string
		count   int
		want    int
	}{
		{"jq", 0, 1},          // curated
		{"unknown", 50000, 2}, // popular
		{"unknown", 1000, 3},  // standard
	}
	for _, tt := range tests {
		got := assignTier(tt.formula, tt.count)
		if got != tt.want {
			t.Errorf("assignTier(%q, %d) = %d, want %d", tt.formula, tt.count, got, tt.want)
		}
	}
}

func TestParseCount(t *testing.T) {
	if got := parseCount("1,234,567"); got != 1234567 {
		t.Errorf("parseCount(\"1,234,567\") = %d, want 1234567", got)
	}
}

func TestMerge_empty(t *testing.T) {
	q := &PriorityQueue{SchemaVersion: 1, Packages: []Package{}}
	added := q.Merge(nil)
	if added != 0 {
		t.Errorf("expected 0 added, got %d", added)
	}
}

// Ensure we don't accidentally use a real temp file
func TestSave_createsDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "queue.json")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	q := &PriorityQueue{SchemaVersion: 1, Packages: []Package{}}
	if err := q.Save(path); err != nil {
		t.Fatalf("save: %v", err)
	}
}

func TestFilterByName_DeduplicatesCaseInsensitive(t *testing.T) {
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "ripgrep", Source: "cargo:ripgrep"},
			{Name: "Bat", Source: "homebrew:bat"},
		},
	}

	packages := []Package{
		{ID: "cargo:ripgrep", Name: "ripgrep"},     // exists (exact case)
		{ID: "cargo:bat", Name: "bat"},             // exists (different case)
		{ID: "cargo:fd-find", Name: "fd-find"},     // new
		{ID: "cargo:hyperfine", Name: "hyperfine"}, // new
		{ID: "homebrew:RIPGREP", Name: "RIPGREP"},  // exists (upper case)
	}

	kept := FilterByName(packages, queue)
	if len(kept) != 2 {
		t.Fatalf("expected 2 kept, got %d", len(kept))
	}
	names := map[string]bool{}
	for _, p := range kept {
		names[p.Name] = true
	}
	if !names["fd-find"] {
		t.Error("expected fd-find to be kept")
	}
	if !names["hyperfine"] {
		t.Error("expected hyperfine to be kept")
	}
}

func TestFilterByName_EmptyQueue(t *testing.T) {
	queue := &batch.UnifiedQueue{Entries: []batch.QueueEntry{}}
	packages := []Package{
		{Name: "tool1"},
		{Name: "tool2"},
	}
	kept := FilterByName(packages, queue)
	if len(kept) != 2 {
		t.Errorf("expected 2 kept for empty queue, got %d", len(kept))
	}
}

func TestFilterByName_EmptyPackages(t *testing.T) {
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{{Name: "jq"}},
	}
	kept := FilterByName(nil, queue)
	if len(kept) != 0 {
		t.Errorf("expected 0 kept for nil packages, got %d", len(kept))
	}
}
