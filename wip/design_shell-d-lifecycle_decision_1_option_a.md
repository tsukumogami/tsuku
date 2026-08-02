# Option A: give the shell.d-writing actions a render capability and drive it from the three sites that already re-point binary symlinks

The binary layer already solved this problem. `createSymlinksForBinaries` is called
from `InstallWithOptions` (`internal/install/manager.go:212`), `Activate`
(`manager.go:454`) and `RemoveVersion`'s promotion (`remove.go:202`) — provably the
complete set of sites that write `ToolState.ActiveVersion`. Option A adds
`m.syncShellD(...)` next to each of those three calls and gives the actions a way to
produce their bytes without running an install.

## Design

### 1. The capability interface

Four opt-in capabilities already exist in `internal/actions`, all resolved by type
assertion against the value returned by `actions.Get(name)`: `Decomposable`
(`decomposable.go:19`), `Preflight` (`preflight.go`), `ActionDescriber`
(`describer.go:22`), `NetworkValidator` (`action.go:127`). The prototype on
`origin/fix/2439-set-env-exports` adds a fifth, `PhaseDeclarer`. This is a sixth, in a
new file.

New file `internal/actions/renderer.go` (**not** on the golden allowlist):

```go
package actions

// ErrNotRenderable is returned by Render when an action can produce its output
// bytes only by executing something. Callers outside an install treat it as
// "leave this file alone and say so", never as a hard failure.
var ErrNotRenderable = errors.New("action output cannot be reproduced without executing the tool")

// RenderedFile is one file an action produces under $TSUKU_HOME.
type RenderedFile struct {
	RelPath string      // relative to $TSUKU_HOME, e.g. "share/shell.d/nvm.bash"
	Mode    os.FileMode // 0600 for everything shell.d
	Content []byte
}

// RenderContext is the subset of ExecutionContext an action needs to compute its
// output for an already-installed version. It carries no staging directory, no
// resolver, no dependency set and no download cache, because a render happens
// long after those are gone.
type RenderContext struct {
	Context        context.Context
	Tool           string // recipe/tool name; set_env's file basename derives from it
	Version        string
	ToolInstallDir string // $TSUKU_HOME/tools/<tool>-<version>, must exist
	ToolsDir       string // $TSUKU_HOME/tools, used to derive $TSUKU_HOME
	LibsDir        string
	WorkDir        string // "" outside an install; see the {work_dir} note below
	Reporter       progress.Reporter
}

func (c *RenderContext) GetReporter() progress.Reporter { ... }

// ShellFileRenderer is implemented by actions that write files under $TSUKU_HOME
// whose content is version-specific but whose path is not. Render must be a pure
// function of (RenderContext, params) plus the contents of ToolInstallDir: it may
// read the tool's directory, and it may not write anything, execute anything, or
// touch the network.
//
// Render is called both during a normal install (by the action's own Execute) and
// during a lifecycle event that changes which version is active. Those two calls
// must produce the same bytes for the same version.
type ShellFileRenderer interface {
	Render(ctx *RenderContext, params map[string]interface{}) ([]RenderedFile, error)
}

// RenderContextFor builds a RenderContext from an ExecutionContext so Execute and
// the lifecycle driver share one construction site.
func (ctx *ExecutionContext) RenderContext() *RenderContext { ... }
```

Two shared helpers also live here, so both writers stop hand-rolling the same loop
(this is also what keeps `dupl` at 250 tokens quiet):

```go
// WriteRenderedFiles writes files under tsukuHome, creating share/shell.d at 0700,
// writing each file atomically at f.Mode, and recording a delete_file CleanupAction
// with the SHA-256 of what was written. Files sharing a RelPath are concatenated in
// call order, matching set_env's multi-step append.
func WriteRenderedFiles(ctx *ExecutionContext, files []RenderedFile) error
```

`install_completions` (`internal/actions/completions.go`) has the identical defect in
`share/completions/`. It is not required for the reproduction and I am scoping it out,
but the interface is deliberately path-generic (`RelPath` relative to `$TSUKU_HOME`,
not "target plus shell") so adding it later is one `Render` method and no driver
change.

### 2. `install_shell_init`

`internal/actions/shell_init.go` (not on the allowlist). The action splits along the
seam it already has — one mode copies bytes off disk, the other runs a program:

