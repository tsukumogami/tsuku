# Platform Metadata Mutation Mechanism

Research report on how platform metadata should be written into TOML recipe files after cross-platform validation.

## 1. Codebase Findings

### Existing Platform Constraint Schema

The `MetadataSection` in `internal/recipe/types.go` (lines 163-168) already defines four platform constraint fields:

```go
SupportedOS          []string `toml:"supported_os,omitempty"`
SupportedArch        []string `toml:"supported_arch,omitempty"`
SupportedLibc        []string `toml:"supported_libc,omitempty"`
UnsupportedPlatforms []string `toml:"unsupported_platforms,omitempty"`
```

Several recipes use these today: `btop.toml` has `supported_os = ["linux"]`, `iterm2.toml` has `supported_os = ["darwin"]`, `cuda.toml` has `supported_os = ["linux"]`.

### Design Doc Proposed Format

DESIGN-registry-scale-strategy.md (lines 621-625) proposes a different format:

```toml
[metadata]
platforms = ["linux-glibc-x86_64", "linux-glibc-arm64", "darwin-x86_64", "darwin-arm64"]
```

This is a composite string format (`os-libc-arch`) rather than the existing decomposed fields (`supported_os`, `supported_arch`, `supported_libc`). The two formats express overlapping but not identical information. The composite format is more precise (each tuple is an explicit platform) while the decomposed format creates a cross-product (all combinations of listed OS x arch x libc).

### TOML Writer Infrastructure

`internal/recipe/writer.go` has a `WriteRecipe` function that serializes a `Recipe` struct to TOML using atomic write-temp-rename. `types.go` also has a `ToTOML()` method that does manual TOML serialization. Both paths exist and work. The `recipeForEncoding` struct in `writer.go` maps directly from `MetadataSection`, so any field added to `MetadataSection` with a `toml` tag is automatically encoded.

### Batch Pipeline Design

DESIGN-batch-recipe-generation.md describes a merge job (Job 4) that collects passing recipes and creates a PR. The validate-platforms job (Job 3) produces "per-recipe, per-platform results" passed as artifacts. The design says "Update platform coverage metadata per recipe" in Job 3 but doesn't specify the mechanism.

### test-changed-recipes.yml

The existing PR validation workflow runs on `ubuntu-latest` and `macos-latest`. It uses an execution-exclusions JSON file for recipes that can't be tested. It does not read or write platform metadata from recipes. Any platform annotation mechanism would need to be compatible with this workflow's expectations.

## 2. Industry Patterns

### Homebrew

Bottles (pre-built binaries) declare platform availability in the formula's `bottle` block. This is written by CI after successful builds -- the `brew bottle` command generates the block and a PR bot updates the formula. The formula author doesn't write bottle availability; CI derives it from build results. This is closest to Option A (CI writes metadata after validation).

### Conda

Uses `platform` selectors as inline comments in `meta.yaml`: `# [linux]`, `# [osx]`. These are author-declared, not CI-derived. Platform support is known upfront from the package's nature.

### Nix

`meta.platforms` is author-declared in the derivation. Hydra (CI) builds on all declared platforms and marks broken ones, but doesn't rewrite the derivation. A separate `meta.broken` flag can be set per platform. This is closest to Option C (metadata stays separate from the recipe).

### Cargo (Rust)

`Cargo.toml` uses `[target.'cfg(...)'.dependencies]` for conditional deps. Platform support is declared by the author via `cfg` attributes in code, not by CI. Not directly comparable since Cargo packages are source-built.

### Pattern Summary

CI-derived platform metadata written back into the package definition is the Homebrew pattern. Most other systems use author-declared platform support. The key difference: Homebrew's bottle block is strictly about pre-built binary availability, while the formula itself works on any platform. Tsuku's situation is similar -- the recipe TOML is platform-independent, but some download URLs only resolve on certain platforms.

## 3. Options Analysis

### Format Decision: Composite vs. Decomposed

Before evaluating mutation mechanisms, the format question matters. Two choices:

**Composite (`platforms = ["linux-glibc-x86_64", ...]`):** Exact enumeration. No ambiguity. Easy to derive from validation results (just list passing platforms). Harder to author manually. Doesn't compose with existing `supported_os`/`supported_arch` fields.

**Decomposed (existing `supported_os`/`supported_arch`/`supported_libc`):** Cross-product semantics. Can't express "linux-arm64 but not linux-x86_64" without also using `unsupported_platforms`. Already in the schema and used by existing recipes.

**Recommendation:** Use the existing decomposed fields when they suffice (e.g., a recipe only works on Linux). Add a new `platforms` field for the batch pipeline's precise enumeration. The CLI should treat `platforms` as an override -- if present, it defines exactly which platforms are supported. If absent, fall back to the decomposed fields (or assume all platforms).

This avoids a schema migration for existing recipes while giving the batch pipeline a precise field.

### Option A: Merge Job Writes Platforms Field

The merge job (Job 4) reads validation result artifacts, determines which platforms each recipe passed on, and writes `platforms = [...]` into the TOML before committing.

**Implementation:**
- Merge job downloads per-recipe result artifacts from Job 3
- For each recipe, if any platform failed, add/update `[metadata].platforms` with passing platforms
- If all platforms passed, omit the field (universal support)
- Use existing `WriteRecipe` or a simpler TOML manipulation (read, patch metadata section, write)

**Pros:**
- Single responsibility: the batch workflow owns the full lifecycle from generation to annotation to merge
- No new CLI surface area
- Platform metadata is written exactly once, at the right time (after all validation completes)
- Recipes in the PR already have correct metadata when CI re-validates

**Cons:**
- Merge job needs TOML read-modify-write logic (but `WriteRecipe` already exists)
- The Go orchestrator (`cmd/batch-generate`) grows in scope
- Can't reuse the annotation logic outside the batch context without extracting it

