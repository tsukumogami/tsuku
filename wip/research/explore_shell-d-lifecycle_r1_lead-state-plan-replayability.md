# Lead: Is the information needed to re-render a tool's shell.d files actually present in `state.json`, and for which installs?

Short answer: mostly yes on the main install path, with one silent data loss
that breaks naive replay (the step's `phase` tag is dropped on the way into
`state.json`), and four install paths where the data is simply absent.

## Findings

### 1. The state.json schema

All types live in `internal/install/state.go`. Reproduced verbatim:

`internal/install/state.go:17-21`

```go
type CleanupAction struct {
	Action      string `json:"action"`                 // "delete_file", "delete_dir"
	Path        string `json:"path"`                   // relative to $TSUKU_HOME
	ContentHash string `json:"content_hash,omitempty"` // SHA-256 hex digest of file content at install time
}
```

`internal/install/state.go:24-33`

```go
type VersionState struct {
	Requested          string            `json:"requested"`
	Binaries           []string          `json:"binaries,omitempty"`
	BinaryChecksums    map[string]string `json:"binary_checksums,omitempty"`
	InstalledAt        time.Time         `json:"installed_at"`
	Plan               *Plan             `json:"plan,omitempty"`
	AppPath            string            `json:"app_path,omitempty"`
	ApplicationSymlink string            `json:"application_symlink,omitempty"`
	CleanupActions     []CleanupAction   `json:"cleanup_actions,omitempty"`
}
```

`internal/install/state.go:38-67`

```go
type Plan struct {
	FormatVersion int          `json:"format_version"`
	Tool          string       `json:"tool"`
	Version       string       `json:"version"`
	Platform      PlanPlatform `json:"platform"`
	GeneratedAt   time.Time    `json:"generated_at"`
	RecipeSource  string       `json:"recipe_source"`
	Deterministic bool         `json:"deterministic"`
	Steps         []PlanStep   `json:"steps"`
}

type PlanPlatform struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

type PlanStep struct {
	Action        string                 `json:"action"`
	Params        map[string]interface{} `json:"params"`
	Evaluable     bool                   `json:"evaluable"`
	Deterministic bool                   `json:"deterministic"`
	URL           string                 `json:"url,omitempty"`
	Checksum      string                 `json:"checksum,omitempty"`
	Size          int64                  `json:"size,omitempty"`
}
```

Active-version and provenance fields sit on `ToolState`
(`internal/install/state.go:86-118`): `ActiveVersion`, `Versions
map[string]VersionState`, the deprecated `Version`, `Source` ("central" /
"local" / "owner/repo"), `RecipeHash` (SHA256 of the recipe TOML bytes),
`PreviousVersion` (one level, used by `tsuku rollback`), plus the
explicit/hidden/dependency bookkeeping.

Libraries use a separate, much thinner type
(`internal/install/state.go:121-125`):

```go
type LibraryVersionState struct {
	UsedBy    []string          `json:"used_by"`
	Checksums map[string]string `json:"checksums,omitempty"`
	Sonames   []string          `json:"sonames,omitempty"`
}
```

Note what is **not** there: no `Plan`, no `CleanupActions`.

Top level (`internal/install/state.go:142-146`) is
`{installed, libs, llm_usage}`. **There is no state-file schema version
field.** The only version number anywhere is `Plan.FormatVersion`, which
versions the plan, not the state file.

### 2. Yes, `VersionState` really carries the plan — with the write site

Write site, `internal/install/manager.go:251-257`:

```go
ts.Versions[version] = VersionState{
	Requested:       opts.RequestedVersion,
	Binaries:        opts.Binaries,
	BinaryChecksums: binaryChecksums,
	InstalledAt:     time.Now(),
	Plan:            opts.Plan,
}
```

`opts.Plan` is `InstallOptions.Plan *Plan` (`internal/install/manager.go:75`).
This is the **only** place `VersionState.Plan` is ever set outside tests.
Cleanup actions are written separately and later, via `Manager.RecordCleanup`
(called from `cmd/tsuku/install_deps.go:613`) — they are not part of the
`VersionState` literal above, which means a version's `plan` and its
`cleanup_actions` are written by two different calls and can diverge (see
§5.2).

