# Lead: How does the shell.d write / record / cleanup path work today, end to end?

All paths are relative to the worktree root
`/home/dgazineu/dev/niwaw/tsuku/tsuku+shell_d_lifecycle-2743e957/public/tsuku/.claude/worktrees/shell-d-lifecycle`.
Line numbers are against branch `docs/shell-d-lifecycle` (based on `origin/main`, tip
`8a7c8908`). PR #2442 (`origin/fix/2439-set-env-exports`) was read as a prototype only and
is called out explicitly wherever it appears.

## Findings

### 1. Producer inventory: what writes into `$TSUKU_HOME/share/shell.d`

I grepped the whole tree (`grep -rn "shell\.d\|shellD" --include="*.go"`, plus `recipes/`,
`website/`, `scripts/`, `docs/`, `Makefile`, `plugins/`, `test/`). On `origin/main` there are
exactly **two** writers of files inside `share/shell.d`, and one of them writes only the cache:

| Writer | File(s) written | Location |
|---|---|---|
| `InstallShellInitAction` | `share/shell.d/{target}.{shell}` | `internal/actions/shell_init.go:160` (source_file), `:226` (source_command) |
| `shellenv.RebuildShellCache` | `share/shell.d/.init-cache.{shell}` (+ `.tmp`, `.lock`) | `internal/shellenv/cache.go:158-166`, lock at `:38` |

Nothing else creates, mutates, or renames a file in that directory. `os.MkdirAll(shellDDir, 0700)`
appears in `internal/actions/shell_init.go:111` and `internal/shellenv/cache.go:33` only.

Near-miss producers that are **not** shell.d, but share the identical `CleanupAction` machinery
and would be missed by a shell.d-only re-render:

- `InstallCompletionsAction` writes `share/completions/{shell}/{target}` (zsh gets an `_` prefix),
  `internal/actions/completions.go:99-104`, `:134`, `:201`. It records cleanup actions with
  content hashes at `internal/actions/completions.go:106-115` — same struct, same state slot,
  same never-re-rendered problem, but `shellFromCleanupPath` returns `""` for those paths so
  they never trigger a cache rebuild. No recipe in `recipes/` currently uses it
  (`grep -rln install_completions recipes/` hits only `recipes/README.md`).
- PR #2442 adds a third shell.d producer: `SetEnvAction` writing
  `share/shell.d/00-env-{recipe-name}.{shell}` for `bash` and `zsh`
  (prototype diff, `internal/actions/set_env.go`, constant `EnvFilePrefix = "00-env-"`).
  On `origin/main` today, `set_env` writes `{install_dir}/env.sh` and nothing reads it — that is
  issue #2439.

Consumers of shell.d, for completeness:

- `internal/config/config.go:446-465` — the managed `$TSUKU_HOME/env` sources
  `share/shell.d/.init-cache.bash` or `.init-cache.zsh` depending on `$BASH_VERSION` /
  `$ZSH_VERSION`. **Only bash and zsh.** Duplicated in `website/install.sh:129-131`.
- `cmd/tsuku/shellenv.go:44-48` — `tsuku shellenv` emits a `. <cache>` line for the shell
  detected from `$SHELL` (bash/zsh/fish, `detectShellForEnv` at `:56-65`).
- `internal/shellenv/doctor.go:52-142` — `CheckShellD`.
- `internal/shellenv/doctor.go:197-209` — `HasShellIntegration(tsukuHome, toolName)` probes
  `{toolName}.bash` / `{toolName}.zsh` by name. This is a second place that hard-codes the
  "filename == tool name" assumption.

### 2. Filenames vs. content: what is version-independent and what is not

`internal/actions/shell_init.go:160` and `:226`:

```go
destPath := filepath.Join(shellDDir, fmt.Sprintf("%s.%s", target, shell))
```

`target` comes straight from `GetString(params, "target")` (`:91`). There is **no** variable
expansion applied to it. `install_shell_init` substitutes `{shell}` and `{install_dir}` by hand,
and only inside `source_command` (`:193-194`); `validateCommandBinary` does its own
`{install_dir}` substitution (`:249`). `set_env` likewise calls `ExpandVars` itself. Params are
otherwise passed to `Execute` verbatim. So a recipe cannot put `{version}` in `target`, and the
filename is structurally version-independent — it is a function of `(recipe's literal target,
shell)` only.

The content is whatever the tool produced at install time:

- `source_file`: a byte-for-byte copy of `{ctx.InstallDir}/{source_file}`
  (`shell_init.go:154`, `copyFile` at `:161`). For `recipes/n/nvm.toml:37` that is `nvm.sh`
  from the 0.40.x source archive — a different file per version.
- `source_command`: stdout of a binary under `ToolInstallDir`
  (`shell_init.go:203-207`), i.e. whatever that version's binary prints. This routinely embeds
  the absolute versioned install path.
- PR #2442's `set_env` file is the extreme case: its entire content is
  `export NVM_DIR='<tools/nvm-0.40.6>'`, a versioned absolute path.

Both files are written `0600`, the directory `0700`, and the directory is re-`chmod`ed on every
install even when it already existed (`shell_init.go:111-117`).

Only one recipe in the tree uses `install_shell_init`: `recipes/n/nvm.toml:35-39`
(`source_file = "nvm.sh"`, `target = "nvm"`, `shells = ["bash", "zsh"]`).

### 3. `RecordCleanup`, `CleanupAction`, and where it lives in `state.json`

The struct is declared twice, once per layer, with identical fields:

- `internal/actions/action.go:54-60` (executor-side, accumulated on `ExecutionContext.CleanupActions`, `action.go:38`)
- `internal/install/state.go:15-21` (state-side, serialized)

```go
type CleanupAction struct {
	Action      string `json:"action"`                 // "delete_file", "delete_dir"
	Path        string `json:"path"`                   // relative to $TSUKU_HOME
	ContentHash string `json:"content_hash,omitempty"` // SHA-256 hex digest of file content at install time
}
```

`cmd/tsuku/install_deps.go:744-755` (`convertCleanupActions`) copies one into the other.

Recording, in order:

1. `recordCleanup` (`internal/actions/shell_init.go:143-150`) appends
   `{Action: "delete_file", Path: "share/shell.d/{target}.{shell}", ContentHash: hash}`.
   Note it is **append-only** — writing the same path twice in one install appends a duplicate.
   PR #2442 adds `upsertCleanup` to fix that for `set_env`.
2. `ContentHash` is `sha256.Sum256` hex of the bytes actually on disk. For `source_file` the file
   is written, `chmod`ed, then **read back** and hashed (`shell_init.go:169-173`) — so the hash is
   of the destination, not the source. For `source_command` it is the hash of the captured stdout
   (`:230`). Helper: `contentHash` at `shell_init.go:136-139`.
3. `Executor.GetCleanupActions()` (`internal/executor/executor.go:356-360`) returns
   `e.ctx.CleanupActions` — everything accumulated across *both* phases, not just post-install.
4. `Manager.RecordCleanup(name, actions)` (`internal/install/state_ops.go:79-94`) resolves
   `ts.ActiveVersion` and does `vs.CleanupActions = actions` — an **overwrite**, on the active
   version only. It no-ops when `actions` is empty or the active version has no `VersionState`.

Storage location: `state.json` → `installed.<tool>.versions.<version>.cleanup_actions`
(`internal/install/state.go:32`, `VersionState.CleanupActions`, `json:"cleanup_actions,omitempty"`).
Per-version, not per-tool. That is the crux: N versions each carry their own copy of the same
version-independent path with a different hash.

**Only one call site records anything.** `grep -rn "RecordCleanup(" --include="*.go" .` (minus
tests) yields exactly `cmd/tsuku/install_deps.go:613`, inside `installWithDependencies` — the path
used by `tsuku install` and by `tsuku update` (via `runInstallWithReporter`). `cmd/tsuku/plan_install.go:125-138`
(`tsuku plan --install`) collects the same cleanup actions and rebuilds the cache but **never calls
`RecordCleanup`**: `plan_install.go:141-143` only sets `IsExplicit`. Files installed that way are
orphans — never deleted on remove, no hash for doctor.

### 4. `Manager.executeCleanupActions` and the `otherPaths` skip

`internal/install/remove.go:326-356`.

```go
otherPaths := make(map[string]bool)
for v, vs := range toolState.Versions {
	if v == version { continue }
	for _, ca := range vs.CleanupActions { otherPaths[ca.Path] = true }
}

affectedShells := make(map[string]bool)
for _, ca := range actions {
	if otherPaths[ca.Path] {
		fmt.Printf("   Cleanup: skipping %s (still referenced by another version)\n", ca.Path)
		continue
	}
	m.executeSingleCleanup(ca)
	if shell := shellFromCleanupPath(ca.Path); shell != "" { affectedShells[shell] = true }
}
```

