package recipe

import "fmt"

// AllLinuxFamilies lists all supported Linux distribution families.
// Used when a recipe requires files for all families (FamilyVarying, FamilyMixed).
var AllLinuxFamilies = []string{"debian", "rhel", "arch", "alpine", "suse"}

// RecipeFamilyPolicy describes how a recipe relates to Linux families.
// This determines which platform+family combinations need golden files.
type RecipeFamilyPolicy int

const (
	// FamilyNone: No Linux-applicable steps exist, so no family policy applies.
	// Result: No Linux platforms at all (Darwin-only recipe)
	FamilyNone RecipeFamilyPolicy = iota

	// FamilyAgnostic: Has Linux steps, but no family constraints or variation.
	// Result: Generic Linux platforms (no family qualifier in golden files)
	FamilyAgnostic

	// FamilyVarying: At least one step uses {{linux_family}} interpolation.
	// Result: All families (each produces different output)
	FamilyVarying

	// FamilySpecific: All Linux steps target specific families, no unconstrained steps.
	// Result: Only the families explicitly targeted
	FamilySpecific

	// FamilyMixed: Has both family-constrained and unconstrained Linux steps.
	// Result: All families (some steps filtered per family)
	FamilyMixed
)

// String returns the policy name for debugging and logging.
func (p RecipeFamilyPolicy) String() string {
	switch p {
	case FamilyNone:
		return "FamilyNone"
	case FamilyAgnostic:
		return "FamilyAgnostic"
	case FamilyVarying:
		return "FamilyVarying"
	case FamilySpecific:
		return "FamilySpecific"
	case FamilyMixed:
		return "FamilyMixed"
	default:
		return fmt.Sprintf("RecipeFamilyPolicy(%d)", p)
	}
}

// RecipeAnalysis contains the full analysis of a recipe's platform support.
// Returned by AnalyzeRecipe; used by SupportedPlatforms.
type RecipeAnalysis struct {
	Policy         RecipeFamilyPolicy
	FamiliesUsed   map[string]bool // For FamilySpecific/FamilyMixed - tracks which families are used
	SupportsDarwin bool            // Derived from step analysis, not hardcoded
}

// Platform represents a supported platform for a recipe.
// Used in the supported_platforms output from tsuku info.
type Platform struct {
	OS          string `json:"os"`
	Arch        string `json:"arch"`
	LinuxFamily string `json:"linux_family,omitempty"` // Only set for family-aware recipes
}

// AnalyzeRecipe computes the family policy and OS support for a recipe.
// It iterates all steps and aggregates their constraints to determine
// which platforms the recipe supports and how it varies by family.
func AnalyzeRecipe(recipe *Recipe) *RecipeAnalysis {
	familiesUsed := make(map[string]bool)
	hasFamilyVaryingStep := false
	hasUnconstrainedLinuxSteps := false
	hasAnyLinuxSteps := false
	hasAnyDarwinSteps := false

	for _, step := range recipe.Steps {
		analysis := step.Analysis()

		// If analysis is nil (backward compat when loader doesn't set it),
		// treat as unconstrained
		if analysis == nil {
			hasAnyLinuxSteps = true
			hasAnyDarwinSteps = true
			hasUnconstrainedLinuxSteps = true
			continue
		}

		// Track OS support from step constraints
		// Unconstrained (nil or empty OS) means both OSes
		// Explicit OS constraint means only that OS
		if analysis.Constraint == nil || analysis.Constraint.OS == "" {
			hasAnyLinuxSteps = true
			hasAnyDarwinSteps = true
		} else if analysis.Constraint.OS == "linux" {
			hasAnyLinuxSteps = true
		} else if analysis.Constraint.OS == "darwin" {
			hasAnyDarwinSteps = true
			continue // Skip family analysis for darwin-only steps
		}

		// Family analysis (only for Linux-applicable steps)
		// Handle constrained+varying: interpolation within a family constraint
		if analysis.FamilyVarying {
			if analysis.Constraint != nil && analysis.Constraint.LinuxFamily != "" {
				// Constrained+varying: interpolation only happens within this family
				familiesUsed[analysis.Constraint.LinuxFamily] = true
			} else {
				// Unconstrained varying: needs all families
				hasFamilyVaryingStep = true
			}
		} else if analysis.Constraint != nil && analysis.Constraint.LinuxFamily != "" {
			familiesUsed[analysis.Constraint.LinuxFamily] = true
		} else if analysis.Constraint == nil || analysis.Constraint.OS == "" || analysis.Constraint.OS == "linux" {
			hasUnconstrainedLinuxSteps = true
		}
	}

	// Determine policy - no nil sentinel, explicit enum for each case
	var policy RecipeFamilyPolicy
	if !hasAnyLinuxSteps {
		policy = FamilyNone
	} else if hasFamilyVaryingStep {
		policy = FamilyVarying
	} else if len(familiesUsed) == 0 {
		policy = FamilyAgnostic
	} else if hasUnconstrainedLinuxSteps {
		policy = FamilyMixed
	} else {
		policy = FamilySpecific
	}

	return &RecipeAnalysis{
		Policy:         policy,
		FamiliesUsed:   familiesUsed,
		SupportsDarwin: hasAnyDarwinSteps,
	}
}

// SupportedPlatforms returns all platforms the recipe supports.
// This is the list that should be used for golden file generation/validation.
func SupportedPlatforms(recipe *Recipe) []Platform {
	analysis := AnalyzeRecipe(recipe)

	var platforms []Platform

	// Add darwin platforms only if recipe supports darwin (derived from analysis)
	if analysis.SupportsDarwin {
		platforms = append(platforms,
			Platform{OS: "darwin", Arch: "amd64"},
			Platform{OS: "darwin", Arch: "arm64"},
		)
	}

	// Add Linux platforms based on policy
	switch analysis.Policy {
	case FamilyNone:
		// No Linux platforms - no family policy applies

	case FamilyAgnostic:
		// Generic Linux (no family qualifier)
		platforms = append(platforms,
			Platform{OS: "linux", Arch: "amd64"},
			Platform{OS: "linux", Arch: "arm64"},
		)

	case FamilyVarying, FamilyMixed:
		// All families needed
		for _, family := range AllLinuxFamilies {
			platforms = append(platforms,
				Platform{OS: "linux", Arch: "amd64", LinuxFamily: family},
				Platform{OS: "linux", Arch: "arm64", LinuxFamily: family},
			)
		}

	case FamilySpecific:
		// Only specific families
		for family := range analysis.FamiliesUsed {
			platforms = append(platforms,
				Platform{OS: "linux", Arch: "amd64", LinuxFamily: family},
				Platform{OS: "linux", Arch: "arm64", LinuxFamily: family},
			)
		}
	}

	return platforms
}
