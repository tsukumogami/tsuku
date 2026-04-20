package notices

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteAndReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	notice := &Notice{
		Tool:             "node",
		AttemptedVersion: "20.18.2",
		Error:            "checksum mismatch",
		Timestamp:        time.Now().Truncate(time.Second),
		Shown:            false,
	}

	if err := WriteNotice(dir, notice); err != nil {
		t.Fatalf("WriteNotice: %v", err)
	}

	all, err := ReadAllNotices(dir)
	if err != nil {
		t.Fatalf("ReadAllNotices: %v", err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d notices, want 1", len(all))
	}
	if all[0].Tool != "node" {
		t.Errorf("Tool = %q, want %q", all[0].Tool, "node")
	}
	if all[0].AttemptedVersion != "20.18.2" {
		t.Errorf("AttemptedVersion = %q, want %q", all[0].AttemptedVersion, "20.18.2")
	}
	if all[0].Error != "checksum mismatch" {
		t.Errorf("Error = %q, want %q", all[0].Error, "checksum mismatch")
	}
	if all[0].Shown {
		t.Error("Shown should be false")
	}
}

func TestReadAllSkipsCorruptAndNonJSON(t *testing.T) {
	dir := t.TempDir()

	// Valid notice
	if err := WriteNotice(dir, &Notice{
		Tool: "good", AttemptedVersion: "1.0", Error: "fail",
		Timestamp: time.Now(), Shown: false,
	}); err != nil {
		t.Fatal(err)
	}

	// Corrupt JSON
	if err := os.WriteFile(filepath.Join(dir, "bad.json"), []byte("not json{"), 0644); err != nil {
		t.Fatal(err)
	}

	// Non-JSON file
	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Dotfile
	if err := os.WriteFile(filepath.Join(dir, ".hidden"), nil, 0644); err != nil {
		t.Fatal(err)
	}

	all, err := ReadAllNotices(dir)
	if err != nil {
		t.Fatalf("ReadAllNotices: %v", err)
	}
	if len(all) != 1 {
		t.Errorf("got %d notices, want 1 (should skip corrupt/non-JSON/dotfiles)", len(all))
	}
}

func TestReadUnshownFilters(t *testing.T) {
	dir := t.TempDir()

	if err := WriteNotice(dir, &Notice{
		Tool: "shown-tool", AttemptedVersion: "1.0", Error: "fail",
		Timestamp: time.Now(), Shown: true,
	}); err != nil {
		t.Fatal(err)
	}

	if err := WriteNotice(dir, &Notice{
		Tool: "unshown-tool", AttemptedVersion: "2.0", Error: "fail",
		Timestamp: time.Now(), Shown: false,
	}); err != nil {
		t.Fatal(err)
	}

	unshown, err := ReadUnshownNotices(dir)
	if err != nil {
		t.Fatalf("ReadUnshownNotices: %v", err)
	}
	if len(unshown) != 1 {
		t.Fatalf("got %d unshown, want 1", len(unshown))
	}
	if unshown[0].Tool != "unshown-tool" {
		t.Errorf("Tool = %q, want %q", unshown[0].Tool, "unshown-tool")
	}
}

