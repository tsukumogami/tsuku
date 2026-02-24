---
status: Proposed
problem: |
  When testing cargo_build recipes across 5 Linux families, each family
  independently installs the same Rust toolchain and fetches the same cargo
  registry content. This redundant work adds 15-25 minutes per recipe test,
  frequently pushing CI jobs past the 60-minute timeout. The existing download
  and container image caches don't help because ecosystem toolchain installation
  happens at runtime in ephemeral containers that are destroyed after each run.
decision: |
  Introduce "foundation images" as a second level of container image caching.
  Foundation images extend the existing package images by pre-installing
  ecosystem toolchains at /opt/ecosystem/, outside the workspace mount that
  would shadow them. The sandbox script bridges pre-installed toolchains into
  the workspace via symlinks. Foundation images are built using the existing
  runtime.Build() infrastructure with generated Dockerfiles that run tsuku
  inside a RUN command. Separately, cargo registry content is shared across
  families via a read-write volume mount.
rationale: |
  Foundation images extend the proven container image cache pattern (deterministic
  hash, build-once-reuse-many) to a second level without introducing new
  mechanisms. Building via generated Dockerfiles reuses existing infrastructure,
  produces reproducible images, and keeps tsuku as the single installation
  authority. The symlink bridge solves workspace mount shadowing without modifying
  the mount strategy. We chose not to share compiled artifacts across families
  because different libc environments require independent compilation for
  correctness.
---

# DESIGN: Sandbox Build Cache

## Status

Proposed

## Context and Problem Statement

Tsuku's sandbox system tests recipes inside isolated containers, one per Linux family. When a recipe uses `cargo_build`, each family container independently:

1. Installs the Rust toolchain (~2-3 minutes)
2. Runs `cargo fetch` to populate the cargo registry with crate metadata and source tarballs (~1-2 minutes)
3. Compiles the entire dependency tree from scratch (~10-40 minutes for heavy crates)

For a single recipe tested across 5 families (debian, rhel, arch, suse, alpine), that's 5 independent Rust installations and 5 independent compilations of the same dependency tree. On CI runners with limited CPU, heavy crates like komac, cargo-nextest, or probe-rs-tools frequently hit the 60-minute job timeout even though a single-family build completes in 15-20 minutes.

The current caching in the sandbox has two levels:

- **Download cache**: Source tarballs and archives are fetched once and shared across families via a read-only volume mount at `/workspace/tsuku/cache/downloads`. This works well for avoiding redundant downloads.
- **Container image cache**: Images are keyed by a deterministic hash of (base image + system packages + repositories). The image `tsuku/sandbox-cache:debian-{hash}` is built once and reused. This avoids re-running `apt-get install` every time.

But neither level helps with what happens _inside_ the running container. The Rust toolchain installation, cargo registry population, and dependency compilation all happen at runtime in an ephemeral workspace that's destroyed when the container exits. There's no mechanism to carry forward the ecosystem setup from one sandbox run to the next.

The same pattern applies to other ecosystem build actions. An `npm_install` recipe installs Node.js on every family before running `npm install`. A future `pip_build` action would install Python on every family before running `pip install`. The problem generalizes beyond Rust.

### Scope

**In scope:**
- Caching ecosystem toolchain installations across sandbox runs for the same family
- Sharing platform-independent cargo registry content across families
- Integration with the existing container image cache in `container_spec.go`
- Working with both Docker and Podman runtimes

**Out of scope:**
- Sharing compiled artifacts (.rlib files) across families (different libc, different system libraries)
- Cross-architecture caching (x86_64 vs arm64 produce different toolchains)
- Persistent caching across CI runs (GitHub Actions cache integration is a separate concern)
- Changes to how `cargo_build.go` handles CARGO_HOME isolation within a single build

## Decision Drivers

