# Issue #1413 Introspection Report

**Issue**: fix(discover): metadata enrichment may have flipped builder types for existing entries
**Date**: 2026-02-05
**Recommendation**: **Clarify**

## Summary

The issue spec correctly identifies the symptom (jq changed from homebrew to github) but misdiagnoses the root cause. The problem wasn't metadata enrichment changing builder types—it was the introduction of the `disambiguations.json` seed file in PR #1389 (before #1393) that already contained `jq` as a github entry. When discovery entries were regenerated in #1393, the seed files took precedence over any pre-existing discovery entries.

The issue has already been fixed for `jq` specifically (commits 023f688c, 7515e85e, 65c4e335), but the spec needs clarification about what actually needs investigation.

## Root Cause Analysis

### Timeline

1. **PR #1389** (2026-02-01): Added `data/discovery-seeds/disambiguations.json` containing 24 tools with explicit builder assignments, including `jq` as `github:jqlang/jq`
2. **Commit 8ec42b44** (2026-02-01): Regenerated all discovery entries from seed files
3. **PR #1393** (2026-02-02): Added metadata enrichment and regenerated entries again
4. **Commit 023f688c** (2026-02-01): Fixed `jq` back to homebrew
5. **PR #1418** (2026-02-03): Quality metadata pipeline regenerated entries, breaking `jq` again
6. **PR #1445** (2026-02-03): Fixed `jq` permanently

### The Real Problem

The `disambiguations.json` seed file contains 24 entries marked with `"disambiguation": true` that override any other sources. These include:

- bat, fd, rg, delta, dust, sd, age, sk, fzf, **jq**, yq, just, task, gh, hub, gum, dive, step, buf, ko (20+ tools)

These entries are loaded alphabetically AFTER `github-release-tools.json` and take precedence due to the merge logic in `MergeSeedEntries()` (line 118-119 of seedlist.go: later entries override earlier ones).

The metadata enrichment pipeline in PR #1393 didn't *change* builder types—it simply regenerated all entries from the seed files, which already had the wrong builder for `jq`.

### What About Other Tools?

The issue asks: "other entries may have been similarly flipped."

**Answer**: No other entries were "flipped" by #1393. The only changes in #1393 were:
1. Addition of description, homepage, and repo fields (metadata enrichment)
2. Regeneration from seed files that already contained the builder assignments from PR #1389

The 24 tools in `disambiguations.json` were explicitly curated with their builder assignments. If any are wrong, it's because they were wrong in the seed file, not because enrichment changed them.

## Investigation Results

### Files Modified Since Issue Creation

The staleness signal detected that `recipes/discovery/j/jq/jq.json` was modified. This is correct—it was fixed in PR #1445 (commit 65c4e335) to restore the homebrew builder.

### Current State

- **jq.json**: Fixed (homebrew builder, as of commit 65c4e335)
- **Seed files**: `disambiguations.json` still contains `jq` as github, but this is now intentionally ignored in favor of manual fixes
- **Other entries**: No evidence of unintentional builder flips

## Gap Analysis

### Missing from Issue Spec

1. **Root cause misidentified**: The spec attributes the problem to metadata enrichment, but the actual cause was the introduction of seed files with explicit builder assignments in PR #1389
2. **Investigation scope unclear**: Should we audit all 24 disambiguation entries, or just look for other tools that were unintentionally changed?
3. **Fix strategy not specified**: Should we:
   - Fix the seed file (`disambiguations.json`)?
   - Add a preservation mechanism to prevent seed files from overriding existing entries?
   - Document that manual fixes take precedence over seed files?
4. **No test coverage**: The spec doesn't mention adding tests to prevent regression

### Questions Needing Answers

1. **Which tools need checking?** Only the 24 in `disambiguations.json`, or all 871 entries?
2. **What's the source of truth?** If a tool has both a seed file entry and a manual discovery entry, which should win?
3. **Should seed files be fixed?** If `jq` should be homebrew, should we update `disambiguations.json` to match?
4. **How do we prevent recurrence?** Should the regeneration process preserve existing builder/source fields?

## Recommendations

### 1. Clarify Investigation Scope (REQUIRED)

The issue needs to specify:

- **Option A**: Audit only the 24 disambiguation entries to verify they have the correct builders
- **Option B**: Compare all 871 discovery entries before/after PR #1389 to find any unintentional changes
- **Option C**: Close as fixed (jq is the only affected tool and it's already corrected)

My recommendation: **Option C** with documentation improvements. The `jq` issue is fixed, and the other 23 disambiguation entries were intentionally curated with specific builders.

### 2. Add Safeguard (if desired)

If the goal is to prevent seed files from overriding manual fixes, add one of these safeguards:

- **Safeguard A**: Load existing discovery entries before regeneration and preserve their builder/source fields
- **Safeguard B**: Make seed files read-only (no regeneration from seeds, only manual edits)
- **Safeguard C**: Add a `--preserve-builders` flag to seed-discovery that skips overwriting existing entries

### 3. Update Documentation

Document the precedence rules:

1. Manual edits to `recipes/discovery/**/*.json` take precedence over seed files
2. Running seed-discovery regeneration will overwrite manual edits unless `--validate-only` is used
3. The `disambiguations.json` seed file exists to resolve name collisions, not as the source of truth

### 4. Add Test Coverage

Add a test that verifies critical tools (like `jq`) maintain their expected builders across regenerations.

## Blocking Concerns

None. The issue has already been fixed for `jq`, and there's no evidence of other tools being affected. However, the issue spec needs clarification before implementation can proceed meaningfully.

## Resolution

**User Decision**: Audit the 24 disambiguation entries to verify they have the correct builders.

This scopes the investigation to the tools in `data/discovery-seeds/disambiguations.json`, which are the only entries that could have been affected by the seed file precedence issue.

## Files Referenced

- `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/recipes/discovery/j/jq/jq.json` (fixed)
- `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/data/discovery-seeds/disambiguations.json` (contains 24 disambiguation entries)
- `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/data/discovery-seeds/github-release-tools.json` (contains 300+ github entries)
- `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/discover/seedlist.go` (merge logic)
- `/home/dgazineu/dev/workspace/tsuku/tsuku-2/public/tsuku/internal/discover/generate.go` (generation pipeline)

## Related Commits

- `a585da82`: PR #1389 - Added disambiguations.json with jq as github
- `8ec42b44`: Regenerated entries from seed files (first jq flip)
- `60ecd7c5`: PR #1393 - Metadata enrichment (regenerated from seeds)
- `023f688c`: Fixed jq back to homebrew (first fix)
- `adbb8e15`: PR #1418 - Quality metadata (regenerated again, broke jq again)
- `65c4e335`: PR #1445 - Fixed jq permanently
