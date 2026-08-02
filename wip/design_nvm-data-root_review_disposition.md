# Phase 5/6 Review Disposition — nvm-data-root

Three reviews: mandatory security, architecture, pragmatism. Every finding is either
adopted or rejected with a reason. Two findings converged independently from different
lenses, which is the strongest signal in the set.

## Adopted — scope cuts

| # | Change | Source | Effect |
|---|---|---|---|
| 1 | **Drop the `dir` parameter from `install_program_files`.** The destination is computed Go-side by `dataDir(ctx)`. | security (must-change 1) **and** pragmatism (top cut), independently | Deletes the design's highest-severity security section outright. `dir` admitted `$TSUKU_HOME/share/shell.d` and `$TSUKU_HOME/bin` — both live surfaces — so a recipe could copy an attacker-controlled tarball file into the user's shell startup, bypassing `--no-shell-init`. No recipe-controlled path survives to validate. |
| 2 | **Cut `--purge`.** Removal preserves the data root and prints its path; the documented remedy is `rm -rf <printed path>`. | pragmatism | Serves no acceptance criterion. Also moots security must-change 2: `--purge` would have built an `os.RemoveAll` target from `args[0]` with no tool-name validation, and "tolerate an absent state entry" deletes the only guard that currently stops a traversal name (`filepath.Join("/home/u/.tsuku", "../../../etc/passwd")` = `/etc/passwd`, verified). The removal question is still answered explicitly — tsuku never deletes the estate — which is what the issue asked for. |
| 3 | **Cut the recipe-validator denylist rule** to a follow-up issue. | pragmatism | One subject, and that subject is fixed in this PR. A buggy rule turns the Validate Recipes job red across 1,449 recipes under `--strict`. Prospective value, present risk. |
| 4 | **Collapse `migrate_data_dir` into the `set_env` path**, gated on the data-root variable name, following `fixPipxShebangs` (`internal/install/manager.go:177`). | architecture (M3/W1) | Drops one registered action, one `ActionEvaluability` entry, one validator concern, one plugin-skill entry, and the `ExecutePhase`-abort hazard that forced the "return `nil` on every failure" contract. Also makes the ordering unbreakable: the design no longer has to state "ordered after `set_env`" as a constraint a future recipe edit can violate. |
| 5 | **Cut `doctor` verdicts 3 and 4** (rollback-crossed-boundary, upstream-`$HOME/.nvm` note). Keep verdicts 1 and 2 plus `--fix`, gated on nvm being installed and kept as one contiguous hunk. | pragmatism, architecture (W3) | Verdict 3 diagnoses a bug this design defers — it should ship with its fix. Verdict 4 is a support-FAQ answer wearing a health check. Verdict 2 (WARN) is retained because it is the only channel that tells the never-updated population where their data is, which acceptance criterion 4 requires. |

## Adopted — correctness and security hardening

| # | Change | Source |
|---|---|---|
| 6 | **Every stat in the merge-move is `os.Lstat`**, and every type test is `Mode().IsDir()` on an `Lstat` result or `DirEntry.IsDir()`. A symlink on either side is a conflict: leave it, report it, never follow it. | security 3 — with `os.Stat`, a symlink at `src/versions` pointing at `$HOME/Documents` makes the merge enumerate that directory and rename its entries into the data root. A move-anything primitive built out of an algorithm whose headline invariant is "no deletion primitive". |
| 7 | **The merge-move takes the entries to move as an argument and never removes its own source.** | security (informational) — the unconditional `os.Remove(src)` fits neither call shape, and for the pre-fix population the source *is* `share/shell.d`. "The migration deletes shell.d" is a surprising sentence to have to write later. |
| 8 | **`files` resolution is `EvalSymlinks` + containment, not lexical.** Open with `O_RDONLY\|O_NOFOLLOW` and `Stat` the handle so the type check and the read see the same object; write the temp destination with `O_CREATE\|O_EXCL\|O_NOFOLLOW`. | security 4 — a lexical `..` rejection says nothing about a symlinked directory component, and `os.Stat` reports a symlink-to-regular-file as regular. `copyFile` cannot be reused: it uses plain `os.Create`, which follows a destination symlink. |
| 9 | **Delete the design's claim that the extractor contains tarball symlinks.** It is false: `isPathWithinDirectory` and `validateSymlinkTarget` are both purely lexical, and a three-entry tarball (`a -> "."`, `b -> "a/.."`, then `b/pwned`) writes outside the destination — verified empirically. The new actions defend themselves rather than inheriting containment. | security 5 |
| 10 | **File modes:** `0755` if the source has any execute bit, else `0644` — not derived from the tar header. `data/` is created `0700`, matching `shell.d` and `completions` rather than `tools/`. | security (informational) |
| 11 | **`DataDir` gets the same `if != ""` guard `ShareDir` has in `EnsureDirectories`.** | architecture M2 — without it, `filepath.Join("", "data")` = `"data"` and `os.MkdirAll` creates a stray relative directory in the test process's working directory. Partial `Config{}` literals exist at `cmd/tsuku/doctor_test.go:48` and `post_install_test.go:26`. |
| 12 | **The unexpanded-`{data_dir}` guard is a runtime check in the expansion path**, mirroring `CheckUnexpandedDepVars` (`internal/actions/util.go:56-69`), not a list of expanding actions inside `validator.go`. | architecture M4 — a literal list in the validator drifts from the actions that actually expand, and the failure is a hard `--strict` error against a correct recipe. |
| 13 | **The notice is written from `cmd/tsuku/post_install.go`**, which already holds config, by re-running the shape detector — not from inside an action. | architecture M1 — writing from the action needs a `NoticesDir` on `ExecutionContext` (the most-constructed struct in the tree), plumbing through five construction sites, and a new `internal/actions` → `internal/notices` edge. Shape re-detection needs none of that and no state handoff. |
| 14 | **Assert `filepath.Dir(ctx.ToolsDir) == config.HomeDir`** rather than assuming it. | security (informational) |

