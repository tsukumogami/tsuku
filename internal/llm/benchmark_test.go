package llm

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"
)

// benchmarkTestMatrix is the shared test matrix from builders.
// We embed a copy here because the benchmark runs in the llm package,
// and we need the test case definitions.
//
//go:embed testdata/benchmark-matrix.json
var benchmarkTestMatrixJSON []byte

// benchmarkTestMatrix represents the structure of the test matrix.
type benchmarkTestMatrix struct {
	Description string                      `json:"description"`
	Tests       map[string]benchmarkTestDef `json:"tests"`
}

// benchmarkTestDef is a single test case for benchmarking.
type benchmarkTestDef struct {
	Tool     string   `json:"tool"`
	Builder  string   `json:"builder"`
	Desc     string   `json:"desc"`
	Action   string   `json:"action"`
	Format   string   `json:"format"`
	Features []string `json:"features"`
}

// BenchmarkResult holds metrics for a single provider + test case run.
type BenchmarkResult struct {
	Provider    string    `json:"provider"`
	TestCase    string    `json:"test_case"`
	Tool        string    `json:"tool"`
	Passed      bool      `json:"passed"`
	RepairTurns int       `json:"repair_turns"`
	LatencyMS   float64   `json:"latency_ms"`
	Error       string    `json:"error,omitempty"`
	Timestamp   time.Time `json:"timestamp"`
}

// BenchmarkSummary aggregates results across test cases for a single provider.
type BenchmarkSummary struct {
	Provider       string  `json:"provider"`
	TotalTests     int     `json:"total_tests"`
	PassCount      int     `json:"pass_count"`
	PassRate       float64 `json:"pass_rate"`
	FirstTryRate   float64 `json:"first_try_rate"`
	AvgRepairTurns float64 `json:"avg_repair_turns"`
	LatencyP50MS   float64 `json:"latency_p50_ms"`
	LatencyP99MS   float64 `json:"latency_p99_ms"`
}

// BenchmarkReport is the full benchmark output written to JSON.
type BenchmarkReport struct {
	RunDate   string             `json:"run_date"`
	Results   []BenchmarkResult  `json:"results"`
	Summaries []BenchmarkSummary `json:"summaries"`
}

// benchmarkRunner executes test cases against a provider and collects metrics.
type benchmarkRunner struct {
	provider Provider
	mu       sync.Mutex
	results  []BenchmarkResult
}

func newBenchmarkRunner(provider Provider) *benchmarkRunner {
	return &benchmarkRunner{
		provider: provider,
	}
}

