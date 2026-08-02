# Security Review: nvm-data-root

Reviewed: `docs/designs/DESIGN-nvm-data-root.md` and
`wip/design_nvm-data-root_decision_{1,2,3,4}_report.md`, against the code in this
worktree. The design is not implemented, so every finding below is about a stated
guard, or about code the design assumes behaves a certain way. Two claims were
checked empirically with throwaway Go programs (both since deleted); their output is
quoted.

## Verdict

**Findings that must change the design.** Five, of which two are the design giving
itself a weaker guard than it thinks it has (`install_program_files.dir`, `--purge`),
one is an under-specified invariant that is wrong if implemented the obvious way
(`mergeMove` type tests), one is a validation that does not survive a symlinked
ancestor (`files`), and one is a security claim in the doc that is factually false
against the current extractor.

Nothing here argues against the architecture. The location choice, the
no-`delete_dir` rule, the "no deletion primitive" claim, and the `--purge`
foreground/confirm shape all hold up under examination — see the last section.

---

## Must-change findings

### 1. `install_program_files.dir` is a write-anywhere-in-`$TSUKU_HOME` primitive, and `$TSUKU_HOME` contains two directories that are executed

**What.** The design specifies `dir` as a free-form recipe parameter, "expanded then
validated to be absolute, inside `$TSUKU_HOME`, and **not** under `tools/`"
(`docs/designs/DESIGN-nvm-data-root.md:393-394`; decision 2 report at
`wip/design_nvm-data-root_decision_2_report.md:77-84` is wider still — it also permits
"inside the user's home"). That predicate admits `$TSUKU_HOME/share/shell.d` and
`$TSUKU_HOME/bin`.

**Why it matters.** Both are live surfaces:

- `share/shell.d` is concatenated into the shell init cache that `$TSUKU_HOME/env`
  sources. `RebuildShellCache` includes every `*.bash`/`*.zsh` file it finds
  (`internal/shellenv/cache.go:62-88`), and `ShellDSelection.excludes` returns
  **false** for any path it does not know (`internal/shellenv/selection.go:35-40`) —
  an unrecorded fragment is not filtered out, it is sourced.
- `$TSUKU_HOME/bin` is treated as a PATH location by tsuku's own check
  (`cmd/tsuku/install.go:404-408`) and is documented as such.

So `dir = "/home/u/.tsuku/share/shell.d"` plus `files = ["evil.bash"]` copies an
attacker-controlled file (contents come from the release tarball) straight into the
user's shell startup. A recipe can already get shell code into `shell.d` through
`install_shell_init` — that is the action's purpose — but that path is gated by
`--no-shell-init` (`internal/actions/shell_init.go:112-115`) and its filename is
version-keyed through `shellDFileName`, so the selection machinery can reason about
it. `install_program_files` deliberately ignores `--no-shell-init`
(`DESIGN-nvm-data-root.md:398`), so routing through it is a clean bypass of the one
flag a user has for saying "do not touch my shell".

The `not under tools/` half of the predicate is also doing no security work — it is a
correctness check against a recipe author writing `{install_dir}`, which the design
says outright. It is the "inside `$TSUKU_HOME`" half that is load-bearing, and it is
too coarse.

**Concrete fix.** Drop the `dir` parameter. The action always writes to the tool's own
data directory, computed Go-side by the same `dataDir(ctx)` helper the design already
specifies (`DESIGN-nvm-data-root.md:383-387`), which derives from `ctx.ToolsDir` and
the recipe name and reuses `envTargetName`'s single-path-segment validation
(`internal/actions/set_env.go:204-219`). The recipe surface becomes:

```toml
[[steps]]
action = "install_program_files"
files = ["nvm.sh", "nvm-exec"]
```

This is strictly better than validating `dir`: there is no recipe-controlled path
left to validate, the containment check disappears along with its symlink caveat
(see finding 3), and `{data_dir}` is already per-tool so `dir` carries no information
the action does not already have. If a future tool genuinely needs a second location,
that is a new parameter with its own decision, not a hole left open now.

If `dir` is kept anyway, the validation must be `dir == dataDir(ctx)` — an equality,
not a containment — which is the same thing spelled less directly.

### 2. `--purge` deletes a path built from an unvalidated CLI argument, and the design removes the only guard that currently stops it

**What.** `--purge` "computes its target from config (`data/<tool>`), not from a
recorded cleanup action" (`wip/design_nvm-data-root_decision_1_report.md:157-158`) and
"must tolerate an absent state entry, so a user who already ran `tsuku remove nvm` can
still reclaim the directory" (`:176-178`, echoed at `DESIGN-nvm-data-root.md:502-503`).

