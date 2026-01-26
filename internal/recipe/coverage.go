package recipe

import (
	"fmt"
	"slices"
)

// CoverageReport contains the results of analyzing a recipe for platform coverage.
type CoverageReport struct {
	Recipe        string   // Recipe name
	HasGlibc      bool     // True if recipe has steps that support glibc
	HasMusl       bool     // True if recipe has steps that support musl
	HasDarwin     bool     // True if recipe has steps that support darwin
	SupportedLibc []string // Explicit supported_libc constraint from metadata
	Warnings      []string // Non-blocking issues (e.g., tools missing musl support)
	Errors        []string // Blocking issues (e.g., libraries missing musl without constraint)
}

// AnalyzeRecipeCoverage analyzes a recipe for platform coverage.
// It checks step when clauses to determine which platforms are supported.
// Returns errors for library recipes missing musl support (without explicit constraint).
// Returns warnings for tool recipes with library dependencies missing musl support.
func AnalyzeRecipeCoverage(r *Recipe) CoverageReport {
	report := CoverageReport{
		Recipe:        r.Metadata.Name,
		SupportedLibc: r.Metadata.SupportedLibc,
	}

	// Analyze steps for platform coverage
	for _, step := range r.Steps {
		if step.When == nil || step.When.IsEmpty() {
			// Unconditional step - counts for all platforms
			report.HasGlibc = true
			report.HasMusl = true
			report.HasDarwin = true
			continue
		}

		if stepMatchesGlibc(step.When) {
			report.HasGlibc = true
		}
		if stepMatchesMusl(step.When) {
			report.HasMusl = true
		}
		if stepMatchesDarwin(step.When) {
			report.HasDarwin = true
		}
	}

	// Check if musl is explicitly excluded via supported_libc constraint
	muslExcluded := len(report.SupportedLibc) > 0 &&
		!slices.Contains(report.SupportedLibc, "musl")

	// Generate errors/warnings for missing coverage
	if !report.HasMusl && !muslExcluded {
		if r.IsLibrary() {
			report.Errors = append(report.Errors,
				fmt.Sprintf("library recipe '%s' has no musl path and no explicit constraint (add supported_libc = [\"glibc\"] with unsupported_reason if musl cannot be supported)",
					r.Metadata.Name))
		} else if hasLibraryDependencies(r) {
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("recipe '%s' depends on libraries but has no musl path (tool will not work on Alpine/musl systems)",
					r.Metadata.Name))
		}
	}

	return report
}

// stepMatchesGlibc returns true if the step's when clause could match a glibc Linux target.
func stepMatchesGlibc(w *WhenClause) bool {
	if w == nil {
		return true
	}

	// If libc is specified, check if glibc is included
	if len(w.Libc) > 0 {
		if !slices.Contains(w.Libc, "glibc") {
			return false
		}
	}

	// If OS is specified, check if linux is included (glibc is Linux-only)
	if len(w.OS) > 0 {
		if !slices.Contains(w.OS, "linux") {
			return false
		}
	}

	// If platform is specified, check if any linux platform is included
	if len(w.Platform) > 0 {
		hasLinux := false
		for _, p := range w.Platform {
			if len(p) >= 5 && p[:5] == "linux" {
				hasLinux = true
				break
			}
		}
		if !hasLinux {
			return false
		}
	}

	return true
}

// stepMatchesMusl returns true if the step's when clause could match a musl Linux target.
func stepMatchesMusl(w *WhenClause) bool {
	if w == nil {
		return true
	}

	// If libc is specified, check if musl is included
	if len(w.Libc) > 0 {
		if !slices.Contains(w.Libc, "musl") {
			return false
		}
	}

	// If OS is specified, check if linux is included (musl is Linux-only)
	if len(w.OS) > 0 {
		if !slices.Contains(w.OS, "linux") {
			return false
		}
	}

	// If platform is specified, check if any linux platform is included
	if len(w.Platform) > 0 {
		hasLinux := false
		for _, p := range w.Platform {
			if len(p) >= 5 && p[:5] == "linux" {
				hasLinux = true
				break
			}
		}
		if !hasLinux {
			return false
		}
	}

	return true
}

// stepMatchesDarwin returns true if the step's when clause could match a darwin target.
func stepMatchesDarwin(w *WhenClause) bool {
	if w == nil {
		return true
	}

	// libc filter doesn't affect darwin (darwin doesn't have glibc/musl)
	// but if libc is specified, the step is Linux-only
	if len(w.Libc) > 0 {
		return false
	}

	// If OS is specified, check if darwin is included
	if len(w.OS) > 0 {
		if !slices.Contains(w.OS, "darwin") {
			return false
		}
	}

	// If platform is specified, check if any darwin platform is included
	if len(w.Platform) > 0 {
		hasDarwin := false
		for _, p := range w.Platform {
			if len(p) >= 6 && p[:6] == "darwin" {
				hasDarwin = true
				break
			}
		}
		if !hasDarwin {
			return false
		}
	}

	return true
}

// hasLibraryDependencies returns true if the recipe has any library dependencies.
// A dependency is a library if its recipe has type = "library".
func hasLibraryDependencies(r *Recipe) bool {
	// Check recipe-level dependencies
	if len(r.Metadata.Dependencies) > 0 {
		return true
	}

	// Check step-level dependencies
	for _, step := range r.Steps {
		if len(step.Dependencies) > 0 {
			return true
		}
	}

	return false
}

// ValidateCoverageForRecipes validates libc coverage for multiple recipes.
// Returns a slice of validation errors (recipes with coverage issues).
func ValidateCoverageForRecipes(recipes []*Recipe) []CoverageReport {
	var reports []CoverageReport
	for _, r := range recipes {
		report := AnalyzeRecipeCoverage(r)
		if len(report.Errors) > 0 || len(report.Warnings) > 0 {
			reports = append(reports, report)
		}
	}
	return reports
}
