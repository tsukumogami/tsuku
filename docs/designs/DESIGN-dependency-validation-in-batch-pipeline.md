---
status: Accepted
problem: |
  The Homebrew builder writes unresolvable dependency names into generated
  recipes without checking whether those dependencies exist in the tsuku
  registry. The batch pipeline classifies these as successes rather than
  blocked entries, hiding the true scope of missing dependencies from the
  dashboard. This means the batch priority queue can't properly order work
  by blocker impact, and tools appear installable when they actually can't
  resolve their dependency chain.
decision: |
  Validate dependencies inside the Homebrew builder before writing
  RuntimeDependencies to the recipe. Pass a RegistryChecker from create.go
  at both call sites, backed by the recipe Loader with satisfies support.
  When any dependency is missing, return a DeterministicFailedError with
  FailureCategoryMissingDep, listing all missing names in the error message
  using the format "recipe X not found in registry". The CLI exits with
  code 8 (ExitDependencyFailed), which the batch orchestrator already maps
  to the missing_dep category.
rationale: |
  The infrastructure for this validation already exists but isn't wired up.
  The RegistryChecker interface, the missing_dep error category, exit code
  8, the orchestrator's regex-based blocker extraction, and the dashboard's
  blocker reporting are all in place. The fix is a wiring change, not new
  architecture. Validating inside the builder catches problems before the
  recipe is written, keeping builder output always valid. Collecting all
  missing deps before failing gives the batch pipeline complete blocker
  information in a single pass.
---

# DESIGN: Dependency Validation in Batch Pipeline

## Status

Accepted

## Context and Problem Statement

When the Homebrew builder generates a recipe, it copies all dependency names from the Homebrew API into `runtime_dependencies` without checking whether those dependencies exist in the tsuku registry. The `RegistryChecker` interface and `WithRegistryChecker()` option were built for exactly this validation, but `create.go` never passes a checker to `NewHomebrewBuilder()`.

The result: recipes that look valid but fail at install time with missing dependencies. The batch pipeline treats these as generation successes because no error is emitted during the `create` step. Packages that should be classified as "blocked" end up as "success", which:

- Hides the true blocker landscape from the dashboard's top blockers list
- Prevents `queue-maintain requeue` from tracking and resolving dependency chains
- Produces recipes that users can't actually install
- Means the reorder step can't prioritize high-leverage dependency recipes

The fix is straightforward: the infrastructure exists but isn't connected. `RegistryChecker`, `FailureCategoryMissingDep`, `ExitDependencyFailed` (code 8), the orchestrator's `extractBlockedByFromOutput()`, and the dashboard's blocker reporting are all implemented. They just need wiring.

### Scope

**In scope:**
- Connecting `RegistryChecker` to `NewHomebrewBuilder()` in `create.go`
- Validating dependencies against the registry (including satisfies lookups)
- Returning proper errors with the `missing_dep` category
- Exiting with code 8 so the batch pipeline classifies these as blocked

**Out of scope:**
- Adding validation to other builders (Cargo, PyPI, etc.) -- follow-up work
- Changing the batch orchestrator or dashboard -- they already handle this correctly
- Modifying the requeue or reorder logic -- already works for blocked entries

## Decision Drivers

- **Use existing infrastructure**: All the pieces exist. The goal is connecting them, not building new ones.
- **Batch pipeline compatibility**: Error messages must match the regex `"recipe (\S+) not found in registry"` that `extractBlockedByFromOutput()` already parses.
- **Satisfies support**: Dependency lookups must resolve aliases (e.g., `openssl@3` resolves to the `openssl` recipe via `[metadata.satisfies]`).
- **Complete information per pass**: The batch pipeline runs entries once per batch. Collecting all missing deps in a single error (not failing on the first) gives full blocker information without re-running.
- **Minimal blast radius**: Only `create.go` and `homebrew.go` need changes. The `runDiscovery()` call site (line ~1044) doesn't currently have a `recipe.Loader` in scope, so it will need one passed in or constructed -- a minor expansion. The rest of the pipeline already handles the `missing_dep` category correctly.

## Considered Options

### Decision 1: Where to validate dependencies

