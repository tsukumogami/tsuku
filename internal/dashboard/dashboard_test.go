package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/batch"
)

func TestComputeQueueStatus_aggregates(t *testing.T) {
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "jq", Source: "homebrew:jq", Priority: 1, Status: "success", Confidence: "curated"},
			{Name: "fd", Source: "homebrew:fd", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "bat", Source: "homebrew:bat", Priority: 1, Status: "failed", Confidence: "auto"},
			{Name: "fzf", Source: "homebrew:fzf", Priority: 2, Status: "pending", Confidence: "auto"},
			{Name: "ripgrep", Source: "cargo:ripgrep", Priority: 2, Status: "blocked", Confidence: "curated"},
			{Name: "exa", Source: "homebrew:exa", Priority: 2, Status: "success", Confidence: "auto"},
		},
	}

	status := computeQueueStatus(queue, nil)

	if status.Total != 6 {
		t.Errorf("Total: got %d, want 6", status.Total)
	}

	// Check by_status aggregation
	wantByStatus := map[string]int{
		"success": 2,
		"pending": 2,
		"failed":  1,
		"blocked": 1,
	}
	for s, want := range wantByStatus {
		if got := status.ByStatus[s]; got != want {
			t.Errorf("ByStatus[%q]: got %d, want %d", s, got, want)
		}
	}

	// Check by_tier aggregation
	if tier1 := status.ByTier[1]; tier1["success"] != 1 || tier1["pending"] != 1 || tier1["failed"] != 1 {
		t.Errorf("Tier 1 breakdown incorrect: %v", tier1)
	}
	if tier2 := status.ByTier[2]; tier2["success"] != 1 || tier2["pending"] != 1 || tier2["blocked"] != 1 {
		t.Errorf("Tier 2 breakdown incorrect: %v", tier2)
	}
}

func TestComputeQueueStatus_empty(t *testing.T) {
	queue := &batch.UnifiedQueue{Entries: []batch.QueueEntry{}}
	status := computeQueueStatus(queue, nil)

	if status.Total != 0 {
		t.Errorf("Total: got %d, want 0", status.Total)
	}
	if len(status.ByStatus) != 0 {
		t.Errorf("ByStatus should be empty: %v", status.ByStatus)
	}
}

func TestLoadFailures_legacyFormat(t *testing.T) {
	path := filepath.Join("testdata", "failures.jsonl")
	blockers, categories, details, err := loadFailures(path)
	if err != nil {
		t.Fatalf("loadFailures: %v", err)
	}

	// Check blockers extracted from legacy format
	// glib blocks: imagemagick, ffmpeg
	if len(blockers["glib"]) != 2 {
		t.Errorf("glib blockers: got %d, want 2", len(blockers["glib"]))
	}
	// gmp blocks: imagemagick, coreutils
	if len(blockers["gmp"]) != 2 {
		t.Errorf("gmp blockers: got %d, want 2", len(blockers["gmp"]))
	}

	// Check categories from both formats
	// Legacy: 2 missing_dep, 1 validation_failed
	// Per-recipe: 1 api_error, 1 validation_failed
	if categories["missing_dep"] != 2 {
		t.Errorf("missing_dep: got %d, want 2", categories["missing_dep"])
	}
	if categories["validation_failed"] != 2 {
		t.Errorf("validation_failed: got %d, want 2", categories["validation_failed"])
	}
	if categories["api_error"] != 1 {
		t.Errorf("api_error: got %d, want 1", categories["api_error"])
	}

	// Check details are captured
	if len(details) == 0 {
		t.Error("details should not be empty for legacy format")
	}
}

