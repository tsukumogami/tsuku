# Lead: What validation exists today, what does each guard actually cover, and where does a guard stop short of the composition or the consumer it appears to protect?

All paths are relative to the repo root
`public/tsuku/.claude/worktrees/activation-injection`. Everything marked
DEMONSTRATED was reproduced against a binary built from this tree
(`go build ./cmd/tsuku`) with a throwaway `TSUKU_HOME` under the job scratch
directory; nothing was written into the repo and the scratch tree was removed.

---

## Findings

### 0. The headline: this is a class, not two instances

The two known defects are not the set. `.tsuku.toml` is parsed with **exactly
one** semantic check (`MaxTools`), and the tool key it yields flows unvalidated
into at least four distinct sink families: a filesystem path (`ToolBinDir`), a
registry **URL** path, a registry **disk-cache write** path, and shell-evaluated
`export` text. Meanwhile the codebase contains at least eleven guards that
would have caught it, three of which live in the same two packages as the
sinks. The pattern is consistent enough to name: **tsuku validates at the point
where a developer was thinking about a specific input, never at the boundary
where untrusted data enters.**

Two corrections to the brief's stated premises, both load-bearing:

1. **Shape A is understated for activation.** The brief says the version
   component of `ToolBinDir` is guarded and the name is not. At the activation
   call site (`internal/shellenv/activate.go:90`) **neither** component is
   guarded. `ValidateRequested` (`internal/install/pin.go:89`) and
   `install.ValidateVersionString` (`internal/install/state.go:413`) are never
   called on any path that reaches activation. The version guards are real, but
   they live in install/update/remove; activation is a different consumer, so
   Shape A at this call site is really Shape B twice over. DEMONSTRATED below.

2. **`SplitOrgKey` does not reject `..` universally.** It rejects `..` only for
   keys that contain `/`. `internal/project/orgkey.go:17-19` returns early for
   any key without a slash, **before** the `..` check at line 22. So
   `SplitOrgKey("..")` returns `("", "..", false, nil)` — no error. The brief's
   description holds for the traversal cases that matter (which all need `/`),
   but the guard is not the universal `..` rejector it reads as, and a fix that
   leans on it must not assume otherwise.

There is also a third correction worth carrying: `SplitOrgKey`'s **only** call
site (`internal/project/resolver.go:35`) **discards its error**. Lines 36-38 are
`if err != nil || !isOrg { continue }`. So even the one consumer that calls the
guard does not reject on it — a traversal key is silently skipped from the
reverse map and left intact in `config.Tools` for every other consumer to read.
The guard has zero enforcement anywhere in the binary today.

---

### 1. Guard inventory

Only guards in scope: those whose bypass reaches a filesystem path component or
shell-evaluated text. "Sink reached" answers *what the bypassing consumer hits*.

