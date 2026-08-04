---
status: Current
problem: |
  Files under $TSUKU_HOME/share/shell.d have version-independent names but
  version-specific contents, and nothing ever re-renders one. Installing a second
  version overwrites the first version's file, so removing either version, switching
  the active version with activate, or rolling back all leave shell.d holding some
  other version's content with a ContentHash in state.json that no longer matches
  disk. For nvm this points NVM_DIR at a directory that no longer exists; tsuku
  doctor detects the mismatch and doctor --fix cannot repair it.
decision: |
  Version-key the shell.d filename as <target>@<version>.<shell>, so installing a
  second version no longer overwrites the first, and teach the cache builder to
  exclude a file only when state.json records that path for an installed version that
  is not the active one. A file no version records is included exactly as today. The
  three sites that assign ToolState.ActiveVersion each rebuild the affected caches
  after their state write, so activate, rollback, and multi-version removal all select
  the correct version's file. set_env is routed through the same share/shell.d
  delivery so its exports reach the user's shell, resolving issue #2439.
rationale: |
  Re-rendering keeps the invariant "this file's content must agree with the active
  version" and adds machinery that stays correct only while every future lifecycle
  event remembers to call it. Version-keying removes the invariant: content is fixed
  when written and is only ever reachable from the version that wrote it, so
  ContentHash becomes permanently accurate rather than stale the moment a second
  version lands. It is also the only candidate that handles install_shell_init's
  source_command mode, versions with no stored plan, and --no-shell-init uniformly,
  because it never re-derives bytes. Excluding only recorded-but-inactive files rather
  than gating on the manifest keeps unrecorded writers behaving as they do today.
---

# Design: a correct lifecycle for share/shell.d

## Status

Current

## Context and Problem Statement

`$TSUKU_HOME/share/shell.d/` holds shell fragments that `RebuildShellCache`
concatenates alphabetically into `.init-cache.{bash,zsh}`, which `$TSUKU_HOME/env`
sources. Two actions write into it: `install_shell_init` today, and `set_env` once
issue #2439 is fixed by routing its exports through the same delivery.

The filenames are a pure function of `(the recipe's literal target, shell)`. The
contents are whatever that version produced — for `install_shell_init`'s `source_file`
mode, a byte copy of a file from the tool's own directory; for its `source_command`
mode, the stdout of a binary in that directory; for `set_env`, exports that name
`{install_dir}`, which is versioned by definition.

Nothing re-renders these files. They are written at install time and are otherwise only
deleted. The consequence, reproduced end to end:

```
tsuku install nvm@0.40.5     -> 00-env-nvm.bash: NVM_DIR=.../tools/nvm-0.40.5
tsuku install nvm@0.40.6     -> 00-env-nvm.bash: NVM_DIR=.../tools/nvm-0.40.6
tsuku remove  nvm@0.40.6 --force
   "Cleanup: skipping share/shell.d/00-env-nvm.bash (still referenced by another version)"
active version: 0.40.5
00-env-nvm.bash: NVM_DIR=.../tools/nvm-0.40.6   <- directory no longer exists
```

The skip is deliberate: `Manager.executeCleanupActions` builds an `otherPaths` set and
refuses to delete a path another installed version still references, because deleting it
would break the survivor. It is correct about existence and silent about content.

This is not specific to `set_env`. The same run leaves `nvm.bash` — written by the
pre-existing `install_shell_init` — holding 0.40.6's entire `nvm.sh` after 0.40.6 is
removed, and that half reproduces on `main` today with no changes at all.

Three further facts, each established by reading the code:

- **Exactly three non-test sites assign `ToolState.ActiveVersion`**:
  `Manager.InstallWithOptions`, `Manager.Activate`, and `Manager.RemoveVersion`'s
  implicit promotion. `createSymlinksForBinaries` is already called from all three, so
  the binary layer solved this problem one artifact over. `Rollback` is not a fourth
  site — it delegates to `Activate`, as does auto-apply's failure rollback in
  `internal/updates`. shell.d is missing the binary layer's equivalent.
