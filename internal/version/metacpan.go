package version

import (
	"bytes"
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
	// maxMetaCPANResponseSize limits response body to prevent memory exhaustion (10MB)
	// Most distributions have <100KB metadata; 10MB is a safe upper bound
	maxMetaCPANResponseSize = 10 * 1024 * 1024
)

// MetaCPAN API response for /release/{distribution} endpoint
type metacpanRelease struct {
	Distribution string `json:"distribution"` // Distribution name (e.g., "App-Ack")
	Version      string `json:"version"`      // Version number (e.g., "3.7.0")
	Author       string `json:"author"`       // CPAN author ID
	DownloadURL  string `json:"download_url"` // Download URL for tarball
	Status       string `json:"status"`       // Release status (e.g., "latest")
}

// MetaCPAN API response for POST /release/_search endpoint
type metacpanSearchResponse struct {
	Hits struct {
		Hits []struct {
			Source metacpanRelease `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

// Pre-compile regex for distribution name validation (performance)
// Distribution names: start with letter, can contain letters, numbers, hyphens
// Examples: App-Ack, Perl-Critic, File-Slurp
var distributionNameRegex = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*(-[A-Za-z0-9]+)*$`)

// isValidMetaCPANDistribution validates CPAN distribution names
// Distribution names: start with letter, contain letters/numbers/hyphens, max 128 chars
// Rejects module names containing "::" - those need conversion first
func isValidMetaCPANDistribution(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}

	// Reject module names (contain ::)
	if strings.Contains(name, "::") {
		return false
	}

	return distributionNameRegex.MatchString(name)
}

// normalizeModuleToDistribution converts a Perl module name to distribution format
// Example: App::Ack -> App-Ack
func normalizeModuleToDistribution(name string) string {
	return strings.ReplaceAll(name, "::", "-")
}

// ResolveMetaCPAN fetches the latest version from MetaCPAN API
//
// API: GET https://fastapi.metacpan.org/v1/release/{distribution}
// Returns: Latest stable version for the distribution
func (r *Resolver) ResolveMetaCPAN(ctx context.Context, distribution string) (*VersionInfo, error) {
	// Validate distribution name to prevent injection
	if !isValidMetaCPANDistribution(distribution) {
		// Check if it's a module name that needs conversion
		if strings.Contains(distribution, "::") {
			return nil, &ResolverError{
				Type:    ErrTypeValidation,
				Source:  "metacpan",
				Message: fmt.Sprintf("invalid distribution name: %s (use %s instead of module name)", distribution, normalizeModuleToDistribution(distribution)),
			}
		}
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "metacpan",
			Message: fmt.Sprintf("invalid distribution name: %s", distribution),
		}
	}

	// Build URL using configured registry
	registryURL := r.metacpanRegistryURL
	if registryURL == "" {
		registryURL = "https://fastapi.metacpan.org/v1"
	}

	baseURL, err := url.Parse(registryURL)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "metacpan",
			Message: "failed to parse base URL",
			Err:     err,
		}
	}

	// SECURITY: Enforce HTTPS
	if baseURL.Scheme != "https" {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "metacpan",
			Message: fmt.Sprintf("MetaCPAN registry must use HTTPS, got: %s", baseURL.Scheme),
		}
	}

	apiURL := baseURL.JoinPath("release", distribution)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "metacpan",
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
		return nil, WrapNetworkError(err, "metacpan", "failed to fetch distribution info")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "metacpan",
			Message: fmt.Sprintf("distribution %s not found on MetaCPAN", distribution),
		}
	}

	// Handle rate limiting
	if resp.StatusCode == 429 {
		return nil, &ResolverError{
			Type:    ErrTypeRateLimit,
			Source:  "metacpan",
			Message: "MetaCPAN rate limit exceeded",
		}
	}

	if resp.StatusCode != 200 {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "metacpan",
			Message: fmt.Sprintf("MetaCPAN returned status %d", resp.StatusCode),
		}
	}

	// SECURITY: Validate Content-Type to prevent MIME confusion
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "metacpan",
			Message: fmt.Sprintf("unexpected content-type: %s (expected application/json)", contentType),
		}
	}

	// Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxMetaCPANResponseSize)

	var release metacpanRelease
	if err := json.NewDecoder(limitedReader).Decode(&release); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "metacpan",
			Message: "failed to parse MetaCPAN response",
			Err:     err,
		}
	}

	if release.Version == "" {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "metacpan",
			Message: fmt.Sprintf("no version found for distribution %s", distribution),
		}
	}

	return &VersionInfo{
		Tag:     release.Version,
		Version: release.Version,
	}, nil
}

