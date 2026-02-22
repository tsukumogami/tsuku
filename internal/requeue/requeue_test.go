package requeue

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/batch"
)

// writeJSONL is a test helper that writes lines to a JSONL file.
func writeJSONL(t *testing.T, dir, filename string, lines []string) {
	t.Helper()
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("write JSONL: %v", err)
	}
}

// Scenario 4: no blocked entries in the queue -- Run should return zero
// counts and leave the queue unchanged.
func TestRun_NoBlockedEntries(t *testing.T) {
	failDir := t.TempDir()
	writeJSONL(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:curl","category":"missing_dep","blocked_by":["openssl"]}]}`,
	})

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "openssl", Source: "homebrew:openssl", Priority: 1, Status: "success", Confidence: "auto"},
			{Name: "curl", Source: "homebrew:curl", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "wget", Source: "homebrew:wget", Priority: 2, Status: "failed", Confidence: "auto"},
		},
	}

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Requeued != 0 {
		t.Errorf("Requeued: got %d, want 0", result.Requeued)
	}
	if result.Remaining != 0 {
		t.Errorf("Remaining: got %d, want 0", result.Remaining)
	}
	if len(result.Details) != 0 {
		t.Errorf("Details: got %d entries, want 0", len(result.Details))
	}

	// Statuses should be unchanged
	for _, e := range queue.Entries {
		switch e.Name {
		case "openssl":
			if e.Status != "success" {
				t.Errorf("openssl status: got %q, want success", e.Status)
			}
		case "curl":
			if e.Status != "pending" {
				t.Errorf("curl status: got %q, want pending", e.Status)
			}
		case "wget":
			if e.Status != "failed" {
				t.Errorf("wget status: got %q, want failed", e.Status)
			}
		}
	}
}

// Scenario 4: all blockers resolved -- every blocked entry should flip to
// pending because all dependencies have status "success".
func TestRun_AllBlockersResolved(t *testing.T) {
	failDir := t.TempDir()
	writeJSONL(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:curl","category":"missing_dep","blocked_by":["openssl"]},{"package_id":"homebrew:wget","category":"missing_dep","blocked_by":["openssl","zlib"]}]}`,
	})

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "openssl", Source: "homebrew:openssl", Priority: 1, Status: "success", Confidence: "auto"},
			{Name: "zlib", Source: "homebrew:zlib", Priority: 1, Status: "success", Confidence: "auto"},
			{Name: "curl", Source: "homebrew:curl", Priority: 2, Status: "blocked", Confidence: "auto"},
			{Name: "wget", Source: "homebrew:wget", Priority: 2, Status: "blocked", Confidence: "auto"},
		},
	}

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Requeued != 2 {
		t.Errorf("Requeued: got %d, want 2", result.Requeued)
	}
	if result.Remaining != 0 {
		t.Errorf("Remaining: got %d, want 0", result.Remaining)
	}
	if len(result.Details) != 2 {
		t.Errorf("Details: got %d entries, want 2", len(result.Details))
	}

	// Both should now be pending
	for _, e := range queue.Entries {
		if e.Name == "curl" && e.Status != "pending" {
			t.Errorf("curl status: got %q, want pending", e.Status)
		}
		if e.Name == "wget" && e.Status != "pending" {
			t.Errorf("wget status: got %q, want pending", e.Status)
		}
	}

	// Check Change details
	for _, c := range result.Details {
		if c.Name == "wget" {
			if len(c.ResolvedBy) != 2 {
				t.Errorf("wget ResolvedBy: got %d, want 2", len(c.ResolvedBy))
			}
		}
	}
}

