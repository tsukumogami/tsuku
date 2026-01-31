package batch

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tsukumogami/tsuku/internal/seed"
)

func TestSelectCandidates_filtersCorrectly(t *testing.T) {
	queue := &seed.PriorityQueue{
		SchemaVersion: 1,
		Packages: []seed.Package{
			{ID: "homebrew:ripgrep", Name: "ripgrep", Status: "pending", Tier: 1},
			{ID: "homebrew:bat", Name: "bat", Status: "pending", Tier: 2},
			{ID: "homebrew:fd", Name: "fd", Status: "success", Tier: 1},
			{ID: "cargo:serde", Name: "serde", Status: "pending", Tier: 1},
			{ID: "homebrew:jq", Name: "jq", Status: "pending", Tier: 3},
		},
	}

	orch := NewOrchestrator(Config{
		Ecosystem: "homebrew",
		BatchSize: 10,
		MaxTier:   2,
	}, queue)

	candidates := orch.selectCandidates()

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(candidates))
	}
	if candidates[0].ID != "homebrew:ripgrep" {
		t.Errorf("expected first candidate homebrew:ripgrep, got %s", candidates[0].ID)
	}
	if candidates[1].ID != "homebrew:bat" {
		t.Errorf("expected second candidate homebrew:bat, got %s", candidates[1].ID)
	}
}

func TestSelectCandidates_respectsBatchSize(t *testing.T) {
	queue := &seed.PriorityQueue{
		SchemaVersion: 1,
		Packages: []seed.Package{
			{ID: "homebrew:a", Name: "a", Status: "pending", Tier: 1},
			{ID: "homebrew:b", Name: "b", Status: "pending", Tier: 1},
			{ID: "homebrew:c", Name: "c", Status: "pending", Tier: 1},
		},
	}

	orch := NewOrchestrator(Config{
		Ecosystem: "homebrew",
		BatchSize: 2,
		MaxTier:   3,
	}, queue)

	candidates := orch.selectCandidates()

	if len(candidates) != 2 {
		t.Fatalf("expected 2 candidates (batch size limit), got %d", len(candidates))
	}
}