The set is keyed on `ca.Path` alone — `ContentHash` is not consulted. Since paths are
version-independent, any tool with two installed versions has every shell.d path in `otherPaths`,
so **no shell.d file is ever deleted while a second version exists**. Two consequences:

- The file survives, holding the *removed* version's content.
- `affectedShells` stays empty for those paths, because the `affectedShells[shell] = true` line
  sits after the `continue`. `m.rebuildShellCaches(affectedShells)` at `remove.go:161` therefore
  rebuilds nothing, so `.init-cache.bash` also keeps the removed version's content.

`executeSingleCleanup` (`remove.go:360-377`) joins `m.config.HomeDir` with the relative path and
does `os.Remove` / `os.RemoveAll`; failures print a warning and never block. Unknown action names
warn and return.

`shellFromCleanupPath` (`remove.go:386-395`) requires the literal prefix `share/shell.d/` and
returns the extension minus the dot. `share/completions/...` returns `""`.

`RemoveAllVersions` (`remove.go:233-243`) deliberately skips the cross-version check — it iterates
every version's actions and deletes everything, then rebuilds the union of affected shells.

`RemoveVersion` continues past cleanup: removes the version directory (`:148`), picks
`getMostRecentVersion` by `InstalledAt` as the new active if the removed one was active (`:178`),
and re-points binary symlinks (`:202`). It never touches shell.d again after `:161`.

This is exactly the exploration's reproduction. `tsuku remove nvm@0.40.6` routes to
`RemoveVersion` (`cmd/tsuku/remove.go:86`; `--force` only affects the `RequiredBy` warning at
`remove.go:60-67`, it does not change which method runs).

### 5. `shellenv.RebuildShellCache`

`internal/shellenv/cache.go:29-169`. Signature:
`RebuildShellCache(tsukuHome string, shell string, contentHashes ...map[string]string) error`.

Sequence:

1. `MkdirAll(share/shell.d, 0700)`, then `flock` on `share/shell.d/.lock` (`:33-43`).
2. `os.ReadDir`, keeping entries whose name ends in `.{shell}`, excluding `.init-cache.{shell}`
   and `.lock` (`:62-94`). `Lstat` rejects symlinks and non-regular files with a stderr warning.
   Note it does **not** skip other dotfiles.
3. `sort.Strings(files)` — plain alphabetical, and this ordering is the whole reason PR #2442
   picked the `00-env-` prefix (its diff adds a comment at this line explaining the contract).
4. For each file, if a hash is supplied for `share/shell.d/{name}` and is non-empty, verify it;
   on mismatch print a warning to stderr and **exclude the file**. A file with no stored hash is
   included unverified (`:119-131`).
5. Emit, per file (`:140-148`):

   ```
   # tsuku: <toolname>\n
   { # begin <toolname>\n
   <file contents, newline-terminated>
   } 2>/dev/null || true\n
   ```

   `<toolname>` is `strings.TrimSuffix(name, "."+shell)`. A brace group, not a subshell, so
   functions and exports reach the caller; stderr suppressed and exit status swallowed so one
   tool's failure does not abort the rest.
6. Write `share/shell.d/.init-cache.{shell}.tmp` at `0600` and `os.Rename` over the cache (`:157-166`).
   If no source files exist, or every file was excluded by hash verification, the cache file is
   **removed** (`:98-102`, `:151-155`).

Call sites (`grep -rn "RebuildShellCache(" --include="*.go" .`, minus tests) — five:

| Call site | Passes hashes? |
|---|---|
| `internal/install/remove.go:400` (`rebuildShellCaches`) | no |
| `internal/install/update.go:58` (`ExecuteStaleCleanup`) | no |
| `cmd/tsuku/install_deps.go:595` | no |
| `cmd/tsuku/plan_install.go:134` | no |
| `cmd/tsuku/doctor.go:66` (`--fix` only) | **yes** |

So hash verification is dead code on every path except `tsuku doctor --fix`. Every install and
remove rebuilds the cache from whatever bytes happen to be on disk.

There is no CLI command that rebuilds the cache directly. `--rebuild-cache` is mentioned in
`docs/designs/current/DESIGN-shell-env-integration.md:221` but `grep -rn "rebuild-cache"
--include="*.go"` finds nothing — it does not exist.

