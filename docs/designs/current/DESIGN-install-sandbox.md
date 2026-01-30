---
status: Current
problem: Sandbox testing behavior was scattered across individual builders rather than being a unified recipe-driven operation, preventing independent invocation and creating duplicated action knowledge.
decision: Implement a NetworkValidator interface on actions and a SandboxRequirements struct that derives container configuration from plan content, enabling unified sandbox testing via a single Sandbox() entry point.
rationale: Co-locating metadata with action code provides compile-time enforcement and a single source of truth. The separate SandboxRequirements struct maintains backwards compatibility with existing plans while enabling independent invocation via tsuku install --sandbox.
---

# Centralized Sandbox Testing

## Status

Current

## Context and Problem Statement

Tsuku's container-based sandbox testing ensures recipes work correctly by executing them in isolated containers. The eval+plan architecture enables offline sandbox testing by caching downloads during plan generation on the host, then running `tsuku install --plan` in a container with cached assets.

Before centralization, sandbox behavior was scattered across individual builders. The GitHub Release builder called `Sandbox()` with no network, the Homebrew bottle builder did the same, and the Homebrew source builder called a separate `SandboxSourceBuild()` with host network. Each builder independently decided whether to sandbox, which method to call, what container image to use, whether network was needed, and how to handle failures.

This created several problems. Sandbox testing couldn't be invoked outside the `tsuku create` flow — a user modifying a recipe couldn't test it in isolation. The decision of which sandbox method to use depended on transient builder context rather than information derivable from the recipe itself. And `detectRequiredBuildTools()` contained a switch statement mapping action names to apt packages, duplicating knowledge that should live with the actions.

Network requirements were also implicit. Some actions need network access (cargo_build fetches crates, go_build fetches modules, package managers need repositories), but this wasn't surfaced anywhere. The system handled it by choosing different sandbox methods, but the requirement wasn't derivable from recipe analysis.

## Decision Drivers

- Sandbox requirements should be derivable from recipe/plan content alone
- Each action should declare its own requirements
- Sandbox testing must work outside the `tsuku create` flow
- Existing recipes and plans must continue to work
- Knowledge about actions shouldn't be duplicated between action code and sandbox executor

## Considered Options

Two independent decisions shaped the design: where to store action metadata and how to surface sandbox requirements.

For action metadata, the main options were static registry maps (extending the existing pattern of separate maps per dimension), interface methods on actions (co-locating metadata with implementation), and a structured metadata registry (combining multiple dimensions in one map). Interface methods were chosen because they provide compile-time enforcement, self-documentation, and consistency with the broader direction of migrating static maps to interface methods.

For surfacing sandbox requirements, the options were plan-level aggregate fields (pre-computed during plan generation), per-step metadata in ResolvedStep (full granularity but larger plans), and a separate SandboxRequirements struct computed from any plan. The separate struct was chosen because it works with existing plans without regeneration, maintains clean separation of concerns, and enables independent invocation from any code that has a plan.

## Decision Outcome

Actions implement a `RequiresNetwork()` method declaring their network requirements. A `ComputeSandboxRequirements()` function derives container configuration by querying each action in the plan. A single `Sandbox()` method on the executor replaces the previous split between `Sandbox()` and `SandboxSourceBuild()`.

The `detectRequiredBuildTools()` switch statement was eliminated. Build tool dependencies are now handled through each action's `Dependencies()` method, which tsuku's normal dependency resolution uses during plan execution.

## Solution Architecture

### Action Metadata

The `NetworkValidator` interface (`internal/actions/action.go`, line 87) declares a single method: `RequiresNetwork() bool`. All actions embed `BaseAction` (line 104), which provides a default `RequiresNetwork()` returning false. Actions that need network access override this.

21 actions return true for `RequiresNetwork()`:
- **Ecosystem actions**: cargo_build, cargo_install, go_build, go_install, npm_install, npm_exec, pip_install, pip_exec, pipx_install, gem_install, gem_exec, cpan_install, nix_realize, nix_install
- **System package managers**: apt_install, apt_repo, apt_ppa, brew_install, brew_cask
- **General**: run_command (conservative default)

