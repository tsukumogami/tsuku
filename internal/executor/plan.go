package executor

import (
	"fmt"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/actions"
)

// PlanFormatVersion is the current version of the installation plan format.
// Readers should reject plans with unsupported versions.
// Version history:
//   - Version 1: Original format with composite actions in plans
//   - Version 2: Composite actions decomposed to primitives (introduced in #440)
const PlanFormatVersion = 2

// InstallationPlan represents a fully-resolved, deterministic specification
// for installing a tool. Plans capture the exact URLs, checksums, and steps
// needed to reproduce an installation.
type InstallationPlan struct {
	// FormatVersion enables future evolution of the plan format.
	// Currently 1.
	FormatVersion int `json:"format_version"`

	// Metadata
	Tool        string    `json:"tool"`
	Version     string    `json:"version"`
	Platform    Platform  `json:"platform"`
	GeneratedAt time.Time `json:"generated_at"`

	// Recipe provenance
	RecipeHash   string `json:"recipe_hash"`   // SHA256 of recipe file content
	RecipeSource string `json:"recipe_source"` // "registry" or file path

	// Deterministic indicates whether the entire plan is deterministic.
	// A plan is deterministic only if ALL steps are deterministic.
	// False if any step uses ecosystem primitives (go_build, cargo_build, etc.)
	// which have residual non-determinism from compiler versions, native extensions, etc.
	Deterministic bool `json:"deterministic"`

	// Resolved steps
	Steps []ResolvedStep `json:"steps"`

	// Verify contains the verification command and pattern from the recipe.
	// This is needed when executing from a plan file without the original recipe.
	Verify *PlanVerify `json:"verify,omitempty"`

	// Metadata from the recipe (needed for install_binaries directory mode checks)
	RecipeType string `json:"recipe_type,omitempty"` // "tool" or "library"
}

// PlanVerify captures verification information from the recipe.
type PlanVerify struct {
	Command string `json:"command,omitempty"`
	Pattern string `json:"pattern,omitempty"`
}

// Platform identifies the target operating system and architecture.
type Platform struct {
	OS   string `json:"os"`   // e.g., "linux", "darwin", "windows"
	Arch string `json:"arch"` // e.g., "amd64", "arm64"
}

// ResolvedStep represents a single installation step with all templates
// expanded and parameters resolved.
type ResolvedStep struct {
	// Action is the action name (e.g., "download", "extract", "install_binaries")
	Action string `json:"action"`

	// Params contains the resolved action parameters
	Params map[string]interface{} `json:"params"`

	// Evaluable indicates whether this step can be deterministically reproduced.
	// False for run_command, ecosystem installs (npm, pip, cargo, etc.)
	Evaluable bool `json:"evaluable"`

	// Deterministic indicates whether this step produces identical results
	// given identical inputs. Tier 1 (core) primitives are deterministic.
	// Tier 2 (ecosystem) primitives like go_build, cargo_build have residual
	// non-determinism from compiler versions, native extensions, etc.
	Deterministic bool `json:"deterministic"`

	// For download steps only - these capture the resolved URL and computed checksum
	URL      string `json:"url,omitempty"`
	Checksum string `json:"checksum,omitempty"` // SHA256 in hex format
	Size     int64  `json:"size,omitempty"`     // File size in bytes
}

// ActionEvaluability classifies actions by whether they can be deterministically
// evaluated and reproduced. This map is used during plan generation to set
// the Evaluable field on ResolvedStep.
//
// As of format version 2, plans only contain primitive actions. Composite
// actions are decomposed during plan generation.
//
// Evaluable actions have predictable, reproducible outcomes:
// - Download actions: URL and checksum can be captured
// - File operations: Parameters are deterministic
//
// Non-evaluable actions cannot guarantee reproducibility:
// - run_command: Arbitrary shell execution
// - Package manager actions: External dependency resolution
var ActionEvaluability = map[string]bool{
	// Primitive actions - evaluable (direct execution with deterministic outcomes)
	"download":          true,
	"extract":           true,
	"chmod":             true,
	"install_binaries":  true,
	"set_env":           true,
	"set_rpath":         true,
	"link_dependencies": true,
	"install_libraries": true,
	"validate_checksum": true,

	// Ecosystem primitives - evaluable through ecosystem-specific configuration
	"npm_exec": true,

	// System package actions - not evaluable (external package managers)
	"apt_install":  false,
	"yum_install":  false,
	"brew_install": false,

	// Ecosystem package managers - not evaluable (external dependency resolution)
	"npm_install":   false,
	"pipx_install":  false,
	"cargo_install": false,
	"gem_install":   false,
	"cpan_install":  false,
	"go_install":    false,
	"nix_install":   false,

	// Shell execution - not evaluable (arbitrary commands)
	"run_command": false,
}

