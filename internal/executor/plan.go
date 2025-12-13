package executor

import "time"

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

	// Resolved steps
	Steps []ResolvedStep `json:"steps"`
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
