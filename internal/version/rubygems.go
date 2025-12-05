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
	// maxRubyGemsResponseSize limits response body to prevent memory exhaustion (10MB)
	// Most gems have <100KB metadata; 10MB is a safe upper bound
	maxRubyGemsResponseSize = 10 * 1024 * 1024
)

// RubyGems API response for versions endpoint
type rubyGemsVersion struct {
	Number     string `json:"number"`     // Version number (e.g., "4.3.3")
	Platform   string `json:"platform"`   // Platform (usually "ruby")
	Prerelease bool   `json:"prerelease"` // Whether it's a prerelease
}

// Pre-compile regex for gem name validation (performance)
var gemNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// isValidRubyGemsPackageName validates gem names
// Gem names: start with letter, alphanumeric + hyphens + underscores, max 100 chars
func isValidRubyGemsPackageName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	return gemNameRegex.MatchString(name)
}

// ResolveRubyGems fetches the latest version from RubyGems.org API
//
// API: https://rubygems.org/api/v1/versions/<gem>.json
// Returns: Latest stable version (excludes prereleases)
func (r *Resolver) ResolveRubyGems(ctx context.Context, gemName string) (*VersionInfo, error) {
	versions, err := r.ListRubyGemsVersions(ctx, gemName)
	if err != nil {
		return nil, err
	}

	if len(versions) == 0 {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "rubygems",
			Message: fmt.Sprintf("no versions found for gem %s", gemName),
		}
	}

	// First version is the latest (already sorted, excluding prereleases)
	latest := versions[0]
	return &VersionInfo{
		Tag:     latest,
		Version: latest,
	}, nil
}

// ListRubyGemsVersions fetches all available versions from RubyGems.org
//
// API: https://rubygems.org/api/v1/versions/<gem>.json
// Returns: Sorted list (newest first) of all stable versions for "ruby" platform
func (r *Resolver) ListRubyGemsVersions(ctx context.Context, gemName string) ([]string, error) {
	// Validate gem name to prevent injection
	if !isValidRubyGemsPackageName(gemName) {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "rubygems",
			Message: fmt.Sprintf("invalid gem name: %s", gemName),
		}
	}

	// Build URL using configured registry
	registryURL := r.rubygemsRegistryURL
	if registryURL == "" {
		registryURL = "https://rubygems.org"
	}

	baseURL, err := url.Parse(registryURL)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "rubygems",
			Message: "failed to parse base URL",
			Err:     err,
		}
	}

	// SECURITY: Enforce HTTPS
	if baseURL.Scheme != "https" {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "rubygems",
			Message: fmt.Sprintf("RubyGems registry must use HTTPS, got: %s", baseURL.Scheme),
		}
	}

	apiURL := baseURL.JoinPath("api", "v1", "versions", gemName+".json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "rubygems",
			Message: "failed to create request",
			Err:     err,
		}
	}

	// Set headers (User-Agent required for good API citizenship)
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Accept-Encoding", "identity") // Prevent compression attacks

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "rubygems", "failed to fetch gem info")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "rubygems",
			Message: fmt.Sprintf("gem %s not found on RubyGems.org", gemName),
		}
	}

	// Handle rate limiting
	if resp.StatusCode == 429 {
		return nil, &ResolverError{
			Type:    ErrTypeRateLimit,
			Source:  "rubygems",
			Message: "RubyGems.org rate limit exceeded",
		}
	}

	if resp.StatusCode != 200 {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "rubygems",
			Message: fmt.Sprintf("RubyGems.org returned status %d", resp.StatusCode),
		}
	}

	// SECURITY: Validate Content-Type to prevent MIME confusion
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "rubygems",
			Message: fmt.Sprintf("unexpected content-type: %s (expected application/json)", contentType),
		}
	}

	// Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxRubyGemsResponseSize)

	var response []rubyGemsVersion
	if err := json.NewDecoder(limitedReader).Decode(&response); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "rubygems",
			Message: "failed to parse RubyGems.org response",
			Err:     err,
		}
	}

	// Extract stable versions (exclude prereleases, filter to "ruby" platform)
	versions := make([]string, 0, len(response))
	for _, v := range response {
		if !v.Prerelease && v.Platform == "ruby" {
			versions = append(versions, v.Number)
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
