# Documentation Plan: registry-refresh-ttl-semantics

Generated from: docs/plans/PLAN-registry-refresh-ttl-semantics.md
Issues analyzed: 2
Total entries: 2

---

## doc-1: README.md
**Section**: Registry Management
**Prerequisite issues**: #1
**Update type**: modify
**Status**: updated
**Details**: Remove the `tsuku update-registry --all` example and its "Force refresh all cached recipes" comment. Plain `tsuku update-registry` now always force-refreshes all cached recipes, so the separate `--all` example is no longer accurate and should be dropped or its description updated to reflect the new default behavior.

---

## doc-2: plugins/tsuku-user/skills/tsuku-user/SKILL.md
**Section**: Core CLI Commands — Utilities table
**Prerequisite issues**: #1
**Update type**: modify
**Status**: updated
**Details**: Update the `tsuku update-registry` row in the Utilities table. The `--force` flag listed under Common Flags does not exist; the relevant flags are `--dry-run` and `--recipe`. Remove any reference to `--all` (which is deleted by Issue 1) and correct the Common Flags column to reflect the actual available flags after the change.
