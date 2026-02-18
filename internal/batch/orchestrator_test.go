package batch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSelectCandidates_filtersCorrectly(t *testing.T) {
	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "ripgrep", Source: "homebrew:ripgrep", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "bat", Source: "homebrew:bat", Priority: 2, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "fd", Source: "homebrew:fd", Priority: 1, Status: StatusSuccess, Confidence: ConfidenceAuto},
			{Name: "serde", Source: "cargo:serde", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "jq", Source: "homebrew:jq", Priority: 3, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	// Without ecosystem filter, all pending entries within MaxTier are selected.
	// ripgrep (tier 1), bat (tier 2), serde (tier 1) pass; fd (success) and jq (tier 3) are excluded.
	orch := NewOrchestrator(Config{
		BatchSize: 10,
		MaxTier:   2,
	}, queue)

	candidates := orch.selectCandidates()

	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates, got %d", len(candidates))
	}
	names := make([]string, len(candidates))
	for i, idx := range candidates {
		names[i] = queue.Entries[idx].Name
	}
	expected := []string{"ripgrep", "bat", "serde"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("candidate[%d] = %q, want %q", i, names[i], want)
		}
	}
}

func TestSelectCandidates_respectsBatchSize(t *testing.T) {
	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "a", Source: "homebrew:a", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "b", Source: "homebrew:b", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "c", Source: "homebrew:c", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize: 2,
		MaxTier:   3,
	}, queue)

	candidates := orch.selectCandidates()

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates (batch size limit), got %d", len(candidates))
	}
}

func TestSelectCandidates_emptyQueue(t *testing.T) {
	queue := &UnifiedQueue{SchemaVersion: 1}

	orch := NewOrchestrator(Config{
		BatchSize: 10,
		MaxTier:   3,
	}, queue)

	candidates := orch.selectCandidates()

	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates, got %d", len(candidates))
	}
}

func TestSelectCandidates_skipsBackoffEntries(t *testing.T) {
	future := nowFunc().Add(24 * time.Hour)
	past := nowFunc().Add(-1 * time.Hour)

	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "ready", Source: "homebrew:ready", Priority: 1, Status: StatusFailed, Confidence: ConfidenceAuto, FailureCount: 1, NextRetryAt: nil},
			{Name: "backing-off", Source: "homebrew:backing-off", Priority: 1, Status: StatusFailed, Confidence: ConfidenceAuto, FailureCount: 2, NextRetryAt: &future},
			{Name: "past-backoff", Source: "homebrew:past-backoff", Priority: 1, Status: StatusFailed, Confidence: ConfidenceAuto, FailureCount: 2, NextRetryAt: &past},
			{Name: "pending-ok", Source: "homebrew:pending-ok", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize: 10,
		MaxTier:   3,
	}, queue)

	candidates := orch.selectCandidates()

	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates (skip backing-off), got %d", len(candidates))
	}
	// Should include: ready (idx 0), past-backoff (idx 2), pending-ok (idx 3)
	names := make([]string, len(candidates))
	for i, idx := range candidates {
		names[i] = queue.Entries[idx].Name
	}
	expected := []string{"ready", "past-backoff", "pending-ok"}
	for i, want := range expected {
		if names[i] != want {
			t.Errorf("candidate[%d] = %q, want %q", i, names[i], want)
		}
	}
}

func TestSelectCandidates_includesFailedEntries(t *testing.T) {
	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "pending", Source: "cargo:pending", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "failed-retry", Source: "cargo:failed-retry", Priority: 1, Status: StatusFailed, Confidence: ConfidenceAuto, FailureCount: 1},
			{Name: "blocked", Source: "cargo:blocked", Priority: 1, Status: StatusBlocked, Confidence: ConfidenceAuto},
			{Name: "excluded", Source: "cargo:excluded", Priority: 1, Status: StatusExcluded, Confidence: ConfidenceAuto},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize: 10,
		MaxTier:   3,
	}, queue)

	candidates := orch.selectCandidates()

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates (pending + failed), got %d", len(candidates))
	}
	if queue.Entries[candidates[0]].Name != "pending" {
		t.Errorf("first candidate = %q, want pending", queue.Entries[candidates[0]].Name)
	}
	if queue.Entries[candidates[1]].Name != "failed-retry" {
		t.Errorf("second candidate = %q, want failed-retry", queue.Entries[candidates[1]].Name)
	}
}

func TestSelectCandidates_selectsAllEcosystems(t *testing.T) {
	// Without ecosystem filter, entries from all ecosystems are selected.
	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "bat", Source: "github:sharkdp/bat", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "ripgrep", Source: "cargo:ripgrep", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "jq", Source: "homebrew:jq", Priority: 1, Status: StatusPending, Confidence: ConfidenceCurated},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize: 10,
		MaxTier:   3,
	}, queue)

	candidates := orch.selectCandidates()
	if len(candidates) != 3 {
		t.Fatalf("expected 3 candidates (all ecosystems), got %d", len(candidates))
	}
}

