# Explore Scope: tsuku-homebrew-dylib-chaining

## Visibility

Public

## Core Question

**How big is this problem?** Specifically: how many curated and would-be-curated tsuku recipes are blocked because the homebrew action does not chain dylibs from sibling tsuku-installed library deps into a tool recipe's RPATH (`fixLibraryDylibRpaths` is gated on `Type == "library"`)? The answer determines whether the right fix is a recipe-side workaround (set_rpath chains, source builds) or a new/updated primitive in tsuku that recipe authors can reuse.

## What's in scope

- Quantifying the blast radius across the recipe registry today (curated and uncurated).
- Identifying which classes of tools are blocked by this gap (e.g., terminal tools that link libutf8proc, GUI/curses tools, anything depending on libidn2/gettext/libunistring outside system paths).
- Understanding how existing recipes work around or sidestep the issue (curl source-builds; wget relies on system libs; pcre2 ships dylibs because it's `Type == "library"`).
- Capturing what the analogous mechanism does for `Type == "library"` recipes today (`fixLibraryDylibRpaths` walks `ctx.Dependencies.InstallTime` and adds dep `lib/` directories as RPATHs).
- Both Linux ELF (patchelf) and macOS Mach-O (install_name_tool / @rpath) paths — they're different mechanisms but the same conceptual gap.

## What's out of scope (for this exploration)

- Picking a specific solution. That's the artifact this exploration produces (PRD, design, or recipe issue).
- The pkg-config arm64 bottle gap (#2374) — different shape, already addressed by #2375.
- The `findMake` `gmake` fallback — already a separate recipe-side fix in flight.
- Refactoring the entire homebrew action.

## Stakes

- If the blast radius is small (a handful of recipes), recipe-side workarounds are appropriate and the architectural gap is a documented limitation.
- If the blast radius is large (a whole class of common-tool homebrew bottles), every new author hits this gap and is blocked or has to invent the same workaround. The right answer is a primitive.
- Either way, the answer changes how #2377 ships and what the recipe-author guide says about runtime_dependencies on tool recipes.

## Research Leads

1. **Survey the curated recipe registry for tool recipes that use the `homebrew` action AND declare `runtime_dependencies` (or per-step `dependencies`).** These are the recipes that have ALREADY been hand-coded to acknowledge the gap. How many are there, and which ones actually work in CI today? If many "work," investigate why — they may rely on system-available libs (debian's libssl, libidn2, etc.) that mask the gap on Linux but would fail on minimal containers or distros without those libs.

2. **Survey the curated recipe registry for `supported_os = ["linux"]` or `unsupported_platforms` entries that mention dylib/bottle reasons in comments.** These are recipes that SCOPED DOWN to avoid the gap. curl is the known example. How many other recipes punted on macOS or Linux because of this? List them.

3. **Survey the top-100 priority list (`#2260`) and identify how many entries are tools whose homebrew bottle has known transitive dep chains beyond what's available as system packages.** This is the "would-be-curated" count. Use `homebrew-core/Formula/<name>.rb` source (via the homebrew API) to enumerate dependencies for the top-100 tool formulas. Tools that depend on `libevent`, `utf8proc`, `libidn2`, `libunistring`, `gettext`, `pcre2`, `nghttp2`, `libgit2`, `libssh2`, `oniguruma`, etc. that aren't system packages are the at-risk set.

4. **Investigate how `fixLibraryDylibRpaths` works for `Type == "library"` recipes** — specifically, what changes if the `Type == "library"` gate is lifted for tool recipes. What's the actual code path (`internal/actions/homebrew_relocate.go:103`, `:607-614`)? What would the behavior change be for existing tool recipes that don't currently declare runtime_dependencies (probably nothing — the walk is a no-op when the deps list is empty)? What test surface needs updating?

5. **Catalog the recipe-side workarounds that already exist in the registry** (set_rpath chains in curl, mode=output verify in openjdk, source-build paths in pcre2, system-package fallbacks where they exist). Are these patterns convergent (the same shape repeated 3+ times) or divergent (each recipe invents its own approach)? Convergent + frequent → strong signal that the right level for the fix is tsuku-core.

6. **Enumerate Homebrew bottle install patterns across the recipe registry** — group by: `homebrew + install_binaries` (the basic shape), `homebrew + install_binaries + set_rpath` (curl pattern), `homebrew + install_binaries with explicit lib outputs` (pcre2 pattern). The distribution tells us which patterns are reused enough to deserve a primitive.

## Coverage Notes

The first three leads quantify the blast radius. Leads 4-6 inform the design of a fix if the blast radius warrants tsuku-core changes. The exploration should NOT pick a solution — its output is "the problem affects N recipes; here's the shape of the solution space."

## Stop signal

Stop the exploration when leads 1-3 produce a defensible blast radius number with examples. If that number is < 5 recipes, route to a recipe-author follow-up issue. If > 15, route to a tsuku-core PRD or design doc. In between, capture the trade-offs and let the user decide.

---

## Round 2 — Reusing `runtime_dependencies` (empirical stress test)

The design doc currently proposes a new `chained_lib_dependencies` field. Reviewer challenge: dispatch on the dep's `Type` instead. If the dep is a library, chain its lib dir; if the dep is a tool, the existing wrapper-PATH path handles it. No new field needed.

This round's job is to find what actually breaks under that simpler model. Empirical only — agents run experiments inside docker containers, never touch the host's `$TSUKU_HOME`. The findings feed back into the same design doc.

### Constraints for round 2 agents

- All experiments run inside docker containers. No writes to the user's `$TSUKU_HOME` or anywhere outside the agent's working directory or temp dirs created inside containers.
- Build a tsuku-test binary on the host (`make build-test`) and mount it read-only into the containers.
- Each container starts from a clean `tsuku/sandbox-cache:debian-...` or `fedora` image to mirror the test-recipe matrix.
- Each agent reports concrete reproductions (commands + output) for any breakage it finds.

### Round 2 Research Leads

7. **Survey existing recipes that already declare `runtime_dependencies` and classify each dep.** For all 323 recipes that use the field, classify each dep: is it a `Type == "library"` recipe, a `Type == "tool"` recipe, or absent from the registry? Are there recipes mixing both types in one list? If we add automatic RPATH chaining for library deps, this is the universe of recipes whose behavior changes — even if the change is mostly "RPATH gets a new entry that wasn't loaded before, but is harmless because the binary doesn't reference the SONAME."

8. **Stress test 1 — recipes with all-library `runtime_dependencies`.** Pick 3 candidates (likely git, wget, plus one less-curated). For each, on a `debian:bookworm-slim` container, install via tsuku, manually patch the binary's RPATH to include the dep lib dirs (simulating the augmented behavior), then run the verify command. Does it work? Does it break? Capture exact output.

9. **Stress test 2 — recipes with mixed `runtime_dependencies` (tools + libraries).** Find recipes where runtime_dependencies includes a tool and a library mixed. Check what happens if the augmented engine adds RPATH entries for tools (which don't have a lib/ dir, so the entry is non-existent). Does the binary's load fail, succeed-with-warning, or succeed-silently? Test in docker.

10. **Stress test 3 — recipes with `runtime_dependencies` deps that don't actually publish dylibs.** Some library recipes might publish only static archives or no lib/ outputs. If the augmented engine adds an RPATH entry to a non-existent or empty lib/ dir, what happens at runtime?

11. **Stress test 4 — recipes whose binary references SONAMES NOT in `runtime_dependencies`.** Check whether augmenting `runtime_dependencies` is *sufficient*: are there cases where the bottle binary needs a lib that isn't declared in `runtime_dependencies`? If so, the augmentation alone isn't enough — there's a distinct gap (declaration completeness) that the design has to address separately.

12. **Stress test 5 — auto-generated recipes.** The earlier exploration noted ~323 recipes use `runtime_dependencies`, "mostly auto-generated by the homebrew builder for wrapper-script PATH purposes." Find these. Are the auto-generated values *correct* per the bottle's actual SONAMES, or are they coarse approximations? If coarse, augmenting would be a false-positive amplifier (chains in libs that the binary doesn't need). Quantify.

13. **Stress test 6 — moving `$TSUKU_HOME` after install.** With a tsuku-installed tool that has its RPATH chained via the augmented mechanism (using `$ORIGIN` / `@loader_path`-relative paths), copy the entire `$TSUKU_HOME` to a different prefix inside the container and re-run the binary. Does it still work? This validates the design's portability claim.

### Round 2 stop signal

Each lead produces evidence (commands + output) supporting one of:
- "Augmenting `runtime_dependencies` is safe — no concrete breakage observed."
- "Augmenting causes problem X — the design must add Y to handle it."
- "Augmenting is insufficient — there's a separate gap Z that the design must address."

Write findings into a round-2 file. After all 6 leads return, update the design doc with the empirically-validated revised proposal (dispatch on dep type; no new field) — OR retain the new-field design if the leads surface a real blocker.