The validation could happen in two places: inside the Homebrew builder itself (before it writes `RuntimeDependencies`), or in the CLI layer after the builder returns a recipe. Both are viable, but they have different failure modes and different implications for the builder's contract.

#### Chosen: Validate inside the builder

The builder already has access to the `RegistryChecker` via the `WithRegistryChecker()` option. After collecting dependencies from the Homebrew API and before writing them to `recipe.Metadata.RuntimeDependencies`, the builder checks each dependency against the registry (including satisfies lookups). If any are missing, it returns a `DeterministicFailedError` with `FailureCategoryMissingDep`.

This keeps the builder's output contract clean: if a recipe is returned, all its dependencies are resolvable. The builder already does dependency tree discovery in `DiscoverDependencyTree()` and marks nodes with `HasRecipe`/`NeedsGenerate` flags. The validation is a natural extension of that existing logic.

#### Alternatives Considered

**Validate in the CLI layer after recipe generation**: The CLI would call the builder, get a recipe back, then check each `RuntimeDependency` against the registry before writing the TOML file.
Rejected because it weakens the builder's contract -- callers would need to remember to validate after every call, and the builder could return recipes with unresolvable dependencies. The builder already has the registry checker interface; using it there keeps validation co-located with dependency collection.

**Validate in the orchestrator's `generate()` method**: The batch orchestrator would check dependencies after recipe creation, before marking the entry as success.
Rejected because it splits validation from the data source -- the builder knows the dependency names, the orchestrator only sees CLI output. It would also leave the interactive `tsuku create` path unvalidated.

**Silently filter unresolvable deps from the recipe**: Instead of failing, just omit dependencies that don't have recipes.
Rejected because partial dependency lists create harder-to-debug install failures. A tool that needs `openssl` but whose recipe omits it will fail at runtime with a cryptic linker or "library not found" error, far harder to diagnose than a clear "missing dependency" message at generation time.

### Decision 2: Fail-fast vs collect-all for missing dependencies

When multiple dependencies are missing, the builder could either fail on the first one or check them all and report the complete list. The batch pipeline benefits differ significantly between these approaches.

#### Chosen: Collect all missing dependencies then fail

Check every dependency in `RuntimeDependencies` against the registry. Collect all missing names, then return a single error listing them all. The error message includes one `"recipe X not found in registry"` line per missing dependency so the orchestrator's regex extracts the complete blocker set.

This matters for the batch pipeline: `extractBlockedByFromOutput()` populates the `BlockedBy` field on the failure record, and the dashboard uses `BlockedBy` to compute the top blockers list. With all blockers reported at once, the reorder step can accurately compute transitive blocking impact scores from a single batch run.

#### Alternatives Considered

**Fail on first missing dependency**: Return immediately when the first unresolvable dependency is found.
Rejected because the batch pipeline would need multiple runs to discover all blockers for a single entry. Each run processes entries once, so failing fast means N runs to discover N blockers -- wasting batch capacity and delaying accurate blocker impact scoring.

## Decision Outcome

### Summary

The fix wires up three existing components that were built for this exact purpose but never connected.

In `create.go`, both call sites for `NewHomebrewBuilder()` (the builder registry at line ~507 and the discovery probers at line ~1044) need to pass `WithRegistryChecker(checker)` where `checker` wraps the recipe `Loader`. The checker's `HasRecipe()` method should first try a direct recipe lookup, then fall back to `LookupSatisfies()` for alias resolution. This handles cases like `openssl@3` mapping to the `openssl` recipe.

In the Homebrew builder, after collecting dependencies from the Homebrew API and before writing them to `RuntimeDependencies`, the builder iterates each dependency and checks `b.registry.HasRecipe(dep)`. Missing dependencies are collected into a list. If any are missing, the builder returns a `DeterministicFailedError` with `FailureCategoryMissingDep` and a message containing `"recipe <name> not found in registry"` for each missing dependency.

The CLI's error handler already catches `DeterministicFailedError`. For `FailureCategoryMissingDep`, it should exit with code 8 (`ExitDependencyFailed`) instead of code 9 (`ExitDeterministicFailed`). The batch orchestrator already maps exit code 8 to the `missing_dep` category, extracts blocker names from the output, sets the queue entry to `StatusBlocked`, and records `BlockedBy` in the failure record. The dashboard already displays blocked packages, computes top blockers, and the requeue system already promotes blocked entries to pending when their dependencies resolve.

