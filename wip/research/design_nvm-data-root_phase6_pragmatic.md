# Pragmatism Review: nvm-data-root

## Verdict

**Approve with cuts.** The core — a stable `data/` root, a `{data_dir}` placeholder, program-file
copies, and a merge-move in nvm's own post-install — is the right shape and each piece is
load-bearing. Roughly a third of the surrounding surface is not. Four items serve no acceptance
criterion at all, and one parameter is single-handedly responsible for the design's
highest-severity security section.

## Cut

| Item | Criterion served | Cut | Degrades to |
|---|---|---|---|
| `--purge` flag on `tsuku remove` | **none** | Yes | User runs `rm -rf ~/.tsuku/data/nvm` — which is exactly the path the remove message already prints. Saves a flag, an interactive confirm, `ExitNotInteractive` plumbing, the "tolerate an absent state entry" special case, and their tests. |
| `internal/nvmdata` leaf package | **none** | Yes — inline it | One unexported file next to `migrate_data_dir.go` in `internal/actions`. Nothing changes. The "one deletable unit" argument is satisfied by a file as well as by a directory; `internal/actions` and `internal/install` already don't import each other, so the dependency-edge argument is defending against an edge nobody was going to add. |
| `doctor` check + `--fix` stanza (all four verdicts) | **none directly** | Yes | The notice already carries the message *and* the literal `mv` remedy, on the one path (background auto-apply) where a `reporter.Warn` would be swallowed. The design itself rejects "`doctor` alone" because nothing in non-test Go points at `tsuku doctor` — that argument cuts equally against `doctor` as the *repair* channel. And `--fix` cannot repair either documented failure mode: `mergeMove` stops on `EXDEV` and on conflicts, so retrying reproduces the same result. It repairs only transient failures. |
| `doctor` verdict 4 (upstream `$HOME/.nvm`, `ok` with a note) | **none** | Yes, unconditionally | Nothing is wrong, nothing is done, nothing is fixed. This is a support-FAQ answer wearing a health check. If you keep any doctor check at all, drop this one first. |
| `doctor` verdict 3 (rollback crossed the boundary) | **none** | Yes | Diagnostics for a bug the design explicitly defers (`Activate` never re-runs `set_env`). Ship the diagnostic with the fix, in the follow-up. |
| Recipe-validator denylist rule | **none** | Yes → follow-up issue | The rule has exactly one subject, and that subject is being corrected in this same PR. A denylist of variable names is a guess about recipes nobody has written. The "cheapest moment" argument is true but it argues for *when*, not *whether*; the moment is equally cheap next month. Failure mode of not shipping it is identical to the failure mode the design already accepts for an omitted variable. |
| The `dir` parameter on `install_program_files` | — | Yes — **highest-value cut in the review** | Compute the destination Go-side as `dataDir(ctx)`. This deletes the entire "Recipe-controlled paths become a delete primitive if unvalidated" section: no `filepath.Clean` + absolute check + inside-`$TSUKU_HOME` check + not-under-`tools/` check, and no recipe-named path ever reaches `executeSingleCleanup` (`internal/install/remove.go:417-434`). One deleted param removes a security consideration, a preflight branch, and its tests. The action currently has one caller and will have one caller. |
| Partial-removal messaging ("rescued but not pointed at") | none | Trim | Print the data root path unconditionally on remove; skip the branch that distinguishes partial from whole removal. |

## Keep

| Item | Why it earns its place |
|---|---|
| `$TSUKU_HOME/data/` in `internal/config` | Criteria 1 and 2. Genuinely three lines, following `ShareDir` at `internal/config/config.go:333`, `:367`, `:392-393`. The `share/` rejection is correct: `share/` carries a regenerable-contents policy and this data needs the inverse. |
| `{data_dir}`, per-tool, expanded Go-side | Criteria 1 and 2. `GetStandardVars` (`internal/actions/util.go:28-37`) expands six placeholders, none stable, and `set_env` single-quotes values, so there is no recipe-only escape. Keeping expansion local to the consuming actions rather than widening `GetStandardVars` is the right call. Per-tool over bare-root is right — a bare root re-permits `{data_dir}/nvm-{version}`. |
| `install_program_files` (minus `dir`) | Not literally in the criteria — `nvm exec`/`nvm run` are not among the named subcommands — but criterion 3 says "behave as with upstream," and shipping a known `rc=127` on two subcommands trades one bug for another. The copies-over-symlinks argument is sound and verified: `Manager.Activate` (`internal/install/manager.go:417-481`) repoints symlinks and rebuilds caches without running post-install, so anything a recipe materializes tracks the last-installed version. |
| `migrate_data_dir` in nvm's post-install, after `set_env` | Criterion 4. The "data and pointer move in the same event" argument is the load-bearing insight of the document, and the rejection of a general install hook is correct on the merits (the extra firings are precisely the unsafe ones). |
| `mergeMove` with no deletion primitive, no `copyDir` fallback | This is what makes the operation safe unattended. The `os.Lstat` requirement is real. Keep the invariant comment and its test. |
| Rescue call at `remove.go:151` and `:258` | Six lines calling the same helper. Note: strictly, `tsuku remove` is not "retention," so criterion 4 does not require this — but it is the same class of data loss the issue is about and it is nearly free. The deprecated `Manager.Remove` (`internal/install/remove.go:21`) has **zero non-test callers**; do not touch it. |
| `notices.KindDataMigration` + render case | Nine lines, and without them the notice falls into the `default` branch at `internal/updates/notify.go:249-252`, which prints `"Update failed: nvm -> : <err>"` followed by `Run ... 'tsuku rollback nvm' to revert` — telling a user with a half-migrated estate to run a command that cannot help. The non-empty-`Error` routing trick is genuinely clever and free. Keep. |
| Recipe change + header comment | Criterion 5. |
| Fast guard in `cmd/tsuku/shelld_lifecycle_test.go` | Verified: the file exists, has no `testing.Short()` guard, and sits where install/actions/shellenv meet. |
| Plugin skill updates | Mandated by `CLAUDE.md:136-155`, not discretionary. Not scope creep. |