func TestSelectCandidates_emptyQueue(t *testing.T) {
	queue := &seed.PriorityQueue{SchemaVersion: 1}

	orch := NewOrchestrator(Config{
		Ecosystem: "homebrew",
		BatchSize: 10,
		MaxTier:   3,
	}, queue)

	candidates := orch.selectCandidates()

	if len(candidates) != 0 {
		t.Fatalf("expected 0 candidates, got %d", len(candidates))
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

func TestSetStatus(t *testing.T) {
	queue := &seed.PriorityQueue{
		SchemaVersion: 1,
		Packages: []seed.Package{
			{ID: "homebrew:ripgrep", Status: "pending"},
			{ID: "homebrew:bat", Status: "pending"},
		},
	}

	orch := NewOrchestrator(Config{}, queue)
	orch.setStatus("homebrew:ripgrep", "in_progress")

	if queue.Packages[0].Status != "in_progress" {
		t.Errorf("expected in_progress, got %s", queue.Packages[0].Status)
	}
	if queue.Packages[1].Status != "pending" {
		t.Errorf("expected pending unchanged, got %s", queue.Packages[1].Status)
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

	queue := &seed.PriorityQueue{
		SchemaVersion: 1,
		Packages: []seed.Package{
			{ID: "homebrew:testpkg", Name: "testpkg", Status: "pending", Tier: 1},
		},
	}

	orch := NewOrchestrator(Config{
		Ecosystem:   "homebrew",
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
	if result.Generated != 0 {
		t.Errorf("expected generated 0, got %d", result.Generated)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("expected 1 failure, got %d", len(result.Failures))
	}
	if result.Failures[0].Category != "validation_failed" {
		t.Errorf("expected category validation_failed, got %s", result.Failures[0].Category)
	}

	// Queue status should be updated
	if queue.Packages[0].Status != "failed" {
		t.Errorf("expected queue status failed, got %s", queue.Packages[0].Status)
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

	queue := &seed.PriorityQueue{
		SchemaVersion: 1,
		Packages: []seed.Package{
			{ID: "homebrew:testpkg", Name: "testpkg", Status: "pending", Tier: 1},
		},
	}

	orch := NewOrchestrator(Config{
		Ecosystem:   "homebrew",
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

	if result.Generated != 1 {
		t.Errorf("expected generated 1, got %d", result.Generated)
	}
	if result.Failed != 0 {
		t.Errorf("expected failed 0, got %d", result.Failed)
	}
	if queue.Packages[0].Status != "success" {
		t.Errorf("expected queue status success, got %s", queue.Packages[0].Status)
	}
}

func TestRun_validationFailureMissingDep(t *testing.T) {
	tmpDir := t.TempDir()
	fakeBin := filepath.Join(tmpDir, "tsuku")
	// Fake binary: "create" succeeds, "install" fails with missing dep error
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
    echo "Checking dependencies for coreutils..." >&2
    echo "  Resolving dependency 'dav1d'..." >&2
    echo "Error: registry: recipe dav1d not found in registry" >&2
    echo "" >&2
    echo "Suggestion: Verify the recipe name is correct." >&2
    echo "Error: failed to install dependency 'dav1d': registry: recipe dav1d not found in registry" >&2
    exit 6
    ;;
esac
`
	if err := os.WriteFile(fakeBin, []byte(script), 0755); err != nil {
		t.Fatal(err)
	}

	queue := &seed.PriorityQueue{
		SchemaVersion: 1,
		Packages: []seed.Package{
			{ID: "homebrew:coreutils", Name: "coreutils", Status: "pending", Tier: 1},
		},
	}

	orch := NewOrchestrator(Config{
		Ecosystem:   "homebrew",
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

	if result.Generated != 1 {
		t.Errorf("expected generated 1, got %d", result.Generated)
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
	if f.Category != "missing_dep" {
		t.Errorf("expected category missing_dep, got %s", f.Category)
	}
	if len(f.BlockedBy) != 1 || f.BlockedBy[0] != "dav1d" {
		t.Errorf("expected BlockedBy [dav1d], got %v", f.BlockedBy)
	}

	// Recipe file should be cleaned up
	recipePath := recipeOutputPath(filepath.Join(tmpDir, "recipes"), "coreutils")
	if _, err := os.Stat(recipePath); !os.IsNotExist(err) {
		t.Errorf("expected recipe file to be removed after validation failure")
	}

	if queue.Packages[0].Status != "blocked" {
		t.Errorf("expected queue status blocked, got %s", queue.Packages[0].Status)
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

	queue := &seed.PriorityQueue{
		SchemaVersion: 1,
		Packages: []seed.Package{
			{ID: "homebrew:testpkg", Name: "testpkg", Status: "pending", Tier: 1},
		},
	}

	orch := NewOrchestrator(Config{
		Ecosystem:   "homebrew",
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

func TestClassifyValidationFailure(t *testing.T) {
	tests := []struct {
		name         string
		output       string
		wantCategory string
		wantBlocked  []string
	}{
		{
			name:         "single missing dep",
			output:       "Error: registry: recipe dav1d not found in registry\nError: failed to install dependency 'dav1d'",
			wantCategory: "missing_dep",
			wantBlocked:  []string{"dav1d"},
		},
		{
			name:         "multiple missing deps",
			output:       "recipe libfoo not found in registry\nrecipe libbar not found in registry",
			wantCategory: "missing_dep",
			wantBlocked:  []string{"libfoo", "libbar"},
		},
		{
			name:         "duplicate dep mentioned twice",
			output:       "recipe dav1d not found in registry\nfailed to install dependency 'dav1d': registry: recipe dav1d not found in registry",
			wantCategory: "missing_dep",
			wantBlocked:  []string{"dav1d"},
		},
		{
			name:         "no dep pattern",
			output:       "Error: download failed: 404 Not Found",
			wantCategory: "validation_failed",
			wantBlocked:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			category, blockedBy := classifyValidationFailure([]byte(tt.output))
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
