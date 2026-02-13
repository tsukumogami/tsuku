package addon

import (
	"fmt"
	"runtime"
)

// PlatformKey returns the platform identifier for the current system.
// Format: "GOOS-GOARCH" (e.g., "darwin-arm64", "linux-amd64")
func PlatformKey() string {
	return runtime.GOOS + "-" + runtime.GOARCH
}

// BinaryName returns the addon binary name for the current platform.
// On Windows, this includes the .exe extension.
func BinaryName() string {
	if runtime.GOOS == "windows" {
		return "tsuku-llm.exe"
	}
	return "tsuku-llm"
}

// GetCurrentPlatformInfo returns the PlatformInfo for the current system.
// Returns an error if the current platform is not supported.
func GetCurrentPlatformInfo() (*PlatformInfo, error) {
	manifest, err := GetManifest()
	if err != nil {
		return nil, fmt.Errorf("failed to load manifest: %w", err)
	}

	return manifest.GetPlatformInfo(PlatformKey())
}
