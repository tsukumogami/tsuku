package version

import (
	"context"
	"fmt"
	"strings"
)

// RubyGemsProvider resolves versions from RubyGems.org registry.
// Implements both VersionResolver and VersionLister interfaces.
type RubyGemsProvider struct {
	resolver *Resolver
	gemName  string
}

// NewRubyGemsProvider creates a provider for Ruby gems
func NewRubyGemsProvider(resolver *Resolver, gemName string) *RubyGemsProvider {
	return &RubyGemsProvider{
		resolver: resolver,
		gemName:  gemName,
	}
}

// ListVersions returns all available versions from RubyGems.org (newest first)
func (p *RubyGemsProvider) ListVersions(ctx context.Context) ([]string, error) {
	return p.resolver.ListRubyGemsVersions(ctx, p.gemName)
}

// ResolveLatest returns the latest version from RubyGems.org
func (p *RubyGemsProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return p.resolver.ResolveRubyGems(ctx, p.gemName)
}

// ResolveVersion resolves a specific version for RubyGems packages.
// Validates that the requested version exists.
// Supports fuzzy matching (e.g., "2.5" matches "2.5.33" but not "2.50.0")
func (p *RubyGemsProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list RubyGems versions: %w", err)
	}

	// Check for exact match
	for _, v := range versions {
		if v == version {
			return &VersionInfo{Tag: version, Version: version}, nil
		}
	}

	// Try fuzzy matching (e.g., "2.5" matches "2.5.33" but not "2.50.0")
	for _, v := range versions {
		if strings.HasPrefix(v, version+".") {
			return &VersionInfo{Tag: v, Version: v}, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for gem %s", version, p.gemName)
}

// SourceDescription returns a human-readable source description
func (p *RubyGemsProvider) SourceDescription() string {
	return fmt.Sprintf("rubygems:%s", p.gemName)
}
