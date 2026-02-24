# Architect Review: DESIGN-sandbox-build-cache.md

## Review Summary

The design proposes two caching mechanisms for the sandbox system: (1) foundation images that pre-install ecosystem toolchains into Docker images at `/opt/ecosystem/`, and (2) a shared cargo registry cache mounted as a read-write volume across families. The review evaluates problem specificity, alternative coverage, rejection rationale, unstated assumptions, and strawman risk.

---

## 1. Problem Statement Specificity

**Verdict: Sufficient, with one gap.**

The problem statement quantifies the waste well: 5 independent Rust installations and 5 independent compilations per recipe. The distinction between download cache (already solved), container image cache (already solved), and runtime ecosystem work (the gap) is clearly drawn.

The gap: the problem statement says "heavy crates like komac, cargo-nextest, or probe-rs-tools frequently hit the 60-minute job timeout," but the design's two proposed caches address only ~15-25 minutes of the ~60-minute cost (toolchain install + cargo fetch). The actual compilation of the dependency tree -- "10-40 minutes for heavy crates" -- is explicitly out of scope. The document should state this limitation more prominently in the summary. A reader could misunderstand the 60-minute timeout mention as implying the design solves the timeout problem. It reduces the window but doesn't eliminate it for the heaviest crates.

**Recommendation:** Add a sentence to the Decision Outcome summary clarifying that foundation images + registry cache reduce per-recipe overhead by approximately 15-25 minutes total across 5 families, but compilation time remains unaddressed and is the dominant cost for heavy crates.

---

## 2. Missing Alternatives

### 2a. Volume-mount with path fixup (partial consideration)

The "Volume-mounted toolchain from host" alternative was rejected because "the toolchain installation path includes hardcoded references to its location (rustup sets RUSTUP_HOME and CARGO_HOME)." This is accurate for rustup specifically, but the design could have considered installing the toolchain to `/opt/ecosystem/` on the host and mounting that path into containers at the same path. The mount target matches the installation path, so there's no relocation problem.

This would produce the same result as the `docker commit` approach but without the commit step, and the cached toolchain would survive `docker system prune`. The tradeoff is that the host filesystem becomes the cache instead of the Docker image store, which changes cleanup semantics.

**Impact:** Not blocking. The `docker commit` approach is a reasonable choice. But the volume-mount alternative was rejected for a reason that only applies when host and container paths differ, and the design already chose `/opt/ecosystem/` as a fixed path. The rejection rationale should acknowledge this distinction.

### 2b. Multi-stage Dockerfile with COPY --from

Not considered. Instead of `docker commit`, the foundation image could be built with a multi-stage Dockerfile:
```dockerfile
FROM package-image AS builder
RUN tsuku install rust --force
FROM package-image
COPY --from=builder /opt/ecosystem /opt/ecosystem
```

This produces a Dockerfile-reproducible image (addresses the negative consequence about non-reproducibility), and works with both Docker and Podman. It requires the tsuku binary to be available inside the build context, which the sandbox already handles (it mounts the binary). The tradeoff is a more complex Dockerfile template.

**Impact:** Advisory. `docker commit` is simpler and the non-reproducibility consequence is already mitigated (automatic rebuild). But this alternative deserves mention in the rejection list since the reproducibility concern is noted as a negative consequence.

### 2c. Compilation caching via sccache/shared target directory

The design explicitly excludes sharing compiled artifacts across families (correctly, due to libc differences). But it doesn't consider sharing compiled artifacts across runs of the *same family*. A per-family compilation cache (e.g., mounting `target/` or using sccache with a per-family key) would address the 10-40 minute compilation cost that the current design leaves untouched.

**Impact:** Out of scope for this design (the scope section explicitly excludes it), but should be mentioned as a future direction since the problem statement highlights compilation time as the dominant cost.

---

## 3. Rejection Rationale Evaluation

### 3a. "Dockerfile RUN commands for ecosystem installation" -- Fair

The rejection reasoning is sound: duplicating installation logic between Dockerfile RUN commands and tsuku's own install path creates divergence risk. The design correctly identifies that tsuku should be the single source of truth.

### 3b. "BuildKit cache mounts" -- Fair

The Podman compatibility argument is valid. BuildKit cache mounts have inconsistent cross-runtime support. The rejection is well-grounded.

### 3c. "Volume-mounted toolchain from host" -- Partially unfair

