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

	"github.com/BurntSushi/toml"
	"github.com/tsukumogami/tsuku/internal/recipe"
)

const (
	// maxPyPIResponseSize limits response body to prevent memory exhaustion (10MB)
	maxPyPIResponseSize = 10 * 1024 * 1024
	// maxPyProjectSize limits pyproject.toml to prevent memory exhaustion (1MB)
	maxPyProjectSize = 1 * 1024 * 1024
	// pyprojectFetchTimeout is the timeout for fetching pyproject.toml from repository
	pyprojectFetchTimeout = 10 * time.Second
)

// pypiPackageResponse represents the PyPI API response for a package
type pypiPackageResponse struct {
	Info struct {
		Name        string            `json:"name"`
		Summary     string            `json:"summary"`
		HomePage    string            `json:"home_page"`
		ProjectURLs map[string]string `json:"project_urls"`
	} `json:"info"`
}

// pyprojectToml represents the relevant parts of pyproject.toml
type pyprojectToml struct {
	Project struct {
		Scripts map[string]string `toml:"scripts"`
	} `toml:"project"`
	Tool struct {
		Poetry struct {
			Scripts map[string]string `toml:"scripts"`
		} `toml:"poetry"`
	} `toml:"tool"`
}

// Pre-compile regex for package name validation
var pypiPackageNameRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9._-]*[a-zA-Z0-9])?$`)

// PyPIBuilder generates recipes for Python packages from PyPI
type PyPIBuilder struct {
	httpClient  *http.Client
	pypiBaseURL string
}

// NewPyPIBuilder creates a new PyPIBuilder with the given HTTP client.
// If httpClient is nil, a default client with timeouts will be created.
func NewPyPIBuilder(httpClient *http.Client) *PyPIBuilder {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}
	return &PyPIBuilder{
		httpClient:  httpClient,
		pypiBaseURL: "https://pypi.org",
	}
}

// NewPyPIBuilderWithBaseURL creates a new PyPIBuilder with custom PyPI URL (for testing)
func NewPyPIBuilderWithBaseURL(httpClient *http.Client, baseURL string) *PyPIBuilder {
	b := NewPyPIBuilder(httpClient)
	b.pypiBaseURL = baseURL
	return b
}

// Name returns the builder identifier
func (b *PyPIBuilder) Name() string {
	return "pypi"
}

// RequiresLLM returns false as this builder uses ecosystem APIs, not LLM.
func (b *PyPIBuilder) RequiresLLM() bool {
	return false
}

// CanBuild checks if the package exists on PyPI
func (b *PyPIBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error) {
	packageName := req.Package
	if !isValidPyPIPackageName(packageName) {
		return false, nil
	}

	// Query PyPI API to check if package exists
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
func (b *PyPIBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
	return NewDeterministicSession(b.Build, req), nil
}

// Build generates a recipe for the package
func (b *PyPIBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	if !isValidPyPIPackageName(req.Package) {
		return nil, fmt.Errorf("invalid package name: %s", req.Package)
	}

	// Fetch package metadata from PyPI
	pkgInfo, err := b.fetchPackageInfo(ctx, req.Package)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch package info: %w", err)
	}

	result := &BuildResult{
		Source:   fmt.Sprintf("pypi:%s", req.Package),
		Warnings: []string{},
	}

	// Try to discover executables from pyproject.toml
	executables, execWarnings := b.discoverExecutables(ctx, pkgInfo)
	result.Warnings = append(result.Warnings, execWarnings...)

	// Determine homepage
	homepage := pkgInfo.Info.HomePage
	if homepage == "" {
		// Try project_urls
		if pkgInfo.Info.ProjectURLs != nil {
			if h, ok := pkgInfo.Info.ProjectURLs["Homepage"]; ok {
				homepage = h
			} else if h, ok := pkgInfo.Info.ProjectURLs["Repository"]; ok {
				homepage = h
			} else if h, ok := pkgInfo.Info.ProjectURLs["Source"]; ok {
				homepage = h
			}
		}
	}

	// Build the recipe
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        req.Package,
			Description: pkgInfo.Info.Summary,
			Homepage:    homepage,
		},
		Version: recipe.VersionSection{
			Source: "pypi",
		},
		Steps: []recipe.Step{
			{
				Action: "pipx_install",
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

// fetchPackageInfo fetches package metadata from PyPI API
func (b *PyPIBuilder) fetchPackageInfo(ctx context.Context, packageName string) (*pypiPackageResponse, error) {
	baseURL, err := url.Parse(b.pypiBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	apiURL := baseURL.JoinPath("pypi", packageName, "json")

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
		return nil, fmt.Errorf("package %s not found on PyPI", packageName)
	}

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("PyPI rate limit exceeded")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("PyPI returned status %d", resp.StatusCode)
	}

	// Validate content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return nil, fmt.Errorf("unexpected content-type: %s", contentType)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxPyPIResponseSize)

	var pkgResp pypiPackageResponse
	if err := json.NewDecoder(limitedReader).Decode(&pkgResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &pkgResp, nil
}

// discoverExecutables attempts to find executable names from pyproject.toml
// Returns the executables list and any warnings generated during discovery
func (b *PyPIBuilder) discoverExecutables(ctx context.Context, pkgInfo *pypiPackageResponse) ([]string, []string) {
	var warnings []string

	// Get source URL from project_urls
	sourceURL := ""
	if pkgInfo.Info.ProjectURLs != nil {
		if s, ok := pkgInfo.Info.ProjectURLs["Repository"]; ok {
			sourceURL = s
		} else if s, ok := pkgInfo.Info.ProjectURLs["Source"]; ok {
			sourceURL = s
		} else if s, ok := pkgInfo.Info.ProjectURLs["Source Code"]; ok {
			sourceURL = s
		}
	}

	if sourceURL == "" {
		warnings = append(warnings, "No source code URL found; using package name as executable")
		return []string{pkgInfo.Info.Name}, warnings
	}

	// Parse source URL to construct pyproject.toml URL
	pyprojectURL := b.buildPyprojectURL(sourceURL)
	if pyprojectURL == "" {
		warnings = append(warnings, fmt.Sprintf("Could not parse source URL %s; using package name as executable", sourceURL))
		return []string{pkgInfo.Info.Name}, warnings
	}

	// Fetch and parse pyproject.toml
	executables, err := b.fetchPyprojectExecutables(ctx, pyprojectURL)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Could not fetch pyproject.toml: %v; using package name as executable", err))
		return []string{pkgInfo.Info.Name}, warnings
	}

	if len(executables) == 0 {
		// No scripts found, use package name
		return []string{pkgInfo.Info.Name}, warnings
	}

	return executables, warnings
}

// buildPyprojectURL constructs the raw pyproject.toml URL from a source URL
// Currently only supports GitHub repositories
func (b *PyPIBuilder) buildPyprojectURL(sourceURL string) string {
	// Parse the source URL
	parsed, err := url.Parse(sourceURL)
	if err != nil {
		return ""
	}

	// Only support GitHub for now
	if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
		return ""
	}

	// Extract owner/repo from path (handle trailing slashes, .git suffix, tree paths)
	path := strings.TrimSuffix(parsed.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimPrefix(path, "/")

	// Handle paths like "owner/repo/tree/branch/subdir"
	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}

	owner := parts[0]
	repo := parts[1]

	// Construct raw.githubusercontent.com URL for pyproject.toml
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/pyproject.toml", owner, repo)
}

// fetchPyprojectExecutables fetches pyproject.toml and extracts executable names
func (b *PyPIBuilder) fetchPyprojectExecutables(ctx context.Context, pyprojectURL string) ([]string, error) {
	// Create context with timeout for pyproject.toml fetch
	ctx, cancel := context.WithTimeout(ctx, pyprojectFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", pyprojectURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pyproject.toml: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch pyproject.toml: status %d", resp.StatusCode)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxPyProjectSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read pyproject.toml: %w", err)
	}

	// Parse TOML
	var pyproject pyprojectToml
	if err := toml.Unmarshal(data, &pyproject); err != nil {
		return nil, fmt.Errorf("failed to parse pyproject.toml: %w", err)
	}

	// Extract executable names from [project.scripts] or [tool.poetry.scripts]
	var executables []string

	// Check [project.scripts] (PEP 621 standard)
	for name := range pyproject.Project.Scripts {
		if isValidExecutableName(name) {
			executables = append(executables, name)
		}
	}

	// Check [tool.poetry.scripts] if no standard scripts found
	if len(executables) == 0 {
		for name := range pyproject.Tool.Poetry.Scripts {
			if isValidExecutableName(name) {
				executables = append(executables, name)
			}
		}
	}

	return executables, nil
}

// isValidPyPIPackageName validates a PyPI package name
func isValidPyPIPackageName(name string) bool {
	if name == "" || len(name) > 214 {
		return false
	}

	// Reject dangerous patterns
	if strings.Contains(name, "..") || strings.Contains(name, "/") ||
		strings.Contains(name, "\\") || strings.HasPrefix(name, "-") {
		return false
	}

	// Single character names are valid
	if len(name) == 1 {
		return (name[0] >= 'a' && name[0] <= 'z') ||
			(name[0] >= 'A' && name[0] <= 'Z') ||
			(name[0] >= '0' && name[0] <= '9')
	}

	return pypiPackageNameRegex.MatchString(name)
}

// Probe checks if a package exists on PyPI.
func (b *PyPIBuilder) Probe(ctx context.Context, name string) (*ProbeResult, error) {
	_, err := b.fetchPackageInfo(ctx, name)
	if err != nil {
		return &ProbeResult{Exists: false}, nil
	}
	return &ProbeResult{
		Exists: true,
		Source: name,
	}, nil
}
