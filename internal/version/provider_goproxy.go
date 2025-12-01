package version

import (
	"context"
	"fmt"
	"strings"
)

// GoProxyProvider resolves versions from proxy.golang.org for Go modules.
// Implements both VersionResolver and VersionLister interfaces.
//
// Go module versions follow the go.mod convention with "v" prefix:
// - Module versions: "v1.2.3" (with "v" prefix)
// - This is distinct from Go toolchain versions which have no prefix
type GoProxyProvider struct {
	resolver   *Resolver
	modulePath string
}

// NewGoProxyProvider creates a provider for Go module versions
func NewGoProxyProvider(resolver *Resolver, modulePath string) *GoProxyProvider {
	return &GoProxyProvider{
		resolver:   resolver,
		modulePath: modulePath,
	}
}

// ListVersions returns all available versions for the module (newest first)
func (p *GoProxyProvider) ListVersions(ctx context.Context) ([]string, error) {
	return p.resolver.ListGoProxyVersions(ctx, p.modulePath)
}

// ResolveLatest returns the latest version for the module
func (p *GoProxyProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return p.resolver.ResolveGoProxy(ctx, p.modulePath)
}

// ResolveVersion resolves a specific version for the module.
// Validates that the requested version exists.
// Accepts versions with or without "v" prefix.
func (p *GoProxyProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	// Normalize: ensure version has "v" prefix for comparison
	normalizedVersion := version
	if !strings.HasPrefix(version, "v") {
		normalizedVersion = "v" + version
	}

	// Validate that the requested version exists
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list versions for %s: %w", p.modulePath, err)
	}

	// Check if exact version exists
	for _, v := range versions {
		if v == normalizedVersion {
			return &VersionInfo{
				Tag:     normalizedVersion,
				Version: strings.TrimPrefix(normalizedVersion, "v"),
			}, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for module %s", version, p.modulePath)
}

// SourceDescription returns a human-readable source description
func (p *GoProxyProvider) SourceDescription() string {
	return "proxy.golang.org"
}
