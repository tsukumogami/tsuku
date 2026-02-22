# Architecture Review: Deterministic Library Recipe Generation

**Design:** `docs/designs/DESIGN-library-recipe-generation.md`
**Reviewer:** architect-reviewer
**Date:** 2026-02-22

---

## 1. Is the architecture clear enough to implement?

**Mostly yes, with three gaps.**

The function signatures, data structures, and control flow are well-specified. An implementer can follow the design from `extractBottleContents` through `generateLibraryRecipe` to `classifyDeterministicFailure` without ambiguity. The phasing is logical.

However, three implementation-critical details are either wrong or missing:

### Gap 1: Two serialization paths, only one addressed

The design identifies that `ToTOML()` doesn't emit `metadata.type` and says it "must be updated." But the actual recipe-writing path in the create command uses `recipe.WriteRecipe()` (`/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/recipe/writer.go:45`), which calls `toEncodable()` and delegates to BurntSushi's TOML encoder. Because `MetadataSection.Type` has a `toml:"type"` struct tag, **`WriteRecipe` already handles `type = "library"` correctly.**

`ToTOML()` is a separate hand-coded serializer used only by the sandbox validation executor (`/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/validate/executor.go:211`). The design should clarify which path needs the fix. If sandbox validation needs to work with library recipes (likely yes for the validation/repair flow), `ToTOML()` still needs updating -- but it's a sandbox concern, not a recipe-writing concern.

**Additionally, `ToTOML()` is missing more than just `type`.** It doesn't emit `dependencies`, `runtime_dependencies`, `satisfies`, `tier`, or any other `MetadataSection` field beyond `name`, `description`, `homepage`, and `version_format`. If sandbox validation runs against generated library recipes (which set `Metadata.Type` and `Metadata.Dependencies`), the serialized TOML will be incomplete. The design should scope the `ToTOML()` fix to include at least `type` and `dependencies`.

### Gap 2: Existing library recipes have no [version] section

All 22 existing library recipes omit the `[version]` section entirely. The design inherits `generateDeterministicRecipe`'s pattern of setting:

```go
Version: recipe.VersionSection{
    Source:  "homebrew",
    Formula: info.Name,
},
```

This produces a `[version]` section in generated library recipes that no existing library recipe has. The recipes are structurally valid (the schema allows it), but the design promises "generated and hand-written recipes look the same." This is a mismatch.

The decision on whether to include `[version]` is a product choice, not a structural error. But the design should acknowledge it explicitly rather than silently diverging.

### Gap 3: Empty [verify] section in generated output

The design says library recipes omit `[verify]`. But `WriteRecipe` uses BurntSushi encoding of the full `recipeForEncoding` struct, which always includes `Verify VerifySection`. Since `VerifySection` has no `omitempty` tag, an empty `[verify]` section will be emitted:

```toml
[verify]
```

Most existing library recipes don't have this. Some (like `readline.toml`) have a verify with `command = "ls lib/libreadline.a"`. The empty section is harmless but doesn't match the "same structure" goal. The implementer needs to either strip the empty verify post-serialization or ensure `WriteRecipe` skips it.

---

## 2. Are there missing components or interfaces?

### Missing: `platformContents.Libc` handling for macOS

The proposed `platformContents` struct has:
```go
type platformContents struct {
    OS       string        // "linux" or "darwin"
    Libc     string        // "glibc" or "" (macOS has no libc distinction)
    Contents *bottleContents
}
```

But existing library recipes with `when` clauses use different patterns:
- Linux glibc: `when = { os = ["linux"], libc = ["glibc"] }`
- macOS: `when = { os = ["darwin"] }`

The `Libc` field on `platformContents` maps to the `libc` field in the `WhenClause`. For macOS, the `when` clause should NOT include `libc` at all (not even `libc = []`). The design says `Libc` is `""` for macOS, but the implementer needs to know that an empty string means "omit from when clause" rather than "set to empty."

This is an interface precision issue, not a missing component. But the mapping from `platformContents` to `WhenClause` construction should be explicit.

### Missing: Decision on which pattern to follow

The 22 existing library recipes split into two distinct patterns:

**Pattern A -- No `when` clauses (13 recipes):** brotli, zstd, libcurl, libnghttp2, readline, expat, libpng, libssh2, libnghttp3, libngtcp2, libxml2, proj, geos. These mix `.so` and `.dylib` in a single `outputs` list without platform-conditional steps.

