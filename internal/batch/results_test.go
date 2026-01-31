package batch

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteFailures_createsFile(t *testing.T) {
	tmpDir := t.TempDir()

	failures := []FailureRecord{
		{
			PackageID: "homebrew:imagemagick",
			Category:  "missing_dep",
			BlockedBy: []string{"libpng", "libjpeg"},
			Message:   "formula imagemagick requires dependencies without tsuku recipes",
			Timestamp: time.Date(2026, 1, 29, 10, 0, 0, 0, time.UTC),
		},
	}

	err := WriteFailures(tmpDir, "homebrew", failures)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	path := filepath.Join(tmpDir, "homebrew.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	var record FailureFile
	if err := json.Unmarshal(data, &record); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	if record.SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", record.SchemaVersion)
	}
	if record.Ecosystem != "homebrew" {
		t.Errorf("expected ecosystem homebrew, got %s", record.Ecosystem)
	}
	if len(record.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(record.Failures))
	}
	if record.Failures[0].PackageID != "homebrew:imagemagick" {
		t.Errorf("expected package_id homebrew:imagemagick, got %s", record.Failures[0].PackageID)
	}
}

func TestWriteFailures_appends(t *testing.T) {
	tmpDir := t.TempDir()

	batch1 := []FailureRecord{
		{PackageID: "homebrew:a", Category: "api_error", Message: "timeout", Timestamp: time.Now().UTC()},
	}
	batch2 := []FailureRecord{
		{PackageID: "homebrew:b", Category: "missing_dep", Message: "needs libfoo", Timestamp: time.Now().UTC()},
	}

	if err := WriteFailures(tmpDir, "homebrew", batch1); err != nil {
		t.Fatal(err)
	}
	if err := WriteFailures(tmpDir, "homebrew", batch2); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "homebrew.jsonl"))
	if err != nil {
		t.Fatal(err)
	}

	// Should have two lines (two JSONL entries)
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("expected 2 JSONL lines, got %d", lines)
	}
}

func TestWriteFailures_emptySlice(t *testing.T) {
	tmpDir := t.TempDir()

	err := WriteFailures(tmpDir, "homebrew", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// File should not exist
	path := filepath.Join(tmpDir, "homebrew.jsonl")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected file to not exist for empty failures")
	}
}
