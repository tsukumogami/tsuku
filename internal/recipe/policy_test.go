package recipe

import (
	"sort"
	"testing"
)

func TestRecipeFamilyPolicy_String(t *testing.T) {
	tests := []struct {
		policy RecipeFamilyPolicy
		want   string
	}{
		{FamilyNone, "FamilyNone"},
		{FamilyAgnostic, "FamilyAgnostic"},
		{FamilyVarying, "FamilyVarying"},
		{FamilySpecific, "FamilySpecific"},
		{FamilyMixed, "FamilyMixed"},
		{RecipeFamilyPolicy(99), "RecipeFamilyPolicy(99)"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := tt.policy.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Helper to create a recipe with steps that have pre-set analysis
func makeRecipeWithAnalysis(steps []stepWithAnalysis) *Recipe {
	recipe := &Recipe{
		Metadata: MetadataSection{Name: "test"},
		Verify:   VerifySection{Command: "test --version"},
	}
	for _, s := range steps {
		step := Step{
			Action: s.action,
			When:   s.when,
			Params: s.params,
		}
		step.SetAnalysis(s.analysis)
		recipe.Steps = append(recipe.Steps, step)
	}
	return recipe
}

type stepWithAnalysis struct {
	action   string
	when     *WhenClause
	params   map[string]interface{}
	analysis *StepAnalysis
}

func TestAnalyzeRecipe_FamilyNone(t *testing.T) {
	// Darwin-only recipe (when.os: darwin)
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "download",
			analysis: &StepAnalysis{Constraint: &Constraint{OS: "darwin"}},
		},
	})

	analysis := AnalyzeRecipe(recipe)

	if analysis.Policy != FamilyNone {
		t.Errorf("expected FamilyNone, got %s", analysis.Policy)
	}
	if !analysis.SupportsDarwin {
		t.Error("expected SupportsDarwin=true")
	}
	if len(analysis.FamiliesUsed) != 0 {
		t.Errorf("expected empty FamiliesUsed, got %v", analysis.FamiliesUsed)
	}
}

func TestAnalyzeRecipe_FamilyAgnostic(t *testing.T) {
	// Recipe with download action, no family refs
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "download",
			analysis: &StepAnalysis{Constraint: nil, FamilyVarying: false},
		},
	})

	analysis := AnalyzeRecipe(recipe)

	if analysis.Policy != FamilyAgnostic {
		t.Errorf("expected FamilyAgnostic, got %s", analysis.Policy)
	}
	if !analysis.SupportsDarwin {
		t.Error("expected SupportsDarwin=true for unconstrained step")
	}
}

func TestAnalyzeRecipe_FamilyVarying(t *testing.T) {
	// Recipe with {{linux_family}} in URL template
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action: "download",
			params: map[string]interface{}{
				"url": "https://example.com/{{linux_family}}/tool.tar.gz",
			},
			analysis: &StepAnalysis{Constraint: nil, FamilyVarying: true},
		},
	})

	analysis := AnalyzeRecipe(recipe)

	if analysis.Policy != FamilyVarying {
		t.Errorf("expected FamilyVarying, got %s", analysis.Policy)
	}
}

func TestAnalyzeRecipe_FamilySpecific_Single(t *testing.T) {
	// Recipe with only apt_install
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "apt_install",
			analysis: &StepAnalysis{Constraint: &Constraint{OS: "linux", LinuxFamily: "debian"}},
		},
	})

	analysis := AnalyzeRecipe(recipe)

	if analysis.Policy != FamilySpecific {
		t.Errorf("expected FamilySpecific, got %s", analysis.Policy)
	}
	if !analysis.FamiliesUsed["debian"] {
		t.Error("expected debian in FamiliesUsed")
	}
	if len(analysis.FamiliesUsed) != 1 {
		t.Errorf("expected 1 family, got %d", len(analysis.FamiliesUsed))
	}
	// apt_install is linux-only, so no darwin
	if analysis.SupportsDarwin {
		t.Error("expected SupportsDarwin=false for linux-only step")
	}
}

func TestAnalyzeRecipe_FamilySpecific_Multi(t *testing.T) {
	// Recipe with apt_install + dnf_install
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "apt_install",
			analysis: &StepAnalysis{Constraint: &Constraint{OS: "linux", LinuxFamily: "debian"}},
		},
		{
			action:   "dnf_install",
			analysis: &StepAnalysis{Constraint: &Constraint{OS: "linux", LinuxFamily: "rhel"}},
		},
	})

	analysis := AnalyzeRecipe(recipe)

	if analysis.Policy != FamilySpecific {
		t.Errorf("expected FamilySpecific, got %s", analysis.Policy)
	}
	if !analysis.FamiliesUsed["debian"] {
		t.Error("expected debian in FamiliesUsed")
	}
	if !analysis.FamiliesUsed["rhel"] {
		t.Error("expected rhel in FamiliesUsed")
	}
	if len(analysis.FamiliesUsed) != 2 {
		t.Errorf("expected 2 families, got %d", len(analysis.FamiliesUsed))
	}
}

