# Decision 1: what keeps a tool's shell.d files correct for the active version

**Chosen: Option C, refined — version-key the shell.d filename, and have the cache
builder exclude a file only when `state.json` knows about it and it does not belong to
an active version.**

## How the decision ran

Three agents each built the strongest version of one option and then attacked it. Two
adjudicators cross-examined the results on different axes and reached different answers:
the acceptance-criteria adjudicator chose C, the landing-risk adjudicator chose B and
named A in its own counter-argument. That split is the useful part of the record.

## What settled it

**A fails the documented happy path.** `install_shell_init`'s `source_command` mode
cannot be re-rendered without executing a binary from the tool directory, so A refuses
it. No shipped recipe uses the mode today, which is how A justifies the gap — but
`recipes/README.md` presents `source_command = "{install_dir}/bin/tool-name init {shell}"`
as the example for post-install shell integration. It is the recommended pattern for
every tool with a `tool init bash` subcommand. A design whose coverage hole is the
documented pattern is carrying more risk than "zero recipes" implies. A also fails for
`VersionState.Plan == nil`, and its criterion-3 pass works by rewriting the recorded
`ContentHash` rather than reproducing the bytes, which makes the hash a weaker invariant
than the criterion implies.

**B cannot help anyone who already has the bug**, taxes a file parsed on every command,
and — decisively — stores a third copy of bytes that already exist twice. Both
adjudicators independently reached the same observation: `install_shell_init`'s
`source_file` mode reads `ctx.InstallDir`, the deleted staging directory, and that is the
*only* reason the content looks unrecoverable. `Manager.InstallWithOptions` copies
`workDir/.install` wholesale into the versioned tool directory, so a byte-identical copy
is already on disk for every installed version. Storing a compressed third copy in state
to recover bytes that never left is not a good trade.

**C removes the property that has to be maintained.** A and B both keep the invariant
"this file's content must agree with the active version" and add machinery to maintain
it — machinery that is correct only as long as every future lifecycle event remembers to
call it. C deletes the invariant. A file's content is fixed when written and never has to
agree with anything again, because it is only ever reachable from the version that wrote
it. `ContentHash` stops being a fact with a short shelf life and becomes a permanent
property. `source_command`, nil plans, and `--no-shell-init` all stop being special cases,
because nothing is ever re-derived.

**Two of C's advertised cost advantages are false and were struck.** C does not avoid
arming the golden-plan workflow — the #2439 fix ships in the same PR and touches
`internal/actions/action.go`, `internal/executor/plan.go`, and `internal/recipe/types.go`,
all on the allowlist. And `cmd/tsuku` is not a test-free package; it has 32 test files.
Neither correction changes the ranking, because neither favours a different option.

## The refinement, and why it matters

C as championed makes the cache a strict projection of `state.json`: only recorded,
active files are sourced. The risk adjudicator found that this converts three unrecorded
writers into silent breakage — `cmd/tsuku/plan_install.go` (fixable), and two that C did
not budget for: `Executor.installSingleDependency` builds a throwaway `ExecutionContext`
whose cleanup actions are discarded, and `cmd/tsuku/install_lib.go` never collects them
at all. A manifest-gated cache would silently drop a working integration written by
either.

The gate is not necessary. The rule that actually fixes the bug is narrower:

> Exclude a shell.d file from the cache if and only if `state.json` records that path for
> some installed version **and** that version is not the active one.

A file no version records is included exactly as today. That single change:

- eliminates the unrecorded-writer hazard entirely — paths the dependency installer and
  the library installer write behave precisely as they do on `main`;
- keeps the parameter optional, so passing nothing means excluding nothing, which is
  today's semantics — most of the 19 existing `RebuildShellCache` tests and the 11
  `CheckShellD` tests keep passing unchanged rather than each needing a fabricated
  manifest;
- still fixes the reproduction, because after the rename v1's and v2's files are distinct
  paths and only the active one is in the active set.

`cmd/tsuku/plan_install.go`'s missing `RecordCleanup` is still folded in — the two
post-install blocks are the same 25 lines copy-pasted and have already drifted apart once
— but it is now a correctness improvement rather than a prerequisite that gates whether
the feature silently breaks people.

## What C forces into scope, and what that costs

Five consumers must change or the rename introduces a regression. Four are pre-existing
bugs that C improves; one is a defect C creates.

| Consumer | Why | Pre-existing? |
|---|---|---|
| `internal/install/update.go` `StaleCleanupActions` | Old and new versions now share no shell.d path, so every old action reads as stale and `ExecuteStaleCleanup` deletes the rollback target's files. Silent: `CheckShellD` iterates directory entries, so a missing file produces no mismatch and `doctor` reports clean while the tool has vanished. | **No — C creates this.** Needs the `otherPaths` guard `remove.go` already has, plus a dedicated update-then-rollback test. |
| `internal/shellenv/doctor.go` `isCacheStale` | Recomputes the expected cache over every file on disk with no filtering, so it must apply the identical exclusion or `doctor` fails permanently on any multi-version install. | Yes — aligning the two also fixes the pre-existing `--fix` non-convergence. |
| `internal/shellenv/doctor.go` `HasShellIntegration` | Probes `{toolName}.bash` by name. Already wrong today, since the filename derives from the recipe's `target`, which need not equal the tool name. | Yes. |
| `internal/updates/gc.go` | Deletes tool directories without touching `state.json`, so a GC'd version's shell.d files would leak (~324 KB per GC'd nvm version, background auto-apply path only). | Yes — fixing it also closes the hole where a GC'd version's surviving `VersionState` holds the `otherPaths` skip open. |
| `cmd/tsuku/update.go` `warnShellInitChanges` and the two display-name derivations | Compare by path; must compare by `(target, shell)`. The cache comment and `doctor`'s `ActiveScripts` derive a display name from the filename and would show `nvm@0.40.6`. | Cosmetic / mechanical. |

The `StaleCleanupActions` regression is the single biggest risk in this decision. It is a
five-line guard, but its failure mode is deleting a live file that no existing test
covers, so it gets a dedicated test and a mutation test.

## Consequences for the brief's framing

The brief expected re-rendering, and said so while explicitly leaving the shape open:
"the shape above is an observation, not a mandated design." The answer this decision
reaches is that re-rendering is the wrong primitive — the right move is to make it
unnecessary. Lifecycle hooks still go in at all three `ActiveVersion` write sites, but
what they do is recompute a concatenation rather than reproduce a past version's bytes.

The capability C structurally cannot have is **repair**: if a user deletes or edits a
shell.d file, nothing can regenerate it. Adjudication established repair is not in scope
— criterion 3 is scoped by its own first clause, "after any of the above", meaning
operations tsuku itself performs, and no criterion mentions `--fix`. If repair is wanted
later, the cheap path is not a render capability: for `source_file` mode the bytes are
byte-identical to `tools/<tool>-<version>/<source_file>`, so a repair is a `copyFile` that
needs the stored plan only to learn the parameter. That is recorded as a follow-up.