// ListMetaCPANVersions fetches all available versions from MetaCPAN
//
// API: POST https://fastapi.metacpan.org/v1/release/_search
// Returns: Sorted list (newest first) of all versions for the distribution
func (r *Resolver) ListMetaCPANVersions(ctx context.Context, distribution string) ([]string, error) {
	// Validate distribution name to prevent injection
	if !isValidMetaCPANDistribution(distribution) {
		// Check if it's a module name that needs conversion
		if strings.Contains(distribution, "::") {
			return nil, &ResolverError{
				Type:    ErrTypeValidation,
				Source:  "metacpan",
				Message: fmt.Sprintf("invalid distribution name: %s (use %s instead of module name)", distribution, normalizeModuleToDistribution(distribution)),
			}
		}
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "metacpan",
			Message: fmt.Sprintf("invalid distribution name: %s", distribution),
		}
	}

	// Build URL using configured registry
	registryURL := r.metacpanRegistryURL
	if registryURL == "" {
		registryURL = "https://fastapi.metacpan.org/v1"
	}

	baseURL, err := url.Parse(registryURL)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "metacpan",
			Message: "failed to parse base URL",
			Err:     err,
		}
	}

	// SECURITY: Enforce HTTPS
	if baseURL.Scheme != "https" {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "metacpan",
			Message: fmt.Sprintf("MetaCPAN registry must use HTTPS, got: %s", baseURL.Scheme),
		}
	}

	apiURL := baseURL.JoinPath("release", "_search")

	// Build Elasticsearch query for all releases of this distribution
	// We request up to 1000 versions (more than any reasonable distribution would have)
	query := map[string]interface{}{
		"query": map[string]interface{}{
			"term": map[string]interface{}{
				"distribution": distribution,
			},
		},
		"size":    1000,
		"_source": []string{"version", "status"},
		"sort": []map[string]interface{}{
			{"date": map[string]string{"order": "desc"}},
		},
	}

	queryBody, err := json.Marshal(query)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "metacpan",
			Message: "failed to build search query",
			Err:     err,
		}
	}

	req, err := http.NewRequestWithContext(ctx, "POST", apiURL.String(), bytes.NewReader(queryBody))
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "metacpan",
			Message: "failed to create request",
			Err:     err,
		}
	}

	// Set headers
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "identity") // Prevent compression attacks

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "metacpan", "failed to search versions")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "metacpan",
			Message: fmt.Sprintf("distribution %s not found on MetaCPAN", distribution),
		}
	}

	// Handle rate limiting
	if resp.StatusCode == 429 {
		return nil, &ResolverError{
			Type:    ErrTypeRateLimit,
			Source:  "metacpan",
			Message: "MetaCPAN rate limit exceeded",
		}
	}

	if resp.StatusCode != 200 {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "metacpan",
			Message: fmt.Sprintf("MetaCPAN returned status %d", resp.StatusCode),
		}
	}

	// SECURITY: Validate Content-Type to prevent MIME confusion
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "metacpan",
			Message: fmt.Sprintf("unexpected content-type: %s (expected application/json)", contentType),
		}
	}

	// Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxMetaCPANResponseSize)

	var searchResp metacpanSearchResponse
	if err := json.NewDecoder(limitedReader).Decode(&searchResp); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "metacpan",
			Message: "failed to parse MetaCPAN response",
			Err:     err,
		}
	}

	// Extract versions from search results, avoiding duplicates
	seen := make(map[string]bool)
	versions := make([]string, 0, len(searchResp.Hits.Hits))
	for _, hit := range searchResp.Hits.Hits {
		v := hit.Source.Version
		if v != "" && !seen[v] {
			seen[v] = true
			versions = append(versions, v)
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