Conversion into storage form is `executor.ToStoragePlan`
(`internal/executor/plan_conversion.go:7-39`); the inverse is
`FromStoragePlan` (`:43-75`).

The plan is genuinely reused today: `getOrGeneratePlanWith` reads it back
through `StateManager.GetCachedPlan` (`internal/install/state_tool.go:142-159`)
and re-executes it after `executor.ValidateCachedPlan`
(`cmd/tsuku/install_deps.go:98-109`). So "replay the stored plan" is an
existing, exercised code path — just not for a single step.

### 3. The round trip is lossy, and the loss lands exactly on our step

`executor.ResolvedStep` has a `Phase` field
(`internal/executor/plan.go:134-136`):

```go
// Phase is the lifecycle phase for this step: "install" (default), "post-install", etc.
// Empty string is treated as "install" for backward compatibility.
Phase string `json:"phase,omitempty"`
```

`install.PlanStep` has **no** `Phase` field, and `ToStoragePlan`
(`internal/executor/plan_conversion.go:14-24`) does not copy it. Five plan-level
fields are also dropped: `Dependencies`, `Verify`, `RecipeType`, `Binaries`
(all present on `InstallationPlan`, `internal/executor/plan.go:31-74`; all
absent from `install.Plan`).

I proved this empirically with a throwaway test in `internal/executor`
(`ToStoragePlan` → `json.Marshal` → `json.Unmarshal` → `FromStoragePlan`,
since removed):

```
stored JSON: {"format_version":5,...,"steps":[{"action":"install_shell_init",
  "params":{"mode":493,"shells":["bash","zsh"],"source_file":"nvm.sh","target":"nvm"},
  "evaluable":false,"deterministic":false}]}
step phase after round trip: "" (StepPhase="install")
RecipeType="" Binaries=[] Verify=<nil> Dependencies=[]
shells param type: []interface {} value [bash zsh]
mode param type: float64 value 493
```

Consequences:

- `StepPhase` (`internal/executor/executor.go:605-610`) maps `""` to
  `"install"`, so `ExecutePhase(plan, "post-install")` on a plan restored from
  state finds **zero** steps and returns nil
  (`internal/executor/executor.go:620-630`). You cannot re-run the post-install
  phase from stored state.
- Worse, `ExecutePlan` skips non-install steps
  (`internal/executor/executor.go:553-556`) — so a restored plan would run
  `install_shell_init` inside the install phase, where `ToolInstallDir` is
  `""` (`internal/executor/executor.go:512`). For `source_command` recipes
  that fails hard: `validateCommandBinary` returns
  `"source_command requires ToolInstallDir to be set in ExecutionContext"`
  (`internal/actions/shell_init.go:242-244`).
- Any re-render has to select the step by **action name**, not by phase.
- No existing test covers the drop.
  `internal/executor/plan_conversion_test.go:219-300` round-trips a plan and
  asserts field-by-field, but never checks `Phase`, `Dependencies`, `Verify`,
  `RecipeType`, or `Binaries` — which is why the loss went unnoticed.

Params themselves survive well enough. They are `map[string]interface{}` on
both sides, so JSON typing shifts (`[]string` → `[]interface{}`, `int` →
`float64`), but the accessors already normalize: `GetInt` handles
`int`/`int64`/`float64` (`internal/actions/util.go:208-225`) and
`GetStringSlice` handles `[]string`/`[]interface{}`
(`internal/actions/util.go:247-270`). For `install_shell_init` specifically the
params are `target`, `source_file` / `source_command`, `shells` — all strings
and string slices, all recovered correctly.

### 4. Which install paths populate the plan

There are exactly two `InstallWithOptions` call sites, and both set `Plan`:

| Path | Entry point | Plan stored? | CleanupActions stored? |
|---|---|---|---|
| `tsuku install <tool>` and everything that wraps it | `cmd/tsuku/install_deps.go:557` | Yes | Yes (`:612-616`) |
| `tsuku install --plan` | `cmd/tsuku/plan_install.go:100` | Yes | **No** |
| Library recipes | `cmd/tsuku/install_lib.go:158` → `Manager.InstallLibrary` | **No** (no field) | **No** (no field) |
| Executor-level dependency installs | `internal/executor/executor.go:774` | **No state entry at all** | **No** |
| Pre-existing installs from older tsuku | `State.migrateToMultiVersion` | **No** | **No** |

