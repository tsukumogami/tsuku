---
title: "Maintainer Review - Issue 2 (source tracking)"
phase: review
role: maintainer-reviewer
---

# Maintainer Review - Issue 2: feat(state): add source tracking to ToolState

## Overall Assessment

The implementation is well-structured. The migration is clean, idempotent, and well-tested. The `recipeSourceFromProvider` mapping function is documented with clear rationale. A few findings below, one blocking.

## Findings

### 1. Two value spaces with confusable names (Blocking)

`Plan.RecipeSource` stores raw provider tags (`"registry"`, `"embedded"`, `"local"`). `ToolState.Source` stores normalized tags (`"central"`, `"local"`, `"owner/repo"`). The field names are similar enough that the next developer will assume they hold the same values.

The mapping between them happens in two places:
- `cmd/tsuku/helpers.go:159` (`recipeSourceFromProvider`) -- for new installs
- `internal/install/state_tool.go:133` (`migrateSourceTracking`) -- for migration

Neither `Plan.RecipeSource` nor `ToolState.Source` has a comment explaining which value space it belongs to. The `Plan` struct field at `state.go:35` has no comment at all. The next developer who sees `Plan.RecipeSource = "registry"` and `ToolState.Source = "central"` will wonder why they differ, and may "fix" one to match the other.

**Fix:** Add a one-line comment to `Plan.RecipeSource` explaining its value space: `// RecipeSource is the raw provider tag ("registry", "embedded", "local"); see recipeSourceFromProvider for the mapping to ToolState.Source values.`

### 2. Magic strings for source values (Advisory)

The string `"central"` appears in 13 `.go` files (production + test), and `"local"` appears similarly. There are named constants for the *input* side (`recipe.SourceRegistry`, `recipe.SourceEmbedded`, `recipe.SourceLocal`) but none for the *output* side. A typo in any of the `"central"` literals -- say `"Central"` or `"centra"` -- would silently break the migration or comparison logic with no compile-time catch.

This is advisory rather than blocking because the test coverage is thorough enough to catch most typos, and the value set is small. But as Issue 3+ adds code that switches on these values (e.g., `GetFromSource`), the risk grows.

**Fix:** Define constants like `SourceCentral = "central"` alongside the `ToolState` type.

### 3. ToolState.Source not set inline during install (Advisory)

`InstallWithOptions` never sets `ToolState.Source`. New installs rely on the lazy migration in `Load()` to backfill from `Plan.RecipeSource`. Within the same process, between `Save()` and the next `Load()`, `ToolState.Source` is empty.

No current code reads `Source` in the install path, so this doesn't cause a bug today. But the `ToolState` comment at `state.go:85` says "Populated on new installs" -- the next developer will expect it to be set during install, not lazily after reload. If future code (e.g., post-install telemetry or a "just installed from X" message) reads `Source` before reloading state, it'll get an empty string.

**Fix:** Either set `ts.Source` in the install `UpdateTool` callback alongside `ts.IsExplicit`, or update the comment to say "Populated lazily on Load() via migration."

### 4. Scrutiny file has a factual error (Advisory)

`wip/research/implement-doc_scrutiny_justification_issue2.md:39` says: "Migration (`state_tool.go:145`) maps Plan.RecipeSource 'embedded' -> Source 'embedded'." The actual code at `state_tool.go:144-148` only has a case for `"local"`; everything else (including `"embedded"`) defaults to `"central"`. The scrutiny's Finding 1 is based on this incorrect reading, claiming an inconsistency that doesn't exist. The migration and `recipeSourceFromProvider` are actually consistent -- both map embedded to central.

Not blocking since the code is correct, but the scrutiny artifact is misleading if a future developer reads it for context.

## What's Clear

- The migration function is easy to follow: skip if already set, check plan, default to central. Good.
- Test names accurately describe what they verify. `TestMigrateSourceTracking_InfersFromPlan` with table-driven subtests covers the full mapping matrix.
- The `recipeSourceFromProvider` comment explains *why* embedded maps to central, not just *that* it does. This is the kind of comment that saves the next developer from "fixing" it.
- Idempotency test is a smart inclusion for migration code.
