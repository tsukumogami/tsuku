# Static Binary Handling in Tier 2

**Date:** 2026-01-17
**Purpose:** Define how to handle Go/Rust binaries with zero DT_NEEDED entries

## Summary

Static binaries (common in Go/Rust) have zero DT_NEEDED entries. Tier 2 should detect this and report appropriately without requiring recipe metadata.

## Detection Methods

### ELF (Linux)

A binary is statically linked if:
1. No `PT_INTERP` program header (no dynamic linker reference)
2. Zero `DT_NEEDED` entries

```go
func IsStaticallyLinkedELF(path string) (bool, error) {
    f, err := elf.Open(path)
    if err != nil {
        return false, err
    }
    defer f.Close()

    // Check for PT_INTERP segment
    hasInterp := false
    for _, prog := range f.Progs {
        if prog.Type == elf.PT_INTERP {
            hasInterp = true
            break
        }
    }

    // Check DT_NEEDED count
    deps, _ := f.ImportedLibraries()

    // Static if no interpreter AND no dependencies
    return !hasInterp && len(deps) == 0, nil
}
```

### Mach-O (macOS)

A binary is statically linked if:
1. Zero `LC_LOAD_DYLIB` load commands
2. No reference to `dyld`

```go
func IsStaticallyLinkedMachO(path string) (bool, error) {
    f, err := macho.Open(path)
    if err != nil {
        return false, err
    }
    defer f.Close()

    // Check imported libraries
    deps, _ := f.ImportedLibraries()
    return len(deps) == 0, nil
}
```

### Unified Detection

```go
// internal/verify/static.go

func IsStaticallyLinked(path string) (bool, error) {
    // Read first bytes to detect format
    file, err := os.Open(path)
    if err != nil {
        return false, err
    }
    defer file.Close()

    var magic [4]byte
    if _, err := file.Read(magic[:]); err != nil {
        return false, err
    }

    // ELF magic: 0x7F 'E' 'L' 'F'
    if magic[0] == 0x7f && magic[1] == 'E' && magic[2] == 'L' && magic[3] == 'F' {
        return IsStaticallyLinkedELF(path)
    }

    // Mach-O magic: various
    if magic[0] == 0xfe && magic[1] == 0xed && magic[2] == 0xfa {
        return IsStaticallyLinkedMachO(path)
    }
    if magic[0] == 0xcf && magic[1] == 0xfa && magic[2] == 0xed {
        return IsStaticallyLinkedMachO(path)
    }

    return false, fmt.Errorf("unknown binary format")
}
```

## Recipe Patterns

### Go Recipes

From `go_build` action (`internal/actions/go_build.go`):
- Default: `CGO_ENABLED=0` → static binary
- Optional: `cgo_enabled = true` → dynamic binary with libc deps

```toml
# Static (default)
[[steps]]
action = "go_build"
# No cgo_enabled → CGO_ENABLED=0 → static

# Dynamic (CGO enabled)
[[steps]]
action = "go_build"
cgo_enabled = true  # CGO_ENABLED=1 → dynamic
```

### Rust Recipes

From `cargo_install` and `cargo_build` actions:
- Default: static linking for most dependencies
- May have system deps (libc) depending on crate

### Distribution

From burden analysis (tier2-design-tracker.md):
- ~40% of recipes are Go/Rust tools → mostly static
- ~30% pre-built downloads → varies
- ~30% other (source builds, ecosystem) → mostly dynamic

## Recommended UX

### Option A: Explicit Message (RECOMMENDED)

```
$ tsuku verify ripgrep
Verifying ripgrep (version 14.1.0)...

  Tier 1: Header validation
    bin/rg: OK (ELF x86_64)

  Tier 2: Dependency validation
    No dynamic dependencies (statically linked)

ripgrep verified successfully
```

**Why this is best:**
- Transparent: User knows why no deps are shown
- Educational: Explains static linking
- Consistent: Same output format as dynamic binaries
- No false concerns: User won't wonder "why no deps?"

### Option B: Skip Silently

```
$ tsuku verify ripgrep
Verifying ripgrep (version 14.1.0)...

  Tier 1: Header validation
    bin/rg: OK (ELF x86_64)

ripgrep verified successfully
```

**Why not:** Users may wonder if Tier 2 ran at all.

### Option C: Show as Tier 2 Skipped

```
  Tier 2: Skipped (no dynamic dependencies)
```

**Also acceptable** but less informative than Option A.

## CGO Edge Case

Go binaries with `CGO_ENABLED=1` have dynamic dependencies:

```
$ readelf -d /path/to/cgo-binary | grep NEEDED
  NEEDED: libc.so.6
  NEEDED: libpthread.so.0
```

**Handling:** These are system libraries, already covered by system library registry. No special handling needed - they'll be classified as `DepSystem` and skipped.

## Recipe Metadata: NOT Required

**Recommendation:** Do NOT add `static_binary = true` metadata.

**Reasons:**
1. **Detectable:** Binary format reveals static linking reliably
2. **No burden:** Recipe authors don't need to know/specify
3. **Accurate:** Binary is source of truth, not recipe declaration
4. **Consistent:** Tier 1 doesn't need metadata; Tier 2 shouldn't either

## Integration with Tier 2

```go
// In ValidateDependencies or calling code

func validateBinaryDeps(binaryPath string, cfg *Config) []DepResult {
    info, err := ValidateHeader(binaryPath)
    if err != nil {
        return []DepResult{{Status: DepError, Error: err}}
    }

    // Check for static binary
    if len(info.Dependencies) == 0 {
        isStatic, _ := IsStaticallyLinked(binaryPath)
        if isStatic {
            return []DepResult{{
                Name:   binaryPath,
                Status: DepStaticBinary,
                Note:   "No dynamic dependencies (statically linked)",
            }}
        }
        // Unusual: dynamic binary with no deps
        return []DepResult{{
            Name:   binaryPath,
            Status: DepNoDeps,
            Note:   "No dependencies",
        }}
    }

    // Proceed with normal Tier 2 validation
    return validateDependencies(binaryPath, info.Dependencies, cfg)
}
```

## New Status Values

```go
const (
    DepSystem       DepStatus = iota // System library, skip
    DepValid                         // Tsuku-managed, validated
    DepMissing                       // Tsuku-managed, not found
    DepInvalid                       // Tsuku-managed, validation failed
    DepWarning                       // Unknown dependency
    DepStaticBinary                  // Binary is statically linked (NEW)
    DepNoDeps                        // Dynamic binary with no deps (rare)
)
```

## Summary Table

| Question | Answer |
|----------|--------|
| How to detect static? | Check PT_INTERP (ELF) + DT_NEEDED count |
| What to show? | "No dynamic dependencies (statically linked)" |
| Need recipe metadata? | No - infer from binary |
| Handle CGO-enabled Go? | System lib patterns already handle libc deps |
| Where to implement? | `internal/verify/static.go` (new) |
