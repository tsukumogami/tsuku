---
status: Proposed
problem: |
  tsuku's homebrew action does not chain dylibs from sibling tsuku-installed
  library deps into a tool recipe's RPATH on either Linux or macOS. The
  mechanism exists for `Type == "library"` recipes (`fixLibraryDylibRpaths`)
  but is gated to that one type, so tool recipes whose homebrew bottles
  reference non-system shared libraries can't wire those refs to matching
  tsuku-installed deps. The result is N×M coordination cost (every new tool
  recipe needs custom RPATH chains, plus its dep recipes hand-coordinated to
  publish the right dylib names) and a steady stream of recipes that ship
  with `supported_os = ["linux"]` or `unsupported_platforms` because the
  bottle path doesn't work cleanly. Exploration measured >15 affected
  recipes (top-100 strict: 10; counting workaround-dependent existing
  recipes: 19+; counting macOS punts: 26+).
decision: |
  Strengthen the existing `homebrew` action to chain dep dylibs automatically.
  Recipe authors declare a new metadata-level field
  `chained_lib_dependencies = ["dep1", "dep2"]`. The homebrew action's
  relocate phase walks that field, lifts the existing `Type == "library"`
  gate in `fixLibraryDylibRpaths`, and adds a Linux ELF equivalent. RPATH
  entries use `$ORIGIN`-relative (Linux) or `@loader_path`-relative (macOS)
  paths into the stable `$TSUKU_HOME/libs/<dep>-<version>/lib` layout, so
  installed binaries survive a `$TSUKU_HOME` move. Existing
  `Type == "library"` recipes use the same path. Existing tool recipes that
  don't declare the new field are unchanged.
rationale: |
  Recipe authors did not converge on any opt-in workaround pattern (across
  1168 homebrew-using recipes, exactly 0 use the existing `set_rpath`
  primitive to chain deps). A new opt-in primitive would face the same
  adoption gap. Strengthening the existing `homebrew` action moves the fix
  to the level the blast radius warrants (>15 affected recipes) and applies
  the chain whenever recipe authors make their dep set explicit, regardless
  of whether they remember a separate step. The new explicit field
  (`chained_lib_dependencies`) avoids overloading `runtime_dependencies`
  (which already drives wrapper PATH semantics in 323 recipes) and
  per-step `dependencies` (which means build-time inputs and is per-platform
  duplicated). Relative-path patching matches the existing library pattern
  and avoids baking absolute paths into binaries.
upstream: wip/explore_tsuku-homebrew-dylib-chaining_findings.md
---

# DESIGN: Tsuku Homebrew Dylib Chaining for Tool Recipes

## Status

Proposed

## Context and Problem Statement

tsuku installs tools and libraries into versioned per-recipe directories under
`$TSUKU_HOME/tools/<recipe>-<version>/` and `$TSUKU_HOME/libs/<recipe>-<version>/`.
For most tool recipes, this layout is invisible to the user — `tsuku install <tool>`
puts a binary in `$TSUKU_HOME/tools/current/<tool>` and the user runs it.

A class of tool recipes ships its binary as a Homebrew bottle (e.g., `tmux`,
`git`, `wget`). Bottles are pre-compiled archives with hard-coded references
to other shared libraries — `tmux`'s bottle references `libutf8proc.so.3`,
`libevent-2.1.7.so.7`, and `libncursesw.so.6`; `git`'s bottle references
`libpcre2-8.0.dylib`. On the developer machines that build the bottles, those
shared libraries live in Homebrew's prefix (`/home/linuxbrew/.linuxbrew/lib/`
on Linux or `/opt/homebrew/lib/` on macOS) and are found via the bottle's
RPATH. After tsuku unpacks a bottle into its own per-recipe layout, those
sibling libraries are not at the paths the bottle expects.

tsuku partially handles this for **library recipes** (`Type == "library"`):
the function `fixLibraryDylibRpaths` in `internal/actions/homebrew_relocate.go`
walks `ctx.Dependencies.InstallTime` and adds each dep's `lib/` directory to
the binary's RPATH. But the function is gated on `Type == "library"`, so tool
recipes — which are most consumers of these chained dylibs — never get the
walk. The Linux side has no equivalent function at all: `fixElfRpath` sets
exactly one `$ORIGIN`-relative path and never touches dep lib dirs.

