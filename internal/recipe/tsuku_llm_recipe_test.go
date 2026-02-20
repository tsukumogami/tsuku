package recipe

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
)

// recipeRelPath is the path to the tsuku-llm recipe relative to the repo root.
const recipeRelPath = "recipes/t/tsuku-llm.toml"

// findRepoRoot walks up from the current file's directory to find the repo root
// (identified by go.mod). Returns an error if not found.
func findRepoRoot() (string, error) {
	_, filename, _, _ := runtime.Caller(0)
	dir := filepath.Dir(filename)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}

// loadTsukuLLMRecipe loads and parses the tsuku-llm recipe from the recipes directory.
func loadTsukuLLMRecipe(t *testing.T) *Recipe {
	t.Helper()
	root, err := findRepoRoot()
	if err != nil {
		t.Fatalf("failed to find repo root: %v", err)
	}
	path := filepath.Join(root, recipeRelPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read recipe: %v", err)
	}
	var r Recipe
	if err := toml.Unmarshal(data, &r); err != nil {
		t.Fatalf("failed to parse recipe TOML: %v", err)
	}
	return &r
}

// pipelineBuildMatrix returns the artifact suffixes produced by the release pipeline.
// Sourced from .github/workflows/llm-release.yml build matrix.
//
// Artifact naming: tsuku-llm-v{version}-{suffix}
// macOS builds omit the backend because Metal is the only backend per architecture.
func pipelineBuildMatrix() []string {
	return []string{
		"darwin-arm64",
		"darwin-amd64",
		"linux-amd64-cuda",
		"linux-amd64-vulkan",
		"linux-amd64-cpu",
		"linux-arm64-cuda",
		"linux-arm64-vulkan",
		"linux-arm64-cpu",
	}
}

// TestTsukuLLMRecipeAssetPatternsMatchPipeline validates that every asset_pattern
// in the recipe corresponds 1:1 with an artifact in the release pipeline's build matrix.
func TestTsukuLLMRecipeAssetPatternsMatchPipeline(t *testing.T) {
	r := loadTsukuLLMRecipe(t)

	// Extract asset_pattern suffixes from recipe steps.
	// Each pattern is "tsuku-llm-v{version}-<suffix>".
	prefix := "tsuku-llm-v{version}-"
	var recipeSuffixes []string
	for i, step := range r.Steps {
		pattern, ok := step.Params["asset_pattern"].(string)
		if !ok {
			t.Fatalf("step %d: missing or invalid asset_pattern", i)
		}
		if !strings.HasPrefix(pattern, prefix) {
			t.Errorf("step %d: asset_pattern %q does not start with %q", i, pattern, prefix)
			continue
		}
		suffix := strings.TrimPrefix(pattern, prefix)
		recipeSuffixes = append(recipeSuffixes, suffix)
	}

	pipelineSuffixes := pipelineBuildMatrix()

	// Sort both for stable comparison.
	sort.Strings(recipeSuffixes)
	sort.Strings(pipelineSuffixes)

	// Check 1:1 correspondence.
	if len(recipeSuffixes) != len(pipelineSuffixes) {
		t.Errorf("recipe has %d asset patterns but pipeline produces %d artifacts\n  recipe:   %v\n  pipeline: %v",
			len(recipeSuffixes), len(pipelineSuffixes), recipeSuffixes, pipelineSuffixes)
		return
	}

	for i := range recipeSuffixes {
		if recipeSuffixes[i] != pipelineSuffixes[i] {
			t.Errorf("mismatch at index %d: recipe has %q, pipeline has %q",
				i, recipeSuffixes[i], pipelineSuffixes[i])
		}
	}
}

// TestTsukuLLMRecipePlatformConstraints validates that ValidatePlatformConstraints
// passes with no errors on the recipe's metadata.
func TestTsukuLLMRecipePlatformConstraints(t *testing.T) {
	r := loadTsukuLLMRecipe(t)

	warnings, err := r.ValidatePlatformConstraints()
	if err != nil {
		t.Errorf("ValidatePlatformConstraints() returned error: %v", err)
	}
	for _, w := range warnings {
		t.Errorf("unexpected warning: %s", w.Message)
	}
}

// TestTsukuLLMRecipePlatformMetadata verifies the recipe declares the correct
// platform support metadata.
func TestTsukuLLMRecipePlatformMetadata(t *testing.T) {
	r := loadTsukuLLMRecipe(t)

	// supported_os
	expectedOS := []string{"linux", "darwin"}
	if len(r.Metadata.SupportedOS) != len(expectedOS) {
		t.Errorf("supported_os = %v, want %v", r.Metadata.SupportedOS, expectedOS)
	} else {
		for i, os := range expectedOS {
			if r.Metadata.SupportedOS[i] != os {
				t.Errorf("supported_os[%d] = %q, want %q", i, r.Metadata.SupportedOS[i], os)
			}
		}
	}

	// supported_arch
	expectedArch := []string{"amd64", "arm64"}
	if len(r.Metadata.SupportedArch) != len(expectedArch) {
		t.Errorf("supported_arch = %v, want %v", r.Metadata.SupportedArch, expectedArch)
	} else {
		for i, arch := range expectedArch {
			if r.Metadata.SupportedArch[i] != arch {
				t.Errorf("supported_arch[%d] = %q, want %q", i, r.Metadata.SupportedArch[i], arch)
			}
		}
	}

	// supported_libc
	expectedLibc := []string{"glibc"}
	if len(r.Metadata.SupportedLibc) != len(expectedLibc) {
		t.Errorf("supported_libc = %v, want %v", r.Metadata.SupportedLibc, expectedLibc)
	} else {
		for i, libc := range expectedLibc {
			if r.Metadata.SupportedLibc[i] != libc {
				t.Errorf("supported_libc[%d] = %q, want %q", i, r.Metadata.SupportedLibc[i], libc)
			}
		}
	}

	// unsupported_reason must be non-empty
	if r.Metadata.UnsupportedReason == "" {
		t.Error("unsupported_reason should be set")
	}
}

