package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"

	"github.com/tsukumogami/tsuku/internal/config"
	"github.com/tsukumogami/tsuku/internal/httputil"
)

// VersionInfo contains both the original tag and normalized version
type VersionInfo struct {
	Tag     string // Original tag (e.g., "v1.2.3" or "1.2.3")
	Version string // Normalized version (e.g., "1.2.3")
}

// Resolver resolves versions for different sources
type Resolver struct {
	client              *github.Client // GitHub API client
	httpClient          *http.Client   // HTTP client for non-GitHub requests (injectable for testing)
	registry            *Registry      // Custom version source registry
	npmRegistryURL      string         // npm registry URL (injectable for testing)
	pypiRegistryURL     string         // PyPI registry URL (injectable for testing)
	cratesIORegistryURL string         // crates.io registry URL (injectable for testing)
	rubygemsRegistryURL string         // RubyGems.org registry URL (injectable for testing)
	metacpanRegistryURL string         // MetaCPAN registry URL (injectable for testing)
	homebrewRegistryURL string         // Homebrew API URL (injectable for testing)
	goDevURL            string         // go.dev URL (injectable for testing)
	goProxyURL          string         // Go module proxy URL (injectable for testing)
	authenticated       bool           // Whether GitHub requests are authenticated
}

// NewHTTPClient creates an HTTP client with security hardening and proper timeouts.
// The timeout is configurable via TSUKU_API_TIMEOUT environment variable (default: 30s).
//
// Security features:
//   - DisableCompression: true - prevents decompression bomb attacks
//   - SSRF protection via redirect validation (blocks private, loopback, link-local IPs)
//   - DNS rebinding protection (resolves hostnames and validates all IPs)
//   - HTTPS-only redirects
//   - Redirect chain limit (5 redirects max)
//
// This function is exported for use by other packages that need secure HTTP clients.
// It wraps httputil.NewSecureClient with the version resolver's default options.
func NewHTTPClient() *http.Client {
	return httputil.NewSecureClient(httputil.ClientOptions{
		Timeout:      config.GetAPITimeout(),
		DialTimeout:  10 * time.Second,
		MaxRedirects: 5,
	})
}

// validateIP checks if an IP is allowed (not private, loopback, link-local, etc.)
// This is a thin wrapper around httputil.ValidateIP for internal use and testing.
func validateIP(ip net.IP, host string) error {
	return httputil.ValidateIP(ip, host)
}

// New creates a new version resolver with optional configuration.
// If GITHUB_TOKEN environment variable is set, it will be used for authenticated requests.
// Options can be used to override default registry URLs for testing.
func New(opts ...Option) *Resolver {
	var githubHTTPClient *http.Client
	authenticated := false

	// Check for GitHub token in environment
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		githubHTTPClient = oauth2.NewClient(context.Background(), ts)
		authenticated = true
	}

	r := &Resolver{
		client:              github.NewClient(githubHTTPClient),
		httpClient:          NewHTTPClient(),                   // HTTP client with proper timeouts
		registry:            NewRegistry(),                     // Initialize with default resolvers
		npmRegistryURL:      "https://registry.npmjs.org",      // Production default
		pypiRegistryURL:     "https://pypi.org",                // Production default
		cratesIORegistryURL: "https://crates.io",               // Production default
		rubygemsRegistryURL: "https://rubygems.org",            // Production default
		metacpanRegistryURL: "https://fastapi.metacpan.org/v1", // Production default
		goDevURL:            "https://go.dev",                  // Production default
		goProxyURL:          "https://proxy.golang.org",        // Production default
		authenticated:       authenticated,
	}

	// Apply options
	for _, opt := range opts {
		opt(r)
	}

	return r
}

// wrapGitHubRateLimitError converts a GitHub API rate limit error to a GitHubRateLimitError
// with detailed information for the user. Returns nil if the error is not a rate limit error.
// The context parameter describes what operation was being performed (e.g., version resolution).
func (r *Resolver) wrapGitHubRateLimitError(err error, context GitHubRateLimitContext) *GitHubRateLimitError {
	var rateLimitErr *github.RateLimitError
	if errors.As(err, &rateLimitErr) {
		return &GitHubRateLimitError{
			Limit:         rateLimitErr.Rate.Limit,
			Remaining:     rateLimitErr.Rate.Remaining,
			ResetTime:     rateLimitErr.Rate.Reset.Time,
			Authenticated: r.authenticated,
			Context:       context,
			Err:           err,
		}
	}
	return nil
}