// runTestCase executes a single test case and records the result.
// It simulates a multi-turn recipe generation conversation with the provider,
// tracking whether extract_pattern was called and how many turns it took.
func (r *benchmarkRunner) runTestCase(t *testing.T, testID string, tc benchmarkTestDef) BenchmarkResult {
	t.Helper()

	// Use a generous timeout for local models which may be slower.
	timeout := 3 * time.Minute
	if os.Getenv("LLM_BENCHMARK_TIMEOUT") != "" {
		if d, err := time.ParseDuration(os.Getenv("LLM_BENCHMARK_TIMEOUT")); err == nil {
			timeout = d
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	start := time.Now()

	// Build a request mimicking what the builder would send
	req := &CompletionRequest{
		SystemPrompt: buildSystemPrompt(),
		Messages: []Message{
			{
				Role: RoleUser,
				Content: fmt.Sprintf(
					"Analyze this tool: %s. Builder type: %s. Expected action: %s. Description: %s. "+
						"Call extract_pattern when you have determined the asset-to-platform mappings.",
					tc.Tool, tc.Builder, tc.Action, tc.Desc,
				),
			},
		},
		Tools:     buildToolDefs(),
		MaxTokens: 4096,
	}

	result := BenchmarkResult{
		Provider:  r.provider.Name(),
		TestCase:  testID,
		Tool:      tc.Tool,
		Timestamp: time.Now(),
	}

	// Run multi-turn conversation (up to MaxTurns)
	messages := req.Messages
	for turn := 0; turn < MaxTurns; turn++ {
		resp, err := r.provider.Complete(ctx, &CompletionRequest{
			SystemPrompt: req.SystemPrompt,
			Messages:     messages,
			Tools:        req.Tools,
			MaxTokens:    req.MaxTokens,
		})
		if err != nil {
			result.Error = err.Error()
			result.LatencyMS = float64(time.Since(start).Milliseconds())
			result.RepairTurns = turn
			r.recordResult(result)
			return result
		}

		// Add assistant response to conversation
		messages = append(messages, Message{
			Role:      RoleAssistant,
			Content:   resp.Content,
			ToolCalls: resp.ToolCalls,
		})

		// Check for extract_pattern call
		for _, tc := range resp.ToolCalls {
			if tc.Name == ToolExtractPattern {
				result.Passed = true
				result.RepairTurns = turn
				result.LatencyMS = float64(time.Since(start).Milliseconds())
				r.recordResult(result)
				return result
			}

			// For other tool calls, simulate a response
			messages = append(messages, Message{
				Role: RoleUser,
				ToolResult: &ToolResult{
					CallID:  tc.ID,
					Content: fmt.Sprintf("Simulated result for %s", tc.Name),
				},
			})
		}

		// If no tool calls, the provider finished without extracting a pattern
		if len(resp.ToolCalls) == 0 && resp.StopReason == "end_turn" {
			result.Error = "conversation ended without extract_pattern"
			result.RepairTurns = turn
			break
		}
	}

	result.LatencyMS = float64(time.Since(start).Milliseconds())
	if !result.Passed && result.Error == "" {
		result.Error = fmt.Sprintf("max turns (%d) exceeded", MaxTurns)
		result.RepairTurns = MaxTurns
	}

	r.recordResult(result)
	return result
}

func (r *benchmarkRunner) recordResult(res BenchmarkResult) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.results = append(r.results, res)
}

func (r *benchmarkRunner) getResults() []BenchmarkResult {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]BenchmarkResult, len(r.results))
	copy(out, r.results)
	return out
}

// computeSummary produces aggregate statistics from collected results.
func computeSummary(providerName string, results []BenchmarkResult) BenchmarkSummary {
	s := BenchmarkSummary{
		Provider:   providerName,
		TotalTests: len(results),
	}
	if len(results) == 0 {
		return s
	}

	var totalRepairTurns int
	var firstTryCount int
	var latencies []float64

	for _, r := range results {
		if r.Passed {
			s.PassCount++
			totalRepairTurns += r.RepairTurns
			if r.RepairTurns == 0 {
				firstTryCount++
			}
		}
		latencies = append(latencies, r.LatencyMS)
	}

	s.PassRate = float64(s.PassCount) / float64(s.TotalTests) * 100.0
	s.FirstTryRate = float64(firstTryCount) / float64(s.TotalTests) * 100.0

	if s.PassCount > 0 {
		s.AvgRepairTurns = float64(totalRepairTurns) / float64(s.PassCount)
	}

	// Compute percentiles
	sort.Float64s(latencies)
	s.LatencyP50MS = percentile(latencies, 50)
	s.LatencyP99MS = percentile(latencies, 99)

	return s
}

// percentile computes the p-th percentile of a sorted slice.
func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	if len(sorted) == 1 {
		return sorted[0]
	}
	rank := p / 100.0 * float64(len(sorted)-1)
	lower := int(math.Floor(rank))
	upper := int(math.Ceil(rank))
	if lower == upper {
		return sorted[lower]
	}
	frac := rank - float64(lower)
	return sorted[lower]*(1-frac) + sorted[upper]*frac
}

// writeReport writes the benchmark report to a JSON file.
func writeReport(t *testing.T, report *BenchmarkReport) string {
	t.Helper()

	outputDir := os.Getenv("LLM_BENCHMARK_OUTPUT")
	if outputDir == "" {
		outputDir = filepath.Join(os.TempDir(), "tsuku-llm-benchmark")
	}

	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Logf("Warning: could not create output directory %s: %v", outputDir, err)
		return ""
	}

	filename := fmt.Sprintf("benchmark-%s.json", time.Now().Format("20060102-150405"))
	path := filepath.Join(outputDir, filename)

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Logf("Warning: could not marshal report: %v", err)
		return ""
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		t.Logf("Warning: could not write report to %s: %v", path, err)
		return ""
	}

	return path
}