All other actions (download, extract, chmod, install_binaries, configure_make, cmake_build, meson_build, etc.) inherit the default false. The "fail-closed" design means unknown actions run without network. If they actually need it, the container execution fails with a clear timeout or DNS error, prompting the developer to add the method.

Action metadata originally used static registry maps (`deterministicActions`, `ActionDependencies`). These have been migrated to interface methods: `IsDeterministic()` and `Dependencies()` on the `Action` interface, alongside `RequiresNetwork()`. Each action declares all its metadata through methods on the `BaseAction` embedding pattern.

### Requirements Computation

`SandboxRequirements` (`internal/sandbox/requirements.go`, line 58) contains three fields: `RequiresNetwork` (bool), `Image` (container image string), and `Resources` (ResourceLimits struct with memory, CPUs, pids, and timeout).

`ComputeSandboxRequirements()` (line 78) iterates the plan's steps, queries each action via the NetworkValidator interface, and aggregates the results. If any step requires network, the entire sandbox gets network access. The function also checks for build actions (configure_make, cmake_build, cargo_build, go_build) to upgrade resources even for offline builds that still need more memory and CPU for compilation.

Two resource profiles exist: default limits (2 GB memory, 2 CPUs, 100 pids, 2-minute timeout) via `DefaultLimits()` (line 31), and source build limits (4 GB memory, 4 CPUs, 500 pids, 15-minute timeout) via `SourceBuildLimits()` (line 43).

### Container Images

The default image is `debian:bookworm-slim` for binary installations, and `ubuntu:22.04` for source builds requiring network. Beyond these defaults, the system derives family-specific images from the plan's platform information (`internal/sandbox/container_spec.go`): debian:bookworm-slim for Debian family, fedora:41 for RHEL, archlinux:base for Arch, alpine:3.19 for Alpine, and opensuse/leap:15 for SUSE.

The original design proposed Alpine as the sole base image. This was abandoned because musl libc caused compatibility issues with binaries linked against glibc. Using family-specific images also enables testing recipes with distribution-specific package manager actions (apt_install on Debian, dnf_install on Fedora, etc.).

### Runtime Detection

The `RuntimeDetector` (`internal/validate/runtime.go`, line 79) auto-detects available container runtimes in preference order:

1. Podman (preferred for native rootless support)
2. Docker rootless (security-hardened)
3. Docker with group membership (least preferred, warns about root-equivalent access)

Detection results are cached. If no runtime is found, sandbox testing is skipped with a warning. This respects tsuku's philosophy of working with existing system configuration rather than requiring installation.

Rootless container support requires system-level configuration (subordinate UID/GID mappings in `/etc/subuid` and `/etc/subgid`, newuidmap/newgidmap binaries, kernel support for unprivileged user namespaces). Tsuku doesn't attempt to configure these — it detects what's available and uses the best option. The detection uses a hybrid approach: fast configuration checks first, followed by a verification container run if configuration looks valid.

### Unified Sandbox Executor

The `Sandbox()` method on `Executor` (`internal/sandbox/executor.go`, line 117) is the single entry point for all sandbox testing. It accepts a plan and `SandboxRequirements`, then:

1. Detects the container runtime
2. Derives container specification from system requirements
3. Builds or caches the container image
4. Mounts the tsuku binary, plan file, and download cache into the container
5. Configures network access (`--network=none` or `--network=host`) based on requirements
6. Applies resource limits (memory, CPU, pids, timeout)
7. Runs `tsuku install --plan` inside the container
8. Returns a `SandboxResult` with success/failure and output

The previous split between `Sandbox()` (for binary recipes with no network) and `SandboxSourceBuild()` (for source recipes with host network) was eliminated. Both cases now flow through the same code path, with behavior determined by `SandboxRequirements`.

### Parallel Safety