**Changes required:**
- Add `Platforms` field to `MetadataSection` (schema change)
- Add platform annotation logic to `cmd/batch-generate` or `internal/batch`
- No changes to `test-changed-recipes.yml` (it doesn't check platforms)

**Complexity:** Low-medium. The TOML writer exists. The merge job already collects results.

### Option B: Separate `tsuku annotate-platforms` CLI Command

A new CLI subcommand reads a validation results file (JSON) and updates the recipe TOML.

```bash
tsuku annotate-platforms --results validation-results.json --recipe recipes/r/ripgrep.toml
```

**Implementation:**
- New subcommand in `cmd/tsuku/`
- Reads JSON results, maps to platform list, writes `platforms` field
- Merge job calls this command for each recipe

**Pros:**
- Reusable: operators can annotate platforms manually or from any CI system
- Testable in isolation (unit tests for the command)
- Follows tsuku's pattern of CLI commands for all operations
- Could be used by `test-changed-recipes.yml` in the future

**Cons:**
- New CLI surface area to maintain
- The batch pipeline already installs tsuku; calling an extra command per recipe adds overhead (minimal)
- Results file format becomes a contract between CI and CLI

**Changes required:**
- Add `Platforms` field to `MetadataSection`
- New CLI command (`cmd/tsuku/annotate_platforms.go` or similar)
- Merge job invokes the command

**Complexity:** Medium. New command, but straightforward.

### Option C: Platforms Omitted, Sidecar Files

Don't write platform metadata to the recipe. Store it in a sidecar file (e.g., `recipes/r/ripgrep.platforms.json`) or derive it from validation artifacts at PR-CI time.

**Pros:**
- Recipe TOML stays "pure" -- no CI-derived fields mixed with author-declared fields
- No schema change to recipe format
- Sidecar files can have richer data (failure reasons, timestamps)

**Cons:**
- Two files per recipe increases registry complexity
- CLI must know to read sidecar files when checking platform support
- `test-changed-recipes.yml` must be updated to read sidecar files and skip non-matching platforms
- Breaks the "recipe is self-contained" principle
- Every consumer of recipes (CLI install, CI validation, registry listing) needs sidecar awareness

**Changes required:**
- Sidecar file format and schema
- CLI changes to read sidecar files during install (check platform before attempting)
- CI workflow changes to read sidecar files
- Registry listing changes (website, `tsuku recipes` command)

**Complexity:** High. Touches many consumers.

### Option D: Two-Phase (Generate with Placeholder, Refine After Validation)

Generator writes `platforms = ["all"]` or omits the field. After validation, a post-processing step rewrites with actual results.

**Pros:**
- Explicit two-phase model makes the lifecycle clear
- The intermediate state ("all" or omitted) is safe -- it means "not yet validated" which is equivalent to "assumed universal"

**Cons:**
- `platforms = ["all"]` is a sentinel value that the CLI must understand
- If the refinement step fails or is skipped, recipes ship without platform restrictions (potentially causing install failures on unsupported platforms)
- Functionally identical to Option A -- the merge job writes the final value. The "two-phase" label doesn't add value; it just describes what Option A already does (generate without platforms, add them after validation).

**Changes required:** Same as Option A plus sentinel value handling.

**Complexity:** Same as Option A with extra complexity for sentinel values.

### Maintainability: Adding New Platforms

When a new platform is added (e.g., `linux-riscv64`):

- **Options A, B, D:** Existing recipes don't list the new platform, so they're implicitly unsupported. A bulk re-validation run would add the new platform to recipes that pass. The `platforms` field uses an allowlist, so omission means "not validated" rather than "not supported."
- **Option C:** Sidecar files would also need re-generation. Same re-validation needed, but sidecar file creation is an extra step.

All options require re-validation when adding platforms. None is significantly better than the others here.

## 4. Recommendation

**Option A: Merge job writes platforms field**, with one refinement from Option B.

### Rationale

Option A is the simplest path that solves the problem. The merge job already collects validation results and assembles the PR. Adding platform annotation is a natural extension of that job. The TOML writer infrastructure exists. No new CLI surface area is needed for the initial implementation.

The refinement: implement the annotation logic as a Go function in `internal/batch/` (not as a CLI command), but structure it so it can be promoted to a CLI command later if needed. This gives Option A's simplicity now with an easy path to Option B's reusability later.

### Specific Design

1. **Schema:** Add `Platforms []string` with `toml:"platforms,omitempty"` to `MetadataSection`. Values use the composite format from the design doc: `linux-glibc-x86_64`, `linux-glibc-arm64`, `linux-musl-x86_64`, `darwin-arm64`, `darwin-x86_64`.

2. **Semantics:** If `platforms` is present, the recipe is only supported on listed platforms. If absent, the recipe is assumed to work on all platforms (backward compatible with existing recipes). The existing `supported_os`/`supported_arch`/`supported_libc` fields remain for author-declared constraints.

3. **Precedence:** `platforms` (CI-derived, precise) takes precedence over decomposed fields when both are present. In practice, batch-generated recipes will use `platforms`; hand-authored recipes will use decomposed fields.

4. **Merge job logic:**
   - Read validation result artifacts (JSON per recipe per platform)
   - If all 5 platforms passed: omit `platforms` field (universal)
   - If 1-4 platforms passed: set `platforms` to passing list
   - If 0 platforms passed: exclude recipe from PR (existing behavior)
   - Write updated recipe using `WriteRecipe`

5. **No changes to `test-changed-recipes.yml`:** The existing workflow validates on ubuntu and macos. It doesn't need to read the `platforms` field because it tests whatever recipes are in the PR. The platforms field is consumed by `tsuku install` at runtime, not by CI.

### Key Trade-offs

- Merging CI-derived metadata into the recipe file mixes concerns (author content + CI results in one file). This is the Homebrew pattern and works well in practice.
- The `platforms` field creates a second way to express platform constraints alongside the decomposed fields. Clear precedence rules prevent ambiguity.
- No reusable CLI command for annotation. If this becomes needed, extract `internal/batch/annotate.go` into a `tsuku annotate-platforms` command.