**Pattern B -- Platform-conditional `when` clauses (9 recipes):** gmp, abseil, gettext, giflib, jpeg-turbo, cairo, fontconfig, apr, pcre2. These have separate step pairs per platform with `when` clauses.

The design proposes generating Pattern B (multi-platform with `when` clauses). This is the right call for correctness -- a single outputs list with both `.so` and `.dylib` means the installer tries to symlink `.dylib` files on Linux (they won't exist in the bottle). The Pattern A recipes work because `install_binaries` in directory mode skips missing outputs silently (it creates symlinks for files that exist and ignores others).

But the design should state this decision explicitly: "Generated recipes use Pattern B (separate per-platform steps) because Pattern A relies on silent-skip behavior that is fragile and may break if install_binaries gains strict mode." Or acknowledge that Pattern A is acceptable and simpler. Right now, the design implicitly picks Pattern B without discussing Pattern A's existence.

### Present and correct: `isLibraryFile` filter

The extension filter function is well-specified. The `strings.Contains(name, ".so.")` check for versioned shared objects is the right approach -- it catches `libgc.so.1.5.0` without matching unrelated files.

### Present and correct: Error classification

The subcategory approach via `knownSubcategories` map entry is structurally clean. No schema migration, no new enum constant. The bracket-tag pattern (`[library_only]`) in error messages integrates with the existing `extractSubcategory` Level 1 parsing in `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/dashboard/failures.go:59`.

---

## 3. Are the implementation phases correctly sequenced?

**Yes, with one dependency inversion to fix.**

### Phase 1 (Bottle content scanning) -- Correct

Refactoring `extractBottleBinaries` to `extractBottleContents` is a clean rename-and-extend. The backward-compatible `listBottleBinaries` wrapper maintains the existing tool recipe path. No structural risk.

### Phase 2 (Library recipe generation) -- Dependency issue

The design says Phase 2 includes "Update `ToTOML()` to emit `metadata.type` when set." As discussed in Gap 1, the `WriteRecipe` path already handles `type`. The `ToTOML()` fix matters for sandbox validation, which is needed if the repair flow runs on library recipes.

However, the design also says Phase 2 should "Test with real library bottles (bdw-gc, tree-sitter)." This implies sandbox testing, which needs the `ToTOML()` fix to be correct. So the fix is correctly placed in Phase 2, but the rationale should be "for sandbox validation" rather than "for recipe writing."

Phase 2 also bundles the `library_only` subcategory addition. Since this is a one-line map entry in `knownSubcategories`, bundling it here is fine -- no sequencing dependency.

### Phase 3 (Multi-platform support) -- Correct

Multi-platform depends on single-platform working first. The GHCR cross-platform download is architecturally sound -- the existing `downloadBottleBlob` / `fetchGHCRManifest` / `getBlobSHAFromManifest` chain already handles arbitrary platform tags. No new HTTP client or auth path needed.

### Phase 4 (Pipeline integration) -- Correct

Running against existing `complex_archive` failures is the right validation step. No structural concerns.

**Recommendation:** Phase 2 and Phase 3 could be merged if the implementer is comfortable. Single-platform library generation without multi-platform support produces recipes that match Pattern A (mix both extensions). If the goal is Pattern B from day one, Phase 3 is needed before the generated recipes are shipped.

---

## 4. Are there simpler alternatives?

### Alternative 1: Generate Pattern A (no `when` clauses)

Instead of downloading two bottles and generating platform-conditional steps, scan the current platform's bottle and list all files. The resulting recipe has a single `homebrew` + `install_binaries` pair with mixed `.so` and `.dylib` entries. This matches Pattern A (13 of 22 existing library recipes).

**Advantage:** No multi-platform bottle download. Simpler code. Phases 2 and 3 merge into one.

**Disadvantage:** The outputs list includes files from only one platform. Linux bottles have `.so` but not `.dylib`; macOS bottles have `.dylib` but not `.so`. The generator would need to synthetically add the missing platform's extensions (e.g., for each `libfoo.so`, add `libfoo.dylib`), which is heuristic and fragile.

**Verdict:** Not simpler in practice. The multi-platform scan is cleaner than guessing cross-platform file names.

### Alternative 2: Skip tarball scan entirely, use formula API metadata

Some Homebrew formulas list their installed files in the API. If the formula metadata included `lib/` contents, we could skip the bottle download.

**Verdict:** The formula API doesn't reliably list file contents. The bottle download is already in the code path. Not viable.

### Alternative 3: Don't generate library recipes; just improve classification

Instead of generating recipes, add `library_only` subcategory detection and leave recipe authoring as manual. This reduces the change to a small classification improvement.

**Verdict:** Viable if the ROI of automating 20-40 recipes isn't worth the implementation cost. But the mechanical nature of library recipes makes automation high-value per line of code.

---

## Structural Findings

### Finding 1: `ToTOML()` metadata gap is broader than documented -- Advisory

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/recipe/types.go:52`

`ToTOML()` emits only `name`, `description`, `homepage`, and `version_format` from `MetadataSection`. It skips `type`, `dependencies`, `runtime_dependencies`, `tier`, `satisfies`, and all platform constraint fields. The design only calls out `type`, but sandbox validation of library recipes also needs `dependencies` to be emitted.

This is pre-existing technical debt, not introduced by this design. The design should scope the fix to `type` and `dependencies` at minimum.

**Advisory** -- the gap exists today and doesn't compound from this design. But fixing only `type` while leaving `dependencies` broken would create confusion during Phase 2 sandbox testing.

### Finding 2: Two library recipe patterns undocumented -- Advisory

**Files:** `recipes/b/brotli.toml` (Pattern A, no when clauses), `recipes/g/gmp.toml` (Pattern B, with when clauses)

The design references "all 22 existing library recipes" as following one pattern, but there are two distinct patterns. 13 recipes use a single unconditional step pair; 9 use platform-conditional step pairs. The design generates Pattern B, which is correct for new recipes, but should document why Pattern A exists and whether it should be migrated.

**Advisory** -- not a structural violation. The generated recipes will be correct regardless. But the design rationale is stronger if it acknowledges both patterns.

### Finding 3: No architectural violations in the proposed changes

The design modifies code within `internal/builders/homebrew.go` (the Homebrew builder) and adds a map entry in `internal/dashboard/failures.go`. All changes stay within their architectural layer:

- No action dispatch bypass (library recipes use `homebrew` + `install_binaries` actions via the recipe struct, not direct calls)
- No provider bypass (version resolution stays in the `homebrew` provider)
- No state contract changes (no new state fields)
- No CLI surface changes (the `create --from homebrew --deterministic-only` flag already exists)
- No dependency direction violations (builders depend on recipe types, not the reverse)
- No parallel patterns (the library recipe path is a new branch in the existing deterministic generation flow, not a parallel flow)

The design fits the architecture.

### Finding 4: `RuntimeDependencies` vs `Dependencies` field mismatch -- Advisory

**File:** `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/builders/homebrew.go:1905`

The existing `generateDeterministicRecipe` sets `Metadata.RuntimeDependencies` from `info.Dependencies`. But existing library recipes (gmp.toml, brotli.toml, etc.) use `dependencies` (install-time), not `runtime_dependencies`. Homebrew's formula JSON `dependencies` field represents runtime dependencies, so the existing code's use of `RuntimeDependencies` is semantically correct. But the generated output won't match hand-written library recipes.

The design should specify which field to use for library recipes and whether to match the hand-written convention or the semantic meaning.

**Advisory** -- the field choice affects recipe correctness but doesn't violate architecture.

---

## Summary

The design is architecturally sound. It extends the existing deterministic generation path without introducing parallel patterns, bypassing dispatchers, or violating dependency direction. The four-phase implementation sequence is correctly ordered.

Key recommendations:

1. **Clarify the two serialization paths.** `WriteRecipe` already handles `type`; `ToTOML()` needs the fix for sandbox validation only. Fix `ToTOML()` to also emit `dependencies` while touching it.

2. **Acknowledge the two existing library recipe patterns.** The design generates Pattern B (with `when` clauses). This is correct. State why explicitly.

3. **Decide on `[version]` section presence.** All 22 existing library recipes omit it. Either include it (and document the divergence) or omit it (and explain why library recipes don't need version resolution).

4. **Handle the empty `[verify]` serialization.** Either add `omitempty` to the `Verify` field in `recipeForEncoding`, post-process the output, or accept the cosmetic difference.

5. **Specify `Dependencies` vs `RuntimeDependencies`.** Match the hand-written convention (`dependencies`) or the semantic meaning (`runtime_dependencies`), and document the choice.

None of these are blocking architectural violations. They're precision gaps that will surface during implementation and cause confusion if not addressed upfront.