- **The skipped cleanup also skips the cache rebuild.** `affectedShells[shell] = true`
  sits after the `continue`, so the reproduction leaves `.init-cache.bash` holding the
  removed version's content too. The cache is not merely stale; it is never touched.
  `RemoveVersion` additionally rebuilds caches *before* its promotion, so even a correct
  set would rebuild against the pre-promotion world.
- **`doctor --fix` cannot converge.** It repairs only `CacheStale`, never
  `HashMismatches`. When it does rebuild, `RebuildShellCache` excludes the mismatched
  file and deletes the cache outright if that empties it — turning a wrong `NVM_DIR`
  into no `NVM_DIR`. The re-check then reports stale again, because `isCacheStale`
  recomputes the expected concatenation without hash filtering. Verified empirically.

Only removing the *active* version is broken; removing an inactive version already
leaves correct content. There is a third failure mode the reproduction does not show: if
the surviving version predates cleanup recording and has no `CleanupActions`,
`otherPaths` is empty, the skip does not fire, and the file is deleted outright, leaving
the active version with no shell integration at all.

## Decision Drivers

- **Correctness across every event that changes the active version**, not just the two
  named in the bug report. The trigger set must be provable, not argued.
- **Both writers.** Fixing only `set_env` would leave `NVM_DIR` correct while the
  sourced script came from a different version — a more confusing state than today's.
- **No silent breakage of a working shell integration.** Three code paths write shell.d
  without recording a cleanup action, and a design that stops sourcing unrecorded files
  would remove a user's tool from their shell with no error and no log line.
- **`ContentHash` should mean something.** Today the hash recorded for v1 describes a
  file v2's install already overwrote. The record is stale the moment a second version
  lands, which is why `doctor` reports a mismatch that nothing can fix.
- **Offline and side-effect-free.** `remove`, `activate`, and `rollback` are offline
  operations today. A mechanism that executes a tool binary or fetches a recipe during
  a version switch changes that.
- **One reviewable PR.** The worst failure mode of a change in this area is deleting a
  file the user depends on, so blast radius and test coverage are first-order concerns.

## Considered Options

Four mechanisms were built out in full — each championed by an agent that then attacked
its own design — and cross-examined on two axes, acceptance-criteria conformance and
landing risk. The summary:

### Option A — re-render on lifecycle events

Give the writing actions a render capability that reproduces their bytes for a given
`(tool, version)` without running an install, and drive it from `internal/install` at
the three `ActiveVersion` sites. Inputs come from `VersionState.Plan.Steps[].Params`,
selected by action name because `ToStoragePlan` silently drops the `Phase` field.

Genuinely strong: the trigger set is a `grep` result rather than a judgement call, it
needs no `state.json` schema change, and gating writes on paths the newly-active version
already recorded closes the `--no-shell-init` hole for free.

Rejected on coverage. `source_command` mode can only be reproduced by executing a binary
from the tool directory, which is not something `tsuku remove` should do, so A refuses
it — and `recipes/README.md` documents `source_command` as *the* pattern for tools with
a `tool init bash` subcommand. A also cannot help a version whose stored plan is nil, and
its `ContentHash` agreement is achieved by rewriting the record rather than reproducing
the bytes. It additionally requires severing the `actions -> version -> install` import
cycle by moving `internal/install/pin.go` to a leaf package: eight files of mechanical
collateral, correct in itself, but noise in this diff.

### Option B — store the rendered bytes per version in state

`CleanupAction` already lives on `VersionState`, so it is already per-version — the shape
activate and rollback want. Store the content alongside the hash and write it back when
that version becomes active.

