# Findings: tsuku-homebrew-dylib-chaining

## Round 1 (recap)

The blast radius is **>15 affected recipes** by every accounting (top-100 strict: 10; counting workaround-dependent existing recipes: 19+; counting macOS punts: 26+). Recipe authors did not converge on any reusable workaround pattern (across 1168 homebrew-using recipes, exactly 0 use the existing `set_rpath` action to chain deps). The fix lives at tsuku-core.

## Round 2 (empirical) — reusing `runtime_dependencies`

Each lead ran in a docker container (no host writes). Findings reproducible from the per-lead artifacts in `wip/research/explore_tsuku-homebrew-dylib-chaining_r2_lead-*.md`.

### Lead 7 — Classify deps in `runtime_dependencies`

- 298 recipes declare a non-empty `runtime_dependencies`.
- By dep classification: **219 all-library**, **41 all-tool**, **38 mixed**, **0 with missing deps**.
- 9 deps in the mixed category are `Type` = "tool" (recipe lacks `type = "library"`) but actually ship libraries that consumers link against: `xz`, `ncurses`, `sdl2`, `perl`, `sqlite`, `fontconfig`, `glew`, `luajit`, `apr`. A naive "filter by `Type == "library"` only" would silently miss these.

### Lead 8 — Stress test all-library deps

- `git` (deps: pcre2): works after RPATH includes `$TSUKU_HOME/libs/pcre2-*/lib`. Without RPATH chain, fails with `libpcre2-8.so.0: cannot open shared object file`.
- `wget` (deps: openssl, gettext, libidn2, libunistring): works after RPATH includes all four dep lib dirs. **Critical detail**: must use `patchelf --force-rpath --set-rpath` (writes `DT_RPATH`), not `--add-rpath` (writes `DT_RUNPATH`, which has subtle resolution differences and produced `libunistring.so.5 => not found`).
- `calcurse` (declares only `gettext`, but binary needs ncurses too): **breaks** — under-declared dep list. No RPATH change rescues an incomplete declaration.

### Lead 9 — Mixed-type deps; do nonsense RPATH entries harm?

