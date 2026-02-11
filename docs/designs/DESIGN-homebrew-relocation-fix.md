---
status: Accepted
problem: |
  Homebrew bottles contain hardcoded build paths that must be rewritten during
  installation. The homebrew_relocate action extracts full paths (including suffixes
  like /libexec/bin/tool) but replaces them entirely with the install prefix, losing
  the relative path suffix. This causes 63+ recipes to fail post-install verification.
decision: |
  Modify extractBottlePrefixes() to use marker-based path parsing: identify the
  /.install/<formula>/<version> boundary and extract only the bottle prefix portion.
  In relocatePlaceholders(), replace only the prefix while preserving any suffix.
  For install_mode=directory, exclude the .install subdirectory during copy.
rationale: |
  The marker-based approach is more reliable than regex because bottle paths have
  consistent structure up to the version boundary but arbitrary structure after.
  Fixing extraction at the source is less invasive than rewriting replacement logic.
  The directory exclusion is minimal and avoids broader architectural changes.
---

# Homebrew Bottle Relocation Fix

## Status

Accepted

## Context and Problem Statement

Homebrew bottles contain hardcoded paths from their build environment that must be rewritten during installation. The `homebrew_relocate` action handles this, but a bug causes path suffix loss:

1. `extractBottlePrefixes()` (homebrew_relocate.go:699-742) scans binaries for bottle paths
2. It extracts the **full path** including any suffix, e.g., `/tmp/action-validator-XXX/.install/cocoapods/1.16.2/libexec/bin/pod`
3. `relocatePlaceholders()` replaces these with just the install prefix
4. Result: `/home/user/.tsuku/tools/cocoapods-1.16.2` instead of `/home/user/.tsuku/tools/cocoapods-1.16.2/libexec/bin/pod`

This causes 63+ recipes to fail post-install verification with "binary not found" errors despite successful installation. Related issues:
- #1583: Homebrew bottle path issues (primary bug)
- #879: Homebrew macOS failures (same root cause)
- #1581: install_mode=directory structure issues (secondary bug)

### Scope

**In scope:**
- Fix path suffix preservation in `extractBottlePrefixes()`
- Fix `install_mode=directory` recursive copy issue
- Ensure backward compatibility with working recipes
- Verify fix on both Linux and macOS

**Out of scope:**
- Rewriting the entire relocation system
- Changes to recipe format
- New testing frameworks

## Decision Drivers

- **Correctness**: Path replacement must preserve relative path suffixes
- **Backward compatibility**: Can't break recipes that currently work
- **Minimal invasiveness**: Fix the bug without rewriting the entire relocation system
- **Testability**: Solution must be verifiable via post-install checks
- **Platform coverage**: Must work on both Linux (patchelf) and macOS (install_name_tool)

## Considered Options

### Decision 1: Path Prefix Extraction Strategy

The core bug is that `extractBottlePrefixes()` extracts full paths but `relocatePlaceholders()` replaces them entirely. We need to preserve the suffix after the version boundary.

#### Chosen: Marker-based prefix extraction

Parse paths to find the `/.install/<formula>/<version>` boundary and extract only the bottle prefix portion. The function returns `map[string]string` mapping full paths to their prefix portion, allowing `relocatePlaceholders()` to replace only the prefix.

**Implementation:**
```go
// extractBottlePrefixes now returns map[fullPath]prefix
// e.g., "/tmp/action-validator-XXX/.install/pod/1.16.2/libexec/bin/pod"
//    -> "/tmp/action-validator-XXX/.install/pod/1.16.2"
```

This approach:
- Uses the stable `/.install/` marker to find the boundary
- Parses formula/version from the path structure
- Returns both the full path (for matching) and prefix (for replacement)

#### Alternatives Considered

**Regex-based suffix preservation**: Extract full path, use regex to identify and preserve suffix.
Rejected because it requires capturing groups across variable path depths, making the regex fragile and hard to debug when edge cases arise.

**Path component analysis**: Split paths into components and replace selectively.
Rejected because bottle paths don't follow a consistent component count after the version - some have 1 component, others have 4+.

### Decision 2: install_mode=directory Copy Direction

