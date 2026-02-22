# Phase 4 Review: Library Recipe Generation Design

## Reviewer: Architect

## 1. Problem Statement Specificity

The problem statement is specific enough to evaluate solutions against. It names the exact function (`extractBottleBinaries` at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/builders/homebrew.go:1566`), the exact failure mode (bin/ empty leads to "no binaries found" error classified as `complex_archive`), and the exact target pattern (22 existing library recipes using `type = "library"` + `install_mode = "directory"`).

One gap: the problem says "70 unique packages have hit complex_archive" but does not quantify how many of those are true libraries vs. Python packages, multi-tool bundles, or other types. The design should state the expected hit rate. If only 15 of those 70 are libraries, the return on complexity is different than if 50 are.

**Recommendation:** Add a sentence estimating the library fraction of the 70, even as a rough range (e.g., "estimated 20-40 are pure libraries based on manual inspection of a sample").

## 2. Missing Alternatives

### Decision 1 (Detection): No missing alternatives

The three options cover the design space well. The keg_only API flag rejection is grounded -- it is indeed unreliable for detecting library-only packages. Scanning lib/ in the already-downloaded bottle is the obvious right answer.

### Decision 2 (Output List): One missing alternative worth mentioning

**Minimal outputs (only .a and .pc files, no versioned symlinks):** Several existing library recipes include versioned symlinks like `lib/libreadline.so.8`, `lib/libreadline.so.8.2`, `lib/libreadline.8.dylib` (see `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/recipes/r/readline.toml`). These are the most fragile entries -- they break on every minor version bump. An alternative is to enumerate only unversioned `.so`, `.a`, `.dylib`, `.pc`, and headers, excluding versioned symlinks. This trades off completeness for durability.

This matters because the design already calls out "fragility of enumerated file lists across library versions" as an accepted risk. The alternative of filtering versioned symlinks would reduce that fragility significantly.

**Recommendation:** Evaluate this as a fourth option under Decision 2, or at minimum document why versioned symlinks are included (presumably because existing recipes like readline.toml include them and consistency with the established pattern matters more than durability).

### Decision 3 (Platform Handling): No missing alternatives

Multi-platform with when clauses is the established pattern. The existing library recipes split into three categories:

1. Platform-unconditional (brotli, readline, zstd at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/recipes/b/brotli.toml`) -- single set of outputs containing both `.so` and `.dylib` entries
2. Platform-conditional with libc split (gmp, cairo at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/recipes/g/gmp.toml`) -- separate homebrew+install_binaries step pairs with `when` clauses for linux/glibc vs darwin, plus musl fallback
3. Minimal unconditional (libpng at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/recipes/l/libpng.toml`) -- shared static files only

The design says "download bottles for linux-x86_64 and darwin-arm64, scan each, generate platform-conditional steps." This matches pattern #2 above. However, some existing recipes use pattern #1 (mixing .so and .dylib in a single unconditional outputs list). The design should explicitly state which pattern it will generate and why.

### Decision 4 (Failure Categories): One structural concern

Adding `library_only` as a new `DeterministicFailureCategory` requires a schema change to `failure-record.schema.json` (at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/data/schemas/failure-record.schema.json`), which currently enumerates:

```json
"enum": ["missing_dep", "no_bottles", "build_from_source", "complex_archive", "api_error", "validation_failed"]
```

The `DeterministicFailureCategory` constants in `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/builders/errors.go:138-145` must stay in sync with this schema. The design acknowledges this coupling exists but does not state whether `library_only` should be added to the schema enum or whether it is a subcategory of `complex_archive`.

**Recommendation:** Clarify whether `library_only` is:
- A new top-level category in the schema (requires schema migration, dashboard updates)
- A subcategory/tag within `complex_archive` (uses existing `knownSubcategories` pattern in `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/dashboard/failures.go:41-50`)

The subcategory approach fits the existing architecture better. The dashboard already has `extractSubcategory()` that parses bracketed tags and keyword patterns. Adding `library_only` there is zero-schema-change. Adding it as a top-level category is a schema contract change that touches the Go constants, the JSON schema, and the dashboard parsing.

## 3. Rejection Rationale Assessment

### Decision 1 rejections: Fair

- "keg_only is unreliable, still need bottle for file list" -- correct. The Homebrew API metadata does not include keg_only in the formula JSON that `homebrewFormulaInfo` parses (checked: the struct at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/builders/homebrew.go:250-286` has no keg_only field, and grepping the codebase for "keg_only" returns zero results).
- Hybrid rejection is fair -- it's additional complexity for an unreliable signal.

### Decision 2 rejections: Fair but incomplete

- "Only lib/ files" rejection makes sense -- headers are needed.
- "Wildcard patterns" rejection is fair and correctly identifies scope creep.
- Missing: why not include `lib/pkgconfig/*.pc` files specifically? Multiple existing recipes include them (gmp, cairo, brotli, zstd). The "enumerate all lib/ and include/" wording implies pkgconfig files are included since they're under lib/, but this should be explicit.

### Decision 3 rejections: Fair

- "Single-platform" rejection is reasonable for a recipe generator aiming for complete output.
- "Template with placeholders" rejection is well-reasoned -- TODOs in committed recipes are bad for the batch pipeline.

### Decision 4 rejections: Reasonable but thin