As noted in 2a, the rejection cites path relocation issues, but the design already chose `/opt/ecosystem/` as a fixed path. If the host installs to `/opt/ecosystem/` and the container mounts at `/opt/ecosystem/`, there's no relocation. The rejection conflates "mounting a toolchain installed at one path into a container at a different path" with volume mounts in general. The real tradeoff is host filesystem management vs Docker image store management, not path incompatibility.

### 3d. "Don't share registry content" -- Fair

Straightforward cost/benefit analysis. 5-10 minutes of redundant network I/O for identical content is wasteful.

### 3e. "Share the entire CARGO_HOME" -- Fair

Correctly identifies that compiled artifacts are platform-specific. The registry-only sharing is the right boundary.

### 3f. "Cargo-specific caching" -- Fair

The generalization argument is strong: `ActionDeps.InstallTime` already declares ecosystem dependencies uniformly across all actions. Building the mechanism around this existing interface is the natural extension.

---

## 4. Unstated Assumptions

### 4a. "The plan finds the pre-installed toolchain and skips its installation step"

This is the most critical assumption in the design and it's underspecified. The design says: "The sandbox script prepends `/opt/ecosystem/bin` to PATH so the plan finds the pre-installed toolchain and skips its installation step."

Looking at the actual executor code (`executor.go:590-616`), the skip logic is:
```go
// Skip if already installed (deduplication)
if _, err := os.Stat(finalDir); err == nil {
    fmt.Printf("\nSkipping dependency: %s@%s (already installed)\n", dep.Tool, dep.Version)
```

This checks for `$TSUKU_HOME/tools/{name}-{version}/` -- which is `/workspace/tsuku/tools/rust-1.82.0/` inside the container. But the foundation image installs Rust at `/opt/ecosystem/`, not at `/workspace/tsuku/tools/rust-1.82.0/`. The `/workspace` mount shadows the workspace, and the skip check looks at the workspace path.

So the skip mechanism **will not fire** as currently implemented. The executor will attempt to re-install Rust even though it's already at `/opt/ecosystem/bin`. The design needs one of:

1. A check in `installSingleDependency` that also looks in `/opt/ecosystem/`
2. The foundation image builder creates a marker at `/opt/ecosystem/tools/rust-1.82.0/` and the executor is taught to check that path
3. The sandbox script creates symlinks from `/workspace/tsuku/tools/rust-1.82.0/` to `/opt/ecosystem/tools/rust-1.82.0/` before running the plan

This is a correctness gap, not a design preference issue. Without addressing it, the foundation image provides Rust on PATH but the plan will still attempt its own Rust installation as a dependency step.

**Impact: Blocking.** The design should specify the exact mechanism by which the executor skips dependency installation when the toolchain exists at `/opt/ecosystem/` rather than `$TSUKU_HOME/tools/`.

### 4b. Foundation image builds need network access

Building the foundation image requires starting a container from the package image and running `tsuku install rust --force`. This requires network access (to download the Rust toolchain). The design doesn't address how the foundation image build container gets network access, resource limits, or timeout configuration. These details matter because the foundation build is itself a container execution.

The design should specify whether `BuildFoundationImage` uses the same `runtime.Run()` path (without `--rm`, since it needs to commit) or a different code path.

**Impact: Advisory.** Implementation detail, but the design should acknowledge that `docker commit` requires running a container without `--rm`, which is different from the current `runtime.Run()` contract (which always passes `--rm`). This will require either a new runtime method or modifications to `RunOptions`.

### 4c. TSUKU_HOME in foundation image build

The design says the foundation image builder runs `tsuku install <dep> --force` with `TSUKU_HOME=/opt/ecosystem`. This means tsuku will create `/opt/ecosystem/bin/`, `/opt/ecosystem/tools/`, etc. The sandbox script later prepends `/opt/ecosystem/bin` to PATH.

But the main plan execution uses `TSUKU_HOME=/workspace/tsuku`. If a dependency like Rust installs to `/opt/ecosystem/tools/rust-1.82.0/bin/cargo`, and cargo_build's `ResolveCargo()` searches `$TSUKU_HOME/tools/*/bin/cargo` (which is `/workspace/tsuku/tools/*/bin/cargo`), it won't find the pre-installed cargo. The PATH prepend would make `cargo` findable via PATH, but `ResolveCargo()` in `util.go` does an explicit glob search in `$TSUKU_HOME/tools/`:

```go
func ResolveCargo() string {
```

This function needs to be checked to confirm it falls back to PATH lookup when the glob finds nothing.

**Impact: Advisory.** Likely works because `ResolveCargo()` falls back to `"cargo"` which resolves via PATH. But the design should confirm this for each ecosystem tool resolver, not just assume it.

### 4d. Concurrent foundation image builds

The CI workflow runs 5 families in parallel for each recipe. If all 5 need a foundation image and none exists, all 5 will try to build their respective foundation images concurrently. This is fine (they're building different images: one per family). But if two recipes need the same foundation image for the same family, the second one might attempt to build while the first is still building.

The existing `ContainerImageName` + `ImageExists` pattern has no locking. This is fine for the existing package image cache (built during `DeriveContainerSpec`, which runs once per recipe-family pair). But foundation images are more expensive to build (2-3 minutes), so the race window is wider.

**Impact: Advisory.** The current CI workflow runs families in parallel *within* a recipe but recipes sequentially within a batch. So the same foundation image won't be built concurrently by the same job. But the design should document this assumption.

### 4e. `docker commit` captures the full filesystem diff

`docker commit` captures all filesystem changes in the running container. This includes not just `/opt/ecosystem/` but also any temp files, package manager caches, or log files created during the `tsuku install rust` process inside the container. The resulting image could be larger than necessary.

The design doesn't mention cleanup steps before committing. A `rm -rf /tmp/* /var/cache/*` before commit would reduce image size.

**Impact: Advisory.** Disk space consequence is already acknowledged in the design, but the image size could be larger than the ~500MB estimate if cleanup isn't done.

---

## 5. Strawman Analysis

**No option is a strawman.** Each rejected alternative addresses a real aspect of the problem:

- Dockerfile RUN commands: legitimate approach, just creates divergence
- BuildKit cache mounts: would be ideal if Podman support were consistent
- Volume-mounted toolchain: would work with path alignment (rejection is slightly unfair but the option is real)
- Cargo-specific caching: would work, just doesn't generalize

The chosen approach is the strongest option given the constraints (Docker+Podman, no relocation, follows existing patterns).

---

## 6. Specific Technical Analysis

### 6a. Workspace mount shadowing and `/opt/ecosystem/`

The analysis of mount shadowing is correct. The `/workspace` mount at `executor.go:308-313` will shadow anything the Docker image contains at that path. Installing to `/opt/ecosystem/` cleanly avoids this.

However, the design should note that `/opt/ecosystem/` is a new convention that introduces a second TSUKU_HOME-like directory structure. The foundation image builder uses `TSUKU_HOME=/opt/ecosystem`, creating `/opt/ecosystem/bin/`, `/opt/ecosystem/tools/`, etc. This is effectively a second tsuku installation inside the same container. The design should state this explicitly and specify that the sandbox script must handle the PATH merge correctly (foundation `/opt/ecosystem/bin` comes first, then workspace `/workspace/tsuku/bin`).

### 6b. `docker commit` vs alternatives

`docker commit` is the right choice for this use case. The alternatives (multi-stage Dockerfile, Dockerfile RUN with tsuku) all require encoding tsuku's installation logic into a Dockerfile, which the design correctly wants to avoid. `docker commit` captures the result of running tsuku inside a container, which preserves the single-source-of-truth property.

The main cost is non-reproducibility: `docker commit` images can't be rebuilt from a Dockerfile alone. The design's mitigation (automatic rebuild on demand) is sufficient.

### 6c. Cargo registry sharing safety

The read-write registry mount is the design's weakest isolation guarantee. The security section correctly identifies that a malicious cargo build could modify the shared registry. The mitigation (scoped to a single recipe's multi-family run + cargo checksums on read) is adequate for the threat model.

One concern not addressed: if the first family's `cargo fetch` fails partway through, subsequent families will see a partially populated registry. Cargo should handle this gracefully (it fetches missing entries), but the design should state this explicitly.

The design says `buildDeterministicCargoEnv()` would "create the isolated `.cargo-home` with a symlink from `registry/` to the shared mount." This is a clean approach that preserves the isolated CARGO_HOME pattern while sharing only the registry directory. The symlink approach is correct.

### 6d. General vs cargo-specific scope

The generalization decision is well-justified. `ActionDeps.InstallTime` already provides a uniform declaration across all ecosystem actions:

