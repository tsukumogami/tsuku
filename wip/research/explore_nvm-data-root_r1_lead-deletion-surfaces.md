# Lead: Enumerate every code path in tsuku that can delete something under `$TSUKU_HOME`

All paths below are relative to the tsuku monorepo root. Line numbers are from the
`nvm-data-root` worktree at the time of investigation.

## Findings

### 1. What actually lives under `$TSUKU_HOME`

From `internal/config/config.go:318-370` (the `Config` struct and `DefaultConfig`):

| Path | Field | Created by `EnsureDirectories`? |
|---|---|---|
| `$TSUKU_HOME/tools` | `ToolsDir` | yes |
| `$TSUKU_HOME/tools/current` | `CurrentDir` | yes |
| `$TSUKU_HOME/recipes` | `RecipesDir` | yes |
| `$TSUKU_HOME/registry` | `RegistryDir` | yes |
| `$TSUKU_HOME/libs` | `LibsDir` | yes |
| `$TSUKU_HOME/apps` | `AppsDir` | yes (macOS `.app` bundles) |
| `$TSUKU_HOME/cache` | `CacheDir` | yes |
| `$TSUKU_HOME/cache/versions` | `VersionCacheDir` | yes |
| `$TSUKU_HOME/cache/downloads` | `DownloadCacheDir` | yes |
| `$TSUKU_HOME/cache/cargo-registry` | `CargoRegistryCacheDir` | yes |
| `$TSUKU_HOME/cache/keys` | `KeyCacheDir` | yes |
| `$TSUKU_HOME/cache/taps` | `TapCacheDir` | yes |
| `$TSUKU_HOME/share` | `ShareDir` | yes (conditionally, `config.go:392`) |
| `$TSUKU_HOME/share/hooks` | — | yes (`config.go:393`) |
| `$TSUKU_HOME/config.toml` | `ConfigFile` | n/a |

Not in the struct but created at runtime:

- `$TSUKU_HOME/bin` — shim directory (`internal/shim/manager.go:44`), and the first
  entry on `PATH` per `config.go:448`.
- `$TSUKU_HOME/share/shell.d/` — shell fragments plus `.init-cache.{bash,zsh}` and
  `.lock` (`internal/shellenv/cache.go:31`, `internal/install/shelld.go:11`
  `shellDPrefix = "share/shell.d/"`).
- `$TSUKU_HOME/share/completions/{shell}/` — completion files
  (`internal/actions/completions.go:109`).
- `$TSUKU_HOME/cache/updates/` — update-check entries (`internal/updates/cache.go:179-181`).
- `$TSUKU_HOME/cache/distributed/` — distributed-registry cache
  (`cmd/tsuku/main.go:132`, `cmd/tsuku/install_distributed.go:165`).
- `$TSUKU_HOME/cache/binary-index.db` (`config.go:432`).
- `$TSUKU_HOME/notices/` (`internal/notices/notices.go:239`).
- `$TSUKU_HOME/state.json`, `state.json.lock` (`internal/install/state.go`,
  `internal/updates/apply.go:109`).
- `$TSUKU_HOME/env`, `$TSUKU_HOME/env.local` (`config.go:436-440`, `config.go:519`).
- `$TSUKU_HOME/addons/tsuku-llm` (legacy; see site 12 below).

**`share/` exists today and holds `hooks/`, `shell.d/`, and `completions/`.**
Nothing garbage-collects `share/`. The only code that deletes anything under it is
`executeSingleCleanup` acting on a *specific recorded path* (site 5) and
`RebuildShellCache` deleting its own `.init-cache.<shell>` when no fragments remain
(`internal/shellenv/cache.go:104`). No walker enumerates `share/` and prunes by age,
by prefix, or by "not referenced in state". That is the single most important fact
for this exploration.

### 2. Deletion sites

Sites are numbered; the table at the end summarizes.

**Site 1 — `internal/updates/gc.go:82`, `GarbageCollectVersions`.**
Path construction:

```go
prefix := toolName + "-"                    // gc.go:34
activeDir := toolName + "-" + activeVersion // gc.go:35
previousDir := toolName + "-" + previousVersion
...
if !strings.HasPrefix(name, prefix) { continue }   // gc.go:45
if name == activeDir { continue }                  // gc.go:50
if previousVersion != "" && name == previousDir { continue }
age := now.Sub(info.ModTime()); if age < retention { continue }  // gc.go:65-68
dirPath := toolsDir + "/" + name                   // gc.go:81
os.RemoveAll(dirPath)                              // gc.go:82
```

