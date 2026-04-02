# Explore Scope: shell-env-integration

## Visibility

Public

## Core Question

When `tsuku install` runs a tool with `install_shell_init` (like niwa), the shell
functions get written to `~/.tsuku/share/shell.d/` and the init cache is rebuilt.
But `~/.tsuku/env` — the static file sourced from `.bashrc` — only sets PATH. It
doesn't source the init cache, so tools' shell functions silently don't load in new
terminals. How do we fix this so any tool with `install_shell_init` automatically
works after install, without users needing to manually edit their dotfiles?

## Context

- `~/.tsuku/env` is intentional — written by the installer, sourced from `.bashrc`
- `tsuku shellenv` exists separately and does source `.init-cache.bash` conditionally
- The two mechanisms have diverged: `env` only handles PATH, `shellenv` handles both
- The installer script is the primary path; self-update is the other reliable path
- tsuku also needs to handle users who installed it outside the official installer
  (warn them, offer `tsuku doctor` or equivalent to fix their shell setup)
- The niwa recipe also has a bug (`source_command` used bare binary name instead
  of `{install_dir}/bin/niwa`) — that's a separate but related fix in the niwa repo

## In Scope

- How `~/.tsuku/env` is generated and updated
- The relationship between `~/.tsuku/env` and `tsuku shellenv`
- How the installer and self-update path configure the shell
- `tsuku doctor` as a detection/repair mechanism
- How many recipes use `install_shell_init` (scope of the problem)
- Migration path for existing users with old `~/.tsuku/env`

## Out of Scope

- The niwa recipe `source_command` fix (known, separate PR in niwa repo)
- Changing the fundamental tsuku architecture (no major refactors)
- Non-bash/zsh shells

## Research Leads

1. **How is `~/.tsuku/env` generated, and who writes it?**
   Understand when and by what code path `~/.tsuku/env` is created. Is it written
   by the installer script, by `tsuku` itself during some setup command, or both?
   Does anything ever update it after initial creation?

2. **What does `tsuku shellenv` do, and why do both `env` and `shellenv` exist?**
   Read `cmd/tsuku/shellenv.go` in detail. Understand why `tsuku shellenv` exists
   alongside `~/.tsuku/env`. What did the design intend for each mechanism? Is there
   any existing plan to consolidate them?

3. **What does `tsuku doctor` currently check, and can it detect/repair broken shell setups?**
   Read the recently added doctor command. What checks does it run? Does it check
   whether the init cache is being sourced? Could it be the right place to detect
   and repair shell integration issues for users with old env files?

4. **How many recipes use `install_shell_init`, and what do they require?**
   Search the recipe registry and codebase for `install_shell_init` usage. How
   common is this action? Are there other tools besides niwa that would silently
   fail with the current `~/.tsuku/env`?

5. **How does the installer script configure the shell, and what does the self-update path do?**
   Find the install script (likely in the repo or referenced from README). What does
   it write to `.bashrc`/`.zshrc`? Does the self-update mechanism also update shell
   configuration? Is there a gap between what the installer sets up and what tsuku
   now needs?
