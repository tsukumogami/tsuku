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
	// maxCratesIOResponseSize limits response body to prevent memory exhaustion (10MB)
	maxCratesIOResponseSize = 10 * 1024 * 1024
	// maxCargoTomlSize limits Cargo.toml to prevent memory exhaustion (1MB)
	maxCargoTomlSize = 1 * 1024 * 1024
	// cargoTomlFetchTimeout is the timeout for fetching Cargo.toml from repository
	cargoTomlFetchTimeout = 10 * time.Second
)

// cratesIOCrateResponse represents the crates.io API response for a crate
type cratesIOCrateResponse struct {
	Crate struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Homepage    string `json:"homepage"`
		Repository  string `json:"repository"`
	} `json:"crate"`
}

// cargoTomlBinSection represents a [[bin]] section in Cargo.toml
type cargoTomlBinSection struct {
	Name string `toml:"name"`
	Path string `toml:"path"`
}

// cargoToml represents the relevant parts of Cargo.toml for executable discovery
type cargoToml struct {
	Package struct {
		Name string `toml:"name"`
	} `toml:"package"`
	Bin []cargoTomlBinSection `toml:"bin"`
}

// Pre-compile regex for crate name validation
var crateNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// CargoBuilder generates recipes for Rust crates from crates.io
type CargoBuilder struct {
	httpClient      *http.Client
	cratesIOBaseURL string
}

// NewCargoBuilder creates a new CargoBuilder with the given HTTP client.
// If httpClient is nil, a default client with timeouts will be created.
func NewCargoBuilder(httpClient *http.Client) *CargoBuilder {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}
	return &CargoBuilder{
		httpClient:      httpClient,
		cratesIOBaseURL: "https://crates.io",
	}
}

// NewCargoBuilderWithBaseURL creates a new CargoBuilder with custom crates.io URL (for testing)
func NewCargoBuilderWithBaseURL(httpClient *http.Client, baseURL string) *CargoBuilder {
	b := NewCargoBuilder(httpClient)
	b.cratesIOBaseURL = baseURL
	return b
}

// Name returns the builder identifier
func (b *CargoBuilder) Name() string {
	return "crates.io"
}

// RequiresLLM returns false as this builder uses ecosystem APIs, not LLM.
func (b *CargoBuilder) RequiresLLM() bool {
	return false
}

// CanBuild checks if the crate exists on crates.io
func (b *CargoBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error) {
	packageName := req.Package
	if !isValidCrateName(packageName) {
		return false, nil
	}

	// Query crates.io API to check if crate exists
	_, err := b.fetchCrateInfo(ctx, packageName)
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
func (b *CargoBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
	return NewDeterministicSession(b.Build, req), nil
}

// Build generates a recipe for the crate
func (b *CargoBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	if !isValidCrateName(req.Package) {
		return nil, fmt.Errorf("invalid crate name: %s", req.Package)
	}

	// Fetch crate metadata from crates.io
	crateInfo, err := b.fetchCrateInfo(ctx, req.Package)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch crate info: %w", err)
	}

	result := &BuildResult{
		Source:   fmt.Sprintf("crates.io:%s", req.Package),
		Warnings: []string{},
	}

	// Try to discover executables from Cargo.toml
	executables, execWarnings := b.discoverExecutables(ctx, crateInfo)
	result.Warnings = append(result.Warnings, execWarnings...)

	// Build the recipe
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        req.Package,
			Description: crateInfo.Crate.Description,
			Homepage:    crateInfo.Crate.Homepage,
		},
		Version: recipe.VersionSection{
			Source: "crates_io",
		},
		Steps: []recipe.Step{
			{
				Action: "cargo_install",
				Params: map[string]interface{}{
					"crate":       req.Package,
					"executables": executables,
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: fmt.Sprintf("%s --version", executables[0]),
		},
	}

	// Use repository as homepage if homepage is empty
	if r.Metadata.Homepage == "" && crateInfo.Crate.Repository != "" {
		r.Metadata.Homepage = crateInfo.Crate.Repository
	}

	result.Recipe = r
	return result, nil
}

