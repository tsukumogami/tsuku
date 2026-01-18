# Issue 990 Introspection

## Context Reviewed

- Design doc: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/docs/designs/DESIGN-library-verify-deps.md`
- Sibling issues reviewed: #978, #979, #980, #981, #982, #983, #984, #985, #986, #989 (all 10 closed)
- Prior patterns identified:
  - `ValidateDependencies` signature evolved from issue #989's spec; now takes 9 parameters
  - A `ValidateDependenciesSimple` wrapper exists for common use cases
  - `DepResult` struct has `Recipe` field, not `Provider` as specified in issue
  - No `DepCategory` constant for `DepTsukuManaged` that would map to `tsuku:` prefix in output

## Gap Analysis

### Minor Gaps

1. **Function signature mismatch**: Issue #990 shows calling `ValidateDependencies(installDir, state, nil)` with 3 parameters, but the actual implementation takes 9 parameters. However, `ValidateDependenciesSimple(binaryPath, state, tsukuHome)` provides the simplified interface that can be used. The issue just needs to use the correct function.

2. **Output format field naming**: Issue specifies `r.Provider` in `displayDependencyResults()` but actual `DepResult` uses `r.Recipe`. This is a minor naming difference easily accommodated.

3. **Category constant naming**: Issue shows `verify.DepTsukuManaged` which correctly matches the actual constant name in `classify.go`.

4. **Binary path vs install dir**: The `ValidateDependencies` functions expect a path to a specific binary file, not a directory. For tools, the integration needs to find the actual binary files in the install directory. For libraries, `findLibraryFiles()` already returns individual file paths.

### Moderate Gaps

1. **No `displayDependencyResults()` output specification for errors**: Issue spec shows output format for passing deps only. Need to clarify how failed/unknown dependencies should be displayed. The `DepResult.Status` and `DepResult.Error` fields suggest failures should be reported.

2. **Tool verification step numbering**: Issue shows adding "Step 5" for dependency validation, but current `verifyVisibleTool()` ends at Step 4 (binary integrity). For hidden tools, there's no step numbering. The integration point is clear but step numbers need verification.

3. **Exit code on partial failure**: Issue says validation failures result in `ExitVerifyFailed`, but doesn't specify if this is immediate per-dep failure or aggregate. The design doc says "FAIL on undeclared deps" which suggests any failure should exit.

### Major Gaps

None identified. The infrastructure in #989 provides all necessary functionality.

## Recommendation

**Proceed**

## Rationale

All prerequisite issues (#978-#989) are closed and the implementation matches the design. The gaps identified are minor adjustments:

1. Use `ValidateDependenciesSimple()` wrapper or construct the full parameter list
2. Adapt field names (`Recipe` vs `Provider` in display)
3. Handle the binary vs directory distinction when iterating files

These are implementation details that don't change the scope or approach.

## Proposed Amendments

No amendments needed to the issue specification. The gaps are addressable during implementation with straightforward adaptations:

1. For tools: iterate over binaries in the install directory and call `ValidateDependenciesSimple` on each
2. For libraries: reuse existing `findLibraryFiles()` results and validate each
3. Display format should show `Recipe` as the provider name (matches design terminology "tsuku:openssl")
4. Report validation failures inline and aggregate for exit code decision