```go
func (a *InstallShellInitAction) Render(rc *RenderContext, params map[string]interface{}) ([]RenderedFile, error) {
	target, shells, err := a.parseParams(params)   // extracted from today's Execute
	if err != nil { return nil, err }
	if _, hasCmd := GetString(params, "source_command"); hasCmd {
		return nil, fmt.Errorf("install_shell_init target %q: %w", target, ErrNotRenderable)
	}
	sourceFile, _ := GetString(params, "source_file")
	return a.renderSourceFile(rc, sourceFile, target, shells)
}

func (a *InstallShellInitAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
	if ctx.NoShellInit { ctx.GetReporter().Log("   Skipping shell init (--no-shell-init)"); return nil }
	target, shells, err := a.parseParams(params)
	if err != nil { return err }
	if cmd, hasCmd := GetString(params, "source_command"); hasCmd {
		return a.executeSourceCommand(ctx, cmd, target, shells)   // unchanged
	}
	sourceFile, _ := GetString(params, "source_file")
	files, err := a.renderSourceFile(ctx.RenderContext(), sourceFile, target, shells)
	if err != nil { return err }
	return WriteRenderedFiles(ctx, files)
}
```

`renderSourceFile` is the single producer for the renderable mode. It reads the source
from a resolved root rather than from `ctx.InstallDir`:

```go
func renderSourceRoot(rc *RenderContext) string {
	if rc.ToolInstallDir != "" { return rc.ToolInstallDir }
	return rc.InstallDir   // RenderContext keeps InstallDir only for this fallback
}
```

**Is that behavior-preserving during a normal install?** Yes, and specifically because
of when the step runs. `recipes/n/nvm.toml` declares no `phase`, so the step executes
inside `ExecutePlan`'s install loop, where `ExecutionContext.ToolInstallDir` is
hardcoded to `""` (`internal/executor/executor.go:512`). The fallback therefore selects
`InstallDir` — today's exact path, byte for byte. If a recipe *does* declare
`phase = "post-install"`, `SetToolInstallDir` has run
(`cmd/tsuku/install_deps.go:578`) and the tool directory is the wholesale copy of
`workDir/.install` that `InstallWithOptions` renamed into place
(`manager.go:168-201`) — the same bytes by construction. One caveat, stated as a caveat
rather than waved away: `fixPipxShebangs` rewrites shebangs in the staging copy before
the rename (`manager.go:180`), so for a pipx-installed tool whose `source_file` is a
venv script the two roots are *not* byte-identical. No recipe does this and none
plausibly will, but it is the one place the substitution is not provably a no-op, and
it is a reason the driver must tolerate a hash that differs from the recorded one
(see §6).

**Does Render execute `source_command`? No.** The consequence during `tsuku remove` is
the whole argument: a removal command spawning a process out of a tool directory in
order to reproduce a config file is not something a user can predict, and it converts
a `remove` that should always succeed into one that can hang, fail offline, or observe
a different ambient environment than the install did. Refusing is not free — such a
tool's shell.d file simply keeps the wrong version's content — so the refusal is paired
with the containment in §7: the driver reports the path as unrenderable, and the
active version's recorded hash is threaded into `RebuildShellCache`, which excludes the
mismatching file. The tool goes from silently wrong to loudly absent, and `tsuku doctor`
already names it.

**Does "no shipped recipe uses `source_command`" change the answer?** It changes the
cost of the refusal to literally zero today, and it is why I am comfortable shipping
with a known uncovered mode. It should not change the *contract*, for two reasons.
The recipe validator accepts the mode, so a recipe can land tomorrow with no signal
that it is lifecycle-unsafe. And once "renderable" is a documented property, the
absence of a `source_command` recipe is a fact worth keeping true. So the design adds
one thing: `Preflight` gains a **warning** (not an error) on `source_command` —
`"install_shell_init: source_command output cannot be re-rendered on activate,
rollback, or remove; prefer source_file"` — which surfaces at recipe-authoring time via
the existing `ValidateAction` path.

### 3. `set_env`

Post-#2442, `SetEnvAction.Execute` already builds the whole file into a
`strings.Builder` before writing, so the split is nearly mechanical:

```go
func (a *SetEnvAction) Render(rc *RenderContext, params map[string]interface{}) ([]RenderedFile, error) {
	envVars, err := a.parseVars(params["vars"])
	...
	vars := GetStandardVars(rc.Version, rc.ToolInstallDir, rc.WorkDir, rc.LibsDir)
	var buf strings.Builder
	for _, v := range envVars {
		value := ExpandVars(v.Value, vars)
		if err := validateEnvValue(v.Name, value); err != nil { return nil, err }
		fmt.Fprintf(&buf, "export %s=%s\n", v.Name, shellQuote(value))
	}
	target := EnvFilePrefix + rc.Tool     // was envTargetName(ctx) off ctx.Recipe
	files := make([]RenderedFile, 0, len(envShells))
	for _, shell := range envShells {
		files = append(files, RenderedFile{
			RelPath: shellDCleanupPath(target, shell), Mode: 0600, Content: []byte(buf.String()),
		})
	}
	return files, nil
}
```

