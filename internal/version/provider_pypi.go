package version

import (
	"context"
	"fmt"
	"strings"
)

// PyPIProvider resolves versions from PyPI JSON API.
// Implements both VersionResolver and VersionLister interfaces.
type PyPIProvider struct {
	resolver    *Resolver
	packageName string
}

// NewPyPIProvider creates a provider for PyPI packages
func NewPyPIProvider(resolver *Resolver, packageName string) *PyPIProvider {
	return &PyPIProvider{
		resolver:    resolver,
		packageName: packageName,
	}
}

// ListVersions returns all available versions from PyPI (newest first)
func (p *PyPIProvider) ListVersions(ctx context.Context) ([]string, error) {
	return p.resolver.ListPyPIVersions(ctx, p.packageName)
}

// ResolveLatest returns the latest version from PyPI
func (p *PyPIProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return p.resolver.ResolvePyPI(ctx, p.packageName)
}

// ResolveVersion resolves a specific version for PyPI packages.
// Validates that the requested version exists in PyPI.
// Supports fuzzy matching (e.g., "1.2" matches "1.2.3")
func (p *PyPIProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	// Validate that the requested version exists in PyPI
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list PyPI versions: %w", err)
	}

	// Check if exact version exists
	for _, v := range versions {
		if v == version {
			return &VersionInfo{Tag: version, Version: version}, nil
		}
	}

	// Try fuzzy matching (e.g., "1.2" matches "1.2.3" but not "1.20.0")
	for _, v := range versions {
		if strings.HasPrefix(v, version+".") {
			return &VersionInfo{Tag: v, Version: v}, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for PyPI package %s", version, p.packageName)
}

// SourceDescription returns a human-readable source description
func (p *PyPIProvider) SourceDescription() string {
	return fmt.Sprintf("PyPI:%s", p.packageName)
}