The claims in the brief are both confirmed:
`internal/userconfig/userconfig.go:169` sets `DefaultVersionRetention = 7 * 24 * time.Hour`,
returned by `UpdatesVersionRetention()` at `userconfig.go:450-459`; and the only
protections are the active version and the previous version.

**It is not restricted to `<name>-<version>`.** It is restricted to *string prefix*
`<name>-` inside `$TSUKU_HOME/tools`. Before deleting, it calls
`reaper.ReapVersion(toolName, version)` with `version := strings.TrimPrefix(name, prefix)`
(`gc.go:72-77`), but `ReapVersion` returns `nil` (a silent no-op) for any version not
in that tool's state (`internal/install/remove.go:395-398`), so the "second lock on the
door" does not stop it. See Surprises.

**Who calls it, and does it run unattended?** Exactly one caller:
`internal/updates/apply.go:153`, inside `MaybeAutoApply`, after each successful
auto-applied update. `MaybeAutoApply` runs from `cmd/tsuku/cmd_apply_updates.go:57`
(`tsuku apply-updates`), which is spawned **detached, with no stdio, from any tsuku
command** by `updates.MaybeSpawnAutoApply` (`cmd/tsuku/main.go:77` →
`internal/updates/trigger.go:126-129`, `spawnDetached` at `trigger.go:64-70`).
Auto-apply defaults to **enabled** (`userconfig.go:396-398`: nil config → `true`),
suppressed only under `CI=true` (`userconfig.go:393`). So yes: fully unattended,
on by default, invisible.

`tsuku update <tool>` does **not** GC — `cmd/tsuku/update.go` never calls
`GarbageCollectVersions`. Neither does `tsuku install`. GC is exclusively an
auto-apply side effect.

**Site 2 — `internal/install/remove.go:151`, `RemoveVersion`.**
`toolDir := m.config.ToolDir(name, version)` → `$TSUKU_HOME/tools/<name>-<version>`
(`config.go:406-408`). Always exactly that; `version` is validated against path
traversal by `ValidateVersionString` (`remove.go:123`) and must exist in
`toolState.Versions` (`remove.go:137-140`). Trigger: `tsuku remove <tool>@<version>`
(`cmd/tsuku/remove.go:86`).

**Site 3 — `internal/install/remove.go:258`, `RemoveAllVersions`.**
Loops `for version := range toolState.Versions` and removes each `ToolDir(name, version)`.
Trigger: `tsuku remove <tool>` with no `@version` (`cmd/tsuku/remove.go:96`), and
recursively for orphaned dependencies via `cleanupOrphans` (`cmd/tsuku/remove.go:160`).

**Site 4 — `internal/install/remove.go:50`, deprecated `Manager.Remove`.**
Removes only the active version's `ToolDir`. **Dead code** — no non-test caller;
`cleanupOrphans` explicitly uses `RemoveAllVersions` instead (`remove.go:158-160` comment).

**Site 5 — `internal/install/remove.go:417-434`, `executeSingleCleanup`.**
This is the only deletion in the codebase whose path is *data-driven*:

```go
func (m *Manager) executeSingleCleanup(ca CleanupAction) {
	absPath := filepath.Join(m.config.HomeDir, ca.Path)   // remove.go:418
	switch ca.Action {
	case "delete_file": err = os.Remove(absPath)          // remove.go:423
	case "delete_dir":  err = os.RemoveAll(absPath)       // remove.go:425
	...
```

`ca.Path` is a `$TSUKU_HOME`-relative string read out of `state.json`
(`internal/install/state.go:15-19, 32`). There is **no containment check, no
allowlist, no prefix restriction** on `ca.Path` at this call site — the only thing
keeping it honest is that the paths are written by tsuku's own actions
(`internal/actions/shell_init.go:205,237` and `internal/actions/completions.go:110-111`,
both `share/...`). Four callers:

- `executeCleanupActions` from `RemoveVersion` (`remove.go:147`) — skips any path
  another *installed version of the same tool* still records (`remove.go:345-368`).
