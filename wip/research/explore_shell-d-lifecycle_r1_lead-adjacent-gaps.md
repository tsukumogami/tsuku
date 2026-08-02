# Lead: Which of four known adjacent gaps must be fixed for the shell.d re-render acceptance criteria to hold, and which are separable into their own issues?

All line numbers are against the current branch base, which is `origin/main` at
`4d470df9` (`docs/shell-d-lifecycle` adds only a docs commit on top).

Verdict summary:

| Gap | Description accurate? | Blocks acceptance? | Pre-existing? |
|-----|----------------------|--------------------|---------------|
| 1. `--plan` never records cleanup | Yes | No, with one caveat (see below) | Pre-existing |
| 2. Library installs skip post-install | Yes, and worse than stated | No | Pre-existing |
| 3. `installSingleDependency` ignores phases | Yes, and worse than stated | No | Pre-existing |
| 4. Already-installed short-circuit | Yes, verified from code | No, but it constrains the design | Pre-existing |

None of the four blocks the acceptance criteria as written. That is the headline,
and it is a slightly uncomfortable one, because three of the four are genuine
bugs that a reader of the eventual PR will expect to see addressed. The
recommendation at the end of each section says which to fold in anyway.

## Findings

### Gap 1: `cmd/tsuku/plan_install.go` never calls `mgr.RecordCleanup`

**Confirmed, exactly as described.**

`runPlanBasedInstall` sets `ToolInstallDir` and runs the post-install phase at
`cmd/tsuku/plan_install.go:118-121`, collects the resulting cleanup actions at
line 125, derives the affected shells and rebuilds their caches at lines 127-138
— and then drops `postInstallCleanup` on the floor. The next statement (line
141) moves on to setting `IsExplicit`. There is no `RecordCleanup` call anywhere
in the file; the only call site in the tree is
`cmd/tsuku/install_deps.go:613`.

The claim that it affects `install_shell_init` identically is correct. Nothing
in that block inspects which action produced a cleanup action —
`exec.GetCleanupActions()` returns whatever the post-install phase accumulated,
so `install_shell_init` loses its record through `--plan` today, on `main`,
before any `set_env` change. **Pre-existing.**

Worth stating plainly: `cmd/tsuku/install_deps.go:575-616` and
`cmd/tsuku/plan_install.go:115-146` are the same 25-line post-install block,
copy-pasted, and gap 1 *is* the divergence between the two copies. The re-render
work will want a third caller of that block. Extracting a shared helper is the
actual fix; adding a fourth divergent copy is not.

**Does the acceptance test fail if we leave this alone?** Only if the test is
written to install through `--plan`. Walk the criteria:

- "Two installed versions, remove either one" — normal install path, gap 1 not
  on it.
- "recorded `ContentHash` in `state.json` matches disk" — through `--plan`
  there is no recorded hash at all, so the criterion is vacuously satisfied
  rather than violated.
- "`tsuku doctor` reports no content-hash mismatch" — doctor only compares
  paths that have a recorded hash (`internal/shellenv/doctor.go:99-110`, the
  `if expectedHash, ok := contentHashes[relPath]; ok` guard). No record means no
  comparison means no mismatch. Doctor passes.
- "uninstall removes what it wrote" — this one *is* violated through `--plan`,
  but issue #2439's criterion is about `set_env`'s own uninstall behavior, which
  a normal-path test demonstrates.

The caveat: `plugins/tsuku-recipes/skills/recipe-test/SKILL.md:57-58` prescribes
`tsuku eval <tool> | tsuku install --plan - --sandbox --force` as *the* sandbox
testing recipe, and lines 68-74 do the same for cross-family testing. If whoever
writes the acceptance test follows the documented testing workflow, they land on
the `--plan` path and gap 1 becomes a blocker by accident. So: not a blocker in
principle, a likely blocker in practice.

**Verdict: separable, but fold it in.** It is a two-line addition mirroring
`install_deps.go:612-616`, it sits inside code the re-render work has to touch
anyway, and leaving it means the acceptance test has to avoid the project's own
documented test command.

**Independent issue, if separated:** "`tsuku install --plan` orphans shell.d
files on remove because it never records the post-install cleanup actions that
the normal install path records."

