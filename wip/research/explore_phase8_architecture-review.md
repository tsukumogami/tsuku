# Architecture Review: Sandbox Build Cache (DESIGN-sandbox-build-cache.md)

Reviewer: architect-reviewer
Design: `docs/designs/DESIGN-sandbox-build-cache.md`
Relevant code: `internal/sandbox/executor.go`, `internal/sandbox/container_spec.go`, `internal/actions/cargo_build.go`, `internal/validate/runtime.go`, `internal/executor/executor.go`, `internal/executor/plan.go`

---

## Architecture Assessment

### 1. Clarity: Is the architecture clear enough to implement?

Yes, with one significant gap (see item 5 below). The three-level image hierarchy (base -> package -> foundation) is well-defined. The data flow section maps each step to a concrete function. The interaction between foundation images and the existing `Executor.Sandbox()` flow is described precisely enough that an implementer could write the code from the design alone.

The `docker commit` build process (start container, run tsuku install, commit) is specified step by step. The naming scheme (`tsuku/sandbox-foundation:{family}-{hash16}`) follows the existing pattern from `ContainerImageName()` in `container_spec.go`, so there's no ambiguity about format.

### 2. Missing components or interfaces

**2a. `runtime.Commit()` method -- new interface method required (acknowledged in design)**

The design mentions "Add `runtime.Commit()` method to the runtime interface" in Phase 1. This is correct. The current `Runtime` interface in `internal/validate/runtime.go:26-42` has `Run`, `Build`, and `ImageExists`. Adding `Commit` requires implementing it on both `podmanRuntime` and `dockerRuntime`. The design acknowledges this but doesn't specify the signature. Suggested:

```go
Commit(ctx context.Context, containerID string, imageName string) error
```

However, the design's build process (start container, run tsuku, commit) also needs a way to run a container _without_ `--rm` and get its container ID back. The current `Run()` method always passes `--rm` (see `runtime.go:286` for podman and `runtime.go:425` for docker). This means the foundation image build can't use `Run()` as-is. The design should specify either:
- A separate `RunDetached()` method, or
- A modified `Run()` with an option to skip `--rm`
- Direct shell-out to `docker run` / `docker commit` from the foundation builder, bypassing the runtime interface

