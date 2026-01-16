# DESIGN: Library Verification for tsuku verify Command

**Status:** Proposed

## Context and Problem Statement

Tsuku now supports library recipes (`type = "library"`) for installing shared libraries that tools depend on at runtime. For example, `gcc-libs` provides `libstdc++` and `libgcc_s` for binaries compiled with GCC. Libraries are installed to `$TSUKU_HOME/libs/{name}-{version}/` and are linked at runtime by dependent tools.

However, the `tsuku verify` command does not support libraries. When users run `tsuku verify gcc-libs`, the command fails with "Recipe for 'gcc-libs' does not define verification" because:

1. Libraries have no executables to run for verification
2. Library recipes have no `[verify]` section (the validator explicitly skips this check for libraries)
3. The verify command's design assumes all tools produce an executable output

This creates an inconsistency in user experience: users can install libraries with `tsuku install gcc-libs`, but cannot verify them. This matters because:

- Users expect verification to work uniformly across installed packages
- Libraries are critical dependencies; a corrupted `libstdc++.so` could cause subtle runtime failures
- The existing binary integrity verification (checksum comparison) would be valuable for libraries

### Scope

**In scope:**
- Verification mechanism for library recipes
- Library-specific output in `tsuku verify` command
- File existence and integrity checks for installed libraries
- Recipe schema changes for library verification (if needed)

**Out of scope:**
- Runtime verification (checking if libraries actually load)
- Symbol table validation (checking exported symbols)
- ABI compatibility checking (verifying library versions match expectations)
- Transitive dependency verification (`tsuku verify nodejs` does not verify `gcc-libs`)
- ELF/Mach-O header verification (adds platform-specific complexity)

## Decision Drivers

- **Consistency**: Verification should work for all installable package types
- **Simplicity**: Library verification should not require complex runtime checks
- **Existing patterns**: Reuse binary integrity verification infrastructure where possible
- **Recipe maintainability**: Minimize required additions to library recipes
- **User clarity**: Output should clearly communicate what was verified

## Implementation Context

### Existing Patterns

**Binary integrity verification (verify.go:17-51):**
- Computes SHA256 checksums of installed binaries
- Compares against checksums stored at installation time in `state.json`
- Reports mismatches with file paths and hash prefixes

**Library installation (library.go:23-51):**
- Copies library files from work directory to `$TSUKU_HOME/libs/{name}-{version}/`
- Tracks `used_by` relationships in state
- Does not currently store checksums

**Install libraries action (install_libraries.go):**
- Glob patterns like `["lib/*.so*", "lib/*.dylib"]` specify which files to install
- Preserves symlinks (critical for library versioning like `libfoo.so.2 -> libfoo.so.2.0.9`)

**Recipe validation (validator.go):**
- Libraries skip `[verify]` section validation
- Libraries skip visibility requirements

### Conventions to Follow

- Verification should exit with `ExitVerifyFailed` (code 4) on failure
- Verification output uses consistent formatting (`printInfo`, `printInfof`)
- State tracking follows existing patterns in `state.json`

### Anti-patterns to Avoid

- Runtime dependency on external tools (like `ldd` or `nm`)
- Platform-specific verification logic that would be hard to test
- Overly complex verification that obscures simple failures

### Research Summary

**Industry patterns from package managers:**

| System | Verification Model | Key Feature |
|--------|-------------------|-------------|
| RPM | `rpm -V` compares files against stored MD5/SHA256 checksums | Checks 9+ attributes (size, perms, checksum, ownership) |
| dpkg/apt | `dpkg -V` or `debsums` compares against package manifest | Config files handled specially |
| Pacman | `pacman -Qk` verifies presence and integrity | Database contains MD5 hashes |
| Nix | `nix store verify` validates NAR hash | Content-addressed paths |
| Homebrew | Functional verification via test blocks | Compiles/runs test code against library |

**Common patterns:**
1. **Hash-based integrity**: All systems store checksums at install time and compare during verification
2. **Manifest model**: Package maintains list of expected files with attributes
3. **Graceful degradation**: Handle pre-existing packages without stored checksums
4. **Attribute checking**: Beyond content - permissions, ownership, timestamps (RPM)

