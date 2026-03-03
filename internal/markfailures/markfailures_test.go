package markfailures

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
)

func TestLoadFailureMap_LegacyBatchFormat(t *testing.T) {
	dir := t.TempDir()
	content := `{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:neovim","category":"missing_dep","blocked_by":["tree-sitter","luv"]},{"package_id":"homebrew:tmux","category":"install_failed"}]}`
	writeJSONL(t, dir, "homebrew-2026-01-01T00-00-00Z.jsonl", content)

	fm, err := LoadFailureMap(dir)
	if err != nil {
		t.Fatal(err)
	}

	// neovim: missing_dep with blocked_by
	neo, ok := fm["neovim"]
	if !ok {
		t.Fatal("expected neovim in failure map")
	}
	if neo.TotalFailures != 1 {
		t.Errorf("neovim.TotalFailures = %d, want 1", neo.TotalFailures)
	}
	if !neo.HasMissingDep {
		t.Error("neovim.HasMissingDep should be true")
	}
	if len(neo.BlockedBy) != 2 {
		t.Errorf("neovim.BlockedBy = %v, want [tree-sitter luv]", neo.BlockedBy)
	}

	// tmux: install_failed
	tmux, ok := fm["tmux"]
	if !ok {
		t.Fatal("expected tmux in failure map")
	}
	if tmux.TotalFailures != 1 {
		t.Errorf("tmux.TotalFailures = %d, want 1", tmux.TotalFailures)
	}
	if !tmux.HasOtherFailure {
		t.Error("tmux.HasOtherFailure should be true")
	}
	if tmux.HasMissingDep {
		t.Error("tmux.HasMissingDep should be false")
	}
}

func TestLoadFailureMap_PerRecipeFormat(t *testing.T) {
	dir := t.TempDir()
	lines := `{"schema_version":1,"recipe":"gitui","platform":"darwin-arm64","category":"generation_failed"}
{"schema_version":1,"recipe":"gitui","platform":"darwin-x86_64","category":"generation_failed"}`
	writeJSONL(t, dir, "batch-2026-01-01T00-00-00Z.jsonl", lines)

	fm, err := LoadFailureMap(dir)
	if err != nil {
		t.Fatal(err)
	}

	gitui, ok := fm["gitui"]
	if !ok {
		t.Fatal("expected gitui in failure map")
	}
	if gitui.TotalFailures != 2 {
		t.Errorf("gitui.TotalFailures = %d, want 2", gitui.TotalFailures)
	}
	if !gitui.HasOtherFailure {
		t.Error("gitui.HasOtherFailure should be true")
	}
}

func TestLoadFailureMap_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	// Two separate batch runs, same package
	writeJSONL(t, dir, "homebrew-run1.jsonl",
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"install_failed"}]}`)
	writeJSONL(t, dir, "homebrew-run2.jsonl",
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"install_failed"}]}`)

	fm, err := LoadFailureMap(dir)
	if err != nil {
		t.Fatal(err)
	}

	ff := fm["ffmpeg"]
	if ff == nil {
		t.Fatal("expected ffmpeg in failure map")
	}
	if ff.TotalFailures != 2 {
		t.Errorf("ffmpeg.TotalFailures = %d, want 2", ff.TotalFailures)
	}
}

func TestLoadFailureMap_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	fm, err := LoadFailureMap(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(fm) != 0 {
		t.Errorf("expected empty map, got %d entries", len(fm))
	}
}

func TestLoadFailureMap_DeduplicatesBlockedBy(t *testing.T) {
	dir := t.TempDir()
	// Same blocked_by dep appears in two records
	writeJSONL(t, dir, "f1.jsonl",
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:vim","category":"missing_dep","blocked_by":["python@3.14"]}]}`)
	writeJSONL(t, dir, "f2.jsonl",
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:vim","category":"missing_dep","blocked_by":["python@3.14"]}]}`)

	fm, err := LoadFailureMap(dir)
	if err != nil {
		t.Fatal(err)
	}

	vim := fm["vim"]
	if vim == nil {
		t.Fatal("expected vim in failure map")
	}
	if len(vim.BlockedBy) != 1 {
		t.Errorf("vim.BlockedBy = %v, want [python@3.14]", vim.BlockedBy)
	}
}

func TestRun_MarkPendingAsFailed(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "f.jsonl",
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:tmux","category":"install_failed"}]}`)

	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "tmux", Source: "homebrew:tmux", Priority: 2, Status: batch.StatusPending, Confidence: "auto", FailureCount: 0},
		},
	}

	result, err := Run(queue, dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.MarkedFailed != 1 {
		t.Errorf("MarkedFailed = %d, want 1", result.MarkedFailed)
	}
	if queue.Entries[0].Status != batch.StatusFailed {
		t.Errorf("status = %s, want failed", queue.Entries[0].Status)
	}
	if queue.Entries[0].FailureCount != 1 {
		t.Errorf("failure_count = %d, want 1", queue.Entries[0].FailureCount)
	}
	if queue.Entries[0].NextRetryAt == nil {
		t.Error("next_retry_at should be set")
	}
}