`Execute` becomes `Render` + `WriteRenderedFiles`; the append-across-steps behaviour
`upsertCleanup`/`cleanupRecorded` implements today becomes the RelPath-concatenation
rule inside `WriteRenderedFiles`, which the driver applies identically when it groups
rendered files by path. Two `set_env` steps in one recipe therefore produce the same
bytes on install and on re-render, which is the property the prototype's append logic
was reaching for.

The prototype's `envTargetName` reads `ctx.Recipe.Metadata.Name`. `RenderContext` has
no `Recipe`, so the driver supplies `Tool` from the state key. Those are the same
string on every path that records cleanup actions.

One new `Preflight` **error** falls out of this and is worth having on its own merits:
reject `{work_dir}` in a `set_env` value. `RenderContext.WorkDir` is `""` outside an
install, so a recipe exporting `{work_dir}` would render differently at activate time
than at install time — and the exported value would point at a deleted temp directory
either way.

If Option A lands before #2439's fix, `set_env` still writes `{install_dir}/env.sh`,
implements nothing, and the driver ignores it. Nothing in this design depends on the
merge order.

### 4. Severing the import edge

`internal/install` cannot import `internal/actions` today. The cycle is
`actions -> version -> install`, closed by exactly one edge:
`internal/version/resolve.go:7`. That file uses four symbols from `internal/install`,
all of them from `internal/install/pin.go` — `ValidateRequested`,
`PinLevelFromRequested`, `PinChannel`, `VersionMatchesPin`. The whole 106-line file is
pure `strings`/`unicode` string logic, and **nothing inside `internal/install` uses any
of it** (verified: `grep` finds only `pin.go` and `pin_test.go` in that package).

So the sever is a whole-file move, not an extraction:

1. `git mv internal/install/pin.go internal/pin/pin.go`, `package pin`, drop the
   stuttering prefixes: `PinLevel` → `Level`, `PinLevelFromRequested` →
   `LevelFromRequested`, `PinLatest`/`PinMajor`/`PinMinor`/`PinExact`/`PinChannel` →
   `Latest`/`Major`/`Minor`/`Exact`/`Channel`. `ValidateRequested` and
   `VersionMatchesPin` keep their names.
2. `git mv internal/install/pin_test.go internal/pin/pin_test.go`, same renames.
3. Update the five importers: `internal/version/resolve.go`,
   `internal/updates/apply.go`, `internal/updates/checker.go`,
   `cmd/tsuku/outdated.go`, `cmd/tsuku/update.go`.

`internal/pin` imports only `fmt`, `strings`, `unicode`, so it is a leaf and cannot
re-close any cycle.

**Verified empirically in this worktree**, not reasoned about: I performed the move,
added `internal/install/zz_probe.go` containing
`import _ "github.com/tsukumogami/tsuku/internal/actions"`, and ran `go build ./...`
(clean), `go vet` on install/pin/version (clean) and `go test ./internal/install/ -run
TestPin` (ok). Then reverted. The edge severs and `internal/install -> internal/actions`
becomes legal.

Two consequences worth naming. First, `internal/install -> internal/executor` stays a
cycle (`internal/executor/plan_conversion.go` imports `install`) — Option A does not
need it, because the driver walks `install.Plan.Steps` directly and never constructs an
`executor.InstallationPlan`. Second, and better than expected: once the cycle is gone,
the "tests cannot live in `internal/install`" constraint evaporates. The driver's tests
are ordinary `package install` tests that import `internal/actions`, not `cmd/tsuku`
tests in a package with no test infrastructure at all.

**This is the design's only contact with a golden-allowlisted path.**
`.github/workflows/validate-golden-code.yml:37` lists `internal/version/*.go`, and the
sever edits `internal/version/resolve.go` (four identifier substitutions and one import
line). Nothing else it touches is allowlisted: `internal/actions/renderer.go`,
`shell_init.go`, `set_env.go` and `completions.go` are all absent from the list, and
`action.go`, `decomposable.go`, `internal/executor/plan.go`, `plan_conversion.go` and
`internal/recipe/types.go` are untouched. §"Cost" says what to do about the armed
workflow, and the self-attack gives the fallback that avoids it entirely.

### 5. The driver

New file `internal/install/shelld.go`. One method, called from three places.

```go
// syncShellD makes the shell.d files owned by tool `name` match the content that
// version `activeVersion` produced, and updates the recorded hashes to match.
// It never fails the caller: every problem is a warning, because the caller is
// always in the middle of an install, an activate, or a remove that has already
// committed its state change.
//
// Returns the set of shells whose files changed, so the caller can rebuild those
// caches, and the active version's path->hash map for that rebuild.
func (m *Manager) syncShellD(ctx context.Context, name, activeVersion string) (map[string]bool, map[string]string)
```

Algorithm:

