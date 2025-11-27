// Package buildinfo provides version information derived from Go build metadata.
package buildinfo

import (
	"fmt"
	"runtime/debug"
)

// Version returns the version string for the current build.
//
// For tagged releases (via go install), returns the tag (e.g., "v0.1.0").
// For development builds, returns a pseudo-version with commit info:
//   - "dev-<hash>" for clean builds (e.g., "dev-abc123def456")
//   - "dev-<hash>-dirty" for builds with uncommitted changes
//   - "dev" if no VCS info is available
//   - "unknown" if build info cannot be read (rare)
func Version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	// Check if this is a tagged release (go install from a tag)
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	// Development build - construct pseudo-version from VCS info
	return devVersion(info)
}

// devVersion constructs a development version string from build info.
// Returns "dev-<hash>[-dirty]" if VCS info is available, otherwise "dev".
func devVersion(info *debug.BuildInfo) string {
	var revision string
	var modified bool

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modified = setting.Value == "true"
		}
	}

	if revision == "" {
		return "dev"
	}

	// Truncate revision to 12 characters (standard Git short hash length)
	if len(revision) > 12 {
		revision = revision[:12]
	}

	version := fmt.Sprintf("dev-%s", revision)
	if modified {
		version += "-dirty"
	}

	return version
}
