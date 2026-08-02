# Option B: Record each version's rendered shell.d bytes at install time and write them back whenever that version becomes active

All line numbers are against branch `docs/shell-d-lifecycle` (based on `origin/main` at
`4d470df9`). Prototype references are to `origin/fix/2439-set-env-exports`, read but not
modified.

## Design

### The move that makes this cheap: capture in `internal/install`, not `internal/actions`

The obvious implementation threads a `Content` field down from the actions that write
shell.d files — `install_shell_init` (`internal/actions/shell_init.go:143-150`), the
prototype's `set_env`, and `install_completions`
(`internal/actions/completions.go:106-115`) — into `actions.CleanupAction`
(`internal/actions/action.go:54-60`), through `convertCleanupActions`
(`cmd/tsuku/install_deps.go:744-755`), into `install.CleanupAction`
(`internal/install/state.go:17-21`).

Do not do that. `internal/actions/action.go` is on the `validate-golden-code.yml` path
allowlist (`.github/workflows/validate-golden-code.yml:15`), and a Go struct cannot be
split across files, so any change to `CleanupAction` or `ExecutionContext` arms a workflow
that regenerates plans against live upstream and currently fails on unrelated Homebrew
bottle drift.

It is also unnecessary. `CleanupAction.Path` is already relative to `$TSUKU_HOME`
(`state.go:19`), `Manager` already holds `m.config.HomeDir` (used by `executeSingleCleanup`,
`internal/install/remove.go:360-377`), and `Manager.RecordCleanup`
(`internal/install/state_ops.go:79-94`) runs at `cmd/tsuku/install_deps.go:613` — after the
post-install phase has written the files and after the cache rebuild at `:595`. The bytes
are sitting on disk at a known path when `RecordCleanup` is called. It can read them back
itself.

**Consequence: Option B touches no file on the golden-plan allowlist.** `internal/actions`
is untouched entirely; `internal/executor` is untouched entirely; `plan_conversion.go`,
`internal/executor/plan.go`, and `internal/recipe/types.go` are untouched. The change is
confined to `internal/install` plus a doctor branch in `cmd/tsuku`.

It also means the mechanism is action-agnostic by construction. `install_shell_init` in both
modes, the prototype's `set_env`, and `install_completions` are covered with zero per-action
code, because all three already record a `delete_file` action with a `ContentHash` through
the same funnel.

### Schema

One field, on `install.CleanupAction` only (`internal/install/state.go:17-21`):

```go
type CleanupAction struct {
	Action      string `json:"action"`                 // "delete_file", "delete_dir"
	Path        string `json:"path"`                   // relative to $TSUKU_HOME
	ContentHash string `json:"content_hash,omitempty"` // SHA-256 hex digest of file content at install time

	// Content is the gzip-compressed bytes this version wrote to Path, so the
	// file can be restored when this version becomes active again. encoding/json
	// marshals []byte as base64, so this is one base64 string in state.json.
	//
	// Empty means "not recoverable from state": versions installed before this
	// field existed, installs that never reached RecordCleanup, and any action
	// that records no ContentHash. Restore treats empty as a warning, never as
	// a reason to delete the file on disk.
	Content []byte `json:"content,omitempty"`
}
```

`actions.CleanupAction` (`internal/actions/action.go:54-60`) is **unchanged**.

**Encoding: gzip, then base64 via `[]byte`.** Measured on `nvm.sh` 0.40.6 (161,810 bytes):
raw JSON-escaped string 171,553 bytes; base64 of the raw bytes 215,748 (worse than the
escaped string — `[]byte` without compression is a mistake); gzip -9 then base64 46,236.
Gzip is a 3.7x reduction against the best uncompressed encoding and is the only encoding
worth shipping. `omitempty` on a `[]byte` elides both nil and zero-length, so no existing
state.json grows by a single byte until its tool is reinstalled.

### Capture

`internal/install/state_ops.go`, one new unexported method plus two lines in
`RecordCleanup`:

