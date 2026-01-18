# Issue 981 Introspection

## Context Reviewed

- Design doc: `docs/designs/DESIGN-library-verify-deps.md`
- Sibling issues reviewed: #979 (closed), #984 (closed)
- Prior patterns identified:
  - `internal/verify/header.go` - ELF/Mach-O validation patterns, panic recovery, error categorization
  - `internal/verify/types.go` - `ErrorCategory` type, `ValidationError` struct
  - `internal/actions/system_action.go` - `IsExternallyManaged()` interface method

## Gap Analysis

### Minor Gaps

1. **ErrorCategory numbering scheme**: The current `types.go` uses `iota` for error categories (0-5). Issue #981 specifies `ErrABIMismatch = 10` as an explicit value per design decision #2. This is intentional to leave room for future Tier 2 error categories (10, 11, 12 reserved per design). The implementation should add:
   ```go
   // Tier 2 error categories (explicit values per design decision #2)
   ErrABIMismatch ErrorCategory = 10
   ```

2. **String() method update**: The `ErrorCategory.String()` method needs a new case for `ErrABIMismatch`. Following the existing pattern:
   ```go
   case ErrABIMismatch:
       return "ABI mismatch"
   ```

3. **Pattern for static binary detection**: The issue mentions returning "statically linked" in test scenarios but does not provide explicit guidance on return value. Based on design doc context, static binaries should return `nil` (pass) with no special messaging - the caller determines how to report static binaries.

### Moderate Gaps

None identified. The issue spec is detailed enough for implementation.

### Major Gaps

None identified. The issue does not conflict with closed sibling issues.

## Recommendation

**Proceed**

The issue spec is complete and well-aligned with the design document. The closed sibling issues (#979, #984) are in Track B (Actions) and do not affect Track C (Verify Infrastructure) where this issue resides. All minor gaps can be resolved from design document context without user input.

## Implementation Notes (from introspection)

1. Follow the pattern in `header.go` for:
   - Panic recovery (`defer func() { if r := recover()...`)
   - Using Go's `debug/elf` package
   - Returning `*ValidationError` with appropriate category

2. The `ErrorCategory` constants at value 10+ mark Tier 2 validation. Keep existing Tier 1 constants (0-5) using iota, then explicitly define Tier 2 constants starting at 10.

3. The design document shows PT_INTERP check as the first step in validation flow (step 2a), consumed by #989 (recursive validation).
