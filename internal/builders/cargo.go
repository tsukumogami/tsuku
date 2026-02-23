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
	// maxCratesIOResponseSize limits response body to prevent memory exhaustion (10MB)
	maxCratesIOResponseSize = 10 * 1024 * 1024
)

// cratesIOCrateResponse represents the crates.io API response for a crate
type cratesIOCrateResponse struct {
	Crate struct {
		Name            string `json:"name"`
		Description     string `json:"description"`
		Homepage        string `json:"homepage"`
		Repository      string `json:"repository"`
		RecentDownloads int    `json:"recent_downloads"`
		// exact_match is not used but documented for reference
	} `json:"crate"`
	Versions []cratesIOVersion `json:"versions"`
}

// cratesIOVersion represents a single version object from the crates.io API.
type cratesIOVersion struct {
	BinNames []string `json:"bin_names"`
	Yanked   bool     `json:"yanked"`
}

// Pre-compile regex for crate name validation
var crateNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// CargoBuilder generates recipes for Rust crates from crates.io
type CargoBuilder struct {
	httpClient      *http.Client
	cratesIOBaseURL string
	// cachedCrateInfo stores the last fetchCrateInfo response so that
	// AuthoritativeBinaryNames() can return bin_names without re-fetching.
	// Populated during Build() and read by the orchestrator via
	// the BinaryNameProvider interface (#1938).
	cachedCrateInfo *cratesIOCrateResponse
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

	// Cache the response for BinaryNameProvider (#1938)
	b.cachedCrateInfo = crateInfo

	result := &BuildResult{
		Source:   fmt.Sprintf("crates.io:%s", req.Package),
		Warnings: []string{},
	}

	// Discover executables from the crates.io API bin_names field
	executables, execWarnings := b.discoverExecutables(ctx, crateInfo)
	result.Warnings = append(result.Warnings, execWarnings...)

	// Build the recipe
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        req.Package,
			Description: strings.TrimSpace(crateInfo.Crate.Description),
			Homepage:    crateInfo.Crate.Homepage,
		},
		Version: recipe.VersionSection{},
		Steps: []recipe.Step{
			{
				Action: "cargo_install",
				Params: map[string]interface{}{
					"crate":       req.Package,
					"executables": executables,
				},
			},
		},
		Verify: cargoVerifySection(executables[0]),
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

