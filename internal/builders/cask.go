package builders

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

const (
	// maxCaskAPIResponseSize limits response body to prevent memory exhaustion (10MB)
	maxCaskAPIResponseSize = 10 * 1024 * 1024
)

// CaskBuilder generates recipes from Homebrew Cask metadata.
// It queries the Homebrew Cask API and generates recipes using
// the cask version provider and app_bundle action.
type CaskBuilder struct {
	httpClient     *http.Client
	homebrewAPIURL string
}

// NewCaskBuilder creates a new CaskBuilder with the given HTTP client.
// If httpClient is nil, a default client with timeouts will be created.
func NewCaskBuilder(httpClient *http.Client) *CaskBuilder {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}
	return &CaskBuilder{
		httpClient:     httpClient,
		homebrewAPIURL: "https://formulae.brew.sh",
	}
}

// NewCaskBuilderWithBaseURL creates a new CaskBuilder with custom API URL (for testing)
func NewCaskBuilderWithBaseURL(httpClient *http.Client, baseURL string) *CaskBuilder {
	b := NewCaskBuilder(httpClient)
	b.homebrewAPIURL = baseURL
	return b
}

// Name returns the builder identifier
func (b *CaskBuilder) Name() string {
	return "cask"
}

// RequiresLLM returns false as this builder uses deterministic API metadata.
func (b *CaskBuilder) RequiresLLM() bool {
	return false
}

// CanBuild checks if the cask exists and has supported artifacts (app or binary).
func (b *CaskBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error) {
	caskName := req.Package
	if !isValidCaskName(caskName) {
		return false, nil
	}

	// Query cask API to check if cask exists and has supported artifacts
	caskInfo, err := b.fetchCaskInfo(ctx, caskName)
	if err != nil {
		// Not found is not an error - just means we can't build it
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}

	// Check for supported artifacts
	_, _, _, err = b.extractArtifacts(caskInfo.Artifacts)
	if err != nil {
		// Unsupported artifact type - we can't build this cask
		return false, nil
	}

	return true, nil
}

// NewSession creates a new build session for the given request.
func (b *CaskBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
	return NewDeterministicSession(b.Build, req), nil
}

// Build generates a recipe for the cask
func (b *CaskBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	if !isValidCaskName(req.Package) {
		return nil, fmt.Errorf("invalid cask name: %s", req.Package)
	}

	// Fetch cask metadata
	caskInfo, err := b.fetchCaskInfo(ctx, req.Package)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cask info: %w", err)
	}

	// Extract artifacts
	appName, binaries, hasApp, err := b.extractArtifacts(caskInfo.Artifacts)
	if err != nil {
		return nil, fmt.Errorf("unsupported cask: %w", err)
	}

	result := &BuildResult{
		Source:   fmt.Sprintf("cask:%s", req.Package),
		Warnings: []string{},
	}

	// Determine description - use first name element or token
	description := caskInfo.Desc
	if description == "" && len(caskInfo.Name) > 0 {
		description = caskInfo.Name[0]
	}

	// Build the recipe
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:        req.Package,
			Description: description,
			Homepage:    caskInfo.Homepage,
		},
		Version: recipe.VersionSection{
			Source: "cask",
			Cask:   caskInfo.Token,
		},
	}

	// Build app_bundle step parameters
	stepParams := map[string]interface{}{
		"url":      "{{version.url}}",
		"checksum": "{{version.checksum}}",
	}

	if hasApp {
		stepParams["app_name"] = appName
	}

	if len(binaries) > 0 {
		stepParams["binaries"] = binaries
	}

	// Add symlink_applications for app bundles
	if hasApp {
		stepParams["symlink_applications"] = true
	}

	r.Steps = []recipe.Step{
		{
			Action: "app_bundle",
			Params: stepParams,
		},
	}

	// Set verify command
	if len(binaries) > 0 {
		// Use first binary for verification
		r.Verify = recipe.VerifySection{
			Command: fmt.Sprintf("%s --version", binaries[0]),
		}
	} else if hasApp {
		// Use app bundle existence for verification
		r.Verify = recipe.VerifySection{
			Command: fmt.Sprintf("test -d \"$TSUKU_HOME/apps/%s-%s.app\"", req.Package, "{{version}}"),
		}
	}

	// Add warnings if no checksum
	if caskInfo.SHA256 == "" || caskInfo.SHA256 == ":no_check" {
		result.Warnings = append(result.Warnings,
			"Cask does not provide checksum verification (--allow-no-checksum may be required)")
	}

	result.Recipe = r
	return result, nil
}

