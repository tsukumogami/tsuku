package version

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/tsukumogami/tsuku/internal/version/pep440"
)

// PyPIProvider resolves versions from PyPI JSON API.
// Implements both VersionResolver and VersionLister interfaces.
//
// When pythonMajorMinor is non-empty, ResolveLatest and ListVersions
// filter the release list by per-release `requires_python` against the
// supplied Python major.minor. This is opt-in via NewPyPIProviderForPipx;
// the bare NewPyPIProvider constructor preserves today's "absolute
// latest" semantics for non-pipx callers.
//
// User-pinned versions (ResolveVersion) are never filtered — an explicit
// pin is authoritative.
type PyPIProvider struct {
	resolver         *Resolver
	packageName      string
	pythonMajorMinor string // empty when not constructed for pipx
}

// NewPyPIProvider creates a provider for PyPI packages with no
// Python-compat filtering. Behavior matches the pre-#2331 contract.
func NewPyPIProvider(resolver *Resolver, packageName string) *PyPIProvider {
	return &PyPIProvider{
		resolver:    resolver,
		packageName: packageName,
	}
}

// NewPyPIProviderForPipx creates a provider that filters releases by
// `requires_python` against pythonMajorMinor (e.g., "3.10"). Used by
// the version provider factory when constructing a PyPI provider for
// a `pipx_install` recipe step. ResolveLatest walks the release list
// newest-first and returns the first compatible release; ListVersions
// returns only compatible releases.
func NewPyPIProviderForPipx(resolver *Resolver, packageName, pythonMajorMinor string) *PyPIProvider {
	return &PyPIProvider{
		resolver:         resolver,
		packageName:      packageName,
		pythonMajorMinor: pythonMajorMinor,
	}
}

// ListVersions returns available versions from PyPI (newest first).
// When the provider was constructed with a Python major.minor, the
// list is filtered to releases compatible with that Python.
func (p *PyPIProvider) ListVersions(ctx context.Context) ([]string, error) {
	if p.pythonMajorMinor == "" {
		return p.resolver.ListPyPIVersions(ctx, p.packageName)
	}
	releases, err := p.resolver.listPyPIReleasesWithMetadata(ctx, p.packageName)
	if err != nil {
		return nil, err
	}
	target, err := pep440.ParseVersion(p.pythonMajorMinor)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "pypi",
			Message: fmt.Sprintf("invalid bundled Python version %q", p.pythonMajorMinor),
			Err:     err,
		}
	}
	filtered := make([]string, 0, len(releases))
	for _, r := range releases {
		if isPyPIPrerelease(r.Version) {
			continue
		}
		if isPyPIReleaseCompatible(r.RequiresPython, target) {
			filtered = append(filtered, r.Version)
		}
	}
	return filtered, nil
}

// ResolveLatest returns the latest PyPI release. When the provider has
// a Python major.minor, returns the newest release whose `requires_python`
// is satisfied by that Python; if none, returns a typed
// *ResolverError with Type == ErrTypeNoCompatibleRelease.
func (p *PyPIProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	if p.pythonMajorMinor == "" {
		return p.resolver.ResolvePyPI(ctx, p.packageName)
	}
	releases, err := p.resolver.listPyPIReleasesWithMetadata(ctx, p.packageName)
	if err != nil {
		return nil, err
	}
	if len(releases) == 0 {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "pypi",
			Message: fmt.Sprintf("no releases found for %s", p.packageName),
		}
	}
	target, err := pep440.ParseVersion(p.pythonMajorMinor)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "pypi",
			Message: fmt.Sprintf("invalid bundled Python version %q", p.pythonMajorMinor),
			Err:     err,
		}
	}
	for _, r := range releases {
		if isPyPIPrerelease(r.Version) {
			continue
		}
		if isPyPIReleaseCompatible(r.RequiresPython, target) {
			return &VersionInfo{Tag: r.Version, Version: r.Version}, nil
		}
	}
	// No compatible release. Surface a typed pre-flight error using
	// the absolute-latest release for context.
	latest := releases[0]
	return nil, &ResolverError{
		Type:   ErrTypeNoCompatibleRelease,
		Source: "pypi",
		Message: fmt.Sprintf(
			"no release of %s is compatible with bundled Python %s (latest is %s, requires Python %s)",
			p.packageName, p.pythonMajorMinor, latest.Version, pep440.Canonical(latest.RequiresPython),
		),
	}
}

// ResolveVersion resolves a specific version for PyPI packages. User
// pins are authoritative — this path is never filtered by
// pythonMajorMinor, regardless of how the provider was constructed.
// Validates that the requested version exists in PyPI; supports fuzzy
// matching (e.g., "1.2" matches "1.2.3").
func (p *PyPIProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	// Always use the unfiltered listing for user-pin lookups.
	versions, err := p.resolver.ListPyPIVersions(ctx, p.packageName)
	if err != nil {
		return nil, fmt.Errorf("failed to list PyPI versions: %w", err)
	}

	// Check if exact version exists
	if slices.Contains(versions, version) {
		return &VersionInfo{Tag: version, Version: version}, nil
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

// isPyPIReleaseCompatible reports whether a PyPI release with the
// given `requires_python` string is compatible with target.
//
// Empty/missing `requires_python` is treated as compatible (matches
// pip's behavior for legacy releases that predate PEP 345).
//
// Unparseable specifiers (including unsupported operators rejected
// by the pep440 evaluator, e.g., `~=`, `===`) cause the release to be
// treated as incompatible — conservative, since we can't verify
// satisfaction. The walker then continues with the next release.
func isPyPIReleaseCompatible(requiresPython string, target pep440.Version) bool {
	if strings.TrimSpace(requiresPython) == "" {
		return true
	}
	spec, err := pep440.ParseSpecifier(requiresPython)
	if err != nil {
		return false
	}
	return spec.Satisfies(target)
}

// isPyPIPrerelease reports whether v is a PEP 440 prerelease, dev,
// or post-release string (e.g., "2.17.9rc1", "1.0.0a1", "1.0.0b2",
// "1.0.0.dev1", "1.0.0.post1"). The check is purely textual — any
// alphabetic character after the leading numeric segments triggers
// the skip. Used to mirror pip's default behavior of preferring
// stable releases unless `--pre` is requested.
func isPyPIPrerelease(v string) bool {
	for i := 0; i < len(v); i++ {
		c := v[i]
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') {
			return true
		}
	}
	return false
}
