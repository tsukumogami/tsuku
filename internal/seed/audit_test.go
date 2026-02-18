package seed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
)

func TestAuditEntry_JSONRoundTrip(t *testing.T) {
	now := time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC)
	prev := "homebrew:ripgrep"

	entry := AuditEntry{
		DisambiguationRecord: batch.DisambiguationRecord{
			Tool:            "ripgrep",
			Selected:        "cargo:ripgrep",
			Alternatives:    []string{"homebrew:ripgrep", "github:BurntSushi/ripgrep"},
			SelectionReason: "10x_popularity_gap",
			DownloadsRatio:  14.0,
			HighRisk:        false,
		},
		ProbeResults: []AuditProbeResult{
			{Source: "cargo:ripgrep", Downloads: 1250000, VersionCount: 47, HasRepository: true},
			{Source: "homebrew:ripgrep", Downloads: 89000, VersionCount: 12, HasRepository: true},
		},
		PreviousSource:  &prev,
		DisambiguatedAt: now,
		SeedingRun:      now,
	}

	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// Verify flattened structure: DisambiguationRecord fields at top level.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}

	requiredKeys := []string{
		"tool", "selected", "alternatives", "selection_reason",
		"downloads_ratio", "high_risk", "probe_results",
		"previous_source", "disambiguated_at", "seeding_run",
	}
	for _, key := range requiredKeys {
		if _, ok := raw[key]; !ok {
			t.Errorf("expected top-level key %q in JSON output", key)
		}
	}

	// Round-trip: unmarshal back.
	var decoded AuditEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Tool != "ripgrep" {
		t.Errorf("Tool = %q, want ripgrep", decoded.Tool)
	}
	if decoded.Selected != "cargo:ripgrep" {
		t.Errorf("Selected = %q, want cargo:ripgrep", decoded.Selected)
	}
	if decoded.SelectionReason != "10x_popularity_gap" {
		t.Errorf("SelectionReason = %q, want 10x_popularity_gap", decoded.SelectionReason)
	}
	if decoded.DownloadsRatio != 14.0 {
		t.Errorf("DownloadsRatio = %f, want 14.0", decoded.DownloadsRatio)
	}
	if decoded.HighRisk {
		t.Error("HighRisk should be false")
	}
	if len(decoded.Alternatives) != 2 {
		t.Errorf("Alternatives count = %d, want 2", len(decoded.Alternatives))
	}
	if len(decoded.ProbeResults) != 2 {
		t.Errorf("ProbeResults count = %d, want 2", len(decoded.ProbeResults))
	}
	if decoded.ProbeResults[0].Source != "cargo:ripgrep" {
		t.Errorf("ProbeResults[0].Source = %q, want cargo:ripgrep", decoded.ProbeResults[0].Source)
	}
	if decoded.ProbeResults[0].Downloads != 1250000 {
		t.Errorf("ProbeResults[0].Downloads = %d, want 1250000", decoded.ProbeResults[0].Downloads)
	}
	if decoded.PreviousSource == nil || *decoded.PreviousSource != "homebrew:ripgrep" {
		t.Errorf("PreviousSource = %v, want homebrew:ripgrep", decoded.PreviousSource)
	}
	if !decoded.DisambiguatedAt.Equal(now) {
		t.Errorf("DisambiguatedAt = %v, want %v", decoded.DisambiguatedAt, now)
	}
	if !decoded.SeedingRun.Equal(now) {
		t.Errorf("SeedingRun = %v, want %v", decoded.SeedingRun, now)
	}
}

func TestAuditEntry_NullPreviousSource(t *testing.T) {
	entry := AuditEntry{
		DisambiguationRecord: batch.DisambiguationRecord{
			Tool:            "httpie",
			Selected:        "pypi:httpie",
			SelectionReason: "single_match",
		},
		ProbeResults:    []AuditProbeResult{{Source: "pypi:httpie", Downloads: 50000}},
		PreviousSource:  nil,
		DisambiguatedAt: time.Now().UTC(),
		SeedingRun:      time.Now().UTC(),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	// previous_source should be null in JSON.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal raw: %v", err)
	}
	if string(raw["previous_source"]) != "null" {
		t.Errorf("previous_source = %s, want null", string(raw["previous_source"]))
	}
}

