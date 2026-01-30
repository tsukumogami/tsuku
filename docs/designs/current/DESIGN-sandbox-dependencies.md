---
status: Current
problem: Sandbox testing fails for tools with declared dependencies because plan generation doesn't embed dependency information, so containers receive plans without the dependency tree needed for installation.
decision: Ensure all plan generation paths pass RecipeLoader, producing self-contained plans with embedded dependency trees that work identically in normal and sandbox modes.
rationale: Plans must be self-contained to execute in isolated containers. By passing RecipeLoader during plan generation, the existing format v3 dependency embedding infrastructure produces plans that include the full recursive dependency tree without any changes to the executor or plan format.
---

# Self-Contained Plans for Sandbox Dependencies

## Status

Current

## Context and Problem Statement

When users run `tsuku install <tool> --sandbox` for tools with declared recipe dependencies, the sandbox test fails because dependencies aren't installed in the container. For example, installing curl (which depends on openssl and zlib) in sandbox mode produces a plan with zero dependency steps, and the container build fails when OpenSSL can't be found.

The root cause was that sandbox plan generation didn't pass `RecipeLoader` to `GeneratePlan()`. Without a recipe loader, the plan generator can't look up dependency recipes, so it produces plans without embedded dependencies. Normal install mode had the same problem until PR #808 fixed it by threading `RecipeLoader` through. Sandbox mode was never updated.

More broadly, this exposed the fact that installation plans need to be self-contained: a plan must include everything required for execution, including the full recursive dependency tree. This is especially important for sandbox mode, where the container has no access to the recipe registry.

## Decision Drivers

- Plans must be executable in isolated containers without access to the recipe registry
- Sandbox mode should produce identical plans to normal install mode
- The fix should reuse the existing format v3 dependency embedding infrastructure
- Eval-time dependencies (tools needed during plan generation, like python-standalone for pipx recipes) must be handled

## Considered Options

The core decision was whether to embed dependencies in plans (making plans self-contained) or handle dependencies outside the plan (pre-installing on the host and mounting into containers). Pre-installing was rejected because it makes plans dependent on host state, which defeats the purpose of sandbox testing across different Linux families. Re-deriving dependencies from plan steps at execution time was rejected because it duplicates resolution logic and still requires recipe context. Passing the recipe alongside the plan was rejected because it only fixes the sandbox case without solving general plan portability.

The implementation decision was between adding RecipeLoader to the existing duplicate code (quick fix) versus extracting a shared plan generation function. Extraction was chosen to eliminate the code duplication between `install_deps.go` and `install_sandbox.go`.

## Decision Outcome

All plan generation paths now pass `RecipeLoader`, and sandbox plan generation uses a shared `generateInstallPlan()` function in `cmd/tsuku/helpers.go`. The duplicate `generatePlanFromRecipe()` that existed in `install_sandbox.go` was deleted.

The result is that `tsuku install curl --sandbox` now produces a plan containing the full dependency tree (openssl, zlib, and their transitive dependencies), which the container executor installs before building the main tool.

## Solution Architecture

### How Plans Become Self-Contained

When `RecipeLoader` is provided to `GeneratePlan()`, the plan generator recursively resolves all dependencies declared in the recipe's `dependencies` field, plus implicit dependencies from actions (e.g., `cmake_build` implies a cmake dependency). For each dependency, it loads the dependency recipe via `RecipeLoader`, generates a sub-plan, and embeds it in the parent plan's `Dependencies` field.

The plan format (v3) stores dependencies as a recursive tree of `DependencyPlan` entries. Each entry contains the tool name, version, its own nested dependencies, and the resolved installation steps. The `Platform` field on `InstallationPlan` records the OS, architecture, and Linux family the plan was generated for.

### Plan Generation Paths

There are two plan generation entry points, both of which pass `RecipeLoader`:

1. **`getOrGeneratePlanWith()`** in `install_deps.go` - Used by `tsuku install`. Includes plan caching: it checks for a cached plan before generating a fresh one. Passes `RecipeLoader` via the `planRetrievalConfig` struct (`cfg.RecipeLoader` at line 127).

2. **`generateInstallPlan()`** in `helpers.go` - Used by `tsuku install --sandbox` and `tsuku eval`. No caching. Passes `RecipeLoader` directly (line 124). This replaced the duplicate `generatePlanFromRecipe()` that previously existed in `install_sandbox.go`.

Both paths call `Executor.GeneratePlan()` with a `PlanConfig` that includes `RecipeLoader`, `AutoAcceptEvalDeps: true`, and an `OnEvalDepsNeeded` callback for installing eval-time dependencies.

### Sandbox Execution Flow

When `tsuku install curl --sandbox` runs:

