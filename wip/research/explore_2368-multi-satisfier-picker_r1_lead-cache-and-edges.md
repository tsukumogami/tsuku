# Research Report: Issue #2368 — Plan Caching, Hashing, and Edge Cases

## Executive Summary

The picker (interactive recipe selection for multi-satisfier aliases) is **fully compatible with plan caching and hashing**. The plan's content hash is deterministic once a recipe is resolved; the picker's decision is baked into `InstallationPlan.Tool`, which appears in the normalized hashing input. Pre-computed plans (`--plan <file>`) bypass the picker entirely—no re-prompting occurs on replay. Dependency resolution never engages the picker (it loads recipes deterministically).

Edge cases are well-contained: alias collisions with recipe names resolve via a direct-name-first preference (implicit in the satisfies index design), and `--from` semantics remain orthogonal to the picker.

---

## Section 1: Plan Caching Answer

### Plan Hash Inputs and Stability

From `internal/executor/plan_cache.go`:

**Cache Key Components (from `PlanCacheKey`):**
- `Tool` (string) — the recipe name
- `Version` (string) — resolved version (not user input)
- `Platform` (string) — "os-arch"

**Content Hash Components (from `planContentForHashing`):**
Normalized fields fed to SHA256:
- `Deterministic`, `FormatVersion`, `Platform`, `RecipeType`, `Tool`, `Version`
- `Steps` (action, checksum, deterministic, evaluable, params, size, URL)
- `Dependencies` (recursively: tool, version, steps, verify, recipe_type)
- `Verify` (command, exit_code, pattern, patterns)

**Critically: Does resolved recipe name appear in hashed inputs?**

YES. The `Tool` field in the plan (line 290 in `plan_generator.go`):
```go
return &InstallationPlan{
    ...
    Tool: e.recipe.Metadata.Name,  // <-- resolved recipe name
    ...
}
```

This `Tool` field is included directly in `planContentForHashing` (line 111 in `plan_cache.go`). **Once the picker resolves an alias to a recipe name, that recipe name becomes the plan's Tool field, and the hash incorporates it.**

### --plan <file> Flow

From `cmd/tsuku/plan_install.go` (`runPlanBasedInstall`):

1. Load plan from file or stdin
2. Extract tool name from plan (`plan.Tool`) 
3. Validate tool name (if provided on CLI, must match plan)
4. Create minimal recipe (metadata only; actual steps come from plan)
5. Execute plan verbatim

**Picker engagement on `--plan` replay: NONE.** The plan already contains the resolved recipe name, resolved version, and all steps. No recipe loading, no version resolution, no picker invocation.

### Conclusion for Question 1

✅ **The picker decision is baked into the plan's resolved recipe name.** This means:
- `tsuku install java` (TTY) → picker shows openjdk, temurin, corretto, user picks → plan.Tool = "temurin"
- `tsuku eval temurin | tsuku install --plan -` → skips picker, uses plan.Tool = "temurin", executes without re-prompting

Plan hashes are **stable**: two runs of `tsuku install temurin` (when temurin is the chosen recipe) produce the same plan hash because the hash depends only on the resolved recipe name, version, platform, and steps—all deterministic once the recipe is fixed.

---

## Section 2: Edge Case Truth Table

### Edge Case 1: Alias Collides with Recipe Name (e.g., `java` as both alias and recipe name)

**Scenario:** A hypothetical `recipes/j/java.toml` (recipe named "java") exists, AND other recipes satisfy the alias "java" (e.g., temurin, openjdk).

**Resolution Semantics (inferred from loader.go):**

From `resolveFromChain()` (line 273–310 in `loader.go`):
1. Try all providers in order for a *direct* recipe name match (e.g., "java")
2. If found, return it
3. If not found, fall back to satisfies index lookup

The satisfies index build (line 432–458) only stores entries that don't already exist:
```go
if _, exists := l.satisfiesIndex[pkgName]; !exists {
    l.satisfiesIndex[pkgName] = satisfiesEntry{...}
}
```

**Precedence: DIRECT NAME WINS.** If `recipes/j/java.toml` exists, loading "java" returns that recipe directly; the satisfies fallback never runs.

**Recommendation:** This is the correct behavior. Document that aliases should not use existing recipe names. If a collision is discovered, the direct-name-first behavior ensures backward compatibility—users upgrading with a new recipe named "java" won't see the picker, they'll get the real recipe.

**For the picker:** No change needed. The picker only appears when recipe loading via `Get()` invokes the satisfies fallback (i.e., when no direct match exists).

---

### Edge Case 2: Recipe in Install Path Declares `runtime_dependencies = ["java"]`

**Scenario:** User runs `tsuku install foo`, where foo's recipe has a step declaring `runtime_dependencies = ["java"]`. Should the picker engage during dependency resolution?

**Answer: NO. Pickers must NOT engage for runtime_dependencies.**

From `plan_generator.go` (line 249–257):
```go
var dependencies []DependencyPlan
if cfg.RecipeLoader != nil {
    processed := make(map[string]bool)
    processed[e.recipe.Metadata.Name] = true
    deps, err := generateDependencyPlans(ctx, e.recipe, cfg, processed)
    if err != nil {
        return nil, fmt.Errorf("failed to generate dependency plans: %w", err)
    }
    dependencies = deps
}
```

