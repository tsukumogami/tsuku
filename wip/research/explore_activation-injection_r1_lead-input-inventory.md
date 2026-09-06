# Lead: What is the real inventory of attacker-controlled input, and how far does each reach?

All paths are relative to the repo root (`public/tsuku`). Line numbers are from the
`activation-injection` worktree.

## Findings

### Inventory 1 — a repository the user cloned

**The complete parsed schema of `.tsuku.toml` is two lines of Go.**

```go
type ProjectConfig struct {
	Tools map[string]ToolRequirement `toml:"tools"`   // internal/project/config.go:29
}
type ToolRequirement struct {
	Version string `toml:"version"`                  // internal/project/config.go:36
}
```

There is no env block, no scripts block, no registry override, no path field, no
lockfile reference, no hooks table. `ToolRequirement.UnmarshalTOML`
(`internal/project/config.go:44-62`) accepts a bare string or an inline table and reads
exactly one key, `version`. A table with extra keys is accepted and the extra keys are
discarded.

| Field | Type | Parsed at | Used for | Sinks it reaches | Validated? |
|---|---|---|---|---|---|
| `[tools]` map key (the tool name) | arbitrary TOML key -> Go `string` | `internal/project/config.go:29`, decoded at `:163` | tool identity everywhere downstream | **filesystem path component** (`cfg.ToolBinDir(name, ...)` at `internal/shellenv/activate.go:90`); **shell-evaluated text** (that path is joined into `PATH` at `:107` and emitted through `%q` at `:151`); **`os/exec` argument** via `runInstall(Tool: name)` at `cmd/tsuku/install_project.go:260` and `mgr.Install(recipeName)` at `cmd/tsuku/cmd_shim.go:85` | **No. Nothing validates it, anywhere.** |
| `[tools]` map value, string form or `version` in table form | `string` | `internal/project/config.go:44-62` | version pin | **filesystem path component** (`ToolBinDir(name, req.Version)`); **shell-evaluated text** (same `PATH` string) | Only in one consumer: `internal/updates/apply.go:36` calls `install.ValidateRequested`. **Not on the activation path.** |
| anything else in the file | — | — | — | nothing | silently discarded |

**What `parseConfigFile` actually enforces.** Only the count. `internal/project/config.go:167`:

```go
if len(cfg.Tools) > MaxTools {
	return nil, fmt.Errorf("parsing %s: declares %d tools, maximum is %d", path, len(cfg.Tools), MaxTools)
}
```

That is the entire validation body. The claim under test is correct.

**What `.tsuku.toml` parsing rejects today.** I ran the exact `UnmarshalTOML` and
`ProjectConfig` against BurntSushi/toml v1.5.0 (the pinned version, `go.mod:6`). Results:

| Input | Outcome |
|---|---|
| `registry = "http://evil"` alongside `[tools]` | **accepted**, key lands in `md.Undecoded()`, which nothing checks |
| `[scripts]` section | **accepted**, undecoded, ignored |
| `"../../../../tmp/x" = "1.0"` | **accepted**, key is the literal traversal string |
| `"foo$(id)" = "1.0"` | **accepted** |
| `"a\nb" = "1.0"` | **accepted**, key contains a real newline |
| `"" = "1.0"` | **accepted**, empty tool name |
| `node = "1$(id)"` | **accepted** (version with shell metacharacters) |
| `node = { version = "1", cmd = "x" }` | **accepted**, `cmd` discarded |
| `node = 3` | rejected: `toml: line 2 (last key "tools.node"): tool requirement must be a version string or a table, got int64` |
| `node = ["a"]` | rejected, same shape, `got []interface {}` |
| `node = { version = 3 }` | rejected: `version field must be a string, got int64` |
| `tools = "x"` | **accepted silently**, `Tools` is nil |

So the only rejects today are type errors raised from `UnmarshalTOML`, and they surface
as `fmt.Errorf("parsing %s: %w", path, err)` (`internal/project/config.go:164`).

**The error path.** `parseConfigFile` -> `LoadProjectConfig` returns `(nil, err)`
(`:96-98`). From there:

- `shellenv.ComputeActivation` wraps once more: `"loading project config: %w"`
  (`internal/shellenv/activate.go:50`), and `tsuku hook-env` returns it to cobra, which
  prints to stderr. The hook is `eval "$(tsuku hook-env bash)"`
  (`internal/hooks/tsuku-activate.bash:3`), so stdout is empty and the message is visible
  on stderr. Nonzero exit does not break the prompt.
