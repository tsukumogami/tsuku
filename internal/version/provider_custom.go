package version

import (
	"context"
	"fmt"
)

// CustomProvider delegates to the registry system for custom sources.
// It only implements VersionResolver (not VersionLister) because custom
// sources may not have APIs to list all versions.
type CustomProvider struct {
	resolver *Resolver
	source   string
}

// NewCustomProvider creates a provider for custom version sources
func NewCustomProvider(resolver *Resolver, source string) *CustomProvider {
	return &CustomProvider{
		resolver: resolver,
		source:   source,
	}
}

// ResolveLatest returns the latest version using the custom registry
func (p *CustomProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return p.resolver.ResolveCustom(ctx, p.source)
}

// ResolveVersion resolves a specific version using the custom registry.
// Custom sources use the registry system which handles version resolution.
func (p *CustomProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	return p.resolver.ResolveCustomVersion(ctx, p.source, version)
}

// SourceDescription returns a human-readable source description
func (p *CustomProvider) SourceDescription() string {
	return fmt.Sprintf("custom:%s", p.source)
}

// Note: CustomProvider does NOT implement ListVersions() because not all custom
// sources have APIs to enumerate versions. The CLI will check if the provider
// implements VersionLister before calling ListVersions().