```go
func (m *Manager) RecordCleanup(name string, actions []CleanupAction) error {
	if len(actions) == 0 {
		return nil
	}
	actions = m.captureContent(actions)
	return m.state.UpdateTool(name, func(ts *ToolState) { /* unchanged */ })
}

// captureContent fills Content for every delete_file action that recorded a
// ContentHash, by reading back the bytes the action just wrote. The hash is
// re-verified against what was read, so a stored Content can never disagree
// with its stored ContentHash: a mismatch means the file changed between the
// write and this read, and the safe response is to store nothing and let
// restore fall back.
func (m *Manager) captureContent(actions []CleanupAction) []CleanupAction {
	out := make([]CleanupAction, len(actions))
	copy(out, actions)
	for i := range out {
		if out[i].Action != "delete_file" || out[i].ContentHash == "" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.config.HomeDir, out[i].Path))
		if err != nil || contentHashHex(data) != out[i].ContentHash {
			continue
		}
		gz, err := gzipBytes(data)
		if err != nil {
			continue
		}
		out[i].Content = gz
	}
	return out
}
```

`contentHashHex` duplicates four lines from `internal/actions/shell_init.go:136-139`; that
is below `dupl`'s 250-token threshold and cannot be shared without an import edge. `gosec`
G304 will flag the `os.ReadFile` on a non-constant path — the path is tsuku-authored and
joined under `HomeDir`, so it takes a `// #nosec G304` with that justification, matching how
the rest of the package handles state-relative reads. `errcheck` is satisfied because every
error is inspected.

### Restore

`internal/install/restore.go` (new file, ~70 lines):

```go
// RestoreShellFiles rewrites the shell.d and completions files recorded by the
// given version, so the files on disk match the version that is becoming active.
// It never deletes: an action whose Content is empty is reported in missing and
// left alone, because turning wrong content into no content is the failure mode
// doctor --fix already produces (see internal/shellenv/cache.go:151-155).
//
// Returns the paths that could not be restored, in state.json-relative form.
func (m *Manager) RestoreShellFiles(ts *ToolState, version string) (missing []string, err error)
```

Behavior, per `delete_file` action of `ts.Versions[version].CleanupActions`:

1. Skip actions whose `Path` is not under `share/shell.d/` or `share/completions/`. The
   vocabulary is otherwise unbounded and restore should not resurrect arbitrary paths.
2. `Content == nil` → append to `missing`, continue.
3. Decompress; verify `contentHashHex(plain) == ContentHash`; on mismatch append to
   `missing` and continue. This is the second half of the "cannot disagree" guarantee.
4. `os.MkdirAll(dir, 0700)` then `os.Chmod(dir, 0700)`, mirroring
   `internal/actions/shell_init.go:111-117`.
5. Write via temp file + `os.Rename` at mode `0600`, mirroring `shell_init.go:165, 227`.
   Restore must set the mode explicitly rather than inherit anything.
6. Collect affected shells via the existing `ShellFromCleanupPath`
   (`internal/install/remove.go:386-395`) and call `m.rebuildShellCaches`
   (`remove.go:398-404`). `internal/install` already imports `internal/shellenv`
   (`remove.go:13`), so this needs no new edge.

Callers warn once per tool when `missing` is non-empty, naming the tool, the version, and
the remedy — not once per path.

### The three `ActiveVersion` write sites

| Site | Action |
|---|---|
| `Manager.InstallWithOptions` (`manager.go:260`) | **No change.** Install re-runs the post-install phase and writes its own files; it is already correct. This is why the fix has two insertion points, not three. |
| `Manager.Activate` (`manager.go:463`) | Call `RestoreShellFiles(toolState, version)` after `createSymlinksForBinaries` (`manager.go:454`) and before the `UpdateTool` at `:459`. Ordering matters: on a restore error, `ActiveVersion` has not yet moved, so the user is left in the pre-activate state rather than a state that claims a version whose files were never written. Covers `tsuku activate`, `tsuku rollback` (`manager.go:364` delegates), and auto-apply's failure rollback (`internal/updates/apply.go:173`) — the last of which calls `Manager.Activate` directly from `internal/updates`, so no `cmd`-level driver would reach it. |
| `Manager.RemoveVersion`'s promotion (`remove.go:179`) | Call `RestoreShellFiles(toolState, newActiveVersion)` in the `wasActive && newActiveVersion != ""` branch, after `createSymlinksForBinaries` (`remove.go:202`). This is the reported reproduction. |

