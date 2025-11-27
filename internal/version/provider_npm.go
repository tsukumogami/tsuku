package version

import (
	"context"
	"fmt"
	"strings"
)

// NpmProvider resolves versions from npm registry.
// Implements both VersionResolver and VersionLister interfaces.
type NpmProvider struct {
	resolver    *Resolver
	packageName string
}

// NewNpmProvider creates a provider for npm-based tools
func NewNpmProvider(resolver *Resolver, packageName string) *NpmProvider {
	return &NpmProvider{
		resolver:    resolver,
		packageName: packageName,
	}
}

// ListVersions returns all available versions from npm registry (newest first)
func (p *NpmProvider) ListVersions(ctx context.Context) ([]string, error) {
	return p.resolver.ListNpmVersions(ctx, p.packageName)
}

// ResolveLatest returns the latest version from npm registry
func (p *NpmProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return p.resolver.ResolveNpm(ctx, p.packageName)
}

// ResolveVersion resolves a specific version for npm packages.
// Validates that the requested version exists in npm registry.
// Supports fuzzy matching (e.g., "1.2" matches "1.2.3")
func (p *NpmProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	// Validate that the requested version exists in npm registry
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list npm versions: %w", err)
	}

	// Check if exact version exists
	for _, v := range versions {
		if v == version {
			return &VersionInfo{Tag: version, Version: version}, nil
		}
	}

	// Try fuzzy matching (e.g., "1.2" matches "1.2.3" but not "1.20.0")
	// Note: HasPrefix with "." ensures we match "1.2.x" and not "1.20.x"
	// Examples:
	//   "1.2" + "." = "1.2." matches "1.2.3" ✓ but not "1.20.0" ✗
	//   "1" + "." = "1." matches "1.20.0" ✓ but not "10.0.0" ✗
	for _, v := range versions {
		if strings.HasPrefix(v, version+".") {
			return &VersionInfo{Tag: v, Version: v}, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for npm package %s", version, p.packageName)
}

// SourceDescription returns a human-readable source description
func (p *NpmProvider) SourceDescription() string {
	return fmt.Sprintf("npm:%s", p.packageName)
}
