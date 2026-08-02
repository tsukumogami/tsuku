# Cross-examination: acceptance-criteria conformance

Adjudicated against the code on branch `docs/shell-d-lifecycle` (tip `89dbc7e3`, based on
`origin/main`) and against `origin/fix/2439-set-env-exports` for the #2439 prototype.
Every claim below that decides an outcome was read in the source, not taken from an
advocate.

## Two corrections that change the field before the matrix

**The #2439 prototype is part of the deliverable, and it arms the golden-plan workflow no
matter which option wins.** Criterion 4 requires that `set_env` exports reach the user's
shell and that `{install_dir}` expands from `ToolInstallDir`. Neither holds on `main`:
`internal/actions/set_env.go:46` writes `{install_dir}/env.sh` and nothing ever sources it,
and `GetStandardVars` is called with `ctx.InstallDir`, the staging directory. Criterion 1
requires the fix "for **both** `set_env` and `install_shell_init`", which is only meaningful
once `set_env` writes into `share/shell.d/`. So the prototype ships with this work — and the
prototype touches `internal/actions/action.go`, `internal/executor/plan.go`, and
`internal/recipe/types.go`, three files on the `validate-golden-code.yml` allowlist
(`.github/workflows/validate-golden-code.yml:15,37`).

That nullifies a cost argument both B and C lean on. C states "**No allowlisted golden-plan
file is touched**" and calls the saving "real, not merely theoretical"; B states "Golden-plan
workflow: not triggered." Both are true of the option in isolation and false of the PR that
would actually ship. The workflow is armed either way. Option A's "one allowlisted file"
(`internal/version/resolve.go`) therefore costs nothing incremental — it is one more file in
a diff that already trips the filter. **Golden-workflow arming is not a discriminator between
these three options.**

**`cmd/tsuku` is not a package without tests.** A and C both repeat the exploration's claim
that `cmd/tsuku` "has no unit tests today". There are 32 `package main` test files, including
`cmd/tsuku/doctor_test.go`, which already contains `TestDoctorFix_CacheRebuildWithHashes` and
`TestDoctorFix_NeverCallsRebuildWithNilHashes` — exactly the shape criterion 5 needs. The
test-placement constraint the exploration flagged is much weaker than the options assume, and
it stops being a reason to prefer B (or to sever the import cycle for A).

## Criterion-by-criterion matrix

| # | Criterion | A (render capability) | B (bytes in state) | C (version-keyed name + manifest) |
|---|---|---|---|---|
| 1 | Remove either of two versions, shell.d correct for the active one, both writers | **Pass** — `syncShellD` at `remove.go`'s promotion branch renders the survivor's bytes; `ownedNow` gates writes to paths that version recorded | **Pass** (post-B installs only) — restores the survivor's stored bytes; inert for anything installed before B lands | **Pass by construction** — the survivor's file was never overwritten, because installing v2 wrote a different filename; only the cache is recomputed |
| 2 | `activate` and rollback re-render shell.d | **Pass** — hook in `Manager.Activate` (`manager.go:459`) covers activate, rollback (`manager.go:364` delegates) and auto-apply's rollback (`updates/apply.go:173`); fails for `source_command` and `Plan == nil` | **Pass** (post-B installs only) — `RestoreShellFiles` at the same hook, same three paths | **Pass by construction, with a hook** — same three sites gain a `RebuildShellCache`, but the *files* are not re-rendered; what changes is which file the cache sources. See below |
| 3 | `doctor` reports no hash mismatch; recorded `ContentHash` matches disk | **Pass, by rewriting the record** — §6 recomputes the hash and overwrites `ContentHash`, so agreement is achieved by mutating the record rather than reproducing the bytes. Fails wherever Render is refused | **Pass** — restore writes bytes verified against the recorded hash, so the record is never rewritten | **Pass by construction** — a version's file is only ever written by that version, so its hash is permanently correct. **Conditional on C's own mandated `isCacheStale` fix**; without it `doctor` FAILs forever on any multi-version install |
| 4 | #2439's criteria still hold | **Partial** — A rewrites `set_env.Execute` as `Render` + `WriteRenderedFiles` and replaces `envTargetName(ctx.Recipe.Metadata.Name)` with `RenderContext.Tool`. Verified equal on both recording paths (`install_deps.go` uses `toolName`; `plan_install.go:49` sets `Name: effectiveToolName`), but it is a rewrite of the code criterion 4 protects | **Pass, by non-interference** — B touches no file in `internal/actions` at all | **Pass** — filename changes, behaviour does not. The `00-env-` sort guarantee survives and is strengthened; C's charset rule (`^[A-Za-z_]...`) subsumes the prototype's `HasPrefix(target, EnvFilePrefix)` rejection, since a leading `0` is now illegal |

