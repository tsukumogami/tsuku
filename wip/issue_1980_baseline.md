# Issue 1980 Baseline

## Environment
- Date: 2026-03-01
- Branch: fix/cargo-lib-recipes
- Base commit: c86f3de1 (main)

## Test Results
- All tests pass (short mode)
- Build: OK

## Pre-existing Issues
None relevant.

## Findings

The 6 affected entries are queue entries in `data/queues/priority-queue.json`, not recipe files on disk.
All have `status: "pending"`, `failure_count: 0`, and `source: "crates.io:<name>"`.

- `b3sum.toml` already exists in `recipes/b/` (correct recipe for the blake3 crate's CLI).
- None of the 6 appear in the discovery registry or curated list.
- The batch orchestrator passes `entry.Source` directly to `--from`.