Each call site is three lines around one shared helper, so `dupl` at 250 tokens does not
fire.

Two behaviors fall out for free. `Manager.Activate` currently returns nil early when the
version is already active (`manager.go:437-439`) — leave that, since restore is only needed
on a transition. And restore inherently honors `--no-shell-init`: a version installed with
that flag records no `CleanupAction` at all (`shell_init.go:86-89`, and the prototype's
`set_env` does the same), so activating it restores nothing and resurrects nothing. A
plan-replay mechanism would have to solve that separately; Option B gets it by construction.

### What restore does when `Content` is absent

Absent `Content` covers every version installed before this change, plus the four paths the
state research enumerated: `tsuku install --plan` (never calls `RecordCleanup`,
`cmd/tsuku/plan_install.go:141-143`), library installs (`LibraryVersionState` has no field,
`state.go:121-125`), executor-level dependency installs (no state entry at all,
`internal/executor/executor.go:774-919`), and migrated pre-plan entries.

Restore leaves the file untouched, adds the path to `missing`, and still rebuilds the shell
cache. The caller prints one warning:

```
Warning: nvm 0.40.5 has no recorded content for share/shell.d/nvm.bash
         (installed before tsuku recorded shell init content).
         The file on disk may belong to a different version.
         Run: tsuku doctor
```

This is strictly better than today, where the same situation is silent, the file keeps the
wrong version's bytes, and — because `affectedShells[shell] = true` sits after the
`continue` at `remove.go:345-349` — the cache is not even rebuilt.

It is also the honest limit of the mechanism: **Option B cannot be the only mechanism.**
Pre-change versions need something else, and the cheapest something else is the containment
half of Option D — thread the active version's content hashes into the four
`RebuildShellCache` call sites that pass none (`remove.go:400`, `update.go:58`,
`install_deps.go:595`, `plan_install.go:134`), so an unrestorable file is *excluded* from
the cache rather than silently sourced. That converts "wrong exports, silently" into
"no exports, and doctor says why". It is a handful of lines and it is compatible with every
option on the table, not just this one.

### `doctor --fix`

`cmd/tsuku/doctor.go:198-215` already builds the expected-hash map from each tool's active
version and `internal/shellenv/doctor.go:98-110` already reports `HashMismatches`. Option B
makes the repair a two-line branch: for each mismatched path, find the owning tool's active
version and call `RestoreShellFiles`. That closes the non-convergence described in the
exploration, because after restore the on-disk bytes match the recorded hash and
`RebuildShellCache` stops excluding the file.

### Permissions

**shell.d files are `0600` and the directory `0700`** (`internal/actions/shell_init.go:111-117,
165, 227`). **`state.json` is written `0644`** — `internal/install/state.go:251` and `:320`,
and the real file on this machine is `-rw-r--r--`. The `.tmp` staging file is also `0644`.

So B1 as literally described moves deliberately-`0600` content into a world-readable file.
That is a real regression and the design must fix it: change both `os.WriteFile(tmpPath,
data, 0644)` calls to `0600`. Existing state.json files self-heal on the next save, because
the mode comes from the temp file and `os.Rename` carries it. Nothing else reads state.json
— `grep` finds only `cmd/tsuku/doctor.go:159` (a `Stat`, no read) and a string match in
`internal/telemetry/event.go:531` used for error classification, so nothing uploads it.
Tightening to `0600` is safe.

### Secrets

`source_command` output is arbitrary program stdout (`shell_init.go:203-207`, which leaves
`c.Env` unset so the command inherits the ambient environment) and `set_env` values are
arbitrary strings from the recipe. Today those bytes already exist on disk in a `0600` file.
With state.json at `0600`, the *file-permission* exposure is equalized rather than widened.

