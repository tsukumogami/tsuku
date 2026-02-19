package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/batch"
)

func TestExtractDeps(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    []string
	}{
		{
			name:    "single dependency",
			message: "Error: registry: recipe ada-url not found in registry",
			want:    []string{"ada-url"},
		},
		{
			name:    "multiple dependencies",
			message: "recipe glib not found in registry\nrecipe gmp not found in registry",
			want:    []string{"glib", "gmp"},
		},
		{
			name:    "duplicate dependencies deduplicated",
			message: "recipe ada-url not found in registry\nrecipe ada-url not found in registry",
			want:    []string{"ada-url"},
		},
		{
			name:    "no matches",
			message: "some other error message",
			want:    nil,
		},
		{
			name:    "empty message",
			message: "",
			want:    nil,
		},
		{
			name:    "path traversal rejected",
			message: "recipe ../etc/passwd not found in registry",
			want:    nil,
		},
		{
			name:    "slash in name rejected",
			message: "recipe foo/bar not found in registry",
			want:    nil,
		},
		{
			name:    "angle brackets rejected",
			message: "recipe <script> not found in registry",
			want:    nil,
		},
		{
			name:    "valid dep mixed with invalid",
			message: "recipe valid-dep not found in registry\nrecipe ../bad not found in registry",
			want:    []string{"valid-dep"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractDeps(tt.message)
			if len(got) != len(tt.want) {
				t.Fatalf("extractDeps: got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("extractDeps[%d]: got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestIsAlreadyRemediated(t *testing.T) {
	tests := []struct {
		name      string
		category  string
		blockedBy []string
		want      bool
	}{
		{"missing_dep with deps", "missing_dep", []string{"glib"}, true},
		{"recipe_not_found with deps", "recipe_not_found", []string{"glib"}, true},
		{"missing_dep empty deps", "missing_dep", nil, false},
		{"validation_failed with deps", "validation_failed", []string{"glib"}, false},
		{"validation_failed empty deps", "validation_failed", nil, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isAlreadyRemediated(tt.category, tt.blockedBy)
			if got != tt.want {
				t.Errorf("isAlreadyRemediated(%q, %v): got %v, want %v",
					tt.category, tt.blockedBy, got, tt.want)
			}
		})
	}
}

func TestIsValidDependencyName(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"simple", "glib", true},
		{"with-hyphen", "ada-url", true},
		{"with-at", "openssl@3", true},
		{"empty", "", false},
		{"slash", "foo/bar", false},
		{"backslash", "foo\\bar", false},
		{"dot-dot", "foo..bar", false},
		{"lt", "foo<bar", false},
		{"gt", "foo>bar", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isValidDependencyName(tt.input)
			if got != tt.want {
				t.Errorf("isValidDependencyName(%q): got %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestRemediateLine_legacyBatchFormat(t *testing.T) {
	// Legacy batch format with a failure that has "not found in registry" in message
	// but wrong category and empty blocked_by.
	input := `{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:jq","category":"validation_failed","message":"Error: registry: recipe oniguruma not found in registry","timestamp":"2026-02-07T03:03:36Z"}]}`

	stats := &remediationStats{
		UniqueDeps:     make(map[string]bool),
		RemediatedPkgs: make(map[string]bool),
	}

	patched, changed, err := remediateLine(input, stats)
	if err != nil {
		t.Fatalf("remediateLine: %v", err)
	}
	if !changed {
		t.Fatal("expected line to be changed")
	}

	// Parse the patched output.
	var record legacyRecord
	if err := json.Unmarshal([]byte(patched), &record); err != nil {
		t.Fatalf("unmarshal patched: %v", err)
	}

	if len(record.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(record.Failures))
	}

	f := record.Failures[0]
	if f.Category != "missing_dep" {
		t.Errorf("category: got %q, want %q", f.Category, "missing_dep")
	}
	if len(f.BlockedBy) != 1 || f.BlockedBy[0] != "oniguruma" {
		t.Errorf("blocked_by: got %v, want [oniguruma]", f.BlockedBy)
	}

	if stats.RecordsUpdated != 1 {
		t.Errorf("RecordsUpdated: got %d, want 1", stats.RecordsUpdated)
	}
	if !stats.UniqueDeps["oniguruma"] {
		t.Error("oniguruma should be in UniqueDeps")
	}
	if !stats.RemediatedPkgs["homebrew:jq"] {
		t.Error("homebrew:jq should be in RemediatedPkgs")
	}
}

func TestRemediateLine_alreadyRemediated(t *testing.T) {
	// Record already has correct category and blocked_by -- should not change.
	input := `{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:node","category":"missing_dep","blocked_by":["ada-url"],"message":"recipe ada-url not found in registry","timestamp":"2026-02-07T03:03:36Z"}]}`

	stats := &remediationStats{
		UniqueDeps:     make(map[string]bool),
		RemediatedPkgs: make(map[string]bool),
	}

	_, changed, err := remediateLine(input, stats)
	if err != nil {
		t.Fatalf("remediateLine: %v", err)
	}
	if changed {
		t.Fatal("line should not be changed (already remediated)")
	}
	if stats.RecordsUpdated != 0 {
		t.Errorf("RecordsUpdated: got %d, want 0", stats.RecordsUpdated)
	}
}

func TestRemediateLine_perRecipeFormat(t *testing.T) {
	// Per-recipe format should be skipped entirely.
	input := `{"schema_version":1,"recipe":"watchexec","platform":"linux-alpine-musl-x86_64","exit_code":6,"category":"deterministic","timestamp":"2026-02-18T14:01:11Z"}`

	stats := &remediationStats{
		UniqueDeps:     make(map[string]bool),
		RemediatedPkgs: make(map[string]bool),
	}

	_, changed, err := remediateLine(input, stats)
	if err != nil {
		t.Fatalf("remediateLine: %v", err)
	}
	if changed {
		t.Fatal("per-recipe format should not be changed")
	}
	if stats.RecordsScanned != 1 {
		t.Errorf("RecordsScanned: got %d, want 1", stats.RecordsScanned)
	}
}

func TestRemediateLine_noMatchInMessage(t *testing.T) {
	// Legacy format but message doesn't contain "not found in registry".
	input := `{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:uv","category":"validation_failed","message":"some other error","timestamp":"2026-02-07T03:03:36Z"}]}`

	stats := &remediationStats{
		UniqueDeps:     make(map[string]bool),
		RemediatedPkgs: make(map[string]bool),
	}

	_, changed, err := remediateLine(input, stats)
	if err != nil {
		t.Fatalf("remediateLine: %v", err)
	}
	if changed {
		t.Fatal("line without matching message should not be changed")
	}
}

func TestRemediateFile_integration(t *testing.T) {
	dir := t.TempDir()

	// Write a JSONL file with mixed records.
	lines := []string{
		// Needs remediation: validation_failed with matching message.
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:jq","category":"validation_failed","message":"Error: registry: recipe oniguruma not found in registry","timestamp":"2026-02-07T03:03:36Z"}]}`,
		// Already correct: missing_dep with blocked_by.
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:node","category":"missing_dep","blocked_by":["ada-url"],"message":"recipe ada-url not found in registry","timestamp":"2026-02-07T03:03:36Z"}]}`,
		// Per-recipe format: skipped.
		`{"schema_version":1,"recipe":"watchexec","platform":"linux-x86_64","exit_code":6,"category":"deterministic","timestamp":"2026-02-18T14:01:11Z"}`,
	}

	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	stats := &remediationStats{
		UniqueDeps:     make(map[string]bool),
		RemediatedPkgs: make(map[string]bool),
	}

	if err := remediateFile(path, stats); err != nil {
		t.Fatalf("remediateFile: %v", err)
	}

	if stats.RecordsUpdated != 1 {
		t.Errorf("RecordsUpdated: got %d, want 1", stats.RecordsUpdated)
	}
	if stats.RecordsScanned != 3 {
		t.Errorf("RecordsScanned: got %d, want 3 (2 legacy failures + 1 per-recipe)", stats.RecordsScanned)
	}

	// Read the patched file and verify.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read patched file: %v", err)
	}

	patchedLines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(patchedLines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(patchedLines))
	}

	// First line should be patched.
	var record legacyRecord
	if err := json.Unmarshal([]byte(patchedLines[0]), &record); err != nil {
		t.Fatalf("unmarshal patched line: %v", err)
	}
	if record.Failures[0].Category != "missing_dep" {
		t.Errorf("patched category: got %q, want %q", record.Failures[0].Category, "missing_dep")
	}
	if len(record.Failures[0].BlockedBy) != 1 || record.Failures[0].BlockedBy[0] != "oniguruma" {
		t.Errorf("patched blocked_by: got %v, want [oniguruma]", record.Failures[0].BlockedBy)
	}
}

func TestRemediateFile_idempotent(t *testing.T) {
	dir := t.TempDir()

	line := `{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:jq","category":"validation_failed","message":"Error: registry: recipe oniguruma not found in registry","timestamp":"2026-02-07T03:03:36Z"}]}`

	path := filepath.Join(dir, "test.jsonl")
	if err := os.WriteFile(path, []byte(line+"\n"), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	// First run: should change.
	stats1 := &remediationStats{
		UniqueDeps:     make(map[string]bool),
		RemediatedPkgs: make(map[string]bool),
	}
	if err := remediateFile(path, stats1); err != nil {
		t.Fatalf("first run: %v", err)
	}
	if stats1.RecordsUpdated != 1 {
		t.Fatalf("first run RecordsUpdated: got %d, want 1", stats1.RecordsUpdated)
	}

	// Capture file contents after first run.
	after1, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after first run: %v", err)
	}

	// Second run: should not change.
	stats2 := &remediationStats{
		UniqueDeps:     make(map[string]bool),
		RemediatedPkgs: make(map[string]bool),
	}
	if err := remediateFile(path, stats2); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if stats2.RecordsUpdated != 0 {
		t.Errorf("second run RecordsUpdated: got %d, want 0", stats2.RecordsUpdated)
	}

	// File contents should be identical.
	after2, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second run: %v", err)
	}
	if string(after1) != string(after2) {
		t.Errorf("file changed on second run:\n  after1: %s\n  after2: %s", after1, after2)
	}
}

func TestRemediateQueue(t *testing.T) {
	dir := t.TempDir()
	queuePath := filepath.Join(dir, "priority-queue.json")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "jq", Source: "homebrew:jq", Priority: 1, Status: "failed", Confidence: "curated"},
			{Name: "bat", Source: "homebrew:bat", Priority: 1, Status: "failed", Confidence: "curated"},
			{Name: "fd", Source: "homebrew:fd", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "node", Source: "homebrew:node", Priority: 1, Status: "blocked", Confidence: "auto"},
		},
	}

	if err := batch.SaveUnifiedQueue(queuePath, queue); err != nil {
		t.Fatalf("save queue: %v", err)
	}

	stats := &remediationStats{
		UniqueDeps: make(map[string]bool),
		RemediatedPkgs: map[string]bool{
			"homebrew:jq": true, // This one was remediated in failures.
			// homebrew:bat was NOT remediated, so should stay "failed".
		},
	}

	if err := remediateQueue(queuePath, stats); err != nil {
		t.Fatalf("remediateQueue: %v", err)
	}

	if stats.QueueFlipped != 1 {
		t.Errorf("QueueFlipped: got %d, want 1", stats.QueueFlipped)
	}

	// Reload and verify.
	updated, err := batch.LoadUnifiedQueue(queuePath)
	if err != nil {
		t.Fatalf("reload queue: %v", err)
	}

	for _, entry := range updated.Entries {
		switch entry.Name {
		case "jq":
			if entry.Status != "blocked" {
				t.Errorf("jq status: got %q, want %q", entry.Status, "blocked")
			}
		case "bat":
			if entry.Status != "failed" {
				t.Errorf("bat status: got %q, want %q (not remediated)", entry.Status, "failed")
			}
		case "fd":
			if entry.Status != "pending" {
				t.Errorf("fd status: got %q, want %q", entry.Status, "pending")
			}
		case "node":
			if entry.Status != "blocked" {
				t.Errorf("node status: got %q, want %q", entry.Status, "blocked")
			}
		}
	}
}

