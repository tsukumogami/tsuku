package recipe

import (
	"testing"

	"github.com/BurntSushi/toml"
)

func TestAnalyzeRecipeCoverage_UnconditionalSteps(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "test-tool"},
		Steps: []Step{
			{Action: "download_file"}, // No when clause - unconditional
		},
	}

	report := AnalyzeRecipeCoverage(r)

	if !report.HasGlibc {
		t.Error("expected HasGlibc=true for unconditional step")
	}
	if !report.HasMusl {
		t.Error("expected HasMusl=true for unconditional step")
	}
	if !report.HasDarwin {
		t.Error("expected HasDarwin=true for unconditional step")
	}
	if len(report.Errors) > 0 {
		t.Errorf("expected no errors, got %v", report.Errors)
	}
	if len(report.Warnings) > 0 {
		t.Errorf("expected no warnings, got %v", report.Warnings)
	}
}

func TestAnalyzeRecipeCoverage_GlibcOnlyStep(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "test-tool"},
		Steps: []Step{
			{
				Action: "homebrew",
				When:   &WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}},
			},
		},
	}

	report := AnalyzeRecipeCoverage(r)

	if !report.HasGlibc {
		t.Error("expected HasGlibc=true for glibc-only step")
	}
	if report.HasMusl {
		t.Error("expected HasMusl=false for glibc-only step")
	}
	if report.HasDarwin {
		t.Error("expected HasDarwin=false for glibc-only step")
	}
}

func TestAnalyzeRecipeCoverage_MuslOnlyStep(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "test-tool"},
		Steps: []Step{
			{
				Action: "apk_install",
				When:   &WhenClause{OS: []string{"linux"}, Libc: []string{"musl"}},
			},
		},
	}

	report := AnalyzeRecipeCoverage(r)

	if report.HasGlibc {
		t.Error("expected HasGlibc=false for musl-only step")
	}
	if !report.HasMusl {
		t.Error("expected HasMusl=true for musl-only step")
	}
	if report.HasDarwin {
		t.Error("expected HasDarwin=false for musl-only step")
	}
}

func TestAnalyzeRecipeCoverage_DarwinOnlyStep(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "test-tool"},
		Steps: []Step{
			{
				Action: "homebrew",
				When:   &WhenClause{OS: []string{"darwin"}},
			},
		},
	}

	report := AnalyzeRecipeCoverage(r)

	if report.HasGlibc {
		t.Error("expected HasGlibc=false for darwin-only step")
	}
	if report.HasMusl {
		t.Error("expected HasMusl=false for darwin-only step")
	}
	if !report.HasDarwin {
		t.Error("expected HasDarwin=true for darwin-only step")
	}
}

func TestAnalyzeRecipeCoverage_LibraryMissingMusl(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "bad-library", Type: "library"},
		Steps: []Step{
			{
				Action: "homebrew",
				When:   &WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}},
			},
		},
	}

	report := AnalyzeRecipeCoverage(r)

	if len(report.Errors) != 1 {
		t.Errorf("expected 1 error, got %d: %v", len(report.Errors), report.Errors)
	}
	if len(report.Errors) > 0 && report.Errors[0] == "" {
		t.Error("expected non-empty error message")
	}
}

func TestAnalyzeRecipeCoverage_LibraryWithConstraint(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name:          "constrained-library",
			Type:          "library",
			SupportedLibc: []string{"glibc"},
		},
		Steps: []Step{
			{
				Action: "homebrew",
				When:   &WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}},
			},
		},
	}

	report := AnalyzeRecipeCoverage(r)

	if len(report.Errors) > 0 {
		t.Errorf("expected no errors for library with explicit constraint, got %v", report.Errors)
	}
	if len(report.Warnings) > 0 {
		t.Errorf("expected no warnings for library with explicit constraint, got %v", report.Warnings)
	}
}

func TestAnalyzeRecipeCoverage_ToolWithLibraryDepsMissingMusl(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{
			Name:         "tool-with-deps",
			Type:         "tool",
			Dependencies: []string{"zlib"},
		},
		Steps: []Step{
			{
				Action: "download_file",
				When:   &WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}},
			},
		},
	}

	report := AnalyzeRecipeCoverage(r)

	if len(report.Errors) > 0 {
		t.Errorf("expected no errors for tool (only warnings), got %v", report.Errors)
	}
	if len(report.Warnings) != 1 {
		t.Errorf("expected 1 warning, got %d: %v", len(report.Warnings), report.Warnings)
	}
}

