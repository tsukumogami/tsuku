package builders

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteBaseline_MinimumPassRate(t *testing.T) {
	dir := t.TempDir()

	tests := []struct {
		name      string
		results   map[string]string
		wantErr   bool
		errSubstr string
	}{
		{
			name: "all passing - accepted",
			results: map[string]string{
				"test_a": "pass",
				"test_b": "pass",
				"test_c": "pass",
			},
			wantErr: false,
		},
		{
			name: "exactly 50% - accepted",
			results: map[string]string{
				"test_a": "pass",
				"test_b": "fail",
			},
			wantErr: false,
		},
		{
			name: "below 50% - rejected",
			results: map[string]string{
				"test_a": "pass",
				"test_b": "fail",
				"test_c": "fail",
			},
			wantErr:   true,
			errSubstr: "minimum is 50%",
		},
		{
			name: "all failing - rejected",
			results: map[string]string{
				"test_a": "fail",
				"test_b": "fail",
			},
			wantErr:   true,
			errSubstr: "minimum is 50%",
		},
		{
			name:    "empty results - accepted (no division by zero)",
			results: map[string]string{},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := writeBaselineToDir(dir, "test-provider", "test-model", tt.results)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				if tt.errSubstr != "" && !strings.Contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q does not contain %q", err.Error(), tt.errSubstr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			// Verify file was written with correct content
			data, err := os.ReadFile(filepath.Join(dir, "test-provider.json"))
			if err != nil {
				t.Fatalf("failed to read written baseline: %v", err)
			}
			var b qualityBaseline
			if err := json.Unmarshal(data, &b); err != nil {
				t.Fatalf("failed to parse written baseline: %v", err)
			}
			if b.Provider != "test-provider" {
				t.Errorf("provider = %q, want %q", b.Provider, "test-provider")
			}
			if b.Model != "test-model" {
				t.Errorf("model = %q, want %q", b.Model, "test-model")
			}
			if len(b.Baselines) != len(tt.results) {
				t.Errorf("baselines count = %d, want %d", len(b.Baselines), len(tt.results))
			}
		})
	}
}

func TestReportRegressions_NoRegression(t *testing.T) {
	baseline := &qualityBaseline{
		Provider: "test",
		Model:    "test-model",
		Baselines: map[string]string{
			"test_a": "pass",
			"test_b": "pass",
			"test_c": "fail",
			"test_d": "fail",
		},
	}

	results := map[string]string{
		"test_a": "pass",
		"test_b": "pass",
		"test_c": "fail",
		"test_d": "fail",
	}

	hadRegressions := reportRegressions(t, baseline, results)
	if hadRegressions {
		t.Errorf("expected no regressions, but reportRegressions returned true")
	}
}

func TestReportRegressions_ImprovementOnly(t *testing.T) {
	baseline := &qualityBaseline{
		Provider: "test",
		Model:    "test-model",
		Baselines: map[string]string{
			"test_a": "pass",
			"test_b": "pass",
			"test_c": "fail",
			"test_d": "fail",
		},
	}

	results := map[string]string{
		"test_a": "pass",
		"test_b": "pass",
		"test_c": "pass", // improvement
		"test_d": "fail",
	}

	hadRegressions := reportRegressions(t, baseline, results)
	if hadRegressions {
		t.Errorf("expected no regressions for improvement-only case")
	}
}

func TestReportRegressions_OrphanedDetected(t *testing.T) {
	baseline := &qualityBaseline{
		Provider: "test",
		Model:    "test-model",
		Baselines: map[string]string{
			"test_a": "pass",
			"test_b": "pass",
			"test_c": "fail",
		},
	}

	// test_b is missing from current run -- should be flagged as orphaned
	results := map[string]string{
		"test_a": "pass",
		"test_c": "fail",
	}

	diff := compareBaseline(baseline, results)
	if len(diff.Orphaned) != 1 || diff.Orphaned[0] != "test_b" {
		t.Errorf("expected orphaned [test_b], got %v", diff.Orphaned)
	}
	if len(diff.Regressions) != 0 {
		t.Errorf("expected no regressions, got %v", diff.Regressions)
	}
}

