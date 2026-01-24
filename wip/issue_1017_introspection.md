# Issue 1017 Introspection

## Context Reviewed
- Design doc: docs/designs/DESIGN-library-verify-dlopen.md
- Sibling issues reviewed: #1014, #1015, #1016, #1020
- Prior patterns identified: BatchError type, invokeBatch function signature, cmd structure

## Gap Analysis

### Minor Gaps

1. **Integration point with existing batch code**: The issue mentions "invokeBatch() must use sanitized environment" but #1016 already implemented `invokeBatch()`. The sanitization must be added to the existing function at line 273 (where `cmd` is created). The `cmd.Env` assignment must happen before `cmd.Run()`.

2. **Config access pattern**: The issue mentions `config.Config` provides `TsukuHome` but the current `invokeBatch()` function doesn't have access to `cfg`. Need to either:
   - Pass `tsukuHome` string as parameter to `invokeBatch`
   - Or pass `cfg *config.Config` through the call chain

   Looking at the code, `InvokeDltest` also doesn't have `cfg` access - it only receives `helperPath` and `paths`. The entry point that has `cfg` is `EnsureDltest`.

   **Resolution**: Follow the design doc which shows `sanitizeEnvForHelper(tsukuHome string)` - pass `tsukuHome` as a string parameter rather than the full config object.

3. **Path validation location**: The issue says "Path validation before the batch loop" in `InvokeDltest()` or caller. The design doc shows validation happening inside the invoke function. Following design doc pattern: validate in `InvokeDltest` before processing batches.

4. **libsDir construction**: The design shows `libsDir := filepath.Join(tsukuHome, "libs")`. The issue references `$TSUKU_HOME/libs/` - need to use the same pattern consistently.

### Moderate Gaps

None identified. The issue spec is comprehensive with clear acceptance criteria and implementation notes.

### Major Gaps

None identified. The issue aligns with the design document and doesn't conflict with #1016 implementation.

## Recommendation

Proceed with implementation. Minor gaps are resolvable:
- Add `tsukuHome` parameter to function signatures
- Integrate `cmd.Env` assignment into existing `invokeBatch` function
- Add `validateLibraryPaths` call at start of `InvokeDltest`

## Implementation Approach (from gap analysis)

1. Add `sanitizeEnvForHelper(tsukuHome string) []string` function
2. Add `validateLibraryPaths(paths []string, libsDir string) error` function
3. Modify `InvokeDltest` signature to accept `tsukuHome string` parameter
4. Modify `invokeBatchWithRetry` and `invokeBatch` signatures to accept `tsukuHome string`
5. Set `cmd.Env = sanitizeEnvForHelper(tsukuHome)` in `invokeBatch` before `cmd.Run()`
6. Add path validation at start of `InvokeDltest`
7. Update caller in the codebase to pass `tsukuHome`
