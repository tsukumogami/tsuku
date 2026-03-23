# Research: Trigger Models for On-First-Use Without Shell Hooks

## Summary

Four primary trigger models exist. For non-interactive contexts (scripts, CI), only two work reliably: **explicit wrapper** (`tsuku run <cmd>`) and **shim-based interception**. The command-not-found hook and prompt-hook models require interactive shell setup and are insufficient on their own. The binary index (issue #1677) is NOT required for the project-declared tool use case -- it solves a different problem (unknown command → find recipe). Start with explicit wrapper, add shims as optional enhancement.

---

## Trigger Models

### Model A: Shim-Based Interception (asdf pattern)

Wrapper scripts placed in PATH intercept command invocations, check tool status, and dispatch to the real binary.

- **Works non-interactively?** Medium -- works in scripts if PATH is inherited, but CI environments may not persist PATH setup between jobs
- **Developer changes commands?** No (transparent)
- **Overhead:** ~120ms per command (shell script startup cost; Go-based shims ~10ms)
- **Implementation:** Shim generation on install, regeneration on recipe changes
- **Maintenance:** Requires `tsuku reshim` after new tools installed

### Model B: Explicit Wrapper -- `tsuku run` (devbox/mise-exec pattern)

User wraps commands: `tsuku run jq .foo data.json`. tsuku reads project config, installs if needed, executes.

- **Works non-interactively?** Excellent -- works everywhere
- **Developer changes commands?** Yes (prefix with `tsuku run`)
- **Overhead:** None to shell; only tool install time on first use
- **Implementation:** Low complexity, builds on existing install logic
- **Maintenance:** None

### Model C: Command-Not-Found Hook (shell-specific)

Shell function intercepts failed commands (`command_not_found_handle` for bash, etc.), calls `tsuku suggest` or `tsuku run`.

- **Works non-interactively?** No -- requires shell hook setup
- **Developer changes commands?** No (transparent in interactive shell)
- **Overhead:** ~50ms (binary index lookup)
- **Implementation:** High -- three shell implementations, index management
- **Maintenance:** High

### Model D: Project-Aware Environment Wrapper (nix-shell --run pattern)

`eval $(tsuku shell)` or `tsuku exec <cmd>` reads tsuku.toml, installs tools, modifies PATH, runs command.

- **Works non-interactively?** Good for explicit form; No for prompt-hook form
- **Developer changes commands?** Yes (activation step) or No (if hooks set up)
- **Overhead:** ~5-10ms config parsing + tool install on first use
- **Implementation:** High -- config parsing, version matching, PATH manipulation

---

## Comparison Table

| Model | Works in CI | Changes commands | Overhead | Complexity |
|-------|------------|-----------------|----------|------------|
| Shim-based | Medium | No | ~120ms/cmd | Medium |
| Explicit wrapper (`tsuku run`) | Excellent | Yes | None (shell) | Low |
| Command-not-found hook | No | No | ~50ms lookup | High |
| Project env wrapper (explicit) | Good | Yes | ~10ms + install | High |
| Project env wrapper (hook) | No | No | Variable | High |

---

## Key Finding: Binary Index Not Required for Project-Declared Tools

When a tool is in tsuku.toml, the recipe is already known. The binary index solves "unknown command → find recipe." These are separate lookup paths:

- **Project-declared path**: Load tsuku.toml → recipe name known → direct install
- **Unknown command path**: User types unknown command → binary index → find recipe

The binary index (Block 1) is valuable for the command-not-found interactive UX. It is NOT a prerequisite for the tsuku.toml auto-install use case.

---

## Recommendations for tsuku

**Primary (universal):** Implement `tsuku run` (Model B) first. Low complexity, works everywhere, no shell integration required.

**Secondary (interactive enhancement):** Add shim generation as optional. Shims enable transparent invocation in interactive shells without requiring `tsuku run` prefix.

**Complementary:** Shell activation (`eval $(tsuku shell)`) for scripts that need to use tools directly. One activation line at top of script, then tools available by name.

**Deferred:** Command-not-found hook (Model C) -- valuable for interactive discovery UX but not required for the core project-tool use case.
