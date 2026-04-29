package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	// Max response size: 10MB (largest npm package metadata is ~17MB, PyPI is smaller)
	maxPyPIResponseSize = 10 * 1024 * 1024
)

// PyPI API response structure
type pypiPackageInfo struct {
	Info struct {
		Version string `json:"version"` // Latest version
		Name    string `json:"name"`
	} `json:"info"`
	// Releases maps version → file dicts. Each file dict carries its
	// own `requires_python`. In practice all files for a release share
	// the same value, so the first non-empty entry is authoritative.
	// Releases with no files (yanked-only, etc.) appear as an empty
	// slice and are still surfaced as version keys.
	Releases map[string][]pypiReleaseFile `json:"releases"`
}

// pypiReleaseFile carries the per-file metadata tsuku consumes from
// PyPI's JSON API. Only `requires_python` and `yanked` are read; other
// fields are intentionally unmodelled to keep the struct narrow.
type pypiReleaseFile struct {
	RequiresPython string `json:"requires_python"`
	Yanked         bool   `json:"yanked"`
}

// ResolvePyPI fetches the latest version from PyPI JSON API
//
// API: https://pypi.org/pypi/<package>/json
// Returns: Latest stable version from info.version
func (r *Resolver) ResolvePyPI(ctx context.Context, packageName string) (*VersionInfo, error) {
	// Validate package name (prevent injection)
	if !isValidPyPIPackageName(packageName) {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "pypi",
			Message: fmt.Sprintf("invalid PyPI package name: %s", packageName),
		}
	}

	// SECURITY: Use url.Parse for proper URL construction (not fmt.Sprintf)
	// Use r.pypiRegistryURL to support custom registries in tests
	registryURL := r.pypiRegistryURL
	if registryURL == "" {
		registryURL = "https://pypi.org" // Default if not set
	}

	baseURL, err := url.Parse(registryURL)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: "failed to parse base URL",
			Err:     err,
		}
	}
	apiURL := baseURL.JoinPath("pypi", packageName, "json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: "failed to create request",
			Err:     err,
		}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "pypi", "failed to fetch package info")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "pypi",
			Message: fmt.Sprintf("package %s not found on PyPI", packageName),
		}
	}

	if resp.StatusCode != 200 {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: fmt.Sprintf("unexpected status code: %d", resp.StatusCode),
		}
	}

	// SECURITY: Limit response size to prevent decompression bomb / memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxPyPIResponseSize)

	var pkgInfo pypiPackageInfo
	if err := json.NewDecoder(limitedReader).Decode(&pkgInfo); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "pypi",
			Message: "failed to parse PyPI response",
			Err:     err,
		}
	}

	version := pkgInfo.Info.Version
	if version == "" {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "pypi",
			Message: "no version found in PyPI response",
		}
	}

	return &VersionInfo{
		Tag:     version,
		Version: version,
	}, nil
}

// ListPyPIVersions fetches all available versions from PyPI
//
// Returns: Sorted list (newest first) of all release versions
func (r *Resolver) ListPyPIVersions(ctx context.Context, packageName string) ([]string, error) {
	// Validate package name
	if !isValidPyPIPackageName(packageName) {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "pypi",
			Message: fmt.Sprintf("invalid PyPI package name: %s", packageName),
		}
	}

	// SECURITY: Use url.Parse for proper URL construction
	// Use r.pypiRegistryURL to support custom registries in tests
	registryURL := r.pypiRegistryURL
	if registryURL == "" {
		registryURL = "https://pypi.org" // Default if not set
	}

	baseURL, err := url.Parse(registryURL)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: "failed to parse base URL",
			Err:     err,
		}
	}
	apiURL := baseURL.JoinPath("pypi", packageName, "json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: "failed to create request",
			Err:     err,
		}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "pypi", "failed to fetch package info")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	// SECURITY: Limit response size to prevent decompression bomb / memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxPyPIResponseSize)

	var pkgInfo pypiPackageInfo
	if err := json.NewDecoder(limitedReader).Decode(&pkgInfo); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "pypi",
			Message: "failed to parse PyPI response",
			Err:     err,
		}
	}

	// Extract version strings from releases map
	versions := make([]string, 0, len(pkgInfo.Releases))
	for version := range pkgInfo.Releases {
		versions = append(versions, version)
	}

	// Sort by semver (newest first) using existing compareVersions pattern
	sort.Slice(versions, func(i, j int) bool {
		// Try to parse as semver
		v1, err1 := semver.NewVersion(versions[i])
		v2, err2 := semver.NewVersion(versions[j])

		if err1 == nil && err2 == nil {
			// Both are valid semver, compare properly (reversed for newest first)
			return v2.LessThan(v1)
		}

		// Fall back to string comparison (reversed)
		return versions[i] > versions[j]
	})

	return versions, nil
}

