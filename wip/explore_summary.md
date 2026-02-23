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
Close the three gaps with targeted extensions, then migrate all four affected CI workflows. Add verify command execution using the plan's Verify fields with Go-side pattern matching shared with the validate package. Add --env for explicit env passthrough with key filtering. Add --json for machine-readable output. Then replace docker run blocks in test-recipe.yml, recipe-validation-core.yml, batch-generate.yml, and validate-golden-execution.yml with sandbox calls. Retry and batching stay as workflow-layer concerns consuming sandbox JSON output.

**Rationale:**
All three gaps have natural extension points in existing code. The plan already carries verify info that the sandbox ignores. The RunOptions struct accepts arbitrary env vars. The SandboxResult struct has everything needed for JSON output. Migrating workflows incrementally limits risk while each migration proves the pattern. After migration, all recipe validation uses the same code path locally and in CI.

## Current Status
**Phase:** 5 - Decision
**Last Updated:** 2026-02-22