Measured out of contention. `nvm.sh` is 161,810 bytes; gzipped and base64'd it is 46,236
bytes per version per shell, and `state.json` is parsed in full by effectively every
command — a measured 240 µs to 5.0 ms on a clean install. A content-addressed sidecar
avoids the hot-path cost, but then B is storing a third copy of bytes that already exist
twice: `InstallWithOptions` copies `workDir/.install` wholesale into the versioned tool
directory, so the `source_file` content is already on disk for every installed version.
B also cannot improve any install that predates it.

### Option D — containment only

Thread the active version's content hashes through the four `RebuildShellCache` call
sites that pass none today, so a mismatched file is excluded rather than sourced, and add
a `doctor --fix` repair.

Rejected as a primary mechanism: it converts "wrong exports, silently" into "no exports",
which for a tool that exists only as a shell function is not clearly better, and it
leaves `activate` still doing nothing. Its useful half is subsumed by the chosen option,
which passes the same information for a different reason.

### Option C — version-key the filename (chosen)

Stop trying to keep one file in agreement with whichever version is active; give each
version its own file.

## Decision Outcome

**Version-key the shell.d filename, and exclude from the cache only those files that
`state.json` records for an installed version that is not the active one.**

Writers produce `share/shell.d/<target>@<version>.<shell>`, and `set_env` produces
`share/shell.d/00-env-<name>@<version>.<shell>`. Installing v2 no longer touches v1's
file, so v1's recorded `ContentHash` describes a file only v1 ever wrote and stays
accurate permanently. Removing a version deletes exactly its own files, because the
`otherPaths` skip cannot fire on paths no two versions share. Activate and rollback
need no content derivation at all: the target version's file was never overwritten, so
switching is a cache rebuild.

The whole class of bug becomes unrepresentable rather than corrected. That is the
argument for it: A, B, and D each keep the invariant and add machinery that must be
invoked correctly at every future lifecycle event; C deletes the invariant, so there is
no event that can be forgotten.

**The exclusion rule is deliberately narrower than "source only what state records."**
Championed as a strict manifest gate, C would have silently broken three writers that
produce shell.d files without recording a cleanup action: `cmd/tsuku/plan_install.go`,
`Executor.installSingleDependency` (which builds a throwaway execution context whose
cleanup actions are discarded), and `cmd/tsuku/install_lib.go` (which never collects
them). Excluding only *recorded-but-inactive* files leaves every unrecorded file behaving
exactly as it does on `main`, keeps the parameter optional so that passing nothing means
excluding nothing, and still fixes the reproduction — because after the rename, v1's and
v2's files are distinct paths and only the active one is in the active set.

This design does not deliver a re-render. The brief anticipated one while explicitly
leaving the shape open; the conclusion is that re-rendering is the wrong primitive and
the better move is to make it unnecessary. Lifecycle hooks still go in at all three
`ActiveVersion` sites, but what they do is recompute a concatenation rather than
reproduce a past version's bytes.

**Repair is out of scope and is structurally impossible under this design.** If a user
deletes or edits a shell.d file, nothing regenerates it. The acceptance criteria scope
`doctor` cleanliness to "after any of the above" — operations tsuku itself performs — and
no criterion mentions `--fix`. Should repair be wanted later, the cheap path is not a
render capability: for `source_file` mode the bytes are byte-identical to
`tools/<tool>-<version>/<source_file>`, so a repair is a `copyFile` that needs the stored
plan only to learn the parameter name.

## Solution Architecture

### Naming

```
$TSUKU_HOME/share/shell.d/<target>@<version>.<shell>
$TSUKU_HOME/share/shell.d/00-env-<name>@<version>.<shell>
```

