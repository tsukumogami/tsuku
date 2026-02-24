# Exploration Summary: Sandbox Build Cache

## Problem (Phase 1)
When testing cargo_build recipes across 5 Linux families, each family independently installs the same Rust toolchain and compiles the same dependency tree. For heavy crates this means 5x the compilation time, frequently hitting CI timeout limits.

## Decision Drivers (Phase 1)
- Preserve family isolation for final binary output
- Don't break determinism guarantees (SOURCE_DATE_EPOCH=0, CARGO_INCREMENTAL=0)
- Compatible with both Docker and Podman runtimes
- Follow existing patterns (download cache mount, container image caching)
- Work both locally and in CI (GitHub Actions)
- Minimize complexity in sandbox executor

## Research Findings (Phase 2)
- Workspace mount at /workspace shadows anything pre-installed in Docker image at that path
- docker commit works with both Docker and Podman, avoids BuildKit dependency
- Cargo registry content is platform-independent, safe to share across families
- BuildKit cache mounts have inconsistent Podman support, should be avoided

## Options (Phase 3)
- Decision 1 (Ecosystem caching): Foundation images via docker commit > Dockerfile RUN commands > BuildKit cache mounts > Volume-mounted toolchain
- Decision 2 (Cargo registry): Shared read-write volume mount > Don't share > Share entire CARGO_HOME
- Decision 3 (Scope): General mechanism with cargo as first consumer > Cargo-specific caching

## Decision (Phase 5)

**Problem:**
When testing cargo_build recipes across 5 Linux families, each family independently installs the same Rust toolchain and fetches the same cargo registry content. This redundant work adds 15-25 minutes per recipe test, frequently pushing CI jobs past the 60-minute timeout. The existing download and container image caches don't help because ecosystem toolchain installation happens at runtime in ephemeral containers that are destroyed after each run.

**Decision:**
Introduce "foundation images" as a second level of container image caching. Foundation images extend the existing package images by pre-installing ecosystem toolchains at `/opt/ecosystem/`, outside the workspace mount that would shadow them. The sandbox script bridges the pre-installed toolchain into the workspace via symlinks. Foundation images are built using the existing `runtime.Build()` infrastructure with generated Dockerfiles that run tsuku inside a RUN command. Separately, cargo registry content is shared across families via a read-write volume mount, following the existing download cache pattern.

**Rationale:**
Foundation images extend the proven container image cache pattern (deterministic hash, build-once-reuse-many) to a second level without introducing new mechanisms. Building via generated Dockerfiles reuses existing infrastructure, produces reproducible images, and keeps tsuku as the single installation authority. The symlink bridge solves workspace mount shadowing without modifying the mount strategy. Cargo registry sharing is safe because the content is platform-independent and cargo verifies checksums on read. We chose not to share compiled artifacts across families because different libc environments require independent compilation for correctness.

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-02-24
