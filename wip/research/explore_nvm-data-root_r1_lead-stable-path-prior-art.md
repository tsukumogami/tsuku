# Lead: Does tsuku already have -- or nearly have -- a concept of a stable, unversioned, tool-owned directory that outlives any single tool version?

Short answer: tsuku has exactly one stable, unversioned, *tsuku*-owned area
(`$TSUKU_HOME/share/`), and zero concept of a stable *tool*-owned data directory.
There is no `DataDir`, no `{tsuku_home}` placeholder, and no primitive a recipe can
reach for. Every path a recipe can name today is either the versioned tool directory
or the versioned libs directory.

## Findings

### 1. `internal/config/` -- the full `$TSUKU_HOME` map

`internal/config/config.go:319-335` is the whole namespace:

```go
// Config holds tsuku configuration
type Config struct {
	HomeDir               string // $TSUKU_HOME
	ToolsDir              string // $TSUKU_HOME/tools
	CurrentDir            string // $TSUKU_HOME/tools/current
	RecipesDir            string // $TSUKU_HOME/recipes
	RegistryDir           string // $TSUKU_HOME/registry (cached recipes from remote registry)
	LibsDir               string // $TSUKU_HOME/libs (shared libraries)
	AppsDir               string // $TSUKU_HOME/apps (macOS application bundles)
	CacheDir              string // $TSUKU_HOME/cache
	VersionCacheDir       string // $TSUKU_HOME/cache/versions
	DownloadCacheDir      string // $TSUKU_HOME/cache/downloads
	CargoRegistryCacheDir string // $TSUKU_HOME/cache/cargo-registry
	KeyCacheDir           string // $TSUKU_HOME/cache/keys (PGP public keys)
	TapCacheDir           string // $TSUKU_HOME/cache/taps (Homebrew tap metadata)
	ShareDir              string // $TSUKU_HOME/share (shared data files, e.g. shell hook files)
	ConfigFile            string // $TSUKU_HOME/config.toml
}
```

Versioned vs stable:

| Path | Versioned? | Created by | Notes |
|---|---|---|---|
| `tools/<name>-<version>/` | **yes** | `config.ToolDir` (`config.go:406`) | this is what `{install_dir}` expands to |
| `libs/<name>-<version>/` | **yes** | `config.LibDir` (`config.go:421`) | `{libs_dir}` is the *parent*, contents versioned |
| `apps/<name>-<version>.app` | **yes** | `config.AppDir` (`config.go:426`) | |
| `tools/current/` | stable dir, versioned *contents* | `EnsureDirectories` | binary symlinks, see §4 |
| `share/` | **stable** | `EnsureDirectories` (`config.go:392-394`) | tsuku-owned, not tool-owned |
| `share/hooks/` | **stable** | `EnsureDirectories` | `cmd/tsuku/cmd_hook.go:71` |
| `share/shell.d/` | stable dir, **version-keyed filenames** | the writing actions, lazily | see §2 |
| `share/completions/` | stable dir, version-keyed filenames | `internal/actions/completions.go:79` | no recipe uses it today |
| `cache/*`, `registry/`, `recipes/` | stable | `EnsureDirectories` | tsuku-owned, disposable |
| `env`, `env.local`, `config.toml` | stable files | `EnsureEnvFile` (`config.go:477`) | `env.local` is the only user-owned artifact |

**There is a `share/`, and tsuku owns it, not tools.** `EnsureDirectories`
(`config.go:373-403`) creates `share/` and `share/hooks/` only; `share/shell.d/` and
`share/completions/` are created on demand by the actions that write into them
(`set_env.go:151-159`, `shell_init.go:144-159`, `completions.go:79`) and by
`shellenv.RebuildShellCache` (`cache.go:31-35`). `share/shell.d/` is mode `0700`,
files `0600`. `ShareDir` is treated as optional -- `config.go:390-394` guards on
`c.ShareDir != ""` for callers that build a `Config` literal without it.