// fetchCrateInfo fetches crate metadata from crates.io API
func (b *CargoBuilder) fetchCrateInfo(ctx context.Context, crateName string) (*cratesIOCrateResponse, error) {
	baseURL, err := url.Parse(b.cratesIOBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	apiURL := baseURL.JoinPath("api", "v1", "crates", crateName)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// crates.io requires a User-Agent header
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch crate info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("crate %s not found on crates.io", crateName)
	}

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("crates.io rate limit exceeded")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("crates.io returned status %d", resp.StatusCode)
	}

	// Validate content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return nil, fmt.Errorf("unexpected content-type: %s", contentType)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxCratesIOResponseSize)

	var crateResp cratesIOCrateResponse
	if err := json.NewDecoder(limitedReader).Decode(&crateResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &crateResp, nil
}

// discoverExecutables attempts to find executable names from Cargo.toml
// Returns the executables list and any warnings generated during discovery
func (b *CargoBuilder) discoverExecutables(ctx context.Context, crateInfo *cratesIOCrateResponse) ([]string, []string) {
	var warnings []string

	// If no repository URL, fall back to crate name
	if crateInfo.Crate.Repository == "" {
		warnings = append(warnings, "No repository URL found; using crate name as executable")
		return []string{crateInfo.Crate.Name}, warnings
	}

	// Parse repository URL to construct Cargo.toml URL
	cargoTomlURL := b.buildCargoTomlURL(crateInfo.Crate.Repository)
	if cargoTomlURL == "" {
		warnings = append(warnings, fmt.Sprintf("Could not parse repository URL %s; using crate name as executable", crateInfo.Crate.Repository))
		return []string{crateInfo.Crate.Name}, warnings
	}

	// Fetch and parse Cargo.toml
	executables, err := b.fetchCargoTomlExecutables(ctx, cargoTomlURL)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Could not fetch Cargo.toml: %v; using crate name as executable", err))
		return []string{crateInfo.Crate.Name}, warnings
	}

	if len(executables) == 0 {
		// No [[bin]] sections found, use crate/package name
		return []string{crateInfo.Crate.Name}, warnings
	}

	return executables, warnings
}

// buildCargoTomlURL constructs the raw Cargo.toml URL from a repository URL
// Currently only supports GitHub repositories
func (b *CargoBuilder) buildCargoTomlURL(repoURL string) string {
	// Parse the repository URL
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return ""
	}

	// Only support GitHub for now
	if parsed.Host != "github.com" && parsed.Host != "www.github.com" {
		return ""
	}

	// Extract owner/repo from path (handle trailing slashes, .git suffix)
	path := strings.TrimSuffix(parsed.Path, "/")
	path = strings.TrimSuffix(path, ".git")
	path = strings.TrimPrefix(path, "/")

	parts := strings.Split(path, "/")
	if len(parts) < 2 {
		return ""
	}

	owner := parts[0]
	repo := parts[1]

	// Construct raw.githubusercontent.com URL
	// Try main branch first (most common), then master
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/Cargo.toml", owner, repo)
}

// fetchCargoTomlExecutables fetches Cargo.toml and extracts executable names
func (b *CargoBuilder) fetchCargoTomlExecutables(ctx context.Context, cargoTomlURL string) ([]string, error) {
	// Create context with timeout for Cargo.toml fetch
	ctx, cancel := context.WithTimeout(ctx, cargoTomlFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", cargoTomlURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Cargo.toml: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch Cargo.toml: status %d", resp.StatusCode)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxCargoTomlSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read Cargo.toml: %w", err)
	}

	// Parse TOML
	var cargo cargoToml
	if err := toml.Unmarshal(data, &cargo); err != nil {
		return nil, fmt.Errorf("failed to parse Cargo.toml: %w", err)
	}

	// Extract executable names from [[bin]] sections
	var executables []string
	for _, bin := range cargo.Bin {
		if bin.Name != "" && isValidExecutableName(bin.Name) {
			executables = append(executables, bin.Name)
		}
	}

	return executables, nil
}

// isValidCrateName validates a crate name
func isValidCrateName(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	return crateNameRegex.MatchString(name)
}

// isValidExecutableName validates an executable name
// Prevents shell metacharacter injection
func isValidExecutableName(name string) bool {
	if name == "" || len(name) > 255 {
		return false
	}

	// Must match: start with alphanumeric or underscore, contain only alphanumeric, underscores, dots, hyphens
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_][a-zA-Z0-9._-]*$`, name)
	return matched
}
