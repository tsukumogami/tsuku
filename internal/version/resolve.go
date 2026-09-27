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

		// Nothing matched as spelled. Retry ignoring a leading "v" on both
		// sides, so "2.37.1" finds the tag "v2.37.1" and "v2.37.1" finds a
		// list the provider already stripped. This runs only after the exact
		// pass, so a pin that matched before still resolves to the same entry
		// even when the list holds both "1.0" and "v1.0".
		bareRequested := trimVersionV(requested)
		for _, v := range versions {
			if install.VersionMatchesPin(trimVersionV(v), bareRequested) {
				return provider.ResolveVersion(ctx, v)
			}
		}
		return nil, fmt.Errorf("version %s not found", requested)
	}

	// VersionResolver-only providers: use fuzzy prefix matching
	return provider.ResolveVersion(ctx, requested)
}

// trimVersionV strips one leading "v" when a digit follows it. Anything else
// is left alone: "v" on its own would otherwise become "" (latest), and tags
// such as "vim-9.0" are names rather than prefixed versions. normalizeVersion
// is not used here because it also strips "go" and path segments.
func trimVersionV(s string) string {
	if len(s) > 1 && s[0] == 'v' && s[1] >= '0' && s[1] <= '9' {
		return s[1:]
	}
	return s
}
