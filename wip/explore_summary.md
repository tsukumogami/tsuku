# Exploration Summary: CI Job Consolidation

## Problem (Phase 1)
PR CI runs spawn 50-86 GitHub Actions jobs due to matrix strategies that create separate runners for each Linux distro family and each integration test tool, even though all families run on the same ubuntu-latest runner type and use Docker containers. Queue wait times of 7-11 minutes dwarf actual test execution times of 30-50 seconds. This inflates total CI duration and burns runner minutes without proportional value.

## Decision Drivers (Phase 1)
- Queue wait time is the primary bottleneck (measured: 7-11 min wait for 30-50 sec tests)
- Linux families can all be tested on any ubuntu-latest runner via containers
- macOS and architecture splits (arm64 vs x86_64) genuinely need separate runners
- GHA groups provide structured output without needing separate jobs
- test-recipe.yml already demonstrates the optimal containerization pattern
- Changes should be incremental, not a rewrite of all 53 workflows

## Research Findings (Phase 2)
- 53 workflows, ~86 jobs per worst-case PR, ~28 per typical Go-only PR
- test-recipe.yml containerizes 5 Linux families in 1 job (gold standard)
- build-essentials.yml macOS aggregates all tests in 1 job with GHA groups
- integration-tests.yml and build-essentials.yml use family-per-job matrices (wasteful)
- test.yml integration-linux spawns 9 separate jobs for 9 tools on same runner type
- sandbox-tests.yml spawns 9 more jobs from the same test matrix
- ~41 jobs could be eliminated through containerization + serialization

## Options (Phase 3)
- **1A: Containerize family matrices** - Apply test-recipe.yml pattern to all workflows
- **1B: Keep family matrices** - Status quo
- **2A: Serialize integration tests with GHA groups** - One job per arch/OS
- **2B: Keep tool-per-job matrix** - Status quo
- **2C: Artifact-based binary sharing** - Build once, share via artifacts (complementary)
- **3A: Incremental rollout** - One workflow at a time
- **3B: Big-bang refactor** - All at once
- **3C: Reusable workflow extraction** - Shared container loop workflow

## Phase 4 Review Feedback (incorporated)
- Added queue-time data from run 22285437491 (7-11 min waits)
- Added sandbox-tests.yml as consolidation target (+8 jobs savings)
- Fixed read-only mount claim (actually read-write)
- Fixed scope contradiction (macOS serialization is in scope)
- Added artifact-based binary sharing as considered alternative
- Strengthened reusable workflow rejection with drift mitigation
- Added wall-time comparison math
- Added correlated failure risk to trade-offs
- Added per-test timeout requirement

## Decision (Phase 5)

**Problem:**
PR CI runs in the tsuku repo spawn 50-86 GitHub Actions jobs because matrix strategies create separate runners for each Linux distribution family and each integration test tool. All Linux families run on ubuntu-latest with Docker containers, so each separate runner incurs queue wait and setup overhead for no isolation benefit. Measured queue waits of 7-11 minutes for tests that execute in 30-50 seconds show that queue congestion is the dominant cost, and runner-minute costs scale with job count rather than actual test duration.

**Decision:**
Apply the containerization pattern from test-recipe.yml to all workflows that currently use family-per-job matrices, and serialize integration tests within single runners using GHA groups. This reduces worst-case PR job count by roughly 48% (from ~86 to ~45 jobs) while preserving identical test coverage. The primary split remains architecture (arm64 vs x86_64) and OS (Linux vs macOS). Within each Linux runner, all family variants run sequentially in Docker containers, and tool-level tests serialize with GHA groups for structured output. Each consolidated test is wrapped in a per-test timeout to prevent hangs from blocking subsequent tests.

**Rationale:**
The project already has a proven pattern in test-recipe.yml that runs 5 Linux families in a single runner via Docker containers, and build-essentials.yml macOS jobs that aggregate multiple tools with GHA groups. Extending these patterns to the remaining workflows is low-risk because the container and group mechanisms are already battle-tested. The wall-time trade-off favors serialization for short tests: measured queue waits of 7-11 minutes per job far exceed the 30-50 second execution times, so serializing 9 tests into one job (total ~8-12 min) is faster end-to-end than 9 parallel jobs each waiting 7-11 minutes. An incremental rollout lets each workflow be validated independently.

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-02-22
