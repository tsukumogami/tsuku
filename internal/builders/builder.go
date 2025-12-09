package builders

import (
	"context"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// BuildRequest contains builder-specific parameters for recipe generation.
type BuildRequest struct {
	// Package is the tool name the user wants (e.g., "gh", "ripgrep")
	Package string

	// Version is the optional specific version to install (empty = latest)
	Version string

	// SourceArg is a builder-specific argument. For example:
	// - github builder: "cli/cli" (owner/repo)
	// - crates.io builder: unused (Package is the crate name)
	SourceArg string
}

// Builder generates recipes for packages from a specific ecosystem.
// Builders are invoked via "tsuku create" to generate recipes that are
// written to the user's local recipes directory ($TSUKU_HOME/recipes/).
type Builder interface {
	// Name returns the builder identifier (e.g., "crates_io", "rubygems", "github")
	Name() string

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
}