- Tested `libzip` with `xz` (tool dep — has no libs/, lives under tools/) and `zstd` (library dep). Pointed RPATH at `/tmp/tsuku/libs/xz-5.8.3/lib` (path that doesn't exist).
- glibc's loader silently `stat()`s the missing path, gets ENOENT, moves on. Exit 0.
- Stress-tested with 50 garbage RPATH entries (1987 bytes). Still works.
- **The engine does NOT need to filter by `Type == "library"`.** Naively constructing `$LibsDir/<dep>-<v>/lib` for every dep entry is functionally equivalent to filtering — wrong entries are inert, costing only a few extra `stat` syscalls at process startup.

### Lead 10 — RPATH entries to empty/missing dirs

- Tested empty dir, non-existent dir, regular file (instead of dir), EACCES dir, setuid binary, 1000-entry RPATH (23 KB), double colons, trailing slashes.
- All variants: exit 0, zero stderr, no warnings, no `errno` propagation, no `LD_DEBUG=libs` error markers.
- Cost: ~10 `stat()` syscalls per missing-lib lookup. 100 bogus entries → ~5.7 ms per process spawn — invisible for CLI tools.
- **Empty/missing/bogus RPATH entries are entirely harmless on Linux.**

### Lead 11 — Declaration completeness (the surprise)

This is the round's most important finding. Recipes are systematically **under-declared**.

- `git`: declared `pcre2`. Binary's `readelf -d` NEEDED list also references `libz.so.1` (zlib). zlib HAS a tsuku recipe; recipe doesn't list it.
- `wget`: declared `openssl, gettext, libidn2, libunistring`. Binary also references `libz.so.1` and `libuuid.so.1`. Plus a SONAME version mismatch: bottle was built against `libunistring.so.5`, declared dep ships a different major.
- `coreutils`: declared deps don't cover `libacl.so.1` and `libattr.so.1`. The source libraries (`acl`, `attr`) don't have tsuku recipes at all.

The under-declarations stay invisible because the debian/ubuntu test container resolves NEEDED entries to `/lib/x86_64-linux-gnu/...` from the system. Tsuku's installed deps under `$TSUKU_HOME/libs/*/lib/` are never consulted by the loader — the system shadows them.

**On a minimal container without those system libs, every one of these recipes would fail.** This explains why tmux/git/wget appear to "work" in CI today — debian/ubuntu/etc. ship the missing libs system-wide, masking the bug. The bug is structural; CI doesn't catch it because the test images are too forgiving.

### Lead 12 — Auto-generated `runtime_dependencies`

- 316 of 323 (~97.8%) recipes that declare the field were auto-generated. Source: a verbatim copy of homebrew formula JSON's `dependencies` field. No readelf/SONAME inspection.
- In 7 measurable bottle samples: declared deps are **never over-declared** (each declared dep maps to a real NEEDED SONAME).
- But routinely **under-declared**: missing brew-treats-as-system libs (`libstdc++`, `libgcc_s`, `libz`) and transitive deps.
- **Augmentation is safe from a false-positive standpoint.** Using these values to populate RPATH would not chain libs the binary doesn't need.
- **But augmentation alone is insufficient.** The under-declaration is systematic, not random.

### Lead 13 — `$TSUKU_HOME` portability

- `$ORIGIN`-relative RPATHs (e.g. `$ORIGIN/../../../libs/<dep>-<v>/lib`) survive `mv /tmp/tsuku-A /tmp/tsuku-B`.
- Symlink-portability: `ln -sf /tmp/tsuku-B /tmp/tsuku-A` and born-symlinked layouts both work. `$ORIGIN` honors the symlink-traversed path.
- Chained dylibs (lib → lib → lib) relocate cleanly when each link carries its own `$ORIGIN`-relative RPATH. `DT_RUNPATH` is sufficient (patchelf 0.14's default).
- Absolute-path baseline breaks after move: `libdep.so.1: cannot open shared object file`.
- **Design's portability claim validated.** Strict win over the existing `set_rpath` absolute-path template.

## Round 2 conclusions

The empirical evidence is conclusive on three points:

1. **Reusing `runtime_dependencies` is safe and sufficient *for the deps that are declared*.** No false-positive risk. No need to filter by `Type`. Bogus entries are inert. The new field (`chained_lib_dependencies`) is unnecessary complexity.

2. **But declaration alone is not enough.** Round 2 surfaced a deeper, previously-invisible structural bug: recipes' declared deps DO NOT cover everything the binary's NEEDED list requires. The gaps are masked today by the test environment's permissive system-lib resolution. The design must include a **SONAME-driven completeness check** (`readelf -d` / `otool -L` on the bottle binary at install time, cross-referenced against tsuku-installable libraries) — or auto-discovery from the binary itself, not just the declared deps.

3. **Portability via `$ORIGIN` / `@loader_path`-relative paths is empirically validated**, including under symlinked install dirs. `DT_RPATH` should be used (via `patchelf --force-rpath --set-rpath`), not `DT_RUNPATH`, to avoid subtle resolution differences.

## What the design must change

- **D2 (recipe declaration)**: drop the new `chained_lib_dependencies` field. Reuse `runtime_dependencies`.
- **Add a new design dimension**: **SONAME-driven completeness scan**. At install time after bottle extraction, run `readelf -d` (Linux) or `otool -L` (macOS) on the binary, identify NEEDED SONAMES, classify each:
  - System library (in container's standard ldconfig path) → no action
  - SONAME provided by a tsuku-installable library → if not in declared deps, warn (recipe is under-declared); auto-include the lib's `lib/` dir in the RPATH chain
  - SONAME with no tsuku recipe → log as a coverage gap; the recipe's bottle path is fragile on minimal containers
- **D3 (path form)**: confirmed `$ORIGIN`/`@loader_path`-relative paths into the known layout. Use `DT_RPATH` not `DT_RUNPATH`.
- **D1 (where the fix lives)**: unchanged — strengthen the existing homebrew action.

## Decisions made during round 2

- **Reuse `runtime_dependencies` instead of introducing `chained_lib_dependencies`.** Rationale: empirical evidence shows the augmentation is safe (no false positives), and the new field violated the design's own "authors should not need to know about RPATH semantics" constraint.
- **Add SONAME completeness scanning** as a separate design dimension. Rationale: empirical evidence shows declarations are systematically under-declared and the test environment is too permissive to catch it. Without this, augmenting `runtime_dependencies` alone leaves the structural bug in place.
- **Use `DT_RPATH` (`patchelf --force-rpath --set-rpath`).** Rationale: empirical evidence shows `DT_RUNPATH` has subtle resolution differences that broke wget's libunistring lookup.