`installWithDependencies` (`cmd/tsuku/install_deps.go:203`) is the single funnel
for the normal path. `tsuku update` (`cmd/tsuku/update.go:136,335`), background
auto-apply (`cmd/tsuku/cmd_apply_updates.go:49`), `tsuku run` autoinstall
(`cmd/tsuku/cmd_run.go:156`), `tsuku install --project`
(`cmd/tsuku/install_project.go:230,256`), `tsuku create`
(`cmd/tsuku/create.go:411`) and the distributed/`owner/repo` path all call
`runInstall` / `runInstallWithReporter` into it. So those are all covered.

`install.HiddenInstallOptions()` (`internal/install/hidden.go:11`) is dead code
— referenced only from its own test — so it is not a coverage gap.

### 5. The four gaps, in detail

**5.1 Executor-level dependency installs write nothing.**
`Executor.installSingleDependency` (`internal/executor/executor.go:774-919`)
builds its own throwaway `actions.ExecutionContext`
(`:839-860`), executes **every** step in `dep.Steps` with no phase filter
(`:874-897`, contrast `ExecutePlan`'s `StepPhase(step) != "install"` skip),
copies the result into `$TSUKU_HOME/tools/{name}-{version}/` (`:900-907`), and
returns. It never touches `StateManager`. So a dependency installed this way
has no `Versions` entry, no plan, and no cleanup actions. If such a recipe
carried an `install_shell_init` step, the step would run (with
`ToolInstallDir: ""`, `:843`) and its `CleanupActions` would accumulate on the
local `execCtx`, which is discarded — `Executor.GetCleanupActions` reads
`e.ctx` (`internal/executor/executor.go:356-361`), a different object. The
shell.d file would be written and then be entirely invisible to tsuku.

**5.2 `tsuku install --plan` stores the plan but not the cleanup actions.**
`runPlanBasedInstall` executes the post-install phase and rebuilds shell caches
(`cmd/tsuku/plan_install.go:118-138`) but never calls `mgr.RecordCleanup` —
compare `cmd/tsuku/install_deps.go:612-616`, which does. The result is a
version whose `plan` contains the `install_shell_init` step while its
`cleanup_actions` array is empty: the shell.d file exists on disk, `tsuku
remove` will not delete it, and `tsuku doctor` has no hash for it. This path
also never calls `SetNoShellInit`, so `--plan` installs always write shell.d.

**5.3 Library installs cannot record any of this.**
`LibraryVersionState` has no `Plan` and no `CleanupActions` field, and
`Manager.InstallLibrary` (`internal/install/library.go:46-105`) writes only
checksums and `used_by`. Separately, `cmd/tsuku/install_lib.go` calls
`ExecutePlan` (`:138`) but never `ExecutePhase(..., "post-install")`, so a
library recipe's post-install steps are silently skipped. A library that
declared `install_shell_init` **without** `phase = "post-install"` would run
during the install phase and write the file with no record of it.

**5.4 Migrated entries have neither.**
`State.migrateToMultiVersion` (`internal/install/state_ops.go`, the
`migrateToMultiVersion` body) synthesizes a `VersionState` with only
`Requested: ""`, `Binaries`, and `InstalledAt: timeNow()` (a best-effort
timestamp, not the real install time). `Plan` and `CleanupActions` are nil.
The rest of the code treats a nil plan as "cache miss and regenerate"
(`cmd/tsuku/install_deps.go:98-110`) or falls back to a default
(`migrateSourceTracking`, `internal/install/state_ops.go:172-204`, infers
`Source` from `Plan.RecipeSource` and defaults to `"central"` when the plan is
absent). Nothing errors on a missing plan; it just quietly means less
information. There is no schema-version gate that would let a future
re-render distinguish "installed before plans were stored" from "installed by
a path that doesn't store plans."

One more soft gap: `--no-shell-init`
(`cmd/tsuku/install.go:388` → `exec.SetNoShellInit`,
`cmd/tsuku/install_deps.go:449` → `ctx.NoShellInit`,
`internal/actions/shell_init.go:86-89`) is **not persisted**. The stored plan
still contains the `install_shell_init` step; only the absence of
`cleanup_actions` records the user's opt-out. A re-render driven purely off
the plan would create a file the user explicitly declined — and it cannot
distinguish that case from 5.2/5.4.

### 6. Is the recipe a viable alternative source of truth?

Not reliably.

- **Offline availability is not guaranteed.** Only 19 recipes are compiled into
  the binary (`internal/recipe/embedded.go:13`, `//go:embed recipes/*.toml`;
  `internal/recipe/recipes/` holds 19 files) against 1449 in `recipes/`.
  Everything else comes from the on-disk registry cache or HTTP, and the cache
  is TTL'd (`internal/registry/cache.go:16`, `DefaultCacheTTL = 24 * time.Hour`;
  `internal/recipe/disk_cache.go:177-193`, with a `stale-if-error` window).
  A remove or activate performed offline, days after install, may find nothing.
