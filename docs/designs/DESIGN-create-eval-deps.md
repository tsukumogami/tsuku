---
status: Proposed
problem: |
  When tsuku create runs with sandbox validation enabled (the default), plan
  generation fails for ecosystem recipes (npm, cargo, pypi, gem) because the
  orchestrator doesn't wire up two things the plan generator needs: an
  OnEvalDepsNeeded callback for installing eval-time dependencies on the host,
  and a RecipeLoader for embedding install-time dependencies in the plan. All
  four builder CI workflows work around this with --skip-sandbox, which means
  sandbox validation is never exercised in CI.
decision: |
  Add OnEvalDepsNeeded, AutoAcceptEvalDeps, and RecipeLoader fields to
  OrchestratorConfig. The orchestrator's generatePlan method passes them
  through to PlanConfig, matching how tsuku install and tsuku eval already
  handle this. create.go provides the implementations using the existing
  installEvalDeps function and shared recipe loader. The --yes flag controls
  whether eval deps are auto-accepted or prompted. Builder CI tests then
  remove --skip-sandbox.
rationale: |
  The callback-in-config approach follows the pattern already established by
  PlanConfig.OnEvalDepsNeeded. tsuku install and tsuku eval both wire up these
  same fields and work correctly; the orchestrator was simply never updated to
  do the same. Threading both OnEvalDepsNeeded (for host-side plan generation)
  and RecipeLoader (for container-side dependency embedding) fixes the full
  pipeline rather than just one stage of it.
---

# Auto-Install Eval-Time Dependencies in Create Sandbox Mode

## Status

Proposed

## Context and Problem Statement

Running `tsuku create ruff --from pypi --yes` without `--skip-sandbox` fails:

```
Error building recipe: sandbox validation error: failed to generate installation plan:
failed to generate plan: failed to resolve step pipx_install:
missing eval-time dependencies: [python-standalone]
(install with: tsuku install python-standalone)
```

The orchestrator's `generatePlan` method creates a `PlanConfig` without two fields that the plan generator needs for ecosystem recipes:

1. **`OnEvalDepsNeeded`** -- The plan generator calls `Decompose()` on composite actions like `pipx_install`, which needs `python-standalone` on the host to run `pip download`. Without the callback, plan generation fails hard when eval-time deps are missing.

2. **`RecipeLoader`** -- Even if eval-time deps are satisfied, the generated plan won't include install-time dependencies. `cargo_build` needs `rust` inside the sandbox container. `pip_exec` needs `python-standalone`. Without `RecipeLoader`, the plan generator can't look up these dependency recipes, so the plan enters the container incomplete and execution fails.

Both `tsuku install` and `tsuku eval` already wire up these fields correctly. The orchestrator (used only by `tsuku create`) was never updated.

All four builder CI workflows (`npm`, `cargo`, `pypi`, `gem`) use `--skip-sandbox` to avoid this. The `TODO(#1287)` comments in these files reference the original issue tracking this gap.

## Decision Drivers

- Sandbox validation should work end-to-end for ecosystem recipes, not just for download-and-extract recipes
- The fix should reuse the existing `installEvalDeps` function and `RecipeLoader` infrastructure
- `--yes` should control auto-acceptance of eval-time deps, consistent with how it controls toolchain install consent
- Builder CI should exercise sandbox validation (or at minimum, plan generation) rather than skipping it entirely

## Considered Options

### Decision 1: How to Wire the Callbacks

The orchestrator lives in `internal/builders/` and can't depend on `cmd/tsuku/`. The host-side install mechanism (`installEvalDeps`) lives in `cmd/tsuku/eval.go`.

#### Chosen: Callback fields in OrchestratorConfig

Add `OnEvalDepsNeeded`, `AutoAcceptEvalDeps`, and `RecipeLoader` to `OrchestratorConfig`. The orchestrator's `generatePlan` passes them through to `PlanConfig`. `create.go` provides concrete implementations.

This matches how `PlanConfig` already works: the plan generator defines the interface (callback signature and `RecipeLoader` interface), callers provide implementations. The orchestrator becomes a pass-through, which is the right level of coupling.

#### Alternatives Considered

