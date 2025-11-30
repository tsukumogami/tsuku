package version

import (
	"context"
	"fmt"
	"strings"
)

// GoToolchainProvider resolves versions from go.dev/dl JSON API.
// Implements both VersionResolver and VersionLister interfaces.
//
// Go toolchain versions follow a different convention than Go modules:
// - Toolchain versions: "1.23.4" (no "v" prefix)
// - Module versions: "v1.2.3" (with "v" prefix)
type GoToolchainProvider struct {
	resolver *Resolver
}

// NewGoToolchainProvider creates a provider for Go toolchain versions
func NewGoToolchainProvider(resolver *Resolver) *GoToolchainProvider {
	return &GoToolchainProvider{
		resolver: resolver,
	}
}

// ListVersions returns all available stable Go versions (newest first)
func (p *GoToolchainProvider) ListVersions(ctx context.Context) ([]string, error) {
	return p.resolver.ListGoToolchainVersions(ctx)
}

// ResolveLatest returns the latest stable Go version
func (p *GoToolchainProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return p.resolver.ResolveGoToolchain(ctx)
}

// ResolveVersion resolves a specific version for Go toolchain.
// Validates that the requested version exists.
// Supports fuzzy matching (e.g., "1.23" matches "1.23.4")
func (p *GoToolchainProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	// Validate that the requested version exists
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list Go versions: %w", err)
	}

	// Check if exact version exists
	for _, v := range versions {
		if v == version {
			return &VersionInfo{Tag: version, Version: version}, nil
		}
	}

	// Try fuzzy matching (e.g., "1.23" matches "1.23.4" but not "1.24.0")
	// Note: HasPrefix with "." ensures we match "1.23.x" and not "1.230.x"
	for _, v := range versions {
		if strings.HasPrefix(v, version+".") {
			return &VersionInfo{Tag: v, Version: v}, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for Go toolchain", version)
}

// SourceDescription returns a human-readable source description
func (p *GoToolchainProvider) SourceDescription() string {
	return "go.dev/dl"
}