1. Load `ToolState`. Compute `ownedNow` — the `share/shell.d/...` paths in
   `Versions[activeVersion].CleanupActions` — and `ownedEver`, the union over all
   versions. **`ownedNow` is the authorization gate: the driver only ever writes a path
   the newly-active version already recorded at its own install time.** It cannot
   invent a file, and it derives nothing from the plan except content.
2. If `ownedNow` is empty, skip straight to step 5.
3. Stat `cfg.ToolDir(name, activeVersion)`. Missing (a GC'd directory whose state entry
   survives — `internal/updates/gc.go` deletes directories without touching
   `state.json`) means warn and skip to step 5.
4. Render. `vs.Plan` nil means warn once, naming the tool, and skip to step 5.
   Otherwise, in plan-step order:

   ```go
   rc := &actions.RenderContext{ Context: ctx, Tool: name, Version: activeVersion,
       ToolInstallDir: m.config.ToolDir(name, activeVersion),
       ToolsDir: m.config.ToolsDir, LibsDir: m.config.LibsDir,
       Reporter: m.getReporter() }

   rendered := map[string][]byte{}
   for _, step := range vs.Plan.Steps {
       r, ok := actions.Get(step.Action).(actions.ShellFileRenderer)
       if !ok { continue }
       files, err := r.Render(rc, step.Params)
       switch {
       case errors.Is(err, actions.ErrNotRenderable): // warn, record as unrenderable
       case err != nil:                               // warn
       default:
           for _, f := range files { rendered[f.RelPath] = append(rendered[f.RelPath], f.Content...) }
       }
   }
   ```

   Steps are selected by **action name via the registry type assertion**, never by
   `Phase` — `executor.ToStoragePlan` drops `Phase`
   (`internal/executor/plan_conversion.go:14-24`) and `FromStoragePlan` cannot restore
   it, so every step in a stored plan reads as phase `""`. Option A never needs the
   field, which is why it needs no change to `plan_conversion.go` and no `state.json`
   schema change.

   Params survive the JSON round trip: `GetString`, `GetStringSlice` and `GetInt`
   already normalize `[]interface{}` and `float64` (`internal/actions/util.go:208-270`).

5. Apply, under the shell.d file lock (new export
   `shellenv.WithShellDLock(tsukuHome string, fn func() error) error`, wrapping the
   existing unexported `acquireFileLock`; the driver releases it before any
   `RebuildShellCache`, which takes the same lock):
   - for each `p` in `ownedNow` with rendered content: compare to the bytes on disk; if
     identical, do nothing; otherwise write atomically (temp + `os.Rename` in the same
     directory, `0600`) and mark `p`'s shell affected.
   - for each `p` in `ownedEver \ ownedNow`: `os.Remove`, mark affected. This is the
     demotion case — promoting to a version installed with `--no-shell-init` should
     leave no file behind.
   - never touch a path outside `ownedEver`.
6. Rewrite `Versions[activeVersion].CleanupActions[i].ContentHash` for every path
   written, via a new `Manager.setVersionCleanup(name, version string, actions
   []CleanupAction) error` (the existing `RecordCleanup` resolves `ts.ActiveVersion`
   internally, which is wrong here — during `RemoveVersion` the driver runs against an
   explicitly-passed version).
7. Return the affected-shell set and the active version's post-sync path→hash map.

### 6. Hash semantics

**Recompute, and rewrite the record.** The re-rendered content does *not* have to match
the active version's recorded hash.

The argument for requiring a match is that the recorded hash then becomes a real
invariant `doctor` can verify. The argument against is decisive: if the render
disagrees with the record, the alternatives are to write nothing — leaving the *other*
version's content, the exact bug — or to write and refuse to update, leaving `doctor`
in a permanent FAIL it cannot clear. Both are worse than the wrong-hash state they
replace. So the driver writes the rendered bytes (they are derived from the version
that is now active, which is the best available truth), updates the record, and warns:
`"shell init for nvm re-rendered with different content than recorded at install time"`.

This keeps the hash meaningful as a *tamper check between lifecycle events*, which is
what it actually is today, rather than promoting it to a reproducibility proof the
`source_file`/pipx-shebang case cannot honour. Two situations produce a legitimate
mismatch and both deserve the warning rather than a failure: the tool directory was
edited after install, and the byte-identity caveat in §2.

Nothing here changes `CleanupAction`'s shape, `VersionState`'s shape, or `state.json`'s
schema. Option A is the only candidate on the table with that property.

### 7. Ordering, and the `affectedShells` bug

Three call sites, all placed **after** the state write that sets the new
`ActiveVersion`, because the driver takes the version explicitly and because a failure
must not leave state uncommitted:

- `Manager.InstallWithOptions` (`manager.go`), after the `UpdateTool` at `:249-272`.
  During install the post-install phase has already written the files, so `syncShellD`
  is a no-op comparison that rewrites nothing — but it also *deletes the demoted paths
  of the version being replaced*, which is the `--no-shell-init`-on-upgrade case
  `ExecuteStaleCleanup` misses under `update --all` and background auto-apply.
