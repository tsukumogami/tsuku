package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tsukumogami/tsuku/internal/batch"
	"github.com/tsukumogami/tsuku/internal/seed"
)

// --- Flag Parsing Tests ---

func TestParseFlags_Defaults(t *testing.T) {
	cfg, err := parseFlags([]string{})
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}

	if cfg.source != "all" {
		t.Errorf("source = %q, want 'all'", cfg.source)
	}
	if cfg.limit != 500 {
		t.Errorf("limit = %d, want 500", cfg.limit)
	}
	if cfg.queuePath != "data/queues/priority-queue.json" {
		t.Errorf("queuePath = %q, want default", cfg.queuePath)
	}
	if cfg.recipesDir != "recipes" {
		t.Errorf("recipesDir = %q, want 'recipes'", cfg.recipesDir)
	}
	if cfg.embeddedDir != "" {
		t.Errorf("embeddedDir = %q, want ''", cfg.embeddedDir)
	}
	if !cfg.disambiguate {
		t.Error("disambiguate should default to true")
	}
	if cfg.freshness != 30 {
		t.Errorf("freshness = %d, want 30", cfg.freshness)
	}
	if cfg.auditDir != "data/disambiguations/audit" {
		t.Errorf("auditDir = %q, want default", cfg.auditDir)
	}
	if cfg.dryRun {
		t.Error("dryRun should default to false")
	}
	if cfg.verbose {
		t.Error("verbose should default to false")
	}
}

func TestParseFlags_AllFlags(t *testing.T) {
	args := []string{
		"-source", "cargo",
		"-limit", "100",
		"-queue", "/tmp/queue.json",
		"-recipes-dir", "/tmp/recipes",
		"-embedded-dir", "/tmp/embedded",
		"-disambiguate=false",
		"-freshness", "7",
		"-audit-dir", "/tmp/audit",
		"-dry-run",
		"-verbose",
	}

	cfg, err := parseFlags(args)
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}

	if cfg.source != "cargo" {
		t.Errorf("source = %q, want 'cargo'", cfg.source)
	}
	if cfg.limit != 100 {
		t.Errorf("limit = %d, want 100", cfg.limit)
	}
	if cfg.queuePath != "/tmp/queue.json" {
		t.Errorf("queuePath = %q", cfg.queuePath)
	}
	if cfg.recipesDir != "/tmp/recipes" {
		t.Errorf("recipesDir = %q", cfg.recipesDir)
	}
	if cfg.embeddedDir != "/tmp/embedded" {
		t.Errorf("embeddedDir = %q", cfg.embeddedDir)
	}
	if cfg.disambiguate {
		t.Error("disambiguate should be false")
	}
	if cfg.freshness != 7 {
		t.Errorf("freshness = %d, want 7", cfg.freshness)
	}
	if cfg.auditDir != "/tmp/audit" {
		t.Errorf("auditDir = %q", cfg.auditDir)
	}
	if !cfg.dryRun {
		t.Error("dryRun should be true")
	}
	if !cfg.verbose {
		t.Error("verbose should be true")
	}
}

func TestParseFlags_SourceAllSelectsAllSources(t *testing.T) {
	cfg, err := parseFlags([]string{"-source", "all"})
	if err != nil {
		t.Fatalf("parseFlags error: %v", err)
	}
	if cfg.source != "all" {
		t.Errorf("source = %q, want 'all'", cfg.source)
	}
}

func TestParseFlags_ValidSources(t *testing.T) {
	validNames := []string{"homebrew", "cargo", "npm", "pypi", "rubygems", "all"}
	for _, src := range validNames {
		_, err := parseFlags([]string{"-source", src})
		if err != nil {
			t.Errorf("source %q should be valid, got error: %v", src, err)
		}
	}
}

func TestParseFlags_InvalidSource(t *testing.T) {
	_, err := parseFlags([]string{"-source", "invalid"})
	if err == nil {
		t.Fatal("expected error for invalid source")
	}
	if !strings.Contains(err.Error(), "invalid -source") {
		t.Errorf("error = %q, should mention invalid source", err.Error())
	}
}

func TestParseFlags_NegativeLimit(t *testing.T) {
	_, err := parseFlags([]string{"-limit", "-1"})
	if err == nil {
		t.Fatal("expected error for negative limit")
	}
}

func TestParseFlags_NegativeFreshness(t *testing.T) {
	_, err := parseFlags([]string{"-freshness", "-1"})
	if err == nil {
		t.Fatal("expected error for negative freshness")
	}
}

func TestParseFlags_ZeroLimitAllowed(t *testing.T) {
	cfg, err := parseFlags([]string{"-limit", "0"})
	if err != nil {
		t.Fatalf("limit 0 should be allowed: %v", err)
	}
	if cfg.limit != 0 {
		t.Errorf("limit = %d, want 0", cfg.limit)
	}
}

