# Architecture Review: Tool Lifecycle Hooks Design

## Reviewer Role

Architect reviewer -- evaluating structural fit with the existing codebase.

## Summary Assessment

The design is well-structured and architecturally sound. It extends existing patterns (action registry, step array, state struct) rather than introducing parallel ones. Three structural issues need resolution before implementation, one of which is blocking.

---

## 1. Is the architecture clear enough to implement?

**Mostly yes**, with one significant gap.

### Blocking: Phase field exists on recipe Step but not on ResolvedStep

The design adds `Phase` to `recipe.Step` (the TOML-level struct) but doesn't address `executor.ResolvedStep` (the plan-level struct). The plan generator (`GeneratePlan`) converts `recipe.Step` into `executor.ResolvedStep`, which is what `ExecutePlan` actually iterates. `ResolvedStep` currently has no `Phase` field.

This creates an implementation ambiguity: should `ExecutePhase` filter at the recipe step level (before plan generation) or at the resolved step level (during plan execution)? The design says the executor gains `ExecutePhase`, but the executor works with `InstallationPlan.Steps` (which are `ResolvedStep`), not `recipe.Steps` directly.

**Options:**
- (a) Add `Phase` to `ResolvedStep` and serialize it in the plan JSON. This keeps the plan self-contained (plans can be executed without the recipe).
- (b) Filter steps by phase during plan generation, producing separate plans per phase. This aligns with the existing model where plans are phase-specific, but means the plan format implicitly represents "install" phase only.
- (c) Generate a single plan with phase-tagged resolved steps. The executor filters at execution time.

Recommendation: Option (a) is the most consistent with the plan-as-contract pattern. The plan already contains everything needed for execution; adding phase maintains that property. Option (b) would work but creates an implicit coupling between plan generation and phase semantics.

### Advisory: ExecutePlan/ExecutePhase relationship underspecified

The design says "the existing `ExecutePlan` method becomes a wrapper that calls `ExecutePhase("install")`." But `ExecutePlan` currently handles plan validation, platform checks, resource limits, dependency installation, and step execution. `ExecutePhase` needs to either (a) accept a pre-validated plan and skip those checks, or (b) be a lower-level method that only does step filtering and execution within the existing `ExecutePlan` flow.

Option (b) is simpler and avoids duplicating validation logic. The design should clarify that `ExecutePhase` is an internal method called within `ExecutePlan`, not a replacement for it.

---

## 2. Are there missing components or interfaces?

### Advisory: install_shell_init runs the installed binary -- needs ExecutionContext access to tool install path

The `install_shell_init` action with `source_command` needs to invoke the just-installed binary (e.g., `niwa shell-init bash`). During post-install, the binary is in the install directory but may not yet be symlinked to `$TSUKU_HOME/bin`. The action needs the tool's install path on `ExecutionContext`.

`ExecutionContext` already has `InstallDir` and `ToolInstallDir`, but `ToolInstallDir` is set to `""` in `ExecutePlan`. The implementation needs to set this correctly before running post-install phase steps. The design should note this wiring requirement.

### Advisory: RebuildShellCache caller coordination

The design says cache rebuild is called by install, remove, and update. Currently these flows are in different packages (`cmd/tsuku/` for install/update orchestration, `internal/install/` for state/removal). The `RebuildShellCache` function needs to be callable from both. The proposed location (`internal/shellenv/cache.go`) is correct for this -- it's a lower-level package accessible to both callers. No structural issue, but the wiring should be explicit in the implementation phases.

### Advisory: No mention of plan generation changes for post-install steps

`GeneratePlan` currently iterates all recipe steps and resolves them into `ResolvedStep`. Post-install steps with `source_command` are not evaluable (their output is dynamic). The plan generator needs to either include them with `Evaluable: false` or exclude them entirely and handle them outside the plan flow. The design should specify which.

---

## 3. Are the implementation phases correctly sequenced?

**Phase sequencing has a dependency inversion between Phase 2 and Phase 3.**

Phase 2 (shell.d and shellenv) says it wires cache rebuild into the install flow. But install doesn't know which files were created until post-install actions run and return `CleanupAction` entries -- which is Phase 3's deliverable. In Phase 2, the install flow would need to hardcode which shells to rebuild, or rebuild all shells unconditionally.