- "No new category" rejection says "user wants better observability" -- this is a preference statement, not a technical argument. The real argument is that `complex_archive` is a catch-all that lumps libraries with Python packages and multi-tool bundles, making gap analysis less actionable. State it that way.

## 4. Unstated Assumptions

### Assumption 1: The bottle tarball structure is consistent for library packages

The design assumes `formula/version/lib/` and `formula/version/include/` paths. This is the Homebrew convention, but it should be stated explicitly. The existing `extractBottleBinaries` function at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/builders/homebrew.go:1593` hardcodes `parts[2] == "bin"` and `len(parts) >= 4`. The library scanner will need similar structural assumptions for lib/ and include/.

### Assumption 2: ToTOML can serialize library recipes correctly

The `ToTOML()` method at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/recipe/types.go:52` does NOT emit `metadata.type`. It hand-encodes `name`, `description`, `homepage`, and `version_format`, then relies on `toml.NewEncoder` for steps. For library recipes, the generated TOML will be missing `type = "library"` unless `ToTOML` is updated.

This is a **blocking implementation gap** that the design should acknowledge. The `generateDeterministicRecipe` function builds a `recipe.Recipe` struct and the batch pipeline (or `tsuku create`) calls `ToTOML()` to write the file. If `type` is not serialized, the generated recipe won't be recognized as a library by `IsLibrary()` after being written and re-read.

Similarly, `install_mode = "directory"` and `outputs` (vs `binaries`) must appear in the step params for the generated recipe to match the existing pattern. The current `generateDeterministicRecipe` at line 1888 uses `"binaries"` as the params key, not `"outputs"`. Library recipes in the registry use `outputs`. The design should state whether it will use `outputs` (matching convention) or `binaries` (matching current code).

### Assumption 3: Libraries don't need a [verify] section

The explore summary says "Libraries are exempt from [verify] section requirement (can't execute .so to verify)." Most library recipes in the registry indeed have no `[verify]`. But readline.toml does:

```toml
[verify]
command = "ls lib/libreadline.a"
pattern = "libreadline"
mode = "output"
```

The design should state whether generated library recipes will include a verify section (possibly a file-existence check) or omit it entirely. The `generateDeterministicRecipe` function currently generates a verify command like `"<binary> --version"` -- this must be changed for libraries.

### Assumption 4: The recipe struct can represent platform-conditional steps

The design says "multi-platform with when clauses." The current `generateDeterministicRecipe` produces steps without `When` clauses. The code at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/builders/homebrew.go:1888-1901` creates `recipe.Step` structs with nil `When`. For platform-conditional output, the function needs to create multiple step pairs with `When` clauses, which requires changes to the generation logic. This is implementable (the Step struct supports it) but the design should acknowledge the code shape change.

### Assumption 5: Downloading multiple bottles (linux + darwin) won't hit rate limits

The design says "download bottles for linux-x86_64 and darwin-arm64." The current code downloads one bottle per call. For batch generation of 70+ packages times 2 platforms, that's 140+ GHCR downloads. GHCR has anonymous rate limits. The design doesn't discuss whether this is a concern. The existing code fetches an anonymous token per call (`getGHCRToken`), so 140 token requests plus 140 blob downloads. This may be fine for GHCR's limits, but it should be stated as a known factor.

## 5. Strawman Check

None of the rejected options are strawmen. They are all plausible alternatives with real trade-offs:

- Homebrew API metadata (keg_only) is a legitimate approach used by other tools; it just happens to be unreliable for this use case.
- Single-platform generation is simpler and valid for a first iteration, though the multi-platform choice is defensible.
- Wildcard patterns would be a real improvement, but the scope creep argument is solid.

The options analysis is balanced.

## Summary of Findings

### Items requiring design clarification (not blocking review, but should be addressed before implementation):

1. **Quantify the library fraction of complex_archive failures.** Without this, we can't evaluate the return on the multi-platform scanning complexity.

2. **Clarify library_only as subcategory vs top-level category.** The subcategory approach (`knownSubcategories` map in dashboard/failures.go) fits the architecture better and avoids a schema migration. Recommend subcategory.

3. **Acknowledge ToTOML gap.** `ToTOML()` does not emit `metadata.type`. Either the design includes updating `ToTOML` or the generated recipe must use a different serialization path.

4. **State which recipe pattern will be generated.** Pattern #1 (unconditional, mixed extensions) or Pattern #2 (platform-conditional with when clauses). Existing recipes use both.

5. **Decide on `outputs` vs `binaries` key.** Current code uses `"binaries"`; existing library recipes use `outputs`. The `"binaries"` key is documented as deprecated in the install_binaries action.

6. **Specify verify section handling for libraries.** Omit entirely, or generate a file-existence check like readline.toml uses.

### Structural architecture concern (advisory, not blocking):

The `generateDeterministicRecipe` function at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/builders/homebrew.go:1834` currently produces a tool-style recipe (steps without when clauses, binary paths in bin/, verify via `--version`). Making it also produce library-style recipes (steps with when clauses, outputs in lib/ and include/, type = "library", no verify or file-check verify) means the function needs a significant branch. Consider whether this should be a separate `generateDeterministicLibraryRecipe` function alongside the existing one, called from the same detection point, rather than growing the existing function with conditional logic. This is a code organization suggestion, not an architectural concern -- either approach works within the builder pattern.