**Applicable to tsuku:**
- Extend existing `BinaryChecksums` pattern to library files
- Store checksums in `LibraryVersionState` in state.json
- Follow graceful degradation pattern for pre-existing libraries

## Considered Options

### Option 1: File Existence Verification

Add a `[verify]` section to library recipes specifying expected files. The verify command checks that these files exist in the library directory.

Recipe changes:
```toml
[verify]
files = ["lib/libstdc++.so.6", "lib/libgcc_s.so.1"]
```

Verify command changes:
- Detect library type from recipe metadata
- For libraries, check file existence instead of running a command
- Report missing files

**Pros:**
- Simple to implement and understand
- Explicit about what files a library should contain
- No runtime dependencies

**Cons:**
- Doesn't detect corruption (file exists but is truncated/corrupted)
- Requires recipe changes for all library recipes
- Symlink chains could be broken but files "exist"

### Option 2: Checksum Verification (Extend Binary Integrity)

Extend the existing binary integrity system to libraries. Store checksums of library files at installation time and verify them.

Implementation:
- Modify `install_libraries` action to compute and store checksums
- Modify library installation to save checksums to state.json
- Verify command checks library checksums

**Pros:**
- Detects corruption, not just missing files
- Reuses existing checksum infrastructure
- No recipe changes needed - verification is automatic

**Cons:**
- Requires state.json schema changes for library checksums
- Symlinks need special handling (verify target, not link)
- Pre-existing libraries won't have stored checksums

### Option 3: Pattern-Based Verification

Library recipes already specify patterns in `install_libraries` action. Use these same patterns for verification - check that the installed directory matches the expected pattern.

Implementation:
- Parse `install_libraries` patterns from recipe
- Verify at least one file matches each pattern
- Optionally verify checksum of matched files

**Pros:**
- No new recipe sections needed
- Patterns already define expected library structure
- Works with existing recipes immediately

**Cons:**
- Patterns might be too broad (*.so* matches many files)
- Doesn't verify specific required files
- Pattern matching alone doesn't ensure completeness

### Option 4: Hybrid Approach (Checksums + Optional Files)

Store checksums automatically during installation. Allow optional `[verify]` section for libraries to specify required files for additional validation.

Implementation:
- Always compute and store checksums at install time
- Verify checksums by default
- If `[verify]` section exists, also check specified files

Recipe (optional):
```toml
[verify]
required_files = ["lib/libstdc++.so.6"]  # Optional: specific files that must exist
```

**Pros:**
- Automatic checksum verification works out of the box
- Optional explicit verification for important files
- Graceful handling of symlink chains (verify the final target)

**Cons:**
- More complex than single approach
- Two sources of truth (checksums + explicit files)

### Evaluation Against Decision Drivers

| Option | Consistency | Simplicity | Reuse Patterns | Recipe Maintenance | User Clarity |
|--------|-------------|------------|----------------|--------------------|--------------|
| Option 1: File Existence | Good | Good | Poor | Poor (requires changes) | Good |
| Option 2: Checksums | Good | Good | Good | Good (automatic) | Good |
| Option 3: Patterns | Fair | Good | Good | Good (automatic) | Fair |
| Option 4: Hybrid | Good | Fair | Good | Good | Good |

## Decision Outcome

**Chosen option: Option 2 (Checksum Verification)**

This option directly extends the existing binary integrity verification to libraries, providing automatic corruption detection without requiring recipe changes.

### Rationale

Option 2 was chosen because:
- **Consistency**: Uses the same verification mechanism as tools (checksum comparison)
- **Existing patterns**: Directly extends `BinaryChecksums` infrastructure from DESIGN-checksum-pinning
- **No recipe changes**: Works automatically for all library recipes
- **Industry alignment**: Follows the hash-based integrity pattern used by RPM, dpkg, Pacman, and Nix

Alternatives were rejected because:
- **Option 1 (File Existence)**: Doesn't detect corruption; requires explicit recipe changes
- **Option 3 (Pattern-Based)**: Too coarse - patterns don't identify specific required files
- **Option 4 (Hybrid)**: Over-engineered; checksums alone provide sufficient verification

### Trade-offs Accepted

