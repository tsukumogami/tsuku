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
	// maxMetaCPANResponseSize limits response body to prevent memory exhaustion (10MB)
	maxMetaCPANResponseSize = 10 * 1024 * 1024
)

// metacpanRelease represents the MetaCPAN API response for /release/{distribution}
type metacpanRelease struct {
	Distribution string `json:"distribution"` // Distribution name (e.g., "App-Ack")
	Version      string `json:"version"`      // Version number (e.g., "3.7.0")
	Abstract     string `json:"abstract"`     // Short description
}

// Pre-compile regex for distribution name validation
// Distribution names: start with letter, can contain letters, numbers, hyphens
var cpanDistributionRegex = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9]*(-[A-Za-z0-9]+)*$`)

// CPANBuilder generates recipes for CPAN distributions from metacpan.org
type CPANBuilder struct {
	httpClient      *http.Client
	metacpanBaseURL string
}

// NewCPANBuilder creates a new CPANBuilder with the given HTTP client.
// If httpClient is nil, a default client with timeouts will be created.
func NewCPANBuilder(httpClient *http.Client) *CPANBuilder {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}
	return &CPANBuilder{
		httpClient:      httpClient,
		metacpanBaseURL: "https://fastapi.metacpan.org/v1",
	}
}

// NewCPANBuilderWithBaseURL creates a new CPANBuilder with custom MetaCPAN URL (for testing)
func NewCPANBuilderWithBaseURL(httpClient *http.Client, baseURL string) *CPANBuilder {
	b := NewCPANBuilder(httpClient)
	b.metacpanBaseURL = baseURL
	return b
}

// Name returns the builder identifier
func (b *CPANBuilder) Name() string {
	return "cpan"
}

// RequiresLLM returns false as this builder uses ecosystem APIs, not LLM.
func (b *CPANBuilder) RequiresLLM() bool {
	return false
}

// CanBuild checks if the distribution exists on MetaCPAN
func (b *CPANBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error) {
	// Normalize module name to distribution name if needed
	distribution := normalizeToDistribution(req.Package)

	if !isValidCPANDistribution(distribution) {
		return false, nil
	}

	// Query MetaCPAN API to check if distribution exists
	_, err := b.fetchDistributionInfo(ctx, distribution)
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
func (b *CPANBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
	return NewDeterministicSession(b.Build, req), nil
}

// Build generates a recipe for the CPAN distribution
func (b *CPANBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	// Normalize module name to distribution name if needed
	distribution := normalizeToDistribution(req.Package)

	if !isValidCPANDistribution(distribution) {
		return nil, fmt.Errorf("invalid distribution name: %s", distribution)
	}

	// Fetch distribution metadata from MetaCPAN
	distInfo, err := b.fetchDistributionInfo(ctx, distribution)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch distribution info: %w", err)
	}

	result := &BuildResult{
		Source:   fmt.Sprintf("metacpan:%s", distribution),
		Warnings: []string{},
	}

	// Infer executable name from distribution
	executable, warning := inferExecutableName(distribution)
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}

	// Build the recipe
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:         executable,
			Description:  distInfo.Abstract,
			Homepage:     fmt.Sprintf("https://metacpan.org/dist/%s", distribution),
			Dependencies: []string{"perl"},
		},
		Version: recipe.VersionSection{
			Source: fmt.Sprintf("metacpan:%s", distribution),
		},
		Steps: []recipe.Step{
			{
				Action: "cpan_install",
				Params: map[string]interface{}{
					"distribution": distribution,
					"executables":  []string{executable},
				},
			},
		},
		Verify: recipe.VerifySection{
			Command: fmt.Sprintf("%s --version", executable),
		},
	}

	result.Recipe = r
	return result, nil
}

// fetchDistributionInfo fetches distribution metadata from MetaCPAN API
func (b *CPANBuilder) fetchDistributionInfo(ctx context.Context, distribution string) (*metacpanRelease, error) {
	baseURL, err := url.Parse(b.metacpanBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	apiURL := baseURL.JoinPath("release", distribution)

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// MetaCPAN recommends a User-Agent header
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch distribution info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("distribution %s not found on metacpan.org", distribution)
	}

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("metacpan.org rate limit exceeded")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("metacpan.org returned status %d", resp.StatusCode)
	}

	// Validate content type
	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		return nil, fmt.Errorf("unexpected content-type: %s", contentType)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxMetaCPANResponseSize)

	var release metacpanRelease
	if err := json.NewDecoder(limitedReader).Decode(&release); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &release, nil
}

// normalizeToDistribution converts module names (App::Ack) to distribution format (App-Ack)
func normalizeToDistribution(name string) string {
	return strings.ReplaceAll(name, "::", "-")
}

// isValidCPANDistribution validates CPAN distribution names
// Distribution names: start with letter, contain letters/numbers/hyphens, max 128 chars
func isValidCPANDistribution(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}
	return cpanDistributionRegex.MatchString(name)
}

// inferExecutableName infers the executable name from a distribution name
// Common patterns:
//   - App-Ack -> ack
//   - App-cpanminus -> cpanm (special case)
//   - Perl-Critic -> perlcritic
//   - File-Slurp -> file-slurp (library, no executable)
//
// Returns the executable name and an optional warning if inference is uncertain
func inferExecutableName(distribution string) (string, string) {
	name := distribution
	warning := ""

	// Handle App- prefix (most CLI tools)
	if strings.HasPrefix(name, "App-") {
		name = strings.TrimPrefix(name, "App-")
		// Convert to lowercase and remove hyphens
		name = strings.ToLower(name)
		name = strings.ReplaceAll(name, "-", "")
		return name, ""
	}

	// For non-App distributions, the executable name is less predictable
	// Convert to lowercase and join with empty string
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "-", "")

	warning = fmt.Sprintf("Inferred executable '%s' from distribution '%s'; verify this is correct", name, distribution)

	return name, warning
}

// Probe checks if a distribution exists on CPAN.
func (b *CPANBuilder) Probe(ctx context.Context, name string) (*ProbeResult, error) {
	dist := normalizeToDistribution(name)
	_, err := b.fetchDistributionInfo(ctx, dist)
	if err != nil {
		return nil, nil
	}
	return &ProbeResult{Source: dist}, nil
}