// Scenario 5: partial resolution -- some blocked entries have all blockers
// resolved while others still have unresolved blockers.
func TestRun_PartialResolution(t *testing.T) {
	failDir := t.TempDir()
	writeJSONL(t, failDir, "failures.jsonl", []string{
		// curl blocked by openssl (which is success)
		`{"schema_version":1,"ecosystem":"homebrew","recipe":"curl","category":"missing_dep","blocked_by":["openssl"]}`,
		// ffmpeg blocked by gmp (which is NOT success)
		`{"schema_version":1,"ecosystem":"homebrew","recipe":"ffmpeg","category":"missing_dep","blocked_by":["gmp"]}`,
	})

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "openssl", Source: "homebrew:openssl", Priority: 1, Status: "success", Confidence: "auto"},
			{Name: "gmp", Source: "homebrew:gmp", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "curl", Source: "homebrew:curl", Priority: 2, Status: "blocked", Confidence: "auto"},
			{Name: "ffmpeg", Source: "homebrew:ffmpeg", Priority: 2, Status: "blocked", Confidence: "auto"},
		},
	}

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Requeued != 1 {
		t.Errorf("Requeued: got %d, want 1", result.Requeued)
	}
	if result.Remaining != 1 {
		t.Errorf("Remaining: got %d, want 1", result.Remaining)
	}

	// curl should be flipped to pending
	for _, e := range queue.Entries {
		if e.Name == "curl" && e.Status != "pending" {
			t.Errorf("curl status: got %q, want pending", e.Status)
		}
		if e.Name == "ffmpeg" && e.Status != "blocked" {
			t.Errorf("ffmpeg status: got %q, want blocked", e.Status)
		}
	}

	if len(result.Details) != 1 {
		t.Fatalf("Details: got %d entries, want 1", len(result.Details))
	}
	if result.Details[0].Name != "curl" {
		t.Errorf("Details[0].Name: got %q, want curl", result.Details[0].Name)
	}
}

// Scenario 5: empty failures directory -- LoadBlockerMap should return an
// error (no JSONL files), which Run propagates.
func TestRun_EmptyFailuresDir(t *testing.T) {
	failDir := t.TempDir()

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "curl", Source: "homebrew:curl", Priority: 2, Status: "blocked", Confidence: "auto"},
		},
	}

	_, err := Run(queue, failDir)
	if err == nil {
		t.Fatal("expected error for empty failures directory, got nil")
	}
	if !strings.Contains(err.Error(), "no failure files") {
		t.Errorf("expected 'no failure files' error, got: %v", err)
	}

	// Queue should be unchanged
	if queue.Entries[0].Status != "blocked" {
		t.Errorf("status should be unchanged: got %q", queue.Entries[0].Status)
	}
}

// Scenario 6: multiple blockers where one is unresolved -- the entry should
// remain blocked because not ALL of its blockers are resolved.
func TestRun_MultipleBlockersOneUnresolved(t *testing.T) {
	failDir := t.TempDir()
	writeJSONL(t, failDir, "failures.jsonl", []string{
		// ffmpeg blocked by both gmp and libx264
		`{"schema_version":1,"ecosystem":"homebrew","recipe":"ffmpeg","category":"missing_dep","blocked_by":["gmp","libx264"]}`,
	})

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "gmp", Source: "homebrew:gmp", Priority: 1, Status: "success", Confidence: "auto"},
			{Name: "libx264", Source: "homebrew:libx264", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "ffmpeg", Source: "homebrew:ffmpeg", Priority: 2, Status: "blocked", Confidence: "auto"},
		},
	}

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.Requeued != 0 {
		t.Errorf("Requeued: got %d, want 0", result.Requeued)
	}
	if result.Remaining != 1 {
		t.Errorf("Remaining: got %d, want 1", result.Remaining)
	}

	// ffmpeg should remain blocked
	if queue.Entries[2].Status != "blocked" {
		t.Errorf("ffmpeg status: got %q, want blocked", queue.Entries[2].Status)
	}
}