func TestAnalyzeRecipe_FamilyMixed(t *testing.T) {
	// Recipe with download + apt_install
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "download",
			analysis: &StepAnalysis{Constraint: nil, FamilyVarying: false},
		},
		{
			action:   "apt_install",
			analysis: &StepAnalysis{Constraint: &Constraint{OS: "linux", LinuxFamily: "debian"}},
		},
	})

	analysis := AnalyzeRecipe(recipe)

	if analysis.Policy != FamilyMixed {
		t.Errorf("expected FamilyMixed, got %s", analysis.Policy)
	}
	if !analysis.FamiliesUsed["debian"] {
		t.Error("expected debian in FamiliesUsed")
	}
}

func TestAnalyzeRecipe_ConstrainedPlusVarying(t *testing.T) {
	// Step with both family constraint AND {{linux_family}} interpolation
	// This step only runs on debian but uses interpolation
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action: "download",
			params: map[string]interface{}{
				"url": "https://example.com/{{linux_family}}-tool.tar.gz",
			},
			analysis: &StepAnalysis{
				Constraint:    &Constraint{LinuxFamily: "debian"},
				FamilyVarying: true,
			},
		},
	})

	analysis := AnalyzeRecipe(recipe)

	// Should be FamilySpecific because the interpolation is constrained to debian
	if analysis.Policy != FamilySpecific {
		t.Errorf("expected FamilySpecific for constrained+varying, got %s", analysis.Policy)
	}
	if !analysis.FamiliesUsed["debian"] {
		t.Error("expected debian in FamiliesUsed")
	}
	if len(analysis.FamiliesUsed) != 1 {
		t.Errorf("expected 1 family (not all families), got %d", len(analysis.FamiliesUsed))
	}
}

func TestAnalyzeRecipe_LinuxOnlyStep(t *testing.T) {
	// Step with when.os: linux (no family constraint)
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "download",
			analysis: &StepAnalysis{Constraint: &Constraint{OS: "linux"}},
		},
	})

	analysis := AnalyzeRecipe(recipe)

	if analysis.Policy != FamilyAgnostic {
		t.Errorf("expected FamilyAgnostic, got %s", analysis.Policy)
	}
	if analysis.SupportsDarwin {
		t.Error("expected SupportsDarwin=false for linux-only step")
	}
}

func TestAnalyzeRecipe_NilAnalysis(t *testing.T) {
	// Recipe with step that has nil analysis (backward compat)
	recipe := &Recipe{
		Metadata: MetadataSection{Name: "test"},
		Steps: []Step{
			{Action: "download"},
		},
		Verify: VerifySection{Command: "test --version"},
	}

	analysis := AnalyzeRecipe(recipe)

	// Should treat as unconstrained
	if analysis.Policy != FamilyAgnostic {
		t.Errorf("expected FamilyAgnostic for nil analysis, got %s", analysis.Policy)
	}
	if !analysis.SupportsDarwin {
		t.Error("expected SupportsDarwin=true for nil analysis")
	}
}

func TestSupportedPlatforms_FamilyNone(t *testing.T) {
	// Darwin-only recipe
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "brew_install",
			analysis: &StepAnalysis{Constraint: &Constraint{OS: "darwin"}},
		},
	})

	platforms := SupportedPlatforms(recipe)

	// Should only have darwin platforms
	if len(platforms) != 2 {
		t.Errorf("expected 2 platforms, got %d", len(platforms))
	}
	for _, p := range platforms {
		if p.OS != "darwin" {
			t.Errorf("expected only darwin platforms, got %s", p.OS)
		}
		if p.LinuxFamily != "" {
			t.Errorf("expected no LinuxFamily for darwin, got %s", p.LinuxFamily)
		}
	}
}

func TestSupportedPlatforms_FamilyAgnostic(t *testing.T) {
	// Generic recipe
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "download",
			analysis: &StepAnalysis{Constraint: nil},
		},
	})

	platforms := SupportedPlatforms(recipe)

	// Should have darwin + generic linux (4 total)
	if len(platforms) != 4 {
		t.Errorf("expected 4 platforms, got %d", len(platforms))
	}

	// Check no linux_family is set
	for _, p := range platforms {
		if p.OS == "linux" && p.LinuxFamily != "" {
			t.Errorf("expected no LinuxFamily for FamilyAgnostic, got %s", p.LinuxFamily)
		}
	}
}

