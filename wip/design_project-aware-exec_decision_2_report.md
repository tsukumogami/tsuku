<!-- decision:start id="shim-generation-architecture" status="assumed" -->
### Decision: Shim Generation Architecture

**Context**

Shims provide transparent tool invocation: typing `go build` instead of `tsuku exec go build`. They're thin wrappers in `$TSUKU_HOME/bin/` that delegate to `tsuku exec`, which handles project config lookup, version resolution, auto-install consent, and process handoff.

The existing PATH layout already supports shims naturally. Shell activation (Block 5) prepends project-specific tool bin dirs before `$TSUKU_HOME/bin/`, so when activation is running, real binaries win over shims. When activation isn't available -- CI, scripts, Makefiles -- shims fire and call `tsuku exec`. This means shims and shell activation coexist without conflict: activation is the fast path for interactive shells, shims are the fallback for everything else.

The autoinstall security model (consent modes, TTY detection, config permission checks, verification gates, audit logging) is already complete in `autoinstall.Runner.Run`. Shims reuse it entirely by delegating to `tsuku exec`.

**Assumptions**

- Shell script shim startup overhead (~5-10ms) is acceptable on top of tsuku exec's <50ms target. Total wall time for cached tools is ~55-60ms. If this proves too slow, the multi-call binary approach (Alternative 4) can be adopted later without changing the user-facing commands.
- Users will shim a manageable number of tools (tens, not thousands). Directory scanning for `tsuku shim list` won't be a bottleneck.

**Chosen: Explicit Per-Tool Shell Script Shims**

Users create shims for specific tools via `tsuku shim install <tool>`. Each shim is a shell script in `$TSUKU_HOME/bin/`:

```sh
#!/bin/sh
exec tsuku exec "$(basename "$0")" -- "$@"
```

The command set:
- `tsuku shim install <tool>` -- creates shims for all binaries the recipe provides. Refuses to overwrite existing files (including the tsuku binary itself). Prints which shims were created.
- `tsuku shim uninstall <tool>` -- removes shims for the tool's binaries. Only removes files that are tsuku shims (checks content), not arbitrary files.
- `tsuku shim list` -- lists installed shims with their target tools.

Shim content is static. The shim calls `tsuku exec`, which reads `.tsuku.toml` at runtime to resolve the project-pinned version, falls back to the globally installed version, and applies the full consent model if installation is needed. This means:
- No shim regeneration when `.tsuku.toml` changes
- No shim regeneration when tools are installed/updated
- The same shim works across all projects

Security for untrusted repos: shim creation is an explicit user action that doesn't reference any project config. When a shimmed command runs in an untrusted repo, `tsuku exec` finds the repo's `.tsuku.toml` and resolves the declared version. But installation goes through the consent model -- in auto mode, the verification gate requires the recipe to have checksums, and the config permission gate prevents a tampered config from silently enabling auto mode. In confirm mode (the default), the user gets a prompt. In non-interactive contexts without explicit auto mode opt-in, the TTY gate blocks installation entirely.

**Rationale**

Explicit shim creation matches tsuku's philosophy of user control over their environment. The auto-all approach (asdf pattern) would create shims for every installed tool, conflicting with the existing `tools/current/` symlink mechanism and intercepting commands users didn't intend to route through tsuku. Project-scoped auto-shims would need a tracking registry for which project owns which shim, adding complexity for marginal benefit over running `tsuku exec` directly. The multi-call binary saves ~10ms per invocation but adds build complexity that isn't justified at this stage.

Static shims are the key simplification. Because the shim always calls `tsuku exec` (which does version resolution at runtime), there's nothing to update when project configs change or new tool versions are installed. This eliminates the "stale shim" problem that plagues more complex shim systems.

**Alternatives Considered**

- **Auto-Shim All Installed Tools (asdf pattern)**: Every install creates shims, every remove deletes them. Rejected because it creates a fundamental conflict with `tools/current/` (both would provide the same command, with bin/ winning) and intercepts commands the user didn't ask to shim. Converting all tools/current/ to shims is a larger architectural change than Block 6 should make.

- **Project-Scoped Auto-Shims**: Shims created/removed based on .tsuku.toml content during activation. Rejected because it adds lifecycle tracking complexity (which project owns which shim, conflict resolution for overlapping tools), has race conditions between projects, and doesn't help CI -- by the time you activate, you could just run `tsuku exec`.

- **Compiled Multi-Call Binary**: A single Go binary where symlinks/hardlinks invoke shim behavior based on argv[0]. Saves ~10ms shell startup overhead. Rejected for now because the 50ms target applies to the tsuku exec path itself, not the total shim overhead. Can be adopted later as a performance optimization without changing user-facing commands, since the shim interface (`tsuku shim install/uninstall/list`) stays the same.

**Consequences**

Users get a clean opt-in model: `tsuku shim install go` in their shell profile or CI setup, then `go build` just works everywhere. Shell activation users don't need shims at all (real binaries take precedence). CI users can choose between `tsuku exec go build` (no setup) or `tsuku shim install go && go build` (setup once, use naturally).

The `$TSUKU_HOME/bin/` directory gains a dual role: it holds the tsuku binary and optional shims. The `tsuku shim` subcommand manages the shim subset. Content-based identification (checking if a file is a tsuku shim by reading its content) prevents accidental removal of non-shim files.

`tools/current/` remains the mechanism for global tool activation. Shims sit between shell activation and tools/current/ in PATH precedence, providing project-aware version resolution that tools/current/ can't do.
<!-- decision:end -->
