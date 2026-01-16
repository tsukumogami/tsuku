package version

import (
	"context"
	"fmt"
	"strings"
)

// MetaCPANProvider resolves versions from MetaCPAN registry.
// Implements both VersionResolver and VersionLister interfaces.
type MetaCPANProvider struct {
	resolver     *Resolver
	distribution string
}

// NewMetaCPANProvider creates a provider for CPAN distributions
func NewMetaCPANProvider(resolver *Resolver, distribution string) *MetaCPANProvider {
	return &MetaCPANProvider{
		resolver:     resolver,
		distribution: distribution,
	}
}

// ListVersions returns all available versions from MetaCPAN (newest first)
func (p *MetaCPANProvider) ListVersions(ctx context.Context) ([]string, error) {
	return p.resolver.ListMetaCPANVersions(ctx, p.distribution)
}

// ResolveLatest returns the latest version from MetaCPAN
func (p *MetaCPANProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return p.resolver.ResolveMetaCPAN(ctx, p.distribution)
}

// ResolveVersion resolves a specific version for CPAN distributions.
// Validates that the requested version exists.
// Supports fuzzy matching (e.g., "3.7" matches "3.7.0" but not "3.70")
// Normalizes versions by stripping "v" prefix to handle MetaCPAN's version format
// (API returns "v1.0.35" but users may pass "1.0.35")
func (p *MetaCPANProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list MetaCPAN versions: %w", err)
	}

	// Normalize the requested version for comparison
	normalizedRequested := normalizeVersion(version)

	// Check for exact match (comparing normalized versions)
	for _, v := range versions {
		if v == version || v == "v"+version || normalizeVersion(v) == normalizedRequested {
			return &VersionInfo{Tag: v, Version: normalizeVersion(v)}, nil
		}
	}

	// Try fuzzy matching (e.g., "3.7" matches "3.7.0" but not "3.70")
	// Compare using normalized versions
	for _, v := range versions {
		normalizedV := normalizeVersion(v)
		if strings.HasPrefix(normalizedV, normalizedRequested+".") {
			return &VersionInfo{Tag: v, Version: normalizedV}, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for distribution %s", version, p.distribution)
}

// SourceDescription returns a human-readable source description
func (p *MetaCPANProvider) SourceDescription() string {
	return fmt.Sprintf("metacpan:%s", p.distribution)
}
