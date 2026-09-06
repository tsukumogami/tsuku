# Lead: Which externally-supplied values reach filesystem path construction as a path component, and which of them are validated?

## Findings

### The shape of the answer

The defect class is real and larger than the one known instance, but it is not
uniformly distributed. It splits cleanly along a package boundary:

- **`internal/actions` is well defended.** Every place a recipe-supplied string
  becomes a path component there has an explicit, correctly-reasoned guard, and
  the comments show the author thought about composition rather than about one
  input. Archive extraction is contained by a kernel-enforced `os.Root` handle.
  This package is not where the bugs are.
- **`internal/config`, `internal/install`, `internal/updates`, and
  `internal/registry` are not defended.** The tool *name* is joined into paths
  that are created, renamed into, symlinked, and `os.RemoveAll`'d, with no guard
  anywhere on the name.

The organising fact is that tsuku already owns a correct, documented,
central name validator — `recipe.IsValidRecipeName`
(`internal/recipe/name.go:21`), whose doc comment calls itself "the single
source of truth for 'is this string a well-formed recipe identifier?'". It has
exactly three non-test callers: `internal/distributed/cache.go:68`,
`internal/index/rebuild.go:180`, and `internal/recipe/validator.go:190`. The
install path never calls it. So this is not a missing control; it is a control
that exists and was not wired to the sinks that need it.

### Findings table

Severity column meanings: **Exploitable** = traversal reaches a consequential
operation and I can trace an input the attacker controls. **Constrained** =
traversal reaches the sink but something real limits it. **Not reachable** = the
component cannot carry traversal.

| # | Sink (file:line, expression) | External value | How it gets there | Guard before the join? | What an attacker achieves |
|---|---|---|---|---|---|
| 1 | `internal/config/config.go:433` `filepath.Join(c.ToolsDir, fmt.Sprintf("%s-%s", name, version))` | tool name | CLI arg; `recipe.Metadata.Name` via `--recipe`; `.tsuku.toml` key | **None on `name`.** `version` guarded by `ValidateVersionString` | Root cause of #2-#6. `ToolDir("../../../../tmp/pwn","1.0")` = `/tmp/pwn-1.0` (verified) |
| 2 | `internal/install/manager.go:153` `toolDir := m.config.ToolDir(name, version)` then `os.Rename` of staging into it | tool name | `installWithDependencies` passes `args.Tool` verbatim (`cmd/tsuku/install_deps.go:234`, `:656`) | None | **Write outside the tree.** Whole extracted archive lands at an attacker-chosen directory |
| 3 | `internal/install/manager.go:405` `filepath.Join(m.config.ToolsDir, fmt.Sprintf(".%s-%s.staging", name, version))`, then `os.RemoveAll` at `:156` and `os.MkdirAll` at `:161` | tool name | same as #2 | None | **Delete + write outside the tree.** See note below on the accidental leading-dot mitigation |
| 4 | `internal/install/remove.go:49`, `:150`, `:257` `os.RemoveAll(m.config.ToolDir(name, version))` | tool name (from `state.json` keys at `:150`/`:257`; from CLI at `:49`) | `tsuku remove <name>`; state iteration | None | **Recursive delete outside the tree** |
| 5 | `internal/updates/gc.go:93` `filepath.Join(toolsDir, toolName+"-"+version)` then `os.RemoveAll` at `:121` | tool name from `state.json` | `internal/updates/apply.go:153` passes `entry.Tool` | `version` guarded at `:85`; **`toolName` not** | **Recursive delete outside the tree**, constrained by the `os.Lstat`/`IsDir()` check at `:102` — the path must already be a directory |
| 6 | `internal/shellenv/activate.go:90` `cfg.ToolBinDir(name, req.Version)` prepended to `PATH` | `.tsuku.toml` `[tools]` key | `LoadProjectConfig` → `result.Config.Tools`; keys never validated (`internal/project/config.go:157-171` checks only count) | None | **PATH injection.** Constrained: the traversed dir must exist and be named `<x>-<version>/bin` (`os.Stat` skip at `:91`) |
| 7 | `internal/registry/registry.go:95` `filepath.Join(r.CacheDir, firstLetter, name+".toml")` | recipe name | `FetchRecipe` → `cachePath`; name is whatever was requested | None | Read/write of a `.toml` outside the cache tree |
| 8 | `internal/registry/registry.go:85-86` `fmt.Sprintf("%s/recipes/%s/%s.toml", r.BaseURL, firstLetter, name)` then `os.ReadFile` (`:118`) when `isLocal` | recipe name | local-directory registry | None, **and no `filepath.Clean` at all** | **Read outside the tree**, any file ending `.toml` |
| 9 | `internal/recipe/provider_unified.go:276` `string(name[0]) + "/" + name + ".toml"` → `FSStore.Get` → `filepath.Join(s.dir, path)` (`internal/recipe/backing_store.go:69`) | recipe name | grouped-layout local provider | None | Read outside the recipes tree (verified: yields `/tmp/pwn.toml`) |
| 10 | `internal/recipe/disk_cache.go:306` `filepath.Join(c.dir, key)` and `:311-314` metaPath | cache key = recipe store path from #9 | `HTTPStore` → `DiskCache` | None | Write outside the cache tree |
| 11 | `internal/seed/audit.go:50` `filepath.Join(dir, entry.Tool+".json")`, `os.WriteFile` | tool name in a seed queue file | `WriteAuditEntry` | None | Write outside the audit dir. **Low value** — seed tooling is maintainer-side, not on a user's install path |
| 12 | `internal/shim/manager.go:79` `os.WriteFile(filepath.Join(m.binDir, name), ShimContent, 0755)` | recipe binary names | `extractBinaryNames` | `filepath.Base` applied (`:202`), but `".."` not excluded (only `"."` and `"/"`) | **Not exploitable.** `Join(binDir,"..")` is a directory, so `os.WriteFile` fails |
| 13 | `internal/install/manager.go:524` `filepath.Join(m.config.ToolDir(toolName, version), binaryPath)` (symlink target) | tool name | `createBinarySymlink` | **Yes** — `ValidateSymlinkTarget(targetPath, m.config.ToolsDir)` at `:527` | **Nothing.** Containment is checked on the composed path, so it catches `..` in the name even though its doc comment only mentions version |
| 14 | `internal/config/config.go:443` `CurrentSymlink(name)` → `os.Remove` (`remove.go:305`) | tool name | `removeToolEntirely` | `filepath.Base` on binaries (`remove.go:279`, `:287`); **the `binaries[name] = true` fallback at `:301` is unguarded** | Delete of one symlink outside the tree; narrow (`os.Remove`, not `RemoveAll`) |

