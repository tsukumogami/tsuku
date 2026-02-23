package builders

import (
	"archive/zip"
	"bufio"
	"bytes"
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
	// maxWheelDownloadSize limits wheel artifact download to 20MB
	maxWheelDownloadSize = 20 * 1024 * 1024
	// maxExecutablesPerPackage is the upper bound on executable count per package
	maxExecutablesPerPackage = 50
)

// pypiPackageResponse represents the PyPI API response for a package
type pypiPackageResponse struct {
	Info struct {
		Name        string            `json:"name"`
		Summary     string            `json:"summary"`
		HomePage    string            `json:"home_page"`
		ProjectURLs map[string]string `json:"project_urls"`
		Classifiers []string          `json:"classifiers"`
	} `json:"info"`
	Releases map[string]json.RawMessage `json:"releases"`
	URLs     []pypiURLEntry             `json:"urls"`
}

// pypiURLEntry represents a single download artifact in the PyPI API response.
type pypiURLEntry struct {
	PackageType string      `json:"packagetype"`
	URL         string      `json:"url"`
	Filename    string      `json:"filename"`
	Digests     pypiDigests `json:"digests"`
}

// pypiDigests holds hash digests for a PyPI artifact.
type pypiDigests struct {
	SHA256 string `json:"sha256"`
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

// defaultTopPyPIURL is the static dump of top PyPI packages by downloads.
const defaultTopPyPIURL = "https://hugovk.github.io/top-pypi-packages/top-pypi-packages-30-days.min.json"

// PyPIBuilder generates recipes for Python packages from PyPI
type PyPIBuilder struct {
	httpClient  *http.Client
	pypiBaseURL string
	topPyPIURL  string
	// cachedWheelExecutables stores executables discovered from wheel artifacts
	// so that AuthoritativeBinaryNames() can return them without re-downloading.
	// Populated during Build() and read by the orchestrator via
	// the BinaryNameProvider interface (#1939).
	cachedWheelExecutables []string
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
		topPyPIURL:  defaultTopPyPIURL,
	}
}

// NewPyPIBuilderWithBaseURL creates a new PyPIBuilder with custom PyPI URL (for testing)
func NewPyPIBuilderWithBaseURL(httpClient *http.Client, baseURL string) *PyPIBuilder {
	b := NewPyPIBuilder(httpClient)
	b.pypiBaseURL = baseURL
	b.topPyPIURL = baseURL + "/top-pypi-packages-30-days.min.json"
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

	// Discover executables: try wheel first, fall back to pyproject.toml
	executables, execWarnings, fromWheel := b.discoverExecutables(ctx, pkgInfo)
	result.Warnings = append(result.Warnings, execWarnings...)

	// Cache wheel-discovered executables for BinaryNameProvider (#1939)
	if fromWheel {
		b.cachedWheelExecutables = make([]string, len(executables))
		copy(b.cachedWheelExecutables, executables)
	}

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
		Version: recipe.VersionSection{},
		Steps: []recipe.Step{
			{
				Action: "pipx_install",
				Params: map[string]interface{}{
					"package":     req.Package,
					"executables": executables,
				},
			},
		},
		Verify: &recipe.VerifySection{
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

// discoverExecutables tries wheel-based discovery first, falling back to
// pyproject.toml from GitHub. Returns the executables list, any warnings, and
// whether the result came from wheel discovery (used for BinaryNameProvider caching).
func (b *PyPIBuilder) discoverExecutables(ctx context.Context, pkgInfo *pypiPackageResponse) ([]string, []string, bool) {
	var warnings []string

	// Try wheel-based discovery first
	executables, wheelWarnings := b.discoverFromWheel(ctx, pkgInfo)
	warnings = append(warnings, wheelWarnings...)
	if len(executables) > 0 {
		return executables, warnings, true
	}

	// Fall back to pyproject.toml from GitHub
	fallbackExecs, fallbackWarnings := b.discoverFromPyproject(ctx, pkgInfo)
	warnings = append(warnings, fallbackWarnings...)
	return fallbackExecs, warnings, false
}

// discoverFromWheel attempts to download a wheel artifact and extract
// console_scripts from entry_points.txt inside the .dist-info directory.
func (b *PyPIBuilder) discoverFromWheel(ctx context.Context, pkgInfo *pypiPackageResponse) ([]string, []string) {
	var warnings []string

	// Find a suitable wheel from the urls array
	wheel := b.findBestWheel(pkgInfo.URLs)
	if wheel == nil {
		warnings = append(warnings, "No wheel artifact available; trying pyproject.toml fallback")
		return nil, warnings
	}

	// Download the wheel in-memory
	data, err := downloadArtifact(ctx, b.httpClient, wheel.URL, downloadArtifactOptions{
		MaxSize:              maxWheelDownloadSize,
		ExpectedSHA256:       wheel.Digests.SHA256,
		ExpectedContentTypes: []string{"application/zip", "application/octet-stream", "binary/octet-stream"},
	})
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Wheel download failed: %v; trying pyproject.toml fallback", err))
		return nil, warnings
	}

	// Parse the ZIP archive and extract entry_points.txt
	executables, err := b.extractConsoleScripts(data, pkgInfo.Info.Name)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Wheel parsing failed: %v; trying pyproject.toml fallback", err))
		return nil, warnings
	}

	if len(executables) == 0 {
		warnings = append(warnings, "No console_scripts in wheel; trying pyproject.toml fallback")
		return nil, warnings
	}

	return executables, warnings
}