- `cargo_build`: `InstallTime: ["rust"]`
- `go_build`: `InstallTime: ["go"]`
- `npm_install`: `InstallTime: ["nodejs"]`
- `cmake_build`: `InstallTime: ["cmake", "make", "zig", "pkg-config"]`

The `ExtractEcosystemDeps()` function can read these from the plan's dependency tree, compute a hash, and build a foundation image -- all without knowing anything about Rust or Go specifically. The cargo registry cache is correctly treated as a separate, cargo-specific optimization (Phase 2).

---

## 7. Architectural Fit Assessment

### Follows existing patterns

The design extends the existing two-level cache (base image + package image) to three levels (+ foundation image). The naming convention (`tsuku/sandbox-foundation:{family}-{hash16}`) follows `tsuku/sandbox-cache:{family}-{hash16}`. The hash-based caching pattern is preserved. This is the right structural approach.

### Runtime interface extension

Adding `runtime.Commit()` to the `validate.Runtime` interface is a clean extension. Both Docker and Podman support `docker commit` / `podman commit`. However, the design should specify whether `Commit` is added to the `Runtime` interface directly (requiring implementation in both `podmanRuntime` and `dockerRuntime`) or as a separate interface that `BuildFoundationImage` type-asserts against. Given that both runtimes support commit, adding it directly to the interface is simpler.

The current `runtime.Run()` always passes `--rm`. Foundation image building needs a container that persists long enough to be committed. The design needs a `RunOptions` flag like `KeepContainer bool` or a separate `RunAndCommit` method on the runtime. This is not addressed in the design.

### New file placement

`internal/sandbox/foundation.go` is the right location. The foundation image logic is sandbox-specific and belongs in the sandbox package. Putting `ExtractEcosystemDeps`, `FoundationImageName`, and `BuildFoundationImage` together in one file follows the existing pattern where `container_spec.go` contains `DeriveContainerSpec`, `ContainerImageName`, and the build logic.

### No dependency direction violations

The proposed `foundation.go` imports from `internal/executor` (for `InstallationPlan`) and `internal/validate` (for `Runtime`). Both are lower-level than `sandbox`. No circular dependencies introduced.

---

## 8. Findings Summary

| # | Finding | Level |
|---|---------|-------|
| 1 | Dependency skip mechanism doesn't work: executor checks `$TSUKU_HOME/tools/` which is shadowed by workspace mount; pre-installed toolchain at `/opt/ecosystem/` won't be found by the skip check | Blocking |
| 2 | Volume-mount alternative rejected for reason that doesn't apply when host and container paths match | Advisory |
| 3 | Missing: how `runtime.Run()` (which passes `--rm`) works with `docker commit` (which needs the container to persist) | Advisory |
| 4 | Problem statement mentions 60-minute timeout but proposed solution addresses only ~15-25 minutes of the cost; compilation time is dominant and unaddressed | Advisory |
| 5 | `docker commit` captures full filesystem diff including temp files; no cleanup step specified | Advisory |
| 6 | Partial cargo fetch by first family could leave incomplete registry for subsequent families | Advisory |
| 7 | `ResolveCargo()` and similar ecosystem resolvers need to be verified to fall back to PATH when TSUKU_HOME glob finds nothing | Advisory |

---

## 9. Recommendations

1. **Specify the dependency skip mechanism.** The design must describe how the executor knows to skip installing Rust when it's at `/opt/ecosystem/` instead of `$TSUKU_HOME/tools/rust-1.82.0/`. Options: (a) modify `installSingleDependency` to check both paths, (b) have the sandbox script create symlinks, or (c) have the foundation image builder create the expected directory structure. This is the single blocking finding.

2. **Revise the volume-mount rejection.** Acknowledge that host-to-container mounting at the same path avoids relocation issues. The real tradeoff is cache location (Docker image store vs host filesystem), not path incompatibility.

3. **Specify the container lifecycle for `docker commit`.** Current `runtime.Run()` uses `--rm`. Foundation image building needs a different lifecycle. Describe whether this is a new runtime method, a flag on `RunOptions`, or direct CLI invocation.

4. **Add a cleanup step before `docker commit`.** Remove temp files and package caches to reduce foundation image size.

5. **Clarify the speedup estimate in the summary.** The design's title problem is the 60-minute timeout, but the solution addresses only the toolchain+registry portion. State the expected savings clearly so implementers and reviewers calibrate expectations.