What is genuinely new is aggregation and portability: state.json is the file a user copies
into a bug report or a support thread, and it becomes the single place where every tool's
shell-init content is concentrated. No recipe in the registry uses `source_command` today
(`recipes/n/nvm.toml` is the only `install_shell_init` consumer and it uses `source_file`),
and registry recipes are public and reviewed, so the concrete risk today is near zero. The
design should still say so in the field comment and in the docs, so that a future
`source_command` recipe author knows the output is persisted.

### `Content` and `ContentHash` cannot disagree

Three enforcement points:

1. **Capture** verifies `contentHashHex(read) == ContentHash` before storing, and stores
   nothing on mismatch. There is no path that writes `Content` without checking.
2. **Restore** verifies the same identity after decompressing, and treats a mismatch as
   "missing" rather than writing bytes that contradict the recorded hash.
3. A new `doctor` check can verify the invariant across all versions, not just the active
   one, since it is now checkable offline for the first time.

The only way to produce a disagreement is to hand-edit state.json, and both runtime ends
detect it.

### Files touched

- `internal/install/state.go` — one field, plus `0644` → `0600` on two `os.WriteFile` calls.
- `internal/install/state_ops.go` — `captureContent`, two lines in `RecordCleanup`.
- `internal/install/restore.go` — new, `RestoreShellFiles` plus gzip helpers.
- `internal/install/manager.go` — three lines in `Activate`.
- `internal/install/remove.go` — three lines in the promotion branch.
- `cmd/tsuku/doctor.go` — the `--fix` repair branch.
- Tests: `internal/install` is `package install` throughout and needs neither `actions` nor
  `executor` for any of this, so unlike the plan-replay and render-capability options the
  tests can live in `internal/install` rather than being exiled to `cmd/tsuku`.

**Golden-plan workflow: not triggered.** No file on the `validate-golden-code.yml` allowlist
is touched.

## Measured cost

Method: a real `state.json` exists on this machine at `/home/dgazineu/.tsuku/state.json`,
2,600,146 bytes, 16 tools, 96 version entries, 80 cleanup actions. `nvm.sh` came from the
actual upstream tarball the recipe downloads
(`https://github.com/nvm-sh/nvm/archive/refs/tags/v0.40.6.tar.gz`, per
`recipes/n/nvm.toml:16`); it is **161,810 bytes**, not the ~100 KB the exploration assumed.
A throwaway benchmark in `internal/install` drove the real `StateManager.Load()` against
synthetic inflations of that file (20 iterations after a warm-up); it has been deleted and
`git status --porcelain` is clean. End-to-end figures are five runs of a locally built
`tsuku list` against each state file with `TSUKU_HOME` pointed at a scratch copy.

**The baseline is the most important measurement, and it is not what the objection assumed.**

| | bytes |
|---|---|
| real `state.json` | 2,600,146 |
| same, with every `plan` field stripped | 41,353 |

**state.json is already 98.4% stored install plans.** One tool (`vercel`, 13 versions of an
npm recipe) accounts for 2,331,241 bytes on its own. `StateManager.Load()` on that file takes
**39.3 ms**, and `tsuku list` end to end takes **42 ms** against **10 ms** for
`tsuku --version`, which touches no state. State loading already dominates the fast commands.
Option B is not proposing to turn a lean file into a blob store; it is proposing to add a
second blob to a file that is already one.

**Incremental cost, measured against that real baseline.** Each nvm version records four
`delete_file` actions: `nvm.{bash,zsh}` (161,810 bytes each) plus the prototype's
`00-env-nvm.{bash,zsh}` (~50 bytes each).

