# Lead: What can a tsuku recipe actually express today, and what would a fix have to add?

All file:line citations are against the worktree at
`/home/dgazineu/dev/niwaw/tsuku/tsuku+nvm_data_root-d26936a0/public/tsuku/.claude/worktrees/nvm-data-root`
(HEAD `1bcdf236`, which includes PR #2465 / commit `d396aeec`).

## Findings

### 1. `set_env`: implementation, validation, and the complete placeholder list

`set_env` is `SetEnvAction` in `internal/actions/set_env.go:39`. It is registered
in `internal/actions/action.go:209`.

**The placeholder set for `value` is exactly six names, and that is the whole
list.** `Execute` builds its variable map at `internal/actions/set_env.go:147`:

```go
vars := GetStandardVars(ctx.Version, ctx.ToolInstallDir, ctx.WorkDir, ctx.LibsDir)
```

`GetStandardVars` is `internal/actions/util.go:28-37` and returns:

| Placeholder | Source at `set_env` execution time | Expands to |
|---|---|---|
| `{version}` | `ctx.Version` | resolved version, e.g. `0.40.3` |
| `{os}` | `MapOS(runtime.GOOS)` (`util.go:31`, `util.go:72`) | `linux` / `darwin` / `windows` |
| `{arch}` | `MapArch(runtime.GOARCH)` (`util.go:32`, `util.go:86`) | `amd64` / `arm64` / `386` |
| `{install_dir}` | `ctx.ToolInstallDir` | `$TSUKU_HOME/tools/<name>-<version>` |
| `{work_dir}` | `ctx.WorkDir` | the temporary staging work directory |
| `{libs_dir}` | `ctx.LibsDir` | `$TSUKU_HOME/libs` |

Substitution itself is a dumb `strings.ReplaceAll` loop over that map,
`ExpandVars` at `internal/actions/util.go:19-25`. There is no allowlist and no
"unknown placeholder" check anywhere in the `set_env` path — an unrecognized
`{foo}` is simply passed through into the export verbatim.

Note what is **not** available to `set_env`:

- `{tsuku_home}` — does not exist anywhere in the codebase. The only hit for the
  string in Go source is an unrelated JSON tag at `cmd/tsuku/config.go:284`.
- `{home}` — does not exist.
- `{share_dir}`, `{bin_dir}`, `{tools_dir}` — do not exist.
- `{deps.<name>.version}` — exists as a concept (`GetStandardVarsWithDeps`,
  `internal/actions/util.go:44`) but `set_env` calls the plain `GetStandardVars`,
  so dependency-version placeholders are **not** usable in a `set_env` value.
- `{binary}` and `{PYTHON}` — `run_command`-only, injected at
  `internal/actions/run_command.go:80-85`.

**There is no placeholder today that yields a stable, unversioned path.** Every
one of the six either carries the version (`{install_dir}`, `{version}`), is
ephemeral (`{work_dir}`), or is the wrong tree (`{libs_dir}` is
`$TSUKU_HOME/libs`, a shared library directory that `link_dependencies` and
`install_libraries` write into).

**Validation.** `Preflight` (`internal/actions/set_env.go:58-87`) checks only:
`vars` present, parses as an array of `{name, value}` string maps, non-empty,
each name matches `^[A-Za-z_][A-Za-z0-9_]*$` (`envVarNamePattern`,
`set_env.go:16`, checked by `validateEnvName` at `set_env.go:223`), and each
*literal* value contains no `\n`, `\r`, or NUL (`validateEnvValue`,
`set_env.go:236`). `Execute` re-runs `validateEnvValue` on the *expanded* value
at `set_env.go:166` — the comment at `set_env.go:232-235` spells out that
Preflight sees the literal and Execute sees what actually reaches the file.
Values are single-quoted with POSIX escaping by `shellQuote`
(`set_env.go:245-247`), so no metacharacter in a value is interpreted.

Recipe-level validation added by #2465 lives in `internal/recipe/validator.go`:
`set_env` is rejected in library recipes (`validator.go:500-506`) and any
explicit `phase =` that disagrees with the action's declared default is rejected
(`validator.go:508-516`), routed through the new
`ActionValidator.DefaultPhase` interface method
(`internal/recipe/version_validator.go:73-77`, implemented at
`internal/actions/preflight.go:123`).

**Exact code change to add a stable-path placeholder.** Minimal and local — three
lines in `set_env.go`, with a reorder. `tsukuHome` is already derived at
`set_env.go:150` (`filepath.Dir(ctx.ToolsDir)`), just *after* `vars` is built at
line 147. Move the derivation above line 147 and add:

```go
tsukuHome := filepath.Dir(ctx.ToolsDir)
vars := GetStandardVars(ctx.Version, ctx.ToolInstallDir, ctx.WorkDir, ctx.LibsDir)
vars["tsuku_home"] = tsukuHome
vars["share_dir"] = filepath.Join(tsukuHome, "share")
shellDDir := filepath.Join(tsukuHome, "share", "shell.d")
```

No validation change is required — there is no placeholder allowlist to extend.
No plan-format change is required (see §2: `set_env` values are not expanded at
plan generation, so the literal survives into the plan and is expanded at
execution). The cost is: the two lines above, tests in
`internal/actions/set_env_test.go`, and a row in the placeholder documentation
at `plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md`
(`### set_env`, lines 435-489).

Adding it to `GetStandardVars` globally instead would be a much bigger change:
the function's signature takes four positional strings
(`internal/actions/util.go:28`) and has ~30 call sites across the actions
package, none of which have a `tsukuHome` in hand. Do it locally in `set_env`.

### 2. Where expansion happens — the full trace from `nvm.toml` to the shell.d file

The value `"{install_dir}"` from `recipes/n/nvm.toml` goes through **two separate
expansion passes with two disjoint variable sets**, at two different times.

**Hop 1 — recipe load.** `recipes/n/nvm.toml` declares
`vars = [{name = "NVM_DIR", value = "{install_dir}"}]`. TOML decoding produces
`Step.Params["vars"]` as `[]interface{}` of `map[string]interface{}`
(`recipe.Step`, `internal/recipe/types.go:390-404`). The value is an opaque
string; the recipe layer never looks inside it.

**Hop 2 — plan generation (`GeneratePlan`).** `set_env` is not decomposable, so
it takes the non-decomposable branch at
`internal/executor/plan_generator.go:471-489`. Params are expanded by
`expandParams` → `expandValue` → `expandVarsInString`
(`plan_generator.go:604-636`); `expandValue` recurses through slices and maps, so
it *does* reach the `value` string inside the `vars` array. The variable map at
that point is built at `plan_generator.go:205-216` and contains only:
`version`, `version_tag`, `os`, `arch`, plus dotted `version.<metadata>` keys.
**`{install_dir}` is not in that map, so it passes through untouched** and is
written into the plan JSON literally as `"{install_dir}"`. The step lands in the
plan as a `ResolvedStep` with `Phase: ""` — `internal/executor/plan.go:132-138`
explicitly documents that the phase must *not* be resolved at generation time so
a later change to an action's default phase still reaches already-stored plans.
`set_env` is marked evaluable in `ActionEvaluability`
(`internal/executor/plan.go:180`).

**Hop 3 — plan retrieval.** `tsuku install` always goes through
`getOrGeneratePlanWith` (`cmd/tsuku/install_deps.go:61-135`), which first tries a
locally cached plan keyed by tool@version (`install_deps.go:95-108`) before
generating fresh. This matters for migration — see Surprises.

**Hop 4 — install phase.** `ExecutePlan` skips the step:
`internal/executor/executor.go:554` (`if StepPhase(step) != "install" { continue }`).
`StepPhase` (`executor.go:610-616`) sees the empty `Phase` and asks the registry
via `actions.DefaultPhase` (`internal/actions/action.go:147-158`), which finds
`SetEnvAction` implements `PhaseDeclarer` and returns `"post-install"`
(`internal/actions/set_env.go:53-55`).

**Hop 5 — `SetToolInstallDir`.** `cmd/tsuku/install_deps.go:577` calls
`exec.SetToolInstallDir(cfg.ToolDir(toolName, version))`, which sets
`ctx.ToolInstallDir` (`internal/executor/executor.go:347-351`). The plan-install
path does the same at `cmd/tsuku/plan_install.go:117`.

**Hop 6 — post-install phase.** `exec.ExecutePhase(globalCtx, plan, "post-install")`
(`cmd/tsuku/install_deps.go:578`, and `cmd/tsuku/plan_install.go:118`) filters by
`StepPhase` (`executor.go:628-633`) and calls `action.Execute(e.ctx, step.Params)`
at `executor.go:654`.

**Hop 7 — `SetEnvAction.Execute`.** The second expansion pass. `vars` is built at
`set_env.go:147` from `ctx.ToolInstallDir`; the loop at `set_env.go:164-171` does
`value := ExpandVars(envVar.Value, vars)` and writes
`export NVM_DIR='<expanded>'` into a buffer.

**Hop 8 — the file write.** `tsukuHome := filepath.Dir(ctx.ToolsDir)`
(`set_env.go:150`), `shellDDir := $TSUKU_HOME/share/shell.d` (`set_env.go:151`),
created 0700 (`set_env.go:153-159`). Filename comes from `shellDFileName(target,
version, shell)` = `<target>@<version>.<shell>`
(`internal/actions/shell_init.go:179-181`), with `target = "00-env-" + recipe
name` from `envTargetName` (`set_env.go:204-219`, prefix constant at
`set_env.go:27`). For nvm that is
`$TSUKU_HOME/share/shell.d/00-env-nvm@0.40.3.bash` and `...zsh` (shells fixed to
bash/zsh by `envShells`, `set_env.go:21`). Written 0600 at `set_env.go:191`, and a
`delete_file` cleanup action recording the `$TSUKU_HOME`-relative path plus a
content hash is upserted at `set_env.go:196` (`upsertCleanup`,
`shell_init.go:229-242`; path formula in `shellDCleanupPath`,
`shell_init.go:188-190`).

**Hop 9 — recording and cache rebuild.** `finishPostInstall`
(`cmd/tsuku/post_install.go:26-51`) records the cleanup actions into state, then
rebuilds `.init-cache.<shell>` via `shellenv.RebuildShellCache`
(`internal/shellenv/cache.go:30`), which concatenates `share/shell.d/*.<shell>`
in alphabetical order — the reason the `00-env-` prefix must sort ahead of
`nvm@<version>.bash`.

So: **`{install_dir}` is expanded at execution time, in the post-install phase,
inside `SetEnvAction.Execute`.** Not at plan generation, not at shell.d render
time (there is no separate render step — Execute writes the final text).

### 3. `install_dir`

`ctx.ToolInstallDir` is documented at `internal/actions/action.go:19` as
`$TSUKU_HOME/tools/{name}-{version}/`. It is set from
`cfg.ToolDir(name, version)` at `cmd/tsuku/install_deps.go:577` and
`cmd/tsuku/plan_install.go:117`. `Config.ToolDir` is
`internal/config/config.go:406-408`:

```go
return filepath.Join(c.ToolsDir, fmt.Sprintf("%s-%s", name, version))
```

`ToolsDir` is `filepath.Join(tsukuHome, "tools")` (`config.go:356`), where
`tsukuHome` is `$TSUKU_HOME` or `~/.tsuku` (`config.go:340-352`).

**Yes, it is always `$TSUKU_HOME/tools/<name>-<version>`**, with one wrinkle
worth naming: `ctx.InstallDir` is a *different* field — the staging directory
`$TSUKU_HOME/tools/.install/` (`action.go:18`). During the install phase
`{install_dir}` expands from `ctx.InstallDir` (staging) for most actions;
`set_env` deliberately uses `ToolInstallDir` and fails loudly if it is empty
(`set_env.go:124-133`). `install_shell_init` still reads the staging dir for its
*source* file (`shell_init.go:246`), which is correct — it copies out of staging
during install.

Concretely for the issue: `NVM_DIR` today resolves to
`$TSUKU_HOME/tools/nvm-<version>`, which is exactly the directory
`GarbageCollectVersions` deletes.

### 4. Can a recipe create a directory declaratively today?

**No. There is no `mkdir`, `ensure_dir`, `symlink`, `copy`, or `mkdir_p` action.**

Full registered action list (`internal/actions/action.go:202-272`, plus five
self-registering actions in their own files):

**Core / file operations**
- `download` — download with checksum verification (`download.go:19`)
- `download_file` — deterministic download, checksum required (`download_file.go:20`)
- `extract` — archive extraction (`extract.go:57`)
- `chmod` — make files executable (`chmod.go:9`)
- `install_binaries` — copy built artifacts into the tool dir / directory-mode install (`install_binaries.go:13`)
- `install_libraries` — library file installation preserving symlinks (`install_libraries.go:10`)
- `link_dependencies` — symlink a tool's `lib/` into the shared library location (`link_dependencies.go:10`)
- `set_rpath` — RPATH modification for relocatable loading (`set_rpath.go:16`)
- `text_replace` — literal/regex substitution in a file (`text_replace.go:104`)
- `apply_patch` / `apply_patch_file` — apply a patch via system `patch` (`apply_patch.go:13`, `apply_patch_file.go:11`)
- `set_env` — export env vars into the user's shell (`set_env.go:29`)
- `run_command` — arbitrary `sh -c` execution (`run_command.go:10`)
- `require_system` — assert a system prerequisite (`require_system.go`)

**System package managers** — `apt_install`, `apt_repo`, `apt_ppa`,
`brew_install`, `brew_cask`, `dnf_install`, `dnf_repo`, `pacman_install`,
`apk_install`, `zypper_install` (`apt_actions.go`, `brew_actions.go`,
`dnf_actions.go`, `linux_pm_actions.go`). Each installs/registers OS packages.

**Ecosystem package managers** — `npm_install`, `npm_exec`, `pipx_install`,
`pip_install`, `pip_exec`, `cargo_install`, `cargo_build`, `gem_install`,
`gem_exec`, `install_gem_direct`, `cpan_install`, `go_install`, `go_build`,
`nix_install`, `nix_realize`. Each installs or builds within an isolated
prefix/venv/GEM_HOME/store.

**Build systems** — `configure_make`, `cmake_build`, `meson_build`,
`setup_build_env` (the last populates `ctx.Env` for the others).

**Homebrew** — `homebrew` (pull + extract GHCR bottles, `homebrew.go:24`),
`homebrew_relocate` (fix `@@HOMEBREW_PREFIX@@` placeholders, `homebrew_relocate.go:19`).

**Download composites** — `download_archive`, `github_archive`, `github_file`,
`fossil_archive` (`composites.go:96`, `:415`, `:882`, `fossil_archive.go:11`).

**Lifecycle / shell** — `install_shell_init` (write `share/shell.d/<target>@<version>.<shell>`,
`shell_init.go:48`), `install_completions` (write
`share/completions/<shell>/<target>`, `completions.go:11`).

**macOS** — `app_bundle` (install `.app` bundles, `app_bundle.go:14`).

**System configuration** — `group_add`, `service_enable`, `service_start`,
`require_command`, `manual` (`system_config.go:10`, `:229`, `:370`).

**Which of these could create or populate a stable data directory?**

- `run_command` (`internal/actions/run_command.go:56-111`) is the only one that
  can. It runs `sh -c` with `cmd.Env` unset, so it inherits `os.Environ()` —
  meaning `${TSUKU_HOME:-$HOME/.tsuku}` is reachable from inside a command. But:
  it defaults to the **install** phase (no `PhaseDeclarer`), its `{install_dir}`
  expands from `ctx.InstallDir` (the *staging* dir, `run_command.go:79`), it has
  no `{tsuku_home}` placeholder, and `Preflight` emits an explicit warning when
  the command contains `~/.tsuku`, `$HOME/.tsuku`, `.tsuku/tools/`, or
  `.tsuku/bin/` (`run_command.go:31-44`). It is also marked non-evaluable
  (`internal/executor/plan.go:212`) and `RequiresNetwork() == true`
  (`run_command.go:15`), which forces `network=host` in sandbox CI. Using it to
  `mkdir -p` a data root would work but fights the design in four places at once.
- `install_binaries` with `install_mode = "directory"` copies the extracted tree
  into `$TSUKU_HOME/tools/<name>-<version>` — versioned by construction, no help.
- `install_shell_init` and `install_completions` are the only actions that write
  under `$TSUKU_HOME/share`, and both write a single file, not a directory tree.
- `link_dependencies` / `install_libraries` write under `$TSUKU_HOME/libs`, which
  is a versioned library tree, not a data root.

**Do any run on every install including upgrades?** Every step of the plan —
install phase and post-install phase — runs on every fresh install and on every
version upgrade, because an upgrade is a full install of the new version into a
new `<name>-<version>` directory. What does **not** run: an install that
short-circuits on "already installed" executes no steps at all (documented in
the action reference at
`plugins/tsuku-recipes/skills/recipe-author/references/action-reference.md:485-488`).
There is no "first install only" hook and no "once per tool" phase. The declared
phases are `install`, `post-install`, `pre-remove`, `pre-update`
(`internal/recipe/types.go:392-395`); only `install` and `post-install` have
execution call sites (`ExecutePlan` at `executor.go:554`, `ExecutePhase` at
`cmd/tsuku/install_deps.go:578` and `cmd/tsuku/plan_install.go:118`).

### 5. Recipe schema — is there a place to declare persistent state?

**No.** `Recipe` is `internal/recipe/types.go:14-21`: `Metadata`, `Version`,
`Resources`, `Patches`, `Steps`, `Verify`. `MetadataSection`
(`types.go:164-207`) has 22 fields — name, description, homepage,
version_format, requires_sudo, the four dependency lists, tier, type,
llm_validation, curated, binaries, satisfies, and the five platform-constraint
fields. **There is no `data_dir`, no `state_dir`, no persistent-path field, and
no free-form `[metadata]`-style escape hatch** — TOML decoding is into a typed
struct, so an unknown key is silently dropped rather than carried.

`Resources` (`types.go:25-30`) is the closest structural analogue but points the
wrong way: it stages *downloads into the build*, not persistent state out of it.

**Cost of adding a persistent-data-dir field.** Following the pattern #2465 used:

1. Field on `MetadataSection` (`types.go:164`) — one line.
2. `Recipe.ToTOML` round-trip encoding (`types.go:161-171` region, the metadata
   block at `types.go:55-71`) — the serializer is hand-written per field, so a
   new field that must survive round-tripping needs an explicit branch. Missing
   this is a silent data-loss bug (the `[[verify.additional]]` comment at
   `types.go:151-154` records exactly that hazard being caught).
3. Validation in `internal/recipe/validator.go` — path shape, no traversal, no
   absolute paths, plus tests in `validator_test.go`.
4. Plumbing to whatever consumes it. If a data dir must survive `tsuku remove`,
   it must specifically **not** be recorded as a `CleanupAction` — those are
   executed as `os.Remove` / `os.RemoveAll` on removal
   (`internal/install/remove.go:417-433`).
5. Plan carriage, if the plan-install path needs it: `InstallationPlan`
   (`internal/executor/plan.go:31-74`) carries `RecipeType` and `Binaries` as
   precedent for mirroring metadata into the plan. Adding a field there is
   additive-and-omitempty, which #2440 established does **not** require a
   `PlanFormatVersion` bump (`plan.go:22-25`) — but it *does* change every golden
   plan JSON if the field is non-empty for any embedded recipe.
6. Documentation in the recipe-author skill reference.

That is materially more expensive than the placeholder change in §1 — roughly
"one field across four files plus goldens" versus "two lines in one file".

### 6. Golden plan machinery

Two workflows are relevant.

**`.github/workflows/validate-golden-code.yml`** fires on PRs touching a fixed
`paths:` allowlist (lines 6-45). The armed source files are:
`cmd/tsuku/eval.go`; `internal/executor/plan_generator.go`, `plan.go`,
`plan_conversion.go`; `internal/actions/decomposable.go`, `action.go`,
`composites.go`, `download.go`, `homebrew.go`, `cargo_install.go`,
`npm_install.go`, `pipx_install.go`, `gem_install.go`, `go_install.go`,
`nix_install.go`, `fossil_archive.go`, `apply_patch.go`;
`internal/recipe/types.go`, `loader.go`, `platform.go`; `internal/version/*.go`
(excluding tests); the workflow file itself; and
`testdata/golden/code-validation-exclusions.json`.

Lines 49-65 carry an explicit "these do NOT trigger" list covering
execution-only files. **`internal/actions/set_env.go` is on neither list** — it
is simply absent, which puts it in the same practical bucket as the
execution-only files: editing it does not arm this workflow.

The job runs `./scripts/validate-all-golden.sh --os linux --category embedded`
(line 105).

**`.github/workflows/validate-recipe-golden-files.yml`** fires on
`internal/recipe/recipes/**/*.toml`, `recipes/**/*.toml`,
`testdata/golden/exclusions.json`, and the workflow file (lines 6-11). This is
the one a `recipes/n/nvm.toml` edit arms.

**Is there a golden plan for nvm? No.** `testdata/golden/plans/` contains exactly
one subdirectory, `embedded/`, holding 19 recipes: `bash ca-certificates cmake
gcc-libs go libyaml make meson ninja openssl patchelf perl pkg-config
python-standalone ruby rust zig zlib`. There is no `testdata/golden/plans/n/`
and no per-letter registry directory at all, even though
`scripts/regenerate-golden.sh:112-122` supports the `<letter>/<recipe>` layout.
`find testdata/golden -iname "*nvm*"` returns nothing.

**So: changing `recipes/n/nvm.toml` requires regenerating zero golden files.**
The recipe-golden workflow will run (it is armed by the `recipes/**/*.toml`
path), but nvm has no golden artifact for it to diff. Changing `set_env.go`
requires zero golden regeneration because that file arms no golden workflow.

The regeneration command, should it ever be needed, is
`./scripts/regenerate-golden.sh nvm` — it auto-detects registry category
(`regenerate-golden.sh:93-107`), requires `GITHUB_TOKEN`
(`regenerate-golden.sh:24-34`), and pins dependency versions from any existing
golden via `--pin-from` (`regenerate-golden.sh:280-285`).

Separately: `nvm.toml` sets `curated = true`, which enrolls it in
`.github/workflows/curated-nightly.yml` — the discovery step greps
`curated = true` across `recipes/` and `internal/recipe/recipes/`
(curated-nightly.yml:33) and runs the full cross-platform validation suite
nightly, filing an issue on failure. Any change to nvm's install behavior gets
exercised there.

### 7. Grounding the issue's GC claim

Confirmed. `GarbageCollectVersions` (`internal/updates/gc.go:25-90`) walks
`$TSUKU_HOME/tools`, matches `<toolName>-` prefixed directories, protects only
the active version (`gc.go:50-52`) and the rollback target (`gc.go:55-57`), and
`os.RemoveAll`s anything older than the retention window (`gc.go:65-85`).
Retention defaults to 7 days — `DefaultVersionRetention = 7 * 24 * time.Hour`
(`internal/userconfig/userconfig.go:169`), returned by
`UpdatesVersionRetention()` (`userconfig.go:452-459`), and passed in at
`internal/updates/apply.go:152-153`. Since `NVM_DIR == {install_dir} ==
$TSUKU_HOME/tools/nvm-<version>`, every Node version, global npm package, and
alias nvm wrote lives inside a directory on that deletion path.

Nothing under `$TSUKU_HOME/share` is garbage-collected. The only deletions there
are recorded `CleanupAction` paths executed on remove
(`internal/install/remove.go:417-433`) or on stale-cleanup during update
(`internal/install/update.go:51-76`), and `ExecuteStaleCleanup` additionally
refuses to delete any path still recorded by an installed version
(`update.go:55`, `update.go:65-67`, `recordedPaths` at `update.go:81-95`).
`RebuildShellCache` reads and concatenates but never deletes non-cache files
(`internal/shellenv/cache.go:59-80`). **A path under `$TSUKU_HOME/share/` that no
recipe records as a cleanup action is never deleted by tsuku.** That is the
property a stable data root needs, and it holds today without any code change.

## Implications

**The "point NVM_DIR at a stable tsuku-owned path" shape is cheap in code.** The
only genuine gap is the placeholder — two lines in `internal/actions/set_env.go`
plus tests and one doc row. `$TSUKU_HOME/share/` is already the right tree
(shell.d and completions live there), is already created by
`Config.EnsureDirectories` (`internal/config/config.go:392-394`), and is already
outside every GC path. Because the recipe change touches no golden plan and the
`set_env.go` change arms no golden workflow, the CI blast radius is unit tests
plus the nightly curated run.

**The one thing the fix does not get for free is directory creation.** No action
can `mkdir` declaratively, and `set_env` writes an export without touching the
target path. Whether that matters depends entirely on whether nvm creates
`$NVM_DIR` itself when it is set to a non-existent path — a question for the
upstream lead, not answerable from tsuku's source. If it does not, the options
are: add a small `ensure_dir`-shaped action (new file + `Register` in
`action.go:202` + an `ActionEvaluability` entry in `plan.go:180` region, since
unknown actions default to non-evaluable at `plan.go:217-224`, + preflight +
tests + docs), or lean on `run_command`, which works but trips a hardcoded-path
warning, is non-evaluable, and declares `RequiresNetwork() == true`.

**The "leave NVM_DIR at `$HOME/.nvm`, drop the export" shape costs essentially
zero code** — delete the `[[steps]] action = "set_env"` block from
`recipes/n/nvm.toml` and the comment above it. No Go change, no goldens. It also
gives up the `00-env-` ordering guarantee that the whole of #2465 was built to
provide, and the recipe's own comment (nvm.toml lines 7-12) documents why that
ordering exists: `nvm.sh` only honors `NVM_DIR` when it is already set, and
otherwise self-locates to the directory it was sourced from — which, under
tsuku, is `$TSUKU_HOME/share/shell.d/`. Dropping the export without
understanding that self-location behavior is the risky move, not the cheap one.

**A schema field for persistent state is the expensive shape** and buys nothing
the placeholder does not, unless something other than `set_env` needs to know
about the data root.

## Surprises

**A `set_env` value is expanded twice, by two different code paths, with
disjoint variable sets, at times separated by a plan cache.** Plan generation
(`plan_generator.go:604-636`, vars from `plan_generator.go:205-216`) expands
`{version}`, `{version_tag}`, `{os}`, `{arch}`, and `{version.<meta>}` and
freezes the results into the plan JSON. Execution (`set_env.go:147`,
`util.go:19`) then expands `{install_dir}`, `{work_dir}`, `{libs_dir}` — and
*also* `{version}`, `{os}`, `{arch}`, which are already gone. The second pass's
`{os}`/`{arch}` come from `runtime.GOOS`/`runtime.GOARCH`, not the plan's target
platform, so a cross-platform-generated plan and a locally-generated one take
different routes to the same answer. Nothing is broken today, but it means the
answer to "what does this placeholder expand to" depends on which pass claims it
first, and nothing in either file mentions the other.

**Recipe changes can be masked by the local plan cache.** `getOrGeneratePlanWith`
(`cmd/tsuku/install_deps.go:95-108`) returns a locally cached plan keyed by
`tool@version` before ever consulting the recipe. Since the `set_env` value is
carried into the plan verbatim, a user who already has a cached plan for
`nvm@<version>` would keep the old `{install_dir}` literal even after the recipe
is fixed — until they upgrade to a new version, run with `--fresh`, or the
cached plan fails `ValidateCachedPlan`. Any migration story for existing nvm
users has to account for this.

**`{deps.<name>.version}` is documented as a general recipe placeholder
(`internal/actions/util.go:39-43`) but is unavailable in `set_env` values**,
because `set_env` calls `GetStandardVars` rather than `GetStandardVarsWithDeps`.
There is no error for this — the literal `{deps.foo.version}` would be written
straight into the export. `CheckUnexpandedDepVars` (`util.go:58-69`) exists to
catch exactly that and is not called from `set_env`.

**`{libs_dir}` is already a stable, unversioned tsuku-owned path**
(`$TSUKU_HOME/libs`) available to `set_env` today. It is the wrong tree
semantically — it is where `install_libraries` and `link_dependencies` place
shared `.so`/`.dylib` files — but it demonstrates that a non-`tools/` path
reaching a `set_env` value requires no new machinery whatsoever.

**`install_completions` is absent from `ActionEvaluability`**
(`internal/executor/plan.go:180-212`), so it silently evaluates as non-evaluable
via the unknown-action default at `plan.go:217-224`, unlike its sibling
`install_shell_init` which is explicitly `true`. Unrelated to this issue, but it
means any new lifecycle action must remember to register there or quietly make
every recipe using it non-reproducible.

**There is no golden plan for any registry recipe at all** — only the 19 embedded
ones. `scripts/regenerate-golden.sh` carries full support for the
`testdata/golden/plans/<letter>/<recipe>/` layout (lines 112-122, 96-97) that
nothing currently uses.

## Open Questions

1. **Does nvm create `$NVM_DIR` when it is set to a non-existent path?** This is
   the single fact that decides whether the fix is two lines or two lines plus a
   new action. Belongs to the upstream lead.
2. **What happens to users who already have Node versions under
   `$TSUKU_HOME/tools/nvm-<version>/`?** Repointing `NVM_DIR` orphans them
   silently rather than deleting them faster, but they are still on the GC path.
   Is a migration step in scope, and if so, what action performs it (nothing
   declarative can move a directory today)?
3. **What is the naming convention for a per-tool data root under `share/`?**
   `share/shell.d/` and `share/completions/` are function-scoped, not
   tool-scoped. `share/nvm/` would be the first tool-scoped entry, and whether
   the placeholder should be `{share_dir}` (recipe composes `{share_dir}/nvm`)
   or a tool-scoped `{data_dir}` (tsuku composes `share/<name>`) is a design
   call, not a code-cost one — both are the same two lines.
4. **Should the stale-plan-cache path get an explicit invalidation** for users
   who installed nvm before the fix, or is "it corrects itself on next upgrade"
   acceptable?
5. **Does `tsuku doctor`'s shell.d hash check react** when a fragment's content
   changes because the recipe changed but the version did not? Worth a look at
   `internal/shellenv/doctor.go` before shipping, since the recorded
   `ContentHash` is per-path and the path is version-keyed.

## Summary

`set_env` supports exactly six placeholders in its `value` field — `{version}`,
`{os}`, `{arch}`, `{install_dir}`, `{work_dir}`, `{libs_dir}` — all from
`GetStandardVars` at `internal/actions/util.go:28`, and none of them yields a
stable unversioned path, while `{install_dir}` resolves to
`$TSUKU_HOME/tools/<name>-<version>`, precisely the directory
`GarbageCollectVersions` deletes after 7 days. Adding a `{tsuku_home}` or
`{share_dir}` placeholder is a two-line change in `internal/actions/set_env.go`
(the `tsukuHome` value is already computed three lines below the var map) with
no validation change, no plan-format change, and no golden regeneration — nvm has
no golden plan and `set_env.go` arms no golden workflow — whereas a recipe-schema
field for persistent state would cost a field plus hand-written TOML
serialization plus validation plus goldens. The biggest open question is whether
nvm creates `$NVM_DIR` itself when pointed at a non-existent path, because no
tsuku action can create a directory declaratively today and `run_command` is the
only workaround.
