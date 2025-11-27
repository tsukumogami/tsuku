package version

import (
	"context"
	"fmt"
	"strings"
)

// GitHubProvider resolves versions from GitHub releases/tags.
// Implements both VersionResolver and VersionLister interfaces.
type GitHubProvider struct {
	resolver  *Resolver
	repo      string // owner/repo format (e.g., "rust-lang/rust")
	tagPrefix string // optional prefix to filter tags (e.g., "ruby-")
}

// NewGitHubProvider creates a provider for GitHub-based tools
func NewGitHubProvider(resolver *Resolver, repo string) *GitHubProvider {
	return &GitHubProvider{
		resolver: resolver,
		repo:     repo,
	}
}

// NewGitHubProviderWithPrefix creates a provider that filters tags by prefix
// The prefix is stripped from version strings (e.g., "ruby-3.3.10" -> "3.3.10")
func NewGitHubProviderWithPrefix(resolver *Resolver, repo, tagPrefix string) *GitHubProvider {
	return &GitHubProvider{
		resolver:  resolver,
		repo:      repo,
		tagPrefix: tagPrefix,
	}
}

// ListVersions returns all available versions from GitHub releases/tags (newest first)
func (p *GitHubProvider) ListVersions(ctx context.Context) ([]string, error) {
	versions, err := p.resolver.ListGitHubVersions(ctx, p.repo)
	if err != nil {
		return nil, err
	}

	// If no tag prefix, return as-is
	if p.tagPrefix == "" {
		return versions, nil
	}

	// Filter by prefix and strip it
	var filtered []string
	for _, v := range versions {
		if strings.HasPrefix(v, p.tagPrefix) {
			stripped := strings.TrimPrefix(v, p.tagPrefix)
			filtered = append(filtered, stripped)
		}
	}
	return filtered, nil
}

// isStableVersion checks if a version string represents a stable release
// Returns false for preview, alpha, beta, rc releases
func isStableVersion(version string) bool {
	lower := strings.ToLower(version)
	unstablePatterns := []string{"preview", "alpha", "beta", "rc", "dev", "snapshot", "nightly"}
	for _, pattern := range unstablePatterns {
		if strings.Contains(lower, pattern) {
			return false
		}
	}
	return true
}

// ResolveLatest returns the latest stable version from GitHub
func (p *GitHubProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	// If no tag prefix, use standard resolution
	if p.tagPrefix == "" {
		return p.resolver.ResolveGitHub(ctx, p.repo)
	}

	// With prefix, get filtered list and return first stable version
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, err
	}
	if len(versions) == 0 {
		return nil, fmt.Errorf("no versions found matching prefix %q", p.tagPrefix)
	}

	// Find the first stable version (skip preview, alpha, beta, rc releases)
	for _, v := range versions {
		if isStableVersion(v) {
			return &VersionInfo{
				Version: v,
				Tag:     p.tagPrefix + v,
			}, nil
		}
	}

	// Fallback to first version if no stable version found
	return &VersionInfo{
		Version: versions[0],
		Tag:     p.tagPrefix + versions[0],
	}, nil
}

// ResolveVersion resolves a specific version constraint for GitHub releases.
// Handles fuzzy matching (e.g., "1.29" -> "1.29.3")
func (p *GitHubProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	// If no tag prefix, use standard resolution
	if p.tagPrefix == "" {
		return p.resolver.ResolveGitHubVersion(ctx, p.repo, version)
	}

	// With prefix, check if exact version exists in filtered list
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, err
	}

	// Try exact match first
	for _, v := range versions {
		if v == version {
			return &VersionInfo{
				Version: v,
				Tag:     p.tagPrefix + v,
			}, nil
		}
	}

	// Try prefix match (fuzzy)
	for _, v := range versions {
		if strings.HasPrefix(v, version) {
			return &VersionInfo{
				Version: v,
				Tag:     p.tagPrefix + v,
			}, nil
		}
	}

	return nil, fmt.Errorf("version %q not found in %s (with prefix %q)", version, p.repo, p.tagPrefix)
}

// SourceDescription returns a human-readable source description
func (p *GitHubProvider) SourceDescription() string {
	return fmt.Sprintf("GitHub:%s", p.repo)
}
