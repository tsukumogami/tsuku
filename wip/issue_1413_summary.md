# Issue 1413 Summary

## What Was Implemented

Audited the 20 disambiguation entries in `data/discovery-seeds/disambiguations.json` and corrected builder assignments for 4 tools that have existing homebrew recipes. These tools were incorrectly marked with `github` builder, causing them to fail under `--deterministic-only` mode.

## Changes Made

- `data/discovery-seeds/disambiguations.json`: Updated builder and source for 4 tools:
  - `dust`: Changed from `github:bootandy/dust` to `homebrew:dust`
  - `fd`: Changed from `github:sharkdp/fd` to `homebrew:fd`
  - `task`: Changed from `github:go-task/task` to `homebrew:go-task`
  - `yq`: Changed from `github:mikefarah/yq` to `homebrew:yq`
  - Added `_comment` field explaining builder selection criteria
- `scripts/audit-disambiguation-builders.sh`: Created audit script to compare disambiguation entries against existing recipes

## Key Decisions

- **Scope reduction**: The introspection phase revealed that the root cause was the `disambiguations.json` seed file (PR #1389), not the metadata enrichment (PR #1393). Focused the audit on the 24 disambiguation entries instead of all 849 discovery entries.
- **Builder selection**: Tools with existing `homebrew` action recipes should use `homebrew` builder since it's deterministic and doesn't require LLM, unlike the `github` builder.
- **Preserved github_archive tools**: Tools using `github_archive` action (age, fzf, gum, just) correctly use `github` builder since github_archive is also deterministic.

## Trade-offs Accepted

- **No safeguard implementation**: Did not implement automatic preservation of builder/source fields during regeneration. This was addressed by fixing the seed file itself and adding documentation via the `_comment` field.
- **Manual audit vs automated**: Created a simple bash audit script instead of a more complex Go tool, prioritizing implementation speed and maintainability.

## Test Coverage

- No new tests added (existing seed loading tests cover the format)
- All discover package tests pass (0.391s runtime)
- Verified changes with audit script showing 0 mismatches after corrections

## Known Limitations

- The audit script relies on convention (looking for `recipes/{first-letter}/{tool}.toml`)
- Future regenerations from seed files could reintroduce issues if the seed file is edited incorrectly
- The `_comment` field in JSON serves as documentation but isn't enforced by tooling

## Future Improvements

- Add test coverage to verify critical tools maintain expected builders across regenerations
- Consider implementing a `--preserve-builders` flag for seed-discovery command
- Document the precedence rules between seed files and manual discovery entry edits
