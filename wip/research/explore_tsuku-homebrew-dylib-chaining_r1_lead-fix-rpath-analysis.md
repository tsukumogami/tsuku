# Lead: fix-rpath analysis -- macOS dylib RPATH chaining for tool recipes

Investigation of `fixLibraryDylibRpaths` and the broader RPATH-fixup paths in
tsuku, focused on what changes if the `Type == "library"` gate is lifted so
tool recipes can chain dylib dependency directories the same way library
recipes do today.

Working directory: `/home/dgazineu/dev/niwaw/tsuku/tsukumogami-4/public/tsuku`.

## 1. The exact code path on macOS

### 1.1 The library gate (homebrew_relocate.go)

In `internal/actions/homebrew_relocate.go` the `Type == "library"` check is
applied in three places, all early in `Execute`:

- **Line 57** -- selects the placeholder install path: libraries use
  `$LibsDir/{recipe-name}-{version}`; tools use `ToolInstallDir` (or
  `InstallDir` fallback).
- **Line 92** -- selects the cellar path: for libraries it's the parent dir
  (`$LibsDir`); for tools it's the same as the install path.
- **Line 103** -- the gate that matters here:
  ```go
  if ctx.Recipe != nil && ctx.Recipe.Metadata.Type == "library" && runtime.GOOS == "darwin" {
      if err := a.fixLibraryDylibRpaths(ctx, installPath, reporter); err != nil {
          return fmt.Errorf("failed to fix library dylib RPATHs: %w", err)
      }
  }
  ```

If this gate is lifted (or extended to include `Type == "tool"`),
`fixLibraryDylibRpaths` will run for tool recipes too on darwin.

### 1.2 What `fixLibraryDylibRpaths` does (lines 574-674)

It operates strictly on `.dylib` files found by walking
`filepath.Join(ctx.WorkDir, "lib")`. For each `.dylib`:

1. Iterates `ctx.Dependencies.InstallTime` (line 609) -- note: **not**
   `ctx.Dependencies.Runtime`. For each dep it constructs
   `filepath.Join(ctx.LibsDir, "{name}-{version}", "lib")`.
2. Adds each constructed path as a Mach-O `LC_RPATH` via
   `install_name_tool -add_rpath`.
3. Adds `@loader_path` so the dylib also finds siblings in its own dir.
4. Re-signs with `codesign -f -s -` (best-effort).
5. Silently no-ops in three cases: `runtime.GOOS != "darwin"`,
   `WorkDir/lib` doesn't exist, or no `.dylib` files are present, or
   `len(depLibPaths) == 0`.

Critical: the dependency paths it constructs target `$LibsDir`, not
`$ToolsDir`. So if a tool depends on something installed under
`$TSUKU_HOME/tools/`, the constructed path won't exist and the rpath entry
becomes dead.

### 1.3 What `fixMachoRpath` does for tools (lines 433-572)

`fixMachoRpath` is what tool recipes get today (called per-binary by
`fixBinaryRpath` from inside `relocatePlaceholders`). For each Mach-O
binary or dylib it:

- Deletes any existing `LC_RPATH` containing `HOMEBREW`.
- Adds a single new RPATH that is either `@loader_path` or
  `@loader_path/<rel>` pointing to a same-tree `lib/` directory (sibling or
  one level up from the binary). Lines 491-508 do this lookup.
- If the binary is a `.dylib` (lines 521-529), updates its `LC_ID_DYLIB` to
  `@rpath/<basename>`.
- For each non-self `LC_LOAD_DYLIB` entry that contains `HOMEBREW` or `@@`
  (lines 534-560), rewrites it to `@rpath/<basename>`. References that
  already use absolute paths (e.g., `/opt/homebrew/opt/openssl/...`) are
  left alone unless they contain `HOMEBREW`.
- Re-signs on arm64.

**Key gap:** `fixMachoRpath` only sets a single `@loader_path` rpath.
It never adds rpaths pointing into other tools' or libs' directories. So
when a tool's relocated binary has a `LC_LOAD_DYLIB` rewritten to
`@rpath/libfoo.dylib`, the dynamic loader has no path to find `libfoo` in
a sibling dependency install -- only in the tool's own `lib/` if it
exists.

