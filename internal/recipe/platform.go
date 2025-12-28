package recipe

import (
	"fmt"
	"runtime"
	"strings"
)

// UnsupportedPlatformError is returned when a recipe doesn't support the current platform
type UnsupportedPlatformError struct {
	RecipeName           string
	CurrentOS            string
	CurrentArch          string
	SupportedOS          []string
	SupportedArch        []string
	UnsupportedPlatforms []string
}

func (e *UnsupportedPlatformError) Error() string {
	var msg strings.Builder
	fmt.Fprintf(&msg, "recipe '%s' is not available for %s/%s\n\n",
		e.RecipeName, e.CurrentOS, e.CurrentArch)

	// Determine if we have constraints to show
	hasAllowlist := len(e.SupportedOS) > 0 || len(e.SupportedArch) > 0
	hasDenylist := len(e.UnsupportedPlatforms) > 0

	if hasAllowlist || hasDenylist {
		msg.WriteString("Platform constraints:\n")

		// Show allowlist
		osStr := "all"
		if len(e.SupportedOS) > 0 {
			osStr = strings.Join(e.SupportedOS, ", ")
		}

		archStr := "all"
		if len(e.SupportedArch) > 0 {
			archStr = strings.Join(e.SupportedArch, ", ")
		}

		fmt.Fprintf(&msg, "  Allowed: %s OS, %s arch\n", osStr, archStr)

		// Show denylist if present
		if hasDenylist {
			fmt.Fprintf(&msg, "  Except: %s\n", strings.Join(e.UnsupportedPlatforms, ", "))
		}
	}

	return msg.String()
}

// TsukuSupportedOS returns the operating systems that tsuku supports
func TsukuSupportedOS() []string {
	return []string{"linux", "darwin"}
}

// TsukuSupportedArch returns the architectures that tsuku supports
func TsukuSupportedArch() []string {
	return []string{"amd64", "arm64"}
}

// containsString checks if a string slice contains a given value
func containsString(slice []string, value string) bool {
	for _, item := range slice {
		if item == value {
			return true
		}
	}
	return false
}

// SupportsPlatform returns true if the recipe supports the given OS and architecture.
// Uses complementary hybrid: (allowlist_os × allowlist_arch) - denylist_platforms
//
// Platform support is computed as:
//  1. Build Cartesian product of supported_os × supported_arch (defaults: all OS, all arch)
//  2. Remove platforms in unsupported_platforms list (default: empty)
//
// Examples:
//   - Missing fields: supports all platforms
//   - supported_os = ["linux"]: supports Linux on any arch
//   - unsupported_platforms = ["darwin/arm64"]: supports all except macOS ARM64
//   - supported_os = ["linux", "darwin"] + unsupported_platforms = ["darwin/arm64"]:
//     supports Linux (any arch) and macOS (non-ARM64)
func (r *Recipe) SupportsPlatform(targetOS, targetArch string) bool {
	// Build allowlist with defaults
	// nil = use defaults (tsuku-supported platforms)
	// empty slice [] = explicit empty set (no platforms)
	supportedOS := r.Metadata.SupportedOS
	if supportedOS == nil {
		supportedOS = TsukuSupportedOS() // Default: tsuku-supported OS
	}

	supportedArch := r.Metadata.SupportedArch
	if supportedArch == nil {
		supportedArch = TsukuSupportedArch() // Default: tsuku-supported arch
	}

	// Check if in Cartesian product (allowlist)
	inAllowlist := containsString(supportedOS, targetOS) && containsString(supportedArch, targetArch)
	if !inAllowlist {
		return false
	}

	// Check if in denylist (exceptions)
	platformTuple := fmt.Sprintf("%s/%s", targetOS, targetArch)
	inDenylist := containsString(r.Metadata.UnsupportedPlatforms, platformTuple)

	return !inDenylist
}

// SupportsPlatformRuntime is a convenience method that checks platform support
// using the current runtime's GOOS and GOARCH values.
func (r *Recipe) SupportsPlatformRuntime() bool {
	return r.SupportsPlatform(runtime.GOOS, runtime.GOARCH)
}

// NewUnsupportedPlatformError creates an UnsupportedPlatformError for the current platform
func (r *Recipe) NewUnsupportedPlatformError() *UnsupportedPlatformError {
	return &UnsupportedPlatformError{
		RecipeName:           r.Metadata.Name,
		CurrentOS:            runtime.GOOS,
		CurrentArch:          runtime.GOARCH,
		SupportedOS:          r.Metadata.SupportedOS,
		SupportedArch:        r.Metadata.SupportedArch,
		UnsupportedPlatforms: r.Metadata.UnsupportedPlatforms,
	}
}