- `RemoveAllVersions` (`remove.go:248`) — runs every version's actions with
  **no cross-version guard at all** ("when removing all versions, no cross-version
  safety check is needed -- everything goes", `remove.go:243-244`).
- `ReapVersion` (`remove.go:400`), reached **from garbage collection** — same
  same-tool cross-version guard.
- `ExecuteStaleCleanup` from `tsuku update` (`internal/install/update.go:68`,
  called at `cmd/tsuku/update.go:183`) — guarded by `recordedPaths()`
  (`update.go:83-97`), the set of cleanup paths recorded by **every version of every
  installed tool**. This is the strongest guard in the codebase.

Only `delete_file` is ever produced today. `delete_dir` is implemented and reachable
but **no action emits it** (grep for `"delete_dir"` returns only the switch case and
two doc comments). The directory-cleanup machinery already exists, unused.

**Site 6 — `internal/install/remove.go:157` and `:291`, app bundles.**
`os.RemoveAll(versionState.AppPath)` (`$TSUKU_HOME/apps/<name>-<version>.app`,
`config.go:426-428`) and `os.Remove(vs.ApplicationSymlink)` (`~/Applications/...`).
Paths come from state, not recomputed.

**Site 7 — `internal/install/manager.go:157, 172, 183, 192, 199, 217`, install path.**
Three distinct removals:
- `os.RemoveAll(stagingDir)` — `$TSUKU_HOME/tools/.<name>-<version>.staging`
  (`manager.go:399`), cleaned before and after the copy.
- `os.RemoveAll(toolDir)` at `manager.go:183`: *"If the final directory already
  exists (e.g., reinstalling same version), remove it before the atomic rename."*
- `os.RemoveAll(toolDir)` at `manager.go:217`: rollback when symlink/wrapper
  creation fails.

Line 183 matters for nvm independently of GC: reinstalling the **same** version of
nvm (`tsuku install nvm@0.40.3` twice, or a repair/reinstall) blows away
`$TSUKU_HOME/tools/nvm-0.40.3` wholesale, taking `NVM_DIR` with it. No retention
period, no prompt, no protection.

**Site 8 — `internal/registry/registry.go:368`, `Registry.ClearCache`.**
`os.RemoveAll(r.CacheDir)` = `$TSUKU_HOME/registry`, then recreates it. No non-test
caller found (`cmd/tsuku/update_registry.go:218` calls a different, in-memory
`loader.ClearCache()`).

**Site 9 — `internal/registry/cache_manager.go:203-206` and `internal/registry/cache.go:156`.**
Per-entry `os.Remove` of `<registry>/<name>.toml` and `.meta`. Trigger:
`tsuku cache cleanup` (`cmd/tsuku/cache_cleanup.go:116` → `CleanupWithDetails`,
default 30-day age; `--force-limit` does LRU eviction to a size cap).

**Site 10 — `internal/distributed/cache.go:293`.**
`os.RemoveAll(oldestPath)` under `$TSUKU_HOME/cache/distributed/<owner>/<repo>` —
LRU eviction of the oldest cached distributed registry.

**Site 11 — `internal/version/tap_cache.go:183`.**
`os.RemoveAll(c.dir)` = `$TSUKU_HOME/cache/taps`.

**Site 12 — `internal/llm/addon/manager.go:283-299`, `cleanupLegacyPath`.**
```go
legacyPaths := []string{
	filepath.Join(m.homeDir, "addons", "tsuku-llm"),
	filepath.Join(m.homeDir, "tools", "tsuku-llm"), // old non-versioned layout
}
```
Deletes them if they are directories. **This is existing precedent that an
unversioned directory sitting directly under `$TSUKU_HOME/tools/` is treated as
legacy garbage to be reaped.**

**Site 13 — `internal/shim/manager.go:123`.** `os.Remove` of `$TSUKU_HOME/bin/<shim>`,
gated on `IsShim(p)` (content match) so it never removes a foreign file.

**Site 14 — `internal/shellenv/cache.go:104, 162, 173`.** Removes only
`share/shell.d/.init-cache.<shell>` (and a temp file). `RebuildShellCache` reads
`share/shell.d`, filters, and writes — it never deletes fragments
(`cache.go:48-96`).

**Site 15 — `internal/notices/notices.go:187`** and **`internal/updates/cache.go:141`** —
per-file removal in `$TSUKU_HOME/notices/` and `$TSUKU_HOME/cache/updates/`.

**Site 16 — outside `$TSUKU_HOME`, listed for completeness.**
`internal/hook/uninstall.go:122,132` (fish `conf.d` files and rc-file marker blocks
in `$HOME`); `internal/executor/executor.go:274,815`, `internal/actions/*.go`
`defer os.RemoveAll(tempDir)`, `internal/sandbox/*`, `internal/validate/*`
(`internal/validate/cleanup.go:69` sets `tempDir: os.TempDir()`, and only removes
directories prefixed `tsuku-validate-`), `internal/testutil/testutil.go:19`. None of
these touch `$TSUKU_HOME`.

### 3. Deletion-site table

Candidate data roots evaluated: **A** = `$TSUKU_HOME/tools/nvm-data` (or any
unversioned dir under `tools/`), **B** = `$TSUKU_HOME/share/nvm`,
**C** = `$TSUKU_HOME/data/nvm` or `$TSUKU_HOME/state/nvm` (new top-level dir).

| # | Site | Path computed | Trigger | Protections today | Eats A? | Eats B? | Eats C? | Minimal fix |
|---|---|---|---|---|---|---|---|---|
| 1 | `updates/gc.go:82` | `$TSUKU_HOME/tools/<any dir with prefix "nvm-">` | auto-apply success, detached bg process, on by default | active dir, previous dir, mtime < 7d, (no-op) ReapVersion | **YES** (`nvm-data` has prefix `nvm-`) | no | no | Put the data dir outside `tools/`. If A is chosen: add a name allowlist derived from `toolState.Versions` instead of prefix matching |
| 2 | `install/remove.go:151` | `config.ToolDir(name, version)` | `tsuku remove nvm@X` | version validated + must be in state | no | no | no | none needed (but see "remove semantics") |
| 3 | `install/remove.go:258` | `config.ToolDir(name, version)` per version | `tsuku remove nvm`, orphan cleanup | none beyond state enumeration | no | no | no | none needed |
| 4 | `install/remove.go:50` | `config.ToolDir(name, activeVersion)` | none (dead code) | none | no | no | no | none |
| 5 | `install/remove.go:425` | `filepath.Join(HomeDir, ca.Path)` — **arbitrary** | remove, remove-all, **GC via ReapVersion**, `tsuku update` stale cleanup | same-tool cross-version skip; `recordedPaths()` (update path only); none in remove-all | **only if recorded** | **only if recorded** | **only if recorded** | If we want `tsuku remove` to delete the data dir, this is the lever (emit `delete_dir share/nvm`). If we want it preserved, emit nothing — but then nothing ever cleans it |
| 6 | `install/remove.go:157,291` | `versionState.AppPath` (macOS) | remove | state-sourced | no | no | no | none |
| 7 | `install/manager.go:183` | `config.ToolDir(name, version)` | **any install of an already-present version** | none | no | no | no | none needed; this is the argument *against* keeping data inside `install_dir` |
| 8 | `registry/registry.go:368` | `$TSUKU_HOME/registry` | no live caller | n/a | no | no | no | none |
| 9 | `registry/cache_manager.go:203` | `$TSUKU_HOME/registry/<name>.toml` | `tsuku cache cleanup` | 30d age / size cap | no | no | no | none |
| 10 | `distributed/cache.go:293` | `$TSUKU_HOME/cache/distributed/<o>/<r>` | LRU on cache write | oldest-only | no | no | no | none |
| 11 | `version/tap_cache.go:183` | `$TSUKU_HOME/cache/taps` | tap cache clear | n/a | no | no | no | none |
| 12 | `llm/addon/manager.go:297` | `$TSUKU_HOME/tools/tsuku-llm`, `$TSUKU_HOME/addons/tsuku-llm` | addon manager startup | exact-name list, must be a dir | no (name-specific) | no | no | none — but it is precedent that unversioned `tools/<name>` is "legacy" |
| 13 | `shim/manager.go:123` | `$TSUKU_HOME/bin/<shim>` | `tsuku shim uninstall` | `IsShim()` content check | no | no | no | none |
| 14 | `shellenv/cache.go:104` | `share/shell.d/.init-cache.<shell>` | cache rebuild with 0 fragments | filename-exact | no | no | no | none |
| 15 | `notices/notices.go:187`, `updates/cache.go:141` | `$TSUKU_HOME/notices/*`, `cache/updates/*` | notice/entry consumption | filename-exact | no | no | no | none |

### 4. Does `tsuku remove` have any notion of user data that outlives the tool?

**No.** `cmd/tsuku/remove.go` has exactly one flag, `--force` (`remove.go:183`), and it
guards a completely different concern: whether other tools declare this one in
`RequiredBy` (`remove.go:58-66`). There is no `--purge`, no `--keep-data`, no prompt,
no `data` concept anywhere in `internal/install/`. `RemoveAllVersions` runs every
recorded cleanup action unconditionally (`remove.go:243-253`) and then
`removeToolEntirely` (`remove.go:274-311`), which drops symlinks and the state entry.
The word "data" does not appear as a concept in the state schema
(`internal/install/state.go`).

**Precedent in the registry:** `nvm` is the **only** recipe in the entire registry
(1449 recipes) that uses `set_env` — `grep -rln "set_env" recipes/` returns
`recipes/n/nvm.toml` alone. It is also one of only two files mentioning
`install_shell_init` (the other is `recipes/README.md`). So there is no second recipe
with this problem to copy from.

The *near*-precedents solve it by not solving it: `pyenv` (`recipes/p/pyenv.toml`),
`rbenv` (`recipes/r/rbenv.toml`), and `rustup` (`recipes/r/rustup.toml`) all install
only their binaries and set **no** environment variables, so their data roots default
to upstream's own choice under `$HOME` (`~/.pyenv`, `~/.rbenv`, `~/.rustup`),
entirely outside `$TSUKU_HOME` and untouched by every site in the table above. That is
the existing de-facto answer: user data lives in `$HOME`, tsuku never manages it.

## Implications

**The GC is not the whole problem, and probably not even the urgent half.**
`internal/install/manager.go:183` deletes an existing `toolDir` before the atomic
rename on *every* install of an already-present version. Any fix that keeps NVM_DIR
inside `{install_dir}` — including "extend the retention period" or "protect more
versions in GC" — still loses the user's Node versions on a same-version reinstall or
repair. The data root has to leave `tools/<name>-<version>` entirely.

**`share/` is the natural home and needs zero new protection code.** It already
exists (`config.go:333, 367, 392-393`), already holds three kinds of durable
non-tool-versioned state (`hooks/`, `shell.d/`, `completions/`), and **no code path
enumerates it for deletion**. A `$TSUKU_HOME/share/nvm` data root would be untouched
by all 15 in-`$TSUKU_HOME` deletion sites as written. `$TSUKU_HOME/data/<tool>` or
`state/<tool>` are equally safe but require adding a new top-level directory to
`Config` and `EnsureDirectories`; `share/` requires nothing.

**Anything under `tools/` is disqualified**, and not marginally so. Site 1 will eat
`tools/nvm-data` by prefix match after 7 days, silently, from a detached background
process. Site 12 shows the project already treats unversioned `tools/<name>` as
legacy garbage. Site 7 would clobber it on reinstall if the name ever collided.

**The `delete_dir` cleanup action is the ready-made lever for `tsuku remove`
semantics.** `internal/install/remove.go:425` already implements it; nothing emits it.
Recording `{action: "delete_dir", path: "share/nvm"}` from the recipe would make
`tsuku remove nvm` delete the user's Node versions and `tsuku remove nvm@X` (with
another version still installed) preserve them, via the existing cross-version guard
at `remove.go:345-368`. Recording nothing preserves the data forever and leaks it.
**Note the trap:** cleanup actions are also executed by `ReapVersion`
(`remove.go:400`), which garbage collection calls (`gc.go:74`). A `delete_dir` on a
stable path is protected there only as long as at least one other installed version
records the same path — so a single-version install whose directory ages past
retention would have its data dir reaped by GC. If `delete_dir` is used, GC's
`ReapVersion` path needs an explicit exclusion, or the recipe needs the data-root
cleanup recorded outside the per-version `CleanupActions` list.