// ResolveGitHub resolves the latest version from a GitHub repository
// Falls back to tags API if releases API returns 404 (some repos use tags without releases)
func (r *Resolver) ResolveGitHub(ctx context.Context, repo string) (*VersionInfo, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s (expected owner/repo)", repo)
	}
	owner, repoName := parts[0], parts[1]

	release, _, err := r.client.Repositories.GetLatestRelease(ctx, owner, repoName)
	if err != nil {
		// Check for rate limit errors first
		if rateLimitErr := r.wrapGitHubRateLimitError(err, GitHubContextVersionResolution); rateLimitErr != nil {
			return nil, rateLimitErr
		}

		// Handle network errors gracefully
		if strings.Contains(err.Error(), "network is unreachable") ||
			strings.Contains(err.Error(), "no such host") ||
			strings.Contains(err.Error(), "dial tcp") {
			return nil, fmt.Errorf("network unavailable: %w", err)
		}

		// If 404, repository may use tags without releases (e.g., golang/go)
		// Fall back to listing tags
		if strings.Contains(err.Error(), "404") {
			return r.resolveFromTags(ctx, owner, repoName)
		}

		return nil, fmt.Errorf("failed to get latest release: %w", err)
	}

	tag := *release.TagName
	return &VersionInfo{
		Tag:     tag,
		Version: normalizeVersion(tag),
	}, nil
}

// resolveFromTags resolves version from repository tags when releases aren't available
func (r *Resolver) resolveFromTags(ctx context.Context, owner, repoName string) (*VersionInfo, error) {
	// Fetch multiple pages of tags to find valid versions
	// golang/go has ~500 tags with go1.x tags appearing later in the list
	var allTags []*github.RepositoryTag
	opts := &github.ListOptions{PerPage: 100}

	// Fetch up to 500 tags (5 pages)
	for page := 1; page <= 5; page++ {
		opts.Page = page
		tags, _, err := r.client.Repositories.ListTags(ctx, owner, repoName, opts)
		if err != nil {
			// Check for rate limit errors first
			if rateLimitErr := r.wrapGitHubRateLimitError(err, GitHubContextVersionResolution); rateLimitErr != nil {
				return nil, rateLimitErr
			}
			return nil, fmt.Errorf("failed to list tags: %w", err)
		}

		if len(tags) == 0 {
			break // No more tags
		}

		allTags = append(allTags, tags...)

		// Early exit if we have enough tags with valid versions
		if len(allTags) >= 100 {
			// Check if we have any valid version tags before continuing
			hasValidTag := false
			for _, tag := range allTags {
				if tag.Name != nil {
					normalized := normalizeVersion(*tag.Name)
					if normalized != "" && isValidVersion(normalized) &&
						!strings.Contains(*tag.Name, "weekly") {
						hasValidTag = true
						break
					}
				}
			}
			if hasValidTag {
				break // We have valid tags, stop fetching
			}
		}
	}

	if len(allTags) == 0 {
		return nil, fmt.Errorf("no tags found for %s/%s", owner, repoName)
	}

	// Find latest semantic version tag
	// For repos like golang/go, filter for "go1.x.x" pattern
	var latestTag string
	var latestVersion string

	for _, tag := range allTags {
		if tag.Name == nil {
			continue
		}
		tagName := *tag.Name

		// Skip obvious non-release tags
		if strings.Contains(tagName, "weekly") ||
			strings.Contains(strings.ToLower(tagName), "beta") ||
			strings.Contains(strings.ToLower(tagName), "-rc") {
			continue
		}

		// Normalize and compare versions
		normalized := normalizeVersion(tagName)

		// Skip if normalization resulted in empty string or invalid version
		if normalized == "" || !isValidVersion(normalized) {
			continue
		}

		if latestVersion == "" || compareVersions(normalized, latestVersion) > 0 {
			latestVersion = normalized
			latestTag = tagName
		}
	}

	if latestTag == "" {
		return nil, fmt.Errorf("no valid version tags found for %s/%s", owner, repoName)
	}

	return &VersionInfo{
		Tag:     latestTag,
		Version: latestVersion,
	}, nil
}

