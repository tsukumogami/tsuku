---
summary:
  constraints:
    - Field uses `omitempty` tag for backward compatibility with existing state files
    - Must integrate with existing LibraryVersionState struct pattern
    - No migration needed - field is optional
  integration_points:
    - internal/install/state.go (LibraryVersionState struct)
    - internal/install/state_lib.go (SetLibrarySonames helper method)
    - internal/install/state_test.go (unit tests)
  risks:
    - Ensure JSON serialization round-trip works correctly
    - Verify backward compatibility with old state files
  approach_notes: |
    Simple foundational change: add Sonames []string field to LibraryVersionState,
    add helper method SetLibrarySonames(), and write tests. No behavioral changes
    to existing code - this just provides storage for downstream issues #983 and #986.
---

# Implementation Context: Issue #978

**Source**: docs/designs/DESIGN-library-verify-deps.md (None)

## Design Excerpt

This issue is part of Track A (State + Extraction) in the Tier 2 Dependency Validation milestone.

The `Sonames` field enables downstream features:
- #983 (soname extraction) will extract sonames from binaries
- #985 (store sonames on install) will populate this field during library install
- #986 (SonameIndex) will build a reverse index from stored sonames

Key design decision from DESIGN-library-verify-deps.md:

```go
type LibraryVersionState struct {
    UsedBy    []string          `json:"used_by"`
    Checksums map[string]string `json:"checksums,omitempty"`
    Sonames   []string          `json:"sonames,omitempty"` // Auto-discovered
}
```

The `omitempty` tag ensures existing state files load correctly without the field.