func TestRemediateQueue_noChanges(t *testing.T) {
	dir := t.TempDir()
	queuePath := filepath.Join(dir, "priority-queue.json")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "jq", Source: "homebrew:jq", Priority: 1, Status: "blocked", Confidence: "curated"},
		},
	}

	if err := batch.SaveUnifiedQueue(queuePath, queue); err != nil {
		t.Fatalf("save queue: %v", err)
	}

	// Get file mod time before.
	info1, _ := os.Stat(queuePath)

	stats := &remediationStats{
		UniqueDeps:     make(map[string]bool),
		RemediatedPkgs: map[string]bool{"homebrew:jq": true},
	}

	if err := remediateQueue(queuePath, stats); err != nil {
		t.Fatalf("remediateQueue: %v", err)
	}

	// Queue should not be rewritten (jq is already blocked).
	if stats.QueueFlipped != 0 {
		t.Errorf("QueueFlipped: got %d, want 0", stats.QueueFlipped)
	}

	// Verify file was not rewritten by checking it wasn't modified.
	info2, _ := os.Stat(queuePath)
	if info2.ModTime() != info1.ModTime() {
		t.Error("queue file was modified when no changes were needed")
	}
}

func TestRemediateLine_multipleFailuresInOneRecord(t *testing.T) {
	// A legacy record with multiple failures -- some need remediation, some don't.
	input := `{"schema_version":1,"ecosystem":"homebrew","failures":[` +
		`{"package_id":"homebrew:jq","category":"validation_failed","message":"recipe oniguruma not found in registry","timestamp":"2026-02-07T03:03:36Z"},` +
		`{"package_id":"homebrew:uv","category":"validation_failed","message":"some other error","timestamp":"2026-02-07T03:03:37Z"},` +
		`{"package_id":"homebrew:node","category":"missing_dep","blocked_by":["ada-url"],"message":"recipe ada-url not found in registry","timestamp":"2026-02-07T03:03:38Z"}` +
		`]}`

	stats := &remediationStats{
		UniqueDeps:     make(map[string]bool),
		RemediatedPkgs: make(map[string]bool),
	}

	patched, changed, err := remediateLine(input, stats)
	if err != nil {
		t.Fatalf("remediateLine: %v", err)
	}
	if !changed {
		t.Fatal("expected line to be changed")
	}

	// Should have scanned 3 records, updated 1.
	if stats.RecordsScanned != 3 {
		t.Errorf("RecordsScanned: got %d, want 3", stats.RecordsScanned)
	}
	if stats.RecordsUpdated != 1 {
		t.Errorf("RecordsUpdated: got %d, want 1", stats.RecordsUpdated)
	}

	// Verify the patched output.
	var record legacyRecord
	if err := json.Unmarshal([]byte(patched), &record); err != nil {
		t.Fatalf("unmarshal patched: %v", err)
	}

	// jq should be patched.
	if record.Failures[0].Category != "missing_dep" {
		t.Errorf("jq category: got %q, want %q", record.Failures[0].Category, "missing_dep")
	}
	if len(record.Failures[0].BlockedBy) != 1 || record.Failures[0].BlockedBy[0] != "oniguruma" {
		t.Errorf("jq blocked_by: got %v, want [oniguruma]", record.Failures[0].BlockedBy)
	}

	// uv should be unchanged.
	if record.Failures[1].Category != "validation_failed" {
		t.Errorf("uv category: got %q, want %q", record.Failures[1].Category, "validation_failed")
	}

	// node should be unchanged (already correct).
	if record.Failures[2].Category != "missing_dep" {
		t.Errorf("node category: got %q, want %q", record.Failures[2].Category, "missing_dep")
	}
}

func TestSortedKeys(t *testing.T) {
	m := map[string]bool{"zlib": true, "ada-url": true, "glib": true}
	got := sortedKeys(m)
	want := []string{"ada-url", "glib", "zlib"}

	if len(got) != len(want) {
		t.Fatalf("sortedKeys: got %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("sortedKeys[%d]: got %q, want %q", i, got[i], want[i])
		}
	}
}