Criterion 5 (end-to-end multi-version test that sources `$TSUKU_HOME/env`) and criterion 6
(`go test`/`vet`/`gofmt`) are satisfiable by all three and do not discriminate. Criterion 7
(assess `plugins/tsuku-recipes` and `plugins/tsuku-user` skills) is triggered for all three,
because the #2439 prototype already edits
`plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md` — whose `set_env`
entry currently reads "Creates an env.sh file that tsuku sources when activating the tool",
which is false on `main`. `plugins/tsuku-user/skills/tsuku-user/SKILL.md:117` also describes
shell.d and needs a look under C's rename.

## Q1 detail — criterion 2 and Option C

The criterion says "Switching the active version (`activate`, and rollback) **re-renders
shell.d**." Under C, `Manager.Activate` gains a `RebuildShellCache` call; no `share/shell.d/*`
file is written. C's advocate concedes the reframing: "activate re-renders shell.d" becomes
"activate rebuilds a cache from files already on disk."

**The case that this is an evasion.** The criterion names an action ("re-renders") and an
object ("shell.d"). C performs neither: nothing is rendered, and the object it touches is a
derived cache, not the shell.d files. A reviewer reading the criterion would reasonably expect
`ls -l share/shell.d/` to show a changed mtime after `tsuku activate`. More substantively, the
criterion probably encodes an intent — *the mechanism must be able to produce a version's
content at a moment when that version is not being installed* — and C is precisely the option
that declines to build that capability. Reading it as satisfied lets C redefine the
requirement to match what it does.

**The case that this is satisfaction.** Three things decide it.

First, `share/shell.d/.init-cache.{bash,zsh}` *is* a shell.d file, and it is the only one the
user's shell ever reads. `config.EnvFileContent` (`internal/config/config.go:446-465`) sources
exactly `share/shell.d/.init-cache.$shell` and nothing else — no glob, no per-tool sourcing.
So "re-render shell.d" and "recompute the cache" name the same observable change to the
directory the user consumes. Under C, `activate 0.40.5` changes the bytes in shell.d and
changes what the shell gets.

Second, and decisively, criterion 5 supplies the criteria author's own operational definition:
"source `$TSUKU_HOME/env` in a subshell, assert the variable points at the surviving version's
directory. **Asserting on file contents alone is explicitly not sufficient.**" That sentence
rules out a file-content reading of criterion 2 and demands a behavioural one. C passes the
behavioural test that criterion 5 mandates; a design that rewrote `nvm.bash` but forgot to
rebuild the cache would fail it. The criteria are written to be indifferent to *how* the right
bytes reach the shell.

Third, the brief itself says "The remaining direction is genuine re-rendering ... the shape
above is an observation, not a mandated design." "Re-render" is descriptive vocabulary from
the problem statement, not a specified mechanism.

**Decision: satisfaction, recorded as pass-by-construction with a hook.** C does not eliminate
the lifecycle hook — it eliminates the content derivation at the hook. That distinction is
worth naming in the design doc precisely because C's advocate is right that "no re-render at
all" is not accurate and the doc should not say it.

One caveat that is not an evasion but is a real difference: `RebuildShellCache` derives its
wrapper comment from the filename (`internal/shellenv/cache.go:135`,
`toolName := strings.TrimSuffix(name, suffix)`), and `CheckShellD` builds `ActiveScripts` the
same way (`internal/shellenv/doctor.go:88`). Under C the cache would read `# tsuku: nvm@0.40.6`
and `tsuku doctor` would list the tool as `nvm@0.40.6`. Cosmetic, but it refutes C's stated
claim that "**nothing parses the filename**" — two places do, for display — and it adds two
more sites to C's forced-change list that the advocate's walk missed.

## Q2 — is repair in scope

**No, not general repair. Yes, convergence.**