// ResolveGitHubVersion resolves a specific version/tag from a GitHub repository
func (r *Resolver) ResolveGitHubVersion(ctx context.Context, repo, version string) (*VersionInfo, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s (expected owner/repo)", repo)
	}
	// owner, repoName := parts[0], parts[1]

	// Try to find the release by tag
	// Note: GitHub API expects "tags/v1.0.0" or just "v1.0.0" depending on how it was created
	// We'll try to list tags and find a match if direct lookup fails or if we need to fuzzy match

	// First, try to list tags to find a match
	tags, err := r.ListGitHubVersions(ctx, repo)
	if err != nil {
		return nil, err
	}

	// Look for exact match or match with "v" prefix
	for _, t := range tags {
		if t == version || t == "v"+version || normalizeVersion(t) == version {
			return &VersionInfo{
				Tag:     t,
				Version: normalizeVersion(t),
			}, nil
		}
	}

	return nil, fmt.Errorf("version %s not found for %s", version, repo)
}

// ListGitHubVersions lists available versions (tags) for a GitHub repository
func (r *Resolver) ListGitHubVersions(ctx context.Context, repo string) ([]string, error) {
	parts := strings.Split(repo, "/")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid repo format: %s (expected owner/repo)", repo)
	}
	owner, repoName := parts[0], parts[1]

	opts := &github.ListOptions{PerPage: 100}
	tags, _, err := r.client.Repositories.ListTags(ctx, owner, repoName, opts)
	if err != nil {
		// Check for rate limit errors first
		if rateLimitErr := r.wrapGitHubRateLimitError(err, GitHubContextVersionResolution); rateLimitErr != nil {
			return nil, rateLimitErr
		}
		// Handle network errors gracefully
		if strings.Contains(err.Error(), "network is unreachable") ||
			strings.Contains(err.Error(), "no such host") ||
			strings.Contains(err.Error(), "dial tcp") {
			return nil, fmt.Errorf("network unavailable: %w", err)
		}
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}

	var versions []string
	for _, tag := range tags {
		if tag.Name != nil {
			versions = append(versions, *tag.Name)
		}
	}

	return versions, nil
}

// ResolveHashiCorp resolves the latest version from HashiCorp releases
// For now, this is a placeholder - real implementation would query releases.hashicorp.com
func (r *Resolver) ResolveHashiCorp(ctx context.Context, product string) (string, error) {
	// Placeholder: In production, this would fetch from https://releases.hashicorp.com/{product}/
	// For validation, we'll use common known versions as fallback
	knownVersions := map[string]string{
		"terraform": "1.6.6",
		"vault":     "1.15.4",
		"consul":    "1.17.1",
		"nomad":     "1.7.2",
		"packer":    "1.10.0",
		"boundary":  "0.15.0",
		"waypoint":  "0.11.4",
		"vagrant":   "2.4.0",
	}

	if version, ok := knownVersions[product]; ok {
		return version, nil
	}

	return "", fmt.Errorf("unknown HashiCorp product: %s (version resolution not implemented)", product)
}

// ResolveNodeJS resolves the latest LTS version from Node.js dist site
func (r *Resolver) ResolveNodeJS(ctx context.Context) (*VersionInfo, error) {
	// Node.js publishes version info at https://nodejs.org/dist/index.json
	url := "https://nodejs.org/dist/index.json"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "network is unreachable") ||
			strings.Contains(err.Error(), "no such host") ||
			strings.Contains(err.Error(), "dial tcp") {
			return nil, fmt.Errorf("network unavailable: %w", err)
		}
		return nil, fmt.Errorf("failed to fetch Node.js versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Node.js dist site returned status %d", resp.StatusCode)
	}

	var versions []struct {
		Version string      `json:"version"`
		LTS     interface{} `json:"lts"` // Can be string (LTS name) or false
	}

	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Find the latest LTS version
	for _, v := range versions {
		if v.LTS != nil && v.LTS != false {
			// Strip "v" prefix from version
			version := normalizeVersion(v.Version)
			return &VersionInfo{
				Tag:     v.Version, // Keep "v" prefix for URL
				Version: version,   // Without "v" for display
			}, nil
		}
	}

	// Fallback: use the latest version (first in list)
	if len(versions) > 0 {
		version := normalizeVersion(versions[0].Version)
		return &VersionInfo{
			Tag:     versions[0].Version,
			Version: version,
		}, nil
	}

	return nil, fmt.Errorf("no Node.js versions found")
}