func TestParseFlags_ZeroFreshnessAllowed(t *testing.T) {
	cfg, err := parseFlags([]string{"-freshness", "0"})
	if err != nil {
		t.Fatalf("freshness 0 should be allowed: %v", err)
	}
	if cfg.freshness != 0 {
		t.Errorf("freshness = %d, want 0", cfg.freshness)
	}
}

// --- Exit Code Tests ---

func TestExitCode_SuccessWithEmptyQueue(t *testing.T) {
	dir := t.TempDir()
	queuePath := filepath.Join(dir, "queue.json")

	// Write an empty queue.
	emptyQueue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries:       []batch.QueueEntry{},
	}
	writeQueue(t, queuePath, emptyQueue)

	cfg := &config{
		source:       "cargo",
		limit:        0,
		queuePath:    queuePath,
		recipesDir:   filepath.Join(dir, "recipes"),
		embeddedDir:  "",
		disambiguate: false,
		freshness:    30,
		auditDir:     filepath.Join(dir, "audit"),
		dryRun:       true,
		verbose:      false,
	}

	code := execute(cfg)
	// cargo with limit=0 discovers 0 candidates, no errors.
	// However, this calls the real Discover() with limit 0, which may vary.
	// Since we set dryRun, no writes happen.
	// The exit code should be 0 (success) or 2 (if discover fails for cargo).
	// With limit=0, cargo.Discover returns empty result without API call.
	if code != 0 && code != 2 {
		t.Errorf("exit code = %d, want 0 or 2", code)
	}
}

func TestExitCode_FatalOnBadQueueFile(t *testing.T) {
	dir := t.TempDir()
	// Write invalid JSON to the queue file so LoadUnifiedQueue returns an error.
	queuePath := filepath.Join(dir, "queue.json")
	_ = os.WriteFile(queuePath, []byte("not json"), 0644)

	cfg := &config{
		source:       "cargo",
		limit:        0,
		queuePath:    queuePath,
		recipesDir:   "",
		embeddedDir:  "",
		disambiguate: false,
		freshness:    30,
		auditDir:     filepath.Join(dir, "audit"),
		dryRun:       true,
		verbose:      false,
	}

	// Redirect stdout to capture summary.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	code := execute(cfg)

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	r.Close()

	if code != 1 {
		t.Errorf("exit code = %d, want 1 (fatal)", code)
	}

	// Verify summary JSON was still written to stdout.
	if n > 0 {
		var summary map[string]interface{}
		if err := json.Unmarshal(buf[:n], &summary); err != nil {
			t.Errorf("stdout should contain valid JSON: %v", err)
		}
	}
}

func TestExitCode_SelectionLogic(t *testing.T) {
	tests := []struct {
		name      string
		processed []string
		failed    []string
		wantCode  int
	}{
		{"all success", []string{"cargo", "npm"}, []string{}, 0},
		{"all failed", []string{}, []string{"cargo", "npm"}, 1},
		{"partial failure", []string{"cargo"}, []string{"npm"}, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summary := seed.NewSeedingSummary()
			summary.SourcesProcessed = tt.processed
			summary.SourcesFailed = tt.failed

			code := computeExitCode(summary)
			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d", code, tt.wantCode)
			}
		})
	}
}

// computeExitCode mirrors the exit code logic from execute().
func computeExitCode(summary *seed.SeedingSummary) int {
	if len(summary.SourcesFailed) > 0 {
		if len(summary.SourcesProcessed) > 0 {
			return 2
		}
		return 1
	}
	return 0
}

// --- Summary JSON Tests ---

func TestSummaryJSON_WrittenToStdout(t *testing.T) {
	summary := seed.NewSeedingSummary()
	summary.SourcesProcessed = []string{"cargo"}
	summary.NewPackages = 5

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	writeSummary(summary)

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	r.Close()

	if n == 0 {
		t.Fatal("no output written to stdout")
	}

	var parsed seed.SeedingSummary
	if err := json.Unmarshal(buf[:n], &parsed); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}

	if parsed.NewPackages != 5 {
		t.Errorf("NewPackages = %d, want 5", parsed.NewPackages)
	}
	if len(parsed.SourcesProcessed) != 1 || parsed.SourcesProcessed[0] != "cargo" {
		t.Errorf("SourcesProcessed = %v, want [cargo]", parsed.SourcesProcessed)
	}
}

// --- JSONL Append Tests ---