The recipe registry today demonstrates the cost of this gap:

- Across 1168 recipes that use the `homebrew` action, **0** use the existing
  recipe-level `set_rpath` action to chain deps (curl is the only recipe
  that uses `set_rpath` with a chained-deps template, and curl walked away
  from the bottle entirely on Linux).
- 26 recipes shipped with `supported_os = ["linux"]` or
  `unsupported_platforms = [...]` to skip macOS specifically because their
  bottles' @rpath references can't be resolved.
- 2 recipes (`git`, `wget`) work via per-step `dependencies` lists that the
  homebrew action's existing decomposition consumes, but the path is fragile
  — the wget recipe required hand-patching `gettext.toml` to expose
  `libintl.8.dylib` for the bottle's @rpath, and the same dance has to repeat
  for every new tool's dep set.
- The top-100 priority list contains at least 10 tools whose bottles depend
  on non-system shared libraries (node, ripgrep, bat, eza, neovim, htop,
  delta, ollama, shellcheck, hadolint), so the gap blocks the next wave of
  curation work.

Recipe authors have not converged on any reusable workaround. A
recipe-side fix would require every tool author to invent the same chain
of `set_rpath` template strings and coordinate dylib publishing with their
dep recipes' authors. The exploration concluded the right level for the
fix is tsuku-core; this design specifies it.

## Decision Drivers

- **Backward compatibility.** Existing `Type == "library"` recipes
  (`pcre2`, `libnghttp3`, `libevent`, `utf8proc`, plus 134 others) must not
  regress. Existing tool recipes that work today (with or without
  workarounds) must keep working unchanged.
- **Author ergonomics.** Recipe authors should not have to know about
  install-dir layout or RPATH semantics. Declaring a dep set should be
  enough to make the chain work.
- **Cross-platform consistency.** macOS and Linux both have the gap. The
  solution should read the same on both platforms — recipe authors should
  not need to write platform-specific RPATH chains by hand.
- **Portability of installed binaries.** A binary installed at one
  `$TSUKU_HOME` should still work if the user moves `$TSUKU_HOME` to a new
  location (e.g., reinstalls into a different prefix). Absolute paths baked
  into binaries break this.
- **No new workaround that authors will ignore.** Recipe authors did not
  adopt the existing `set_rpath` action for this purpose. A new opt-in
  primitive would face the same fate. The fix must apply automatically
  when authors declare the dep set, not require them to remember a
  separate step.
- **No silent semantic change to existing fields.** The `runtime_dependencies`
  field is consumed today by 323 recipes for wrapper PATH semantics; layering
  dylib-chaining onto the same field creates a second silent meaning that
  conflicts with the wrapper-PATH consumer.

## Considered Options

This design decomposes into three independent decisions. Each was evaluated
on its own; the integration is checked in *Decision Outcome*.

### Decision 1: Where the fix lives

A reusable solution to the dylib-chaining gap can live in three places: as
a new composite action wrapping the existing `homebrew` action with extra
chaining behavior; as a strengthening of the existing `homebrew` action so
it chains deps automatically when they're declared; or as a new
recipe-callable action that recipe authors invoke explicitly after
`homebrew`.

**Key assumptions:**
- The wiring change to flow the chosen dep field into `ctx.Dependencies`
  lands together; without it, the action has no input.
- Tool deps installed under `$ToolsDir` are out of scope; chaining targets
  only `$LibsDir`-based library deps.
- Pattern C library recipes (`pcre2`, `libnghttp3`, `libevent`, `utf8proc`)
  do not regress.

#### Chosen: Strengthen the existing `homebrew` action

The existing `homebrew` action gains automatic chaining behavior: when the
recipe declares the new dep field (Decision 2), the action's relocate phase
walks the deps and patches the binary's RPATH (Decision 3). Recipes already
using the `homebrew` action without the new field are unchanged. The new
field is the single signal that triggers chaining — there is no separate
opt-in step.

This routes the fix to tsuku-core, where the >15-recipe blast radius says
it belongs, and applies it automatically wherever recipe authors make their
dep set explicit.

#### Alternatives Considered

**(a) New composite action `homebrew_chained`** — Forces opt-in renames
across affected recipes; recipe authors who don't migrate stay broken. The
exploration found 0 of 1168 homebrew-using recipes adopted the existing
opt-in `set_rpath` primitive for chaining; a new opt-in faces the same
adoption gap. It also creates two parallel homebrew code paths to keep in
sync, increasing maintenance cost.