// IsActionEvaluable returns whether an action can be deterministically evaluated.
// Unknown actions are considered non-evaluable for safety.
func IsActionEvaluable(action string) bool {
	evaluable, known := ActionEvaluability[action]
	if !known {
		// Unknown actions are not evaluable for safety
		return false
	}
	return evaluable
}

// ValidationError describes a plan validation failure.
type ValidationError struct {
	Step    int    // 0-indexed step number
	Action  string // Action name that failed validation
	Message string // Human-readable error description
}

// Error implements the error interface for ValidationError.
func (e *ValidationError) Error() string {
	return fmt.Sprintf("step %d (%s): %s", e.Step, e.Action, e.Message)
}

// PlanValidationError wraps multiple validation errors into a single error.
type PlanValidationError struct {
	Errors []ValidationError
}

// Error implements the error interface for PlanValidationError.
func (e *PlanValidationError) Error() string {
	if len(e.Errors) == 0 {
		return "plan validation failed"
	}
	if len(e.Errors) == 1 {
		return fmt.Sprintf("invalid plan: %s", e.Errors[0].Error())
	}
	var msgs []string
	for _, err := range e.Errors {
		msgs = append(msgs, err.Error())
	}
	return fmt.Sprintf("invalid plan: %d errors:\n  - %s", len(e.Errors), strings.Join(msgs, "\n  - "))
}

// ValidatePlan checks that a plan contains only primitive actions and that
// download actions have required checksum data. Returns nil if the plan is valid,
// or a PlanValidationError containing all validation failures.
//
// Validation rules:
//   - All step actions must be primitives (as defined by actions.IsPrimitive)
//   - Download actions must have a non-empty Checksum field (security requirement)
//   - Format version must be supported (currently only version 2)
func ValidatePlan(plan *InstallationPlan) error {
	var errors []ValidationError

	// Check format version
	if plan.FormatVersion < 2 {
		errors = append(errors, ValidationError{
			Step:    -1,
			Action:  "",
			Message: fmt.Sprintf("unsupported plan format version %d (expected >= 2)", plan.FormatVersion),
		})
	}

	// Validate each step
	for i, step := range plan.Steps {
		// Check if action is a primitive
		if !actions.IsPrimitive(step.Action) {
			// Check if it's a known composite action
			if actions.IsDecomposable(step.Action) {
				errors = append(errors, ValidationError{
					Step:    i,
					Action:  step.Action,
					Message: fmt.Sprintf("composite action %q should have been decomposed at eval time", step.Action),
				})
			} else if actions.Get(step.Action) != nil {
				errors = append(errors, ValidationError{
					Step:    i,
					Action:  step.Action,
					Message: fmt.Sprintf("action %q is neither primitive nor decomposable", step.Action),
				})
			} else {
				errors = append(errors, ValidationError{
					Step:    i,
					Action:  step.Action,
					Message: fmt.Sprintf("unknown action %q", step.Action),
				})
			}
		}

		// Check checksum for download actions
		if step.Action == "download" && step.Checksum == "" {
			errors = append(errors, ValidationError{
				Step:    i,
				Action:  step.Action,
				Message: "download action missing checksum (security requirement)",
			})
		}
	}

	if len(errors) > 0 {
		return &PlanValidationError{Errors: errors}
	}
	return nil
}

// ValidatePlanStrict is a stricter version of ValidatePlan that returns
// individual errors for inspection. It returns the same errors as ValidatePlan
// but in slice form for programmatic access.
func ValidatePlanStrict(plan *InstallationPlan) []ValidationError {
	err := ValidatePlan(plan)
	if err == nil {
		return nil
	}
	if pve, ok := err.(*PlanValidationError); ok {
		return pve.Errors
	}
	return nil
}
