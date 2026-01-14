package version

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
)

// TapProvider resolves versions from third-party Homebrew taps via GitHub.
// It fetches formula files directly from GitHub raw content and parses them
// to extract version, bottle URLs, and checksums.
type TapProvider struct {
	resolver   *Resolver
	tap        string // e.g., "hashicorp/tap"
	formula    string // e.g., "terraform"
	httpClient *http.Client
}

// TapVersionInfo contains parsed metadata from a tap formula.
// This is an internal type used during parsing; the provider returns
// standard VersionInfo with Metadata map.
type TapVersionInfo struct {
	Version   string            // e.g., "1.7.0"
	Formula   string            // Full formula name
	BottleURL string            // Platform-specific bottle URL
	Checksum  string            // SHA256 checksum for this platform
	Extra     map[string]string // Additional metadata
}

// NewTapProvider creates a provider for third-party Homebrew taps
func NewTapProvider(resolver *Resolver, tap, formula string) *TapProvider {
	return &TapProvider{
		resolver:   resolver,
		tap:        tap,
		formula:    formula,
		httpClient: resolver.httpClient,
	}
}

// formulaLocations returns the possible paths where a formula file might be located
// in a Homebrew tap repository.
var formulaLocations = []string{
	"Formula/%s.rb",
	"HomebrewFormula/%s.rb",
	"%s.rb", // root directory
}

// ResolveLatest returns the latest version from the tap formula.
// The returned VersionInfo includes Metadata with "bottle_url", "checksum",
// "formula", and "tap" fields.
func (p *TapProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	// Parse tap into owner and repo
	owner, repo, err := parseTap(p.tap)
	if err != nil {
		return nil, err
	}

	// Try to fetch formula from different possible locations
	var content string
	var fetchErr error
	for _, loc := range formulaLocations {
		path := fmt.Sprintf(loc, p.formula)
		content, fetchErr = p.fetchFormulaFile(ctx, owner, repo, path)
		if fetchErr == nil {
			break
		}
	}
	if content == "" {
		return nil, fmt.Errorf("formula '%s' not found in tap '%s'", p.formula, p.tap)
	}

	// Parse the formula file
	info, err := parseFormulaFile(content)
	if err != nil {
		return nil, fmt.Errorf("failed to parse formula '%s': %w", p.formula, err)
	}

	// Select platform-specific bottle
	goos := runtime.GOOS
	goarch := runtime.GOARCH
	macOSVersion := 0 // TODO: Detect actual macOS version in future enhancement

	platformTags := getPlatformTags(goos, goarch, macOSVersion)
	if len(platformTags) == 0 {
		return nil, fmt.Errorf("no bottle available for platform %s/%s", goos, goarch)
	}

	// Find first matching platform
	var selectedPlatform string
	var checksum string
	for _, tag := range platformTags {
		if cs, ok := info.Checksums[tag]; ok {
			selectedPlatform = tag
			checksum = cs
			break
		}
	}

	if selectedPlatform == "" {
		return nil, fmt.Errorf("no bottle available for platform %s/%s (tried: %v)", goos, goarch, platformTags)
	}

	// Check for root_url - required for bottle URL construction
	if info.RootURL == "" {
		return nil, fmt.Errorf("no root_url specified in bottle block; cannot construct bottle URL")
	}

	// Construct bottle URL
	bottleURL := buildBottleURL(info.RootURL, p.formula, info.Version, selectedPlatform)

	return &VersionInfo{
		Tag:     info.Version,
		Version: info.Version,
		Metadata: map[string]string{
			"bottle_url": bottleURL,
			"checksum":   "sha256:" + checksum,
			"formula":    p.formula,
			"tap":        p.tap,
		},
	}, nil
}

// ResolveVersion resolves a specific version for the tap formula.
// For tap formulas, we can only resolve the version that's currently in the formula file.
func (p *TapProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	// Get the latest version first
	info, err := p.ResolveLatest(ctx)
	if err != nil {
		return nil, err
	}

	// Validate that the requested version matches what's in the formula
	if info.Version != version {
		return nil, fmt.Errorf("version %s not found for formula %s in tap %s (current version is %s)",
			version, p.formula, p.tap, info.Version)
	}

	return info, nil
}

// SourceDescription returns a human-readable source description
func (p *TapProvider) SourceDescription() string {
	return fmt.Sprintf("Tap:%s/%s", p.tap, p.formula)
}

// parseTap splits a tap string (e.g., "hashicorp/tap") into owner and repo parts.
// The repo is prefixed with "homebrew-" for the GitHub repository name.
func parseTap(tap string) (owner, repo string, err error) {
	parts := strings.Split(tap, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid tap format: %s (expected owner/repo)", tap)
	}
	owner = parts[0]
	repo = "homebrew-" + parts[1]
	return owner, repo, nil
}

// fetchFormulaFile fetches a formula file from GitHub raw content.
// URL format: https://raw.githubusercontent.com/{owner}/{repo}/HEAD/{path}
func (p *TapProvider) fetchFormulaFile(ctx context.Context, owner, repo, path string) (string, error) {
	url := fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/HEAD/%s", owner, repo, path)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch formula: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", fmt.Errorf("formula not found at %s", path)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub returned status %d", resp.StatusCode)
	}

	// Limit response size (10MB should be more than enough for any formula)
	limitedReader := io.LimitReader(resp.Body, 10*1024*1024)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return "", fmt.Errorf("failed to read formula: %w", err)
	}

	return string(body), nil
}