`<tool>` is `args[0]` split on `@`, with no validation whatsoever
(`cmd/tsuku/remove.go:30-39`). There is no `ValidateToolName` in the tree — the only
path-safety validator on this route is `ValidateVersionString`
(`internal/install/state.go:333`, called at `internal/install/remove.go:123`), which
covers the version and not the name. The single thing standing between a traversal
name and `os.RemoveAll` today is the state lookup: `RemoveAllVersions` returns
`tool %q is not installed` when `toolState == nil` (`internal/install/remove.go:239-241`).

**Why it matters.** "Tolerate an absent state entry" deletes exactly that guard for
the new code path. `filepath.Join` cleans before it returns, so a traversal name
escapes silently — verified:

```
Join("/home/u/.tsuku", "../../../etc/passwd") = /etc/passwd
```

`tsuku remove ../../../../tmp/anything --purge` then resolves to `/tmp/anything` and
`os.RemoveAll`s it. The interactive confirmation does print the resolved path, so this
is not silent, but "the user typed a weird string and hit y" is not a control — and
the same shape is reachable from a shell script or an alias.

**Concrete fix.** Validate the tool name as a single path segment before it is used to
build any path — reject anything where `name != filepath.Base(name)`, `name == "."`,
`name == ".."`, or `strings.ContainsAny(name, "/\\")`. `envTargetName`
(`internal/actions/set_env.go:209-217`) already contains exactly this check; lift it
into a shared `ValidateToolName` and call it at the top of `removeCmd.Run`
(`cmd/tsuku/remove.go:30`), which also hardens the pre-existing
`ToolDir(name, version)` construction at `internal/install/remove.go:150` and
`internal/config/config.go:406-408`. Belt and braces: have `--purge` require that
`data/<tool>` exists **and** that `<tool>` resolves to a known recipe, rather than
purging whatever the string names.

### 3. `mergeMove` needs `Lstat` semantics for the *type* tests, not only for the existence check

**What.** The design's security section says only: "it must not resolve symlinks when
deciding whether a destination path 'exists' ... Using `os.Lstat` for the existence
check is the requirement" (`DESIGN-nvm-data-root.md:561-564`). The algorithm has a
second predicate — "if both sides are directories, recurse"
(`:416-418`, decision 3 report `:180-182`) — and the design says nothing about how
that one is evaluated.

**Why it matters.** `os.Stat` and `os.Lstat` disagree on a symlink to a directory, and
the recursion is what turns the disagreement into data movement. Verified:

```
entry versions: DirEntry.IsDir=false  os.Stat.IsDir=true  os.Lstat.IsDir=false
```

Implemented with `os.Stat`, a symlink at `dst/versions` pointing anywhere is "a
directory", the existence check passes, and `mergeMove` recurses into the symlink's
target and renames entries into it. Worse in the other direction: a symlink at
`src/versions` pointing at, say, `$HOME/Documents`, with a real `dst/versions`
present, makes `mergeMove` enumerate `$HOME/Documents` and rename its entries into
`data/nvm/versions`. That is a move-anything primitive built out of an algorithm whose
headline invariant is "no deletion primitive" — the invariant is about `RemoveAll`, and
it does not cover renaming things out of a directory the migration was never pointed at.

Who plants the symlink matters, and finding 5 supplies a plausible planter.

**Concrete fix.** State the invariant as *every* stat in `mergeMove` is `os.Lstat`,
and every type test is `Mode().IsDir()` on an `Lstat` result or `DirEntry.IsDir()`
from `os.ReadDir` (which is already lstat-semantics — see the probe output above).
Treat a symlink on either side as "not a directory", i.e. a conflict: leave it, report
it, never follow it. Add a test with a symlink planted on each side; it is two
`os.Symlink` calls and it is the test that pins the invariant.

### 4. The `files` check "reject a source that is not a regular file" does not survive a symlinked ancestor

**What.** `DESIGN-nvm-data-root.md:555-560`: "`files` entries are rejected at preflight
if absolute or containing `..`, and `Execute` rejects a source that is not a regular
file, so a symlink planted in the release tarball cannot be used to read outside the
tool directory."

**Why it matters.** Two gaps.

- The lexical `..` rejection says nothing about a **symlinked directory component**.
  `files = ["sub/nvm.sh"]` where `sub` is a symlink resolves outside the tool
  directory while containing no `..` and being relative. The final-component check
  then sees a perfectly regular file and copies it.
