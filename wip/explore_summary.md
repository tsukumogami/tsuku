# Exploration Summary: Sandbox Build Cache

## Key Insight
Map plan dependency layers to Docker image layers. Switch from broad workspace mount to targeted file-level mounts so the container's filesystem preserves pre-installed tools from Docker layers. The executor's existing skip logic works natively.

## Architecture
1. Host generates plan (versions resolved)
2. Extract + flatten dependency tree from plan.Dependencies
3. Generate Dockerfile: one RUN per dep, canonical (alphabetical) order, TSUKU_HOME=/workspace/tsuku
4. docker build (layer caching reuses matching deps)
5. Sandbox runs with targeted mounts (plan, script, cache, output) -- not broad /workspace mount

## Design Decisions
- Per-dep Docker layers (not single foundation image) for cross-recipe layer sharing
- Alphabetical canonical ordering for deterministic layer positioning
- Targeted mounts replace broad workspace mount -- eliminates shadowing problem entirely
- No symlink bridge, no /opt/ecosystem/, no alternate install paths
- tsuku install --plan inside RUN commands (tsuku stays single installation authority)
- Dynamic user-machine operation first; CI adapts to the format
- Additive BuildFromDockerfile() on Runtime interface (existing Build() unchanged)
- Download cache stays read-only; only output dir is writable
- Targeted mounts applied unconditionally (with or without foundation image)

## Decision (Phase 5)

**Problem:**
When testing recipes across Linux families, each family independently installs
the same ecosystem toolchains inside ephemeral containers that are destroyed
after each run. There's no mechanism to carry forward this work. The plan
already contains a structured dependency tree with resolved versions, but
nothing maps this tree to reusable container image layers.

**Decision:**
Map plan dependency layers to Docker image layers. Each InstallTime dependency
in the plan becomes a separate RUN command in a generated Dockerfile, installed
in canonical order. The sandbox mount strategy changes from a single broad
workspace mount to targeted mounts for specific files, so pre-installed
dependencies in the container image live at the standard $TSUKU_HOME path and
are found by the executor's existing skip logic.

**Rationale:**
Docker already solves the "don't redo identical work" problem through layer
caching. Targeted mounts avoid the workspace shadowing problem entirely --
the container's filesystem at $TSUKU_HOME is preserved, so pre-installed tools
are discovered natively without symlink bridges or alternate install paths.
Canonical ordering maximizes shared prefixes between recipes. Using tsuku
itself inside RUN commands keeps it as the single installation authority.

## Current Status
**Phase:** Complete (Proposed, awaiting approval)
**Last Updated:** 2026-02-28