// Scenario 6: blocked entry with no matching failure record -- the entry
// should remain blocked because we can't determine what's blocking it.
// This can happen when failure data has aged out.
func TestRun_BlockedEntryNoFailureRecord(t *testing.T) {
	failDir := t.TempDir()
	// Failure records only mention curl, not ffmpeg
	writeJSONL(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","recipe":"curl","category":"missing_dep","blocked_by":["openssl"]}`,
	})

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "openssl", Source: "homebrew:openssl", Priority: 1, Status: "success", Confidence: "auto"},
			{Name: "curl", Source: "homebrew:curl", Priority: 2, Status: "blocked", Confidence: "auto"},
			{Name: "ffmpeg", Source: "homebrew:ffmpeg", Priority: 2, Status: "blocked", Confidence: "auto"},
		},
	}

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// curl should be unblocked (openssl resolved)
	if result.Requeued != 1 {
		t.Errorf("Requeued: got %d, want 1", result.Requeued)
	}
	// ffmpeg should remain blocked (no failure record)
	if result.Remaining != 1 {
		t.Errorf("Remaining: got %d, want 1", result.Remaining)
	}

	if queue.Entries[1].Status != "pending" {
		t.Errorf("curl status: got %q, want pending", queue.Entries[1].Status)
	}
	if queue.Entries[2].Status != "blocked" {
		t.Errorf("ffmpeg status: got %q, want blocked", queue.Entries[2].Status)
	}
}

// TestRun_QueueModifiedInPlace verifies that Run modifies the queue entries
// directly rather than returning a new queue.
func TestRun_QueueModifiedInPlace(t *testing.T) {
	failDir := t.TempDir()
	writeJSONL(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","recipe":"curl","category":"missing_dep","blocked_by":["openssl"]}`,
	})

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "openssl", Source: "homebrew:openssl", Priority: 1, Status: "success", Confidence: "auto"},
			{Name: "curl", Source: "homebrew:curl", Priority: 2, Status: "blocked", Confidence: "curated", FailureCount: 5},
		},
	}

	_, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// The same queue pointer should have the updated status
	if queue.Entries[1].Status != "pending" {
		t.Errorf("status not updated in place: got %q, want pending", queue.Entries[1].Status)
	}
	// Other fields should be preserved
	if queue.Entries[1].Confidence != "curated" {
		t.Errorf("confidence changed: got %q, want curated", queue.Entries[1].Confidence)
	}
	if queue.Entries[1].FailureCount != 5 {
		t.Errorf("failure_count changed: got %d, want 5", queue.Entries[1].FailureCount)
	}
}