### Sinks that are properly guarded (negative results)

These matter as much as the findings, because they show the guard pattern the
codebase already knows how to write.

| Sink | Guard | Note |
|---|---|---|
| Archive extraction, `internal/actions/extract.go:31` `openDestRoot` | `os.Root` handle; kernel enforces every component | **No zip slip.** The dest is resolved *through* a root anchored on `workDir` (`:37-51`) specifically so an earlier entry's symlink cannot re-anchor a later one. The comments at `:62-71` and `:86-97` correctly identify the lexical checks as diagnostics, not the boundary. This is the strongest control in the tree |
| `$TSUKU_HOME/data/<tool>`, `internal/actions/data_dir.go:39` `recipePathSegment` | full path-segment check | Comment names the threat exactly: "A recipe name reaches this from a distributed registry, so it is untrusted input" |
| shell.d exports filename, `internal/actions/set_env.go:209` `envTargetName` | same segment check, plus `@` rejection | |
| shell.d init filename, `internal/actions/shell_init.go:43` `shellDTargetPattern` | `^[A-Za-z_][A-Za-z0-9._-]*$` | |
| `install_program_files` destination | destination is *computed*, not recipe-named (`internal/actions/install_program_files.go:84-93`) | The comment explains this is deliberate: a recipe-named dest could reach `share/shell.d` and inject login-shell code |
| Distributed recipe cache, `internal/distributed/cache.go:67` `validateRecipeName`, `:77` `repoDir` | name + owner/repo both checked | The network-facing cache is the one cache that *is* guarded |
| Notices, `internal/notices/notices.go:211` `validateNoticeName` | applied at write (`:91`) and remove (`:183`) | |
| Runtime dependency names, `internal/recipe/validator.go:155` | strict pattern + `IsValidRecipeName` | Build `metadata.dependencies` gets **no** equivalent check — see Surprises |
| Shell cache rebuild, `internal/shellenv/cache.go:79`, `:124` | names come from `os.ReadDir` | Already single components; also rejects symlinks and non-regular files |
| GC version component, `internal/updates/gc.go:85` | `install.ValidateVersionString` | Guards the version half of the very same expression whose name half is unguarded |

### Reachability: how a hostile name actually arrives

This is the part worth being precise about, because "unvalidated" and
"exploitable" are not the same claim.