// TestRecipeQualityBenchmark runs the test matrix against multiple providers
// and reports comparative quality metrics.
//
// By default, this test verifies the benchmark infrastructure using mock providers.
// To run against real providers, set:
//
//	LLM_BENCHMARK=true go test -v -run TestRecipeQualityBenchmark ./internal/llm/
//
// Environment variables:
//   - LLM_BENCHMARK=true: enable real provider benchmarks (requires API keys)
//   - LLM_BENCHMARK_OUTPUT: directory for JSON report output (default: $TMPDIR/tsuku-llm-benchmark)
//   - ANTHROPIC_API_KEY: required for Claude provider
//   - GOOGLE_API_KEY or GEMINI_API_KEY: required for Gemini provider
func TestRecipeQualityBenchmark(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping benchmark in short mode")
	}

	// Load test matrix
	var matrix benchmarkTestMatrix
	if err := json.Unmarshal(benchmarkTestMatrixJSON, &matrix); err != nil {
		t.Fatalf("failed to parse benchmark-matrix.json: %v", err)
	}

	if len(matrix.Tests) == 0 {
		t.Fatal("benchmark matrix has no test cases")
	}
	t.Logf("Loaded %d test cases from benchmark matrix", len(matrix.Tests))

	// Determine which providers to use
	useReal := os.Getenv("LLM_BENCHMARK") == "true"

	var providers []Provider
	if useReal {
		t.Log("Running benchmark against real providers")
		providers = discoverProviders(t)
		if len(providers) == 0 {
			t.Fatal("LLM_BENCHMARK=true but no providers available (set API keys)")
		}
	} else {
		t.Log("Running benchmark with mock providers (set LLM_BENCHMARK=true for real providers)")
		providers = createMockBenchmarkProviders()
	}

	// Collect test case IDs in deterministic order
	testIDs := sortedKeys(matrix.Tests)

	// Run benchmark for each provider
	allResults := make([]BenchmarkResult, 0)
	summaries := make([]BenchmarkSummary, 0, len(providers))

	for _, provider := range providers {
		t.Run("provider="+provider.Name(), func(t *testing.T) {
			runner := newBenchmarkRunner(provider)

			for _, testID := range testIDs {
				tc := matrix.Tests[testID]
				t.Run(testID, func(t *testing.T) {
					result := runner.runTestCase(t, testID, tc)
					if result.Passed {
						t.Logf("PASS: %s (turns=%d, latency=%.0fms)",
							tc.Tool, result.RepairTurns, result.LatencyMS)
					} else {
						t.Logf("FAIL: %s (error=%s, latency=%.0fms)",
							tc.Tool, result.Error, result.LatencyMS)
					}
				})
			}

			results := runner.getResults()
			summary := computeSummary(provider.Name(), results)
			summaries = append(summaries, summary)
			allResults = append(allResults, results...)

			t.Logf("Summary for %s: pass_rate=%.1f%%, first_try=%.1f%%, "+
				"avg_repair=%.1f, p50=%.0fms, p99=%.0fms",
				provider.Name(),
				summary.PassRate,
				summary.FirstTryRate,
				summary.AvgRepairTurns,
				summary.LatencyP50MS,
				summary.LatencyP99MS,
			)
		})
	}

	// Generate report
	report := &BenchmarkReport{
		RunDate:   time.Now().Format(time.RFC3339),
		Results:   allResults,
		Summaries: summaries,
	}

	if path := writeReport(t, report); path != "" {
		t.Logf("Benchmark report written to: %s", path)
	}

	// Log cross-provider comparison
	if len(summaries) > 1 {
		t.Log("--- Cross-Provider Comparison ---")
		for _, s := range summaries {
			t.Logf("  %s: pass_rate=%.1f%%, first_try=%.1f%%, avg_repair=%.1f, p50=%.0fms",
				s.Provider, s.PassRate, s.FirstTryRate, s.AvgRepairTurns, s.LatencyP50MS)
		}
	}
}

