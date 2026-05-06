# Decision 3: How is the chain APPLIED at install time on each OS?

## Question

How is the dylib/shared-library RPATH chain APPLIED, on each OS, when a tool
installation is post-processed?

- **(a)** Generalize `fixLibraryDylibRpaths` (lift the `Type == "library"` gate)
  AND add a Linux equivalent that walks `ctx.Dependencies` and patches RPATH.
- **(b)** Inject ABSOLUTE paths to the dep `lib/` directories
  (`$LibsDir/{name}-{ver}/lib`) baked into the binary at install time.
- **(c)** Use `$ORIGIN`-relative / `@loader_path`-relative paths into a known
  layout (e.g. `$ORIGIN/../../../libs/{name}-{ver}/lib`).

This decision is about the *value* injected and the *driver* that injects it.
It is independent of how the recipe declares the chain (decision 2) and of
whether the deps reach `ctx.Dependencies` at all (a wiring problem the lead
in `wip/research/explore_tsuku-homebrew-dylib-chaining_r1_lead-fix-rpath-analysis.md`
documents in detail).

## Context recap

Today's patching primitives (read end-to-end in
`internal/actions/homebrew_relocate.go` and `internal/actions/set_rpath.go`):

- **macOS, tools (`fixMachoRpath`, lines 433-572):** rewrites `LC_LOAD_DYLIB`
  entries that contain `HOMEBREW`/`@@` to `@rpath/<basename>`, sets
  `LC_ID_DYLIB` for `.dylib` outputs to `@rpath/<basename>`, and adds ONE
  `LC_RPATH` of `@loader_path` or `@loader_path/<rel>` pointing at a
  same-tree `lib/`. **Never adds dep `lib/` directories as RPATHs.**

- **macOS, libraries (`fixLibraryDylibRpaths`, lines 574-674):** walks
  `ctx.WorkDir/lib/*.dylib`, then iterates `ctx.Dependencies.InstallTime` and
  adds an absolute `LC_RPATH` of `$LibsDir/{depname}-{depver}/lib` per dep,
  plus `@loader_path`. Re-signs. Gated on `runtime.GOOS == "darwin" &&
  Type == "library"` at line 103.

- **Linux, all (`fixElfRpath`, lines 334-401):** removes existing RPATH and
  sets ONE rpath of `$ORIGIN` or `$ORIGIN/<rel>` pointing at a same-tree
  `lib/`. **Never iterates `ctx.Dependencies`.** No equivalent of
  `fixLibraryDylibRpaths` exists for either tools or libraries on Linux.

- **`SetRpathAction` (recipe-callable, `set_rpath.go`):** the recipe author
  composes the rpath value and passes it via the `rpath` parameter. Variable
  expansion via `GetStandardVarsWithDeps` resolves `{libs_dir}` and
  `{deps.<name>.version}`. `validateRpath` (lines 406-460) requires absolute
  rpath entries to live inside `ctx.LibsDir`. Both Linux and macOS backends
  exist. This is what `recipes/c/curl.toml` line 48 uses today:
  `"$ORIGIN/../lib:{libs_dir}/openssl-{deps.openssl.version}/lib:{libs_dir}/zlib-{deps.zlib.version}/lib"` --
  a HYBRID of `$ORIGIN`-relative for the tool's own libs plus absolute paths
  for chained dep libs.

So today the production system already mixes "absolute path baked at install
time" (curl's `set_rpath`) with `$ORIGIN`-relative rpaths (everything else).
The question is what convention the *automated* chaining path should use.

Layout invariants worth restating:

```
$TSUKU_HOME/
├── tools/<recipe>-<version>/{bin,lib}/
└── libs/<recipe>-<version>/{lib,...}/
```

A tool binary at `$TSUKU_HOME/tools/curl-8.17.0/bin/curl` reaches its OpenSSL
chained lib via either:
- absolute: `$TSUKU_HOME/libs/openssl-3.5.4/lib`
- `$ORIGIN`-relative: `$ORIGIN/../../../libs/openssl-3.5.4/lib`
  (i.e. `bin/../../../libs/...`)

