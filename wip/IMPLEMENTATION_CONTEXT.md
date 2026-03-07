## Goal

Fix recipe validation failures in nightly scheduled CI runs caused by redundant homebrew version fields.

## Context

The `test.yml` workflow runs nightly via cron (`0 0 * * *`). Since March 4th, the Validate Recipes job fails on `main` because 943 recipes specify `source="homebrew"` with an explicit `formula` field, which strict-mode validation now flags as redundant (the homebrew action infers the formula automatically).

Example warning:
```
version: [version] source="homebrew" with formula="aerc" is redundant;
homebrew action infers this automatically
```

This causes the README test badge to show red even though PR-triggered runs pass fine.

## Acceptance Criteria

- [ ] Nightly scheduled CI recipe validation job on `main` passes
- [ ] Either the redundant fields are removed from affected recipes, or the validation strictness is adjusted appropriately
- [ ] No recipes lose correct version resolution behavior

## Dependencies

None