| # | Guard (file:line) | Rejects | Data guarded | Call sites | Consumers of the same data that do NOT call it | Bypass reaches |
|---|---|---|---|---|---|---|
| G1 | `internal/project/orgkey.go:16` `SplitOrgKey` | `..` **only in keys containing `/`**; malformed `owner/repo` | `.tsuku.toml` tool keys | `internal/project/resolver.go:35` — **and the error is discarded** (`resolver.go:36-38`) | `internal/shellenv/activate.go:73-90` (activation); `cmd/tsuku/install_project.go:72-82` (`tsuku install`); `cmd/tsuku/cmd_shim.go:78` (`tsuku shim install`); `internal/updates/apply.go:36` | FS path, URL path, disk write, shell text |
| G2 | `internal/recipe/name.go:21` `IsValidRecipeName` | `""`, `/`, `\`, `..`, NUL | recipe names before path/URL construction | `internal/distributed/cache.go:68` (via `validateRecipeName`, used at :152, :173, :221); `internal/index/rebuild.go:180`; `internal/recipe/validator.go:190` | `internal/recipe/provider_unified.go:271` `recipePath`; `internal/registry/registry.go:86` `recipeURL` / `:95` `cachePath`; `internal/registry/cache_manager.go:198`; `internal/config/config.go:432` `ToolDir` | FS read (`backing_store.go:69`), FS write (`disk_cache.go:139-149`), URL path |
| G3 | `internal/install/symlink.go:46` `ValidateSymlinkTarget` | target outside `$TSUKU_HOME/tools/` (with trailing-separator guard) | a composed tool path | `internal/install/manager.go:527` | `internal/shellenv/activate.go:90-101` — the *exact* containment predicate activation needs, exported, never called | FS path prepended to `PATH` |
| G4 | `internal/install/manager.go:687` `validateShellSafePath` | `\n \r " ' \` $ \ ;` in a path about to be interpolated into a shell script | a composed tool path headed for shell text | `manager.go:591`, `:615`, `:676` | `internal/shellenv/activate.go:107` + `FormatExports` at `:134-152` — same data class (a `ToolBinDir` result), same sink class (shell-evaluated text), no check | shell-evaluated text |
| G5 | `internal/install/state.go:413` `install.ValidateVersionString` | `..`, `/`, `\` | version strings before path joins | `internal/install/remove.go:123`; `internal/install/manager.go:429`; `internal/updates/gc.go:85` | `internal/shellenv/activate.go:90`; `internal/autoinstall/run.go:102`; `internal/config/config.go:432` itself | FS path |
| G6 | `internal/version/transform.go:29` `version.ValidateVersionString` | length > 128; charset outside `^[a-zA-Z0-9._+\-@/]+$` — **note `/` and `.` are allowed, so it does NOT reject `../../x`** | version strings | `transform.go:55` only (internal to `TransformVersion`) | everything else | n/a (see Surprise S3) |
| G7 | `internal/install/pin.go:89` `ValidateRequested` | non-`[alnum . @ -]`, `..`, `/`, `\` | the `Requested` pin string | `internal/updates/apply.go:37`; `internal/version/resolve.go:18` | `internal/shellenv/activate.go:82-90` reads the same `.tsuku.toml` version value and never calls it | FS path |
| G8 | `cmd/tsuku/install_distributed.go:37` `parseDistributedName` | `..` — by **returning nil**, i.e. falling through rather than erroring | CLI args and `.tsuku.toml` keys | `cmd/tsuku/install_project.go:74`; `cmd/tsuku/install.go` (CLI arg path) | — (it is called on both paths) | see F2: rejection-by-fallthrough hands the name to the *unguarded* standard install path |
| G9 | `internal/recipe/loader.go:139` `splitQualifiedName` | malformed `owner/repo` qualifier — **no `..` check at all** | qualified recipe names `owner/repo:recipe` | `internal/recipe/loader.go:75` | — | the bare half goes to `provider.Get()`; contained today only because `GitHubBackingStore.Get` (`internal/distributed/backing_store.go:33-45`) resolves through a map lookup |
| G10 | `internal/notices/notices.go:211` `validateNoticeName` | `""`, `..`, `/`, `\`, stray `--` | notice filenames | `notices.go:92` (write), `:183` (remove) | — complete for its data | — |
| G11 | `internal/actions/shell_init.go:55` `validateShellDTarget` (`^[A-Za-z_][A-Za-z0-9._-]*$`) | anything outside the pattern | shell.d fragment filenames | `shell_init.go:102`, `:142` | — complete for its data | — |
| G12 | `internal/actions/data_dir.go:44` and `internal/actions/set_env.go:216` (`name != filepath.Base(name) \|\| name == "." \|\| name == ".." \|\| ContainsAny(name, "/\\")`) | non-single-segment recipe names | `Recipe.Metadata.Name` before a path join | their own call sites | `config.ToolDir` / `ToolBinDir`, which join the same recipe name | FS path |
| G13 | `internal/recipe/validator.go:155` `validateRuntimeDependencyNames` (`^[a-z0-9._-]+$` + `..`, `/`, `\`, NUL, leading `-`, then `IsValidRecipeName` again) | as listed | `runtime_dependencies` entries | `validator.go` | — complete for its data | — |

G10–G13 are the guards that are *right*: applied at the point the value is
minted or loaded, complete for their data, no second consumer. They are the
existence proof that the codebase knows how to do this — which makes G1–G5 a
placement failure, not a knowledge gap.

---

### 2. The bypass call paths (evidence)

#### F1 — Activation: both components of `ToolBinDir` unvalidated (DEMONSTRATED)

Path: `cmd/tsuku/hook_env.go:36` → `shellenv.ComputeActivation`
(`internal/shellenv/activate.go:42`) → `project.LoadProjectConfig`
(`internal/project/config.go:79`) → `parseConfigFile` (`:156`, MaxTools only) →
loop at `activate.go:73-102` iterating raw map keys → `cfg.ToolBinDir(name,
req.Version)` at `:90` → `filepath.Join(ToolsDir, name+"-"+version, "bin")`
(`internal/config/config.go:432-438`) → `os.Stat` → `filepath.Abs` →
`strings.Join(binDirs, ":") + ":" + basePath` at `:107` → `FormatExports` at
`:123`.

Nothing between the TOML decode and the `Join` inspects either string.

Name component, demonstrated with a benign marker directory:

```toml
[tools]
"../MARKER" = "v1"
```

`tsuku hook-env bash` emitted, as the first PATH entry:

```
export PATH="$TSUKU_HOME/MARKER-v1/bin:…"
```

`$TSUKU_HOME/MARKER-v1/bin` is outside `$TSUKU_HOME/tools/`. Additional `..`
segments climb further; the only constraint is that the directory must exist,
because of the `os.Stat` at `activate.go:91`.

Version component, same harness:

```toml
[tools]
T = "1/../../VERMARKER"
```

emitted `$TSUKU_HOME/VERMARKER/bin` as the first PATH entry. So the version
escapes too. This is what refutes the "version is guarded" half of Shape A **at
this call site**: G5 and G7 exist but neither is on this path.

This directly falsifies both halves of the mitigation claim at
`docs/designs/current/DESIGN-shell-env-activation.md:419` ("All paths
constrained to `$TSUKU_HOME/tools/`, name validation"). There is no name
validation, and paths are not constrained. The residual-risk column ("Tool names
with unusual characters could construct unexpected paths") describes the actual
defect and grades it Low.

#### F2 — `tsuku install`: rejection-by-fallthrough (DEMONSTRATED)

`cmd/tsuku/install_project.go:74` calls `parseDistributedName` (G8), which
returns `nil` for any name containing `..` (`install_distributed.go:42-45`). Its
own comment says such names "fall through to the regular recipe lookup path and
produce a not-found error". So the traversal name is not rejected — it is
handed to the **standard** install path at `install_project.go:256-262` as
`installArgs{Tool: "../../evil"}`. `installWithDependencies`
(`cmd/tsuku/install_deps.go:233`) does not validate `args.Tool` either.

The name then reaches `RegistryProvider.recipePath`
(`internal/recipe/provider_unified.go:271-279`), which builds
`string(name[0]) + "/" + name + ".toml"`, and that string is used as both the
HTTP path and the disk-cache key.

Demonstrated with `.tsuku.toml`:

```toml
[tools]
"../../evil" = "1.0.0"
T2 = "1/../../X"
".." = "9"
```

`tsuku install --dry-run` printed:

```
Tools: ..@9, ../../evil@1.0.0, T2@1/../../X

