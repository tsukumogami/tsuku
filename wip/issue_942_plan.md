# Issue 942 Implementation Plan

## Summary

Add a `Checksums` field to `LibraryVersionState` for storing SHA256 checksums of library files.

## Files to Modify

1. `internal/install/state.go` - Add the `Checksums` field
2. `internal/install/state_test.go` - Add tests for serialization/deserialization

## Implementation Steps

### Step 1: Add Checksums field to LibraryVersionState

Update `LibraryVersionState` in `internal/install/state.go:95-98`:

```go
type LibraryVersionState struct {
    UsedBy    []string          `json:"used_by"`             // Tools that depend on this library version
    Checksums map[string]string `json:"checksums,omitempty"` // SHA256 checksums of library files (path -> checksum)
}
```

The `omitempty` tag ensures backward compatibility - existing state.json files without checksums will load correctly and serialize without the checksums field when empty.

### Step 2: Add unit tests

Add tests in `internal/install/state_test.go`:

1. Test serialization/deserialization with checksums
2. Test backward compatibility (load state without checksums)
3. Test that empty checksums map is omitted from JSON

## Risk Assessment

- **Low risk**: Simple struct field addition
- **Backward compatible**: Using `omitempty` ensures old state.json files load correctly
- **No behavior change**: This only adds a field; downstream issue #946 will populate it

## Testing Strategy

- Unit tests for JSON round-trip with checksums
- Verify backward compatibility with existing test patterns
