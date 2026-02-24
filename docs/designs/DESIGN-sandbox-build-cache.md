---
status: Proposed
problem: |
  When testing recipes across Linux families, each family independently installs
  the same ecosystem toolchains (Rust, Node.js, etc.) inside ephemeral containers
  that are destroyed after each run. There's no mechanism to carry forward this
  work. The plan already contains a structured dependency tree with resolved
  versions, but nothing maps this tree to reusable container image layers.
decision: |
  Map plan dependency layers to Docker image layers. Each InstallTime dependency
  in the plan becomes a separate RUN command in a generated Dockerfile, installed
  in canonical order. Docker's native layer caching handles cross-recipe reuse
  automatically: two recipes that both need Rust 1.82.0 share the same cached
  layer. Dependencies are installed at /opt/ecosystem/ to avoid workspace mount
  shadowing, with symlinks bridging them into the workspace at runtime.
rationale: |
  Docker already solves the "don't redo identical work" problem through layer
  caching. Rather than building a custom cache management system, we generate
  Dockerfiles where each plan dependency is a separate RUN command and let
  Docker/Podman handle reuse. Canonical ordering of dependencies maximizes
  the shared prefix between recipes. Using tsuku itself inside RUN commands
  keeps it as the single installation authority. This works dynamically on
  developer machines -- no pre-built images or registry infrastructure needed.
---

# DESIGN: Sandbox Build Cache

## Status

Proposed

## Context and Problem Statement

Tsuku's sandbox runs recipe installations inside isolated containers, one per Linux family. When a recipe uses `cargo_build`, each family container independently:

1. Installs the Rust toolchain (~2-3 minutes)
2. Runs `cargo fetch` to populate the cargo registry (~1-2 minutes)
3. Compiles the entire dependency tree (~10-40 minutes for heavy crates)

For 5 families, that's 5 independent Rust installations and 5 compilations. On CI, heavy crates frequently hit the 60-minute job timeout.

The sandbox currently caches at two levels:

- **Download cache**: Artifacts fetched once, shared across families via read-only volume mount
- **Container image cache**: Images keyed by hash of (base image + system packages), built once per unique package set

Neither level helps with what happens at runtime. The Rust toolchain, cargo registry, and compilation all happen inside ephemeral containers. There's no mechanism to reuse ecosystem setup across sandbox runs.

### How the sandbox works

The sandbox is always invoked with a pre-resolved plan. Even when a user runs `tsuku install --sandbox cargo-nextest` without providing a plan, tsuku generates the plan on the host first (resolving versions, downloading artifacts, computing checksums), then passes the resolved plan into the container. The container runs `tsuku install --plan plan.json --force` -- it never calls version providers or resolves anything.

The plan has a tree structure. `InstallationPlan.Dependencies` contains `DependencyPlan` entries, each with its own `Steps` and potentially nested `Dependencies`. For a cargo_build recipe, the plan typically looks like:

```
InstallationPlan (cargo-nextest v0.24.5)
  Dependencies:
    [0] DependencyPlan (rust v1.82.0)
         Steps: [download, extract, install_binaries, ...]
  Steps: [cargo_build ...]
```

This tree structure maps naturally to Docker image layers. Each dependency in the tree is work that could be cached and reused by other recipes that need the same dependency at the same version.

### Dependency categories

Tsuku has three dependency categories (`ActionDeps` in `actions/action.go`):

- **EvalTime**: Needed during plan generation (host-side, for `Decompose()`). Not relevant to container caching.
- **InstallTime**: Needed during execution. These become `DependencyPlan` entries in the plan tree. This is what should be cached as Docker layers.
- **Runtime**: Tracked but not installed by tsuku during installation. Not relevant here.

### Scope

**In scope:**
- Mapping InstallTime dependencies from the plan to Docker image layers
- Cross-recipe layer sharing via Docker's native caching
- Dynamic operation on developer machines (no pre-built images or registry)
- Working with both Docker and Podman

**Out of scope:**
- Sharing compiled artifacts (.rlib files) across families
- Cross-architecture caching (x86_64 vs arm64)
- Changes to how `cargo_build.go` handles CARGO_HOME isolation