This is the gap that motivates the lead.

## 2. Behavior change if the `Type == "library"` gate is lifted

### 2.1 Tool recipes with no `runtime_dependencies` and no `dependencies`

No observable change. `fixLibraryDylibRpaths` walks `WorkDir/lib`; if no
`.dylib` files exist (typical: a tool ships only an executable in `bin/`),
it returns at line 603-605. Even if dylibs do exist, the dep walk at
lines 609-614 is a no-op when `ctx.Dependencies.InstallTime` is empty
(line 617-620 returns early). Nothing happens.

### 2.2 Tool recipes that DO declare runtime_dependencies and currently work

This is where it gets subtle. `ctx.Dependencies.InstallTime` for a tool
recipe is populated by one of two paths:

**Path A -- pre-resolved (cmd/tsuku/install_deps.go:386-408):**

```go
if len(r.Metadata.Dependencies) > 0 {
    resolvedDeps := actions.ResolvedDeps{InstallTime: make(map[string]string)}
    for _, depName := range r.Metadata.Dependencies { ... }
    exec.SetResolvedDeps(resolvedDeps)
}
```

This iterates `r.Metadata.Dependencies` only. **`r.Metadata.RuntimeDependencies`
is never propagated into `ctx.Dependencies` via this path.** This is
gap #2 (independent of the gate question).

**Path B -- buildResolvedDepsFromPlan (executor.go:493-495 + 962-983):**

When `e.resolvedDeps` is empty, the executor falls back to
`buildResolvedDepsFromPlan(plan.Dependencies)` which puts every dep tree
node into `InstallTime`. But `plan.Dependencies` is built by
`generateDependencyPlans` (plan_generator.go:676-734), which iterates
`deps.InstallTime` only (line 712). **runtime deps never reach the plan's
DependencyPlan tree.** This is gap #3.

**Net effect for tools that declare runtime_dependencies today:**

- Lifting only the `Type == "library"` gate gives tool recipes
  `fixLibraryDylibRpaths`, but `ctx.Dependencies.InstallTime` will be
  empty for runtime-only deps, so the dep walk no-ops and the rpath
  chain still doesn't get added. *Behavior change: zero.* The fix is
  inert without a corresponding wire-up of runtime deps into the
  execution context.

- Tool recipes that currently put their dylib deps in `dependencies`
  (install-time) WOULD see new rpaths get added if those dep recipes
  install to `$TSUKU_HOME/libs/{name}-{version}/lib`. For tools whose
  deps are themselves tools (installed under `$TSUKU_HOME/tools/`), the
  constructed library paths are wrong and the new rpath entries point
  at non-existent dirs. The dynamic loader silently skips missing rpath
  entries, so this is not a regression in dlopen behavior, but it does
  bloat `LC_RPATH` and wastes a `codesign` re-sign.

