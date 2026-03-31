package updates

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWriteAndReadEntry(t *testing.T) {
	dir := t.TempDir()
	entry := &UpdateCheckEntry{
		Tool:            "node",
		ActiveVersion:   "20.11.0",
		Requested:       "20",
		LatestWithinPin: "20.18.2",
		LatestOverall:   "23.1.0",
		Source:          "GitHub:nodejs/node",
		CheckedAt:       time.Now().Truncate(time.Second),
		ExpiresAt:       time.Now().Add(24 * time.Hour).Truncate(time.Second),
	}

	if err := WriteEntry(dir, entry); err != nil {
		t.Fatalf("WriteEntry: %v", err)
	}

	got, err := ReadEntry(dir, "node")
	if err != nil {
		t.Fatalf("ReadEntry: %v", err)
	}
	if got == nil {
		t.Fatal("ReadEntry returned nil")
	}
	if got.Tool != "node" {
		t.Errorf("Tool = %q, want %q", got.Tool, "node")
	}
	if got.ActiveVersion != "20.11.0" {
		t.Errorf("ActiveVersion = %q, want %q", got.ActiveVersion, "20.11.0")
	}
	if got.Requested != "20" {
		t.Errorf("Requested = %q, want %q", got.Requested, "20")
	}
	if got.LatestWithinPin != "20.18.2" {
		t.Errorf("LatestWithinPin = %q, want %q", got.LatestWithinPin, "20.18.2")
	}
	if got.LatestOverall != "23.1.0" {
		t.Errorf("LatestOverall = %q, want %q", got.LatestOverall, "23.1.0")
	}
}

func TestReadEntryNotFound(t *testing.T) {
	dir := t.TempDir()
	got, err := ReadEntry(dir, "nonexistent")
	if err != nil {
		t.Fatalf("ReadEntry should return nil error for missing file: %v", err)
	}
	if got != nil {
		t.Fatal("ReadEntry should return nil for missing file")
	}
}

func TestReadEntryCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "corrupt.json")
	if err := os.WriteFile(path, []byte("not json{"), 0644); err != nil {
		t.Fatal(err)
	}

	got, err := ReadEntry(dir, "corrupt")
	if err == nil {
		t.Fatal("ReadEntry should return error for corrupt file")
	}
	if got != nil {
		t.Fatal("ReadEntry should return nil entry for corrupt file")
	}
}

func TestWriteEntryCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	entry := &UpdateCheckEntry{
		Tool:          "ripgrep",
		ActiveVersion: "14.0.0",
		LatestOverall: "14.1.0",
		CheckedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}

	if err := WriteEntry(dir, entry); err != nil {
		t.Fatalf("WriteEntry should create directory: %v", err)
	}

	got, err := ReadEntry(dir, "ripgrep")
	if err != nil || got == nil {
		t.Fatal("should be able to read entry after directory creation")
	}
}

func TestWriteEntryAtomic(t *testing.T) {
	dir := t.TempDir()
	entry := &UpdateCheckEntry{
		Tool:          "test-tool",
		ActiveVersion: "1.0.0",
		LatestOverall: "1.1.0",
		CheckedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}

	if err := WriteEntry(dir, entry); err != nil {
		t.Fatal(err)
	}

	// Verify no .tmp files remain
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if filepath.Ext(e.Name()) == ".tmp" {
			t.Errorf("temp file %s should not remain after write", e.Name())
		}
	}
}

