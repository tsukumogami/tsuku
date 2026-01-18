# Issue 982 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-library-verify-deps.md` (Step 5, Security Mitigations)
- Sibling issues reviewed: #978, #979, #980, #981, #983, #984, #986 (all closed)
- Prior patterns identified:
  - Panic recovery pattern: All binary parsing functions use `defer func() { if r := recover()... }` (see `soname.go`, `abi.go`)
  - Error categorization: `ValidationError` struct with `ErrorCategory` enum in `types.go`
  - File location: New verify features go in `internal/verify/` package
  - Magic detection helpers: Local copies to avoid coupling (see `readMagicForSoname`, `detectFormatForSoname`)
  - Named return values with panic recovery: Functions use `func Name() (result type, err error)` pattern

## Gap Analysis

### Minor Gaps

1. **File location specified**: Issue mentions `internal/verify/deps.go`, but based on pattern from closed issues, a separate `internal/verify/rpath.go` file is more appropriate. The `wip/IMPLEMENTATION_CONTEXT.md` already suggests this.

2. **Error category constants**: Issue #982 requires security checks but doesn't specify error categories. The existing `types.go` has explicit constant values (design decision #2). New error categories may be needed for:
   - Path traversal errors (11 is taken by `ErrUnknownDependency` from #986)
   - RPATH limit exceeded
   - Unexpanded variable errors

   Need to add new constants with explicit values (12, 13, etc.) consistent with prior work.

3. **Panic recovery pattern**: Issue spec doesn't mention panic recovery, but all sibling implementations use it. Must include `defer func() { if r := recover() }` in `ExtractRpaths()` and any function parsing binary data.

4. **IsPathVariable() exists**: The `system_libs.go` already has an `IsPathVariable()` function that checks for `$ORIGIN`, `@rpath`, etc. The `ExpandPathVariables()` function should use this or coordinate with it.

5. **Mach-O RPATH extraction**: Issue mentions `f.Loads` for `LC_RPATH`, but the existing `soname.go` shows that Go's `macho.LoadCmd` constants aren't always exported. The pattern from #983 (`lcIDDylib macho.LoadCmd = 0xd`) suggests defining `lcRpath macho.LoadCmd = 0x8000001c` locally.

### Moderate Gaps

None identified. The issue specification is well-aligned with the design document and patterns established in prior issues.

### Major Gaps

None identified. The issue can proceed with the minor gaps addressed during implementation.

## Recommendation

**Proceed**

The issue specification is complete and well-aligned with both the design document and patterns established by closed sibling issues. Minor gaps are all resolvable from reviewing prior work without user input:
- Use `internal/verify/rpath.go` as the file location (per IMPLEMENTATION_CONTEXT.md)
- Add error categories with explicit values (12, 13, etc.)
- Apply panic recovery pattern from sibling implementations
- Coordinate with existing `IsPathVariable()` in `system_libs.go`
- Use local constant definition for Mach-O LC_RPATH (pattern from #983)

## Proposed Amendments

No amendments needed - the issue specification is sufficient. The minor gaps above should be incorporated into the implementation plan.

## Integration Points

The downstream issue #989 expects:
- `ExtractRpaths(path string) ([]string, error)` - extracts RPATH entries
- `ExpandPathVariables(dep, binaryPath string, rpaths []string) (string, error)` - expands path variables

The function signatures in issue #982 match these expectations exactly.
