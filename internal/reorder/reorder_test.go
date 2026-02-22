package reorder

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/batch"
)

// writeFailures is a test helper that writes JSONL failure data to a temp file.
func writeFailures(t *testing.T, dir string, filename string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir failures: %v", err)
	}
	content := ""
	for _, line := range lines {
		content += line + "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("write failures: %v", err)
	}
}

// entryNames returns just the names from a slice of QueueEntry.
func entryNames(entries []batch.QueueEntry) []string {
	names := make([]string, len(entries))
	for i, e := range entries {
		names[i] = e.Name
	}
	return names
}

// Scenario 1: entries with higher blocking counts should sort before entries
// with lower blocking counts within the same tier.
func TestReorder_HighBlockingScoreFirst(t *testing.T) {
	dir := t.TempDir()
	failDir := filepath.Join(dir, "failures")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "alpha", Source: "homebrew:alpha", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "gmp", Source: "homebrew:gmp", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "zlib", Source: "homebrew:zlib", Priority: 2, Status: "pending", Confidence: "auto"},
		},
	}

	// gmp blocks 3 packages, zlib blocks 1, alpha blocks 0
	writeFailures(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"},{"package_id":"homebrew:imagemagick","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"},{"package_id":"homebrew:coreutils","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:curl","category":"missing_dep","blocked_by":["zlib"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
	})

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	names := entryNames(queue.Entries)

	// gmp (score=3) before zlib (score=1) before alpha (score=0)
	if names[0] != "gmp" {
		t.Errorf("expected gmp first, got %s", names[0])
	}
	if names[1] != "zlib" {
		t.Errorf("expected zlib second, got %s", names[1])
	}
	if names[2] != "alpha" {
		t.Errorf("expected alpha third, got %s", names[2])
	}

	if result.TotalEntries != 3 {
		t.Errorf("TotalEntries: got %d, want 3", result.TotalEntries)
	}
}

// Scenario 2: tier boundaries must be preserved -- tier 1 entries always
// appear before tier 2 entries, regardless of blocking scores.
func TestReorder_TierBoundariesPreserved(t *testing.T) {
	dir := t.TempDir()
	failDir := filepath.Join(dir, "failures")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "tier1-noblockers", Source: "homebrew:tier1-noblockers", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "tier2-highblocker", Source: "homebrew:tier2-highblocker", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "tier3-megablocker", Source: "homebrew:tier3-megablocker", Priority: 3, Status: "pending", Confidence: "auto"},
		},
	}

	// tier3-megablocker blocks 10 packages, tier2-highblocker blocks 5, tier1 blocks 0
	lines := []string{}
	for i := 0; i < 10; i++ {
		lines = append(lines, `{"schema_version":1,"ecosystem":"homebrew","recipe":"pkg`+
			string(rune('a'+i))+`","category":"missing_dep","blocked_by":["tier3-megablocker"]}`)
	}
	for i := 0; i < 5; i++ {
		lines = append(lines, `{"schema_version":1,"ecosystem":"homebrew","recipe":"blocked`+
			string(rune('a'+i))+`","category":"missing_dep","blocked_by":["tier2-highblocker"]}`)
	}
	writeFailures(t, failDir, "failures.jsonl", lines)

	_, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	names := entryNames(queue.Entries)

	// Tier 1 must still be first, even though tier 3 has highest blocking score
	if names[0] != "tier1-noblockers" {
		t.Errorf("tier 1 entry must be first, got %s", names[0])
	}
	if names[1] != "tier2-highblocker" {
		t.Errorf("tier 2 entry must be second, got %s", names[1])
	}
	if names[2] != "tier3-megablocker" {
		t.Errorf("tier 3 entry must be third, got %s", names[2])
	}

	// Verify priorities are preserved in output
	for i, entry := range queue.Entries {
		if i > 0 && entry.Priority < queue.Entries[i-1].Priority {
			t.Errorf("tier boundary violated at index %d: priority %d after %d", i, entry.Priority, queue.Entries[i-1].Priority)
		}
	}
}

