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
// Python-compat filtering. ResolveLatest returns the absolute-latest
// release from PyPI's `info.version`. For pipx_install callers that
// have a bundled-Python context, use NewPyPIProviderForPipx instead.
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
// is intentionally NOT filtered so user pins (`tsuku install foo@2`)
// route through the full version list.
//
// Panics if pythonMajorMinor is empty — call NewPyPIProvider instead
// when no Python context is available, so the silent-fallthrough trap
// (caller forgets to plumb pythonMajorMinor and gets unfiltered
// behavior) cannot fire.
func NewPyPIProviderForPipx(resolver *Resolver, packageName, pythonMajorMinor string) *PyPIProvider {
	if pythonMajorMinor == "" {
		panic("pep440: NewPyPIProviderForPipx requires non-empty pythonMajorMinor; use NewPyPIProvider for non-pipx callers")
	}
	return &PyPIProvider{
		resolver:         resolver,
		packageName:      packageName,
		pythonMajorMinor: pythonMajorMinor,
	}
}

// ListVersions returns all available versions from PyPI (newest first).
// The list is NOT filtered by pythonMajorMinor even when set —
// ListVersions is the path used by user pins (boundary-aware partial
// matches like `tsuku install foo@2`), and an explicit pin is
// authoritative even if it produces an incompatible install. Auto-
// resolution filtering happens inside ResolveLatest.
func (p *PyPIProvider) ListVersions(ctx context.Context) ([]string, error) {
	return p.resolver.ListPyPIVersions(ctx, p.packageName)
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
		// Skip yanked releases for auto-resolution. User pins still
		// surface yanked versions via ResolveVersion (the user-pin
		// path bypasses this method entirely).
		if r.Yanked {
			continue
		}
		// Skip PEP 440 prereleases (e.g., "2.17.9rc1") to match pip's
		// default behavior of preferring stable releases. .post and
		// .dev releases are not skipped — pip installs .post by
		// default, and .dev is only excluded by pip's --pre flag,
		// which tsuku does not expose.
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

// isPyPIPrerelease reports whether v carries a PEP 440 PRE-release
// suffix (a/alpha, b/beta, c/rc/pre/preview) — versions that pip
// excludes by default unless `--pre` is passed.
//
// Notably NOT prereleases under this rule:
//   - `.postN` (post-releases): pip installs these by default; they
//     represent packaging fixes to a final release.
//   - `.devN` (dev-releases): excluded by pip with `--pre`, but tsuku
//     does not expose `--pre`, and `.dev` releases are vanishingly
//     rare in real tsuku-curated tools. Treating them as stable here
//     errs toward "match pip's default" — if a recipe ever resolves
//     to a `.dev` release, the recipe author can pin via the
//     user-pin path or file an issue to harden this rule.
//
// The PEP 440 grammar places the pre-release marker immediately after
// the numeric release segments, optionally preceded by `.`, `-`, or
// `_` — e.g., "1.0a1", "1.0.a1", "1.0-a1", "1.0_a1", "1.0rc1".
// Implementation: scan past the dotted-numeric prefix, then check the
// first non-numeric character against the known pre-release markers.
func isPyPIPrerelease(v string) bool {
	// Skip the leading numeric / dot segments.
	i := 0
	for i < len(v) {
		c := v[i]
		if (c >= '0' && c <= '9') || c == '.' {
			i++
			continue
		}
		break
	}
	if i >= len(v) {
		return false
	}
	// Optional separator before the suffix.
	if v[i] == '-' || v[i] == '_' {
		i++
		if i >= len(v) {
			return false
		}
	}
	suffix := v[i:]
	// Lower-case the leading letters for matching.
	for _, prefix := range []string{"alpha", "beta", "preview", "pre", "rc", "a", "b", "c"} {
		if hasASCIIPrefixCI(suffix, prefix) {
			// Ensure the next character (if any) is a digit or
			// terminator — avoids classifying "1.0.cookie" or similar
			// false positives, though such versions don't occur in
			// real PyPI metadata.
			rest := suffix[len(prefix):]
			if rest == "" {
				return true
			}
			c := rest[0]
			if c >= '0' && c <= '9' {
				return true
			}
		}
	}
	return false
}

// hasASCIIPrefixCI reports whether s starts with prefix, comparing
// case-insensitively in the ASCII range only.
func hasASCIIPrefixCI(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := range len(prefix) {
		a := s[i]
		b := prefix[i]
		if a >= 'A' && a <= 'Z' {
			a += 'a' - 'A'
		}
		if a != b {
			return false
		}
	}
	return true
}
