# Implementation Plan: Derive Version from Go Build Info

**Status: IMPLEMENTED**

## Issue #2 Summary

Replace the hardcoded version constant with dynamic version detection using Go's `runtime/debug.ReadBuildInfo()`.

**Current state:**
```go
var Version = "0.3.0"
```

**Expected behavior:**
- Tagged releases should report exact version tag (e.g., `v0.1.0`)
- Development builds should report pseudo-version with commit hash (e.g., `v0.0.0-20250115120000-abc123def456`)
- No manual version bumps required

## Technical Background

Go's `runtime/debug.ReadBuildInfo()` returns build information embedded by the Go toolchain:

1. **Module version** (`info.Main.Version`): For `go install` from tagged releases, this contains the version tag. For development builds, it contains `(devel)`.

2. **VCS information** (in `info.Settings`): Contains `vcs.revision` (commit hash), `vcs.time` (commit timestamp), and `vcs.modified` (dirty flag).

## Implementation Plan

### Step 1: Create a new `internal/buildinfo` package

Create a new package to encapsulate version detection logic:

**File: `internal/buildinfo/version.go`**

```go
package buildinfo

import (
    "runtime/debug"
    "fmt"
    "strings"
)

// Version returns the version string for the current build.
// For tagged releases (via go install), returns the tag (e.g., "v0.1.0").
// For development builds, returns a pseudo-version with commit info.
func Version() string {
    info, ok := debug.ReadBuildInfo()
    if !ok {
        return "unknown"
    }

    // Check if this is a tagged release
    if info.Main.Version != "" && info.Main.Version != "(devel)" {
        return info.Main.Version
    }

    // Development build - construct pseudo-version from VCS info
    return devVersion(info)
}

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

    // Truncate revision to 12 characters (standard short hash)
    if len(revision) > 12 {
        revision = revision[:12]
    }

    version := fmt.Sprintf("dev-%s", revision)
    if modified {
        version += "-dirty"
    }

    return version
}
```

### Step 2: Update `cmd/tsuku/main.go`

Replace the hardcoded version with the new buildinfo package:

**Changes:**
1. Remove the `Version` variable declaration
2. Import the new `buildinfo` package
3. Update the `rootCmd` to use `buildinfo.Version()`

```go
import (
    // ... existing imports
    "github.com/tsuku-dev/tsuku/internal/buildinfo"
)

var rootCmd = &cobra.Command{
    Use:     "tsuku",
    Short:   "A modern, universal package manager for development tools",
    // ...
    Version: buildinfo.Version(),
}
```

### Step 3: Add unit tests

**File: `internal/buildinfo/version_test.go`**

```go
package buildinfo

import (
    "runtime/debug"
    "testing"
)

func TestDevVersion(t *testing.T) {
    tests := []struct {
        name     string
        info     *debug.BuildInfo
        expected string
    }{
        {
            name:     "no vcs info",
            info:     &debug.BuildInfo{},
            expected: "dev",
        },
        {
            name: "with revision",
            info: &debug.BuildInfo{
                Settings: []debug.BuildSetting{
                    {Key: "vcs.revision", Value: "abc123def456789"},
                },
            },
            expected: "dev-abc123def456",
        },
        {
            name: "with revision and dirty",
            info: &debug.BuildInfo{
                Settings: []debug.BuildSetting{
                    {Key: "vcs.revision", Value: "abc123def456789"},
                    {Key: "vcs.modified", Value: "true"},
                },
            },
            expected: "dev-abc123def456-dirty",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := devVersion(tt.info)
            if got != tt.expected {
                t.Errorf("devVersion() = %q, want %q", got, tt.expected)
            }
        })
    }
}
```

### Step 4: Update documentation

Update `cmd/tsuku/main.go` comments to reflect the new version behavior.

## File Changes Summary

| File | Action | Description |
|------|--------|-------------|
| `internal/buildinfo/version.go` | Create | New package for version detection |
| `internal/buildinfo/version_test.go` | Create | Unit tests for version detection |
| `cmd/tsuku/main.go` | Modify | Remove hardcoded version, use buildinfo package |

## Testing Strategy

1. **Unit tests**: Test the `devVersion()` helper function with various inputs
2. **Manual testing**:
   - Build locally and verify `tsuku --version` shows `dev-<hash>` or `dev-<hash>-dirty`
   - Install via `go install` from a tag and verify version matches tag

## Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| `ReadBuildInfo()` returns `nil` in some edge cases | Return "unknown" as fallback |
| VCS settings not available in all build scenarios | Gracefully fall back to "dev" |
| Breaking change for scripts that parse version output | Version format remains semver-compatible |

## Open Questions

1. Should we include the commit timestamp in dev versions? The issue mentions it, but `dev-<hash>` is more concise and commonly used.
2. Should `--version` output be more verbose (showing commit, dirty status separately)?

## Acceptance Criteria

- [x] `go build && ./tsuku --version` shows version with commit info
- [ ] `go install github.com/tsuku-dev/tsuku/cmd/tsuku@v0.x.0` shows `v0.x.0` (verified after tagging)
- [x] No manual version updates required in code
- [x] All existing tests pass
- [x] New unit tests for version detection

## Implementation Notes

The implementation revealed that Go's module system already provides pseudo-versions in a standard format when building from a git repository. The `info.Main.Version` field returns:
- For tagged releases: the exact tag (e.g., `v0.1.0`)
- For development builds: a pseudo-version like `v0.0.0-20251127214841-62fb69e4edb6+dirty`

This means the `devVersion()` fallback function is only used when VCS info is unavailable (e.g., building outside a git repo or from a vendored dependency). The pseudo-version format includes timestamp and commit hash, which exceeds the original requirements.

## Review Feedback Incorporated

Three independent agents reviewed this plan. Key feedback incorporated:
1. Set `rootCmd.Version` in `init()` rather than inline (explicit initialization)
2. Added integration test for `Version()` function
3. Added comprehensive test cases for edge cases (empty revision, short hash, etc.)