- `cmd/tsuku/install_project.go:57` prints the error and exits `ExitGeneral`.
- `project.FindProjectDir` (`internal/project/config.go:118-124`) **swallows the error
  and returns `""`** — documented as such.

That is the reporting shape a validator should match: return the error from
`parseConfigFile`, one `parsing %s: ...` line naming the file. Note the counter-example
the fix must **not** copy: `ActivationResult.Skipped`
(`internal/shellenv/activate.go:25,117`) is populated on every skip and **no production
caller ever reads it** — only `activate_test.go` does. Adding rejects to `Skipped` would
be silently dropping them.

**Is `.tsuku.toml` the only repo-local file tsuku discovers?** Yes. I grepped every
dot-prefixed literal filename in `internal/` and `cmd/`. The only repo-relative discovery
walk is `LoadProjectConfig`. `.tsuku-recipes` (`internal/distributed/client.go:30`) is a
directory name used in **remote GitHub** URLs, never probed locally. The local recipe
provider is rooted at `$TSUKU_HOME/recipes` (`internal/recipe/provider_local.go:5`,
wired at `cmd/tsuku/main.go:122`), not at the project. `.tsuku-shims.json`,
`.last-check`, `.notified`, `.self-update.lock`, `.init-cache*` all live under
`$TSUKU_HOME`. No lockfile, no `.tsuku/` directory.

**One thing the schema hides: `.tsuku.toml` can name a remote registry.** A key with a
slash is an org-scoped entry. `cmd/tsuku/install_project.go:74` runs
`parseDistributedName` over every key, and `:107` calls `ensureDistributedSource` for
each distinct `owner/repo`, which registers the source in user config and then installs
from it (`:216-236`). So a cloned repo declaring
`"attacker/repo:tool" = "1.0"` is a cloned repo declaring *its own recipe registry*. The
gates are `validateRegistrySource` (`cmd/tsuku/registry.go:129`, `owner/repo` shape only)
and an interactive `Proceed? [Y/n]`. **The confirmation prompt hides the source**: line
`cmd/tsuku/install_project.go:130-132` sets `displayName = t.Distributed.RecipeName`, so
the user is asked to approve `tool@1.0` with no indication it comes from
`attacker/repo`. Worth flagging even though it is a UX-of-consent issue rather than a
composition bug.

### Inventory 2 — a recipe from the bundled or cached registry

Schema at `internal/recipe/types.go:14-20`: `metadata`, `version`, `resources`,
`patches`, `steps`, `verify`. The honest framing is that **a recipe is arbitrary code
execution by design** — `run_command` does `exec.CommandContext(ctx.Context, "sh", "-c",
command)` (`internal/actions/run_command.go`), and `Step.Params` is
`map[string]interface{}` (`internal/recipe/types.go:401`), so enumerating "which recipe
field reaches shell" is enumerating the contract. What matters for this exploration is
the small set of recipe fields that become **tsuku's own** filenames and paths, because
those persist beyond the install and are read by later, unrelated code.