// TestTsukuLLMRecipeUnsupportedPlatformError validates that the error message for
// an unsupported platform contains useful information for the user.
func TestTsukuLLMRecipeUnsupportedPlatformError(t *testing.T) {
	r := loadTsukuLLMRecipe(t)

	tests := []struct {
		name               string
		currentOS          string
		currentArch        string
		currentLibc        string
		expectedSubstrings []string
	}{
		{
			name:        "musl/Alpine Linux is unsupported",
			currentOS:   "linux",
			currentArch: "amd64",
			currentLibc: "musl",
			expectedSubstrings: []string{
				"tsuku-llm",
				"linux/amd64",
				"musl",
				"Libc: glibc",
			},
		},
		{
			name:        "Windows is unsupported",
			currentOS:   "windows",
			currentArch: "amd64",
			currentLibc: "",
			expectedSubstrings: []string{
				"tsuku-llm",
				"windows/amd64",
				"linux, darwin",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := &UnsupportedPlatformError{
				RecipeName:        r.Metadata.Name,
				CurrentOS:         tt.currentOS,
				CurrentArch:       tt.currentArch,
				CurrentLibc:       tt.currentLibc,
				SupportedOS:       r.Metadata.SupportedOS,
				SupportedArch:     r.Metadata.SupportedArch,
				SupportedLibc:     r.Metadata.SupportedLibc,
				UnsupportedReason: r.Metadata.UnsupportedReason,
			}

			errMsg := err.Error()

			for _, substr := range tt.expectedSubstrings {
				if !contains(errMsg, substr) {
					t.Errorf("expected substring %q in error message:\n%s", substr, errMsg)
				}
			}
		})
	}
}

// TestTsukuLLMRecipeMuslNotSupported verifies that the recipe rejects musl platforms
// via SupportsPlatformWithLibc.
func TestTsukuLLMRecipeMuslNotSupported(t *testing.T) {
	r := loadTsukuLLMRecipe(t)

	// musl should be rejected
	if r.SupportsPlatformWithLibc("linux", "amd64", "musl") {
		t.Error("recipe should not support musl, but SupportsPlatformWithLibc returned true")
	}

	// glibc should be accepted
	if !r.SupportsPlatformWithLibc("linux", "amd64", "glibc") {
		t.Error("recipe should support glibc, but SupportsPlatformWithLibc returned false")
	}
}

// TestTsukuLLMRecipeStepCoverage validates that every supported platform/GPU
// combination has exactly one matching step.
func TestTsukuLLMRecipeStepCoverage(t *testing.T) {
	r := loadTsukuLLMRecipe(t)

	// Define every expected target configuration.
	// Each one should match exactly one step.
	type targetConfig struct {
		name string
		os   string
		arch string
		gpu  string
	}

	targets := []targetConfig{
		{"darwin-arm64", "darwin", "arm64", "apple"},
		{"darwin-amd64", "darwin", "amd64", "apple"},
		{"linux-amd64-nvidia", "linux", "amd64", "nvidia"},
		{"linux-amd64-amd", "linux", "amd64", "amd"},
		{"linux-amd64-intel", "linux", "amd64", "intel"},
		{"linux-amd64-none", "linux", "amd64", "none"},
		{"linux-arm64-nvidia", "linux", "arm64", "nvidia"},
		{"linux-arm64-amd", "linux", "arm64", "amd"},
		{"linux-arm64-intel", "linux", "arm64", "intel"},
		{"linux-arm64-none", "linux", "arm64", "none"},
	}

	for _, tc := range targets {
		t.Run(tc.name, func(t *testing.T) {
			target := NewMatchTarget(tc.os, tc.arch, "", "glibc", tc.gpu)
			matchCount := 0
			for _, step := range r.Steps {
				if step.When == nil || step.When.Matches(target) {
					matchCount++
				}
			}
			if matchCount == 0 {
				t.Errorf("no step matches target %s (os=%s, arch=%s, gpu=%s)",
					tc.name, tc.os, tc.arch, tc.gpu)
			}
			if matchCount > 1 {
				t.Errorf("%d steps match target %s (os=%s, arch=%s, gpu=%s); expected exactly 1",
					matchCount, tc.name, tc.os, tc.arch, tc.gpu)
			}
		})
	}
}

// TestTsukuLLMRecipeUnsupportedReason validates that the unsupported_reason
// mentions key information about why certain platforms are excluded.
func TestTsukuLLMRecipeUnsupportedReason(t *testing.T) {
	r := loadTsukuLLMRecipe(t)

	reason := r.Metadata.UnsupportedReason

	expectedSubstrings := []string{
		"glibc",
		"musl",
	}

	for _, substr := range expectedSubstrings {
		if !contains(reason, substr) {
			t.Errorf("unsupported_reason should mention %q, got: %s", substr, reason)
		}
	}
}