- **The recipe revision is not pinned.** `ToolState.RecipeHash` is a SHA256 of
  the TOML bytes (`internal/install/state.go:100-102`), and it is written only
  on the distributed (`owner/repo`) path
  (`cmd/tsuku/install_distributed.go:221-227`) plus opportunistically in
  `cmd/tsuku/install.go:287-291` and `cmd/tsuku/install_project.go:245`. A hash
  detects drift; it does not let you recover the original content. Nothing
  stores the recipe bytes.
- **Re-resolving can produce different content.** `Plan.RecipeSource` is a raw
  provider tag — `"registry"`, `"embedded"`, or `"local"`
  (`internal/install/state.go:44-47`). For `"local"`, the recipe was a file
  path that may no longer exist or may have been edited. For `"registry"`, the
  central registry moves independently of installed tools; re-resolving
  `install_shell_init`'s params off a newer recipe risks rendering content that
  never corresponded to the installed version.

The stored plan is strictly better provenance than the recipe: it is the
resolved, per-version snapshot. Its weakness is coverage (§5), not fidelity.

### 7. Execution context an `install_shell_init` re-render needs

`actions.ExecutionContext` has 22 fields
(`internal/actions/action.go:15-52`). Only a handful matter here, because
`install_shell_init` (`internal/actions/shell_init.go:82-236`) reads exactly:
`NoShellInit`, `ToolsDir`, `InstallDir` (source_file mode), `ToolInstallDir`
(source_command mode), and `Reporter`. It does **not** read `Recipe`, `Env`,
`ExecPaths`, `Dependencies`, `Version`, `OS`, or `Arch` — and
`executeSourceCommand` leaves `c.Env` unset (`:203-207`), so the spawned
command inherits the ambient process environment rather than anything the
executor assembled.

Reconstructible at remove/activate time:

| Field | Reconstructible? | How |
|---|---|---|
| `ToolsDir` | Yes | `cfg.ToolsDir`; the action derives `$TSUKU_HOME/share/shell.d` as `filepath.Dir(ctx.ToolsDir)` (`shell_init.go:108-109`) |
| `ToolInstallDir` | Yes | `cfg.ToolDir(name, version)`; the surviving/newly-active version's directory still exists — `Activate` even asserts it (`internal/install/manager.go:442-445`) |
| `Reporter` / `Logger` | Yes | trivially |
| step params (`target`, `shells`, `source_file`, `source_command`) | **Only if the plan is stored** | `VersionState.Plan.Steps[i].Params`, selected by `Action == "install_shell_init"` |
| `Version` / `OS` / `Arch` | Yes | map key, plus `Plan.Platform` (`internal/install/state.go:53-56`) |
| `InstallDir` | **No** | the ephemeral `$TMPDIR/.../.install` staging dir, gone after `exec.Cleanup()` |
| `NoShellInit` | **No** | never persisted (§5.4 tail) |

The `InstallDir` gap is recoverable in practice rather than in principle:
`Manager.InstallWithOptions` copies `workDir/.install` wholesale into the
staging dir and then atomically renames it to `toolDir`
(`internal/install/manager.go:168-201`), so for a given version, any path under
`InstallDir` has an identical twin under `ToolInstallDir`. A re-render can
resolve `source_file` against `cfg.ToolDir(name, version)` and get byte-identical
content — the recipe in the tree today, `recipes/n/nvm.toml`
(`source_file = "nvm.sh"`), works under that substitution. The action as
written cannot do this; it hardcodes `ctx.InstallDir` (`shell_init.go:154`).