func TestSelectCandidates_filterEcosystem(t *testing.T) {
	// When FilterEcosystem is set, only matching entries are selected.
	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "bat", Source: "github:sharkdp/bat", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "ripgrep", Source: "cargo:ripgrep", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "jq", Source: "homebrew:jq", Priority: 1, Status: StatusPending, Confidence: ConfidenceCurated},
		},
	}

	tests := []struct {
		filter    string
		wantCount int
		wantName  string
	}{
		{"github", 1, "bat"},
		{"cargo", 1, "ripgrep"},
		{"homebrew", 1, "jq"},
		{"npm", 0, ""},
	}

	for _, tt := range tests {
		t.Run(tt.filter, func(t *testing.T) {
			orch := NewOrchestrator(Config{
				BatchSize:       10,
				MaxTier:         3,
				FilterEcosystem: tt.filter,
			}, queue)

			candidates := orch.selectCandidates()
			if len(candidates) != tt.wantCount {
				t.Fatalf("expected %d candidates for %s, got %d", tt.wantCount, tt.filter, len(candidates))
			}
			if tt.wantCount > 0 && queue.Entries[candidates[0]].Name != tt.wantName {
				t.Errorf("expected %s, got %s", tt.wantName, queue.Entries[candidates[0]].Name)
			}
		})
	}
}

func TestSelectCandidates_breakerOpen(t *testing.T) {
	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "bat", Source: "github:sharkdp/bat", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "ripgrep", Source: "cargo:ripgrep", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "jq", Source: "homebrew:jq", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "fzf", Source: "homebrew:fzf", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize: 10,
		MaxTier:   3,
		BreakerState: map[string]string{
			"homebrew": "open",
			"cargo":    "closed",
		},
	}, queue)

	candidates := orch.selectCandidates()

	// homebrew entries should be skipped (breaker open), github and cargo pass (no state = closed).
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates (homebrew breaker open), got %d", len(candidates))
	}
	names := make([]string, len(candidates))
	for i, idx := range candidates {
		names[i] = queue.Entries[idx].Name
	}
	if names[0] != "bat" {
		t.Errorf("candidate[0] = %q, want bat", names[0])
	}
	if names[1] != "ripgrep" {
		t.Errorf("candidate[1] = %q, want ripgrep", names[1])
	}
}

func TestSelectCandidates_breakerHalfOpen(t *testing.T) {
	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "jq", Source: "homebrew:jq", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "fzf", Source: "homebrew:fzf", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "wget", Source: "homebrew:wget", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "ripgrep", Source: "cargo:ripgrep", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "serde", Source: "cargo:serde", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize: 10,
		MaxTier:   3,
		BreakerState: map[string]string{
			"homebrew": "half-open",
			"cargo":    "half-open",
		},
	}, queue)

	candidates := orch.selectCandidates()

	// Half-open allows at most 1 entry per ecosystem: 1 homebrew + 1 cargo = 2 total.
	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates (1 homebrew probe + 1 cargo probe), got %d", len(candidates))
	}
	names := make([]string, len(candidates))
	for i, idx := range candidates {
		names[i] = queue.Entries[idx].Name
	}
	if names[0] != "jq" {
		t.Errorf("candidate[0] = %q, want jq (first homebrew probe)", names[0])
	}
	if names[1] != "ripgrep" {
		t.Errorf("candidate[1] = %q, want ripgrep (first cargo probe)", names[1])
	}
}

func TestRecipeOutputPath(t *testing.T) {
	tests := []struct {
		name     string
		dir      string
		toolName string
		want     string
	}{
		{"simple", "recipes", "ripgrep", "recipes/r/ripgrep.toml"},
		{"uppercase", "recipes", "Bat", "recipes/b/Bat.toml"},
		{"nested dir", "out/recipes", "fd", "out/recipes/f/fd.toml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := recipeOutputPath(tt.dir, tt.toolName)
			if got != tt.want {
				t.Errorf("recipeOutputPath(%q, %q) = %q, want %q", tt.dir, tt.toolName, got, tt.want)
			}
		})
	}
}

func TestCategoryFromExitCode(t *testing.T) {
	tests := []struct {
		code int
		want string
	}{
		{5, "api_error"},
		{6, "validation_failed"},
		{7, "validation_failed"},
		{8, "missing_dep"},
		{9, "deterministic_insufficient"},
		{1, "validation_failed"},
	}

	for _, tt := range tests {
		got := categoryFromExitCode(tt.code)
		if got != tt.want {
			t.Errorf("categoryFromExitCode(%d) = %q, want %q", tt.code, got, tt.want)
		}
	}
}