// TestRun_LegacyBatchFormatFailures verifies that Run correctly processes
// failure data in the legacy batch format (with failures array).
func TestRun_LegacyBatchFormatFailures(t *testing.T) {
	failDir := t.TempDir()
	writeJSONL(t, failDir, "batch.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:curl","category":"missing_dep","blocked_by":["openssl"]},{"package_id":"homebrew:wget","category":"missing_dep","blocked_by":["zlib"]}]}`,
	})

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "openssl", Source: "homebrew:openssl", Priority: 1, Status: "success", Confidence: "auto"},
			{Name: "zlib", Source: "homebrew:zlib", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "curl", Source: "homebrew:curl", Priority: 2, Status: "blocked", Confidence: "auto"},
			{Name: "wget", Source: "homebrew:wget", Priority: 2, Status: "blocked", Confidence: "auto"},
		},
	}

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// curl should be unblocked (openssl is success)
	if result.Requeued != 1 {
		t.Errorf("Requeued: got %d, want 1", result.Requeued)
	}
	// wget should remain blocked (zlib is pending, not success)
	if result.Remaining != 1 {
		t.Errorf("Remaining: got %d, want 1", result.Remaining)
	}

	if queue.Entries[2].Status != "pending" {
		t.Errorf("curl status: got %q, want pending", queue.Entries[2].Status)
	}
	if queue.Entries[3].Status != "blocked" {
		t.Errorf("wget status: got %q, want blocked", queue.Entries[3].Status)
	}
}

// TestRun_ChangeDetailsPopulated verifies that Result.Details contains
// correct Name and ResolvedBy fields for each flipped entry.
func TestRun_ChangeDetailsPopulated(t *testing.T) {
	failDir := t.TempDir()
	writeJSONL(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","recipe":"curl","category":"missing_dep","blocked_by":["openssl","zlib"]}`,
	})

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "openssl", Source: "homebrew:openssl", Priority: 1, Status: "success", Confidence: "auto"},
			{Name: "zlib", Source: "homebrew:zlib", Priority: 1, Status: "success", Confidence: "auto"},
			{Name: "curl", Source: "homebrew:curl", Priority: 2, Status: "blocked", Confidence: "auto"},
		},
	}

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(result.Details) != 1 {
		t.Fatalf("Details: got %d entries, want 1", len(result.Details))
	}

	change := result.Details[0]
	if change.Name != "curl" {
		t.Errorf("Change.Name: got %q, want curl", change.Name)
	}
	if len(change.ResolvedBy) != 2 {
		t.Errorf("Change.ResolvedBy: got %d, want 2", len(change.ResolvedBy))
	}

	// Both openssl and zlib should be in ResolvedBy
	resolvedSet := make(map[string]bool)
	for _, dep := range change.ResolvedBy {
		resolvedSet[dep] = true
	}
	if !resolvedSet["openssl"] {
		t.Error("expected openssl in ResolvedBy")
	}
	if !resolvedSet["zlib"] {
		t.Error("expected zlib in ResolvedBy")
	}
}

// TestBuildReverseIndex verifies the blocker map inversion.
func TestBuildReverseIndex(t *testing.T) {
	blockerMap := map[string][]string{
		"gmp":     {"homebrew:ffmpeg", "homebrew:coreutils"},
		"openssl": {"homebrew:curl", "homebrew:ffmpeg"},
	}

	reverse := buildReverseIndex(blockerMap)

	// ffmpeg is blocked by both gmp and openssl
	if len(reverse["ffmpeg"]) != 2 {
		t.Errorf("ffmpeg blockers: got %d, want 2", len(reverse["ffmpeg"]))
	}

	// curl is blocked by openssl only
	if len(reverse["curl"]) != 1 {
		t.Errorf("curl blockers: got %d, want 1", len(reverse["curl"]))
	}
	if reverse["curl"][0] != "openssl" {
		t.Errorf("curl blocker: got %q, want openssl", reverse["curl"][0])
	}

	// coreutils is blocked by gmp only
	if len(reverse["coreutils"]) != 1 {
		t.Errorf("coreutils blockers: got %d, want 1", len(reverse["coreutils"]))
	}
}

// TestBuildReverseIndex_DeduplicatesDeps verifies that duplicate dependencies
// are not added to the reverse index.
func TestBuildReverseIndex_DeduplicatesDeps(t *testing.T) {
	blockerMap := map[string][]string{
		// gmp blocks ffmpeg twice (from multiple failure records)
		"gmp": {"homebrew:ffmpeg", "homebrew:ffmpeg"},
	}

	reverse := buildReverseIndex(blockerMap)

	// Should have gmp only once for ffmpeg
	if len(reverse["ffmpeg"]) != 1 {
		t.Errorf("ffmpeg blockers: got %v, want [gmp]", reverse["ffmpeg"])
	}
}

// TestBareName verifies extraction of bare names from package IDs.
func TestBareName(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"homebrew:ffmpeg", "ffmpeg"},
		{"cargo:ripgrep", "ripgrep"},
		{"ffmpeg", "ffmpeg"},
		{"a:b:c", "b:c"},
	}

	for _, tt := range tests {
		got := bareName(tt.input)
		if got != tt.want {
			t.Errorf("bareName(%q): got %q, want %q", tt.input, got, tt.want)
		}
	}
}