`@` is the separator because it is already tsuku's user-facing tool-version separator
(`tsuku install nvm@0.40.6`), so a directory listing reads itself. `-` is unavailable:
`config.ToolDir` names versioned directories `name + "-" + version`, and that mapping is
not injective — a tool whose name contains a hyphen collides with a shorter-named tool at
a hyphenated version. Nothing can read the layout back by prefix and recover the pair,
and every place that has tried has produced a real bug: reclamation deleting a
prefix-sharing tool's entire directory (#2474), an eval dependency on `git` reported
satisfied when only `git-lfs` is installed (#2480), and `libcurl` resolving to
`libcurl-source`'s version (#2481). Copying that convention into the one place we are
making a bug unrepresentable would import a latent one.

`install_shell_init`'s preflight gains `target` must match `^[A-Za-z_][A-Za-z0-9._-]*$`.
This runs in recipe-validation CI, subsumes the prototype's narrower
"target may not claim the `00-env-` prefix" rejection, and is satisfied by
`recipes/n/nvm.toml`'s `target = "nvm"` — the only recipe using the action.

**Injectivity.** `(target, version, shell) -> filename` is injective. Suppose
`t₁@v₁.s₁ == t₂@v₂.s₂`. No target contains `@`, so the first `@` is the separator in both
decompositions and `t₁ == t₂`. Of the legal shells `{bash, zsh, fish}` none is a suffix
of another, so `s₁ == s₂`. Trimming both leaves `v₁ == v₂`. The awkward case resolves:
tool `foo-1` at version `2` gives `foo-1@2.bash`; tool `foo` at version `1-2` gives
`foo@1-2.bash`.

**Sort order is strengthened, not weakened.** The exports-before-init contract that the
`00-env-` prefix exists to guarantee currently rests on "no plausible init filename sorts
before `00-env-`". Under the target charset rule, init filenames begin with `[A-Za-z_]`
(≥ 0x41) and export filenames begin with `0` (0x30), so the ordering holds for every
legal target with no case analysis.

### The active-set projection

`internal/install` already imports `internal/shellenv`, so the type lives in `shellenv`
and the constructor in `install` — the direction that already compiles. No import surgery,
no new capability interface, no reading of the stored plan.

```go
// package shellenv

// ShellDSelection tells the cache builder which recorded shell.d files belong to a
// tool's active version. A file that appears in Known but not Active belongs to an
// installed-but-inactive version and is excluded from the cache. A file in neither is
// unrecorded and is included unchanged, which is the behaviour on every release to date.
type ShellDSelection struct {
    Active map[string]string // $TSUKU_HOME-relative path -> recorded SHA-256
    Known  map[string]string // every recorded shell.d path, active or not
}
```

```go
// package install

// BuildShellDSelection projects every installed tool's recorded cleanup actions onto
// share/shell.d. Pure; no filesystem access.
func BuildShellDSelection(state *State) shellenv.ShellDSelection
```

The builder is the loop `cmd/tsuku/doctor.go` already runs to collect active-version
hashes, extended to walk non-active versions into `Known`. A `Manager` wrapper serves
in-package callers so the two hooks cannot get it wrong.

`RebuildShellCache` and `CheckShellD` keep their variadic-optional shape, so passing
nothing means excluding nothing and every existing caller and test keeps its current
semantics. The selection loop gains one condition: skip an entry that is in `Known` and
not in `Active`. Hash verification, symlink rejection, the brace-group wrapper, and
atomic replacement are unchanged.

### Lifecycle hooks

All three sites rebuild **after** the state write that sets the new active version. The
manifest is derived from state, so a rebuild that runs first sees the old world.

| Site | Covers | Change |
|---|---|---|
| `Manager.InstallWithOptions` | install, upgrade, `tsuku run` autoinstall, auto-apply | already rebuilds; now passes a selection |
| `Manager.Activate` | `tsuku activate`, `tsuku rollback`, auto-apply's failure rollback | new call after `UpdateTool` |
| `Manager.RemoveVersion` promotion | removing one version among several | new call; the existing rebuild moves below the promotion |

Placing the hook in `Manager.Activate` rather than in `cmd/tsuku` is what covers
auto-apply's rollback, which calls `Manager.Activate` directly from `internal/updates`
with stdout and stderr redirected to `/dev/null`.

`executeCleanupActions` also gets the two-line fix that populates `affectedShells` on the
skip branch, so a path retained for another version still triggers a rebuild.

### Forced consumer changes

| Consumer | Change | Pre-existing bug? |
|---|---|---|
| `StaleCleanupActions` / `ExecuteStaleCleanup` | Needs the `otherPaths` guard `remove.go` already has. Without it, upgrading deletes the rollback target's shell.d files, and the failure is silent because `CheckShellD` iterates directory entries so a *missing* file produces no mismatch. | **No — this rename creates it.** Highest-risk item in the change. |
| `isCacheStale` | Must apply the identical exclusion or `doctor` fails permanently on any multi-version install. | Yes — aligning it also fixes the `--fix` non-convergence described above. |
| `HasShellIntegration` | Probes `{toolName}.bash` by name. Already wrong, since the filename derives from `target`, which need not equal the tool name. Replaced by reading the active version's recorded paths. | Yes |
| `GarbageCollectVersions` | Must run the deleted version's `delete_file` cleanups and drop its `VersionState`, or a GC'd version's files leak (~324 KB per GC'd nvm version). | Yes — also closes the hole where a GC'd version holds the `otherPaths` skip open for a directory that no longer exists. |
| `warnShellInitChanges`, cache comment, `doctor`'s `ActiveScripts` | Compare and display by `(target, shell)` rather than by raw filename, so output reads `nvm` not `nvm@0.40.6`. | Cosmetic |
| `cmd/tsuku/plan_install.go` | Gains the `RecordCleanup` it has never made, via one shared helper for the post-install block duplicated in `install_deps.go`. | Yes |