Dry-run completed with 3 tools failing to resolve.
  ..: recipe not found: HTTP 404 from https://raw.githubusercontent.com/tsukumogami/tsuku/main/recipes/...toml
  ../../evil: recipe not found: HTTP 404 from https://raw.githubusercontent.com/tsukumogami/tsuku/evil.toml
  T2: recipe not found: HTTP 404 from https://raw.githubusercontent.com/tsukumogami/tsuku/main/recipes/T/T2.toml
```

Note the second line: the constructed URL climbed out of `main/recipes/` to
`/tsukumogami/tsuku/evil.toml`. Pushing that further, still benign and still
404:

```toml
[tools]
"../../../../example-nonexistent-owner/reg/main/tool" = "1"
```

produced `HTTP 404 from https://raw.githubusercontent.com/example-nonexistent-owner/reg/main/tool.toml`.

That is full control of the owner, repo, ref and path segments of the
central-registry fetch, from a file checked into a repository. The host does not
change, so this is not SSRF; it is **recipe substitution** — a `.tsuku.toml` can
point the "central registry" at any GitHub repo. A recipe that resolves is then
parsed and executed by the normal install machinery.

#### F3 — The same key is the registry disk-cache write key (code-established)

`HTTPStore.Get` (`internal/recipe/http_store.go:80-81`) sets `key := path`,
where `path` is `recipePath(name)` from F2, and on a 200 the content is written
via `DiskCache.Put` (`internal/recipe/disk_cache.go:139-149`):
`contentPath := filepath.Join(c.dir, key)`, then `os.MkdirAll(filepath.Dir(...))`
and `os.WriteFile(contentPath, data, 0644)`. `contentPath` at
`disk_cache.go:305-307` is a bare `filepath.Join` with no containment check.

So the same traversal that redirects the fetch also redirects where the fetched
bytes land: attacker-chosen path, attacker-chosen content, created with
`MkdirAll`. I proved the key carries the traversal (the URL in F2 is derived
from the identical string) but did not land a 200 in the harness — outbound
`curl` is gated in this environment and the benign 200 target I picked
(`recipes/g/go.toml` on `main`) also 404'd, so the write itself is
code-established rather than executed. The legacy `internal/registry` path has
the identical shape: `Registry.cachePath` (`registry.go:90-96`) →
`Registry.CacheRecipe` (`:338-352`) → `MkdirAll` + `WriteFile`. Its one guarded
caller is `internal/index/rebuild.go:180`, which calls `IsValidRecipeName` first
(G2) — the other callers, in `internal/registry/cached_registry.go:204-222`, do
not.

