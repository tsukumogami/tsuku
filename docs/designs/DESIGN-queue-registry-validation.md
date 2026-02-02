---
status: Proposed
problem: |
  The batch pipeline hardcodes `--force` when installing generated recipes for validation, meaning any queue entry that duplicates an existing recipe will silently overwrite it. Seed-time filtering (#1267) prevents new duplicates from entering the queue, but entries already present before that change -- or intentionally re-queued by operators -- have no safety net.
decision: |
  Add an optional `force_override` boolean to the queue package schema, a CI validation script that cross-references pending entries against `recipes/`, and update the orchestrator to pass `--force` only when `force_override` is set. CI fails if any pending entry matches an existing recipe without `force_override: true`.
rationale: |
  A schema-level field makes intent explicit and auditable in version control. CI catches problems before the batch pipeline runs, which is cheaper than runtime failures. The orchestrator change ensures `--force` is never used blindly, closing the gap between queue data and pipeline behavior.
---

# Queue Registry Validation

## Status

Proposed

## Context and Problem Statement

The batch recipe generation pipeline processes entries from `data/priority-queue.json`, generates recipes, validates them using `tsuku install --force`, and creates PRs. The `--force` flag is hardcoded in the orchestrator (`internal/batch/orchestrator.go:232`), so any generated recipe that matches an existing one in `recipes/` will overwrite it without warning.

Issue #1267 added seed-time filtering via `FilterExistingRecipes`, which prevents new duplicates from entering the queue during `seed-queue` runs. But this doesn't help with entries already in the queue before that change, nor does it handle intentional re-queuing where an operator wants to regenerate a recipe (e.g., after upstream changes).

There's currently no mechanism to distinguish "this duplicate is accidental" from "I deliberately re-queued this package for regeneration." The `--force` flag is applied uniformly, making every batch run a potential overwrite risk.

## Decision Drivers

- Existing recipes must not be overwritten without explicit operator intent
- Re-generation intent should be visible and auditable in the queue data (not a runtime flag)
- The queue schema uses `additionalProperties: false`, so new fields require a schema change
- Validation should catch problems in CI before the batch pipeline runs
- The solution should work with the existing `validate-queue.sh` script pattern

## Considered Options

### Decision 1: Where to Validate

The core question is where to check for duplicates between queue entries and existing recipes. This check needs to happen before the batch pipeline overwrites anything, but the right layer affects both reliability and developer experience.

#### Chosen: CI Script Cross-Referencing

Add a shell script (`scripts/validate-queue-recipes.sh`) that reads `data/priority-queue.json`, iterates over `pending` entries, and checks whether a corresponding `.toml` file exists in `recipes/`. The script runs in the existing CI workflow alongside `validate-queue.sh`.

This catches duplicates at PR review time, before any batch pipeline run. It uses the same tooling pattern as existing validation scripts and doesn't require building Go code.

#### Alternatives Considered

**Orchestrator-only runtime check**: Check for existing recipes inside the orchestrator before calling `tsuku install`. Rejected because it catches problems too late -- the batch workflow has already started, and failing mid-run wastes CI time. It also doesn't give reviewers visibility into the problem during PR review.

**Go-based validation tool**: Build a `cmd/validate-queue-recipes` Go binary that does the cross-referencing. Rejected because this adds build complexity for what amounts to a `jq` + file existence check. Shell scripts are simpler for CI validation tasks and match the existing `validate-queue.sh` pattern.

**Remove `--force` entirely**: Delete the `--force` flag from the orchestrator and require operators to manually delete recipes before re-queuing. Rejected because it makes intentional regeneration a multi-step manual process (delete recipe, commit, re-queue, commit) and loses the ability to express regeneration intent atomically in the queue.

### Decision 2: How to Express Override Intent

When an operator intentionally re-queues a package, the system needs a way to distinguish this from an accidental duplicate. The signal must be auditable and survive across pipeline runs.

#### Chosen: Schema-Level `force_override` Field

Add an optional `force_override` boolean field to the package item schema in `data/schemas/priority-queue.schema.json`. When set to `true`, the CI validation script skips that entry, and the orchestrator passes `--force` to `tsuku install`. When absent or `false`, the entry is treated as a normal queue item that should not overwrite existing recipes.

This makes intent explicit in the queue data file, visible in git diffs, and auditable through PR review.

#### Alternatives Considered

**Environment variable at pipeline runtime**: Pass a `FORCE_PACKAGES=bat,fd` variable to the batch workflow. Rejected because it's not auditable in version control, can't be reviewed in PRs, and is easy to misconfigure.

**Separate override file**: Maintain a `data/force-overrides.json` alongside the queue. Rejected because it splits related data across files, creating synchronization risk. The intent belongs with the queue entry it modifies.

## Decision Outcome

**Chosen option: Schema field + CI validation + orchestrator update**

### Summary

Three changes work together to prevent accidental overwrites. First, the queue schema gains an optional `force_override` boolean on package items. Second, a CI validation script cross-references pending entries against `recipes/` and fails if any match exists without `force_override: true`. Third, the orchestrator reads `force_override` from each queue entry and only passes `--force` to `tsuku install` when it's set.

The CI check runs alongside existing schema validation in `validate-queue.sh`, catching problems during PR review. The orchestrator change ensures the pipeline itself respects the flag, even if CI is bypassed. Together, these close the gap between queue data and pipeline behavior.

Operators who want to regenerate a recipe set `force_override: true` on the specific queue entry and commit it. The intent is visible in the PR diff and can be reviewed before merging.

### Rationale

The three components reinforce each other: the schema field captures intent, CI enforces it before merge, and the orchestrator respects it at runtime. No single layer is sufficient alone -- CI can be bypassed, the orchestrator runs too late for review, and the schema field is meaningless without enforcement.

### Trade-offs Accepted

By choosing this option, we accept:
- A schema version bump is needed, and existing queue files need to remain valid (the field is optional, so they will)
- The CI script adds ~5 seconds to PR validation time
- Operators must edit the queue JSON directly to set `force_override` (there's no CLI for it yet)

These are acceptable because the schema remains backward compatible, the CI cost is negligible, and direct JSON editing matches the current operator workflow for queue management.

## Solution Architecture

### Overview

The solution adds three components that validate queue entries against existing recipes at two points: CI time (before merge) and runtime (during batch generation).

### Components

**Schema change** (`data/schemas/priority-queue.schema.json`): Add `force_override` as an optional boolean in the package item `properties` object (alongside `id`, `source`, `name`, etc.). Since `additionalProperties: false` is set on items, the field must be in `properties` for validation to pass. No schema version bump needed since the field is optional and existing files remain valid.

**CI validation script** (`scripts/validate-queue-recipes.sh`): Shell script that:
1. Reads `data/priority-queue.json` using `jq`
2. Filters to `pending` status entries
3. For each entry, constructs the expected recipe path: `recipes/{first_letter}/{name}.toml`
4. Checks if the file exists
5. If it exists and `force_override` is not `true`, records a failure
6. Exits non-zero if any failures found, printing each conflict as `CONFLICT: {name} -> {recipe_path}` and a summary count
7. Prints `OVERRIDE: {name}` for entries with `force_override: true` (informational, not a failure)

The script runs in a new `validate-queue-data.yml` workflow triggered on PRs that modify `data/priority-queue.json`. This workflow also runs the existing `validate-queue.sh` for schema validation. Only `pending` entries are checked -- other statuses (`success`, `failed`, `blocked`, `skipped`) are excluded since they won't be processed by the batch pipeline.

**Orchestrator update** (`internal/batch/orchestrator.go`): Read `force_override` from `seed.Package` and conditionally include `--force` in the `tsuku install` args. The `seed.Package` struct needs a `ForceOverride` field, and the queue loader needs to parse it.

### Key Interfaces

The `seed.Package` struct gains a field:

```go
type Package struct {
    ID       string `json:"id"`
    Source   string `json:"source"`
    Name     string `json:"name"`
    // ... existing fields ...
    ForceOverride bool `json:"force_override,omitempty"`
}
```

The orchestrator's `validate` method changes from hardcoded `--force` to conditional:

```go
args := []string{"install", "--json", "--recipe", recipePath}
if pkg.ForceOverride {
    args = append(args, "--force")
}
```

### Data Flow

1. Operator edits `data/priority-queue.json` (optionally setting `force_override`)
2. PR CI runs `validate-queue-recipes.sh` -- fails if pending entries match existing recipes without `force_override`
3. PR merges after review
4. Batch pipeline loads queue, orchestrator reads `force_override` per entry
5. Orchestrator passes `--force` only when `force_override: true`

## Implementation Approach

### Phase 1: Schema and Validation Script

- Add `force_override` to `data/schemas/priority-queue.schema.json`
- Add `ForceOverride` field to `seed.Package` struct
- Create `scripts/validate-queue-recipes.sh`
- Create `.github/workflows/validate-queue-data.yml` that runs both `validate-queue.sh` and `validate-queue-recipes.sh` on PRs modifying `data/priority-queue.json`

### Phase 2: Orchestrator Update

- Change `orchestrator.go` validate method to conditionally use `--force`
- Update orchestrator tests
- Depends on Phase 1 for the `ForceOverride` field on `seed.Package`

Both phases can be combined into a single PR since the changes are small and the vulnerability window (unconditional `--force` still active between Phase 1 and Phase 2) is unnecessary to maintain.

## Security Considerations

### Download Verification

This feature doesn't directly download artifacts, but it controls which recipes reach the batch pipeline -- and recipes specify download URLs. The key improvement is that removing unconditional `--force` means the orchestrator will respect existing recipe checksums by default. Only entries with explicit `force_override` bypass this, and those are visible in PR review.

### Execution Isolation

The CI validation script only reads files and performs existence checks -- no elevated permissions needed. The orchestrator change is a net improvement: it removes unconditional `--force`, reducing the scope of what auto-generated recipes can overwrite. Recipes with `force_override` still execute with the same permissions as before, but the intent is now explicit.

### Supply Chain Risks

The `force_override` field could be used to intentionally overwrite a curated recipe with an auto-generated one. This is mitigated by PR review: the `force_override: true` change is visible in the diff, and reviewers can verify the intent. The CI script also prints `OVERRIDE: {name}` for flagged entries, surfacing them in CI output.

A time-of-check/time-of-use gap exists between CI validation and batch pipeline execution -- if the queue is modified after CI passes but before the pipeline runs, the validation is stale. This is mitigated by the orchestrator's own `force_override` check at runtime, which provides a second layer of protection.

### User Data Exposure

**Not applicable** -- no user data is accessed or transmitted. The feature operates entirely on repository data files and queue metadata.

## Consequences

### Positive

- Existing recipes are protected from accidental overwrites
- Re-generation intent is explicit and auditable in version control
- CI catches problems before batch pipeline runs
- The orchestrator no longer blindly uses `--force`

### Negative

- Operators must manually edit JSON to set `force_override`
- The CI script adds a dependency on `jq` in the CI environment

### Mitigations

- `jq` is pre-installed on GitHub Actions runners, so no additional setup needed
- A future `seed-queue` CLI flag could automate setting `force_override` for specific packages