**Pre-install eval deps in create.go before the orchestrator runs.** Scan the recipe's steps for eval-time deps and install them before calling `orchestrator.Create()`. Rejected because the recipe isn't known until after `session.Generate()`, which runs inside the orchestrator. Splitting the orchestrator's flow to extract the recipe mid-cycle would complicate its API for no gain.

**Static builder-to-dep mapping.** Maintain a map like `{pypi: ["python-standalone"], cargo: ["rust"]}` and pre-install before orchestration. Rejected because it duplicates knowledge that actions already declare via `Dependencies().EvalTime`, and would drift as actions change.

### Decision 2: User Consent Model

#### Chosen: Auto-accept when --yes, prompt otherwise

`--yes` already implies consent for toolchain installation and recipe overwrite in `tsuku create`. Eval-time dependency installation follows the same rule: with `--yes`, deps install silently with a log line; without `--yes`, the user is prompted.

This is implemented by passing `createAutoApprove` (the `--yes` flag) as `AutoAcceptEvalDeps` in the orchestrator config. The existing `installEvalDeps` function already handles both modes.

#### Alternatives Considered

**Always auto-accept.** Like `tsuku install`, which always auto-accepts eval deps. Rejected because `tsuku create` has an explicit consent model via `--yes`, and silently installing tools without the flag contradicts that model.

**Always prompt.** Even with `--yes`. Rejected because it breaks the automated `--yes` workflow that CI and scripts depend on.

## Decision Outcome

**Chosen: 1A + 2A**

### Summary

Three fields are added to `OrchestratorConfig`: `OnEvalDepsNeeded func(deps []string, autoAccept bool) error`, `AutoAcceptEvalDeps bool`, and `RecipeLoader actions.RecipeLoader`. The orchestrator's `generatePlan` method includes them in the `PlanConfig` it constructs, alongside the existing `Downloader` and `DownloadCache` fields.

In `create.go`, the orchestrator is configured with `installEvalDeps` as the callback, the `--yes` flag as `AutoAcceptEvalDeps`, and the shared `loader` as `RecipeLoader`. This is roughly three lines of configuration.

With these changes, the full flow for `tsuku create ruff --from pypi --yes` becomes:

1. Toolchain check installs `pipx` on the host (existing behavior)
2. Builder generates a recipe with `pipx_install` step
3. Orchestrator calls `generatePlan` with the new config
4. Plan generator hits `pipx_install`, discovers `python-standalone` is missing
5. `OnEvalDepsNeeded` installs `python-standalone` on the host
6. Decomposition runs: `pipx_install` becomes `pip_exec` with locked requirements
7. `RecipeLoader` embeds `python-standalone` as an install-time dependency in the plan
8. Sandbox container receives a complete plan, installs `python-standalone` first, then runs `pip_exec`

After verifying the fix works, the four builder CI workflows remove `--skip-sandbox` and the `TODO(#1287)` comments.

### Rationale

The callback approach keeps the orchestrator decoupled from `cmd/tsuku/` while giving it the same capabilities that `tsuku install` and `tsuku eval` already have. Both problems (eval-time on host, install-time in container) must be fixed together; fixing only eval-time would get past plan generation but still fail inside the sandbox.

Tying `AutoAcceptEvalDeps` to `--yes` is the natural choice because `--yes` already controls every other consent prompt in `tsuku create`.

## Solution Architecture

### OrchestratorConfig Changes

```go
type OrchestratorConfig struct {
    // ... existing fields ...

    // OnEvalDepsNeeded is called when eval-time dependencies are missing
    // during plan generation. The callback should install the dependencies.
    // If nil and deps are missing, plan generation fails with an error.
    OnEvalDepsNeeded func(deps []string, autoAccept bool) error

    // AutoAcceptEvalDeps controls whether the OnEvalDepsNeeded callback
    // installs dependencies without prompting.
    AutoAcceptEvalDeps bool

    // RecipeLoader loads recipes for dependency resolution during plan
    // generation. When set, the plan includes embedded dependency trees
    // so sandbox containers can install deps without registry access.
    RecipeLoader actions.RecipeLoader
}
```

### generatePlan Threading