- **Don't redo identical work**: If two sandbox runs need the same Rust version on the same family, the toolchain should be installed once
- **Preserve family isolation**: Final binaries must be compiled independently per family. Sharing compiled artifacts between different libc environments breaks correctness guarantees
- **Maintain determinism**: `SOURCE_DATE_EPOCH=0` and `CARGO_INCREMENTAL=0` must remain in effect. Any caching must not interfere with these flags
- **Follow existing patterns**: The download cache (read-only mount) and container image cache (hash-based naming) are proven patterns. New caching should feel similar
- **Runtime compatibility**: Must work with Docker and Podman. Both support image layer caching and volume mounts. Avoid BuildKit-specific features that Podman doesn't support
- **Incremental delivery**: The solution should be decomposable into independently useful pieces, not an all-or-nothing change

## Research Findings

### How Ecosystem Dependencies Flow Through the Sandbox

When the sandbox executor runs a plan for a cargo_build recipe, the plan's action dependencies include "rust" (declared in `cargo_build.go:18`). The plan generator resolves this to concrete installation steps for the Rust toolchain. The sandbox script then runs `tsuku install --plan plan.json --force`, which executes all steps sequentially: first the Rust installation, then the cargo build.

The container image built by `DeriveContainerSpec()` contains only system packages (curl, ca-certificates, build-essential, etc.). The Rust toolchain is installed at _runtime_ inside the ephemeral workspace. This is why the toolchain installation repeats on every run.

### Workspace Mount Shadowing

The sandbox mounts a fresh host temporary directory at `/workspace` (`executor.go:309-313`). This mount shadows anything the Docker image contains at that path. Since `TSUKU_HOME=/workspace/tsuku`, any toolchain pre-installed into the image at that path would be invisible to the running container.

This means we can't simply add Rust installation to the Dockerfile's RUN commands and expect it to persist. The workspace mount will shadow it. Any toolchain caching must either:
- Install to a path _outside_ `/workspace` (e.g., `/opt/ecosystem/`)
- Populate the workspace _after_ the mount is applied (e.g., copy or bind-mount)

### Docker and Podman Layer Caching

Both Docker and Podman cache image layers by content hash. Each `RUN` command in a Dockerfile creates a layer. If the command and all parent layers haven't changed, the layer is reused from cache. This is automatic and doesn't require special configuration.

`docker commit` snapshots a running container's filesystem as a new image layer. This works with both Docker and Podman and produces a reusable image. The approach is simpler than Dockerfile-based builds for capturing runtime state (like a toolchain installed by tsuku).

BuildKit-specific features like `RUN --mount=type=cache` provide persistent cache mounts between builds, but Podman's support for this is inconsistent across versions. The sandbox needs to work with both runtimes, so BuildKit-specific features should be avoided or used only as optimization.

### Cargo Registry Is Platform-Independent

`CARGO_HOME/registry/cache/` and `registry/src/` contain crate tarballs and extracted source. This content is identical regardless of which Linux family it was downloaded on. `cargo fetch` on debian downloads the exact same crate files as on alpine.

The cargo build action creates an isolated `CARGO_HOME` per build (`cargo_build.go:527`), then runs `cargo fetch --locked` to populate the registry. Sharing the registry content across families would eliminate redundant downloads. The pattern matches the existing download cache: mount a shared directory read-only, let each build use it.

However, sharing compiled dependencies (the `target/` directory) is unsafe across families. glibc and musl produce different binaries, and even between glibc families, differences in system library versions can affect compilation of crates with native dependencies (C bindings, sys crates).

### Existing Container Image Caching

`ContainerImageName()` in `container_spec.go:368` generates deterministic image names from a hash of (base image + packages + repositories). The executor checks if this image exists and builds it only if needed. This pattern can be extended to a second level of caching.

The current naming scheme: `tsuku/sandbox-cache:{family}-{hash16}`

A foundation image could follow: `tsuku/sandbox-foundation:{family}-{deps_hash16}`

### CI Workflow Patterns

The `test-recipe.yml` workflow already runs families in parallel per recipe and shares a download cache via symlink. Adding foundation image caching would slot into the existing flow: build foundation images once (potentially in a setup step), then reference them when running per-family sandbox tests.

## Considered Options

### Decision 1: How Should Ecosystem Toolchains Be Cached?

