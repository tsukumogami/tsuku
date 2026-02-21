# Architecture Review: Issue #1816

**Issue**: #1816 docs(ci): document batch size configuration and tuning
**Review focus**: architect
**Date**: 2026-02-21

---

## Scope of Changes

Files changed:
- `docs/workflow-validation-guide.md` -- new "CI Batch Configuration" section appended (lines 149-219)
- `CONTRIBUTING.md` -- one paragraph added after CI workflow table (line 614)

This is a documentation-only issue. No workflow YAML, no Go code, no config file changes.

---

## Design Alignment

### Documentation placement

The batch configuration documentation is appended to `docs/workflow-validation-guide.md`. This file already documents the `validate-all-recipes.yml` workflow (platform validation, auto-constraining). Adding batch configuration here is a reasonable fit since the file covers CI workflow operational guidance, though the original scope was limited to the platform validation workflow specifically.

The design doc's Phase 3 says "Document the batch size parameter and how to tune it" without prescribing a location. The implementation chose to extend an existing operational guide rather than create a new standalone document. This follows the codebase's existing pattern: `docs/` contains focused operational guides (e.g., `r2-golden-storage-runbook.md`, `GUIDE-recipe-verification.md`), and CI workflow guidance clusters in one place rather than being split per-workflow.

No architectural concern.

### Cross-reference pattern

The CONTRIBUTING.md addition follows the existing pattern in that file: the CI Validation Workflows table describes what each workflow does, and the new paragraph below it describes the batching behavior that affects all recipe CI workflows. The cross-reference to `docs/workflow-validation-guide.md#ci-batch-configuration` uses a relative link with an anchor, consistent with how CONTRIBUTING.md links to other docs (e.g., `docs/EMBEDDED_RECIPES.md`, `docs/DESIGN-golden-plan-testing.md`).

No architectural concern.

### Configuration documentation matches implementation

The guide's JSON snippet for `.github/ci-batch-config.json` accurately reflects the actual file contents:
- `test-changed-recipes.linux: 15`
- `validate-golden-recipes.default: 20`

The workflows confirm they read from these exact keys with the correct fallback defaults. The documentation does not fabricate config entries that don't exist (the design doc's aspirational `validate-golden-execution` entries are correctly omitted).

No architectural concern.

### Consistency with existing batching patterns

My memory notes document three CI batching patterns:
1. **macOS aggregated pattern** -- single job, all recipes
2. **Per-recipe matrix pattern** -- one job per recipe (being replaced)
3. **Container-family batching** -- one job per Linux family

The documentation correctly describes the new fourth pattern (ceiling-division batching) without conflating it with the existing patterns. The "How Batching Works" subsection explains the ceiling-division mechanics. The "Follow-Up" subsection correctly notes that `validate-golden-execution.yml` still uses per-recipe matrix jobs, maintaining awareness of the coexisting patterns.

No architectural concern.

---

## Findings

### Advisory: Documentation section appended to a tangentially related guide

**File**: `docs/workflow-validation-guide.md`, lines 149-219

The "CI Batch Configuration" section is appended to a guide whose title and existing content are about the `validate-all-recipes.yml` workflow (platform validation and auto-constraining). Batch configuration applies to `test-changed-recipes.yml` and `validate-golden-recipes.yml` -- different workflows entirely. A reader looking for batch tuning guidance would need to know to look in the "Recipe Validation Workflow Guide."

This does not compound: the CONTRIBUTING.md cross-reference provides discoverability, and the guide's file name (`workflow-validation-guide.md`) is generic enough to plausibly cover multiple workflow topics. If CI documentation grows further, splitting into per-workflow guides or renaming this file would be straightforward and wouldn't require touching code.

**Severity**: Advisory. The placement works and the cross-reference makes it findable, but the section is a thematic outlier in the current document.

---

## Summary

No blocking findings. The documentation changes fit the existing documentation architecture: operational CI guidance in `docs/`, contributor-facing overview in `CONTRIBUTING.md`, cross-references between them. The content accurately reflects the implemented batching infrastructure from #1814 and #1815 without introducing misleading descriptions or phantom configuration.

The one advisory note is about documentation organization -- the batch config section lives in a guide whose original scope was platform validation. This is contained and easily reorganized later if the docs grow.