func TestRun_MarkPendingAsBlocked(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "f.jsonl",
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:neovim","category":"missing_dep","blocked_by":["tree-sitter"]}]}`)

	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "neovim", Source: "homebrew:neovim", Priority: 1, Status: batch.StatusPending, Confidence: "auto", FailureCount: 0},
		},
	}

	result, err := Run(queue, dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.MarkedBlocked != 1 {
		t.Errorf("MarkedBlocked = %d, want 1", result.MarkedBlocked)
	}
	if queue.Entries[0].Status != batch.StatusBlocked {
		t.Errorf("status = %s, want blocked", queue.Entries[0].Status)
	}
}

func TestRun_ExpireBackoff(t *testing.T) {
	dir := t.TempDir()
	// No failure data needed for expiry test, but we need at least an
	// empty dir (LoadFailureMap returns empty map on no files)

	pastTime := time.Now().Add(-2 * time.Hour)
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "tmux", Source: "homebrew:tmux", Priority: 2, Status: batch.StatusFailed, Confidence: "auto", FailureCount: 1, NextRetryAt: &pastTime},
		},
	}

	result, err := Run(queue, dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Retried != 1 {
		t.Errorf("Retried = %d, want 1", result.Retried)
	}
	if queue.Entries[0].Status != batch.StatusPending {
		t.Errorf("status = %s, want pending", queue.Entries[0].Status)
	}
}

func TestRun_DoNotExpireActiveBackoff(t *testing.T) {
	dir := t.TempDir()

	futureTime := time.Now().Add(24 * time.Hour)
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "tmux", Source: "homebrew:tmux", Priority: 2, Status: batch.StatusFailed, Confidence: "auto", FailureCount: 1, NextRetryAt: &futureTime},
		},
	}

	result, err := Run(queue, dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.Retried != 0 {
		t.Errorf("Retried = %d, want 0", result.Retried)
	}
	if queue.Entries[0].Status != batch.StatusFailed {
		t.Errorf("status = %s, want failed", queue.Entries[0].Status)
	}
}

func TestRun_Idempotency(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "f.jsonl",
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:tmux","category":"install_failed"}]}`)

	// Entry already has failure_count matching JSONL data
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "tmux", Source: "homebrew:tmux", Priority: 2, Status: batch.StatusPending, Confidence: "auto", FailureCount: 1},
		},
	}

	result, err := Run(queue, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should NOT be re-marked because failure_count already matches
	if result.MarkedFailed != 0 {
		t.Errorf("MarkedFailed = %d, want 0 (idempotency)", result.MarkedFailed)
	}
	if queue.Entries[0].Status != batch.StatusPending {
		t.Errorf("status = %s, want pending (unchanged)", queue.Entries[0].Status)
	}
}

func TestRun_SkipsSuccessEntries(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "f.jsonl",
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:wget","category":"install_failed"}]}`)

	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "wget", Source: "homebrew:wget", Priority: 2, Status: batch.StatusSuccess, Confidence: "auto", FailureCount: 0},
		},
	}

	result, err := Run(queue, dir)
	if err != nil {
		t.Fatal(err)
	}

	if result.MarkedFailed != 0 {
		t.Errorf("MarkedFailed = %d, want 0", result.MarkedFailed)
	}
	if queue.Entries[0].Status != batch.StatusSuccess {
		t.Errorf("status = %s, want success (unchanged)", queue.Entries[0].Status)
	}
}

func TestRun_MixedFailuresPreferBlocked(t *testing.T) {
	dir := t.TempDir()
	// Package has both missing_dep and install_failed. Missing_dep with
	// blocked_by takes precedence.
	writeJSONL(t, dir, "f.jsonl",
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:vim","category":"missing_dep","blocked_by":["python@3.14"]},{"package_id":"homebrew:vim","category":"install_failed"}]}`)

	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "vim", Source: "homebrew:vim", Priority: 1, Status: batch.StatusPending, Confidence: "auto", FailureCount: 0},
		},
	}

	result, err := Run(queue, dir)
	if err != nil {
		t.Fatal(err)
	}

	// Should be blocked, not failed, because missing_dep takes precedence
	if result.MarkedBlocked != 1 {
		t.Errorf("MarkedBlocked = %d, want 1", result.MarkedBlocked)
	}
	if queue.Entries[0].Status != batch.StatusBlocked {
		t.Errorf("status = %s, want blocked", queue.Entries[0].Status)
	}
}

func TestComputeRetryAt(t *testing.T) {
	now := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		failures int
		wantDur  time.Duration
	}{
		{1, 1 * time.Hour},       // 2^0 = 1h
		{2, 2 * time.Hour},       // 2^1 = 2h
		{3, 4 * time.Hour},       // 2^2 = 4h
		{5, 16 * time.Hour},      // 2^4 = 16h
		{20, 7 * 24 * time.Hour}, // capped at 7 days
	}

	for _, tt := range tests {
		got := computeRetryAt(now, tt.failures)
		want := now.Add(tt.wantDur)
		if !got.Equal(want) {
			t.Errorf("computeRetryAt(now, %d) = %v, want %v (delta %v)",
				tt.failures, got, want, got.Sub(want))
		}
	}
}

func writeJSONL(t *testing.T, dir, name, content string) {
	t.Helper()
	err := os.WriteFile(filepath.Join(dir, name), []byte(content+"\n"), 0644)
	if err != nil {
		t.Fatal(err)
	}
}
