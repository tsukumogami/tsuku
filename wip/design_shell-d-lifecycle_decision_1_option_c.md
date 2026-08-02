# Option C: version-key the shell.d filename and let state, not the directory listing, decide what gets sourced

All line numbers are against branch `docs/shell-d-lifecycle` (based on `origin/main`,
tip `8a7c8908`). The prototype branch `origin/fix/2439-set-env-exports` was read via
`git diff origin/main...origin/fix/2439-set-env-exports` and is not modified.

## Design

### The two halves

Option C is usually described as one change — rename the file — but it is two, and the
second is the one that carries the weight.

1. **Writers name files `<target>@<version>.<shell>`.** Installing v2 no longer touches
   v1's file. Each version's recorded `ContentHash` describes a file only that version
   ever wrote, so it is correct permanently and by construction.
2. **`RebuildShellCache` sources exactly the files that state says belong to an active
   version, and nothing else.** Today the cache is a directory listing; under C it is a
   projection of `state.json` onto the directory. This is what makes a non-active
   version's file unreachable rather than merely differently-named.

Half 1 alone does not fix anything — it just puts two files in the directory where one
used to be, and today's builder would concatenate both. Half 2 is the design.

### Naming scheme

```
$TSUKU_HOME/share/shell.d/<target>@<version>.<shell>
$TSUKU_HOME/share/shell.d/00-env-<recipe-name>@<version>.<shell>   (set_env, prototype)
```

Examples, from `recipes/n/nvm.toml`:

```
share/shell.d/00-env-nvm@0.40.6.bash
share/shell.d/00-env-nvm@0.40.6.zsh
share/shell.d/nvm@0.40.6.bash
share/shell.d/nvm@0.40.6.zsh
```

`@` is the separator because it is already tsuku's user-facing tool-version separator
(`tsuku install nvm@0.40.6`, `tsuku remove nvm@0.40.6`), so the directory listing reads
itself. It also sidesteps the ambiguity that `-` carries — and that ambiguity is not
hypothetical in this codebase: `updates.GarbageCollectVersions`
(`internal/updates/gc.go:31-36`) already matches version directories by the prefix
`toolName + "-"`, so a tool named `foo` will happily consider `tools/foo-bar-1.0/` its
own when `foo-bar` is a separate installed tool. `config.ToolDir`
(`internal/config/config.go:406-408`) has the same shape. Copying that convention into
shell.d would import a latent bug into the one place where we are trying to make a bug
unrepresentable.

**One new constraint, on `target` only.** `install_shell_init`'s Preflight
(`internal/actions/shell_init.go:44-72`) gains:

```go
target must match ^[A-Za-z_][A-Za-z0-9._-]*$
```

`actions.ValidateAction` (`internal/actions/preflight.go:86`) is what recipe validation
calls, so a recipe violating this is rejected in CI before it can be published. The only
recipe using `install_shell_init` today is `recipes/n/nvm.toml:36-39` with
`target = "nvm"`, which passes. This constraint replaces the prototype's narrower
`strings.HasPrefix(target, EnvFilePrefix)` rejection and subsumes it.

