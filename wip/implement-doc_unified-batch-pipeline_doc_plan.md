# Documentation Plan: unified-batch-pipeline

Generated from: docs/designs/DESIGN-unified-batch-pipeline.md
Issues analyzed: 3
Total entries: 2

---

## doc-1: docs/runbooks/batch-operations.md
**Section**: Batch Rollback
**Prerequisite issues**: #1741
**Update type**: modify
**Status**: updated
**Details**: Update batch ID format in examples from `2026-01-28-001` to date-only format (e.g., `2026-02-17`). Update expected output for `git log --grep` to show mixed-ecosystem commit messages instead of per-ecosystem ones (e.g., "add 3 homebrew, 5 cargo recipes" instead of "add homebrew recipes"). Adjust example recipe paths to include multiple ecosystems.

---

## doc-2: docs/runbooks/batch-operations.md
**Section**: Batch Success Rate Drop
**Prerequisite issues**: #1743
**Update type**: modify
**Status**: pending
**Details**: Update the dashboard investigation step (step 1) to mention that the health panel now shows per-ecosystem breakdown within each batch run and breaker-skip indicators. Operators can see which ecosystems were skipped due to open circuit breakers directly in the dashboard, without needing to query batch-control.json first.