### Gap 2: `cmd/tsuku/install_lib.go` never runs the post-install phase

**Confirmed, and the gap is deeper than "a missing call."**

`installLibrary` calls `exec.ExecutePlan` at `cmd/tsuku/install_lib.go:138` and
never calls `ExecutePhase`, `SetToolInstallDir`, or `GetCleanupActions`. The
grep for those three across `cmd/` returns only `plan_install.go:118-125` and
`install_deps.go:578-586`. So any post-install step in a library recipe is
silently skipped — `install_shell_init` as much as `set_env`. **Pre-existing.**

The part the original description missed: there is nowhere to *put* the record
even if the phase ran. `LibraryVersionState` (`internal/install/state.go:121-125`)
has exactly three fields — `UsedBy`, `Checksums`, `Sonames`. No
`CleanupActions`. `install.LibraryInstallOptions{}` at `install_lib.go:156` is
an empty struct. Making libraries support post-install is therefore a state
schema change plus a removal-path change (`internal/install/remove.go` only walks
`ToolState.Versions`), not a one-line addition. That is a real issue on its own,
not a rider.

**Does the acceptance test fail?** No. The criteria say nothing about libraries,
and no library recipe could exercise it: `nvm.toml` is the only recipe in the
entire registry that uses either action (grep for `action = "set_env"` and
`action = "install_shell_init"` across `**/*.toml` returns
`recipes/n/nvm.toml:32` and `:36`, nothing else), and nvm is a tool.

**Verdict: separable.**