A library at `$TSUKU_HOME/libs/libcurl-8.17.0/lib/libcurl.dylib` reaches OpenSSL
via:
- absolute: `$TSUKU_HOME/libs/openssl-3.5.4/lib`
- `@loader_path`-relative: `@loader_path/../../openssl-3.5.4/lib`

Both forms are encodable in ELF (`patchelf --set-rpath`) and Mach-O
(`install_name_tool -add_rpath`).

---

## Option (a): Generalize the existing functions (lift the gate, add Linux twin)

### Code sketch

**macOS side** -- minimal change at `homebrew_relocate.go:103`:

```go
// Before
if ctx.Recipe != nil && ctx.Recipe.Metadata.Type == "library" && runtime.GOOS == "darwin" {
    if err := a.fixLibraryDylibRpaths(ctx, installPath, reporter); err != nil { ... }
}

// After
if runtime.GOOS == "darwin" && a.shouldChainDeps(ctx) {
    if err := a.fixDarwinDepRpaths(ctx, installPath, reporter); err != nil { ... }
}
```

`fixDarwinDepRpaths` is the renamed `fixLibraryDylibRpaths`, with two
generalizations:

1. Walk both `WorkDir/lib/*.dylib` (libraries) AND `WorkDir/bin/*` Mach-O
   binaries (tools) -- because tool homebrew bottles install executables to
   `bin/` and link them against chained dylibs.
2. Iterate BOTH `ctx.Dependencies.InstallTime` and `ctx.Dependencies.Runtime`,
   so runtime-only deps are chained too (assuming the wiring fix from
   decision 2 lands; see "Assumptions" below).

`shouldChainDeps` is true when the recipe has any deps in either map.

**Linux side** -- new function, called from the same conditional:

```go
if a.shouldChainDeps(ctx) {
    if runtime.GOOS == "linux" {
        if err := a.fixLinuxDepRpaths(ctx, installPath, reporter); err != nil { ... }
    } else if runtime.GOOS == "darwin" {
        if err := a.fixDarwinDepRpaths(ctx, installPath, reporter); err != nil { ... }
    }
}
```

```go
func (a *HomebrewRelocateAction) fixLinuxDepRpaths(ctx *ExecutionContext, installPath string, reporter progress.Reporter) error {
    patchelfPath, err := a.findPatchelf(ctx)
    if err != nil {
        return err
    }
    depLibPaths := buildDepLibPaths(ctx) // from .InstallTime + .Runtime
    if len(depLibPaths) == 0 {
        return nil
    }
    elfFiles, err := a.findElfFilesNeedingChain(ctx.WorkDir) // bin/ + lib/
    if err != nil {
        return err
    }
    for _, p := range elfFiles {
        existing := readExistingRpath(patchelfPath, p)
        // Append (don't replace) so $ORIGIN entry from fixElfRpath survives
        newRpath := joinRpath(existing, depLibPaths)
        if err := setRpath(patchelfPath, p, newRpath); err != nil {
            return err
        }
    }
    return nil
}
```

**Subtle ordering issue on Linux:** `fixElfRpath` already runs once per binary
inside `relocatePlaceholders` (called from `fixBinaryRpath`). It sets a single
`$ORIGIN`-style rpath. `fixLinuxDepRpaths` then runs at the end of `Execute`.
Because `patchelf --set-rpath` REPLACES the value, `fixLinuxDepRpaths` must
either:
- read the existing rpath with `patchelf --print-rpath` and append, or
- be the sole rpath setter (move the `$ORIGIN` logic from `fixElfRpath` into
  `fixLinuxDepRpaths` so there's one place that owns rpath composition).

The cleaner refactor is the second: have `fixLinuxDepRpaths` and
`fixDarwinDepRpaths` each compose the FINAL rpath list (`$ORIGIN`/`@loader_path`
component + dep components), and have the per-binary callers in
`relocatePlaceholders` only handle non-rpath fixes (interpreter, install_name,
LC_LOAD_DYLIB rewrites). This is more code churn but eliminates the read-back
dance.

### Path form

This option leaves the path form open: (a) is "use the right driver"; the
inner value is whatever the function builds. As written today,
`fixLibraryDylibRpaths` builds **absolute** paths
(`$LibsDir/{name}-{ver}/lib`). So choosing (a) without further specification
implies (b)-style absolute paths -- you'd have to also pick (c) explicitly to
get relative paths.

In other words, (a) is orthogonal to (b) vs (c); it picks the *plumbing*, not
the *value*. The decision needs to specify both. We treat (a) as "automated
driver, path form per (b) or (c)."

### `$TSUKU_HOME` move

Inherits whatever (b) or (c) does for the path form. With absolute paths
(the current behavior of `fixLibraryDylibRpaths`), moving `$TSUKU_HOME` breaks
already-installed tools; with `$ORIGIN`-relative paths, they survive moves.

### Dep upgrade

Dep version is encoded in the RPATH (`openssl-3.5.4/lib`). When openssl
upgrades to 3.5.5, `$LibsDir/openssl-3.5.4/lib` keeps existing as long as the
old lib install isn't GC'd, so the binary keeps working. To pick up the new
version's symbols/fixes, the dependent tool must be reinstalled (which
re-runs `homebrew_relocate` with new versions and rewrites the RPATH). This
matches `set_rpath`'s current behavior for curl.

