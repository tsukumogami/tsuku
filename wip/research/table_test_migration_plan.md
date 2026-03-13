# Table-Driven Test Migration Plan

Analysis of test files for consolidation into table-driven tests with `t.Run` subtests.

## Files Analyzed

- `internal/actions/gem_exec_test.go` (1292 lines)
- `internal/actions/composites_test.go` (758 lines)
- `internal/builders/repair_loop_test.go` (346 lines)
- `cmd/tsuku/sysdeps_test.go` (267 lines)
- `cmd/tsuku/dependency_test.go` (575 lines)

---

## Opportunity 1: GemExec Parameter Validation Tests

**File:** `internal/actions/gem_exec_test.go`

**Tests to merge:**
- `TestGemExecAction_RequiresSourceDir` (line 18)
- `TestGemExecAction_RequiresCommand` (line 33)
- `TestGemExecAction_ValidatesGemfileExists` (line 57)

**Table struct:**
```go
tests := []struct {
    name        string
    params      map[string]interface{}
    setupFiles  map[string]string // relative path -> content (files to create in workDir)
    errContains string
}
```

**Line savings:** ~35 lines (3 functions at ~15 lines each -> 1 table at ~25 lines)

**Merge type:** Mechanical. All three follow the same pattern: create a `GemExecAction`, set up an `ExecutionContext`, call `Execute` with specific params, assert a specific error string. The only variance is which files exist in the workdir and which params are passed.

---

## Opportunity 2: GemExec Version Validation Tests (Ruby + Bundler)

**File:** `internal/actions/gem_exec_test.go`

**Tests to merge:**
- `TestGemExecAction_ValidateRubyVersion_WithMock` (line 723)
- `TestGemExecAction_ValidateBundlerVersion_WithMock` (line 751)
- `TestGemExecAction_ValidateRubyVersion_BadOutput` (line 779)
- `TestGemExecAction_ValidateBundlerVersion_BadOutput` (line 804)

**Table struct:**
```go
tests := []struct {
    name           string
    mockBinary     string   // "ruby" or "bundle"
    mockOutput     string   // what the mock script prints
    validateFunc   func(version string) error
    checkVersion   string
    expectErr      bool
    errContains    string
}
```

**Line savings:** ~50 lines (4 functions at ~25 lines each -> 1 table at ~45 lines)

**Merge type:** Mechanical. All four create a mock executable in a temp dir, override PATH, then call either `validateRubyVersion` or `validateBundlerVersion` and check the result. The structure is identical; only the binary name, mock output, method called, and expected result differ.

---

## Opportunity 3: GemExec BuildEnvironment Tests

**File:** `internal/actions/gem_exec_test.go`

**Tests to merge:**
- `TestGemExecAction_BuildEnvironment` (line 151)
- `TestGemExecAction_BuildEnvironmentWithCustomVars` (line 200)
- `TestGemExecAction_BuildEnvironmentEmptyOutputDir` (line 709)

**Table struct:**
```go
tests := []struct {
    name          string
    sourceDir     string
    outputDir     string
    useLockfile   bool
    customEnv     map[string]string
    wantEnvVars   []string  // env vars that must be present
    rejectEnvVars []string  // env vars that must NOT be present
}
```

**Line savings:** ~40 lines (3 functions totaling ~65 lines -> 1 table at ~30 lines)

**Merge type:** Mostly mechanical. The assertion patterns differ slightly -- the first test checks 4 specific vars, the second checks 2 custom vars, the third checks a negative condition. The table struct above with `wantEnvVars`/`rejectEnvVars` unifies these cleanly.

---

## Opportunity 4: GemExec FindBundler Tests

**File:** `internal/actions/gem_exec_test.go`

**Tests to merge:**
- `TestGemExecAction_FindBundler_InToolsDir` (line 236)
- `TestGemExecAction_FindBundler_NotFound` (line 260)

**Table struct:**
```go
tests := []struct {
    name           string
    setupRubyBin   bool
    expectFound    bool
}
```

**Line savings:** ~15 lines (2 functions at ~22 lines each -> 1 table at ~30 lines)

**Merge type:** Mechanical, but marginal savings. Only 2 cases. Worth doing if touching this area anyway, not worth a dedicated change.

---

## Opportunity 5: GemExec Integration Tests (Execute with mock bundler)

**File:** `internal/actions/gem_exec_test.go`

**Tests to merge:**
- `TestGemExecAction_ExecuteWithMockBundler` (line 349)
- `TestGemExecAction_ExecuteWithExecutablesVerification` (line 396)
- `TestGemExecAction_ExecutableVerificationFails` (line 452)
- `TestGemExecAction_BundlerExecutionFails` (line 507)
- `TestGemExecAction_NonInstallCommand` (line 552)
- `TestGemExecAction_BundlerNotFound` (line 594)