func TestLoadFailures_perRecipeFormat(t *testing.T) {
	// Create temp file with only per-recipe format records
	dir := t.TempDir()
	path := filepath.Join(dir, "per-recipe.jsonl")
	content := `{"recipe": "node", "platform": "linux-x86_64", "exit_code": 1, "category": "api_error"}
{"recipe": "python", "platform": "darwin-arm64", "exit_code": 1, "category": "validation_failed"}
{"recipe": "ruby", "platform": "linux-arm64", "exit_code": 1, "category": "api_error"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	blockers, categories, _, err := loadFailures(path)
	if err != nil {
		t.Fatalf("loadFailures: %v", err)
	}

	// These per-recipe records don't have blocked_by, so blockers should be empty
	if len(blockers) != 0 {
		t.Errorf("blockers should be empty for per-recipe format without blocked_by: %v", blockers)
	}

	// Check category counts
	if categories["api_error"] != 2 {
		t.Errorf("api_error: got %d, want 2", categories["api_error"])
	}
	if categories["validation_failed"] != 1 {
		t.Errorf("validation_failed: got %d, want 1", categories["validation_failed"])
	}
}

func TestLoadFailures_perRecipeWithBlockedBy(t *testing.T) {
	// Create temp file with per-recipe format including blocked_by
	dir := t.TempDir()
	path := filepath.Join(dir, "per-recipe-blocked.jsonl")
	content := `{"schema_version":1,"recipe":"node","platform":"linux-x86_64","exit_code":8,"category":"missing_dep","blocked_by":["ada-url"]}
{"schema_version":1,"recipe":"ffmpeg","platform":"linux-x86_64","exit_code":8,"category":"missing_dep","blocked_by":["dav1d","glib"]}
{"schema_version":1,"recipe":"procs","platform":"linux-x86_64","exit_code":6,"category":"deterministic"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	blockers, categories, details, err := loadFailures(path)
	if err != nil {
		t.Fatalf("loadFailures: %v", err)
	}

	// Check blockers were extracted from per-recipe format
	if len(blockers) != 3 {
		t.Errorf("blockers: got %d entries, want 3 (ada-url, dav1d, glib)", len(blockers))
	}
	if len(blockers["ada-url"]) != 1 {
		t.Errorf("ada-url blocks: got %d, want 1", len(blockers["ada-url"]))
	}
	if len(blockers["dav1d"]) != 1 {
		t.Errorf("dav1d blocks: got %d, want 1", len(blockers["dav1d"]))
	}

	// Check details were populated
	if len(details) != 2 {
		t.Errorf("details: got %d entries, want 2 (node, ffmpeg)", len(details))
	}
	if details["homebrew:node"].Category != "missing_dep" {
		t.Errorf("node category: got %q, want %q", details["homebrew:node"].Category, "missing_dep")
	}
	if len(details["homebrew:ffmpeg"].BlockedBy) != 2 {
		t.Errorf("ffmpeg blocked_by: got %d, want 2", len(details["homebrew:ffmpeg"].BlockedBy))
	}

	// Check category counts
	if categories["missing_dep"] != 2 {
		t.Errorf("missing_dep: got %d, want 2", categories["missing_dep"])
	}
	if categories["deterministic"] != 1 {
		t.Errorf("deterministic: got %d, want 1", categories["deterministic"])
	}
}

func TestLoadFailures_missingFile(t *testing.T) {
	_, _, _, err := loadFailures("/nonexistent/path.jsonl")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadFailures_malformedLines(t *testing.T) {
	path := filepath.Join("testdata", "malformed.jsonl")
	blockers, categories, _, err := loadFailures(path)
	if err != nil {
		t.Fatalf("loadFailures: %v", err)
	}

	// Should have processed valid lines only (3 valid per-recipe records)
	// valid1: success, valid2: api_error, valid3: missing_dep
	if categories["success"] != 1 {
		t.Errorf("success: got %d, want 1", categories["success"])
	}
	if categories["api_error"] != 1 {
		t.Errorf("api_error: got %d, want 1", categories["api_error"])
	}
	if categories["missing_dep"] != 1 {
		t.Errorf("missing_dep: got %d, want 1", categories["missing_dep"])
	}

	// No blockers in per-recipe format
	if len(blockers) != 0 {
		t.Errorf("blockers should be empty: %v", blockers)
	}
}

func TestComputeTopBlockers_deduplication(t *testing.T) {
	// Simulate a dependency blocking the same package multiple times
	blockers := map[string][]string{
		"glib": {"imagemagick", "ffmpeg", "imagemagick", "ffmpeg", "gstreamer"},
		"gmp":  {"coreutils", "coreutils"}, // duplicate
	}

	result := computeTopBlockers(blockers, 10)

	// glib should dedupe to 3 unique packages
	if result[0].Dependency != "glib" || result[0].Count != 3 {
		t.Errorf("glib: got count %d, want 3", result[0].Count)
	}
	// gmp should dedupe to 1 unique package
	if result[1].Dependency != "gmp" || result[1].Count != 1 {
		t.Errorf("gmp: got count %d, want 1", result[1].Count)
	}
}

func TestComputeTopBlockers_limit(t *testing.T) {
	blockers := make(map[string][]string)
	for i := 0; i < 20; i++ {
		dep := string(rune('a' + i))
		blockers[dep] = []string{"pkg1", "pkg2"}
	}

	result := computeTopBlockers(blockers, 5)

	if len(result) != 5 {
		t.Errorf("limit: got %d blockers, want 5", len(result))
	}
}

func TestComputeTopBlockers_packagesTruncation(t *testing.T) {
	blockers := map[string][]string{
		"glib": {"pkg1", "pkg2", "pkg3", "pkg4", "pkg5", "pkg6", "pkg7"},
	}

	result := computeTopBlockers(blockers, 10)

	if len(result[0].Packages) != 5 {
		t.Errorf("packages should be truncated to 5, got %d", len(result[0].Packages))
	}
}

func TestComputeTopBlockers_sortsByCount(t *testing.T) {
	blockers := map[string][]string{
		"small":  {"pkg1"},
		"large":  {"pkg1", "pkg2", "pkg3", "pkg4", "pkg5"},
		"medium": {"pkg1", "pkg2", "pkg3"},
	}

	result := computeTopBlockers(blockers, 10)

	if result[0].Dependency != "large" || result[0].Count != 5 {
		t.Errorf("first should be large with count 5, got %s with %d", result[0].Dependency, result[0].Count)
	}
	if result[1].Dependency != "medium" || result[1].Count != 3 {
		t.Errorf("second should be medium with count 3, got %s with %d", result[1].Dependency, result[1].Count)
	}
	if result[2].Dependency != "small" || result[2].Count != 1 {
		t.Errorf("third should be small with count 1, got %s with %d", result[2].Dependency, result[2].Count)
	}
}

func TestLoadMetrics(t *testing.T) {
	path := filepath.Join("testdata", "batch-runs.jsonl")
	runs, records, err := loadMetrics(path)
	if err != nil {
		t.Fatalf("loadMetrics: %v", err)
	}

	if len(runs) != 3 {
		t.Fatalf("runs: got %d, want 3", len(runs))
	}

	// Check first run
	if runs[0].BatchID != "2026-01-30-homebrew" {
		t.Errorf("BatchID: got %q, want 2026-01-30-homebrew", runs[0].BatchID)
	}
	if runs[0].Total != 10 {
		t.Errorf("Total: got %d, want 10", runs[0].Total)
	}
	if runs[0].Merged != 6 {
		t.Errorf("Merged: got %d, want 6", runs[0].Merged)
	}

	// Check rate calculation
	expectedRate := 6.0 / 10.0
	if runs[0].Rate != expectedRate {
		t.Errorf("Rate: got %f, want %f", runs[0].Rate, expectedRate)
	}

	// Check ecosystems and duration are populated
	if runs[0].Ecosystems["homebrew"] != 10 {
		t.Errorf("Ecosystems[homebrew]: got %d, want 10", runs[0].Ecosystems["homebrew"])
	}
	if runs[0].Duration != 120 {
		t.Errorf("Duration: got %d, want 120", runs[0].Duration)
	}

	// Check that records are also returned
	if len(records) != 3 {
		t.Errorf("records: got %d, want 3", len(records))
	}
}

func TestLoadMetrics_missingFile(t *testing.T) {
	_, _, err := loadMetrics("/nonexistent/path.jsonl")
	if err == nil {
		t.Error("expected error for missing file, got nil")
	}
}

func TestLoadMetrics_malformedLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "malformed-metrics.jsonl")
	content := `{"batch_id": "valid1", "ecosystem": "homebrew", "total": 10, "merged": 8, "timestamp": "2026-01-01T00:00:00Z"}
not valid json
{"batch_id": "valid2", "ecosystem": "homebrew", "total": 5, "merged": 3, "timestamp": "2026-01-02T00:00:00Z"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	runs, _, err := loadMetrics(path)
	if err != nil {
		t.Fatalf("loadMetrics: %v", err)
	}

	// Should skip malformed lines
	if len(runs) != 2 {
		t.Errorf("runs: got %d, want 2", len(runs))
	}
}

func TestLoadMetrics_zeroTotal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "zero-total.jsonl")
	content := `{"batch_id": "empty-batch", "ecosystem": "homebrew", "total": 0, "merged": 0, "timestamp": "2026-01-01T00:00:00Z"}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	runs, _, err := loadMetrics(path)
	if err != nil {
		t.Fatalf("loadMetrics: %v", err)
	}

	// Rate should be 0 when total is 0 (avoid division by zero)
	if runs[0].Rate != 0 {
		t.Errorf("Rate should be 0 for zero total, got %f", runs[0].Rate)
	}
}