Read literally, criterion 3 is scoped by its own first clause: "**After any of the above**,
`tsuku doctor` reports no content-hash mismatch." "The above" is criteria 1 and 2 — removal and
activation, operations tsuku itself performs. The criterion demands that tsuku's own lifecycle
events not leave a mismatch behind. It says nothing about a file a user deleted, edited, or
lost to a disk error. No criterion mentions `--fix` at all.

The brief reinforces this by classification: it "states, as *problem framing* rather than as a
criterion, that `doctor --fix` cannot repair the hash mismatch today." Framing explains why the
bug matters; it is not a deliverable. Criterion 5's test is a prevention test — install, remove,
source, assert — with no damage-and-repair step anywhere in it.

So C's top falsifier ("when `doctor` reports a hash mismatch, must `--fix` fix it?") answers
**no**, and C's biggest self-identified weakness is neutralized for this decision. All three
options satisfy criterion 3 by making the tsuku-caused mismatch not happen; only B and A can
additionally repair externally-caused damage, and nothing asks them to.

**The one thing that *is* forced, and that the criteria do not name explicitly.**
`doctor --fix` cannot converge today: `RebuildShellCache` excludes a hash-mismatched file
(`cache.go:119-131`) while `isCacheStale` recomputes the expected concatenation over every file
on disk with no hash filtering (`shellenv/doctor.go:146-174`), so the re-check always reports
stale. That is a pre-existing bug. But it becomes *this PR's* bug for any option that starts
passing hashes into `RebuildShellCache` on the install and remove paths, because every newly
excluded file turns into a permanent `doctor` FAIL — a direct criterion-3 violation. C mandates
aligning the two functions; **Option A explicitly declines** ("That is a separate bug and should
be a separate issue") while simultaneously threading hashes into `m.rebuildShellCaches`. A
cannot have it both ways: with hashes threaded and `isCacheStale` unaligned, the very fallback
A relies on for `source_command` and nil-plan versions produces a `doctor` that fails forever.

## Q3 — the hard cases

| Case | A | B | C |
|---|---|---|---|
| `install_shell_init` in `source_command` mode | **Fail** — typed refusal (`ErrNotRenderable`); file keeps the wrong version's bytes, gets excluded from the cache, tool vanishes from new shells, `doctor` FAILs (permanently, given A's stance on `isCacheStale`) | **Pass** — stored bytes are mode-agnostic; this is B's one unique capability | **Pass by construction** — the bytes are never re-derived, so the mode is irrelevant |
| `VersionState.Plan == nil` | **Fail** — the plan is A's only content input; warn and no-op | **Pass** — B never reads the plan | **Pass** — C never reads the plan; it is the only option that touches the stored plan not at all |
| `--no-shell-init`, not persisted | **Pass** — `ownedNow` is the gate, and A additionally *deletes* the demoted version's file on promotion | **Weak** — restore never deletes, so promoting to an opt-out version leaves the other version's file on disk and in the cache; the user still gets the tool they declined | **Pass** — the opt-out version records no path, so nothing is in `Active`; the older file is on disk and unreachable |
| Tool directory GC'd, `VersionState` survives | **Partial** — stat check, warn, leave; degrades to containment | **Partial** — restore reads state, not the directory, so it will happily write a file exporting a path to a deleted directory. `Activate` refuses a missing directory (`manager.go:440-443`) but `RemoveVersion`'s promotion via `getMostRecentVersion` does not | **Partial → Pass** with C's mandated GC change; without it, a permanent per-version disk leak (see Q5) |

`source_command` deserves more weight than the advocates give it. A argues the refusal costs
"literally zero today" because no shipped recipe uses the mode. True of `recipes/`, but
`recipes/README.md:44-51` presents `source_command = "{install_dir}/bin/tool-name init {shell}"`
as *the* documented example of a post-install shell-integration step — it is the recommended
pattern for every tool with a `tool init bash` subcommand. A design whose coverage gap is the
documented happy path is carrying more risk than "zero recipes" suggests. C and B both cover it;
only A does not.

Netting the four cases: **C covers three cleanly and the fourth with a change it already
mandates. B covers two and is weak on one. A covers one and fails two.** A's two failures are
its own stated structural limits, not oversights — but they are exactly the cases where a
lifecycle hook has nothing to work from.

## Q4 — hybrids

