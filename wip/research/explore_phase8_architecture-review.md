# Architecture Review: Phase 8

## Summary

The design is structurally sound. It identifies two proven patterns (container loops, GHA group serialization) already present in the codebase and proposes extending them to other workflows. The decisions are well-justified, the trade-off analysis holds, and the incremental migration strategy is correct. No new infrastructure is introduced; this is purely workflow refactoring.

However, the job count arithmetic has several internal inconsistencies, and the implementation phases are incomplete -- three consolidations that appear in the "After" topology are missing from the phase list. These need to be corrected before implementation begins, or the phase PRs won't deliver the claimed totals.

## Findings

### 1. Phase 1 savings sum to 28, not 27 (Advisory)

The Phase 1 heading says "save 27 jobs" but the four items sum to 28:

- build-essentials.yml test-sandbox-multifamily: 10 -> 2 (save 8)
- test.yml integration-linux: 9 -> 1 (save 8)
- sandbox-tests.yml sandbox-linux: 9 -> 1 (save 8)
- integration-tests.yml checksum-pinning: 5 -> 1 (save 4)

Total: 8 + 8 + 8 + 4 = 28.

This is a documentation error. The actual plan is fine.

### 2. test.yml "Before" count of 18 is off (Advisory)

The summary table claims test.yml has 18 jobs before consolidation. Counting the actual jobs that fire on a worst-case PR (touching Go, recipes, Rust, and test scripts -- which is the stated worst case):

- matrix (1)
- check-artifacts (1)
- lint-workflows (1)
- unit-tests (1)
- lint-tests (1)
- functional-tests (1)
- rust-test (2)
- validate-recipes (1)
- integration-linux (9)
- integration-macos (1)

That's 19. Adding llm-integration and llm-quality (which require LLM file changes, outside the stated worst case) would bring it to 21. The design's topology diagram in the "After" section accounts for 8 of these (matrix + unit-tests + lint-tests + functional-tests + rust-test(2) + validate-recipes + integration-linux + integration-macos = 10), implying check-artifacts, lint-workflows, and the LLM jobs are excluded from the count. If we exclude check-artifacts and lint-workflows (non-test housekeeping), the "Before" count is 17, not 18. If we include lint-workflows but not check-artifacts (or vice versa), we get 18.

The counting convention for which jobs are included in the totals needs to be stated explicitly. The summary table, the topology diagram, and the actual workflow file need to agree.

### 3. Three consolidations missing from implementation phases (Blocking for plan completeness)

The "After" topology for integration-tests.yml shows 6 jobs, down from 20. The phases explicitly cover:

| Phase | Consolidation | Savings |
|-------|--------------|---------|
| Phase 1 | checksum-pinning: 5 -> 1 | 4 |
| Phase 2 | homebrew-linux: 4 -> 1 | 3 |
| Phase 2 | dlopen tests: 6 -> 2 | 4 |

That accounts for 11 savings. But 20 -> 6 = 14 savings. The missing 3 are:

- **library-integrity**: 2 -> 1 (save 1). Currently a 2x1 matrix (zlib, libyaml on debian). The topology shows it as a single job.
- **library-dlopen-musl**: 3 -> 1 (save 2). Currently 3 library-specific jobs, each using `container: image: golang:1.23-alpine`. The topology shows it as 1 job with the comment "already containerized."

These need to be added to Phase 2, or the topology diagram needs to be corrected to show 9 jobs instead of 6.

### 4. library-dlopen-musl consolidation mechanism is unclear (Advisory)

The current musl dlopen jobs use GHA's `container:` directive (`image: golang:1.23-alpine`), which runs the entire job inside a container. This is architecturally different from the "container loop" pattern where a bare ubuntu-latest runner iterates through Docker images via `docker run`.

Consolidating the 3 musl library tests into 1 job means serializing them inside a single `golang:1.23-alpine` container runner. This works fine, but the implementation pattern is different from the other consolidations. The musl job needs to:
1. Keep using `container: image: golang:1.23-alpine` (single container)
2. Install Rust once
3. Loop through [zlib, libyaml, gcc-libs] with GHA groups