func TestSupportedPlatforms_FamilyVarying(t *testing.T) {
	// Recipe with {{linux_family}} interpolation
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "download",
			analysis: &StepAnalysis{Constraint: nil, FamilyVarying: true},
		},
	})

	platforms := SupportedPlatforms(recipe)

	// Should have darwin (2) + all linux families (5 * 2 = 10) = 12
	expectedCount := 2 + len(AllLinuxFamilies)*2
	if len(platforms) != expectedCount {
		t.Errorf("expected %d platforms, got %d", expectedCount, len(platforms))
	}

	// Verify all families are present
	familiesFound := make(map[string]bool)
	for _, p := range platforms {
		if p.OS == "linux" && p.LinuxFamily != "" {
			familiesFound[p.LinuxFamily] = true
		}
	}
	for _, family := range AllLinuxFamilies {
		if !familiesFound[family] {
			t.Errorf("expected family %s to be present", family)
		}
	}
}

func TestSupportedPlatforms_FamilySpecific(t *testing.T) {
	// apt_install only
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "apt_install",
			analysis: &StepAnalysis{Constraint: &Constraint{OS: "linux", LinuxFamily: "debian"}},
		},
	})

	platforms := SupportedPlatforms(recipe)

	// Should have only debian linux (2 arches) = 2
	if len(platforms) != 2 {
		t.Errorf("expected 2 platforms, got %d", len(platforms))
	}

	for _, p := range platforms {
		if p.OS != "linux" {
			t.Errorf("expected only linux platforms, got %s", p.OS)
		}
		if p.LinuxFamily != "debian" {
			t.Errorf("expected LinuxFamily=debian, got %s", p.LinuxFamily)
		}
	}
}

func TestSupportedPlatforms_FamilyMixed(t *testing.T) {
	// download + apt_install
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "download",
			analysis: &StepAnalysis{Constraint: nil},
		},
		{
			action:   "apt_install",
			analysis: &StepAnalysis{Constraint: &Constraint{OS: "linux", LinuxFamily: "debian"}},
		},
	})

	platforms := SupportedPlatforms(recipe)

	// Should have darwin (2) + all linux families (5 * 2 = 10) = 12
	expectedCount := 2 + len(AllLinuxFamilies)*2
	if len(platforms) != expectedCount {
		t.Errorf("expected %d platforms, got %d", expectedCount, len(platforms))
	}
}

func TestSupportedPlatforms_Architectures(t *testing.T) {
	// Verify both amd64 and arm64 are included
	recipe := makeRecipeWithAnalysis([]stepWithAnalysis{
		{
			action:   "download",
			analysis: &StepAnalysis{Constraint: nil},
		},
	})

	platforms := SupportedPlatforms(recipe)

	archCounts := make(map[string]int)
	for _, p := range platforms {
		archCounts[p.Arch]++
	}

	if archCounts["amd64"] == 0 {
		t.Error("expected amd64 architecture")
	}
	if archCounts["arm64"] == 0 {
		t.Error("expected arm64 architecture")
	}
}

func TestAllLinuxFamilies(t *testing.T) {
	// Verify the expected families are present
	expected := []string{"debian", "rhel", "arch", "alpine", "suse"}

	if len(AllLinuxFamilies) != len(expected) {
		t.Errorf("expected %d families, got %d", len(expected), len(AllLinuxFamilies))
	}

	// Sort for comparison
	sortedExpected := make([]string, len(expected))
	copy(sortedExpected, expected)
	sort.Strings(sortedExpected)

	sortedActual := make([]string, len(AllLinuxFamilies))
	copy(sortedActual, AllLinuxFamilies)
	sort.Strings(sortedActual)

	for i := range sortedExpected {
		if sortedExpected[i] != sortedActual[i] {
			t.Errorf("family mismatch at %d: expected %s, got %s", i, sortedExpected[i], sortedActual[i])
		}
	}
}

func TestAnalyzeRecipe_EmptyRecipe(t *testing.T) {
	// Recipe with no steps
	recipe := &Recipe{
		Metadata: MetadataSection{Name: "empty"},
		Steps:    []Step{},
		Verify:   VerifySection{Command: "test --version"},
	}

	analysis := AnalyzeRecipe(recipe)

	// Empty recipe has no Linux or Darwin steps
	if analysis.Policy != FamilyNone {
		t.Errorf("expected FamilyNone for empty recipe, got %s", analysis.Policy)
	}
	if analysis.SupportsDarwin {
		t.Error("expected SupportsDarwin=false for empty recipe")
	}
}

func TestSupportedPlatforms_EmptyRecipe(t *testing.T) {
	recipe := &Recipe{
		Metadata: MetadataSection{Name: "empty"},
		Steps:    []Step{},
		Verify:   VerifySection{Command: "test --version"},
	}

	platforms := SupportedPlatforms(recipe)

	if len(platforms) != 0 {
		t.Errorf("expected 0 platforms for empty recipe, got %d", len(platforms))
	}
}