**Was the prototype's "reject `set_env` in library recipes at validation" the
right call?** Mostly yes, with one correction. Rejecting at recipe-validation
time is genuinely better than a runtime failure: the author learns at authoring
time, and the check is cheap
(`internal/recipe/validator.go`, the `step.Action == "set_env" && r.IsLibrary()`
branch in the prototype's `validateSteps`). But it is asymmetric — it bans
`set_env` from library recipes while `install_shell_init` stays permitted in the
same recipes and is silently dropped by the same missing call. That documents a
rule its neighbour violates, which is the kind of thing that reads as arbitrary
six months later. Extend the rejection to cover any action whose declared
default phase is `post-install` (the prototype already has `actions.DefaultPhase`
plumbed through `recipe.ActionValidator` for exactly this shape of check), and
file the underlying "libraries never run post-install" gap as its own issue.
Extending it breaks no existing recipe, since nvm is the only user and it is a
tool.

**Independent issue, if separated:** "Library installs never execute the
post-install phase and `LibraryVersionState` has no field to record cleanup
actions, so shell-integration actions in library recipes are silently dropped."

### Gap 3: `Executor.installSingleDependency` executes every step inline with no phases

**Confirmed. The claimed line range is close but off, and the consequence is
worse than "`ToolInstallDir` is empty."**

The function is `internal/executor/executor.go:774-919` (the claim said
857-911). The relevant parts:

- Line 843: the freshly constructed `actions.ExecutionContext` has
  `ToolInstallDir: ""`, written as an explicit literal.
- Lines 863-871: every step is validated up front, with no phase awareness.
- Lines 874-897: every step is executed inline in plan order. `StepPhase` is
  never consulted; `dep.Steps` has no phase filter applied at all.

The part the description missed: the `execCtx` built at lines 839-860 is a
*local* variable, not `e.ctx`. `Executor.GetCleanupActions`
(`internal/executor/executor.go:353-356`) reads from `e.ctx`. So any
`CleanupAction` a step appends to `execCtx.CleanupActions` is discarded when the
function returns. Empty `ToolInstallDir` is one symptom; the thrown-away cleanup
record is the other, and it is the one that produces orphan files.

Concretely, on `main` today, for a tool with `install_shell_init` reached as a
plan-embedded dependency:

- `source_command` variant: `validateCommandBinary` rejects an empty
  `ToolInstallDir` (`internal/actions/shell_init.go:242-244`), the step returns
  an error, and `executor.go:894-896` aborts the parent tool's whole install.
  Fails loudly, which is the tolerable outcome.
- `source_file` variant: no `ToolInstallDir` check. `tsukuHome` is derived from
  `ctx.ToolsDir` (`shell_init.go:108-109`), which *is* populated, so it writes
  real files into `$TSUKU_HOME/share/shell.d/` and then throws away the cleanup
  record. Silent orphans that no remove will ever clean up.

**Pre-existing**, in both variants.

**Does the acceptance test fail?** No. Nothing depends on nvm, and nvm is the
only recipe using either action.

**Verdict: separable.**

**Was the prototype's "fail loudly" the right call?** Yes, and unlike gap 2 it
cannot be moved to validation time. Whether a recipe gets installed as another
tool's dependency is not a property of the recipe, so there is nothing to
validate at authoring time — a runtime guard is the only place the check can
live. The prototype's error text already anticipates the confusing case ("note
that a recipe installed as another tool's dependency cannot use set_env"). The
one improvement worth making: the failure surfaces during an install the user
did not ask for, naming a recipe they may not recognise, so the message should
name the parent tool. That is polish, not a design question.

**Independent issue, if separated:** "`Executor.installSingleDependency` runs
plan-embedded dependency steps inline with no phase filtering, an empty
`ToolInstallDir`, and a discarded `CleanupActions` slice, so post-install
actions either fail or orphan files."

### Gap 4: the already-installed short-circuit, and `--force`

**Confirmed from the code. The empirical claim holds and I can show why.**

The short-circuit is `cmd/tsuku/install_deps.go:502-516`. It sits *after* plan
generation (line 496) and returns at line 515 having executed nothing:

```go
if mgr.IsVersionInstalled(toolName, planVersion) {
    reporter.Status(fmt.Sprintf("%s@%s is already installed", toolName, planVersion))
    if err := recordInstallRelationship(mgr, toolName, parent, isExplicit); err != nil {
```

`IsVersionInstalled` (`internal/install/manager.go:397-404`) is a pure state
lookup — `_, exists := toolState.Versions[version]`. It does not stat the
install directory, and it certainly does not compare recorded hashes against
disk. So the decision to skip is made on the presence of a map key.

On `--force`: `installForce` is declared at `cmd/tsuku/install.go:25` and
registered at line 373 with the help text "Skip security warnings and proceed
without prompts". Its only reads are `install.go:185`, `:238`, `:244`, and
`:689` — unregistered-source approval and recipe-file overwrite. Grepping
`cmd/tsuku/install_deps.go` for `force` or `Force` returns nothing at all. The
flag never reaches the short-circuit. The claim is correct.

Nor does anything else bypass it. `--fresh` reaches `getOrGeneratePlan` via
`planCfg.Fresh` (`install_deps.go:491`), which only forces fresh plan
*generation* — the short-circuit runs afterwards on the freshly generated plan
and skips execution anyway. The full flag list is `install.go:372-390`; there is
no `--reinstall`. Which is awkward, because `cmd/tsuku/verify.go:553` and
`cmd/tsuku/verify.go:748` both tell the user to "Run 'tsuku install <tool>
--reinstall' to restore original." That flag does not exist. The only actual
recovery is `tsuku remove <tool>@<version>` followed by a fresh install.

**Pre-existing.** The short-circuit predates any `set_env` change.

**Does the acceptance test fail?** No, by the letter. The test installs two
fresh versions, removes one, and inspects the result; no step re-installs an
already-installed version, so the short-circuit is never on the path.

But it constrains the design, and the exploration should know that before
picking a mechanism. If re-render is implemented as "replay the stored plan's
post-install phase from `state.json`", gap 4 is irrelevant — that is a new code
path. If it is implemented as "shell out to the existing install flow to
regenerate the file", gap 4 blocks it outright, because that flow returns at
line 515 before executing anything. Any design that reaches for "just reinstall
it" needs gap 4 fixed first.

It also determines whether existing users get the fix at all. Someone who
already has nvm installed will never pick up a corrected `set_env` without
manually removing and reinstalling — and the message telling them how to do that
points at a flag that does not exist.

**Verdict: separable, but it gates one branch of the design space.** Decide the
re-render mechanism first; if the chosen mechanism replays stored plans, file
gap 4 separately.

**Independent issue, if separated:** "`tsuku install` short-circuits on an
already-installed version with no way to bypass — `--force` only suppresses
security prompts and the `--reinstall` flag that `tsuku verify` recommends does
not exist."

### Plugin skills survey

`CLAUDE.md:151-162` binds `internal/actions/`, `internal/recipe/`, and
`internal/executor/` to `tsuku-recipe-author` / `tsuku-recipe-test`, and
`internal/shellenv/` plus `cmd/tsuku/` to `tsuku-user`. The re-render work
touches all of those, so all three skills are in scope. What each currently
documents:

**`plugins/tsuku-recipes/skills/recipe-author/SKILL.md`**

- Line 81: the action table says `set_env` "Export environment variables via
  env.sh". Wrong the moment `set_env` writes shell.d instead. This is the
  one-line change the prototype already makes.
- Line 83-84: `install_shell_init` and `install_completions` rows. Accurate but
  minimal.
- The skill documents no phase concept anywhere. If the work lands
  `PhaseDeclarer` / a `phase` field authors can set, this is new surface with no
  skill coverage at all.

**`plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md`**

- Lines 397-404, the `set_env` entry: "Creates an env.sh file that tsuku sources
  when activating the tool." Both halves are false today — nothing sources an
  `env.sh`, which is precisely issue #2439. The parameter table also describes
  `vars` as a "map" of "name-value pairs", but the action parses a list of
  `{name, value}` objects (`recipe.EnvVar`). That is a second, separate
  documentation error the prototype's rewrite should catch. This file *is*
  #2439's "the action reference documents actual behavior" criterion.
- Lines 418-427, `install_shell_init`: parameters are accurate. Says nothing
  about `$TSUKU_HOME/share/shell.d/{target}.{shell}` as the output path,
  nothing about alphabetical source ordering, and nothing about the `00-env-`
  prefix reservation the prototype introduces.
- Lines 429-438, `install_completions`: same shape, same omissions.

**`plugins/tsuku-user/skills/tsuku-user/SKILL.md`**

- Line 117: "`tsuku shellenv` prints the PATH exports and sources any
  tool-specific shell init scripts from `$TSUKU_HOME/share/shell.d/`." Accurate,
  and the only user-facing mention of shell.d.
- Line 125: enumerates what doctor checks — "shell init caches are current" is
  the closest it comes to the content-hash check. It does not mention hash
  mismatch as a distinct diagnostic, so a new or changed doctor message is
  uncovered surface.
- Lines 88 and 127: `--rebuild-cache` and `--fix` are documented as repairs.
  Neither re-renders shell.d (see Surprises), so if the work adds a repair path
  this line needs updating.
- **The command tables at lines 74-92 do not mention `activate` or `rollback`
  at all.** Two of the three events the acceptance criteria name are entirely
  undocumented in the user skill. If re-render is wired into them, that is new
  surface with no existing text to correct.

**`plugins/tsuku-recipes/skills/recipe-test/SKILL.md`**

- Lines 44-74: the sandbox and cross-family testing commands all route through
  `tsuku install --plan -`. As noted under gap 1, that is the path that drops
  cleanup records. If gap 1 stays unfixed, this skill actively steers testers
  onto the broken path.
- Line 105: lists `tsuku doctor` as an infrastructure check with no detail.

`plugins/tsuku-recipes/AGENTS.md` has no relevant content (its only "phase" hit
at line 56 is about testing phases).

## Implications

The four gaps are all real and all pre-existing, and none of them stands between
the feature and its acceptance criteria. That means the re-render work can be
scoped tightly: the criteria are about `remove`, `activate`, and `rollback` on
the normal install path, and the normal install path already records cleanup
actions with content hashes correctly. What is missing is a re-render step, not
a repair of the recording machinery.

The one gap I would fold in regardless is gap 1, on the grounds that the two
post-install blocks are duplicated code that has already drifted apart once, the
re-render work adds a third consumer, and the project's own documented sandbox
test command routes through the copy that is broken.

Gap 4 is the one to resolve *before* choosing a mechanism rather than after. It
does not block the criteria, but it forecloses "re-render by reinstalling",
which is otherwise the cheapest-looking design. Decide whether re-render replays
the stored plan or writes files directly, and gap 4's relevance settles itself.

The blast radius today is one recipe. `recipes/n/nvm.toml` is the only recipe
using `set_env` or `install_shell_init`, and exactly one recipe uses
`install_completions`. That is worth knowing for two reasons: the acceptance
test has one realistic subject, and any behavior change to these actions breaks
essentially nothing in the registry.

## Surprises

**Doctor can detect the mismatch but `--fix` makes it worse.**
`RebuildShellCache` takes optional content hashes, and when a file's hash does
not match it prints a warning and *excludes the file from the cache*
(`internal/shellenv/cache.go:120-131`). The install paths call it without hashes
(`install_deps.go:595`, `plan_install.go:134`), and so does the remove path
(`internal/install/remove.go:398-404`) — so after a remove the shell keeps
sourcing the stale content. But `tsuku doctor --fix` *does* pass hashes
(`cmd/tsuku/doctor.go:66`), so running the documented repair on a version-skewed
shell.d drops the tool's exports out of the cache entirely. Stale `NVM_DIR`
becomes no `NVM_DIR`. There is no third state in which the correct value
appears. Whatever re-render design lands has to be the thing that fixes this,
because `--fix` currently cannot.

**The mechanism that creates the bug is deliberate, tested code.**
`Manager.executeCleanupActions` (`internal/install/remove.go:326-356`) explicitly
skips deleting a shell.d path when another version of the same tool also
references it, and prints "Cleanup: skipping %s (still referenced by another
version)". That multi-version safety check is what preserves the file — and
nothing then re-renders it. The bug is not an oversight in the removal path; it
is the missing second half of an intentional decision.