**Table struct:**
```go
tests := []struct {
    name            string
    mockBundleScript string
    params          map[string]interface{}
    setupExecutables []string       // executables to create in output/bin/
    setupToolsDir    bool           // whether to create ruby+bundler in tools/
    overridePath     bool           // whether to override PATH to prevent system bundler
    expectErr        bool
    errContains      string
}
```

**Line savings:** ~120 lines (6 functions averaging ~40 lines each = ~240 lines -> ~120 lines for table + shared setup)

**Merge type:** Requires judgment. These tests share massive boilerplate (create workDir, write Gemfile, write Gemfile.lock, create mock bundler, set up ExecutionContext), but the setup details vary: some create executables, some use failing bundler scripts, some override PATH. A shared setup helper function plus a table would work, but the table struct needs enough fields to capture the variance. High value due to large line savings, but the setup helper needs careful design to avoid becoming an abstraction that's harder to read than the originals.

---

## Opportunity 6: GemExec "must fail at validation" Tests

**File:** `internal/actions/gem_exec_test.go`

**Tests to merge:**
- `TestGemExecAction_WithRubyVersionValidation` (line 635)
- `TestGemExecAction_WithBundlerVersionValidation` (line 672)

**Table struct:**
```go
tests := []struct {
    name           string
    extraParams    map[string]interface{}
    errAlternatives []string  // any of these substrings is acceptable
}
```

**Line savings:** ~25 lines (2 functions at ~35 lines each -> 1 table at ~40 lines)

**Merge type:** Mechanical. Identical structure: create workDir with Gemfile+lockfile, call Execute with a version param, assert error contains one of two acceptable strings. The "accept either X or Y error" pattern is identical in both.

---

## Opportunity 7: Composite Action Name Tests

**File:** `internal/actions/composites_test.go`

**Tests to merge:**
- `TestDownloadArchiveAction_Name` (line 116)
- `TestGitHubArchiveAction_Name` (line 124)
- `TestGitHubFileAction_Name` (line 132)

**Table struct:**
```go
tests := []struct {
    name     string
    action   Action
    wantName string
}
```

**Line savings:** ~15 lines (3 functions at ~6 lines each -> 1 table at ~12 lines)

**Merge type:** Mechanical. Trivial. Each is a one-liner assertion on `Name()`.

---

## Opportunity 8: RepairLoop Session Generation Tests

**File:** `internal/builders/repair_loop_test.go`

**Tests to merge:**
- `TestRepairLoop_FixesBrokenRecipe` (line 89)
- `TestRepairLoop_GeneratesRecipeWithoutSandbox` (line 182)

**Table struct:**
```go
tests := []struct {
    name              string
    builderOpts       []Option  // WithTelemetryClient, etc.
    wantRepairAttempts int
    wantProviderCalls  int
}
```

**Line savings:** ~30 lines (2 functions at ~40 lines each -> 1 table at ~50 lines)

**Merge type:** Requires judgment. These are nearly identical (same mock response, same mock server, same assertions) but differ in builder options and the specific assertions. The difference is intentional: one tests telemetry integration, the other tests sandbox absence. A comment in the table entries would preserve that intent, but a reviewer might wonder why they're grouped. Advisory -- not high priority.

---

## Opportunity 9: GemExec Relative Path Tests

**File:** `internal/actions/gem_exec_test.go`

**Tests to merge:**
- `TestGemExecAction_RelativeSourceDir` (line 283)
- `TestGemExecAction_RelativeOutputDir` (line 317)

**Table struct:**
```go
tests := []struct {
    name        string
    params      map[string]interface{}  // includes relative source_dir or output_dir
    badError    string                   // error that should NOT appear
}
```

**Line savings:** ~20 lines (2 functions at ~30 lines each -> 1 table at ~40 lines)

**Merge type:** Requires judgment. The assertion styles are slightly different (one checks a specific error string, the other checks two possible error strings). The negative assertion ("this error should not appear") is unusual and the table struct needs to represent it cleanly.

---

## Opportunity 10: ResolveRuntimeDeps Tests

**File:** `cmd/tsuku/dependency_test.go`

**Tests to merge:**
- `TestResolveRuntimeDeps_NoRuntimeDeps` (line 448)
- `TestResolveRuntimeDeps_WithRuntimeDeps` (line 467)
- `TestResolveRuntimeDeps_MissingDep` (line 505)

