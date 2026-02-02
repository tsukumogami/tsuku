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
	// maxGoProxyResponseSize limits response body to prevent memory exhaustion (1MB)
	maxGoProxyResponseSize = 1 * 1024 * 1024
)

// goProxyLatestResponse represents the proxy.golang.org /@latest response
type goProxyLatestResponse struct {
	Version string `json:"Version"` // e.g., "v1.2.3"
	Time    string `json:"Time"`    // e.g., "2024-01-15T10:30:00Z"
}

// Pre-compile regex for Go module path validation
// Go module paths: domain/path format with alphanumeric, slashes, hyphens, dots, underscores
var goModuleRegex = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9.-]*(/[a-zA-Z0-9._-]+)+$`)

// GoBuilder generates recipes for Go modules from proxy.golang.org
type GoBuilder struct {
	httpClient     *http.Client
	goProxyBaseURL string
}

// NewGoBuilder creates a new GoBuilder with the given HTTP client.
// If httpClient is nil, a default client with timeouts will be created.
func NewGoBuilder(httpClient *http.Client) *GoBuilder {
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: 60 * time.Second,
		}
	}
	return &GoBuilder{
		httpClient:     httpClient,
		goProxyBaseURL: "https://proxy.golang.org",
	}
}

// NewGoBuilderWithBaseURL creates a new GoBuilder with custom proxy URL (for testing)
func NewGoBuilderWithBaseURL(httpClient *http.Client, baseURL string) *GoBuilder {
	b := NewGoBuilder(httpClient)
	b.goProxyBaseURL = baseURL
	return b
}

// Name returns the builder identifier
func (b *GoBuilder) Name() string {
	return "go"
}

// RequiresLLM returns false as this builder uses ecosystem APIs, not LLM.
func (b *GoBuilder) RequiresLLM() bool {
	return false
}

// CanBuild checks if the module exists on proxy.golang.org
func (b *GoBuilder) CanBuild(ctx context.Context, req BuildRequest) (bool, error) {
	if !isValidGoModule(req.Package) {
		return false, nil
	}

	// Query Go proxy to check if module exists
	_, err := b.fetchModuleInfo(ctx, req.Package)
	if err != nil {
		// Not found is not an error - just means we can't build it
		if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "gone") {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// NewSession creates a new build session for the given request.
func (b *GoBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
	return NewDeterministicSession(b.Build, req), nil
}

// Build generates a recipe for the Go module
func (b *GoBuilder) Build(ctx context.Context, req BuildRequest) (*BuildResult, error) {
	if !isValidGoModule(req.Package) {
		return nil, fmt.Errorf("invalid Go module path: %s", req.Package)
	}

	// Validate module exists by fetching info from Go proxy
	if _, err := b.fetchModuleInfo(ctx, req.Package); err != nil {
		return nil, fmt.Errorf("failed to fetch module info: %w", err)
	}

	result := &BuildResult{
		Source:   fmt.Sprintf("goproxy:%s", req.Package),
		Warnings: []string{},
	}

	// Infer executable name from module path
	executable, warning := inferGoExecutableName(req.Package)
	if warning != "" {
		result.Warnings = append(result.Warnings, warning)
	}

	// Build the recipe
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name:         executable,
			Description:  fmt.Sprintf("Go CLI tool from %s", req.Package),
			Homepage:     fmt.Sprintf("https://pkg.go.dev/%s", req.Package),
			Dependencies: []string{"go"},
		},
		Version: recipe.VersionSection{
			Source: "goproxy",
		},
		Steps: []recipe.Step{
			{
				Action: "go_install",
				Params: map[string]interface{}{
					"module":      req.Package,
					"executables": []string{executable},
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

// fetchModuleInfo fetches module metadata from proxy.golang.org
func (b *GoBuilder) fetchModuleInfo(ctx context.Context, modulePath string) (*goProxyLatestResponse, error) {
	// Encode module path (uppercase letters need special encoding)
	encodedPath := encodeGoModulePath(modulePath)

	baseURL, err := url.Parse(b.goProxyBaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %w", err)
	}
	apiURL := baseURL.JoinPath(encodedPath, "@latest")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Go proxy recommends a User-Agent header
	req.Header.Set("User-Agent", "tsuku/1.0 (https://github.com/tsukumogami/tsuku)")
	req.Header.Set("Accept", "application/json")

	resp, err := b.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch module info: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("module %s not found on proxy.golang.org", modulePath)
	}

	if resp.StatusCode == 410 {
		return nil, fmt.Errorf("module %s is gone (retracted)", modulePath)
	}

	if resp.StatusCode == 429 {
		return nil, fmt.Errorf("proxy.golang.org rate limit exceeded")
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("proxy.golang.org returned status %d", resp.StatusCode)
	}

	// Limit response size
	limitedReader := io.LimitReader(resp.Body, maxGoProxyResponseSize)

	var moduleResp goProxyLatestResponse
	if err := json.NewDecoder(limitedReader).Decode(&moduleResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &moduleResp, nil
}

// encodeGoModulePath encodes a Go module path for use in proxy URLs
// Uppercase letters in module paths are encoded as ! followed by lowercase
// Example: github.com/User/Repo -> github.com/!user/!repo
func encodeGoModulePath(path string) string {
	var result strings.Builder
	for _, c := range path {
		if c >= 'A' && c <= 'Z' {
			result.WriteByte('!')
			result.WriteByte(byte(c - 'A' + 'a'))
		} else {
			result.WriteRune(c)
		}
	}
	return result.String()
}

// isValidGoModule validates Go module paths to prevent command injection
// Valid paths: domain/path format with alphanumeric, slashes, hyphens, dots, underscores
// Must contain at least one slash, max 256 characters
func isValidGoModule(path string) bool {
	if path == "" || len(path) > 256 {
		return false
	}

	// Must contain at least one slash (domain/path format)
	if !strings.Contains(path, "/") {
		return false
	}

	// Must start with a letter (domain names start with letters)
	first := path[0]
	if !((first >= 'a' && first <= 'z') || (first >= 'A' && first <= 'Z')) {
		return false
	}

	// Reject shell metacharacters and path traversal
	if strings.ContainsAny(path, ";$`|&<>(){}[]'\"\\") {
		return false
	}

	// Reject double slashes and path traversal
	if strings.Contains(path, "//") || strings.Contains(path, "..") {
		return false
	}

	return goModuleRegex.MatchString(path)
}

// inferGoExecutableName infers the executable name from a Go module path
// Uses the last segment of the path as the executable name
// Examples:
//   - github.com/jesseduffield/lazygit -> lazygit
//   - github.com/golangci/golangci-lint/cmd/golangci-lint -> golangci-lint
//   - mvdan.cc/gofumpt -> gofumpt
//
// Returns the executable name and an optional warning
func inferGoExecutableName(modulePath string) (string, string) {
	parts := strings.Split(modulePath, "/")
	if len(parts) == 0 {
		return "unknown", "Could not infer executable name from module path"
	}

	// Use the last path segment as the executable name
	executable := parts[len(parts)-1]

	// Validate the inferred name
	if executable == "" || !isValidExecutableName(executable) {
		return "unknown", fmt.Sprintf("Could not infer valid executable name from '%s'", modulePath)
	}

	return executable, ""
}

// Probe checks if a module exists on proxy.golang.org.
func (b *GoBuilder) Probe(ctx context.Context, name string) (*ProbeResult, error) {
	_, err := b.fetchModuleInfo(ctx, name)
	if err != nil {
		return nil, nil
	}
	return &ProbeResult{Source: name}, nil
}
