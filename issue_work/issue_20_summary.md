# Issue #20 Summary

## What Was Implemented

Created an error formatting system that extracts actionable suggestions from structured error types and displays them to users. The implementation leverages the Suggester interface pattern, where both `version.ResolverError` and `registry.RegistryError` already have `Suggestion()` methods.

## Changes Made

- `internal/errmsg/errmsg.go`: New package with `Suggester` interface, `FormatError()` and `Fprint()` functions
- `internal/errmsg/errmsg_test.go`: Comprehensive tests for error formatting
- `cmd/tsuku/helpers.go`: Added `printError()` helper using errmsg.Fprint
- `cmd/tsuku/install.go`: Updated to use printError for error output
- `cmd/tsuku/versions.go`: Updated to use printError for error output
- `cmd/tsuku/update.go`: Updated to use printError for error output
- `cmd/tsuku/outdated.go`: Updated to use printError for error output

## Key Decisions

- **Suggester interface approach**: Used interface type assertion instead of importing concrete error types, avoiding import cycles and keeping the errmsg package dependency-free
- **Error chain traversal**: Used `errors.Unwrap()` to walk the error chain, finding suggestions in wrapped errors
- **Preserved additional help text**: For recipe loading errors, kept the "tsuku create" ecosystem hint alongside the structured suggestion

## Trade-offs Accepted

- **Some hardcoded help text remains**: The "tsuku create" hint is specific to recipe loading and not easily generalized into the error type itself

## Test Coverage

- New tests added: 3 test functions (FormatError, Fprint, extractSuggestion) with multiple sub-tests
- errmsg package: 100% coverage

## Known Limitations

- Only CLI commands that were updated will show suggestions; other error output paths (e.g., in config, create commands) still use direct fmt.Fprintf

## Example Output

Before:
```
Error: registry: recipe nonexistent not found in registry
```

After:
```
Error: registry: recipe nonexistent not found in registry

Suggestion: Verify the recipe name is correct. Run 'tsuku recipes' to list available recipes
```
