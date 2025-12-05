package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	// maxCratesIOResponseSize limits response body to prevent memory exhaustion (10MB)
	maxCratesIOResponseSize = 10 * 1024 * 1024
)

// crates.io API response structures
type cratesIOVersionsResponse struct {
	Versions []cratesIOVersion `json:"versions"`
	// Meta contains pagination info (total, next_page) - not currently used
}

type cratesIOVersion struct {
	Num    string `json:"num"`    // Version number (e.g., "0.18.4")
	Yanked bool   `json:"yanked"` // Whether the version was yanked
}

// Pre-compile regex for crate name validation (performance)
var crateNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// isValidCratesIOPackageName validates crate names
// Crate names: start with letter, alphanumeric + hyphens + underscores, max 64 chars
func isValidCratesIOPackageName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	return crateNameRegex.MatchString(name)
}

// ResolveCratesIO fetches the latest version from crates.io API
//
// API: https://crates.io/api/v1/crates/<crate>/versions
// Returns: Latest non-yanked version
func (r *Resolver) ResolveCratesIO(ctx context.Context, crateName string) (*VersionInfo, error) {
	versions, err := r.ListCratesIOVersions(ctx, crateName)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "crates_io",
			Message: fmt.Sprintf("no versions found for crate %s", crateName),
		}
	}

	// First version is the latest (already sorted)
	latest := versions[0]
	return &VersionInfo{
		Tag:     latest,
		Version: latest,
	}, nil
}

// ListCratesIOVersions fetches all available versions from crates.io
//
// Returns: Sorted list (newest first) of all non-yanked versions
func (r *Resolver) ListCratesIOVersions(ctx context.Context, crateName string) ([]string, error) {
	// Validate crate name to prevent injection
	if !isValidCratesIOPackageName(crateName) {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "crates_io",
			Message: fmt.Sprintf("invalid crate name: %s", crateName),
		}
	}

	// Build URL using configured registry
	registryURL := r.cratesIORegistryURL
	if registryURL == "" {
		registryURL = "https://crates.io" // Default
	}

	baseURL, err := url.Parse(registryURL)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "crates_io",
			Message: "failed to parse base URL",
			Err:     err,
		}
	}
	apiURL := baseURL.JoinPath("api", "v1", "crates", crateName, "versions")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "crates_io",
			Message: "failed to create request",
			Err:     err,
		}
	}

	// crates.io requires a User-Agent header (mandatory per their API policy)
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")
	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "crates_io", "failed to fetch crate info")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "crates_io",
			Message: fmt.Sprintf("crate %s not found on crates.io", crateName),
		}
	}

	// Handle rate limiting
	if resp.StatusCode == 429 {
		return nil, &ResolverError{
			Type:    ErrTypeRateLimit,
			Source:  "crates_io",
			Message: "crates.io rate limit exceeded",
		}
	}

	if resp.StatusCode != 200 {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "crates_io",
			Message: fmt.Sprintf("crates.io returned status %d", resp.StatusCode),
		}
	}

	// SECURITY: Validate Content-Type to prevent MIME confusion
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "crates_io",
			Message: fmt.Sprintf("unexpected content-type: %s (expected application/json)", contentType),
		}
	}

	// Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxCratesIOResponseSize)

	var response cratesIOVersionsResponse
	if err := json.NewDecoder(limitedReader).Decode(&response); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "crates_io",
			Message: "failed to parse crates.io response",
			Err:     err,
		}
	}

	// Extract non-yanked versions
	versions := make([]string, 0, len(response.Versions))
	for _, v := range response.Versions {
		if !v.Yanked {
			versions = append(versions, v.Num)
		}
	}

	// Sort by semver (newest first)
	sort.Slice(versions, func(i, j int) bool {
		v1, err1 := semver.NewVersion(versions[i])
		v2, err2 := semver.NewVersion(versions[j])

		if err1 == nil && err2 == nil {
			return v2.LessThan(v1) // Newest first
		}

		// Fall back to string comparison (reversed for newest first)
		return versions[i] > versions[j]
	})

	return versions, nil
}

// CratesIOProvider resolves versions from crates.io registry.
// Implements both VersionResolver and VersionLister interfaces.
type CratesIOProvider struct {
	resolver  *Resolver
	crateName string
}

// NewCratesIOProvider creates a provider for Rust crates
func NewCratesIOProvider(resolver *Resolver, crateName string) *CratesIOProvider {
	return &CratesIOProvider{
		resolver:  resolver,
		crateName: crateName,
	}
}

// ListVersions returns all available versions from crates.io (newest first)
func (p *CratesIOProvider) ListVersions(ctx context.Context) ([]string, error) {
	return p.resolver.ListCratesIOVersions(ctx, p.crateName)
}

// ResolveLatest returns the latest version from crates.io
func (p *CratesIOProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return p.resolver.ResolveCratesIO(ctx, p.crateName)
}

// ResolveVersion resolves a specific version for crates.io packages.
// Validates that the requested version exists.
// Supports fuzzy matching (e.g., "0.18" matches "0.18.4")
func (p *CratesIOProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	versions, err := p.ListVersions(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list crates.io versions: %w", err)
	}

	// Check for exact match
	for _, v := range versions {
		if v == version {
			return &VersionInfo{Tag: version, Version: version}, nil
		}
	}

	// Try fuzzy matching (e.g., "0.18" matches "0.18.4" but not "0.180.0")
	for _, v := range versions {
		if strings.HasPrefix(v, version+".") {
			return &VersionInfo{Tag: v, Version: v}, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for crate %s", version, p.crateName)
}

// SourceDescription returns a human-readable source description
func (p *CratesIOProvider) SourceDescription() string {
	return fmt.Sprintf("crates.io:%s", p.crateName)
}