- `Manager.Activate` (`manager.go:459-464`), after the `UpdateTool`. This is the site
  that covers `tsuku activate`, `tsuku rollback` (which is a five-line wrapper around
  `Activate`, `manager.go:364`) and auto-apply's silent failure rollback
  (`internal/updates/apply.go:173`) — three user-visible paths, one insertion point,
  and the reason the driver has to live inside `Manager` rather than in `cmd/tsuku`.
  `Activate` currently does not import `internal/shellenv`; it will now, transitively
  through the driver.
- `Manager.RemoveVersion` (`remove.go:190-205`), immediately after the
  `createSymlinksForBinaries` call in the `wasActive && newActiveVersion != ""` branch.

Then the cache. `RemoveVersion` today calls `m.rebuildShellCaches(affectedShells)` at
`remove.go:161` — **before** the promotion at `:174-205`, so even if the set were
correct the rebuild would run against pre-promotion content. That call moves down,
below `syncShellD`, and unions the two shell sets.

And the skipped-cleanup path has to start reporting. `executeCleanupActions`
(`remove.go:342-352`) puts `affectedShells[shell] = true` *after* the `continue`, so
the case that produces this whole bug — path still referenced by another version —
reports no affected shell and the cache is never touched at all. Two lines:

```go
if otherPaths[ca.Path] {
	fmt.Printf("   Cleanup: skipping %s (still referenced by another version)\n", ca.Path)
	if shell := shellFromCleanupPath(ca.Path); shell != "" {
		affectedShells[shell] = true   // content is about to change under us
	}
	continue
}
```

Finally, `m.rebuildShellCaches` grows a hash argument and passes it through to
`shellenv.RebuildShellCache`, using the map `syncShellD` returned. Four of the five
`RebuildShellCache` call sites pass no hashes today, so the exclusion logic at
`internal/shellenv/cache.go:119-131` is inert everywhere except `doctor --fix`.
Threading it here is what makes the `ErrNotRenderable` fallback degrade loudly instead
of silently: an unrenderable file keeps the wrong content, fails verification, and is
kept out of the cache. That is Option D's cheap half, and Option A needs it — not as a
competitor but as the floor underneath the cases the render cannot reach.

(One wart inherited, not fixed: when hash exclusion empties the buffer,
`RebuildShellCache` deletes the cache file outright (`cache.go:151-155`), and
`isCacheStale` recomputes without hash filtering (`shellenv/doctor.go:146-174`), so
`doctor --fix` never converges. That is a separate bug and should be a separate issue;
Option A makes it reachable on more paths, which is an argument for fixing it in the
same milestone.)

### 8. Files touched