| scenario | state.json bytes | `Load()` | `tsuku list` |
|---|---|---|---|
| baseline + nvm, no content | 2,585,551 | 40.2 ms | 42 ms |
| + 1 version, raw | 2,928,825 | 45.7 ms | 48 ms |
| + 2 versions, raw | 3,302,573 | 48.2 ms | — |
| + 5 versions, raw | 4,304,985 | 60.1 ms | 66 ms |
| + 13 versions, raw | 7,057,315 | 81.2 ms | 86 ms |
| + 40 versions, raw | 16,644,952 | 154.7 ms | — |
| + 5 versions, gzip+base64 | 3,069,931 | 41.4 ms | — |
| + 40 versions, gzip+base64 | 6,357,113 | 62.6 ms | — |

So one nvm version costs **+343 KB and +6 ms** raw, or **+92 KB and ~+1 ms** gzipped. Gzip
takes the five-version case from +24 ms back to +1 ms.

**The 40-version column is not hypothetical.** The real state.json holds 40 version entries
for `niwa` and 13 for `vercel`, because version entries are only removed by an explicit
`tsuku remove <tool>@<version>` (`internal/install/remove.go:175` is the only
`delete(ts.Versions, ...)` in non-test code). `GarbageCollectVersions`
(`internal/updates/gc.go:15-70`) deletes tool *directories* after
`DefaultVersionRetention = 7 * 24 * time.Hour` (`internal/userconfig/userconfig.go:169`) and
never opens state.json. A tool on a weekly release cadence accumulates version entries
without bound, and under Option B each one carries its own copy of the content. Forty
versions of nvm is 16.6 MB and 155 ms raw; gzipped it is 6.4 MB and 63 ms. Gzip does not
change the shape of the curve, only its slope.

**The relative cost on a clean install is much worse than on the fat baseline, and this is
the number that should worry a reviewer.** A synthetic fresh user — five single-version
tools with small plans:

| scenario | state.json bytes | `Load()` |
|---|---|---|
| 5 tools, no nvm | 7,046 | 240 µs |
| + nvm, 1 version, no content | 8,029 | 273 µs |
| + nvm, 1 version, raw | 358,361 | 4.99 ms |
| + nvm, 3 versions, raw | 1,060,587 | 15.4 ms |
| + nvm, 3 versions, gzip+base64 | 288,997 | 2.83 ms |

Installing one tool takes that user's per-command state load from 240 µs to **5.0 ms — a
20x regression**, or 2.8 ms (12x) gzipped. The absolute numbers are small; the shape is
ugly, and it is entirely attributable to one 158 KB shell script.

**The refinement that removes the tax entirely.** `ContentHash` is already a SHA-256 of
exactly the bytes we want to store. Writing the blob to a content-addressed sidecar —
`$TSUKU_HOME/share/rendered/<sha256>`, file `0600`, directory `0700` — and keying it off the
`ContentHash` that state.json *already carries* gives the same per-version restore semantics
with **no schema change at all** and **zero** hot-path cost: state.json stays at 2,585,551
bytes and 40.2 ms. It also fixes the permission question by construction (the blob keeps
`0600` and never enters a `0644` file), makes `Content`/`ContentHash` disagreement
impossible by definition rather than by check, and deduplicates identical content across
versions for free. The costs it adds are a second store to keep consistent with state.json
and a sweep for unreferenced blobs when a version entry is deleted. I would ship this
variant. It is the same mechanism — record each version's rendered bytes and write them
back — with the bytes stored somewhere that is not read on every `tsuku list`.

## Why it wins

**It is the only option with no import-cycle work.** The cycle is real —
`actions -> version -> install`, closed by `internal/version/resolve.go:7` calling the
17-line `install.ValidateRequested` (`internal/install/pin.go:89-106`) — and every other
mechanism needs it severed, or needs a driver outside `internal/install` plus a callback
seam on `Manager` so that `internal/updates`' direct `mgr.Activate` call
(`internal/updates/apply.go:173`) is not missed. Option B needs neither. Restoring bytes is
pure state manipulation in the package that already owns the state, the symlink re-point,
and the cache rebuild.

