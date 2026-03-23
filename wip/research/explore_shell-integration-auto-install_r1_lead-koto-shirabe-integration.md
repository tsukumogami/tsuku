# Research: Koto + Shirabe Integration Model with tsuku

## Summary

koto has zero references to tsuku and no auto-install logic. Both koto and shirabe currently assume koto is on PATH -- the README's claim that "koto is installed automatically on first skill invocation" is aspirational, not implemented. The integration model is currently Model B (rely on PATH), which means the developer must manually install koto before shirabe skills work. No tsuku.toml exists in either repo. The user's described feature (auto-install on first use) is a gap across all three projects.

---

## koto Architecture

- Rust-based workflow orchestration engine, standalone binary
- Installable via GitHub releases or a custom install script
- Has a tsuku recipe at `/.tsuku-recipes/koto.toml` in the tsuku repo -- so tsuku CAN install koto, but koto doesn't call tsuku
- No built-in mechanism to detect or install missing tools
- Agent integration design specifies skills are distributed as Claude Code plugins
- Zero references to tsuku in koto's codebase

## shirabe Architecture

- Five workflow skills (/explore, /design, /prd, /plan, /work-on) backed by koto state machines
- README states koto is "installed automatically on first skill invocation" -- this is not implemented
- No tsuku.toml or dependency declaration of any kind
- Has a Stop hook for workflow continuation but no tool installation hooks
- Assumes koto is available on PATH when skills execute

## Integration Model Assessment

| Model | Status | Description |
|-------|--------|-------------|
| A: koto calls `tsuku install` | Not implemented | koto has no tsuku references |
| B: Rely on PATH | Current state | Both assume koto is on PATH |
| C: tsuku.toml activates tools | In design, not built | Planned as Blocks 4+5 in DESIGN-shell-integration-building-blocks.md |
| D: koto ships dependency metadata | Not applicable | koto is standalone, doesn't declare tool deps |

## Current Gaps

1. **No auto-install mechanism**: Neither koto nor shirabe can trigger tsuku to install dependencies
2. **No tsuku.toml**: Neither repo declares tool requirements
3. **README claim is aspirational**: "installed automatically" in shirabe README is not true today
4. **No CI path**: Even if shell hooks were set up, there's no trigger for scripts/CI

## Implications

The user's described use case requires ALL of the following to be built:
1. tsuku.toml support (Block 4) in a project that uses shirabe
2. Either: shell activation (Block 5) for interactive use, OR `tsuku run` / shims for non-interactive use
3. koto's recipe in tsuku's registry (already exists as `.tsuku-recipes/koto.toml`)

koto does NOT need to call tsuku itself. The integration model is entirely tsuku-side:
- Developer's project has tsuku.toml declaring `koto`
- tsuku handles installation and PATH/execution
- koto is unaware of tsuku at runtime

The "installed automatically" claim in shirabe's README represents the aspirational end state that the shell integration building blocks are designed to deliver.