**Whatever is chosen must survive `--no-shell-init`.** `set_env` writes nothing and
records no `CleanupActions` when `ctx.NoShellInit` is set
(`internal/actions/set_env.go:100-104`), so a data-root mechanism built purely on
`set_env` + cleanup actions silently does nothing for those users.

## Surprises

**1. `GarbageCollectVersions` deletes other tools' directories when one recipe name is
a prefix of another.** The filter is a raw `strings.HasPrefix(name, toolName+"-")`
(`gc.go:45`) with no validation that the remainder is a version. `ReapVersion` looks
like a backstop but returns `nil` for a version it does not recognize
(`remove.go:395-398`), so `os.RemoveAll` runs anyway. I verified this empirically with
a throwaway test against the real function (created, run, deleted):

```
GarbageCollectVersions(nil, dir, "git", "2.47.0", "", 7*24*time.Hour, time.Now())
→ DELETED: git-lfs-3.5.0
```

The registry has **59 such name pairs**, including `git`/`git-lfs`, `git`/`git-delta`,
`git`/`git-town`, `docker`/`docker-compose`, `docker`/`docker-buildx`,
`kubectl`/`kubectl-ai`, `helm`/`helm-docs`, `consul`/`consul-template`,
`bat`/`bat-extras`, `argocd`/`argocd-autopilot`, `golangci-lint`/`golangci-lint-langserver`.
Conditions to fire: both tools installed, auto-apply enabled (default), an update
applied for the shorter-named tool, and the longer-named tool's directory mtime older
than 7 days — i.e. any user who installed `git-lfs` a week ago and then had `git`
auto-update. The victim's `state.json` entry survives, so `tsuku list` still shows it
while its `bin/` symlinks dangle. The existing test
`TestGarbageCollectVersions_IgnoresOtherTools` (`gc_test.go:84-96`) uses
`ripgrep` vs `node`, which cannot collide, so the case is untested.