The `install_mode=directory` in `install_binaries.go` copies from `ctx.WorkDir` to `ctx.InstallDir`, but `ctx.InstallDir` is `workDir/.install` (a subdirectory of workDir). This causes recursive copy issues.

#### Chosen: Exclusion-based copy

Modify `CopyDirectory` (or introduce a variant) that skips the `.install` subdirectory when copying. This prevents the recursive copy while preserving the existing directory structure.

**Implementation:**
```go
// CopyDirectoryExcluding skips paths matching the exclusion pattern
func CopyDirectoryExcluding(src, dst string, exclude string) error {
    // Walk src, skip any path containing exclude
}
```

#### Alternatives Considered

**Change directory structure**: Put installDir outside workDir entirely.
Rejected because it would require modifying executor.go, manager.go, and ~15 test files, and could break external tooling that depends on the current structure.

## Decision Outcome

**Chosen: Marker-based prefix extraction + exclusion-based copy**

### Summary

The fix modifies `extractBottlePrefixes()` to parse bottle paths and identify the version boundary using the `/.install/<formula>/<version>` pattern. Instead of returning just the full paths, it returns a map from full path to prefix, allowing `relocatePlaceholders()` to perform targeted replacement that preserves path suffixes.

The path parsing algorithm:
1. Find `/tmp/action-validator-` marker in content
2. Extract path up to delimiter (whitespace, quote, etc.)
3. Find `/.install/` in the path
4. Parse the formula name and version after `/.install/`
5. Return the prefix portion ending at the version boundary

For `install_mode=directory`, the `installDirectoryWithSymlinks()` function calls `CopyDirectory` with an exclusion pattern to skip the `.install` subdirectory. This is a minimal change that doesn't affect other callers.

### Rationale

The marker-based approach is reliable because Homebrew's bottle extraction always creates the `/.install/<formula>/<version>` structure. This boundary is stable and predictable, unlike the arbitrary structure after the version (which depends on the formula's internal layout).

Returning a map from `extractBottlePrefixes()` is a minimal API change - callers that only need prefixes can use map values, while `relocatePlaceholders()` can use both keys (for matching) and values (for replacement).

The exclusion-based copy is the least invasive fix for the directory issue. It doesn't change the fundamental workDir/installDir relationship, just prevents the obvious recursive copy problem.

## Solution Architecture

### Component 1: Modified extractBottlePrefixes()

**Location:** `internal/actions/homebrew_relocate.go:699-742`

**Current signature:**
```go
func (a *HomebrewRelocateAction) extractBottlePrefixes(content []byte, prefixes map[string]bool)
```

**New signature:**
```go
func (a *HomebrewRelocateAction) extractBottlePrefixes(content []byte, prefixMap map[string]string)
```

The map key is the full path found in the binary, and the value is the prefix portion (ending at version boundary).

**Parsing logic:**
```go
// After finding /tmp/action-validator- and extracting full path:
installIdx := strings.Index(pathStr, "/.install/")
if installIdx == -1 {
    continue
}

// Path after /.install/ is formula/version/...
afterInstall := pathStr[installIdx+len("/.install/"):]
parts := strings.SplitN(afterInstall, "/", 3)  // formula, version, rest
if len(parts) < 2 {
    continue
}

// Prefix is everything up to and including version
prefix := pathStr[:installIdx] + "/.install/" + parts[0] + "/" + parts[1]
prefixMap[pathStr] = prefix
```

### Component 2: Modified relocatePlaceholders()

**Location:** `internal/actions/homebrew_relocate.go:215-218`

**Current code:**
```go
for prefix := range bottlePrefixes {
    newContent = bytes.ReplaceAll(newContent, []byte(prefix), prefixReplacement)
}
```

**New code:**
```go
for fullPath, prefix := range bottlePrefixes {
    // Replace prefix portion only, preserving suffix
    suffix := fullPath[len(prefix):]

    // Security: validate suffix doesn't contain traversal attempts
    if strings.Contains(suffix, "..") {
        // Skip paths with traversal attempts - defense in depth
        continue
    }

    replacement := prefixPath + suffix
    newContent = bytes.ReplaceAll(newContent, []byte(fullPath), []byte(replacement))
}
```

### Component 3: Exclusion in CopyDirectory

**Location:** `internal/actions/install_binaries.go:338`

