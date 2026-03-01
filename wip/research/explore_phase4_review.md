# Architect Review: DESIGN-sandbox-build-cache.md

## Review Summary

The design proposes mapping plan dependencies to Docker image layers for caching, using targeted file mounts instead of the current broad workspace mount. Three decisions are presented: per-dependency Docker layers, alphabetical ordering, and targeted mounts.

The problem statement is well-grounded in the current code. The alternatives analysis is mostly fair. There are no blocking structural issues. There is one consistency concern that should be documented explicitly (state.json flow under targeted mounts), one interface change that needs a strategy decision, and several minor items.

---

## 1. Problem Statement Assessment

The problem statement is specific and grounded. It correctly identifies:

- The current two-level cache structure (download cache + container image cache) at `executor.go:242-249` and `executor.go:194-226`
- That neither level caches runtime ecosystem work (toolchain installation, registry population, compilation)
- The plan tree structure and how `DependencyPlan` entries map to cacheable units
- The dependency category taxonomy (EvalTime, InstallTime, Runtime) and why only InstallTime matters

The scope section draws clean boundaries. "Out of scope" items (cross-arch caching, shared .rlib files) are genuine non-goals, not deferred problems that would undermine the design.

One improvement: the problem statement mentions "5 families" and "60-minute job timeout" without qualifying that these are CI-specific constraints. The design driver "Dynamic, user-machine operation" suggests the primary target is developer machines, but the problem statement is framed around CI pain. This doesn't affect the technical design, but it could lead to priority confusion during implementation. The Consequences section (line 491) does clarify the savings target toolchain installation, not compilation.

---

## 2. Missing Alternatives

### Decision 1 (layer mapping): No missing alternatives

The three options considered (per-dep layers, single foundation image, docker commit) cover the reasonable design space. BuildKit cache mounts are correctly rejected for Podman compatibility reasons.

### Decision 2 (ordering): No missing alternatives

Alphabetical is the right choice. Frequency-based ordering is correctly rejected for requiring global state.

### Decision 3 (mount strategy): One missing alternative

**Overlay mount.** Docker and Podman both support `--mount type=overlay` (distinct from the storage driver), where a host directory is overlaid on top of the container's filesystem at a specific path. This would allow the container's `$TSUKU_HOME` from image layers to serve as the lower layer, with a host tmpdir as the upper layer receiving writes. The host reads the upper layer after the container exits.

This avoids the file-level mount granularity issues while preserving image layer visibility. However, it has real drawbacks: overlay mounts require specific kernel support (overlayfs), Podman's rootless mode has restrictions on overlay semantics, and the upper/lower layer merge semantics are more complex than targeted mounts. The design's chosen approach (targeted mounts) is likely still preferable, but the overlay option should be explicitly rejected with rationale rather than absent.

---

## 3. Rejection Rationale Assessment

### "Single foundation image with all deps" (Decision 1) -- Fair

The claim that recipes with partially overlapping dependency sets get no sharing is correct. Docker layers are position-sensitive, so `[rust]` and `[openssl, rust]` as single-RUN images share nothing.

### "Docker commit after runtime installation" (Decision 1) -- Fair

The current code passes `--rm` via `runtime.Run()` (containers are cleaned up). Changing the container lifecycle is a real cost. The Dockerfile approach is also reproducible, which commit-based images are not.

### "BuildKit cache mounts" (Decision 1) -- Fair

Podman's BuildKit compatibility is inconsistent, and `--mount=type=cache` doesn't survive `--cache-from`/`--cache-to` export. The rationale is specific and verifiable.

### "Full workspace mount with symlink bridge" (Decision 3) -- Fair

Introducing `/opt/ecosystem/` as a second `$TSUKU_HOME`-like path would create a parallel pattern. Every sandbox script would need a bridge step, and state.json consistency between the two paths is an explicit problem the alternative identifies but doesn't solve.

### "Full workspace mount with executor modification" (Decision 3) -- Fair

Coupling the executor to `/opt/ecosystem/` violates dependency direction: the executor is a general-purpose component in `internal/executor/`, and the sandbox layout is a sandbox-specific concern in `internal/sandbox/`. The executor shouldn't import sandbox knowledge.

### "Volume-mounted toolchain from host" (Decision 3) -- Fair

Host-side toolchain management is a parallel cache system, which contradicts the "Docker-native" decision driver. The cleanup semantics argument is valid (host files survive `docker system prune`).