The Rust toolchain takes 2-3 minutes to install per family. For 5 families, that's 10-15 minutes of redundant work per recipe test. We need a way to install the toolchain once per family and reuse it across recipe runs that need the same ecosystem.

The challenge is the workspace mount shadowing described above: anything installed to `/workspace/tsuku/` in the Docker image is hidden by the ephemeral host mount. The caching mechanism must work around this.

#### Chosen: Foundation images with external toolchain path

Build a second level of cached Docker images ("foundation images") that contain ecosystem toolchains installed outside the workspace mount point. The foundation image extends the existing package image by adding the toolchain at `/opt/ecosystem/`.

The sandbox executor:
1. Analyzes the plan to identify ecosystem dependencies and their versions from `plan.Dependencies`
2. Computes a foundation image name: `tsuku/sandbox-foundation:{family}-{hash}` where hash includes package image hash + ecosystem deps
3. If the foundation image doesn't exist, builds it using the existing `runtime.Build()` method with a generated Dockerfile that COPYs tsuku into the image and runs `RUN TSUKU_HOME=/opt/ecosystem tsuku install <dep> --force`
4. Runs the recipe's sandbox from the foundation image instead of the package image
5. The sandbox script bridges the ecosystem deps into the workspace by symlinking `/opt/ecosystem/tools/*` into `/workspace/tsuku/tools/` and `/opt/ecosystem/bin/*` into `/workspace/tsuku/bin/`. This ensures the plan executor finds the pre-installed toolchain at `$TSUKU_HOME/tools/` and skips reinstallation

The Dockerfile approach reuses the existing `runtime.Build()` infrastructure (no new interface methods), produces reproducible images (rebuildable from the same Dockerfile), and keeps tsuku as the single source of truth for installation logic. The symlink bridge solves the workspace mount shadowing problem: the tools exist in the image at `/opt/ecosystem/` and appear at the expected `$TSUKU_HOME/tools/` path via symlinks created after the mount is applied.

#### Alternatives Considered

**Ecosystem installation via raw shell commands in Dockerfile**: Add ecosystem-specific RUN commands to the generated Dockerfile (e.g., `RUN curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh`). Rejected because this duplicates installation logic that tsuku already handles, creating a second code path that must be kept in sync. The chosen approach runs tsuku itself inside the RUN command, keeping it as the single source of truth.

**BuildKit cache mounts**: Use `RUN --mount=type=cache,target=/opt/ecosystem` in the generated Dockerfile to persist the toolchain across builds. Rejected because BuildKit cache mounts have inconsistent Podman support, are scoped to a single BuildKit instance (breaking CI), and their contents aren't included in `--cache-from`/`--cache-to` exports. The sandbox must work equally well with Docker and Podman.

**Volume-mounted toolchain from host**: Mount a host directory containing a pre-installed toolchain into containers as a read-only volume at `/opt/ecosystem/`, similar to the download cache pattern. This would work since the container path can match the installation path. Rejected because it requires pre-installing the toolchain on the host outside of the sandbox workflow, adds host state that must be managed separately from the container lifecycle, and doesn't travel with the image (a foundation image can be saved/loaded between hosts, while a volume mount requires the host directory to exist). For CI, maintaining host-level toolchain directories across ephemeral runners adds operational complexity that image-based caching avoids.

### Decision 2: How Should Cargo Registry Content Be Shared?

The cargo registry (`CARGO_HOME/registry/`) is populated by `cargo fetch` with crate metadata and source tarballs. This content is platform-independent but currently downloaded independently per family per build. We need to decide whether and how to share it.

#### Chosen: Shared cargo registry cache as a read-write volume

Mount a host directory at a well-known location inside the container. The first family's `cargo fetch` populates it, and subsequent families read from it. Unlike the download cache (read-only), this needs to be read-write so cargo can add index metadata and extracted sources.

The cargo registry cache lives at `$TSUKU_HOME/cache/cargo-registry/` on the host and is mounted into the container. The `buildDeterministicCargoEnv()` function in `cargo_build.go` would set `CARGO_HOME` to point at this shared location's parent, or more precisely, create the isolated `.cargo-home` with a symlink from `registry/` to the shared mount.