// caskAPIResponse represents the Homebrew Cask API response structure.
// This includes the artifacts array which is needed for builder functionality.
type caskAPIResponse struct {
	Token    string   `json:"token"`    // Cask name (e.g., "visual-studio-code")
	Version  string   `json:"version"`  // Version string (e.g., "1.96.4")
	SHA256   string   `json:"sha256"`   // SHA256 checksum
	URL      string   `json:"url"`      // Download URL
	Name     []string `json:"name"`     // Human-readable name(s)
	Desc     string   `json:"desc"`     // Description
	Homepage string   `json:"homepage"` // Homepage URL

	// Artifacts is a heterogeneous array where each element is an object
	// with one key (app, binary, pkg, zap, etc.)
	// Example: [{"app": ["Firefox.app"]}, {"binary": ["{{appdir}}/Contents/MacOS/firefox"]}]
	Artifacts []map[string]interface{} `json:"artifacts"`

	// Variations for architecture-specific overrides
	Variations map[string]caskVariation `json:"variations"`
}

// caskVariation represents an OS/architecture-specific override
type caskVariation struct {
	URL     string `json:"url,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Version string `json:"version,omitempty"`
}

// fetchCaskInfo fetches cask metadata from Homebrew Cask API
func (b *CaskBuilder) fetchCaskInfo(ctx context.Context, caskName string) (*caskAPIResponse, error) {
	baseURL, err := url.Parse(b.homebrewAPIURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	apiURL := baseURL.JoinPath("api", "cask", caskName+".json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch cask info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("cask %s not found", caskName)
	}

	if resp.StatusCode == http.StatusTooManyRequests {
		return nil, fmt.Errorf("Homebrew API rate limit exceeded")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Homebrew API returned status %d", resp.StatusCode)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxCaskAPIResponseSize)

	var caskResp caskAPIResponse
	if err := json.NewDecoder(limitedReader).Decode(&caskResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Apply architecture-specific variations
	b.applyVariations(&caskResp)

	return &caskResp, nil
}

// applyVariations applies architecture-specific overrides to the cask response
func (b *CaskBuilder) applyVariations(info *caskAPIResponse) {
	if len(info.Variations) == 0 {
		return
	}

	goarch := runtime.GOARCH

	if goarch == "arm64" {
		// For Apple Silicon, look for arm64_* variations
		for key, v := range info.Variations {
			if strings.HasPrefix(key, "arm64_") {
				if v.URL != "" {
					info.URL = v.URL
				}
				if v.SHA256 != "" {
					info.SHA256 = v.SHA256
				}
				if v.Version != "" {
					info.Version = v.Version
				}
				return
			}
		}
	} else {
		// For Intel (amd64), look for non-arm64 variations
		for key, v := range info.Variations {
			if !strings.HasPrefix(key, "arm64_") {
				if v.URL != "" {
					info.URL = v.URL
				}
				if v.SHA256 != "" {
					info.SHA256 = v.SHA256
				}
				if v.Version != "" {
					info.Version = v.Version
				}
				return
			}
		}
	}
}

// extractArtifacts parses the artifacts array and extracts app name and binary paths.
// Returns (appName, binaries, hasApp, error).
// Returns error if the cask uses unsupported artifact types (pkg, preflight, postflight).
func (b *CaskBuilder) extractArtifacts(artifacts []map[string]interface{}) (string, []string, bool, error) {
	var appName string
	var binaries []string
	hasApp := false

	for _, artifact := range artifacts {
		// Check for unsupported artifact types
		if _, ok := artifact["pkg"]; ok {
			return "", nil, false, &CaskUnsupportedArtifactError{ArtifactType: "pkg"}
		}
		if _, ok := artifact["preflight"]; ok {
			return "", nil, false, &CaskUnsupportedArtifactError{ArtifactType: "preflight"}
		}
		if _, ok := artifact["postflight"]; ok {
			return "", nil, false, &CaskUnsupportedArtifactError{ArtifactType: "postflight"}
		}
		if _, ok := artifact["installer"]; ok {
			return "", nil, false, &CaskUnsupportedArtifactError{ArtifactType: "installer"}
		}

		// Extract app artifact
		if appArr, ok := artifact["app"]; ok {
			if apps, ok := appArr.([]interface{}); ok && len(apps) > 0 {
				if name, ok := apps[0].(string); ok {
					appName = name
					hasApp = true
				}
			}
		}

		// Extract binary artifacts
		if binArr, ok := artifact["binary"]; ok {
			if bins, ok := binArr.([]interface{}); ok {
				for _, bin := range bins {
					if binPath, ok := bin.(string); ok {
						normalized := normalizeBinaryPath(binPath, appName)
						// Extract just the binary name for the binaries array
						binaries = append(binaries, normalized)
					}
				}
			}
		}
	}

	// Must have at least app or binary artifact
	if !hasApp && len(binaries) == 0 {
		return "", nil, false, fmt.Errorf("no app or binary artifacts found in cask")
	}

	return appName, binaries, hasApp, nil
}

// normalizeBinaryPath converts Homebrew binary paths to paths relative to the .app bundle.
// Homebrew uses {{appdir}} placeholder to reference the app bundle root.
// Example: "{{appdir}}/Visual Studio Code.app/Contents/Resources/app/bin/code"
// becomes: "Contents/Resources/app/bin/code"
func normalizeBinaryPath(binPath, appName string) string {
	// Remove {{appdir}}/ prefix if present
	binPath = strings.TrimPrefix(binPath, "{{appdir}}/")

	// If the path includes the app bundle name, extract the relative path
	if appName != "" && strings.Contains(binPath, appName+"/") {
		parts := strings.SplitN(binPath, appName+"/", 2)
		if len(parts) == 2 {
			return parts[1]
		}
	}

	// If path starts with Contents/, it's already relative to app bundle
	if strings.HasPrefix(binPath, "Contents/") {
		return binPath
	}

	// For binary-only paths (no app bundle), just return the basename
	// These are typically symlinks, so we use the basename
	parts := strings.Split(binPath, "/")
	return parts[len(parts)-1]
}

// CaskUnsupportedArtifactError indicates a cask uses artifact types not supported by tsuku.
type CaskUnsupportedArtifactError struct {
	ArtifactType string
}

func (e *CaskUnsupportedArtifactError) Error() string {
	return fmt.Sprintf("cask uses unsupported %s artifact (tsuku only supports app and binary artifacts)", e.ArtifactType)
}

// CaskNotFoundError indicates a cask was not found in the Homebrew Cask API.
type CaskNotFoundError struct {
	CaskName string
}

func (e *CaskNotFoundError) Error() string {
	return fmt.Sprintf("cask %s not found in Homebrew Cask registry", e.CaskName)
}

// isValidCaskName validates Homebrew Cask names.
// Cask names follow similar rules to formula names:
// - Lowercase letters, numbers, hyphens, underscores
// - May contain @ for versioned casks
// - No path separators or shell metacharacters
func isValidCaskName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}

	// Reject dangerous patterns
	if strings.Contains(name, "..") || strings.Contains(name, "/") ||
		strings.Contains(name, "\\") || strings.HasPrefix(name, "-") {
		return false
	}

	// Check allowed characters
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '@' || c == '.') {
			return false
		}
	}

	return true
}

// Probe checks if a cask exists on Homebrew Cask.
func (b *CaskBuilder) Probe(ctx context.Context, name string) (*ProbeResult, error) {
	_, err := b.fetchCaskInfo(ctx, name)
	if err != nil {
		return &ProbeResult{Exists: false}, nil
	}
	return &ProbeResult{
		Exists: true,
		Source: name,
	}, nil
}
