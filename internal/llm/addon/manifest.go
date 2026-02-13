// Package addon provides download and verification for the tsuku-llm addon binary.
package addon

import (
	_ "embed"
	"encoding/json"
	"fmt"
)

//go:embed manifest.json
var manifestData []byte

// Manifest defines the tsuku-llm addon release information.
// This is embedded in the tsuku binary for supply chain security.
type Manifest struct {
	// Version is the addon release version (e.g., "0.1.0")
	Version string `json:"version"`
	// Platforms maps "GOOS-GOARCH" to platform-specific info
	Platforms map[string]PlatformInfo `json:"platforms"`
}

// PlatformInfo contains download URL and checksum for a platform.
type PlatformInfo struct {
	// URL is the download URL for this platform's binary
	URL string `json:"url"`
	// SHA256 is the hex-encoded SHA256 checksum
	SHA256 string `json:"sha256"`
}

// cachedManifest holds the parsed manifest to avoid repeated parsing.
var cachedManifest *Manifest

// GetManifest returns the embedded addon manifest.
// The manifest is parsed once and cached for subsequent calls.
func GetManifest() (*Manifest, error) {
	if cachedManifest != nil {
		return cachedManifest, nil
	}

	var m Manifest
	if err := json.Unmarshal(manifestData, &m); err != nil {
		return nil, fmt.Errorf("failed to parse addon manifest: %w", err)
	}

	cachedManifest = &m
	return &m, nil
}

// GetPlatformInfo returns platform info for the given platform key.
// Returns an error if the platform is not supported.
func (m *Manifest) GetPlatformInfo(platformKey string) (*PlatformInfo, error) {
	info, ok := m.Platforms[platformKey]
	if !ok {
		return nil, fmt.Errorf("unsupported platform: %s", platformKey)
	}
	return &info, nil
}