- For tools with `dependencies` that ARE libraries: `fixLibraryDylibRpaths`
  walks `WorkDir/lib` for dylib files. Tool homebrew bottles often unpack
  their dylibs into `WorkDir/lib/` too (the bottle's standard layout), so
  the walk would find them and add the dep rpaths. *This is actually the
  desired behavior.*

So the gate alone is necessary-but-not-sufficient. It does no harm where
tool-only deps are listed (just dead rpath entries) and helps where
library deps are listed in `dependencies`. The runtime-deps-only case
needs additional wiring.

### 2.3 Behavior change for existing library recipes

None expected. Library recipes are unchanged.

## 3. The Linux equivalent gap

### 3.1 What runs for tools on Linux today

The Linux path through `homebrew_relocate.go` ends in `fixElfRpath` (lines
334-401), called for each binary that contains a Homebrew placeholder. It:

- Removes existing RPATH (line 356).
- Picks a new RPATH that is either `$ORIGIN` or `$ORIGIN/<rel>` pointing
  to a same-tree `lib/` (lines 365-385). Single rpath, no dependency
  paths.
- Calls `patchelf --force-rpath --set-rpath <newRpath>` (line 387).
- Optionally fixes the ELF interpreter (line 395).

**There is no Linux equivalent of `fixLibraryDylibRpaths`** -- the
homebrew_relocate path on Linux never iterates `ctx.Dependencies` to add
RPATH entries pointing at dependency `lib/` directories. Lifting the
darwin-only gate at line 103 does nothing for Linux because the function
itself short-circuits at line 578 (`if runtime.GOOS != "darwin"`).

### 3.2 The standalone `set_rpath` action (set_rpath.go)

`internal/actions/set_rpath.go` is a separate, recipe-callable primitive
(`SetRpathAction`). Its `Execute` method (lines 33-118) does support
chained dependency RPATHs, but only via explicit recipe authoring:

- Line 43 builds vars including `ctx.Dependencies` via
  `GetStandardVarsWithDeps`. Recipes can write
  `rpath = "$ORIGIN/../lib:{libs.openssl.libdir}:{libs.zlib.libdir}"`
  and the action will expand and validate the paths.
- The Linux backend `setRpathLinux` (lines 154-208) calls
  `patchelf --force-rpath --set-rpath` exactly like `fixElfRpath`, but
  with a fully composed rpath value supplied by the recipe.
- The macOS backend `setRpathMacOS` (lines 211-270) does the equivalent
  with `install_name_tool -add_rpath` per colon-separated entry.
- `validateRpath` (lines 406-460) requires absolute paths to live under
  `ctx.LibsDir`, so this is geared at chaining libraries (not other
  tools).

So the recipe-side workaround that exists today is "call set_rpath after
homebrew_relocate with a hand-built rpath chain referencing dep libs".
This works on both OSes, but it requires the recipe author to know exactly
which deps need to be chained, and it relies on the deps being libraries
(so they're under `$LibsDir`). It does not cover homebrew bottles whose
embedded `LC_LOAD_DYLIB`/`DT_NEEDED` references are already rewritten by
`homebrew_relocate` to `@rpath/foo.dylib`/`foo.so` -- the loader still
needs an rpath entry that points at the dep's lib dir, which `set_rpath`
can provide if invoked.

### 3.3 The Linux gap, summarized

There's no `homebrew_relocate`-driven RPATH-chaining path on Linux for
either tools or libraries. Library recipes whose dylibs need to find
dep libraries on Linux must either:

(a) declare `dependencies` AND ensure those deps install under `$LibsDir`
    AND ship their own `set_rpath` step that builds the chain manually, or
(b) rely on `LD_LIBRARY_PATH` set by a wrapper, or
(c) rely on $ORIGIN-relative layout (which homebrew bottles don't honor).

The macOS-only `fixLibraryDylibRpaths` is Linux's equivalent missing
primitive too -- it's just that Linux library recipes that currently work
do so by other means (typically (a) above).

## 4. Does runtime_dependencies get into ctx.Dependencies today?

**No.** Three independent gaps:

1. **install_deps.go:386-408 (pre-resolved path):** only iterates
   `r.Metadata.Dependencies`. `RuntimeDependencies` is never put into
   `resolvedDeps.InstallTime`.
2. **install_lib.go:75-100 (library install pre-resolved path):** same
   pattern, install-time only.
3. **plan_generator.go:702-714 (plan dep tree path):** only iterates
   `deps.InstallTime`. `deps.Runtime` is intentionally excluded from
   `DependencyPlan`s.

Where runtime_dependencies DO end up:

- They get **installed** by the install command's pre-loop at
  `install_deps.go:325-338` (separate `installWithDependencies` calls
  before the executor runs).
- They get **stored in tool state** at `install_deps.go:561` as
  `ts.RuntimeDependencies = mapKeys(resolvedDeps.Runtime)` (display only).
- They get **stored in install options** at `install_deps.go:503-507` as
  `installOpts.RuntimeDependencies` -- this is consumed by wrapper-script
  generation for tools that need `LD_LIBRARY_PATH`-style activation, not
  by the executor's `ExecutionContext`.

So: `r.Metadata.RuntimeDependencies` reaches the *system* (filesystem,
state, wrappers) but never reaches `ctx.Dependencies` while the executor
runs. Any RPATH-fixup primitive that wants to chain runtime deps has to
either:

- have its caller put them into `ctx.Dependencies.Runtime` (or
  `InstallTime`), via a small wiring change in `install_deps.go` and
  `install_lib.go` (and probably `buildResolvedDepsFromPlan` /
  `generateDependencyPlans` so cached plans stay correct), or
- read `r.Metadata.RuntimeDependencies` directly from `ctx.Recipe`
  (hacky, doesn't get version info without a separate state lookup).

## 5. Test surface that would need updating

### 5.1 Direct tests of `fixLibraryDylibRpaths`

`internal/actions/homebrew_relocate_test.go` covers `Dependencies()`,
`extractBottlePrefixes`, and `findPatchelf*` only. There is **no
existing test that directly exercises `fixLibraryDylibRpaths`** or the
`Type == "library"` gate at line 103. Lifting the gate touches no
existing assertions.

New tests needed if the gate is lifted (or replaced with a more general
condition):

- A test that asserts the function runs for `Type == "tool"` recipes on
  darwin when `ctx.Dependencies.InstallTime` is non-empty.
- A test that asserts the function still no-ops when there are no
  dylibs or no deps.
- A test that asserts dead rpath entries (deps with no `lib/`) are
  tolerated (or, preferably, filtered before the `add_rpath` call).
- A test that the dep path is constructed against the right base
  directory: today it's hard-coded to `ctx.LibsDir`. If tool deps need
  to be supported, the test must distinguish `$ToolsDir/{name}-{ver}/lib`
  from `$LibsDir/{name}-{ver}/lib`, or assert one is preferred.

### 5.2 Tests of the dep-wiring gap (gap #1-3 above)

If the wiring fix is bundled (so runtime deps DO reach `ctx.Dependencies`):

- `cmd/tsuku/install_deps.go` doesn't have a focused unit test for the
  `SetResolvedDeps` block at lines 386-408. The closest is
  `cmd/tsuku/dependency_test.go::TestResolveRuntimeDeps` which tests the
  helper, not its propagation into the executor.
- `internal/executor/executor_test.go::TestSetResolvedDeps` (line 1372)
  asserts that `SetResolvedDeps` round-trips through `ctx.Dependencies`.
  This test already exists and would just need a runtime-deps variant.
- `internal/executor/plan_generator_test.go` (or equivalent) would need a
  test that runtime deps DO appear in `DependencyPlan` -- this changes
  cached plan format and may invalidate `plan_cache_test.go` golden
  fixtures.
- `internal/install/state_test.go` already round-trips
  `RuntimeDependencies` (lines 1428-1439), so no change there.

### 5.3 Validation tests

`internal/recipe/validator_runtime_deps_test.go` and
`validator_runtime_deps_integration_test.go` cover the multi-satisfier
alias rule for runtime_dependencies. Untouched by the wiring fix.

### 5.4 Integration

A sandbox test using a tool recipe that depends on a library installed to
`$LibsDir` (e.g., a homebrew tool bottle that links against a homebrew
library bottle) would be the end-to-end check. There isn't an existing
fixture like this.

## 6. Implications for the core question (recipe workaround vs. new primitive)

Lifting the `Type == "library"` gate alone does nothing for tools that
declare only `runtime_dependencies` -- the rpath chain only gets added
when `ctx.Dependencies.InstallTime` is populated, and runtime deps never
land there. So the minimum useful change is two-part:

1. Wire `r.Metadata.RuntimeDependencies` (and its resolved versions) into
   `ctx.Dependencies` -- either into `Runtime` (and then have
   `fixLibraryDylibRpaths` read both maps) or into `InstallTime` (matching
   how `buildResolvedDepsFromPlan` already conflates them today). The
   latter is less invasive but conflates the two semantically.
2. Lift the `Type == "library"` gate at homebrew_relocate.go:103 (or
   replace it with `runtime.GOOS == "darwin" && (Type == "library" ||
   has dylib-using deps)`).

Even with both changes, `fixLibraryDylibRpaths` constructs paths as
`$LibsDir/{name}-{ver}/lib`. Tools whose deps are *tools* (under
`$ToolsDir`) won't be helped. So a third change -- look up each dep's
actual install root via state or executor context -- is also required if
the goal is to chain across tool deps too. (For the homebrew-bottles
case, deps are typically themselves homebrew library bottles installed
under `$LibsDir`, which would Just Work.)

The set_rpath primitive (`internal/actions/set_rpath.go`) already provides
a recipe-callable equivalent. A recipe author can append a `set_rpath`
step after `homebrew` with an explicit `rpath` that interpolates dep
paths via `{libs.openssl.libdir}` etc. This is the recipe-side workaround
and it works today on both OSes for library deps. It does require the
author to know which deps need chaining and to enumerate them by hand.

If the consensus is "do this in tsuku, not in every recipe," the
narrowest correct fix is:

- Wire runtime deps into ctx.Dependencies (gaps #1-3).
- Generalize `fixLibraryDylibRpaths` (rename it, drop the gate) to run
  for any darwin homebrew install whose deps live under `$LibsDir`.
- Add an analogous Linux helper (`fixToolElfRpaths` or similar) that
  composes `$ORIGIN/../lib:<dep1lib>:<dep2lib>` and calls patchelf, so
  the macOS/Linux behavior matches.
- Update `homebrew_relocate_test.go` accordingly and add an integration
  fixture.

## 7. File / line index for follow-up

| File | Line(s) | What's there |
|------|---------|--------------|
| `internal/actions/homebrew_relocate.go` | 57, 92, 103 | Three `Type == "library"` checks (Execute) |
| `internal/actions/homebrew_relocate.go` | 334-401 | `fixElfRpath` (Linux tool path) -- single `$ORIGIN` rpath |
| `internal/actions/homebrew_relocate.go` | 433-572 | `fixMachoRpath` (macOS tool path) -- single `@loader_path` rpath |
| `internal/actions/homebrew_relocate.go` | 574-674 | `fixLibraryDylibRpaths` (macOS library-only chain) |
| `internal/actions/homebrew_relocate.go` | 609 | reads `ctx.Dependencies.InstallTime` (not Runtime) |
| `internal/actions/set_rpath.go` | 33-118 | `SetRpathAction.Execute` -- recipe-callable primitive |
| `internal/actions/action.go` | 13-40 | `ExecutionContext` (Dependencies field) |
| `internal/actions/resolver.go` | 31-34 | `ResolvedDeps {InstallTime, Runtime map[string]string}` |
| `internal/actions/resolver.go` | 187-204 | Recipe-level RuntimeDependencies handling in resolver |
| `internal/recipe/types.go` | 169-172 | `Dependencies` and `RuntimeDependencies` fields |
| `internal/executor/executor.go` | 313-319, 489-521 | `SetResolvedDeps` and ctx.Dependencies wiring |
| `internal/executor/executor.go` | 962-983 | `buildResolvedDepsFromPlan` -- flattens plan deps into InstallTime |
| `internal/executor/plan_generator.go` | 702-714 | `generateDependencyPlans` -- iterates `deps.InstallTime` only |
| `cmd/tsuku/install_deps.go` | 325-338 | runtime_dependencies installed via separate pre-loop |
| `cmd/tsuku/install_deps.go` | 386-408 | `SetResolvedDeps` -- iterates `r.Metadata.Dependencies` only |
| `cmd/tsuku/install_deps.go` | 503-507, 561 | runtime deps recorded in installOpts and tool state |
| `cmd/tsuku/install_lib.go` | 75-100 | Library install -- same install-time-only `SetResolvedDeps` pattern |
| `internal/actions/homebrew_relocate_test.go` | 1-205 | Existing tests -- nothing covers `fixLibraryDylibRpaths` or the gate |
