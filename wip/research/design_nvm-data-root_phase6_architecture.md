# Architecture Review: nvm-data-root

Reviewed against the tree, not the prose. Every load-bearing claim in the design that
this review depends on was re-derived from source; where the design is right I say so
and stop, where it is right for a weaker reason than it gives I say that too.

## Verdict

**Approve with changes.**

The four-clock framing is not rhetoric — I checked all four reachability paths and they
are genuinely distinct code paths with distinct triggers, and no single mechanism covers
them. This is a coherent system, not an accretion. Two structural pieces are wrong:
`migrate_data_dir` advertises generality it does not have, and the design has an
unaccounted package edge (`internal/actions` → `internal/notices`) with real plumbing
behind it that appears nowhere in Components or Implementation Approach.

## Must-change

### M1. The notice writer is unspecified, and it forces a new package edge

Components lists only "`internal/notices` — a `KindDataMigration` constant and one render
case," and Implementation Approach step 7 is "Notice kind and render case." Neither says
who *writes* the notice. That is not a detail; it determines a dependency edge.

Today `internal/notices` is imported by `internal/progress/inbox_reporter.go:112`,
`internal/updates/checker.go:149`, `internal/notices/subscriber.go`, and `cmd/tsuku`.
Nothing under `internal/actions` or `internal/executor` imports it — I checked. The design
explicitly rejects routing the notice through `reporter.Warn` (correctly: `InboxReporter.Stop`
at `internal/progress/inbox_reporter.go:91-118` collapses all accumulated messages into one
`WriteNotice` keyed on `r.toolName`). So the write has to come from `migrate_data_dir`
itself, which means:

- a new `NoticesDir` field on `ExecutionContext` (`internal/actions/action.go:15-52` has no
  notices directory today — it carries `ToolsDir`, `LibsDir`, `AppsDir`, `CurrentDir`,
  `DownloadCacheDir`, `KeyCacheDir`, and no notices path);
- plumbing it through every `ExecutionContext` construction site — `internal/executor`,
  `internal/sandbox`, `internal/validate`, `internal/builders`, `cmd/tsuku`;
- a new `internal/actions` → `internal/notices` import.

The edge is acyclic (`internal/notices` imports only stdlib plus `internal/installevents`,
per `internal/notices/subscriber.go:7`), so this is cost, not a violation. But it is
materially more than "nine lines," and it widens `ExecutionContext` — the most-constructed
struct in the tree — to serve one action. Name it, or move the write to a layer that
already holds a notices dir (the post-install caller in `internal/executor` or
`cmd/tsuku`, which can read the action's returned outcome).

Secondary, cheap: `WriteNotice` (`internal/notices/notices.go:91`) writes `<Tool>.json`,
and `validateNoticeName` (`:211-235`) reserves `--` for the `lib--` library sentinel.
`nvm-data-migration` passes validation, but it occupies the *tool-name* namespace, and
renderers print `n.Tool` as a tool name. The repo already has a sentinel convention for
exactly this problem. Either use one or state in the doc that you are deliberately
squatting a tool name.

### M2. `DataDir` must follow `ShareDir`'s guard in `EnsureDirectories`

The design says "one entry in the `EnsureDirectories` slice" and "none of the test
`Config{}` literals need touching." Both are true only *together with the guard*, which
the design does not mention.

`EnsureDirectories` (`internal/config/config.go:372-403`) puts thirteen fields in an
unguarded `dirs` slice, then appends `ShareDir` behind `if c.ShareDir != ""` at `:390-394`,
with the comment "optional for backward compatibility with code that constructs Config
directly without setting it." Tests do exactly that — `cmd/tsuku/doctor_test.go:48` and
`cmd/tsuku/post_install_test.go:26` build partial `Config{}` literals. An unguarded
`DataDir` entry makes `filepath.Join("", "data")` → `"data"`, and `os.MkdirAll` creates a
stray relative directory in the test process's working directory. Add the guard alongside
`ShareDir`'s and say so.

### M3. `migrate_data_dir` is nvm-specific machinery wearing a general name

`install_program_files` is fine — see "structurally sound" below. `migrate_data_dir` is not,
and the problem is the name plus the registry surface, not the timing.

The timing argument is correct and I grant it fully. `set_env.go:165-169` expands to an
absolute literal and `shellQuote`s it into a version-keyed fragment; nothing in the tree
rewrites a fragment's body outside an nvm install (`Activate` at
`internal/install/manager.go:417-481` calls `rebuildShellCachesForTool` at `:479` and
nothing else). So nvm's post-install genuinely is the only moment where the move and the
rewrite are one event. That reasoning holds.