func TestReadAllEntries(t *testing.T) {
	dir := t.TempDir()

	// Write two valid entries
	for _, name := range []string{"node", "ripgrep"} {
		entry := &UpdateCheckEntry{
			Tool:          name,
			ActiveVersion: "1.0.0",
			LatestOverall: "2.0.0",
			CheckedAt:     time.Now(),
			ExpiresAt:     time.Now().Add(24 * time.Hour),
		}
		if err := WriteEntry(dir, entry); err != nil {
			t.Fatal(err)
		}
	}

	// Write a corrupt file
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("nope"), 0644); err != nil {
		t.Fatal(err)
	}

	// Write a dotfile (should be skipped)
	if err := os.WriteFile(filepath.Join(dir, ".last-check"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	// Write a non-json file (should be skipped)
	if err := os.WriteFile(filepath.Join(dir, ".lock"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	results, err := ReadAllEntries(dir)
	if err != nil {
		t.Fatalf("ReadAllEntries: %v", err)
	}

	// Should have 2 valid entries (node, ripgrep). Bad, dotfile, and lock are skipped.
	if len(results) != 2 {
		t.Errorf("got %d entries, want 2", len(results))
	}
}

func TestReadAllEntriesMissingDir(t *testing.T) {
	results, err := ReadAllEntries("/nonexistent/path")
	if err != nil {
		t.Fatalf("should return nil for missing directory: %v", err)
	}
	if results != nil {
		t.Fatal("should return nil for missing directory")
	}
}

func TestRemoveEntry(t *testing.T) {
	dir := t.TempDir()
	entry := &UpdateCheckEntry{
		Tool:          "to-remove",
		ActiveVersion: "1.0.0",
		LatestOverall: "1.0.0",
		CheckedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(24 * time.Hour),
	}
	if err := WriteEntry(dir, entry); err != nil {
		t.Fatal(err)
	}

	if err := RemoveEntry(dir, "to-remove"); err != nil {
		t.Fatalf("RemoveEntry: %v", err)
	}

	got, err := ReadEntry(dir, "to-remove")
	if err != nil || got != nil {
		t.Fatal("entry should be gone after remove")
	}
}

func TestRemoveEntryNotFound(t *testing.T) {
	dir := t.TempDir()
	if err := RemoveEntry(dir, "nonexistent"); err != nil {
		t.Fatalf("RemoveEntry should not error on missing file: %v", err)
	}
}

func TestTouchSentinel(t *testing.T) {
	dir := t.TempDir()
	if err := TouchSentinel(dir); err != nil {
		t.Fatalf("TouchSentinel: %v", err)
	}

	path := filepath.Join(dir, SentinelFile)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("sentinel file should exist: %v", err)
	}

	// Mtime should be recent
	if time.Since(info.ModTime()) > 5*time.Second {
		t.Error("sentinel mtime should be recent")
	}
}

func TestTouchSentinelCreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested")
	if err := TouchSentinel(dir); err != nil {
		t.Fatalf("TouchSentinel should create directory: %v", err)
	}

	path := filepath.Join(dir, SentinelFile)
	if _, err := os.Stat(path); err != nil {
		t.Fatal("sentinel file should exist after directory creation")
	}
}

func TestIsCheckStale(t *testing.T) {
	dir := t.TempDir()

	// No sentinel = stale
	if !IsCheckStale(dir, 24*time.Hour) {
		t.Error("missing sentinel should be stale")
	}

	// Fresh sentinel
	if err := TouchSentinel(dir); err != nil {
		t.Fatal(err)
	}
	if IsCheckStale(dir, 24*time.Hour) {
		t.Error("fresh sentinel should not be stale")
	}

	// Expired sentinel (set mtime to 25 hours ago)
	path := filepath.Join(dir, SentinelFile)
	old := time.Now().Add(-25 * time.Hour)
	os.Chtimes(path, old, old)
	if !IsCheckStale(dir, 24*time.Hour) {
		t.Error("expired sentinel should be stale")
	}
}

func TestWriteEntryWithError(t *testing.T) {
	dir := t.TempDir()
	entry := &UpdateCheckEntry{
		Tool:          "failing-tool",
		ActiveVersion: "1.0.0",
		LatestOverall: "",
		CheckedAt:     time.Now(),
		ExpiresAt:     time.Now().Add(24 * time.Hour),
		Error:         "network timeout",
	}

	if err := WriteEntry(dir, entry); err != nil {
		t.Fatal(err)
	}

	got, err := ReadEntry(dir, "failing-tool")
	if err != nil || got == nil {
		t.Fatal("should read error entry")
	}
	if got.Error != "network timeout" {
		t.Errorf("Error = %q, want %q", got.Error, "network timeout")
	}
}
