# Lead: Does the same class reach process execution rather than shell text?

Scope applied: externally-supplied values that become an `os/exec` argument, an
interpreter argument, a generated script body, or an environment variable handed
to a child process. Plain `exec.Command("foo", userValue)` with no shell, no
flag-injection potential and no executable selection is deliberately NOT reported.

## The bound, stated up front

The `os/exec` surface is large and almost entirely argv-safe.

- **139 non-test `exec.Command`/`exec.CommandContext` call sites across 40 files**
  (`grep -rnE "exec\.Command(Context)?\(" --include=*.go internal/ cmd/`, minus
  `_test.go` and comment lines).
- **Exactly 4 of those pass a string to a shell**: `internal/actions/run_command.go:96`
  and `cmd/tsuku/verify.go:251`, `:309`, `:392`. All four are `sh -c`.
- **`argv[0]` is never an externally-supplied string.** Every call site uses either a
  string literal (`"apk"`, `"rpm"`, `"tar"`, `"curl"`, `"hdiutil"`, `"uname"`,
  `"docker"`, `"sh"`), a path resolved by `exec.LookPath` / `LookPathInDirs`
  (`internal/actions/util.go:826-842`) against tsuku-owned tool directories, or
  `os.Executable()` (`internal/updates/trigger.go:121`, `:135`). The two sites where
  `argv[0]` is a variable named `command` — `internal/actions/require_system.go:149`
  and `internal/actions/system_config.go:310` — take it from a recipe parameter, i.e.
  registry trust, and resolve it through the ambient PATH like any other tool probe.
- **There is no `git` invocation anywhere in non-test code.** `grep -rn '"git"' --include=*.go internal/ cmd/`
  returns only a JSON struct tag (`internal/builders/go.go:40`) and a string in a
  prefix-matching comment (`internal/install/list.go:81`). The entire classic class —
  `--upload-pack=`, `ext::`, `GIT_SSH_COMMAND`, a repo slug or URL that git parses as
  an option — **does not exist in this codebase**. Registry and distributed-recipe
  fetching are plain HTTPS against `raw.githubusercontent.com` /
  `objects.githubusercontent.com` / `api.github.com`, with a host allowlist
  (`internal/distributed/client.go:19-28`).
- **No `LD_PRELOAD` anywhere.** Environment additions handed to children are
  `PATH`, `LD_LIBRARY_PATH`/`DYLD_LIBRARY_PATH`, `GEM_HOME`/`GEM_PATH`, `CARGO_HOME`,
  `NP_LOCATION`, `PERL5LIB`, `SOURCE_DATE_EPOCH` — every one built by `filepath.Join`
  from tsuku-owned directories or `os.Environ()`, never from an unvalidated external
  string. The env vars tsuku *reads* that select an executable (`TSUKU_LLM_BINARY`,
  `internal/llm/addon/manager.go:124`) are the invoking user's own environment and
  cross no trust boundary.

So the honest answer to the lead is: **the class does not extend into argv.** It
extends into exactly two places on this side of the line — the four `sh -c` sites,
and about a dozen generated shell scripts. Findings below are confined to those.

## Findings

### F1. `.tsuku.toml` can point tsuku at an arbitrary GitHub repo for recipes, and the consent prompt hides which repo

**Trust level: hostile repository.** This is the strongest finding on this lead
because it is the only one where a value a hostile repo fully controls reaches a
shell without needing anything else to go wrong.

Chain:

1. `internal/project/config.go:79` loads `.tsuku.toml` from the project directory.
   Tool keys are parsed by `internal/project/orgkey.go:16` `SplitOrgKey`, which
   accepts `owner/repo` and `owner/repo:toolname` forms. The only validation is a
   `..` rejection and an "exactly two path segments" shape check — the owner and
   repo are otherwise arbitrary.
2. `cmd/tsuku/install_project.go:206` builds `qualifiedName := dArgs.Source + ":" + dArgs.RecipeName`
   and loads the recipe through the distributed provider
   (`cmd/tsuku/install_project.go:212`), which fetches
   `https://raw.githubusercontent.com/<owner>/<repo>/<branch>/.tsuku-recipes/<name>.toml`
   (`internal/distributed/provider.go:49-60`).
3. That recipe is an ordinary recipe. If it contains a `run_command` step, its
   `command` string is handed verbatim to `exec.CommandContext(ctx.Context, "sh", "-c", command)`
   at `internal/actions/run_command.go:96`. Arbitrary code execution as the user,
   at install time.

Two things make this worse than "you chose to trust that repo":