## Decision Drivers

- **Map plan structure to cache structure**: The plan already describes what needs to be installed and in what order. The caching mechanism should mirror this structure
- **Docker-native**: Use Docker/Podman's layer caching rather than building a parallel cache system
- **Dynamic, user-machine operation**: A developer running `tsuku install --sandbox` should benefit from cached layers from previous runs without any setup
- **Cross-recipe reuse**: If recipe A and recipe B both need Rust 1.82.0, the Rust layer should be shared automatically
- **Preserve family isolation**: Each family builds independently. No sharing of compiled artifacts between different libc environments
- **CI adapts to the format**: Design the caching for local usage; CI workflows restructure to take advantage of it

## Research Findings

### Workspace Mount Shadowing

The sandbox mounts a fresh host directory at `/workspace` (`executor.go:309-313`). Since `TSUKU_HOME=/workspace/tsuku`, anything the Docker image contains at that path is hidden. Dependencies cached in the image must be installed outside `/workspace` (e.g., `/opt/ecosystem/`) and bridged into the workspace via symlinks at runtime.

### Docker Layer Caching Mechanics

Each `RUN` command in a Dockerfile creates a layer. Docker caches a layer when (parent layer + command text) haven't changed. Layers are position-sensitive: layer N is cached only if layers 0..N-1 are identical to a previous build. This means dependency ordering matters -- recipes must install dependencies in the same canonical order to maximize shared prefixes.

Both Docker and Podman support this layer caching natively. No BuildKit-specific features are needed.

### Plan Dependencies Are Self-Contained

Each `DependencyPlan` in the plan tree contains fully resolved `Steps` with concrete URLs, versions, and checksums. A DependencyPlan can be converted to a standalone `InstallationPlan` and passed to `tsuku install --plan`. This means each dependency can be installed independently via a Dockerfile `RUN` command that receives its plan as input.

### Canonical Ordering Enables Layer Sharing

If we always install dependencies in the same order across all recipes, Docker's layer caching maximizes reuse. Two recipes with different dependency sets but a common prefix share all layers up to where they diverge.

Consider three recipes:
- Recipe A needs: `[rust@1.82.0]`
- Recipe B needs: `[nodejs@22, rust@1.82.0]`
- Recipe C needs: `[openssl@3.0, rust@1.82.0]`

With alphabetical ordering: nodejs < openssl < rust. So:
- Recipe A's layers: `[..., nodejs@22]` -- wait, A doesn't need nodejs

Actually, only the dependencies each recipe declares appear in its Dockerfile. The key insight: recipes with identical dependency lists share all layers. Recipes that share a prefix of the sorted dependency list share those prefix layers.

For the most common case (single dependency like "rust"), all cargo_build recipes share the same Rust layer.

## Considered Options

### Decision 1: How Should Plan Dependencies Map to Docker Layers?

The plan's `Dependencies` tree contains the work that repeats across recipes. We need to decide how to convert this tree into cacheable Docker image layers.

#### Chosen: One RUN command per dependency in canonical order

Flatten the plan's dependency tree (DFS: install transitive deps before their dependents), sort the result canonically, and generate a Dockerfile where each dependency is a separate `RUN` command. Docker's layer caching handles reuse automatically.

Generated Dockerfile for a recipe that needs Rust 1.82.0 (which itself depends on nothing):

```dockerfile
FROM tsuku/sandbox-cache:debian-{pkg_hash}
COPY tsuku /usr/local/bin/tsuku
COPY plans/dep-00-rust.json /tmp/plans/
ENV TSUKU_HOME=/opt/ecosystem
ENV PATH=/opt/ecosystem/bin:$PATH
RUN tsuku install --plan /tmp/plans/dep-00-rust.json --force
RUN rm -rf /usr/local/bin/tsuku /tmp/plans
```

For a recipe that needs Rust and OpenSSL:

```dockerfile
FROM tsuku/sandbox-cache:debian-{pkg_hash}
COPY tsuku /usr/local/bin/tsuku
COPY plans/dep-00-openssl.json /tmp/plans/
COPY plans/dep-01-rust.json /tmp/plans/
ENV TSUKU_HOME=/opt/ecosystem
ENV PATH=/opt/ecosystem/bin:$PATH
RUN tsuku install --plan /tmp/plans/dep-00-openssl.json --force
RUN tsuku install --plan /tmp/plans/dep-01-rust.json --force
RUN rm -rf /usr/local/bin/tsuku /tmp/plans
```

