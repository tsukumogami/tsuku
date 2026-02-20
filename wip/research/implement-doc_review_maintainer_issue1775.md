# Maintainer Review: #1775 Thread GPU Through Plan Generation

## Review Scope

Issue #1775 wires GPU detection into plan generation so that recipe steps with `gpu` conditions actually filter at plan time. Changes span 4 files:

- `internal/executor/plan_generator.go` -- GPU auto-detection, depCfg propagation, dependency target construction
- `internal/executor/plan_generator_test.go` -- GPU filtering tests, dependency propagation test, LinuxFamily propagation test
- `internal/executor/executor.go` -- `shouldExecute()` now passes `DetectGPU()` to `NewMatchTarget`
- `internal/executor/filter_test.go` -- GPU step filtering tests for `FilterStepsByTarget`

## Findings

### 1. BLOCKING: GPU auto-detection mutates `cfg` parameter, but LinuxFamily auto-detection does not -- inconsistent pattern creates a trap

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/plan_generator.go:87-104`

```go
// Detect linux_family if on Linux and not provided
linuxFamily := cfg.LinuxFamily
if targetOS == "linux" && linuxFamily == "" {
    detectedFamily, err := platform.DetectFamily()
    // ... assigns to linuxFamily local variable
}

// Auto-detect GPU vendor when not provided
if cfg.GPU == "" {
    cfg.GPU = platform.DetectGPU()  // mutates cfg directly
}
```

LinuxFamily detection writes to a local variable (`linuxFamily`), leaving `cfg.LinuxFamily` unchanged. GPU detection mutates `cfg.GPU` directly. The commit summary says this is intentional: "Auto-detect GPU by mutating cfg.GPU so the value propagates to generateDependencyPlans() and depCfg without additional plumbing."

The pragmatic trick works -- `cfg.GPU` is populated early, and because `depCfg` copies `cfg.GPU`, dependencies inherit it. But the next developer will see two auto-detection blocks side by side using opposite patterns and won't know if the difference is intentional. They might "fix" the LinuxFamily pattern to match GPU (mutation), or "fix" the GPU pattern to match LinuxFamily (local variable), breaking propagation in either case.

The comments on lines 101-103 ("Auto-detect GPU vendor when not provided") don't signal that mutation is the mechanism for downstream propagation. A developer reading `depCfg` at line 757 would see `GPU: cfg.GPU` and assume `cfg` was the caller's original config, not that `GeneratePlan` silently populated it.

**Suggestion**: Either (a) make both use the same pattern -- mutate `cfg.LinuxFamily` too and use it consistently downstream (removes the `linuxFamily` local), or (b) keep the local variable pattern for both and explicitly set `GPU` in `depCfg` from the local. If keeping the mutation, add a comment like `// Mutating cfg.GPU so it propagates to depCfg in generateDependencyPlans()`.

### 2. ADVISORY: `shouldExecute()` calls `platform.DetectGPU()` on every invocation

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/executor.go:133`

```go
target := recipe.NewMatchTarget(runtime.GOOS, runtime.GOARCH, "", "", platform.DetectGPU())
```

`shouldExecute()` is called once per step during `DryRun()` and `Execute()` flows. `DetectGPU()` reads sysfs files on Linux every time. For a recipe with 10 steps, that's 10 filesystem scans. This isn't a correctness issue -- the result is stable across calls -- but it's a performance trap the next developer might not expect when they see a simple `NewMatchTarget` call.

The plan-generation path (`GeneratePlan`) detects once and reuses. The execution path (`shouldExecute`) detects per-step. The next developer might think the execution path already has a cached value and won't realize it re-scans.

**Suggestion**: Cache the result in the `Executor` struct, or detect once at the start of `Execute()`/`DryRun()` and pass it through. Not blocking because sysfs reads are fast (microsecond-scale) and `shouldExecute` is called a small number of times.

### 3. ADVISORY: `shouldExecute()` still passes empty strings for `linuxFamily` and `libc`

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/executor.go:133`

```go
target := recipe.NewMatchTarget(runtime.GOOS, runtime.GOARCH, "", "", platform.DetectGPU())
```

GPU was added (fixing the #1773 advisory), but `linuxFamily` and `libc` remain empty. This means `shouldExecute()` won't filter steps with `when = { linux_family = "debian" }` or `when = { libc = ["glibc"] }` conditions at runtime. Those steps would match regardless of the actual family.

This predates #1775 and isn't a regression. The issue description says "thread GPU through plan generation", not "fix all shouldExecute gaps." But since the code was touched and GPU was added, the remaining empty parameters are conspicuous. A comment would prevent the next person from thinking the family/libc gaps are bugs they need to fix (they're acceptable because runtime execution targets the current host and implicit constraints handle family filtering).

### 4. ADVISORY: Test names for GPU-related tests don't verify the filtered step's params

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/filter_test.go:267-343`

`TestFilterStepsByTarget_GPU` checks `wantLen` and `wantActs` but `wantActs` for all GPU cases is `[]string{"chmod", "install_binaries"}`. Since three GPU-filtered steps all use `chmod`, the test can't distinguish which `chmod` was selected. For example, the "nvidia GPU selects nvidia step plus unconditional" case would also pass if the "amd" step leaked through instead of the "nvidia" step.

The parallel test in `plan_generator_test.go:1901` (`TestGeneratePlan_GPUFiltering`) does verify the correct step was selected by checking the `path` parameter value. That coverage is solid. But the `filter_test.go` test gives a false sense of specificity -- the test name promises GPU filtering validation that the assertions don't deliver.

**Suggestion**: Add a `wantParams` check to `TestFilterStepsByTarget_GPU` to verify the correct chmod step was selected (e.g., checking the "path" parameter is "cuda-binary" for nvidia), or add a comment noting that param-level verification is covered by `TestGeneratePlan_GPUFiltering`.

### 5. ADVISORY: `TestGeneratePlan_DepCfgLinuxFamilyPropagation` name suggests it tests a bug fix from this issue

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/plan_generator_test.go:2206`

```go
func TestGeneratePlan_DepCfgLinuxFamilyPropagation(t *testing.T) {
    // Verify the pre-existing bug fix: LinuxFamily is now propagated through depCfg.
```

The comment says "pre-existing bug fix" which is accurate (the LinuxFamily gap was flagged in #1773's review). But the test name doesn't make it clear this was a bonus fix in #1775. A next developer looking at this test might think LinuxFamily propagation was always there. The comment helps, but since this test was introduced to verify a fix, naming it `TestGeneratePlan_DepCfgLinuxFamilyPropagationFix` or adding the issue number would make the intent clearer. Minor point.

## Overall Assessment

The implementation achieves its goal: GPU flows from auto-detection through `PlanConfig`, into target construction, through dependency plan generation via `depCfg`, and into the runtime `shouldExecute()` path. The test coverage is strong, particularly `TestGeneratePlan_GPUPropagationThroughDependencies` which validates the critical dependency chain that was the blocking finding from #1773's review.

The one blocking finding is the inconsistent mutation pattern between LinuxFamily (local variable) and GPU (`cfg.GPU` mutation). The trick works today, but the divergent patterns are a maintenance trap. The fix is a one-line comment explaining why GPU mutates `cfg` directly, or a small refactor to make both patterns consistent.

The `shouldExecute()` change correctly adds GPU detection for the runtime path. The per-call detection and empty linuxFamily/libc are pre-existing patterns that don't regress correctness.

The LinuxFamily propagation fix (`cfg.LinuxFamily` in `depCfg`) was a good bonus fix that resolves the pre-existing gap flagged in #1773's review.
