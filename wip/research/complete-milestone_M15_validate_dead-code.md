# Dead Code Scanner Report - Milestone M15

**Milestone:** M15 - Deterministic Recipe Execution
**Date:** 2025-12-18
**Scanner Role:** Dead Code Scanner

## Executive Summary

This report identifies leftover development artifacts related to milestone M15 (Deterministic Recipe Execution). The scan found **2 findings** requiring attention: debug print statements and a placeholder issue reference.

## Scanning Methodology

The scan covered the following artifact categories:

1. **TODO Comments for Closed Issues** - Searched for TODO/FIXME/HACK/XXX comments referencing milestone issues
2. **Debug Code Patterns** - Searched for console.log, fmt.Println with debug messages, commented-out code
3. **Unused Feature Flags** - Searched for feature flag patterns that may be always-on/off
4. **Leftover Test Artifacts** - Searched for .only/.skip markers and disabled tests

## Findings

### 1. Debug Print Statements in npm_exec.go

**Location:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/actions/npm_exec.go:262-263`

```go
if err != nil {
    fmt.Printf("   Debug: node exec failed: %v\n", err)
    fmt.Printf("   Debug: output: %s\n", string(output))
    return fmt.Errorf("node.js not found or failed to execute: %w", err)
}
```

**Issue:** Debug print statements using `fmt.Printf` with "Debug:" prefix remain in production code. These should either be:
- Removed if no longer needed
- Converted to proper logging using the internal/log package
- Converted to user-facing error messages if the information is valuable

**Severity:** Low - Does not affect functionality but violates logging conventions

**Recommendation:** Remove debug prints or convert to structured logging

### 2. Placeholder Issue Reference in gem_install.go

**Location:** `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/actions/gem_install.go:331`

```go
// Special case: bundler cannot install itself via bundle install
// Use direct gem install instead. This is a known architectural limitation
// where bundler's self-referential nature prevents decomposition.
// See issue #XXX for further investigation.
```

**Issue:** Comment contains placeholder `#XXX` instead of a real issue number. This appears to be documentation of a known limitation but lacks a tracking issue.

**Severity:** Low - Documentation gap, not a code issue

**Recommendation:** Either:
- Create a tracking issue for the bundler self-install limitation and update the reference
- Remove the "See issue #XXX" line if no tracking is needed
- Change to a more generic comment like "This is a known architectural limitation"

## Non-Findings (Clean Areas)

The following areas were scanned and found clean:

### Test Skip Markers
- All `t.Skip()` calls found are legitimate conditional skips for:
  - Environment-based skips (API keys, short mode, platform checks)
  - Integration test guards
  - Tool availability checks
- No milestone-related skips found that should be removed

### TODO/FIXME Comments
- No TODO/FIXME/HACK comments referencing M15, milestone 15, or closed issues
- No orphaned TODO comments found

### Feature Flags
- Found "feature flags" reference in `cargo_build.go:213` but this is a legitimate comment about Cargo's `--no-default-features` and `--all-features` flags, not a dead feature flag

### Commented Code
- Minimal commented code found
- Most comments are documentation or explanatory notes
- No large blocks of commented-out code from M15 work

### Milestone-Related Code
- Scanned 60 Go files containing "deterministic" references
- Scanned 69 Go files containing "ExecutionContext" references
- Scanned 44 files containing "decompos" references
- All references are active code, not dead artifacts

### WIP Directory
- Only contains current milestone completion artifacts:
  - `wip/complete-milestone_M15_state.json`
  - `wip/research/complete-milestone_M15_validate_metadata.md`
- No leftover artifacts from M15 implementation

## Detailed Analysis

### Debug Code Pattern Search

**Pattern:** `fmt.Printf.*debug|fmt.Println.*debug`
**Results:** 2 instances in npm_exec.go (lines 262-263)

These are the only debug print statements found in the codebase. They appear to be diagnostic code added during development.

### TODO/FIXME/HACK Search

**Pattern:** `(TODO|FIXME|HACK|XXX|NOTE).*(?:M15|milestone 15|deterministic|execution context|decompos)`
**Results:** No matches

No TODO comments referencing milestone-specific work were found.

### Commented Code Search

**Pattern:** `^\s*//\s*(fmt\.|log\.|return\s+|if\s+.*==)`
**Results:** Minimal matches, all legitimate documentation

No significant blocks of commented-out code found.

### Test Artifact Search

**Pattern:** `\.only\(|\.skip\(|t\.Skip\(`
**Results:** 52 instances of `t.Skip()`, all legitimate

All test skips are conditional based on:
- Environment variables (ANTHROPIC_API_KEY, GOOGLE_API_KEY, etc.)
- Platform checks (Linux vs non-Linux)
- Short mode (`testing.Short()`)
- Tool availability (npm, nix-portable, patch)

None are related to M15 milestone work.

## Impact Assessment

### High Priority
None.

### Medium Priority
None.

### Low Priority
1. Remove debug print statements in npm_exec.go
2. Resolve placeholder issue reference in gem_install.go

## Recommendations

1. **Short-term:** Remove or convert the debug print statements in npm_exec.go to proper logging
2. **Short-term:** Address the #XXX placeholder in gem_install.go by either creating an issue or removing the reference
3. **Long-term:** Consider adding a linting rule to detect debug print patterns in production code

## Conclusion

The codebase is remarkably clean with respect to dead code artifacts. Only 2 low-severity findings were identified:
- Debug print statements in npm_exec.go
- Placeholder issue reference in gem_install.go

Neither finding blocks milestone completion, but both should be addressed for code quality.

## Appendix: Search Statistics

- Files scanned: ~300 Go files
- TODO/FIXME patterns searched: 0 matches
- Debug print patterns: 2 instances
- Test skip markers: 52 instances (all legitimate)
- Commented code blocks: None significant
- Feature flags: 0 dead flags found
- Placeholder references: 1 (#XXX)