### 6. `tsuku doctor`, `CacheStale`, and what `--fix` can actually repair

`cmd/tsuku/doctor.go:194-252` is check 6, "Shell integration".

Hash collection (`doctor.go:197-215`): loads state, and for **each tool's active version only**
(`activeVer := ts.ActiveVersion`, falling back to the legacy `ts.Version`) copies every
`ca.ContentHash != ""` into a flat `map[path]hash`. Non-active versions' hashes are ignored, which
is the correct intent but is exactly what turns a stale shell.d file into a permanent mismatch
report.

`shellenv.CheckShellD(homeDir, contentHashes)` (`internal/shellenv/doctor.go:52-142`) returns
`ShellDCheckResult` (`doctor.go:12-31`):

- `ActiveScripts map[string][]string` — tool names per shell. Built only for `bash` and `zsh`
  (hard-coded loop at `:89`).
- `CacheStale map[string]bool` — computed only for shells that appear in `shellFiles`, i.e. again
  only bash and zsh (`:136-139`).
- `HashMismatches []string` — filenames whose on-disk SHA-256 differs from the stored hash
  (`:98-110`). Computed for *any* filename, including `.fish`.
- `Symlinks []string` — shell.d entries that are symlinks (`:80-83`).
- `SyntaxErrors []ShellSyntaxError` — `bash -n` / `zsh -n` results (`:112-127`, `checkShellSyntax`
  at `:178-195`; silently skipped when the shell is not on `PATH`).

`CacheStale` precisely (`isCacheStale`, `internal/shellenv/doctor.go:146-174`): read
`.init-cache.{shell}`; if unreadable, stale iff any source files exist. Otherwise re-derive the
expected concatenation using the *same* wrapper format as `RebuildShellCache` and compare byte
strings. **`isCacheStale` does not take or consult content hashes.** So "stale" means "the cache
does not equal the concatenation of what is on disk right now", with no notion of whether what is
on disk is correct.

`HasIssues()` (`doctor.go:40-47`) is true if any shell is stale or any of the three lists is
non-empty. Check 6 then prints FAIL and sets `failed = true` (`cmd/tsuku/doctor.go:235-251`).

What `--fix` does (`cmd/tsuku/doctor.go:46-80`): exactly two repairs — rewrite `$TSUKU_HOME/env`
via `cfg.EnsureEnvFile()` when it differs from `config.EnvFileContent`, and call
`RebuildShellCache(homeDir, shell, contentHashes)` for each shell where `CacheStale[shell]` is
true. It does **not** repair hash mismatches, symlinks, or syntax errors, and there is no code
anywhere that regenerates the source file a mismatched hash refers to.

I verified the two interesting states empirically with a scratch test against
`internal/shellenv` (written, run, then deleted):

- **Hash mismatch, cache consistent with disk** (the multi-version-remove state):
  `CacheStale = {bash: false}`, `HashMismatches = [nvm.bash]`, `HasIssues() = true`.
  `doctor` exits non-zero; `--fix` finds no stale shell and does nothing at all. Permanent FAIL.
- **Hash mismatch AND cache stale**: `--fix` calls `RebuildShellCache` with hashes, which excludes
  the mismatched file; with `nvm.bash` the only bash file the buffer ends up empty and the cache
  file is **deleted** (`cache.go:151-155`). The re-check then sees files present and no cache, so
  `CacheStale` is `true` again. `--fix` is not idempotent here and cannot converge — and it has
  removed a cache that was previously loading nvm.

### 7. Is there any existing re-render path?

No. Searching every write to `share/shell.d/*` yields only `shell_init.go` (install time) and
`cache.go` (the cache). The three events that change which version is active all leave shell.d
untouched:

- **`tsuku activate <tool> <version>`** — `cmd/tsuku/activate.go:49` → `Manager.Activate`
  (`internal/install/manager.go:~410-470`). It re-points binary symlinks (`:454`) and updates
  `ActiveVersion`/`PreviousVersion` (`:459-464`). No `shellenv` import, no shell.d reference.
- **`tsuku rollback <tool>`** — `internal/install/manager.go:364` delegates straight to
  `Activate`, so it inherits the same gap.