#### F4 — Local-registry read traversal (code-established)

`FSStore.Get` (`internal/recipe/backing_store.go:68-70`) is
`os.ReadFile(filepath.Join(s.dir, path))` with no containment check, fed the
same `recipePath(name)`. `Registry.fetchLocalRecipe`
(`internal/registry/registry.go:118-120`) is the same shape via `recipeURL`.
This is precisely the case `IsValidRecipeName`'s doc comment names — "prevent
path traversal in local-registry deployments" — and neither path calls it. The
read is contained to files that parse as recipe TOML, so it is closer to an
existence-and-content oracle than a general file read, but the traversal is
unbounded.

#### F5 — `tsuku shim install` (code-established)

`cmd/tsuku/cmd_shim.go:78` collects raw `.tsuku.toml` keys and passes each to
`shim.Manager.Install` (`internal/shim/manager.go:53`), which calls
`loader.Get(recipeName, …)` — the F2/F4 sinks again. The shim *filenames* come
from the recipe rather than the key and pass through `filepath.Base`
(`manager.go:202`), so they are contained — but the filter at `manager.go:203`
skips `"."` and `"/"` and **not** `".."`, so a recipe declaring a binary named
`..` yields `filepath.Join(binDir, "..")` = `$TSUKU_HOME`, and `os.WriteFile`
onto a directory fails. Harmless as written, one condition away from not being.

#### F6 — `internal/updates/gc.go`: Shape A inside an explicit guard comment

`gc.go:85` validates `version` with `install.ValidateVersionString` and the
surrounding comment (lines 76-84) reasons carefully about *which* of the two
same-named validators applies. Nine lines later, `:93`:

```go
dirPath := filepath.Join(toolsDir, toolName+"-"+version)
```

`toolName` is never validated, and the sink is `os.RemoveAll`. The developer
was thinking about the version — literally writing a paragraph about it — and
composed it with an unchecked name in the next statement. This is the purest
specimen of Shape A in the tree.

---

### 3. Shape A: compositions where one part is validated and the other is not

| Composition (file:line) | Guarded part | Unguarded part | Sink |
|---|---|---|---|
| `internal/config/config.go:432` `ToolDir` = `Join(ToolsDir, name+"-"+version)` | version, **only for callers in `install`/`updates`** (G5) | `name`, always; and version too on the `shellenv` and `autoinstall` paths | FS path → `PATH` entry, `os.RemoveAll`, install target |
| `internal/updates/gc.go:93` | `version` (G5, `:85`) | `toolName` | `os.RemoveAll` |
| `internal/recipe/provider_unified.go:274-276` `recipePath` | — | `name` (both the `string(name[0])` group letter and the name itself) | HTTP URL path + disk-cache write key |
| `internal/registry/registry.go:86` `recipeURL` / `:95` `cachePath` | callers via `index/rebuild.go:180` only | `name` for every other caller | URL path; `MkdirAll`+`WriteFile` |
| `internal/registry/cache_manager.go:198` `deleteEntry` | — | `name` (via `firstLetter(name)` + `name+".toml"`) | `os.Remove` |
| `internal/shim/manager.go:70,79` | binary names via `filepath.Base` | `".."` survives the `manager.go:203` filter | `os.WriteFile` |
| `internal/install/manager.go:614-616` | the **composed** `depBinDir` is checked with `validateShellSafePath` (G4) — the one place the composition itself is validated | — | shell script text |
| `internal/shellenv/activate.go:90,107` | — | both `name` and `version`; and the composed path is checked by neither G3 nor G4 | `PATH` + shell-evaluated `export` text |

`manager.go:614-616` is worth calling out as the counter-example: it is the only
place in the tree that validates the *output* of `ToolBinDir` rather than its
inputs, and it does so immediately before shell interpolation. `shellenv` does
the same composition for the same purpose and validates neither.

---

### 4. Question 1 — Which guards run at parse/load time, which at use time?

**Parse/load time (cover every downstream consumer): four.**

- `internal/project/config.go:167` — `MaxTools`. The only `.tsuku.toml`
  parse-time check, and it is a resource bound, not a validity check.
- `internal/recipe/validator.go:155` (G13) — recipe `runtime_dependencies`, at
  recipe-load validation.
