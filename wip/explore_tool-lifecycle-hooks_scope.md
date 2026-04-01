# Explore Scope: tool-lifecycle-hooks

## Visibility

Public

## Core Question

Tsuku installs tools by downloading binaries and symlinking them, but some tools
need more than that -- shell functions, env files, completions, cleanup on removal.
Niwa's shell-integration design (tsukumogami/niwa#39) deferred tsuku generalization
because no mechanism exists. What should tsuku's lifecycle hook system look like so
tools can customize their post-install setup, pre-uninstall cleanup, and pre-upgrade
migrations?

## Context

The niwa shell-integration design doc (DESIGN-shell-integration.md in
tsukumogami/niwa PR #39) explicitly decided that niwa should own its own shell
integration rather than wait for tsuku to provide it. The rationale was that tsuku
has no post-install shell mechanism, generalizing would cost 200+ LOC across two
repos with only one consumer, and cobra handles completions per-tool.

However, tsuku already has building blocks: `set_env` and `run_command` actions
exist, shell hooks (command-not-found, activation) are installed by tsuku itself,
and `tsuku shellenv` manages PATH. What's missing is a way for recipes to declare
lifecycle behavior beyond the install step sequence.

The current install flow is: resolve version -> execute action steps -> copy to
tool dir -> symlink binaries -> verify. There's no post-install configuration
phase, no pre-uninstall cleanup, and no pre-upgrade migration step.

## In Scope

- Recipe-level lifecycle hook declarations (post-install, pre-uninstall, pre-upgrade)
- Per-tool shell integration sourced via tsuku shellenv or similar
- How hooks interact with tsuku's existing action system
- Security model for tool-defined hooks
- The specific niwa use case as a validating example

## Out of Scope

- Changes to niwa's shell-integration design itself
- Tsuku's core install/action system redesign
- Fish shell support (deferred in niwa's design too)
- Per-project `.tsuku.toml` activation (already exists)

## Research Leads

1. **What lifecycle hooks do other package managers support, and how do they declare them?**
   Homebrew has post_install and caveats, nix has activation scripts, apt has
   maintainer scripts. Understanding the landscape helps avoid reinventing poorly.

2. **How close is tsuku's current action system to supporting lifecycle hooks?**
   The `[[steps]]` array, `run_command`, and `set_env` actions exist. Can these be
   extended with a lifecycle phase qualifier, or does the system need new primitives?

3. **What specific post-install configuration do tools need beyond binary symlinking?**
   Niwa needs shell-init eval. Other tools may need completions, env files, man pages,
   or daemon registration. Survey the recipe registry for tools that would benefit.

4. **How should per-tool shell integration compose with `tsuku shellenv`?**
   Should shellenv gain a plugin directory (shell.d/) that it sources? Should tools
   register init scripts? How do tools that need eval-init (niwa, direnv-style) fit?

5. **What security constraints should lifecycle hooks operate under?**
   Post-install hooks that run arbitrary code change tsuku's trust model. What
   sandboxing or review is needed? How do other managers handle this?

6. **How should pre-uninstall and pre-upgrade hooks work in practice?**
   Uninstall needs to undo post-install config (remove shell.d entries, env vars).
   Upgrade needs to migrate state. What ordering guarantees matter?
