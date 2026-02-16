package dashboard

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	runs, err := loadMetrics(path)
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
}

func TestLoadMetrics_missingFile(t *testing.T) {
	_, err := loadMetrics("/nonexistent/path.jsonl")
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

	runs, err := loadMetrics(path)
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

	runs, err := loadMetrics(path)
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
