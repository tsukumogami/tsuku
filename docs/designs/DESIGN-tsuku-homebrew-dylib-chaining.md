---
status: Proposed
problem: |
  tsuku's homebrew action does not chain dylibs from sibling tsuku-installed
  library deps into a tool recipe's RPATH on either Linux or macOS. The
  mechanism exists for `Type == "library"` recipes (`fixLibraryDylibRpaths`)
  but is gated to that one type, so tool recipes whose homebrew bottles
  reference non-system shared libraries can't wire those refs to matching
  tsuku-installed deps. The result is a steady stream of recipes that ship
  with `supported_os = ["linux"]` or `unsupported_platforms` because the
  bottle path doesn't work cleanly. Exploration measured >15 affected
  recipes (top-100 strict: 10; counting workaround-dependent existing
  recipes: 19+; counting macOS punts: 26+). Round 2 surfaced a deeper bug:
  recipes are systematically under-declared (git misses zlib; wget misses
  zlib + libuuid; coreutils misses acl + attr); the test containers' system
  libs shadow the gaps, so the bug is invisible in CI but breaks on
  minimal containers.
decision: |
  Strengthen the existing `homebrew` action to chain dep dylibs automatically.
  Two coordinated mechanisms: (1) walk the recipe's existing
  `metadata.runtime_dependencies` and emit RPATH entries pointing at each
  dep's `lib/` directory; (2) at install time after bottle extraction, run
  `readelf -d` (Linux) / `otool -L` (macOS) on the binary, identify NEEDED
  SONAMES not provided by the system, cross-reference against
  tsuku-installable libraries, and warn (or auto-include) when the recipe's
  declarations don't cover them. RPATH entries use `$ORIGIN`-relative
  (Linux) or `@loader_path`-relative (macOS) paths into the stable
  `$TSUKU_HOME/libs/<dep>-<version>/lib` layout, written via
  `patchelf --force-rpath --set-rpath` (DT_RPATH, not DT_RUNPATH).
