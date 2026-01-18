# Issue #986 Baseline

## Branch
`feature/986-soname-index`

## Starting Point
- Commit: c46a9141 (main)
- All tests passing

## Test Results
```
ok  github.com/tsukumogami/tsuku
ok  github.com/tsukumogami/tsuku/cmd/tsuku
ok  github.com/tsukumogami/tsuku/internal/install
ok  github.com/tsukumogami/tsuku/internal/verify
... (all packages pass)
```

## Issue Summary
Implement SonameIndex and ClassifyDependency for Tier 2 dependency validation. Provides O(1) reverse lookups from soname to recipe and categorizes dependencies.

## Dependencies
- #978 (Add Sonames field) - DONE (merged in main)
- #980 (System library patterns) - DONE (merged in main)