// pypiRelease pairs a version string with the per-file metadata
// tsuku consumes. Used by PyPIProvider's pipx-aware paths to filter
// by `requires_python` and yanked status without making a second
// HTTP fetch.
type pypiRelease struct {
	Version        string
	RequiresPython string
	// Yanked is true when every file for this release is yanked.
	// Yanked releases are excluded from auto-resolution but remain
	// visible to user pins (matching pip's --no-pre-style semantics:
	// explicit is authoritative).
	Yanked bool
}

// listPyPIReleasesWithMetadata returns all releases newest-first with
// their `requires_python` string. Reuses the same fetch path and
// 10 MB response cap as ListPyPIVersions; the caller's tests can
// substitute the registry URL via Resolver.pypiRegistryURL.
func (r *Resolver) listPyPIReleasesWithMetadata(ctx context.Context, packageName string) ([]pypiRelease, error) {
	if !isValidPyPIPackageName(packageName) {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "pypi",
			Message: fmt.Sprintf("invalid PyPI package name: %s", packageName),
		}
	}

	registryURL := r.pypiRegistryURL
	if registryURL == "" {
		registryURL = "https://pypi.org"
	}
	baseURL, err := url.Parse(registryURL)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: "failed to parse base URL",
			Err:     err,
		}
	}
	apiURL := baseURL.JoinPath("pypi", packageName, "json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: "failed to create request",
			Err:     err,
		}
	}
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "pypi", "failed to fetch package info")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "pypi",
			Message: fmt.Sprintf("package %s not found on PyPI", packageName),
		}
	}
	if resp.StatusCode != 200 {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	limitedReader := io.LimitReader(resp.Body, maxPyPIResponseSize)
	var pkgInfo pypiPackageInfo
	if err := json.NewDecoder(limitedReader).Decode(&pkgInfo); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "pypi",
			Message: "failed to parse PyPI response",
			Err:     err,
		}
	}

	versions := make([]string, 0, len(pkgInfo.Releases))
	for v := range pkgInfo.Releases {
		versions = append(versions, v)
	}
	sort.Slice(versions, func(i, j int) bool {
		v1, err1 := semver.NewVersion(versions[i])
		v2, err2 := semver.NewVersion(versions[j])
		if err1 == nil && err2 == nil {
			return v2.LessThan(v1)
		}
		return versions[i] > versions[j]
	})

	releases := make([]pypiRelease, 0, len(versions))
	for _, v := range versions {
		// All files for a release share the same `requires_python` in
		// practice; take the first non-empty value as authoritative.
		// A release is yanked when all its files are yanked; partially-
		// yanked releases (rare) keep their non-yanked files available
		// and are not treated as yanked here.
		var rp string
		yanked := true
		files := pkgInfo.Releases[v]
		if len(files) == 0 {
			yanked = false // no files at all (some legacy entries) — leave selectable
		}
		for _, f := range files {
			if f.RequiresPython != "" && rp == "" {
				rp = f.RequiresPython
			}
			if !f.Yanked {
				yanked = false
			}
		}
		releases = append(releases, pypiRelease{Version: v, RequiresPython: rp, Yanked: yanked})
	}
	return releases, nil
}

// isValidPyPIPackageName validates PyPI package names
//
// PyPI allows: lowercase letters, numbers, hyphens, underscores, periods
// Length: 1-214 characters
// Pattern: ^[a-z0-9]([a-z0-9._-]*[a-z0-9])?$
//
// Security: Prevents command injection and path traversal
func isValidPyPIPackageName(name string) bool {
	if name == "" || len(name) > 214 {
		return false
	}

	// Reject dangerous patterns
	if strings.Contains(name, "..") || strings.Contains(name, "/") ||
		strings.Contains(name, "\\") || strings.HasPrefix(name, "-") {
		return false
	}

	// Check allowed characters (simplified)
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '.') {
			return false
		}
	}

	return true
}
