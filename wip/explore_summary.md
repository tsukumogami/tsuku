# Exploration Summary: Batch Platform Validation

## Problem (Phase 1)
The batch pipeline generates recipes on Linux x86_64 and validates they install on that single platform, but recipes claim to support all platforms by default. No validation runs on arm64, musl, or macOS before the PR is created, so broken platform-specific downloads or binaries ship to users undetected.

## Decision Drivers (Phase 1)
- macOS runners cost 10x Linux (1000 min/week budget)
- Progressive validation catches ~80% failures on cheapest platform first
- Partial coverage is acceptable (>=1 platform) with constraint metadata
- Merge job must write platform constraints based on validation results
- PR-time validation for new recipes has no golden baseline in R2

## Research Findings (Phase 2)
- Batch orchestrator runs `tsuku install --force --recipe` on Linux only (orchestrator.go:203-256)
- Platform constraint fields exist and work: supported_os, unsupported_platforms, supported_libc
- test-changed-recipes.yml does install on Linux + macOS for PRs, but doesn't generate plans
- Golden execution validation skips for new registry recipes (no R2 baseline)
- publish-golden-to-r2.yml generates but doesn't validate before uploading
- Progressive validation (Jobs 3-4) from batch design is unimplemented

## Current Status
**Phase:** 3 - Options
**Last Updated:** 2026-01-31
