# Explore Scope: shell-d-lifecycle

## Visibility

Public

## Scope

Tactical

## Execution Mode

auto (the dispatch brief carries an explicit autonomy mandate: resolve contested
choices with the decision framework rather than blocking on a human)

## Core Question

Nothing in tsuku ever re-renders a `$TSUKU_HOME/share/shell.d/*` file. The
filenames are version-independent but their contents are version-specific, so any
event that changes which version is active — upgrade, activate, rollback, or the
removal of one version among several — leaves shell.d holding some other version's
content, with a `ContentHash` in `state.json` that no longer matches disk and no
repair path. What is the smallest correct mechanism that re-renders a tool's
shell.d files from whichever version is actually active, for every action that
writes there?

## Context

The gap surfaced while fixing issue #2439 (`set_env` had no effect). The narrow
fix, prototyped in PR #2442, routes `set_env` through the same `share/shell.d`
delivery `install_shell_init` already uses. That works for a single installed
version, but makes the missing re-render load-bearing: the reproduction is
`install nvm@0.40.5`, `install nvm@0.40.6`, `remove nvm@0.40.6 --force`, which
leaves `00-env-nvm.bash` pointing at the deleted `tools/nvm-0.40.6` while 0.40.5
is the active version.

The same run leaves `nvm.bash` — written by the pre-existing `install_shell_init`
— holding 0.40.6's entire `nvm.sh` script, so this is not a `set_env` defect. Any
fix must cover both actions; fixing only one produces a more confusing state than
today's.

Three cheap fixes are already ruled out with reasons: content cannot be restored
from state (`CleanupAction` stores only a non-invertible SHA-256), the file cannot
simply be deleted (the `otherPaths` skip in `Manager.executeCleanupActions` exists
precisely to protect the surviving version), and the content cannot be made
version-independent (`{install_dir}` means the versioned directory by definition).

Known constraints: PR #2442's branch must not be modified and the new PR must be
self-contained; `recipes/p/playwright.toml` is off limits; changes under
`internal/actions/`, `internal/executor/`, `internal/recipe/`, or
`internal/shellenv/` oblige a same-PR assessment of the `plugins/tsuku-recipes/`
and `plugins/tsuku-user/` skills; `main` may carry unregenerated
`testdata/golden/plans/embedded/gcc-libs/` drift.

Stakes: a stale shell.d entry points a user's live shell at a directory that no
longer exists, and `tsuku doctor` can detect it but `doctor --fix` cannot repair
it. Tests that assert only on file contents are explicitly insufficient — a prior
end-to-end attempt passed against a deliberately broken implementation.

## In Scope

- Re-rendering shell.d for every lifecycle event that changes the active version:
  install, upgrade, `activate`, rollback, and removal of one version among several
- Both writing actions: `set_env` and `install_shell_init`
- Keeping the recorded `ContentHash` in `state.json` consistent with disk, so
  `tsuku doctor` reports clean afterwards
- Whether `doctor --fix` should gain a repair path for content-hash mismatch
- Carrying issue #2439's own acceptance criteria (the `set_env` fix itself)
- Deciding, and stating, which of the four adjacent gaps belong in this feature

## Out of Scope

- Modifying PR #2442's branch (`fix/2439-set-env-exports`)
- `recipes/p/playwright.toml`
- Refactors of the action registry or executor beyond what the chosen design needs

## Research Leads

1. **How does the shell.d write / record / cleanup path work today, end to end?**
   Establish ground truth for every producer and consumer: which actions write
   under `share/shell.d`, how `RecordCleanup` and `ContentHash` are computed and
   stored, how `RebuildShellCache` concatenates, and what `doctor` checks and
   `doctor --fix` can repair. Everything downstream depends on this being exact.

2. **What is the complete set of lifecycle events that change which version is
   active, and what does each do today?** Install, upgrade, `activate`, rollback,
   `remove` of one version, `remove --all`, pin changes, project-level activation.
   A re-render hook is only correct if it fires on all of them; the brief names
   `Activate` and `Rollback` but the enumeration should be exhaustive rather than
   assumed.

3. **Is the information needed to re-render actually present in `state.json`, and
   for which installs?** The brief asserts `VersionState.plan` carries the
   surviving version's steps. Verify the schema, check whether the plan survives
   round-tripping, whether it is populated on every install path (`install`,
   `install --plan`, dependency installs, library installs), and what happens for
   versions installed by an older tsuku that never stored one.

4. **What re-render mechanisms are available, and what does each cost?** Replaying
   post-install steps from the stored plan; a dedicated render pass that is not an
   action execution; regenerating from the recipe rather than the plan; recording
   rendered content rather than a hash. This is the actual design question — the
   brief's "replay post-install steps" shape is an observation, not a mandate.

5. **Which of the four adjacent gaps must be fixed for the acceptance criteria to
   hold, and which are separable?** `plan_install.go` never calling `RecordCleanup`;
   `install_lib.go` never running a post-install phase;
   `Executor.installSingleDependency` ignoring phases with an empty `ToolInstallDir`;
   the already-installed short-circuit that `--force` does not bypass. Each needs
   an in-scope / own-issue call with a stated reason.

6. **What can the existing test infrastructure express for a multi-version
   lifecycle test, and where are its blind spots?** The acceptance criteria demand
   sourcing `$TSUKU_HOME/env` in a subshell and mutation-testing the guards.
   Establish what helpers exist for isolated `TSUKU_HOME`, synthetic recipes, and
   multi-version installs without network access.

7. **How do comparable version managers keep generated shell fragments consistent
   with the active version?** asdf, mise, rbenv/pyenv, nix profiles, Homebrew, and
   the `profile.d` convention all face the same problem. Worth knowing which ones
   re-render, which regenerate on every shell start, and which sidestep it with a
   stable indirection path.