- **`tsuku remove <tool>@<version>`** with another version present — section 4 above.
- **`tsuku update`** — `cmd/tsuku/update.go:172-192` is the closest thing to lifecycle awareness
  that exists. It snapshots the old version's cleanup actions (`:120-127`), computes
  `StaleCleanupActions(old, new)` (`internal/install/update.go:14-36`, a set difference on
  `(action, path)`), deletes only paths the new version no longer produces
  (`ExecuteStaleCleanup`, `update.go:43-62`), and calls `warnShellInitChanges`
  (`cmd/tsuku/update.go:232-258`) which compares old vs. new `ContentHash` for shared paths and
  emits `"shell init changed for <tool> (<shell>)"`. It *notices* content change and warns; it
  never re-renders. And because update goes through a full install, the new version's write has
  already overwritten the file anyway — the warning is after the fact.

Two related ordering facts worth recording:

- `recipes/n/nvm.toml:35-39` has **no `phase` key**, so `StepPhase` returns `"install"`
  (`internal/executor/executor.go:605-610`) and the step runs inside `ExecutePlan`
  (`executor.go:554` skips anything that is not `"install"`), i.e. **before**
  `mgr.InstallWithOptions` at `cmd/tsuku/install_deps.go:568` and before the tool exists in
  `state.json`. `source_file` is read from the staging `ctx.InstallDir`, which works, but if the
  install fails between the shell.d write and `RecordCleanup` (`:613`) the file is orphaned with
  no state record. A `source_command` variant cannot run in the install phase at all —
  `validateCommandBinary` errors on an empty `ToolInstallDir` (`shell_init.go:242-244`), so it
  requires an explicit `phase = "post-install"`.
- `--no-shell-init` (`cmd/tsuku/install.go:388` → `Executor.SetNoShellInit`,
  `internal/executor/executor.go:321-324` → `ctx.NoShellInit`) makes `install_shell_init` return
  early at `shell_init.go:86-89` writing nothing and recording nothing. A tool installed with that
  flag and then reinstalled without it produces different cleanup-action sets for two versions of
  the same tool — another way the `otherPaths` set can be asymmetric.

## Implications

The exploration's framing holds, and the mechanism is narrower than it might look. Filenames are
purely `(target, shell)`; contents are per-version; `ContentHash` is per-version in
`versions.<v>.cleanup_actions`; and the only writer is `install_shell_init` at install time. A
re-render mechanism therefore needs one thing tsuku does not currently have anywhere: the ability
to reproduce a given version's shell.d bytes *without* running an install. For `source_file` that
is a copy from `tools/<tool>-<version>/<source_file>`, which is cheap and offline. For
`source_command` it means re-executing that version's binary, which is neither offline nor
guaranteed reproducible. Those two cases are not equally solvable.

The re-render trigger set is `activate`, `rollback`, and the "removed one version among several"
branch of `RemoveVersion` — plus, arguably, `RemoveVersion` when it silently skips deletion, since
that path today rebuilds no cache either. `update` is already covered by the reinstall, and
`RemoveAllVersions` already deletes everything.

Any re-render must also rebuild the init cache, because the cache is a byte-for-byte
concatenation and nothing else will notice the change. And it should decide what to do about the
`ContentHash` in state — either re-render to match the recorded hash for the newly active version,
or recompute. Only the first gives doctor anything to verify against.

Two producer-inventory traps for a re-render implementation: `install_completions` uses the same
`CleanupAction` struct and the same per-version state slot but a different directory (and
`shellFromCleanupPath` returns `""` for it), and `cmd/tsuku/plan_install.go` writes shell.d files
that never reach `state.json` at all, so there is nothing to re-render them *from*.

## Surprises

**`doctor --fix` cannot repair the state this bug produces, and in one variant makes it worse.**
Verified empirically. When the cache matches disk but the hash does not match the active version's
record, `--fix` does nothing and `doctor` fails forever. When the cache is *also* stale, `--fix`
rebuilds with hashes, excludes the mismatched file, and — if it is the only file for that shell —
deletes the cache outright, silently disabling nvm in new shells; the re-check still reports stale,
so it never converges.

**Hash verification is effectively off.** Four of the five `RebuildShellCache` call sites pass no
hashes, so the tamper-detection path only runs under `doctor --fix`. Every install and remove
rebuilds the cache from unverified bytes. `DESIGN-shell-env-integration.md:448` states the opposite
intent.