What does not follow is that it must be a *recipe action*. As specified it is:

- registered, so it appears in `RegisteredNames()` (`internal/recipe/validator.go:489`
  builds the typo-suggestion map from it) — a recipe author will find `migrate_data_dir`,
  use it, and silently get nvm's predicate table applied to their tree;
- backed by a detection table (Populations A/B/C) that hardcodes `versions`, `alias`,
  `.cache`, `default-packages`, `current` — nvm's on-disk layout, with no parameter to
  vary it;
- owed an `ActionEvaluability` entry (`internal/executor/plan.go:174-213`), a validator
  concern, and a `tsuku-recipe-author` plugin-skill entry — the full cost of a public
  recipe-schema surface;
- documented as returning `nil` on every runtime failure. An action that structurally
  cannot fail is not an action.

All of that for a step the design plans to delete once the affected releases age out.

Minimum fix: rename to something that does not promise generality (`migrate_nvm_data_dir`),
and state in the doc that it is not a general facility. Better fix in W1.

### M4. The validator's `{data_dir}` allow-list will drift

Implementation Approach step 2 puts a rule in `internal/recipe/validator.go` rejecting
`{data_dir}` in any action that does not expand it, and step 9 correctly notes
`internal/recipe` cannot import `internal/actions`. That means the list of expanding
actions becomes a literal inside `validator.go`, disconnected from the actions that
actually expand it. Add a fourth expander later and the rule rejects a valid recipe; the
failure is a hard error at `--strict`, so it turns the Validate Recipes job red for a
correct recipe.

The repo already solved this shape. `{deps.*}` is likewise expanded by only some actions
(`GetStandardVarsWithDeps`, `internal/actions/util.go:44-54`, used by `set_rpath.go:43` and
`configure_make.go:105`), and the guard is `CheckUnexpandedDepVars` (`util.go:56-69`) — a
*runtime* check inside the action, where the knowledge lives, not a list in the validator.

Mirror that: make the authority a `CheckUnexpandedDataDir(s, context)` called from the
generic expansion path, so an unexpanded `{data_dir}` errors from the action that saw it
regardless of which actions expand it. Keep the validator rule as the cheap early check,
but drive it through the existing `GetActionValidator()` indirection rather than a literal,
or accept the literal and write down the drift risk. As written the doc has neither.

## Worth considering

### W1. Collapse `migrate_data_dir` into the `set_env` path

The strongest version of M3. Have `set_env` call `nvmdata.MigrateIfNeeded(...)` immediately
after writing the fragment, gated on the same known-data-root-variable denylist the
Decision 4 validator rule already defines. This is strictly *tighter* than a second recipe
step: the design currently has to state "ordered after `set_env`" as a constraint a future
recipe edit can violate, and with the call inline the ordering is not expressible wrongly.
It also drops one registry action, one evaluability entry, one validator concern, one
plugin-skill entry, and the `ExecutePhase`-abort hazard that forced the "return `nil` on
every failure" contract in the first place.

The objection is that it puts a named special case inside a general action. The repo
already does this on the general install path: `fixPipxShebangs` is called unconditionally
from `Manager.installDirectoryMode` at `internal/install/manager.go:177`, with `// Ignore
errors - not all tools use pipx`. That is precisely this shape — an ecosystem-specific
fixup, named honestly, on a general path, costing zero recipe schema. It is a better
precedent to follow than inventing a general-sounding action.