## Adopted — gaps to close

| # | Gap | Source |
|---|---|---|
| 15 | The integration job itself was never an implementation step. The script needs a job in `integration-tests.yml` with `TSUKU_REGISTRY_URL` pointed at the head ref, or the modified recipe is never fetched. | pragmatism (missing 1) |
| 16 | Acceptance criterion 3 names `nvm alias default` explicitly; the test sketch asserted only `nvm ls` and `nvm exec`. Add an alias readback in a fresh shell after the upgrade. | pragmatism (missing 2) |
| 17 | `data/README` was named as the mitigation for the `rm -rf $TSUKU_HOME` consequence but appeared in no step. | pragmatism (missing 3) |
| 18 | The never-updates-nvm population needs a stated channel — the recipe header and the release note, not a health check. | pragmatism (missing 4) |

## Rejected

| # | Finding | Why |
|---|---|---|
| R1 | **Inline `internal/nvmdata` into `internal/actions`** — "the dependency-edge argument is defending against an edge nobody was going to add" (pragmatism). | Factually wrong, and checked: neither `internal/install` nor `internal/actions` imports the other today, and the removal rescue call site is *in* `internal/install`. Inlining into `internal/actions` would create exactly the `install → actions` edge. The same reviewer keeps the rescue, so the shared home is load-bearing. |
| R2 | **Cut the `doctor` check entirely** (pragmatism). | Partly adopted — two of four verdicts are cut. The WARN verdict survives because it is the only surface that tells an un-migrated user where their data is, which acceptance criterion 4's second clause requires. The reviewer's argument that `--fix` cannot repair `EXDEV` or conflicts is correct and is now stated in the design rather than implied. |

## Partially adopted

**Architecture W2 — the leaf package splits on the wrong axis.** The objection is sound:
`internal/` holds 44 packages and none is named for a tool; ecosystem-specific code in this
repo lives as *files inside general packages* (`brew_actions.go`, `gem_install.go`,
`nix_portable.go`). Adopted by renaming the package to **`internal/datamigration`** with the
general merge primitive in `merge.go` and the nvm predicate table in `nvm.go`. That matches
the repo's actual file-level convention, avoids the tool-named-package precedent, keeps the
nvm part a single deletable file, and leaves `internal/install` importing something general
it can keep after the migration is deleted.

**Architecture W4 — two definitions of the data-root path.** Not restructured. The design
follows the `ShareDir` precedent exactly, and the precedent is mildly bad. Adopted at
minimum: a shared constant for the `"data"` segment so the two computations cannot drift on
the spelling.

## New follow-up issues this review generated

- **The archive extractor's containment is purely lexical** and a symlink chain escapes it,
  verified with a three-entry tarball. Pre-existing, wider than this feature, and the most
  serious thing the review found.
- **No `ValidateToolName` exists.** `cmd/tsuku/remove.go:30-39` splits `args[0]` on `@` with
  no path-segment validation; the only thing currently preventing a traversal name from
  reaching a path construction is the state lookup. `envTargetName`
  (`internal/actions/set_env.go:209-217`) already has the check to lift.
- The recipe-validator denylist rule (cut item 3).
- Making `Activate` re-run `set_env`, with `doctor` verdict 3 shipping alongside it.