By choosing this option, we accept:
- Pre-existing libraries (installed before this feature) will show "SKIPPED" during verification
- Users who want to verify pre-existing libraries must reinstall them

These are acceptable because:
- The graceful degradation pattern is established by DESIGN-checksum-pinning
- Reinstallation is straightforward: `tsuku install <library> --reinstall`

## Solution Architecture

### Overview

```
┌──────────────────┐     ┌──────────────────┐     ┌──────────────────┐
│  Library Install │────▶│   State Save     │────▶│   Verify Flow    │
│                  │     │                  │     │                  │
│ 1. Download      │     │ 3. Compute       │     │ 5. Load stored   │
│ 2. install_libs  │     │    checksums     │     │    checksums     │
│                  │     │ 4. Save to       │     │ 6. Recompute     │
│                  │     │    state.json    │     │ 7. Compare       │
└──────────────────┘     └──────────────────┘     └──────────────────┘
```

### State Schema Extension

Extend `LibraryVersionState` to include checksums:

```go
type LibraryVersionState struct {
    UsedBy    []string          `json:"used_by"`
    Checksums map[string]string `json:"checksums,omitempty"` // NEW: path -> SHA256 hex
}
```

### Component Changes

**`internal/install/state.go`:**
- Add `Checksums` field to `LibraryVersionState`

**`internal/install/library.go`:**
- After copying library files, compute checksums of all non-symlink files
- Store checksums in state via new `SetLibraryChecksums()` method

**`internal/install/checksum.go`:**
- Add `ComputeLibraryChecksums(libDir string) (map[string]string, error)`
- Reuse existing `ComputeFileChecksum()` for individual files
- Handle symlinks: resolve to target, checksum the actual file

**`cmd/tsuku/verify.go`:**
- Detect library type from recipe metadata
- For libraries: skip command execution, perform checksum verification only
- Output format matches tool verification style

### Verification Flow for Libraries

```
$ tsuku verify gcc-libs
Verifying gcc-libs (version 15.2.0)...
  Type: library (installed to $TSUKU_HOME/libs/)
  Integrity: Verifying 4 files...
  Integrity: OK (4 files verified)
gcc-libs is correctly installed
```

With tampering detected:
```
$ tsuku verify gcc-libs
Verifying gcc-libs (version 15.2.0)...
  Type: library (installed to $TSUKU_HOME/libs/)
  Integrity: Verifying 4 files...
  Integrity: MODIFIED
    lib/libstdc++.so.6.0.33: expected abc123..., got def456...
    WARNING: Library file may have been modified after installation.
    Run 'tsuku install gcc-libs --reinstall' to restore original.
```

Pre-feature installation:
```
$ tsuku verify gcc-libs
Verifying gcc-libs (version 15.2.0)...
  Type: library (installed to $TSUKU_HOME/libs/)
  Integrity: SKIPPED (no stored checksums - pre-feature installation)
gcc-libs is installed (integrity not verified)
```

### Symlink Handling

Library files often include symlinks for versioning:
```
lib/libstdc++.so -> libstdc++.so.6
lib/libstdc++.so.6 -> libstdc++.so.6.0.33
lib/libstdc++.so.6.0.33  (actual file)
```

Strategy:
1. **During install**: Checksum only real files (not symlinks)
2. **During verify**:
   - Verify real file checksums
   - Verify symlinks still point to expected targets (optional enhancement)

This matches the existing `install_libraries` action which preserves symlinks.

## Implementation Approach

### Step 1: Extend State Schema

Add `Checksums` field to `LibraryVersionState` in `internal/install/state.go`:

```go
type LibraryVersionState struct {
    UsedBy    []string          `json:"used_by"`
    Checksums map[string]string `json:"checksums,omitempty"`
}
```

### Step 2: Add State Manager Method

Add to `internal/install/state.go`:

```go
func (sm *StateManager) SetLibraryChecksums(name, version string, checksums map[string]string) error {
    // Load, modify, save pattern following existing AddLibraryUsedBy
}
```

### Step 3: Add Library Checksum Computation

Create helper in `internal/install/checksum.go`:

