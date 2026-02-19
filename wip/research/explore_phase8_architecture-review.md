# Architecture Review: DESIGN-gpu-backend-selection.md (Revised Design)

**Reviewer**: Architect Reviewer
**Date**: 2026-02-19
**Design Status**: Proposed (revised -- platform extension + addon-to-recipe conversion)

## Summary

The revised design extends the platform detection system with GPU vendor identification and converts tsuku-llm from an addon-with-embedded-manifest into a standard recipe. Both directions are architecturally sound and follow established patterns. The platform extension mirrors the libc detection pattern closely. The addon-to-recipe migration eliminates the only non-recipe binary distribution path in tsuku.

Six findings total: two blocking, four advisory.

---

## Finding 1: `NewTarget` and `NewMatchTarget` signature change cascade is under-scoped

**Severity: Blocking**

Adding a `gpu` field to `platform.Target` requires changing the constructor from `NewTarget(platform, linuxFamily, libc string)` to `NewTarget(platform, linuxFamily, libc, gpu string)`. The design acknowledges this at line 584 ("any code that constructs `MatchTarget` or `Target` must be updated") but the implementation phases don't enumerate the affected callsites.

Current callsite count from the codebase:

**`platform.NewTarget()`** -- approximately 49 callsites:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/plan_generator.go:111` -- primary plan generation path
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/plan_generator.go:668` -- dependency plan generation path
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/cmd/tsuku/sysdeps.go:210,212` -- CLI system deps command
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/cmd/tsuku/info.go:447` -- CLI info command
- 20+ test files across `internal/executor/filter_test.go`, `internal/sandbox/`, `internal/verify/`, `internal/actions/system_action_test.go`

**`recipe.NewMatchTarget()`** -- approximately 27 callsites:
- `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/executor.go:132` -- runtime execution path
- Test files across `internal/recipe/`, `internal/executor/`, `internal/actions/`

The two plan generator callsites are the critical production paths. They construct `platform.Target` using `platform.NewTarget()` and pass it to `WhenClause.Matches()` via the `Matchable` interface. If the `gpu` field isn't threaded through here, GPU filtering silently does nothing (GPU value is `""`, no `when.gpu` matches fire).

The `PlanConfig` struct at `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/plan_generator.go:17-62` currently has `OS`, `Arch`, and `LinuxFamily` override fields. It needs a `GPU` field (auto-detect if empty, like `LinuxFamily`). The design's Phase 1 step 10 says "Update plan generator to pass GPU through `PlanConfig`" but doesn't address:
- The constructor signature change and its 76-callsite cascade
- The `executor.go:132` execution-time `NewMatchTarget` call
- The `FilterStepsByTarget` path in `executor/filter.go` that passes `platform.Target` to `WhenClause.Matches()`

**Recommendation**: Phase 1 should explicitly list every production-path callsite that needs GPU threading. Test callsites can pass `""` but must be updated for compilation. This is the largest mechanical change in the design and underestimating it risks a partial implementation that compiles but doesn't actually filter on GPU.

---

## Finding 2: `llm.backend` override creates step matching ambiguity for CUDA

**Severity: Blocking**

The design proposes that `installViaRecipe()` overrides the target's GPU value before plan generation (lines 411-418). The override table shows `llm.backend = cuda` maps to target GPU `"nvidia"`. But the recipe (lines 197-204) maps all GPU vendors to the Vulkan variant:

```toml
# Vulkan step
when = { os = ["linux"], arch = "amd64", gpu = ["nvidia", "amd", "intel"] }
asset_pattern = "tsuku-llm-v{version}-linux-amd64-vulkan"
```

If a CUDA step is added (lines 457-465):

```toml
# CUDA step
when = { os = ["linux"], arch = "amd64", gpu = ["nvidia"] }
asset_pattern = "tsuku-llm-v{version}-linux-amd64-cuda"
```

Both steps match when `GPU() == "nvidia"`. The recipe system's `WhenClause` matching is AND-based with all-matching-steps-execute semantics. Looking at `GeneratePlan()` in `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/plan_generator.go:178-208`, every step that passes both implicit constraint and explicit `when` clause filtering is included in the plan. There is no mutual exclusivity mechanism -- both `github_file` steps would execute, downloading both Vulkan and CUDA binaries.

The design acknowledges this at line 577 ("creating potential matching ambiguity") but doesn't resolve it.

Three options:

1. **Keep CUDA out of the recipe.** When `llm.backend = cuda`, the LLM code patches the asset pattern before plan generation rather than relying on when-clause matching. This keeps the recipe clean (no ambiguous steps) and the override in LLM-specific code.

2. **Use separate recipes** (e.g., `tsuku-llm-cuda`). The override resolves the recipe name, not the target GPU.

3. **Add a backend dimension** to `WhenClause`. This generalizes the problem but introduces a new matching concept for a single consumer.

Option 1 is most consistent with the design's stated philosophy of keeping the `llm.backend` override in LLM-specific code. But "modify the target GPU" (the current proposal) is structurally different from "modify the recipe steps" -- the implementation path changes.

**Recommendation**: Resolve before implementation. The recipe as shown (all GPU vendors -> Vulkan, no-GPU -> CPU) works without ambiguity for the default path. Only the CUDA override creates the problem. If CUDA override is deferred to a follow-up, this finding becomes moot for initial implementation.

---

## Finding 3: `WhenClause` serialization and deserialization need GPU support

