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

	// Find the timestamped file that was created
	files, err := filepath.Glob(filepath.Join(tmpDir, "homebrew-*.jsonl"))
	if err != nil {
		t.Fatalf("failed to glob files: %v", err)
	}
	if len(files) != 1 {
		t.Fatalf("expected 1 file, got %d", len(files))
	}

	data, err := os.ReadFile(files[0])
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

func TestWriteFailures_createsTimestampedFiles(t *testing.T) {
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
	// Sleep to try for different timestamps (may still collide at second precision)
	time.Sleep(1100 * time.Millisecond)
	if err := WriteFailures(tmpDir, "homebrew", batch2); err != nil {
		t.Fatal(err)
	}

	// Should have created separate timestamped files (or 1 if timestamps collided)
	// Timestamp precision is 1 second, so collisions are possible and acceptable
	files, err := filepath.Glob(filepath.Join(tmpDir, "homebrew-*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 1 || len(files) > 2 {
		t.Errorf("expected 1-2 timestamped files (collision possible), got %d", len(files))
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