// TestBenchmarkResultSerialization verifies that benchmark results can be
// serialized to and from JSON for CI tracking.
func TestBenchmarkResultSerialization(t *testing.T) {
	report := &BenchmarkReport{
		RunDate: "2026-02-15T10:00:00Z",
		Results: []BenchmarkResult{
			{
				Provider:    "claude",
				TestCase:    "llm_github_stern_baseline",
				Tool:        "stern",
				Passed:      true,
				RepairTurns: 0,
				LatencyMS:   1234.5,
				Timestamp:   time.Date(2026, 2, 15, 10, 0, 0, 0, time.UTC),
			},
			{
				Provider:    "local",
				TestCase:    "llm_github_stern_baseline",
				Tool:        "stern",
				Passed:      true,
				RepairTurns: 1,
				LatencyMS:   5678.9,
				Timestamp:   time.Date(2026, 2, 15, 10, 0, 1, 0, time.UTC),
			},
		},
		Summaries: []BenchmarkSummary{
			{
				Provider:       "claude",
				TotalTests:     1,
				PassCount:      1,
				PassRate:       100.0,
				FirstTryRate:   100.0,
				AvgRepairTurns: 0,
				LatencyP50MS:   1234.5,
				LatencyP99MS:   1234.5,
			},
		},
	}

	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal report: %v", err)
	}

	var parsed BenchmarkReport
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if len(parsed.Results) != 2 {
		t.Errorf("expected 2 results, got %d", len(parsed.Results))
	}
	if len(parsed.Summaries) != 1 {
		t.Errorf("expected 1 summary, got %d", len(parsed.Summaries))
	}
	if parsed.Results[0].Provider != "claude" {
		t.Errorf("expected provider 'claude', got %q", parsed.Results[0].Provider)
	}
	if parsed.Summaries[0].PassRate != 100.0 {
		t.Errorf("expected pass rate 100.0, got %.1f", parsed.Summaries[0].PassRate)
	}
}

// TestComputeSummary verifies the summary statistics computation.
func TestComputeSummary(t *testing.T) {
	results := []BenchmarkResult{
		{Passed: true, RepairTurns: 0, LatencyMS: 100},
		{Passed: true, RepairTurns: 1, LatencyMS: 200},
		{Passed: true, RepairTurns: 0, LatencyMS: 150},
		{Passed: false, RepairTurns: 5, LatencyMS: 3000, Error: "timeout"},
		{Passed: true, RepairTurns: 2, LatencyMS: 500},
	}

	summary := computeSummary("test-provider", results)

	if summary.TotalTests != 5 {
		t.Errorf("TotalTests: got %d, want 5", summary.TotalTests)
	}
	if summary.PassCount != 4 {
		t.Errorf("PassCount: got %d, want 4", summary.PassCount)
	}
	if summary.PassRate != 80.0 {
		t.Errorf("PassRate: got %.1f, want 80.0", summary.PassRate)
	}
	// First try: 2 out of 5 = 40%
	if summary.FirstTryRate != 40.0 {
		t.Errorf("FirstTryRate: got %.1f, want 40.0", summary.FirstTryRate)
	}
	// Average repair turns for passed: (0 + 1 + 0 + 2) / 4 = 0.75
	if math.Abs(summary.AvgRepairTurns-0.75) > 0.01 {
		t.Errorf("AvgRepairTurns: got %.2f, want 0.75", summary.AvgRepairTurns)
	}
}

// TestPercentile verifies percentile calculation.
func TestPercentile(t *testing.T) {
	tests := []struct {
		name     string
		values   []float64
		p        float64
		expected float64
	}{
		{"empty", nil, 50, 0},
		{"single", []float64{42.0}, 50, 42.0},
		{"p50 of two", []float64{10.0, 20.0}, 50, 15.0},
		{"p99 of three", []float64{10.0, 20.0, 30.0}, 99, 29.8},
		{"p50 of five", []float64{100, 150, 200, 500, 3000}, 50, 200},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sorted := make([]float64, len(tt.values))
			copy(sorted, tt.values)
			sort.Float64s(sorted)

			got := percentile(sorted, tt.p)
			if math.Abs(got-tt.expected) > 0.1 {
				t.Errorf("percentile(%.0f): got %.1f, want %.1f", tt.p, got, tt.expected)
			}
		})
	}
}

