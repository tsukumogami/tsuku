# DESIGN: Project-Aware Exec Wrapper

## Status

Proposed

## Upstream Design Reference

Parent: [DESIGN: Shell Integration Building Blocks](DESIGN-shell-integration-building-blocks.md)
Block 6 -- the convergence point between Track A (command interception/auto-install) and Track B (project configuration/shell activation). This design specifies `tsuku exec` and optional shim generation that bridge both tracks at command invocation time.

Depends on:
- Block 3 (#1679, auto-install flow) -- `autoinstall.Runner.Run` with `ProjectVersionResolver` interface (implemented)
- Block 4 (#1680, project configuration) -- `project.LoadProjectConfig` (implemented)
- Block 5 (#1681, shell environment activation) -- `shellenv.ComputeActivation` (implemented)

## Context and Problem Statement

Track A (Blocks 1-3) handles command discovery and auto-install: when a user types an unknown command, the binary index identifies the recipe, and `tsuku run` installs it. Track B (Blocks 4-5) handles project environments: `.tsuku.toml` declares tool requirements, and shell activation puts the right versions on PATH.

These tracks converge in interactive shells with hooks set up. But they don't converge in three important contexts:

1. **CI pipelines**: No shell hooks, no prompt. A CI script needs `tsuku exec koto` to find `koto` in `.tsuku.toml`, install the right version, and run it.
2. **Shell scripts**: A Makefile or bash script calls `go build` but doesn't have tsuku shell activation. `tsuku exec go build` should use the project-declared Go version.
3. **Users without hooks**: Developers who don't want prompt hooks still want `tsuku exec node server.js` to use the project's Node version.

The auto-install flow (`autoinstall.Runner.Run`) already accepts a `ProjectVersionResolver` interface, designed for exactly this integration. Block 6 implements that interface using `LoadProjectConfig`, adds `tsuku exec` as the CLI entry point, and optionally generates shims for transparent invocation.

### Scope

**In scope:**
- `tsuku exec <command> [args]` command semantics
- `ProjectVersionResolver` implementation using `LoadProjectConfig`
- Fallback to binary index when command not in `.tsuku.toml`
- Optional shim generation: `tsuku shim install`, `tsuku shim uninstall`
- CI usage patterns
- Security model for shim-based auto-install

**Out of scope:**
- Changes to the auto-install flow itself (Block 3, already implemented)
- Changes to the binary index (Block 1, already implemented)
- LLM recipe discovery

## Decision Drivers

- **Works without shell hooks**: `tsuku exec` must function in CI, scripts, and non-interactive shells
- **Performance**: Cached tool lookup (already installed) must complete in under 50ms
- **Compatibility**: Must work with existing `tsuku run` (Track A) and `tsuku shell` (Track B) without conflicts
- **Simplicity**: Leverage the existing `ProjectVersionResolver` interface rather than building new abstractions
- **Security**: Shim-based auto-install in untrusted repos must have a clear consent model
