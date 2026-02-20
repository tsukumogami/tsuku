package builders

import (
	"context"
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/tsukumogami/tsuku/internal/llm"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

//go:embed llm-test-matrix.json
var llmTestMatrixJSON []byte

var updateBaseline = flag.Bool("update-baseline", false, "update quality baseline files")

// qualityBaseline represents a per-provider quality baseline file.
type qualityBaseline struct {
	Provider  string            `json:"provider"`
	Model     string            `json:"model"`
	Baselines map[string]string `json:"baselines"`
}

// llmTestMatrix represents the structure of llm-test-matrix.json
type llmTestMatrix struct {
	Description string                 `json:"description"`
	Tests       map[string]llmTestCase `json:"tests"`
}

// llmTestCase represents a single test case in the matrix
type llmTestCase struct {
	Builder     string   `json:"builder"`      // "github" or "homebrew"
	Tool        string   `json:"tool"`         // Tool name
	Repo        string   `json:"repo"`         // GitHub repo (for github builder)
	Formula     string   `json:"formula"`      // Homebrew formula name (for homebrew builder)
	Recipe      string   `json:"recipe"`       // Path to ground truth recipe
	Action      string   `json:"action"`       // Expected action type
	Format      string   `json:"format"`       // Archive format (for github_archive)
	BuildSystem string   `json:"build_system"` // Build system (for source builds)
	Desc        string   `json:"desc"`         // Test description
	Features    []string `json:"features"`     // Features being tested
}

// detectProvider checks environment variables in priority order and returns
// the provider name and a configured llm.Provider. Returns empty strings if
// no provider is available.
func detectProvider(t *testing.T) (string, llm.Provider) {
	t.Helper()

	// Priority 1: Local provider via TSUKU_LLM_BINARY
	if os.Getenv("TSUKU_LLM_BINARY") != "" {
		provider := llm.NewLocalProvider()
		return provider.Name(), provider
	}

	// Priority 2: Claude via ANTHROPIC_API_KEY
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		provider, err := llm.NewClaudeProvider()
		if err != nil {
			t.Fatalf("ANTHROPIC_API_KEY set but failed to create Claude provider: %v", err)
		}
		return provider.Name(), provider
	}

	// Priority 3: Gemini via GOOGLE_API_KEY
	if os.Getenv("GOOGLE_API_KEY") != "" {
		provider, err := llm.NewGeminiProvider(context.Background())
		if err != nil {
			t.Fatalf("GOOGLE_API_KEY set but failed to create Gemini provider: %v", err)
		}
		return provider.Name(), provider
	}

	return "", nil
}

// baselineDir returns the path to the baselines directory, relative to the
// repository root. The directory is resolved from the test working directory.
func baselineDir() string {
	candidates := []string{
		"../../testdata/llm-quality-baselines",
		"testdata/llm-quality-baselines",
	}
	for _, c := range candidates {
		if info, err := os.Stat(c); err == nil && info.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs
		}
	}
	// Fall back to the expected location from internal/builders/
	abs, _ := filepath.Abs("../../testdata/llm-quality-baselines")
	return abs
}

// loadBaseline reads a per-provider baseline file. Returns nil if the file
// does not exist (first run for this provider).
func loadBaseline(providerName string) (*qualityBaseline, error) {
	return loadBaselineFromDir(baselineDir(), providerName)
}

// loadBaselineFromDir reads a baseline file from a specific directory.
func loadBaselineFromDir(dir, providerName string) (*qualityBaseline, error) {
	path := filepath.Join(dir, providerName+".json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading baseline %s: %w", path, err)
	}
	var b qualityBaseline
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("parsing baseline %s: %w", path, err)
	}
	return &b, nil
}

// writeBaseline writes results as a new baseline file. Returns an error if
// the pass rate is below the minimum threshold (50%).
func writeBaseline(providerName, model string, results map[string]string) error {
	return writeBaselineToDir(baselineDir(), providerName, model, results)
}

// writeBaselineToDir writes a baseline file to a specific directory.
func writeBaselineToDir(dir, providerName, model string, results map[string]string) error {
	// Sanity check: require at least 50% pass rate
	passed := 0
	for _, status := range results {
		if status == baselinePass {
			passed++
		}
	}
	total := len(results)
	if total > 0 && float64(passed)/float64(total) < 0.5 {
		return fmt.Errorf(
			"refusing to write baseline: only %d/%d (%.0f%%) cases passed, minimum is 50%%",
			passed, total, float64(passed)/float64(total)*100,
		)
	}

	b := qualityBaseline{
		Provider:  providerName,
		Model:     model,
		Baselines: results,
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling baseline: %w", err)
	}
	data = append(data, '\n')

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("creating baseline directory: %w", err)
	}
	path := filepath.Join(dir, providerName+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("writing baseline %s: %w", path, err)
	}
	return nil
}