Nothing under `share/` is keyed by tool with a stable name. Everything in it is
either global (`hooks/`, `.init-cache.*`) or version-keyed (`<target>@<version>.<shell>`).

**No prior art for a data directory.** `grep -rn 'XDG_DATA\|DataDir\|data_dir\|StateDir'`
over `internal/` and `cmd/` returns nothing. The concept does not exist.

### 2. `internal/shellenv/` and the shell.d mechanism after PR #2465 (d396aeec)

Commit `d396aeec` ("fix(shellenv): keep shell.d correct for the active version across
the tool lifecycle", #2465, 2026-08-02) implemented `DESIGN-shell-d-lifecycle.md`.
What the mechanism looks like now:

**Naming.** `internal/actions/shell_init.go:179-190` is the single source of truth:

```go
func shellDFileName(target, version, shell string) string {
	return fmt.Sprintf("%s@%s.%s", target, version, shell)
}

func shellDCleanupPath(target, version, shell string) string {
	return "share/shell.d/" + shellDFileName(target, version, shell)
}
```

So fragments are **version-keyed**: `share/shell.d/nvm@0.40.6.bash`, and for `set_env`
`share/shell.d/00-env-nvm@0.40.6.bash` (the `00-env-` prefix is
`actions.EnvFilePrefix`, `set_env.go:27`). `shellDVersion` (`shell_init.go:195`) hard
*fails* if `ctx.Version` is empty rather than collapsing versions onto one filename.

**Where they live.** `$TSUKU_HOME/share/shell.d/`, derived as
`filepath.Dir(ctx.ToolsDir)` in both writers (`set_env.go:150`, `shell_init.go:143`).

**Who writes.** Exactly two actions: `install_shell_init` (`shell_init.go`) and
`set_env` (`set_env.go`). Both run at install time. `set_env` now declares
`DefaultPhase() == "post-install"` (`set_env.go:53-55`) so `{install_dir}` expands
from `ctx.ToolInstallDir` (the permanent directory) rather than the staging directory,
and it errors loudly when `ToolInstallDir == ""` (`set_env.go:124-133`).

**Who deletes, and when.** Each write records a `CleanupAction{Action: "delete_file",
Path: "share/shell.d/...", ContentHash: sha256}` on the execution context
(`shell_init.go:204-242`), which `cmd/tsuku/post_install.go:31` persists via
`Manager.RecordCleanup` onto the *active version's* `VersionState`
(`internal/install/state_ops.go:79-93`). Deletion happens in
`Manager.executeSingleCleanup` (`internal/install/remove.go:415-433`), driven from
`RemoveVersion`, `RemoveAllVersions`, `ReapVersion` (GC, `remove.go:384-412`), and
`ExecuteStaleCleanup` on upgrade. `executeSingleCleanup` supports two verbs:
`delete_file` and `delete_dir` -- **`delete_dir` already exists and does
`os.RemoveAll(filepath.Join(m.config.HomeDir, ca.Path))`**, though nothing records one
today.

**Who decides what gets sourced.** `install.BuildShellDSelection`
(`internal/install/shelld.go:21-49`) projects every installed tool's recorded cleanup
paths into `shellenv.ShellDSelection{Active, Known}`
(`internal/shellenv/selection.go:18-21`). `RebuildShellCache` (`shellenv/cache.go:29`)
concatenates `share/shell.d/*.<shell>` alphabetically into `.init-cache.<shell>`,
skipping anything `Known`-but-not-`Active` (`cache.go:70-73`, `selection.go:34-40`),
rejecting symlinks and non-regular files, verifying recorded hashes, and wrapping each
fragment in a brace group. `$TSUKU_HOME/env` sources that cache
(`config.go:450-459`). Rebuilds fire at the three `ActiveVersion` assignment sites,
after the state write (`shelld.go:85-105`).

**Why this matters for a fix.** Any `NVM_DIR` change ships through `set_env`, which is
version-keyed and post-install. Two hard constraints fall out of the code:

- **`set_env` values are single-quoted and never shell-expanded.**
  `shellQuote` (`set_env.go:245-247`) emits `export NVM_DIR='<value>'`. A recipe
  cannot write `value = "$HOME/.nvm"` or `"${TSUKU_HOME:-$HOME/.tsuku}/share/nvm"` and
  have the shell expand it. Whatever path is chosen must be **fully resolved at
  install time by tsuku**, not by the shell.
- **The placeholder vocabulary has no stable path in it.** `GetStandardVars`
  (`internal/actions/util.go:28-37`) offers exactly `{version}`, `{os}`, `{arch}`,
  `{install_dir}`, `{work_dir}`, `{libs_dir}`, plus `{deps.<name>.version}`. There is
  no `{tsuku_home}` and no `{data_dir}`. **A recipe-only fix is not expressible today** --
  any stable-path option needs a new placeholder (or a new action) in Go.

### 3. `internal/install/symlink.go`

The whole file is 68 lines, two functions, and each has exactly one caller
(`internal/install/manager.go:521` and `:526`, both inside `createBinarySymlink`).

`AtomicSymlink(target, linkPath)` (`symlink.go:14-41`) is **fully generic**: it
`os.Symlink`s to `<dir>/.<name>.tmp` and `os.Rename`s over the destination. It never
inspects the target, never stats it, and does not care whether it is a file or a
directory. It would create a stable directory symlink unchanged. One caveat: the
atomic-replace trick works when the destination is a symlink or absent; `os.Rename`
over an existing *real directory* fails with `ENOTEMPTY`/`EISDIR`, so a
symlink-to-directory scheme must never have to replace a materialized directory.

`ValidateSymlinkTarget(target, toolsDir)` (`symlink.go:46-68`) is **not**
binary-specific but **is** `tools/`-specific. It rejects any target that is not
`toolsDir` itself or under `toolsDir + "/"`, comparing `filepath.Abs`-cleaned strings
with a trailing-separator guard so `tools-malicious` does not match `tools`. It does
*not* call `EvalSymlinks`, so it validates the lexical path, not the resolved one.

Consequence: `ValidateSymlinkTarget` would **reject** a symlink pointing at
`$TSUKU_HOME/share/...` or anywhere outside `tools/`. A "stable dir under `share/`,
symlinked from somewhere" design either does not use this validator or has to widen
it -- and widening a path-traversal guard is a security-review item, not a drive-by.

### 4. `tools/current/` -- a flat binary namespace, verified

`config.CurrentSymlink(name)` is `filepath.Join(c.CurrentDir, name)`
(`config.go:416-418`) and its only real callers key it on a **binary basename**, not a
tool name:

```go
// internal/install/manager.go:509-517
binaryName := filepath.Base(binaryPath)
symlinkPath := m.config.CurrentSymlink(binaryName)
targetPath := filepath.Join(m.config.ToolDir(toolName, version), binaryPath)
```

`createSymlinksForBinaries` (`manager.go:533`) loops the recipe's declared binaries and
makes one entry per binary. `createBinaryWrapper` (`manager.go:579`) writes a *wrapper
script* to the same path for tools with runtime deps. So an entry in `current/` is
"one executable, named as it will be typed", and two different tools shipping a binary
of the same name collide there by design.

It is on `PATH`. `config.EnvFileContent:448`:

```
export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"
```

and `cmd/tsuku/verify.go:284-290`, `internal/autoinstall/run.go:114` both treat it as
the PATH lookup directory.

**The upstream design doc's claim is correct: `tools/current/` is a binary namespace,
not a per-tool directory namespace.** What breaks if you hang a per-tool directory
there:

1. **Namespace collision with binaries.** `current/nvm` as a data directory and
   `current/nvm` as a binary symlink are the same path. Any tool whose binary name
   equals the directory name silently fights it.
2. **It is on `PATH`.** Directories on `PATH` are inert for lookup but the shell stats
   every entry; more importantly `verify.go:453` and `run.go` reason about `current/`
   entries as executables and would report confusing results.
3. **`AtomicSymlink` cannot replace it once materialized.** `os.Rename` over a real
   directory fails, so a per-tool *directory* (as opposed to a dirlink) there is not
   updatable by the existing primitive.
4. **`current/` lives inside `tools/`,** which is the GC scan root -- see the surprise
   in §7.

### 5. `DESIGN-shell-d-lifecycle.md` -- what it settled, what it left open

**Settled (do not re-litigate):**

- Version-key the shell.d filename as `<target>@<version>.<shell>` and give each
  version its own fragment (Option C, `:170-186`). Options A (re-render on lifecycle
  events), B (store rendered bytes in state), and D (containment only) were each built
  out and rejected, with reasons: A cannot reproduce `source_command` output without
  executing a tool binary during `tsuku remove`; B costs 46 KB/version/shell in a file
  every command parses; D turns "wrong exports" into "no exports".
- Exclude from the cache only *recorded-but-inactive* fragments, never an unrecorded
  file, because three writers produce shell.d files without recording cleanup
  (`plan_install.go`, `Executor.installSingleDependency`, `install_lib.go`) and a strict
  manifest gate would silently disable working integrations (`:188-196`).
- `set_env` writes shell.d instead of an unread `env.sh`, expands `{install_dir}` from
  `ToolInstallDir`, declares `post-install`, validates names against
  `[A-Za-z_][A-Za-z0-9_]*`, rejects newlines in values, and single-quotes with POSIX
  escaping (`:312-325`).
- Lifecycle rebuilds go at the three `ActiveVersion` sites, after the state write
  (`:283-299`).
- **Repair is out of scope and structurally impossible** under this design: nothing
  regenerates a shell.d file a user deletes (`:204-210`).
- No migration pass; legacy unversioned records and new version-keyed ones are
  indistinguishable to the projection (`:344-361`).

**Explicitly left open (`:451-472`, "Deliberately out of scope"):**

- `install_completions` has the identical defect in `share/completions/`.
- Library recipes never run post-install and `LibraryVersionState` has no cleanup field;
  `set_env` is rejected in library recipes so the gap fails loudly.
- `Executor.installSingleDependency` runs steps inline with no phase filter and an
  empty `ToolInstallDir`.
- The already-installed short-circuit that `--force` does not bypass -- **so an
  existing nvm install will not pick up a recipe fix by re-running `tsuku install`.**
- **Issue #2464 itself**, quoted verbatim at `:466-472`:

  > `recipes/n/nvm.toml`'s `NVM_DIR` model. `NVM_DIR` is nvm's data root as well as its
  > program directory -- it holds `versions/node/`, `alias/`, `.cache/`, and
  > `default-packages`. Pointing it at a tsuku-versioned directory puts every node version
  > and every global npm package the user installs inside a directory tsuku garbage-collects.
  > This design makes the fragment correct; it does not make the model correct, and shipping
  > it without saying so would be misleading. Separate issue, and the more consequential one
  > for nvm users.

**Prior art the doc carries.** The lead expected Homebrew `opt_prefix` and Nix profile
links in "Considered Options". **They are not there.** The four options are A/B/C/D as
summarized above; the document contains no reference to Homebrew, Nix, `opt_prefix`, or
profile generations. What it *does* contribute that bears on this choice:

- **The `-` separator is poisoned.** `:222-226`: "`GarbageCollectVersions` and
  `config.ToolDir` already match versioned directories by the `name + "-"` prefix,
  which is ambiguous for a tool whose name contains a hyphen". This is why `@` was
  chosen for filenames -- and it is the same reason a stable directory must not be
  named `tools/nvm-data`.
- **Injectivity of the naming scheme is treated as a security property** (`:390-397`),
  because everything in the cache is executed by the user's shell.
- **A precedent for "make the bug class unrepresentable rather than correct it"**
  (`:182-186`). That framing argues for a data root that is *structurally* outside
  anything tsuku deletes, over a fix that teaches GC to skip a path.

### 6. Blast radius -- which recipes face the program-vs-data split

**`recipes/n/nvm.toml` is the only recipe in the registry that uses `set_env`, and the
only one that uses `install_shell_init`.** Verified: `grep -rl 'set_env' recipes/` and
`grep -rn 'install_shell_init' recipes/*/*.toml` both return `recipes/n/nvm.toml`
alone, out of 1,449 recipes. No recipe uses `install_completions`.

So no other recipe can have this bug *by construction*: with no `set_env`, no recipe
exports any environment variable at all, and every other version manager falls back to
its upstream default data root, which is outside `$TSUKU_HOME`.

| recipe | data env var exported by tsuku | value | versioned? | same bug? |
|---|---|---|---|---|
| `n/nvm.toml` | **`NVM_DIR`** (`set_env`, line 35-37) | `{install_dir}` = `$TSUKU_HOME/tools/nvm-<version>` | **yes** | **YES** |
| `p/pyenv.toml` | none | upstream default `PYENV_ROOT` = `~/.pyenv` | no | no |
| `r/rbenv.toml` | none | upstream default `RBENV_ROOT` = `~/.rbenv` | no | no |
| `a/asdf.toml` | none | upstream default `ASDF_DATA_DIR` = `~/.asdf` | no | no |
| `r/rustup.toml` | none | upstream defaults `RUSTUP_HOME` = `~/.rustup`, `CARGO_HOME` = `~/.cargo` | no | no |
| `f/fnm.toml` | none | upstream default `FNM_DIR` under `~/.local/share/fnm` | no | no |
| `m/mise.toml` | none | upstream default `MISE_DATA_DIR` under `~/.local/share/mise` | no | no |
| `u/uv.toml` | none | upstream defaults under `~/.local/share/uv` | no | no |
| `p/pipx.toml` | none | upstream default `PIPX_HOME` under `~/.local/share/pipx` | no | no |
| `d/deno.toml` | none | upstream defaults `DENO_DIR`, `DENO_INSTALL_ROOT` under `~` | no | no |
| `b/bun.toml` | none | upstream default `BUN_INSTALL` = `~/.bun` | no | no |
| `n/n.toml` | none | upstream default `N_PREFIX` = `/usr/local` | no | no -- **different** bug, see below |
| `d/direnv.toml`, `p/poetry.toml` | none | n/a | no | no |

Absent from the registry entirely: `sdkman`, `volta`, `goenv`, `jenv`, `nodenv`,
`cargo` (standalone), `rye`, `conda`.

Caveat on the "value" column: the upstream defaults are stated from knowledge of those
tools, not from anything in this repo -- the repo's only fact is that the recipe sets
nothing. What matters for blast radius is the *tsuku* half, and that half is
unambiguous from the grep.

Two things this table does *not* let us off the hook for:

- **`nvm` is unique for a structural reason, not an accidental one.**
  `recipes/n/nvm.toml:8-12` explains it: nvm is a shell function, and "nvm.sh only
  honors `NVM_DIR` when it is already set and otherwise self-locates to the directory
  it was sourced from." Because tsuku copies `nvm.sh` into
  `share/shell.d/nvm@<version>.bash`, self-location would resolve to `share/shell.d/` --
  worse than the tool directory. The recipe *must* set `NVM_DIR` to something. Every
  other manager on the list resolves its data root independently of where its binary
  lives, so it needs no export.
- **`n/n.toml` has an adjacent, different gap.** It installs via `npm_install`
  (`internal/actions/npm_install.go:89` runs `npm install -g --prefix=<installDir>`),
  and `n`'s `N_PREFIX` defaults to `/usr/local`, which needs root. That is a
  "won't work without sudo" problem, not a "tsuku deletes your data" problem. Worth a
  separate issue; not this one.

### 7. Surprises

- **`$TSUKU_HOME/tools/` is a GC scan root that matches by string prefix, so *any*
  stable directory placed under `tools/` whose name begins with `<toolname>-` is a
  deletion target.** `internal/updates/gc.go:34-47`: `prefix := toolName + "-"`, then
  every directory entry matching that prefix which is neither `<name>-<active>` nor
  `<name>-<previous>` and is older than the retention window (7 days by default,
  `internal/userconfig/userconfig.go:452`) gets `os.RemoveAll`'d. `tools/nvm-data` would
  be reaped on the first background auto-apply that upgrades nvm. This is exactly the
  trap `DESIGN-shell-d-lifecycle.md:222-226` warned about for filenames, one directory
  level up. Note also that `tools/current/` survives only because "current" does not
  start with any tool name plus a hyphen.
- **`delete_dir` cleanup already exists and is wired end to end but has no producer.**
  `internal/install/remove.go:424` handles it, `internal/install/state.go:18` and
  `internal/actions/action.go:57` both document it in the `CleanupAction` schema, and
  no action emits one. A stable per-tool data directory that should be removed on
  `tsuku remove <tool>` (all versions) has half its machinery sitting unused.
- **`executeSingleCleanup` does `filepath.Join(m.config.HomeDir, ca.Path)` with no
  traversal validation** (`remove.go:416`). Cleanup paths are trusted because only
  tsuku's own actions produce them, but a design that lets a *recipe* influence a
  cleanup path would be introducing an `os.RemoveAll` primitive driven by recipe data.
- **`set_env` single-quotes its values, so no `$HOME`/`$TSUKU_HOME` expansion is
  possible** (`set_env.go:245-247`). Combined with the absence of a `{tsuku_home}`
  placeholder in `GetStandardVars` (`util.go:28-37`), a recipe-only fix cannot be
  written today. Every candidate direction requires Go changes.
- **An existing nvm install will not pick up a recipe fix.** The already-installed
  short-circuit that `--force` does not bypass is itself an out-of-scope item in
  `DESIGN-shell-d-lifecycle.md:463-465`. Whatever we decide, migration for users who
  already have `NVM_DIR` pointing at a versioned directory is a real, separate problem
  -- and those are precisely the users with node versions to lose.
- **`ValidateSymlinkTarget` has exactly one caller and is `tools/`-scoped.** It is not
  a general "is this path safe" helper; treating it as one would either fail closed on
  a `share/`-based design or require widening a security boundary.
- The design doc's prior-art section does **not** contain the Homebrew `opt_prefix` /
  Nix profile-link comparison the lead expected. If that comparison is wanted, it has
  to be done fresh.

## Implications

**This is a one-recipe bug with a zero-recipe fix path.** Only nvm is affected, and
only nvm can be affected, because `set_env` has exactly one user. But the fix cannot be
made in the recipe: there is no placeholder that names a stable location and no way to
get shell expansion past `shellQuote`. So the decision is not "nvm-specific patch vs.
general primitive" -- it is "how general should the necessarily-Go-side primitive be,
given exactly one caller."

**The stable location almost certainly cannot live under `tools/`.** The GC prefix
scan makes anything named `tools/<toolname>-*` a deletion candidate, and `tools/current/`
is a binary namespace that a per-tool directory does not belong in. `share/` is the
only existing stable, tsuku-owned area, and it is already the place where "things that
outlive a version but are keyed to a tool" live -- though everything there is currently
version-keyed *inside* a stable directory, which is the opposite of what is needed.

**Whatever is chosen, `set_env` is the delivery mechanism and it stays version-keyed.**
The export fragment `share/shell.d/00-env-nvm@<version>.bash` will keep being rewritten
per version; only the *value* it carries needs to become stable. That is a small change
to what `set_env` can expand, not a change to the shell.d lifecycle -- which is good
news, because the lifecycle design was expensive and is settled.

**`delete_dir` + the `otherPaths` guard is the existing shape for "delete this when the
last version goes."** If the data directory should be reclaimed on full uninstall (and
it is at least arguable that a directory holding the user's node versions should *not*
be), the machinery exists and needs a producer -- but `RecordCleanup` attaches actions
to a *version*, so a directory that must outlive all versions is an awkward fit for a
per-version cleanup record, and the existing `otherPaths` skip in
`executeCleanupActions` is what would keep it alive while any version remains.