// TestComputeSummaryEmpty handles the edge case of no results.
func TestComputeSummaryEmpty(t *testing.T) {
	summary := computeSummary("empty", nil)
	if summary.TotalTests != 0 {
		t.Errorf("TotalTests: got %d, want 0", summary.TotalTests)
	}
	if summary.PassRate != 0 {
		t.Errorf("PassRate: got %.1f, want 0", summary.PassRate)
	}
}

// discoverProviders detects available real providers from environment.
func discoverProviders(t *testing.T) []Provider {
	t.Helper()
	var providers []Provider

	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		p, err := NewClaudeProvider()
		if err != nil {
			t.Logf("Warning: could not create Claude provider: %v", err)
		} else {
			providers = append(providers, p)
			t.Log("Claude provider available")
		}
	}

	ctx := context.Background()
	if os.Getenv("GOOGLE_API_KEY") != "" || os.Getenv("GEMINI_API_KEY") != "" {
		p, err := NewGeminiProvider(ctx)
		if err != nil {
			t.Logf("Warning: could not create Gemini provider: %v", err)
		} else {
			providers = append(providers, p)
			t.Log("Gemini provider available")
		}
	}

	if IsAddonRunning() {
		p := NewLocalProvider()
		providers = append(providers, p)
		t.Log("Local provider available")
	}

	return providers
}

// createMockBenchmarkProviders returns mock providers that simulate
// the behavior of real providers for infrastructure testing.
func createMockBenchmarkProviders() []Provider {
	// Mock "claude" that always succeeds on first try
	claudeResponses := make([]*CompletionResponse, 0, 50)
	for i := 0; i < 50; i++ {
		claudeResponses = append(claudeResponses, &CompletionResponse{
			ToolCalls: []ToolCall{
				{
					ID:   fmt.Sprintf("call_%d", i),
					Name: ToolExtractPattern,
					Arguments: map[string]any{
						"mappings": []any{
							map[string]any{
								"os": "linux", "arch": "amd64",
								"asset": "tool_linux_amd64.tar.gz", "format": "tar.gz",
							},
						},
						"executable":     "tool",
						"verify_command": "tool --version",
					},
				},
			},
			StopReason: "tool_use",
			Usage:      Usage{InputTokens: 500, OutputTokens: 100},
		})
	}

	// Mock "local" that needs one repair turn sometimes
	localResponses := make([]*CompletionResponse, 0, 100)
	for i := 0; i < 100; i++ {
		if i%3 == 0 {
			// Simulate a fetch_file call first
			localResponses = append(localResponses, &CompletionResponse{
				ToolCalls: []ToolCall{
					{
						ID:        fmt.Sprintf("local_fetch_%d", i),
						Name:      ToolFetchFile,
						Arguments: map[string]any{"path": "README.md"},
					},
				},
				StopReason: "tool_use",
				Usage:      Usage{InputTokens: 400, OutputTokens: 80},
			})
		}
		// Then extract_pattern
		localResponses = append(localResponses, &CompletionResponse{
			ToolCalls: []ToolCall{
				{
					ID:   fmt.Sprintf("local_extract_%d", i),
					Name: ToolExtractPattern,
					Arguments: map[string]any{
						"mappings": []any{
							map[string]any{
								"os": "linux", "arch": "amd64",
								"asset": "tool_linux_amd64.tar.gz", "format": "tar.gz",
							},
						},
						"executable":     "tool",
						"verify_command": "tool --version",
					},
				},
			},
			StopReason: "tool_use",
			Usage:      Usage{InputTokens: 400, OutputTokens: 80},
		})
	}

	return []Provider{
		NewMockProvider("claude", claudeResponses),
		NewMockProvider("local", localResponses),
	}
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]benchmarkTestDef) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
