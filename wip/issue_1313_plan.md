# Issue 1313 Implementation Plan

## Summary

Add entry validation to `ParseRegistry` that rejects entries with empty `builder` or `source` fields. Also fix pre-existing stdlib log import in chain.go.

## Files to Modify

- `internal/discover/registry.go` — Add validateEntries() called from ParseRegistry
- `internal/discover/registry_test.go` — Add tests for validation
- `internal/discover/chain.go` — Fix stdlib log import (pre-existing lint failure)

## Implementation Steps

- [ ] Add validateEntries() to ParseRegistry in registry.go
- [ ] Add tests for empty builder, empty source, missing fields
- [ ] Fix chain.go stdlib log import