func TestGenerate_integration(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "dashboard.json")

	opts := Options{
		QueueFile:   filepath.Join("testdata", "priority-queue.json"),
		FailuresDir: "testdata",
		MetricsDir:  "testdata",
		OutputFile:  outputPath,
	}

	if err := Generate(opts); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	// Read and parse output
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var dash Dashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify queue status
	if dash.Queue.Total != 6 {
		t.Errorf("Queue.Total: got %d, want 6", dash.Queue.Total)
	}

	// Verify blockers
	if len(dash.Blockers) == 0 {
		t.Error("expected blockers, got none")
	}

	// Verify failures
	if len(dash.Failures) == 0 {
		t.Error("expected failures, got none")
	}

	// Verify runs (newest first, limited to 10)
	if len(dash.Runs) != 3 {
		t.Errorf("Runs: got %d, want 3", len(dash.Runs))
	}
	// Should be reversed (newest first)
	if dash.Runs[0].BatchID != "2026-02-01-homebrew" {
		t.Errorf("First run should be newest: got %s", dash.Runs[0].BatchID)
	}

	// Verify generated_at is set
	if dash.GeneratedAt == "" {
		t.Error("GeneratedAt should be set")
	}
}

func TestGenerate_missingFailuresDir(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "dashboard.json")

	opts := Options{
		QueueFile:   filepath.Join("testdata", "priority-queue.json"),
		FailuresDir: "/nonexistent",
		MetricsDir:  "testdata",
		OutputFile:  outputPath,
	}

	// Should not error, failures are non-fatal
	if err := Generate(opts); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var dash Dashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Queue should still be populated
	if dash.Queue.Total != 6 {
		t.Errorf("Queue.Total: got %d, want 6", dash.Queue.Total)
	}

	// Blockers should be empty (failures not loaded)
	if len(dash.Blockers) != 0 {
		t.Errorf("Blockers should be empty: %v", dash.Blockers)
	}
}

func TestGenerate_missingMetricsDir(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "dashboard.json")

	opts := Options{
		QueueFile:   filepath.Join("testdata", "priority-queue.json"),
		FailuresDir: "testdata",
		MetricsDir:  "/nonexistent",
		OutputFile:  outputPath,
	}

	// Should not error, metrics are non-fatal
	if err := Generate(opts); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var dash Dashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Runs should be omitted (nil/empty)
	if len(dash.Runs) != 0 {
		t.Errorf("Runs should be empty: %v", dash.Runs)
	}
}

func TestGenerate_missingQueueFile(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "dashboard.json")

	opts := Options{
		QueueFile:   "/nonexistent/queue.json",
		FailuresDir: "testdata",
		MetricsDir:  "testdata",
		OutputFile:  outputPath,
	}

	// batch.LoadUnifiedQueue returns empty queue for missing file, so this should succeed
	if err := Generate(opts); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var dash Dashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Queue should be empty
	if dash.Queue.Total != 0 {
		t.Errorf("Queue.Total: got %d, want 0", dash.Queue.Total)
	}
}