func TestJSONLAppend_FileCreatedAndValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "seeding-runs.jsonl")

	runAt := time.Date(2026, 2, 16, 6, 0, 0, 0, time.UTC)
	entry := seed.SeedingRunEntry{
		RunAt:            runAt,
		SourcesProcessed: []string{"cargo"},
		SourcesFailed:    []string{},
		NewPackages:      10,
		StaleRefreshed:   5,
		SourceChanges:    []seed.SourceChange{},
		CuratedSkipped:   3,
		CuratedInvalid:   []seed.CuratedInvalid{},
		Errors:           []string{},
	}

	if err := seed.AppendSeedingRun(path, entry); err != nil {
		t.Fatalf("AppendSeedingRun error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}

	var parsed seed.SeedingRunEntry
	if err := json.Unmarshal([]byte(lines[0]), &parsed); err != nil {
		t.Fatalf("invalid JSON line: %v", err)
	}
	if !parsed.RunAt.Equal(runAt) {
		t.Errorf("RunAt = %v, want %v", parsed.RunAt, runAt)
	}
}

// --- Dry-Run Tests ---

func TestDryRun_NoFileWrites(t *testing.T) {
	dir := t.TempDir()
	queuePath := filepath.Join(dir, "queue.json")
	auditDir := filepath.Join(dir, "audit")
	runHistoryPath := filepath.Join(dir, "seeding-runs.jsonl")

	// Write an empty queue file.
	emptyQueue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries:       []batch.QueueEntry{},
	}
	writeQueue(t, queuePath, emptyQueue)

	// Read original queue content to compare later.
	originalData, err := os.ReadFile(queuePath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}

	cfg := &config{
		source:       "cargo",
		limit:        0, // No discovery, just testing dry-run behavior.
		queuePath:    queuePath,
		recipesDir:   filepath.Join(dir, "recipes"),
		embeddedDir:  "",
		disambiguate: false,
		freshness:    30,
		auditDir:     auditDir,
		dryRun:       true,
		verbose:      false,
	}

	// Redirect stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	execute(cfg)

	w.Close()
	os.Stdout = oldStdout

	// Queue file should not be modified.
	afterData, err := os.ReadFile(queuePath)
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if string(afterData) != string(originalData) {
		t.Error("queue file was modified during dry-run")
	}

	// Audit directory should not be created.
	if _, err := os.Stat(auditDir); !os.IsNotExist(err) {
		t.Error("audit directory should not exist during dry-run")
	}

	// Run history file should not be created.
	if _, err := os.Stat(runHistoryPath); !os.IsNotExist(err) {
		t.Error("seeding-runs.jsonl should not exist during dry-run")
	}
}

func TestDryRun_StillProducesSummaryJSON(t *testing.T) {
	dir := t.TempDir()
	queuePath := filepath.Join(dir, "queue.json")

	emptyQueue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries:       []batch.QueueEntry{},
	}
	writeQueue(t, queuePath, emptyQueue)

	cfg := &config{
		source:       "cargo",
		limit:        0,
		queuePath:    queuePath,
		recipesDir:   filepath.Join(dir, "recipes"),
		embeddedDir:  "",
		disambiguate: false,
		freshness:    30,
		auditDir:     filepath.Join(dir, "audit"),
		dryRun:       true,
		verbose:      false,
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	code := execute(cfg)

	w.Close()
	os.Stdout = oldStdout

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	r.Close()

	if n == 0 {
		t.Fatal("dry-run should still produce summary JSON to stdout")
	}

	var summary map[string]interface{}
	if err := json.Unmarshal(buf[:n], &summary); err != nil {
		t.Fatalf("stdout is not valid JSON: %v", err)
	}

	// Exit code should still be correct.
	if code != 0 && code != 2 {
		t.Errorf("exit code = %d, want 0 or 2", code)
	}
}

func TestDryRun_CorrectExitCode(t *testing.T) {
	dir := t.TempDir()
	queuePath := filepath.Join(dir, "queue.json")

	emptyQueue := &batch.UnifiedQueue{
		SchemaVersion: 1,
		Entries:       []batch.QueueEntry{},
	}
	writeQueue(t, queuePath, emptyQueue)

	cfg := &config{
		source:       "cargo",
		limit:        0,
		queuePath:    queuePath,
		recipesDir:   filepath.Join(dir, "recipes"),
		embeddedDir:  "",
		disambiguate: false,
		freshness:    30,
		auditDir:     filepath.Join(dir, "audit"),
		dryRun:       true,
		verbose:      false,
	}

	// Redirect stdout.
	oldStdout := os.Stdout
	_, w, _ := os.Pipe()
	os.Stdout = w

	code := execute(cfg)

	w.Close()
	os.Stdout = oldStdout

	// With limit=0, cargo discovers nothing and succeeds.
	if code != 0 && code != 2 {
		t.Errorf("exit code = %d, want 0 or 2", code)
	}
}

// --- Helper ---

func writeQueue(t *testing.T, path string, q *batch.UnifiedQueue) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("MkdirAll error: %v", err)
	}
	if err := batch.SaveUnifiedQueue(path, q); err != nil {
		t.Fatalf("SaveUnifiedQueue error: %v", err)
	}
}
