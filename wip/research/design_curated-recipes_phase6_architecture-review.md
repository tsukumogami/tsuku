# Architecture Review: DESIGN-curated-recipes.md

**Reviewer:** Architecture review agent
**Date:** 2026-04-16
**Document:** `docs/designs/DESIGN-curated-recipes.md`

---

## Summary

The design is clear and well-scoped. A developer could start from it. The three decisions (TOML flag, `ci.curated` array, recipe + discovery entry) are independent, well-motivated, and correctly sequenced. The main gaps are in the CI layer: the nightly testing workflow description is vague about which workflow file to extend and how it integrates with the real nightly infrastructure, and the design conflates two different nightly workflows without resolving the ambiguity. There are also minor issues with the `ToTOML` serializer and the discovery entry schema.

---

## Detailed Findings

### 1. `MetadataSection.Curated bool` is the right place

`internal/recipe/types.go` line 153 defines `MetadataSection`. Adding `Curated bool \`toml:"curated"\`` there is correct. Zero value (`false`) means "not curated," so all 184 existing handcrafted recipes continue to work without touching. The field doesn't need to be read by any executor path — it's metadata-only, and the struct already has similar advisory fields (`Tier`, `LLMValidation`).

**One gap:** `ToTOML()` (lines 56–70) manually serializes `MetadataSection` fields. It doesn't serialize `Tier` or `LLMValidation` either, so it's consistent — but if `ToTOML` is ever used to round-trip curated recipes (e.g., by the batch pipeline to avoid overwriting them), the `Curated` flag will be silently dropped. The design says the flag helps the batch pipeline avoid overwriting manually authored content, which implies the pipeline reads it after parsing — not via `ToTOML`. This is fine, but worth a note: `ToTOML` is not the serialization path for curated recipes.

### 2. `test-matrix.json` schema: `ci.curated` is compatible, but the workflow integration is hand-wavy

The actual `test-matrix.json` schema uses `ci.linux`, `ci.macos`, and `ci.scheduled` as arrays of test IDs that reference the `tests` object. The design's proposed `ci.curated` array is described as a flat list of recipe paths (`"recipes/c/claude.toml"`), which is a **different schema** from the existing arrays. The existing arrays hold test IDs like `"archive_golang_directory"` that map to entries in `tests`; the proposed `ci.curated` array would hold recipe paths directly.

