# Issue #114: Detect Specific Errors in Registry Client

## Baseline

### Issue Summary
Update the registry client in `internal/registry/` to detect specific error conditions and return appropriate error types, similar to what was done in #113 for version providers.

### Current State

**File:** `internal/registry/registry.go`

Current error handling uses generic `fmt.Errorf()`:
- Line 72-73: `fmt.Errorf("invalid recipe name")`
- Line 77: `fmt.Errorf("failed to create request: %w", err)`
- Line 82: `fmt.Errorf("failed to fetch recipe: %w", err)` - network errors
- Line 87: `fmt.Errorf("recipe not found in registry: %s", name)` - HTTP 404
- Line 91: `fmt.Errorf("registry returned status %d for recipe %s", ...)` - other HTTP errors

### Dependencies
- #112 (define specific error types) - COMPLETED
- #113 (detect specific errors in version providers) - COMPLETED (provides pattern to follow)

### Blocking
- #20 (improve error messages with actionable suggestions)

### Starting Commit
Will be created after this baseline is committed.
