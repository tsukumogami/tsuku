package builders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

const (
	// maxNpmResponseSize limits response body to prevent memory exhaustion (10MB)
	maxNpmResponseSize = 10 * 1024 * 1024
)

// npmPackageResponse represents the npm registry API response for a package
type npmPackageResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Homepage    string `json:"homepage"`
	Repository  any    `json:"repository"` // can be string or object
	DistTags    struct {
		Latest string `json:"latest"`
	} `json:"dist-tags"`
	Versions map[string]npmVersionInfo `json:"versions"`
}

// npmVersionInfo represents version-specific metadata
type npmVersionInfo struct {
	Bin any `json:"bin"` // can be string or map[string]string
}

// Pre-compile regex for npm package name validation
// npm package names follow these rules:
// - Can be scoped (@scope/package) or unscoped (package)
// - Must be lowercase
// - Can contain hyphens, dots, underscores
// - Max length: 214 characters
var npmPackageNameRegex = regexp.MustCompile(`^(@[a-z0-9][\w.-]*/)?[a-z0-9][\w.-]*$`)

// NpmBuilder generates recipes for Node.js packages from npm registry
type NpmBuilder struct {
	httpClient     *http.Client
	npmRegistryURL string
}

// NewNpmBuilder creates a new NpmBuilder with the given HTTP client.
// If httpClient is nil, a default client with timeouts will be created.
func NewNpmBuilder(httpClient *http.Client) *NpmBuilder {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}
	return &NpmBuilder{
		httpClient:     httpClient,
		npmRegistryURL: "https://registry.npmjs.org",
	}
}

// NewNpmBuilderWithBaseURL creates a new NpmBuilder with custom registry URL (for testing)
func NewNpmBuilderWithBaseURL(httpClient *http.Client, baseURL string) *NpmBuilder {
	b := NewNpmBuilder(httpClient)
	b.npmRegistryURL = baseURL
	return b
}

// Name returns the builder identifier
func (b *NpmBuilder) Name() string {
	return "npm"
}

// RequiresLLM returns false as this builder uses ecosystem APIs, not LLM.
func (b *NpmBuilder) RequiresLLM() bool {
	return false
}

// CanBuild checks if the package exists on npm registry
func (b *NpmBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error) {
	packageName := req.Package
	if !isValidNpmPackageNameForBuilder(packageName) {
		return false, nil
	}

	// Query npm registry API to check if package exists
	_, err := b.fetchPackageInfo(ctx, packageName)
	if err != nil {
		// Not found is not an error - just means we can't build it
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// NewSession creates a new build session for the given request.
func (b *NpmBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
	return NewDeterministicSession(b.Build, req), nil
}

// Build generates a recipe for the package
func (b *NpmBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	if !isValidNpmPackageNameForBuilder(req.Package) {
		return nil, fmt.Errorf("invalid package name: %s", req.Package)
	}

	// Fetch package metadata from npm registry
	pkgInfo, err := b.fetchPackageInfo(ctx, req.Package)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package info: %w", err)
	}

	result := &BuildResult{
		Source:   fmt.Sprintf("npm:%s", req.Package),
		Warnings: []string{},
	}

	// Discover executables from bin field
	executables, execWarnings := b.discoverExecutables(pkgInfo)
	result.Warnings = append(result.Warnings, execWarnings...)

	// Determine homepage
	homepage := pkgInfo.Homepage
	if homepage == "" {
		// Try repository field
		homepage = extractRepositoryURL(pkgInfo.Repository)
	}

	// Build the recipe
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        req.Package,
			Description: pkgInfo.Description,
			Homepage:    homepage,
		},
		Version: recipe.VersionSection{
			Source: "npm",
		},
		Steps: []recipe.Step{
			{
				Action: "npm_install",
				Params: map[string]interface{}{
					"package":     req.Package,
					"executables": executables,
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: fmt.Sprintf("%s --version", executables[0]),
		},
	}

	result.Recipe = r
	return result, nil
}

// fetchPackageInfo fetches package metadata from npm registry API
func (b *NpmBuilder) fetchPackageInfo(ctx context.Context, packageName string) (*npmPackageResponse, error) {
	baseURL, err := url.Parse(b.npmRegistryURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}

	// Handle scoped packages (@scope/package)
	apiURL := baseURL.JoinPath(packageName)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("package %s not found on npm", packageName)
	}

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("npm rate limit exceeded")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("npm returned status %d", resp.StatusCode)
	}

	// Validate content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return nil, fmt.Errorf("unexpected content-type: %s", contentType)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxNpmResponseSize)

	var pkgResp npmPackageResponse
	if err := json.NewDecoder(limitedReader).Decode(&pkgResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &pkgResp, nil
}