### set_env (issue #2439)

`set_env` is rewritten to write `share/shell.d/00-env-<name>@<version>.{bash,zsh}`
instead of an `env.sh` nothing reads, and to expand `{install_dir}` from
`ToolInstallDir` rather than the staging directory. Because `ToolInstallDir` is populated
only between `ExecutePlan` and `ExecutePhase("post-install")`, the action declares
`post-install` through an optional `PhaseDeclarer` interface that `StepPhase` consults
via the action registry when a recipe names no phase — resolving at execution time rather
than at plan-generation time, so stored plans keep routing correctly.

Variable names must match `[A-Za-z_][A-Za-z0-9_]*`, values may not contain newlines, and
values are single-quoted with POSIX escaping, checked after placeholder substitution.
`set_env` is rejected in library recipes, and a `phase` override the action cannot honour
is rejected, both at validation time.

## Implementation Approach

Sequenced so each step is independently verifiable.

1. **The #2439 fix.** Route `set_env` through shell.d with validation and quoting; add
   `PhaseDeclarer`; update `recipes/n/nvm.toml` and the action reference. Lands the
   second writer the lifecycle work must cover.
2. **Version-keyed filenames.** Both writers, plus the `target` charset preflight.
3. **The active-set projection.** `ShellDSelection`, `BuildShellDSelection`, the
   exclusion condition in `RebuildShellCache`, and the matching filter in `isCacheStale`.
4. **Lifecycle hooks and the removal-path fixes.** The `Activate` and promotion hooks,
   the rebuild reorder, the `affectedShells` skip fix.
5. **Forced consumer changes.** `StaleCleanupActions` guard, `HasShellIntegration`
   replacement, GC cleanup, display-name derivation, `plan_install.go` `RecordCleanup`.
6. **Tests**, including the end-to-end multi-version subshell test and mutation testing.
7. **Plugin skills**, per the repo's plugin-maintenance table.

### Migration

There is no rename pass, no schema migration, and no first-run hook. The projection is
keyed on recorded paths rather than on a naming convention, so a legacy record
(`share/shell.d/nvm.bash`) and a new one (`share/shell.d/nvm@0.40.7.bash`) are
indistinguishable to every consumer. A legacy file whose version is active is in `Active`
and is sourced exactly as before.