- `internal/index/rebuild.go:180` (G2) — validates names **at the point they are
  written into the binary index**. This is the model: because the index write
  boundary is guarded, every index *reader* — including
  `internal/autoinstall/run.go:102`, which feeds `match.Recipe` straight into
  `ToolBinDir` — inherits the guarantee without knowing it exists.
- `internal/distributed/cache.go:152/173/221` (G2) — at the cache boundary.

**Use time (cover exactly one consumer): everything else.** G1, G3, G4, G5, G6,
G7, G8, G9, G11, G12, and `internal/actions`' many local `..` checks are all
called at the sink by one caller. G10 (`notices`) is use-time but happens to
have complete coverage because its data has only two consumers and both call it.

The split maps onto the blast radius exactly. The index is the only untrusted
data source in the tool-name flow that is guarded at its boundary, and it is the
only one with no known bypass. `.tsuku.toml` is guarded at no boundary and has
bypasses into four sink families.

**This answers the placement question.** "Add a check to activation" would fix
F1 and leave F2, F3, F4 and F5 live, because those are four other consumers of
the same map. The fix belongs at `parseConfigFile`
(`internal/project/config.go:156-171`) — the single funnel every consumer goes
through, since all five entry points reach the map only via
`LoadProjectConfig`. A parse-time check is also the only placement that makes
`ConfigResult.Config.Tools` safe to hand out as a plain `map[string]…`, which
`Resolver.Tools()` (`resolver.go:87-92`) already does.

---

### 5. Question 2 — Is `MaxTools` really all that `.tsuku.toml` parsing enforces?

Yes. The parse path end to end is `LoadProjectConfig`
(`internal/project/config.go:79`) → symlink resolution and the ceiling walk →
`parseConfigFile` (`:156`) → `os.ReadFile` → `toml.Decode` → the `MaxTools`
check at `:167` → return.

**Rejected today:** unreadable file (`:159`); TOML that does not decode
(`:163`); a `[tools]` entry that is neither a string nor a table, or a table
whose `version` is a non-string — both from `ToolRequirement.UnmarshalTOML`
(`:44-61`), which surfaces as a decode error; more than 256 entries (`:167`);
and a `startDir` whose symlinks will not resolve (`:80`).

**Not rejected:** anything about the key. No length bound, no charset, no
path-segment check, no `..` check, no absolute-path check, no NUL check, no
control characters, no check that an org-scoped key is well formed. Nothing
about the version value either: no charset, no length, no `..` — `MaxVersionLength`
(`internal/version/transform.go:12`) is not on this path. Duplicate keys are
TOML's problem, and the empty version string is explicitly allowed (activation
skips it at `activate.go:83`).

**Error path shape.** All four rejections return `(nil, error)` wrapped as
`fmt.Errorf("parsing %s: …", path, …)` or `"reading %s: …"`, and the error
propagates whole-file: one bad entry fails the entire config. Callers differ in
how loudly:

- `cmd/tsuku/install_project.go:56-59` — `printError` then
  `exitWithCode(ExitGeneral)`.
- `cmd/tsuku/cmd_shim.go:69-72` — message to stderr, `ExitGeneral`.
- `internal/shellenv/activate.go:49-51` — wrapped as `"loading project config:
  %w"` and returned; `cmd/tsuku/hook_env.go` surfaces it.
- `cmd/tsuku/cmd_run.go:95` — `projectCfg, _ := project.LoadProjectConfig(cwd)`.
  **The error is discarded.** `tsuku run` treats an unparseable config as no
  config.
- `internal/project/config.go:118-124` — `FindProjectDir` discards the error by
  documented design.

So a fix that rejects bad keys at parse time inherits an existing, consistent
whole-file-error shape for three of five consumers, and will be silently
swallowed by the other two. Two things follow. First, **match the existing
shape**: `fmt.Errorf("parsing %s: tool %q …", path, key, reason)`, returned from
`parseConfigFile`, listing every offending key rather than the first — the
`MaxTools` message is a one-liner because there is only one way to trip it,
whereas a charset rejection wants to name the keys. Second, the brief's
requirement that the fix "report rejects rather than silently skip them" is at
risk at `cmd_run.go:95` regardless of what `parseConfigFile` returns; that
`_` needs to become a real error path, or `tsuku run` will keep degrading to
"no project config" on a rejected file.

Also note `buildBareToOrgMap`'s `continue` on error (`resolver.go:36-38`) is the
existing precedent for *silent skip*, and it is precisely the behaviour that let
this class survive. It should become an error path or be deleted in favour of
the parse-time check.

---