**Current code:**
```go
if err := CopyDirectory(ctx.WorkDir, ctx.InstallDir); err != nil {
```

**New approach:** Add exclusion support to `CopyDirectory` or create `CopyDirectoryExcluding`:

```go
// Skip .install subdirectory to prevent recursive copy
if err := CopyDirectoryExcluding(ctx.WorkDir, ctx.InstallDir, ".install"); err != nil {
```

### Data Flow

```
Binary file content
       |
       v
extractBottlePrefixes() ─────────────────┐
       |                                  |
       v                                  v
  Full paths found               Prefix portion parsed
  (map keys)                     (map values)
       |                                  |
       └──────────┬───────────────────────┘
                  |
                  v
       relocatePlaceholders()
                  |
                  v
       Replace prefix with install path,
       keeping suffix intact
                  |
                  v
       Updated binary with correct paths
```

## Implementation Approach

### Phase 1: Fix path extraction (fixes #1583, #879)

1. Modify `extractBottlePrefixes()` to return `map[string]string`
2. Implement prefix parsing logic
3. Update `relocatePlaceholders()` to use full path for matching, prefix for replacement
4. Update callers to use new map type
5. Add debug logging for parsed prefixes

### Phase 2: Fix directory copy (fixes #1581)

1. Add `CopyDirectoryExcluding()` function or modify `CopyDirectory()`
2. Update `installDirectoryWithSymlinks()` to use exclusion
3. Add test case for recursive copy scenario

### Phase 3: Validation

1. Run integration tests on affected recipes (make, cmake, ninja, pkg-config)
2. Verify on both Linux and macOS
3. Check that previously-working recipes still work

## Security Considerations

### Download Verification

**Not applicable** - this feature does not download external artifacts. The fix modifies how already-downloaded Homebrew bottles have their paths rewritten. The download and checksum verification happens before `homebrew_relocate` is invoked, and those mechanisms are unchanged.

### Execution Isolation

**Unchanged** - the relocation happens in the same temporary work directory as before (`/tmp/action-validator-*`). The fix:
- Does not require new file system permissions
- Does not change the scope of files accessed (still only the work directory)
- Does not introduce new network access
- Does not change privilege levels

The only change is *how* paths are parsed and replaced, not *where* the action operates.

### Supply Chain Risks

**Unchanged** - we're using the same Homebrew bottles from the same sources (ghcr.io/homebrew). The fix corrects a bug in path handling, not in source selection or verification.

One consideration: if an attacker could craft a bottle with a malicious path that our parser handles differently than intended, they might be able to write to unintended locations. However:
- The replacement path is always under `$TSUKU_HOME/tools/<name>-<version>/`
- The suffix is extracted from the binary content, not user input
- Existing path validation in `install_binaries` rejects `..` traversal attempts

### User Data Exposure

**Not applicable** - this feature does not access or transmit user data. The fix only operates on binary files downloaded as part of installation, rewriting paths embedded in those binaries. No user files are read, and no data is sent externally.

### Mitigations

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Path suffix traversal attack | Explicit `..` validation in suffix before replacement; skip paths with traversal attempts | None - defense in depth alongside checksum verification |
| Malicious path in bottle content | Suffix validated for traversal; bottles are SHA256-checksummed | Attacker would need to compromise Homebrew's signing |
| Incorrect path parsing | Debug logging shows parsed prefixes; post-install verification catches failures | Edge cases in path format could cause unexpected behavior |
| Recursive copy in directory mode | Explicit exclusion of `.install` directory | None - exclusion is explicit and tested |
| Null byte injection | Path delimiter detection includes whitespace and common terminators; replacement happens on byte arrays | Theoretical concern; mitigated by checksum verification |

## Consequences

### Positive

- Unblocks 63+ recipes that currently fail verification
- Closes issues #1583, #879, and #1581
- Minimal code change (< 50 lines modified)
- Backward compatible - working recipes stay working

### Negative

- Adds complexity to prefix extraction logic
- Assumes bottle path format is stable (`/tmp/action-validator-*`)
- Debug output will show more information (prefixes and suffixes)

### Mitigations

- Format assumption is validated by existing bottle builds in CI
- Debug output can be controlled via log level
- Comprehensive test coverage for edge cases
