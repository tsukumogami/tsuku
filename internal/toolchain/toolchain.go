package toolchain

import (
	"fmt"
	"os/exec"
)

// Info contains information about a toolchain required for an ecosystem
type Info struct {
	// Binary is the executable name to check for (e.g., "cargo")
	Binary string
	// Name is the human-readable toolchain name (e.g., "Cargo")
	Name string
	// Language is the programming language (e.g., "Rust")
	Language string
	// TsukuRecipe is the tsuku recipe name to install this toolchain (e.g., "rust")
	TsukuRecipe string
}

// ecosystemToolchains maps ecosystem names to their required toolchain info
var ecosystemToolchains = map[string]Info{
	"crates.io": {
		Binary:      "cargo",
		Name:        "Cargo",
		Language:    "Rust",
		TsukuRecipe: "rust",
	},
	"rubygems": {
		Binary:      "gem",
		Name:        "gem",
		Language:    "Ruby",
		TsukuRecipe: "ruby",
	},
	"pypi": {
		Binary:      "pipx",
		Name:        "pipx",
		Language:    "Python",
		TsukuRecipe: "pipx",
	},
	"npm": {
		Binary:      "npm",
		Name:        "npm",
		Language:    "Node.js",
		TsukuRecipe: "nodejs",
	},
}

// LookPathFunc is the function used to check if a binary exists in PATH.
// This can be overridden in tests.
var LookPathFunc = exec.LookPath

// GetInfo returns the toolchain info for an ecosystem, or nil if unknown
func GetInfo(ecosystem string) *Info {
	if info, ok := ecosystemToolchains[ecosystem]; ok {
		return &info
	}
	return nil
}

// CheckAvailable checks if the toolchain for an ecosystem is available in PATH.
// Returns nil if available, or an error with a helpful message if not.
func CheckAvailable(ecosystem string) error {
	info := GetInfo(ecosystem)
	if info == nil {
		// Unknown ecosystem, no check needed
		return nil
	}

	_, err := LookPathFunc(info.Binary)
	if err != nil {
		return fmt.Errorf("%s is required to create recipes from %s. Install %s or run: tsuku install %s",
			info.Name, ecosystem, info.Language, info.TsukuRecipe)
	}

	return nil
}

// IsAvailable returns true if the toolchain for an ecosystem is available in PATH
func IsAvailable(ecosystem string) bool {
	return CheckAvailable(ecosystem) == nil
}