Recommended fix: merge Phase 2 and Phase 3, or resequence so that `CleanupAction` recording (Phase 3) comes before the shellenv integration (Phase 2). The state contract for cleanup actions is a prerequisite for knowing *which* shells to rebuild.

Alternatively, accept that Phase 2 rebuilds all shell caches unconditionally (cheap operation) and Phase 3 makes it targeted. This is pragmatic and avoids resequencing.

---

## 4. Are there simpler alternatives we overlooked?

### The design is already at the right level of complexity for the problem.

The three-mechanism approach (phase field, shell.d cache, state-tracked cleanup) is justified by the constraints: recipes need lifecycle declaration, shells need fast init, removal needs offline cleanup. Removing any one mechanism would leave a gap.

### One simplification worth considering: defer `source_command`, ship `source_file` only in Phase 1

The security section already suggests this. `source_file` is simpler (no binary execution, no output validation, no need to resolve tool install paths at runtime). It covers tools that ship pre-built init scripts in their archives. `source_command` adds significant complexity (binary path resolution, output scanning, security validation). Deferring it to a later phase reduces Phase 1 scope without losing the most common use case.

However, checking the niwa example: niwa generates init via `niwa shell-init {shell}`, which requires `source_command`. If niwa is the primary motivating consumer, `source_file` alone won't satisfy the MVP. The design should clarify whether niwa can ship static init scripts in its archive as a workaround for Phase 1.

### Alternative not in the design: skip the cache, source shell.d files directly via a glob

The design rejected "direct sourcing without tsuku mediation" for good reasons (shell syntax variation, no ordering control). But there's a middle option: `tsuku shellenv` could emit a loop that sources each file, rather than a single cached file:

```bash
for f in "$TSUKU_HOME/share/shell.d/"*.bash; do [ -f "$f" ] && . "$f"; done
```

This eliminates the cache rebuild mechanism entirely. Startup cost: one glob + N file reads vs. one file read. For 5-15 tools, the difference is <2ms. The cache adds complexity (rebuild triggers, staleness detection, atomic writes, hash verification) for a marginal performance gain.

This is worth evaluating. If the startup budget allows it, removing the cache simplifies the design substantially and eliminates a class of bugs (stale cache, concurrent rebuild races). The hash verification in the security section becomes unnecessary since there's no single cache file to tamper with.

---

## Structural Fit Assessment

| Concern | Verdict | Details |
|---------|---------|---------|
| Action dispatch | Clean | `install_shell_init` and `install_completions` register via `actions.Register()` like all other actions |
| Recipe schema | Clean | `Phase` on `Step` is a natural extension; backward-compatible via zero-value default |
| State contract | Clean | `CleanupActions` on `VersionState` has clear consumers (remove flow, update flow) |
| CLI surface | Clean | `shellenv` output is additive; no new subcommands |
| Dependency direction | Clean | `internal/shellenv/cache.go` is a new leaf package with no upward imports |
| Parallel patterns | None detected | Design extends existing patterns throughout |

### Blocking Finding

**ResolvedStep lacks Phase field.** The design proposes `ExecutePhase` on the executor, but the executor operates on `InstallationPlan.Steps` (type `[]ResolvedStep`), not `recipe.Steps`. `ResolvedStep` has no `Phase` field. Without resolving this, implementers will either add a parallel step-filtering path that bypasses the plan system, or add the field ad hoc without considering plan serialization implications. Specify whether phase belongs on `ResolvedStep` and how it flows through plan generation.

### Advisory Findings

1. **Phase 2/3 sequencing**: Cache rebuild in Phase 2 depends on cleanup tracking from Phase 3. Either merge them or accept unconditional rebuilds in Phase 2.
2. **ToolInstallDir wiring**: `ExecutionContext.ToolInstallDir` is empty during plan execution. Post-install `source_command` needs it to find the installed binary.
3. **Plan generator unspecified for post-install steps**: Should post-install steps appear in the generated plan? If yes, with what evaluability marking?
4. **Cache vs. glob trade-off**: The shell.d cache adds rebuild/staleness/hash complexity for marginal startup gains. A glob-source loop in shellenv output may be sufficient for the expected tool count (5-15).
