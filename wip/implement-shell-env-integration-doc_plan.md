# Documentation Plan: shell-env-integration

Generated from: docs/plans/PLAN-shell-env-integration.md
Issues analyzed: 2
Total entries: 4

---

## doc-1: docs/GUIDE-shell-env-customization.md
**Section**: (multiple sections)
**Prerequisite issues**: Issue 1, Issue 2
**Update type**: modify
**Status**: pending
**Details**: This guide already exists and documents the env/env.local split, migration behavior, and `tsuku doctor --fix`. Verify all content matches the implemented behavior: the managed `$TSUKU_HOME/env` comment text, the migration rules (export lines only, comments dropped, dedup-safe), and the `tsuku doctor --fix` output format (`Env file... FAIL` / `Env file is outdated (run: tsuku doctor --fix)`). The guide is pre-written against the design; confirm it matches the actual implementation rather than writing new content.

---

## doc-2: docs/guides/shell-integration.md
**Section**: Per-Tool Shell Init
**Prerequisite issues**: Issue 1
**Update type**: modify
**Status**: updated
**Details**: The "Per-Tool Shell Init" section (line 172) currently says init scripts in `$TSUKU_HOME/share/shell.d/` are sourced by `tsuku shellenv` automatically. After Issue 1, the init cache is sourced by `$TSUKU_HOME/env` at shell startup — no subprocess needed, no `tsuku shellenv` call required. Update this paragraph to reflect the new mechanism: `$TSUKU_HOME/env` sources the appropriate per-shell init cache (`.init-cache.bash` or `.init-cache.zsh`) automatically, so tools with shell functions are available in every new terminal after install. Remove or qualify the claim that `tsuku shellenv` is what sources init scripts.

---

## doc-3: README.md
**Section**: Tool Shell Integration
**Prerequisite issues**: Issue 1
**Update type**: modify
**Status**: updated
**Details**: Two places need updating. (1) The feature bullet at line 19 says "Recipes can register shell functions and completions automatically via `tsuku shellenv`" — update to reflect that shell functions are loaded via `$TSUKU_HOME/env` at shell startup, not via the `tsuku shellenv` command. (2) The "Tool Shell Integration" section (around line 896) repeats the same claim. Update the paragraph to explain that init scripts in `$TSUKU_HOME/share/shell.d/` are sourced via the managed `$TSUKU_HOME/env` file on every shell start, so no explicit `tsuku shellenv` call is needed. The `--no-shell-init` flag description and `tsuku info <tool>` note can stay unchanged.

---

## doc-4: README.md
**Section**: Tool Shell Integration (doctor --fix)
**Prerequisite issues**: Issue 1, Issue 2
**Update type**: modify
**Status**: pending
**Details**: After Issue 2 ships, add a brief note in the "Tool Shell Integration" section explaining what to do when shell functions aren't loading after install: run `tsuku doctor` to check if the env file is outdated, and `tsuku doctor --fix` to repair it. This gives users a self-service path when the automatic migration via `tsuku install` hasn't run yet. Keep it short — the full explanation lives in `docs/GUIDE-shell-env-customization.md`.