func TestAnalyzeRecipeCoverage_ToolWithoutDeps(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "standalone-tool"},
		Steps: []Step{
			{
				Action: "download_file",
				When:   &WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}},
			},
		},
	}

	report := AnalyzeRecipeCoverage(r)

	if len(report.Errors) > 0 {
		t.Errorf("expected no errors for tool without deps, got %v", report.Errors)
	}
	if len(report.Warnings) > 0 {
		t.Errorf("expected no warnings for tool without deps, got %v", report.Warnings)
	}
}

func TestAnalyzeRecipeCoverage_AllThreePaths(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "full-coverage", Type: "library"},
		Steps: []Step{
			{
				Action: "homebrew",
				When:   &WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}},
			},
			{
				Action: "apk_install",
				When:   &WhenClause{OS: []string{"linux"}, Libc: []string{"musl"}},
			},
			{
				Action: "homebrew",
				When:   &WhenClause{OS: []string{"darwin"}},
			},
		},
	}

	report := AnalyzeRecipeCoverage(r)

	if !report.HasGlibc {
		t.Error("expected HasGlibc=true")
	}
	if !report.HasMusl {
		t.Error("expected HasMusl=true")
	}
	if !report.HasDarwin {
		t.Error("expected HasDarwin=true")
	}
	if len(report.Errors) > 0 {
		t.Errorf("expected no errors, got %v", report.Errors)
	}
	if len(report.Warnings) > 0 {
		t.Errorf("expected no warnings, got %v", report.Warnings)
	}
}

func TestAnalyzeRecipeCoverage_StepLevelDependencies(t *testing.T) {
	r := &Recipe{
		Metadata: MetadataSection{Name: "tool-with-step-deps"},
		Steps: []Step{
			{
				Action:       "download_file",
				When:         &WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}},
				Dependencies: []string{"openssl"},
			},
		},
	}

	report := AnalyzeRecipeCoverage(r)

	// Should have warning because it has step-level dependencies
	if len(report.Warnings) != 1 {
		t.Errorf("expected 1 warning for tool with step-level deps, got %d: %v", len(report.Warnings), report.Warnings)
	}
}