// Baseline result status constants. These values are persisted in baseline JSON
// files and used as map values in test results. Changing them requires migrating
// existing baseline files.
const (
	baselinePass = "pass"
	baselineFail = "fail"
)

// baselineKey constructs the key used to identify a test case in baseline files.
// The format is "<testID>_<tool>" where testID comes from the test matrix and
// tool is the tool name from the test case. This key is used both as the Go
// subtest name and as the map key in baseline JSON files; changing this format
// requires migrating existing baselines.
func baselineKey(testID, tool string) string {
	return testID + "_" + tool
}

// baselineDiff holds the result of comparing current test results against a baseline.
type baselineDiff struct {
	Regressions  []string // previously-passing cases that now fail
	Improvements []string // previously-failing cases that now pass
	Orphaned     []string // baseline entries with no matching result (test renamed or removed)
}

// compareBaseline computes the diff between current results and a baseline.
func compareBaseline(baseline *qualityBaseline, results map[string]string) baselineDiff {
	var diff baselineDiff

	for name, baselineStatus := range baseline.Baselines {
		currentStatus, ok := results[name]
		if !ok {
			// Baseline entry has no matching result -- test was renamed or removed
			diff.Orphaned = append(diff.Orphaned, name)
			continue
		}
		if baselineStatus == baselinePass && currentStatus == baselineFail {
			diff.Regressions = append(diff.Regressions, name)
		}
		if baselineStatus == baselineFail && currentStatus == baselinePass {
			diff.Improvements = append(diff.Improvements, name)
		}
	}

	// Sort for deterministic output
	sort.Strings(diff.Regressions)
	sort.Strings(diff.Improvements)
	sort.Strings(diff.Orphaned)

	return diff
}

// reportRegressions compares current results against a baseline and reports
// regressions (previously-passing cases that now fail). Returns true if
// regressions were found.
func reportRegressions(t *testing.T, baseline *qualityBaseline, results map[string]string) bool {
	t.Helper()

	diff := compareBaseline(baseline, results)

	if len(diff.Improvements) > 0 {
		t.Logf("Improvements (previously-failing cases now passing):")
		for _, name := range diff.Improvements {
			t.Logf("  + %s", name)
		}
		t.Logf("Run with -update-baseline to update the baseline file.")
	}

	if len(diff.Orphaned) > 0 {
		t.Errorf("Orphaned baseline entries (test renamed or removed?):")
		for _, name := range diff.Orphaned {
			t.Errorf("  ? %s: in baseline but not in current results", name)
		}
		t.Errorf("Run with -update-baseline to update the baseline file.")
	}

	if len(diff.Regressions) > 0 {
		t.Errorf("Quality regressions detected against %s baseline:", baseline.Provider)
		for _, name := range diff.Regressions {
			t.Errorf("  - %s: was pass, now fail", name)
		}
	}

	return len(diff.Regressions) > 0 || len(diff.Orphaned) > 0
}

// providerModel returns a human-readable model identifier for the provider.
func providerModel(providerName string) string {
	switch providerName {
	case "claude":
		return llm.Model
	case "gemini":
		return llm.GeminiModel
	case "local":
		return "local"
	default:
		return providerName
	}
}

