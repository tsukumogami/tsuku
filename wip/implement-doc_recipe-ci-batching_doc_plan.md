# Documentation Plan: recipe-ci-batching

Generated from: docs/designs/DESIGN-recipe-ci-batching.md
Issues analyzed: 3
Total entries: 2

---

## doc-1: docs/workflow-validation-guide.md
**Section**: (new section) CI Batch Configuration
**Prerequisite issues**: #1814, #1815, #1816
**Update type**: modify
**Status**: pending
**Details**: Add a new section covering recipe CI batching: what batch sizes control, where they're configured (`.github/ci-batch-config.json`), how to use the `batch_size_override` input on `workflow_dispatch` for experimentation, guidelines for increasing or decreasing batch sizes based on job duration, the valid range (1-50) enforced by the guard clause, and a note about follow-up work for `validate-golden-execution.yml` per-recipe jobs that still need batching.

---

## doc-2: CONTRIBUTING.md
**Section**: CI Validation Workflows
**Prerequisite issues**: #1814, #1815, #1816
**Update type**: modify
**Status**: pending
**Details**: Add a brief note in or near the "CI Validation Workflows" table (inside the "Golden File Testing" section) explaining that recipe CI uses batched jobs -- each CI check may cover multiple recipes rather than one per job. This helps contributors understand why a single check name covers multiple recipes and where to find per-recipe results (expand the job's `::group::` annotations in the Actions log).