Docker caches each layer based on (parent + command + COPY content). If the rust plan JSON is identical between recipes (same version, same URLs, same checksums), the COPY and RUN layers are cached. Recipes with the same dependency prefix share those layers.

The tsuku binary and plan files are copied into the build context and cleaned up in the final layer. Each `RUN` command installs one dependency to `$TSUKU_HOME=/opt/ecosystem`, where it persists across layers. Subsequent RUN commands see tools installed by previous layers.

#### Alternatives Considered

**Single foundation image with all deps**: Install all dependencies in one `RUN` command, producing a single foundation image per unique dependency set. Rejected because this prevents layer sharing between recipes with partially overlapping dependency sets. Recipe A needing `[rust]` and recipe B needing `[openssl, rust]` would produce two separate images with no shared layers.

**Docker commit after runtime installation**: Run the plan's dependencies inside a container, then `docker commit` to snapshot the state. Rejected because `runtime.Run()` passes `--rm` (containers are cleaned up automatically), requiring a new container lifecycle. The Dockerfile approach reuses existing `runtime.Build()` infrastructure. Commit-based images are also not reproducible from a Dockerfile.

**BuildKit cache mounts**: Use `RUN --mount=type=cache` for persistent directories across builds. Rejected because Podman support is inconsistent, cache mounts are scoped to a single BuildKit instance (not shared across CI runners), and their contents aren't included in `--cache-from`/`--cache-to` exports.

### Decision 2: How Should Dependencies Be Ordered?

Docker layer caching is position-sensitive. Layer N is cached only if layers 0..N-1 match. The order in which we install dependencies determines which recipes share layers.

#### Chosen: Alphabetical by dependency name

Sort dependencies alphabetically by tool name. This is deterministic, easy to understand, and produces consistent ordering regardless of which recipe triggered the build.

Within the flattened dependency tree, transitive dependencies sort independently. If rust depends on llvm, the flattened list might be `[llvm, rust]` (alphabetical). This is stable because it doesn't depend on tree structure.

For the most common case (single ecosystem dep like "rust"), all cargo_build recipes produce identical Dockerfiles and share all layers.

#### Alternatives Considered

**Frequency-based ordering**: Sort by how frequently each dependency appears across all recipes (most common first). Rejected because it requires global knowledge of all recipes and changes as the recipe registry grows. Alphabetical is stateless and deterministic.

**Tree-structure ordering**: Preserve the plan's dependency tree order (DFS). Rejected because different recipes might resolve the same dependencies in different tree positions, producing different Dockerfiles for the same logical content.

### Decision 3: How Should the Workspace Bridge Work?

Dependencies are installed at `/opt/ecosystem/` inside the Docker image, but the plan executor expects them at `$TSUKU_HOME/tools/` which is under the mounted workspace. We need a bridge.

#### Chosen: Symlink bridge in sandbox script

The sandbox script creates symlinks before executing the plan:

```sh
# Bridge pre-installed ecosystem deps into workspace
if [ -d /opt/ecosystem/tools ]; then
  for tool_dir in /opt/ecosystem/tools/*/; do
    tool_name=$(basename "$tool_dir")
    ln -sf "$tool_dir" "/workspace/tsuku/tools/$tool_name"
  done
  for bin in /opt/ecosystem/bin/*; do
    [ -f "$bin" ] && ln -sf "$bin" "/workspace/tsuku/bin/$(basename "$bin")"
  done
fi
```

The plan executor finds tools at `$TSUKU_HOME/tools/rust-1.82.0/` (via symlink) and skips reinstallation. `ResolveCargo()` and similar functions find binaries via PATH (which includes `/opt/ecosystem/bin/`) or by scanning `$TSUKU_HOME/tools/*/bin/`.

#### Alternatives Considered