// TestLLMGroundTruth validates LLM-generated recipes against ground truth.
// The test detects the active provider from environment variables in priority
// order: TSUKU_LLM_BINARY (local) > ANTHROPIC_API_KEY (Claude) >
// GOOGLE_API_KEY (Gemini). It skips when no provider is configured.
//
// Both builders receive the detected provider via factory injection using
// WithFactory and WithHomebrewFactory.
//
// Test cases are defined in llm-test-matrix.json, with each test validating
// a specific variation to isolate failures.
//
// After all cases run, results are compared against the per-provider baseline
// in testdata/llm-quality-baselines/<provider>.json. The test fails if a
// previously-passing case now fails (regression). Use -update-baseline to
// write new baseline files.
//
// To run:
//
//	ANTHROPIC_API_KEY=sk-... go test -run TestLLMGroundTruth ./internal/builders/
//	ANTHROPIC_API_KEY=sk-... go test -run TestLLMGroundTruth ./internal/builders/ -update-baseline
func TestLLMGroundTruth(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping LLM integration test in short mode")
	}

	providerName, provider := detectProvider(t)
	if provider == nil {
		t.Skip("Skipping LLM integration test: no provider configured (set TSUKU_LLM_BINARY, ANTHROPIC_API_KEY, or GOOGLE_API_KEY)")
	}
	t.Logf("Using provider: %s", providerName)

	// Create factory with the detected provider and inject into builders
	factory := llm.NewFactoryWithProviders(
		map[string]llm.Provider{providerName: provider},
		llm.WithPrimaryProvider(providerName),
	)
	githubBuilder := NewGitHubReleaseBuilder(WithFactory(factory))
	homebrewBuilder := NewHomebrewBuilder(WithHomebrewFactory(factory))

	// Load test matrix
	var matrix llmTestMatrix
	if err := json.Unmarshal(llmTestMatrixJSON, &matrix); err != nil {
		t.Fatalf("Failed to parse llm-test-matrix.json: %v", err)
	}

	// Find the recipes directory (relative to test file)
	recipesDir := findRecipesDir(t)

	// Create output directory for generated recipes
	outputDir := filepath.Join(os.TempDir(), "tsuku-llm-test")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}
	t.Logf("Generated recipes will be saved to: %s", outputDir)

	// Run tests in order (GitHub release tests, then Homebrew source builds)
	testIDs := []string{
		"llm_github_stern_baseline", "llm_github_tflint_zip", "llm_github_helix_tar_xz",
		"llm_github_ast-grep_rust_triple", "llm_github_k9s_capitalized_os", "llm_github_trivy_custom_arch",
		"llm_github_gitleaks_x64", "llm_github_gotop_v_prefix", "llm_github_gobuster_no_version",
		"llm_github_age_strip_dirs", "llm_github_liberica_multi_binary", "llm_github_btop_install_subpath",
		"llm_github_fly_binary_rename", "llm_github_k3d_file_baseline", "llm_github_cosign_file_rename",
		"llm_github_minikube_file_no_mapping", "llm_github_kopia_macos_mapping", "llm_github_cargo-deny_musl",
	}

	// Track per-test results. Validation mismatches are logged (not errors)
	// so individual subtests don't fail the parent. The baseline regression
	// check at the end is the sole pass/fail gate.
	results := make(map[string]string)

	for _, testID := range testIDs {
		tc, ok := matrix.Tests[testID]
		if !ok {
			t.Errorf("Test case %s not found in matrix", testID)
			continue
		}

		key := baselineKey(testID, tc.Tool)
		t.Run(key, func(t *testing.T) {
			t.Logf("Testing: %s - %s", tc.Tool, tc.Desc)

			// Per-test timeout: 10 minutes for CPU inference with local model
			// (cloud providers finish in ~10s, but local 0.5B on CPU needs longer)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			// Load ground truth recipe
			groundTruthPath := filepath.Join(recipesDir, tc.Recipe)
			expected, err := loadRecipe(groundTruthPath)
			if err != nil {
				t.Logf("FAIL: could not load ground truth recipe: %v", err)
				results[key] = baselineFail
				return
			}

			// Select builder and build request based on test case
			var result *BuildResult
			var session BuildSession
			if tc.Builder == "homebrew" {
				req := BuildRequest{
					Package:   tc.Tool,
					SourceArg: tc.Formula + ":source",
				}
				session, err = homebrewBuilder.NewSession(ctx, req, nil)
			} else {
				req := BuildRequest{
					Package:   tc.Tool,
					SourceArg: tc.Repo,
				}
				session, err = githubBuilder.NewSession(ctx, req, nil)
			}
			if err != nil {
				t.Logf("FAIL: could not create session: %v", err)
				results[key] = baselineFail
				return
			}
			defer func() { _ = session.Close() }()

			result, err = session.Generate(ctx)
			if err != nil {
				t.Logf("FAIL: LLM recipe generation failed: %v", err)
				results[key] = baselineFail
				return
			}

			generated := result.Recipe

			// Save generated recipe for debugging
			outputPath := filepath.Join(outputDir, tc.Tool+".toml")
			if err := recipe.WriteRecipe(generated, outputPath); err != nil {
				t.Logf("Warning: failed to save generated recipe: %v", err)
			} else {
				t.Logf("Generated recipe saved to: %s", outputPath)
			}

			// Validate and record result
			var mismatches []string
			if tc.Builder == "homebrew" {
				mismatches = validateHomebrewSourceRecipe(t, tc, generated, expected)
			} else {
				mismatches = validateGitHubRecipe(t, tc, generated, expected)
			}

			if len(mismatches) > 0 {
				for _, m := range mismatches {
					t.Logf("MISMATCH: %s", m)
				}
				results[key] = baselineFail
			} else {
				results[key] = baselinePass
			}
		})

		// If the subtest didn't set a result (e.g., panic), mark as fail
		if _, ok := results[key]; !ok {
			results[key] = baselineFail
		}
	}

	// Log summary
	passCount := 0
	for _, status := range results {
		if status == baselinePass {
			passCount++
		}
	}
	t.Logf("Results: %d/%d passed", passCount, len(results))

	// Handle baseline update or regression check
	if *updateBaseline {
		model := providerModel(providerName)
		if err := writeBaseline(providerName, model, results); err != nil {
			t.Fatalf("Failed to update baseline: %v", err)
		}
		t.Logf("Baseline written for provider %s (%d cases)", providerName, len(results))
		return
	}

	// Load baseline for regression comparison
	baseline, err := loadBaseline(providerName)
	if err != nil {
		t.Fatalf("Failed to load baseline: %v", err)
	}
	if baseline == nil {
		t.Logf("No baseline file for provider %s; skipping regression check. Run with -update-baseline to create one.", providerName)
		return
	}

	reportRegressions(t, baseline, results)
}

