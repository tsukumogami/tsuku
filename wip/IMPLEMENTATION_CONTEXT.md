---
summary:
  constraints:
    - Use Go standard library only (debug/elf, debug/macho) - no external dependencies
    - Handle gracefully when soname/install name is absent (return empty string, not error)
    - Support fat/universal binaries on macOS
  integration_points:
    - internal/verify/soname.go (new file)
    - internal/verify/soname_test.go (new file)
    - Downstream: #985 will use these functions to store sonames during install
    - Downstream: SonameIndex (#986) will consume the stored sonames
  risks:
    - Mach-O LC_ID_DYLIB vs LC_LOAD_DYLIB distinction needs careful handling
    - Test fixtures require real binary files or careful mocking
    - Universal binary handling on macOS may need special attention
  approach_notes: |
    Implement four functions in internal/verify/soname.go:
    1. ExtractELFSoname - uses debug/elf.DynString(DT_SONAME)
    2. ExtractMachOInstallName - iterates f.Loads for LC_ID_DYLIB
    3. ExtractSoname - auto-detects format and delegates
    4. ExtractSonames - scans directory, extracts from all library files

    The extraction functions are foundational - used by #985 to populate
    the Sonames field in state.json that we added in #978.
---

# Implementation Context: Issue #983

**Source**: docs/designs/DESIGN-library-verify-deps.md

## Design Excerpt

This issue is part of Track A (State + Extraction) in the Tier 2 Dependency Validation milestone.

The soname extraction functions enable:
- #985 (store sonames during install) to populate the Sonames field
- #986 (SonameIndex) to build reverse lookup from sonames

Key design decision from DESIGN-library-verify-deps.md:

```go
// At library install time, tsuku extracts sonames and stores them in state.json
type LibraryVersionState struct {
    UsedBy    []string          `json:"used_by"`
    Checksums map[string]string `json:"checksums,omitempty"`
    Sonames   []string          `json:"sonames,omitempty"` // Auto-discovered
}
```

The extraction functions provide the "auto-discovery" mechanism that populates this field.