| File | Change |
|---|---|
| `internal/pin/pin.go` | new (moved from `internal/install/pin.go`) |
| `internal/pin/pin_test.go` | moved |
| `internal/install/pin.go` | deleted |
| `internal/version/resolve.go` | import + 4 identifiers — **golden-allowlisted** |
| `internal/updates/apply.go`, `internal/updates/checker.go` | import + identifiers |
| `cmd/tsuku/outdated.go`, `cmd/tsuku/update.go` | import + identifiers |
| `internal/actions/renderer.go` | new: interface, `RenderContext`, `RenderedFile`, `ErrNotRenderable`, `WriteRenderedFiles` |
| `internal/actions/shell_init.go` | `Render`; `Execute` reuses it; `sourceRoot` fallback; `source_command` Preflight warning |
| `internal/actions/set_env.go` | `Render`; `Execute` reuses it; `{work_dir}` Preflight error (post-#2442) |
| `internal/install/shelld.go` | new: the driver |
| `internal/install/manager.go` | two call sites |
| `internal/install/remove.go` | one call site, `affectedShells` fix, cache-rebuild reorder, hash threading |
| `internal/install/state_ops.go` | `setVersionCleanup` |
| `internal/shellenv/lock.go` | new: `WithShellDLock` |
| `cmd/tsuku/plan_install.go` + `install_deps.go` | fold the duplicated post-install block into one helper that calls `RecordCleanup` |

## Why it wins

**It is the only candidate whose trigger set is provable rather than argued.** The
three `ActiveVersion` writers are a `grep` result, not a judgement call, and
`createSymlinksForBinaries` already sits at all three. A reviewer can verify coverage
with one command. Every other option has to reason about *when* to act; A inherits an
answer the codebase already got right for the binary layer.

**It needs no `state.json` change.** No `Phase` field on `PlanStep`, no `Content` blob,
no `NoShellInit` flag, no schema-version gate in a file that has never had one. The
inputs are `VersionState.Plan.Steps[].Params`, which already round-trips correctly, and
`VersionState.CleanupActions`, which already exists per-version. Options B and (in its
plan-replay form) A-via-`ExecutePhase` both require schema work with a backfill story
for every already-installed version; this does not.

**`ownedNow` as the authorization gate closes the `--no-shell-init` hole for free.**
Because the driver may only write paths the active version already recorded, a version
installed with `--no-shell-init` — which writes nothing and records nothing — is
structurally incapable of having a file resurrected. So are `--plan` installs,
migrated pre-plan entries, library installs, and executor-level dependency installs.
The four "no usable plan" paths and the opt-out ambiguity collapse into one no-op rule
that needs no new persisted signal. That was the single sharpest objection to a
plan-driven design and it is answered by construction rather than by a flag.

**Render is genuinely a pure function for the mode that matters.** The objection to a
render capability is that one implementor cannot honour "no side effects". True — and
the design makes that a typed, named refusal (`ErrNotRenderable`) with a containment
path, rather than an unspoken caveat. For `source_file` and for `set_env`, Render is
string and byte manipulation over an existing directory: table-testable in
`internal/actions` with no filesystem for `set_env`, and with a `t.TempDir()` for
`install_shell_init`.

**`Execute` cannot drift from `Render`, because `Execute` is `Render` plus a write.**
That is not aspirational: `set_env` on the prototype already builds its full content in
a buffer before writing, and `install_shell_init`'s `source_file` mode is a copy whose
hash is computed by reading the destination back — replacing "copy then re-read" with
"produce bytes, write bytes, hash bytes" removes a read and a failure mode.

**It composes with the containment rather than competing with it.** Threading the
active version's hashes into `RebuildShellCache` is Option D's whole design; Option A
needs it anyway for the unrenderable case. Shipping A ships D's guarantee too, with the
addition that the common case gets the right content instead of no content.

**It fixes `activate` and `rollback`, which no containment-only option does.** Option D
leaves `tsuku activate nvm 0.40.5` writing nothing to shell.d and — with hashes
threaded — actively excluding nvm from the cache. For a tool that exists only as a
shell function, "activate makes it disappear" is not a smaller bug than "activate does
nothing".

## Self-attack

**"A version installed before plans were stored has a nil `Plan`, and you just warn."**
Stands, and it is the honest limit. `migrateToMultiVersion` synthesizes a `VersionState`
with `Plan: nil`, so promoting to such a version leaves the other version's content on
disk. The mitigations are real but partial: such a version usually also has nil
`CleanupActions`, so `ownedNow` is empty and the driver no-ops silently rather than
warning about a tool with no shell.d at all; and where it does have cleanup actions but
no plan, the hash threading excludes the file from the cache, so the user gets absence
plus a `doctor` line rather than silent wrongness. There is no way to render content for
a version whose install left no description of itself. Any design that claims otherwise
is claiming to recover information that was never written down — except Option B, which
would have had to store the bytes *at install time*, and cannot help these versions
either.

**"The four install paths that record no usable plan are still broken."** Partly
answered, partly conceded. `--plan` installs are the sharp one: `plan_install.go` writes
shell.d files and never calls `RecordCleanup` (`RecordCleanup` has exactly one non-test
caller, `install_deps.go:613`), so those files are outside `ownedEver` entirely —
never deleted on remove, never hash-checked, never re-rendered. I am folding the fix in
(one shared helper for the two copy-pasted post-install blocks, which have already
drifted apart once), which converts `--plan` from unmanaged to managed. Library installs
have no `Plan` or `CleanupActions` *fields* on `LibraryVersionState`, and
`Executor.installSingleDependency` writes no state at all. Both stay broken and both are
pre-existing; neither can write a shell.d file under any shipped recipe today. They
should be named in the PR body as known-uncovered, not quietly omitted.

**"Does `--no-shell-init` get resurrected?"** No, and this is the design's best answer.
The gate is `ownedNow`, not the plan. A `--no-shell-init` install records nothing, so
there is nothing to render. The design goes one step further and *deletes* the previous
version's file when promoting to an opt-out version, which is the behaviour a user who
passed the flag would expect and which nothing does today. The residual gap: a user who
passed `--no-shell-init` on the *older* version and not the newer one, then removes the
newer one, gets the file deleted on promotion — correct, but it is a state change the
user did not ask for during a `remove`. It is announced in the reporter output.

**"A mid-re-render failure leaves the system in a third state."** Bounded, not
eliminated. Each file is written temp+rename in the same directory, so no file is ever
half-written. Across files, a failure after writing `nvm.bash` and before `nvm.zsh`
leaves bash on the new version and zsh on the old — but the recorded hash is updated
only for files actually written, so `doctor` names zsh specifically, and the whole
operation is idempotent: re-running `activate`, or the next lifecycle event, converges.
The genuinely uncomfortable part is that the caller has already committed its state
change by then, because the driver runs after the state write. Running it before would
trade a stale file for a `remove` that fails after deleting a directory. Given that
`executeSingleCleanup` and the post-install phase both already warn-and-continue, the
warning-only policy is the codebase's existing answer to this class, not a new one.

**"Re-rendering during `tsuku remove` is not a removal command's job."** I disagree, and
narrowly. `remove <tool>@<version>` with other versions present is not only a deletion —
it is an *implicit activation*: it assigns `ActiveVersion` (`remove.go:179`) and already
re-points binary symlinks to the promoted version (`remove.go:202`). Declining to
re-render shell.d there means leaving the command in a state where the binaries point at
0.40.5 and the shell function is 0.40.6's. The render reads only the *surviving*
version's directory and never touches the removed one. Where the objection lands
correctly is `source_command`, and that is exactly the case Render refuses. If the
objection were fully right, the right fix would be to make `RemoveVersion` delegate its
promotion to `Activate` rather than duplicate it — which is a good idea independently
and would make this a two-site change instead of three.

**"Severing the import edge loosens the layering permanently."** This one mostly stands.
Once `internal/install` may import `internal/actions`, it may also transitively reach
`recipe`, `registry`, `httputil`, `sonameindex`, `index`, `autoinstall` and `project` —
and every `cmd/tsuku` command and `internal/updates` imports `install`. Nothing stops a
future contributor from reaching for `actions` casually in `remove.go`. Three
mitigations, none of them airtight: the dependency is confined to one new file
(`internal/install/shelld.go`) with a comment saying so; the direction is at least
*correct* (install is a higher layer than actions, and the cycle only existed because a
17-line string validator was in the wrong package); and moving `pin.go` out is a
strictly good change on its own — it does not belong in `install` and `install` never
used it. What I cannot claim is that the sever is invisible. It touches five files
outside the feature and one of them is golden-allowlisted.

**"You said `Render` is pure, then gave it a `Reporter` and let it read the filesystem."**
Fair on the wording. The contract is narrower than "pure" and the interface doc says so
explicitly: Render may read `ToolInstallDir` and may emit warnings; it may not write,
execute, or use the network. `install_shell_init`'s `source_file` mode reads a file, so
"no filesystem" was never on the table. What matters for the driver is that calling
Render during a `remove` cannot change anything, and that holds.

**"Two implementors is a thin justification for a capability interface."** Three
counting `install_completions`, which has the identical defect and which the interface
covers with no driver change. And the alternative to an interface is a `switch
step.Action` in the driver with the byte-production logic duplicated out of each action
— which is the drift the interface exists to prevent. `NetworkValidator` and
`ActionDescriber` both have similarly narrow implementor sets in this codebase.

**"`WriteRenderedFiles` centralizes the write, so a bug there breaks every writer at
once."** True and intentional. The current situation is that `shell_init.go` and the
prototype's `set_env.go` implement mkdir-0700 / chmod / write-0600 / hash / record
separately and have already diverged (`recordCleanup` appends, `upsertCleanup`
replaces). One tested helper is the smaller risk.

**"You are relying on `dupl` not firing across three near-identical `Render`
implementations."** The two `Render` bodies are structurally different (one copies a
file per shell, one formats exports into a buffer) and both are well under 250 tokens
after `WriteRenderedFiles` absorbs the shared tail. The real `dupl` risk is in the
tests, where multi-version scenarios repeat; those must be table-driven subtests.