---

## 4. Unstated Assumptions and Edge Cases

### 4a. state.json consistency under targeted mounts

This is the design's most important undocumented data flow.

When a foundation image is built via `RUN tsuku install --plan /tmp/plans/dep-00-rust.json --force`, the `runPlanBasedInstall` function at `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/cmd/tsuku/plan_install.go:95-111` writes to state.json via `mgr.InstallWithOptions()` and `mgr.GetState().UpdateTool()`. This means the foundation image's `/workspace/tsuku/state.json` will contain entries for the pre-installed dependencies (e.g., rust@1.82.0).

When the sandbox then runs `tsuku install --plan /workspace/plan.json --force` for the main recipe, that command also writes to state.json (same code path). The executor calls `installSingleDependency()` at `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/executor/executor.go:590-726`, which checks `os.Stat(finalDir)` at line 604 to skip already-installed deps. **The directory check works correctly** -- tools at `$TSUKU_HOME/tools/rust-1.82.0/` from the image layer will be found because the targeted mounts don't shadow that path.

After the plan executor finishes, `runPlanBasedInstall` calls `mgr.InstallWithOptions(effectiveToolName, plan.Version, exec.WorkDir(), installOpts)` to install the main tool. This reads state.json (via `StateManager.Load()`), adds the new tool entry, and writes it back. **This should work** because the dependency entries from the image's state.json persist through Load/Save.

However, there's a fragile coupling: the sandbox script currently runs `mkdir -p /workspace/tsuku/recipes`, `mkdir -p /workspace/tsuku/bin`, `mkdir -p /workspace/tsuku/tools` (see `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/sandbox/executor.go:503-506`). These create directories but don't touch state.json, so they're safe. But the design says these `mkdir -p` lines "become unnecessary" with foundation images and should be conditional based on whether `/workspace/tsuku/tools` exists. If a future change to the sandbox script initializes an empty state.json (e.g., a defensive `echo '{}' > state.json`), it would clobber the image's state.

**Recommendation (Advisory):** Document the state.json flow explicitly: (a) each `RUN tsuku install --plan` in the Dockerfile writes to state.json, (b) the final image's state.json contains all dependency entries, (c) the sandbox script must not reinitialize state.json, and (d) the main `tsuku install --plan` reads and extends the existing state.json. This prevents future maintainers from inadvertently breaking the flow.

### 4b. Build() interface change strategy

The design says "Extend `runtime.Build()` to accept a build context directory" and calls this "non-trivial." The current interface at `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/internal/validate/runtime.go:36-38`:

```go
Build(ctx context.Context, imageName, baseImage string, buildCommands []string) error
```

Both `podmanRuntime.Build()` (line 345) and `dockerRuntime.Build()` (line 485) currently pass `"."` as the build context. The foundation image approach needs a custom build context directory containing the tsuku binary and plan JSON files, and a pre-generated Dockerfile rather than the output of `generateDockerfile()`.

This is an interface change to `validate.Runtime`, which has mock implementations in tests (`validate/executor_test.go:36`). Three options:

1. Change the `Build()` signature (breaking all implementations including mocks)
2. Add a new method (e.g., `BuildFromDockerfile(ctx, imageName, dockerfile, contextDir)`) to the interface
3. Have the foundation builder call docker/podman directly, bypassing the Runtime interface