func TestRun_withFakeBinary(t *testing.T) {
	// Create a fake tsuku binary that always fails with exit code 6
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "tsuku")
	err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho 'deterministic failed' >&2\nexit 6\n"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "testpkg", Source: "homebrew:testpkg", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize:   10,
		MaxTier:     3,
		QueuePath:   filepath.Join(tmpDir, "queue.json"),
		OutputDir:   filepath.Join(tmpDir, "recipes"),
		FailuresDir: filepath.Join(tmpDir, "failures"),
		TsukuBin:    fakeBin,
	}, queue)

	result, err := orch.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Total != 1 {
		t.Errorf("expected total 1, got %d", result.Total)
	}
	if result.Failed != 1 {
		t.Errorf("expected failed 1, got %d", result.Failed)
	}
	if result.Succeeded != 0 {
		t.Errorf("expected succeeded 0, got %d", result.Succeeded)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.Failures))
	}
	if result.Failures[0].Category != "validation_failed" {
		t.Errorf("expected category validation_failed, got %s", result.Failures[0].Category)
	}

	// Queue entry status should be updated
	if queue.Entries[0].Status != StatusFailed {
		t.Errorf("expected queue status failed, got %s", queue.Entries[0].Status)
	}
	// Failure count should be incremented
	if queue.Entries[0].FailureCount != 1 {
		t.Errorf("expected failure_count 1, got %d", queue.Entries[0].FailureCount)
	}
}

func TestRun_successfulGeneration(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "tsuku")
	// Fake binary that succeeds and creates a file
	script := `#!/bin/sh
# Parse --output flag
while [ $# -gt 0 ]; do
  case "$1" in
    --output) shift; mkdir -p "$(dirname "$1")"; echo "[metadata]" > "$1"; shift ;;
    *) shift ;;
  esac
done
exit 0
`
	err := os.WriteFile(fakeBin, []byte(script), 0755)
	if err != nil {
		t.Fatal(err)
	}

	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "testpkg", Source: "homebrew:testpkg", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto, FailureCount: 2},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize:   10,
		MaxTier:     3,
		QueuePath:   filepath.Join(tmpDir, "queue.json"),
		OutputDir:   filepath.Join(tmpDir, "recipes"),
		FailuresDir: filepath.Join(tmpDir, "failures"),
		TsukuBin:    fakeBin,
	}, queue)

	result, err := orch.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Succeeded != 1 {
		t.Errorf("expected succeeded 1, got %d", result.Succeeded)
	}
	if result.Failed != 0 {
		t.Errorf("expected failed 0, got %d", result.Failed)
	}
	if queue.Entries[0].Status != StatusSuccess {
		t.Errorf("expected queue status success, got %s", queue.Entries[0].Status)
	}
	// Success should reset failure count
	if queue.Entries[0].FailureCount != 0 {
		t.Errorf("expected failure_count reset to 0, got %d", queue.Entries[0].FailureCount)
	}
	if queue.Entries[0].NextRetryAt != nil {
		t.Errorf("expected next_retry_at nil after success, got %v", queue.Entries[0].NextRetryAt)
	}
}