**"`GarbageCollectVersions` can delete the directory you are about to render from."**
Handled by the stat in step 3, and it is a pre-existing hole this design does not widen:
GC deletes tool directories without touching `state.json`
(`internal/updates/gc.go:15-70`), so a version with no directory keeps its
`CleanupActions` and keeps holding the `otherPaths` skip open. Option A degrades to
warn-and-leave. Any design keyed on "another version still references this path"
inherits the same hole.

## What would make this the wrong choice

Four falsifiers, in descending order of how likely I think they are.

**If arming `validate-golden-code.yml` is a hard blocker.** The sever necessarily edits
`internal/version/resolve.go`, and `internal/version/*.go` is on the allowlist. There is
no way around it: the edge runs *into* `internal/install` from `internal/version`, so
breaking it means editing a file in `internal/version`, and a deletion counts as a
change for a `paths:` filter. If the workflow's current red state on unrelated Homebrew
bottle drift means this PR cannot merge, Option A must fall back to a driver that does
not import `actions` at all: declare `Renderer`/`RenderContext`/`RenderedFile` in a new
leaf package `internal/shellrender` that imports only stdlib, have `internal/actions`'
`init()` register its two implementors into that package's registry, and have
`internal/install` consume `shellrender.Get(step.Action)`. No cycle, no allowlisted
file, no wiring at 20 `install.New` call sites. The cost is a second registry parallel
to `actions.registry`, a design that no longer matches the type-assert-on-`actions.Get`
idiom, and a link-order-dependent silent no-op if any future binary imports `install`
without `actions`. That fallback is strictly uglier, and if it is the only version that
can ship, Option A has lost most of its claim to being the idiomatic choice.

