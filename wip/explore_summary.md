# Exploration Summary: System Library Backfill Strategy

## Problem (Phase 1)

Tools in the batch generation queue get blocked when they depend on system libraries tsuku doesn't yet provide. The reactive loop (fail, block, create library, auto-requeue) works end-to-end with 0 current blockers, but 2,830 pending entries will discover new missing libraries one batch cycle at a time without proactive action. Library recipes also lack a cross-platform testing workflow.

## Decision Drivers (Phase 1)

- Maximize tools unblocked per library added (ROI)
- Follow established library recipe patterns
- Integrate with existing failure analysis and dashboard infrastructure
- Cover all ecosystems, not just Homebrew
- Library recipes must be tested on darwin and arm64 before merge
- Platform gaps surface naturally through the reactive loop

## Research Findings (Phase 2)

- 22 library recipes exist; only 3 declare satisfies metadata
- dep-mapping.json replaced by per-recipe satisfies metadata (PR #1824)
- Requeue-on-recipe-merge automated (PR #1830), 0 blocked entries remain
- Structured error subcategories (PR #1854) improve failure classification
- 14 known blocker libraries identified from historical failure data
- batch-generate.yml has full platform matrix but no single-recipe mode
- tsuku install has --sandbox and --target-family flags for local testing

## Options (Phase 3)

7 decisions made:
1. Proactive + reactive (not reactive-only)
2. Batch orchestrator execution for discovery (not API analysis)
3. Standard tsuku create + friction log (not dedicated generator)
4. Follow dependency chains leaf-first (no depth ceiling)
5. Match blocking platform + reactive feedback (not all-platforms-required)
6. Queue progression metrics (not coverage ratio)
7. New test-recipe GHA workflow (not adapting batch-generate)

## Decision (Phase 5)

**Problem:**
The batch pipeline has 2,830 pending entries that will discover missing library dependencies one batch cycle at a time. Library recipes created on linux-amd64 need cross-platform testing before merge, and no lightweight workflow exists for this.

**Decision:**
Run the batch orchestrator against pending entries to discover needed libraries, create recipes using standard tsuku create with manual fixes where needed, and validate across all platforms via a new test-recipe GHA workflow before merge. Platform failures lead to when filters, not blocked merges. Each manual fix is logged as a pipeline enhancement.

**Rationale:**
Proactive discovery avoids per-library round trips through failure. The friction log turns manual fixes into pipeline improvements. Cross-platform CI catches platform issues early. The test-recipe workflow is a blocking prerequisite that must merge to main before library recipe work begins.

## Current Status
**Phase:** 8 - Final Review (reviews complete, feedback incorporated)
**Last Updated:** 2026-02-22
