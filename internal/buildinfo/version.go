// Package buildinfo provides version information derived from build metadata.
package buildinfo

import (
	"fmt"
	"runtime/debug"
)

// These variables are set at build time via ldflags by goreleaser.
// For dev builds, they remain empty and we fall back to VCS info.
var (
	version string
	commit  string
)

// Version returns the version string for the current build.
//
// For releases built with goreleaser, returns the injected version (e.g., "v0.1.0").
// For development builds, returns a pseudo-version with commit info:
//   - "dev-<hash>" for clean builds
//   - "dev-<hash>-dirty" for builds with uncommitted changes
//   - "dev" if no VCS info is available
//   - "unknown" if build info cannot be read
func Version() string {
	// Use injected version if available (goreleaser builds)
	if version != "" {
		return version
	}

	// Fall back to VCS info for dev builds
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "unknown"
	}

	// Check if this is a tagged release via go install
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}

	return devVersion(info)
}

// devVersion constructs a development version string from build info.
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

	// Use injected commit if available, otherwise use VCS revision
	if revision == "" && commit != "" {
		revision = commit
	}

	if revision == "" {
		return "dev"
	}

	// Truncate revision to 12 characters
	if len(revision) > 12 {
		revision = revision[:12]
	}

	ver := fmt.Sprintf("dev-%s", revision)
	if modified {
		ver += "-dirty"
	}

	return ver
}
