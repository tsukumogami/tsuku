# Issue #984 Baseline

## Branch
`feature/984-externally-managed-for`

## Starting Point
- Commit: d25d6f3c (main)
- All tests passing

## Test Results
```
ok  	github.com/tsukumogami/tsuku	11.140s
ok  	github.com/tsukumogami/tsuku/cmd/tsuku	0.046s
ok  	github.com/tsukumogami/tsuku/internal/actions	(cached)
ok  	github.com/tsukumogami/tsuku/internal/recipe	0.097s
... (all packages pass)
```

## Issue Summary
Add `IsExternallyManagedFor(target Matchable, actionLookup func(string) interface{}) bool` method to Recipe struct.

## Dependencies
- #979 (IsExternallyManaged on SystemAction) - COMPLETED, merged in d25d6f3c