**A `{tsuku_home}` or `{data_dir}` placeholder is the smallest lever.** Adding it to
`GetStandardVars` makes the value expressible; the harder question is what path it
should name and who creates it, which is a design decision, not a code one.

## Open Questions

1. **Where should the stable root live?** `$TSUKU_HOME/share/<tool>/`,
   `$TSUKU_HOME/data/<tool>/` (a new top-level sibling), or outside `$TSUKU_HOME`
   entirely at `~/.nvm` (nvm's own upstream default, which is what a non-tsuku install
   would use and what every other manager on the table effectively does)? The last
   option needs no new tsuku concept at all -- but puts user data outside the directory
   tsuku claims to own, and `tsuku remove nvm` then leaves it behind forever.
2. **Should it be reclaimed on full uninstall?** `delete_dir` exists. But deleting a
   directory containing every node version the user installed, during `tsuku remove nvm`,
   is arguably worse than leaking it. This wants a product decision.
3. **Migration for existing installs.** Users who already have `NVM_DIR` pointing at
   `tools/nvm-<version>` have node versions there now. The already-installed
   short-circuit means reinstall does not fix them. Do we move the data, symlink the
   old location, or just print a notice? This may be the largest part of the work.
4. **New placeholder vs. new action.** `{data_dir}` in `GetStandardVars` is two lines
   and immediately available to every action (including `run_command`, which is a
   larger surface than intended). A dedicated action or a `set_env` value form is
   narrower. Which one depends on whether we expect a second caller.
