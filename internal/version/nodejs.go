package version

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ResolveNodeJS resolves the latest LTS version from Node.js dist site
func (r *Resolver) ResolveNodeJS(ctx context.Context) (*VersionInfo, error) {
	// Node.js publishes version info at https://nodejs.org/dist/index.json
	url := "https://nodejs.org/dist/index.json"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		if strings.Contains(err.Error(), "network is unreachable") ||
			strings.Contains(err.Error(), "no such host") ||
			strings.Contains(err.Error(), "dial tcp") {
			return nil, fmt.Errorf("network unavailable: %w", err)
		}
		return nil, fmt.Errorf("failed to fetch Node.js versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Node.js dist site returned status %d", resp.StatusCode)
	}

	var versions []struct {
		Version string      `json:"version"`
		LTS     interface{} `json:"lts"` // Can be string (LTS name) or false
	}

	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Find the latest LTS version
	for _, v := range versions {
		if v.LTS != nil && v.LTS != false {
			// Strip "v" prefix from version
			version := normalizeVersion(v.Version)
			return &VersionInfo{
				Tag:     v.Version, // Keep "v" prefix for URL
				Version: version,   // Without "v" for display
			}, nil
		}
	}

	// Fallback: use the latest version (first in list)
	if len(versions) > 0 {
		version := normalizeVersion(versions[0].Version)
		return &VersionInfo{
			Tag:     versions[0].Version,
			Version: version,
		}, nil
	}

	return nil, fmt.Errorf("no Node.js versions found")
}