I would take this. It is not a must-change because the declared-step version does work;
it is a scope *reduction* that makes the single-PR constraint easier, not harder.

### W2. `internal/nvmdata` splits on the wrong axis

Verified: `internal/` holds 44 packages and none is named for a tool. Ecosystem-specific
code exists in quantity but always as *files inside general packages* —
`internal/actions/brew_actions.go`, `apt_actions.go`, `gem_install.go`, `cargo_build.go`,
`nix_portable.go`. A tool-named package would be the first of its kind.

The design's justification is real: `internal/install` needs the merge-move for the removal
rescue, and `internal/actions` and `internal/install` do not import each other in either
direction (I checked both). A leaf genuinely avoids creating that edge.

But the thing `internal/install` needs is `mergeMove` — a tool-agnostic directory-merge
primitive with no nvm in it. The nvm-specific part is the A/B/C predicate table. Splitting
the package by *tool* rather than by *generality* puts a general primitive behind a
tool-named import in `internal/install`, which reads as `install` depending on nvm.

Consider: the merge-move in a genuinely general leaf, the nvm predicates in
`internal/actions/nvm_data.go` next to their only consumer. Same single-cut disposability,
matching the repo's actual file-level convention, no tool-named package precedent, and
`internal/install` ends up importing something it can keep after the migration is deleted.

The disposability argument does carry; it just does not require a *tool-named package* to
carry it. A file is also one deletion unit.

### W3. The doctor check would be the first tool-specific check

`runDoctorChecks` (`cmd/tsuku/doctor.go:99-289`) is a flat procedural function with eight
checks, every one generic: home directory, `tools/current` in PATH, `bin` in PATH, state
file, env file, shell caches, orphaned staging, stale notices. An nvm check is a new kind
of thing there.

The four-verdict table itself is fine — WARN already exists as a third level at `:256` and
`:284`, so the design is not inventing vocabulary. Two asks: gate the check on nvm being
installed so it emits nothing for the overwhelming majority of users, and keep it one
contiguous hunk so the eventual deletion is a single cut, consistent with the
disposability story told everywhere else.

### W4. Two independent definitions of the data-root path

The helper does `filepath.Join(filepath.Dir(ctx.ToolsDir), "data", <name>)` while
`config.DataDir` does `filepath.Join(tsukuHome, "data")`. These can drift.

This is not a new sin — it is exactly what `share/` already does (`config.ShareDir` at
`config.go:367` versus `filepath.Dir(ctx.ToolsDir)` + `"share"` at `set_env.go:150-151`,
`shell_init.go:144`, `completions.go:79`), and `ShareDir` has precisely one real consumer
(`cmd/tsuku/cmd_hook.go:71`). So the design is following precedent, and the precedent is
mildly bad. Worth a shared constant for the `"data"` segment at minimum; not worth
restructuring `share/` in this PR.

## What is structurally sound

**`$TSUKU_HOME/data/` is the right addition, and the framing undersells it.** This is not a
"fourth top-level tree." `Config` (`internal/config/config.go:319-335`) already names
`tools`, `recipes`, `registry`, `libs`, `apps`, `cache` (with four subdirectories), and
`share`, plus `bin`, `notices`, `env`, `state.json`, and `config.toml` on disk. Top-level
trees are cheap here by established practice, and the design's three-line cost estimate is
accurate: struct field, `DefaultConfig` entry, `EnsureDirectories` entry.

**The `share/` rejection is a real distinction, cited from the wrong evidence.** The design
argues from policy ("share/ holds only regenerable state"). That description is factually
correct — `shell.d`, `hooks`, `completions` are all tsuku-generated — but a policy argument
invites the reply "then change the policy." The dispositive facts are mechanical and
stronger: `internal/shellenv/doctor.go:85-92` enumerates `share/shell.d` and reports every
symlink it finds as a finding, and `internal/shellenv/cache.go:104,162,173` actively writes
and removes inside that tree. Putting a directory that can be the largest thing on the
machine under a tree that a health check enumerates and a cache builder rewrites is a
category error independent of any stated policy. Cite the enumerator.