// PlatformConstraintWarning represents a non-critical issue with platform constraints
type PlatformConstraintWarning struct {
	Message string
}

func (w *PlatformConstraintWarning) Error() string {
	return w.Message
}

// ValidatePlatformConstraints performs edge case validation on platform fields.
// Returns warnings for no-op constraints, errors for empty result sets.
//
// Warnings (fail in strict mode):
//   - unsupported_platforms contains entry not in (supported_os × supported_arch)
//
// Errors:
//   - Result set of supported platforms is empty (all platforms excluded)
func (r *Recipe) ValidatePlatformConstraints() (warnings []PlatformConstraintWarning, err error) {
	// Compute effective supported platforms
	// nil = use defaults (tsuku-supported platforms)
	// empty slice [] = explicit empty set (no platforms)
	supportedOS := r.Metadata.SupportedOS
	if supportedOS == nil {
		supportedOS = TsukuSupportedOS()
	}

	supportedArch := r.Metadata.SupportedArch
	if supportedArch == nil {
		supportedArch = TsukuSupportedArch()
	}

	// Build Cartesian product
	allowedPlatforms := make(map[string]bool)
	for _, os := range supportedOS {
		for _, arch := range supportedArch {
			allowedPlatforms[fmt.Sprintf("%s/%s", os, arch)] = true
		}
	}

	// Check for no-op exclusions (warning in strict mode)
	for _, unsupported := range r.Metadata.UnsupportedPlatforms {
		if !allowedPlatforms[unsupported] {
			warnings = append(warnings, PlatformConstraintWarning{
				Message: fmt.Sprintf(
					"unsupported_platforms contains '%s' which is not in (supported_os × supported_arch); this constraint has no effect",
					unsupported,
				),
			})
		} else {
			delete(allowedPlatforms, unsupported)
		}
	}

	// Check for empty result set (error)
	if len(allowedPlatforms) == 0 {
		return warnings, fmt.Errorf(
			"platform constraints result in no supported platforms (all platforms excluded)",
		)
	}

	return warnings, nil
}

// GetSupportedPlatforms returns a list of all supported platform tuples ("os/arch")
// after applying the complementary hybrid constraint logic.
func (r *Recipe) GetSupportedPlatforms() []string {
	supportedOS := r.Metadata.SupportedOS
	if supportedOS == nil {
		supportedOS = TsukuSupportedOS()
	}

	supportedArch := r.Metadata.SupportedArch
	if supportedArch == nil {
		supportedArch = TsukuSupportedArch()
	}

	// Build Cartesian product
	var platforms []string
	for _, os := range supportedOS {
		for _, arch := range supportedArch {
			platform := fmt.Sprintf("%s/%s", os, arch)
			// Skip if in denylist
			if !containsString(r.Metadata.UnsupportedPlatforms, platform) {
				platforms = append(platforms, platform)
			}
		}
	}

	return platforms
}

// FormatPlatformConstraints returns a human-readable string describing the platform constraints.
// Used for displaying constraints in commands like `tsuku info`.
func (r *Recipe) FormatPlatformConstraints() string {
	hasConstraints := len(r.Metadata.SupportedOS) > 0 ||
		len(r.Metadata.SupportedArch) > 0 ||
		len(r.Metadata.UnsupportedPlatforms) > 0

	if !hasConstraints {
		return "all platforms"
	}

	var parts []string

	// Show OS constraints
	if len(r.Metadata.SupportedOS) > 0 {
		parts = append(parts, fmt.Sprintf("OS: %s", strings.Join(r.Metadata.SupportedOS, ", ")))
	} else {
		parts = append(parts, "OS: all")
	}

	// Show arch constraints
	if len(r.Metadata.SupportedArch) > 0 {
		parts = append(parts, fmt.Sprintf("Arch: %s", strings.Join(r.Metadata.SupportedArch, ", ")))
	} else {
		parts = append(parts, "Arch: all")
	}

	// Show exceptions
	if len(r.Metadata.UnsupportedPlatforms) > 0 {
		parts = append(parts, fmt.Sprintf("Except: %s", strings.Join(r.Metadata.UnsupportedPlatforms, ", ")))
	}

	return strings.Join(parts, " | ")
}

// StepValidationError represents an error found during step validation
type StepValidationError struct {
	StepIndex int
	Message   string
}

func (e *StepValidationError) Error() string {
	return fmt.Sprintf("step %d: %s", e.StepIndex, e.Message)
}

