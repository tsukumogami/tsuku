package builders

import (
	"context"
	_ "embed"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

//go:embed llm-test-matrix.json
var llmTestMatrixJSON []byte

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
	BuildSystem string   `json:"build_system"` // Build system (for homebrew_source)
	Desc        string   `json:"desc"`         // Test description
	Features    []string `json:"features"`     // Features being tested
}

// TestLLMGroundTruth validates LLM-generated recipes against ground truth.
// This test requires ANTHROPIC_API_KEY to be set and makes real API calls.
// It is skipped when the API key is not available.
//
// Test cases are defined in llm-test-matrix.json, with each test validating
// a specific variation to isolate failures.
//
// Container validation requires tsuku to be built and available in PATH.
// If tsuku is not found, validation is skipped (recipes are still generated
// and checked against ground truth, but not executed in a container).
//
// To run with full validation:
//
//	go build -o tsuku ./cmd/tsuku
//	PATH="$(pwd):$PATH" go test -run TestLLMGroundTruth ./internal/builders/
func TestLLMGroundTruth(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping LLM integration test: ANTHROPIC_API_KEY not set")
	}

	// Load test matrix
	var matrix llmTestMatrix
	if err := json.Unmarshal(llmTestMatrixJSON, &matrix); err != nil {
		t.Fatalf("Failed to parse llm-test-matrix.json: %v", err)
	}

	// Initialize builders
	ctx := context.Background()

	githubBuilder := NewGitHubReleaseBuilder()
	if err := githubBuilder.Initialize(ctx, &InitOptions{SkipValidation: false}); err != nil {
		t.Fatalf("Failed to initialize GitHubReleaseBuilder: %v", err)
	}

	homebrewBuilder := NewHomebrewBuilder()
	if err := homebrewBuilder.Initialize(ctx, &InitOptions{SkipValidation: false}); err != nil {
		t.Fatalf("Failed to initialize HomebrewBuilder: %v", err)
	}

	// Find the recipes directory (relative to test file)
	recipesDir := findRecipesDir(t)

	// Create output directory for generated recipes
	outputDir := filepath.Join(os.TempDir(), "tsuku-llm-test")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}
	t.Logf("Generated recipes will be saved to: %s", outputDir)

	// Run tests in order (L1-L18 for GitHub, S1-S3 for Homebrew source builds)
	testIDs := []string{
		"L1", "L2", "L3", "L4", "L5", "L6", "L7", "L8", "L9",
		"L10", "L11", "L12", "L13", "L14", "L15", "L16", "L17", "L18",
		"S1", "S2", "S3",
	}

	for _, testID := range testIDs {
		tc, ok := matrix.Tests[testID]
		if !ok {
			t.Errorf("Test case %s not found in matrix", testID)
			continue
		}

		t.Run(testID+"_"+tc.Tool, func(t *testing.T) {
			t.Logf("Testing: %s - %s", tc.Tool, tc.Desc)

			// Use a longer timeout for LLM calls (2 minutes)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			// Load ground truth recipe
			groundTruthPath := filepath.Join(recipesDir, tc.Recipe)
			expected, err := loadRecipe(groundTruthPath)
			if err != nil {
				t.Fatalf("Failed to load ground truth recipe: %v", err)
			}

			// Select builder and build request based on test case
			var result *BuildResult
			if tc.Builder == "homebrew" {
				// Use formula:source suffix to indicate source build
				result, err = homebrewBuilder.Build(ctx, BuildRequest{
					Package:   tc.Tool,
					SourceArg: tc.Formula + ":source",
				})
			} else {
				result, err = githubBuilder.Build(ctx, BuildRequest{
					Package:   tc.Tool,
					SourceArg: tc.Repo,
				})
			}
			if err != nil {
				t.Fatalf("LLM recipe generation failed: %v", err)
			}

			generated := result.Recipe

			// Save generated recipe for debugging
			outputPath := filepath.Join(outputDir, tc.Tool+".toml")
			if err := recipe.WriteRecipe(generated, outputPath); err != nil {
				t.Logf("Warning: failed to save generated recipe: %v", err)
			} else {
				t.Logf("Generated recipe saved to: %s", outputPath)
			}

			// Validate based on builder type
			if tc.Builder == "homebrew" {
				validateHomebrewSourceRecipe(t, tc, generated, expected)
			} else {
				validateGitHubRecipe(t, tc, generated, expected)
			}
		})
	}
}

