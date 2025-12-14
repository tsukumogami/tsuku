package builders

import (
	"context"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// InitOptions contains options for builder initialization.
type InitOptions struct {
	// SkipValidation disables container validation for LLM-generated recipes.
	SkipValidation bool

	// ProgressReporter receives progress updates during build operations.
	// If nil, no progress is reported.
	ProgressReporter ProgressReporter
}

// BuildRequest contains builder-specific parameters for recipe generation.
type BuildRequest struct {
	// Package is the tool name the user wants (e.g., "gh", "ripgrep")
	Package string

	// Version is the optional specific version to install (empty = latest)
	Version string

	// SourceArg is a builder-specific argument passed from the --from flag.
	// The builder is responsible for parsing any builder-specific syntax.
	// Examples:
	// - github builder: "cli/cli" (owner/repo)
	// - homebrew builder: "jq" or "jq:source" (formula with optional :source suffix)
	// - crates.io builder: unused (Package is the crate name)
	SourceArg string
}

// Builder generates recipes for packages from a specific ecosystem.
// Builders are invoked via "tsuku create" to generate recipes that are
// written to the user's local recipes directory ($TSUKU_HOME/recipes/).
type Builder interface {
	// Name returns the builder identifier (e.g., "crates_io", "rubygems", "github")
	Name() string

	// Initialize performs builder-specific setup.
	// For LLM builders, this creates the LLM factory, validates the API key,
	// and sets up validation executor and progress reporter.
	// For ecosystem builders, this is a no-op.
	// Returns error if initialization fails (e.g., missing API key).
	Initialize(ctx context.Context, opts *InitOptions) error

	// RequiresLLM returns true if this builder uses LLM for recipe generation.
	// Used by CLI to apply LLM-specific behaviors like budget checks,
	// preview prompts, and cost tracking.
	RequiresLLM() bool

	// CanBuild checks if this builder can handle the package.
	// It typically queries the ecosystem's API to verify the package exists.
	CanBuild(ctx context.Context, packageName string) (bool, error)

	// Build generates a recipe for the package.
	// If req.Version is empty, the recipe will use a version provider for dynamic resolution.
	Build(ctx context.Context, req BuildRequest) (*BuildResult, error)
}

// BuildResult contains the generated recipe and metadata about the build process.
type BuildResult struct {
	// Recipe is the generated recipe struct
	Recipe *recipe.Recipe

	// Warnings contains human-readable messages about generation uncertainty.
	// For example: "Could not determine executables from Cargo.toml; using crate name"
	Warnings []string

	// Source identifies where the metadata came from (e.g., "crates.io:ripgrep")
	Source string

	// RepairAttempts is the number of repair attempts made after validation failures.
	// Only populated by builders that support validation (e.g., GitHubReleaseBuilder).
	RepairAttempts int

	// Provider is the LLM provider that generated the recipe (e.g., "claude", "gemini").
	// Only populated by LLM-based builders.
	Provider string

	// ValidationSkipped indicates validation was skipped (e.g., no container runtime).
	ValidationSkipped bool

	// Cost is the estimated cost in USD for LLM-based generation.
	// Only populated by LLM-based builders (e.g., GitHubReleaseBuilder).
	Cost float64
}
