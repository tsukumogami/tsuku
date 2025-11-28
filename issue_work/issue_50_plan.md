# Issue 50 Implementation Plan

## Summary

Add dorny/paths-filter to detect documentation-only changes and conditionally skip integration tests.

## Approach

Modify `.github/workflows/test.yml` to:
1. Add a `changes` job that uses paths-filter to detect if only docs changed
2. Merge the existing `matrix` job into `changes` (combine into one prerequisite job)
3. Add `if:` conditionals to integration jobs to skip when docs-only

### Why Merge Matrix into Changes

The `matrix` job already runs as a prerequisite for integration jobs. Rather than adding another prerequisite job, we can combine the paths-filter logic into the same job. This:
- Reduces workflow complexity
- Avoids adding another job to the workflow
- Keeps the prerequisite chain simple: changes -> integration-linux/macos

## Files to Modify

- `.github/workflows/test.yml` - Add paths-filter and conditionals

## Implementation Steps

- [ ] Add paths-filter action to detect code changes (not just docs)
- [ ] Add output for `code` filter (true when non-docs files changed)
- [ ] Merge filter logic into existing `matrix` job
- [ ] Add `if:` condition to `integration-linux` job
- [ ] Add `if:` condition to `integration-macos` job
- [ ] Add comments documenting skip patterns

## Skip Patterns

Files that are **documentation-only** (when changed exclusively, skip integration tests):
- `**/*.md`
- `docs/**`
- `.github/ISSUE_TEMPLATE/**`

We detect this by checking if `code` filter is `false` (meaning no non-docs files changed).

## Testing Strategy

- Manual verification: Create PR with only docs changes, verify integration tests are skipped
- Verify unit tests still run on docs-only PRs

## Success Criteria

- [ ] paths-filter detects code vs docs changes
- [ ] Integration tests skipped for docs-only PRs
- [ ] Unit tests always run
- [ ] Comments document skip patterns