// findBestWheel selects the best wheel artifact from the PyPI urls array.
// It prefers platform-independent wheels (py3-none-any) over platform-specific ones.
func (b *PyPIBuilder) findBestWheel(urls []pypiURLEntry) *pypiURLEntry {
	var bestWheel *pypiURLEntry
	for i := range urls {
		if urls[i].PackageType != "bdist_wheel" {
			continue
		}
		if bestWheel == nil {
			bestWheel = &urls[i]
			continue
		}
		// Prefer platform-independent wheel
		if strings.Contains(urls[i].Filename, "-py3-none-any") ||
			strings.Contains(urls[i].Filename, "-py2.py3-none-any") {
			bestWheel = &urls[i]
		}
	}
	return bestWheel
}

// extractConsoleScripts reads the wheel ZIP and parses entry_points.txt from
// the .dist-info directory to extract [console_scripts] entries.
func (b *PyPIBuilder) extractConsoleScripts(wheelData []byte, packageName string) ([]string, error) {
	reader, err := zip.NewReader(bytes.NewReader(wheelData), int64(len(wheelData)))
	if err != nil {
		return nil, fmt.Errorf("invalid wheel ZIP: %w", err)
	}

	// Look for entry_points.txt in the .dist-info directory.
	// The normalized package name follows PEP 503: lowercase, hyphens become underscores.
	normalizedName := normalizePyPIName(packageName)
	suffix := ".dist-info/entry_points.txt"

	var entryPointsFile *zip.File
	for _, f := range reader.File {
		// Match either {normalized_name}-{version}.dist-info/entry_points.txt
		// or any .dist-info/entry_points.txt that ends with our suffix.
		if !strings.HasSuffix(f.Name, suffix) {
			continue
		}
		dirName := strings.TrimSuffix(f.Name, "/entry_points.txt")
		dirBase := dirName
		if idx := strings.LastIndex(dirName, "/"); idx >= 0 {
			dirBase = dirName[idx+1:]
		}
		// The dist-info directory is named {normalized}-{version}.dist-info
		// Check that it starts with our normalized package name
		dirLower := strings.ToLower(dirBase)
		if strings.HasPrefix(dirLower, normalizedName+"-") ||
			strings.HasPrefix(dirLower, normalizedName+".") {
			entryPointsFile = f
			break
		}
		// If we haven't found a specific match yet, keep this as a fallback
		if entryPointsFile == nil {
			entryPointsFile = f
		}
	}

	if entryPointsFile == nil {
		return nil, fmt.Errorf("entry_points.txt not found in wheel")
	}

	rc, err := entryPointsFile.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open entry_points.txt: %w", err)
	}
	defer rc.Close()

	return parseConsoleScripts(rc)
}

// parseConsoleScripts parses an entry_points.txt file and extracts executable
// names from the [console_scripts] section. Each line in that section has the
// format: name = module:function [extras]
func parseConsoleScripts(r io.Reader) ([]string, error) {
	scanner := bufio.NewScanner(r)
	inConsoleScripts := false
	var executables []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section := strings.ToLower(strings.Trim(line, "[]"))
			inConsoleScripts = section == "console_scripts"
			continue
		}

		if !inConsoleScripts {
			continue
		}

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse "name = module:function" -- extract just the name
		parts := strings.SplitN(line, "=", 2)
		if len(parts) < 2 {
			continue
		}

		name := strings.TrimSpace(parts[0])
		if name == "" {
			continue
		}

		if !isValidExecutableName(name) {
			continue
		}

		executables = append(executables, name)
		if len(executables) >= maxExecutablesPerPackage {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse entry_points.txt: %w", err)
	}

	return executables, nil
}