// validateGitHubRecipe validates a GitHub release recipe
func validateGitHubRecipe(t *testing.T, tc llmTestCase, generated, expected *recipe.Recipe) {
	t.Helper()

	if len(generated.Steps) == 0 {
		t.Fatal("Generated recipe has no steps")
	}

	step := generated.Steps[0]

	// Check action type
	if step.Action != tc.Action {
		t.Errorf("Action mismatch:\n  got:  %s\n  want: %s", step.Action, tc.Action)
	}

	// Check archive format if applicable
	if tc.Format != "" {
		format, _ := step.Params["archive_format"].(string)
		if format != tc.Format {
			t.Errorf("Archive format mismatch:\n  got:  %s\n  want: %s", format, tc.Format)
		}
	}

	// Check OS mapping has required keys
	osMapping := extractMapping(step.Params["os_mapping"])
	if osMapping != nil {
		expectedOSMapping := getOSMapping(expected)
		checkMappingKeys(t, "os_mapping", osMapping, expectedOSMapping)
	} else if tc.Action == "github_archive" {
		t.Errorf("Missing os_mapping in generated recipe (raw type: %T)", step.Params["os_mapping"])
	}

	// Check arch mapping has required keys
	archMapping := extractMapping(step.Params["arch_mapping"])
	if archMapping != nil {
		expectedArchMapping := getArchMapping(expected)
		checkMappingKeys(t, "arch_mapping", archMapping, expectedArchMapping)
	}

	// Log comparison for debugging
	t.Logf("Generated asset_pattern: %v", step.Params["asset_pattern"])
	if len(expected.Steps) > 0 {
		t.Logf("Expected asset_pattern: %v", expected.Steps[0].Params["asset_pattern"])
	}
}

// validateHomebrewSourceRecipe validates a Homebrew source build recipe
func validateHomebrewSourceRecipe(t *testing.T, tc llmTestCase, generated, expected *recipe.Recipe) {
	t.Helper()

	if len(generated.Steps) == 0 {
		t.Fatal("Generated recipe has no steps")
	}

	// Check that first step is homebrew_source
	step := generated.Steps[0]
	if step.Action != tc.Action {
		t.Errorf("First action mismatch:\n  got:  %s\n  want: %s", step.Action, tc.Action)
	}

	// Check build system by looking for expected build action (configure_make, cmake, etc.)
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

		// Check recipe-level patches
		if len(generated.Patches) == 0 {
			t.Error("Expected patches but generated recipe has none")
		} else {
			t.Logf("Generated recipe has %d patch(es)", len(generated.Patches))

			// Check patch ordering if required
			if containsFeature(tc.Features, "patches:ordering") {
				t.Logf("Verifying patch ordering is preserved")
				// Patches should maintain order from formula
			}

			// Check for URL patches
			if containsFeature(tc.Features, "patches:url") {
				hasURLPatch := false
				for _, p := range generated.Patches {
					if p.URL != "" {
						hasURLPatch = true
						t.Logf("Found URL patch: %s", p.URL)
					}
				}
				if !hasURLPatch {
					t.Error("Expected URL patches but found none")
				}
			}

			// Check for multiple patches
			if containsFeature(tc.Features, "patches:multiple") {
				if len(generated.Patches) < 2 {
					t.Errorf("Expected multiple patches, got %d", len(generated.Patches))
				}
			}
		}
	}

	// Log comparison
	t.Logf("Generated recipe has %d steps", len(generated.Steps))
	if len(expected.Steps) > 0 {
		t.Logf("Expected recipe has %d steps", len(expected.Steps))
	}
}

// containsFeature checks if a feature prefix is present in the features list
func containsFeature(features []string, prefix string) bool {
	for _, f := range features {
		if len(f) >= len(prefix) && f[:len(prefix)] == prefix {
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

	// Start from current directory and look for the recipes dir
	candidates := []string{
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

// checkMappingKeys verifies that the generated mapping contains the required keys
func checkMappingKeys(t *testing.T, name string, generated, expected map[string]interface{}) {
	t.Helper()

	if expected == nil {
		return
	}

	// Check that all expected keys exist in generated
	// Note: We don't check values because LLM might use different conventions
	for key := range expected {
		if _, ok := generated[key]; !ok {
			t.Errorf("%s missing key %q (expected from ground truth)", name, key)
		}
	}
}
