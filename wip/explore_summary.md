# Exploration Summary: Consolidate System Dependency Extraction

## Problem (Phase 1)

Two overlapping implementations extract system packages from recipes/plans: `executor/system_deps.go` (5 actions, no repos) and `sandbox/packages.go` (11 actions, full repository support). The original design intended shared code, but implementations diverged. The `info --deps-only --system` command misses repository configurations that sandbox already handles.

## Decision Drivers (Phase 1)

- Reduce code duplication between implementations
- Maximize coverage: `--deps-only --system` should include repositories
- Maintain backward compatibility with existing consumers
- Follow the original design intent from DESIGN-recipe-driven-ci-testing
- Keep install-recipe-deps.sh helper script working

## Decision (Phase 5)

Consolidate extraction into a single SystemRequirements type in internal/executor that handles all system dependency actions (packages and repositories). Both info --deps-only and sandbox will use this unified extraction. The info command gains --repos flag for JSON output of repository configurations, and the helper script is extended to set up repositories before installing packages.

## Rationale (Phase 5)

Placing the consolidated code in internal/executor follows the original design intent and keeps package-level dependencies clean (sandbox can import executor, but not vice versa). The SystemRequirements struct from sandbox already has the right shape. Backward compatibility is preserved: the default text output of --deps-only --system remains package names only, while --repos adds repository output in JSON format.

## Current Status
**Phase:** 8 - Final Review
**Last Updated:** 2026-02-11
