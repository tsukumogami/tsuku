# Issue 607 Implementation Plan

## Summary

Split the `download` action into a recipe-level composite and a plan-level primitive:
- `download` (composite): For recipes with placeholders and checksum URLs
- `download_file` (primitive): For plans with resolved URLs and required inline checksums

## Approach

Based on user feedback, the design enforces clear separation of concerns:

1. **`download` action** (becomes composite):
   - Parameters: `url`, `dest`, `checksum_url`, `os_mapping`, `arch_mapping`
   - Removes inline `checksum` parameter (contradicts purpose)
   - Implements `Decomposable` interface
   - Decomposes to `download_file` primitive with computed checksum

2. **`download_file` action** (new primitive):
   - Parameters: `url`, `dest`, `checksum` (required), `checksum_algo`, `size`
   - No placeholder expansion
   - No os/arch mappings (URL already resolved)
   - Fails if checksum is missing

This design ensures:
- Recipes use `download` with placeholders - checksums computed at eval time
- Plans only contain `download_file` - checksums enforced by type
- Static file downloads in recipes should use `download_file` directly

### Alternatives Considered

- **Keep download as primitive with optional checksum**: User rejected - mixing concerns
- **Add validation-only approach**: Would not provide compile-time safety

## Files to Modify

- `internal/actions/download.go` - Convert to composite, implement `Decomposable`
- `internal/actions/download_test.go` - Update tests for new behavior
- `internal/actions/decomposable.go` - Register `download_file` as primitive, remove `download`
- `internal/actions/action.go` - Register `DownloadFileAction`
- `internal/executor/plan.go` - Update validation to reject `download` in plans
- `internal/executor/plan_test.go` - Update plan validation tests
- `internal/actions/composites.go` - Update `Decompose` to return `download_file`
- `internal/actions/homebrew.go` - Update `Decompose` to return `download_file`
- `internal/actions/homebrew_source.go` - Update `Decompose` to return `download_file`
- `internal/actions/apply_patch.go` - Update `Decompose` to return `download_file`

## Files to Create

- `internal/actions/download_file.go` - New `download_file` primitive action

## Implementation Steps

- [ ] 1. Create `download_file` primitive action
- [ ] 2. Register `download_file` in action registry and primitives list
- [ ] 3. Convert `download` to composite with `Decomposable` interface
- [ ] 4. Remove inline `checksum` from `download`, keep only `checksum_url`
- [ ] 5. Update all `Decompose` methods to return `download_file` instead of `download`
- [ ] 6. Update plan validation to reject `download` actions
- [ ] 7. Update tests for new behavior
- [ ] 8. Run full test suite and fix any issues

## Testing Strategy

- Unit tests:
  - `download_file` requires checksum (fails without)
  - `download` decomposes to `download_file` with checksum
  - Plan validation rejects `download` actions
- Integration tests: Existing recipe tests should continue passing
- Manual verification: `tsuku eval` produces plans with `download_file`

## Risks and Mitigations

- **Breaking existing recipes using inline checksum**: Migration - inline checksum users should switch to `download_file` for static URLs
- **Breaking Decompose implementations**: Systematic update of all Decomposable actions

## Success Criteria

- [ ] `download_file` primitive exists and requires checksum
- [ ] `download` implements `Decomposable` with `checksum_url` support
- [ ] `download` no longer accepts inline `checksum` parameter
- [ ] All `Decompose` methods return `download_file` steps
- [ ] Plan validation rejects `download` (should be decomposed)
- [ ] All tests pass
- [ ] CI passes

## Open Questions

None - design clarified by user feedback.
