package version

import (
	"context"
	"fmt"

	"github.com/tsukumogami/tsuku/internal/install"
)

// ResolveWithinBoundary resolves the latest version within the pin boundary
// defined by the requested constraint. It routes through different resolution
// strategies based on the provider type and constraint:
//
//   - Empty requested: delegates to provider.ResolveLatest()
//   - VersionLister provider: filters the cached version list by pin boundary
//   - VersionResolver-only provider: delegates to provider.ResolveVersion()
func ResolveWithinBoundary(ctx context.Context, provider VersionResolver, requested string) (*VersionInfo, error) {
	if err := install.ValidateRequested(requested); err != nil {
		return nil, fmt.Errorf("invalid version constraint: %w", err)
	}

	// No constraint: resolve absolute latest
	if requested == "" || requested == "latest" {
		return provider.ResolveLatest(ctx)
	}

	// Channel pins: delegate to ResolveVersion for provider-specific handling
	if install.PinLevelFromRequested(requested) == install.PinChannel {
		return provider.ResolveVersion(ctx, requested)
	}

	// For VersionLister providers, filter the cached version list
	if lister, ok := provider.(VersionLister); ok {
		versions, err := lister.ListVersions(ctx)
		if err != nil {
			// Fallback to ResolveVersion if listing fails
			return provider.ResolveVersion(ctx, requested)
		}

		// Find the highest version matching the pin boundary.
		// Versions are returned newest first by convention.
		for _, v := range versions {
			if install.VersionMatchesPin(v, requested) {
				// Resolve to get the full VersionInfo with Tag field
				return provider.ResolveVersion(ctx, v)
			}
		}
		return nil, fmt.Errorf("version %s not found", requested)
	}

	// VersionResolver-only providers: use fuzzy prefix matching
	return provider.ResolveVersion(ctx, requested)
}
