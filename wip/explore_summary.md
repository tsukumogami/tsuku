# Exploration Summary: Sandbox Build Cache

## Key Insight
Map plan dependency layers to Docker image layers. The plan already contains a structured dependency tree with resolved versions. Each InstallTime dependency becomes a separate RUN command in a generated Dockerfile. Docker's native layer caching handles cross-recipe reuse automatically.

## Architecture
1. Host generates plan (versions resolved)
2. Extract + flatten dependency tree from plan.Dependencies
3. Generate Dockerfile: one RUN per dep, canonical (alphabetical) order
4. docker build (layer caching reuses matching deps)
5. Sandbox runs from foundation image with symlink bridge

## Design Decisions
- Per-dep Docker layers (not single foundation image) for cross-recipe layer sharing
- Alphabetical canonical ordering for deterministic layer positioning
- Symlink bridge from /opt/ecosystem/ into /workspace/tsuku/ to handle mount shadowing
- tsuku install --plan inside RUN commands (tsuku stays single installation authority)
- Dynamic user-machine operation first; CI adapts to the format

## Current Status
**Phase:** Complete (Proposed, awaiting approval)
**Last Updated:** 2026-02-24