**Severity: Advisory**

The design shows `WhenClause.Matches()` extension and mentions `IsEmpty()` in Phase 1 step 8. But three additional methods in `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/types.go` also need updates:

- `IsEmpty()` (line 248): must check `len(w.GPU) == 0`
- `UnmarshalTOML()` (line 367): needs GPU array parsing block following the libc pattern (lines 431-444)
- `ToMap()` (line 489): needs GPU serialization block following the libc pattern (lines 519-521)

Phase 1 step 8 says "update `Matches()`, `IsEmpty()`, and TOML unmarshaling" which covers two of three. `ToMap()` isn't mentioned. The `MergeWhenClause()` function at line 584 also needs consideration -- it merges implicit constraints with explicit when clauses, and `Constraint` at line 549 doesn't have a GPU field. The design's Phase 1 step 9 says "Update `MergeWhenClause()` for GPU constraint checking" but `recipe.Constraint` (distinct from `actions.Constraint`) would need a GPU field for this to work, which is a deeper change than the design implies.

This is advisory because the libc pattern is clear precedent and an implementer following it will discover these naturally.

---

## Finding 4: Addon package dependency direction needs care

**Severity: Advisory**

The design proposes that `EnsureAddon()` calls `installViaRecipe()`, which "reuses the existing executor pipeline" (line 409). Currently, `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/llm/addon/` has no dependency on `internal/executor/` or `internal/recipe/`. The dependency flows through `cmd/tsuku/` which orchestrates both.

If `installViaRecipe()` is implemented by having `addon/manager.go` import `internal/executor/`, that creates a new dependency from a specialized package to core infrastructure. Not inherently wrong, but it increases coupling.

The design's Phase 2 step 3 mentions adding `LLMBackend() string` to the `LLMConfig` interface in `factory.go`. Similarly, the `AddonManager` should accept an `Installer` interface (e.g., `Install(ctx context.Context, recipeName string) error`) injected at construction time, rather than importing the executor directly. The CLI layer provides the concrete implementation.

This keeps `addon/`'s dependency set narrow and follows the existing pattern where `cmd/tsuku/` is the composition root.

---

## Finding 5: `DetectTarget()` restructuring for non-Linux paths

**Severity: Advisory**

The design's `DetectTarget()` sketch (lines 324-336) correctly calls `DetectGPU()` unconditionally and routes non-Linux through `NewTarget(platform, "", "", gpu)`. But the current code at `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/platform/family.go:125-126` bypasses `NewTarget()` entirely on non-Linux:

```go
if runtime.GOOS != "linux" {
    return Target{Platform: platform}, nil
}
```

This direct struct literal construction means GPU won't flow into the target on macOS unless this early return is restructured to use `NewTarget()`. The design shows the correct restructured code, so this isn't a gap in intent. Worth noting because the restructuring changes the non-Linux path from a simple struct literal to a `NewTarget()` call, and the macOS test coverage for this path should be verified.

---

## Finding 6: PCI sysfs edge cases

**Severity: Advisory**

The design's detection approach is solid. Edge cases to address during implementation:

- **Containers without PCI passthrough**: `/sys/bus/pci/devices/` may be empty or absent. `filepath.Glob` returns nil on no matches, so `DetectGPU()` naturally returns `"none"`. No special handling needed, but the function's doc comment should mention this.

- **Multiple discrete GPUs** (e.g., NVIDIA + AMD in a workstation): The design's priority order (NVIDIA > AMD > Intel) is reasonable for LLM use. Should be documented as a deliberate choice, not an implementation detail, since it affects which binary users get.

- **Class code coverage**: The design lists `0x0300xx` (VGA) and `0x0302xx` (3D controller). NVIDIA Tesla/datacenter GPUs use `0x0302`. Some compute accelerators use `0x0380xx` (other display controller). The initial implementation should cover `0x0300` and `0x0302`; `0x0380` can be added if needed.

- **Permissions**: Confirmed that `/sys/bus/pci/devices/*/class` and `*/vendor` are world-readable (`0444`) on all major distributions. No privilege escalation needed.

---

## Structural Assessment

The revised design makes two architecturally sound decisions:

1. **GPU detection follows the libc pattern exactly.** Same detection-at-target-time flow, same `Matchable` interface extension, same `WhenClause` field addition, same plan generator threading. No parallel mechanisms introduced. The code at `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/platform/libc.go` establishes the pattern and `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/recipe/types.go:302-315` shows how `WhenClause.Matches()` consumes it. The GPU extension is a direct extension of both.

2. **Addon-to-recipe conversion eliminates divergence.** The addon's embedded manifest (`manifest.go`), custom platform keys (`platform.go:11` -- `GOOS + "-" + GOARCH` vs the recipe system's `GOOS + "/" + GOARCH`), download code (`download.go`), and verification code (`verify.go`) are a parallel distribution path. Removing them in favor of the recipe system reduces the number of binary distribution paths from 2 to 1.

The two blocking findings are implementation-level design gaps, not architectural direction problems. Finding 1 (constructor cascade) is mechanical but must be planned explicitly because of the 76-callsite impact. Finding 2 (CUDA override ambiguity) is a design tension that the simplest fix is to defer the CUDA step to a follow-up, making the initial recipe unambiguous.

The overall direction is correct. The platform extension is the right place for GPU awareness, the recipe system is the right distribution mechanism, and the addon lifecycle code correctly stays separate since daemon management has no recipe equivalent.