The third option would be a dispatch bypass (my heuristic #1 for the Runtime interface). The first or second option extends the interface cleanly. This is a **blocking gap** -- the implementer will have to decide on one of these approaches, and the wrong choice creates a parallel pattern.

**2b. No `Remove` / `Prune` method on Runtime interface**

Phase 3 mentions "cleanup logic to prune stale foundation images." The `Runtime` interface has no `Remove` or `Prune` method. This will need to be added, or the pruning logic will shell out directly, bypassing the runtime abstraction. Minor gap -- Phase 3 is far enough out that this can be addressed then.

**2c. Foundation image build needs network access**

The design says the foundation image is built by running `tsuku install <dep> --force` inside a container. For Rust, this downloads the toolchain from the internet. The current `Sandbox()` method controls network via `RunOptions.Network`. The foundation image builder needs to ensure the build container has network access (`--network=host`). This is implicit in the design but should be explicit, since the sandbox's default for build actions is to enable network -- but the foundation build container is not a sandbox run, so it doesn't go through `ComputeSandboxRequirements()`.

### 3. Implementation phase sequencing

The sequencing is correct: Phase 1 (foundation images) delivers the largest time savings (toolchain installation). Phase 2 (cargo registry cache) is additive and independent. Phase 3 (CI integration) depends on Phase 1.

One potential reordering to consider: Phase 2 (cargo registry cache) could be implemented *before* Phase 1, since it's simpler (just adding a volume mount and a symlink in `buildDeterministicCargoEnv()`) and requires no new interface methods. This would give a quick win while Phase 1's `Commit()` interface extension is designed properly. But the current ordering is also fine since Phase 1 delivers more impact.

### 4. Simpler alternatives overlooked?

**4a. Pre-populating /workspace before the plan runs (instead of /opt/ecosystem)**

Instead of building a foundation image with the toolchain at `/opt/ecosystem/`, the sandbox script could copy the toolchain from a host-side cache into `/workspace/tsuku/tools/` *after* the workspace mount is applied. This avoids the `docker commit` complexity entirely -- just mount a read-only cache volume and `cp -a` at script start.

The design rejects the volume-mount approach because "the toolchain installation path includes hardcoded references to its location" (rustup sets RUSTUP_HOME and CARGO_HOME). This is valid for a *direct mount* (mount at one path, tool expects another), but a *copy* into the expected path would preserve path consistency. The downside is the copy time for ~500MB per family per run. Whether this is faster than building a foundation image once depends on usage patterns. For CI (many recipes, same toolchain), foundation images win. For local dev (few recipes), the copy approach is simpler.

This is not a flaw in the design -- the design's approach is better for the stated use case (CI). But it's worth noting that a simpler copy-based approach exists for cases where foundation image complexity is undesirable.

**4b. Multi-stage Dockerfile instead of docker commit**

Instead of `docker commit`, the foundation image could be built with a multi-stage Dockerfile:

```dockerfile
FROM tsuku/sandbox-cache:debian-{hash} AS base
COPY tsuku /usr/local/bin/tsuku
ENV TSUKU_HOME=/opt/ecosystem
RUN tsuku install rust --force
```

This produces a reproducible Dockerfile (addressing the negative consequence about non-reproducible images) and uses the existing `runtime.Build()` method, avoiding the need for a new `Commit()` interface method. The `Build()` method already accepts base image + build commands.

The design doesn't consider this option. It would require generating RUN commands that invoke tsuku, but that's straightforward since the foundation builder already knows the dependency list. This alternative avoids extending the Runtime interface while using the existing `Build` path.

The design's objection to "Dockerfile RUN commands for ecosystem installation" was about hardcoding `curl | sh`-style installation. But using tsuku itself as the installer inside a RUN command is different -- it keeps tsuku as the single source of truth. This deserves evaluation.

### 5. ExtractEcosystemDeps: does it work given how plans work?

This is the most significant architectural concern.

The design says `ExtractEcosystemDeps` "analyzes a plan and returns its ecosystem dependencies" by "reading `ActionDependencies.InstallTime`." But the plan (`InstallationPlan`) doesn't contain `ActionDependencies` at all. It contains:

- `Steps []ResolvedStep` -- each step has `Action` (string) and `Params` (map)
- `Dependencies []DependencyPlan` -- nested plans for install-time deps, each with `Tool`, `Version`, and `Steps`

The `ActionDependencies.InstallTime` field exists on the *action implementations* (e.g., `CargoBuildAction.Dependencies()` returns `ActionDeps{InstallTime: []string{"rust"}}`), not on the plan. To extract ecosystem deps from a plan, the implementation would need to:

1. Iterate over `plan.Steps`
2. For each step, look up the action by name via `actions.Get(step.Action)`
3. Call `action.Dependencies()` to get `ActionDeps.InstallTime`
4. Collect the results

This is feasible and is already the pattern used by `ComputeSandboxRequirements()` in `requirements.go` (which calls `actions.Get(step.Action)` to check `RequiresNetwork()`). But the design's description is misleading -- it says "reading `ActionDependencies.InstallTime`" as if this is a field on the plan, when it's actually a lookup against the action registry.

More importantly, **the plan already contains the dependency information in a different form**: `plan.Dependencies[]` is a list of `DependencyPlan` objects, each with `Tool` and `Version`. For a cargo_build recipe, `plan.Dependencies` already contains `{Tool: "rust", Version: "1.82.0", Steps: [...]}`. The ecosystem deps can be extracted directly from `plan.Dependencies` without consulting the action registry at all.

This raises a design question: should `ExtractEcosystemDeps` read from `plan.Dependencies` (which is the plan's own representation of what needs pre-installing), or from the action registry (which is the canonical declaration of what each action needs)?

Reading from `plan.Dependencies` is simpler and more correct for foundation images, because it includes the resolved version. Reading from the action registry gives you dependency names ("rust") but not versions -- you'd need a second step to find the version from `plan.Dependencies` anyway.

**Recommendation**: `ExtractEcosystemDeps` should iterate `plan.Dependencies` and return them directly. The design's interface signature (`func ExtractEcosystemDeps(plan *executor.InstallationPlan) []EcosystemDep`) is correct, but the implementation should use `plan.Dependencies`, not action registry lookups. The design text should be updated to reflect this.

However, there's a subtlety: not all `plan.Dependencies` are ecosystem toolchains. A recipe might depend on `perl` (for openssl-sys build scripts) or `nodejs` (for npm_exec). The foundation image should include all of these, not just "rust." The design's general-purpose framing handles this correctly -- any install-time dependency benefits from pre-installation. So iterating all `plan.Dependencies` is the right approach.

### 6. Docker commit approach: operationally sound?

Mostly yes, with caveats:

**Layering**: `docker commit` creates a single flat layer on top of the base image. For a Rust toolchain (~500MB), this means a 500MB layer per foundation image. With 5 families, that's ~2.5GB of image layers. This is acknowledged in the Consequences section. On CI runners with limited disk (14GB free on ubuntu-latest), this could be tight if multiple ecosystem versions coexist.

**Podman compatibility**: `podman commit` works similarly to `docker commit`. The design correctly notes this. No issues expected.

**Non-reproducibility**: If the foundation image is lost (pruned, CI cache eviction), it must be rebuilt. The rebuild runs tsuku install again, which may download a different version if the recipe's version resolution has changed. This is mitigated by the hash including the dep version, so a version change produces a different hash (different image name). If the same version is re-fetched, the result should be identical. Acceptable.

**Race condition in parallel families**: The CI workflow runs 5 families in parallel. If two families attempt to build the same foundation image simultaneously (e.g., both need rust 1.82.0 on debian), `docker commit` could race. One container commits, the other overwrites with an identical image, or one fails. Since each family has its own base image (debian, rhel, etc.), this won't happen across families. Within a single family, only one recipe runs at a time per the current workflow structure. No race condition in practice.

**Alternative noted in item 4b above**: Using `runtime.Build()` with a generated Dockerfile avoids the need for `Commit()` entirely and produces Dockerfile-reproducible images. This should be seriously considered before committing to the `docker commit` approach.

---

## Security Assessment

### 1. Attack vectors not considered

**1a. Foundation image poisoning via stale images**

If an attacker can write to the local Docker image store (requires Docker group access or root), they can replace a foundation image with a poisoned one. The executor checks `runtime.ImageExists()` and uses the image if it exists -- there's no integrity verification of the image content.

Mitigation already exists: the sandbox itself runs with container isolation, so a poisoned foundation image can only affect the sandbox test results, not the host. The trust boundary is the container runtime. If an attacker has Docker group access, they already have root-equivalent access on the host, so this doesn't meaningfully expand the attack surface.

Severity: not applicable (attacker with Docker access already has higher privileges).

**1b. TSUKU_HOME=/opt/ecosystem exposes a second tool directory**

The foundation image runs `tsuku install rust --force` with `TSUKU_HOME=/opt/ecosystem`. This creates a full tsuku directory structure at `/opt/ecosystem/` (bin, tools, state.json, etc.). When the sandbox script later runs with `TSUKU_HOME=/workspace/tsuku`, there are two independent tsuku installations in the container. The `/opt/ecosystem/bin` is prepended to PATH.

If a tool installed in the foundation image has a vulnerable binary, it persists in the foundation image across sandbox runs until the image is rebuilt. Unlike ephemeral sandbox runs where the workspace is destroyed after each run, foundation image content survives.

Mitigation: Foundation images are version-pinned. When the toolchain version changes, a new foundation image is built. Stale foundation images are pruned (Phase 3). The window of exposure is between a vulnerability disclosure and the next toolchain version update. This is the same exposure window as any cached container image.

Severity: low. Accepted risk with pruning mitigation.

**1c. Cargo registry cache -- concurrent write corruption**

The design notes the cargo registry mount is read-write. If two families run `cargo fetch` simultaneously to the same shared directory, cargo's registry operations may not be safe for concurrent writes. Cargo uses file locks (`flock`) on registry index operations, which should serialize access. But this depends on the filesystem supporting `flock` inside a container with a bind mount.

On Linux with ext4/xfs and Docker/Podman bind mounts, `flock` works correctly. On overlayfs or tmpfs, behavior may vary. Since the mount is a host directory bind-mounted into the container, it's the host filesystem, so `flock` should work.

The CI workflow runs families in parallel (background processes). All share the same `SHARED_CACHE` directory. If the cargo registry cache is a subdirectory of `SHARED_CACHE`, concurrent `cargo fetch` across families could hit lock contention. This would slow things down but shouldn't corrupt data, since cargo's locking is designed for this.

Severity: low. Concurrent access is serialized by cargo's own locking. Performance impact only.

### 2. Mitigation sufficiency

The mitigations listed in the design are adequate for the identified risks:

- **Disk usage**: Pruning on version change + `docker system prune` is reasonable.
- **Registry write access**: Scoping to single recipe run + cargo's checksum verification is sufficient.
- **Podman compatibility**: Avoiding BuildKit-specific features is correct.

One mitigation gap: the design doesn't specify what happens if `docker commit` fails mid-operation (e.g., disk full, timeout). The foundation image builder should handle partial failures gracefully -- remove the incomplete image if it was partially created, and fall back to running without a foundation image. The current code pattern for package images (check exists, build if missing) handles this naturally since a failed build won't produce the image, so the next run will retry. But `docker commit` failure modes are different from `docker build` failure modes, so this deserves explicit error handling in the implementation.

### 3. Residual risk to escalate

No residual risks require escalation. The security model is sound: foundation images are local-only, built from the same source images, and run under the same container isolation. The cargo registry cache introduces a shared mutable surface, but cargo's own integrity checks (Cargo.lock checksums) prevent tampering from affecting build correctness.

### 4. "Not applicable" justifications

**User Data Exposure: "Not applicable"** -- Correctly justified. Foundation images contain only compiler toolchains. The cargo registry cache contains open-source crate source. No user-specific data is involved.

However, the design should note one edge case: if a recipe's `cargo_build` step includes environment variables (via `ExtraEnv` in `SandboxRequirements`), those are set at sandbox run time, not during foundation image construction. API tokens or credentials set via `--env` won't leak into foundation images. This is correct behavior but worth documenting explicitly.

### 5. Read-write cargo registry mount: significant risk?

No. The risk is contained by three factors:

1. **Scope**: The mount is shared only across families for a single recipe within one sandbox invocation. Cross-recipe isolation is maintained.

2. **Cargo's verification**: `cargo fetch --locked` verifies crate checksums against `Cargo.lock`. A tampered registry entry would cause a checksum mismatch and fail the build.

3. **Same-recipe equivalence**: All families for the same recipe fetch the same crates from the same `Cargo.lock`. There's no scenario where family A fetches a different crate version than family B.

The only scenario where this matters is if a compromised cargo binary in one family's container modifies the shared registry to inject malicious source. But a compromised cargo binary already means the container itself is compromised, which is a pre-existing trust assumption (the container runtime is the trust boundary).

The design could strengthen this by making the mount read-only for all families after the first family's fetch completes, but the operational complexity (coordinating "first fetch done" across parallel containers) outweighs the marginal security benefit.

---

## Structural Fit

### Follows existing patterns

- **Image naming**: `tsuku/sandbox-foundation:{family}-{hash16}` follows the `tsuku/sandbox-cache:{family}-{hash16}` pattern from `ContainerImageName()`.
- **Hash computation**: Deterministic, sorted inputs, SHA256, first 16 hex chars. Same approach.
- **Executor option pattern**: `WithCargoRegistryCacheDir()` follows `WithDownloadCacheDir()` in `executor.go:84-89`.
- **Plan analysis**: Iterating `plan.Steps` with `actions.Get()` follows `ComputeSandboxRequirements()` in `requirements.go`.
- **New file placement**: `internal/sandbox/foundation.go` is correctly scoped to the sandbox package.

### Potential structural concern

The `BuildFoundationImage` method is on `*Executor`, which means it has access to the executor's runtime detector but also uses `runtime.Run()` + `runtime.Commit()` (or an alternative). If the foundation image builder grows complex, it might warrant its own type (e.g., `FoundationBuilder`). But starting as a method on `*Executor` is fine for Phase 1. If it grows beyond ~100 lines, extract it.

### No parallel pattern introduction

The design extends the existing image cache hierarchy rather than creating a parallel caching mechanism. Foundation images use the same `ImageExists()` / build-if-missing pattern. The cargo registry cache reuses the volume mount pattern from the download cache. No new patterns introduced.

---

## Summary of Findings

| # | Finding | Severity | Section |
|---|---------|----------|---------|
| 1 | `Run()` always passes `--rm`; foundation build needs a container that persists for commit. Design doesn't specify how to handle this. Either extend `Run()` with an option, add `RunDetached()`, or use the existing `Build()` with a generated Dockerfile (recommended). | Blocking | Architecture 2a |
| 2 | `ExtractEcosystemDeps` should read from `plan.Dependencies` (which has Tool+Version), not from action registry lookups as the design text implies. The data is already in the plan. | Advisory | Architecture 5 |
| 3 | Multi-stage Dockerfile via existing `runtime.Build()` is a simpler alternative to `docker commit` that avoids extending the Runtime interface and produces reproducible images. Not evaluated in the design. | Advisory | Architecture 4b |
| 4 | Foundation build container needs explicit network access (`--network=host`). Design is implicit about this. | Advisory | Architecture 2c |
| 5 | Error handling for `docker commit` failure (partial image cleanup, fallback to no-foundation-image execution) should be specified. | Advisory | Security 2 |
| 6 | Concurrent `cargo fetch` from parallel families is serialized by cargo's own `flock`. Documented in design but the lock mechanism isn't identified. | Not blocking | Security 1c |
| 7 | Read-write cargo registry mount risk is adequately mitigated. | Not blocking | Security 5 |