// ValidateStepsAgainstPlatforms validates that step mappings and install_guide
// entries are consistent with the recipe's platform constraints.
//
// This validation checks:
// 1. os_mapping keys exist in the final set of supported OS values
// 2. arch_mapping keys exist in the final set of supported arch values
// 3. require_system steps with install_guide:
//   - Platform tuple keys (os/arch) exist in supported platforms
//   - OS-only keys exist in supported OS set
//   - All platforms have coverage via exact tuple, OS key, or fallback
//
// Returns a slice of errors for any inconsistencies found.
func (r *Recipe) ValidateStepsAgainstPlatforms() []error {
	var errors []error

	// Get final supported platforms and extract unique OS/arch sets
	platforms := r.GetSupportedPlatforms()
	supportedOS := make(map[string]bool)
	supportedArch := make(map[string]bool)

	for _, platform := range platforms {
		parts := strings.Split(platform, "/")
		if len(parts) == 2 {
			supportedOS[parts[0]] = true
			supportedArch[parts[1]] = true
		}
	}

	// Validate each step
	for i, step := range r.Steps {
		// Check os_mapping
		if osMapping, ok := step.Params["os_mapping"].(map[string]interface{}); ok {
			for os := range osMapping {
				if !supportedOS[os] {
					errors = append(errors, &StepValidationError{
						StepIndex: i,
						Message:   fmt.Sprintf("os_mapping contains '%s' which is not in the recipe's supported platforms", os),
					})
				}
			}
		}

		// Check arch_mapping
		if archMapping, ok := step.Params["arch_mapping"].(map[string]interface{}); ok {
			for arch := range archMapping {
				if !supportedArch[arch] {
					errors = append(errors, &StepValidationError{
						StepIndex: i,
						Message:   fmt.Sprintf("arch_mapping contains '%s' which is not in the recipe's supported platforms", arch),
					})
				}
			}
		}

		// Check install_guide coverage for require_system steps
		// Supports both platform tuples (os/arch) and OS-only keys with hierarchical fallback
		if step.Action == "require_system" {
			if installGuide, ok := step.Params["install_guide"].(map[string]interface{}); ok {
				// Validate that tuple keys exist in supported platforms
				for key := range installGuide {
					if strings.Contains(key, "/") {
						// Validate tuple format and check against supported platforms
						if !containsString(platforms, key) && key != "fallback" {
							errors = append(errors, &StepValidationError{
								StepIndex: i,
								Message:   fmt.Sprintf("install_guide contains platform tuple '%s' which is not in the recipe's supported platforms", key),
							})
						}
					} else if key != "fallback" {
						// Validate OS-only key exists in supported OS set
						if !supportedOS[key] {
							errors = append(errors, &StepValidationError{
								StepIndex: i,
								Message:   fmt.Sprintf("install_guide contains OS key '%s' which is not in the recipe's supported platforms", key),
							})
						}
					}
				}

				// Check coverage: each platform must be covered by exact tuple, OS key, or fallback
				for _, platform := range platforms {
					parts := strings.Split(platform, "/")
					if len(parts) != 2 {
						continue
					}
					os := parts[0]

					// Check hierarchical fallback: tuple → OS → fallback
					hasTupleGuide := false
					if _, ok := installGuide[platform]; ok {
						hasTupleGuide = true
					}

					hasOSGuide := false
					if _, ok := installGuide[os]; ok {
						hasOSGuide = true
					}

					hasFallback := false
					if _, ok := installGuide["fallback"]; ok {
						hasFallback = true
					}

					if !hasTupleGuide && !hasOSGuide && !hasFallback {
						errors = append(errors, &StepValidationError{
							StepIndex: i,
							Message:   fmt.Sprintf("install_guide missing coverage for platform '%s' (no exact tuple, OS key '%s', or fallback)", platform, os),
						})
					}
				}
			}
		}

		// Validate when clause platform tuples and OS arrays
		if step.When != nil {
			// Validate platform tuples exist in supported platforms
			for _, platform := range step.When.Platform {
				// Check tuple format
				if !strings.Contains(platform, "/") {
					errors = append(errors, &StepValidationError{
						StepIndex: i,
						Message:   fmt.Sprintf("when.platform contains invalid tuple '%s' (must be 'os/arch' format)", platform),
					})
					continue
				}

				// Check against supported platforms
				if !containsString(platforms, platform) {
					errors = append(errors, &StepValidationError{
						StepIndex: i,
						Message:   fmt.Sprintf("when.platform contains '%s' which is not in the recipe's supported platforms", platform),
					})
				}
			}

			// Validate OS values exist in supported OS set
			for _, os := range step.When.OS {
				if !supportedOS[os] {
					errors = append(errors, &StepValidationError{
						StepIndex: i,
						Message:   fmt.Sprintf("when.os contains '%s' which is not in the recipe's supported platforms", os),
					})
				}
			}
		}
	}

	return errors
}