**C plus "the cheap half of Option D" is not a hybrid — it is already C.** C changes
`RebuildShellCache` and `CheckShellD` from a variadic-optional hash map to a required
`ShellDManifest`, and the manifest carries `map[path]hash` for the active version at all five
call sites. Threading the active version's hashes through the four sites that pass none today
*is* C's construction, motivated for a different reason (under version-keyed naming, "pass
nothing" would mean "concatenate every version of every tool"). There is no extra scope to
weigh. Worth stating plainly in the design doc so a reviewer does not think it was skipped.

The important corollary, which applies to any option and is easy to get wrong: **you cannot
thread hashes without also aligning `isCacheStale`.** Doing one without the other converts every
excluded file into a permanent `doctor` failure and breaks criterion 3. C treats this as a
prerequisite; A treats it as someone else's issue while doing the thing that triggers it.

**C plus a minimal `source_file`/`set_env` render as a `doctor --fix` repair path: reject.**
It reintroduces exactly the machinery C exists to avoid — a render entry point, a
`RenderContext`, and either the `internal/version` → `internal/install` import sever or a
parallel registry — for a capability Q2 establishes is not in scope. It also lands in the same
PR as the #2439 prototype, the manifest gate, the `StaleCleanupActions` fix, the `isCacheStale`
alignment, the GC obligation, the `plan_install.go` `RecordCleanup` addition, and the
`HasShellIntegration` replacement. That PR is already at the edge of reviewable. This is scope
creep and it endangers the single reviewable PR.

If repair later proves necessary, there is a cheaper path than a render capability that C
should note as a follow-up rather than build now: for `source_file` mode the bytes are
byte-identical to `tools/<tool>-<version>/<source_file>` (`InstallWithOptions` copies
`workDir/.install` wholesale at `manager.go:168-199`), so a repair is a `copyFile`, not a
render. It needs the stored plan only to learn the `source_file` parameter.

## Q5 — verification of Option C's three forced changes

**1. `StaleCleanupActions` would delete a still-installed version's shell.d file. CONFIRMED, and
it is worse than "a genuine regression" — it is a silent one.**

`internal/install/update.go:14-36` computes old-minus-new keyed on `(action, path)`. Under
version-keyed naming the old and new versions share no shell.d path, so *every* old action falls
through as stale. `ExecuteStaleCleanup` (`update.go:43-62`) then calls `executeSingleCleanup`,
which is an unconditional `os.Remove` (`remove.go:358-377`). The single caller is
`cmd/tsuku/update.go:182-183`, guarded only by `newVersion != previousVersion &&
len(oldCleanupActions) > 0` — the normal update path.

What breaks concretely: `tsuku update nvm` from 0.40.6 to 0.40.7 deletes
`share/shell.d/nvm@0.40.6.bash` and `.zsh` while 0.40.6 is still in `ts.Versions` and is the
`PreviousVersion` rollback target. A subsequent `tsuku rollback nvm` then re-points binaries and
rebuilds the cache from a manifest naming files that no longer exist. The failure is silent in
the worst way: because `CheckShellD` iterates *directory entries*, a missing file produces no
hash mismatch and no stale-cache signal — `doctor` reports clean while nvm has silently
disappeared from new shells. Criterion 3 would pass vacuously while criterion 2 fails.

Would an existing test catch it? **No.** `internal/install/update_test.go`'s
`TestStaleCleanupActions` is a pure-function table whose every case is hand-written with
version-independent paths (`share/shell.d/tool.bash`, `share/shell.d/tool.zsh`). Renaming the
production filenames changes nothing about that table; it keeps passing. There is no integration
test exercising `update` → `rollback` against a real shell.d directory.

**2. `isCacheStale` would return true forever. CONFIRMED.**

`internal/shellenv/doctor.go:146-174` reads the cache, then rebuilds the expected content by
concatenating every file in `files` — and `files` comes from `CheckShellD`'s loop over all
directory entries (`doctor.go:66-92`), with no hash filtering and no manifest. Under C the
directory holds one file per installed version while the cache holds only the active one, so the
recomputed expectation never equals the cache and `CacheStale[shell]` is `true` on every
multi-version install. `HasIssues()` returns true, `runDoctorChecks` sets `failed`, and
`tsuku doctor` exits non-zero permanently. C is right that this is mandatory, and right that
aligning it also fixes the pre-existing `--fix` non-convergence.

**3. GC would leak shell.d files. CONFIRMED. Magnitude: real but modest, and bounded to one path.**