### 8. Where the state currently goes stale (confirming the framing)

- `Manager.RemoveVersion` → `executeCleanupActions`
  (`internal/install/remove.go:326-356`): when another version of the same tool
  lists the same `Path`, deletion is skipped with a printed note (`:345-348`).
  The file keeps whatever content the removed version wrote; the surviving
  version's `ContentHash` no longer matches. Nothing re-renders.
- `Manager.Activate` (`internal/install/manager.go:411-470`): updates symlinks
  and `ActiveVersion` only. Never touches shell.d, never rebuilds the shell
  cache. Same for `tsuku rollback` (`cmd/tsuku/cmd_rollback.go`, 83 lines, no
  plan or shell.d reference).
- `RebuildShellCache` is called during remove with **no** hashes
  (`internal/install/remove.go:398-404`, variadic empty), so it happily
  concatenates the stale content. Only `tsuku doctor` passes hashes
  (`cmd/tsuku/doctor.go:196-214`) — and it collects them for the **active
  version only**. When it does, `RebuildShellCache` *excludes* the mismatching
  file from the cache entirely (`internal/shellenv/cache.go:118-130`), so
  `doctor --fix` turns a wrong-content shell hook into a missing one.

### 9. Current real-world exposure

Only one shipped recipe uses the action: `recipes/n/nvm.toml`
(`action = "install_shell_init"`, `source_file = "nvm.sh"`, `target = "nvm"`,
`shells = ["bash", "zsh"]`). It does **not** set `phase = "post-install"`, so
today the step runs during the install phase, not post-install. That is legal
— phase must be set explicitly (`internal/recipe/types.go:537-539`; the plan
generator only propagates a non-empty phase,
`internal/executor/plan_generator.go:264-269`) — and it works because
`source_file` mode reads from `InstallDir`, which is populated during the
install phase. A `source_command` recipe without `phase = "post-install"`
would fail at install time.

## Implications

1. **A plan-driven re-render is feasible on the main install path and only
   there.** For `tsuku install` and everything funnelling through
   `installWithDependencies`, `state.json` has the action name and every param
   `install_shell_init` reads. Combined with `cfg.ToolDir(name, version)` for
   both `source_file` and `source_command` modes, that is enough to regenerate
   the exact content for any version whose directory still exists — which is
   precisely the set of versions a remove-one-of-several or activate needs.
2. **Selection must be by action name, not phase.** The phase tag does not
   survive the trip into `state.json`, so `ExecutePhase(plan, "post-install")`
   on a restored plan is a no-op. Either the re-render filters
   `Steps[i].Action == "install_shell_init"` directly, or `install.PlanStep`
   gains a `phase` field first (a state-schema change with a
   backfill question for every already-installed version).
3. **The action needs a source-directory indirection.** `source_file` mode is
   hardwired to `ctx.InstallDir`, the deleted staging dir. Re-rendering needs
   it to read from the versioned tool dir instead — a behaviour-preserving
   change during install (the two are byte-identical) that makes the action
   usable outside an install.
4. **Four install paths cannot be repaired by any state-driven design.**
   Executor-level dependency installs (no state at all), library installs (no
   fields), `--plan` installs (plan but no cleanup actions), and pre-plan
   migrated entries. A re-render has to detect and degrade gracefully — the
   nil-plan case must be a documented no-op with a clear diagnostic, not a
   crash and not a silent skip.
5. **`--no-shell-init` is an ambiguity trap.** Absent `cleanup_actions` means
   either "user opted out", "installed via `--plan`", or "installed before
   cleanup tracking". Re-rendering off the plan alone would resurrect files a
   user deliberately declined. Any design needs a positive signal, not an
   inference from absence.
6. **`doctor --fix` currently makes the symptom worse.** It is the only caller
   that passes content hashes, and a mismatch excludes the file from the shell
   cache rather than repairing it. If a re-render lands, `doctor --fix` is the
   natural place to invoke it — the detection is already there.