func TestWriteAuditEntry_CreatesDirectoryAndFile(t *testing.T) {
	dir := t.TempDir()
	auditDir := filepath.Join(dir, "nested", "audit")

	entry := AuditEntry{
		DisambiguationRecord: batch.DisambiguationRecord{
			Tool:            "ripgrep",
			Selected:        "cargo:ripgrep",
			Alternatives:    []string{"homebrew:ripgrep"},
			SelectionReason: "10x_popularity_gap",
			DownloadsRatio:  14.0,
		},
		ProbeResults: []AuditProbeResult{
			{Source: "cargo:ripgrep", Downloads: 1250000, VersionCount: 47, HasRepository: true},
		},
		DisambiguatedAt: time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC),
		SeedingRun:      time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC),
	}

	if err := WriteAuditEntry(auditDir, entry); err != nil {
		t.Fatalf("WriteAuditEntry: %v", err)
	}

	// Verify file exists.
	path := filepath.Join(auditDir, "ripgrep.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}

	// Verify it's valid JSON with indentation.
	var decoded AuditEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("parse written file: %v", err)
	}
	if decoded.Tool != "ripgrep" {
		t.Errorf("Tool = %q, want ripgrep", decoded.Tool)
	}
	if decoded.Selected != "cargo:ripgrep" {
		t.Errorf("Selected = %q, want cargo:ripgrep", decoded.Selected)
	}
}

func TestWriteAuditEntry_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()

	first := AuditEntry{
		DisambiguationRecord: batch.DisambiguationRecord{
			Tool:            "tool",
			Selected:        "npm:tool",
			SelectionReason: "single_match",
		},
		DisambiguatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		SeedingRun:      time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}

	if err := WriteAuditEntry(dir, first); err != nil {
		t.Fatalf("write first: %v", err)
	}

	// Write a second entry for the same tool with different data.
	prev := "npm:tool"
	second := AuditEntry{
		DisambiguationRecord: batch.DisambiguationRecord{
			Tool:            "tool",
			Selected:        "cargo:tool",
			SelectionReason: "10x_popularity_gap",
		},
		PreviousSource:  &prev,
		DisambiguatedAt: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
		SeedingRun:      time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC),
	}

	if err := WriteAuditEntry(dir, second); err != nil {
		t.Fatalf("write second: %v", err)
	}

	// Read back and verify the second entry won.
	entry, err := ReadAuditEntry(dir, "tool")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if entry.Selected != "cargo:tool" {
		t.Errorf("Selected = %q, want cargo:tool (overwritten)", entry.Selected)
	}
	if entry.PreviousSource == nil || *entry.PreviousSource != "npm:tool" {
		t.Errorf("PreviousSource = %v, want npm:tool", entry.PreviousSource)
	}
}