func TestMarkShown(t *testing.T) {
	dir := t.TempDir()

	if err := WriteNotice(dir, &Notice{
		Tool: "test-tool", AttemptedVersion: "1.0", Error: "fail",
		Timestamp: time.Now(), Shown: false,
	}); err != nil {
		t.Fatal(err)
	}

	if err := MarkShown(dir, "test-tool"); err != nil {
		t.Fatalf("MarkShown: %v", err)
	}

	all, err := ReadAllNotices(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 || !all[0].Shown {
		t.Error("notice should be marked as shown")
	}
}

func TestMarkShownNonExistent(t *testing.T) {
	dir := t.TempDir()
	// Should be a no-op, not an error
	if err := MarkShown(dir, "nonexistent"); err != nil {
		t.Fatalf("MarkShown on nonexistent should be nil: %v", err)
	}
}

func TestRemoveNotice(t *testing.T) {
	dir := t.TempDir()

	if err := WriteNotice(dir, &Notice{
		Tool: "to-remove", AttemptedVersion: "1.0", Error: "fail",
		Timestamp: time.Now(), Shown: false,
	}); err != nil {
		t.Fatal(err)
	}

	if err := RemoveNotice(dir, "to-remove"); err != nil {
		t.Fatalf("RemoveNotice: %v", err)
	}

	all, err := ReadAllNotices(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Error("notice should be gone after remove")
	}
}

func TestRemoveNoticeNonExistent(t *testing.T) {
	dir := t.TempDir()
	if err := RemoveNotice(dir, "nonexistent"); err != nil {
		t.Fatalf("RemoveNotice on nonexistent should be nil: %v", err)
	}
}

func TestWriteOverwritesPrevious(t *testing.T) {
	dir := t.TempDir()

	if err := WriteNotice(dir, &Notice{
		Tool: "tool", AttemptedVersion: "1.0", Error: "first error",
		Timestamp: time.Now(), Shown: false,
	}); err != nil {
		t.Fatal(err)
	}

	if err := WriteNotice(dir, &Notice{
		Tool: "tool", AttemptedVersion: "2.0", Error: "second error",
		Timestamp: time.Now(), Shown: false,
	}); err != nil {
		t.Fatal(err)
	}

	all, err := ReadAllNotices(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 1 {
		t.Fatalf("got %d notices, want 1 (overwrite)", len(all))
	}
	if all[0].AttemptedVersion != "2.0" {
		t.Errorf("AttemptedVersion = %q, want %q (should be overwritten)", all[0].AttemptedVersion, "2.0")
	}
}

func TestReadAllMissingDir(t *testing.T) {
	all, err := ReadAllNotices("/nonexistent/path")
	if err != nil {
		t.Fatalf("should return nil for missing directory: %v", err)
	}
	if all != nil {
		t.Fatal("should return nil for missing directory")
	}
}

func TestWriteCreatesDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	if err := WriteNotice(dir, &Notice{
		Tool: "tool", AttemptedVersion: "1.0", Error: "fail",
		Timestamp: time.Now(), Shown: false,
	}); err != nil {
		t.Fatalf("WriteNotice should create directory: %v", err)
	}

	all, err := ReadAllNotices(dir)
	if err != nil || len(all) != 1 {
		t.Fatal("should read notice after directory creation")
	}
}

func TestKindDeserializeMissing(t *testing.T) {
	// A JSON object with no "kind" key should produce Kind == "" (KindUpdateResult).
	data := `{"tool":"mytool","attempted_version":"1.0","error":"fail","timestamp":"2024-01-01T00:00:00Z","shown":false}`
	var n Notice
	if err := json.Unmarshal([]byte(data), &n); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if n.Kind != KindUpdateResult {
		t.Errorf("Kind = %q, want %q (KindUpdateResult)", n.Kind, KindUpdateResult)
	}
}

func TestKindRoundTrip(t *testing.T) {
	// Marshal a Notice with Kind = KindAutoApplyResult, unmarshal, assert preserved.
	n := Notice{
		Tool:             "mytool",
		AttemptedVersion: "1.0",
		Error:            "fail",
		Timestamp:        time.Now().Truncate(time.Second),
		Kind:             KindAutoApplyResult,
	}
	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got Notice
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Kind != KindAutoApplyResult {
		t.Errorf("Kind = %q, want %q", got.Kind, KindAutoApplyResult)
	}
}

func TestKindOmitEmptyInJSON(t *testing.T) {
	// Marshal a Notice with Kind == "" and assert the JSON does not contain "kind".
	n := Notice{
		Tool:             "mytool",
		AttemptedVersion: "1.0",
		Error:            "fail",
		Timestamp:        time.Now().Truncate(time.Second),
		Kind:             KindUpdateResult,
	}
	data, err := json.Marshal(n)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(data), "kind") {
		t.Errorf("JSON output should not contain \"kind\" when Kind is empty, got: %s", data)
	}
}

func TestReadAllSortedByTimestamp(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()

	if err := WriteNotice(dir, &Notice{
		Tool: "newer", AttemptedVersion: "2.0", Error: "fail",
		Timestamp: now, Shown: false,
	}); err != nil {
		t.Fatal(err)
	}

	if err := WriteNotice(dir, &Notice{
		Tool: "older", AttemptedVersion: "1.0", Error: "fail",
		Timestamp: now.Add(-1 * time.Hour), Shown: false,
	}); err != nil {
		t.Fatal(err)
	}

	all, err := ReadAllNotices(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("got %d, want 2", len(all))
	}
	if all[0].Tool != "older" {
		t.Errorf("first notice should be older, got %q", all[0].Tool)
	}
	if all[1].Tool != "newer" {
		t.Errorf("second notice should be newer, got %q", all[1].Tool)
	}
}