func TestRun_validationFailureBlocked(t *testing.T) {
	tests := []struct {
		name         string
		category     string
		exitCode     int
		pkgName      string
		blockedBy    string
		stderrMsg    string
		jsonResponse string
	}{
		{
			name:      "missing_dep",
			category:  "missing_dep",
			exitCode:  8,
			pkgName:   "coreutils",
			blockedBy: "dav1d",
			stderrMsg: `echo "Checking dependencies for coreutils..." >&2
    echo "Error: failed to install dependency 'dav1d'" >&2`,
			jsonResponse: `{"status":"error","category":"missing_dep","message":"failed to install dependency dav1d","missing_recipes":["dav1d"],"exit_code":8}`,
		},
		{
			name:         "recipe_not_found",
			category:     "recipe_not_found",
			exitCode:     3,
			pkgName:      "wget",
			blockedBy:    "libidn2",
			stderrMsg:    `echo "Error: recipe libidn2 not found in registry" >&2`,
			jsonResponse: `{"status":"error","category":"recipe_not_found","message":"recipe libidn2 not found in registry","missing_recipes":["libidn2"],"exit_code":3}`,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			fakeBin := filepath.Join(tmpDir, "tsuku")
			script := fmt.Sprintf(`#!/bin/sh
case "$1" in
  create)
    while [ $# -gt 0 ]; do
      case "$1" in
        --output) shift; mkdir -p "$(dirname "$1")"; echo "[metadata]" > "$1"; shift ;;
        *) shift ;;
      esac
    done
    exit 0
    ;;
  install)
    %s
    echo '%s'
    exit %d
    ;;
esac
`, tc.stderrMsg, tc.jsonResponse, tc.exitCode)
			if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
				t.Fatal(err)
			}

			queue := &UnifiedQueue{
				SchemaVersion: 1,
				Entries: []QueueEntry{
					{Name: tc.pkgName, Source: "homebrew:" + tc.pkgName, Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
				},
			}

			orch := NewOrchestrator(Config{
				BatchSize:   10,
				MaxTier:     3,
				QueuePath:   filepath.Join(tmpDir, "queue.json"),
				OutputDir:   filepath.Join(tmpDir, "recipes"),
				FailuresDir: filepath.Join(tmpDir, "failures"),
				TsukuBin:    fakeBin,
			}, queue)

			result, err := orch.Run()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result.Succeeded != 0 {
				t.Errorf("expected succeeded 0, got %d", result.Succeeded)
			}
			if result.Blocked != 1 {
				t.Errorf("expected blocked 1, got %d", result.Blocked)
			}
			if result.Failed != 0 {
				t.Errorf("expected failed 0, got %d", result.Failed)
			}
			if len(result.Recipes) != 0 {
				t.Errorf("expected 0 validated recipes, got %d", len(result.Recipes))
			}
			if len(result.Failures) != 1 {
				t.Fatalf("expected 1 failure, got %d", len(result.Failures))
			}

			f := result.Failures[0]
			if f.Category != tc.category {
				t.Errorf("expected category %s, got %s", tc.category, f.Category)
			}
			if len(f.BlockedBy) != 1 || f.BlockedBy[0] != tc.blockedBy {
				t.Errorf("expected BlockedBy [%s], got %v", tc.blockedBy, f.BlockedBy)
			}

			// Recipe file should be cleaned up
			recipePath := recipeOutputPath(filepath.Join(tmpDir, "recipes"), tc.pkgName)
			if _, err := os.Stat(recipePath); !os.IsNotExist(err) {
				t.Errorf("expected recipe file to be removed after validation failure")
			}

			if queue.Entries[0].Status != StatusBlocked {
				t.Errorf("expected queue status blocked, got %s", queue.Entries[0].Status)
			}
		})
	}
}

func TestRun_validationFailureGeneric(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "tsuku")
	// Fake binary: "create" succeeds, "install" fails without dep pattern
	script := `#!/bin/sh
case "$1" in
  create)
    while [ $# -gt 0 ]; do
      case "$1" in
        --output) shift; mkdir -p "$(dirname "$1")"; echo "[metadata]" > "$1"; shift ;;
        *) shift ;;
      esac
    done
    exit 0
    ;;
  install)
    echo "Error: download failed: 404 Not Found" >&2
    exit 6
    ;;
esac
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "testpkg", Source: "homebrew:testpkg", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize:   10,
		MaxTier:     3,
		QueuePath:   filepath.Join(tmpDir, "queue.json"),
		OutputDir:   filepath.Join(tmpDir, "recipes"),
		FailuresDir: filepath.Join(tmpDir, "failures"),
		TsukuBin:    fakeBin,
	}, queue)

	result, err := orch.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	f := result.Failures[0]
	if f.Category != "validation_failed" {
		t.Errorf("expected category validation_failed, got %s", f.Category)
	}
	if len(f.BlockedBy) != 0 {
		t.Errorf("expected empty BlockedBy, got %v", f.BlockedBy)
	}
}

func TestParseInstallJSON(t *testing.T) {
	tests := []struct {
		name         string
		stdout       string
		exitCode     int
		wantCategory string
		wantBlocked  []string
	}{
		{
			name:         "valid JSON with missing recipes",
			stdout:       `{"status":"error","category":"missing_dep","message":"failed","missing_recipes":["dav1d","libfoo"],"exit_code":8}`,
			exitCode:     8,
			wantCategory: "missing_dep",
			wantBlocked:  []string{"dav1d", "libfoo"},
		},
		{
			name:         "valid JSON no missing recipes",
			stdout:       `{"status":"error","category":"validation_failed","message":"bad tarball","missing_recipes":[],"exit_code":6}`,
			exitCode:     6,
			wantCategory: "validation_failed",
			wantBlocked:  []string{},
		},
		{
			name:         "invalid JSON falls back to exit code",
			stdout:       "not json at all",
			exitCode:     8,
			wantCategory: "missing_dep",
			wantBlocked:  nil,
		},
		{
			name:         "empty stdout falls back to exit code",
			stdout:       "",
			exitCode:     6,
			wantCategory: "validation_failed",
			wantBlocked:  nil,
		},
		{
			name:         "JSON with empty category uses exit code",
			stdout:       `{"status":"error","category":"","missing_recipes":["x"],"exit_code":8}`,
			exitCode:     8,
			wantCategory: "missing_dep",
			wantBlocked:  []string{"x"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category, blockedBy := parseInstallJSON([]byte(tt.stdout), tt.exitCode)
			if category != tt.wantCategory {
				t.Errorf("category = %q, want %q", category, tt.wantCategory)
			}
			if len(blockedBy) != len(tt.wantBlocked) {
				t.Fatalf("blockedBy = %v, want %v", blockedBy, tt.wantBlocked)
			}
			for i, got := range blockedBy {
				if got != tt.wantBlocked[i] {
					t.Errorf("blockedBy[%d] = %q, want %q", i, got, tt.wantBlocked[i])
				}
			}
		})
	}
}

func TestEcosystemRateLimits(t *testing.T) {
	// Verify all expected ecosystems have rate limits
	expected := map[string]time.Duration{
		"homebrew": 1 * time.Second,
		"cargo":    1 * time.Second,
		"npm":      1 * time.Second,
		"pypi":     1 * time.Second,
		"go":       1 * time.Second,
		"rubygems": 6 * time.Second,
		"cpan":     1 * time.Second,
		"cask":     1 * time.Second,
		"github":   2 * time.Second,
	}
	for eco, want := range expected {
		got, ok := ecosystemRateLimits[eco]
		if !ok {
			t.Errorf("missing rate limit for ecosystem %q", eco)
			continue
		}
		if got != want {
			t.Errorf("ecosystemRateLimits[%q] = %v, want %v", eco, got, want)
		}
	}
}

func TestRun_rateLimiting(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping rate limit timing test in short mode")
	}

	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "tsuku")
	// Fake binary that succeeds for both create and install
	script := `#!/bin/sh