func TestReadAuditEntry_MissingFile(t *testing.T) {
	dir := t.TempDir()

	entry, err := ReadAuditEntry(dir, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry != nil {
		t.Fatalf("expected nil for missing file, got %+v", entry)
	}
}

func TestReadAuditEntry_ParsesValidFile(t *testing.T) {
	dir := t.TempDir()

	// Write a valid audit file manually.
	content := `{
  "tool": "bat",
  "selected": "cargo:bat",
  "alternatives": ["homebrew:bat"],
  "selection_reason": "10x_popularity_gap",
  "downloads_ratio": 12.5,
  "high_risk": false,
  "probe_results": [
    {"source": "cargo:bat", "downloads": 900000, "version_count": 35, "has_repository": true},
    {"source": "homebrew:bat", "downloads": 72000, "version_count": 8, "has_repository": true}
  ],
  "previous_source": null,
  "disambiguated_at": "2026-02-16T06:00:00Z",
  "seeding_run": "2026-02-16T06:00:00Z"
}
`
	if err := os.WriteFile(filepath.Join(dir, "bat.json"), []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	entry, err := ReadAuditEntry(dir, "bat")
	if err != nil {
		t.Fatalf("ReadAuditEntry: %v", err)
	}
	if entry == nil {
		t.Fatal("expected non-nil entry")
	}
	if entry.Tool != "bat" {
		t.Errorf("Tool = %q, want bat", entry.Tool)
	}
	if entry.Selected != "cargo:bat" {
		t.Errorf("Selected = %q, want cargo:bat", entry.Selected)
	}
	if entry.DownloadsRatio != 12.5 {
		t.Errorf("DownloadsRatio = %f, want 12.5", entry.DownloadsRatio)
	}
	if len(entry.ProbeResults) != 2 {
		t.Errorf("ProbeResults count = %d, want 2", len(entry.ProbeResults))
	}
	if entry.PreviousSource != nil {
		t.Errorf("PreviousSource = %v, want nil", entry.PreviousSource)
	}
	expected := time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC)
	if !entry.DisambiguatedAt.Equal(expected) {
		t.Errorf("DisambiguatedAt = %v, want %v", entry.DisambiguatedAt, expected)
	}
}

func TestReadAuditEntry_InvalidJSON(t *testing.T) {
	dir := t.TempDir()

	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("{invalid"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	_, err := ReadAuditEntry(dir, "bad")
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestHasSource(t *testing.T) {
	entry := &AuditEntry{
		ProbeResults: []AuditProbeResult{
			{Source: "cargo:ripgrep", Downloads: 1250000},
			{Source: "homebrew:ripgrep", Downloads: 89000},
		},
	}

	tests := []struct {
		source string
		want   bool
	}{
		{"cargo:ripgrep", true},
		{"homebrew:ripgrep", true},
		{"github:BurntSushi/ripgrep", false},
		{"npm:ripgrep", false},
		{"", false},
	}

	for _, tt := range tests {
		got := HasSource(entry, tt.source)
		if got != tt.want {
			t.Errorf("HasSource(entry, %q) = %v, want %v", tt.source, got, tt.want)
		}
	}
}

func TestHasSource_NilEntry(t *testing.T) {
	if HasSource(nil, "cargo:ripgrep") {
		t.Error("HasSource(nil, ...) should return false")
	}
}

func TestHasSource_EmptyProbeResults(t *testing.T) {
	entry := &AuditEntry{}
	if HasSource(entry, "cargo:ripgrep") {
		t.Error("HasSource with empty ProbeResults should return false")
	}
}

func TestPreviousSource_PopulatedCorrectly(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC)

	// First write: new package, no previous source.
	first := AuditEntry{
		DisambiguationRecord: batch.DisambiguationRecord{
			Tool:            "fd",
			Selected:        "homebrew:fd",
			SelectionReason: "single_match",
		},
		ProbeResults:    []AuditProbeResult{{Source: "homebrew:fd", Downloads: 50000}},
		PreviousSource:  nil,
		DisambiguatedAt: now,
		SeedingRun:      now,
	}

	if err := WriteAuditEntry(dir, first); err != nil {
		t.Fatalf("write first: %v", err)
	}

	// Verify first write has null previous_source.
	entry, err := ReadAuditEntry(dir, "fd")
	if err != nil {
		t.Fatalf("read first: %v", err)
	}
	if entry.PreviousSource != nil {
		t.Errorf("first write: PreviousSource = %v, want nil", entry.PreviousSource)
	}

	// Second write: re-disambiguation with previous_source set.
	prev := "homebrew:fd"
	second := AuditEntry{
		DisambiguationRecord: batch.DisambiguationRecord{
			Tool:            "fd",
			Selected:        "cargo:fd-find",
			SelectionReason: "10x_popularity_gap",
			DownloadsRatio:  15.0,
		},
		ProbeResults: []AuditProbeResult{
			{Source: "cargo:fd-find", Downloads: 800000, VersionCount: 30, HasRepository: true},
			{Source: "homebrew:fd", Downloads: 50000, VersionCount: 5, HasRepository: true},
		},
		PreviousSource:  &prev,
		DisambiguatedAt: now.Add(24 * time.Hour),
		SeedingRun:      now.Add(24 * time.Hour),
	}

	if err := WriteAuditEntry(dir, second); err != nil {
		t.Fatalf("write second: %v", err)
	}

	// Verify second write has previous_source set.
	entry, err = ReadAuditEntry(dir, "fd")
	if err != nil {
		t.Fatalf("read second: %v", err)
	}
	if entry.PreviousSource == nil {
		t.Fatal("second write: PreviousSource should not be nil")
	}
	if *entry.PreviousSource != "homebrew:fd" {
		t.Errorf("second write: PreviousSource = %q, want homebrew:fd", *entry.PreviousSource)
	}
	if entry.Selected != "cargo:fd-find" {
		t.Errorf("second write: Selected = %q, want cargo:fd-find", entry.Selected)
	}
}
