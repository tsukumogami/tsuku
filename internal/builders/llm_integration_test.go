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
	Tool     string   `json:"tool"`
	Repo     string   `json:"repo"`
	Recipe   string   `json:"recipe"`
	Action   string   `json:"action"`
	Format   string   `json:"format"`
	Desc     string   `json:"desc"`
	Features []string `json:"features"`
}

// TestLLMGroundTruth validates LLM-generated recipes against ground truth.
// This test requires ANTHROPIC_API_KEY to be set and makes real API calls.
// It is skipped when the API key is not available.
//
// Test cases are defined in llm-test-matrix.json, with each test validating
// a specific variation to isolate failures.
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

	// Create the builder with default factory (auto-detects from env)
	ctx := context.Background()
	builder, err := NewGitHubReleaseBuilder(ctx)
	if err != nil {
		t.Fatalf("Failed to create GitHubReleaseBuilder: %v", err)
	}

	// Find the recipes directory (relative to test file)
	recipesDir := findRecipesDir(t)

	// Create output directory for generated recipes
	outputDir := filepath.Join(os.TempDir(), "tsuku-llm-test")
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		t.Fatalf("Failed to create output directory: %v", err)
	}
	t.Logf("Generated recipes will be saved to: %s", outputDir)

	// Run tests in order (L1, L2, ..., L18)
	testIDs := []string{
		"L1", "L2", "L3", "L4", "L5", "L6", "L7", "L8", "L9",
		"L10", "L11", "L12", "L13", "L14", "L15", "L16", "L17", "L18",
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

			// Generate recipe using LLM
			result, err := builder.Build(ctx, BuildRequest{
				Package:   tc.Tool,
				SourceArg: tc.Repo,
			})
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

			// Validate key fields
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
				// github_archive should have os_mapping
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
		})
	}
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