## Surprises

- **`ToStoragePlan` drops five fields and no test notices.** `Phase`,
  `Dependencies`, `Verify`, `RecipeType`, and `Binaries` are all silently lost
  on the way into `state.json`. The existing round-trip test
  (`internal/executor/plan_conversion_test.go:219-300`) asserts field-by-field
  and happens to omit every one of them. This is not only a shell.d problem:
  the *plan cache* reads these plans back and re-executes them
  (`cmd/tsuku/install_deps.go:98-109`), meaning a cache hit today executes a
  plan whose nested `Dependencies` and `Verify` are gone. That looks like an
  independent latent bug worth its own issue.
- **`Executor.installSingleDependency` writes nothing to `state.json`.** I
  expected dependency installs to be a *lossy* state path; they are a
  *nonexistent* one. Tools land in `$TSUKU_HOME/tools/` with no `Versions`
  entry whatsoever.
- **`tsuku install --plan` records the plan but forgets the cleanup actions.**
  The two blocks are near-identical between `plan_install.go` and
  `install_deps.go` except that one calls `RecordCleanup` and the other does
  not. Shell.d files written by `--plan` installs are permanently orphaned:
  never deleted on remove, never hash-checked by doctor.
- **`nvm` does not use `phase = "post-install"`.** The lifecycle-hooks design
  and PRD both describe `install_shell_init` as a post-install step, but the
  only recipe that uses it omits the phase and runs in the install phase. The
  post-install machinery is, in practice, exercised by no shipped recipe.
- **There is no state-file schema version.** Migrations are detected
  structurally (`Version != "" && ActiveVersion == ""`). Adding a field that
  older versions must backfill has no version gate to hang off.
- **Content re-derivation from the tool dir is more available than expected.**
  Because `.install` is copied verbatim into the versioned tool directory,
  `source_file` content is still on disk for every installed version. The
  blocker for `source_file` recipes is purely that the action looks in the
  wrong place, not that the bytes are gone.

## Open Questions

1. Should `install.PlanStep` gain a `phase` field, or should re-render select
   by action name? The former fixes the plan-cache bug too but needs a backfill
   story for versions already in `state.json`.
2. Is re-rendering `source_command` at remove/activate time acceptable? It
   executes a tool binary at a moment the user asked to *remove* something, and
   the output may differ from install time if the command reads ambient
   environment. `validateCommandBinary` requires the binary to still exist —
   fine for the surviving version, but what about a command that shells out?
3. How should a re-render signal "this version predates plan storage"? A
   warning on every remove would be noise for the many tools that have no
   shell.d files at all; the check needs to be conditional on the tool actually
   owning a shell.d path.
4. Should `--no-shell-init` be persisted (e.g. a `VersionState` bool) so
   re-render can honour it, or is the absence of `cleanup_actions` a good
   enough proxy once `--plan` is fixed to record them?
5. Are the `--plan`, library, and executor-dependency gaps in scope for this
   work, or separate issues? They are pre-existing bugs that a re-render design
   surfaces rather than causes.
6. Does anything write shell.d besides `install_shell_init`? I only traced this
   one action; `install_completions` is mentioned in
   `docs/designs/current/DESIGN-tool-lifecycle-hooks.md:320` but I did not
   verify whether it exists and whether it shares the same lifecycle problem.

## Summary

The plan needed to re-render a shell.d file is genuinely stored in `state.json`
for `tsuku install` and every command that funnels through it, and the params
`install_shell_init` reads survive the JSON round trip intact — but the step's
`phase` tag does not, so replay must select by action name and cannot use
`ExecutePhase`. Four paths have no usable data at all: executor-level
dependency installs write nothing to state, library installs have no fields for
a plan or cleanup actions, `tsuku install --plan` stores the plan but never
records cleanup actions, and pre-plan installs migrate to a nil plan — a
re-render must degrade gracefully on all four rather than assume the plan is
there. The biggest open question is whether to add a `phase` field to
`install.PlanStep` (which would also fix a latent plan-cache bug where cached
plans re-execute with their `Dependencies` and `Verify` silently dropped) or to
route around the loss by filtering on action name.
