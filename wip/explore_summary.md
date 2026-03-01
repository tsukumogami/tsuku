# Exploration Summary: Sandbox Build Cache

## Key Insight
Group recipes by ecosystem in CI. Build foundation images from plan dependencies as Docker layers. First recipe in a batch builds the image; subsequent recipes find it cached. Targeted mounts preserve the container's $TSUKU_HOME so pre-installed tools are found natively.

## Architecture
1. Host generates plan (versions resolved)
2. Extract + flatten dependency tree from plan.Dependencies
3. Generate Dockerfile: one RUN per dep, TSUKU_HOME=/workspace/tsuku
4. docker build (first recipe builds, subsequent recipes find it cached)
5. Sandbox runs with targeted mounts (plan, script, cache, output)

## Design Decisions
- Foundation images pre-install ecosystem deps as Docker layers
- CI batches recipes by ecosystem (rust, nodejs, etc.) so images are reused within batch jobs
- Targeted mounts replace broad workspace mount -- no shadowing, no symlink bridge
- tsuku install --plan inside RUN commands (tsuku stays single installation authority)
- Additive BuildFromDockerfile() on Runtime interface
- Download cache stays read-only; only output dir is writable

## Decision (Phase 5)

**Problem:**
When testing recipes across Linux families, each family independently installs
the same ecosystem toolchains inside ephemeral containers. CI batches recipes
arbitrarily, so even when multiple cargo_build recipes run on the same runner,
each one re-installs Rust from scratch. There's no mechanism to carry forward
ecosystem setup work between sandbox runs on the same machine.

**Decision:**
Build foundation images that pre-install ecosystem dependencies as Docker
layers. Group recipes by ecosystem in CI so all cargo_build recipes share a
single foundation image per batch job. The sandbox mount strategy changes from
a broad workspace mount to targeted file-level mounts, so the foundation
image's pre-installed tools at $TSUKU_HOME are preserved and found by the
executor's existing skip logic.

**Rationale:**
Docker's layer caching already solves "don't redo identical work." When
recipes are grouped by ecosystem, they produce identical Dockerfiles. The
first recipe builds the foundation image; subsequent recipes find it cached.
Targeted mounts avoid the workspace shadowing problem -- the container's
$TSUKU_HOME lives on its own filesystem where Docker layers persist naturally.
CI restructuring to batch by ecosystem is what turns the caching mechanism
into actual time savings.

## Current Status
**Phase:** Complete (Proposed, awaiting approval)
**Last Updated:** 2026-02-28
