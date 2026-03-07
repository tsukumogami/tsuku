# Issue 2095 Summary

## Changes
Removed redundant version source fields from 941 recipes:
- 922 homebrew recipes: cleared `source="homebrew"` and `formula` in [version]
- 18 cargo recipes: cleared `source="crates_io"` in [version]
- 1 github recipe: cleared `github_repo` in [version]

## Approach
Used Python script to parse TOML, identify redundancies, then apply targeted
line-level edits to preserve formatting. Only modified fields within [version]
sections, leaving [[steps]] untouched.

## Validation
- All 40 test packages pass
- Strict validation: 1399 recipes checked, 3 pre-existing failures remain
  (cargo-nextest, chamber, libevent -- unrelated issues)
- Down from 943+ failures before the fix

## Pre-existing failures (not in scope)
- cargo-nextest: missing upstream verification warnings
- chamber: invalid platform constraints
- libevent: step references unsupported platform