Option 2 is cleanest architecturally (additive, doesn't break existing callers). Option 3 would bypass the dispatch pattern. The design should specify which approach.

**Recommendation (Advisory):** Specify the interface extension strategy. Option 2 (additive new method) avoids breaking existing code and maintains the Runtime abstraction.

### 4c. Download cache mount mode change

The current code mounts the download cache as **read-only** (`executor.go:317`: `ReadOnly: true`). The design specifies the download cache as **read-write** ("The download cache mount is read-write because the container may need to download additional artifacts during installation that weren't pre-fetched during host-side plan generation").

This reverses the current security posture. A compromised container could poison the shared cache for subsequent runs. The Security Considerations section discusses cargo registry mounts (Phase 2) but doesn't mention this change for the download cache.

**Recommendation (Advisory):** Either keep the download cache read-only (the plan should have all artifacts pre-fetched, so additional downloads shouldn't be needed), or document the security trade-off.

### 4d. No-deps fallback could be eliminated

The design proposes that when a recipe has no InstallTime dependencies, the sandbox falls back to the current single workspace mount. This creates two code paths in `Executor.Sandbox()`:

1. Foundation image + targeted mounts (when deps exist)
2. Current broad workspace mount (when no deps)

The sandbox script generation and result reading are also conditional (markers written to `/workspace/output/` vs `/workspace/`).

The targeted mount approach could work unconditionally even without a foundation image. Without a foundation image, `/workspace/tsuku/tools/` would be empty in the container, and the sandbox script would create the directory structure. Verification markers would always go to `/workspace/output/`. This would eliminate the conditional path entirely.

**Recommendation (Advisory):** Consider using targeted mounts unconditionally to avoid conditional complexity. The no-deps case is simple enough that both paths work, but maintaining one path is always preferable to maintaining two.

### 4e. Docker file-level bind mount behavior

The design proposes mounting individual files (plan.json, sandbox.sh). Docker file-level bind mounts have a known behavior: if the container process replaces the file via write-to-temp-then-rename (atomic write), the mount binding breaks. This doesn't apply here because both files are mounted read-only. For the output and download cache, the design correctly uses directory mounts. No issue.

The download cache mount at `/workspace/tsuku/cache/downloads` is a directory mount nested inside the image's `$TSUKU_HOME` path (`/workspace/tsuku/`). Docker handles nested directory mounts correctly (the mount overlays that specific path while the rest of the parent directory comes from the image). This is well-supported behavior, but worth a one-line note in the design.

---

## 5. Strawman Analysis

None of the alternatives appear designed to fail. Each has a genuine use case:

- Single foundation image: legitimate approach, just reduces sharing
- Docker commit: simpler lifecycle, just not reproducible
- BuildKit cache mounts: would be ideal if Podman support were consistent
- Symlink bridge: how some CI systems handle Docker caching
- Executor modification: would work, just couples wrong packages

The chosen approach is the strongest option given the constraints.

---

## 6. Structural Fit Assessment

### Positive: Reuses existing patterns

- Uses `tsuku install --plan` inside RUN commands, keeping tsuku as the single installation authority. No bypass of the action dispatch system.
- The executor's existing skip logic (`os.Stat` on `$TSUKU_HOME/tools/{name}-{version}/`) works without modification because targeted mounts don't shadow that path. No parallel skip mechanism needed.
- Foundation images extend the existing container image hierarchy (base -> package -> foundation) rather than introducing a parallel build system.
- The naming convention (`tsuku/sandbox-foundation:{family}-{hash16}`) follows the existing `tsuku/sandbox-cache:{family}-{hash16}`.

### Concern: Runtime interface change

As noted in 4b, extending `Build()` touches the `validate.Runtime` interface. This needs to be done additively (new method, not signature change) to avoid breaking existing implementations.

### Positive: No state contract violations

No new fields are added to the state struct. The existing state.json structure is sufficient. The design reuses `install.Plan` for storage and `executor.InstallationPlan` for execution.

### Positive: Package dependency direction is correct

The foundation image logic lives in `internal/sandbox/`, which already depends on `internal/executor/` and `internal/validate/`. No new upward dependencies are introduced.

### Positive: Targeted mounts are strictly better

The broad workspace mount was a convenience that became an obstacle. Targeted mounts are a refinement, not a parallel pattern. The writable surface area decreases. The only question is whether to keep the broad mount as a fallback (see 4d).

---

## 7. Findings Summary

| # | Finding | Severity |
|---|---------|----------|
| 1 | state.json consistency under targeted mounts is not discussed; the flow where image layers contain dep entries and the sandbox run extends them needs explicit documentation | Advisory |
| 2 | `Build()` interface change strategy (additive new method vs signature change) is unspecified; affects the Runtime interface contract | Advisory |
| 3 | Download cache mode changes from read-only to read-write without security discussion | Advisory |
| 4 | No-deps fallback creates conditional complexity that could be eliminated by using targeted mounts unconditionally | Advisory |
| 5 | Missing alternative: overlay mounts should be explicitly rejected with rationale | Advisory |

No blocking issues. The design fits the existing architecture: it extends the container image hierarchy rather than replacing it, reuses the plan executor's skip logic without modification, keeps tsuku as the installation authority inside containers, and respects package dependency direction. The targeted mount approach cleanly solves the workspace shadowing problem without introducing parallel patterns.
