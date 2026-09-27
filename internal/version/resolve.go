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

		// Versions are returned newest first by convention, so the first
		// match is the highest version within the pin boundary.
		if v, ok := matchPin(versions, requested); ok {
			// Resolve to get the full VersionInfo with Tag field
			return provider.ResolveVersion(ctx, v)
		}

		// The list may be only part of what the upstream has; let the
		// provider look further before reporting the version missing.
		if unlisted, ok := provider.(UnlistedVersionResolver); ok {
			return unlisted.ResolveUnlisted(ctx, requested)
		}
		return nil, fmt.Errorf("version %s not found", requested)
	}

	// VersionResolver-only providers: use fuzzy prefix matching
	return provider.ResolveVersion(ctx, requested)
}

// UnlistedVersionResolver is implemented by a VersionLister whose
// ListVersions can return only part of the upstream's versions, such as the
// first page of an API listing. ResolveWithinBoundary calls ResolveUnlisted
// when nothing in ListVersions matched requested, so the cost of looking
// further is paid only on a miss. It returns an error when the version does
// not exist.
type UnlistedVersionResolver interface {
	ResolveUnlisted(ctx context.Context, requested string) (*VersionInfo, error)
}

// matchPin returns the first entry of versions within the pin boundary of
// requested. It tries the spellings as given first and only then retries
// ignoring a leading "v" on both sides, so "2.37.1" finds the tag "v2.37.1"
// and "v2.37.1" finds a list the provider already stripped. Because the exact
// pass runs first, a pin still resolves to the same entry when the list holds
// both "1.0" and "v1.0".
func matchPin(versions []string, requested string) (string, bool) {
	for _, v := range versions {
		if install.VersionMatchesPin(v, requested) {
			return v, true
		}
	}
	bareRequested := trimVersionV(requested)
	for _, v := range versions {
		if install.VersionMatchesPin(trimVersionV(v), bareRequested) {
			return v, true
		}
	}
	return "", false
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
