# Issue 1413 Implementation Plan

## Summary
Audit the 20 tools in `data/discovery-seeds/disambiguations.json` by comparing their builders against existing TOML recipes, then correct any mismatches to ensure discovery entries route users to the appropriate deterministic builders when available.

## Approach
The root cause was the disambiguations.json seed file (PR #1389), not metadata enrichment. When seed files were introduced, some tools were assigned `github` builder despite having existing recipes that use `homebrew` builder. The `github` builder requires LLM for recipe generation, which fails under `--deterministic-only`, while `homebrew` builder can generate recipes deterministically.

This approach audits each of the 20 disambiguation tools by:
1. Checking if a TOML recipe exists
2. Determining the recipe's builder action (homebrew, github_archive, etc.)
3. Comparing against the disambiguations.json entry
4. Correcting mismatches where deterministic builders are available

### Alternatives Considered
- **Fix after generation**: Manually correct discovery entries after regeneration, but this doesn't prevent the issue from recurring
- **Remove disambiguations.json**: This would lose the disambiguation metadata, making name collision resolution harder
- **Ignore the issue**: Some tools would continue to fail under `--deterministic-only` when they could work with the right builder

## Files to Modify
- `data/discovery-seeds/disambiguations.json` - Correct builder assignments for tools with existing recipes

## Implementation Steps
- [ ] Create audit script to compare disambiguations.json against existing recipes
- [ ] Run audit to identify all mismatched builders
- [ ] Document findings showing current vs. correct builder for each tool
- [ ] Update disambiguations.json with correct builders for tools that have deterministic recipes
- [ ] Verify no tools were inadvertently changed
- [ ] Add comment explaining builder selection criteria for future maintainers

## Testing Strategy
- Unit tests: No new tests needed; existing seed loading tests cover the format
- Integration tests: Verify that corrected tools can be discovered and installed
- Manual verification:
  - Run `tsuku create <tool> --deterministic-only` for tools changed from github to homebrew
  - Verify recipe generation succeeds without LLM
  - Check that discovery lookups return the correct builder

## Risks and Mitigations
- **Risk**: Changing builders for tools that intentionally use github builder
  - **Mitigation**: Only change tools where a recipe exists with a deterministic builder action (homebrew, github_archive with direct releases)
- **Risk**: Breaking existing installations or discovery lookups
  - **Mitigation**: Test discovery lookups before and after to ensure same results
- **Risk**: Future regenerations overwriting manual fixes again
  - **Mitigation**: Add comment in disambiguations.json explaining the criteria so future changes are intentional

## Success Criteria
- [ ] All 20 disambiguation tools audited
- [ ] Builder mismatches identified and documented
- [ ] Tools with homebrew recipes updated to use homebrew builder
- [ ] Tools can be installed with `--deterministic-only` when appropriate
- [ ] No functional test regressions

## Open Questions
None - the scope is well-defined and the approach is straightforward.
