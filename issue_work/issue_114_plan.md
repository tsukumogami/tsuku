# Issue #114: Implementation Plan

## Overview

Add structured error types to the registry client, following the pattern established in #113 for version providers.

## Design Decisions

### Reuse vs. Duplicate Error Types

**Decision:** Create a new `RegistryError` type in the registry package that reuses the `ErrorType` constants from the version package.

**Rationale:**
- The error types (`ErrTypeNetwork`, `ErrTypeNotFound`, etc.) are domain-agnostic and should be reusable
- The registry has different context (Recipe instead of Source)
- Keeping the classification logic in version package avoids duplication

**Alternative considered:** Move error types to a shared package. Rejected because:
- Adds complexity for minimal benefit
- Can be done later if more packages need error types

### Implementation Approach

Create `internal/registry/errors.go` with:
1. `RegistryError` struct with Type, Recipe, Message, Err fields
2. Import `version.ErrorType` and `version.ClassifyError()`
3. Add helper functions similar to version package

## Implementation Steps

### Step 1: Create errors.go

Create `internal/registry/errors.go`:
```go
package registry

import (
    "fmt"
    "github.com/tsuku-dev/tsuku/internal/version"
)

// RegistryError provides structured error information for registry operations
type RegistryError struct {
    Type    version.ErrorType
    Recipe  string
    Message string
    Err     error
}

func (e *RegistryError) Error() string { ... }
func (e *RegistryError) Unwrap() error { ... }
func (e *RegistryError) Suggestion() string { ... }

// WrapNetworkError wraps network errors with proper classification
func WrapNetworkError(err error, recipe, message string) *RegistryError { ... }
```

### Step 2: Update registry.go FetchRecipe

Replace generic errors with `RegistryError`:

| Line | Current | New |
|------|---------|-----|
| 72-73 | `fmt.Errorf("invalid recipe name")` | `&RegistryError{Type: ErrTypeValidation, ...}` |
| 82 | `fmt.Errorf("failed to fetch recipe: %w", err)` | `WrapNetworkError(err, name, "...")` |
| 87 | `fmt.Errorf("recipe not found...")` | `&RegistryError{Type: ErrTypeNotFound, ...}` |
| 91 | `fmt.Errorf("registry returned status %d...")` | Check for 429, then use appropriate type |

### Step 3: Add Tests

Create `internal/registry/errors_test.go`:
- Test `RegistryError.Error()` formatting
- Test `RegistryError.Unwrap()`
- Test `RegistryError.Suggestion()` for different error types
- Test `WrapNetworkError()` classification

## Success Criteria

- [ ] `RegistryError` type defined with proper fields
- [ ] `FetchRecipe` returns `*RegistryError` for all error cases
- [ ] HTTP 404 → `ErrTypeNotFound`
- [ ] HTTP 429 → `ErrTypeRateLimit`
- [ ] Network errors → classified via `WrapNetworkError()`
- [ ] Unit tests pass
- [ ] go vet passes
