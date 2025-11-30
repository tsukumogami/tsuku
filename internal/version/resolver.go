package version

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"
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
	authenticated       bool           // Whether GitHub requests are authenticated
}

// newHTTPClient creates an HTTP client with security hardening and proper timeouts
func newHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 60 * time.Second,
		Transport: &http.Transport{
			DisableCompression: true, // CRITICAL: Prevents decompression bomb attacks
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
			MaxIdleConns:          10,
			IdleConnTimeout:       90 * time.Second,
			DisableKeepAlives:     false,
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			// 1. Prevent redirect to non-HTTPS
			if req.URL.Scheme != "https" {
				return fmt.Errorf("refusing redirect to non-HTTPS URL: %s", req.URL)
			}

			// 2. Limit redirect chain
			if len(via) >= 5 {
				return fmt.Errorf("too many redirects")
			}

			// 3. SSRF Protection: Check redirect target
			host := req.URL.Hostname()

			// 3a. If hostname is already an IP, check it directly
			if ip := net.ParseIP(host); ip != nil {
				if err := validateIP(ip, host); err != nil {
					return err
				}
			} else {
				// 3b. Hostname is a domain - resolve DNS and check ALL resulting IPs
				// This prevents DNS rebinding attacks where evil.com resolves to 127.0.0.1
				ips, err := net.LookupIP(host)
				if err != nil {
					return fmt.Errorf("failed to resolve redirect host %s: %w", host, err)
				}

				for _, ip := range ips {
					if err := validateIP(ip, host); err != nil {
						return fmt.Errorf("refusing redirect: %s resolves to blocked IP %s", host, ip)
					}
				}
			}

			return nil
		},
	}
}

// validateIP checks if an IP is allowed (not private, loopback, link-local, etc.)
func validateIP(ip net.IP, host string) error {
	// Block private IPs (RFC 1918: 10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16)
	if ip.IsPrivate() {
		return fmt.Errorf("refusing redirect to private IP: %s (%s)", host, ip)
	}

	// Block loopback (127.0.0.0/8, ::1)
	if ip.IsLoopback() {
		return fmt.Errorf("refusing redirect to loopback IP: %s (%s)", host, ip)
	}

	// Block link-local unicast (169.254.0.0/16, fe80::/10)
	// CRITICAL: This includes AWS metadata service at 169.254.169.254
	if ip.IsLinkLocalUnicast() {
		return fmt.Errorf("refusing redirect to link-local IP: %s (%s)", host, ip)
	}

	// Block link-local multicast (224.0.0.0/24, ff02::/16)
	if ip.IsLinkLocalMulticast() {
		return fmt.Errorf("refusing redirect to link-local multicast: %s (%s)", host, ip)
	}

	// Block unspecified address (0.0.0.0, ::)
	if ip.IsUnspecified() {
		return fmt.Errorf("refusing redirect to unspecified IP: %s (%s)", host, ip)
	}

	return nil
}

// New creates a new version resolver
// If GITHUB_TOKEN environment variable is set, it will be used for authenticated requests
func New() *Resolver {
	var githubHTTPClient *http.Client
	authenticated := false

	// Check for GitHub token in environment
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		githubHTTPClient = oauth2.NewClient(context.Background(), ts)
		authenticated = true
	}

	return &Resolver{
		client:              github.NewClient(githubHTTPClient),
		httpClient:          newHTTPClient(),              // HTTP client with proper timeouts
		registry:            NewRegistry(),                // Initialize with default resolvers
		npmRegistryURL:      "https://registry.npmjs.org", // Production default
		pypiRegistryURL:     "https://pypi.org",           // Production default
		cratesIORegistryURL: "https://crates.io",          // Production default
		rubygemsRegistryURL: "https://rubygems.org",       // Production default
		authenticated:       authenticated,
	}
}

// NewWithNpmRegistry creates a resolver with custom npm registry (for testing)
func NewWithNpmRegistry(registryURL string) *Resolver {
	var githubHTTPClient *http.Client
	authenticated := false

	// Check for GitHub token in environment
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		githubHTTPClient = oauth2.NewClient(context.Background(), ts)
		authenticated = true
	}

	return &Resolver{
		client:              github.NewClient(githubHTTPClient),
		httpClient:          newHTTPClient(),
		registry:            NewRegistry(),
		npmRegistryURL:      registryURL,
		pypiRegistryURL:     "https://pypi.org",     // Default PyPI
		cratesIORegistryURL: "https://crates.io",    // Default crates.io
		rubygemsRegistryURL: "https://rubygems.org", // Default RubyGems
		authenticated:       authenticated,
	}
}