#### Alternatives Considered

**Don't share registry content**: Let each family run its own `cargo fetch`. Rejected because the registry download takes 1-2 minutes per family and the content is identical. For 5 families that's 5-10 minutes of redundant network I/O per recipe. The download cache already proves that sharing read-only data across families works reliably.

**Share the entire CARGO_HOME**: Instead of just the registry, share the full `CARGO_HOME` including compiled artifacts. Rejected because the `target/` build cache is platform-specific (different libc produce different artifacts), and sharing it could cause subtle build failures or incorrect linking. Only the registry content is safe to share.

### Decision 3: Scope of Foundation Image Support

Foundation images solve the ecosystem caching problem. But should this be specific to Rust/cargo, or should it handle any ecosystem toolchain?

#### Chosen: General foundation image mechanism, cargo_build as first consumer

The foundation image system should be ecosystem-agnostic. It works by:
1. Extracting `ActionDependencies.InstallTime` from the plan's actions
2. Building a foundation image that has those dependencies pre-installed
3. Running the rest of the plan from that foundation image

The mechanism doesn't need to know anything about Rust specifically. Any action that declares install-time dependencies (cargo_build declares "rust", npm_install could declare "nodejs") benefits automatically. Cargo_build is the first and most impactful consumer, but the architecture doesn't need cargo-specific code.

#### Alternatives Considered

**Cargo-specific caching**: Add Rust-specific caching logic to `cargo_build.go` that pre-installs Rust before the sandbox run. Rejected because the problem isn't specific to Rust. Any ecosystem toolchain installation repeats across families. A general mechanism handles current and future ecosystem actions without per-ecosystem code.

## Decision Outcome

**Chosen: Foundation images + shared cargo registry**

### Summary

The sandbox gets a second level of container image caching called "foundation images." These sit between the existing package images (base OS + system packages) and the ephemeral per-recipe sandbox execution. A foundation image contains ecosystem toolchains (like Rust) pre-installed at `/opt/ecosystem/`, outside the workspace mount point that would otherwise shadow them.

When the sandbox executor prepares a plan for execution, it reads `plan.Dependencies` to identify what ecosystem toolchains the plan needs and their resolved versions. It computes a deterministic hash from the package image plus the sorted dependency list, forming a foundation image name like `tsuku/sandbox-foundation:debian-rust1820-{hash16}`. If that image exists, the sandbox starts from it directly. If not, the executor builds it using `runtime.Build()` with a generated Dockerfile. The Dockerfile copies the tsuku binary into the image and runs `RUN TSUKU_HOME=/opt/ecosystem tsuku install <dep> --force` for each dependency. This reuses the existing image build infrastructure and produces reproducible images.

The sandbox script bridges the pre-installed toolchain into the workspace by symlinking `/opt/ecosystem/tools/*` into `/workspace/tsuku/tools/` and `/opt/ecosystem/bin/*` into `/workspace/tsuku/bin/`. These symlinks are created after the workspace mount is applied, so they point from the mounted workspace into the image's filesystem. When tsuku processes the plan's dependency installation steps, it finds the toolchain at `$TSUKU_HOME/tools/rust-1.82.0/` via the symlink and skips reinstallation. The actual build work (cargo fetch, cargo build) runs normally from the pre-installed toolchain.

Separately, `CARGO_HOME/registry/` content is shared across families via a read-write volume mount. The first family's `cargo fetch` populates the shared cache, and subsequent families find the crate metadata and source tarballs already present. This eliminates redundant network I/O for registry content that's identical across families.

Neither caching level shares compiled artifacts between families. Each family still compiles its own dependency tree independently, preserving the isolation guarantees that different libc environments require.

### Rationale

Foundation images follow the same pattern as the existing package image cache: compute a deterministic hash, check if the image exists, build only if missing. This makes the second cache level feel like a natural extension rather than a new concept. Building foundation images via `runtime.Build()` with a generated Dockerfile reuses existing infrastructure and produces reproducible images. Running tsuku inside the Dockerfile's RUN command keeps tsuku as the single source of truth for installation logic without duplicating it.