// validateGitHubRecipe validates a GitHub release recipe and returns any
// mismatches found. Uses t.Logf for diagnostics but does not call t.Errorf
// since the baseline regression check is the sole pass/fail gate.
func validateGitHubRecipe(t *testing.T, tc llmTestCase, generated, expected *recipe.Recipe) []string {
	t.Helper()

	var mismatches []string

	if len(generated.Steps) == 0 {
		return []string{"generated recipe has no steps"}
	}

	step := generated.Steps[0]

	// Check action type
	if step.Action != tc.Action {
		mismatches = append(mismatches, fmt.Sprintf("action: got %s, want %s", step.Action, tc.Action))
	}

	// Check archive format if applicable
	if tc.Format != "" {
		format, _ := step.Params["archive_format"].(string)
		if format != tc.Format {
			mismatches = append(mismatches, fmt.Sprintf("archive_format: got %s, want %s", format, tc.Format))
		}
	}

	// Check OS mapping has required keys
	osMapping := extractMapping(step.Params["os_mapping"])
	if osMapping != nil {
		expectedOSMapping := getOSMapping(expected)
		mismatches = append(mismatches, checkMappingKeys("os_mapping", osMapping, expectedOSMapping)...)
	} else if tc.Action == "github_archive" {
		mismatches = append(mismatches, fmt.Sprintf("missing os_mapping (raw type: %T)", step.Params["os_mapping"]))
	}

	// Check arch mapping has required keys
	archMapping := extractMapping(step.Params["arch_mapping"])
	if archMapping != nil {
		expectedArchMapping := getArchMapping(expected)
		mismatches = append(mismatches, checkMappingKeys("arch_mapping", archMapping, expectedArchMapping)...)
	}

	// Log comparison for debugging
	t.Logf("Generated asset_pattern: %v", step.Params["asset_pattern"])
	if len(expected.Steps) > 0 {
		t.Logf("Expected asset_pattern: %v", expected.Steps[0].Params["asset_pattern"])
	}

	return mismatches
}