- **The confirmation prompt does not show the source.** `cmd/tsuku/install_project.go:129-132`
  sets `displayName = t.Distributed.RecipeName` — the *bare* name — precisely so
  org-scoped tools print "cleanly". A `.tsuku.toml` line reading
  `evil-org/evil-repo:node = "20.16.0"` renders in the tool list as `node@20.16.0`,
  and the prompt at line 165 is `Proceed? [Y/n]` with **empty input defaulting to
  yes**. The user is shown nothing that distinguishes this from installing `node`
  from the central registry.
- **Non-interactive runs skip the prompt entirely.** The gate is
  `if !installYes && isInteractive()` (`cmd/tsuku/install_project.go:164`), and
  `isInteractive()` is a stdin TTY check (`cmd/tsuku/install.go:402`). In CI, or
  under any wrapper that is not a terminal, `tsuku install` in a checked-out hostile
  repo runs the attacker's shell with no prompt at all.

The one mitigation present is `checkSourceCollision` (`cmd/tsuku/install_distributed.go:183`),
which prompts if the tool name is **already installed from a different source**. It
returns `nil` immediately when the tool is not installed (`:186-188`), so it does
nothing on a first install — which is the attack.

Secondary effect worth noting: `cmd/tsuku/install_project.go:216`
`loader.CacheRecipe(dArgs.RecipeName, r)` caches the fetched recipe under the *bare*
name for the rest of the process, so a second tool in the same `.tsuku.toml` that
depends on `node` resolves to the attacker's `node` recipe for that run.

**Severity: high impact, medium likelihood.** Arbitrary code execution from cloning
a repo and running `tsuku install`. Mitigated somewhat by the fact that installing
project-declared tools is inherently a trust act — but the prompt actively conceals
the one fact a user would need to make that trust decision.

### F2. The *resolved* version string is never charset-validated, and it reaches `sh -c`

**Trust level: upstream project (a GitHub release tag, an npm/PyPI/crates.io version
string) — outside the registry's control, though not "hostile repo you cloned".**

There are two version validators and neither covers the value that reaches the shell:

- `internal/install/pin.go:89` `ValidateRequested` is strict (letters, digits, `.`,
  `@`, `-` only; no `/`, no `..`) and is called at
  `internal/version/resolve.go:18`. But it validates the *requested constraint*, not
  the resolved result.
- `internal/version/transform.go:29` `ValidateVersionString` has the right charset
  (`^[a-zA-Z0-9._+\-@/]+$`, 128-char cap) for the resolved value — but it is only
  called from `TransformVersion` (`internal/version/transform.go:55`), and
  **`TransformVersion` has no non-test caller**. `grep -rn TransformVersion --include=*.go .`
  returns the definition and `transform_test.go` only. The validator is dead code.

The resolved version therefore flows unchecked from the provider:
`internal/version/resolver.go:159-163` takes `*release.TagName` from the GitHub API,
runs it through `normalizeVersion` (`internal/version/version_utils.go:9`, which only
strips prefixes) and stores it as `VersionInfo.Version`. That becomes `plan.Version`
(`internal/executor/plan_generator.go:222`) and `ExecutionContext.Version`
(`internal/executor/executor.go:560`).

Where it reaches a shell:

- `internal/actions/run_command.go:79-96`: `vars` from `GetStandardVars(ctx.Version, ctx.InstallDir, …)`
  (`internal/actions/util.go:28`), `command := ExpandVars(cmdPattern, vars)` (line 85),
  then `sh -c command` (line 96). Both `{version}` **and** `{install_dir}` carry it —
  `install_dir` is `$TSUKU_HOME/tools/<name>-<version>`, so a recipe never has to
  mention `{version}` to be affected. `recipes/p/pipx.toml:21` is a live example:
  `mkdir -p {install_dir}/bin && cp $HOME/.local/bin/pipx {install_dir}/bin/pipx`.
- `cmd/tsuku/verify.go:300-309`, `:384-392`, `:244-251`: `{version}` and
  `{install_dir}` substituted into the recipe's verify command, then `sh -c`. Here
  the version is read back from `state.json` (`toolState.ActiveVersion`), so it
  persists across runs.
- `internal/sandbox/executor.go:692-695`:
  `installDir := fmt.Sprintf("$TSUKU_HOME/tools/%s-%s", plan.Tool, plan.Version)`
  spliced unquoted into a generated `/bin/sh` script.

Git tag names permit `;`, `$`, backtick, `|`, `&`, `(`, `)`, `'`, `"` — everything
needed. So an upstream repo that publishes a release tagged `v1.0;curl … | sh` gets
shell execution inside any recipe that uses `run_command` or has a `verify` command.

**Severity: real but bounded.** The attacker here is the upstream project whose
binary the user is about to install and run anyway, so this is not a privilege
escalation in the common case. It matters in two narrower ones: it fires at *install*
time, before any checksum or signature verification would gate the artifact
(`RecipeHasVerification`, `internal/autoinstall/run.go:141-147`, is about whether the
recipe declares verification, not about the version string); and the same unvalidated
string is written into `state.json` and re-expanded by `tsuku verify` on later runs.
It is also the cleanest instance of the exploration's core question — an
externally-supplied value reaching text a shell evaluates — reaching this side of the
codebase, and the fix (call the validator that already exists, at the point the
provider returns) is small.