// Go toolchain API response structure
type goRelease struct {
	Version string `json:"version"` // e.g., "go1.23.4"
	Stable  bool   `json:"stable"`
}

const (
	// Max response size for go.dev/dl API (10MB should be plenty)
	maxGoToolchainResponseSize = 10 * 1024 * 1024
)

// ResolveGoToolchain fetches the latest stable Go version from go.dev/dl
//
// API: https://go.dev/dl/?mode=json
// Returns: Latest stable version without "go" prefix (e.g., "1.23.4")
func (r *Resolver) ResolveGoToolchain(ctx context.Context) (*VersionInfo, error) {
	goDevURL := r.goDevURL
	if goDevURL == "" {
		goDevURL = "https://go.dev" // Default if not set
	}
	apiURL := goDevURL + "/dl/?mode=json"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "go_toolchain",
			Message: "failed to create request",
			Err:     err,
		}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "go_toolchain", "failed to fetch Go releases")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "go_toolchain",
			Message: fmt.Sprintf("go.dev returned status %d", resp.StatusCode),
		}
	}

	// Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxGoToolchainResponseSize)

	var releases []goRelease
	if err := json.NewDecoder(limitedReader).Decode(&releases); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "go_toolchain",
			Message: "failed to parse go.dev response",
			Err:     err,
		}
	}

	// Find the first stable release (list is ordered newest first)
	for _, release := range releases {
		if release.Stable {
			version := normalizeGoToolchainVersion(release.Version)
			if version == "" {
				continue
			}
			return &VersionInfo{
				Tag:     version, // Go toolchain uses version without "v" prefix
				Version: version,
			}, nil
		}
	}

	return nil, &ResolverError{
		Type:    ErrTypeNotFound,
		Source:  "go_toolchain",
		Message: "no stable Go releases found",
	}
}

// ListGoToolchainVersions fetches all available stable Go versions from go.dev/dl
//
// Returns: Sorted list (newest first) of all stable versions without "go" prefix
func (r *Resolver) ListGoToolchainVersions(ctx context.Context) ([]string, error) {
	goDevURL := r.goDevURL
	if goDevURL == "" {
		goDevURL = "https://go.dev" // Default if not set
	}
	apiURL := goDevURL + "/dl/?mode=json"

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "go_toolchain",
			Message: "failed to create request",
			Err:     err,
		}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "go_toolchain", "failed to fetch Go releases")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "go_toolchain",
			Message: fmt.Sprintf("go.dev returned status %d", resp.StatusCode),
		}
	}

	// Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxGoToolchainResponseSize)

	var releases []goRelease
	if err := json.NewDecoder(limitedReader).Decode(&releases); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "go_toolchain",
			Message: "failed to parse go.dev response",
			Err:     err,
		}
	}

	// Extract stable versions (API returns them in newest-first order)
	var versions []string
	for _, release := range releases {
		if release.Stable {
			version := normalizeGoToolchainVersion(release.Version)
			if version != "" {
				versions = append(versions, version)
			}
		}
	}

	return versions, nil
}

// normalizeGoToolchainVersion strips the "go" prefix from Go version strings
// e.g., "go1.23.4" -> "1.23.4"
func normalizeGoToolchainVersion(version string) string {
	return strings.TrimPrefix(version, "go")
}

// Go module proxy API response structure
type goModuleInfo struct {
	Version string `json:"Version"` // e.g., "v1.64.8"
	Time    string `json:"Time"`    // e.g., "2025-03-17T16:54:02Z"
}

