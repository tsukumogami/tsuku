# Architecture Review: DESIGN-seed-queue-pipeline.md

## Review Metadata

- **Design Document**: `docs/designs/DESIGN-seed-queue-pipeline.md`
- **Status**: Proposed
- **Review Date**: 2026-01-30
- **Reviewer Role**: Architecture Analysis

## Executive Summary

The proposed architecture is clean, implementable, and well-scoped. The decision to add merge logic to the existing script (Option A) is sound. However, there are gaps in error handling specifications, component interface definitions, and edge case documentation. The implementation phases are correctly sequenced but could benefit from more explicit rollback criteria and failure scenarios.

## Review Questions

### 1. Is the architecture clear enough to implement?

**Overall: YES, with clarifications needed in 3 areas**

#### Clear Aspects

- **Script modification**: The `--merge` flag behavior is well-specified with pseudocode. An implementer knows exactly what to build.
- **Workflow structure**: The 5-step workflow (checkout, seed, validate, commit-if-changed, push-with-retry) is standard GitHub Actions fare. The commit-and-push pattern borrowed from `batch-operations.yml` is proven.
- **Data flow**: The diagram clearly shows the pipeline: Homebrew API → seed script → validation → git commit.

#### Needs Clarification

**A. Error Handling in --merge Logic**

The design says "reads `data/priority-queue.json` if it exists" but doesn't specify what happens if:

1. The file exists but is corrupt JSON
2. The file exists but fails schema validation
3. The file exists but has an unexpected schema version (future-proofing)

**Recommendation**: Add an error handling section to the script changes describing:
- Parse failures → treat as empty queue and proceed (log warning)
- Schema mismatches → fail fast with clear error message
- Missing required fields → fail fast

**B. Concurrent Access Window**

The design acknowledges "If the batch pipeline modifies `priority-queue.json` while the seed workflow runs, they could conflict" and dismisses it as "unlikely with `workflow_dispatch`" and "low-risk with daily cron since the batch pipeline runs at a different time."

This is handwavy. Questions:

1. What's the batch pipeline's schedule? (Not specified in this design)
2. What happens if both workflows commit simultaneously?
3. Does the git pull-rebase-push retry loop handle this?

**Recommendation**: Document the batch pipeline's schedule explicitly and verify the retry loop handles concurrent commits correctly. Consider adding a note about GitHub's branch protection rules if they exist.

**C. Schema Evolution Path**

The script writes `schema_version: 1` today. What happens when `priority-queue.schema.json` evolves to version 2?

The merge logic says "preserves all fields of existing entries," but if the schema changes:
- Old entries might lack new required fields
- The script needs migration logic or a way to mark entries as needing re-validation

**Recommendation**: Add a section on schema versioning. Options:
1. Reject merging if schema versions don't match
2. Auto-upgrade old entries (add default values for new fields)
3. Include a "last_validated_schema" field per entry

### 2. Are there missing components or interfaces?

**Overall: Mostly complete, with 2 notable gaps**

#### Well-Defined Components

- **seed-queue.sh**: Existing script with documented flags (`--source`, `--limit`, new `--merge`)
- **validate-queue.sh**: Existing validation script
- **Workflow**: Standard GitHub Actions YAML
- **Homebrew API**: Well-known public endpoint (`formulae.brew.sh`)

#### Missing Definitions

**A. Output File Structure for --merge**

The design shows pseudocode for merge logic but doesn't specify the full output format. Key questions:

1. What's the `tiers` object structure? (Mentioned in pseudocode but not defined)
2. What fields exist on a package entry? The design mentions `id`, `status`, `tier`, `added_at`, `metadata` but doesn't link to a schema reference.
3. How are packages ordered in the output? (Insertion order, by tier, alphabetical?)

**Actual impact**: The schema file exists (`data/schemas/priority-queue.schema.json`), but the design should reference it explicitly so implementers know where to look.

**Recommendation**: Add a "Data Format Reference" section pointing to the schema file and showing a minimal example of a merged queue entry.

**B. Workflow Inputs and Outputs**

The workflow has a `limit` input (number, default 100) mentioned in passing. Questions:

1. What's the maximum limit? (Homebrew has thousands of packages)
2. What happens if limit > total packages returned by the API?
3. Does the workflow output any artifacts for debugging? (e.g., API response, pre-merge queue, post-merge queue)

**Recommendation**: Document workflow inputs more formally:

```yaml
workflow_dispatch:
  inputs:
    limit:
      description: 'Max packages to fetch from Homebrew API'
      type: number
      required: false
      default: 100
      min: 1
      max: 10000
```

Add a step to upload pre/post-merge JSON as workflow artifacts for debugging failed runs.

### 3. Are the implementation phases correctly sequenced?

**Overall: YES, with suggestions for added rigor**

#### Correct Sequencing

The 5-step approach is solid:

1. Add `--merge` (isolated change, testable locally)
2. Create workflow (depends on step 1)
3. Initial seed run (proves steps 1-2 work)
4. Validate with batch pipeline (integration test)
5. Graduate to cron (final automation)

#### Suggested Improvements

**A. Add Rollback Criteria**

