# Architecture Review: DESIGN-recipe-ci-batching.md

**Reviewer**: architect-reviewer
**Date**: 2026-02-21
**Document**: `/home/dangazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/docs/designs/DESIGN-recipe-ci-batching.md`

## 1. Is the architecture clear enough to implement?

**Verdict: Yes, with two gaps to close.**

The design is implementable as written. The detection-to-execution data flow is well specified, the batch object schema is concrete (`batch_id`, `recipes` array), and the jq splitting logic is provided inline. An implementer can follow the workflow-specific details in the "Solution Architecture" section to modify each file.

### Gap 1: `validate-golden-recipes.yml` cache key change not addressed

The existing `validate-golden-recipes.yml` uses per-recipe cache keys (line 166-169):

```yaml
key: golden-downloads-${{ matrix.item.recipe }}-${{ matrix.item.category }}
restore-keys: |
  golden-downloads-${{ matrix.item.recipe }}-
  golden-downloads-
```

When the matrix switches from per-recipe to per-batch, the cache key needs to change to a batch-level key (or be removed, since golden validation doesn't download tools). The design doesn't mention this. It's a small implementation detail, but cache key mismatches could silently defeat caching or, worse, cross-pollinate cache entries between recipes that shouldn't share state.

**Severity: Advisory.** Contained to one file; won't create a pattern others copy.

### Gap 2: R2-dependent execution jobs in `validate-golden-execution.yml` are out of scope but share the same problem

The design explicitly scopes out `validate-golden-execution.yml`, stating that container-family batching "already works." This is true for the `validate-linux-containers` job (lines 608-705), which batches by family. However, two other jobs in that workflow use per-recipe matrices:

- `validate-coverage` (line 378): `matrix.recipe` from `fromJson(needs.detect-changes.outputs.recipes)`
- `execute-registry-linux` (line 716): `matrix.recipe` from `fromJson(needs.detect-changes.outputs.registry_recipes)`
- `validate-linux` (line 515): per-file matrix from `linux_matrix`

These have the same N-jobs-per-N-recipes scaling problem the design solves for the other two workflows. The design doesn't need to fix them now, but the "Out of scope" section should explicitly acknowledge they exist and will need the same treatment later. Without this, someone reading the design post-implementation might assume all per-recipe matrices have been batched.

**Severity: Advisory.** No code impact; documentation clarity.

## 2. Are there missing components or interfaces?

### 2a. No missing structural components

The design correctly identifies that no new files are needed -- only modifications to existing workflow YAML. This fits the CI architecture: workflows are self-contained, and the batch splitting logic is inline shell/jq. There's no need for a shared action, reusable workflow, or new script.

### 2b. The "shared shell function" suggestion should be inline instead

The design mentions a "shared shell function (or inline jq)" for `batch_recipes()` (line 203). This is a fork point: a shared function implies either a script file in `.github/scripts/` or a composite action. Either would be a new file that two workflows depend on.

The better choice is inline jq in each workflow's detection step. The jq expression is 6 lines. Duplicating it across two workflows is cheaper than introducing a shared dependency between workflows that currently have no coupling. The workflows already duplicate much larger patterns (Go setup, build steps, failure reporting).

The design allows either approach ("or inline jq"), so this isn't a gap -- just a recommendation for the implementer.

**Severity: Advisory.** No structural risk either way; inline is simpler.

### 2c. Missing: behavior when recipe count is zero after filtering

The detection jobs already handle `has_changes=false` to skip execution jobs. But the design doesn't specify what happens when the recipe list is non-empty before batching but becomes empty after filtering (e.g., all changed recipes are libraries or excluded). The existing workflows handle this (they check `JSON = "[]"`), and the batch splitting step should preserve that behavior.

This is implicit -- the jq expression on an empty array produces `[]`, and `fromJSON('[]')` produces a matrix with no entries, so the job is skipped. But the design should be explicit that the `has_recipes` output is computed after batching, not before.

**Severity: Advisory.** The existing code already handles this; just a documentation gap.

## 3. Are the implementation phases correctly sequenced?

**Verdict: Yes. The sequencing is correct.**

- **Phase 1** (test-changed-recipes.yml Linux) is the right starting point: highest job count, most visible impact, and the macOS pattern in the same file provides a reference implementation. The implementer can literally copy the macOS loop structure (lines 226-258 of the existing workflow) and adapt it.

- **Phase 2** (validate-golden-recipes.yml) depends on Phase 1 proving the pattern works. It's simpler (no platform detection, no macOS handling), so it's correctly second.

- **Phase 3** (tuning) is correctly last. You can't tune batch size without data, and data requires Phases 1-2 to be running in production.

One observation: Phase 1 could be split into two sub-phases -- (a) modify the detection job to output batches, and (b) modify the execution job to consume batches -- and landed as a single PR with both changes. This is an implementation tactic, not a design issue.

## 4. Are there simpler alternatives we overlooked?

### 4a. Considered and correctly rejected: single-job aggregation

The design correctly rejects running all Linux recipes in a single job. For macOS (limited runners), aggregation makes sense. For Linux (abundant runners), parallelism across batches is worth the small per-job overhead.

### 4b. Not considered: `max-parallel` on the existing matrix

GitHub Actions `strategy.max-parallel` can limit concurrency without changing the matrix structure. Setting `max-parallel: 20` on the existing per-recipe matrix would cap runner consumption at 20 concurrent jobs.

This addresses the "queue pressure" concern without any detection-job changes. It does NOT address the "per-job overhead" concern (each job still pays checkout + Go setup + build for one recipe). For a 200-recipe PR, you'd still have 200 jobs, each with 30-45s overhead, just running 20 at a time instead of all at once.

The design's batching approach is strictly better: it reduces both job count and total overhead. `max-parallel` is a band-aid; batching fixes the root cause. The design made the right call, but mentioning `max-parallel` as a rejected alternative would strengthen the rationale.

**Severity: Not a gap.** Just a documentation suggestion.

### 4c. Not considered: build artifact caching across steps (not jobs)

The design's "Out of scope" section mentions "build artifact caching across workflows" but the real low-hanging fruit is within-workflow caching. Each batched job still runs `go build`. With 5 batched jobs, that's 5 builds. The Go module cache (already configured via `setup-go`) handles dependency downloads, but the build itself still runs.

An alternative approach: build tsuku once in the detection job, upload it as an artifact, download it in each batch job. This eliminates N builds and replaces them with 1 build + N artifact downloads (~3-5s each vs ~15-20s build).

The macOS path already builds once (since it's a single job). The detection job in `test-changed-recipes.yml` already builds tsuku (line 36) for platform detection. So the build artifact is already being produced; it's just not being shared.

This is orthogonal to the batching design and correctly out of scope. But it compounds well with batching: 5 batch jobs * 15s saved per build = 75s total. Worth noting as a follow-up.

**Severity: Out of scope.** Not a gap in this design.

## 5. Compatibility with existing workflows

### 5a. `test-changed-recipes.yml` compatibility

The design proposes modifying two jobs in this workflow:
- **Detection job** (`matrix`): Adds a batch splitting step after the existing platform detection. The existing outputs (`recipes`, `has_changes`, `macos_recipes`, `has_macos`) are preserved; new outputs (`batches`, `batch_count`) are added. No breaking change.
- **`test-linux` job**: Changes `matrix.recipe` to `matrix.batch`. This is an internal change; no other workflow references `test-linux` outputs.
- **`test-macos` job**: Unchanged. Correct -- it already uses the aggregated pattern.

The existing detection job builds tsuku for platform detection (line 36). This binary could be reused by batch jobs via artifact upload, but the design doesn't propose this (correctly out of scope).

**Compatibility: Clean.** No conflicts with the existing workflow structure.

### 5b. `validate-golden-recipes.yml` compatibility

The design proposes modifying two jobs:
- **Detection job** (`detect-changes`): Adds batch splitting. Existing outputs preserved.
- **`validate-golden` job**: Changes `matrix.item` to `matrix.batch`. Adds inner loop.

There are two dependencies that need careful handling:

1. **R2 health check**: The `validate-golden` job depends on `r2-health-check` (line 144). This dependency is preserved; each batch job still needs R2 availability information. The design doesn't break this.

2. **Per-recipe R2 golden file check**: Inside the validation step (lines 196-199), each recipe checks whether golden files exist in R2 before validating. When batched, this check moves inside the inner loop. The design's failure-accumulation pattern handles this correctly (skip the recipe, continue to the next).

3. **Category-dependent golden source**: The existing workflow uses `$CATEGORY` (embedded vs registry) to determine the golden source (git vs R2). In a batch, recipes might have mixed categories. The design's batch object includes a `recipes` array but doesn't show whether `category` is preserved per-recipe. Looking at the detection job output:

```json
{"recipe":"ripgrep","category":"registry"}
```

The batch object schema in the design:
```json
{"batch_id": 0, "recipes": [{"tool": "ripgrep", "path": "recipes/r/ripgrep.toml"}, ...]}
```

**This is a gap.** The design's batch recipe object has `tool` and `path` but not `category`. The `validate-golden-recipes.yml` inner loop needs `category` to determine golden source and golden directory. Either:
- (a) Add `category` to the per-recipe object in the batch, or
- (b) Derive `category` from `path` at runtime (embedded paths start with `internal/recipe/recipes/`).

Option (b) matches what the existing detection job already does and avoids schema changes. The implementer should use (b).

**Severity: Advisory.** Solvable at implementation time with no structural impact.

### 5c. Cross-workflow interactions

No other workflows depend on the outputs or status of `test-changed-recipes.yml` or `validate-golden-recipes.yml`. These are PR-check workflows; they report status back to the PR but don't trigger other workflows. Batching doesn't change this.

The `batch-generate.yml` workflow has its own completely separate batching mechanism (batch size for recipe generation, not CI job batching). There's no naming collision or conceptual overlap -- that workflow batches the number of recipes to generate per run, while this design batches the number of recipes to test per CI job. Different concerns, different workflows.

## 6. Structural fit assessment

### Extends existing pattern -- does not introduce a parallel one

The design's core approach (inner loop with `::group::`, failure accumulation, isolated `TSUKU_HOME`) is already implemented in:
- `test-changed-recipes.yml` `test-macos` job (lines 226-258)
- `validate-golden-execution.yml` `validate-macos` job (lines 571-603)
- `validate-golden-execution.yml` `validate-linux-containers` job (lines 631-705)

The design extends this pattern to the remaining per-recipe matrix jobs. It does not introduce a new pattern. This is the right architectural choice.

### jq snippet has a bug

The jq in the design's "Considered Options" section (lines 58-64) references `$RECIPES` as both the piped input and a variable:

```bash
echo "$RECIPES" | jq -c --argjson bs "$BATCH_SIZE" '
  [range(0; length; $bs)] as $starts |
  [$starts[] | {
    batch_id: (. / $bs | floor),
    recipes: $RECIPES[(.):(. + $bs)]
  }]
'
```

`$RECIPES` inside the jq expression is undefined (it would need `--argjson` to bind it). The corrected version in the "Solution Architecture" section (lines 207-215) uses `. as $all` to capture the input, which is correct. The earlier snippet appears to be a draft that wasn't cleaned up. Not a structural issue, but the incorrect snippet could mislead an implementer who reads top-down and stops early.

**Severity: Advisory.** The correct version exists later in the document.

## Summary of findings

| Finding | Severity | Recommendation |
|---------|----------|----------------|
| Cache key change in `validate-golden-recipes.yml` not addressed | Advisory | Mention in implementation notes |
| Per-recipe matrices in `validate-golden-execution.yml` share the same scaling problem | Advisory | Acknowledge in "Out of scope" as future work |
| `category` field missing from batch recipe object | Advisory | Derive from path at runtime |
| `max-parallel` not mentioned as rejected alternative | Advisory | Add to "Alternatives Considered" |
| jq snippet in Decision 1 has a bug (corrected later in doc) | Advisory | Fix the early snippet for consistency |
| Build artifact sharing across batch jobs | Out of scope | Note as follow-up optimization |

**No blocking findings.** The design is architecturally sound, extends proven patterns, and is phased correctly. All findings are advisory -- solvable at implementation time without structural changes.