**Only removing the *active* version is broken.** Removing the inactive version
leaves the active version's content on disk, which is already correct, and the
recorded hash for the active version already matches. The acceptance criterion
says "removing either one", but only one direction currently fails. Unless — and
this is a third failure mode worth testing — the surviving version was installed
before shell.d recording existed and has no `CleanupActions`. Then `otherPaths`
is empty, the skip does not trigger, the file is deleted outright, and the
active version ends up with no shell.d at all.

**`install_completions` has the identical defect and no detector.** It writes
`share/completions/<shell>/<target>` (`internal/actions/completions.go:105-113`)
with a recorded `ContentHash` — same version-independent filename, same
version-specific content. `cmd/tsuku/doctor.go:207-213` collects those hashes
into the same map, but `CheckShellD` only ever scans `share/shell.d`
(`internal/shellenv/doctor.go:58`), so completions staleness is recorded and
never checked. One recipe is affected. The design should say explicitly whether
completions are in or out of scope rather than leave it implied.

**`tsuku install --reinstall` is advertised but does not exist.**
`cmd/tsuku/verify.go:553` and `:748` both recommend it as the recovery for a
modified install. It is not in the flag list at `cmd/tsuku/install.go:372-390`.

## Open Questions

- Does the re-render mechanism replay the stored plan's post-install phase, or
  re-write files from recorded state? Gap 4's relevance and gap 1's urgency both
  hinge on this, and I was asked not to propose a design — but the other leads'
  findings should converge on it before scoping.