### 6. Question 3 — Would validating the derived bare name at parse time break any legitimate config?

**No.** Confirmed against the whole registry.

**The org-key format.** From `SplitOrgKey`
(`internal/project/orgkey.go:8-14`) and
`docs/designs/current/DESIGN-org-scoped-project-config.md`, the four legal key
shapes are:

| Key | source | bare |
|---|---|---|
| `node` | `""` | `node` |
| `tsukumogami/koto` | `tsukumogami/koto` | `koto` |
| `tsukumogami/registry:tool` | `tsukumogami/registry` | `tool` |
| `owner/repo:tool@1.2.3` | `owner/repo` | `tool` (the `@version` suffix is stripped at `:28-30`) |

A naive "reject `/` in the key" would break rows 2–4, which the design
explicitly blesses (quoted TOML keys, `DESIGN-org-scoped-project-config.md:11`).

**The correct check** is the one the brief proposes, and it holds:

1. Call `SplitOrgKey(key)`. **Propagate its error** — do not `continue`. This
   catches malformed sources and `..` in slash-containing keys.
2. Independently reject `..` for the no-slash case, because `SplitOrgKey` returns
   early at `orgkey.go:17-19` before its own `..` check. Falling out of step 3
   is sufficient in practice, but the dependency should be explicit rather than
   incidental.
3. Validate the returned `bare` as a single safe path segment. Reuse
   `runtimeDepNamePattern` (`internal/recipe/validator.go:142`), `^[a-z0-9._-]+$`,
   which is already the codebase's strict recipe-name charset — plus
   `recipe.IsValidRecipeName` for belt and braces, exactly as
   `validateRuntimeDependencyNames` does at `validator.go:189-192`.

Step 3 is load-bearing for a reason step 1 does not cover: `SplitOrgKey` accepts
a colon-suffix containing slashes. `owner/repo:sub/dir/tool` splits cleanly —
`SplitN(source, "/", 3)` at `orgkey.go:41` only inspects the *source* half — and
yields `bare = "sub/dir/tool"`. No `..`, so step 1 passes it; only a charset
check on the bare name stops it.

**Is the bare name what reaches `ToolBinDir`? Partly — and this is a real
subtlety for the fix.**

- **Activation: no.** `internal/shellenv/activate.go:73-90` iterates raw map
  keys and passes the **full key**, org prefix and all, to `ToolBinDir`. It
  never calls `SplitOrgKey`. So today an org-scoped declaration produces
  `$TSUKU_HOME/tools/tsukumogami%2Fkoto-1.0.0/bin`… no, literally
  `tools/tsukumogami/koto-1.0.0/bin` — a nested directory that will never exist,
  so `os.Stat` fails and the tool is silently added to `Skipped`. **Org-scoped
  tools do not activate at all today.** That is a functional bug hiding behind
  the security one, and validating the bare name at parse time will not fix it
  on its own: `activate.go:90` must be changed to pass the bare name.
- **Install: yes.** `install_project.go:230-236` installs with `Tool:
  dArgs.RecipeName` (the bare name), and `manager.go:153`'s `ToolDir(name, …)`
  receives that. So the on-disk directory is keyed by the bare name, which
  confirms the bare name is the right thing to validate.
- **Resolver: yes** — `bareToOrg` maps bare → key, and versions are looked up
  through it.

So: validate the bare name at parse time, and separately fix `activate.go:90` to
use the bare name so the FS layout agrees with what install produces.

**Charset survey — what real tool names actually use.** Across all
**1,449** recipes under `recipes/`:

- the complete character set of every recipe filename is
  `abcdefghijklmnopqrstuvwxyz0123456789._-` — lowercase letters, digits, dot,
  underscore, hyphen, nothing else;
- **zero** names fall outside `^[a-z0-9._-]+$`;
- **zero** names begin with a non-alphanumeric character, and zero begin with a
  digit;
- every `metadata.name` value inside the recipe files matches the same set;
- longest name is 28 characters (`iam-policy-json-to-terraform`);
- only two names use anything beyond `[a-z0-9-]`: **`llama.cpp`** (dot) and
  **`hdrhistogram_c`** (underscore).

Those two are the ones that matter. An allowlist that omits `.` breaks
`llama.cpp`; one that omits `_` breaks `hdrhistogram_c`. `^[a-z0-9._-]+$`
accepts every real name and is already enforced on `runtime_dependencies`, so
adopting it for `.tsuku.toml` keys creates no new dialect.

