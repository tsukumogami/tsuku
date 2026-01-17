# Soname Auto-Discovery for Tier 2 Dependency Validation

**Date:** 2026-01-17
**Purpose:** Design how to automatically discover what sonames a library provides

## Problem

For Tier 2 dependency validation, we need to map sonames back to recipes:
- Binary says: `DT_NEEDED: libssl.so.3`
- We need to know: `libssl.so.3` is provided by recipe `openssl`

Instead of requiring manual `provides` declarations, we can auto-discover this at install time.

## Current State

### What Already Exists

| Component | Location | What It Does |
|-----------|----------|--------------|
| Header parsing | `internal/verify/header.go` | Extracts DT_NEEDED (dependencies), NOT DT_SONAME |
| Library state | `internal/install/state.go` | `LibraryVersionState` with `UsedBy`, `Checksums` |
| Checksum compute | `internal/install/library.go` | `ComputeLibraryChecksums()` - pattern to follow |
| State helpers | `internal/install/state_lib.go` | Atomic state updates |

### What's Missing

- **DT_SONAME extraction** (what library provides, not what it needs)
- **Storage in state.json**
- **Reverse lookup index** for Tier 2 validation

## Recommended Approach

### 1. When to Discover: At Library Install Time

Call `ExtractSonames(libDir)` during `InstallLibrary()` after checksum computation.

**Why install time:**
- Binaries are stable (just installed)
- Matches existing pattern (`ComputeLibraryChecksums()`)
- State is complete before user interacts with library
- Minimal overhead (~50ms per library)

### 2. How to Extract Sonames

**For ELF (Linux):**
```go
func ExtractELFSoname(path string) (string, error) {
    f, err := elf.Open(path)
    if err != nil {
        return "", err
    }
    defer f.Close()

    sonames, err := f.DynString(elf.DT_SONAME)
    if err != nil || len(sonames) == 0 {
        return "", nil // No soname set
    }
    return sonames[0], nil
}
```

**For Mach-O (macOS):**
```go
func ExtractMachOInstallName(path string) (string, error) {
    f, err := macho.Open(path)
    if err != nil {
        return "", err
    }
    defer f.Close()

    // LC_ID_DYLIB contains the install name
    for _, load := range f.Loads {
        if dylib, ok := load.(*macho.Dylib); ok {
            if dylib.Cmd == macho.LC_ID_DYLIB {
                return filepath.Base(dylib.Name), nil
            }
        }
    }
    return "", nil
}
```

**Full extraction function:**
```go
func ExtractSonames(libDir string) ([]string, error) {
    var sonames []string
    seen := make(map[string]bool)

    err := filepath.Walk(libDir, func(path string, info os.FileInfo, err error) error {
        if err != nil || info.IsDir() {
            return err
        }

        // Only process .so and .dylib files
        if !strings.HasSuffix(path, ".so") &&
           !strings.Contains(path, ".so.") &&
           !strings.HasSuffix(path, ".dylib") {
            return nil
        }

        // Resolve symlinks to avoid duplicates
        realPath, err := filepath.EvalSymlinks(path)
        if err != nil || realPath != path {
            return nil // Skip symlinks
        }

        // Extract soname based on platform
        var soname string
        if strings.HasSuffix(path, ".dylib") {
            soname, _ = ExtractMachOInstallName(path)
        } else {
            soname, _ = ExtractELFSoname(path)
        }

        if soname != "" && !seen[soname] {
            seen[soname] = true
            sonames = append(sonames, soname)
        }
        return nil
    })

    return sonames, err
}
```

### 3. Where to Store

**Add field to `LibraryVersionState`:**

```go
// internal/install/state.go
type LibraryVersionState struct {
    UsedBy    []string          `json:"used_by"`
    Checksums map[string]string `json:"checksums,omitempty"`
    Sonames   []string          `json:"sonames,omitempty"` // NEW
}
```

**Example state.json:**
```json
{
  "libs": {
    "openssl": {
      "3.2.1": {
        "used_by": ["ruby-3.3.0"],
        "checksums": {
          "lib/libssl.so.3": "abc123...",
          "lib/libcrypto.so.3": "def456..."
        },
        "sonames": ["libssl.so.3", "libcrypto.so.3"]
      }
    },
    "zlib": {
      "1.3.1": {
        "used_by": ["ruby-3.3.0", "curl-8.5.0"],
        "checksums": {"lib/libz.so.1": "ghi789..."},
        "sonames": ["libz.so.1"]
      }
    }
  }
}
```

### 4. Building the Reverse Index

For Tier 2 validation, build an in-memory index at startup:

```go
// internal/verify/soname_index.go
type SonameIndex struct {
    // soname → recipe name
    SonameToRecipe map[string]string
    // soname → version
    SonameToVersion map[string]string
}

func BuildSonameIndex(state *State) *SonameIndex {
    index := &SonameIndex{
        SonameToRecipe:  make(map[string]string),
        SonameToVersion: make(map[string]string),
    }

    for recipeName, versions := range state.Libs {
        for version, libState := range versions {
            for _, soname := range libState.Sonames {
                index.SonameToRecipe[soname] = recipeName
                index.SonameToVersion[soname] = version
            }
        }
    }

    return index
}
```

### 5. Integration with Tier 2 Validation

```go
func ValidateDependencies(binaryPath string, index *SonameIndex) []DepResult {
    info, _ := ValidateHeader(binaryPath)
    var results []DepResult

    for _, dep := range info.Dependencies {
        if isSystemLibrary(dep) {
            results = append(results, DepResult{
                Name:   dep,
                Status: DepSystem,
            })
            continue
        }

        if recipe, found := index.SonameToRecipe[dep]; found {
            results = append(results, DepResult{
                Name:         dep,
                Status:       DepValid,
                RecipeName:   recipe,
                ResolvedPath: findLibraryPath(recipe, dep),
            })
        } else {
            results = append(results, DepResult{
                Name:   dep,
                Status: DepWarning, // Unknown soname
            })
        }
    }

    return results
}
```

## Code Changes Summary

| File | Change | Effort |
|------|--------|--------|
| `internal/install/state.go` | Add `Sonames []string` to `LibraryVersionState` | Low |
| `internal/install/state_lib.go` | Add `SetLibrarySonames()` helper | Low |
| `internal/install/library.go` | Add `ExtractSonames()`, call in `InstallLibrary()` | Medium |
| `internal/verify/soname.go` | **NEW**: Soname extraction for ELF/Mach-O | Medium |
| `internal/verify/soname_index.go` | **NEW**: `BuildSonameIndex()` | Low |
| `internal/verify/deps.go` | Use index in `ValidateDependencies()` | Medium |

## Advantages

| Benefit | Description |
|---------|-------------|
| **Accurate** | Discovered from actual binaries, not manual declarations |
| **No burden** | Recipe authors don't need to maintain `provides` list |
| **Backward compatible** | Optional field, old state files work |
| **Performant** | Simple array, O(1) lookups via index |
| **Follows patterns** | Same approach as checksum computation |

## Edge Cases

### Multiple sonames from one library
Some libraries have multiple sonames (e.g., versioned + unversioned). Store all:
```json
"sonames": ["libfoo.so.1", "libfoo.so.1.2.3"]
```

### Library with no DT_SONAME
Fallback to filename-based soname (the `.so` filename itself).

### Cross-platform differences
- Linux: DT_SONAME in ELF
- macOS: LC_ID_DYLIB install name

Both stored in same `sonames` field; index lookup works regardless.

## Out of Scope

The optional `provides` field for recipe discoverability is tracked separately in issue #969. This design focuses solely on auto-discovery for Tier 2 runtime validation.