rationale: |
  Recipe authors did not converge on any opt-in workaround pattern (across
  1168 homebrew-using recipes, exactly 0 use the existing `set_rpath`
  primitive to chain deps). A new opt-in primitive would face the same
  adoption gap, and a new declarative field would force authors to learn
  RPATH semantics that the engine should handle. Reusing
  `runtime_dependencies` matches author intent ("this dep is needed at
  runtime") and adds zero new fields. Empirical stress tests (round 2)
  confirmed nonsense RPATH entries are inert (~10 stat syscalls each), so
  the engine doesn't need to filter by dep `Type` — but they also surfaced
  that author declarations are systematically incomplete, so the engine
  must back-stop with binary-NEEDED extraction. Relative-path patching
  survives `$TSUKU_HOME` moves; `DT_RPATH` avoids subtle resolution
  differences with `DT_RUNPATH` that broke wget's libunistring lookup in
  testing.
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
- **No new field for author-facing concepts.** The author's mental model is
  "this dep is needed at runtime" — already expressible via
  `runtime_dependencies`. A new field for "RPATH chaining" leaks engine
  internals into the recipe schema and forces authors to learn RPATH
  semantics. Reuse the existing field instead.
- **Cross-platform consistency.** macOS and Linux both have the gap. The
  solution should read the same on both platforms — recipe authors should
  not need to write platform-specific RPATH chains by hand.
- **Portability of installed binaries.** A binary installed at one
  `$TSUKU_HOME` should still work if the user moves `$TSUKU_HOME` to a new
  location. Absolute paths baked into binaries break this.
- **No new workaround that authors will ignore.** Recipe authors did not
  adopt the existing `set_rpath` action for this purpose. A new opt-in
  primitive would face the same fate. The fix must apply automatically
  when authors declare the dep set, not require them to remember a
  separate step.
- **Surface under-declared recipes loudly.** Empirical evidence (round 2
  exploration) shows recipes are systematically under-declared and the
  test containers' system libs shadow the gaps. The engine must include a
  back-stop that surfaces incomplete declarations rather than silently
  shipping broken-on-minimal-containers binaries.

## Considered Options

This design decomposes into four decisions, evaluated independently and
cross-validated. Decisions 1–3 came from the round-1 exploration; Decision
4 was surfaced by the round-2 empirical investigation, which proved that
declaration-only mechanisms are insufficient because recipes are routinely
under-declared.

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
reuse per-step `dependencies`; or introduce a new explicit field. Round
2's empirical stress tests (run inside docker containers) materially
changed which option ranks best.

**Key assumptions (validated in round 2):**
- Library recipes (`Type == "library"`) continue to use the existing
  implicit path that walks `ctx.Dependencies.InstallTime` from per-step
  `dependencies`. Augmentation only adds new behavior on top.
- Nonsense RPATH entries (paths that don't exist or contain no libraries)
  are inert at runtime. Empirical: 1000-entry RPATHs work; bogus paths
  cost ~10 stat() syscalls each.
- 298 recipes already declare `runtime_dependencies`. Of those, 219 are
  all-library, 41 all-tool, 38 mixed. Across 7 measured auto-generated
  recipes the declared deps are never over-declared (false-positive
  safe), only under-declared.

#### Chosen: Reuse `metadata.runtime_dependencies`

Recipes already declare deps that are needed at runtime via
`runtime_dependencies`. The engine adds a second consumer for that field:
the homebrew action's relocate phase walks the field, takes each dep's
`lib/` directory, and emits an RPATH entry. No new field is introduced;
no recipe migration is forced; existing recipes that already declare the
field automatically gain the chain.

The engine does not need to filter by dep `Type`. Empirical evidence
(round 2 lead 9, lead 10) confirms that nonsense RPATH entries — like the
non-existent `$TSUKU_HOME/libs/<tool>-<v>/lib` for a tool dep — are
inert at runtime. The naive walk is safe.

This option fails round-1's earlier rejection rationale ("backward-compat
risk too high") because that risk turned out to be empirically minimal:
no observed false positives, no observed misbehavior from extra RPATH
entries. The earlier "semantically muddled" objection ("the field would
mean three things") collapsed once the round-2 framing made clear that
all consumers implement the same author-facing semantic ("this dep is
needed at runtime") at different layers.

#### Alternatives Considered

**(b) Reuse per-step `dependencies`** — Wrong grain. Chained libs are a
property of the produced binary, not the build step. The wget recipe
already demonstrates the failure mode: a per-platform `dependencies` list
duplicated across glibc-Linux, musl-Linux, and darwin steps with no
guarantee they stay in sync. The field name doesn't communicate "load
path" — it reads as "install before running this step."

**(c) Introduce a new explicit field `chained_lib_dependencies`** —
Originally chosen in round 1; reversed after round 2. The original
rationale was "semantic clarity"; the round-2 reading is that the field
forces authors to learn RPATH semantics (a violation of one of this
design's own decision drivers) for negligible engine-side benefit.
Empirical evidence dissolved the safety arguments in favor of the new
field: nonsense entries don't break anything, false positives don't
occur, and the existing field already carries the right author-facing
semantic.

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

### Decision 4: Back-stop for under-declared recipes

(New decision, surfaced by round-2 empirical findings.) Recipes are
systematically under-declared: round-2 lead 11 found `git` missing zlib
(`libz.so.1`), `wget` missing zlib + libuuid (`libuuid.so.1`),
`coreutils` missing acl + attr. The gap is invisible in CI today because
the test containers ship those libs system-wide; on minimal containers
(or any environment where the system shadow disappears) the binaries
break.

Three options for back-stopping:

**Key assumptions:**
- A `readelf -d` (Linux) / `otool -L` (macOS) scan of the unpacked bottle
  binary is fast and reliable; the cost is dominated by the bottle
  download already incurred.
- Tsuku has — or can easily build — a SONAME → recipe-name index
  (which library recipe ships which SONAME). Existing library recipes
  declare their `outputs` lists, so the mapping is constructible at plan
  generation time.
- "System library" can be defined as "resolves via the container's
  `ldconfig` cache or default search path" — a runtime check, not a
  compile-time list.

#### Chosen: SONAME-driven completeness scan with auto-include

At install time, after the homebrew bottle is unpacked into the work dir
but before `install_binaries` copies it to the final location:

1. Run `readelf -d <binary>` (Linux) or `otool -L <binary>` (macOS) on
   each binary and shared library in the bottle.
2. Collect the `NEEDED` SONAMES.
3. For each SONAME, check (in order):
   - Is it a system library (resolves via `ldconfig` / default search)?
     → no action. The system-library check is the **first** filter and
     uses the runtime linker's actual resolution (e.g., `ldconfig -p`),
     not a tsuku-internal allowlist that could drift.
   - Look the SONAME up in the single-valued index. If a provider
     exists:
     - **Declared in `runtime_dependencies`:** no action; the chain
       step handles it.
     - **Not declared:** log a warning ("recipe under-declared: binary
       needs `libfoo.so.1` from `foo`; not in `runtime_dependencies`")
       and auto-include the dep's `lib/` dir in the chained RPATH so
       the install still works.
   - If no provider exists: log a coverage gap ("binary needs
     `libfoo.so.1` but no tsuku library recipe ships it; the bottle
     path is fragile on minimal containers"). A small known-gaps
     allowlist (maintained alongside the SONAME index) downgrades
     well-known unowned SONAMES (e.g., `libuuid.so.1`, `libacl.so.1`,
     `libattr.so.1` until library recipes for them land) to debug-level
     so install logs don't grow noisy. Entries are removed from the
     allowlist once the corresponding recipe exists.

If two library recipes claim the same `(platform, SONAME)`, the index
build fails at plan generation time with an error pointing at both
recipes (see Phase 2). Collisions are not a silent runtime concern.

The auto-include behavior closes the structural bug the round 2
exploration surfaced (recipes that work in CI but break on minimal
containers). The warning path keeps recipe authors honest by surfacing
declaration gaps that today are invisible.

#### Alternatives Considered

**(a) Validator-only warning (no auto-include)** — Surfaces the gap but
doesn't fix the install. Authors fix the recipe in the next PR cycle, but
in the meantime the install fails on minimal containers. Worse user
experience than auto-including, no meaningful safety benefit.

**(b) Auto-include silently (no warning)** — Closes the install but
leaves the declaration gap permanent. Recipes that should declare their
deps explicitly stay sloppy, hiding real coverage gaps (SONAMES that
nothing in tsuku ships) under the "it works for me" surface.

**(c) Skip the scan; trust author declarations only** — The original
round-1 design. Empirically rejected: round 2 found 100% of measured
recipes were under-declared by at least one SONAME. Trust-the-author
ships broken binaries.

## Decision Outcome

The four decisions integrate as one mechanism. No new recipe schema field
is introduced.

1. **Recipe** declares `runtime_dependencies = [...]` at metadata level
   (existing field; existing meaning).
2. **Plan generation** (in `internal/executor/plan_generator.go` and
   `internal/install/install_deps.go`) wires `RuntimeDependencies` into
   `ctx.Dependencies.RuntimeDeps` (today this field exists for the
   wrapper-PATH consumer but the values don't reach the executor's
   relocate context — the wiring fix that round-1 lead 4 identified).
3. **The `homebrew` action's relocate phase** acquires two new behaviors,
   sequenced after the existing `fixMachoRpath` / `fixElfRpath` per-binary
   pass (which removes stale RPATHs and sets the `@rpath` / `$ORIGIN`
   anchor). Running after that pass means the new chain entries survive
   the existing `--remove-rpath` step:
   - **SONAME completeness scan.** Runs first (still after
     `fixMachoRpath`/`fixElfRpath`). Calls `readelf -d` / `otool -L` on
     each binary in the unpacked bottle, classifies each NEEDED SONAME
     using the SONAME index, and produces a local `[]chainEntry` slice
     in Execute scope listing auto-included deps. Does **not** mutate
     `ctx.Dependencies` — the `ctx` populated at plan time stays
     read-only inside Execute.
   - **Declared-deps chain.** Walks `ctx.Dependencies.RuntimeDeps`
     ∪ the auto-included `[]chainEntry` slice. For each entry,
     computes a `$ORIGIN`/`@loader_path`-relative RPATH pointing at
     `$TSUKU_HOME/libs/<dep>-<version>/lib`. Adds the entry via
     `patchelf --force-rpath --set-rpath` (Linux) or
     `install_name_tool -add_rpath` (macOS). The walk does **not**
     filter by dep `Type` — empirical evidence (round-2 leads 9, 10)
     shows nonsense entries are inert.

The function previously known as `fixLibraryDylibRpaths` is renamed
`fixDylibRpathChain` (macOS); a new `fixElfRpathChain` provides the
Linux equivalent. To avoid overloading semantics, these new functions
read **`RuntimeDeps`** (and the local auto-included slice) only — they
do **not** also walk `ctx.Dependencies.InstallTime` the way
`fixLibraryDylibRpaths` did. The existing library-recipe install-time
chain (which used `InstallTime`) is migrated separately as a Phase 3
sub-step with golden-fixture coverage so the absolute → relative path
shift is reviewable as its own diff.
4. **Patching** uses `$ORIGIN`-relative (Linux) or `@loader_path`-relative
   (macOS) RPATHs into `$TSUKU_HOME/libs/<dep>-<version>/lib`. The path
   layout is computed at install time using `filepath.Rel` after
   `filepath.EvalSymlinks` on both ends. **Linux uses `DT_RPATH`
   (`patchelf --force-rpath --set-rpath`), not `DT_RUNPATH`** —
   empirical: `DT_RUNPATH` had subtle resolution differences that
   broke wget's libunistring lookup.

Library recipes (`Type == "library"`) get the same chain — their own
`runtime_dependencies` (or per-step `dependencies`, for backward compat)
drives the walk. The previously-implicit library chain becomes explicit
through the same code path. Tool recipes that don't declare any
`runtime_dependencies` get the SONAME scan; if the binary references
non-system SONAMES that map to tsuku libraries, the scan auto-includes
them and warns. If everything resolves via system libs, the scan is a
no-op.

## Solution Architecture

### Components

No recipe schema changes. The work is engine-side.

| Component | Change | File |
|-----------|--------|------|
| Plan generator wiring | Populate `ctx.Dependencies.RuntimeDeps` from `MetadataSection.RuntimeDependencies` (today this field exists for the wrapper consumer but never reaches the executor relocate context) | `internal/install/install_deps.go`, `internal/executor/plan_generator.go` |
| `homebrew` action's relocate phase | (1) Consume `ctx.Dependencies.RuntimeDeps` and emit per-dep RPATH entries; (2) run SONAME completeness scan on the unpacked binary and auto-include + warn on under-declared SONAMES | `internal/actions/homebrew_relocate.go` |
| Renamed/generalized macOS function | `fixLibraryDylibRpaths` → `fixDylibRpathChain`. Replace `Type == "library"` gate with "RuntimeDeps non-empty". Emit `@loader_path`-relative paths via `install_name_tool -add_rpath`. Add `filepath.Join` post-check that the resolved path stays inside `$TSUKU_HOME/libs/`. | `internal/actions/homebrew_relocate.go` |
| New Linux function | `fixElfRpathChain`. Mirror of `fixDylibRpathChain` for ELF. Emits `$ORIGIN`-relative paths via `patchelf --force-rpath --set-rpath` (DT_RPATH, not DT_RUNPATH). Same `filepath.Join` post-check. | `internal/actions/homebrew_relocate.go` |
| New SONAME → recipe index | Build at plan-generation time: scan all installable library recipes, parse their `outputs` for `lib/lib*.so.*` and `lib/lib*.*.dylib` patterns, build a single-valued map `(platform, SONAME) → providing recipe`. The parser requires the path to start with `lib/` and contain no `..` segments, and the basename to start with `lib`. Other entries are skipped (they're not SONAMES). On a duplicate `(platform, SONAME)` insert, index construction fails with a clear error pointing at both providing recipes — collisions are loud, not silent. | `internal/sonameindex/sonameindex.go` (new leaf package — placed outside `internal/install/` so the executor and the homebrew action can both import it without inverting the `executor → install` dependency direction) |
| New SONAME completeness scanner | Runs after bottle extraction. Calls `readelf -d` / `otool -L` to get NEEDED SONAMES. For each, classifies (system / declared / under-declared / uncovered). Warns on under-declared, auto-includes their lib dirs in the chain. | `internal/actions/homebrew_relocate.go` |
| Recipe validator | Validate `runtime_dependencies` entry name pattern (`^[a-z0-9._-]+$`); reject empty strings, `..`, `/`, null bytes, leading `-`. Validate each entry resolves to an installable recipe. Promote `isValidRecipeName` from `internal/distributed/cache.go` and `internal/index/rebuild.go` into a shared helper. | `internal/recipe/validator.go` |
| Test surface | New tests for the SONAME index, completeness scanner, and the chain walk on tool recipes. Existing `Type == "library"` tests carry over by parameterization. Plan-cache golden fixtures regenerate. | `internal/install/soname_index_test.go` (new), `internal/actions/homebrew_relocate_test.go`, `internal/executor/plan_generator_test.go` |

### Data Flow

```
recipes/t/tmux.toml (no schema changes — uses existing field)
  metadata.runtime_dependencies = ["libevent", "utf8proc", "ncurses"]
            |
            v
internal/recipe/loader.go
  parses TOML (existing path)
            |
            v
internal/install/install_deps.go
  resolves each runtime_dependencies entry to recipe + version
  populates ctx.Dependencies.RuntimeDeps (was previously only used for
  wrapper PATH; now also reaches the relocate context)
            |
            v
internal/install/soname_index.go (new)
  at plan-generation time, builds map from every installable library
  recipe's outputs to their SONAMES (e.g., "libpcre2-8.so.0" → "pcre2")
            |
            v
sandbox/host install runs the homebrew action
            |
            v
internal/actions/homebrew.go::Decompose
  bottle is downloaded and unpacked into work_dir/.install/
            |
            v
internal/actions/homebrew_relocate.go::Execute
  (existing: fixMachoRpath / fixElfRpath rewrite cellar refs to @rpath/$ORIGIN)
  (new) SONAME completeness scan on each binary:
    readelf -d <bin>  /  otool -L <bin>
    for each NEEDED SONAME not resolved by system ldconfig:
      look up in soname_index
      if mapped to a tsuku library:
        if in RuntimeDeps: chain step handles it
        else:               warn (under-declared) + auto-include in chain
      else:
        log coverage gap (no tsuku recipe ships this SONAME)
  (new) chain walk over ctx.Dependencies.RuntimeDeps + auto-included deps:
    for each library dep:
      compute @loader_path/$ORIGIN-relative path to $TSUKU_HOME/libs/<dep>-<v>/lib
      verify filepath.Join stays inside $TSUKU_HOME/libs/
      add via install_name_tool / patchelf --force-rpath --set-rpath
            |
            v
internal/actions/install_binaries.go::Execute
  copies work_dir/.install/ to $TSUKU_HOME/tools/<recipe>-<v>/
  symlinks bin/ entries into tools/current/
            |
            v
verify step runs `<tool> --version`
  runtime linker resolves chained dylibs via the patched RPATH
```

### Recipe-author surface

What recipe authors do **not** have to do:
- Learn a new schema field.
- Hand-coordinate dylib SONAMES with their dep recipes.
- Add `set_rpath` chain steps.
- Know about `@rpath`/`$ORIGIN` semantics.

What recipe authors **continue** to do:
- Declare `runtime_dependencies = ["libfoo", "libbar"]` for the libraries
  their tool needs at runtime — exactly as the existing field semantics
  already promise.

A migrated `recipes/t/tmux.toml`:

```toml
[metadata]
name = "tmux"
description = "Terminal multiplexer"
version_format = "raw"
curated = true
runtime_dependencies = ["libevent", "utf8proc", "ncurses"]

[[steps]]
action = "homebrew"
formula = "tmux"
when = { os = ["linux"], libc = ["glibc"] }

[[steps]]
action = "install_binaries"
install_mode = "directory"
outputs = ["bin/tmux"]
when = { os = ["linux"], libc = ["glibc"] }

# (musl Linux + darwin steps follow the same shape — no per-step deps,
#  no platform-specific RPATH fields.)

[verify]
command = "tmux -V"
pattern = "tmux {version}"
```

Comparing to the current `recipes/w/wget.toml`, the per-step
`dependencies = [...]` lists can disappear (the metadata-level
`runtime_dependencies` is the single source). Authors who haven't
migrated keep working — the per-step `dependencies` path also feeds
into the chain (it always did, for library recipes; now also for tools).

### Validator additions

- Each `runtime_dependencies` entry must match the recipe-name pattern
  `^[a-z0-9._-]+$`. `/`, `\`, `..`, leading `-`, and null bytes are
  rejected. This matches existing recipe-name validation in
  `internal/distributed/cache.go::validateRecipeName` and
  `internal/index/rebuild.go::isValidRecipeName`; the validator should
  promote that check to a shared helper.
- Each entry must resolve to an installable recipe.
- Empty strings inside the list are rejected.
- Duplicates are rejected.

### Backward compatibility

- **Recipes without `runtime_dependencies`** (the basic `homebrew +
  install_binaries` shape): the chain walk has no input, so no RPATH
  entries are added beyond what `fixMachoRpath`/`fixElfRpath` already
  produce. The SONAME completeness scan still runs; if the binary needs
  non-system SONAMES that map to tsuku libraries, they're auto-included
  with a warning. Recipes that need nothing beyond system libs are
  unchanged.
- **Library recipes with per-step `dependencies`** (`pcre2`, `libnghttp3`,
  etc.): the chain walk picks up both `runtime_dependencies` and
  `ctx.Dependencies.InstallTime` (existing path). Behavior may shift from
  absolute-path RPATHs to relative ones (a portability improvement); the
  test surface locks both shapes during the migration window.
- **Tool recipes with declared `runtime_dependencies`** (`git`, `wget`,
  316 auto-generated batch recipes): get the chain automatically. The
  empirical evidence shows their declarations are typically correct
  (no over-declaration) but routinely under-declared; the SONAME scan
  closes the gap with a warning that authors can address in a follow-up
  PR.

## Implementation Approach

### Phase 1 — Wire `runtime_dependencies` into the executor's relocate context

Today `metadata.runtime_dependencies` populates the wrapper-PATH consumer
but does not reach `ctx.Dependencies` for the homebrew action's relocate
phase (round-1 lead 4). Add the wiring in `install_deps.go` and
`plan_generator.go`. Add validator rules: name pattern (`^[a-z0-9._-]+$`),
no `..`, no `/`, no leading `-`, no null bytes. Promote `isValidRecipeName`
from `internal/distributed/cache.go` and `internal/index/rebuild.go` into a
shared helper used by the validator.

No functional behavior change yet beyond the validator strictness. Recipes
that already declare `runtime_dependencies` keep working; the executor
just sees the deps now.

**Acceptance:** existing recipes with `runtime_dependencies` still pass
all CI; `runtime_dependencies` reaches `ctx.Dependencies.RuntimeDeps` in
plan-cache golden fixtures. Add a docstring on the `RuntimeDependencies`
field that explicitly enumerates its consumers (today: wrapper-PATH;
post-design: also RPATH chain via `fixDylibRpathChain` /
`fixElfRpathChain`). The docstring is the contract pin so future
maintainers see the dual consumption without code archaeology.

### Phase 2 — Build the SONAME → recipe index

New leaf package `internal/sonameindex/sonameindex.go`. The package
lives outside `internal/install/` so the executor (`internal/executor/`)
and the homebrew action (`internal/actions/`) can both import it
without inverting the existing `executor → install` dependency
direction.

At plan-generation time, walk all installable library recipes, parse
their `outputs` lists for `lib/lib*.so.*` (Linux) and `lib/lib*.*.dylib`
(macOS) patterns, build a single-valued map
`(platform, SONAME) → providing recipe`. Cache per platform.

**Parser validation.** Path must start with `lib/`, must not contain
`..` segments, and the basename must start with `lib`. Other entries
are skipped (they're not SONAMES — for example, a header file under
`include/`). This is loose-on-purpose: the registry is a trust
boundary, and the recipe-name validator already covers traversal
through dep names. The parser's job is to skip non-SONAME entries
cleanly, not to reject malformed inputs.

**Collision handling.** On a duplicate `(platform, SONAME)` insert,
index construction fails with a clear error pointing at both
providing recipes. Collisions are loud at plan-generation time, never
silent. The single-valued index keeps the auto-include code path
trivially deterministic: at most one provider exists per SONAME or the
plan generation already aborted.

The index needs to handle:
- Full SONAMES: `libpcre2-8.so.0`, `libpcre2-8.0.dylib`
- Per-platform variants: a recipe's `outputs` may declare both Linux and
  macOS forms; index both into the same provider entry, keyed by
  `(platform, SONAME)`.
- Versioned variants: `libfoo.so` → `libfoo.so.1` → `libfoo.so.1.2.3`;
  the index should map all three to the same provider.

**Known-gap allowlist.** A small static map of SONAMES with no
tsuku-recipe coverage today (`libuuid.so.1`, `libacl.so.1`,
`libattr.so.1`, etc.) downgrades the "no provider" log line to
debug-level for those entries. Each allowlist entry is removed when
the corresponding library recipe is authored, so the noise-control
mechanism self-clears as Phase 7-style follow-up work lands.

**Acceptance:** index has entries for every dylib output in the
curated library recipe set (`pcre2`, `libnghttp3`, `libevent`,
`utf8proc`, plus the 134 others); unit tests assert correct mapping
and that duplicate inserts fail with a clear error.

### Phase 3 — macOS: generalize `fixLibraryDylibRpaths` to `fixDylibRpathChain`

Rename the function. Replace the `Type == "library"` gate with a check
on `ctx.Dependencies.RuntimeDeps` non-empty. Convert the path-emit
form from absolute to `@loader_path`-relative using `filepath.Rel`
over `EvalSymlinks` on both ends. After `filepath.Rel`, verify that
`filepath.Join(loaderDir, relPath)` resolves back inside
`$TSUKU_HOME/libs/`. A failed check fails the install with a clear
error before any `install_name_tool -add_rpath` runs for that entry.
The `work_dir` is disposable, so a per-entry abort is sufficient —
any partial patching that occurred on prior entries within the same
chain step is in `work_dir` and is discarded with the rest of the
unpacked bottle on failure. No `$TSUKU_HOME/{tools,libs}/` content is
touched until the relocate phase completes successfully.

The new function reads `RuntimeDeps` (and the local auto-included
slice from the SONAME scan) only; it does not also walk
`ctx.Dependencies.InstallTime` the way the old
`fixLibraryDylibRpaths` did. The library-recipe install-time chain
(absolute paths via `InstallTime`) migrates to relative paths in a
**separate sub-step within Phase 3**, with its own golden-fixture diff
so the absolute → relative shift is reviewable in isolation. This
keeps the rename's contract narrow: one function, one source field.

**Order constraint.** The chain step runs **after** the existing
per-binary `fixMachoRpath` pass (which calls `--remove-rpath` to wipe
stale entries and sets the `@rpath` anchor). Running after that pass
means the new chain entries survive the wipe.

**Acceptance:** `tmux` recipe with `runtime_dependencies = ["libevent",
"utf8proc", "ncurses"]` installs and runs cleanly on macOS amd64 + arm64
in the sandbox matrix. The 4 existing `Type == "library"` recipes
(`pcre2`, `libnghttp3`, `libevent`, `utf8proc`) still pass with their
existing dep declarations.

### Phase 4 — Linux: add `fixElfRpathChain`

New function. Mirror of `fixDylibRpathChain` using
`patchelf --force-rpath --set-rpath` (writes `DT_RPATH`). Empirical
evidence (round-2 lead 8) showed `--add-rpath` (which writes
`DT_RUNPATH`) has subtle resolution differences that broke wget's
libunistring lookup; `DT_RPATH` is correct. Same per-entry
`filepath.Join` defense-in-depth post-check; same order constraint
(runs after the per-binary `fixElfRpath` pass).

**Acceptance:** `tmux` recipe with `runtime_dependencies` installs and
runs cleanly on every Linux family (debian, rhel, arch, suse, alpine —
alpine still uses `apk_install`, the chain isn't relevant there). `git`,
`wget` continue to pass.

### Phase 5 — SONAME completeness scan + auto-include

Implement the scanner. After bottle extraction (and after the per-binary
`fixMachoRpath`/`fixElfRpath` pass), for each binary in the unpacked
tree run `readelf -d` (Linux) or `otool -L` (macOS), collect NEEDED
SONAMES, classify each:

- Resolves via system ldconfig / default search → no action
- Maps to a tsuku library in the SONAME index AND in `RuntimeDeps` →
  no action; chain walk handles it
- Maps to a tsuku library AND not in `RuntimeDeps` → log warning,
  add to the local auto-included `[]chainEntry` slice (the
  `RuntimeDeps` data on `ctx` is not mutated)
- No tsuku recipe ships this SONAME → log coverage gap (downgraded
  to debug-level for entries on the known-gap allowlist; see Phase 2)

The chain-emit loop in Phases 3/4 then iterates `RuntimeDeps` ∪ the
auto-included slice.

**Acceptance:** scan correctly identifies the under-declared SONAMES
empirically observed in round 2 (`git` → zlib; `wget` → zlib +
libuuid; `coreutils` → acl + attr). Auto-include closes the install
on minimal containers where today's recipes break. Known-gap
allowlist suppresses noise for SONAMES with no current tsuku recipe.

### Phase 6 — Migrate the punted recipes

Update affected recipes to drop their `supported_os = ["linux"]` /
`unsupported_platforms` restrictions. No recipe-side schema changes
required; the engine handles chaining automatically based on existing
`runtime_dependencies`. Suggested order:

1. `tmux` (closes #2377; demonstrates the integrated mechanism end-to-end)
2. The 13 recipes that punted on darwin (`bedtools`, `cbonsai`,
   `cdogs-sdl`, etc.)
3. The next wave of top-100 tools that needed this to be curated
   (`bat`, `eza`, `htop`, `delta`, etc.)

Each migration is a small recipe-only PR. Per-recipe acceptance: install
+ verify pass on the test-recipe matrix on every supported platform.

### Out of scope (follow-up work)

Phase 5 will surface SONAMES with no current tsuku library recipe
(`libuuid`, `libacl`, `libattr` are the empirically observed
candidates from round 2). Authoring those library recipes is
downstream work — one small PR per recipe, following the existing
library-recipe pattern. The Phase 2 known-gap allowlist suppresses
log noise for these entries until the recipes land. Out of scope for
this design.

## Security Considerations

The chain mechanism extends the trust boundary that already governs
`homebrew` bottles: bottle contents come from `homebrew/core` GHCR with
unchanged sha256 verification, and patches are applied only inside
`$TSUKU_HOME/{tools,libs}/<recipe>-<version>/`. The dep-set entries that
drive the chain are recipe-controlled (via `runtime_dependencies`), so a
recipe is the trust boundary for the chain content. The new SONAME
completeness scanner reads only the bottle's own binaries (already
trusted via sha256) and the SONAME index built from already-trusted
library recipes.

### Threat model

A malicious or buggy recipe in a third-party tap could declare an entry
that, after string interpolation into a filesystem path, escapes
`$TSUKU_HOME/libs/`. For example,
`runtime_dependencies = ["../../etc"]` would, with `filepath.Join`'s
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

The SONAME scanner reads NEEDED entries from the bottle binary's ELF /
Mach-O headers — both well-defined formats parsed by trusted system
tools (`readelf`, `otool`). A malicious bottle could declare bogus
NEEDED SONAMES, but those are then matched against the SONAME index
built from tsuku's own library recipes; an unknown SONAME results in
a "coverage gap" log line, never a path expansion. Auto-include only
fires when the SONAME maps to a known tsuku library, so the recipe
trust boundary still gates the chain.

### Mitigations

1. **Strict validation of `runtime_dependencies` entries at recipe load
   time.** Each entry must match `^[a-z0-9._-]+$` and reject `/`, `\`,
   `..`, leading `-`, and null bytes. This matches existing recipe-name
   validation in the registry cache and index-rebuild paths and is
   promoted to a shared helper applied uniformly. Phase 1 of the
   implementation includes this validator change.

2. **Defense-in-depth at patch time.** Both `fixDylibRpathChain`
   (macOS) and `fixElfRpathChain` (Linux) compute the relative RPATH
   via `filepath.Rel` after `EvalSymlinks`, then verify
   `filepath.Join(loaderDir, relPath)` resolves back into
   `$TSUKU_HOME/libs/`. A failed check fails the install with a clear
   error before any `patchelf` / `install_name_tool` invocation for
   that entry. The `work_dir` is disposable, so partial patching that
   may have occurred on prior entries is discarded with the rest of
   the unpacked bottle on failure — `$TSUKU_HOME/{tools,libs}/` is
   never touched until the relocate phase completes successfully.
   This is included in Phases 3 and 4.

3. **SONAME-index input validation.** The
   `internal/sonameindex/sonameindex.go` parser requires each
   library-recipe `outputs` entry that contributes to the index to
   start with `lib/`, contain no `..` segments, and have a basename
   that starts with `lib`. Other entries are skipped (they aren't
   SONAMES). On a duplicate `(platform, SONAME)` insert, index
   construction fails at plan-generation time with a clear error
   pointing at both providing recipes — collisions are loud, not
   silent. This is included in Phase 2.

4. **SONAME content never reaches `filepath.Join`.** The SONAME scanner
   reads NEEDED entries from bottle-binary headers and uses the SONAME
   string only as a Go map key. Path construction for chained RPATH
   entries always uses the **provider's** known recipe name and version
   (resolved through the validated index), never the SONAME. This
   isolates the bottle-binary trust boundary from path construction and
   is load-bearing for the threat model — implementers must preserve it
   even when adding diagnostic messages or future error formatting.

5. **No user-specific data is embedded in patched binaries.** The
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
  `shellcheck`, `hadolint`) work cleanly in the tsuku layout. Recipe
  authors don't need to learn anything new — the existing
  `runtime_dependencies` field they already use gains the chain.
- **Removes the per-recipe N×M coordination cost.** Today, getting a
  tool to chain a new dep means hand-coordinating with the dep recipe
  to publish the right dylib (the `wget` ↔ `gettext` `libintl.8.dylib`
  patch is a representative example). The chain consumes whatever the
  dep recipe's standard library install ships.
- **Catches under-declared recipes.** The SONAME scan surfaces deps
  the recipe author forgot — empirically observed for `git` (zlib),
  `wget` (zlib + libuuid), `coreutils` (acl + attr). These bugs are
  invisible in CI today because the test containers ship those libs
  system-wide; the scan exposes them.
- **Aligns macOS and Linux semantics.** Recipe authors declare one
  existing field; the engine handles the platform-specific RPATH
  plumbing. Linux ELF gains a chaining function that didn't exist at
  all before.
- **Restores `$TSUKU_HOME`-portability.** Binaries patched with the
  new mechanism use `$ORIGIN` / `@loader_path`-relative RPATHs and
  empirically survive both `mv` and symlink relocation. The existing
  recipe-side `set_rpath` action bakes absolute paths
  (`{libs_dir}/...`) into binaries; the new mechanism is a portability
  improvement.
- **Zero new schema fields.** Recipe authors learn nothing new. The
  decision-table burden goes away — there's only one dep-set field
  for the runtime-availability semantic.

### Negative

- **`runtime_dependencies` now drives two engine consumers.** Today
  it drives wrapper-PATH; the design adds RPATH chaining. Empirical
  evidence (round-2 stress tests) shows the second consumer is
  false-positive-safe (declared deps are never over-declared in the
  measured sample), but a maintainer reading the engine code now needs
  to know two paths consume the same field. Mitigation: docstring on
  the field explicitly enumerates the consumers.
- **Generalizing `fixLibraryDylibRpaths` is a behavior change for the
  function's existing callers.** Library recipes (`pcre2`,
  `libnghttp3`, etc.) currently get absolute-path RPATHs via the
  existing function; after the rename they get relative-path RPATHs.
  The two paths produce different binaries for the same recipe state.
  This is a portability improvement, not a regression — but tests must
  lock both shapes during the migration window.
- **Discoverability of relative-RPATH semantics.** A binary's RPATH is
  not human-friendly to inspect. `tsuku doctor` could grow a check
  that walks installed binaries and validates their RPATH chain
  resolves cleanly — out of scope for this design but a natural
  follow-up.
- **The SONAME scanner adds install-time work.** `readelf -d` /
  `otool -L` on each binary in the bottle — usually a handful of
  binaries plus shared libraries — adds a few hundred milliseconds per
  install. Negligible for interactive installs, possibly meaningful in
  large CI matrices. Mitigation: results are deterministic per bottle
  sha256; cacheable as part of plan caching.
- **The SONAME index needs maintenance.** Adding a new library recipe
  (or changing an existing library's SONAME outputs) requires the
  index to rebuild. The index lives in plan-generation, so it rebuilds
  on every install — but recipe-validation tests should also exercise
  the index for known SONAMES to catch breakage early.
- **Coverage gaps surface as follow-up work.** The scanner will flag
  SONAMES (`libuuid`, `libacl`, `libattr`, etc.) with no tsuku library
  recipe today. The Phase 2 known-gap allowlist downgrades these to
  debug-level logs so install output stays clean; entries are removed
  from the allowlist as the corresponding recipes land. Closing each
  gap is a separate library-recipe PR — out of scope here.

### Mitigations

- Update the recipe-author guide to document the runtime-availability
  semantic of `runtime_dependencies` (already implicit; just made
  explicit), with a note that the SONAME scanner will warn on
  under-declared recipes so authors get fast feedback.
- The existing wget/git per-step `dependencies` patterns continue to
  work — the engine reads both `runtime_dependencies` and per-step
  `dependencies` for the chain, so no recipe migration is forced.
  Recipe authors can drop the per-step list when convenient.