- "Rejects a source that is not a regular file" is ambiguous about `Stat` versus
  `Lstat`. With `os.Stat`, a symlink to a regular file *is* a regular file, and the
  check passes on precisely the input it was written to catch.

The payload is an arbitrary readable file copied to a predictable path under
`data/<tool>/<basename>`. Modest on its own; not something to leave in a package
manager.

**Concrete fix.** Resolve and verify, do not check lexically. After joining, run
`filepath.EvalSymlinks` on both the candidate source and `ctx.ToolInstallDir` and
require containment of the resolved source in the resolved tool dir (the pattern
`validateCommandBinary` already uses at `internal/actions/shell_init.go:355-373`).
Then open the source with `os.OpenFile(src, os.O_RDONLY|syscall.O_NOFOLLOW, 0)` and
`Stat` the *file handle*, so the type check and the read see the same object and the
TOCTOU window closes rather than narrowing. Write the destination with
`O_CREATE|O_EXCL|O_NOFOLLOW` on the temp name before the rename — `copyFile`
(`internal/actions/download_cache.go:319-337`) uses plain `os.Create`, which follows a
symlink at the destination and preserves no mode, so this action cannot reuse it
as-is.

### 5. "A symlink planted in the release tarball cannot be used to read outside the tool directory" is false today — the extractor's containment is purely lexical

**What.** The design's threat model for archives (`DESIGN-nvm-data-root.md:555-560`)
rests on the extractor containing tarball symlinks. It does not.
`isPathWithinDirectory` (`internal/actions/extract.go:19-35`) compares
`filepath.Abs`-cleaned strings with `strings.HasPrefix` and never calls
`EvalSymlinks`; `validateSymlinkTarget` (`:39-55`) resolves the link target with
`filepath.Join`, which is also lexical. Both treat a symlink as if it were a
directory, so a two-hop chain whose lexical resolution stays inside `destPath` while
its real resolution leaves it passes both checks.

**Evidence.** A three-entry tarball — symlink `a -> "."`, symlink `b -> "a/.."`,
regular file `b/pwned` — driven through `ExtractAction.extractTarGz`
(`internal/actions/extract.go:193`) in a throwaway test in `internal/actions`:

```
extractTarGz err = <nil>
ESCAPED: wrote "owned" outside dest at /tmp/TestZZProbeSymlinkEscape.../001/pwned
```

Every entry is validated and accepted; the file lands one level above the destination
directory. The write goes through `os.OpenFile` on a lexically-inside path
(`extract.go:316-343`), so an arbitrary path is reachable by lengthening the chain.

**Why it matters here.** This is pre-existing and not introduced by this design — but
the design cites the guarantee as a reason its own checks are sufficient, and it is
the guarantee that would otherwise stop a compromised release tarball from planting
the symlinks that findings 3 and 4 need. A tarball that can write outside its
destination can plant `$TSUKU_HOME/share/shell.d/versions -> $HOME` before the
migration runs.

**Concrete fix.** Two parts, and only the first belongs to this PR.

1. Delete the claim from `DESIGN-nvm-data-root.md:555-560` and replace it with what is
   actually true: the extractor's containment is lexical, so the new actions must
   defend themselves (findings 1, 3, 4) rather than inherit containment from extract.
2. File the extractor bug separately, alongside the other pre-existing issues already
   listed under "Deliberately out of scope" (`:635-652`). The fix is to resolve each
   entry's parent with `EvalSymlinks` before the containment check, or to extract with
   `openat`-style relative descriptors. It is wider than this feature and should not
   be bundled.

---

## Informational

### The unconditional `os.Remove(src)` does not fit either real call shape

`mergeMove` "finish[es] with `os.Remove(src)`, which fails harmlessly on a non-empty
directory" (`DESIGN-nvm-data-root.md:418-419`). For population B the source is
`tools/nvm-<version>/`, and only an enumerated subset moves (`:442`) — so the trailing
remove either fails (fine) or, if the tool directory happened to be emptied, deletes a
directory `state.json` still lists as installed. For population A the source is
`$TSUKU_HOME/share/shell.d` itself (`:441`), a directory tsuku owns and relies on.
The blast radius is small — `RebuildShellCache` calls `MkdirAll` before it reads
(`internal/shellenv/cache.go:31-35`) — but "the migration deletes shell.d" is a
surprising sentence to have to write later.

Suggest: `mergeMove` takes the entries to move as an argument and never removes its
source; if a caller wants the source pruned, it says so explicitly at a call site
where the identity of the directory is known.

### The cleanup-path root and the action's `$TSUKU_HOME` are computed from different places