This is a workable choice (simpler, doesn't require adding entries to `tests`), but the design doesn't acknowledge the schema difference. The jq expression given — `jq '.ci.curated[]'` — would work for path-based entries, but the nightly workflow would need different logic than the existing matrix workflows, which do `jq -c '[.ci.scheduled[] as $id | {id: $id, tool: .tests[$id].tool, desc: .tests[$id].desc}]'`.

A developer implementing this would need to decide: should `ci.curated` entries follow the existing test-ID pattern (adding entries to `tests`) or be a new recipe-path pattern? The design implies the latter but doesn't state it explicitly.

### 3. The nightly workflow target is ambiguous

The design says "the nightly workflow reads this array" in two places. There are two relevant nightly workflows:

- `scheduled-tests.yml`: installs tools via `tsuku install --force` on ubuntu and macos runners. Uses `ci.linux + ci.scheduled` and `ci.macos + ci.scheduled` from `test-matrix.json`.
- `nightly-registry-validation.yml`: validates plan generation (golden file comparison) and executes a sample of recipes alphabetically. Uses R2 golden files.

The design's claimed "11-platform matrix" (5 Linux x86_64, 4 Linux arm64, 2 macOS) matches `recipe-validation-core.yml`, which is triggered from PR workflows but has a `workflow_call` trigger, not `schedule`. It runs `tsuku install --sandbox --force --recipe`.

None of these exactly matches the description. The design says "creates a GitHub issue with platform, recipe, and failure log attached" and "executes each recipe through the full 11-platform sandbox matrix" — but neither `scheduled-tests.yml` nor `nightly-registry-validation.yml` does both. `recipe-validation-core.yml` does the 11-platform testing but isn't a nightly schedule. `nightly-registry-validation.yml` has issue creation but tests plan generation, not live installs.

**Concrete gap:** Phase 1 says to update "the nightly workflow or `recipe-validation-core.yml`" — but these are different things with different scopes. The implementation needs to decide:
- Add a `schedule` trigger to `recipe-validation-core.yml` (or a new wrapper) to cover curated recipes nightly, OR
- Extend `scheduled-tests.yml` to read `ci.curated` recipe paths and run them (simpler but no sandbox/family matrix)

The design should pick one. The "full 11-platform sandbox matrix" description points to `recipe-validation-core.yml`, but adding a nightly trigger to that workflow for just 8-10 recipes is a significant scope change (it currently runs on all recipes changed in a PR). A new dedicated wrapper calling `recipe-validation-core.yml` with a filtered recipe set is probably cleaner and more implementable, but the design doesn't specify this.

### 4. Lint rule: which file, which check?

Phase 1 says "add the lint check to `recipe-validation-core.yml`." But `recipe-validation-core.yml` is a `workflow_call` reusable workflow that validates TOML structure and plan generation. A "curated = true required" check would be a static lint check on the recipe file, more like what `test.yml`'s `validate-recipes` job does (calling `./tsuku validate --strict`).

The right place is either:
- The `tsuku validate` command itself (adding a validation rule for recipes in the `ci.curated` list), or
- A new step in `recipe-validation-core.yml`'s `prepare` job that reads `ci.curated` and checks each listed recipe for `curated = true`

Adding it to `tsuku validate` is cleaner (the validator already enforces structural requirements) but requires reading `test-matrix.json` from within the validator, which is unusual. A CI script step is simpler. The design doesn't specify which approach and should.

### 5. Discovery entry schema: `downloads` and `has_repository` fields

Existing discovery entries (e.g., `cloudflared.json`, `clamav.json`) include `downloads` and `has_repository` fields alongside `builder`, `source`, `description`, and `homepage`. These are likely populated by the batch pipeline. The design's proposed claude entry omits them:

```json
{
  "builder": "npm",
  "source": "@anthropic-ai/claude-code",
  "description": "Claude Code AI coding assistant",
  "homepage": "https://github.com/anthropics/claude-code"
}
```

This is probably fine (the fields look like optional analytics metadata), but the design should acknowledge that handcrafted discovery entries will omit pipeline-managed fields. The `has_repository` field in particular could affect search ranking or display — worth confirming it's safe to omit.

### 6. Data flow: discovery entry is mischaracterized

The "Data Flow" section says the discovery entry is "consulted only if the recipe lookup misses (e.g., cold cache on first install before central registry is fetched)." This is misleading. The central registry is fetched on demand; a fresh install always goes to the registry. The discovery entry is relevant when:

1. The user runs `tsuku search claude` (search reads discovery entries directly)
2. The ecosystem probe falls through to discovery lookup before the recipe is found

In the normal install flow (`tsuku install claude`), with a warm or populated local registry, the TOML recipe is found first and the discovery entry is never read. The design correctly notes this elsewhere but the data flow section implies the opposite order. The data flow should be rewritten as: "recipe lookup succeeds first; discovery entry is only used in search and as a fallback during probe."

### 7. Phase sequencing is correct; Phase 3 naming inconsistency

Phases 1-4 are correctly sequenced. Phase 3 mentions replacing `kubernetes-cli.toml` "or new `kubectl.toml`" — pick one. The existing file is `recipes/k/kubernetes-cli.toml` (confirmed). Creating `recipes/k/kubectl.toml` alongside it would require the old one to be deleted or given a redirect. The design should specify: add `kubectl.toml`, mark `kubernetes-cli.toml` as `supported_os = ["linux"]` (it already is), and add `kubectl.toml` to `ci.curated`. Same for helm — check whether `recipes/h/helm.toml` already exists.

### 8. No simpler alternatives overlooked

The three decisions are minimal for the problem. The `recipes/core/` alternative was correctly rejected (high change surface). The separate nightly workflow alternative was correctly rejected (duplication). The `NpmBuilder` fix deferral is appropriate.

One alternative the design doesn't consider: adding `ci.curated` entries as test IDs in the `tests` object, making them first-class entries like `npm_claude_basic`. This would follow the existing pattern and let the curated recipes plug into `ci.linux`/`ci.macos` for PR testing too (not just nightly). This is additive, not a replacement. For a curated recipe like `claude` that is explicitly cross-platform, having it run on every PR that touches the recipe file is more valuable than nightly-only. The design is silent on whether curated recipes get PR-time integration test coverage.

### 9. Specific Go types and file paths to make concrete

The design is mostly concrete. Additions needed:

- **`internal/recipe/types.go`**: Specify that `Curated bool \`toml:"curated"\`` is added after `LLMValidation string` (line 165) to follow the existing metadata field ordering.
- **Nightly workflow**: Name the specific workflow file to create/modify (see gap #3).
- **Lint rule location**: Specify the exact check location (see gap #4).
- **`test-matrix.json`**: Clarify that `ci.curated` uses recipe paths (not test IDs), and note the `tests` object does not need corresponding entries.

---

## Recommended Changes

1. **Resolve the nightly workflow ambiguity.** Specify that Phase 1 adds a `schedule` trigger to a new wrapper workflow (e.g., `curated-nightly.yml`) that calls `recipe-validation-core.yml` with a filtered recipe list read from `ci.curated`. This avoids modifying the existing PR-triggered workflow.

2. **Clarify the `ci.curated` schema.** Add a note that entries are recipe paths, not test IDs, and do not require corresponding entries in the `tests` object.

3. **Specify the lint rule location.** Recommend a new step in the `prepare` job of `recipe-validation-core.yml` that cross-checks `ci.curated` paths against `curated = true` presence.

4. **Fix the data flow section.** Correct the characterization of when the discovery entry is consulted (search path and probe fallback, not recipe lookup fallback).

5. **Resolve Phase 3 naming.** Pick `kubectl.toml` explicitly; specify that `kubernetes-cli.toml` remains (it's Linux-only by `supported_os`) and the new recipe is additive.

6. **Note `ToTOML` gap.** Add a callout that `ToTOML` doesn't serialize `Curated` and is not used to round-trip handcrafted recipes (clarifies that the batch pipeline reads the parsed struct, not the serialized output).