This is a bug in its own right, independent of the nvm issue, and probably deserves
its own issue. It also strengthens the case against putting anything unversioned under
`tools/`.

**2. nvm is the only recipe in 1449 that uses `set_env`.** There is no second data
point in the registry, which means whatever convention this exploration lands on is
effectively defining the convention rather than following one.

**3. `delete_dir` is fully implemented and completely unused.** The
recursive-delete branch at `remove.go:425` has existed with no producer. Anyone adding
a producer inherits an untested path.

**4. GC only ever runs from auto-apply.** `tsuku update`, `tsuku install`, and
`tsuku remove` never garbage-collect. So the 7-day timer is not a property of updating
nvm — it is a property of nvm being auto-updated in the background. A user who only
runs `tsuku update nvm` by hand accumulates every old version forever and never loses
data to GC (though they still lose access to it, because `NVM_DIR` moves).

**5. The deprecated `Manager.Remove` is dead code** (`remove.go:21-68`) — no non-test
caller. Worth deleting.

## Open Questions

- **`share/` vs a new `data/`.** `share/` needs no code change and no new directory,
  but its current contents are all tsuku-generated machinery (hooks, shell fragments,
  completions) rather than irreplaceable user data. Mixing "regenerable" and
  "irreplaceable" under one root may make a future `share/` cleanup routine dangerous.
  A distinct `$TSUKU_HOME/data/<tool>` costs three lines in `config.go` and states the
  intent. Needs a decision.
