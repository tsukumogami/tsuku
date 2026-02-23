# Pragmatic Review: Issue #1937

## Summary

No blocking findings. One advisory.

## Findings

### Advisory 1: `unscopedPackageName` is a single-caller helper

`/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/builders/npm.go:288` -- `unscopedPackageName()` is called from exactly one production site (`parseBinField`, line 268). Could be inlined as a two-line conditional. However, the name is descriptive and the function has its own test suite, so it adds modest clarity. **Not blocking** because it's small and bounded.

### Note: Pre-existing fallback gap (out of scope)

`discoverExecutables()` at lines 233, 240, 247 falls back to `pkgInfo.Name` without stripping scope prefixes. For scoped packages like `@angular/cli`, the fallback would produce `@angular/cli` as an executable name. This is pre-existing behavior, not introduced by this diff. Likely worth a follow-up but not blocking here.

## Scope Assessment

The change stays within the issue description:
- Fixes `parseBinField()` to handle string-type `bin` field (the bug)
- Adds `packageName` parameter to `parseBinField` (required by the fix)
- Adds `unscopedPackageName` for scope stripping (required by the fix)
- Tests cover string/map, scoped/unscoped, nil, and invalid type cases
- No unrelated refactoring or feature additions

## Complexity Assessment

The implementation is the simplest correct approach. The type switch on `bin any` mirrors the existing `extractRepositoryURL` pattern already in this file. No new abstractions, interfaces, or configuration were introduced.