5. **Does a general primitive need a per-tool directory registry?** If `{data_dir}`
   expands to `$TSUKU_HOME/share/<tool>/`, who creates it, with what permissions, and
   what happens when two recipes with the same `metadata.name` (impossible today) or a
   recipe and a tsuku-internal subdirectory collide?
6. **Homebrew / Nix prior art.** The design doc does not carry it. If the exploration
   wants `opt_prefix` and profile-generation comparisons, that research has not been
   done in this repo.

## Summary

tsuku has exactly one stable unversioned area, `$TSUKU_HOME/share/`, but it is
tsuku-owned and its contents are version-keyed; there is no concept of a tool-owned
data directory, no `{tsuku_home}`/`{data_dir}` placeholder in `GetStandardVars`
(`internal/actions/util.go:28-37`), and `set_env` single-quotes its values
(`set_env.go:245`) so no shell expansion can substitute for one -- meaning a recipe-only
fix is not expressible and any fix needs Go changes. nvm is the only recipe in the
registry using `set_env` or `install_shell_init` (1 of 1,449), so blast radius is one
recipe, and it is unique for a structural reason -- nvm.sh self-locates unless `NVM_DIR`
is preset -- while every other version manager resolves its own data root outside
`$TSUKU_HOME`; the `DESIGN-shell-d-lifecycle.md` "Deliberately out of scope" section
records this exact issue and settles the shell.d delivery mechanism the fix must ride on.
The biggest open question is where the stable root should live and whether it should
ever be deleted, sharpened by the discovery that `$TSUKU_HOME/tools/` is a GC scan root
matching by `<toolname>-` string prefix (`internal/updates/gc.go:34-47`), so a stable
directory placed there would itself be garbage-collected -- and by the fact that
existing nvm installs cannot pick up a recipe fix at all, because the already-installed
short-circuit is not bypassed by `--force`.