- **What should `tsuku remove nvm` actually do with the Node versions?** Delete
  (surprising, unrecoverable, gigabytes), keep (leaks forever, no way to reclaim), or
  prompt / require `--purge`. There is no existing flag or prompt machinery in
  `cmd/tsuku/remove.go` to hang this on; `--force` means something else.
- **Migration for existing nvm installs.** Users already have Node versions in
  `$TSUKU_HOME/tools/nvm-<version>/`. Does the new recipe move them, symlink them, or
  abandon them? Nothing in `internal/install/` has a migration hook; the only precedent
  is `cleanupLegacyPath` (`internal/llm/addon/manager.go:283`), which deletes rather
  than migrates.
- **Should the data root be generic or nvm-specific?** A generic mechanism (a
  `{data_dir}` placeholder resolving to `$TSUKU_HOME/share/<tool>`, usable from any
  recipe) versus a one-off path literal in `nvm.toml`. With zero other recipes using
  `set_env`, YAGNI argues for the literal — but the literal gives GC and remove no way
  to know the directory is special.
- **Is the GC prefix bug in scope here, or a separate issue?** It is a live data-loss
  bug affecting 59 recipe pairs and is not caused by anything in #2464.

## Summary

Fifteen code paths delete under `$TSUKU_HOME`, but only one is a risk to a stable
per-tool data directory: `GarbageCollectVersions` (`internal/updates/gc.go:82`), which
prefix-matches `<tool>-` inside `$TSUKU_HOME/tools` and runs unattended from a detached
auto-apply process that is enabled by default — so any data root under `tools/` is
disqualified, while `$TSUKU_HOME/share/<tool>` is untouched by every deletion site as
written and needs no new protection code. The bigger surprise is that the GC is not
even the most urgent hazard: `internal/install/manager.go:183` deletes the whole tool
directory before every atomic rename, so reinstalling the same nvm version destroys
NVM_DIR today with no retention period at all — and the same prefix matching in GC
silently deletes unrelated tools for 59 recipe name pairs like `git`/`git-lfs`, which
I confirmed empirically. The biggest open question is what `tsuku remove nvm` should do
with the user's Node versions: `delete_dir` cleanup actions already exist unused at
`remove.go:425` and would implement "purge", but they are also executed by GC's
`ReapVersion`, so wiring them to a stable path needs an explicit GC exclusion.
