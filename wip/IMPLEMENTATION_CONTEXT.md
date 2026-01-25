---
summary:
  constraints:
    - DetectLibc() must check /lib/ld-musl-*.so.1 pattern for musl detection
    - Returns "glibc" as default (not empty string)
    - libc field only populated on Linux; empty on darwin
    - Must integrate with existing Matchable interface
  integration_points:
    - internal/platform/target.go - Add libc field to Target struct
    - internal/platform/family.go - DetectTarget() must call DetectLibc()
    - Matchable interface needs Libc() method for recipe filtering (issue #1110)
  risks:
    - Glob pattern might not work on all musl distros (but 95%+ are Alpine)
    - Test fixtures needed for musl detection in CI (runs on glibc)
  approach_notes: |
    Create internal/platform/libc.go with DetectLibc() function.
    Detection uses filepath.Glob("/lib/ld-musl-*.so.1") pattern.
    Add libc field to Target, update NewTarget/DetectTarget.
    Add Libc() to Matchable interface for downstream issues.
    Use testdata fixtures similar to existing os-release tests.
---

# Implementation Context: Issue #1109

**Source**: docs/designs/DESIGN-platform-compatibility-verification.md

## Key Design Details

This issue is the foundation for the hybrid libc approach. It provides:
1. Runtime libc detection (glibc vs musl)
2. Integration with Target struct
3. Libc() method on Matchable interface

The detection logic from the design:

```go
func DetectLibc() string {
    // Check for musl dynamic linker
    matches, _ := filepath.Glob("/lib/ld-musl-*.so.1")
    if len(matches) > 0 {
        return "musl"
    }
    return "glibc"
}
```

Target struct changes:

```go
type Target struct {
    os     string
    arch   string
    family string
    libc   string  // New field
}

func (t *Target) Libc() string {
    return t.libc
}
```

Matchable interface addition:

```go
type Matchable interface {
    OS() string
    Arch() string
    LinuxFamily() string
    Libc() string  // New method
}
```

## Downstream Dependencies

This issue unblocks:
- #1110 (libc filter) - needs Libc() method on Matchable
- #1112 (system_dependency action) - needs DetectLibc() function