case "$1" in
  create)
    while [ $# -gt 0 ]; do
      case "$1" in
        --output) shift; mkdir -p "$(dirname "$1")"; echo "[metadata]" > "$1"; shift ;;
        *) shift ;;
      esac
    done
    exit 0
    ;;
  install) exit 0 ;;
esac
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "pkg1", Source: "cargo:pkg1", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "pkg2", Source: "cargo:pkg2", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "pkg3", Source: "cargo:pkg3", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	// Temporarily set cargo rate limit to 100ms for fast test
	orig := ecosystemRateLimits["cargo"]
	ecosystemRateLimits["cargo"] = 100 * time.Millisecond
	defer func() { ecosystemRateLimits["cargo"] = orig }()

	orch := NewOrchestrator(Config{
		BatchSize:   10,
		MaxTier:     3,
		QueuePath:   filepath.Join(tmpDir, "queue.json"),
		OutputDir:   filepath.Join(tmpDir, "recipes"),
		FailuresDir: filepath.Join(tmpDir, "failures"),
		TsukuBin:    fakeBin,
	}, queue)

	if err := EnsureOutputDir(filepath.Join(tmpDir, "recipes")); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	result, err := orch.Run()
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Succeeded != 3 {
		t.Fatalf("expected 3 succeeded, got %d", result.Succeeded)
	}

	// 3 packages with 100ms rate limit = at least 200ms (sleep between, not before first)
	minExpected := 180 * time.Millisecond // slight margin for timing
	if elapsed < minExpected {
		t.Errorf("expected at least %v for rate limiting, got %v", minExpected, elapsed)
	}
}