**(c) New recipe-callable action `chain_deps_into_rpath`** — Cleanest
backward-compatibility (recipes only get the new behavior when they
explicitly invoke the action) but requires every author to remember to add
the step. Overlaps semantically with the existing `set_rpath` action,
raising documentation burden. The exploration's empirical signal is that
recipe authors don't reach for explicit chain steps.

### Decision 2: How recipes declare what to chain

Three places to declare the dep set: reuse `metadata.runtime_dependencies`;
reuse per-step `dependencies`; or introduce a new explicit field.

**Key assumptions:**
- Library recipes (`Type == "library"`) continue to use the existing
  implicit path that walks `ctx.Dependencies.InstallTime` from per-step
  `dependencies`. The new field is for tool recipes that opt in.
- Authors are willing to learn one new field in exchange for clear semantics.
- The roughly 13 currently-punted darwin recipes (`tmux`, `curl`, etc.)
  are tractable to migrate one-by-one once the field exists.

#### Chosen: Introduce a new explicit field `chained_lib_dependencies`

The recipe declares, at the metadata level:

```toml
[metadata]
chained_lib_dependencies = ["libevent", "utf8proc", "ncurses"]
```

The field has one consumer (the homebrew action's relocate phase) and one
semantic (chain these libs into the binary's load path). Validator can
enforce that each entry resolves to an installed library recipe.

#### Alternatives Considered

**(a) Reuse `metadata.runtime_dependencies`** — Backward-compat risk too
high. 323 recipes use the field today, mostly auto-generated for wrapper
PATH semantics. Adding a second silent semantic (drive macOS
`install_name_tool` walk) is the breaking-change-disguised-as-additive
shape the constraints forbid. Also semantically muddled — the field would
mean three things in three contexts (wrapper PATH, decomposition input,
RPATH chain).

**(b) Reuse per-step `dependencies`** — Wrong grain. Chained libs are a
property of the produced binary, not the build step. The wget recipe
already demonstrates the failure mode: a per-platform `dependencies` list
duplicated across glibc-Linux, musl-Linux, and darwin steps with no
guarantee they stay in sync. The field name doesn't communicate "load
path" — it reads as "install before running this step."

### Decision 3: How the chain is applied at install time

Three approaches: generalize `fixLibraryDylibRpaths` (lift the
`Type == "library"` gate, add a Linux equivalent); inject absolute paths to
the dep `lib/` directories; or use `$ORIGIN` / `@loader_path`-relative
paths into the known `$TSUKU_HOME/libs/` layout.

**Key assumptions:**
- The `$TSUKU_HOME/{tools,libs}/<recipe>-<version>/` directory layout is a
  stable invariant. Any future flattening would invalidate relative rpaths.
- Users do not split `tools/` and `libs/` across filesystems.
- `filepath.EvalSymlinks` is used on both loader dir and dep lib dir before
  `filepath.Rel`, so the relative path's `..` count matches reality under
  symlinked installs.

#### Chosen: Lift the gate AND use relative paths (combined a + c)

Generalize `fixLibraryDylibRpaths` to fire for tool recipes too (the deps
walk is a no-op when `ctx.Dependencies` is empty, so existing tool
recipes that don't declare the new field are unchanged). Add a Linux ELF
equivalent that walks the same dep set and emits `$ORIGIN`-relative
RPATHs via `patchelf --add-rpath`. On macOS, the corresponding emit uses
`@loader_path`-relative paths via `install_name_tool -add_rpath`.

Relative paths anchor at the binary's own location (e.g.,
`$ORIGIN/../../libs/libevent-2.1.12/lib`) rather than baking
`$TSUKU_HOME` into the binary. A user who moves `$TSUKU_HOME` keeps a
working install.

#### Alternatives Considered

**(b) Absolute paths to the dep `lib/` directories** — Simpler to
implement (the existing `set_rpath` action already does this with template
substitution) but bakes `$TSUKU_HOME` into the binary. A user move breaks
the install. The existing `Type == "library"` `fixLibraryDylibRpaths`
function already emits absolute paths today; generalizing without
addressing this codifies the bug.

**(a) Lift the gate without specifying path form** — Inherits the absolute
form from the existing `fixLibraryDylibRpaths`. Underspecified. Listed
separately because the conceptual move ("apply the existing function to
tools") is sound; it just needs the path-form decision packaged with it.

## Decision Outcome

The three decisions integrate as one mechanism:

1. **Recipe** declares `chained_lib_dependencies = [...]` at metadata level.
   This is the single source of truth for what the binary needs to find at
   runtime.
2. **Plan generation** (in `internal/executor/plan_generator.go` and
   `internal/install/install_deps.go`) reads the new field and adds each
   entry to `ctx.Dependencies.ChainedLibs` (a new field on the executor
   `Dependencies` struct, parallel to `InstallTime` and `RuntimeDeps`).
3. **The `homebrew` action's relocate phase** consumes
   `ctx.Dependencies.ChainedLibs` directly. The function previously known
   as `fixLibraryDylibRpaths` is renamed `fixDylibRpathChain` and applies
   to any recipe that declares the new field, regardless of `Type`. A new
   sibling function `fixElfRpathChain` does the equivalent on Linux ELF
   binaries.
4. **Patching** uses `$ORIGIN`-relative (Linux) or `@loader_path`-relative
   (macOS) RPATHs into `$TSUKU_HOME/libs/<dep>-<version>/lib`. The path
   layout is computed at install time using `filepath.Rel` after
   `filepath.EvalSymlinks` on both ends.

The new field replaces no existing field. Library recipes
(`Type == "library"`) keep their existing `dependencies`-driven path —
they don't need the new field, since their own `lib/` is co-located with
their binary and the existing logic already chains their build-time deps.
Tool recipes that don't declare the new field run unchanged through
`homebrew + install_binaries`, with the existing `fixMachoRpath` /
`fixElfRpath` doing same-tree RPATH setup but no dep walking.

## Solution Architecture

### Components

| Component | Change | File |
|-----------|--------|------|
| Recipe schema | Add `MetadataSection.ChainedLibDependencies []string` | `internal/recipe/types.go` |
| Recipe validator | Validate each entry resolves to an installed library recipe; reject empty strings; reject duplicates | `internal/recipe/validator.go` |
| Executor `Dependencies` struct | Add `ChainedLibs map[string]string` (name → version) | `internal/executor/types.go` (or wherever `ResolvedDeps` lives) |
| Plan generator | Resolve each `chained_lib_dependencies` entry to the planned dep version; populate `ChainedLibs` | `internal/install/install_deps.go`, `internal/executor/plan_generator.go` |
| `homebrew` action's relocate | Consume `ctx.Dependencies.ChainedLibs` instead of `InstallTime`-only walk; apply on tool recipes too | `internal/actions/homebrew_relocate.go` |
| New macOS function `fixDylibRpathChain` | Generalized form of `fixLibraryDylibRpaths` with `Type == "library"` gate replaced by "deps non-empty"; emits `@loader_path`-relative paths | `internal/actions/homebrew_relocate.go` |
| New Linux function `fixElfRpathChain` | Mirror of `fixDylibRpathChain` for ELF; emits `$ORIGIN`-relative paths via `patchelf --add-rpath` (additive — does not replace `fixElfRpath`'s same-tree handling) | `internal/actions/homebrew_relocate.go` |
| Test surface | Existing `homebrew_relocate_test.go` gains tool-recipe chain tests; `executor_test.go` gains `TestSetResolvedDeps_ChainedLibs`; recipe golden fixtures regenerate | `internal/actions/homebrew_relocate_test.go`, `internal/executor/plan_generator_test.go` |

### Data Flow

```
recipes/t/tmux.toml
  metadata.chained_lib_dependencies = ["libevent", "utf8proc", "ncurses"]
            |
            v
internal/recipe/loader.go
  parses TOML; populates MetadataSection.ChainedLibDependencies
            |
            v
internal/install/install_deps.go::resolveDependencies
  resolves each entry to an installed library recipe + version
  populates ResolvedDeps.ChainedLibs = {"libevent":"2.1.12", ...}
            |
            v
internal/executor/plan_generator.go
  attaches ResolvedDeps to InstallationPlan
            |
            v
sandbox/host install runs the homebrew action
            |
            v
internal/actions/homebrew.go::Decompose
  homebrew bottle is downloaded and unpacked into work_dir/.install/
            |
            v
internal/actions/homebrew_relocate.go::Execute
  fixMachoRpath / fixElfRpath rewrite cellar refs to @rpath/$ORIGIN basenames
  if ctx.Dependencies.ChainedLibs is non-empty:
    fixDylibRpathChain (macOS) or fixElfRpathChain (Linux)
    walks ChainedLibs, emits relative paths into $TSUKU_HOME/libs/<dep>-<ver>/lib
            |
            v
internal/actions/install_binaries.go::Execute
  copies work_dir/.install/ to $TSUKU_HOME/tools/<recipe>-<version>/
  symlinks bin/ entries into tools/current/
            |
            v
verify step runs `<tool> --version`
  runtime linker resolves chained dylibs via the patched RPATH
```

### Recipe-author surface

A migrated `recipes/t/tmux.toml` looks like this:

```toml
[metadata]
name = "tmux"
description = "Terminal multiplexer"
version_format = "raw"
curated = true
chained_lib_dependencies = ["libevent", "utf8proc", "ncurses"]

# glibc Linux: Homebrew bottle. The chained_lib_dependencies above drive the
# RPATH chain — no per-step dependencies list needed.
[[steps]]
action = "homebrew"
formula = "tmux"
when = { os = ["linux"], libc = ["glibc"] }

[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["bin/tmux"]
when = { os = ["linux"], libc = ["glibc"] }

# (musl Linux + darwin steps follow the same shape with no per-step deps.)

[verify]
command = "tmux -V"
pattern = "tmux {version}"
```

Comparing to the current `recipes/w/wget.toml`, the per-step
`dependencies = [...]` list disappears; the metadata-level
`runtime_dependencies = [...]` (which today drives wrapper PATH) stays only
when wrapper-PATH semantics are actually wanted.

### Validator additions

- Each `chained_lib_dependencies` entry must match the recipe-name pattern
  `^[a-z0-9._-]+$`. `/`, `\`, `..`, leading `-`, and null bytes are
  rejected. This matches existing recipe-name validation in
  `internal/distributed/cache.go::validateRecipeName` and
  `internal/index/rebuild.go::isValidRecipeName`; the validator should
  promote that check to a shared helper used uniformly.
- Each entry must resolve to a recipe that produces a `Type == "library"`
  recipe. Tool recipes are not valid chained-lib targets (their lib output
  isn't predictable).
- Empty list is allowed (means "no chaining needed"). Empty strings inside
  the list are rejected.
- Duplicates are rejected.
- A non-empty `chained_lib_dependencies` on a recipe that has no `homebrew`
  step is rejected (the field has no consumer).

### Backward compatibility

- Recipes without `chained_lib_dependencies`: no change.
- Library recipes with per-step `dependencies`: no change. The existing
  `fixLibraryDylibRpaths` path stays for them. The new `fixDylibRpathChain`
  / `fixElfRpathChain` only fires on the new field.
- Tool recipes with per-step `dependencies` (today: `git`, `wget`): authors
  are encouraged to migrate to `chained_lib_dependencies`, but no migration
  is forced — the existing per-step path keeps working through the
  homebrew action's existing decomposition.

## Implementation Approach

### Phase 1 — Schema + executor wiring (no behavior change)

Add `MetadataSection.ChainedLibDependencies` to the recipe schema. Add the
validator rules. Add `ResolvedDeps.ChainedLibs` to the executor types.
Wire `chained_lib_dependencies` resolution through `install_deps.go`
into the resolved-deps map. No functional behavior change yet —
`ChainedLibs` is populated but unused. Recipes that declare the field
validate cleanly but don't yet get RPATH chaining.

**Acceptance:** new field validates with `tsuku validate --strict`;
recipes that declare it don't break existing tests.

### Phase 2 — macOS: generalize `fixLibraryDylibRpaths` to `fixDylibRpathChain`

Rename the function. Replace the `Type == "library"` gate with a check on
`ctx.Dependencies.ChainedLibs` (non-empty → run the walk; empty → skip).
Convert the path-emit form from absolute to `@loader_path`-relative using
`filepath.Rel` over `EvalSymlinks` on both ends. After `filepath.Rel`,
verify that `filepath.Join(loaderDir, relPath)` resolves back inside
`$TSUKU_HOME/libs/` — fail the install with a clear error if not (defense
in depth against any path-traversal that slipped past the validator). Add
tests covering tool recipes and library recipes (the latter now use the
new field; the existing implicit path stays for not-yet-migrated library
recipes).

Existing `Type == "library"` recipes that don't declare the new field
stay on the implicit path with absolute paths (no behavior change). A
recipe that opts into the new field starts emitting relative paths.

**Acceptance:** `tmux` recipe with `chained_lib_dependencies = ["libevent",
"utf8proc", "ncurses"]` installs and runs cleanly on macOS amd64 + arm64
in the sandbox matrix.

### Phase 3 — Linux: add `fixElfRpathChain`

New function — mirror of `fixDylibRpathChain` using `patchelf --add-rpath`
with `$ORIGIN`-relative paths. Wired into `fixElfRpath`'s caller as an
additive step (does not replace the existing same-tree RPATH). Same
defense-in-depth `filepath.Join` post-check as Phase 2.

**Acceptance:** `tmux` recipe with the same `chained_lib_dependencies`
installs and runs cleanly on every Linux family (debian, rhel, arch, suse,
alpine — alpine still uses `apk_install`, the chain isn't relevant there).

### Phase 4 — Migrate the punted recipes

Update affected recipes one-by-one to use the new field. Suggested order
based on impact:

1. `tmux` (closes #2377; demonstrates the integrated mechanism)
2. The 13 recipes that punted on darwin (`bedtools`, `cbonsai`,
   `cdogs-sdl`, etc.) — opt-in migrations
3. The next wave of top-100 tools that needed this to be curated (`bat`,
   `eza`, `htop`, `delta`, etc.)

Each migration is a small recipe-only PR. The acceptance criterion per
recipe is the same as Phase 2/3.

### Phase 5 — Optional: deprecate per-step `dependencies` for tool recipes

After enough recipes migrate, document that per-step `dependencies` for
tool-recipe homebrew steps is a legacy form. Keep it working for `git`
and `wget` for backward compat, but recipe-author docs steer new authors
to `chained_lib_dependencies`. No forced migration.

## Security Considerations

The chain mechanism extends the trust boundary that already governs
`homebrew` bottles: bottle contents come from `homebrew/core` GHCR with
unchanged sha256 verification, and patches are applied only inside
`$TSUKU_HOME/{tools,libs}/<recipe>-<version>/`. The new field
(`chained_lib_dependencies`) is recipe-controlled, so a recipe is the
trust boundary for the chain content.

### Threat model

A malicious or buggy recipe in a third-party tap could declare an entry
that, after string interpolation into a filesystem path, escapes
`$TSUKU_HOME/libs/`. For example,
`chained_lib_dependencies = ["../../etc"]` would, with `filepath.Join`'s
`Clean` semantics, collapse upward and produce an RPATH target outside
the install root. The runtime linker resolves the resulting
`$ORIGIN` / `@loader_path`-relative RPATH on every invocation, so this
is a load-path-redirection primitive (the patched binary loads shared
libraries from an attacker-chosen location), not a write primitive.

Argument injection into `patchelf` or `install_name_tool` via shell
metacharacters is not a concern — Go's `exec.Command` uses `execve(2)`
directly and does not invoke a shell. Option injection (a dep name
beginning with `-` being interpreted as a tool flag) is mitigated by the
fact that the constructed argument always starts with the absolute
`LibsDir` path, but the validation below removes the dependency on that
indirect mitigation.

### Mitigations

1. **Strict validation of `chained_lib_dependencies` entries at recipe
   load time.** Each entry must match `^[a-z0-9._-]+$` and reject `/`,
   `\`, `..`, leading `-`, and null bytes. This matches existing
   recipe-name validation in the registry cache and index-rebuild paths
   and should be promoted to a shared helper applied uniformly. Phase 1
   of the implementation includes this validator change.

2. **Defense-in-depth at patch time.** Both `fixDylibRpathChain`
   (macOS) and `fixElfRpathChain` (Linux) compute the relative RPATH
   via `filepath.Rel` after `EvalSymlinks`, then verify
   `filepath.Join(loaderDir, relPath)` resolves back into
   `$TSUKU_HOME/libs/`. An entry that fails this check is rejected
   (the install fails with a clear error rather than producing a binary
   with an out-of-tree RPATH). This is included in Phases 2 and 3.

3. **No user-specific data is embedded in patched binaries.** The
   chosen RPATH form is `$ORIGIN` / `@loader_path`-relative, so the
   binary does not embed `$TSUKU_HOME` or any absolute user path. This
   preserves portability across `$TSUKU_HOME` moves and avoids leaking
   home-directory paths through binaries that may be copied or shared.

### Out of scope

Trust in the bottle contents themselves (a compromised
`homebrew/core` upload) is governed by the existing sha256 verification
in the homebrew action and is not changed by this design. Trust in the
recipe registry (a compromised recipe in `tsuku/recipes` or a
third-party tap) is the same trust boundary as every other recipe field
and is mitigated by the validator rules above.

## Consequences

### Positive

- **Unblocks the next wave of curated tool recipes.** The 10+ top-100
  tools whose homebrew bottles depend on non-system shared libraries
  (`node`, `ripgrep`, `bat`, `eza`, `neovim`, `htop`, `delta`, `ollama`,
  `shellcheck`, `hadolint`) gain a clear, single-field opt-in to make
  those bottles work cleanly in the tsuku layout.
- **Removes the per-recipe N×M coordination cost.** Today, getting a
  tool to chain a new dep means hand-coordinating with the dep recipe
  to publish the right dylib (the `wget` ↔ `gettext` `libintl.8.dylib`
  patch is a representative example). The new field consumes whatever
  the dep recipe's standard library install ships.
- **Aligns macOS and Linux semantics.** Recipe authors declare one
  field; the engine handles the platform-specific RPATH plumbing.
  Linux ELF gains a chaining function that didn't exist at all before.
- **Restores `$TSUKU_HOME`-portability.** Binaries patched with the
  new mechanism use `$ORIGIN` / `@loader_path`-relative RPATHs. The
  existing recipe-side `set_rpath` action bakes absolute paths
  (`{libs_dir}/...`) into binaries; the new mechanism is a portability
  improvement over that pattern.
- **Cleans up the wget pattern.** The duplicated metadata-level
  `runtime_dependencies` + per-step `dependencies` lists in `wget` and
  `git` can be replaced by a single `chained_lib_dependencies` entry,
  with no functional behavior change once recipes migrate.

### Negative

- **One new recipe-schema field to learn.** `chained_lib_dependencies`
  is a third dep-set field alongside `runtime_dependencies` (wrapper
  PATH) and per-step `dependencies` (build-time inputs). Recipe-author
  docs must explain when each applies. The trade-off was deliberate to
  avoid silently overloading either of the existing fields.
- **Generalizing `fixLibraryDylibRpaths` is a behavior change for the
  function's existing callers.** Library recipes that don't yet
  declare `chained_lib_dependencies` keep the existing implicit path
  with absolute RPATHs; library recipes that opt into the new field
  switch to relative RPATHs. The two paths produce different binaries
  for the same recipe state. Test coverage must lock in both behaviors
  to prevent regressions during the migration window.
- **Discoverability of relative-RPATH semantics.** A binary's RPATH is
  not human-friendly to inspect. `tsuku doctor` could grow a check
  that walks installed binaries and validates their RPATH chain
  resolves cleanly — out of scope for this design but a natural
  follow-up.
- **The validator's "entry resolves to an installed library recipe"
  rule depends on the dep being installable on the target platform.**
  A recipe declaring `chained_lib_dependencies = ["foo"]` where `foo`
  is library-only on Linux but tool-only on darwin would need to be
  caught at validate time, not install time. The validator must check
  the dep's `Type` per-platform.

### Mitigations

- Document `chained_lib_dependencies` in the recipe-author guide
  alongside the other dep-set fields, with a decision table:
  - "Need this binary to run after install?" → already handled by
    `homebrew + install_binaries`.
  - "Need a library to be on PATH for the binary's wrapper?" →
    `runtime_dependencies`.
  - "Need a library to be in the binary's load path (RPATH)?" →
    `chained_lib_dependencies`.
  - "Need a tool at build time?" → per-step `dependencies`.
- Migrate the existing wget/git pattern in a follow-up PR (separate
  from the core change) so the old behavior is preserved while
  authors transition.
- Add a `tsuku validate --strict` warning when a tool recipe uses a
  `homebrew` step but declares zero dep fields — the recipe is
  probably implicitly relying on system-available libraries (today's
  silent failure mode for tools like `tmux` on minimal containers).