func TestLoadDisambiguationsFromDir(t *testing.T) {
	dir := t.TempDir()

	// Create test disambiguation file
	content := `{"schema_version":1,"ecosystem":"homebrew","environment":"linux-x86_64","updated_at":"2026-02-13T10:00:00Z","disambiguations":[{"tool":"bat","selected":"crates.io:sharkdp/bat","alternatives":["npm:bat-cli"],"selection_reason":"10x_popularity_gap","downloads_ratio":225.5,"high_risk":false},{"tool":"fd","selected":"crates.io:sharkdp/fd","alternatives":["npm:fd-cli"],"selection_reason":"priority_fallback","downloads_ratio":1.5,"high_risk":true}]}
`
	if err := os.WriteFile(filepath.Join(dir, "homebrew.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	status, err := loadDisambiguationsFromDir(dir)
	if err != nil {
		t.Fatalf("loadDisambiguationsFromDir: %v", err)
	}

	if status == nil {
		t.Fatal("status should not be nil")
	}

	if status.Total != 2 {
		t.Errorf("Total: got %d, want 2", status.Total)
	}

	if status.ByReason["10x_popularity_gap"] != 1 {
		t.Errorf("ByReason[10x_popularity_gap]: got %d, want 1", status.ByReason["10x_popularity_gap"])
	}

	if status.ByReason["priority_fallback"] != 1 {
		t.Errorf("ByReason[priority_fallback]: got %d, want 1", status.ByReason["priority_fallback"])
	}

	if status.HighRisk != 1 {
		t.Errorf("HighRisk: got %d, want 1", status.HighRisk)
	}

	if len(status.NeedReview) != 1 || status.NeedReview[0] != "fd" {
		t.Errorf("NeedReview: got %v, want [fd]", status.NeedReview)
	}
}

func TestLoadDisambiguationsFromDir_MultipleFiles(t *testing.T) {
	dir := t.TempDir()

	// Create two disambiguation files
	file1 := `{"schema_version":1,"ecosystem":"homebrew","disambiguations":[{"tool":"bat","selected":"crates.io:sharkdp/bat","selection_reason":"10x_popularity_gap","high_risk":false}]}
`
	file2 := `{"schema_version":1,"ecosystem":"npm","disambiguations":[{"tool":"exa","selected":"crates.io:ogham/exa","selection_reason":"single_match","high_risk":false}]}
`
	if err := os.WriteFile(filepath.Join(dir, "homebrew.jsonl"), []byte(file1), 0644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "npm.jsonl"), []byte(file2), 0644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	status, err := loadDisambiguationsFromDir(dir)
	if err != nil {
		t.Fatalf("loadDisambiguationsFromDir: %v", err)
	}

	if status.Total != 2 {
		t.Errorf("Total: got %d, want 2", status.Total)
	}

	if status.ByReason["10x_popularity_gap"] != 1 {
		t.Errorf("ByReason[10x_popularity_gap]: got %d, want 1", status.ByReason["10x_popularity_gap"])
	}

	if status.ByReason["single_match"] != 1 {
		t.Errorf("ByReason[single_match]: got %d, want 1", status.ByReason["single_match"])
	}
}

func TestLoadDisambiguationsFromDir_DeduplicatesTools(t *testing.T) {
	dir := t.TempDir()

	// Same tool in multiple files - should only be counted once
	file1 := `{"schema_version":1,"ecosystem":"homebrew","disambiguations":[{"tool":"bat","selected":"crates.io:sharkdp/bat","selection_reason":"10x_popularity_gap","high_risk":false}]}
`
	file2 := `{"schema_version":1,"ecosystem":"npm","disambiguations":[{"tool":"bat","selected":"crates.io:sharkdp/bat","selection_reason":"10x_popularity_gap","high_risk":false}]}
`
	if err := os.WriteFile(filepath.Join(dir, "homebrew.jsonl"), []byte(file1), 0644); err != nil {
		t.Fatalf("write file1: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "npm.jsonl"), []byte(file2), 0644); err != nil {
		t.Fatalf("write file2: %v", err)
	}

	status, err := loadDisambiguationsFromDir(dir)
	if err != nil {
		t.Fatalf("loadDisambiguationsFromDir: %v", err)
	}

	// Should deduplicate to 1 tool
	if status.Total != 1 {
		t.Errorf("Total should be 1 after deduplication, got %d", status.Total)
	}
}

func TestLoadDisambiguationsFromDir_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	status, err := loadDisambiguationsFromDir(dir)
	if err != nil {
		t.Fatalf("loadDisambiguationsFromDir: %v", err)
	}

	// Should return nil for empty directory
	if status != nil {
		t.Errorf("status should be nil for empty directory, got %v", status)
	}
}

func TestLoadDisambiguationsFromDir_MalformedLines(t *testing.T) {
	dir := t.TempDir()

	content := `not valid json
{"schema_version":1,"ecosystem":"homebrew","disambiguations":[{"tool":"bat","selected":"crates.io:sharkdp/bat","selection_reason":"10x_popularity_gap","high_risk":false}]}
also not valid
`
	if err := os.WriteFile(filepath.Join(dir, "mixed.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	status, err := loadDisambiguationsFromDir(dir)
	if err != nil {
		t.Fatalf("loadDisambiguationsFromDir: %v", err)
	}

	// Should skip malformed lines and process valid ones
	if status.Total != 1 {
		t.Errorf("Total: got %d, want 1", status.Total)
	}
}

func TestGenerate_WithDisambiguations(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "dashboard.json")
	disambDir := filepath.Join(dir, "disambiguations")
	if err := os.MkdirAll(disambDir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Create test disambiguation file
	content := `{"schema_version":1,"ecosystem":"homebrew","disambiguations":[{"tool":"bat","selected":"crates.io:sharkdp/bat","selection_reason":"10x_popularity_gap","high_risk":false},{"tool":"fd","selected":"crates.io:sharkdp/fd","selection_reason":"priority_fallback","high_risk":true}]}
`
	if err := os.WriteFile(filepath.Join(disambDir, "homebrew.jsonl"), []byte(content), 0644); err != nil {
		t.Fatalf("write test file: %v", err)
	}

	opts := Options{
		QueueFile:          filepath.Join("testdata", "priority-queue.json"),
		FailuresDir:        "testdata",
		MetricsDir:         "testdata",
		DisambiguationsDir: disambDir,
		OutputFile:         outputPath,
	}

	if err := Generate(opts); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var dash Dashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Verify disambiguations are present
	if dash.Disambiguations == nil {
		t.Fatal("Disambiguations should be populated")
	}

	if dash.Disambiguations.Total != 2 {
		t.Errorf("Disambiguations.Total: got %d, want 2", dash.Disambiguations.Total)
	}

	if dash.Disambiguations.HighRisk != 1 {
		t.Errorf("Disambiguations.HighRisk: got %d, want 1", dash.Disambiguations.HighRisk)
	}

	if len(dash.Disambiguations.NeedReview) != 1 || dash.Disambiguations.NeedReview[0] != "fd" {
		t.Errorf("Disambiguations.NeedReview: got %v, want [fd]", dash.Disambiguations.NeedReview)
	}
}

func TestComputeQueueStatus_unifiedQueueFields(t *testing.T) {
	// Verify that unified queue fields (Ecosystem, Priority, Confidence, FailureCount)
	// are correctly mapped to PackageInfo.
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "ripgrep", Source: "cargo:ripgrep", Priority: 1, Status: "success", Confidence: "curated", FailureCount: 0},
			{Name: "bat", Source: "homebrew:bat", Priority: 2, Status: "failed", Confidence: "auto", FailureCount: 3},
			{Name: "fd", Source: "github:sharkdp/fd", Priority: 1, Status: "pending", Confidence: "curated", FailureCount: 0},
		},
	}

	status := computeQueueStatus(queue, nil)

	if status.Total != 3 {
		t.Errorf("Total: got %d, want 3", status.Total)
	}

	// Check that Ecosystem() is correctly extracted from Source
	successPkgs := status.Packages["success"]
	if len(successPkgs) != 1 {
		t.Fatalf("success packages: got %d, want 1", len(successPkgs))
	}
	if successPkgs[0].Ecosystem != "cargo" {
		t.Errorf("Ecosystem: got %q, want %q", successPkgs[0].Ecosystem, "cargo")
	}
	if successPkgs[0].Priority != 1 {
		t.Errorf("Priority: got %d, want 1", successPkgs[0].Priority)
	}

	// Check the failed entry uses homebrew ecosystem
	failedPkgs := status.Packages["failed"]
	if len(failedPkgs) != 1 {
		t.Fatalf("failed packages: got %d, want 1", len(failedPkgs))
	}
	if failedPkgs[0].Ecosystem != "homebrew" {
		t.Errorf("Ecosystem: got %q, want %q", failedPkgs[0].Ecosystem, "homebrew")
	}

	// Check github ecosystem
	pendingPkgs := status.Packages["pending"]
	if len(pendingPkgs) != 1 {
		t.Fatalf("pending packages: got %d, want 1", len(pendingPkgs))
	}
	if pendingPkgs[0].Ecosystem != "github" {
		t.Errorf("Ecosystem: got %q, want %q", pendingPkgs[0].Ecosystem, "github")
	}
}

func TestComputeQueueStatus_idUsesSource(t *testing.T) {
	// Verify that PackageInfo.ID uses the full Source field (ecosystem:identifier).
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "ripgrep", Source: "cargo:ripgrep", Priority: 1, Status: "success", Confidence: "curated"},
		},
	}

	status := computeQueueStatus(queue, nil)

	pkgs := status.Packages["success"]
	if len(pkgs) != 1 {
		t.Fatalf("packages: got %d, want 1", len(pkgs))
	}
	if pkgs[0].ID != "cargo:ripgrep" {
		t.Errorf("ID: got %q, want %q", pkgs[0].ID, "cargo:ripgrep")
	}
}

func TestLoadQueue_unifiedFormat(t *testing.T) {
	// Verify that loadQueue correctly reads the unified queue testdata.
	queue, err := loadQueue(filepath.Join("testdata", "priority-queue.json"))
	if err != nil {
		t.Fatalf("loadQueue: %v", err)
	}

	if len(queue.Entries) != 6 {
		t.Fatalf("entries: got %d, want 6", len(queue.Entries))
	}

	// Verify unified queue fields are populated
	bat := queue.Entries[2] // bat has failure_count: 2 in testdata
	if bat.Name != "bat" {
		t.Errorf("Name: got %q, want %q", bat.Name, "bat")
	}
	if bat.FailureCount != 2 {
		t.Errorf("FailureCount: got %d, want 2", bat.FailureCount)
	}
	if bat.Confidence != "auto" {
		t.Errorf("Confidence: got %q, want %q", bat.Confidence, "auto")
	}
	if bat.Ecosystem() != "homebrew" {
		t.Errorf("Ecosystem: got %q, want %q", bat.Ecosystem(), "homebrew")
	}

	// Verify a curated entry
	jq := queue.Entries[0]
	if jq.Confidence != "curated" {
		t.Errorf("Confidence: got %q, want %q", jq.Confidence, "curated")
	}
}

