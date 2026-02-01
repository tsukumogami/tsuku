package discover

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSeedList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.json")
	data := []byte(`{
		"category": "github-release",
		"entries": [
			{"name": "ripgrep", "builder": "github", "source": "BurntSushi/ripgrep"},
			{"name": "fd", "builder": "github", "source": "sharkdp/fd"}
		]
	}`)
	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Fatal(err)
	}

	sl, err := LoadSeedList(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sl.Category != "github-release" {
		t.Errorf("category = %q, want %q", sl.Category, "github-release")
	}
	if len(sl.Entries) != 2 {
		t.Errorf("got %d entries, want 2", len(sl.Entries))
	}
}

func TestLoadSeedDir(t *testing.T) {
	dir := t.TempDir()

	// Write two seed files
	f1 := []byte(`{"category": "a", "entries": [{"name": "tool1", "builder": "github", "source": "o/r1"}]}`)
	f2 := []byte(`{"category": "b", "entries": [{"name": "tool2", "builder": "homebrew", "source": "tool2"}]}`)
	if err := os.WriteFile(filepath.Join(dir, "a.json"), f1, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.json"), f2, 0644); err != nil {
		t.Fatal(err)
	}

	entries, err := LoadSeedDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
}

func TestLoadSeedDir_Empty(t *testing.T) {
	dir := t.TempDir()
	entries, err := LoadSeedDir(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("got %d entries, want 0", len(entries))
	}
}

func TestPriorityQueueToSeedEntries(t *testing.T) {
	pq := &PriorityQueueFile{
		Packages: []PriorityQueueEntry{
			{ID: "homebrew:gh", Source: "homebrew", Name: "gh"},
			{ID: "homebrew:jq", Source: "homebrew", Name: "jq"},
		},
	}
	entries := PriorityQueueToSeedEntries(pq)
	if len(entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(entries))
	}
	if entries[0].Builder != "homebrew" {
		t.Errorf("builder = %q, want homebrew", entries[0].Builder)
	}
	if entries[0].Source != "gh" {
		t.Errorf("source = %q, want gh", entries[0].Source)
	}
}

func TestMergeSeedEntries_OverrideWins(t *testing.T) {
	base := []SeedEntry{
		{Name: "jq", Builder: "homebrew", Source: "jq"},
		{Name: "gh", Builder: "homebrew", Source: "gh"},
	}
	override := []SeedEntry{
		{Name: "jq", Builder: "github", Source: "jqlang/jq"},
		{Name: "fd", Builder: "github", Source: "sharkdp/fd"},
	}
	result := MergeSeedEntries(base, override)

	if len(result) != 3 {
		t.Fatalf("got %d entries, want 3", len(result))
	}

	// jq should be overridden to github builder
	found := false
	for _, e := range result {
		if e.Name == "jq" {
			found = true
			if e.Builder != "github" {
				t.Errorf("jq builder = %q, want github (override)", e.Builder)
			}
		}
	}
	if !found {
		t.Error("jq not found in merged result")
	}
}