**Modify plan executor to check /opt/ecosystem/**: Add a fallback path to the dependency installation logic. Rejected because it couples the executor to the sandbox's filesystem layout. The symlink bridge is transparent to the executor.

**Bind-mount individual tool directories**: Instead of symlinking, mount each `/opt/ecosystem/tools/<name>` at `/workspace/tsuku/tools/<name>`. Rejected because it requires knowing which tools exist at container creation time (before the script runs) and adds mount complexity for each dependency.

## Decision Outcome

**Chosen: Per-dependency Docker layers with canonical ordering**

### Summary

Each InstallTime dependency in the plan becomes a separate `RUN` command in a generated Dockerfile. The Dockerfile starts from the existing package image and installs dependencies one at a time, sorted alphabetically, with `TSUKU_HOME=/opt/ecosystem`. Docker's native layer caching handles cross-recipe reuse: two recipes that both need Rust 1.82.0 on debian share the exact same Rust layer because the COPY content (plan JSON) and RUN command are identical.

The dependency plans are extracted from the parent plan's `Dependencies` tree, flattened via DFS traversal, deduplicated, and sorted alphabetically. Each is converted to a standalone `InstallationPlan` JSON and copied into the Docker build context. The tsuku binary is also copied in and removed after installation.

At sandbox runtime, the script creates symlinks from `/opt/ecosystem/tools/*` into `/workspace/tsuku/tools/` before executing the recipe's plan. The plan executor discovers pre-installed dependencies via these symlinks and skips their installation steps.

This approach is fully dynamic. On a developer's machine, the first `tsuku install --sandbox cargo-nextest` builds the Rust layer (~2-3 minutes). Every subsequent cargo_build sandbox run that needs the same Rust version finds the layer cached and starts instantly. No pre-built images, no registry, no external infrastructure. CI benefits the same way: within a job that tests multiple recipes, all recipes after the first share cached dependency layers.

The speedup targets toolchain installation and registry fetching (saving ~15-25 minutes across 5 families per recipe). Compilation time (10-40 min per family) remains unaddressed -- each family still compiles independently. For the heaviest crates, compilation remains the dominant cost.

### Rationale

Docker already solves the "don't redo identical work" problem through layer caching. Instead of building a parallel caching system, we generate Dockerfiles that expose the plan's dependency structure as Docker layers. This is a thin mapping layer, not a new caching system.

Canonical ordering ensures that recipes with different dependency sets still share layers for their common dependencies. Alphabetical sorting is simple, deterministic, and doesn't require global knowledge.

Using `tsuku install --plan` inside the RUN commands keeps tsuku as the single authority for how tools are installed. The plan JSON is the cache key: same plan content produces the same Docker layer, and Docker handles invalidation when the plan changes (new version, different checksums).

## Solution Architecture

### Overview

```
Plan Generation (host)
    |
    v
Extract Dependencies from plan.Dependencies
    |
    v
Flatten + Sort + Generate Standalone Plans
    |
    v
Generate Dockerfile (one RUN per dep)
    |
    v
docker build (layer caching handles reuse)
    |
    v
Foundation Image (all deps pre-installed at /opt/ecosystem/)
    |
    v
Sandbox Execution (symlink bridge + recipe plan)
    |
    v
Recipe Build Output
```

### Dependency Extraction and Flattening

The plan's `Dependencies` field is a tree of `DependencyPlan` entries. The extraction process:

1. **DFS traversal**: Walk the tree depth-first, collecting all dependencies with their resolved steps
2. **Deduplication**: If the same tool appears multiple times (shared transitive dep), keep the first occurrence
3. **Sort**: Alphabetically by tool name
4. **Convert**: Each `DependencyPlan` becomes a standalone `InstallationPlan` JSON (with Steps but no nested Dependencies, since transitive deps are handled by earlier layers)

### Dockerfile Generation

The generated Dockerfile for a debian recipe needing Rust:

```dockerfile
FROM tsuku/sandbox-cache:debian-{pkg_hash}
COPY tsuku /usr/local/bin/tsuku
COPY plans/ /tmp/plans/
ENV TSUKU_HOME=/opt/ecosystem
ENV PATH=/opt/ecosystem/bin:$PATH
RUN tsuku install --plan /tmp/plans/dep-00-rust.json --force
RUN rm -rf /usr/local/bin/tsuku /tmp/plans
```

The build context directory contains:
```
context/
  Dockerfile
  tsuku              (binary, from host)
  plans/
    dep-00-rust.json (standalone plan for rust)
```

### Image Naming

Foundation images are tagged for quick existence checks:

```
tsuku/sandbox-foundation:{family}-{hash16}
```

The hash is computed from the generated Dockerfile content (which encodes the full dependency chain). If the Dockerfile is identical, the image tag is identical. This serves as a fast lookup before running `docker build`.

### Sandbox Script Bridge

When ecosystem dependencies exist, the sandbox script adds a bridge step:

```sh
#!/bin/sh
set -e

# Bridge pre-installed ecosystem deps into workspace
if [ -d /opt/ecosystem/tools ]; then
  for tool_dir in /opt/ecosystem/tools/*/; do
    tool_name=$(basename "$tool_dir")
    ln -sf "$tool_dir" "/workspace/tsuku/tools/$tool_name"
  done
  for bin in /opt/ecosystem/bin/*; do
    [ -f "$bin" ] && ln -sf "$bin" "/workspace/tsuku/bin/$(basename "$bin")"
  done
fi

# Setup TSUKU_HOME
mkdir -p /workspace/tsuku/recipes
mkdir -p /workspace/tsuku/bin
mkdir -p /workspace/tsuku/tools

export PATH=/workspace/tsuku/bin:/opt/ecosystem/bin:$PATH

# Run recipe plan (deps already installed via image layers)
tsuku install --plan /workspace/plan.json --force
```

### Key Interfaces

```go
// foundation.go

// FlatDep represents a flattened dependency with its standalone plan.
type FlatDep struct {
    Tool    string                      // e.g., "rust"
    Version string                      // e.g., "1.82.0"
    Plan    *executor.InstallationPlan  // standalone plan (steps only, no nested deps)
}

// FlattenDependencies extracts and flattens the dependency tree from a plan.
// Returns dependencies in canonical (alphabetical) order.
func FlattenDependencies(plan *executor.InstallationPlan) []FlatDep

// GenerateFoundationDockerfile creates a Dockerfile for the dependency chain.
func GenerateFoundationDockerfile(packageImage string, deps []FlatDep) string

// FoundationImageName returns the image tag based on Dockerfile content hash.
func FoundationImageName(family string, dockerfile string) string

// BuildFoundationImage builds the foundation image if it doesn't exist.
// Creates a temp build context with the tsuku binary and dependency plans,
// then calls runtime.Build().
func (e *Executor) BuildFoundationImage(
    ctx context.Context,
    runtime validate.Runtime,
    packageImage string,
    family string,
    deps []FlatDep,
) (string, error)
```

### Data Flow

1. Plan generation on host resolves all versions and produces `InstallationPlan`
2. `FlattenDependencies(plan)` extracts the dependency tree into a sorted flat list
3. If no deps, proceed with current behavior (no foundation image)
4. `GenerateFoundationDockerfile()` creates the Dockerfile with per-dep RUN commands
5. `FoundationImageName()` hashes the Dockerfile to produce the image tag
6. `runtime.ImageExists()` checks if the image is cached
7. If not cached, `BuildFoundationImage()` creates build context and calls `runtime.Build()`
8. Docker layer caching means only new/changed dependencies are actually built
9. Sandbox runs from the foundation image; symlink bridge connects deps to workspace
10. Plan executor finds deps at `$TSUKU_HOME/tools/` via symlinks, skips reinstallation

## Implementation Approach

### Phase 1: Per-Dependency Foundation Images

Core mechanism: plan dependencies become Docker image layers.

- Create `internal/sandbox/foundation.go` with `FlattenDependencies`, `GenerateFoundationDockerfile`, `FoundationImageName`, `BuildFoundationImage`
- `FlattenDependencies` does DFS traversal of `plan.Dependencies`, deduplicates, sorts alphabetically, converts each to standalone `InstallationPlan` JSON
- `GenerateFoundationDockerfile` produces the Dockerfile with COPY + ENV + per-dep RUN + cleanup
- Extend `runtime.Build()` to accept a build context directory (currently uses `.` as context)
- Update `Executor.Sandbox()` to build/use foundation images when plan has dependencies
- Update `buildSandboxScript()` to add the symlink bridge step
- Unit tests for dependency flattening, Dockerfile generation, image naming
- Integration test: build foundation image for a recipe with Rust dep, verify cargo is available via symlink

### Phase 2: Cargo Registry Cache

Share cargo registry content across families within a single host.

- Add `WithCargoRegistryCacheDir()` option to `Executor`
- Mount cargo registry cache at `/workspace/tsuku/cache/cargo-registry/` (read-write)
- Update `buildDeterministicCargoEnv()` to symlink `CARGO_HOME/registry` to shared mount when available
- Test registry sharing across families

### Phase 3: CI Adaptation

Restructure CI workflows to maximize layer cache reuse.

- Within a batch job, dependency layers are automatically shared across recipes (same Docker daemon, same layer cache)
- Add `docker save`/`docker load` or registry push for cross-job sharing if needed
- Prune stale foundation images when ecosystem versions change

Phase 1 is the core value. Phase 2 adds incremental improvement. Phase 3 is operational.

## Security Considerations

### Download Verification

Foundation images contain toolchains installed by tsuku via `tsuku install --plan`. The plan includes checksums computed during host-side plan generation. The same verification logic runs inside the Dockerfile's RUN command as in any normal tsuku installation. Docker layers cache the verified result.

### Execution Isolation

Foundation images run in the same container isolation as current sandbox images. The security boundary (network restrictions, resource limits, read-only mounts) is unchanged. Toolchains at `/opt/ecosystem/` are baked into the image, not mounted from the host.

The cargo registry mount (Phase 2) is read-write, a weaker guarantee than the read-only download cache. Mitigation: scoped to a single recipe's multi-family run, and cargo verifies checksums on read.

### Supply Chain Risks

Foundation images are built locally using `runtime.Build()` with generated Dockerfiles. They don't introduce new external dependencies. The tsuku binary is copied into the build context temporarily and removed in the final layer.

The plan JSON files copied into the build context contain URLs and checksums. These are generated by the host-side plan generation, which is the same code path used for non-sandbox installations. A compromised plan could cause tsuku to install a malicious toolchain, but this risk is identical to the current non-cached sandbox flow.

### User Data Exposure

Not applicable. Foundation images contain ecosystem toolchains (compiler binaries, standard libraries). No user-specific data is stored.

## Consequences

### Positive

- Toolchain installation happens once per family per version, cached as Docker layers. Subsequent recipes that need the same toolchain start instantly
- Cross-recipe sharing is automatic via Docker's layer caching -- no explicit cache management
- Works dynamically on developer machines with no setup beyond having Docker/Podman
- The plan structure (which already contains the dependency tree) drives the caching -- no parallel bookkeeping
- Generalizes to any ecosystem: npm_install with Node.js, gem_install with Ruby, etc.
- CI benefits from the same mechanism without a CI-specific caching format

### Negative

- Foundation images consume disk space (~500MB per ecosystem per family). With 5 families and 2 ecosystems, that's ~5GB. Mitigation: stale images pruned when versions change; `docker system prune` for manual cleanup
- Extending `runtime.Build()` for custom build contexts is a non-trivial change. Mitigation: backward-compatible, only foundation images use it initially
- Three image levels (base, package, foundation) increase sandbox executor complexity. Mitigation: foundation images are opt-in; recipes without InstallTime deps see no change
- Compilation time (10-40 min per family) remains unaddressed. The savings target toolchain installation and setup, not the build itself
- Layer ordering is position-sensitive: changing the sort order invalidates all cached layers. Mitigation: alphabetical sort is stable and unlikely to change

### Mitigations

| Risk | Mitigation |
|------|------------|
| Disk usage | Prune stale images; `docker system prune` |
| Foundation image loss | Rebuilt on demand; only costs time |
| Sort order change | Alphabetical is stable; no planned changes |
| Executor complexity | Opt-in; no-op without InstallTime deps |
| Runtime compatibility | Uses Dockerfile-based builds; no BuildKit features |