Edge cases to handle: when `RegistryChecker` is nil (the builder should skip validation and behave as today -- this preserves backward compatibility for callers that don't need validation), and when all dependencies resolve via satisfies (the builder should use the canonical recipe name in `RuntimeDependencies`, not the Homebrew alias).

### Rationale

Validating inside the builder and collecting all missing deps work together because the builder is the only place where dependency names are known before the recipe is written. Collecting all of them in one pass means the batch pipeline gets complete blocker information from a single `create` invocation, which flows through the existing orchestrator/dashboard/requeue chain without any changes to those components. The only code that changes is in `create.go` (2 call sites) and `homebrew.go` (validation logic in the dependency writing paths).

## Solution Architecture

### Overview

The change connects three existing layers:

```
create.go (CLI)
    |
    |-- creates RegistryChecker wrapping recipe.Loader
    |-- passes to NewHomebrewBuilder(WithRegistryChecker(checker))
    |
    v
HomebrewBuilder (internal/builders/homebrew.go)
    |
    |-- collects deps from Homebrew API (existing)
    |-- validates each dep against RegistryChecker (new)
    |-- if missing: returns DeterministicFailedError{missing_dep}
    |
    v
CLI error handler (create.go)
    |
    |-- catches DeterministicFailedError
    |-- for missing_dep: exit code 8
    |-- stderr: "recipe X not found in registry" per dep
    |
    v
Batch orchestrator (existing, no changes)
    |
    |-- extractBlockedByFromOutput() parses "recipe X not found"
    |-- sets entry to StatusBlocked with BlockedBy list
    |-- writes FailureRecord to JSONL
    |
    v
Dashboard + Requeue (existing, no changes)
    |
    |-- dashboard: top blockers, blocked package counts
    |-- requeue: blocked -> pending when deps resolve
```

### Components

**RegistryChecker adapter** (new, in `create.go`): A thin wrapper around `recipe.Loader` that implements the `RegistryChecker` interface. `HasRecipe(name)` tries `Loader.Get(name)` first, then `Loader.LookupSatisfies(name)` for alias resolution.

**Dependency validation** (new, in `homebrew.go`): Added as a shared helper called from the three code paths that write `RuntimeDependencies` (`generateRecipe`, `generateLibraryRecipe`, `generateToolRecipe`). When `b.registry` is non-nil, validates each dependency and collects missing ones. An alternative is to validate once in the `generateDeterministicRecipe` dispatcher (which calls both `generateToolRecipe` and `generateLibraryRecipe`), but `generateRecipe` (the LLM fallback path) also writes dependencies independently and would need its own check.

**Error routing** (modified, in `create.go`): The existing `DeterministicFailedError` handler currently catches all categories and unconditionally exits with code 9. This needs a category-based branch: check `detErr.Category == FailureCategoryMissingDep` first and exit with code 8, falling through to exit code 9 for all other categories. Without this branch, the orchestrator would classify missing-dep errors as `generation_failed` (exit 9) instead of `missing_dep` (exit 8), defeating the purpose of the change.

### Key Interfaces

The `RegistryChecker` interface already exists:

```go
type RegistryChecker interface {
    HasRecipe(name string) bool
}
```

The adapter implements this using `recipe.Loader`:

```go
type loaderRegistryChecker struct {
    loader *recipe.Loader
}

func (c *loaderRegistryChecker) HasRecipe(name string) bool {
    _, err := c.loader.Get(name)
    if err == nil {
        return true
    }
    _, found := c.loader.LookupSatisfies(name)
    return found
}
```

### Data Flow

1. `create.go` initializes `recipe.Loader` and wraps it as a `RegistryChecker`
2. Passes checker to `NewHomebrewBuilder(WithRegistryChecker(checker))`
3. Builder fetches formula info from Homebrew API (existing)
4. Builder collects `info.Dependencies` (existing)
5. **New**: Builder validates each dependency against `b.registry.HasRecipe(dep)`
6. If any missing: returns `DeterministicFailedError{Category: FailureCategoryMissingDep, Message: "missing dependencies: recipe X not found in registry; recipe Y not found in registry"}`
7. CLI catches error, prints to stderr, exits with code 8
8. Batch orchestrator parses stderr, extracts `["X", "Y"]`, sets entry to blocked

## Implementation Approach

### Phase 1: Wire up RegistryChecker

1. Add `loaderRegistryChecker` adapter in `create.go`
2. Pass `WithRegistryChecker(checker)` at both `NewHomebrewBuilder()` call sites (the builder registry in `buildCommand` and the discovery probers in `runDiscovery` -- the latter needs a `recipe.Loader` passed in or constructed)
3. Add a `validateDependencies` helper in `homebrew.go` that checks each dep against `b.registry` and collects missing ones. Call it from `generateRecipe`, `generateLibraryRecipe`, and `generateToolRecipe` before writing `RuntimeDependencies`
4. Add category-based branch in the `DeterministicFailedError` handler: `FailureCategoryMissingDep` exits with code 8, all other categories exit with code 9 (preserving current behavior)

### Phase 2: Tests

1. Add unit tests for `loaderRegistryChecker` adapter (direct lookup, satisfies fallback)
2. Add unit tests for the validation path in the Homebrew builder (mock registry, missing deps, all resolved, nil registry)
3. Update existing tests that create `HomebrewBuilder` without a registry checker to verify backward compatibility

### Phase 3: Verify batch pipeline integration

1. Verify that exit code 8 from CLI triggers `missing_dep` category in orchestrator
2. Verify that error message format matches `extractBlockedByFromOutput()` regex
3. Verify that dashboard correctly reports blocked packages with blocker names

Phase 3 is a verification step, not a code change. The orchestrator, dashboard, and requeue system shouldn't need modifications.

## Security Considerations

### Download Verification

Not applicable. This change adds validation of dependency names against a local registry before writing a recipe. No downloads are introduced. The existing download verification for Homebrew bottles is unchanged.

### Execution Isolation

Not applicable. The validation runs during recipe generation, before any tool is installed or executed. No new file system access, network access, or privilege changes are introduced.

### Supply Chain Risks

The dependency names come from the Homebrew JSON API, which is already the trusted source for the Homebrew builder. This change doesn't introduce new supply chain vectors -- it adds a check that dependencies are resolvable before writing them, which is strictly more conservative than the current behavior of trusting all dependency names unconditionally.

One consideration: if the registry checker has an inconsistent view (e.g., a recipe was just added but the checker doesn't see it), the builder would incorrectly reject a valid dependency. This is a false-negative failure mode (blocking when it shouldn't), which is safe -- the entry stays in the queue and gets retried on the next batch run when the registry is up to date.

### User Data Exposure

Not applicable. The validation only reads local recipe files and registry metadata. No user data is accessed or transmitted beyond what the existing `create` command already does.

## Consequences

### Positive

- Batch pipeline accurately classifies blocked entries, making the dashboard's top blockers list reflect reality
- `queue-maintain requeue` can automatically promote entries when their dependencies are added
- The reorder step can prioritize high-impact dependency recipes based on accurate blocker counts
- Users running `tsuku create --from homebrew` interactively get a clear error instead of a recipe that silently fails at install time

### Negative

- Recipes that previously generated "successfully" (but with broken deps) will now fail with exit code 8. This changes the batch pipeline's success count downward and blocked count upward. The numbers were wrong before, but the change will be visible. Expect a large shift: the `satisfies` index currently covers only ~3 recipes (openssl, gcc-libs, python-standalone), so most Homebrew dependency names with version suffixes (e.g., `berkeley-db@5`, `python@3.12`) won't resolve via alias and will correctly show as blocked.
- If the registry checker is stale (recipe added but not yet in the cached registry), valid dependencies could be rejected. This is a conservative failure -- the entry stays blocked and retries on the next batch run.

### Mitigations

- The dashboard already handles the blocked status and displays blocker information. No dashboard changes are needed for the count shift.
- Registry staleness is mitigated by the existing `update-registry` step that runs before batch processing.