`install_program_files` would derive `$TSUKU_HOME` as `filepath.Dir(ctx.ToolsDir)`
(the established idiom — `set_env.go:150`, `completions.go:78`,
`shell_init.go:141`), while `executeSingleCleanup` resolves the recorded relative path
against `m.config.HomeDir` (`internal/install/remove.go:418`). `DefaultConfig` keeps
them equal (`internal/config/config.go:354-355`), so this is only a hazard for
directly-constructed `Config` literals — of which the test suite has several. Worth
one assertion in the action rather than an assumption.

### Mode should not be derived from an attacker-controlled tar header

The design chmods "from the source mode masked to drop group/other write"
(`DESIGN-nvm-data-root.md:396-397`). The source mode comes from `header.Mode` via
`os.FileMode(header.Mode)` at `internal/actions/extract.go:334`. Go's `syscallMode`
drops setuid/setgid unless the corresponding `os.FileMode` bits are set, and a raw tar
`04000` does not set `os.ModeSetuid`, so the obvious attack does not land — but
deriving a mode from tarball bytes at all is unnecessary here. Simpler and
unambiguous: `0755` if the source has any execute bit, else `0644`.

### `data/` directory mode

`EnsureDirectories` creates everything `0755` (`internal/config/config.go:396-400`),
so `data/` and its contents would be world-readable on a shared machine. That matches
`tools/`, but `data/` is user data, and the two directories in the tree that hold
things tsuku considers sensitive are both `0700` (`shell.d` at `set_env.go:153-159`,
`completions` at `completions.go:126-131`). `0700` for `data/` costs nothing — nvm
`mkdir -p`s its own subtree underneath and does not care. Note that this only affects
the root: nvm creates `versions/` etc. itself, under the user's umask.

### The validator rule: assert with comma-ok, and note it runs *before* the action's own parser

The rule is sited "immediately after the existing action-keyed `set_env` block at
`:500-506`" (`wip/design_nvm-data-root_decision_4_report.md`), which is *before* the
`av.ValidateAction` call at `internal/recipe/validator.go:519-555` and before the
`if actionResult.HasErrors() { continue }` bail at `:552-554`. So the new rule sees
malformed `vars` that `SetEnvAction.Preflight` would have rejected — it cannot assume
the shape has already been checked.

