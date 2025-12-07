package version

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/Masterminds/semver/v3"
)

const (
	// Max response size: 1MB (Homebrew formula responses are typically small)
	maxHomebrewResponseSize = 1 * 1024 * 1024
)

// Homebrew API response structure for formula info
// API: https://formulae.brew.sh/api/formula/{formula}.json
type homebrewFormulaInfo struct {
	Name     string `json:"name"`
	FullName string `json:"full_name"`
	Versions struct {
		Stable string `json:"stable"`
		Head   string `json:"head,omitempty"`
		Bottle bool   `json:"bottle"`
	} `json:"versions"`
	Revision      int      `json:"revision"`
	VersionScheme int      `json:"version_scheme"`
	Deprecated    bool     `json:"deprecated"`
	Disabled      bool     `json:"disabled"`
	Versioned     []string `json:"versioned_formulae"` // List of versioned formulae (e.g., "openssl@1.1")
}

// ResolveHomebrew fetches the latest stable version from Homebrew API
//
// API: https://formulae.brew.sh/api/formula/{formula}.json
// Returns: Latest stable version from versions.stable
func (r *Resolver) ResolveHomebrew(ctx context.Context, formula string) (*VersionInfo, error) {
	// Validate formula name (prevent injection)
	if !isValidHomebrewFormula(formula) {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "homebrew",
			Message: fmt.Sprintf("invalid Homebrew formula name: %s", formula),
		}
	}

	// SECURITY: Use url.Parse for proper URL construction
	registryURL := r.homebrewRegistryURL
	if registryURL == "" {
		registryURL = "https://formulae.brew.sh"
	}

	baseURL, err := url.Parse(registryURL)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "homebrew",
			Message: "failed to parse base URL",
			Err:     err,
		}
	}
	apiURL := baseURL.JoinPath("api", "formula", formula+".json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "homebrew",
			Message: "failed to create request",
			Err:     err,
		}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "homebrew", "failed to fetch formula info")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "homebrew",
			Message: fmt.Sprintf("formula %s not found on Homebrew", formula),
		}
	}

	if resp.StatusCode != 200 {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "homebrew",
			Message: fmt.Sprintf("unexpected status code: %d", resp.StatusCode),
		}
	}

	// SECURITY: Limit response size to prevent memory exhaustion
	limitedReader := io.LimitReader(resp.Body, maxHomebrewResponseSize)

	var formulaInfo homebrewFormulaInfo
	if err := json.NewDecoder(limitedReader).Decode(&formulaInfo); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "homebrew",
			Message: "failed to parse Homebrew response",
			Err:     err,
		}
	}

	version := formulaInfo.Versions.Stable
	if version == "" {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "homebrew",
			Message: "no stable version found in Homebrew response",
		}
	}

	// Check if formula is disabled
	if formulaInfo.Disabled {
		return nil, &ResolverError{
			Type:    ErrTypeNotFound,
			Source:  "homebrew",
			Message: fmt.Sprintf("formula %s is disabled", formula),
		}
	}

	return &VersionInfo{
		Tag:     version,
		Version: version,
	}, nil
}

// ListHomebrewVersions fetches available versions for a Homebrew formula.
// Note: Homebrew primarily provides the latest stable version. Versioned formulae
// (e.g., openssl@1.1) are listed via the versioned_formulae field, but historical
// versions of a single formula are not exposed via the API.
//
// Returns: List of available versions (stable version + versioned formula versions)
func (r *Resolver) ListHomebrewVersions(ctx context.Context, formula string) ([]string, error) {
	// Validate formula name
	if !isValidHomebrewFormula(formula) {
		return nil, &ResolverError{
			Type:    ErrTypeValidation,
			Source:  "homebrew",
			Message: fmt.Sprintf("invalid Homebrew formula name: %s", formula),
		}
	}

	// SECURITY: Use url.Parse for proper URL construction
	registryURL := r.homebrewRegistryURL
	if registryURL == "" {
		registryURL = "https://formulae.brew.sh"
	}

	baseURL, err := url.Parse(registryURL)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "homebrew",
			Message: "failed to parse base URL",
			Err:     err,
		}
	}
	apiURL := baseURL.JoinPath("api", "formula", formula+".json")

	req, err := http.NewRequestWithContext(ctx, "GET", apiURL.String(), nil)
	if err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "homebrew",
			Message: "failed to create request",
			Err:     err,
		}
	}

	req.Header.Set("Accept", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, WrapNetworkError(err, "homebrew", "failed to fetch formula info")
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, &ResolverError{
			Type:    ErrTypeNetwork,
			Source:  "homebrew",
			Message: fmt.Sprintf("HTTP %d", resp.StatusCode),
		}
	}

	// SECURITY: Limit response size
	limitedReader := io.LimitReader(resp.Body, maxHomebrewResponseSize)

	var formulaInfo homebrewFormulaInfo
	if err := json.NewDecoder(limitedReader).Decode(&formulaInfo); err != nil {
		return nil, &ResolverError{
			Type:    ErrTypeParsing,
			Source:  "homebrew",
			Message: "failed to parse Homebrew response",
			Err:     err,
		}
	}

	// Collect versions: stable version + versions from versioned formulae
	versions := []string{}
	if formulaInfo.Versions.Stable != "" {
		versions = append(versions, formulaInfo.Versions.Stable)
	}

	// Extract versions from versioned formulae (e.g., "openssl@1.1" -> "1.1")
	for _, vf := range formulaInfo.Versioned {
		if idx := strings.LastIndex(vf, "@"); idx != -1 {
			v := vf[idx+1:]
			if v != "" {
				versions = append(versions, v)
			}
		}
	}

	// Sort by semver (newest first)
	sort.Slice(versions, func(i, j int) bool {
		v1, err1 := semver.NewVersion(versions[i])
		v2, err2 := semver.NewVersion(versions[j])

		if err1 == nil && err2 == nil {
			return v2.LessThan(v1)
		}
		return versions[i] > versions[j]
	})

	return versions, nil
}

// isValidHomebrewFormula validates Homebrew formula names.
//
// Homebrew formula names:
// - Lowercase letters, numbers, hyphens, underscores
// - May contain @ for versioned formulae (e.g., openssl@1.1)
// - No path separators or shell metacharacters
//
// Security: Prevents command injection and path traversal
func isValidHomebrewFormula(name string) bool {
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