- Are `install_completions` files in scope? They have the same defect and the
  same recorded hashes but no doctor coverage. Excluding them is defensible;
  excluding them silently is not.
- What happens on `rollback` specifically? I confirmed neither
  `cmd/tsuku/activate.go` nor `cmd/tsuku/cmd_rollback.go` nor
  `internal/install/manager.go` references `shellenv` or shell.d at all, but I
  did not trace whether rollback routes through `Manager.Activate` or does its
  own symlink work. That affects where a single re-render hook could sit.
- Does the pre-shell.d-recording upgrade path matter — a version installed by an
  older tsuku with no `CleanupActions`, where the multi-version skip does not
  trigger and the file is deleted outright? Worth an explicit decision.

## Summary

All four gaps are real and all four are pre-existing on `main`, but none of them
blocks the stated acceptance criteria: the normal install path already records
cleanup actions with content hashes correctly, and what is missing is a
re-render step rather than a repair to the recording machinery. The practical
implication is that the work can be scoped tightly to `remove` / `activate` /
`rollback`, with only gap 1 worth folding in — because the two post-install
blocks are duplicated code that has already drifted, and the project's own
documented sandbox test command routes through the broken copy. The biggest open
question is whether re-render replays the stored plan or writes from recorded
state, because that single choice decides whether gap 4's unbypassable
already-installed short-circuit is irrelevant or a hard prerequisite.