The transition then plays out per tool: installing a new version writes a version-keyed
file and records it; `RecordCleanup` overwrites the active version's actions, so the new
file is in `Active` and the legacy file — recorded by the older, now-inactive version —
falls into `Known`-not-`Active` and stops being sourced. Removing the old version deletes
it, because `otherPaths` no longer contains that path.

Two residues, stated rather than buried. A user who never installs a new version keeps
the legacy layout forever, which works. And a user who *already* has two versions sharing
one filename keeps that corruption until one of them is removed — this design stops new
instances rather than repairing existing ones.

### Testing

The end-to-end test installs two versions, removes one, sources `$TSUKU_HOME/env` in a
hermetic `bash --norc --noprofile` subshell, and asserts the variable points at the
surviving version's directory. Asserting on file contents is explicitly insufficient: the
original bug survived review for exactly that reason, and a rendered-but-uncached file
passes a content assertion while the user's shell still gets the wrong value.

Guards are mutation-tested by hand — the repo has no mutation-testing tooling — and each
applied defect is recorded in the PR body. At minimum: revert the `StaleCleanupActions`
guard and confirm the update-then-rollback test fails; skip the cache rebuild after a
promotion and confirm the subshell test fails; exclude the wrong side of the
`Known`/`Active` split and confirm the multi-version test fails; and fix only one of the
two writers and confirm the other's assertion fails.

Tests live in `cmd/tsuku` (`package main`, 32 existing test files) where `install`,
`actions`, `executor`, and `shellenv` already meet, and in the owning packages for unit
coverage. Nothing may be `testing.Short()`-gated, because CI always passes `-short`, and
tests must write only under `t.TempDir()`, because the unit-test job fails on a dirty
`git status --porcelain`.

## Security Considerations

**Everything in the cache is executed by the user's shell.** `.init-cache.{shell}` is
sourced by `$TSUKU_HOME/env` on every new shell, so any byte that reaches it runs with
the user's privileges. This raises the stakes on three surfaces.

