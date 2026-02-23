# Exploration Summary: Sandbox CI Integration

## Problem (Phase 1)
Sandbox can't replace direct docker calls in CI because it lacks verification, env passthrough, and structured output. This blocks CI workflow simplification and creates maintenance burden from duplicated container logic.

## Decision Drivers (Phase 1)
- Incremental migration path (don't require all-at-once CI rewrite)
- Backward compatibility (don't break existing sandbox callers)
- Recipe-driven verification (reuse existing [verify] sections)
- Security: env passthrough must be opt-in, not automatic

## Research Findings (Phase 2)
- The validate package already runs recipe verify commands inside containers. Sandbox doesn't.
- The plan already carries Verify.Command and Verify.Pattern fields. Sandbox ignores them.
- CI passes GITHUB_TOKEN and TSUKU_REGISTRY_URL into containers. Sandbox hardcodes a fixed env set.
- CI produces JSON results with recipe/platform/status/exit_code/attempts and GitHub step summaries.
- CI retry logic (exit code 5, 3 attempts, exponential backoff) is workflow-level, not container-level.

## Options (Phase 3)
- Decision 1 (Verification): Run plan's Verify.Command in sandbox script vs external verify command vs skip
- Decision 2 (Env Passthrough): --env flag vs config file vs automatic passthrough
- Decision 3 (Structured Output): --json flag vs file output vs status codes only

## Decision (Phase 5)

**Problem:**
Tsuku's sandbox can't replace the 12 recipe validation docker calls in CI because it has no post-install verification (just checks exit code 0), no way to pass environment variables like GITHUB_TOKEN into containers, and no machine-readable output format. CI workflows maintain their own container logic with retry, verification, and reporting code that duplicates what sandbox should provide.

**Decision:**
Close the three gaps with targeted extensions to the sandbox package and CLI. Add verify command execution to the sandbox script using the plan's existing Verify fields, mirroring what the validate package already does. Add an --env flag for explicit environment variable passthrough. Add a --json flag for machine-readable output. Keep retry and batching as CI-layer concerns. Defer binary quality checks (ELF linking, RPATH verification) as a separate future effort.

**Rationale:**
All three gaps have natural extension points in the existing code. The plan already carries verify info that the sandbox ignores. The RunOptions struct accepts arbitrary env vars. The SandboxResult struct contains everything needed for JSON output. By keeping changes to the sandbox package and CLI flags, we avoid API changes that would affect the orchestrator or validate packages. Retry and batching stay in CI because they're workflow concerns, not sandbox concerns.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-22