1. `generateInstallPlan()` loads the curl recipe and calls `GeneratePlan()` with `RecipeLoader`
2. The plan generator resolves curl's dependencies (openssl, zlib, etc.) recursively
3. If any action requires eval-time dependencies (e.g., python-standalone), those are installed on the host via the `OnEvalDepsNeeded` callback
4. The resulting plan contains the full dependency tree with resolved steps
5. The sandbox executor writes the plan to a workspace directory
6. A container is created with the tsuku binary mounted
7. Inside the container, `tsuku install --plan /workspace/plan.json` executes the plan
8. The executor installs dependencies depth-first, then executes the main tool's steps

### Dependency Resolution Behaviors

**Cycle detection**: `ResolveDependencies()` in `internal/actions/resolver.go` tracks the resolution path and detects cycles. When a dependency appears in its own ancestor chain, resolution fails with an error showing the full cycle path (e.g., `ninja -> cmake -> ninja`). Cycles can be platform-specific - a cycle that exists on Linux (where cargo depends on patchelf which depends on cargo) might not exist on macOS. Detection runs after platform filtering, so platform-specific cycles only fail on the affected platform.

**Version conflicts**: When multiple dependencies require different versions of the same tool, the first version encountered wins. Resolution order is alphabetical by dependency name, making this deterministic. If the chosen version is incompatible with another dependent's requirements, the build fails at runtime with tool-specific errors. The workaround is to install conflicting tools separately.

**Deduplication at execution time**: During plan execution, the executor checks whether each dependency's installation directory already exists before running its steps (`internal/executor/executor.go`, line 595). If the directory exists, the dependency is skipped. This makes it safe for the same dependency to appear in multiple subtrees of the plan - it's only installed once.

**Resource limits**: The executor validates dependency tree depth (max 5 levels) and total dependency count (max 100) before beginning installation (`validateResourceLimits()` in `internal/executor/executor.go`, line 813). Plans exceeding these limits are rejected immediately. This prevents pathological cases from recipes with runaway transitive dependencies.

**Missing dependency recipes**: When a dependency has no recipe in the registry, plan generation skips it silently. The executor assumes the dependency is available in the execution environment (e.g., a system-installed tool). If it's actually missing, the build fails at the step that invokes it.

### Platform Awareness

Plans record the platform they were generated for in the `Platform` struct: OS, architecture, and optionally Linux family. The `LinuxFamily` field (`internal/executor/plan.go`) uses `omitempty` for backward compatibility with older plans.

At execution time, `validatePlatform()` (`internal/executor/executor.go`, line 795) checks that the plan's OS and architecture match the current system. Linux family validation is handled separately by `ValidatePlan()` in `plan.go`.

Platform constraints are inherited by all nested dependencies - the entire dependency tree is resolved for the same platform.

## Security Considerations

### Download Verification

Dependencies use the same checksum verification as main tools. All downloads include SHA256 checksums from recipes, and verification failures abort installation. This design extends the same verification to dependency downloads in sandbox mode.

### Execution Isolation

Sandbox mode tests recipe correctness across platforms; it does not provide security isolation from malicious code. Two execution contexts exist:

- **Plan generation (host)**: Eval-time dependencies execute on the host with full user permissions. This is an architectural property of plan generation, not specific to this design. Only use sandbox mode with trusted recipes.
- **Plan execution (container)**: All tool and library dependencies install inside the container with limited privileges.

The download cache directory is mounted read-write in the container, which means a malicious recipe could potentially poison cached files. This is a known limitation; using a separate cache directory for sandbox mode would mitigate it.

### Supply Chain Risks

Dependencies are resolved from the tsuku recipe registry using the same trust model as main tools: checksums verify downloads, and recipes are reviewed before merging. No new attack vectors are introduced. Plans are JSON files that could be tampered with between generation and execution, but checksum verification on each download limits the impact - modifying a URL in the plan without a matching checksum causes verification failure.

### User Data Exposure

Plans now include the full dependency tree, which reveals the tool stack being installed. No new external communication occurs; all downloads already happened in normal mode.

## Consequences

### Positive

- Sandbox testing works for tools with dependencies (unblocks multi-family CI testing via issue #806)
- Plans are self-contained and portable within a platform
- Sandbox and normal install modes share plan generation infrastructure
- Bug fixes to plan generation automatically apply to both modes

### Negative

- Eval-time dependencies (like python-standalone) must be installed on the host during plan generation, even for sandbox mode
- Plans are larger because they include the full dependency tree
- Two plan generation entry points remain (`getOrGeneratePlanWith` with caching, `generateInstallPlan` without), though both use the same underlying `GeneratePlan()` with `RecipeLoader`