**Per-version is the shape the problem actually has, and it already exists.**
`CleanupActions` live on `VersionState`, not `ToolState` (`state.go:32`). Switching from
0.40.6 to 0.40.5 reads `Versions["0.40.5"].CleanupActions` and writes exactly what 0.40.5
wrote. Activate and rollback — the two events the brief flags as hardest, because they have
no install to piggyback on — are covered by construction rather than by re-derivation. No
other option gets them for free.

**It is the only option that reproduces the bytes without re-deriving them.** Plan replay,
render capabilities, and recipe regeneration all recompute content and then have to argue
that the recomputation equals what was installed. Two of the three cannot honor that:
`source_command` mode executes a binary out of the tool directory
(`shell_init.go:203-207`), so "render" is `Execute` minus the write, and re-running it during
`tsuku remove` is both a subprocess at an unexpected moment and not guaranteed reproducible.
Recipe regeneration is worse — the registry moves independently of installed tools, so it can
produce content the installed version never had. Stored bytes are the installed version's
bytes, verified against a hash computed at install time.

**It covers all three writers with zero per-action code.** `install_shell_init` in both
modes, the prototype's `set_env` (which routes to shell.d via `DefaultPhase() ->
"post-install"`, unchanged by this design), and `install_completions` — which has the
identical defect in `share/completions/` with no detector at all — all flow through
`RecordCleanup`. Nothing in the design knows what an action is.

**It honors `--no-shell-init` for free**, because a declined version records no cleanup
action and therefore has nothing to restore. Plan-driven mechanisms have to solve this
explicitly, and the state research flagged it as an ambiguity trap.

**It does not arm the golden-plan workflow**, which currently fails on unrelated Homebrew
bottle drift. Options that add a capability interface to `internal/actions/action.go` do.

**It is the most testable.** No executor, no subprocess, no filesystem staging beyond a
`t.TempDir()`, and — because it needs neither `internal/actions` nor `internal/executor` —
the tests live in `internal/install` next to the code, instead of being exiled to
`cmd/tsuku` (the only package where `install`, `actions`, `executor`, and `shellenv` meet,
and which has no unit tests today).

## Self-attack

**"The state-size tax falls on every command, including fast ones."** True, and the numbers
are above: +6 ms per nvm version raw against the real baseline, and a 20x relative
regression (240 µs → 5.0 ms) for a clean install. Gzip reduces it 3.7x. The sidecar variant
removes it entirely. I do not think the inline variant should ship as designed, and I have
said so; the measurement is what makes that a conclusion instead of an opinion. What I will
defend is that the objection is quantitatively weaker than the exploration assumed, because
the baseline is already 2.6 MB and 39 ms of stored plans — the hot path is already paying a
blob tax for a feature nobody questions.

**"A 100 KB script JSON-escaped into a hot-path file is a real regression."** It is 158 KB,
not 100 KB, and it is worse than that because `install_shell_init` writes one copy per
shell, so nvm is 324 KB per version. This objection is correct and I have no answer to it in
the inline variant beyond gzip.

**"Versions predating the change have no content, so B cannot be the only mechanism."**
Conceded without qualification. Every version in the 2.6 MB state.json on this machine would
restore nothing. The degradation is graceful — warn, do not delete, still rebuild the cache
— and it is strictly better than today's silence, but "strictly better than a bug" is not
"fixed". Option B must ship alongside the hash-threading containment described above, and
that containment is not Option B's idea. Anyone comparing options should discount B's
coverage claim accordingly: it fully covers versions installed *after* it lands, and only
improves diagnosis for those installed before.

**"Storing a copy of bytes that already exist on disk is redundant."** This is the sharpest
objection and it is precisely correct for the case that costs all the money. For
`source_file` mode, `Manager.InstallWithOptions` copies `workDir/.install` wholesale into the
versioned tool directory (`manager.go:168-201`), so `tools/nvm-0.40.5/nvm.sh` is byte-identical
to what was written into shell.d. Option B stores a third copy of a file that exists twice
already. For `source_command` output and `set_env` exports the bytes are generated and
genuinely unrecoverable — and those are the cases that cost about fifty bytes each. So the
redundancy objection lands exactly on the 158 KB and misses the cheap cases entirely. The
fix would be to store a *reference* ("copy `nvm.sh` from this version's tool directory")
instead of the bytes, but `RecordCleanup` cannot know a file was a verbatim copy without
being told, and being told means adding a field to `actions.CleanupAction` — back to
`internal/actions/action.go` and the golden-plan workflow. That trade is real and I do not
have a way out of it that keeps B's zero-import-cycle property.

**"Two stores can drift" (against the sidecar variant).** A blob can be deleted while
state.json still references its hash, e.g. by a user cleaning `$TSUKU_HOME`. Restore already
handles this: a missing blob is indistinguishable from absent `Content`, so it takes the
same warn-and-leave-alone path. The reverse — an orphaned blob — wastes disk and needs a
sweep, which is real work the inline variant does not need.

**"Restoring the active version's bytes on `remove` still leaves N-1 versions' content in
state, and nothing ever prunes it."** True and unaddressed. Since `GarbageCollectVersions`
deletes directories without touching state, a tool can hold forty version entries whose
directories are long gone, each carrying 324 KB of content for a version that can never be
activated (`Activate` refuses when the directory is missing, `manager.go:441-445`). A
prune-on-GC pass is the obvious mitigation and it is extra scope that only Option B creates.

**"The `--plan` install path never calls `RecordCleanup` at all."** Correct
(`cmd/tsuku/plan_install.go:141-143` versus `install_deps.go:613`), which means Option B is
inert for every `--plan` install, including the project's own documented sandbox test
command. Folding that gap in is cheap — the two post-install blocks are the same 25 lines,
already drifted apart once — but it is another thing this option needs and does not itself
provide.

**"Gzip in state.json makes the file unreadable to humans."** Partly. A 158 KB single-line
JSON string is not human-readable either, so the loss is small, but users and support flows
do read state.json and an opaque base64 blob is worse than an ugly one. The sidecar variant
avoids this too.

## What would make this the wrong choice

Any one of these:

1. **If `install_shell_init` gains a way to re-read `source_file` from the versioned tool
   directory.** The action hardcodes `ctx.InstallDir` (`shell_init.go:154`) — the deleted
   staging directory — and that is the *only* reason the bytes look unrecoverable. Redirect
   it at `ToolInstallDir` and the 158 KB is recoverable by a `copyFile` with a ten-byte
   reference in state. Every measurement in this document evaporates, and storing the bytes
   becomes an obviously redundant copy of a file already sitting on disk twice. This is the
   real falsifier, it is cheap to do, and it is behavior-preserving during install because
   the two directories are byte-identical. If the decision goes to an option that makes that
   redirection anyway, Option B has no remaining advantage over it except the import cycle.

2. **If `source_command` mode is declared out of scope or removed.** No recipe uses it
   (`recipes/n/nvm.toml` is the only `install_shell_init` consumer and it uses
   `source_file`). Option B's unique claim — reproducing bytes that cannot be re-derived — is
   carried almost entirely by `source_command` and by `set_env`, and `set_env`'s content is
   a fifty-byte export line that a stable-indirection design (`export
   NVM_DIR='$TSUKU_HOME/tools/.active/nvm'`) makes version-independent outright. Take
   `source_command` off the table and B is storing 324 KB per version to avoid a `copyFile`.

3. **If the project decides `state.json` must get smaller rather than larger.** It is 2.6 MB
   today and `tsuku list` spends 42 ms of its 42 ms budget on it. If stored plans are moved
   out of state.json — which the numbers here argue for independently of this decision — then
   the baseline that makes Option B's tax look proportionate disappears, and adding a second
   blob store immediately afterwards would be indefensible.

4. **If the reviewer's bar is "one mechanism, complete coverage."** Option B cannot repair
   any version installed before it lands, and there is no backfill: the bytes for those
   versions exist on disk only for `source_file` recipes, and finding them requires the
   stored plan's params — which is the plan-replay mechanism. An option that repairs
   existing installs on its own is more valuable than an option that only prevents future
   ones, even if it is more code.