func TestLoadQueue_missingFile(t *testing.T) {
	// Verify that missing file returns an empty queue (not an error).
	queue, err := loadQueue("/nonexistent/priority-queue.json")
	if err != nil {
		t.Fatalf("loadQueue: %v", err)
	}
	if len(queue.Entries) != 0 {
		t.Errorf("entries: got %d, want 0", len(queue.Entries))
	}
}

func TestLoadQueue_legacyFormatReturnsError(t *testing.T) {
	// Legacy seed format files (with "tier" instead of "priority", no "source")
	// should produce a clear validation error, not silently degrade.
	dir := t.TempDir()
	path := filepath.Join(dir, "priority-queue.json")
	content := `{"packages":[{"id":"homebrew:jq","name":"jq","source":"homebrew","tier":1,"status":"pending","added_at":"2026-01-01T00:00:00Z"}]}`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	_, err := loadQueue(path)
	if err == nil {
		t.Fatal("expected error for legacy format file, got nil")
	}
	// Error should mention format mismatch
	if !strings.Contains(err.Error(), "no entries parsed") {
		t.Errorf("error should mention format mismatch: %v", err)
	}
}

func TestGenerate_MissingDisambiguationsDir(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "dashboard.json")

	opts := Options{
		QueueFile:          filepath.Join("testdata", "priority-queue.json"),
		FailuresDir:        "testdata",
		MetricsDir:         "testdata",
		DisambiguationsDir: "/nonexistent",
		OutputFile:         outputPath,
	}

	// Should not error, disambiguations are non-fatal
	if err := Generate(opts); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var dash Dashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Disambiguations should be omitted (nil)
	if dash.Disambiguations != nil {
		t.Errorf("Disambiguations should be nil: %v", dash.Disambiguations)
	}
}

func TestLoadHealth_withControlFileAndRecords(t *testing.T) {
	controlPath := filepath.Join("testdata", "batch-control.json")
	records := []MetricsRecord{
		{BatchID: "2026-02-01-homebrew", Ecosystem: "homebrew", Total: 10, Merged: 6, Timestamp: "2026-02-01T12:00:00Z"},
		{BatchID: "2026-02-02-homebrew", Ecosystem: "homebrew", Total: 8, Merged: 0, Timestamp: "2026-02-02T12:00:00Z"},
		{BatchID: "2026-02-03-homebrew", Ecosystem: "homebrew", Total: 12, Merged: 5, Timestamp: "2026-02-03T12:00:00Z"},
	}

	health, err := loadHealth(controlPath, records)
	if err != nil {
		t.Fatalf("loadHealth: %v", err)
	}
	if health == nil {
		t.Fatal("health should not be nil")
	}

	// Check circuit breaker state was loaded
	if len(health.Ecosystems) != 2 {
		t.Errorf("Ecosystems: got %d, want 2", len(health.Ecosystems))
	}
	hw := health.Ecosystems["homebrew"]
	if hw.BreakerState != "closed" {
		t.Errorf("homebrew BreakerState: got %q, want %q", hw.BreakerState, "closed")
	}
	if hw.Failures != 0 {
		t.Errorf("homebrew Failures: got %d, want 0", hw.Failures)
	}

	npm := health.Ecosystems["npm"]
	if npm.BreakerState != "open" {
		t.Errorf("npm BreakerState: got %q, want %q", npm.BreakerState, "open")
	}
	if npm.Failures != 3 {
		t.Errorf("npm Failures: got %d, want 3", npm.Failures)
	}

	// Check last run
	if health.LastRun == nil {
		t.Fatal("LastRun should not be nil")
	}
	if health.LastRun.BatchID != "2026-02-03-homebrew" {
		t.Errorf("LastRun.BatchID: got %q, want %q", health.LastRun.BatchID, "2026-02-03-homebrew")
	}
	if health.LastRun.Total != 12 {
		t.Errorf("LastRun.Total: got %d, want 12", health.LastRun.Total)
	}

	// Check last successful run (the most recent with Merged > 0)
	if health.LastSuccessfulRun == nil {
		t.Fatal("LastSuccessfulRun should not be nil")
	}
	if health.LastSuccessfulRun.BatchID != "2026-02-03-homebrew" {
		t.Errorf("LastSuccessfulRun.BatchID: got %q, want %q", health.LastSuccessfulRun.BatchID, "2026-02-03-homebrew")
	}

	// runs_since_last_success should be 0 since the last run is the successful one
	if health.RunsSinceLastSuccess != 0 {
		t.Errorf("RunsSinceLastSuccess: got %d, want 0", health.RunsSinceLastSuccess)
	}
}

func TestLoadHealth_runsSinceLastSuccess(t *testing.T) {
	// Create a temporary control file
	dir := t.TempDir()
	controlPath := filepath.Join(dir, "batch-control.json")
	if err := os.WriteFile(controlPath, []byte(`{"circuit_breaker":{}}`), 0644); err != nil {
		t.Fatalf("write control file: %v", err)
	}

	records := []MetricsRecord{
		{BatchID: "run-1", Ecosystem: "homebrew", Total: 10, Merged: 5, Timestamp: "2026-02-01T12:00:00Z"},
		{BatchID: "run-2", Ecosystem: "homebrew", Total: 10, Merged: 0, Timestamp: "2026-02-02T12:00:00Z"},
		{BatchID: "run-3", Ecosystem: "homebrew", Total: 10, Merged: 0, Timestamp: "2026-02-03T12:00:00Z"},
		{BatchID: "run-4", Ecosystem: "homebrew", Total: 10, Merged: 0, Timestamp: "2026-02-04T12:00:00Z"},
	}

	health, err := loadHealth(controlPath, records)
	if err != nil {
		t.Fatalf("loadHealth: %v", err)
	}

	// Last successful run should be run-1
	if health.LastSuccessfulRun == nil {
		t.Fatal("LastSuccessfulRun should not be nil")
	}
	if health.LastSuccessfulRun.BatchID != "run-1" {
		t.Errorf("LastSuccessfulRun.BatchID: got %q, want %q", health.LastSuccessfulRun.BatchID, "run-1")
	}

	// 3 runs since last success (run-2, run-3, run-4)
	if health.RunsSinceLastSuccess != 3 {
		t.Errorf("RunsSinceLastSuccess: got %d, want 3", health.RunsSinceLastSuccess)
	}

	// Last run should be run-4
	if health.LastRun.BatchID != "run-4" {
		t.Errorf("LastRun.BatchID: got %q, want %q", health.LastRun.BatchID, "run-4")
	}
}

