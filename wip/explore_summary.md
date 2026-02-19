# Exploration Summary: LLM Testing Strategy

## Problem (Phase 1)

The local LLM runtime (#1707) has two confirmed failure categories (server crashes on long inference, model quality regression on complex naming patterns) but no automated way to catch these regressions. The existing test infrastructure covers lifecycle management and cloud provider quality, but nothing validates local model output quality or server stability under realistic workloads.

## Decision Drivers (Phase 1)

- Must catch the two failure modes from #1738 (server crashes, quality regression) before they reach users
- Should extend existing test infrastructure rather than building parallel systems
- Must work in CI without GPU hardware (CPU-only inference)
- Should support both automated regression detection and manual validation workflows
- Need provider-agnostic quality tests so the same suite works for local, Claude, and Gemini
- Latency and resource constraints of CPU inference shape which tests can run in CI vs locally
- Testing should validate the full flow (Go -> gRPC -> Rust -> llama.cpp) not just individual components

## Research Findings (Phase 2)

Key findings from codebase research:
- 13 Go test files in internal/llm/ with solid unit and mock coverage
- TestLLMGroundTruth in internal/builders/ has 21 test cases but only works with Claude
- lifecycle_integration_test.go tests daemon lifecycle but not quality or stability
- cmd/benchmark/main.go exists as a CLI harness but doesn't track baselines
- No Rust integration tests (only ~43 inline unit tests in tsuku-llm/)
- LocalProvider has no reconnection logic after server crashes
- Grammar-constrained generation is disabled for Qwen (llama.cpp compatibility issue)
- CI runs integration tests gated by change detection on tsuku-llm/ and internal/llm/

## Options (Phase 3)

1. Quality test architecture: Provider-parameterized ground truth vs. separate per-provider suites vs. single threshold vs. snapshot comparison
2. Stability testing: Layered (short CI + long manual) vs. full CI reproduction vs. Rust-only stress vs. memory limits
3. CI integration: Tiered jobs (fast lifecycle + separate quality gate) vs. all-in-one vs. nightly-only vs. mock-based

## Decision (Phase 5)

**Problem:**
The local LLM runtime's test infrastructure covers lifecycle management and cloud provider recipe quality but has no automated way to detect local model quality regressions or server stability failures. This gap let two failure categories (server crashes during long inference, model quality regression on Rust-style naming patterns) reach QA without being caught by any test. Changes to prompts, models, or inference parameters can ship regressions silently.

**Decision:**
Extend the existing ground truth test suite to accept a provider parameter, add per-provider quality baselines in testdata for regression detection, split stability testing into CI-feasible sequential inference tests plus a documented manual runbook for long-running scenarios, and add a separate CI quality gate that triggers on prompt and test matrix changes. Fix the LocalProvider connection caching bug to enable crash-recovery testing.

**Rationale:**
Building on the existing 21-case ground truth matrix avoids duplication and lets the same tests validate all providers. Per-test baselines give precise regression visibility where aggregate thresholds hide failures. Layered stability testing balances CI time budgets against the need to catch long-running crashes. The tiered CI approach keeps most LLM PRs fast while adding quality gates where they matter. This combination would have caught both failure categories from #1738.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-18