// validateHomebrewSourceRecipe validates a Homebrew source build recipe and
// returns any mismatches found.
func validateHomebrewSourceRecipe(t *testing.T, tc llmTestCase, generated, expected *recipe.Recipe) []string {
	t.Helper()

	var mismatches []string

	if len(generated.Steps) == 0 {
		return []string{"generated recipe has no steps"}
	}

	// Check that first step matches expected action
	step := generated.Steps[0]
	if step.Action != tc.Action {
		mismatches = append(mismatches, fmt.Sprintf("action: got %s, want %s", step.Action, tc.Action))
	}

	// Check build system by looking for expected build action
	if tc.BuildSystem != "" {
		hasBuildAction := false
		expectedAction := ""
		switch tc.BuildSystem {
		case "autotools":
			expectedAction = "configure_make"
		case "cmake":
			expectedAction = "cmake"
		case "cargo":
			expectedAction = "cargo_build"
		case "go":
			expectedAction = "go_build"
		}
		for _, s := range generated.Steps {
			if s.Action == expectedAction {
				hasBuildAction = true
				break
			}
		}
		if expectedAction != "" && !hasBuildAction {
			t.Logf("Note: Expected %s action for %s build system not found in steps", expectedAction, tc.BuildSystem)
		}
	}

	// Check patches for source builds
	hasPatches := containsFeature(tc.Features, "patches:")
	if hasPatches {
		t.Logf("Checking patches for %s", tc.Tool)
		if len(generated.Patches) == 0 {
			mismatches = append(mismatches, "expected patches but generated recipe has none")
		} else {
			t.Logf("Generated recipe has %d patch(es)", len(generated.Patches))
			if containsFeature(tc.Features, "patches:url") {
				hasURLPatch := false
				for _, p := range generated.Patches {
					if p.URL != "" {
						hasURLPatch = true
					}
				}
				if !hasURLPatch {
					mismatches = append(mismatches, "expected URL patches but found none")
				}
			}
			if containsFeature(tc.Features, "patches:multiple") && len(generated.Patches) < 2 {
				mismatches = append(mismatches, fmt.Sprintf("expected multiple patches, got %d", len(generated.Patches)))
			}
		}
	}

	// Log comparison
	t.Logf("Generated recipe has %d steps", len(generated.Steps))
	if len(expected.Steps) > 0 {
		t.Logf("Expected recipe has %d steps", len(expected.Steps))
	}

	return mismatches
}

// containsFeature checks if a feature prefix is present in the features list
func containsFeature(features []string, prefix string) bool {
	for _, f := range features {
		if strings.HasPrefix(f, prefix) {
			return true
		}
	}
	return false
}

// loadRecipe loads a recipe from a TOML file
func loadRecipe(path string) (*recipe.Recipe, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var r recipe.Recipe
	if err := toml.Unmarshal(data, &r); err != nil {
		return nil, err
	}

	return &r, nil
}

// findRecipesDir locates the internal/recipe/recipes directory
func findRecipesDir(t *testing.T) string {
	t.Helper()

	// Start from current directory and look for the recipes dir.
	// When running from internal/builders/, "../../recipes" reaches the
	// public registry at the repo root where ground truth recipes live.
	candidates := []string{
		"../../recipes",
		"../recipe/recipes",
		"../../internal/recipe/recipes",
		"internal/recipe/recipes",
	}

	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			absPath, _ := filepath.Abs(candidate)
			return absPath
		}
	}

	// Try to find from GOPATH or module root
	cwd, _ := os.Getwd()
	t.Fatalf("Could not find recipes directory from %s", cwd)
	return ""
}

// getOSMapping extracts os_mapping from a recipe's first step
func getOSMapping(r *recipe.Recipe) map[string]interface{} {
	if len(r.Steps) == 0 {
		return nil
	}
	if m, ok := r.Steps[0].Params["os_mapping"].(map[string]interface{}); ok {
		return m
	}
	return nil
}

// getArchMapping extracts arch_mapping from a recipe's first step
func getArchMapping(r *recipe.Recipe) map[string]interface{} {
	if len(r.Steps) == 0 {
		return nil
	}
	if m, ok := r.Steps[0].Params["arch_mapping"].(map[string]interface{}); ok {
		return m
	}
	return nil
}

// extractMapping converts various map types to map[string]interface{}
func extractMapping(v interface{}) map[string]interface{} {
	if v == nil {
		return nil
	}

	// Try map[string]interface{} first
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}

	// Try map[string]string (common from TOML parsing)
	if m, ok := v.(map[string]string); ok {
		result := make(map[string]interface{})
		for k, val := range m {
			result[k] = val
		}
		return result
	}

	return nil
}

// checkMappingKeys verifies that the generated mapping contains the required
// keys and returns any missing keys as mismatch strings.
func checkMappingKeys(name string, generated, expected map[string]interface{}) []string {
	if expected == nil {
		return nil
	}

	var mismatches []string
	for key := range expected {
		if _, ok := generated[key]; !ok {
			mismatches = append(mismatches, fmt.Sprintf("%s missing key %q", name, key))
		}
	}
	return mismatches
}