```go
// ComputeLibraryChecksums computes SHA256 checksums for all regular files in a library directory.
// Symlinks are skipped; their targets should be checksummed directly.
// All files in the library directory are checksummed (lib/, include/, etc.).
func ComputeLibraryChecksums(libDir string) (map[string]string, error) {
    checksums := make(map[string]string)

    // Canonicalize libDir to handle symlinks
    canonicalLibDir, err := filepath.EvalSymlinks(libDir)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve library directory: %w", err)
    }

    err = filepath.WalkDir(libDir, func(path string, d fs.DirEntry, err error) error {
        if err != nil || d.IsDir() {
            return err
        }

        info, err := d.Info()
        if err != nil {
            return err
        }

        // Skip symlinks - checksum only real files
        if info.Mode()&os.ModeSymlink != 0 {
            return nil
        }

        // Security: verify file is within library directory (prevent symlink escape)
        realPath, err := filepath.EvalSymlinks(path)
        if err != nil {
            return fmt.Errorf("failed to resolve path %s: %w", path, err)
        }
        if !isWithinDir(realPath, canonicalLibDir) {
            return fmt.Errorf("file resolves outside library directory: %s", path)
        }

        relPath, _ := filepath.Rel(libDir, path)
        checksum, err := ComputeFileChecksum(path)
        if err != nil {
            return fmt.Errorf("checksum %s: %w", relPath, err)
        }
        checksums[relPath] = checksum
        return nil
    })

    return checksums, err
}
```

### Step 4: Store Checksums During Library Installation

Modify `Manager.InstallLibrary()` in `internal/install/library.go`:

```go
// After copying files to libDir...
checksums, err := ComputeLibraryChecksums(libDir)
if err != nil {
    return fmt.Errorf("computing library checksums: %w", err)
}

if err := m.state.SetLibraryChecksums(name, version, checksums); err != nil {
    return fmt.Errorf("storing library checksums: %w", err)
}
```

### Step 5: Extend Verify Command

Modify `cmd/tsuku/verify.go` to handle libraries. Insert library detection before the `r.Verify.Command == ""` check:

```go
// After loading recipe...
if r.Metadata.Type == "library" {
    verifyLibrary(r, toolName, cfg)
    return
}

// Check if recipe has verification (existing code)
if r.Verify.Command == "" {
    // ... existing error handling
}
```

The `verifyLibrary()` function:
1. Uses `GetInstalledLibraryVersion()` to find installed version
2. Loads stored checksums from `state.Libs[name][version].Checksums`
3. Recomputes checksums using `ComputeLibraryChecksums()`
4. Compares and reports using existing `ChecksumMismatch` type

### Step 6: Add Tests

- Unit tests for `ComputeLibraryChecksums()`
- Integration test: install library, verify checksums stored, verify passes
- Test graceful degradation for pre-existing libraries

## Security Considerations

### Download Verification

**Not applicable.** Library download verification uses existing recipe mechanisms (checksums, signatures). This design only addresses post-installation verification.

### Execution Isolation

**Low risk.** Library verification does not execute library code. File operations (stat, checksum) require only read access to `$TSUKU_HOME/libs/`.

### Supply Chain Risks

**Not applicable.** This design verifies installed libraries, not their source. Supply chain integrity is handled by recipe checksums at download time.

### User Data Exposure

**Not applicable.** Library verification reads library files and state.json. No user data is accessed or transmitted.

## Consequences

### Positive

- **Consistent UX**: `tsuku verify` works uniformly for tools and libraries
- **Corruption detection**: SHA256 checksums detect post-installation tampering or corruption
- **No recipe changes**: Works automatically with existing library recipes
- **Reuses infrastructure**: Extends proven `BinaryChecksums` pattern from tools
- **Industry-standard approach**: Follows patterns established by RPM, dpkg, Pacman, Nix

### Negative

- **State file size increase**: ~100-200 bytes per installed library version for checksums
- **Pre-existing libraries unverified**: Libraries installed before this feature won't have stored checksums
- **Symlink chain modification undetected**: If symlinks are modified to point to different (but valid) targets, verification won't detect this since only real file checksums are stored. This is a known limitation shared with the existing binary checksum system.

### Neutral

- **Verification time**: Checksumming library files adds ~10-50ms to verification, acceptable for explicit verify command