The `LockManager` (`internal/validate/lock.go`, line 14) prevents interference between concurrent tsuku processes running sandbox tests. It uses filesystem-level flock locking with container ID tracking. Lock metadata includes process ID and creation timestamp. Stale locks from crashed processes are detected and cleaned up via `TryCleanupStale()` (line 169).

### Pre-Download Caching

The `PreDownloader` (`internal/validate/predownload.go`, line 27) downloads assets before container execution and computes SHA256 checksums. Downloads are cached so the container can run with `--network=none` for binary installations. HTTPS is enforced (line 58). The download cache directory is mounted into the container, so the executor doesn't re-download assets that were fetched during plan generation.

### CLI Integration

Two flags on `tsuku install` enable standalone sandbox testing (`cmd/tsuku/install.go`):

- `--sandbox` (line 180): Generates a plan and runs it in an isolated container. Works with tool names (`tsuku install curl --sandbox`), plan files (`tsuku install --plan plan.json --sandbox`), or piped from eval (`tsuku eval curl | tsuku install --plan - --sandbox`).
- `--recipe` (line 181): Accepts a local recipe file path for testing recipes before submitting to the registry. Combined with `--sandbox`, enables `tsuku install --recipe ./my-recipe.toml --sandbox`.

## Implementation Approach

The implementation proceeded in phases: first the NetworkValidator interface and BaseAction embedding, then RequiresNetwork() implementations across all actions, then SandboxRequirements computation, then unifying the executor, then updating builders to use centralized sandbox testing, and finally adding the CLI flags.

Build tool dependencies shifted from a hardcoded `detectRequiredBuildTools()` switch statement to each action's `Dependencies()` method returning `ActionDeps` with `InstallTime` and `Runtime` slices. When `tsuku install --plan` runs inside a container, normal dependency resolution installs build tools automatically. This eliminated the duplicated knowledge between action implementations and the sandbox executor.

## Security Considerations

### Download Verification

The eval+plan architecture caches downloads during plan generation with SHA256 checksums. The container executor mounts these cached assets and verifies checksums during `tsuku install --plan`. No new download paths are introduced by centralization.

### Execution Isolation

Containers run with `--network=none` when possible (binary installations), `--network=host` only when actions explicitly require it (ecosystem builds). The download cache is mounted read-only. Resource limits (memory, CPU, pids, timeout) prevent runaway processes.

The `RequiresNetwork()` method on each action controls network access. If an action incorrectly returns true, it gains unnecessary network access. This is mitigated by code review visibility — the method sits in each action's file, and most actions inherit the default false from BaseAction.

### Supply Chain Risks

The `tsuku install --recipe` flag allows testing arbitrary local recipes. A malicious recipe could consume resources up to container limits, attempt network access if its actions declare RequiresNetwork=true, and execute arbitrary commands within the container. Container isolation contains these risks: no access to host filesystem beyond mounted workspace, and resource limits prevent denial of service.

Network-enabled sandbox tests use `--network=host`, giving containers full network access. A compromised dependency fetched during an ecosystem build could exfiltrate data. Bridge networking with egress filtering to known package registries is a possible future hardening.

### User Data Exposure

Sandbox containers have no access to the user home directory. The workspace contains only the recipe, plan, and download cache. No telemetry or external reporting is added.

## Consequences

### Positive

- Sandbox requirements are derivable from plan content alone, with no builder context needed
- Network requirements are explicit and auditable per action
- Compile-time enforcement catches missing interface implementations
- Users can run `tsuku install <tool> --sandbox` or pipe from eval independently
- The duplicated `detectRequiredBuildTools()` switch was eliminated
- Family-specific container images enable testing distribution-specific actions
- Existing plans work without regeneration

### Negative

- Every action file needed a RequiresNetwork() method (mitigated by BaseAction embedding defaults)
- Requirements are computed from the plan each time rather than cached (acceptable since computation is O(n) over steps, and typical recipes have fewer than 10 steps)
- Network-enabled sandbox gives containers full host network access rather than scoped egress