const (
	// Max response size for Go proxy API (1MB should be plenty for version lists)
	maxGoProxyResponseSize = 1 * 1024 * 1024
)

// encodeModulePath encodes a Go module path for use in proxy URLs.
// Uppercase letters are replaced with '!' followed by the lowercase letter.
// e.g., "github.com/User/Repo" -> "github.com/!user/!repo"
func encodeModulePath(path string) string {
	var result strings.Builder
	for _, r := range path {
		if r >= 'A' && r <= 'Z' {
			result.WriteRune('!')
			result.WriteRune(r + 32) // Convert to lowercase
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
}

// ResolveGoProxy fetches the latest version of a Go module from proxy.golang.org
//
// API: https://proxy.golang.org/{module}/@latest
// Returns: Version with "v" prefix (e.g., "v1.64.8")
func (r *Resolver) ResolveGoProxy(ctx context.Context, modulePath string) (*VersionInfo, error) {
	goProxyURL := r.goProxyURL
	if goProxyURL == "" {
		goProxyURL = "https://proxy.golang.org" // Default if not set
	}

	encodedPath := encodeModulePath(modulePath)
	apiURL := fmt.Sprintf("%s/%s/@latest", goProxyURL, encodedPath)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "goproxy",
			Message: "failed to create request",
			Err:     err,
		}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "goproxy", "failed to fetch module info")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "goproxy",
			Message: fmt.Sprintf("module %s not found", modulePath),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "goproxy",
			Message: fmt.Sprintf("proxy.golang.org returned status %d", resp.StatusCode),
		}
	}

	// Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxGoProxyResponseSize)

	var info goModuleInfo
	if err := json.NewDecoder(limitedReader).Decode(&info); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "goproxy",
			Message: "failed to parse proxy response",
			Err:     err,
		}
	}

	if info.Version == "" {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "goproxy",
			Message: "no version found in response",
		}
	}

	return &VersionInfo{
		Tag:     info.Version,
		Version: strings.TrimPrefix(info.Version, "v"), // Normalize to "1.64.8"
	}, nil
}

// ListGoProxyVersions fetches all available versions of a Go module from proxy.golang.org
//
// API: https://proxy.golang.org/{module}/@v/list
// Returns: List of versions with "v" prefix (e.g., ["v1.64.8", "v1.64.7", ...])
func (r *Resolver) ListGoProxyVersions(ctx context.Context, modulePath string) ([]string, error) {
	goProxyURL := r.goProxyURL
	if goProxyURL == "" {
		goProxyURL = "https://proxy.golang.org" // Default if not set
	}

	encodedPath := encodeModulePath(modulePath)
	apiURL := fmt.Sprintf("%s/%s/@v/list", goProxyURL, encodedPath)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "goproxy",
			Message: "failed to create request",
			Err:     err,
		}
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "goproxy", "failed to fetch version list")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "goproxy",
			Message: fmt.Sprintf("module %s not found", modulePath),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "goproxy",
			Message: fmt.Sprintf("proxy.golang.org returned status %d", resp.StatusCode),
		}
	}

	// Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxGoProxyResponseSize)

	// Response is newline-separated list of versions
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "goproxy",
			Message: "failed to read response",
			Err:     err,
		}
	}

	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	var versions []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			versions = append(versions, line)
		}
	}

	return versions, nil
}

// ResolveCustom resolves a version using a custom source from the registry
// This delegates to the registry which looks up the appropriate resolver function
//
// Example usage in recipes:
//
//	[version]
//	source = "rust_dist"
//
// Returns ResolverError with ErrTypeUnknownSource if the source is not registered
func (r *Resolver) ResolveCustom(ctx context.Context, source string) (*VersionInfo, error) {
	return r.registry.Resolve(ctx, r, source)
}

// ResolveCustomVersion resolves a specific version using a custom source from the registry.
// This allows custom providers to resolve specific versions through the registry system.
func (r *Resolver) ResolveCustomVersion(ctx context.Context, source, version string) (*VersionInfo, error) {
	// For custom sources, we delegate to the registry which knows how to handle specific versions
	// The registry will resolve using the source-specific logic
	return r.registry.ResolveVersion(ctx, r, source, version)
}