// TestRun_usesSourceDirectly verifies that generate() passes pkg.Source
// to the --from flag instead of constructing a source from the ecosystem name.
func TestRun_usesSourceDirectly(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "tsuku")
	// Fake binary that records the --from argument to a marker file
	script := `#!/bin/sh
case "$1" in
  create)
    MARKER_DIR="` + tmpDir + `"
    while [ $# -gt 0 ]; do
      case "$1" in
        --from) shift; echo "$1" > "$MARKER_DIR/from_arg"; shift ;;
        --output) shift; mkdir -p "$(dirname "$1")"; echo "[metadata]" > "$1"; shift ;;
        *) shift ;;
      esac
    done
    exit 0
    ;;
  install) exit 0 ;;
esac
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	// The key scenario: a package named "bat" whose pre-resolved source is
	// github:sharkdp/bat (not homebrew:bat). The orchestrator must pass
	// "github:sharkdp/bat" to --from, not construct "homebrew:bat".
	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "bat", Source: "github:sharkdp/bat", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize:   10,
		MaxTier:     3,
		QueuePath:   filepath.Join(tmpDir, "queue.json"),
		OutputDir:   filepath.Join(tmpDir, "recipes"),
		FailuresDir: filepath.Join(tmpDir, "failures"),
		TsukuBin:    fakeBin,
	}, queue)

	if _, err := orch.Run(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(tmpDir, "from_arg"))
	if err != nil {
		t.Fatalf("failed to read from_arg marker: %v", err)
	}
	got := string(data)
	if got != "github:sharkdp/bat\n" {
		t.Errorf("--from argument = %q, want %q", got, "github:sharkdp/bat\n")
	}
}

func TestRun_failureCountIncrements(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "tsuku")
	err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho 'failed' >&2\nexit 6\n"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "pkg", Source: "cargo:pkg", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto, FailureCount: 0},
		},
	}

	cfg := Config{
		BatchSize:   10,
		MaxTier:     3,
		QueuePath:   filepath.Join(tmpDir, "queue.json"),
		OutputDir:   filepath.Join(tmpDir, "recipes"),
		FailuresDir: filepath.Join(tmpDir, "failures"),
		TsukuBin:    fakeBin,
	}

	// First failure: count goes to 1
	orch := NewOrchestrator(cfg, queue)
	if _, err := orch.Run(); err != nil {
		t.Fatal(err)
	}
	if queue.Entries[0].FailureCount != 1 {
		t.Errorf("after 1st failure: failure_count = %d, want 1", queue.Entries[0].FailureCount)
	}
	if queue.Entries[0].NextRetryAt != nil {
		t.Errorf("after 1st failure: next_retry_at should be nil (no backoff)")
	}

	// Second failure: count goes to 2, backoff kicks in
	queue.Entries[0].Status = StatusFailed // allow re-selection
	orch2 := NewOrchestrator(cfg, queue)
	if _, err := orch2.Run(); err != nil {
		t.Fatal(err)
	}
	if queue.Entries[0].FailureCount != 2 {
		t.Errorf("after 2nd failure: failure_count = %d, want 2", queue.Entries[0].FailureCount)
	}
	if queue.Entries[0].NextRetryAt == nil {
		t.Fatal("after 2nd failure: next_retry_at should not be nil")
	}
}

func TestCalculateNextRetryAt(t *testing.T) {
	now := time.Date(2026, 2, 15, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		failureCount int
		wantNil      bool
		wantDelay    time.Duration
	}{
		{"1st failure - no backoff", 1, true, 0},
		{"2nd failure - 24h", 2, false, 24 * time.Hour},
		{"3rd failure - 72h", 3, false, 72 * time.Hour},
		{"4th failure - 144h", 4, false, 144 * time.Hour},
		{"5th failure - capped at 7d", 5, false, 7 * 24 * time.Hour},
		{"6th failure - capped at 7d", 6, false, 7 * 24 * time.Hour},
		{"10th failure - capped at 7d", 10, false, 7 * 24 * time.Hour},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := calculateNextRetryAt(tt.failureCount, now)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatal("expected non-nil next_retry_at")
			}
			expected := now.Add(tt.wantDelay)
			if !got.Equal(expected) {
				t.Errorf("next_retry_at = %v, want %v (delay %v)", got, expected, tt.wantDelay)
			}
		})
	}
}

func TestRun_successResetsFailureCount(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "tsuku")
	script := `#!/bin/sh
case "$1" in
  create)
    while [ $# -gt 0 ]; do
      case "$1" in
        --output) shift; mkdir -p "$(dirname "$1")"; echo "[metadata]" > "$1"; shift ;;
        *) shift ;;
      esac
    done
    exit 0
    ;;
  install) exit 0 ;;
esac
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	retryAt := nowFunc().Add(-1 * time.Hour)
	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{
				Name:         "pkg",
				Source:       "cargo:pkg",
				Priority:     1,
				Status:       StatusFailed,
				Confidence:   ConfidenceAuto,
				FailureCount: 3,
				NextRetryAt:  &retryAt,
			},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize:   10,
		MaxTier:     3,
		QueuePath:   filepath.Join(tmpDir, "queue.json"),
		OutputDir:   filepath.Join(tmpDir, "recipes"),
		FailuresDir: filepath.Join(tmpDir, "failures"),
		TsukuBin:    fakeBin,
	}, queue)

	result, err := orch.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Succeeded != 1 {
		t.Fatalf("expected 1 succeeded, got %d", result.Succeeded)
	}

	if queue.Entries[0].FailureCount != 0 {
		t.Errorf("failure_count = %d, want 0 after success", queue.Entries[0].FailureCount)
	}
	if queue.Entries[0].NextRetryAt != nil {
		t.Errorf("next_retry_at should be nil after success, got %v", queue.Entries[0].NextRetryAt)
	}
	if queue.Entries[0].Status != StatusSuccess {
		t.Errorf("status = %q, want %q", queue.Entries[0].Status, StatusSuccess)
	}
}