Each phase should specify "when to roll back" in addition to "how to proceed."

Examples:
- **Step 3 (Initial seed)**: If the committed file is >10MB or contains <10 packages, investigate before proceeding
- **Step 5 (Cron graduation)**: If any cron run fails twice in a row, disable the schedule until investigated

**Recommendation**: Add a "Rollback Triggers" subsection to each implementation step.

**B. Define "Successful Run" More Precisely**

The graduation criteria say "3 successful manual runs without issues." What constitutes success?

- Workflow status = green
- Output file validates against schema
- Output file changed (or didn't change, if Homebrew data was stale)
- Commit pushed successfully
- File size within expected range
- Package count within expected range

**Recommendation**: Add explicit success criteria to the graduation section.

**C. Testing the Retry Logic**

The commit-and-push retry loop is copy-pasted from `batch-operations.yml`. Has it been tested with this specific file (`priority-queue.json`)?

Potential edge case: If two workflow runs execute simultaneously (one cron, one manual), they could both pass validation but push conflicting commits. The retry loop should handle this, but it's worth testing.

**Recommendation**: Add a testing substep: "Trigger two workflow runs 10 seconds apart and verify both complete successfully without losing data."

### 4. Are there simpler alternatives we overlooked?

**Overall: The chosen option is the simplest that meets requirements**

#### Why Option A (chosen) is appropriate

- Reuses all existing script infrastructure
- Merge logic is ~30-40 lines (not excessive)
- Keeps script testable outside CI
- Allows full automation (no human bottleneck)

#### Alternative Not Considered: Separate Script for Merge

Instead of adding `--merge` to `seed-queue.sh`, create a separate `merge-queue.sh` that takes two inputs (existing queue, new queue) and outputs merged queue.

**Pros:**
- Separation of concerns (seed vs merge)
- Merge logic is independently testable
- Workflow calls two scripts: `seed-queue.sh --no-merge && merge-queue.sh`

**Cons:**
- More scripts to maintain
- Awkward interface (merge script needs two file arguments)
- Doesn't reduce complexity much (merge logic still needs to be written)

**Verdict**: Not worth the added complexity. Option A is simpler.

#### Alternative Not Considered: Use git merge Strategy

Instead of application-level merge logic, use git's built-in merge for `priority-queue.json`.

Workflow:
1. Create branch from latest main
2. Run `seed-queue.sh` (overwrite mode)
3. Merge branch back to main, resolving conflicts via custom merge driver

**Pros:**
- Leverages git's conflict resolution
- No application merge logic needed

**Cons:**
- JSON merge drivers are complex and error-prone
- Merge conflicts on a data file are tedious to resolve
- Doesn't handle semantic merging (e.g., preserving status fields)

**Verdict**: Way more complex than jq merge logic. Rejected.

#### Overlooked Simplification: Skip Merge, Use Separate Files

Instead of merging into one `priority-queue.json`, write separate files:
- `priority-queue-homebrew.json`
- `priority-queue-cargo.json` (future)
- `priority-queue-npm.json` (future)

Batch pipeline reads all of them.

**Pros:**
- No merge logic needed
- Each source is independent
- Easy to debug per-source issues

**Cons:**
- Batch pipeline needs to read multiple files
- Harder to deduplicate packages that exist in multiple ecosystems
- Doesn't match the schema design (single unified queue)

**Verdict**: Potentially simpler for the seed side, but adds complexity to the consumer (batch pipeline). Worth considering if merge logic becomes problematic, but not worth changing the design now.

## Architecture Gaps Summary

| Gap | Severity | Recommendation |
|-----|----------|----------------|
| Error handling for corrupt queue file | Medium | Add explicit failure modes to script spec |
| Concurrent access documentation | Medium | Document batch pipeline schedule + verify retry logic |
| Schema evolution path | Low | Add versioning strategy section |
| Output file structure reference | Low | Link to schema file + add example |
| Workflow input validation | Low | Document min/max limits formally |
| Rollback criteria | Medium | Add "when to roll back" to each phase |
| Success criteria definition | Medium | Define measurable success for graduation |
| Retry logic testing | Medium | Add test plan for concurrent runs |

## Overall Assessment

**Status: APPROVED with minor revisions recommended**

The architecture is sound and implementable. The identified gaps are documentation clarifications, not fundamental design flaws. The chosen option (A) is the right balance of simplicity and functionality.

### Strengths

1. **Reuses existing components**: No new dependencies, just extends `seed-queue.sh`
2. **Incremental rollout**: Manual trigger → validation → cron is low-risk
3. **Schema-first**: Validation before commit prevents bad data
4. **Clear data flow**: Single-direction pipeline with no circular dependencies

### Weaknesses

1. **Error handling underspecified**: Several "what if" scenarios lack documented behavior
2. **Concurrent access handwaved**: Needs tighter specification
3. **Testing plan minimal**: Graduate to cron after "3 successful runs" is vague

### Recommendation

Proceed with implementation after adding:
1. Error handling section (corrupt file, schema mismatch)
2. Concurrent access mitigation details
3. Explicit success/rollback criteria for each phase

These are documentation additions, not code changes. The core architecture is solid.
