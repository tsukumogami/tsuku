# Exploration Summary: Homebrew Bottle Relocation Fix

## Problem (Phase 1)

Homebrew bottles contain hardcoded paths from their build environment that must be rewritten during installation. The current `homebrew_relocate` action fails to properly rewrite these paths, causing 63+ recipes to fail post-install verification. The core bug: `extractBottlePrefixes()` extracts full paths like `/tmp/build-dir/.install/libexec/bin/tool` but `relocatePlaceholders()` replaces the entire path with just the install prefix, losing the relative suffix (`libexec/bin/tool`).

A secondary issue (`install_mode="directory"`) causes the directory copy to produce incorrect structures for some recipes.

## Decision Drivers (Phase 1)

- **Correctness**: Path replacement must preserve relative path suffixes
- **Backward compatibility**: Can't break recipes that currently work
- **Minimal invasiveness**: Fix the bug without rewriting the entire relocation system
- **Testability**: Solution must be verifiable via post-install checks
- **Platform coverage**: Must work on both Linux (patchelf) and macOS (install_name_tool)

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-10

## Decision (Phase 5)

**Problem:**
Homebrew bottles contain hardcoded build paths that must be rewritten during installation. The `homebrew_relocate` action extracts full paths (including suffixes like `/libexec/bin/tool`) but replaces them entirely with the install prefix, losing the relative path suffix. This causes 63+ recipes to fail post-install verification with "binary not found" errors despite successful installation.

**Decision:**
Modify `extractBottlePrefixes()` to use marker-based path parsing: identify the `/.install/<formula>/<version>` boundary and extract only the bottle prefix portion. In `relocatePlaceholders()`, replace only the prefix while preserving any suffix. For the `install_mode=directory` issue, modify `CopyDirectory` to exclude the `.install` subdirectory to prevent recursive copy.

**Rationale:**
The marker-based approach is more reliable than regex because bottle paths have a consistent structure up to the version boundary but arbitrary structure after it. Fixing the extraction function at the source is less invasive than rewriting the replacement logic. The directory exclusion approach is minimal and avoids broader architectural changes to the workDir/installDir relationship.

## Options (Phase 3)

### Decision 1: Path Prefix Extraction Strategy

**Chosen: Extract only the bottle prefix, not the full path**

Instead of extracting full paths like `/tmp/action-validator-XXX/.install/formula/version/libexec/bin/tool`, extract only the bottle prefix portion (`/tmp/action-validator-XXX/.install/formula/version`) and replace just that prefix, preserving any suffix.

**Alternatives:**
- **Regex-based suffix preservation**: Extract full path, then use regex to identify and preserve suffix. Rejected because it's more complex and error-prone.
- **Path component analysis**: Parse paths into components and replace selectively. Rejected because bottle paths don't follow a consistent structure.

### Decision 2: install_mode=directory Copy Direction

**Chosen: Skip .install subdirectory when copying**

Modify `CopyDirectory` or the caller to exclude the `.install` subdirectory when copying from workDir to workDir/.install. This prevents recursive copy issues.

**Alternatives:**
- **Change directory structure**: Put installDir outside workDir. Rejected because it would break existing behavior and requires larger refactoring.
- **Filter during copy**: Skip any path containing `.install`. Simpler but might miss edge cases.

### Uncertainties

- Unknown how many recipes will be unblocked by this fix (currently 63+ blocked)
- Unknown if there are other bottle path patterns beyond `/tmp/action-validator-*`
- Performance impact of path parsing is likely negligible but not measured

## Review Feedback (Phase 4)

The review agent identified several improvements:

1. **Marker-based approach** is a better alternative for path extraction: Parse paths to find the `/.install/<formula>/<version>` boundary and split there, rather than simple string matching.

2. **Explicit assumptions needed:**
   - Bottle build path format is stable (`/tmp/action-validator-*`)
   - Suffix preservation is always correct (no edge cases)
   - Install path length <= bottle path length (for binary patching)

3. **Rejection rationale strengthened:**
   - Regex-based: Complex because it requires capturing groups across variable path depths
   - Directory structure change: Would require modifying executor.go, manager.go, and all action tests (~15 files)

## Root Cause Analysis

### Issue #1583: Path Suffix Loss

**Location:** `internal/actions/homebrew_relocate.go`

**Bug mechanism:**
1. `extractBottlePrefixes()` (lines 699-742) scans binaries for bottle paths
2. It extracts entire paths like `/tmp/action-validator-11862967/.install/libexec/bin/pod`
3. `relocatePlaceholders()` (lines 215-218) replaces these with just the install prefix
4. Result: `/home/user/.tsuku/tools/tool-1.0` instead of `/home/user/.tsuku/tools/tool-1.0/libexec/bin/pod`

**The fix:** Only replace the bottle prefix portion, preserving the relative path suffix.

### Issue #879: Homebrew Bottle Path Issues

Same root cause as #1583. The macOS curl recipe fails because the homebrew relocation doesn't properly update embedded dylib paths.

### Issue #1581: install_mode=directory Structure

**Location:** `internal/actions/install_binaries.go`

**Bug mechanism:**
1. `installDirectoryWithSymlinks()` copies from `ctx.WorkDir` to `ctx.InstallDir`
2. `ctx.InstallDir` is `workDir/.install` (a subdirectory of workDir)
3. This creates potential recursive copy issues
4. The bottle extraction with `strip_dirs=2` may not produce expected structure

**Affected recipes:** make, cmake, ninja, pkg-config, patchelf

## Scope

**In scope:**
- Fix path suffix preservation in `homebrew_relocate` action
- Verify fix works on both Linux and macOS
- Ensure backwards compatibility with working recipes

**Out of scope:**
- Rewriting the entire relocation system
- Changes to recipe format
- New testing frameworks