| Field | Type | Parsed at | Used for | Sinks it reaches | Validated? |
|---|---|---|---|---|---|
| `metadata.name` | `string` | `internal/recipe/types.go:172` | tool identity, install dir | **filesystem path component**: `plan.Tool = e.recipe.Metadata.Name` (`internal/executor/plan_generator.go:342`) -> `effectiveToolName` -> `cfg.ToolDir(effectiveToolName, plan.Version)` (`cmd/tsuku/plan_install.go:123,130`) | **Weakly.** `validateMetadata` (`internal/recipe/validator.go:272-282`) checks only non-empty, no spaces, and warns on uppercase. It does **not** reject `/`, `..`, `$`, backtick, or newline. `IsValidRecipeName` exists (`internal/recipe/name.go:21`) and is **not** called here. |
| `metadata.name` (again) | `string` | same | shell.d fragment filename | **shell-evaluated text**: `envTargetName` builds `00-env-<name>` (`internal/actions/set_env.go:209-222`), the file is later read back and its basename is written **unquoted** into the sourced init cache at `internal/shellenv/cache.go:149-150` (`"# tsuku: " + toolName`) | **Partially.** `envTargetName` rejects `/`, `\`, `.`, `..`, `@`, and non-`filepath.Base` names — but **not a newline**. A name of `evil\nid` clears both `validateMetadata` and `envTargetName`. |
| `metadata.runtime_dependencies`, `extra_runtime_dependencies` | `[]string` | `types.go:178,180` | dependency names -> wrapper PATH, RPATH | path components, shell-visible paths | **Strictly.** `^[a-z0-9._-]+$` plus explicit `..`, `/`, `\`, NUL, leading-`-` and duplicate checks (`internal/recipe/validator.go:142-199`). This is the codebase's best-in-class name guard. |
| `metadata.dependencies`, `extra_dependencies` | `[]string` | `types.go:177,179` | install-time deps | same class of sinks | **Not** covered by `validateRuntimeDependencyNames` — only the two runtime lists are (`validator.go:261-262`). |
| `steps[].params.target` (`install_shell_init`) | `string` | `internal/actions/shell_init.go:138` | shell.d filename | **shell-evaluated text** (same cache path as above) | **Strictly**, `^[A-Za-z_][A-Za-z0-9._-]*$` (`shell_init.go:52-59`) |
| `steps[].params.*` (everything else) | `any` | `types.go:401` | per-action | up to and including `sh -c` | by design |
| `verify.command` | `string` | `types.go:851` | verification | exec | by design |
| `resources[].url`, `patches[].url` | `string` | `types.go:27,35` | downloads | network, then extraction paths | checksum-gated |

### Inventory 3 — a distributed recipe fetched from GitHub at runtime

**Can a user be pointed at an arbitrary repo?** Yes, three ways, in decreasing
deliberateness: `tsuku registry add owner/repo` (`cmd/tsuku/registry.go:139`),
`tsuku install owner/repo:tool`, and — the one that matters here — a `.tsuku.toml` key
containing a slash, auto-registered by `ensureDistributedSource` during
`tsuku install` with no args (see Inventory 1). Only the `owner/repo` shape is checked
(`discover.ValidateGitHubURL` plus `strings.Count(source, "/") != 1`,
`cmd/tsuku/registry.go:129-136`).

| Value | Source | Parsed at | Used for | Sinks it reaches | Validated? |
|---|---|---|---|---|---|
| `owner`, `repo` | user arg or `.tsuku.toml` key | `cmd/tsuku/install_distributed.go:37-76` | API and raw URLs; cache dir `$TSUKU_HOME/cache/distributed/{owner}/{repo}` | **filesystem path component** (`internal/distributed/cache.go:85`), URL components (`client.go:232`) | Yes at both ends: `parseDistributedName` rejects `..` (`install_distributed.go:43`), `validateRegistrySource` enforces shape, `CacheManager.repoDir` re-checks `..` and separators (`cache.go:80-84`) |
| recipe name after `:` in the `.tsuku.toml` key | cloned repo | `install_distributed.go:58-69` | `runInstall(Tool: dArgs.RecipeName)` (`install_project.go:230-236`), `loader.CacheRecipe` (`:227`) | **filesystem path component**, exec arg | Only the `..` check on the whole key. Not passed through `IsValidRecipeName`. |
| `entry.Name` from the GitHub Contents API listing of `.tsuku-recipes/` | **attacker's repo, a git filename** | `internal/distributed/client.go:272-280` | recipe key in `SourceMeta.Files`; cache filename `{name}.toml` and `{name}.meta.json` | **filesystem path component** (`cache.go:156,182,192`) | At use, by `validateRecipeName` (`cache.go:67-75`) — which wraps `IsValidRecipeName`, so `/`, `\`, `..`, NUL are out. **`$`, backtick, `:`, space, leading `-` and newline are all still allowed**, and a git filename can contain any of them. Nothing filters at listing time (`client.go:272-280` only requires a `.toml` suffix). |
| `manifest.layout`, `manifest.index_url` | attacker's repo, `manifest.json` | `internal/recipe/provider_unified.go:13-16`, fetched `provider.go:51-86` | store path layout; index fetch URL | path shaping; a URL | `index_url` is not validated at parse time |
| `download_url` per entry | Contents API | `client.go:274` | the actual fetch | network | Yes — host allowlist `raw.githubusercontent.com` / `objects.githubusercontent.com` (`client.go:20-23`, enforced at `client.go:167` and `:274`) |
| the whole recipe body | attacker's repo | `internal/recipe/loader.go` | everything in Inventory 2 | everything in Inventory 2 | Inventory 2's guards, which is the weak `metadata.name` check |

## Implications

**The authoritative list for the fix's threat model is three values, not one.**
`ComputeActivation` (`internal/shellenv/activate.go:42-119`) reads exactly three things
that a hostile repository controls, and emits all three:

1. **The `[tools]` map key.** Raw, unvalidated, straight into
   `cfg.ToolBinDir(name, req.Version)` at `:90`, which is
   `filepath.Join(ToolsDir, name+"-"+version, "bin")`
   (`internal/config/config.go:432-439`). Reaches a path component and then, via
   `strings.Join(binDirs, ":")` at `:107`, reaches the `PATH` string that
   `FormatExports` emits. Gated by `os.Stat` at `:91` — the composed directory must
   exist — so exploiting the path half needs an existing target.
2. **`req.Version`.** Equally raw on this path. `install.ValidateRequested` exists and is
   already applied to this exact field in one other consumer
   (`internal/updates/apply.go:36`), which makes the omission here an inconsistency
   rather than an oversight in design. Same `os.Stat` gate.
3. **`result.Dir`** — the directory containing the `.tsuku.toml`. Emitted as
   `export _TSUKU_DIR=%q` (`internal/shellenv/activate.go:151`) **with no `os.Stat` gate
   and no validation of any kind**, on every activation, whether or not a single tool is
   installed. A hostile repository controls this: git tracks any byte but NUL and `/` in
   a path component, so a repo can ship a subdirectory named `a$(...)`, or `` a`...` ``,
   containing a `.tsuku.toml`. `git clone` creates it; `cd` into it fires
   `eval "$(tsuku hook-env bash)"` (`internal/hooks/tsuku-activate.bash:3`).

Point 3 is the one that changes the fix. **A validator that covers only the tool name
does not cover the activation path**, because `Dir` bypasses the composition entirely and
reaches the `%q` emitter directly. It also cannot be fixed by validating an input — `Dir`
is a legitimate absolute path that simply happens to contain shell metacharacters, so the
only correct fix for it is at the emitter (real shell quoting instead of `%q`). That
argues the two known defects are not independent: the name defect is fixable with a
validator, the `Dir` defect is only fixable by fixing `%q`, and fixing `%q` alone would
also neutralise the shell half of the name defect. The validator is still needed for the
path half.

**The class shape holds, and it is symmetric.** The exploration's hypothesis is "a guard
written against the input someone was thinking about rather than against the
composition." Both polarities are present in the same codebase, sometimes in the same
filename:

- `filepath.Join(ToolsDir, name+"-"+version)` — `install.ValidateVersionString` is
  applied to `version` at `internal/install/manager.go:428` before `Activate` composes a
  path, and `name` is never checked. **Guard on version, none on name.**
- `share/shell.d/{target}@{version}.{shell}` — `validateShellDTarget` applies a strict
  regex to `target` (`internal/actions/shell_init.go:52-59`) and `shellDVersion`
  (`shell_init.go:215-220`) returns `ctx.Version` with no check at all. **Guard on name,
  none on version.** Exact inverse of the first, same repo, same filename family.
- `00-env-{name}@{version}.{shell}` vs `{target}@{version}.{shell}` — two writers into
  one filename namespace with two different name guards, one a regex
  (`shell_init.go:52`) and one an ad hoc list (`set_env.go:213-221`) that misses newline.

**Consumers of the `[tools]` key beyond activation**, all reached from the same hostile
file: `cmd/tsuku/install_project.go:72` (install), `cmd/tsuku/cmd_shim.go:78` (shim
install, key -> `mgr.Install(recipeName)`), `internal/updates/apply.go:36` (auto-apply,
the only one that validates anything), `internal/project/resolver.go:68` (safe — matches
against index-supplied `m.Recipe`, and the index rebuild does call `IsValidRecipeName`
at `internal/index/rebuild.go:180`). A validator placed in `parseConfigFile` covers all
of them at once; a validator placed in `ComputeActivation` covers one.

## Surprises

**`shellenv` never calls `SplitOrgKey`.** `internal/project/orgkey.go` exists precisely to
decompose org-scoped `.tsuku.toml` keys, and `resolver.go:25` uses it — but
`activate.go:73` iterates the raw map keys. So a legitimate `tsukumogami/koto = "1.0"`
entry makes activation compute `ToolBinDir("tsukumogami/koto", "1.0")` =
`tools/tsukumogami/koto-1.0/bin`, while the installer put the tool at `tools/koto-1.0`.
The directory never exists, the tool is appended to the unread `Skipped` slice, and the
user gets a project tool that silently does not activate. That is a plain functional bug
sitting on top of the security one, and it means `/` currently reaches `filepath.Join`
through a *supported* config syntax, not only through a malicious one.

**`ActivationResult.Skipped` is write-only in production.** Set at `activate.go:117`,
asserted in `activate_test.go`, read by nothing else. Every silent skip today —
unpinned version, missing directory, `filepath.Abs` failure — is genuinely silent. The
fix cannot report rejects through this field without also giving it a reader.

**`tools = "x"` parses clean.** A scalar where the table belongs yields a nil map and no
error, so a typo'd config silently activates nothing. Unknown top-level keys land in
`md.Undecoded()` and nothing inspects it, so `.tsuku.toml` accepts arbitrary extra
content today. If the schema is ever extended, there is no strict-mode gate to flip.

**The registry-add consent prompt hides the registry.** `install_project.go:130-132`
deliberately shows the bare recipe name "for cleaner output", so the `Proceed? [Y/n]`
list for a hostile `.tsuku.toml` reads `Tools: build-helper@1.0` with no mention of
`attacker/repo`. The source registration warning at `:109` only fires on *failure*.

**`metadata.name` has three different validators depending on where it lands** — a
near-empty one in the recipe validator, a moderate one in `set_env`, and none at all in
the `ToolDir` composition — while `runtime_dependencies`, which is the same kind of
string, gets the strictest treatment in the repo. The strictness is inverted relative to
the trust level of the source.

## Open Questions

- `cmd/tsuku/plan_install.go:30-32` uses `plan.Tool` (i.e. `metadata.name`) only when
  `toolName` is empty. I did not establish which install entry points leave it empty, so
  I cannot say how reachable the `metadata.name` -> `ToolDir` path is in practice.
  Belongs to the path-sinks lead.
- Whether `%q` output is actually exploitable in `fish` (`activate.go:146-148`) as well
  as bash/zsh — fish expands `$var` inside double quotes but handles `$(...)` and
  backticks differently. Belongs to the shell-sinks lead.
- `manifest.index_url` from an attacker's repo is unvalidated at parse time
  (`provider_unified.go:15`); I did not trace whether the eventual fetch re-applies the
  `allowedDownloadHosts` allowlist that `FetchRecipe` uses.
- Windows: `ToolBinDir` composition and the `:` PATH separator differ, and
  `filelock_windows.go` suggests Windows is supported. Not examined.

**Contradicting a scope assumption:** the brief frames the fix as needing a validator
covering "the authoritative list of values that a hostile repository controls and that
reach `internal/shellenv` activation." That list includes `result.Dir`, which is not an
input a validator can reject — it is the user's own directory path. The fix therefore
cannot be validator-only.

## Summary

`.tsuku.toml` has exactly one section, `[tools]`, whose keys are arbitrary strings and whose values are a single optional `version` string; `parseConfigFile` enforces only `len(cfg.Tools) > MaxTools`, rejects only TOML type errors (as `parsing <path>: ...`), and silently accepts traversal sequences, shell metacharacters, newlines, empty keys, and unknown top-level sections, and `.tsuku.toml` is the only repo-local file tsuku discovers.

`ComputeActivation` consumes three hostile-controlled values, not one: the tool name and the version both reach `filepath.Join` and the emitted `PATH` behind an `os.Stat` gate, and `result.Dir` — the directory holding the config, whose name a cloned repo controls — reaches `export _TSUKU_DIR=%q` with no gate and no validation at all, so a name-only validator does not cover the activation path.

The class hypothesis holds in both directions: `manager.Activate` validates the version and not the name of the same composed path, while `shell.d` filenames validate the target with a regex and the version not at all, and `ActivationResult.Skipped` is populated but read by no production caller, so the fix must not report rejects through it.