// Scenario 3: entries with equal blocking scores should be sorted alphabetically
// (stable tiebreaker).
func TestReorder_AlphabeticalTiebreaker(t *testing.T) {
	dir := t.TempDir()
	failDir := filepath.Join(dir, "failures")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "zebra", Source: "homebrew:zebra", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "apple", Source: "homebrew:apple", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "mango", Source: "homebrew:mango", Priority: 2, Status: "pending", Confidence: "auto"},
		},
	}

	// All block exactly 1 package each
	writeFailures(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:p1","category":"missing_dep","blocked_by":["zebra"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:p2","category":"missing_dep","blocked_by":["apple"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:p3","category":"missing_dep","blocked_by":["mango"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
	})

	_, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	names := entryNames(queue.Entries)

	// All same score, should be alphabetical
	if names[0] != "apple" || names[1] != "mango" || names[2] != "zebra" {
		t.Errorf("expected [apple, mango, zebra], got %v", names)
	}
}

// Scenario 4: transitive blocking counts should be used, not just direct counts.
// If gmp blocks ffmpeg, and ffmpeg blocks another package, gmp's score should
// include the transitive chain.
func TestReorder_TransitiveBlockingCounts(t *testing.T) {
	dir := t.TempDir()
	failDir := filepath.Join(dir, "failures")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "gmp", Source: "homebrew:gmp", Priority: 3, Status: "pending", Confidence: "auto"},
			{Name: "zlib", Source: "homebrew:zlib", Priority: 3, Status: "pending", Confidence: "auto"},
		},
	}

	// gmp directly blocks ffmpeg, ffmpeg blocks vlc (transitive chain via gmp)
	// zlib directly blocks 2 packages (curl, wget) but no transitive chain
	writeFailures(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:vlc","category":"missing_dep","blocked_by":["ffmpeg"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:curl","category":"missing_dep","blocked_by":["zlib"],"message":"","timestamp":"2026-01-01T00:00:00Z"},{"package_id":"homebrew:wget","category":"missing_dep","blocked_by":["zlib"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
	})

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	names := entryNames(queue.Entries)

	// gmp: direct=1(ffmpeg) + transitive=1(vlc) = 2
	// zlib: direct=2(curl,wget) + transitive=0 = 2
	// Equal score, alphabetical tiebreaker: gmp before zlib
	if names[0] != "gmp" {
		t.Errorf("expected gmp first (transitive score 2), got %s", names[0])
	}
	if names[1] != "zlib" {
		t.Errorf("expected zlib second (direct score 2), got %s", names[1])
	}

	// Both should have score 2
	if len(result.TopScores) < 2 {
		t.Fatalf("expected at least 2 top scores, got %d", len(result.TopScores))
	}
	for _, s := range result.TopScores {
		if s.Score != 2 {
			t.Errorf("%s score: got %d, want 2", s.Name, s.Score)
		}
	}
}

// Scenario 5: entries with no blocking data should keep their position relative
// to other zero-score entries (alphabetical within tier).
func TestReorder_NoBlockingDataAlphabetical(t *testing.T) {
	dir := t.TempDir()

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "delta", Source: "homebrew:delta", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "bravo", Source: "homebrew:bravo", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "charlie", Source: "homebrew:charlie", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "alpha", Source: "homebrew:alpha", Priority: 1, Status: "pending", Confidence: "auto"},
		},
	}

	// No failures directory at all
	emptyDir := filepath.Join(dir, "empty-failures")
	if err := os.MkdirAll(emptyDir, 0755); err != nil {
		t.Fatal(err)
	}

	result, err := Run(queue, emptyDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	names := entryNames(queue.Entries)

	// All score 0, should be alphabetical
	expected := []string{"alpha", "bravo", "charlie", "delta"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("position %d: got %s, want %s", i, names[i], want)
		}
	}

	if len(result.TopScores) != 0 {
		t.Errorf("TopScores should be empty with no blockers, got %d", len(result.TopScores))
	}
}

// Scenario 6: the tool should handle an empty queue without error.
func TestReorder_EmptyQueue(t *testing.T) {
	dir := t.TempDir()

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries:       []batch.QueueEntry{},
	}

	result, err := Run(queue, filepath.Join(dir, "nonexistent"))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if result.TotalEntries != 0 {
		t.Errorf("TotalEntries: got %d, want 0", result.TotalEntries)
	}
}

