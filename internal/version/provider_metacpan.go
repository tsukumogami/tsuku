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
func (p *MetaCPANProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list MetaCPAN versions: %w", err)
	}

	// Check for exact match
	for _, v := range versions {
		if v == version {
			return &VersionInfo{Tag: version, Version: version}, nil
		}
	}

	// Try fuzzy matching (e.g., "3.7" matches "3.7.0" but not "3.70")
	for _, v := range versions {
		if strings.HasPrefix(v, version+".") {
			return &VersionInfo{Tag: v, Version: v}, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for distribution %s", version, p.distribution)
}

// SourceDescription returns a human-readable source description
func (p *MetaCPANProvider) SourceDescription() string {
	return fmt.Sprintf("metacpan:%s", p.distribution)
}