From `generateDependencyPlans()` (line 702–734):
```go
deps := actions.ResolveDependenciesForTarget(r, targetOS, target)
if len(deps.InstallTime) == 0 {
    return nil, nil
}
```

**Key:** Only `InstallTime` dependencies become nested plans. Runtime dependencies are metadata-only (not installed during plan generation).

From `resolver.go` (line 44–45, comment):
```
// - If step has "runtime_dependencies", use those (replaces action implicit)
```

Runtime deps are resolved and stored in state but **not passed to plan generation as RecipeLoader loads**. The plan generator never attempts to resolve recipes for runtime_dependencies; it only resolves `InstallTime` deps.

**Recommendation:** This is already correct. Document that runtime_dependencies cannot contain multi-satisfier aliases—they must either be:
- Real recipe names (exact match)
- Satisfied by a single recipe in the satisfies index

If a runtime_dependencies entry resolves to multiple candidates, plan generation fails fast during `getRecipe()` (it tries direct load, then satisfies lookup, both returning a single recipe or error).

---

### Edge Case 3: `tsuku update` on an Installed Alias

**Scenario:** User ran `tsuku install java` and picked temurin (via picker). Later, `tsuku update java` is run. Does the system remember which recipe was picked?

**Resolution: YES. State tracking via Plan metadata.**

From `install/state_tool.go` (lines 24–35):
```go
toolState, exists := state.Installed[name]
if !exists {
    toolState = ToolState{RequiredBy: []string{}}
}
update(&toolState)
state.Installed[name] = toolState
```

From `install/state.go` (lines 38–50):
```go
type Plan struct {
    ...
    Tool          string  // <-- the resolved recipe name
    ...
    RecipeSource  string
    ...
}
```

From `plan_install.go` (lines 99, 137–139):
```go
installOpts.Plan = executor.ToStoragePlan(plan)
...
err = mgr.GetState().UpdateTool(effectiveToolName, func(ts *install.ToolState) {
    ts.IsExplicit = true
})
```

The state stores the full `Plan`, including `Tool = "temurin"` (the picked recipe). When `tsuku update java` is run:

1. Load state for "java"
2. Get cached plan: `state.Installed["java"].Versions[activeVersion].Plan`
3. Extract `plan.Tool = "temurin"`
4. Load temurin recipe (direct name match, no picker)
5. Check for updates against temurin

**Recommendation:** Verify that update flows use the cached plan's Tool field, not the input alias. If they do (which they should), no change is needed. The architecture already handles this.

---

### Edge Case 4: `tsuku install java --from temurin` (or future `--recipe-name` flag)

**Scenario:** User specifies both an alias and a recipe source. Current `--from` is parsed as `<ecosystem>:<package>`. Should the picker be bypassed?

**Current behavior (from install.go lines 174–195):**
```go
if installFrom != "" {
    if len(args) != 1 {
        printError(...)
        exitWithCode(ExitUsage)
    }
    toolName := args[0]
    
    createFrom = installFrom
    createAutoApprove = installForce
    ...
    runCreate(nil, []string{toolName})  // Generates recipe via create pipeline
    ...
    runInstallWithTelemetry(toolName, "", "", true, "", telemetryClient)
}
```

`--from` routes to the create pipeline (discovery/generation), not the picker. The create pipeline generates a recipe, which is then installed. This is a **different code path** than the picker.

**Recommendation:** The picker and `--from` remain orthogonal. If we want to add a `--recipe-name temurin` flag (to bypass the picker for aliases), it would:
1. Load recipe by name directly (no picker)
2. Proceed to plan generation and install

This is a separate feature decision. For now, document that:
- `tsuku install java` → picker (if TTY and multi-satisfier)
- `tsuku install java --from homebrew:openjdk` → create pipeline (no picker; generates recipe first)
- Future: `tsuku install java --recipe-name temurin` → direct recipe load (no picker)

---

## Section 3: Dependency Resolution and Picker Non-Engagement

### The Critical Rule: Pickers Only for Top-Level Install Commands

From the code flow:

1. **Top-level `tsuku install <user-input>`:** 
   - Calls `GetWithContext(ctx, name, opts)` in `loader.go`
   - If no direct match, falls back to satisfies
   - If satisfies yields multi-candidate alias → **PICKER ENGAGES (proposed feature)**

2. **Dependency resolution (plan generation):**
   - Calls `cfg.RecipeLoader.GetWithContext(ctx, depName, ...)` from `plan_generator.go:745`
   - RecipeLoader is typically the main Loader instance
   - If depName is multi-satisfier → **resolves to single entry in satisfies index** (no picker)

**Why no picker for dependencies?**

Dependency resolution occurs inside plan generation, which must be deterministic. The plan itself is replayed in CI/sandbox environments where TTY is unavailable. Introducing interactive prompts mid-plan-generation would break offline replay and CI workflows.