// Scenario 7: Run should modify the queue in place without writing any files.
func TestReorder_ModifiesQueueInPlace(t *testing.T) {
	dir := t.TempDir()
	failDir := filepath.Join(dir, "failures")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "zlib", Source: "homebrew:zlib", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "gmp", Source: "homebrew:gmp", Priority: 2, Status: "pending", Confidence: "auto"},
		},
	}

	writeFailures(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
	})

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// Result should report that changes were made
	if result.Reordered == 0 {
		t.Error("expected reorder changes to be reported")
	}

	// Queue should be modified in place: gmp first (has blocking score)
	if queue.Entries[0].Name != "gmp" {
		t.Errorf("expected gmp first in modified queue, got %s", queue.Entries[0].Name)
	}
	if queue.Entries[1].Name != "zlib" {
		t.Errorf("expected zlib second in modified queue, got %s", queue.Entries[1].Name)
	}
}

// Scenario 8: both legacy batch format and per-recipe format failure files
// should contribute to blocking scores.
func TestReorder_MixedFailureFormats(t *testing.T) {
	dir := t.TempDir()
	failDir := filepath.Join(dir, "failures")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "alpha", Source: "homebrew:alpha", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "gmp", Source: "homebrew:gmp", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "openssl", Source: "homebrew:openssl", Priority: 2, Status: "pending", Confidence: "auto"},
		},
	}

	// Legacy format: gmp blocks ffmpeg and coreutils
	// Per-recipe format: openssl blocks curl
	writeFailures(t, failDir, "legacy.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"},{"package_id":"homebrew:coreutils","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
	})
	writeFailures(t, failDir, "per-recipe.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","recipe":"curl","category":"missing_dep","blocked_by":["openssl"]}`,
	})

	_, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	names := entryNames(queue.Entries)

	// gmp (score=2) > openssl (score=1) > alpha (score=0)
	if names[0] != "gmp" {
		t.Errorf("expected gmp first (score=2), got %s", names[0])
	}
	if names[1] != "openssl" {
		t.Errorf("expected openssl second (score=1), got %s", names[1])
	}
	if names[2] != "alpha" {
		t.Errorf("expected alpha third (score=0), got %s", names[2])
	}
}

// TestReorder_EntryFieldsPreserved verifies that reordering preserves all
// fields on QueueEntry (status, confidence, failure_count, etc.).
func TestReorder_EntryFieldsPreserved(t *testing.T) {
	dir := t.TempDir()
	failDir := filepath.Join(dir, "failures")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "zlib", Source: "homebrew:zlib", Priority: 2, Status: "blocked", Confidence: "curated", FailureCount: 3},
			{Name: "gmp", Source: "homebrew:gmp", Priority: 2, Status: "pending", Confidence: "auto", FailureCount: 0},
		},
	}

	writeFailures(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
	})

	_, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// gmp should be first (has blocking score)
	if queue.Entries[0].Name != "gmp" {
		t.Fatalf("expected gmp first, got %s", queue.Entries[0].Name)
	}
	if queue.Entries[0].Status != "pending" {
		t.Errorf("gmp status: got %q, want %q", queue.Entries[0].Status, "pending")
	}
	if queue.Entries[0].Confidence != "auto" {
		t.Errorf("gmp confidence: got %q, want %q", queue.Entries[0].Confidence, "auto")
	}

	// zlib should be second
	if queue.Entries[1].Name != "zlib" {
		t.Fatalf("expected zlib second, got %s", queue.Entries[1].Name)
	}
	if queue.Entries[1].Status != "blocked" {
		t.Errorf("zlib status: got %q, want %q", queue.Entries[1].Status, "blocked")
	}
	if queue.Entries[1].Confidence != "curated" {
		t.Errorf("zlib confidence: got %q, want %q", queue.Entries[1].Confidence, "curated")
	}
	if queue.Entries[1].FailureCount != 3 {
		t.Errorf("zlib failure_count: got %d, want 3", queue.Entries[1].FailureCount)
	}
}

// TestReorder_MultiTierReordering verifies that entries within each tier are
// independently reordered by blocking score.
func TestReorder_MultiTierReordering(t *testing.T) {
	dir := t.TempDir()
	failDir := filepath.Join(dir, "failures")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			// Tier 1
			{Name: "beta", Source: "homebrew:beta", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "openssl", Source: "homebrew:openssl", Priority: 1, Status: "pending", Confidence: "auto"},
			// Tier 2
			{Name: "zlib", Source: "homebrew:zlib", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "gmp", Source: "homebrew:gmp", Priority: 2, Status: "pending", Confidence: "auto"},
			// Tier 3
			{Name: "xyzzy", Source: "homebrew:xyzzy", Priority: 3, Status: "pending", Confidence: "auto"},
			{Name: "bzip2", Source: "homebrew:bzip2", Priority: 3, Status: "pending", Confidence: "auto"},
		},
	}

	writeFailures(t, failDir, "failures.jsonl", []string{
		// openssl blocks 2 packages (tier 1)
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:curl","category":"missing_dep","blocked_by":["openssl"],"message":"","timestamp":"2026-01-01T00:00:00Z"},{"package_id":"homebrew:wget","category":"missing_dep","blocked_by":["openssl"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
		// gmp blocks 3 packages (tier 2)
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"},{"package_id":"homebrew:imagemagick","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"},{"package_id":"homebrew:coreutils","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
		// bzip2 blocks 1 package (tier 3)
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:pigz","category":"missing_dep","blocked_by":["bzip2"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
	})

	_, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	names := entryNames(queue.Entries)

	// Expected order:
	// Tier 1: openssl(2), beta(0)
	// Tier 2: gmp(3), zlib(0)
	// Tier 3: bzip2(1), xyzzy(0)
	expected := []string{"openssl", "beta", "gmp", "zlib", "bzip2", "xyzzy"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("position %d: got %s, want %s (full order: %v)", i, names[i], want, names)
		}
	}
}

// TestReorder_CycleDetection verifies that cycles in the dependency graph
// don't cause infinite loops.
func TestReorder_CycleDetection(t *testing.T) {
	dir := t.TempDir()
	failDir := filepath.Join(dir, "failures")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "A", Source: "homebrew:A", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "B", Source: "homebrew:B", Priority: 2, Status: "pending", Confidence: "auto"},
		},
	}

	// A blocks homebrew:B, B blocks homebrew:A -- a cycle
	writeFailures(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:B","category":"missing_dep","blocked_by":["A"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:A","category":"missing_dep","blocked_by":["B"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
	})

	// Should complete without hanging
	_, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(queue.Entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(queue.Entries))
	}
}

// TestComputeScores verifies score computation for queue entries.
func TestComputeScores(t *testing.T) {
	entries := []batch.QueueEntry{
		{Name: "gmp", Source: "homebrew:gmp", Priority: 2},
		{Name: "zlib", Source: "homebrew:zlib", Priority: 2},
		{Name: "alpha", Source: "homebrew:alpha", Priority: 2},
	}
	blockers := map[string][]string{
		"gmp":  {"homebrew:ffmpeg", "homebrew:coreutils"},
		"zlib": {"homebrew:curl"},
	}

	scores := computeScores(entries, blockers)

	if scores["gmp"] != 2 {
		t.Errorf("gmp score: got %d, want 2", scores["gmp"])
	}
	if scores["zlib"] != 1 {
		t.Errorf("zlib score: got %d, want 1", scores["zlib"])
	}
	if scores["alpha"] != 0 {
		t.Errorf("alpha score: got %d, want 0", scores["alpha"])
	}
}

// TestReorder_ResultReportsMovements verifies the Result struct reports
// which entries moved.
func TestReorder_ResultReportsMovements(t *testing.T) {
	dir := t.TempDir()
	failDir := filepath.Join(dir, "failures")

	queue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries: []batch.QueueEntry{
			{Name: "alpha", Source: "homebrew:alpha", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "gmp", Source: "homebrew:gmp", Priority: 2, Status: "pending", Confidence: "auto"},
		},
	}

	writeFailures(t, failDir, "failures.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["gmp"],"message":"","timestamp":"2026-01-01T00:00:00Z"}]}`,
	})

	result, err := Run(queue, failDir)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// gmp should move from position 1 to position 0 within tier 2
	if result.Reordered < 1 {
		t.Errorf("expected at least 1 reordered entry, got %d", result.Reordered)
	}

	moves, ok := result.EntriesMoved[2]
	if !ok || len(moves) == 0 {
		t.Fatal("expected moves for tier 2")
	}

	// Find gmp's move
	found := false
	for _, m := range moves {
		if m.Name == "gmp" {
			found = true
			if m.From != 1 {
				t.Errorf("gmp From: got %d, want 1", m.From)
			}
			if m.To != 0 {
				t.Errorf("gmp To: got %d, want 0", m.To)
			}
		}
	}
	if !found {
		t.Error("expected gmp in moves list")
	}
}