// NewWithPyPIRegistry creates a resolver with custom PyPI registry (for testing)
func NewWithPyPIRegistry(registryURL string) *Resolver {
	var githubHTTPClient *http.Client
	authenticated := false

	// Check for GitHub token in environment
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		githubHTTPClient = oauth2.NewClient(context.Background(), ts)
		authenticated = true
	}

	return &Resolver{
		client:              github.NewClient(githubHTTPClient),
		httpClient:          newHTTPClient(),
		registry:            NewRegistry(),
		npmRegistryURL:      "https://registry.npmjs.org", // Default npm
		pypiRegistryURL:     registryURL,
		cratesIORegistryURL: "https://crates.io",    // Default crates.io
		rubygemsRegistryURL: "https://rubygems.org", // Default RubyGems
		authenticated:       authenticated,
	}
}

// NewWithCratesIORegistry creates a resolver with custom crates.io registry (for testing)
func NewWithCratesIORegistry(registryURL string) *Resolver {
	var githubHTTPClient *http.Client
	authenticated := false

	// Check for GitHub token in environment
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		githubHTTPClient = oauth2.NewClient(context.Background(), ts)
		authenticated = true
	}

	return &Resolver{
		client:              github.NewClient(githubHTTPClient),
		httpClient:          newHTTPClient(),
		registry:            NewRegistry(),
		npmRegistryURL:      "https://registry.npmjs.org", // Default npm
		pypiRegistryURL:     "https://pypi.org",           // Default PyPI
		cratesIORegistryURL: registryURL,
		rubygemsRegistryURL: "https://rubygems.org", // Default RubyGems
		authenticated:       authenticated,
	}
}