func TestRun_failureRecordUsesSource(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "tsuku")
	err := os.WriteFile(fakeBin, []byte("#!/bin/sh\necho 'failed' >&2\nexit 6\n"), 0755)
	if err != nil {
		t.Fatal(err)
	}

	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "bat", Source: "github:sharkdp/bat", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize:   10,
		MaxTier:     3,
		QueuePath:   filepath.Join(tmpDir, "queue.json"),
		OutputDir:   filepath.Join(tmpDir, "recipes"),
		FailuresDir: filepath.Join(tmpDir, "failures"),
		TsukuBin:    fakeBin,
	}, queue)

	result, err := orch.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.Failures))
	}
	// The failure record should use Source, not a constructed ID
	if result.Failures[0].PackageID != "github:sharkdp/bat" {
		t.Errorf("PackageID = %q, want %q", result.Failures[0].PackageID, "github:sharkdp/bat")
	}
}

func TestRun_perEcosystemResults(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "tsuku")
	// Fake binary: "create" succeeds for all, "install" succeeds for all
	script := `#!/bin/sh
case "$1" in
  create)
    while [ $# -gt 0 ]; do
      case "$1" in
        --output) shift; mkdir -p "$(dirname "$1")"; echo "[metadata]" > "$1"; shift ;;
        *) shift ;;
      esac
    done
    exit 0
    ;;
  install) exit 0 ;;
esac
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{Name: "jq", Source: "homebrew:jq", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "ripgrep", Source: "cargo:ripgrep", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "bat", Source: "github:sharkdp/bat", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
			{Name: "fzf", Source: "homebrew:fzf", Priority: 1, Status: StatusPending, Confidence: ConfidenceAuto},
		},
	}

	orch := NewOrchestrator(Config{
		BatchSize:   10,
		MaxTier:     3,
		QueuePath:   filepath.Join(tmpDir, "queue.json"),
		OutputDir:   filepath.Join(tmpDir, "recipes"),
		FailuresDir: filepath.Join(tmpDir, "failures"),
		TsukuBin:    fakeBin,
	}, queue)

	result, err := orch.Run()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Total != 4 {
		t.Errorf("total = %d, want 4", result.Total)
	}
	if result.Succeeded != 4 {
		t.Errorf("succeeded = %d, want 4", result.Succeeded)
	}

	// Check Ecosystems counts
	if result.Ecosystems["homebrew"] != 2 {
		t.Errorf("Ecosystems[homebrew] = %d, want 2", result.Ecosystems["homebrew"])
	}
	if result.Ecosystems["cargo"] != 1 {
		t.Errorf("Ecosystems[cargo] = %d, want 1", result.Ecosystems["cargo"])
	}
	if result.Ecosystems["github"] != 1 {
		t.Errorf("Ecosystems[github] = %d, want 1", result.Ecosystems["github"])
	}

	// Check PerEcosystem breakdown
	if result.PerEcosystem["homebrew"].Total != 2 {
		t.Errorf("PerEcosystem[homebrew].Total = %d, want 2", result.PerEcosystem["homebrew"].Total)
	}
	if result.PerEcosystem["homebrew"].Succeeded != 2 {
		t.Errorf("PerEcosystem[homebrew].Succeeded = %d, want 2", result.PerEcosystem["homebrew"].Succeeded)
	}
	if result.PerEcosystem["cargo"].Total != 1 {
		t.Errorf("PerEcosystem[cargo].Total = %d, want 1", result.PerEcosystem["cargo"].Total)
	}
	if result.PerEcosystem["github"].Succeeded != 1 {
		t.Errorf("PerEcosystem[github].Succeeded = %d, want 1", result.PerEcosystem["github"].Succeeded)
	}
}

func TestSaveResults_groupsFailuresByEcosystem(t *testing.T) {
	tmpDir := t.TempDir()
	queuePath := filepath.Join(tmpDir, "queues", "queue.json")
	failuresDir := filepath.Join(tmpDir, "failures")

	queue := &UnifiedQueue{
		SchemaVersion: 1,
		Entries:       []QueueEntry{},
	}

	orch := NewOrchestrator(Config{
		BatchSize:   10,
		MaxTier:     3,
		QueuePath:   queuePath,
		FailuresDir: failuresDir,
	}, queue)

	result := &BatchResult{
		BatchID:      "2026-02-17",
		Ecosystems:   map[string]int{"homebrew": 2, "cargo": 1},
		PerEcosystem: map[string]EcosystemResult{},
		Total:        3,
		Failed:       3,
		Timestamp:    nowFunc(),
		Failures: []FailureRecord{
			{PackageID: "homebrew:jq", Category: "validation_failed", Message: "failed", Timestamp: nowFunc()},
			{PackageID: "cargo:serde", Category: "api_error", Message: "timeout", Timestamp: nowFunc()},
			{PackageID: "homebrew:fzf", Category: "validation_failed", Message: "failed", Timestamp: nowFunc()},
		},
	}

	if err := orch.SaveResults(result); err != nil {
		t.Fatalf("SaveResults failed: %v", err)
	}

	// Check that batch-results.json was written
	resultsPath := filepath.Join(tmpDir, "queues", "batch-results.json")
	data, err := os.ReadFile(resultsPath)
	if err != nil {
		t.Fatalf("failed to read batch-results.json: %v", err)
	}
	var savedResult BatchResult
	if err := json.Unmarshal(data, &savedResult); err != nil {
		t.Fatalf("failed to parse batch-results.json: %v", err)
	}
	if savedResult.Total != 3 {
		t.Errorf("saved result total = %d, want 3", savedResult.Total)
	}

	// Check that failure files were grouped by ecosystem
	entries, err := os.ReadDir(failuresDir)
	if err != nil {
		t.Fatalf("failed to read failures dir: %v", err)
	}

	// Should have at least 2 files (one for homebrew, one for cargo)
	if len(entries) < 2 {
		t.Fatalf("expected at least 2 failure files (grouped by ecosystem), got %d", len(entries))
	}

	// Verify filenames contain ecosystem prefixes
	foundHomebrew := false
	foundCargo := false
	for _, e := range entries {
		name := e.Name()
		if len(name) >= 8 && name[:8] == "homebrew" {
			foundHomebrew = true
		}
		if len(name) >= 5 && name[:5] == "cargo" {
			foundCargo = true
		}
	}
	if !foundHomebrew {
		t.Error("expected a homebrew failure file")
	}
	if !foundCargo {
		t.Error("expected a cargo failure file")
	}
}

func TestGenerateBatchID(t *testing.T) {
	// Save and restore nowFunc
	origNow := nowFunc
	defer func() { nowFunc = origNow }()

	nowFunc = func() time.Time {
		return time.Date(2026, 2, 17, 14, 30, 0, 0, time.UTC)
	}

	got := generateBatchID()
	want := "2026-02-17"
	if got != want {
		t.Errorf("generateBatchID() = %q, want %q", got, want)
	}
}

func TestLoadUnifiedQueue_nonExistent(t *testing.T) {
	q, err := LoadUnifiedQueue(filepath.Join(t.TempDir(), "missing.json"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if q.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", q.SchemaVersion)
	}
	if len(q.Entries) != 0 {
		t.Errorf("entries = %d, want 0", len(q.Entries))
	}
}

func TestLoadSaveUnifiedQueue_roundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "queue.json")

	disambiguated := time.Date(2026, 2, 15, 0, 0, 0, 0, time.UTC)
	retryAt := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)

	original := &UnifiedQueue{
		SchemaVersion: 1,
		Entries: []QueueEntry{
			{
				Name:            "ripgrep",
				Source:          "cargo:ripgrep",
				Priority:        1,
				Status:          StatusPending,
				Confidence:      ConfidenceAuto,
				DisambiguatedAt: &disambiguated,
				FailureCount:    0,
				NextRetryAt:     nil,
			},
			{
				Name:            "bat",
				Source:          "github:sharkdp/bat",
				Priority:        1,
				Status:          StatusFailed,
				Confidence:      ConfidenceCurated,
				DisambiguatedAt: &disambiguated,
				FailureCount:    2,
				NextRetryAt:     &retryAt,
			},
		},
	}

	if err := SaveUnifiedQueue(path, original); err != nil {
		t.Fatalf("SaveUnifiedQueue failed: %v", err)
	}

	loaded, err := LoadUnifiedQueue(path)
	if err != nil {
		t.Fatalf("LoadUnifiedQueue failed: %v", err)
	}

	if loaded.SchemaVersion != 1 {
		t.Errorf("schema_version = %d, want 1", loaded.SchemaVersion)
	}
	if len(loaded.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(loaded.Entries))
	}
	if loaded.Entries[0].Name != "ripgrep" {
		t.Errorf("entry[0].Name = %q, want ripgrep", loaded.Entries[0].Name)
	}
	if loaded.Entries[1].Source != "github:sharkdp/bat" {
		t.Errorf("entry[1].Source = %q, want github:sharkdp/bat", loaded.Entries[1].Source)
	}
	if loaded.Entries[1].FailureCount != 2 {
		t.Errorf("entry[1].FailureCount = %d, want 2", loaded.Entries[1].FailureCount)
	}
	if loaded.Entries[1].NextRetryAt == nil {
		t.Fatal("entry[1].NextRetryAt should not be nil")
	}
}

func TestQueueEntry_Ecosystem(t *testing.T) {
	tests := []struct {
		source string
		want   string
	}{
		{"cargo:ripgrep", "cargo"},
		{"github:sharkdp/bat", "github"},
		{"homebrew:jq", "homebrew"},
		{"npm:prettier", "npm"},
		{"noseparator", ""},
	}

	for _, tt := range tests {
		entry := QueueEntry{Source: tt.source}
		if got := entry.Ecosystem(); got != tt.want {
			t.Errorf("Ecosystem(%q) = %q, want %q", tt.source, got, tt.want)
		}
	}
}