Installing to `/opt/ecosystem/` with symlink bridging into the workspace solves the mount shadowing problem cleanly. The symlinks are cheap to create, and because they're created after the workspace mount, they correctly point from the mounted workspace into the image's pre-installed content. This works identically on Docker and Podman.

The cargo registry cache is a simpler version of the download cache pattern. Since registry content is platform-independent, sharing it across families is safe and eliminates the most wasteful duplication (the same crates downloaded 5 times).

## Solution Architecture

### Overview

The architecture adds two caching layers to the existing sandbox execution pipeline:

```
container-images.json
    |
    v
Package Image (tsuku/sandbox-cache:debian-{hash})
    |  -- cached by hash of (base image + packages)
    v
Foundation Image (tsuku/sandbox-foundation:debian-{hash})
    |  -- cached by hash of (package image + ecosystem deps)
    v
Sandbox Execution
    |  -- cargo registry shared via volume mount
    v
Recipe Build Output
```

### Components

**Plan analysis** (`internal/sandbox/foundation.go`, new file): Reads `plan.Dependencies` to extract ecosystem dependencies and their resolved versions. The plan structure already contains `Tool` and `Version` fields for each dependency, so no action registry lookup is needed. Returns a sorted list of (dependency, version) pairs as input to the foundation image hash.

**Foundation image builder** (`internal/sandbox/foundation.go`): Builds foundation images when they don't exist in the local image cache. The build process:
1. Creates a temporary build context directory containing the tsuku binary
2. Generates a Dockerfile: `FROM <package_image>`, `COPY tsuku /usr/local/bin/tsuku`, `RUN TSUKU_HOME=/opt/ecosystem tsuku install <dep> --force && rm /usr/local/bin/tsuku`
3. Calls `runtime.Build()` with the generated Dockerfile and build context
4. Returns the foundation image name