**Table struct:**
```go
tests := []struct {
    name              string
    runtimeDeps       []string
    preInstalled      map[string]string  // tool -> version to pre-install
    expectNil         bool
    expectVersions    map[string]string  // expected resolved versions
}
```

**Line savings:** ~30 lines (3 functions at ~25 lines each -> 1 table at ~40 lines)

**Merge type:** Mostly mechanical. The three tests share the same pattern: create config, optionally install deps, create recipe, call `resolveRuntimeDeps`, check result. The third test has a slightly unusual assertion (accepts both nil and empty map), but that maps cleanly to `expectNil: false, expectVersions: map[string]string{}`.

---

## Opportunity 11: MapKeys Tests

**File:** `cmd/tsuku/dependency_test.go`

**Tests to merge:**
- `TestMapKeys` (line 394) -- already table-driven
- `TestMapKeys_Empty` (line 438)

**Table struct:** Already exists in `TestMapKeys`. The `TestMapKeys_Empty` test adds a nil-input case that should just be added as another row in the existing table.

**Line savings:** ~8 lines

**Merge type:** Mechanical. Just add `{name: "nil map", m: nil, want: 0}` to the existing table and delete `TestMapKeys_Empty`. The nil-specific assertion (`keys == nil` check) is worth preserving in the table test as a comment or additional assertion.

---

## Already Table-Driven (No Action Needed)

These tests already use table-driven patterns:
- `TestGemExecAction_LockDataMode_Validation` (gem_exec_test.go:889) -- good example
- `TestCountLockfileGems_EdgeCases` (gem_exec_test.go:1250)
- `TestGitHubArchiveAction_VerificationEnforcement` (composites_test.go:11)
- `TestDownloadArchiveAction_VerificationEnforcement` (composites_test.go:657)
- `TestExtractSourceFiles` (composites_test.go:142)
- `TestDownloadArchiveAction_Execute_MissingParams` (composites_test.go:204)
- `TestGitHubArchiveAction_Execute_MissingParams` (composites_test.go:249)
- `TestGitHubFileAction_Execute_MissingParams` (composites_test.go:312)
- `TestGitHubArchiveAction_Decompose_MissingParams` (composites_test.go:437)
- `TestHasSystemDeps` (sysdeps_test.go:10)
- `TestGetSystemDepsForTarget` (sysdeps_test.go:80)
- `TestGetTargetDisplayName` (sysdeps_test.go:133)
- `TestResolveTarget` (sysdeps_test.go:191)
- `TestOrphanDetection` (dependency_test.go:186)
- `TestLibraryInstallAllowed` (dependency_test.go:530)

---

## Summary

| # | Opportunity | File | Functions | Est. Line Savings | Type |
|---|------------|------|-----------|-------------------|------|
| 1 | GemExec param validation | gem_exec_test.go | 3 | ~35 | Mechanical |
| 2 | Version validation (ruby/bundler) | gem_exec_test.go | 4 | ~50 | Mechanical |
| 3 | BuildEnvironment | gem_exec_test.go | 3 | ~40 | Mechanical |
| 4 | FindBundler | gem_exec_test.go | 2 | ~15 | Mechanical (marginal) |
| 5 | Execute with mock bundler | gem_exec_test.go | 6 | ~120 | Judgment |
| 6 | Version validation (full Execute) | gem_exec_test.go | 2 | ~25 | Mechanical |
| 7 | Composite action names | composites_test.go | 3 | ~15 | Mechanical |
| 8 | RepairLoop session generation | repair_loop_test.go | 2 | ~30 | Judgment |
| 9 | Relative path tests | gem_exec_test.go | 2 | ~20 | Judgment |
| 10 | ResolveRuntimeDeps | dependency_test.go | 3 | ~30 | Mechanical |
| 11 | MapKeys nil case | dependency_test.go | 1 (+row) | ~8 | Mechanical |

**Total opportunities:** 11 groups, covering 31 test functions
**Total estimated line savings:** ~388 lines
**Mechanical (safe):** 7 opportunities (~193 lines)
**Requires judgment:** 4 opportunities (~195 lines)

### Priority Recommendation

**High value, low risk (do first):**
- Opportunity 5 (Execute with mock bundler) -- largest savings but needs careful helper design
- Opportunity 2 (Version validation) -- clean pattern, good savings
- Opportunity 1 (Param validation) -- straightforward

**Low value (do opportunistically):**
- Opportunity 4 (FindBundler) -- only 2 cases, marginal savings
- Opportunity 11 (MapKeys nil) -- just adding one row to existing table
- Opportunity 7 (Action names) -- trivial functions, not much to gain
