package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
)

const (
	// Max response size: 1MB (Cask responses are typically small)
	maxCaskResponseSize = 1 * 1024 * 1024
)

// CaskProvider resolves versions from Homebrew Cask API.
// API: https://formulae.brew.sh/api/cask/{name}.json
type CaskProvider struct {
	resolver *Resolver
	cask     string
}

// NewCaskProvider creates a provider for Homebrew Casks
func NewCaskProvider(resolver *Resolver, cask string) *CaskProvider {
	return &CaskProvider{
		resolver: resolver,
		cask:     cask,
	}
}

// homebrewCaskInfo represents the response from the Homebrew Cask API.
// API: https://formulae.brew.sh/api/cask/{name}.json
type homebrewCaskInfo struct {
	Token   string `json:"token"`   // Cask name (e.g., "visual-studio-code")
	Version string `json:"version"` // Version string (e.g., "1.96.4")
	SHA256  string `json:"sha256"`  // SHA256 checksum, or ":no_check" if not verified
	URL     string `json:"url"`     // Download URL (may be architecture-specific)

	// Variations contains architecture/OS-specific overrides.
	// Keys are OS variants like "arm64_sonoma", "ventura", etc.
	// Values override url, sha256, and/or version for that variant.
	Variations map[string]caskVariation `json:"variations"`
}

// caskVariation represents an OS/architecture-specific override in the Cask API response.
type caskVariation struct {
	URL     string `json:"url,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
	Version string `json:"version,omitempty"`
}

// ResolveLatest returns the latest stable version from Homebrew Cask API.
// The returned VersionInfo includes Metadata with "url" and "checksum" fields.
func (p *CaskProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	return p.resolver.ResolveCask(ctx, p.cask)
}

// ResolveVersion resolves a specific version for Homebrew Casks.
// Note: The Cask API only provides the current version. This method validates
// that the requested version matches the current version.
func (p *CaskProvider) ResolveVersion(ctx context.Context, version string) (*VersionInfo, error) {
	info, err := p.resolver.ResolveCask(ctx, p.cask)
	if err != nil {
		return nil, err
	}

	// Cask API only provides current version - validate requested version matches
	if info.Version != version {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "cask",
			Message: fmt.Sprintf("version %s not found for cask %s (current version is %s)", version, p.cask, info.Version),
		}
	}

	return info, nil
}

// SourceDescription returns a human-readable source description
func (p *CaskProvider) SourceDescription() string {
	return fmt.Sprintf("Cask:%s", p.cask)
}

// ResolveCask fetches cask information from the Homebrew Cask API.
// Returns VersionInfo with Metadata containing "url" and "checksum" fields.
//
// Architecture selection:
// - arm64 (Apple Silicon): Look for arm64_* variation, fall back to base URL
// - amd64 (Intel): Use base URL or non-arm64 variation
//
// Missing checksum handling:
// - Some casks use ":no_check" or empty checksum
// - These are returned as empty string in Metadata["checksum"]
func (r *Resolver) ResolveCask(ctx context.Context, cask string) (*VersionInfo, error) {
	// Validate cask name (same rules as Homebrew formula)
	if !isValidCaskName(cask) {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "cask",
			Message: fmt.Sprintf("invalid cask name: %s", cask),
		}
	}

	// SECURITY: Use url.Parse for proper URL construction
	registryURL := r.caskRegistryURL
	if registryURL == "" {
		registryURL = "https://formulae.brew.sh"
	}

	baseURL, err := url.Parse(registryURL)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "cask",
			Message: "failed to parse base URL",
			Err:     err,
		}
	}
	apiURL := baseURL.JoinPath("api", "cask", cask+".json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "cask",
			Message: "failed to create request",
			Err:     err,
		}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "cask", "failed to fetch cask info")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "cask",
			Message: fmt.Sprintf("cask %s not found", cask),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "cask",
			Message: fmt.Sprintf("unexpected status code: %d", resp.StatusCode),
		}
	}

	// SECURITY: Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxCaskResponseSize)

	var caskInfo homebrewCaskInfo
	if err := json.NewDecoder(limitedReader).Decode(&caskInfo); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "cask",
			Message: "failed to parse cask response",
			Err:     err,
		}
	}

	// Select architecture-appropriate URL and checksum
	downloadURL, checksum, version := selectCaskVariation(&caskInfo)

	// Handle missing checksum (":no_check" or empty)
	if checksum == ":no_check" {
		checksum = ""
	}

	// Format checksum with sha256 prefix if present
	formattedChecksum := ""
	if checksum != "" {
		formattedChecksum = "sha256:" + checksum
	}

	return &VersionInfo{
		Tag:     version,
		Version: version,
		Metadata: map[string]string{
			"url":      downloadURL,
			"checksum": formattedChecksum,
		},
	}, nil
}

// selectCaskVariation selects the appropriate URL and checksum based on architecture.
// Returns (url, checksum, version) for the current platform.
//
// The Cask API uses OS version names as variation keys (e.g., "sonoma", "arm64_sonoma").
// For arm64, we look for "arm64_*" variations first, then fall back to base.
// For amd64, we use the base URL (which typically targets Intel Macs).
func selectCaskVariation(info *homebrewCaskInfo) (url, checksum, version string) {
	// Start with base values
	url = info.URL
	checksum = info.SHA256
	version = info.Version

	// If no variations, use base values
	if len(info.Variations) == 0 {
		return url, checksum, version
	}

	// Select based on current architecture
	goarch := runtime.GOARCH

	if goarch == "arm64" {
		// For Apple Silicon, look for arm64_* variations
		// Try common macOS version names with arm64 prefix
		for key, v := range info.Variations {
			if strings.HasPrefix(key, "arm64_") {
				// Use arm64 variation
				if v.URL != "" {
					url = v.URL
				}
				if v.SHA256 != "" {
					checksum = v.SHA256
				}
				if v.Version != "" {
					version = v.Version
				}
				return url, checksum, version
			}
		}
	} else {
		// For Intel (amd64), look for non-arm64 variations or use base
		// Try common macOS version names without arm64 prefix
		for key, v := range info.Variations {
			if !strings.HasPrefix(key, "arm64_") {
				// Use Intel variation if it overrides the base
				if v.URL != "" {
					url = v.URL
				}
				if v.SHA256 != "" {
					checksum = v.SHA256
				}
				if v.Version != "" {
					version = v.Version
				}
				return url, checksum, version
			}
		}
	}

	// No matching variation found, use base values
	return url, checksum, version
}

// isValidCaskName validates Homebrew Cask names.
// Cask names follow similar rules to formula names:
// - Lowercase letters, numbers, hyphens, underscores
// - May contain @ for versioned casks (rare)
// - No path separators or shell metacharacters
//
// Security: Prevents command injection and path traversal
func isValidCaskName(name string) bool {
	if name == "" || len(name) > 128 {
		return false
	}

	// Reject dangerous patterns
	if strings.Contains(name, "..") || strings.Contains(name, "/") ||
		strings.Contains(name, "\\") || strings.HasPrefix(name, "-") {
		return false
	}

	// Check allowed characters (same as Homebrew formula)
	for _, c := range name {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') ||
			c == '-' || c == '_' || c == '@' || c == '.') {
			return false
		}
	}

	return true
}