### F3. Generated wrapper scripts interpolate names without the guard one of them already has

**Trust level: upstream package (PyPI wheel, RubyGem, CPAN dist) or recipe registry,
depending on the site.** Same class as the known `FormatExports` `%q` defect: a value
written into a shell script body.

`internal/install/manager.go` does this correctly. `generateWrapperScript`
(`:701`) is guarded by `validateShellSafePath` (`:687`), which rejects
`\n \r " ' ` $ \ ;` before any path is spliced into the script. Since the paths land
inside double quotes, rejecting `$`, backtick, `\` and `"` is sufficient. This is the
model.

The other wrapper generators have no equivalent guard:

| Site | Interpolated | Quoting | Source of the value |
|---|---|---|---|
| `internal/actions/pip_exec.go:254`, `:265` | `exe` | inside `"…"` | filename in the venv's `bin/` — chosen by the PyPI wheel |
| `internal/actions/gem_common.go:12` (`gemWrapperTemplate`) | `exeName`, `rubyBinDir`, `gemHomeRel` | inside `"…"` | gem's `bin/` filename; `gem_install.go:112` rejects shell metacharacters in `executables`, but `gem_exec.go:504-506` passes names discovered from bundler's output with no such check |
| `internal/actions/cpan_install.go:288` | `exe`, `perlDir` | inside `"…"` | cpanm-generated script name |
| `internal/actions/nix_install.go:244`, `internal/actions/nix_realize.go:363` | `exe` | **unquoted**, as the argument to `nix shell … -c %s` | recipe `executables` parameter (registry) |
| `internal/actions/set_rpath.go:356` | `origBaseName` | inside `'…'` | basename of the binary being wrapped |

The `nix` pair is the sloppiest form (bare `%s` in command position) but takes a
registry-controlled value. The `pip`/`gem`/`cpan` trio takes a value an upstream
package controls but places it inside double quotes, so it needs a `$` or a backtick
in a filename to break out — possible on Linux, and a wheel install does not
otherwise execute code, so it is not purely theoretical. `set_rpath` uses single
quotes and breaks on an apostrophe in a filename.

**Severity: low.** In every case the attacker who controls the filename also controls
the binary the wrapper is about to exec, so the injection buys little. Worth fixing as
consistency with `validateShellSafePath` rather than as an exploit.

### F4. Flag injection: looked for it, did not find a reachable instance

This is the case the lead flagged as routinely missed, so here is what I checked and
why each is negative.

- **No `--` separator appears before an operand anywhere in the codebase**, and
  nothing rejects a leading `-`. So the *guard* is uniformly absent; what is also
  absent is a reachable value that could start with `-`.
- `internal/actions/gem_install.go:140-142` passes `"--version", ctx.Version` — the
  best-shaped candidate. A version beginning with `-` would be read by `gem` as a
  flag. `ValidateRequested` (`internal/install/pin.go:89`) does permit a leading `-`
  in a *requested* constraint, but the requested constraint is not what is passed
  here: `ctx.Version` is the resolved value, and resolution requires a match against
  a version the upstream registry actually publishes
  (`internal/version/resolve.go:33-48`, `internal/version/provider_github.go:159-190`).
  I could not construct a path to a resolved version starting with `-`.
- `cargo_install.go:114`, `npm_install.go:85` build `name@version`, so the version is
  never in leading position.
- `homebrew_relocate.go:801`, `meson_build.go:362`, `soname_scan.go:288` feed library
  paths **parsed out of `otool -L` output** back into `install_name_tool -change …`.
  A Mach-O load command whose path starts with `-` would land in flag position. The
  attacker controlling those load commands is the person who built the binary tsuku is
  about to install and the user is about to run, so there is nothing to gain.
- `internal/batch/orchestrator.go:370` runs `tsuku create <pkg.Name> --from <pkg.Source>`
  with values from a queue file. A `pkg.Name` beginning with `-` becomes a flag to
  `tsuku create`. This is maintainer batch tooling operating on a file the maintainer
  wrote; not a user-facing path.
- `internal/verify/dltest.go:180` runs `tsuku install <toolSpec>` with a recipe name
  from the registry; same shape, same dev-tooling caveat.

**Conclusion: no finding.** Adding `--` separators and a leading-`-` rejection at the
few argv positions that take a resolved version would be cheap hardening, but nothing
here is currently exploitable and it should not be scoped into this fix.

### F5. `tsuku run` executable selection: safe, with one path-shaped value that belongs to another lead

