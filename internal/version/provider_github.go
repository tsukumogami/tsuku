package version

import (
	"context"
	"fmt"
	"strings"
)

// DefaultStableQualifiers names hyphenated suffixes that universally signal a
// stable release across upstream conventions. Any version whose prerelease
// component (per SemVer's splitPrerelease) is one of these is treated as
// stable. Recipes whose upstream uses an exotic qualifier override this list
// via [version] stable_qualifiers — the recipe's list replaces the default.
//
// Designed in docs/designs/DESIGN-prerelease-detection.md.
var DefaultStableQualifiers = []string{"release", "final", "lts", "ga", "stable"}

// GitHubProvider resolves versions from GitHub releases/tags.
// Implements both VersionResolver and VersionLister interfaces.
type GitHubProvider struct {
	resolver         *Resolver
	repo             string          // owner/repo format (e.g., "rust-lang/rust")
	tagPrefix        string          // optional prefix to filter tags (e.g., "ruby-")
	stableQualifiers map[string]bool // hyphenated suffixes treated as stable (lowercased)
}

// NewGitHubProvider creates a provider for GitHub-based tools.
// stableQualifiers names hyphenated suffixes the upstream uses as stable
// release qualifiers; pass nil to use DefaultStableQualifiers.
func NewGitHubProvider(resolver *Resolver, repo string, stableQualifiers []string) *GitHubProvider {
	return &GitHubProvider{
		resolver:         resolver,
		repo:             repo,
		stableQualifiers: buildStableQualifierSet(stableQualifiers),
	}
}

// NewGitHubProviderWithPrefix creates a provider that filters tags by prefix.
// The prefix is stripped from version strings (e.g., "ruby-3.3.10" -> "3.3.10").
// stableQualifiers names hyphenated suffixes the upstream uses as stable
// release qualifiers; pass nil to use DefaultStableQualifiers.
func NewGitHubProviderWithPrefix(resolver *Resolver, repo, tagPrefix string, stableQualifiers []string) *GitHubProvider {
	return &GitHubProvider{
		resolver:         resolver,
		repo:             repo,
		tagPrefix:        tagPrefix,
		stableQualifiers: buildStableQualifierSet(stableQualifiers),
	}
}

// buildStableQualifierSet converts a list of qualifier strings to a
// lowercased lookup set. nil or empty input falls back to
// DefaultStableQualifiers.
func buildStableQualifierSet(qualifiers []string) map[string]bool {
	if len(qualifiers) == 0 {
		qualifiers = DefaultStableQualifiers
	}
	set := make(map[string]bool, len(qualifiers))
	for _, q := range qualifiers {
		set[strings.ToLower(q)] = true
	}
	return set
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

// nonSemverUnstableMarkers names prerelease keywords that some upstreams
// embed directly into the version string without a hyphen separator
// (e.g., jq's "1.8.2rc1"). The SemVer-aware splitPrerelease check cannot
// catch these because there is no hyphen to split on; we fall back to a
// substring match for the non-SemVer case.
var nonSemverUnstableMarkers = []string{"alpha", "beta", "rc", "preview", "snapshot", "nightly", "dev"}

// isStableVersion checks if a version string represents a stable release.
//
// The check is two-layered:
//  1. SemVer prerelease (anything after the first hyphen): a non-empty
//     prerelease is unstable unless it matches one of the stableQualifiers
//     (case-insensitive exact match). This catches every SemVer-style
//     prerelease format (alpha, beta, rc, dev, M1, M2, ...) by construction
//     while admitting "this is the release" suffixes used by some
//     JVM-ecosystem upstreams (RELEASE, FINAL, LTS, GA, stable).
//  2. Non-SemVer prerelease (marker spliced into the version without a
//     hyphen, e.g., jq's "1.8.2rc1"): fall back to a substring match
//     against a small fixed keyword list. This preserves the historical
//     behavior for upstreams that don't follow SemVer's prerelease syntax.
//
// Designed in docs/designs/DESIGN-prerelease-detection.md.
func isStableVersion(version string, stableQualifiers map[string]bool) bool {
	_, prerelease := splitPrerelease(version)
	if prerelease != "" {
		return stableQualifiers[strings.ToLower(prerelease)]
	}
	lower := strings.ToLower(version)
	for _, marker := range nonSemverUnstableMarkers {
		if strings.Contains(lower, marker) {
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

	// Find the first stable version (skip prereleases that aren't whitelisted
	// as stable qualifiers).
	for _, v := range versions {
		if isStableVersion(v, p.stableQualifiers) {
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
