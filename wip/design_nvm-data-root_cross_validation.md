# Phase 3: Cross-Validation — nvm-data-root

Four decisions checked against each other and against the skeleton's own claims.
Three conflicts, three corrections. All resolved; none required re-running a decision.

## Conflicts between decisions

**C1. `tsuku remove nvm` destroys unmigrated data, contradicting decision 1.**
Decision 1 settles that removal preserves the data root. Decision 4's validator found
that `internal/install/remove.go` deletes `tools/nvm-<v>` at `:50`, `:151`, and `:258`
with no install path involved, so a user who never migrated loses their whole Node
estate on `tsuku remove nvm` — and `cmd/tsuku/remove.go:160` auto-removes orphaned
dependencies, so nvm can be reaped without the user ever naming it. Decision 1's
guarantee was therefore vacuous for exactly the users it was written to protect.

*Resolved by decision 3*, which adds a rescue call to the same merge-move immediately
before the `RemoveAll` at `:151` and `:258`, skipped under `--purge`. It needs no gate:
`executeCleanupActions` at `:147` has already torn the fragment down, so there is no
live export to invalidate. Decision 1's guarantee becomes unconditional.

**C2. `Activate` and `rollback` repoint `NVM_DIR` across the old/new boundary without
running post-install.** `Manager.Activate` (`manager.go:417-482`) flips `ActiveVersion`
and calls `rebuildShellCachesForTool` at `:479`, which only *selects* which existing
fragment enters the cache — it never rewrites a fragment's bytes. So once the fixed
recipe ships, `tsuku rollback nvm` to a version installed under the old recipe repoints
`NVM_DIR` at `tools/nvm-<old>` while the data now sits at `data/nvm`, and `nvm ls` comes
up empty.

*Resolved as a documented limitation with a detection surface, not a code fix.* The
data is not destroyed and rolling forward restores the view. Fixing it properly means
making `Activate` re-run `set_env` — the same `Activate` gap `DESIGN-shell-d-lifecycle.md`
already reasoned about, wider than this issue, and a change to the activate/rollback
path that this PR should not make. It gets a third `doctor` verdict row so the state is
visible and named rather than silent. Recorded in Consequences and as a follow-up.

**C3. Decision 2 records `delete_file` cleanups; decision 1 forbids `delete_dir`.**
Not a conflict on inspection — different granularity and different targets. Decision 2
records two `delete_file` entries for the program files it placed, which tsuku owns.
Decision 1 forbids ever recording a cleanup for the *data root itself*, because
`ReapVersion` executes cleanup actions from background GC. Both hold simultaneously:
`tsuku remove nvm` takes `nvm.sh` and `nvm-exec` and leaves `versions/` and `alias/`.

## Corrections to the skeleton's own claims

**X1. "A recipe-only change runs no Go jobs at all" is too strong.** `recipes/**` is
absent from the `code` filter (`.github/workflows/test.yml:371-376`), so unit and lint
jobs do not run — but a separate `recipes` filter at `:389-391` matches
`recipes/**/*.toml` and gates the integration jobs, and `platform-integration.yml`
triggers on `recipes/**` with no gate. Corrected to: a recipe-only change runs no
unit or lint job. The conclusion the claim supported — that the fix needs a Go
component to get meaningful coverage — is unaffected, since neither integration job
installs a Node version or upgrades nvm.

**X2. `manager.go:182`'s reachability is overstated.** The pre-rename `os.RemoveAll` is
real, but on the normal path `mgr.IsVersionInstalled` short-circuits at
`cmd/tsuku/install_deps.go:508` before the install ever reaches it. Reaching it requires
handing tsuku an external plan file naming an already-installed version
(`plan_install.go`). Corrected from "fires on every same-version reinstall" to "reachable
on the plan-install path". It remains a reason the data must leave `tools/`, just not
the urgent one.

**X3. "A stable root inside `$TSUKU_HOME` gets isolation for free" is overstated.**
`HOME` is overridden alongside `TSUKU_HOME` in `internal/sandbox/executor.go:333-334`,
`internal/validate/executor.go:243-244`, `internal/validate/source_build.go:157-158`, and
`cmd/tsuku/shelld_lifecycle_test.go:200`. The surviving weaker claim: `Config.HomeDir` is
isolated by parameter, `$HOME` by discipline — and `internal/actions/nix_portable.go:39-43`
and `internal/actions/util.go:653-658` already escape that discipline. The decision does
not rest on it; the claim is corrected rather than deleted.

## Assumption conflicts checked and cleared

- **`--no-shell-init`.** Decision 2 places program files regardless; decision 3's
  migration predicate is false because no fragment exists. Consistent, and the
  consequence is recorded honestly: a `--no-shell-init` user is never migrated, and gets
  program files but no export.
- **Placeholder reachability.** Decision 1 expands `{data_dir}` Go-side in a shared
  `internal/actions` helper rather than widening `GetStandardVars`; decision 2 requires
  the placeholder to be reachable from the placement action. The shared-helper form
  satisfies both. Had decision 1 kept it local to `SetEnvAction.Execute`, this would have
  been a conflict.
- **Notice keying.** Decision 3's notice is keyed `nvm-data-migration`, not `nvm`, so it
  cannot collide with the install-outcome notice that decision 2's action path writes.

## Scope trims applied

- **`set_env` `if_unset` (respect a user-set `NVM_DIR`).** Deferred. Decision 1 raised it
  as an adjacent recommendation and stated explicitly that it can be deferred without
  invalidating the location, the `{data_dir}` spelling, or the removal semantics. It is a
  separate behavior change to `set_env`'s emitted form affecting every future consumer,
  and it belongs in its own issue.
- Everything else from the four decisions is retained. The guardrail in particular must
  ship in the same PR: a `validator.go` change arms the `validator` paths-filter, which
  validates all registry recipes under `--strict`, so splitting it would leave `main` red.
