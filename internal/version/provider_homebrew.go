package version

import (
	"context"
	"fmt"
	"strings"
)

// HomebrewProvider resolves versions from Homebrew API.
// Implements both VersionResolver and VersionLister interfaces.
type HomebrewProvider struct {
	resolver *Resolver
	formula  string
}

// NewHomebrewProvider creates a provider for Homebrew formulae
func NewHomebrewProvider(resolver *Resolver, formula string) *HomebrewProvider {
	return &HomebrewProvider{
		resolver: resolver,
		formula:  formula,
	}
}

// ListVersions returns available versions from Homebrew.
// Note: Homebrew only exposes the current stable version and versioned formulae,
// not historical versions.
func (p *HomebrewProvider) ListVersions(ctx context.Context) ([]string, error) {
	return p.resolver.ListHomebrewVersions(ctx, p.formula)
}

// ResolveLatest returns the latest stable version from Homebrew
func (p *HomebrewProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return p.resolver.ResolveHomebrew(ctx, p.formula)
}

// ResolveVersion resolves a specific version for Homebrew formulae.
// Validates that the requested version matches the available stable version.
// Supports fuzzy matching (e.g., "0.2" matches "0.2.5")
func (p *HomebrewProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	// Get available versions
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list Homebrew versions: %w", err)
	}

	// Check if exact version exists
	for _, v := range versions {
		if v == version {
			return &VersionInfo{Tag: version, Version: version}, nil
		}
	}

	// Try fuzzy matching (e.g., "0.2" matches "0.2.5")
	for _, v := range versions {
		if strings.HasPrefix(v, version+".") {
			return &VersionInfo{Tag: v, Version: v}, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for Homebrew formula %s", version, p.formula)
}

// SourceDescription returns a human-readable source description
func (p *HomebrewProvider) SourceDescription() string {
	return fmt.Sprintf("Homebrew:%s", p.formula)
}