**`RemoveVersion` skipping a cleanup also skips the cache rebuild.** `affectedShells[shell] = true`
is after the `continue` (`remove.go:344-353`), so the reproduction leaves both the shell.d file and
`.init-cache.bash` holding the deleted version's content — the cache is not merely stale, it is
never touched.

**`tsuku plan --install` writes shell.d files it never records.** `cmd/tsuku/plan_install.go:125-138`
rebuilds caches but never calls `RecordCleanup`. Those files leak permanently: `tsuku remove` will
not delete them and `doctor` has no hash for them, so `RebuildShellCache` treats them as legacy and
includes them unverified.

**fish is half-supported.** `install_shell_init` accepts `shells = ["fish"]`
(`shell_init.go:16-20`) and `RebuildShellCache` will happily build `.init-cache.fish`, but
`config.EnvFileContent` only sources the bash and zsh caches (`config.go:451-455`), and
`CheckShellD` only computes `ActiveScripts`/`CacheStale` for bash and zsh
(`internal/shellenv/doctor.go:89`, `:136`). A fish file is hash-checked but its cache is never
staleness-checked, never repaired by `--fix`, and never sourced.

**nvm's `install_shell_init` runs in the install phase, not post-install.** No `phase` key in
`recipes/n/nvm.toml`, so it executes inside `ExecutePlan` against the staging directory, before the
tool is in `state.json`. The doc comments at `cmd/tsuku/install_deps.go:575` and
`plan_install.go:115` ("Execute post-install phase (e.g., install_shell_init)") describe an
arrangement the only real consumer does not use. PR #2442's `set_env` does declare
`DefaultPhase() == "post-install"`, so under that prototype the two shell.d writers for nvm run in
different phases.

**`recordCleanup` is append-only.** `shell_init.go:143-150` appends unconditionally; two steps
writing the same path yield two entries with different hashes, and `doctor`'s flat
`map[path]hash` would keep whichever came last. Not reachable with today's single recipe, but PR
#2442 hits it immediately and adds `upsertCleanup` for it.

## Open Questions

- For `source_command`-based init, is re-running the old version's binary acceptable during
  `activate` / `rollback` / `remove`? It is the only way to reproduce those bytes, it can be slow,
  and it can fail offline. Is caching the rendered bytes in state (rather than just their hash) on
  the table? Note no recipe uses `source_command` today, so this may be deferrable.
- Should the re-render cover `share/completions/` as well, or is a shell.d-only fix acceptable
  given no recipe uses `install_completions` yet? The lifecycle defect is identical.
- What is the intended semantics of `ContentHash` for a re-rendered file — must it match the
  active version's recorded hash exactly (giving doctor a real invariant), or is recomputing on
  re-render acceptable (making the hash a tamper check only against post-render edits)?
- Should `doctor --fix` gain the ability to repair a `HashMismatch` by re-rendering, and should the
  non-convergent `--fix` behaviour (excluding a file, deleting the cache, still reporting stale) be
  fixed as part of this or separately?
- Should `cmd/tsuku/plan_install.go` be made to call `RecordCleanup`? It is a pre-existing leak,
  adjacent but arguably separate.
- Is `getMostRecentVersion`-by-`InstalledAt` (`remove.go:301-320`) the version whose content should
  be re-rendered after a remove, or should it be the version the user would consider "previous"?
  They can differ after a rollback.

## Summary

There is exactly one writer of `$TSUKU_HOME/share/shell.d/*` on main — `install_shell_init`, which
names files `{target}.{shell}` from an unexpanded recipe literal and fills them with that version's
bytes, recording a `delete_file` cleanup action plus a SHA-256 `ContentHash` under
`state.json → installed.<tool>.versions.<v>.cleanup_actions` — and nothing anywhere re-renders an
existing file, so `activate`, `rollback`, and removing one version among several all leave the
wrong version's content on disk with a hash that no longer matches. The gap is worse than a stale
file: `RemoveVersion` records `affectedShells` only for cleanups it actually performed, so the
skipped-because-still-referenced path leaves `.init-cache.{shell}` untouched too, and I confirmed
by test that `doctor --fix` cannot repair the resulting hash mismatch — in the variant where the
cache is also stale it excludes the file, deletes the cache, and still reports stale, never
converging. The biggest open question is `source_command`-based init: re-rendering it requires
re-executing the old version's binary, which is neither offline nor reproducible, so a re-render
design has to decide whether to cache rendered bytes in state or accept that one producer shape
cannot be repaired.