func TestLoadHealth_noSuccessfulRuns(t *testing.T) {
	dir := t.TempDir()
	controlPath := filepath.Join(dir, "batch-control.json")
	if err := os.WriteFile(controlPath, []byte(`{"circuit_breaker":{}}`), 0644); err != nil {
		t.Fatalf("write control file: %v", err)
	}

	records := []MetricsRecord{
		{BatchID: "run-1", Ecosystem: "homebrew", Total: 10, Merged: 0, Timestamp: "2026-02-01T12:00:00Z"},
		{BatchID: "run-2", Ecosystem: "homebrew", Total: 10, Merged: 0, Timestamp: "2026-02-02T12:00:00Z"},
	}

	health, err := loadHealth(controlPath, records)
	if err != nil {
		t.Fatalf("loadHealth: %v", err)
	}

	if health.LastSuccessfulRun != nil {
		t.Errorf("LastSuccessfulRun should be nil when no runs have merges, got %v", health.LastSuccessfulRun)
	}

	// All runs count toward runs_since_last_success
	if health.RunsSinceLastSuccess != 2 {
		t.Errorf("RunsSinceLastSuccess: got %d, want 2", health.RunsSinceLastSuccess)
	}
}

func TestLoadHealth_missingControlFile(t *testing.T) {
	records := []MetricsRecord{
		{BatchID: "run-1", Ecosystem: "homebrew", Total: 10, Merged: 5, Timestamp: "2026-02-01T12:00:00Z"},
	}

	health, err := loadHealth("/nonexistent/batch-control.json", records)
	if err != nil {
		t.Fatalf("loadHealth: %v", err)
	}

	// Health should still be returned (from metrics records)
	if health == nil {
		t.Fatal("health should not be nil when records exist")
	}

	// Ecosystems map should be empty (no control file)
	if len(health.Ecosystems) != 0 {
		t.Errorf("Ecosystems should be empty: %v", health.Ecosystems)
	}

	// Run tracking should still work
	if health.LastRun == nil {
		t.Fatal("LastRun should not be nil")
	}
	if health.LastRun.BatchID != "run-1" {
		t.Errorf("LastRun.BatchID: got %q, want %q", health.LastRun.BatchID, "run-1")
	}
}

func TestLoadHealth_noControlFileNoRecords(t *testing.T) {
	health, err := loadHealth("/nonexistent/batch-control.json", nil)
	if err != nil {
		t.Fatalf("loadHealth: %v", err)
	}

	// Should return nil when there's nothing to report
	if health != nil {
		t.Errorf("health should be nil when no control file and no records, got %v", health)
	}
}

func TestLoadHealth_controlFileOnly(t *testing.T) {
	controlPath := filepath.Join("testdata", "batch-control.json")

	health, err := loadHealth(controlPath, nil)
	if err != nil {
		t.Fatalf("loadHealth: %v", err)
	}

	if health == nil {
		t.Fatal("health should not be nil with control file")
	}

	// Should have ecosystem data from control file
	if len(health.Ecosystems) != 2 {
		t.Errorf("Ecosystems: got %d, want 2", len(health.Ecosystems))
	}

	// Run tracking should be empty
	if health.LastRun != nil {
		t.Errorf("LastRun should be nil without records, got %v", health.LastRun)
	}
	if health.LastSuccessfulRun != nil {
		t.Errorf("LastSuccessfulRun should be nil without records, got %v", health.LastSuccessfulRun)
	}
}

func TestLoadHealth_runInfoFields(t *testing.T) {
	records := []MetricsRecord{
		{BatchID: "run-1", Ecosystem: "homebrew", Total: 10, Merged: 8, Timestamp: "2026-02-01T12:00:00Z"},
	}

	health, err := loadHealth("/nonexistent/batch-control.json", records)
	if err != nil {
		t.Fatalf("loadHealth: %v", err)
	}

	ri := health.LastRun
	if ri == nil {
		t.Fatal("LastRun should not be nil")
	}

	// Verify RunInfo field mapping
	if ri.BatchID != "run-1" {
		t.Errorf("BatchID: got %q, want %q", ri.BatchID, "run-1")
	}
	if ri.Ecosystems["homebrew"] != 10 {
		t.Errorf("Ecosystems[homebrew]: got %d, want 10", ri.Ecosystems["homebrew"])
	}
	if ri.Timestamp != "2026-02-01T12:00:00Z" {
		t.Errorf("Timestamp: got %q, want %q", ri.Timestamp, "2026-02-01T12:00:00Z")
	}
	if ri.Succeeded != 8 {
		t.Errorf("Succeeded: got %d, want 8", ri.Succeeded)
	}
	if ri.Failed != 2 {
		t.Errorf("Failed: got %d, want 2 (total - merged)", ri.Failed)
	}
	if ri.Total != 10 {
		t.Errorf("Total: got %d, want 10", ri.Total)
	}
	if ri.RecipesMerged != 8 {
		t.Errorf("RecipesMerged: got %d, want 8", ri.RecipesMerged)
	}
}

func TestLoadMetrics_ecosystemAndDuration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "batch-runs.jsonl")
	content := `{"batch_id":"run-1","ecosystem":"npm","total":5,"merged":3,"timestamp":"2026-01-01T00:00:00Z","duration_seconds":90}
{"batch_id":"run-2","ecosystem":"cargo","total":8,"merged":6,"timestamp":"2026-01-02T00:00:00Z","duration_seconds":200}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	runs, records, err := loadMetrics(path)
	if err != nil {
		t.Fatalf("loadMetrics: %v", err)
	}

	if len(runs) != 2 {
		t.Fatalf("runs: got %d, want 2", len(runs))
	}

	// Check ecosystems propagation (old format: single ecosystem synthesized into map)
	if runs[0].Ecosystems["npm"] != 5 {
		t.Errorf("runs[0].Ecosystems[npm]: got %d, want 5", runs[0].Ecosystems["npm"])
	}
	if runs[1].Ecosystems["cargo"] != 8 {
		t.Errorf("runs[1].Ecosystems[cargo]: got %d, want 8", runs[1].Ecosystems["cargo"])
	}

	// Check duration propagation
	if runs[0].Duration != 90 {
		t.Errorf("runs[0].Duration: got %d, want 90", runs[0].Duration)
	}
	if runs[1].Duration != 200 {
		t.Errorf("runs[1].Duration: got %d, want 200", runs[1].Duration)
	}

	// Check records are returned
	if len(records) != 2 {
		t.Errorf("records: got %d, want 2", len(records))
	}
	if records[0].DurationSeconds != 90 {
		t.Errorf("records[0].DurationSeconds: got %d, want 90", records[0].DurationSeconds)
	}
}

func TestLoadMetrics_newEcosystemsFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "batch-runs.jsonl")
	content := `{"batch_id":"2026-02-17","ecosystems":{"homebrew":3,"cargo":5,"github":2},"total":10,"merged":8,"timestamp":"2026-02-17T12:00:00Z","duration_seconds":120}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	runs, records, err := loadMetrics(path)
	if err != nil {
		t.Fatalf("loadMetrics: %v", err)
	}

	if len(runs) != 1 {
		t.Fatalf("runs: got %d, want 1", len(runs))
	}

	// New format: ecosystems map should be passed through directly
	if runs[0].Ecosystems["homebrew"] != 3 {
		t.Errorf("Ecosystems[homebrew]: got %d, want 3", runs[0].Ecosystems["homebrew"])
	}
	if runs[0].Ecosystems["cargo"] != 5 {
		t.Errorf("Ecosystems[cargo]: got %d, want 5", runs[0].Ecosystems["cargo"])
	}
	if runs[0].Ecosystems["github"] != 2 {
		t.Errorf("Ecosystems[github]: got %d, want 2", runs[0].Ecosystems["github"])
	}
	if len(runs[0].Ecosystems) != 3 {
		t.Errorf("Ecosystems count: got %d, want 3", len(runs[0].Ecosystems))
	}

	// Records should also carry the ecosystems map
	if records[0].Ecosystems["cargo"] != 5 {
		t.Errorf("records[0].Ecosystems[cargo]: got %d, want 5", records[0].Ecosystems["cargo"])
	}
}

