---
status: Proposed
problem: |
  Tsuku installs tools by downloading binaries and symlinking them, but tools
  that need shell integration (eval-init wrappers, completions, env files) or
  cleanup on removal get a bland, incomplete installation. At least 8-12 tools
  in the registry (direnv, zoxide, mise, niwa) cannot provide their core
  functionality without post-install shell setup. The recipe system has no
  lifecycle phases beyond the install step sequence, the remove flow doesn't
  consult recipes, and the update flow has no pre/post hooks.
---

# DESIGN: Tool Lifecycle Hooks

## Status

Proposed

## Context and Problem Statement

Niwa's shell-integration design (tsukumogami/niwa#39) identified a gap: when
niwa is installed via tsuku, it gets a generic binary-and-symlink installation
that misses its shell function wrapper, completions, and env file setup. The
design explicitly deferred tsuku generalization, noting "tsuku has no post-install
shell mechanism" and that adding one would cost 200+ LOC across two repos with
only one consumer.

Exploration found that generalization is now warranted. A survey of 1,400 recipes
identified 8-12 tools that can't function without post-install shell integration
(direnv, zoxide, asdf, mise) and 200+ that would benefit from completion
registration. The current action system has the building blocks (set_env,
run_command) but no lifecycle phase concept -- all steps execute during a single
install pass. The remove flow deletes directories without consulting recipes,
and the update flow is a plain reinstall with no pre/post hooks.

The design must answer: how should recipes declare lifecycle behavior, how should
per-tool shell integration compose with tsuku's existing shellenv, what security
model applies to tool-defined hooks, and how does cleanup state persist across
install/remove cycles?

## Decision Drivers

- Tsuku's current trust model (no post-install code execution) is a genuine
  security advantage over npm-style arbitrary scripts -- preserve it where possible
- The existing action system's WhenClause infrastructure is proven and extensible
- Shell startup time budget is tight (~50ms total); per-tool init adds 10-30ms each
- Tools must work when installed standalone (not just via tsuku)
- Cleanup must be reliable even without registry access at remove time
- Recipe authors should be able to declare lifecycle behavior in familiar TOML format

## Decisions Already Made

These choices were settled during exploration and should be treated as constraints:

- **Start with declarative hooks (Level 1), not imperative scripts**: Security
  research shows tsuku's current trust model is a strength. A limited vocabulary
  of lifecycle actions (install_shell_init, install_completions, cleanup_paths)
  preserves this while enabling the key use cases. Imperative hooks (Level 2) can
  be added later if declarative proves insufficient.

- **shell.d directory model for composition**: Ecosystem patterns (mise, asdf) and
  tsuku's existing hook machinery support this. Post-install hooks write init
  scripts to $TSUKU_HOME/share/shell.d/{tool}.{shell}. Cached combined scripts
  keep startup under 5ms.

- **Extend existing action system rather than new recipe sections**: The
  WhenClause/Step infrastructure is proven. Adding a phase qualifier is lower
  risk than a recipe schema redesign with separate [lifecycle] sections.

- **Post-install shell integration is the priority**: 8-12 tools can't function
  without it. Completions and service registration are secondary.

- **Store cleanup instructions in state at install time**: The remove flow doesn't
  load recipes today. Storing what was installed (which shell.d files, which
  completions) in state ensures reliable cleanup without registry access.

- **Hooks fail gracefully, not fatally**: Hook failure should warn but not block
  installation or removal. The tool is still installed and usable.
