# Exploration Decisions: tsuku-homebrew-dylib-chaining

## Round 1

- **Recipe-side workarounds are not the sole fix**: blast radius >15 (top-100 strict: 10; counting workaround-dependent existing recipes: 19+; counting macOS punts: 26+) crosses the stop-signal threshold from the scope doc.
- **Tsuku-core is the right level for the fix**: recipe authors did not converge on a stable workaround pattern (1 recipe uses `set_rpath` chain; the canonical case `curl.toml` defers to "until runtime_dependencies supports dylib chaining" as a tsuku-core deliverable).
- **Option 2 (strengthen existing homebrew action) is the leading candidate** for the solution shape, with Options 1 (new composite action) and 3 (new chain_deps_into_rpath action) as alternatives the produced artifact should weigh.

## Round 2

- **Reuse `runtime_dependencies` instead of introducing `chained_lib_dependencies`.** Empirical: bogus RPATH entries are harmless, augmentation is safe from false positives, and a new field violated the design's own "authors should not need to know about RPATH semantics" constraint.
- **Add a SONAME-driven completeness scan** as a separate design dimension. Recipes are systematically under-declared (git misses zlib, wget misses zlib + libuuid, coreutils misses acl + attr). Test containers shadow the gaps with system libs. Declaration-only augmentation leaves the structural bug in place.
- **Use `DT_RPATH` (`patchelf --force-rpath --set-rpath`), not `DT_RUNPATH`.** Empirical: `DT_RUNPATH` produced unexplained `libunistring.so.5 => not found` lookups for wget; `DT_RPATH` worked.
- **Don't filter by dep `Type`.** Empirical: nonsense entries (e.g. tool deps' `libs/<tool>-<v>/lib` paths that don't exist) are inert at runtime, costing ~10 stat() syscalls. The 9 mixed-recipe deps that are tool-typed but ship libraries (xz, ncurses, sdl2, perl, sqlite, fontconfig, glew, luajit, apr) get correct chaining without engine-side type filtering.
- **`$ORIGIN`/`@loader_path`-relative paths confirmed portable** under both `mv` and symlinked layouts. Strict win over the existing `set_rpath` absolute-path template.
