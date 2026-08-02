# Exploration Scope: nvm-data-root

## Visibility

Public

## Scope

Tactical

## Execution Mode

auto (the dispatch brief carries an explicit autonomy mandate: do not stop for
approval at phase boundaries; resolve contested choices with the decision
framework rather than asking). Max rounds: 3.

## Topic

tsukumogami/tsuku#2464 -- `recipes/n/nvm.toml` exports `NVM_DIR={install_dir}`,
which resolves to `$TSUKU_HOME/tools/nvm-<version>`: a tsuku-managed tool version
subject to `GarbageCollectVersions` and a 7-day default retention. nvm treats
`NVM_DIR` as its *data root* (Node versions, aliases, global npm packages,
tarball cache, `default-packages`), not just its program directory. Upgrading nvm
therefore orphans -- and roughly a week later deletes -- everything the user
installed through it.

## Why now

PR #2465 (merged today) fixed #2439, which was that `set_env` had no effect at
all. Before that merge `NVM_DIR` was never really exported, so `nvm.sh`
self-located to `$TSUKU_HOME/share/shell.d` and installs accumulated there --
odd, but never garbage-collected. Making the export work is what moved user data
into a directory tsuku deletes on a timer. Users may already be in that state,
so migration is load-bearing, not optional.

## Leads (Round 1)

| # | Lead | Question it answers |
|---|------|---------------------|
| 1 | nvm upstream data-root contract | What exactly does nvm keep under `NVM_DIR`, how does upstream upgrade, and what does `nvm.sh` require of the directory it is sourced from? |
| 2 | tsuku deletion surfaces | Every code path that can delete something under `$TSUKU_HOME` -- GC, `RemoveVersion`, `RemoveAllVersions`, cleanup actions, uninstall -- and what each already protects. |
| 3 | Stable-path prior art inside tsuku | Does tsuku already have an unversioned, tool-owned, non-GC'd directory concept? `share/`, `shell.d`, `AtomicSymlink`, `ValidateSymlinkTarget`, `tools/current/`. |
| 4 | Recipe/action expressive power | What can a recipe actually express today? `set_env` value templating (`{install_dir}` and friends), post-install hooks, and whether a stable path can be created/populated declaratively. |
| 5 | Migration + detection surface | Where could a one-time migration or warning live, and how do existing installs get detected? Upstream design doc `docs/designs/current/DESIGN-shell-d-lifecycle.md` considered-options section. |
| 6 | Test + CI reality | What CI actually runs on a PR (`@critical` functional scenarios, `-short`, `git status --porcelain` cleanliness, errcheck/dupl), and where a "survives an nvm upgrade" assertion can live and actually run. |