**If `source_command` is going to have real users.** The whole refusal rests on that
mode having zero recipes today. If the project intends `source_command` to be a normal
way to write recipes — it is the shape most tools with a `tool init bash` subcommand
want — then Option A covers the mode that exists and not the mode that is coming, and
the file it cannot render is the file it excludes from the cache. At that point Option B
(store the bytes per version) is the only design that covers both modes uniformly, and
its `state.json`-size objection has to be weighed for real rather than dismissed, since
`source_command` output is typically a few KB, not `nvm.sh`'s 100 KB.

**If the fleet is mostly pre-plan installs.** `ownedNow` plus `vs.Plan` is the whole
input. If a meaningful fraction of installed versions in the wild came from a tsuku old
enough that `migrateToMultiVersion` synthesized their `VersionState`, the driver
no-ops for them and users see containment, not repair. That is measurable — the field
is `Plan == nil` in `state.json` — and if the answer is "most of them", the pragmatic
choice is Option D plus a `doctor --fix` repair, because a repair that runs on demand
can ask the user to reinstall while a lifecycle hook cannot.

**If version-keyed filenames are acceptable.** Option C makes a stale file *unreachable*
rather than *correct*, which is a stronger property than anything a re-render can offer:
there is no window, no failure mode, no dependency on the plan, and no per-version data
at all. Option A is the right choice only if changing `share/shell.d/nvm.bash` to
`share/shell.d/nvm-0.40.6.bash` is unacceptable — because of
`shellenv.HasShellIntegration`'s filename assumption
(`internal/shellenv/doctor.go:197-209`), the prototype's load-bearing `00-env-` sort
prefix, and every user who has hand-referenced a shell.d path. Those are real, but they
are migration costs, not correctness costs. If the project is willing to pay a one-time
migration, C dominates A on the axis that matters most — A is a mechanism that has to
run correctly at three moments, and C is a property that holds without anything running
at all.

## Cost

**Files.** 16 touched: 2 moved, 1 deleted, 4 new (`internal/pin/pin.go`,
`internal/actions/renderer.go`, `internal/install/shelld.go`,
`internal/shellenv/lock.go`), 9 edited. Of the edits, 5 are the mechanical import
rename from the sever.

**Lines.** Order of ~600 non-test lines changed, of which ~230 are the file move
(unchanged logic) and ~250 are genuinely new (the driver ~180, `renderer.go` ~70). Plus
~400 test lines: table-driven `Render` tests in `internal/actions`, and driver tests in
`internal/install` covering install/activate/rollback/remove-of-active,
remove-of-inactive, nil plan, empty `CleanupActions`, missing tool directory,
`ErrNotRenderable`, and the demotion delete.

**CI risk.** **Yes — one golden-allowlisted file.** `internal/version/resolve.go` arms
`validate-golden-code.yml`, which regenerates plans against live upstream and currently
fails on unrelated Homebrew bottle drift on `main`. Expect a red required-looking check
that has nothing to do with this change and must be explained in the PR body or waived.
No other allowlisted path is touched: `internal/actions/action.go`,
`internal/actions/decomposable.go`, `internal/executor/plan.go`,
`internal/executor/plan_conversion.go` and `internal/recipe/types.go` are all untouched,
which is deliberate — the capability lives in a new `renderer.go` precisely to keep
`action.go` clean.

Everything else is quiet. `gofmt`/`go vet`/`golangci-lint` pass on the sever (verified in
this worktree). `errcheck` covers test files and does not exclude `os.WriteFile` or
`os.MkdirAll`, so tests must check them. `dupl` at 250 tokens is the reason
`WriteRenderedFiles` is shared and the reason the driver tests are table-driven. The
unit-test job fails on a dirty `git status --porcelain`, so tests must write only under
`t.TempDir()`. CI passes `-short`, so nothing may be `testing.Short()`-gated. Functional
tests run only `@critical` scenarios on PRs.

**Migration burden.** None for `state.json`: no new fields, no schema version, no
backfill. Recorded `ContentHash` values are rewritten in place the first time a tool sees
a lifecycle event, which is a no-op when the content already matches. `internal/pin` is
an internal package with no external consumers. The one user-visible behaviour change
beyond the fix is the demotion delete (§5, step 5), which removes a shell.d file when
promoting to a version that recorded none — correct, but new, and worth a line in the
release notes.
