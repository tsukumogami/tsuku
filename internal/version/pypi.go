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
	Releases map[string][]struct{} `json:"releases"` // All versions as keys
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
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: "failed to fetch package info",
			Err:     err,
		}
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
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "pypi",
			Message: "failed to fetch package info",
			Err:     err,
		}
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