## Missing

1. **No CI job.** Criterion 6 requires a test that installs nvm, installs a Node version, upgrades
   nvm, and asserts `nvm ls`. The design says the script "goes in `test/scripts/` invoked from
   `integration-tests.yml`" but the eleven-step implementation list has no step for adding the job.
   The trigger is fine — `.github/workflows/integration-tests.yml:12-19` fires on `**/*.go` for
   pull requests, and this PR touches Go — but the job itself has to be written, with the
   `TSUKU_REGISTRY_URL` pointing at the head ref the way the existing jobs do, or the modified
   recipe is never fetched. **Add it as an explicit step.**
2. **Aliases are named in the criteria but absent from the test sketch.** Criterion 3 names
   `nvm alias default` explicitly; the integration script asserts `nvm ls` and `nvm exec` only.
   Add an `nvm alias ls` (or a `default` alias readback in a fresh shell) assertion after the
   upgrade.
3. **`data/README` is named as required in Consequences but appears in no implementation step.**
   It is the only mitigation offered for the `rm -rf $TSUKU_HOME` consequence. Put it in the list
   or drop the claim.
4. **The never-updates-nvm population is neither migrated nor told.** The design concedes this and
   argues the user is not broken, which is true. But criterion 4 reads "migrated, **or** the user
   is told" — if `doctor` is cut, this population gets no channel at all. The honest answer is a
   line in the recipe header and the release notes, not a health check. Say so explicitly rather
   than leaving it to the reader.

## Scope

The deferred list is well chosen and nothing on it is load-bearing for the stated criteria. The
`Activate`/`set_env` rollback gap in particular is correctly deferred — and doctor verdict 3
should be deferred with it rather than shipped as a standalone diagnostic for a bug that stays
open.

The creep runs the other way: four *included* items (`--purge`, the validator rule, the
`$HOME/.nvm` note, the `nvmdata` package boundary) are follow-ups in disguise. This is a design
that solved every problem it found rather than the problem it was given — which is why it is
excellent analysis and a 25-file PR for a labelled bug fix.

## The smaller fix

```
internal/config/config.go        DataDir field + DefaultConfig + EnsureDirectories   (3 lines)
internal/actions/util.go         dataDir(ctx) helper
internal/actions/set_env.go      {data_dir} expansion
internal/actions/install_program_files.go   files list only, destination computed Go-side
internal/actions/migrate_data_dir.go        + unexported mergeMove and predicates, same package
internal/notices/notices.go      KindDataMigration
internal/updates/notify.go       one render case
internal/install/remove.go       rescue at :151 and :258
recipes/n/nvm.toml               set_env value, two steps, header comment
cmd/tsuku/shelld_lifecycle_test.go          fast guard + end-to-end migration assertion
test/scripts/test-nvm-upgrade.sh + a job in integration-tests.yml
plugins/                         per CLAUDE.md
```

Twelve files instead of twenty-five, with no acceptance criterion unserved.

**What it gives up, honestly:** no `--purge` (the user types `rm -rf`, having been told the path);
no second delivery channel for a failed migration (the notice is the channel, and it survives the
background path by construction — which is the whole reason it was designed that way); no
guardrail against a future recipe repeating the bug (one follow-up issue, filed the same day); no
answer printed for the user with a pre-existing `$HOME/.nvm`.

**On the design's own rejections:** its case against `$HOME/.nvm`, `share/nvm`, `state/nvm`, a
bare-root `{data_dir}`, symlinks, hard links, `run_command`, extending `install_shell_init`, a
general install hook, `delete_dir` cleanup, and a one-shot `tsuku migrate` all hold up — I am not
proposing to reopen any of them. The one rejection that boomerangs is "`doctor` alone," whose
stated reason (nothing in the tree points at `tsuku doctor`) applies with equal force to keeping
`doctor` as the repair surface.
