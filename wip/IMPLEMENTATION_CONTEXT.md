# Implementation Context: Issue #942

**Source**: docs/designs/DESIGN-library-verification.md

## Summary

This is a **simple tier** issue from the Library Verification Infrastructure milestone.

**Goal**: Add a `Checksums` field to `LibraryVersionState` to store SHA256 checksums of library files at installation time, enabling integrity verification.

## Design Reference

From DESIGN-library-verification.md, Solution Architecture section:

**Modified: `internal/install/state.go`**
- Add `Checksums map[string]string` to `LibraryVersionState`

The `Checksums` map uses relative paths (within the library directory) as keys and SHA256 hex strings as values.

Example:
```json
{
  "lib/libstdc++.so.6.0.33": "abc123...",
  "lib/libgcc_s.so.1": "def456..."
}
```

## Dependencies

- None (this is the first issue in the dependency chain)

## Downstream Dependencies

- #946: feat(install): compute and store library checksums at install time (requires this field to exist)
- #950: docs: design integrity verification (Tier 4) (requires checksums for comparison)

## Exit Criteria

- [ ] `LibraryVersionState` has a new `Checksums` field of type `map[string]string`
- [ ] The field has JSON tag `json:"checksums,omitempty"` to omit when empty (backward compatibility)
- [ ] Existing state.json files without checksums continue to load correctly
- [ ] Unit tests verify the new field serializes and deserializes correctly