If we want auto-rebind on upgrade we'd need a different scheme entirely
(unversioned symlink path like `$LibsDir/openssl-current/lib`), which is out
of scope for this decision.

### Compatibility with `set_rpath`

Compatible. `set_rpath` is a recipe-driven primitive that runs before
`install_binaries`. It writes its rpath into `WorkDir/.install/bin/<tool>`.
`homebrew_relocate` wouldn't run on the `set_rpath`-built artifact (different
install paths), so there's no collision in current recipes.

If a recipe DID combine `homebrew` + `set_rpath` (it doesn't today), the
`set_rpath` step would run last and replace whatever rpath the homebrew
relocate set. That's the existing recipe-author override and would still work.

### Test surface

Net new tests:
- `fixDarwinDepRpaths` for tool recipes with `Type == "tool"` and InstallTime
  deps.
- `fixLinuxDepRpaths` for both tools and libraries.
- A test that the `$ORIGIN` / `@loader_path` "self lib" entry survives
  alongside the dep entries.
- A test that empty deps produces zero rpath modifications (no spurious
  `codesign` calls, no dead rpath entries).
- An integration sandbox test installing a tool whose only available linkage
  is via a chained library bottle.

Existing tests in `internal/actions/homebrew_relocate_test.go` don't directly
cover `fixLibraryDylibRpaths`, so renaming/generalizing it doesn't break
anything currently asserted.

### Risks

- Without the ctx.Dependencies wiring fix, this option is INERT for
  runtime-only deps. The fix and the wiring must land together.
- "Walk `WorkDir/bin`" requires deciding which binaries are Mach-O/ELF and
  which are scripts. Existing `fixBinaryRpath` already does this -- the new
  function can reuse it.
- Codesign cost on macOS scales with binary count. For tools that ship a few
  binaries this is fine; for tools with hundreds of executables (rare) it
  could double install time.

---

## Option (b): Inject absolute paths to dep `lib/` directories

### Code sketch

Same driver as (a), but the path form is explicitly:

```go
depLibPath := filepath.Join(ctx.LibsDir, fmt.Sprintf("%s-%s", depName, depVersion), "lib")
// e.g. /home/user/.tsuku/libs/openssl-3.5.4/lib
```

This is what `fixLibraryDylibRpaths` does today (line 612). It's also what
`set_rpath` produces when curl's recipe uses `{libs_dir}/openssl-...`.

### `$TSUKU_HOME` move

**Breaks.** If a user moves `~/.tsuku` to `/opt/tsuku` (or sets `TSUKU_HOME`
post-install), every binary's RPATH still points at the old location.
Dynamic loader fails to resolve dep libs. Symptom: `dyld: Library not loaded`
or `error while loading shared libraries`.

Mitigation paths:
- Reinstall everything after a move (tsuku could detect this via
  `state.json.tsuku_home != current_tsuku_home` and prompt).
- Use a trampoline symlink (`$TSUKU_HOME/libs` is always reached via a
  fixed canonical path -- but tsuku has no such path today).

The current `set_rpath`-via-curl pattern has this same brittleness already.
Choosing (b) means accepting "move = reinstall" as supported behavior.

### Symlinked install dir

If `$TSUKU_HOME` is itself a symlink (`~/.tsuku -> /opt/tsuku`), absolute
paths can be either resolved or unresolved depending on what built the path.
`filepath.Join(ctx.LibsDir, ...)` uses whatever `LibsDir` is set to. If the
executor canonicalizes via `filepath.EvalSymlinks` before building the path,
the rpath will point at `/opt/tsuku/libs/...` and `~/.tsuku/libs/...` access
will still work via dyld walking symlinks -- but the binary becomes wedded
to the canonical target. If `LibsDir` is the symlinked form, the rpath
breaks if the symlink is removed.

Today `ctx.LibsDir` is set from `config.GetLibsDir()` which doesn't
canonicalize, so the rpath uses whatever the user's `TSUKU_HOME` resolves to
at install time.

### Dep upgrade

Same as (a)-with-absolute-paths: rpath encodes the version dir, old version
must remain on disk for the dependent tool to keep working, reinstall the
dependent to migrate to new version.

### Compatibility with `set_rpath`

Same form. (b) is essentially "automate what curl's recipe does manually."

### Test surface

- A test that the absolute path is correctly built using `ctx.LibsDir` (not a
  hard-coded `~/.tsuku`).
- A test that demonstrates the "TSUKU_HOME move breaks" failure mode
  explicitly so future changes don't accidentally claim it works.
- `validateRpath` already requires absolute paths to live under `ctx.LibsDir`
  -- existing tests cover this.

### Risks

- Path length limits. PT_DYNAMIC RPATH on Linux has no hard limit but very
  long combined rpaths (5+ deps) start chewing through ELF program header
  space. A 5-dep chain at `/home/longusername/.tsuku/libs/<name>-<ver>/lib`
  is ~70 chars per entry; 5 entries plus separators is ~400 chars. Fine.
- The absolute path embeds the user's home directory. Cross-user binary
  copies are non-portable (a rare but real scenario in shared CI caches).

---

## Option (c): Use `$ORIGIN`-relative / `@loader_path`-relative paths

### Code sketch

For a tool at `$TSUKU_HOME/tools/<recipe>-<ver>/bin/<tool>`:

- macOS: `@loader_path/../../../libs/openssl-3.5.4/lib`
- Linux: `$ORIGIN/../../../libs/openssl-3.5.4/lib`

For a library `.dylib` at `$TSUKU_HOME/libs/<recipe>-<ver>/lib/libfoo.dylib`:

- macOS: `@loader_path/../../openssl-3.5.4/lib`
- Linux: `$ORIGIN/../../openssl-3.5.4/lib`

Computation:

```go
func relativeDepRpath(loaderDir, depLibDir string) (string, error) {
    rel, err := filepath.Rel(loaderDir, depLibDir)
    if err != nil {
        return "", err
    }
    if runtime.GOOS == "darwin" {
        return "@loader_path/" + rel, nil
    }
    return "$ORIGIN/" + rel, nil
}
```

The "loader dir" for an ELF binary or Mach-O executable is the dir containing
the binary; for a `.dylib` it's the dir containing the dylib. `filepath.Rel`
on the canonicalized form gives the right relative path. This works because
both the tool's install dir and the dep's lib dir are children of `$TSUKU_HOME`,
so the relative path stays inside the tsuku tree.

### `$TSUKU_HOME` move

**Survives.** Moving `~/.tsuku` to `/opt/tsuku` (or symlinking, or renaming)
preserves the relative position of `tools/<x>/bin` to `libs/<y>/lib`, so
`@loader_path/../../../libs/...` still resolves. This is the property that
makes this option attractive.

A subtle caveat: if the user moves only `tools/` (or only `libs/`)
out-of-tree, relative paths break. In practice, no one does this; the
TSUKU_HOME contract is "the whole directory moves as a unit."

### Symlinked install dir

`@loader_path` resolves to the realpath of the binary's directory on macOS
(verified: see Apple's `dyld(1)`). `$ORIGIN` similarly resolves to the
binary's directory after symlink resolution on Linux. So if `~/.tsuku` is a
symlink to `/opt/tsuku` and the binary is invoked via `~/.tsuku/bin/curl`,
both `@loader_path` and `$ORIGIN` evaluate to `/opt/tsuku/tools/.../bin`,
and the relative path correctly walks back to `/opt/tsuku/libs/...`. This
makes (c) more robust under symlinking than (b).

### Dep upgrade

Same as (b): version in the path. The relative path encodes
`../../../libs/openssl-3.5.4/lib`, so an openssl 3.5.5 install creates
`libs/openssl-3.5.5/lib` which the binary doesn't see. Reinstall the
dependent to migrate. No advantage over (b) here.

### Compatibility with `set_rpath`

`set_rpath`'s current curl usage emits ABSOLUTE dep paths
(`{libs_dir}/openssl-...`), so picking (c) introduces an inconsistency: the
recipe-driven path uses absolute, the auto-driven path uses relative. Two
options:
1. Live with the inconsistency. Recipe authors who want relative paths can
   author them manually using `$ORIGIN` (set_rpath already accepts this).
2. Update `set_rpath` to translate absolute dep paths to relative when the
   binary is inside `$TSUKU_HOME/tools` and the dep is inside
   `$TSUKU_HOME/libs`. Backwards-compatible (the binary loads either way),
   makes `tsuku` installs uniformly portable.

The validator (`validateRpath` in `set_rpath.go`) already accepts both
absolute (when inside `LibsDir`) and `$ORIGIN`-prefixed relative forms.

### Test surface

- A test for the `relativeDepRpath` helper covering tool->lib and lib->lib
  cases.
- A test that simulates a TSUKU_HOME move and asserts the relative rpath
  still resolves (can be done with a fixture: install to `/tmp/A`, rename to
  `/tmp/B`, exec the binary, expect success).
- A test that the relative path uses canonicalized dirs (not symlink
  forms), so the computed `..` count matches reality.

### Risks

- The relative-path computation must use canonical paths consistently. If
  one side comes from `os.Symlink` chain, the `..` count is wrong.
  Mitigation: `filepath.EvalSymlinks` on both sides before calling
  `filepath.Rel`.
- A future restructure of `$TSUKU_HOME` layout (e.g., flattening tools/libs)
  invalidates every binary's rpath. Today the layout is documented and
  stable; any change would require a migration anyway.
- `@loader_path` on macOS sometimes behaves differently than
  `@executable_path` (the latter is fixed at the main executable's dir; the
  former is per-image). For dylib-to-dylib chains we want `@loader_path` so
  the relative path is from the dylib, not the top-level executable.
  `fixLibraryDylibRpaths` already uses `@loader_path` -- this is correct.

---

## Cross-option matrix

| Aspect | (a) generalize driver | (b) absolute paths | (c) relative paths |
|--------|----------------------|--------------------|--------------------|
| Survives `$TSUKU_HOME` move | depends on path form | NO | YES |
| Survives symlinked TSUKU_HOME | depends | partial | YES |
| Dep upgrade auto-rebind | NO (version in path) | NO | NO |
| Matches existing `set_rpath` usage in curl | yes (uses absolute) | YES | inconsistent unless set_rpath updated |
| Code complexity | high (touch 3 funcs + Linux twin) | low (use existing form) | medium (add Rel helper) |
| Brittleness to layout changes | low | low | medium |
| Driver unification (one chaining function) | YES | YES | YES |

(a) is "where is the chaining done"; (b) and (c) are "what value is written."
The decision must answer both. The realistic packaging is:

- Path form: (b) or (c).
- Driver: do we automate via `homebrew_relocate` (option-a-style) or rely on
  recipe authors to call `set_rpath` (status quo)?

The decision question, as posed, mixes these. We answer both.

---

## Recommendation

**Choose (c): use `$ORIGIN`-relative / `@loader_path`-relative paths, applied
automatically via the generalized `homebrew_relocate` driver from option (a).**

Concretely:

1. Lift the `Type == "library"` gate at `homebrew_relocate.go:103` and
   rename `fixLibraryDylibRpaths` to `fixDarwinDepRpaths` (or similar).
2. Add a Linux twin `fixLinuxDepRpaths` that walks ELF files in `bin/` and
   `lib/` and composes a single rpath value (so it doesn't fight with the
   per-binary `fixElfRpath` -- consolidate rpath ownership).
3. Build dep paths as `@loader_path/<rel>` (macOS) or `$ORIGIN/<rel>`
   (Linux), where `<rel>` is `filepath.Rel(loaderDir, depLibDir)` after
   canonicalizing both with `filepath.EvalSymlinks`.
4. (Optional, follow-up) Update `SetRpathAction` to emit the same relative
   form when the absolute path is inside `LibsDir`, so the recipe-driven and
   auto-driven paths converge. Backwards-compatible.

### Why (c) over (b)

- **TSUKU_HOME portability.** Tsuku's value proposition is "no sudo, no
  system deps, lives entirely in `$TSUKU_HOME`." That contract fits poorly
  with binaries that hardcode the absolute installation path. A user moving
  their `~/.tsuku` to a new machine, a new home directory, or an image-based
  deploy expects things to keep working. (c) preserves that; (b) does not.
- **Symlink resilience.** Some installations live under
  `/opt/something/.tsuku` symlinked from `~/.tsuku`. Both `@loader_path` and
  `$ORIGIN` resolve via the actual file location, so relative rpaths work
  regardless of which path was used to invoke the binary.
- **Consistency with the existing tool-bin pattern.** `fixElfRpath` and
  `fixMachoRpath` already use `$ORIGIN` / `@loader_path` for the
  same-tree `lib/`. Extending the same convention to dep `lib/` dirs is
  internally consistent. (b) introduces a second convention (absolute for
  deps, relative for self-libs).
- **Cost is small.** `filepath.Rel` + `EvalSymlinks` is one helper. The
  driver code is the same as (b).

### Why (c) over status-quo / recipe-author-driven `set_rpath`

The recipe-driven workaround works (curl uses it on Linux), but:

- It requires every recipe author to know about the chaining problem and to
  enumerate dep paths by hand. The recipes survey shows ~323 recipes use
  `runtime_dependencies`; almost none currently chain correctly.
- It hardcodes absolute paths today, which is the same TSUKU_HOME-portability
  problem as (b), so a per-recipe fix doesn't help portability.
- Centralizing the chaining logic in `homebrew_relocate` means recipe
  authors get correct behavior for free. The recipe's job is to declare
  deps; tsuku's job is to wire the loader.

### What this depends on

This recommendation only chains deps that reach `ctx.Dependencies`. Per the
fix-rpath-analysis lead (section 4), `runtime_dependencies` doesn't reach
`ctx.Dependencies` today. Decision 2's chosen interface determines what gets
populated; decision 3's implementation reads what's there. The wiring fix
(decision 2 follow-through) and the chaining fix (this decision) must land
together to be useful.

### Migration / rollout

- Recipe authors using `set_rpath` with absolute `{libs_dir}/...` paths
  (curl) keep working unchanged. The new auto-driver doesn't run on those
  recipes (they don't go through `homebrew_relocate`).
- Recipes using `homebrew` action with chained deps automatically pick up
  the new behavior on next install. Already-installed tools keep their old
  rpaths until the user runs `tsuku update`.
- No state-format changes, no recipe-format changes.

### Open question deferred

Auto-rebind on dep upgrade (rpath via `openssl-current` symlink instead of
`openssl-3.5.4`) is out of scope. It changes the dep-storage layout and has
deeper implications (GC, version pinning, ABI compatibility detection). If
the team wants it, it's a separate decision.

---

## Rejected options

### Reject (b) absolute paths

Best ergonomic match for recipe authors (the path looks like what the recipe
declared), but the TSUKU_HOME-move brittleness is a regression vs the
existing `$ORIGIN`-relative pattern used everywhere else in tsuku. The
status-quo curl recipe demonstrates the brittleness exists; we should not
codify it as the default for automated chaining.

### Reject (a)-only (without specifying path form)

Underspecified. (a) picks the driver; without (b) or (c) attached, the
inherited path form is whatever `fixLibraryDylibRpaths` currently does --
which is absolute. So "(a) only" silently equals (b). The decision must
specify the path form explicitly.

### Reject "leave it to recipe authors" (status quo)

Doesn't scale. ~323 recipes use `runtime_dependencies` and most don't
include the `set_rpath` step that would actually chain deps. The recipes
that do (curl) carry the absolute-path TSUKU_HOME-move bug. Centralizing
fixes both problems.