// normalizePyPIName normalizes a PyPI package name per PEP 503:
// lowercase, runs of hyphens/underscores/periods replaced with a single underscore.
func normalizePyPIName(name string) string {
	name = strings.ToLower(name)
	var result strings.Builder
	prevSep := false
	for _, ch := range name {
		if ch == '-' || ch == '_' || ch == '.' {
			if !prevSep {
				result.WriteRune('_')
				prevSep = true
			}
			continue
		}
		prevSep = false
		result.WriteRune(ch)
	}
	return result.String()
}

// discoverFromPyproject is the fallback discovery path that fetches
// pyproject.toml from GitHub and parses [project.scripts].
func (b *PyPIBuilder) discoverFromPyproject(ctx context.Context, pkgInfo *pypiPackageResponse) ([]string, []string) {
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

// topPyPIPackagesResponse represents the static dump of top PyPI packages.
type topPyPIPackagesResponse struct {
	Rows []struct {
		Project       string `json:"project"`
		DownloadCount int    `json:"download_count"`
	} `json:"rows"`
}

// Discover lists popular CLI packages from PyPI by fetching a static dump
// of top packages and filtering for those with an "Environment :: Console"
// classifier. It rate-limits metadata fetches at 5 requests/second.
// Downloads in the returned candidates are set to 0 because PyPI's per-package
// API does not expose download counts.
func (b *PyPIBuilder) Discover(ctx context.Context, limit int) ([]DiscoveryCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}

	// Fetch the static dump of top PyPI packages.
	req, err := http.NewRequestWithContext(ctx, "GET", b.topPyPIURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create top packages request: %w", err)
	}
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch top PyPI packages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("top PyPI packages returned status %d", resp.StatusCode)
	}

	limitedReader := io.LimitReader(resp.Body, maxPyPIResponseSize)
	var topPkgs topPyPIPackagesResponse
	if err := json.NewDecoder(limitedReader).Decode(&topPkgs); err != nil {
		return nil, fmt.Errorf("parse top PyPI packages: %w", err)
	}

	var candidates []DiscoveryCandidate

	for _, row := range topPkgs.Rows {
		if len(candidates) >= limit {
			break
		}

		select {
		case <-ctx.Done():
			return candidates, ctx.Err()
		default:
		}

		name := row.Project
		if name == "" {
			continue
		}

		// Rate limit: 5 req/s for metadata fetches.
		select {
		case <-ctx.Done():
			return candidates, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}

		// Fetch per-package metadata to check classifiers.
		pkgInfo, err := b.fetchPackageInfo(ctx, name)
		if err != nil {
			// Skip packages we can't fetch (not found, rate limited, etc.)
			continue
		}

		if !hasConsoleClassifier(pkgInfo.Info.Classifiers) {
			continue
		}

		candidates = append(candidates, DiscoveryCandidate{
			Name:      name,
			Downloads: 0, // PyPI API does not expose download counts
		})
	}

	return candidates, nil
}

// hasConsoleClassifier checks if the classifiers list contains
// "Environment :: Console", indicating a CLI tool.
func hasConsoleClassifier(classifiers []string) bool {
	for _, c := range classifiers {
		if c == "Environment :: Console" {
			return true
		}
	}
	return false
}

// AuthoritativeBinaryNames returns the executable names discovered from
// the wheel artifact's entry_points.txt. This implements BinaryNameProvider
// so the orchestrator can cross-check recipe executables against registry
// metadata.
//
// Returns nil if Build() hasn't been called yet (no cached data), or if
// wheel-based discovery was not successful (the builder fell back to
// pyproject.toml). Only wheel-discovered executables are authoritative because
// they come from the published artifact, not from source files.
func (b *PyPIBuilder) AuthoritativeBinaryNames() []string {
	if len(b.cachedWheelExecutables) == 0 {
		return nil
	}
	return b.cachedWheelExecutables
}

// Probe checks if a package exists on PyPI and returns quality metadata.
func (b *PyPIBuilder) Probe(ctx context.Context, name string) (*ProbeResult, error) {
	info, err := b.fetchPackageInfo(ctx, name)
	if err != nil {
		return nil, nil
	}
	hasRepo := false
	if info.Info.ProjectURLs != nil {
		for _, u := range info.Info.ProjectURLs {
			if u != "" {
				hasRepo = true
				break
			}
		}
	}
	return &ProbeResult{
		Source:        name,
		VersionCount:  len(info.Releases),
		HasRepository: hasRepo,
	}, nil
}
