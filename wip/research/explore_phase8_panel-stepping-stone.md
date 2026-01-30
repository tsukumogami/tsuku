# Panel: --env as Stepping Stone to Per-Directory Activation

## The Core Insight: Environments as Backing Store

Named environments CAN be the foundation for per-directory activation if we flip the relationship: instead of `--env` being a manual CLI flag, environments become the **persistent storage layer** that per-directory configs point to. A `.tsuku.toml` file would declare `env = "project-a"`, and tsuku would automatically activate that environment when you cd into that directory. The `--env` flag becomes the low-level primitive for creating and managing these environments, while shell integration (PATH manipulation via eval hooks) provides the automatic activation layer.

This requires one critical redesign: environments must support **reference counting and garbage collection**. If five projects reference `env = "node18-python311"`, that environment should persist until all five remove it. The current proposal treats environments as session-scoped throwaway state; the stepping-stone version treats them as shared, reusable manifests. The directory layout (`envs/<name>/tools/`, `envs/<name>/state.json`) works perfectly as-is—it already provides isolated tool installations and separate state tracking, which is exactly what per-directory configs need.

## What Changes in the Proposal

1. **Add env reference tracking**: `state.json` in each environment records which projects (by absolute path) reference it. `tsuku env gc` removes unreferenced environments.
2. **Make environments declarative**: Support `tsuku env create <name> --from-file .tsuku.toml` to initialize an environment from a manifest, not just imperative install commands.
3. **Decouple from shell sessions**: Drop the session-scoped mental model. Environments are long-lived, named collections of tools that outlive any single terminal session.
4. **Add `tsuku env list --used-by <project-path>`**: Show which environment (if any) a project directory is bound to.

The phase-1 implementation still solves contributor isolation—`tsuku install --env pr-1234 gh@2.40.0` works exactly as proposed—but now it's building infrastructure that `.tsuku.toml` will reuse. When we add per-directory activation in phase 2, we're not retrofitting; we're just adding automatic environment selection on top of an existing, battle-tested environment management system.

## The Activation Layer (Future Phase)

Per-directory activation becomes a thin layer over environments:
- Shell hook (`eval "$(tsuku hook bash)"`) checks for `.tsuku.toml` in current directory or parents
- If found, reads `env = "project-a"` and prepends `$TSUKU_HOME/envs/project-a/bin` to PATH
- If env doesn't exist yet, prompt: "Environment 'project-a' not found. Run `tsuku env sync` to create it?"
- `tsuku env sync` reads `.tsuku.toml`'s tool requirements and installs them into the named environment

This is genuinely incremental: phase 1 gives us the storage layer and manual activation (`--env` flag), phase 2 adds automatic activation (shell hooks + config files). The alternative—building --env as session-scoped state that we later throw away—would be wasted effort. But environments as persistent, declarative, reference-counted tool collections? That's the foundation.

## What Doesn't Change

The directory layout is perfect as-is. `envs/<name>/` mirrors the global structure (`tools/`, `bin/`, `state.json`), so all existing installation logic works unchanged—just with a different root path. The action system, version providers, recipe registry—none of that needs modification. We're adding a layer of indirection (which environment to use) and a registry (which projects reference which environments), not rewriting core installation mechanics.

The honest admission: reference counting and declarative env creation (reading from `.tsuku.toml`) are NOT in the current proposal and represent real design work. But they're additive. If we add those two features, `--env` stops being orthogonal and becomes the literal backing store for per-directory activation. Without them, `--env` is just a session isolation tool that we'd have to work around later.