Two patterns to **not** copy: `shellDTargetPattern`
(`internal/actions/shell_init.go:51`) requires a leading `[A-Za-z_]`, which
would reject any future digit-initial tool name (`7zip`, `2to3`) — fine for its
own purpose, wrong here. And a bare `^[a-z0-9._-]+$` still admits `.` and `..`
as whole names; add the explicit `name == "." || name == ".."` rejection that
`data_dir.go:44` and `set_env.go:216` already use.

---

## Implications

**The fix belongs at `parseConfigFile`, not at activation.** Five consumers read
`Config.Tools` and four of them reach a path, URL or write sink with the raw
key. A check at `internal/shellenv/activate.go` covers one. A check at
`internal/project/config.go:167`, next to `MaxTools`, covers all five and every
consumer added later. The cost is one function and roughly fifteen lines; the
alternative is five checks that must each be remembered.

**Two more changes have to ride along, or the fix is incomplete.** First,
`resolver.go:36-38` must stop discarding `SplitOrgKey`'s error, and
`cmd_run.go:95` must stop discarding `LoadProjectConfig`'s — otherwise the two
consumers most likely to run unattended keep degrading silently. Second,
`activate.go:90` must pass the **bare** name, both because org-scoped
activation is broken today and because the validated value and the value that
reaches the sink should be the same string.

**The registry name path is a second, independent defect of the same class, and
it is worse.** F2 and F3 are not activation bugs — they are reachable from
`tsuku install` in any directory containing a hostile `.tsuku.toml`, and the
sink is recipe substitution plus an attacker-named disk write, not a PATH
entry. `IsValidRecipeName` exists for exactly this, its doc comment names
exactly this deployment, and `internal/recipe/provider_unified.go:271` and
`internal/registry/registry.go:86` never call it. Adding that call at
`recipePath` and at `Registry.FetchRecipe`/`CacheRecipe` is a small, isolated
change and probably higher priority than the activation fix.

**`internal/shellenv` needs `validateShellSafePath`'s check, whatever
`FormatExports` does about `%q`.** Even with correct shell quoting, a path
containing a newline or a backtick has no business on `PATH`. G4 already encodes
that judgement for the wrapper-script generator; `shellenv` should share it —
which means exporting it from `internal/install`, or better, moving both it and
`ValidateSymlinkTarget` somewhere both packages can reach. That belongs in the
shell-sinks lead's write-up too, but the *guard already exists* is my half of it.

**The design record needs correcting.**
`docs/designs/current/DESIGN-shell-env-activation.md:419` asserts a mitigation
that was never implemented, and `DESIGN-org-scoped-project-config.md:88`
records the decision that produced the gap: "Normalize config keys at parse
time … Rejected." The rejection was about *rewriting* keys into bare names, not
about *validating* them, but it is the reason nothing at all happens at parse
time. Any fix should amend both, or the next reader will re-derive the same
false assurance.

---

## Surprises

**S1 — `SplitOrgKey`'s only caller throws its error away.** The brief describes
the guard as "placed one consumer deep". It is worse: placed one consumer deep
*and* not enforced there. `resolver.go:36-38` is `if err != nil || !isOrg {
continue }`. The guard has never rejected anything in production.

**S2 — three parsers for one format, with three different `..` policies.**
`SplitOrgKey` (`internal/project/orgkey.go:16`) rejects `..` when the key has a
slash. `parseDistributedName` (`cmd/tsuku/install_distributed.go:37`) rejects
`..` by returning nil and letting the name proceed down a different, unguarded
path. `splitQualifiedName` (`internal/recipe/loader.go:139`) has no `..` check
at all. `DESIGN-org-scoped-project-config.md:84` says a shared `splitOrgKey`
was extracted "since both `runProjectInstall` and the resolver need the same
name-splitting logic" — the design intended one parser, and three shipped.
`runProjectInstall` uses the one that is not the shared utility.

**S3 — two functions named `ValidateVersionString` that disagree, and the
codebase knows it.** `internal/version/transform.go:29` allows `/` and `.` in
its charset (`^[a-zA-Z0-9._+\-@/]+$`, to support `@scope/pkg@1.2.3`), so it
accepts `../../evil` outright — it is a charset-and-length check, not a
traversal guard, despite the name. `internal/install/state.go:413` is the
traversal guard. `internal/updates/gc.go:76-84` contains a comment explicitly
reasoning about which to use and picking install's. Anyone reading only the
brief's pointer to `internal/version/transform.go` would conclude the version
component is traversal-guarded. It is not, by that function.