I also confirmed the negative claim the rejection depends on: nothing prunes `share/`
wholesale today, so the design is right that `share/` is *safe* and wrong-tree for a
different reason than safety.

**`{data_dir}` as a locally-expanded placeholder does not create a new class.** It creates
a *second instance of an existing one*. `GetStandardVarsWithDeps` (`util.go:44-54`) already
gives `set_rpath` and `configure_make` a strict superset of the placeholder set every other
action sees. Recipe authors already live with a non-uniform placeholder vocabulary. The
design's instinct not to widen `GetStandardVars` — which takes four positional string
arguments and has nine non-test call sites — is right: widening it changes nine signatures
to serve one recipe, and the `{deps.*}` precedent shows the repo's answer to that is a
second constructor, not a wider first one. The validator rule does not prove the split is
wrong; the split is already the house style. (The rule's own siting is M4.)

**`install_program_files` is genuinely general.** It is the missing generalization of what
`install_shell_init` already does: copy named files out of the install directory into a
stable tsuku-owned location and record cleanups (`internal/actions/shell_init.go:110-170`).
Its parameters (`files`, `dir`) carry nothing nvm-specific, it correctly implements
`PhaseDeclarer` (`internal/actions/action.go:131-152`) for the same reason `set_env` does
(`ToolInstallDir` is empty before the atomic rename), and copies-over-symlinks is forced by
verified code rather than preference: `Activate` (`manager.go:417-481`) repoints binaries,
updates state, and calls `rebuildShellCachesForTool` at `:479` — the post-install phase is
nowhere in it. A recipe-placed symlink tracks the last-installed version. That is correct
and well argued.

**The security reasoning on `dir` validation is right and correctly sited.**
`executeSingleCleanup` (`internal/install/remove.go:417-434`) does
`filepath.Join(m.config.HomeDir, ca.Path)` and dispatches to `os.Remove` / `os.RemoveAll`
with no traversal check whatsoever. Any action that records a cleanup for a recipe-named
path is one bad `dir` away from delete-anything. Validating in the action rather than the
recipe is the right call, and the design is correct to call it the highest-severity item.

**The refusal to record a `delete_dir` cleanup for the data root is the single best call in
the document.** `ReapVersion` (`remove.go:384-413`) runs `executeCleanupActions` from
background GC, and `GarbageCollectVersions` (`internal/updates/gc.go:25-45`) reaches it
from the unattended auto-apply loop. A recorded `delete_dir` on the data root would make
this design's own bug fire from a background process. The design saw that and said no.

**The four call sites are four genuinely different reachability paths.** I verified each
independently:
- the version-keyed fragment and `ShellDSelection` — immediate, no GC involved;
- `os.RemoveAll(toolDir)` at `remove.go:50`, `:151`, `:258`, none of which involves an
  install path, which is why cross-validation C1 is right that Decision 1's guarantee was
  vacuous without the rescue;
- `manager.go:182`'s pre-rename `RemoveAll`, correctly downgraded to plan-install-only;
- `Activate`'s post-install gap.

No one mechanism covers all four, and the design does not pretend one does. That is the
answer to "is this an accretion of patches": it is not. The reason there are four pieces is
that there are four paths, each demonstrated from source.

**Layering claims check out.** `internal/actions` and `internal/install` import each other
in neither direction. `internal/actions` → `internal/config` already exists
(`internal/actions/download.go:13`), so a leaf importing `config` adds no new edge kind. The
only unaccounted edge is the notices one (M1).

**The `mergeMove` contract is right.** No `os.RemoveAll`, no overwrite, atomic per entry at
every depth, `os.Lstat` for the existence check so a planted destination symlink cannot
redirect a rename, shape-detected re-runnability with no state flag. The prohibition on a
`copyDir` fallback stated as a doc-comment invariant rather than an omission is the correct
way to prevent a future contributor from "fixing" it. This is the part of the design I would
change least.
