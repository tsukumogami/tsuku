# Maintainer Review: Issue #1815

**Issue**: ci(golden-recipes): batch per-recipe jobs in validate-golden-recipes
**Focus**: maintainability (clarity, readability, duplication)
**Files reviewed**: `.github/ci-batch-config.json`, `.github/workflows/validate-golden-recipes.yml`

---

## Finding 1: Divergent batch-splitting blocks across two workflows (Advisory)

**Location**: `.github/workflows/validate-golden-recipes.yml` lines 149-200 vs `.github/workflows/test-changed-recipes.yml` lines 178-235

The "Split recipes into batches" step is nearly identical in both workflows. The logic is the same: read override, clamp 1-50, fall back to config file, fall back to hardcoded default, split with the jq ceiling-division expression. The only differences that matter are:

1. The config file key (`"validate-golden-recipes".default` vs `"test-changed-recipes".linux`)
2. The hardcoded fallback value (20 vs 15)
3. The output variable names (`batches`/`batch_count` vs `linux_batches`/`has_linux_batches`/`batch_count`)

Everything else -- the override detection, the clamping logic, the jq expression, the empty-input guard -- is character-for-character identical. The next developer who needs to fix the clamping logic or change the jq expression will need to remember to update both places. The design doc explicitly noted "A shared shell function (or inline jq)" as an option; the implementation chose inline duplication.

This is advisory, not blocking. The two workflows are separate files, the duplication is visible (you can grep for `range(0; length; $bs)` and find both), and GitHub Actions YAML doesn't have great abstractions for sharing inline shell. A shared script in `.github/scripts/` would clean this up, but it's not worth blocking on for two instances. If a third workflow (#1816's follow-up for `validate-golden-execution.yml`) copies this block again, extracting to a shared script should become a prerequisite rather than an afterthought.

**Severity**: Advisory.

---

## Finding 2: Failure accumulation mechanism differs between workflows (Advisory)

**Location**: `.github/workflows/validate-golden-recipes.yml` lines 235-293 (FAIL_FILE temp file) vs `.github/workflows/test-changed-recipes.yml` lines 266-303 (also FAIL_FILE temp file -- corrected from initial #1814 review)

Both workflows now use the same `FAIL_FILE` mechanism for failure accumulation. Good. The previous #1814 maintainer review noted the PATH accumulation fix unified the two `test-changed-recipes.yml` loops. This issue correctly follows the same pattern.

However, there's a difference in per-recipe error annotations. In `validate-golden-recipes.yml`, the inner loop emits per-recipe `::error::` annotations when validation fails (lines 273-279), providing actionable remediation guidance per recipe. In `test-changed-recipes.yml`, the inner loop does NOT emit per-recipe `::error::` annotations (only the batch summary at line 299).

This difference is intentional and appropriate: golden validation failures need specific remediation instructions (which script to run, whether R2 will auto-regenerate), while installation failures are self-explanatory from the log output. The next developer won't confuse these because the workflows have fundamentally different inner actions.

**Severity**: Advisory (documenting that the difference is intentional, not flagging it as a problem).

---

## Finding 3: Variable naming inconsistency between batch object fields (Advisory)

**Location**: `.github/workflows/validate-golden-recipes.yml` line 240 (`RECIPE`) vs `.github/workflows/test-changed-recipes.yml` line 280 (`tool`)

The two workflows use different field names in their recipe objects and loop variables:

- `test-changed-recipes.yml`: batch objects contain `{"tool": "...", "path": "..."}`, loop variable `tool`
- `validate-golden-recipes.yml`: batch objects contain `{"recipe": "...", "category": "..."}`, loop variable `RECIPE`

The field name difference (`tool` vs `recipe`) is inherited from the pre-batching detection logic in each workflow. This is fine -- the workflows detect different things and the names reflect their domain (`tool` for an installable tool, `recipe` for a golden-file-validated recipe).

The shell variable casing difference (`tool` lowercase vs `RECIPE` uppercase) is a minor inconsistency. Shell convention uses uppercase for exported/environment variables and lowercase for local variables. `RECIPE` is not exported -- it's a local loop variable, so lowercase would be more conventional and consistent with the other workflow. This won't cause a misread, but it's a small style inconsistency.

**Severity**: Advisory.

---

## Finding 4: Workflow reads clearly and the batching pattern is well-structured

The overall implementation is clear. A developer encountering this workflow for the first time will understand:

1. Detection happens once, splits into batches
2. Each batch gets its own runner, builds once, loops through recipes
3. Failures accumulate and report at the end

The `::group::`/`::endgroup::` annotations, the batch-level job naming (`Validate (batch 1/4)`), and the failure summary all contribute to good CI observability. The R2 availability logic inside the inner loop is necessarily complex (embedded vs registry, R2 up vs down, new recipe vs existing) but each branch has clear comments explaining the scenario. A developer debugging a skipped validation will find the right `::notice::` or `::warning::` annotation and understand why.

The config-file-driven batch size with `workflow_dispatch` override gives operators a knob to tune without touching YAML, and the 1-50 clamping prevents accidental misconfiguration. The fallback chain (override > config file > hardcoded default) is documented inline with a comment.

---

## Summary

| Severity | Count | Details |
|----------|-------|---------|
| Blocking | 0 | -- |
| Advisory | 3 | Duplicated batch-splitting logic across workflows; intentional per-recipe error annotation difference documented; minor variable casing inconsistency |

The implementation is clear and follows the pattern established by #1814. The batch-splitting duplication is the most notable maintainability concern, but it's acceptable for two instances. If the `validate-golden-execution.yml` batching (mentioned in the design doc as future work) copies this block a third time, extraction to a shared script should be required.