The `RUN` command installs the toolchain to `/opt/ecosystem/` and cleans up the tsuku binary (it's not needed in the foundation image). Adding a cleanup step in the same `RUN` command keeps the layer size minimal.

**Foundation image naming** (`internal/sandbox/foundation.go`): Generates deterministic names following the existing `ContainerImageName()` pattern. Hash input: package image name + sorted list of dependency-version pairs. Format: `tsuku/sandbox-foundation:{family}-{hash16}`.

**Updated sandbox executor** (`internal/sandbox/executor.go`): After building/finding the package image, checks for ecosystem dependencies. If present, builds/finds the foundation image and uses it as the container image for the sandbox run. If no ecosystem dependencies, behavior is unchanged.

**Updated sandbox script** (`internal/sandbox/executor.go:buildSandboxScript`): When running from a foundation image, adds a bridge step before the plan execution. The bridge creates symlinks from `/opt/ecosystem/tools/*` into `/workspace/tsuku/tools/` and from `/opt/ecosystem/bin/*` into `/workspace/tsuku/bin/`. It also prepends `/opt/ecosystem/bin` to PATH as a fallback for any tools that ecosystem resolvers (like `ResolveCargo()`) look up via PATH. The plan's dependency installation steps then find the pre-installed toolchain at `$TSUKU_HOME/tools/` via the symlinks and skip reinstallation.

**Cargo registry cache** (`internal/actions/cargo_build.go`): If a cargo registry cache directory exists at `$TSUKU_HOME/cache/cargo-registry/`, `buildDeterministicCargoEnv()` creates a symlink from `CARGO_HOME/registry` to the shared location. Otherwise, falls back to the current behavior (isolated per-build registry).

**Cargo registry mount** (`internal/sandbox/executor.go`): Adds an optional volume mount for the cargo registry cache, similar to the download cache mount. Mounted at `/workspace/tsuku/cache/cargo-registry/` with read-write access.

### Key Interfaces

```go
// foundation.go

// EcosystemDep represents an ecosystem toolchain dependency.
type EcosystemDep struct {
    Name    string // e.g., "rust"
    Version string // e.g., "1.82.0"
}

// ExtractEcosystemDeps analyzes a plan and returns its ecosystem dependencies.
func ExtractEcosystemDeps(plan *executor.InstallationPlan) []EcosystemDep

// FoundationImageName generates a deterministic name for a foundation image.
func FoundationImageName(packageImage string, deps []EcosystemDep) string

// BuildFoundationImage builds a foundation image with ecosystem deps pre-installed.
// Returns the image name.
func (e *Executor) BuildFoundationImage(
    ctx context.Context,
    runtime validate.Runtime,
    packageImage string,
    deps []EcosystemDep,
) (string, error)
```

### Data Flow

1. Plan generation produces a plan with actions and resolved dependencies
2. Sandbox executor calls `ExtractEcosystemDeps(plan)` to read ecosystem deps from `plan.Dependencies`
3. If deps exist, `FoundationImageName(packageImage, deps)` computes the cache key
4. `runtime.ImageExists()` checks for a cached foundation image
5. If missing, `BuildFoundationImage()` creates it via `runtime.Build()` with a generated Dockerfile
6. Sandbox runs from the foundation image; the sandbox script symlinks `/opt/ecosystem/tools/*` into `/workspace/tsuku/tools/`
7. Plan execution finds toolchain at `$TSUKU_HOME/tools/` and skips already-installed dependencies
8. `cargo_build` uses shared registry cache if available; `ResolveCargo()` finds cargo via PATH fallback

## Implementation Approach

### Phase 1: Foundation Image Infrastructure

Build the core foundation image mechanism without cargo-specific changes.

- Create `internal/sandbox/foundation.go` with `ExtractEcosystemDeps`, `FoundationImageName`, `BuildFoundationImage`
- Update `Executor.Sandbox()` to check for ecosystem deps and build/use foundation images
- Update `buildSandboxScript()` to add the symlink bridge step when ecosystem deps exist (symlinks from `/opt/ecosystem/tools/*` to `/workspace/tsuku/tools/`, plus PATH prepend)
- Extend `runtime.Build()` to accept a build context directory (currently uses `.` as context; needs to support a temp dir containing the tsuku binary)
- Add unit tests for plan analysis and image naming
- Add integration test that builds a foundation image and verifies toolchain presence via symlink bridge

### Phase 2: Cargo Registry Cache

Share cargo registry content across families within a single host.

- Add `WithCargoRegistryCacheDir()` option to `Executor`
- Mount cargo registry cache directory into containers at `/workspace/tsuku/cache/cargo-registry/`
- Update `buildDeterministicCargoEnv()` to use shared registry when available
- Update `test-recipe.yml` to create and share a cargo registry cache directory across families
- Add test for registry sharing correctness

### Phase 3: CI Integration

Wire foundation images into the CI workflow.

- Update `test-recipe.yml` to build foundation images in a setup step before per-family testing
- Add cleanup logic to prune stale foundation images (images whose ecosystem version is outdated)
- Document the caching behavior for contributors

Phase 1 delivers the largest speedup (eliminating redundant toolchain installation). Phase 2 adds incremental improvement (eliminating redundant cargo fetch). Phase 3 makes it work in CI. Each phase is independently useful.

## Security Considerations

### Download Verification

Foundation images contain toolchains installed by tsuku, which applies the same download verification as non-sandbox installations. When building a foundation image, tsuku downloads the Rust toolchain using the same checksums and verification logic as a normal `tsuku install rust`. The foundation image captures the verified, installed state. No additional download verification is needed because the same verification code path runs during foundation image construction.

The cargo registry cache contains crate source fetched by cargo with `--locked`, which verifies against `Cargo.lock` checksums. Sharing the registry doesn't bypass this verification because cargo checks checksums when reading from the registry, not just when downloading.

### Execution Isolation

Foundation images run in the same container isolation as current sandbox images. The container's security boundary (network restrictions, resource limits, read-only mounts) is unchanged. The toolchain at `/opt/ecosystem/` is baked into the image, not mounted from the host, so it can't be modified at runtime.

The cargo registry mount is read-write, which is a weaker guarantee than the download cache (read-only). A malicious cargo build in one family could theoretically modify the shared registry to affect subsequent families. Mitigation: the registry mount is shared only across families for the _same recipe_ within a single sandbox invocation. Cross-recipe isolation is maintained because each recipe gets a fresh TSUKU_HOME with a fresh registry symlink.

### Supply Chain Risks

Foundation images are built locally from the same source images used by the package cache. They don't introduce new external dependencies. The `runtime.Build()` operation runs a generated Dockerfile that installs ecosystem deps using tsuku itself. The tsuku binary is copied into the build context temporarily and removed in the same RUN layer.

The cargo registry cache creates a new sharing surface: crate content fetched by one family is used by others. Since cargo verifies checksums from `Cargo.lock` when reading registry content, a tampered registry entry would fail cargo's own verification. The risk is that a compromised crate in the _first_ family's fetch would propagate to other families, but this is the same risk as not sharing (the same `Cargo.lock` produces the same fetches regardless of order).

Foundation image names use deterministic hashing, so an attacker who can modify the hash inputs (ecosystem dep versions) could cause a different image to be used. This requires modifying the plan, which means compromising tsuku's plan generation.

### User Data Exposure

Not applicable. Foundation images contain only ecosystem toolchains (compiler binaries, standard libraries). The cargo registry cache contains open-source crate source code downloaded from crates.io. No user-specific data is stored in either cache.

## Consequences

### Positive

- Ecosystem toolchain installation (Rust, Node.js, etc.) happens once per family instead of once per recipe per family, saving 2-3 minutes per eliminated installation
- Cargo registry content is fetched once instead of 5 times per recipe, saving 1-2 minutes per recipe test
- The pattern generalizes to any ecosystem build action that declares install-time dependencies
- Foundation images follow the existing cache pattern, so the mental model for developers stays consistent
- No changes to determinism guarantees: `SOURCE_DATE_EPOCH=0` and `CARGO_INCREMENTAL=0` remain in effect for the actual build

### Negative

- Foundation images consume disk space. Each ecosystem + family combination produces an image that includes the full toolchain (~500MB for Rust). With 5 families, that's ~2.5GB per ecosystem version. Mitigation: stale images are pruned when the ecosystem version changes, and `docker system prune` handles cleanup.
- Extending `runtime.Build()` to accept a build context directory is a non-trivial change to the runtime interface. The current implementation uses `.` as the build context. Mitigation: the change is backward-compatible (default to `.` when no context directory is specified), and foundation images are the only consumer of this feature initially.
- Two-phase execution adds complexity to the sandbox executor. There are now three possible image levels (base, package, foundation) instead of two. Mitigation: foundation images are opt-in (only used when ecosystem deps exist), so simple recipes that don't need build toolchains see no change.
- The cargo registry mount is read-write, which is a weaker isolation guarantee than the read-only download cache mount. Mitigation: the mount is scoped to a single recipe's multi-family run, not shared across recipes.
- The speedup addresses toolchain installation (~10-15 min across 5 families) and registry fetching (~5-10 min), but compilation time (10-40 min per family) remains unaddressed. For the heaviest crates, compilation is still the dominant cost. This design reduces total CI time significantly but doesn't eliminate the timeout risk for the most expensive builds.

### Mitigations

| Risk | Mitigation |
|------|------------|
| Disk usage from foundation images | Prune images when ecosystem version changes; defer to `docker system prune` for manual cleanup |
| Foundation image loss | Automatic rebuild on demand via `runtime.Build()`; only costs time |
| Executor complexity | Foundation images are opt-in; no-op when no ecosystem deps exist |
| Cargo registry write access | Scoped to single recipe run; cargo verifies checksums on read |
| Partial cargo fetch failure | Cargo handles incomplete registries by fetching missing entries; subsequent families retry failed downloads |
| Runtime compatibility | Uses `runtime.Build()` (Dockerfile-based), supported by both Docker and Podman; avoids BuildKit-specific features |
