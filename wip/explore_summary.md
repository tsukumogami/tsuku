# Exploration Summary: CI Build Essentials Consolidation

## Problem (Phase 1)
The Build Essentials workflow runs 7 separate Linux jobs (libsixel, ninja, sqlite, git, tls-cacerts, zig, homebrew tools) that each allocate their own runner, repeat checkout+Go+build setup, and compete for queue slots. The same workflow already aggregates macOS tests into 2 jobs using GHA groups, proving the pattern works. Applying it to Linux would save 6 runners per trigger.

## Decision Drivers (Phase 1)
- Queue pressure: 7 jobs compete for slots when 1 would suffice
- Proven pattern: macOS arm64 and Intel jobs already serialize 8 tests each
- Setup waste: ~1-2 min of checkout+Go+build repeated 7 times
- Failure isolation: individual jobs give per-tool red/green signals
- Build time variation: git-source takes ~4min, libsixel ~2min, homebrew ~48s

## Research Findings (Phase 2)
- The original CI job consolidation design (DESIGN-ci-job-consolidation.md, now Current) intended for build-essentials Linux tool tests to remain as separate jobs
- macOS already demonstrates the `run_test()` pattern with GHA groups and shared download cache
- sandbox-multifamily already uses container loops (different pattern, not applicable here)
- The No-GCC container test requires a special container environment and can't be serialized with the others
- git-source requires `sudo apt-get install gettext` as a special setup step

## Options (Phase 3)
- Option A: Serialize all 7 Linux tests into 1 job with GHA groups (mirrors macOS pattern)
- Option B: Serialize into 2 groups based on build duration
- Option C: Keep separate jobs but share a pre-built binary via artifacts

## Decision (Phase 5)

**Problem:**
The Build Essentials workflow allocates 7 separate Linux runners for tool tests that share identical setup (checkout, Go install, binary build). Each runner spends 1-2 minutes on this setup before running a test that takes 1-5 minutes. The queue pressure from 7 concurrent jobs delays all of them. The same workflow already proves the fix works: its macOS jobs aggregate 8 tests into a single runner with GHA groups.

**Decision:**
Consolidate the 7 Linux tool-test jobs into a single aggregated job using the same GHA group serialization pattern that macOS already uses. Each test gets its own `::group::` section, fresh `$TSUKU_HOME`, and per-test timeout. git-source keeps its `apt-get install gettext` step inside the serialized loop. The No-GCC container test stays as a separate job since it requires a custom container.

**Rationale:**
The macOS jobs prove this pattern works for exactly these tests. The wall-time cost is modest (serial execution adds ~15 minutes for all 7 tests vs ~5 minutes for the longest parallel job), but queue savings are significant (6 fewer runners competing). The pattern is a direct copy of the macOS implementation already in the same file, reducing the risk of novel failure modes.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-23