func TestLoadMetrics_mixedOldAndNewFormat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "batch-runs.jsonl")
	content := `{"batch_id":"old-run","ecosystem":"homebrew","total":10,"merged":6,"timestamp":"2026-02-01T12:00:00Z","duration_seconds":100}
{"batch_id":"new-run","ecosystems":{"homebrew":3,"cargo":5},"total":8,"merged":7,"timestamp":"2026-02-17T12:00:00Z","duration_seconds":90}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	runs, _, err := loadMetrics(path)
	if err != nil {
		t.Fatalf("loadMetrics: %v", err)
	}

	if len(runs) != 2 {
		t.Fatalf("runs: got %d, want 2", len(runs))
	}

	// Old format: single ecosystem synthesized to {ecosystem: total}
	if runs[0].Ecosystems["homebrew"] != 10 {
		t.Errorf("old run Ecosystems[homebrew]: got %d, want 10", runs[0].Ecosystems["homebrew"])
	}
	if len(runs[0].Ecosystems) != 1 {
		t.Errorf("old run Ecosystems count: got %d, want 1", len(runs[0].Ecosystems))
	}

	// New format: ecosystems map passed through directly
	if runs[1].Ecosystems["homebrew"] != 3 {
		t.Errorf("new run Ecosystems[homebrew]: got %d, want 3", runs[1].Ecosystems["homebrew"])
	}
	if runs[1].Ecosystems["cargo"] != 5 {
		t.Errorf("new run Ecosystems[cargo]: got %d, want 5", runs[1].Ecosystems["cargo"])
	}
}

func TestResolveEcosystems(t *testing.T) {
	// New format takes precedence
	rec := MetricsRecord{
		Ecosystem:  "homebrew",
		Ecosystems: map[string]int{"homebrew": 3, "cargo": 5},
		Total:      8,
	}
	eco := resolveEcosystems(rec)
	if eco["homebrew"] != 3 || eco["cargo"] != 5 {
		t.Errorf("new format: got %v, want {homebrew:3, cargo:5}", eco)
	}

	// Old format synthesizes {ecosystem: total}
	rec2 := MetricsRecord{
		Ecosystem: "npm",
		Total:     12,
	}
	eco2 := resolveEcosystems(rec2)
	if eco2["npm"] != 12 {
		t.Errorf("old format: got %v, want {npm:12}", eco2)
	}

	// Neither set returns nil
	rec3 := MetricsRecord{Total: 5}
	eco3 := resolveEcosystems(rec3)
	if eco3 != nil {
		t.Errorf("empty: got %v, want nil", eco3)
	}
}

func TestLoadFailures_perRecipeWithEcosystem(t *testing.T) {
	// Verify that record.Ecosystem is used for pkgID prefix instead of hardcoded "homebrew"
	dir := t.TempDir()
	path := filepath.Join(dir, "failures.jsonl")
	content := `{"schema_version":1,"ecosystem":"cargo","recipe":"ripgrep","platform":"linux-x86_64","exit_code":8,"category":"missing_dep","blocked_by":["pcre2"]}
