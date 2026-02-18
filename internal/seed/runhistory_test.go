package seed

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendSeedingRun_CreatesFileAndDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "subdir", "seeding-runs.jsonl")

	entry := SeedingRunEntry{
		RunAt:            time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC),
		SourcesProcessed: []string{"cargo"},
		SourcesFailed:    []string{},
		NewPackages:      10,
		SourceChanges:    []SourceChange{},
		CuratedInvalid:   []CuratedInvalid{},
		Errors:           []string{},
	}

	err := AppendSeedingRun(path, entry)
	if err != nil {
		t.Fatalf("AppendSeedingRun error: %v", err)
	}

	// File should exist.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var parsed SeedingRunEntry
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if !parsed.RunAt.Equal(entry.RunAt) {
		t.Errorf("RunAt = %v, want %v", parsed.RunAt, entry.RunAt)
	}
	if parsed.NewPackages != 10 {
		t.Errorf("NewPackages = %d, want 10", parsed.NewPackages)
	}
}

func TestAppendSeedingRun_AppendsToExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seeding-runs.jsonl")

	entry1 := SeedingRunEntry{
		RunAt:            time.Date(2026, 2, 9, 3, 0, 0, 0, time.UTC),
		SourcesProcessed: []string{"homebrew"},
		SourcesFailed:    []string{},
		NewPackages:      5,
		SourceChanges:    []SourceChange{},
		CuratedInvalid:   []CuratedInvalid{},
		Errors:           []string{},
	}
	entry2 := SeedingRunEntry{
		RunAt:            time.Date(2026, 2, 16, 3, 0, 0, 0, time.UTC),
		SourcesProcessed: []string{"cargo", "npm"},
		SourcesFailed:    []string{},
		NewPackages:      12,
		SourceChanges:    []SourceChange{},
		CuratedInvalid:   []CuratedInvalid{},
		Errors:           []string{},
	}

	if err := AppendSeedingRun(path, entry1); err != nil {
		t.Fatalf("first append error: %v", err)
	}
	if err := AppendSeedingRun(path, entry2); err != nil {
		t.Fatalf("second append error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	var first SeedingRunEntry
	if err := json.Unmarshal([]byte(lines[0]), &first); err != nil {
		t.Fatalf("Unmarshal line 1 error: %v", err)
	}
	if first.NewPackages != 5 {
		t.Errorf("line 1 NewPackages = %d, want 5", first.NewPackages)
	}

	var second SeedingRunEntry
	if err := json.Unmarshal([]byte(lines[1]), &second); err != nil {
		t.Fatalf("Unmarshal line 2 error: %v", err)
	}
	if second.NewPackages != 12 {
		t.Errorf("line 2 NewPackages = %d, want 12", second.NewPackages)
	}
}

func TestAppendSeedingRun_RunAtFieldIsRFC3339(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seeding-runs.jsonl")

	runAt := time.Date(2026, 2, 16, 6, 30, 0, 0, time.UTC)
	entry := SeedingRunEntry{
		RunAt:            runAt,
		SourcesProcessed: []string{},
		SourcesFailed:    []string{},
		SourceChanges:    []SourceChange{},
		CuratedInvalid:   []CuratedInvalid{},
		Errors:           []string{},
	}

	if err := AppendSeedingRun(path, entry); err != nil {
		t.Fatalf("AppendSeedingRun error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(string(data))), &raw); err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}

	runAtStr, ok := raw["run_at"].(string)
	if !ok {
		t.Fatal("run_at is not a string")
	}

	parsed, err := time.Parse(time.RFC3339, runAtStr)
	if err != nil {
		t.Fatalf("run_at is not valid RFC3339: %v", err)
	}
	if !parsed.Equal(runAt) {
		t.Errorf("run_at = %v, want %v", parsed, runAt)
	}
}

func TestAppendSeedingRun_NoTempFileLeftOver(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seeding-runs.jsonl")

	entry := SeedingRunEntry{
		RunAt:            time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC),
		SourcesProcessed: []string{},
		SourcesFailed:    []string{},
		SourceChanges:    []SourceChange{},
		CuratedInvalid:   []CuratedInvalid{},
		Errors:           []string{},
	}

	if err := AppendSeedingRun(path, entry); err != nil {
		t.Fatalf("AppendSeedingRun error: %v", err)
	}

	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Error("temp file should not exist after successful append")
	}
}
