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

	"github.com/tsuku-dev/tsuku/internal/recipe"
)

const (
	// maxRubyGemsResponseSize limits response body to prevent memory exhaustion (10MB)
	maxRubyGemsResponseSize = 10 * 1024 * 1024
	// maxGemspecSize limits gemspec to prevent memory exhaustion (1MB)
	maxGemspecSize = 1 * 1024 * 1024
	// gemspecFetchTimeout is the timeout for fetching gemspec from repository
	gemspecFetchTimeout = 10 * time.Second
)

// rubyGemsGemResponse represents the RubyGems API response for a gem
type rubyGemsGemResponse struct {
	Name          string `json:"name"`
	Info          string `json:"info"`
	HomepageURI   string `json:"homepage_uri"`
	SourceCodeURI string `json:"source_code_uri"`
}

// Pre-compile regex for gem name validation
var gemNameRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]*$`)

// GemBuilder generates recipes for Ruby gems from rubygems.org
type GemBuilder struct {
	httpClient      *http.Client
	rubyGemsBaseURL string
}

// NewGemBuilder creates a new GemBuilder with the given HTTP client.
// If httpClient is nil, a default client with timeouts will be created.
func NewGemBuilder(httpClient *http.Client) *GemBuilder {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}
	return &GemBuilder{
		httpClient:      httpClient,
		rubyGemsBaseURL: "https://rubygems.org",
	}
}

// NewGemBuilderWithBaseURL creates a new GemBuilder with custom rubygems URL (for testing)
func NewGemBuilderWithBaseURL(httpClient *http.Client, baseURL string) *GemBuilder {
	b := NewGemBuilder(httpClient)
	b.rubyGemsBaseURL = baseURL
	return b
}

// Name returns the builder identifier
func (b *GemBuilder) Name() string {
	return "rubygems"
}

// CanBuild checks if the gem exists on rubygems.org
func (b *GemBuilder) CanBuild(ctx context.Context, packageName string) (bool, error) {
	if !isValidGemName(packageName) {
		return false, nil
	}

	// Query RubyGems API to check if gem exists
	_, err := b.fetchGemInfo(ctx, packageName)
	if err != nil {
		// Not found is not an error - just means we can't build it
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// Build generates a recipe for the gem
func (b *GemBuilder) Build(ctx context.Context, packageName string, version string) (*BuildResult, error) {
	if !isValidGemName(packageName) {
		return nil, fmt.Errorf("invalid gem name: %s", packageName)
	}

	// Fetch gem metadata from RubyGems
	gemInfo, err := b.fetchGemInfo(ctx, packageName)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch gem info: %w", err)
	}

	result := &BuildResult{
		Source:   fmt.Sprintf("rubygems:%s", packageName),
		Warnings: []string{},
	}

	// Try to discover executables from gemspec
	executables, execWarnings := b.discoverExecutables(ctx, gemInfo)
	result.Warnings = append(result.Warnings, execWarnings...)

	// Build the recipe
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        packageName,
			Description: gemInfo.Info,
			Homepage:    gemInfo.HomepageURI,
		},
		Version: recipe.VersionSection{
			Source: "rubygems",
		},
		Steps: []recipe.Step{
			{
				Action: "gem_install",
				Params: map[string]interface{}{
					"gem":         packageName,
					"executables": executables,
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: fmt.Sprintf("%s --version", executables[0]),
		},
	}

	// Use source_code_uri as homepage if homepage is empty
	if r.Metadata.Homepage == "" && gemInfo.SourceCodeURI != "" {
		r.Metadata.Homepage = gemInfo.SourceCodeURI
	}

	result.Recipe = r
	return result, nil
}

// fetchGemInfo fetches gem metadata from RubyGems API
func (b *GemBuilder) fetchGemInfo(ctx context.Context, gemName string) (*rubyGemsGemResponse, error) {
	baseURL, err := url.Parse(b.rubyGemsBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	apiURL := baseURL.JoinPath("api", "v1", "gems", gemName+".json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// RubyGems recommends a User-Agent header
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsuku-dev/tsuku)")
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch gem info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("gem %s not found on rubygems.org", gemName)
	}

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("rubygems.org rate limit exceeded")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("rubygems.org returned status %d", resp.StatusCode)
	}

	// Validate content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return nil, fmt.Errorf("unexpected content-type: %s", contentType)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxRubyGemsResponseSize)

	var gemResp rubyGemsGemResponse
	if err := json.NewDecoder(limitedReader).Decode(&gemResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &gemResp, nil
}

// discoverExecutables attempts to find executable names from gemspec
// Returns the executables list and any warnings generated during discovery
func (b *GemBuilder) discoverExecutables(ctx context.Context, gemInfo *rubyGemsGemResponse) ([]string, []string) {
	var warnings []string

	// If no source code URL, fall back to gem name
	if gemInfo.SourceCodeURI == "" {
		warnings = append(warnings, "No source code URL found; using gem name as executable")
		return []string{gemInfo.Name}, warnings
	}

	// Parse source URL to construct gemspec URL
	gemspecURL := b.buildGemspecURL(gemInfo.SourceCodeURI, gemInfo.Name)
	if gemspecURL == "" {
		warnings = append(warnings, fmt.Sprintf("Could not parse source URL %s; using gem name as executable", gemInfo.SourceCodeURI))
		return []string{gemInfo.Name}, warnings
	}

	// Fetch and parse gemspec
	executables, err := b.fetchGemspecExecutables(ctx, gemspecURL)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Could not fetch gemspec: %v; using gem name as executable", err))
		return []string{gemInfo.Name}, warnings
	}

	if len(executables) == 0 {
		// No executables found, use gem name
		return []string{gemInfo.Name}, warnings
	}

	return executables, warnings
}

// buildGemspecURL constructs the raw gemspec URL from a source code URL
// Currently only supports GitHub repositories
func (b *GemBuilder) buildGemspecURL(sourceURL, gemName string) string {
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

	// Construct raw.githubusercontent.com URL for gemspec
	return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/%s.gemspec", owner, repo, gemName)
}

// fetchGemspecExecutables fetches gemspec and extracts executable names
func (b *GemBuilder) fetchGemspecExecutables(ctx context.Context, gemspecURL string) ([]string, error) {
	// Create context with timeout for gemspec fetch
	ctx, cancel := context.WithTimeout(ctx, gemspecFetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", gemspecURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsuku-dev/tsuku)")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch gemspec: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("failed to fetch gemspec: status %d", resp.StatusCode)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxGemspecSize)
	data, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read gemspec: %w", err)
	}

	// Parse executables from gemspec content
	// Look for patterns like: spec.executables = ["jekyll"]
	// or: s.executables = gem.files.grep(%r{^exe/}) { |f| File.basename(f) }
	executables := parseGemspecExecutables(string(data))

	return executables, nil
}

// parseGemspecExecutables extracts executables from gemspec content
// Handles common patterns like:
// - spec.executables = ["bin1", "bin2"]
// - s.executables = %w[bin1 bin2]
// - spec.executables << "bin"
func parseGemspecExecutables(content string) []string {
	var executables []string

	// Pattern 1: executables = ["name1", "name2"]
	arrayPattern := regexp.MustCompile(`\.executables\s*=\s*\[([^\]]+)\]`)
	if matches := arrayPattern.FindStringSubmatch(content); len(matches) > 1 {
		// Parse array elements
		elements := strings.Split(matches[1], ",")
		for _, elem := range elements {
			elem = strings.TrimSpace(elem)
			// Remove quotes
			elem = strings.Trim(elem, `"'`)
			if isValidExecutableName(elem) {
				executables = append(executables, elem)
			}
		}
	}

	// Pattern 2: executables = %w[name1 name2]
	wordArrayPattern := regexp.MustCompile(`\.executables\s*=\s*%w[\[\(]([^\]\)]+)[\]\)]`)
	if matches := wordArrayPattern.FindStringSubmatch(content); len(matches) > 1 {
		elements := strings.Fields(matches[1])
		for _, elem := range elements {
			if isValidExecutableName(elem) {
				executables = append(executables, elem)
			}
		}
	}

	return executables
}

// isValidGemName validates a gem name
func isValidGemName(name string) bool {
	if name == "" || len(name) > 100 {
		return false
	}
	return gemNameRegex.MatchString(name)
}