// NewWithRubyGemsRegistry creates a resolver with custom RubyGems registry (for testing)
func NewWithRubyGemsRegistry(registryURL string) *Resolver {
	var githubHTTPClient *http.Client
	authenticated := false

	// Check for GitHub token in environment
	if token := os.Getenv("GITHUB_TOKEN"); token != "" {
		ts := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
		githubHTTPClient = oauth2.NewClient(context.Background(), ts)
		authenticated = true
	}

	return &Resolver{
		client:              github.NewClient(githubHTTPClient),
		httpClient:          newHTTPClient(),
		registry:            NewRegistry(),
		npmRegistryURL:      "https://registry.npmjs.org", // Default npm
		pypiRegistryURL:     "https://pypi.org",           // Default PyPI
		cratesIORegistryURL: "https://crates.io",          // Default crates.io
		rubygemsRegistryURL: registryURL,
		authenticated:       authenticated,
	}
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

// Pre-compile regex for npm package name validation (performance)
var npmPackageNameRegex = regexp.MustCompile(`^(@[a-z0-9]([a-z0-9._-]*[a-z0-9])?/)?[a-z0-9]([a-z0-9._-]*[a-z0-9])?$`)

// isValidNpmPackageName validates npm package name format
// npm package names follow these rules:
// - Can be scoped (@scope/package) or unscoped (package)
// - Must be lowercase
// - Must start and end with alphanumeric (not hyphen, dot, underscore, or tilde)
// - Can contain hyphens, dots, underscores in the middle
// - Max length: 214 characters
// - No consecutive dots (..)
func isValidNpmPackageName(name string) bool {
	if name == "" || len(name) > 214 {
		return false
	}

	// Validate structure: must start and end with alphanumeric
	if !npmPackageNameRegex.MatchString(name) {
		return false
	}

	// Additional validation: no consecutive dots
	if strings.Contains(name, "..") {
		return false
	}

	// For scoped packages, validate both scope and package parts
	if strings.HasPrefix(name, "@") {
		parts := strings.SplitN(name[1:], "/", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return false
		}
	}

	return true
}

// ListNpmVersions lists all available versions for an npm package
// Uses npm registry API: https://registry.npmjs.org/<package>
func (r *Resolver) ListNpmVersions(ctx context.Context, packageName string) ([]string, error) {
	// Validate package name to prevent injection attacks
	if !isValidNpmPackageName(packageName) {
		return nil, fmt.Errorf("invalid npm package name: %s", packageName)
	}

	// Build URL using configured registry
	baseURL, err := url.Parse(r.npmRegistryURL)
	if err != nil {
		return nil, fmt.Errorf("invalid npm registry URL: %w", err)
	}

	// Create a copy to avoid modifying the base URL
	u := *baseURL

	// Append package name to registry path
	// Example: https://registry.npmjs.org + "package" → https://registry.npmjs.org/package
	// Example: https://nexus.com/npm-proxy/ + "package" → https://nexus.com/npm-proxy/package
	// Use u.Path = "/" + packageName for direct setting (url.URL.String() will encode)
	if u.Path == "" || u.Path == "/" {
		u.Path = "/" + packageName
	} else {
		// Registry URL has a path component (e.g., /repository/npm-proxy)
		// For scoped packages like @scope/package, we need to preserve the / in the path
		// url.URL.String() will properly encode the path when serializing
		if u.Path[len(u.Path)-1] == '/' {
			u.Path = u.Path + packageName
		} else {
			u.Path = u.Path + "/" + packageName
		}
	}

	registryURL := u.String()

	// Create HTTP request with context for cancellation/timeout
	req, err := http.NewRequestWithContext(ctx, "GET", registryURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Defense in depth: Explicitly request uncompressed response
	req.Header.Set("Accept-Encoding", "identity")

	// Execute request using resolver's HTTP client (already configured with timeouts)
	resp, err := r.httpClient.Do(req)
	if err != nil {
		// Check for network errors (same pattern as GitHub resolver)
		if strings.Contains(err.Error(), "network is unreachable") ||
			strings.Contains(err.Error(), "no such host") ||
			strings.Contains(err.Error(), "dial tcp") {
			return nil, fmt.Errorf("network unavailable: %w", err)
		}
		return nil, fmt.Errorf("failed to fetch npm package: %w", err)
	}
	defer resp.Body.Close()

	// Handle HTTP status codes
	switch resp.StatusCode {
	case http.StatusOK:
		// Continue processing below
	case http.StatusNotFound:
		return nil, fmt.Errorf("package not found in npm registry: %s", packageName)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("npm registry rate limit exceeded. Please try again later")
	default:
		return nil, fmt.Errorf("npm registry returned status %d", resp.StatusCode)
	}

	// Defense in depth: Reject compressed responses (should never happen with DisableCompression)
	if encoding := resp.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		return nil, fmt.Errorf("compressed responses not supported (got %s)", encoding)
	}

	// Limit response body size to prevent DoS attacks (50MB max)
	// Popular packages like aws-cdk are ~500KB-1MB of metadata
	// Some packages with many versions like serverless can be ~17MB
	const maxNpmResponseSize = 50 * 1024 * 1024 // 50MB
	limitedBody := io.LimitReader(resp.Body, maxNpmResponseSize)

	// Parse JSON response
	// npm registry returns: {"versions": {"1.0.0": {...}, "1.0.1": {...}, ...}}
	var data struct {
		Versions map[string]interface{} `json:"versions"`
	}

	if err := json.NewDecoder(limitedBody).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse npm response: %w", err)
	}

	// Extract version keys from map
	versions := make([]string, 0, len(data.Versions))
	for version := range data.Versions {
		versions = append(versions, version)
	}

	// Sort versions in descending order (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})

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

// normalizeVersion cleans up version tags from GitHub releases
func normalizeVersion(version string) string {
	// Strip "v" prefix
	version = strings.TrimPrefix(version, "v")

	// Handle multi-part tags like "kustomize/v5.7.1" -> "5.7.1"
	if strings.Contains(version, "/") {
		parts := strings.Split(version, "/")
		version = strings.TrimPrefix(parts[len(parts)-1], "v")
	}

	// Handle Release_X_Y_Z format (e.g., "Release_1_15_0" -> "1.15.0")
	if strings.HasPrefix(version, "Release_") {
		version = strings.TrimPrefix(version, "Release_")
		version = strings.ReplaceAll(version, "_", ".")
	}

	// Handle golang-style tags (go1.21.5 -> 1.21.5)
	version = strings.TrimPrefix(version, "go")

	return version
}

// isValidVersion checks if a version string looks like a semantic version
func isValidVersion(v string) bool {
	if v == "" {
		return false
	}

	// Must contain at least one digit
	hasDigit := false
	for _, c := range v {
		if c >= '0' && c <= '9' {
			hasDigit = true
			break
		}
	}

	return hasDigit
}

// compareVersions compares two semantic versions
// Returns: 1 if v1 > v2, -1 if v1 < v2, 0 if equal
func compareVersions(v1, v2 string) int {
	// Simple lexicographic comparison works for most semver strings
	// This handles cases like "1.21.5" vs "1.20.1"
	parts1 := strings.Split(v1, ".")
	parts2 := strings.Split(v2, ".")

	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var p1, p2 int

		// Parse part1
		if i < len(parts1) {
			_, _ = fmt.Sscanf(parts1[i], "%d", &p1)
		}

		// Parse part2
		if i < len(parts2) {
			_, _ = fmt.Sscanf(parts2[i], "%d", &p2)
		}

		if p1 > p2 {
			return 1
		}
		if p1 < p2 {
			return -1
		}
	}

	return 0
}

// ResolveNpm resolves the latest version from npm registry
func (r *Resolver) ResolveNpm(ctx context.Context, packageName string) (*VersionInfo, error) {
	url := fmt.Sprintf("https://registry.npmjs.org/%s/latest", packageName)

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
		return nil, fmt.Errorf("failed to fetch package info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("npm registry returned status %d for package %s", resp.StatusCode, packageName)
	}

	var result struct {
		Version string `json:"version"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	if result.Version == "" {
		return nil, fmt.Errorf("no version found for package %s", packageName)
	}

	return &VersionInfo{
		Tag:     result.Version,
		Version: result.Version,
	}, nil
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