No change to `ValidateVersionString` (`internal/install/state.go:335-346`) is needed.
Version strings keep their current, very loose contract (no `..`, `/`, `\`). Tightening
it would risk making an already-installed tool unremovable.

**Injectivity proof.** Claim: the map `(target, version, shell) -> filename` is injective,
given `target` matches the pattern above (in particular, contains no `@`) and
`shell ∈ {bash, zsh, fish}` (`internal/actions/shell_init.go:16-20`).

Suppose `t₁ + "@" + v₁ + "." + s₁ == t₂ + "@" + v₂ + "." + s₂`.

- Neither `t₁` nor `t₂` contains `@`, so the first `@` in the common string is the
  separator in both decompositions. Therefore `t₁ == t₂`, and the remainders are equal:
  `v₁ + "." + s₁ == v₂ + "." + s₂`.
- Suppose `s₁ ≠ s₂`. Then the common string ends in both `"." + s₁` and `"." + s₂` for
  two distinct members of `{bash, zsh, fish}`. No member of that set is a suffix of
  another (`bash`/`zsh`/`fish` differ in their final three characters: `ash`, `zsh`,
  `ish`), so a string cannot end in `.bash` and `.zsh` simultaneously. Contradiction, so
  `s₁ == s₂`.
- With `s₁ == s₂`, trimming the common suffix gives `v₁ == v₂`. ∎

The brief's worry case resolves cleanly: tool `foo-1` at version `2` gives `foo-1@2.bash`;
tool `foo` at version `1-2` gives `foo@1-2.bash`. Different strings.

Worth stating plainly, because it changes what the proof has to carry: **nothing parses
the filename.** The version is in the name so that two versions cannot share a file and
so that a human reading `ls share/shell.d/` can see what happened. Every consumer that
needs to know which tool or version a file belongs to reads `state.json` instead. The
filename is an opaque collision-free key, and injectivity is the only property required
of it.

**Sort order.** `sort.Strings` at `internal/shellenv/cache.go:105` is a byte-order sort,
and the prototype documents the exports-before-init contract there. The contract survives:

- Exports files start with `0` (0x30). Init files start with the target's first character,
  which the new pattern constrains to `[A-Za-z_]`, i.e. ≥ `A` (0x41). So every
  `00-env-*` file sorts before every init file, for any target, with no case analysis.
  This is stronger than the prototype's guarantee, which rests on "no plausible init
  filename sorts before `00-env-`" and would be broken by a target like `00-a`.
- Cross-tool ordering between two unrelated tools is not a contract anyone states, and
  version-keying does perturb it in one narrow case: where one tool's name is a proper
  prefix of another's and the next character is a digit (`foo` vs `foo0` swap, because
  `@` at 0x40 sorts after `0` at 0x30, where `.` at 0x2E sorted before it). Adding or
  removing any tool already reorders neighbours, so this is not a regression in any
  property the system currently provides — but it is a real difference and should not be
  discovered later.
- Within one tool, several versions' files coexist in the directory, but only one is ever
  in the cache, so intra-tool ordering never arises.

### The cache-builder seam

`internal/install` imports `internal/shellenv` (`internal/install/remove.go:13`), so
`shellenv` cannot import `install`. The manifest type therefore lives in `shellenv` and
the constructor lives in `install`, which is the direction that already works. No import
cycle, no move of `install.ValidateRequested`, no new capability interface.

New type in `internal/shellenv`:

```go
// ShellDManifest is state.json's view of share/shell.d: which files tsuku wrote,
// and which of those belong to a tool's active version.
type ShellDManifest struct {
	// Active maps a $TSUKU_HOME-relative shell.d path to the SHA-256 recorded
	// when it was written. The init cache is exactly these files, in sorted
	// order. A file on disk that is not named here is never sourced.
	Active map[string]string

	// Known maps every shell.d path recorded by any installed version, active
	// or not, to its recorded hash. Known is a superset of Active. Files in
	// Known but not Active are dormant: they belong to an installed version
	// that is not currently active, and they become live if it is activated.
	Known map[string]string
}
```

New constructor in `internal/install` (pure, no filesystem, table-testable):

```go
// BuildShellDManifest projects the recorded cleanup actions of every installed
// tool onto the shell.d directory.
func BuildShellDManifest(state *State) shellenv.ShellDManifest
```

It is the existing loop from `cmd/tsuku/doctor.go:198-215` — iterate `state.Installed`,
resolve `ts.ActiveVersion` with the legacy `ts.Version` fallback, collect
`vs.CleanupActions` whose `Path` has the `share/shell.d/` prefix — extended to also walk
the non-active versions into `Known`. A `Manager` convenience wrapper
(`func (m *Manager) shellDManifest() shellenv.ShellDManifest`) serves the in-package
callers.

Signatures change from variadic-optional to required:

```go
func RebuildShellCache(tsukuHome, shell string, manifest ShellDManifest) error
func CheckShellD(tsukuHome string, manifest ShellDManifest) *ShellDCheckResult
```

Making the parameter required is deliberate: today four of five call sites pass nothing
(`internal/install/remove.go:400`, `internal/install/update.go:58`,
`cmd/tsuku/install_deps.go:595`, `cmd/tsuku/plan_install.go:134`), and under C "pass
nothing" would mean "concatenate every version of every tool" — the worst possible
default. The variadic signature makes the dangerous case the easy one; a required
parameter makes it unrepresentable.

Inside `RebuildShellCache`, the selection loop (`cache.go:66-94`) changes from "every
directory entry ending in `.{shell}`" to "every entry in `manifest.Active` ending in
`.{shell}`, stat'd and read from disk". The symlink and regular-file rejections
(`cache.go:77-90`) stay. Hash verification (`cache.go:119-131`) stays and now means
tampering rather than version confusion, because a wrong-version file can no longer be
in `Active`. The `.init-cache` / `.lock` exclusions (`cache.go:71-73`) become dead code
and can go, since those names are never in the manifest.

That last point is a security improvement worth naming: today anyone who can write into
`share/shell.d/` gets their bytes sourced into every new shell, because unrecorded files
are included unverified as "legacy". Under C an unrecorded file is inert.

The five call sites:

| Call site | Today | Under C |
|---|---|---|
| `internal/install/remove.go:400` (`rebuildShellCaches`) | no hashes | `m.shellDManifest()`; **must move to after the `UpdateTool` that promotes the new active version** (currently called at `remove.go:161`, before the promotion at `:171-186`) |
| `internal/install/update.go:58` (`ExecuteStaleCleanup`) | no hashes | `m.shellDManifest()` |
| `cmd/tsuku/install_deps.go:595` | no hashes | `install.BuildShellDManifest(state)`; **must move to after `mgr.RecordCleanup` at `:613`** |
| `cmd/tsuku/plan_install.go:134` | no hashes | same, and **requires adding the `RecordCleanup` call this path has never made** |
| `cmd/tsuku/doctor.go:66` (`--fix`) | yes, active-version map | the same map, now typed |

Two new call sites, both one line, both inside `internal/install` where `shellenv` is
already imported:

- `Manager.Activate` (`internal/install/manager.go:411-470`), after the `UpdateTool` at
  `:459-464`. Covers `tsuku activate`, `tsuku rollback` (which delegates at
  `manager.go:364`), and auto-apply's silent failure rollback
  (`internal/updates/apply.go:173`) — all three, because all three route through
  `Activate`, and because the hook is inside `install` rather than in `cmd/tsuku`.
- `RemoveVersion`'s promotion branch, alongside the existing
  `createSymlinksForBinaries` at `remove.go:202`.

The ordering discipline is uniform and easy to state: **rebuild the cache after the state
write, never before.** The manifest is derived from state, so a rebuild that runs first
sees the old world.

### What falls out for free

- `executeCleanupActions`'s `otherPaths` skip (`remove.go:332-349`) never fires for a
  version-keyed shell.d path, because no two versions record the same path. Removing a
  version deletes exactly its own files. The mechanism stays in place for
  `share/completions/` and for legacy records; it simply stops being reached for the
  case it was breaking. `affectedShells` gets populated correctly, so the cache is
  rebuilt — fixing the second half of the reproduction (the cache silently retaining the
  removed version's bytes) with no code change at all.
- `install_completions` (`internal/actions/completions.go:99-115`) has the identical
  defect in `share/completions/{shell}/{target}`. The same rename applies verbatim and
  is a small, independent follow-up; C neither requires nor blocks it. There is no
  completions cache, so there is no builder half to do.
- `--no-shell-init` becomes coherent. Install v1 normally, install v2 with the flag: v2
  is active and records no shell.d path, so the manifest has no entry and the tool is
  absent from new shells. That is what the user asked for. Under main, v1's file lingers
  and is sourced.

### Changes forced elsewhere

**`isCacheStale` must filter identically to the builder.** `internal/shellenv/doctor.go:146-174`
recomputes the expected cache from every file on disk with no filtering. Under C, dormant
versions' files are on disk and excluded from the cache, so `isCacheStale` would return
`true` forever on any multi-version install and `doctor` would fail permanently. It has to
take `manifest.Active` and build the same subset. This is mandatory, and it happens to fix
the pre-existing non-convergence two peer leads found independently: today `--fix`
rebuilds a cache that omits a hash-mismatched file while `isCacheStale` recomputes one
that includes it, so `--fix` can never clear the failure. Aligning the two is a
prerequisite for C and a bug fix on its own.

**`StaleCleanupActions` must exclude paths still owned by an installed version.**
`internal/install/update.go:14-36` computes old-minus-new by `(action, path)`. Under C
the old and new versions share no shell.d path, so *every* old action is "stale" and
`ExecuteStaleCleanup` (`update.go:43-62`, called from `cmd/tsuku/update.go:183`) would
delete the previous version's shell.d file while that version is still installed and
still the rollback target. That is a genuine regression the rename introduces, and it has
to be fixed in the same change. The correct rule is the one `otherPaths` already
implements: a path is stale only if no installed version records it. Note that under C
the problem `ExecuteStaleCleanup` was written to solve — the new version dropping a shell
and leaving the old version's file to be sourced — cannot occur, because the old
version's file is not in `Active`.

**`warnShellInitChanges` (`cmd/tsuku/update.go:232-258`) stops firing.** It compares
hashes for paths present in both old and new; under C there are none. Re-deriving it
means comparing by `(target, shell)` rather than by path. This is a small loss of a
useful signal, and re-deriving it is straightforward, but it is work the rename creates
rather than avoids.

**`HasShellIntegration` (`internal/shellenv/doctor.go:197-209`) breaks and should be
replaced rather than patched.** It probes `{toolName}.bash` / `{toolName}.zsh` by name
and has exactly one caller, `cmd/tsuku/info.go:147`. It already carries a wrong
assumption — the filename is built from the recipe's `target`, not the tool name, and
those need not match. The replacement reads the tool's active `CleanupActions` and
reports the shell suffixes of its `share/shell.d/` paths: no filename probing, correct
for a `target != name` recipe, and correct under C. Because it needs state, the lookup
moves to `cmd/tsuku/info.go` (which already builds a `StateManager` twelve lines later at
`info.go:151`), with `shellenv` keeping only a `ShellsFromPaths([]string) []string`
helper. This is the only other place that hard-codes the filename assumption; I grepped
for `HasShellIntegration`, `shell.d`, `shellDDir`, and `init-cache` across all Go, shell,
markdown, and TOML in the tree. `shellFromCleanupPath` (`remove.go:386-395`) uses
`filepath.Ext`, which returns `.bash` for `nvm@0.40.6.bash` and needs no change.
`config.EnvFileContent` (`config.go:446-465`) sources the cache by fixed name and is
unaffected.

**`GarbageCollectVersions` grows a shell.d obligation.** `internal/updates/gc.go:15-69`
deletes tool *directories* and touches neither `state.json` nor cleanup actions. Under
main that leaves nothing behind in shell.d, because there is only ever one file per
tool. Under C, a GC'd version's shell.d files survive it — dormant, unreachable (GC
protects the active and previous versions at `gc.go:38-46`, and `Activate` refuses a
version whose directory is missing at `manager.go:440-443`, so a GC'd version can never
become active again), and permanent. That is a new leak the rename creates, roughly
200 KB per GC'd nvm version, growing without bound for any auto-updated tool with shell
init. GC has to run the deleted version's `delete_file` cleanup actions and drop its
`VersionState`. Doing so also closes the hole a peer lead flagged, where a GC'd version's
surviving `VersionState` holds the `otherPaths` skip open for a directory that no longer
exists.

### Files touched

`internal/actions/shell_init.go`, `internal/shellenv/cache.go`,
`internal/shellenv/doctor.go`, `internal/install/remove.go`, `internal/install/update.go`,
`internal/install/manager.go`, `internal/install/state_ops.go` (or a new
`internal/install/shelld.go` for the manifest builder), `internal/updates/gc.go`,
`cmd/tsuku/install_deps.go`, `cmd/tsuku/plan_install.go`, `cmd/tsuku/doctor.go`,
`cmd/tsuku/info.go`. Plus, on whatever branch merges the prototype,
`internal/actions/set_env.go` for the same filename change.

**No allowlisted golden-plan file is touched.** The workflow at
`.github/workflows/validate-golden-code.yml:6-45` triggers on `internal/actions/action.go`,
`internal/executor/plan.go`, `internal/executor/plan_conversion.go`,
`internal/recipe/types.go` and others; C touches none of them, because it adds no
capability interface, no `Phase` field on `install.PlanStep`, and no recipe schema change.
That workflow currently fails on unrelated Homebrew bottle drift on `main`, so not arming
it is a real, not merely theoretical, saving. C is the only option on the table that reads
the stored plan not at all.

## Migration

Existing installs have `share/shell.d/nvm.bash` on disk and
`share/shell.d/nvm.bash` recorded in `state.json`. There is **no rename pass, no schema
migration, and no first-run hook.** Legacy records are simply valid manifest entries.

The manifest is built from recorded paths, not from a naming convention. A pre-upgrade
record says `share/shell.d/nvm.bash`; that path goes into `Active` if its version is
active, the file on disk is found, its recorded hash verifies, and it is sourced exactly
as before. A post-upgrade record says `share/shell.d/nvm@0.40.7.bash` and is handled the
same way. The builder does not know or care which format it is looking at.

The transition for one tool then plays out on its own:

1. User upgrades tsuku. Nothing changes. `nvm.bash` is still recorded, still active,
   still sourced.
2. User installs nvm 0.40.7. It writes `nvm@0.40.7.bash` and records that path.
   `RecordCleanup` overwrites the active version's actions, and 0.40.7 is active, so
   `Active` now names only the new file. `nvm.bash` is on disk but unrecorded-as-active,
   so it is not sourced. The user gets 0.40.7's content — correct, immediately.
3. User removes 0.40.6. Its cleanup action names `share/shell.d/nvm.bash`; `otherPaths`
   no longer contains that path, because 0.40.7 recorded a different one. The file is
   deleted. The directory is now fully in the new format.

The mechanism that cleans up the old layout is the same mechanism that was broken —
which is a decent sign the shape is right.

Three residues, stated honestly:

- **A user who never installs a new version keeps the legacy layout forever.** That is
  fine; it works.
- **Pre-upgrade multi-version state stays wrong until those versions are removed.** If
  two versions both recorded `share/shell.d/nvm.bash`, they still both do, and activating
  between them still gives a hash mismatch and an excluded file. C fixes everything
  written after the upgrade and grandfathers what came before. It does not retroactively
  repair a directory that is already corrupt.
- **A legacy file for a version whose directory was GC'd** stays on disk, dormant. The
  GC change above only covers versions GC'd after the upgrade.

If the residue is judged unacceptable, a one-shot repair is available and is strictly
additive: for each legacy `share/shell.d/<target>.<shell>` record, hash the file on disk
and compare it against every installed version's recorded hash for that path. On a unique
match, rename the file to that version's key and rewrite the record; on no match or an
ambiguous match, leave it alone. This is a `doctor --fix` repair, not a load-time
migration — putting filesystem renames inside `State.Load()`
(`internal/install/state_tool.go:199-218`, which runs on every command) would be a poor
trade for a case that resolves itself. I would ship without it and add it only if the
residue shows up in practice.

**The one hard prerequisite is `plan_install.go`.** `cmd/tsuku/plan_install.go:125-143`
writes shell.d files and never calls `RecordCleanup` — `grep -rn "RecordCleanup("`
minus tests yields only `cmd/tsuku/install_deps.go:613`. Under main those files are
orphans that get sourced anyway, because unrecorded files are included unverified. Under
C, unrecorded means unsourced, so `tsuku install --plan` would silently produce a tool
with no shell integration. That is a regression, and it is not optional to fix. The fix
is the 25 lines that already exist twice in `install_deps.go` and `plan_install.go` and
have already drifted apart once; extracting them into one helper is the right shape and
is the "gap 1" the exploration flagged as worth folding in regardless.

## Why it wins

**The invariant is structural, not procedural.** Every other option keeps the property
"the file's content must be kept in agreement with the active version" and adds machinery
to maintain it — a replay driver, a render interface, a blob in state. Each of those is
correct only as long as every future lifecycle event remembers to call it. C removes the
property. A file's content is fixed at the moment it is written and never needs to agree
with anything again, because the file is only ever reachable from the version that wrote
it. There is no event that can be forgotten, because there is nothing to do at an event
except recompute a concatenation.

**`ContentHash` becomes a real invariant instead of a fact with a short shelf life.**
Today the hash recorded for 0.40.5 describes a file that 0.40.6's install has already
overwritten; the record is stale the moment a second version lands. Under C it describes
a file only 0.40.5 ever wrote. A mismatch therefore means exactly one thing —
post-install tampering — which is what makes it worth reporting and what makes excluding
the file the right response. `doctor`'s existing detection stops producing false alarms
and starts producing actionable ones.

**Activate and rollback need no content derivation.** This is the concrete payoff. Under
every other option, `Manager.Activate` has to produce 0.40.5's bytes from something: a
replayed plan step (which for `source_command` means executing a binary during a version
switch), a render interface (whose no-side-effects contract `install_shell_init` cannot
honor), a recipe reload (network, and the recipe may have changed upstream), or a blob in
`state.json` (which is loaded in full on every command). Under C, 0.40.5's file was
never touched, so switching to it is one call to `RebuildShellCache`. The
`source_command` problem — the one every peer lead named as the hardest open question —
does not arise, because the bytes are never re-derived. That includes offline operation
and reproducibility, both of which the other options put at risk on paths that are
offline today.

**It fixes three latent bugs it did not set out to fix**, because each is load-bearing
for the design and cannot be deferred: `doctor --fix`'s non-convergence (forced by
aligning `isCacheStale` with the builder), `plan --install`'s missing `RecordCleanup`
(forced by the manifest gate), and GC's failure to touch state (forced by the disk-leak
obligation). Two of the three were found independently by separate research leads as the
most surprising things in the codebase.

**It hardens the directory.** A manifest-gated cache means only files tsuku recorded are
sourced into the user's shell. The current "include unrecorded files unverified" behavior
is a write-to-source pipeline for anything that lands in `share/shell.d/`.

**Cost profile is the lightest of the field.** No import-cycle work — the manifest type
goes in `shellenv` and the constructor in `install`, which is the direction that already
compiles. No plan schema change, so `install.PlanStep`'s dropped `Phase` field is
irrelevant. No new capability interface. No allowlisted golden-plan file, so the workflow
that currently fails on unrelated bottle drift stays disarmed. The manifest builder is a
pure function over `*State` and is table-testable with no filesystem, no executor, and no
subprocess — which matters given the exploration's finding that the test cannot live in
`internal/install` and that `cmd/tsuku` has no unit tests today.

**Disk cost is real but small and in the right place.** nvm.sh is roughly 100 KB (peer
figure; I had no network to verify), so 200 KB per retained version across bash and zsh.
Against `tools/nvm-<version>/`, which already holds a full source tree per version
including its own copy of `nvm.sh`, that is a rounding error. Retention is time-based
(`userconfig.DefaultVersionRetention`, 7 days, `userconfig.go:168-169`) and applies only
to the auto-apply GC path, so manual installs accumulate versions until removed — but
they accumulate multi-megabyte tool directories at the same rate. Compare candidate D,
which puts the same bytes in `state.json`, a file loaded in full on effectively every
command. C puts them in a directory read by one function.

## Self-attack

**"The cache builder now needs to know about tools and versions. You have traded a clean
bug for a muddy abstraction."**

This is the objection I take most seriously, and I think it is half right in a way that
is worth naming precisely. `shellenv` gains no knowledge of tools or versions — the
manifest is `map[path]hash`, and `shellenv` never parses a path or learns what a version
is. But it does gain a *dependency on state having been consulted*, and that is a real
coupling: `RebuildShellCache` is no longer a pure function of a directory, and calling it
correctly now requires knowing that the caller must load state first and must do so after
its own state write. The ordering discipline ("rebuild after the state write, never
before") is exactly the kind of implicit contract that rots.

My answer is that this coupling already exists and is currently expressed badly. The
doctor call site already builds precisely this map, from precisely the active versions,
because that is the only correct thing to pass. The other four sites pass nothing, and
the research showed that is why hash verification is inert across the entire product.
C does not introduce the dependency; it makes the existing one mandatory and typed. A
required parameter that every caller must construct is a worse abstraction than a pure
directory scan and a better one than a variadic where the dangerous default is the empty
call.

What would genuinely reduce this objection, and which I would build: make
`shellDManifest()` a `Manager` method so the two in-package sites cannot get it wrong,
and give `RebuildShellCache` a doc comment that states the ordering contract at the
signature. What would not: pretending the coupling is smaller than it is.

**"Migration is the biggest risk and you have hand-waved it by declaring legacy records
valid."**

The dual-read is not hand-waving — it works because the manifest is keyed on recorded
paths rather than on a naming convention, so the two formats are indistinguishable to
every consumer. But there are two things I am genuinely choosing not to solve, and I want
them on the record rather than buried. First, pre-upgrade multi-version corruption is
grandfathered: if a user already has two versions sharing `nvm.bash`, C does not repair
them, it just stops making new ones. Anyone expecting "upgrade tsuku, run doctor, it's
clean" will be disappointed. Second, the self-healing path in step 3 depends on the user
eventually removing the old version, and plenty of users never remove anything. The
legacy file can sit there indefinitely. It is inert and it is 100 KB, so I think that is
acceptable, but "acceptable" is a judgement and not a proof.

**"You claimed activate needs no re-render, then added a hook to `Activate` anyway."**

Fair, and the brief's framing slightly oversells this. C does not eliminate the lifecycle
hook — `Activate` and `RemoveVersion`'s promotion both gain a `RebuildShellCache` call,
at exactly the three sites that write `ToolState.ActiveVersion`. What C eliminates is the
*content derivation* at those sites. The hook goes from "reproduce a past version's bytes
without running an install" — the thing no peer lead could find a clean answer to — to
"concatenate files that are already on disk." Same number of call sites, radically
different difficulty. That is the honest claim and it is still a strong one, but "no
re-render at all" is not accurate and the design doc should not say it.

**"What happens to a file whose version was garbage-collected?"**

Under the design as first stated: it sits there forever, unreachable and unbounded. I
found this while checking `gc.go` and it is a defect the rename introduces. GC deletes
directories by prefix match and never opens `state.json` (`gc.go:15-69`), so nothing
would ever remove the shell.d files or the `VersionState` of a GC'd version. Two hundred
kilobytes per GC'd nvm version, forever, on the auto-update path — which is the path that
runs without the user watching. The fix (run the version's `delete_file` cleanups and drop
its `VersionState`) is small and is arguably a bug fix, but it is unambiguously scope the
rename creates and I would not have found it if I had not gone looking. It makes me
slightly less confident that I have found all of them.

**"C prevents the bug but adds no repair capability, and repair is what users need."**

This is the strongest objection and I do not have a rebuttal, only a scoping argument.
If a user deletes `nvm@0.40.6.bash`, or edits it, or a disk error corrupts it, C offers
nothing: `doctor` reports a missing or mismatched file and `--fix` cannot regenerate it,
because C's whole premise is that content is never re-derived. Reinstalling does not help
either — `cmd/tsuku/install_deps.go:509-516` short-circuits on an already-installed
version, `--force` only suppresses security prompts, and the `--reinstall` flag that
`tsuku verify` recommends does not exist. So the user's only recovery is
`tsuku remove nvm@0.40.6 && tsuku install nvm@0.40.6`.

The options that can re-render can also repair. That is a capability C structurally
cannot have, and if repair is in scope for this work then C is at best half the answer.
My scoping argument is that repair and prevention are different features with different
triggers — prevention fires on lifecycle events tsuku performs, repair fires on damage
tsuku did not cause — and that shipping prevention first is right because it stops the
bleeding without betting on a mechanism. But if the acceptance criteria include
"`doctor --fix` repairs a damaged shell.d file", C does not meet them and no amount of
renaming will make it.

**"`ExecuteStaleCleanup` and `warnShellInitChanges` both break. How many more are there?"**

I found those two by reading every consumer of a shell.d path in the tree, and both
follow from the same root cause: code that reasons about "the same path across two
versions" has no work to do when paths are version-keyed, and in
`ExecuteStaleCleanup`'s case it does *harmful* work. I believe the list is complete —
the consumers are enumerable and I walked them — but the pattern is one where a missed
case fails by deleting a live file rather than by failing to compile, which is the bad
kind of failure mode. `StaleCleanupActions` in particular deletes the rollback target's
shell.d file if the fix is missed, and no test on `main` would catch it, because
`internal/install/update_test.go` builds its cases from version-independent paths
throughout.

**"The `target` charset constraint is a breaking change to the recipe schema."**

Technically yes; practically no. One recipe uses `install_shell_init`
(`recipes/n/nvm.toml`, `target = "nvm"`), and `actions.ValidateAction` runs in recipe CI,
so a violating recipe cannot be published. But it does forbid a shape that is legal today
(a target starting with a digit, e.g. `7zip`), and if some future recipe wants that, the
sort-order proof needs redoing. The weaker rule "target may not start with `0`" would
suffice and would admit `7zip`; I chose the identifier rule because it is easier to state
and to remember, and because reserving the numeric prefix for tsuku's own ordering slots
is the convention `/etc/profile.d` arrived at independently. That is a taste call, not a
technical one.

## What would make this the wrong choice

Four falsifiers, in descending order of how likely I think each is.

**1. Repair is in scope.** If the acceptance criteria include regenerating a shell.d file
that is missing, corrupted, or user-edited — the natural next request after `doctor`
starts reporting real mismatches instead of false ones — then C cannot deliver it and a
render mechanism has to be built anyway. At that point C's per-version files are extra
bookkeeping layered on top of the mechanism that actually does the work, and the right
answer is the render mechanism alone with a version-independent filename. This is the
falsifier I would test first, and the question to put to whoever owns the acceptance
criteria is simply: *when `doctor` reports a hash mismatch, must `--fix` fix it?* If yes,
C is the wrong primary choice. If no, C is the cheapest correct answer.

**2. The project decides shell.d content should be version-independent by construction.**
If `install_shell_init` changes from copying `nvm.sh` to emitting a sourcing stub against
a stable indirection path (`. "$TSUKU_HOME/opt/nvm/nvm.sh"`, the Homebrew `opt_prefix`
shape the prior-art lead argued is cheap to add here), then the content stops being
version-specific, one file per tool is correct again, and version-keying the filename is
dead weight that has to be migrated back out. C and stable indirection are not
complementary the way indirection and re-render are — they are alternatives, and
indirection is the better one if it can be made to cover the byte-copy case. It cannot
today, which is why C is on the table. If someone makes it cover that case, C loses.

**3. `state.json` cannot be the sole index of what gets sourced.** C's foundation is that
every file in `share/shell.d/` that should be sourced has a corresponding record. If
there is a supported path that writes shell.d without recording — one that cannot be
fixed the way `plan_install.go` can — then a manifest-gated cache silently drops a
working integration, and the whole approach is a breaking change dressed as a bug fix.
I found exactly one such path and it is fixable. If a second turns up that is not, C is
wrong.

**4. Cross-tool sort order turns out to be a contract someone relies on.** The
prefix-plus-digit reordering (`foo` and `foo0` swap) is the only ordering property C
perturbs. Nothing documents cross-tool ordering as a contract and adding any tool already
perturbs it, so I rate this unlikely — but if some recipe pair depends on it, `@` is the
wrong separator and the scheme needs a separator that sorts below every legal target
character, which means reopening the injectivity proof.
