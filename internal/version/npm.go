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
)

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

// ResolveNpm resolves the latest version from npm registry
func (r *Resolver) ResolveNpm(ctx context.Context, packageName string) (*VersionInfo, error) {
	// Defense-in-depth: Validate package name before URL construction
	if !isValidNpmPackageName(packageName) {
		return nil, fmt.Errorf("invalid npm package name: %s", packageName)
	}

	reqURL := fmt.Sprintf("https://registry.npmjs.org/%s/latest", packageName)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
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
