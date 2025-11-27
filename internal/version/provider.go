package version

import "context"

// VersionResolver is the minimal interface all version providers must implement.
// It resolves versions but doesn't require listing all versions (some sources can't).
//
// This interface follows the Interface Segregation Principle (SOLID) by providing
// only the minimum required functionality that all providers can implement.
type VersionResolver interface {
	// ResolveLatest returns the latest stable version from the source
	ResolveLatest(ctx context.Context) (*VersionInfo, error)

	// ResolveVersion resolves a specific version constraint.
	// Handles fuzzy matching (e.g., "1.29" might resolve to "1.29.3").
	ResolveVersion(ctx context.Context, version string) (*VersionInfo, error)

	// SourceDescription returns a human-readable source description.
	// Examples: "GitHub:rust-lang/rust", "npm:turbo", "custom:nodejs_dist"
	SourceDescription() string
}

// VersionLister extends VersionResolver with the ability to list all versions.
// Not all providers can implement this (e.g., custom sources without APIs).
//
// Following the Interface Segregation Principle, this separate interface allows
// providers to opt into listing capability only if they can support it.
type VersionLister interface {
	VersionResolver

	// ListVersions returns all available versions (newest first).
	// Returns an error if the provider doesn't support version listing.
	ListVersions(ctx context.Context) ([]string, error)
}

// VersionProvider is an alias for VersionResolver for backward compatibility
type VersionProvider = VersionResolver
