# Issue 58 Implementation Plan

## Summary

Fix the `dorny/paths-filter` configuration by adding `predicate-quantifier: 'every'` to make negation patterns work correctly.

## Approach

Add `predicate-quantifier: 'every'` to the paths-filter step. This changes the logic from OR (any pattern matches) to AND (all patterns must match), allowing negation patterns to properly exclude files.

Current config (broken):
```yaml
filters: |
  code:
    - '**'
    - '!**/*.md'
    - '!docs/**'
```
With default `some` mode, `**` matches everything so `code` is always true.

Fixed config:
```yaml
predicate-quantifier: 'every'
filters: |
  code:
    - '**'
    - '!**/*.md'
    - '!docs/**'
```
With `every` mode, a file must match ALL patterns. A `.md` file matches `**` but fails `!**/*.md`, so it's excluded.

### Alternatives Considered
- Positive patterns only (e.g., `**/*.go`, `**/*.yaml`): More verbose, needs maintenance when new file types added
- Separate workflow for docs: Over-engineering for this use case

## Files to Modify
- `.github/workflows/test.yml` - Add predicate-quantifier option to paths-filter step

## Files to Create
None

## Implementation Steps
- [x] Add `predicate-quantifier: 'every'` to the dorny/paths-filter step

## Testing Strategy
- The PR itself only modifies a .yml file (workflow code)
- If filtering works correctly, integration tests should be SKIPPED for this PR
- This is a self-validating test: if integration tests run, the fix didn't work

## Risks and Mitigations
- Risk: Might incorrectly skip tests for actual code changes
- Mitigation: Keep `**` as first pattern to match all files, only exclude specific doc patterns

## Success Criteria
- [ ] PR for this fix triggers Unit Tests only (integration tests skipped)
- [ ] Subsequent code PRs still trigger all tests

## Open Questions
None