**The strongest path is `tsuku install --recipe <file>`.**
`runRecipeBasedInstall` (`cmd/tsuku/install.go:481`) parses the recipe, takes
`toolName = r.Metadata.Name` at `:490` with no check, then calls
`loader.CacheRecipe(toolName, r)` at `:497` — which is a bare map write
(`internal/recipe/loader.go:409-411`). That seeds the loader's in-memory cache
under the malicious name, so the subsequent `loader.Get(toolName)` in
`installWithDependencies` succeeds without touching the filesystem or the
network. The recipe file supplies both the name and the recipe, so there is no
prerequisite of an attacker-planted `.toml` anywhere.

The install-time validator does run — `recipe.ValidateRecipe(r)` at
`cmd/tsuku/install_deps.go:322`, and install aborts on errors at `:335`. It does
not help: `validateMetadata` (`internal/recipe/validator.go:271-283`) checks
`name` for emptiness, spaces (error) and lowercase (warning) and nothing else.
`../../../../tmp/pwn` is lowercase and space-free, so it passes. The loader's own
`validate` (`internal/recipe/loader.go:720`) only checks that the name is
non-empty.

**The `.tsuku.toml` path reaches #6 but not #2.** `parseConfigFile`
(`internal/project/config.go:157`) validates only the tool count against
`MaxTools`. Keys flow to `cmd/tsuku/install_project.go:72` and to
`internal/shellenv/activate.go:90`. For activation (#6) that is enough. For a
project-driven *install* (#2), the recipe still has to load under that name, so
the attacker needs a `.toml` at the traversed location — a real constraint.

**A bare CLI name is self-harm, not an attack.** `tsuku install ../../foo`
requires the user to type it, and still needs the recipe to resolve.

**`state.json` drives #4 and #5** but lives inside `$TSUKU_HOME` and is already
user-owned, so it is a persistence/escalation surface rather than an entry
point. Nothing validates names on load (`internal/install/state.go:281`, `:365`
unmarshal straight into the struct).

### Verified path-composition behaviour

I confirmed these with a throwaway `filepath.Join` program outside the repo
(since deleted) rather than asserting them:

- `ToolDir("../../../../tmp/pwn", "1.0")` → `/tmp/pwn-1.0`. Full escape.
- A **leading** `/` in the name does *not* escape: `Join` treats it as relative,
  so `"/etc/cron.d/evil"` yields `tools/etc/cron.d/evil-1.0`. Only `..` escapes.
- The staging path has an **accidental partial mitigation**: `fmt.Sprintf(".%s-%s.staging", ...)`
  prefixes a dot, so a name *starting* with `..` becomes `...` — not a traversal
  component. `.../../../../tmp/pwn` gives `$TSUKU_HOME/tools/../..` only from the
  second component onward. A name that does not start with `..` defeats this
  entirely: `"x/../../../../tmp/pwn"` → `/home/u/tmp/pwn-1.0.staging`. This is
  luck, not a control, and it does not protect `ToolDir` itself.
- `CurrentSymlink("..")` resolves to the tools directory itself. `os.Remove` on a
  non-empty directory fails, so that specific case is inert.

## Implications

**For the scope question, the answer is that the class is wider than two
defects but narrower than "everywhere".** The unvalidated-name-into-`filepath.Join`
bug is not one site; it is one *missing call* that manifests at roughly a dozen
sites, three of which reach `os.RemoveAll` and two of which reach a write. The
`%q`-for-shell-quoting bug someone else is covering is a genuinely separate
defect. So: two root causes, many sinks.

**The fix should be a chokepoint, not a patch per sink.** The right shape is to
validate `name` inside `Config.ToolDir`, `ToolBinDir`, `LibDir`, `AppDir` and
`CurrentSymlink` — the five helpers in `internal/config/config.go:432-454` — by
calling the validator that already exists. That requires giving those helpers an
error return, which is a wide but mechanical change and is the only version of
the fix that cannot be forgotten by the next caller. Patching the dozen call
sites individually reproduces exactly the failure mode that produced this class.
`internal/updates/gc.go:93` and `internal/registry/registry.go:85-95` compose
the path by hand rather than through `Config`, so they need the guard applied
directly; `gc.go`'s comment at `:90-92` already flags that it duplicates
`ToolDir`'s layout.

**A second, cheaper chokepoint is worth adding regardless.** `ValidateSymlinkTarget`
(`internal/install/symlink.go:46`) is the one control in the install package
that works, and it works because it checks *containment of the composed path*
rather than the cleanliness of a component. A `containedIn(path, root)` assertion
before each `os.RemoveAll` / `os.MkdirAll` / `os.Rename` in `internal/install`
and `internal/updates` would hold even if a future name-validation bypass is
found. Defense at both ends is appropriate here because the consequence is
recursive deletion.

**The design record needs correcting.** `docs/designs/current/DESIGN-shell-env-activation.md:419`
claims the mitigation "All paths constrained to $TSUKU_HOME/tools/, name
validation" at severity Low. Both halves are false: paths are not constrained,
and no name validation exists. Notably the same row's residual-risk column says
"Tool names with unusual characters could construct unexpected paths" — so the
risk was seen, described accurately, and then filed as residual under a
mitigation that was never built.

## Surprises

**The codebase is good at this, in one package.** I expected to find uniform
carelessness and found the opposite: `internal/actions` contains several guards
that are better than average, with comments that correctly distinguish lexical
checks from real boundaries (`extract.go:62-71` is a genuinely sophisticated bit
of security reasoning, and the `os.Root` anchoring at `:26-31` defends against
an attack most implementations miss). Whoever wrote those was thinking clearly.
The gap is not skill; it is that nobody applied the same thinking one layer down
in `internal/config`.

**`internal/updates/gc.go:77-88` is the finding in miniature.** The author
pauses before `os.RemoveAll` to validate the version, writes eleven lines of
comment reasoning about *which of the two competing version validators* to use,
concludes "Anything that could steer os.RemoveAll out of the tools directory
does not get there" — and then on line 93 joins the completely unvalidated
`toolName` into the same expression. The hypothesis that the guard was written
against the input someone was thinking about rather than against the composition
is not an inference here; this is a direct demonstration of it.

**`IsValidRecipeName` documents a contract it does not have.** Its comment says
callers "all share this definition so a name accepted by one path is accepted by
all." That is aspirational — the install path accepts names all three of its
callers would reject.

**Build dependencies are unvalidated while runtime dependencies are not.**
`validateRuntimeDependencyNames` (`internal/recipe/validator.go:155`) applies a
strict pattern *and* `IsValidRecipeName` to `runtime_dependencies` and
`extra_runtime_dependencies`. `metadata.dependencies` gets nothing — I grepped
`internal/recipe/validator.go` and `validate.go` and found no reference to
`Metadata.Dependencies`. Those entries recurse into
`installWithDependencies` at `cmd/tsuku/install_deps.go:378-380` with `sub.Tool = dep`,
so they reach the same sinks. Same asymmetry as name-vs-version: the field
someone was thinking about got the guard.

**Absolute paths are not the risk; `..` is.** I had assumed a leading `/` in a
name would be the easy exploit. `filepath.Join` neutralises it. Any fix framed
around "reject absolute paths" would miss the actual vector.

## Open Questions

1. **Is `tsuku install --recipe <file>` considered a trust boundary?** My
   severity ranking for #2 depends on it being one. If the project's position is
   that `--recipe` means "you already chose to run this", the strongest
   reachability argument weakens considerably and the `.tsuku.toml` activation
   path (#6) becomes the most serious finding instead. This is a product
   judgement I cannot make from the code — though I would note the
   `data_dir.go:39` comment treats distributed-registry recipe names as
   untrusted, which suggests the project's own answer is that recipes are not
   trusted input.

2. **Should `Config`'s `*Dir` helpers return errors, or should validation happen
   at the CLI boundary?** The chokepoint argument favours the helpers; the
   blast radius of changing five widely-called signatures favours the boundary.
   I lean toward the helpers because boundary validation has already been tried
   here and is what produced the current state, but the call is a maintainer's.

3. **Is the local-directory registry (`isLocal`, finding #8) a supported
   deployment or a test affordance?** It is the only sink with no `filepath.Clean`
   at all. If it is test-only, it drops off the list; if users point tsuku at a
   local recipe directory, it is a straightforward arbitrary-file-read.

4. **Not investigated, flagged as adjacent:** `internal/install/state.go` applies
   no validation to tool names on load. Whether that matters depends on whether
   `state.json` is in the threat model, which the brief did not settle.

## Summary

The unvalidated tool name is one missing call, not one bug: `recipe.IsValidRecipeName` already exists and documents itself as the single source of truth, but has three callers and none of them is on the install path, so an unvalidated name reaches roughly a dozen sinks — three of which are `os.RemoveAll` and two of which are writes — with `tsuku install --recipe` taking `Metadata.Name` straight from a recipe file into `Config.ToolDir`. The fix has to be a chokepoint in the five `Config` `*Dir` helpers plus a containment assertion before each destructive operation, because patching sites individually is precisely what produced this state — `internal/updates/gc.go:85-93` validates the version half of an expression and leaves the name half unguarded eight lines later. The biggest open question is whether `--recipe` counts as a trust boundary, since that single judgement decides whether the most serious finding is arbitrary write-and-delete at install time or PATH injection via `.tsuku.toml`.