This is actually the GHA group serialization pattern (Decision 2), not the container loop pattern (Decision 1), but applied to a container-based runner. The design's comment "already containerized" on this job is misleading -- it implies no change is needed, but the job still needs to go from 3 matrix entries to 1 serialized job.

### 5. Phase savings don't sum to total claimed (Advisory)

Phase 1 claims 27 (actually 28) + Phase 2 claims 10 = 37-38. But the summary claims 41 total saved. The gap of 3 is the three missing consolidations from Finding 3 above. Once those are added to the phases, the arithmetic resolves.

### 6. sandbox-tests.yml has no macOS equivalent (non-issue, just noting)

The design's "After" topology shows sandbox-tests.yml as `sandbox-linux (1) + sandbox-macos (1)`. Looking at the actual workflow file at `/home/dangazineu/dev/workspace/tsuku/tsuku-5/public/tsuku/.github/workflows/sandbox-tests.yml`, there is no macOS sandbox job -- only `sandbox-tests` with the Linux matrix. The "sandbox-macos (1)" in the topology might be referring to a planned addition or a misread.

Checking the current file: only `matrix` (1) and `sandbox-tests` (9 from Linux matrix) = 10 total. The design says "Before: 10, After: 2, Saved: 8." If After is 2, that's matrix(1) + sandbox-linux(1) = 2. But the topology says "sandbox-linux (1) + sandbox-macos (1)" which would be 3 including matrix. The comment "# already 1" next to sandbox-macos suggests an existing job, but it doesn't exist in the current file.

This is either a misread of the workflow or it refers to a planned change. The Before/After table (10 -> 2, save 8) is internally consistent with just removing the Linux matrix expansion, so the impact estimate is correct regardless.

### 7. Reusable workflow rejection is well-reasoned but drift risk remains (Advisory)

The design correctly rejects extracting the container loop into a reusable workflow due to GHA's poor multi-line input handling. The proposed mitigation is "documenting the canonical pattern and adding CI checks that validate loop structure in new workflows."

This mitigation is referenced but not specified. What CI check validates loop structure? Grep for specific bash patterns? A linter? This should be described at least at the level of "a shellcheck/grep-based check that verifies container loops include timeout, exit code capture, and failure array" -- or explicitly deferred to a follow-up issue.

Without this check, the design's own earlier observation holds: "copying ~30 lines into 6+ files creates drift risk." The drift risk is real but contained to CI workflow files (not application code), so it doesn't compound into the codebase architecture. This is advisory, not blocking.

### 8. Wall-time trade-off analysis holds (Validated)

The design's core argument is sound: for tests with 30-50 second execution times, queue wait (7-11 minutes measured) dominates execution time. Serialization eliminates 8 queue slots, and the resulting sequential execution (~5-8 minutes) is comparable to or less than the current queue-dominated wall time (~12-13 minutes).

The one scenario where this breaks down is if multiple consolidated workflows run simultaneously and all compete for the same runner pool. But this is the same pool they compete for today, just with fewer entries.

## Recommendations

1. **Fix the Phase 1 sum**: Change "save 27 jobs" to "save 28 jobs" in the Phase 1 heading.

2. **Add the three missing consolidations to Phase 2**: library-integrity (2->1), library-dlopen-musl (3->1). Add items 8 and 9 to Phase 2 (renumbering Phase 3 accordingly). This brings Phase 2 from "save 10 jobs" to "save 13 jobs" and makes the phases sum to 28+13=41.

3. **Clarify the test.yml job counting convention**: State whether check-artifacts, lint-workflows, and LLM jobs are included in the count. Adjust the Before/After numbers accordingly so the table is reproducible by counting the workflow file.

4. **Correct the sandbox-tests.yml topology**: Either remove "sandbox-macos (1)" from the diagram or note that it doesn't currently exist.

5. **Specify the drift-prevention mechanism for container loops**: Even a one-sentence description ("A CI check will grep workflow files for container loops that lack `timeout` or failure array patterns") would make the mitigation concrete.

None of these are structural blockers in the architecture-reviewer sense (no codebase patterns are being violated, no contracts broken, no dependency inversions). The design introduces no new infrastructure and extends existing proven patterns. The findings are all in the "plan completeness" category: fix the arithmetic, fill the gaps, and the design is ready to implement.