*Filename injectivity is a security property, not just a correctness one.* If two
`(tool, version)` pairs could produce the same filename, one tool's install would
overwrite another's shell fragment. The `@` separator plus the `target` charset
constraint make the mapping provably injective. Version strings reach the filename from
state, and `ValidateVersionString` already rejects `..`, `/`, and `\`, so a version
cannot escape the directory; the injectivity proof additionally rules out a version
string forging a different tool's name or a different shell suffix.

*The permissive default for unrecorded files is a pre-existing weakness this design
deliberately preserves.* Anyone who can write into `share/shell.d/` gets their bytes
sourced into every new shell, because a file tsuku has no record of is included
unverified. The stricter rule — source only what state records — would close that, and
was the championed form of this design. It was rejected because three code paths write
shell.d without recording, so the strict rule would silently disable working
integrations. Closing the gap properly means fixing those three writers first; that is
recorded as a follow-up rather than smuggled in here, and the directory is `0700` with
files `0600`, so the exposure requires an attacker who can already write as the user.

*`set_env` values are attacker-influenced if a recipe is.* Values are validated after
placeholder substitution — no newlines, names constrained to an identifier pattern — and
single-quoted with POSIX escaping, so shell metacharacters round-trip as data rather than
executing. Recipes are reviewed and public, which is the primary control; the quoting is
defence in depth.

*Nothing new is executed during a lifecycle event.* This is the security advantage of the
chosen option over the re-render alternative, which would have spawned a binary from a
tool directory during `tsuku remove` to reproduce `source_command` output. Under this
design `remove`, `activate`, and `rollback` remain offline and side-effect-free with
respect to tool code.

*Permissions are unchanged*: `share/shell.d/` at `0700`, files at `0600`, writes atomic
via temp-plus-rename in the same directory, under the existing file lock.

## Consequences

**Positive.** The bug class becomes unrepresentable rather than corrected — there is no
lifecycle event that can be forgotten, because nothing needs re-deriving. `ContentHash`
becomes a permanent invariant, so a mismatch means exactly one thing (post-install
tampering) and is worth acting on. `source_command`, versions with no stored plan, and
`--no-shell-init` all stop being special cases. Four pre-existing bugs get fixed because
they are load-bearing: `doctor --fix`'s non-convergence, `plan --install`'s missing
`RecordCleanup`, GC's failure to touch state, and `HasShellIntegration`'s wrong filename
assumption. No `state.json` schema change, no import surgery, no hot-path cost.

**Negative.** The blast radius is wide: twelve non-test files across six packages, and
five consumers that must change or the rename regresses. One of those —
`StaleCleanupActions` — is a defect this design creates, whose failure mode is deleting a
live file that no existing test covers. Repair was structurally impossible when this was
written: a user who deleted a shell.d file had no recovery but remove-and-reinstall, made
worse by the already-installed short-circuit that `--force` does not bypass. Issue #2463
closed that gap. `tsuku install <tool> --reinstall` re-executes the plan and rewrites the
fragment, so the repair path now exists. Pre-existing
multi-version corruption is grandfathered. Disk use grows by one fragment per installed
version per shell — for nvm, ~324 KB per extra version, against a tool directory that
already holds a full copy of the same file.

**Mitigations.** The `StaleCleanupActions` guard gets a dedicated update-then-rollback
test and an explicit mutation test, because it is the one change whose omission fails
silently. The exclusion rule is narrowed to recorded-but-inactive files specifically so
that unrecorded writers keep working, which removes the silent-breakage hazard the
strict form carried. The GC leak is bounded to the background auto-apply path and is
fixed in the same change.

**Deliberately out of scope**, each with a reason rather than an omission:

- **`install_completions`** has the identical defect in `share/completions/`. There is no
  completions cache, so there is no builder half and `doctor` never notices. The same
  rename applies verbatim; separate issue.
- **Library recipes never run a post-install phase**, and `LibraryVersionState` has no
  field to record cleanup actions. Making that work is a state-schema change plus a
  removal-path change. `set_env` is rejected in library recipes at validation so the gap
  fails loudly; separate issue.
- **`Executor.installSingleDependency`** runs every step inline with no phase filter, an
  empty `ToolInstallDir`, and a discarded cleanup slice. It fails loudly for
  `source_command` and silently orphans files for `source_file`; separate issue.
  *Resolved for tool dependencies in issue #2462: the dependency path now filters by
  phase, populates `ToolInstallDir` before post-install steps run, and records its
  cleanup actions against a state entry of the dependency's own. The exclusion rule is
  unchanged and still has writers to protect — `install_lib.go` runs no post-install
  phase, a library pulled in as a dependency has no field to record into, and every
  fragment written by an earlier release stays unrecorded.*
- **The already-installed short-circuit** returns before running any steps and `--force`
  does not bypass it, so an existing install never picks up a fix. `tsuku verify`
  recommends a `--reinstall` flag that does not exist; separate issue.
  *Resolved in issue #2463: `tsuku install <tool> --reinstall` bypasses the
  short-circuit and re-executes the plan, so an existing install can pick up a fix and
  a deleted or modified shell.d fragment has a repair. The flag also bypasses the
  hidden-tool expose and the library path's two reuse checks, and runs
  `StaleCleanupActions` against the record the replaced install left, so a fragment the
  recipe no longer writes is deleted rather than orphaned. Reinstall is scoped to the
  named tool; its dependencies are left alone.*
- **`recipes/n/nvm.toml`'s `NVM_DIR` model.** `NVM_DIR` is nvm's data root as well as its
  program directory — it holds `versions/node/`, `alias/`, `.cache/`, and
  `default-packages`. Pointing it at a tsuku-versioned directory puts every node version
  and every global npm package the user installs inside a directory tsuku garbage-collects.
  This design makes the fragment correct; it does not make the model correct, and shipping
  it without saying so would be misleading. Separate issue, and the more consequential one
  for nvm users.
