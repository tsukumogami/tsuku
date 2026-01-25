---
summary:
  constraints:
    - libc filter only valid when os is omitted or includes "linux"
    - libc values must be "glibc" or "musl" only
    - Error if libc specified with os = ["darwin"]
    - Empty Libc array means "all libc types" (no filtering)
  integration_points:
    - internal/recipe/types.go - WhenClause struct
    - Matchable interface (already updated in #1109 with Libc() method)
    - WhenClause.Matches() logic
    - WhenClause.IsEmpty() method
  risks:
    - Must not break existing recipes without libc filter
    - Validation must not reject darwin-only recipes
    - TOML parsing must handle array syntax properly
  approach_notes: |
    Follow existing WhenClause patterns (OS, Arch, LinuxFamily).
    Add Libc []string field, update Matches() to check on Linux only,
    add validation rules per design doc. Most work is in types.go.
---

# Implementation Context: Issue #1110

**Source**: docs/designs/DESIGN-platform-compatibility-verification.md

## Key Design Points

This issue implements Component 2 (Libc Recipe Filter) from the design.

### WhenClause Changes (from design)

```go
type WhenClause struct {
    Platform       []string `toml:"platform"`
    OS             []string `toml:"os"`
    Arch           string   `toml:"arch"`
    LinuxFamily    string   `toml:"linux_family"`
    PackageManager string   `toml:"package_manager"`
    Libc           []string `toml:"libc"`  // New: ["glibc"], ["musl"], or both
}

func (w *WhenClause) Matches(target Matchable) bool {
    // ... existing checks ...

    // Check libc filter (only applicable on Linux)
    if len(w.Libc) > 0 && target.OS() == "linux" {
        if !contains(w.Libc, target.Libc()) {
            return false
        }
    }
    return true
}
```

### Validation Rules (from design)

```go
func (w *WhenClause) Validate() error {
    // libc filter only valid on Linux
    if len(w.Libc) > 0 {
        if len(w.OS) > 0 && !slices.Contains(w.OS, "linux") {
            return fmt.Errorf("libc filter only valid when os includes 'linux'")
        }
        for _, libc := range w.Libc {
            if libc != "glibc" && libc != "musl" {
                return fmt.Errorf("libc must be 'glibc' or 'musl', got %q", libc)
            }
        }
    }
    return nil
}
```

### Recipe Example (from design)

```toml
[[steps]]
action = "homebrew"
formula = "curl"
when = { os = ["linux"], libc = ["glibc"] }

[[steps]]
action = "system_dependency"
name = "curl"
when = { os = ["linux"], libc = ["musl"] }
```

## Downstream Impact

This unblocks:
- #1111 (step-level deps) - depends on libc filter
- #1113 (supported_libc constraint) - depends on libc filter
- #1115 (coverage validation) - depends on libc filter