`internal/autoinstall/run.go:114`, `:202` build the exec target as
`filepath.Join(r.cfg.CurrentDir, command)` and hand it to `syscall.Exec` via
`execBinary` (`:206-212`). `command` is the name the user typed on the CLI, so a
traversal there is the user acting on their own behalf. Reached through a shim, the
name cannot contain `/` at all: the shim body is fixed
(`internal/shim/manager.go:18`) and passes `"$(basename "$0")"`, properly quoted.

One value on that path does cross a trust boundary but is a *path* sink, not an exec
sink: `internal/autoinstall/run.go:102` calls
`r.cfg.ToolBinDir(match.Recipe, version)` where `version` came from
`ProjectVersionFor` (`internal/project/resolver.go:56-81`) — i.e. straight out of a
hostile repo's `.tsuku.toml`, with no validation on that path (the strict
`ValidateRequested` sits in the *resolution* flow, which this fast path skips). That
is the known `ToolBinDir` defect with a second, hostile-controlled parameter.
**Handing this to the path-sinks lead**; noting it here only so it is not lost in the
seam between the two leads.

## Implications

The two known defects are not the whole set, but the extension on this side of the
line is narrow and the shape of the fix is already visible in the codebase. `sh -c`
appears four times, and the fix for F2 is to call a validator that already exists
(`internal/version/transform.go:29`) at the point providers return a version, rather
than to audit every interpolation. The fix for F3 is to lift
`validateShellSafePath` (`internal/install/manager.go:687`) out of the `install`
package and apply it in the five other wrapper generators. Neither requires touching
the 135 argv-only call sites.

F1 is a different animal and probably a different fix: it is not string handling at
all, it is a consent-UI defect. Whether it belongs in this exploration depends on
whether "values that originate outside the user's control" is read to include *which
repository a recipe comes from*. I would argue it does — `.tsuku.toml` is the hostile
input, the source field in it is unvalidated beyond a shape check, and the
consequence is `sh -c` — but it is worth an explicit call rather than a silent
inclusion, because it lands in `cmd/tsuku` rather than in a sink.

The registry-vs-hostile-repo distinction holds up cleanly here and should be stated
in the writeup: every `sh -c` in the codebase is fed a recipe-authored string, and
for the central registry that is by design, the same way a Homebrew formula is
allowed to run shell. Nothing in the recipe-execution path should be treated as a
defect merely because a hypothetical malicious registry recipe could abuse it.

## Surprises

`TransformVersion` and its validator are entirely dead. The project has the correct
version-charset check written, tested, and never wired up. Someone built the guard and
the call site never landed.

The complete absence of `git` was unexpected for a package manager and removes the
single richest source of findings this lead would normally produce.

The consent prompt deliberately strips the distributed source from the display, with
a comment saying it is "for cleaner output" (`cmd/tsuku/install_project.go:130`). The
concealment is intentional and cosmetic, which is how it survived review.

`gem_install.go:112` rejects shell metacharacters in executable names and
`cargo_install.go:90-95` rejects path separators — targeted defenses that exist in two
actions and nowhere else, including in `gem_exec.go`, which shares the wrapper
generator with the action that does check.

## Open Questions

- Does anything else resolve a project-config version without passing through
  `ResolveWithinBoundary`? The autoinstall fast path (F5) does; a sweep for other
  callers that take `ProjectVersionFor` output directly would settle whether F5 is one
  site or several.
- Is the distributed-recipe source list meant to be gated by an allowlist somewhere
  (a `[sources]` block, a system config)? `recordDistributedSource` and
  `checkSourceCollision` imply a TOFU model, but I found no allowlist and no
  first-install consent specific to the source.
- Should `install.ValidateRequested`'s permissive leading `-` be tightened? It is not
  currently reachable to a flag position, but it is the only reason F4 needed argument
  rather than a one-line dismissal.

## Summary

The class does not meaningfully extend into process execution: of 139 non-test `exec` call sites across 40 files, `argv[0]` is never externally supplied, only four pass a string to `sh -c`, there is no `git` invocation and no `LD_PRELOAD` anywhere, and I found no reachable flag-injection instance — so the argv surface is a genuine negative result, not an unexamined one. What does extend is two things: `.tsuku.toml` can name an arbitrary GitHub repo as a recipe source and the install prompt displays only the bare tool name, hiding it (`cmd/tsuku/install_project.go:129-132`, `:164-165`), and the *resolved* version string is never charset-validated because the validator that exists (`internal/version/transform.go:29`) has no non-test caller, letting an upstream tag name reach `sh -c` through both `{version}` and `{install_dir}`. The biggest open question is whether the consent-UI defect belongs in this fix at all, since it lives in `cmd/tsuku` rather than in a sink and its remedy is to show the source, not to escape a string.