// discoverExecutables reads bin_names from the crates.io API response to
// determine what executables cargo install will produce. It uses the latest
// non-yanked version's bin_names array, falling back to the crate name when
// bin_names is empty (library-only crate) or all versions are yanked.
func (b *CargoBuilder) discoverExecutables(_ context.Context, crateInfo *cratesIOCrateResponse) ([]string, []string) {
	var warnings []string

	// Find bin_names from the latest non-yanked version.
	// The crates.io API returns versions ordered by publication date (newest first).
	var binNames []string
	foundNonYanked := false
	for _, v := range crateInfo.Versions {
		if v.Yanked {
			continue
		}
		foundNonYanked = true
		binNames = v.BinNames
		break
	}

	if !foundNonYanked {
		warnings = append(warnings, "All versions are yanked; using crate name as executable")
		return []string{crateInfo.Crate.Name}, warnings
	}

	// Filter bin_names through executable name validation
	var executables []string
	for _, name := range binNames {
		if isValidExecutableName(name) {
			executables = append(executables, name)
		} else {
			warnings = append(warnings, fmt.Sprintf("Skipping invalid executable name from bin_names: %q", name))
		}
	}

	if len(executables) == 0 {
		// bin_names was empty or all entries were invalid; fall back to crate name
		if len(binNames) == 0 {
			warnings = append(warnings, "No bin_names in API response; using crate name as executable")
		}
		return []string{crateInfo.Crate.Name}, warnings
	}

	return executables, warnings
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

// cargoVerifySection builds the verify section for a cargo crate. Cargo
// subcommands (executables named cargo-*) must be invoked through cargo
// itself for --version to work, so we generate "cargo <subcommand> --version"
// instead of "cargo-<subcommand> --version".
func cargoVerifySection(executable string) *recipe.VerifySection {
	var command string
	if strings.HasPrefix(executable, "cargo-") {
		subcommand := strings.TrimPrefix(executable, "cargo-")
		command = fmt.Sprintf("cargo %s --version", subcommand)
	} else {
		command = fmt.Sprintf("%s --version", executable)
	}
	return &recipe.VerifySection{
		Command: command,
		Pattern: "{version}",
	}
}

// AuthoritativeBinaryNames returns the executable names from the cached
// crates.io API response. This implements BinaryNameProvider so the
// orchestrator can cross-check recipe executables against registry metadata.
//
// Returns nil if Build() hasn't been called yet (no cached data), or if the
// API response has no usable bin_names.
func (b *CargoBuilder) AuthoritativeBinaryNames() []string {
	if b.cachedCrateInfo == nil {
		return nil
	}

	// Use the same logic as discoverExecutables: find the first non-yanked
	// version and return its bin_names (filtered through validation).
	for _, v := range b.cachedCrateInfo.Versions {
		if v.Yanked {
			continue
		}
		var names []string
		for _, name := range v.BinNames {
			if isValidExecutableName(name) {
				names = append(names, name)
			}
		}
		return names
	}

	return nil
}

// Probe checks if a crate exists on crates.io and returns quality metadata.
func (b *CargoBuilder) Probe(ctx context.Context, name string) (*ProbeResult, error) {
	info, err := b.fetchCrateInfo(ctx, name)
	if err != nil {
		return nil, nil
	}
	return &ProbeResult{
		Source:        name,
		Downloads:     info.Crate.RecentDownloads,
		VersionCount:  len(info.Versions),
		HasRepository: info.Crate.Repository != "",
	}, nil
}

// cratesIOSearchResponse represents the crates.io category listing response.
type cratesIOSearchResponse struct {
	Crates []struct {
		Name            string `json:"name"`
		RecentDownloads int    `json:"recent_downloads"`
	} `json:"crates"`
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
}

// Discover lists popular CLI crates from crates.io by querying the
// command-line-utilities category sorted by downloads. It paginates
// through results up to the requested limit, enforcing 1 request/second
// rate limiting between pages.
func (b *CargoBuilder) Discover(ctx context.Context, limit int) ([]DiscoveryCandidate, error) {
	if limit <= 0 {
		return nil, nil
	}

	const perPage = 100
	var candidates []DiscoveryCandidate
	page := 1

	for len(candidates) < limit {
		select {
		case <-ctx.Done():
			return candidates, ctx.Err()
		default:
		}

		baseURL, err := url.Parse(b.cratesIOBaseURL)
		if err != nil {
			return nil, fmt.Errorf("invalid base URL: %w", err)
		}
		apiURL := baseURL.JoinPath("api", "v1", "crates")

		q := apiURL.Query()
		q.Set("category", "command-line-utilities")
		q.Set("sort", "downloads")
		q.Set("per_page", fmt.Sprintf("%d", perPage))
		q.Set("page", fmt.Sprintf("%d", page))
		apiURL.RawQuery = q.Encode()

		req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")
		req.Header.Set("Accept", "application/json")

		resp, err := b.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("fetch crates page %d: %w", page, err)
		}

		if resp.StatusCode == 429 {
			resp.Body.Close()
			return nil, fmt.Errorf("crates.io rate limit exceeded on page %d", page)
		}
		if resp.StatusCode != 200 {
			resp.Body.Close()
			return nil, fmt.Errorf("crates.io returned status %d on page %d", resp.StatusCode, page)
		}

		limitedReader := io.LimitReader(resp.Body, maxCratesIOResponseSize)
		var searchResp cratesIOSearchResponse
		if err := json.NewDecoder(limitedReader).Decode(&searchResp); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("parse crates page %d: %w", page, err)
		}
		resp.Body.Close()

		if len(searchResp.Crates) == 0 {
			break
		}

		for _, c := range searchResp.Crates {
			if len(candidates) >= limit {
				break
			}
			candidates = append(candidates, DiscoveryCandidate{
				Name:      c.Name,
				Downloads: c.RecentDownloads,
			})
		}

		// If we got fewer than a full page, there are no more results.
		if len(searchResp.Crates) < perPage {
			break
		}

		page++

		// Rate limit: 1 request/second between pages.
		select {
		case <-ctx.Done():
			return candidates, ctx.Err()
		case <-time.After(1 * time.Second):
		}
	}

	return candidates, nil
}
