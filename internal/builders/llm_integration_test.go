package builders

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

// Ground truth test cases from Issue #283
// Each case has a tool name, GitHub repo, and the expected key fields
var groundTruthTests = []struct {
	name          string
	repo          string
	groundTruth   string // path relative to internal/recipe/recipes/
	wantAction    string
	wantFormat    string // archive_format for archives, empty for binaries
	wantStripDirs int    // -1 means don't check (varies by LLM)
}{
	{
		name:        "stern",
		repo:        "stern/stern",
		groundTruth: "s/stern.toml",
		wantAction:  "github_archive",
		wantFormat:  "tar.gz",
	},
	{
		name:        "ast-grep",
		repo:        "ast-grep/ast-grep",
		groundTruth: "a/ast-grep.toml",
		wantAction:  "github_archive",
		wantFormat:  "zip",
	},
	{
		name:        "trivy",
		repo:        "aquasecurity/trivy",
		groundTruth: "t/trivy.toml",
		wantAction:  "github_archive",
		wantFormat:  "tar.gz",
	},
	{
		name:        "age",
		repo:        "FiloSottile/age",
		groundTruth: "a/age.toml",
		wantAction:  "github_archive",
		wantFormat:  "tar.gz",
	},
	{
		name:        "k3d",
		repo:        "k3d-io/k3d",
		groundTruth: "k/k3d.toml",
		wantAction:  "github_file",
		wantFormat:  "",
	},
	{
		name:        "kind",
		repo:        "kubernetes-sigs/kind",
		groundTruth: "k/kind.toml",
		wantAction:  "github_file",
		wantFormat:  "",
	},
	{
		name:        "k9s",
		repo:        "derailed/k9s",
		groundTruth: "k/k9s.toml",
		wantAction:  "github_archive",
		wantFormat:  "tar.gz",
	},
	{
		name:        "tflint",
		repo:        "terraform-linters/tflint",
		groundTruth: "t/tflint.toml",
		wantAction:  "github_archive",
		wantFormat:  "zip",
	},
	{
		name:        "atlantis",
		repo:        "runatlantis/atlantis",
		groundTruth: "a/atlantis.toml",
		wantAction:  "github_archive",
		wantFormat:  "zip",
	},
}

// TestLLMGroundTruth validates LLM-generated recipes against ground truth.
// This test requires ANTHROPIC_API_KEY to be set and makes real API calls.
// It is skipped when the API key is not available.
func TestLLMGroundTruth(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("Skipping LLM integration test: ANTHROPIC_API_KEY not set")
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

	for _, tc := range groundTruthTests {
		tc := tc // capture range variable
		t.Run(tc.name, func(t *testing.T) {
			// Use a longer timeout for LLM calls (2 minutes)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()

			// Load ground truth recipe
			groundTruthPath := filepath.Join(recipesDir, tc.groundTruth)
			expected, err := loadRecipe(groundTruthPath)
			if err != nil {
				t.Fatalf("Failed to load ground truth recipe: %v", err)
			}

			// Generate recipe using LLM
			result, err := builder.Build(ctx, BuildRequest{
				Package:   tc.name,
				SourceArg: tc.repo,
			})
			if err != nil {
				t.Fatalf("LLM recipe generation failed: %v", err)
			}

			generated := result.Recipe

			// Save generated recipe for debugging
			outputPath := filepath.Join(outputDir, tc.name+".toml")
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
			if step.Action != tc.wantAction {
				t.Errorf("Action mismatch:\n  got:  %s\n  want: %s", step.Action, tc.wantAction)
			}

			// Check archive format if applicable
			if tc.wantFormat != "" {
				format, _ := step.Params["archive_format"].(string)
				if format != tc.wantFormat {
					t.Errorf("Archive format mismatch:\n  got:  %s\n  want: %s", format, tc.wantFormat)
				}
			}

			// Check OS mapping has required keys
			osMapping := extractMapping(step.Params["os_mapping"])
			if osMapping != nil {
				expectedOSMapping := getOSMapping(expected)
				checkMappingKeys(t, "os_mapping", osMapping, expectedOSMapping)
			} else if tc.wantAction == "github_archive" {
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