`internal/updates/gc.go:15-69` reads `toolsDir`, matches directories by the `toolName + "-"`
prefix, protects the active and previous versions, and `os.RemoveAll`s the rest. It never opens
`state.json` and never consults `CleanupActions`. Under C the GC'd version's shell.d files
survive with no owner that can ever reach them — `Activate` refuses a version whose directory is
missing (`manager.go:440-443`), so the files can never re-enter `Active`.

Magnitude, corrected: `nvm.sh` 0.40.6 is **161,810 bytes** (Option B measured the real upstream
tarball), and `install_shell_init` writes one copy per shell, so **~324 KB per GC'd nvm version**
— not the ~200 KB C estimates. The `00-env-` files add ~50 bytes each and are negligible. But the
leak has exactly one trigger: `GarbageCollectVersions` is called from a single non-test site,
`internal/updates/apply.go:153`, the background auto-apply path, with a 7-day retention
(`userconfig.go:169`). Manual `tsuku install`/`tsuku remove` never GC. And exactly one shipped
recipe writes a large shell.d file. So the realistic worst case today is a user with auto-apply
enabled on nvm: a few hundred KB per year, low single-digit MB over the life of the install.

That is small enough that the leak alone would not justify blocking C — but the GC change C
proposes (run the deleted version's `delete_file` cleanups, drop its `VersionState`) is worth
doing anyway, because it closes the separate pre-existing hole where a GC'd version's surviving
`VersionState` holds the `otherPaths` skip open for a directory that no longer exists, and
because it stops `doctor` from running a `bash -n` syntax check over accumulating dead files on
every invocation.

**Assessment of the set.** All three are real. Two of the three (`isCacheStale`, GC) are
pre-existing bugs that C forces into scope and improves. The first is not: `StaleCleanupActions`
under version-keyed naming is a *new* defect that C creates, whose failure mode is deleting a
live file, which no existing test detects, and which fails silently rather than loudly. C's
advocate names this honestly and rates it correctly. It is the single biggest risk C carries and
it needs a dedicated test — `update` then `rollback` against a real shell.d directory, asserting
the rollback target's file survives — plus the mutation-test discipline criterion 5 requires
(revert the `StaleCleanupActions` guard, confirm the test fails).

## Verdict

**Option C.**

It is the only option that passes all four criteria without a coverage asterisk. A fails
criterion 2 for `source_command` and for `Plan == nil`, and its criterion-3 pass depends on
rewriting the `ContentHash` record rather than reproducing the bytes — which makes the hash a
weaker invariant than the criterion implies, and which A pairs with a deliberate refusal to fix
the `doctor --fix` non-convergence its own hash threading makes reachable. B passes cleanly but
only for versions installed after it lands, cannot repair or even improve any existing install,
concedes it "cannot be the only mechanism," and taxes a hot path that is already the dominant
cost of `tsuku list`. C's criterion 1 and 3 passes are by construction — the surviving version's
file is never overwritten, so its recorded hash is permanently accurate — and Q2 removes the
scope question that was C's own top falsifier. On the four hard cases in Q3, C is the only option
that handles `source_command`, nil plans, and `--no-shell-init` together.

Two of C's cost advantages do not survive scrutiny and should be struck from the design doc: it
does not avoid arming the golden-plan workflow (the #2439 prototype arms it regardless), and
`cmd/tsuku` is not a test-free package. Neither changes the ranking, because neither favours a
different option.

**The single strongest argument against my own verdict.** C sells itself on making the failure
unrepresentable, but the change that gets there has the widest blast radius of the three and its
worst failure mode is silent deletion of a live file. The forced-change list is
`StaleCleanupActions`, `isCacheStale`, `GarbageCollectVersions`, `plan_install.go`'s missing
`RecordCleanup`, the `HasShellIntegration` replacement, `warnShellInitChanges`'s re-derivation,
and — missed by the advocate — the two places that derive a display name from the filename. That
is seven consumers plus the #2439 prototype in one PR, and the advocate's own confidence note
("I would not have found the GC leak if I had not gone looking") applies to the whole list. If a
reviewer's overriding priority is a narrow, reviewable diff whose worst-case failure is a warning
rather than a deletion, Option A's driver is the more contained bet even though it is more code
and covers less: it warns and leaves files alone, where C deletes. My verdict says correctness
coverage beats blast radius here, and that judgement is contestable.
