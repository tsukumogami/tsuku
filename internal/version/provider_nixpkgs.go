package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// NixpkgsProvider resolves versions from nixpkgs channels.
// For MVP, it returns the latest stable nixpkgs channel version.
//
// In Nix, packages are versioned by the nixpkgs channel they come from.
// For example, nixpkgs 24.05 contains a specific version of each package.
// This is different from other package managers where you request a specific
// package version.
type NixpkgsProvider struct {
	resolver *Resolver
}

// NewNixpkgsProvider creates a new nixpkgs version provider
func NewNixpkgsProvider(resolver *Resolver) *NixpkgsProvider {
	return &NixpkgsProvider{resolver: resolver}
}

// SourceDescription returns a human-readable source description
func (p *NixpkgsProvider) SourceDescription() string {
	return "nixpkgs"
}

// ResolveLatest returns the latest stable nixpkgs channel version.
// Queries channels.nixos.org to find the latest release.
func (p *NixpkgsProvider) ResolveLatest(ctx context.Context) (*VersionInfo, error) {
	// Try to fetch the actual latest channel from nixos.org
	version, err := p.fetchLatestChannel(ctx)
	if err != nil {
		// Fallback to hardcoded version if API fails
		// This ensures tsuku still works offline or if the API is down
		version = "24.05"
	}

	return &VersionInfo{
		Version: version,
		Tag:     fmt.Sprintf("nixos-%s", version),
	}, nil
}

// ResolveVersion validates that the requested version looks like a nixpkgs channel.
// Valid formats: "24.05", "23.11", "unstable"
func (p *NixpkgsProvider) ResolveVersion(ctx context.Context, requested string) (*VersionInfo, error) {
	// Validate format
	if !isValidNixpkgsVersion(requested) {
		return nil, fmt.Errorf("invalid nixpkgs version format: %s (expected YY.MM or 'unstable')", requested)
	}

	return &VersionInfo{
		Version: requested,
		Tag:     fmt.Sprintf("nixos-%s", requested),
	}, nil
}

// ListVersions returns available nixpkgs channel versions.
func (p *NixpkgsProvider) ListVersions(ctx context.Context) ([]string, error) {
	// Return known stable channels
	// Could be enhanced to query channels.nixos.org for full list
	return []string{
		"24.05",
		"23.11",
		"23.05",
		"22.11",
		"unstable",
	}, nil
}

// fetchLatestChannel fetches the latest stable channel from channels.nixos.org
func (p *NixpkgsProvider) fetchLatestChannel(ctx context.Context) (string, error) {
	// NixOS publishes channel status at https://channels.nixos.org/
	// We can also check the GitHub API for nixos-* branches
	// For simplicity, check the channels.nixos.org JSON endpoint

	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", "https://channels.nixos.org/nixos-24.05/git-revision", nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// If 24.05 exists, it's the latest stable (as of late 2024)
	// Could be enhanced to dynamically find the latest
	if resp.StatusCode == http.StatusOK {
		return "24.05", nil
	}

	// Try 23.11 as fallback
	req2, _ := http.NewRequestWithContext(ctx, "GET", "https://channels.nixos.org/nixos-23.11/git-revision", nil)
	resp2, err := client.Do(req2)
	if err != nil {
		return "", err
	}
	defer resp2.Body.Close()

	if resp2.StatusCode == http.StatusOK {
		return "23.11", nil
	}

	return "", fmt.Errorf("could not determine latest nixpkgs channel")
}

// isValidNixpkgsVersion validates nixpkgs version format
// Valid: "24.05", "23.11", "unstable"
func isValidNixpkgsVersion(version string) bool {
	if version == "unstable" {
		return true
	}

	// Must be YY.MM format
	if len(version) < 4 || len(version) > 10 {
		return false
	}

	// Allow digits and dots only
	for _, c := range version {
		if !((c >= '0' && c <= '9') || c == '.') {
			return false
		}
	}

	return true
}

// NixpkgsPackageInfo represents package info from NixOS search
type NixpkgsPackageInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Attr    string `json:"attr"`
}

// LookupPackageVersion queries the NixOS package search for a specific package version.
// This is for future enhancement - currently not used.
func (p *NixpkgsProvider) LookupPackageVersion(ctx context.Context, packageAttr string) (string, error) {
	// NixOS has a search API at https://search.nixos.org/
	// This could be used to get actual package versions
	// For now, return empty to indicate we're using channel versioning

	client := &http.Client{Timeout: 10 * time.Second}

	// The NixOS search API endpoint
	// Example: https://search.nixos.org/packages?channel=24.05&query=hello
	url := fmt.Sprintf("https://search.nixos.org/backend/latest-nixos-24.05/_search?q=%s", packageAttr)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("NixOS search API returned %d", resp.StatusCode)
	}

	// Parse response - this is an Elasticsearch response
	var result struct {
		Hits struct {
			Hits []struct {
				Source struct {
					PackageAttr    string `json:"package_attr"`
					PackageVersion string `json:"package_version"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	// Find exact match
	for _, hit := range result.Hits.Hits {
		if hit.Source.PackageAttr == packageAttr {
			return hit.Source.PackageVersion, nil
		}
	}

	// Try prefix match (e.g., "hello" matches "hello")
	for _, hit := range result.Hits.Hits {
		if strings.HasPrefix(hit.Source.PackageAttr, packageAttr) {
			return hit.Source.PackageVersion, nil
		}
	}

	return "", fmt.Errorf("package %s not found in nixpkgs", packageAttr)
}