// discoverExecutables extracts executable names from the bin field
// Returns the executables list and any warnings generated during discovery
func (b *NpmBuilder) discoverExecutables(pkgInfo *npmPackageResponse) ([]string, []string) {
	var warnings []string

	// Get latest version
	latestVersion := pkgInfo.DistTags.Latest
	if latestVersion == "" {
		warnings = append(warnings, "No latest version found; using package name as executable")
		return []string{pkgInfo.Name}, warnings
	}

	// Get version info
	versionInfo, ok := pkgInfo.Versions[latestVersion]
	if !ok {
		warnings = append(warnings, fmt.Sprintf("Version %s not found in versions; using package name as executable", latestVersion))
		return []string{pkgInfo.Name}, warnings
	}

	// Parse bin field
	executables := parseBinField(versionInfo.Bin)
	if len(executables) == 0 {
		warnings = append(warnings, "No bin field found; using package name as executable")
		return []string{pkgInfo.Name}, warnings
	}

	return executables, warnings
}

// parseBinField extracts executable names from the bin field
// The bin field can be:
// - string: the package name is the executable, value is the path
// - map[string]string: keys are executable names, values are paths
func parseBinField(bin any) []string {
	if bin == nil {
		return nil
	}

	switch v := bin.(type) {
	case string:
		// Single executable - we can't determine the name from this
		// Return empty to signal fallback to package name
		return nil
	case map[string]any:
		var executables []string
		for name := range v {
			if isValidExecutableName(name) {
				executables = append(executables, name)
			}
		}
		return executables
	}

	return nil
}

// extractRepositoryURL extracts URL from repository field
// Repository can be a string URL or an object with "url" field
func extractRepositoryURL(repo any) string {
	if repo == nil {
		return ""
	}

	switch v := repo.(type) {
	case string:
		return cleanRepositoryURL(v)
	case map[string]any:
		if urlVal, ok := v["url"].(string); ok {
			return cleanRepositoryURL(urlVal)
		}
	}

	return ""
}

// cleanRepositoryURL cleans up repository URLs
// Handles git+https://, git://, and .git suffix
func cleanRepositoryURL(rawURL string) string {
	// Remove git+ prefix
	url := strings.TrimPrefix(rawURL, "git+")
	// Remove .git suffix
	url = strings.TrimSuffix(url, ".git")
	// Convert git:// to https://
	url = strings.Replace(url, "git://", "https://", 1)
	return url
}

// isValidNpmPackageNameForBuilder validates an npm package name
func isValidNpmPackageNameForBuilder(name string) bool {
	if name == "" || len(name) > 214 {
		return false
	}

	// Reject dangerous patterns
	if strings.Contains(name, "..") || strings.Contains(name, "\\") {
		return false
	}

	// Must be lowercase (npm requires lowercase)
	if strings.ToLower(name) != name {
		return false
	}

	return npmPackageNameRegex.MatchString(name)
}

// Probe checks if a package exists on npm.
func (b *NpmBuilder) Probe(ctx context.Context, name string) (*ProbeResult, error) {
	_, err := b.fetchPackageInfo(ctx, name)
	if err != nil {
		return nil, nil
	}
	return &ProbeResult{Source: name}, nil
}