**S4 — the URL sink is more serious than the filesystem sink.** I went looking
for path traversal and found registry substitution: a `.tsuku.toml` key climbs
out of `raw.githubusercontent.com/tsukumogami/tsuku/main/recipes/` and names an
arbitrary owner, repo, ref and path. A recipe that resolves there is executed by
the normal install machinery. The FS traversal in activation needs a
pre-existing directory at a `$TSUKU_HOME`-relative path; this one needs only a
public repo.

**S5 — org-scoped tools have never activated.** `activate.go:73-90` passes the
full key to `ToolBinDir`, so `"tsukumogami/koto" = "1.0"` looks for
`tools/tsukumogami/koto-1.0/bin`, which install never creates. The tool lands in
`Skipped` and nothing says why. A security fix to this line has to fix the
functional bug too.

**S6 — the codebase has a correct model and follows it once.**
`internal/index/rebuild.go:180` validates at the write boundary, so every index
reader inherits the guarantee. `internal/actions` is full of well-formed
single-segment checks (`data_dir.go:44`, `set_env.go:216`,
`shell_init.go:51`). The knowledge is present; only the boundary placement is
missing, and only for `.tsuku.toml` and the registry name path.

---

## Open Questions

1. **Does F3's disk write actually land?** I established it by code reading —
   `DiskCache.Put` → `filepath.Join(c.dir, key)` → `MkdirAll` + `WriteFile`,
   with `key` proven to carry the traversal — but did not execute a 200 response
   in the harness. A local HTTP registry pointed at `127.0.0.1` would settle it
   in a few minutes. Worth doing before the fix is sized, since an arbitrary
   file write changes the severity.

2. **Is `.tsuku.toml` in scope as an attack surface at all, or only as a
   footgun?** Everything here assumes a hostile `.tsuku.toml` reaching a user's
   working directory — a cloned repo, a CI checkout, a dependency vendored into
   a tree. The design doc grades that Medium ("Auto-activation in cloned
   repos"). If the threat model says a `.tsuku.toml` is as trusted as a
   `Makefile`, the priority ordering changes, though F2's registry substitution
   is arguably still worse than a `Makefile` because nothing tells the user the
   registry was redirected.

3. **Should `ValidateSymlinkTarget` and `validateShellSafePath` move?** Both
   belong to `internal/install` but encode invariants (`inside
   $TSUKU_HOME/tools`, `safe to interpolate into sh`) that `internal/shellenv`
   needs verbatim. A shared `internal/tsukupath`-style package would fix G3 and
   G4 together, but it touches more files than a parse-time check. Someone
   should decide whether that refactor rides with this fix or follows it.

4. **What is the disposition of the recipe-supplied `..` in
   `internal/shim/manager.go:203`?** Harmless today only because
   `os.WriteFile` onto a directory errors. Out of my scope (recipe trust, not
   config trust) but it belongs in whoever owns the recipe-trust boundary.

5. **Is `cmd_run.go:95`'s discarded error deliberate?** `tsuku run` is the
   latency-sensitive path and may be swallowing the error on purpose. If so, a
   rejected config will silently behave as no config there, which conflicts with
   the brief's "report rejects rather than silently skip" requirement, and
   someone needs to choose.

---

## Summary

The two known defects are two members of a class: `.tsuku.toml` parsing enforces
only `MaxTools`, and the unvalidated tool key reaches four sink families — a
filesystem path in activation (demonstrated: `"../MARKER" = "v1"` puts
`$TSUKU_HOME/MARKER-v1/bin` on `PATH`, and the version component escapes too, so
*neither* half of that composition is guarded), a registry URL that climbs out
of `tsukumogami/tsuku` to any GitHub owner/repo (demonstrated), the registry
disk-cache write key, and shell-evaluated `export` text — while at least five
guards that would have caught it (`SplitOrgKey`, whose sole caller discards its
error; `IsValidRecipeName`, absent from the two path builders its own doc
comment names; `ValidateSymlinkTarget`; `validateShellSafePath`;
`install.ValidateVersionString`) sit one consumer deep. The implication is that
the fix is placement, not coverage: validate at `parseConfigFile` — run
`SplitOrgKey`, propagate its error, then check the bare name against
`^[a-z0-9._-]+$`, which every one of the 1,449 registry recipe names already
satisfies (`llama.cpp` and `hdrhistogram_c` are the only two needing more than
`[a-z0-9-]`) — plus call `IsValidRecipeName` at `recipePath` and
`Registry.cachePath`. The biggest open question is whether the registry
disk-cache write actually lands on a 200, because an arbitrary attacker-named
file write with attacker-supplied content is a materially more severe finding
than the PATH manipulation that started this, and it is reachable from `tsuku
install` rather than from activation.