func TestCompareBaseline_Regressions(t *testing.T) {
	baseline := &qualityBaseline{
		Provider: "test",
		Model:    "test-model",
		Baselines: map[string]string{
			"test_a": "pass",
			"test_b": "fail",
		},
	}

	results := map[string]string{
		"test_a": "fail", // regression
		"test_b": "fail",
	}

	diff := compareBaseline(baseline, results)
	if len(diff.Regressions) != 1 {
		t.Errorf("expected 1 regression, got %d", len(diff.Regressions))
	}
	if len(diff.Regressions) > 0 && diff.Regressions[0] != "test_a" {
		t.Errorf("expected regression on test_a, got %s", diff.Regressions[0])
	}
	if len(diff.Improvements) != 0 {
		t.Errorf("expected 0 improvements, got %d", len(diff.Improvements))
	}
}

func TestCompareBaseline_Mixed(t *testing.T) {
	baseline := &qualityBaseline{
		Provider: "test",
		Model:    "test-model",
		Baselines: map[string]string{
			"test_a": "pass",
			"test_b": "pass",
			"test_c": "fail",
			"test_d": "fail",
		},
	}

	results := map[string]string{
		"test_a": "fail", // regression
		"test_b": "pass",
		"test_c": "pass", // improvement
		"test_d": "fail",
	}

	diff := compareBaseline(baseline, results)
	if len(diff.Regressions) != 1 || diff.Regressions[0] != "test_a" {
		t.Errorf("expected regression [test_a], got %v", diff.Regressions)
	}
	if len(diff.Improvements) != 1 || diff.Improvements[0] != "test_c" {
		t.Errorf("expected improvement [test_c], got %v", diff.Improvements)
	}
	if len(diff.Orphaned) != 0 {
		t.Errorf("expected no orphaned entries, got %v", diff.Orphaned)
	}
}

func TestBaselineKey(t *testing.T) {
	got := baselineKey("llm_github_stern_baseline", "stern")
	want := "llm_github_stern_baseline_stern"
	if got != want {
		t.Errorf("baselineKey() = %q, want %q", got, want)
	}
}

func TestLoadBaseline_MissingFile(t *testing.T) {
	// Override baseline dir to a temp directory
	dir := t.TempDir()
	result, err := loadBaselineFromDir(dir, "nonexistent")
	if err != nil {
		t.Fatalf("unexpected error for missing file: %v", err)
	}
	if result != nil {
		t.Fatalf("expected nil for missing baseline, got %+v", result)
	}
}

func TestLoadBaseline_ValidFile(t *testing.T) {
	dir := t.TempDir()

	baseline := qualityBaseline{
		Provider: "test",
		Model:    "test-model",
		Baselines: map[string]string{
			"case_a": "pass",
			"case_b": "fail",
		},
	}
	data, _ := json.MarshalIndent(baseline, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "test.json"), data, 0644); err != nil {
		t.Fatalf("failed to write test baseline: %v", err)
	}

	result, err := loadBaselineFromDir(dir, "test")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected baseline, got nil")
	}
	if result.Provider != "test" {
		t.Errorf("provider = %q, want %q", result.Provider, "test")
	}
	if len(result.Baselines) != 2 {
		t.Errorf("baselines count = %d, want 2", len(result.Baselines))
	}
}

func TestLoadBaseline_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "broken.json"), []byte("{invalid"), 0644); err != nil {
		t.Fatalf("failed to write test file: %v", err)
	}

	_, err := loadBaselineFromDir(dir, "broken")
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestProviderModel(t *testing.T) {
	tests := []struct {
		provider string
		wantNot  string // just check it's not empty
	}{
		{"claude", ""},
		{"gemini", ""},
		{"local", ""},
		{"unknown", ""},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			model := providerModel(tt.provider)
			if model == "" {
				t.Errorf("providerModel(%q) returned empty string", tt.provider)
			}
		})
	}
}