Every assertion must be two-value: `raw.([]interface{})`, `item.(map[string]interface{})`,
`m["name"].(string)`, `m["value"].(string)`. `SetEnvAction.parseVars`
(`internal/actions/set_env.go:250-279`) is the correct model and gets this right
throughout; the failure mode to avoid is the single-value form on a missing key, where
`m["name"]` is `nil` and the assertion panics. Malformed input should make the rule
skip the step (the action's own Preflight will produce the error a line later), never
panic. Two table cases — `vars = "string"` and `vars = [{name = 1}]` — pin it.

Separately, and worth stating in the design so nobody mistakes it for a control: this
rule is a lint, not a security boundary. It runs on the install path unconditionally
(`cmd/tsuku/install_deps.go:281`) but not on the sandbox path, and a hostile recipe
simply picks a variable that is not on the denylist. Its value is preventing an honest
mistake in a future recipe, which is what decision 4 claims for it.

### Concurrency during the migration

A `nvm install` running in another shell while `migrate_data_dir` renames can create
`versions/node/<v>` in the old root after that entry has moved, leaving the estate
split across two locations. No data is destroyed and the next run of the shape
detector merges it. The design already says the operation is "not atomic overall;
atomic per entry" (`DESIGN-nvm-data-root.md:420-422`), which covers this honestly. No
change requested — flagged so it is not mistaken for an oversight.

### Recorded cleanup paths are trusted at removal time

`state.json` is written `0644` (`internal/install/state.go:251,320`) and
`executeSingleCleanup` executes whatever `ca.Path` says, joined onto `HomeDir`, with
no re-validation (`internal/install/remove.go:417-434`). Anyone who can write
`state.json` can therefore direct a delete on the next `tsuku remove` — but they can
also write the user's `~/.bashrc`, so this is not a boundary this design crosses. It
does argue for validating at *record* time and at *execute* time rather than only at
record time, if that ever becomes cheap. Out of scope here.

---

## Areas checked and found sound

- **`executeSingleCleanup` really is unvalidated.** `filepath.Join(m.config.HomeDir,
  ca.Path)` at `internal/install/remove.go:418`, dispatching `os.Remove` /
  `os.RemoveAll` at `:421-426`, no traversal check anywhere in the function. The
  design's characterization is exact, and its conclusion — validate in the action,
  before any cleanup record is written — is the right place for it. Finding 1 is about
  the strength of that validation, not its location.

- **The "no deletion primitive" claim holds, including the case the brief asked about.**
  Verified: `os.Remove` on a symlink to a non-empty directory unlinks the symlink and
  returns nil, leaving the target's contents intact; `os.RemoveAll` on the same input
  behaves identically. So neither the trailing `os.Remove(src)` nor `--purge`'s
  `RemoveAll` can be redirected through a symlink into deleting someone else's tree.
  The residual risk in `mergeMove` is *movement*, not deletion — finding 3.

- **Refusing to record a `delete_dir` cleanup for the data root.** The reasoning at
  `DESIGN-nvm-data-root.md:236-244` checks out against the code: `ReapVersion`
  (`internal/install/remove.go:374-412`) runs cleanup actions from background GC, and
  `executeCleanupActions` skips a path only on exact string equality against another
  installed version's records (`:345-354`), which `--no-shell-init` guarantees will be
  absent (`internal/actions/set_env.go:100-104`). The named sequence really does end
  in an unattended `os.RemoveAll` of the user's Node estate. Keeping the data root out
  of the cleanup ledger entirely is the correct call, and it is the single most
  important structural decision in the design.

- **Making `install_program_files` ignore `--no-shell-init` keeps the cross-version
  guard effective.** Because every install records the same two cleanup paths, reaping
  a non-active version hits the `otherPaths` skip (`internal/install/remove.go:364-368`)
  and does not delete `nvm.sh`/`nvm-exec` out from under the active version. The
  design reaches the right answer; the reasoning at `:398-399` is correct.

- **Garbage collection cannot reach `data/`.** `GarbageCollectVersions` reads only
  `toolsDir` and matches `toolName + "-"` (`internal/updates/gc.go:26-46`). Nothing
  else in the tree enumerates `$TSUKU_HOME`'s children; `cache cleanup` operates on
  `RegistryDir`/`CacheDir` only. Moving the data root out of `tools/` genuinely
  removes it from every automatic deletion path.

- **Population A's "a directory under `share/shell.d` is not tsuku's" is provable, as
  claimed.** Both enumerators skip directories: `internal/shellenv/cache.go:65` and
  `internal/shellenv/doctor.go:75`. Every tsuku writer under `shell.d` writes named
  files.

- **`set_env`'s injection defense is preserved.** `{data_dir}` expands Go-side and the
  result still passes through `shellQuote` (`internal/actions/set_env.go:169,245-247`),
  which single-quotes and escapes embedded quotes. Adding the placeholder does not
  widen this surface. `validateEnvValue` (`:236-241`) still rejects newlines and NUL
  after expansion, which is what matters given the value is now derived from
  `$TSUKU_HOME`.

- **`--purge`'s interactive shape.** `confirmWithUser` returns false on a non-TTY
  (`cmd/tsuku/create.go:167-170`, `isInteractive` at `cmd/tsuku/install.go:396-398`),
  so gating on it alone would make `--purge` a silent no-op in scripts. Erroring with
  `ExitNotInteractive` (`cmd/tsuku/exitcodes.go:47-49`) instead is right, and matches
  the one existing precedent (`cmd/tsuku/cmd_run.go:106-108`). Restricting `--purge` to
  whole-tool removal is also right: the data root is shared across versions, so
  `remove nvm@X --purge` with another version installed would destroy data the
  surviving version still points at. Finding 2 is about the target path, not this
  shape.

- **Reading the active fragment rather than stat-ing `data/nvm/nvm.sh` for the doctor
  predicate.** `Activate` re-selects fragments without rewriting them
  (`internal/install/manager.go:417-481`), so the file-exists shortcut would migrate a
  rolled-back install and break it. The design's reasoning
  (`DESIGN-nvm-data-root.md:460-464`) is correct and the exact-quoted-form match from
  decision 3 (`strings.Contains(content, "export NVM_DIR="+shellQuote(dataDir))`)
  avoids the prefix false-positive.

- **No new privilege, network, or secret.** Both new actions read only the tool's own
  install directory and declare `RequiresNetwork() == false`. The migration package is
  stdlib + `internal/config`. Nothing added here downloads, authenticates, or executes.

- **`{data_dir}` is not reachable by the tool name.** `dataDir(ctx)` derives the
  segment from `ctx.Recipe.Metadata.Name` under `envTargetName`'s validation
  (`internal/actions/set_env.go:209-217`), which rejects `.`, `..`, separators and
  `@`. That is the correct reuse. One implementation note: `envTargetName` *returns*
  `EnvFilePrefix + name`, so the helper must reuse the validation, not the return
  value.