The orchestrator's `generatePlan` method adds the new fields to `PlanConfig`:

```go
plan, err := exec.GeneratePlan(ctx, executor.PlanConfig{
    OS:                 runtime.GOOS,
    Arch:               runtime.GOARCH,
    RecipeSource:       "create",
    Downloader:         downloader,
    DownloadCache:      downloadCache,
    OnEvalDepsNeeded:   o.config.OnEvalDepsNeeded,
    AutoAcceptEvalDeps: o.config.AutoAcceptEvalDeps,
    RecipeLoader:       o.config.RecipeLoader,
})
```

### create.go Wiring

```go
orchestrator := builders.NewOrchestrator(
    builders.WithSandboxExecutor(sandboxExec),
    builders.WithOrchestratorConfig(builders.OrchestratorConfig{
        // ... existing fields ...
        OnEvalDepsNeeded: func(deps []string, autoAccept bool) error {
            return installEvalDeps(deps, autoAccept)
        },
        AutoAcceptEvalDeps: createAutoApprove,
        RecipeLoader:       loader,
    }),
)
```

### CI Workflow Changes

All four builder CI workflows remove `--skip-sandbox` and the `TODO(#1287)` comment:

```yaml
# Before:
# TODO(#1287): remove --skip-sandbox once toolchains auto-install in sandbox
./tsuku create prettier --from npm --yes --skip-sandbox

# After:
./tsuku create prettier --from npm --yes
```

This affects 8 locations (Linux + macOS jobs) across 4 files:
- `.github/workflows/npm-builder-tests.yml`
- `.github/workflows/cargo-builder-tests.yml`
- `.github/workflows/pypi-builder-tests.yml`
- `.github/workflows/gem-builder-tests.yml`

### What Happens on macOS CI

macOS CI runners don't have Docker or Podman. When sandbox validation runs, `validate.NewRuntimeDetector()` returns `ErrNoRuntime`, and the orchestrator returns `SandboxSkipped: true`. The important thing is that **plan generation still runs** (which exercises eval-time dep installation and dependency embedding), even though the container step is skipped.

## Implementation Approach

The change touches two areas:

**Internal library** (`internal/builders/orchestrator.go`): Add three fields to `OrchestratorConfig` and six lines to `generatePlan` to thread them into `PlanConfig`.

**Command layer** (`cmd/tsuku/create.go`): Pass `installEvalDeps`, `createAutoApprove`, and `loader` when constructing the orchestrator config. Three additional fields in the existing config struct literal.

**CI workflows** (4 files): Mechanical removal of `--skip-sandbox` flag and `TODO` comment.

No new packages, interfaces, or test infrastructure. The existing unit tests for the orchestrator use mocks and don't exercise plan generation with real recipes. Integration coverage comes from the builder CI workflows, which will exercise the full flow once `--skip-sandbox` is removed.

## Security Considerations

### Download Verification

Eval-time dependencies are installed via `installEvalDeps`, which calls `runInstallTool` internally. This goes through the standard `tsuku install` path with checksum verification. No new download paths are introduced.

### Execution Isolation

Eval-time dependencies execute on the host with the user's permissions during plan generation. This is the same trust model as `tsuku install` and `tsuku eval`, not new to this change. The sandbox container continues to provide execution isolation for plan validation.

### Supply Chain Risks

No new supply chain vectors. The `RecipeLoader` resolves dependencies from the same local recipe registry that `tsuku install` uses. Dependency recipes go through the same review process as any other recipe.

### User Data Exposure

No change. The `installEvalDeps` function logs what it installs to stderr. No new external communication.

## Consequences

### Positive

- Sandbox validation works end-to-end for ecosystem recipes (npm, cargo, pypi, gem)
- Builder CI exercises plan generation and (on Linux) sandbox validation
- Removes 8 TODO comments and unblocks closing issue #1287
- No new abstractions; just threading existing config through one more call site

### Negative

- Eval-time deps are installed on the host during `tsuku create`, which may surprise users who expect `create` to only generate a recipe file. The logging from `installEvalDeps` mitigates this.
- macOS CI still doesn't exercise the actual sandbox container (no Docker). Linux CI does.
