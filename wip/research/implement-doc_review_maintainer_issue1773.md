# Maintainer Review: #1773 GPU Vendor Detection via PCI sysfs

## Review Scope

Issue #1773 adds GPU vendor detection to the platform package, extends `Target` and `MatchTarget` with a `gpu` field, adds `GPU()` to the `Matchable` interface, and updates ~80 constructor callsites.

## Findings

### 1. BLOCKING: `depCfg` in plan_generator.go does not propagate GPU

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/plan_generator.go:747-759`

The `depCfg` struct that configures dependency plan generation copies `OS`, `Arch`, `OnWarning`, `Downloader`, `DownloadCache`, `AutoAcceptEvalDeps`, `OnEvalDepsNeeded`, `Constraints`, and `PinnedVersion` from the parent config -- but omits `GPU` and `LinuxFamily`.

```go
depCfg := PlanConfig{
    OS:                 cfg.OS,
    Arch:               cfg.Arch,
    RecipeSource:       "dependency",
    OnWarning:          cfg.OnWarning,
    Downloader:         cfg.Downloader,
    DownloadCache:      cfg.DownloadCache,
    AutoAcceptEvalDeps: cfg.AutoAcceptEvalDeps,
    OnEvalDepsNeeded:   cfg.OnEvalDepsNeeded,
    RecipeLoader:       nil,
    Constraints:        cfg.Constraints,
    PinnedVersion:      pinnedVersion,
    // GPU is missing
}
```

This matters for #1773's downstream consumers. When the tsuku-llm recipe (issue #1776) has a CUDA step with `dependencies = ["cuda-runtime"]`, the dependency plan for cuda-runtime will be generated with `GPU: ""`. If cuda-runtime ever adds GPU-filtered steps, they won't match. More immediately, the next developer reading `depCfg` will assume it inherits all target dimensions because it explicitly copies `OS` and `Arch` -- the missing `GPU` (and `LinuxFamily`) looks like an accidental omission, not an intentional choice.

Note: `LinuxFamily` is also missing from `depCfg`, which predates this issue. But since GPU was just added and this is the code path that will matter for GPU dependency recipes, it should be included now.

**Suggestion**: Add `GPU: cfg.GPU` and `LinuxFamily: cfg.LinuxFamily` to the `depCfg` literal.

### 2. ADVISORY: `executor.shouldExecute()` passes empty GPU to `NewMatchTarget`

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/executor.go:132`

```go
target := recipe.NewMatchTarget(runtime.GOOS, runtime.GOARCH, "", "", "")
```

The `shouldExecute` method constructs a `MatchTarget` with empty `linuxFamily`, `libc`, and `gpu`. This is the runtime execution path (not plan generation), so it only needs to match platform conditions. The empty GPU is currently fine because `WhenClause.Matches()` doesn't check GPU yet (that's #1774). But when #1774 adds GPU matching to `WhenClause`, this line will silently fail to match any GPU-filtered step at execution time.

This isn't blocking for #1773 since the GPU field on `WhenClause` doesn't exist yet. But the next person implementing #1774 needs to know about this callsite. A `// TODO(#1774): populate gpu when WhenClause gains GPU matching` comment would prevent the misread.

### 3. ADVISORY: `ValidGPUTypes` in gpu.go is disconnected from `pciVendorToGPU` in gpu_linux.go

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/platform/gpu.go:10` and `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/platform/gpu_linux.go:16-20`

`ValidGPUTypes` lists `["nvidia", "amd", "intel", "apple", "none"]`. The `pciVendorToGPU` map contains `{"0x10de": "nvidia", "0x1002": "amd", "0x8086": "intel"}`. If someone adds a new GPU vendor to `pciVendorToGPU` but forgets to update `ValidGPUTypes`, the detection will return a value that downstream code considers invalid.

The `TestDetectGPU` test validates that `DetectGPU()` returns a value in `ValidGPUTypes`, which would catch the discrepancy on the developer's machine -- but only if that machine has the new GPU. There's no static check that all values in `pciVendorToGPU` appear in `ValidGPUTypes`.

This is a low-risk advisory finding because adding a new GPU vendor is uncommon and the test provides some coverage. A comment on `ValidGPUTypes` noting "keep in sync with pciVendorToGPU values and platform-specific DetectGPU returns" would help.

### 4. ADVISORY: `SetGPU` name is slightly misleading for a value-receiver copy

**File**: `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/platform/target.go:56-59`

```go
func (t Target) SetGPU(gpu string) Target {
    t.gpu = gpu
    return t
}
```

The `Set` prefix in Go conventionally implies mutation. This method returns a copy, which the test at `gpu_test.go:139-154` verifies. The method is fine for its purpose (used in tests that don't care about GPU). But a next developer scanning for mutation bugs might assume `SetGPU` mutates the receiver and miss the return value.

The existing test (`TestTarget_SetGPU`) thoroughly documents the copy semantics, and the value receiver makes it clear to experienced Go developers. This is advisory only. `WithGPU` would be more idiomatic for a copy-returning method, following the `WithVerify` pattern already in `types.go:1038`, but changing it isn't worth the churn if there are already callsites.

### 5. ADVISORY: Several production `PlanConfig{}` callsites don't set GPU

**Files**:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/cmd/tsuku/helpers.go:118`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/cmd/tsuku/install_deps.go:122`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/cmd/tsuku/install_lib.go:111`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/builders/orchestrator.go:297`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/validate/executor.go:188`
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/validate/source_build.go:104`

None of these production `PlanConfig` callsites pass `GPU`. This is technically correct for #1773 because `WhenClause` doesn't check GPU yet -- that's #1775's responsibility ("thread GPU through plan generation"). But the `PlanConfig.GPU` field already exists and has a doc comment saying "Used for GPU-aware recipe step filtering", which creates the impression it's wired up. A next developer might wonder why GPU filtering doesn't work.

This is advisory because #1775 explicitly tracks wiring GPU into these callsites. The issue dependency chain is correct. But the `PlanConfig.GPU` doc comment could note that it takes effect when combined with #1774/#1775 work, to set expectations.

## Overall Assessment

The implementation is clean and follows the established pattern closely. GPU detection mirrors the libc detection approach: platform-specific files, sysfs reads with no subprocess spawning, a root parameter for testing, and mock filesystem fixtures. The constructor cascade was handled systematically across ~80 callsites with empty string for GPU where detection isn't needed.

The one blocking finding is the missing `GPU` propagation in `depCfg`. The dependency plan generation path is the exact code path that GPU runtime recipes (cuda-runtime, vulkan-loader) will exercise, and the omission will cause dependencies to lose GPU awareness. It's a one-line fix.

Test coverage is solid: single-vendor, multi-vendor priority, no-GPU, nonexistent root, `SetGPU` copy semantics, and `isDisplayController` edge cases. The `TestDetectGPU` test that validates against `ValidGPUTypes` on the real system is a nice touch for catching regressions on CI machines with GPUs.