func TestStepMatchesGlibc(t *testing.T) {
	tests := []struct {
		name     string
		when     *WhenClause
		expected bool
	}{
		{"nil when clause", nil, true},
		{"empty when clause", &WhenClause{}, true},
		{"explicit glibc", &WhenClause{Libc: []string{"glibc"}}, true},
		{"explicit musl only", &WhenClause{Libc: []string{"musl"}}, false},
		{"both libc", &WhenClause{Libc: []string{"glibc", "musl"}}, true},
		{"linux os", &WhenClause{OS: []string{"linux"}}, true},
		{"darwin os", &WhenClause{OS: []string{"darwin"}}, false},
		{"linux+darwin os", &WhenClause{OS: []string{"linux", "darwin"}}, true},
		{"linux platform", &WhenClause{Platform: []string{"linux/amd64"}}, true},
		{"darwin platform", &WhenClause{Platform: []string{"darwin/arm64"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stepMatchesGlibc(tt.when)
			if got != tt.expected {
				t.Errorf("stepMatchesGlibc() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStepMatchesMusl(t *testing.T) {
	tests := []struct {
		name     string
		when     *WhenClause
		expected bool
	}{
		{"nil when clause", nil, true},
		{"empty when clause", &WhenClause{}, true},
		{"explicit musl", &WhenClause{Libc: []string{"musl"}}, true},
		{"explicit glibc only", &WhenClause{Libc: []string{"glibc"}}, false},
		{"both libc", &WhenClause{Libc: []string{"glibc", "musl"}}, true},
		{"linux os", &WhenClause{OS: []string{"linux"}}, true},
		{"darwin os", &WhenClause{OS: []string{"darwin"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stepMatchesMusl(tt.when)
			if got != tt.expected {
				t.Errorf("stepMatchesMusl() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestStepMatchesDarwin(t *testing.T) {
	tests := []struct {
		name     string
		when     *WhenClause
		expected bool
	}{
		{"nil when clause", nil, true},
		{"empty when clause", &WhenClause{}, true},
		{"darwin os", &WhenClause{OS: []string{"darwin"}}, true},
		{"linux os", &WhenClause{OS: []string{"linux"}}, false},
		{"linux+darwin os", &WhenClause{OS: []string{"linux", "darwin"}}, true},
		{"libc specified (linux-only)", &WhenClause{Libc: []string{"glibc"}}, false},
		{"darwin platform", &WhenClause{Platform: []string{"darwin/arm64"}}, true},
		{"linux platform", &WhenClause{Platform: []string{"linux/amd64"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stepMatchesDarwin(tt.when)
			if got != tt.expected {
				t.Errorf("stepMatchesDarwin() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHasLibraryDependencies(t *testing.T) {
	tests := []struct {
		name     string
		recipe   *Recipe
		expected bool
	}{
		{
			name: "no dependencies",
			recipe: &Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps:    []Step{{Action: "download_file"}},
			},
			expected: false,
		},
		{
			name: "recipe-level dependencies",
			recipe: &Recipe{
				Metadata: MetadataSection{Name: "test", Dependencies: []string{"zlib"}},
				Steps:    []Step{{Action: "download_file"}},
			},
			expected: true,
		},
		{
			name: "step-level dependencies",
			recipe: &Recipe{
				Metadata: MetadataSection{Name: "test"},
				Steps:    []Step{{Action: "download_file", Dependencies: []string{"openssl"}}},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := hasLibraryDependencies(tt.recipe)
			if got != tt.expected {
				t.Errorf("hasLibraryDependencies() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestValidateCoverageForRecipes(t *testing.T) {
	recipes := []*Recipe{
		// Good recipe - has full coverage
		{
			Metadata: MetadataSection{Name: "good", Type: "library"},
			Steps: []Step{
				{Action: "download_file"}, // Unconditional
			},
		},
		// Bad library - missing musl
		{
			Metadata: MetadataSection{Name: "bad-lib", Type: "library"},
			Steps: []Step{
				{Action: "homebrew", When: &WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}}},
			},
		},
		// Tool with deps but missing musl - warning
		{
			Metadata: MetadataSection{Name: "tool", Dependencies: []string{"zlib"}},
			Steps: []Step{
				{Action: "download_file", When: &WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}}},
			},
		},
	}

	reports := ValidateCoverageForRecipes(recipes)

	// Should have 2 reports: bad-lib (error) and tool (warning)
	if len(reports) != 2 {
		t.Errorf("expected 2 reports with issues, got %d", len(reports))
	}

	// Check that bad-lib has error
	var badLibReport *CoverageReport
	for i := range reports {
		if reports[i].Recipe == "bad-lib" {
			badLibReport = &reports[i]
			break
		}
	}
	if badLibReport == nil {
		t.Error("expected report for bad-lib")
	} else if len(badLibReport.Errors) != 1 {
		t.Errorf("expected 1 error for bad-lib, got %d", len(badLibReport.Errors))
	}
}

// TestTransitiveDepsHavePlatformCoverage verifies that all embedded library recipes
// and their transitive dependencies have proper platform coverage.
func TestTransitiveDepsHavePlatformCoverage(t *testing.T) {
	registry, err := NewEmbeddedRegistry()
	if err != nil {
		t.Fatalf("failed to create embedded registry: %v", err)
	}

	// Parse all recipes into a map for dependency lookup
	recipes := make(map[string]*Recipe)
	for _, name := range registry.List() {
		data, ok := registry.Get(name)
		if !ok {
			continue
		}
		var r Recipe
		if err := toml.Unmarshal(data, &r); err != nil {
			t.Errorf("failed to parse recipe %s: %v", name, err)
			continue
		}
		recipes[name] = &r
	}

	// Check each library recipe and its transitive dependencies
	for name, r := range recipes {
		if !r.IsLibrary() {
			continue
		}

		// Analyze coverage for the library itself
		report := AnalyzeRecipeCoverage(r)
		if len(report.Errors) > 0 {
			t.Errorf("library %s has coverage errors: %v", name, report.Errors)
		}

		// Walk transitive dependencies and check their coverage
		visited := make(map[string]bool)
		checkTransitiveDeps(t, name, r, recipes, visited)
	}
}

// checkTransitiveDeps recursively checks that all dependencies have proper platform coverage.
func checkTransitiveDeps(t *testing.T, rootName string, r *Recipe, recipes map[string]*Recipe, visited map[string]bool) {
	t.Helper()

	// Collect all dependencies (recipe-level and step-level)
	deps := make(map[string]bool)
	for _, d := range r.Metadata.Dependencies {
		deps[d] = true
	}
	for _, step := range r.Steps {
		for _, d := range step.Dependencies {
			deps[d] = true
		}
	}

	// Check each dependency
	for depName := range deps {
		if visited[depName] {
			continue
		}
		visited[depName] = true

		depRecipe, ok := recipes[depName]
		if !ok {
			// Dependency not in embedded registry - skip
			// (it may be an external or system dependency)
			continue
		}

		report := AnalyzeRecipeCoverage(depRecipe)
		if len(report.Errors) > 0 {
			t.Errorf("dependency %s (of %s) has coverage errors: %v", depName, rootName, report.Errors)
		}

		// Recurse into the dependency's dependencies
		checkTransitiveDeps(t, rootName, depRecipe, recipes, visited)
	}
}