`
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	_, _, details, err := loadFailures(path)
	if err != nil {
		t.Fatalf("loadFailures: %v", err)
	}

	// Should use "cargo:" prefix, not "homebrew:"
	if _, ok := details["cargo:ripgrep"]; !ok {
		t.Errorf("expected details[cargo:ripgrep], got keys: %v", details)
	}
	if _, ok := details["homebrew:ripgrep"]; ok {
		t.Error("should not have homebrew:ripgrep key")
	}
}

func TestGenerate_withHealth(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "dashboard.json")

	opts := Options{
		QueueFile:          filepath.Join("testdata", "priority-queue.json"),
		FailuresDir:        "testdata",
		MetricsDir:         "testdata",
		DisambiguationsDir: "/nonexistent",
		ControlFile:        filepath.Join("testdata", "batch-control.json"),
		OutputFile:         outputPath,
	}

	if err := Generate(opts); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var dash Dashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Health should be populated
	if dash.Health == nil {
		t.Fatal("Health should be populated when control file and metrics exist")
	}

	// Check circuit breaker from control file
	if len(dash.Health.Ecosystems) != 2 {
		t.Errorf("Ecosystems: got %d, want 2", len(dash.Health.Ecosystems))
	}

	// Check run tracking from metrics
	if dash.Health.LastRun == nil {
		t.Fatal("LastRun should not be nil")
	}
	if dash.Health.LastRun.BatchID != "2026-02-01-homebrew" {
		t.Errorf("LastRun.BatchID: got %q, want %q", dash.Health.LastRun.BatchID, "2026-02-01-homebrew")
	}

	// All three runs in testdata have merges, so last successful = last run
	if dash.Health.LastSuccessfulRun == nil {
		t.Fatal("LastSuccessfulRun should not be nil")
	}

	// Verify runs also have ecosystems and duration
	if len(dash.Runs) == 0 {
		t.Fatal("Runs should not be empty")
	}
	if dash.Runs[0].Ecosystems["homebrew"] != 15 {
		t.Errorf("Runs[0].Ecosystems[homebrew]: got %d, want 15", dash.Runs[0].Ecosystems["homebrew"])
	}
	if dash.Runs[0].Duration != 180 { // newest first (2026-02-01-homebrew has 180s)
		t.Errorf("Runs[0].Duration: got %d, want 180", dash.Runs[0].Duration)
	}
}

func TestGenerate_missingControlFile(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "dashboard.json")

	opts := Options{
		QueueFile:          filepath.Join("testdata", "priority-queue.json"),
		FailuresDir:        "testdata",
		MetricsDir:         "testdata",
		DisambiguationsDir: "/nonexistent",
		ControlFile:        "/nonexistent/batch-control.json",
		OutputFile:         outputPath,
	}

	// Should not error, missing control file is non-fatal
	if err := Generate(opts); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var dash Dashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// Health should still be populated from metrics records alone
	if dash.Health == nil {
		t.Fatal("Health should not be nil when metrics exist")
	}
	if len(dash.Health.Ecosystems) != 0 {
		t.Errorf("Ecosystems should be empty without control file: %v", dash.Health.Ecosystems)
	}
	if dash.Health.LastRun == nil {
		t.Fatal("LastRun should be populated from metrics")
	}
}

func TestLoadHealth_malformedControlFile(t *testing.T) {
	dir := t.TempDir()
	controlPath := filepath.Join(dir, "batch-control.json")
	if err := os.WriteFile(controlPath, []byte("not valid json"), 0644); err != nil {
		t.Fatalf("write control file: %v", err)
	}

	_, err := loadHealth(controlPath, nil)
	if err == nil {
		t.Error("expected error for malformed control file, got nil")
	}
	if !strings.Contains(err.Error(), "parse control file") {
		t.Errorf("error should mention parse control file: %v", err)
	}
}

func TestLoadCurated_filtersCuratedEntries(t *testing.T) {
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "jq", Source: "homebrew:jq", Priority: 1, Status: "success", Confidence: "curated"},
			{Name: "fd", Source: "homebrew:fd", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "ripgrep", Source: "cargo:ripgrep", Priority: 2, Status: "blocked", Confidence: "curated"},
			{Name: "bat", Source: "homebrew:bat", Priority: 1, Status: "failed", Confidence: "curated"},
			{Name: "exa", Source: "homebrew:exa", Priority: 2, Status: "success", Confidence: "auto"},
		},
	}

	curated := loadCurated(queue)

	if len(curated) != 3 {
		t.Fatalf("curated: got %d, want 3", len(curated))
	}

	// Verify jq (success -> valid)
	if curated[0].Name != "jq" {
		t.Errorf("curated[0].Name: got %q, want %q", curated[0].Name, "jq")
	}
	if curated[0].Source != "homebrew:jq" {
		t.Errorf("curated[0].Source: got %q, want %q", curated[0].Source, "homebrew:jq")
	}
	if curated[0].ValidationStatus != "valid" {
		t.Errorf("curated[0].ValidationStatus: got %q, want %q", curated[0].ValidationStatus, "valid")
	}

	// Verify ripgrep (blocked -> unknown)
	if curated[1].Name != "ripgrep" {
		t.Errorf("curated[1].Name: got %q, want %q", curated[1].Name, "ripgrep")
	}
	if curated[1].ValidationStatus != "unknown" {
		t.Errorf("curated[1].ValidationStatus: got %q, want %q", curated[1].ValidationStatus, "unknown")
	}

	// Verify bat (failed -> invalid)
	if curated[2].Name != "bat" {
		t.Errorf("curated[2].Name: got %q, want %q", curated[2].Name, "bat")
	}
	if curated[2].ValidationStatus != "invalid" {
		t.Errorf("curated[2].ValidationStatus: got %q, want %q", curated[2].ValidationStatus, "invalid")
	}
}

func TestLoadCurated_noCuratedEntries(t *testing.T) {
	queue := &batch.UnifiedQueue{
		Entries: []batch.QueueEntry{
			{Name: "fd", Source: "homebrew:fd", Priority: 1, Status: "pending", Confidence: "auto"},
			{Name: "exa", Source: "homebrew:exa", Priority: 2, Status: "success", Confidence: "auto"},
		},
	}

	curated := loadCurated(queue)

	if len(curated) != 0 {
		t.Errorf("curated should be empty when no curated entries: got %d", len(curated))
	}
}

func TestLoadCurated_emptyQueue(t *testing.T) {
	queue := &batch.UnifiedQueue{Entries: []batch.QueueEntry{}}

	curated := loadCurated(queue)

	if len(curated) != 0 {
		t.Errorf("curated should be empty for empty queue: got %d", len(curated))
	}
}

func TestLoadCurated_statusMapping(t *testing.T) {
	// Test all status values to verify the mapping is complete.
	tests := []struct {
		status string
		want   string
	}{
		{"success", "valid"},
		{"pending", "valid"},
		{"failed", "invalid"},
		{"blocked", "unknown"},
		{"requires_manual", "unknown"},
		{"excluded", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.status, func(t *testing.T) {
			queue := &batch.UnifiedQueue{
				Entries: []batch.QueueEntry{
					{Name: "tool", Source: "homebrew:tool", Priority: 1, Status: tt.status, Confidence: "curated"},
				},
			}

			curated := loadCurated(queue)

			if len(curated) != 1 {
				t.Fatalf("curated: got %d, want 1", len(curated))
			}
			if curated[0].ValidationStatus != tt.want {
				t.Errorf("ValidationStatus for status %q: got %q, want %q", tt.status, curated[0].ValidationStatus, tt.want)
			}
		})
	}
}

func TestGenerate_withCurated(t *testing.T) {
	dir := t.TempDir()
	outputPath := filepath.Join(dir, "dashboard.json")

	opts := Options{
		QueueFile:          filepath.Join("testdata", "priority-queue.json"),
		FailuresDir:        "testdata",
		MetricsDir:         "testdata",
		DisambiguationsDir: "/nonexistent",
		ControlFile:        "/nonexistent/batch-control.json",
		OutputFile:         outputPath,
	}

	if err := Generate(opts); err != nil {
		t.Fatalf("Generate: %v", err)
	}

	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}

	var dash Dashboard
	if err := json.Unmarshal(data, &dash); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	// The testdata queue has 2 curated entries: jq (success) and ripgrep (blocked)
	if len(dash.Curated) != 2 {
		t.Fatalf("Curated: got %d, want 2", len(dash.Curated))
	}

	// Verify jq
	if dash.Curated[0].Name != "jq" {
		t.Errorf("Curated[0].Name: got %q, want %q", dash.Curated[0].Name, "jq")
	}
	if dash.Curated[0].ValidationStatus != "valid" {
		t.Errorf("Curated[0].ValidationStatus: got %q, want %q", dash.Curated[0].ValidationStatus, "valid")
	}

	// Verify ripgrep
	if dash.Curated[1].Name != "ripgrep" {
		t.Errorf("Curated[1].Name: got %q, want %q", dash.Curated[1].Name, "ripgrep")
	}
	if dash.Curated[1].ValidationStatus != "unknown" {
		t.Errorf("Curated[1].ValidationStatus: got %q, want %q", dash.Curated[1].ValidationStatus, "unknown")
	}
}
