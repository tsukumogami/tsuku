package batch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDisambiguationRecord_Fields(t *testing.T) {
	// Verify DisambiguationRecord has all required fields
	record := DisambiguationRecord{
		Tool:            "bat",
		Selected:        "crates.io:sharkdp/bat",
		Alternatives:    []string{"npm:bat-cli", "rubygems:bat"},
		SelectionReason: Selection10xPopularityGap,
		DownloadsRatio:  225.5,
		HighRisk:        false,
	}

	if record.Tool != "bat" {
		t.Errorf("Tool = %q, want %q", record.Tool, "bat")
	}
	if record.Selected != "crates.io:sharkdp/bat" {
		t.Errorf("Selected = %q, want %q", record.Selected, "crates.io:sharkdp/bat")
	}
	if len(record.Alternatives) != 2 {
		t.Errorf("Alternatives length = %d, want 2", len(record.Alternatives))
	}
	if record.SelectionReason != Selection10xPopularityGap {
		t.Errorf("SelectionReason = %q, want %q", record.SelectionReason, Selection10xPopularityGap)
	}
	if record.DownloadsRatio != 225.5 {
		t.Errorf("DownloadsRatio = %f, want 225.5", record.DownloadsRatio)
	}
	if record.HighRisk {
		t.Error("HighRisk = true, want false")
	}
}

func TestDisambiguationRecord_SelectionReasons(t *testing.T) {
	tests := []struct {
		name            string
		selectionReason string
		highRisk        bool
	}{
		{
			name:            "single_match",
			selectionReason: SelectionSingleMatch,
			highRisk:        false,
		},
		{
			name:            "10x_popularity_gap",
			selectionReason: Selection10xPopularityGap,
			highRisk:        false,
		},
		{
			name:            "priority_fallback",
			selectionReason: SelectionPriorityFallback,
			highRisk:        true, // HighRisk is true for priority_fallback
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			record := DisambiguationRecord{
				Tool:            "test-tool",
				Selected:        "ecosystem:source",
				SelectionReason: tt.selectionReason,
				HighRisk:        tt.highRisk,
			}

			if record.SelectionReason != tt.selectionReason {
				t.Errorf("SelectionReason = %q, want %q", record.SelectionReason, tt.selectionReason)
			}
			if record.HighRisk != tt.highRisk {
				t.Errorf("HighRisk = %v, want %v", record.HighRisk, tt.highRisk)
			}
		})
	}
}

func TestDisambiguationRecord_JSONMarshal(t *testing.T) {
	record := DisambiguationRecord{
		Tool:            "bat",
		Selected:        "crates.io:sharkdp/bat",
		Alternatives:    []string{"npm:bat-cli"},
		SelectionReason: Selection10xPopularityGap,
		DownloadsRatio:  100.5,
		HighRisk:        false,
	}

	data, err := json.Marshal(record)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}

	var decoded DisambiguationRecord
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	if decoded.Tool != record.Tool {
		t.Errorf("decoded.Tool = %q, want %q", decoded.Tool, record.Tool)
	}
	if decoded.Selected != record.Selected {
		t.Errorf("decoded.Selected = %q, want %q", decoded.Selected, record.Selected)
	}
	if decoded.SelectionReason != record.SelectionReason {
		t.Errorf("decoded.SelectionReason = %q, want %q", decoded.SelectionReason, record.SelectionReason)
	}
}

func TestBatchResult_Disambiguations(t *testing.T) {
	result := &BatchResult{
		BatchID:      "test-batch",
		Ecosystems:   map[string]int{"homebrew": 3},
		PerEcosystem: map[string]EcosystemResult{},
		Total:        3,
		Succeeded:    2,
		Failed:       1,
		Disambiguations: []DisambiguationRecord{
			{
				Tool:            "bat",
				Selected:        "crates.io:sharkdp/bat",
				SelectionReason: Selection10xPopularityGap,
				HighRisk:        false,
			},
			{
				Tool:            "fd",
				Selected:        "crates.io:sharkdp/fd",
				SelectionReason: SelectionPriorityFallback,
				HighRisk:        true,
			},
		},
	}

	if len(result.Disambiguations) != 2 {
		t.Errorf("Disambiguations length = %d, want 2", len(result.Disambiguations))
	}

	// Check that the summary includes disambiguation info
	summary := result.Summary()
	if !strings.Contains(summary, "Disambiguations: 2") {
		t.Errorf("Summary should mention disambiguations count")
	}
	if !strings.Contains(summary, "1 high-risk") {
		t.Errorf("Summary should mention high-risk count")
	}
	if !strings.Contains(summary, "bat") {
		t.Errorf("Summary should mention 'bat' tool")
	}
}

func TestWriteDisambiguations(t *testing.T) {
	dir := t.TempDir()

	records := []DisambiguationRecord{
		{
			Tool:            "bat",
			Selected:        "crates.io:sharkdp/bat",
			Alternatives:    []string{"npm:bat-cli"},
			SelectionReason: Selection10xPopularityGap,
			DownloadsRatio:  225,
			HighRisk:        false,
		},
	}

	err := WriteDisambiguations(dir, "homebrew", records)
	if err != nil {
		t.Fatalf("WriteDisambiguations failed: %v", err)
	}

	// Verify file was created
	files, err := filepath.Glob(filepath.Join(dir, "homebrew-*.jsonl"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("Expected 1 file, got %d", len(files))
	}

	// Verify content
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var decoded DisambiguationFile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if decoded.SchemaVersion != 1 {
		t.Errorf("SchemaVersion = %d, want 1", decoded.SchemaVersion)
	}
	if decoded.Ecosystem != "homebrew" {
		t.Errorf("Ecosystem = %q, want %q", decoded.Ecosystem, "homebrew")
	}
	if len(decoded.Disambiguations) != 1 {
		t.Errorf("Disambiguations length = %d, want 1", len(decoded.Disambiguations))
	}
}

func TestWriteDisambiguations_Empty(t *testing.T) {
	dir := t.TempDir()

	// Empty records should not create a file
	err := WriteDisambiguations(dir, "homebrew", nil)
	if err != nil {
		t.Fatalf("WriteDisambiguations failed: %v", err)
	}

	files, err := filepath.Glob(filepath.Join(dir, "homebrew-*.jsonl"))
	if err != nil {
		t.Fatalf("Glob failed: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("Expected 0 files for empty records, got %d", len(files))
	}
}