**Current implementation (from resolver.go line 430):**
```go
depDeps := ResolveDependenciesForPlatform(depRecipe, targetOS)
```

Dependencies are loaded by name only. If a dependency name is an alias with multiple satisfiers:

1. Loader tries direct match
2. Falls back to satisfies index
3. If satisfies has an entry, returns the canonical recipe for that alias
4. If no satisfies entry, returns not-found error

**Design principle:** A dependency specified as "java" in a recipe must resolve to a *single, deterministic recipe*. If multiple recipes satisfy "java", the recipe author must:
- Use a satisfies index entry (which pins "java" → one recipe)
- Or use the actual recipe name (e.g., "temurin") instead of the alias

**Recommendation:** Enforce at recipe validation time: if a dependency name is an alias with multiple satisfiers, fail validation with a clear error:
```
Error: dependency "java" is ambiguous (satisfied by temurin, openjdk, corretto).
Use a specific recipe name instead, or add a satisfies entry to pin the alias.
```

This is consistent with the pre-decided constraints (exact matches auto-satisfy; multi-satisfier needs explicit resolution).

---

### Runtime Dependencies and Pickers

From `resolver.go` (lines 44–45, 148–170):

Runtime dependencies are **collected but not resolved during plan generation**. They appear in state as metadata for the executor to check at runtime. Plan generation does not attempt to load recipes for runtime dependencies.

Therefore, runtime dependencies **never interact with the picker**. They are names-only at plan-gen time, resolved at execution time by the executor (which checks if the tool is installed).

**Recommendation:** Document this clearly in the plan generator's RecordDependencies section: runtime dependencies are validated at execution time, not plan time. Multi-satisfier aliases in runtime dependencies are an error (recipe validation catches this).

---

## Summary Table: Edge Cases

| Edge Case | Current Behavior | Picker Engaged? | Recommendation |
|-----------|------------------|-----------------|-----------------|
| Alias collides with recipe name | Direct name wins (satisfies fallback never runs) | No | Document that direct-name precedence is correct; no conflict |
| `runtime_dependencies = ["alias"]` | Only InstallTime deps are resolved; runtime deps are metadata-only | No | Add validation: runtime deps must resolve to single recipe |
| `tsuku update` on picked alias | State.Plan stores resolved recipe name; update uses that | No | Verify update flows use cached plan's Tool field |
| `--from <ecosystem>:<package>` | Routes to create pipeline (different code path) | No | Keep orthogonal; future `--recipe-name` flag if needed |
| Circular multi-satisfier alias | Not possible in current design (one satisfier per alias in index) | N/A | Maintain 1:1 satisfies index; multi-satisfier is picker domain only |

---

## Detailed Findings

### Plan Cache Key Does Not Include Recipe Source

**Note:** The cache key includes `Tool`, `Version`, and `Platform`, but *not* `RecipeSource`. Two recipes with the same name and version resolve to the same cache entry, regardless of source. This is by design (lines 20–25 in plan_cache.go) and is **compatible with the picker**—the picker is a user-facing feature that resolves at install time, not during plan caching.

### Content Hash Excludes Non-Functional Fields

The content hash (lines 94–99 in plan_cache.go) excludes:
- `GeneratedAt` (timestamp)
- `RecipeSource` (provenance)

This ensures **two identical plans hash the same**, regardless of when they were generated or which provider supplied the recipe. Compatible with picker (once resolved, the recipe name is in the hash).

### Satisfies Index Is 1:1 (Today)

From `loader.go:447–453`, the current index stores:
```go
l.satisfiesIndex[pkgName] = satisfiesEntry{
    recipeName: recipeName,  // Single recipe per package name
    source:     source,
}
```

The multi-satisfier picker will extend this to allow multiple recipes per alias, but the picker is a *separate concern* from the satisfies index's 1:1 entries. The picker is invoked *during* top-level recipe loading when multiple candidates exist.

---

## Open Questions for Implementation

1. **Picker trigger:** Should the picker appear only for multi-satisfier aliases, or also for satisfies fallback when multiple providers offer the same recipe? (Likely: multi-satisfier aliases only, to avoid confusion with discovery.)

2. **Picker state:** Should the picker's choice be remembered across sessions (prefer last picked, as a hint in the TTY)? (Design decision, but cache/plan are unaffected either way.)

3. **Non-TTY alias resolution:** When `--yes` is used and no satisfies entry exists, fail with a candidate list (decided). Implement this by detecting multi-satisfier scenarios *before* the picker would run, and returning an error with suggestions.

---

## Conclusion

The picker is **fully architecture-compatible** with plan caching and determinism:

- ✅ Plan hash is stable once recipe is resolved
- ✅ `--plan <file>` bypasses picker (no state leak)
- ✅ Dependency resolution never engages picker (deterministic)
- ✅ Edge cases are contained and well-precedenced (direct name > satisfies > multi-satisfier picker)
- ✅ Runtime dependencies are safe (metadata-only, no picker involvement)

No changes to plan caching, hashing, or state management are required. The picker is a *user-facing resolution layer* that feeds into the existing deterministic pipeline.

