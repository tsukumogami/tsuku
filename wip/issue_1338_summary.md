# Issue 1338 Summary

## What Was Implemented

Wired the discovery resolver chain into `tsuku install` so that unknown tool names automatically fall back to source discovery instead of failing with "recipe not found."

## Changes Made

- `cmd/tsuku/install.go`: Added `tryDiscoveryFallback()` function that calls `runDiscovery()` from create.go, then forwards to the create+install pipeline on success. In the normal install loop, added a pre-check via `loader.Get()` — when no recipe exists, tries discovery before the full install flow. This avoids the confusing intermediate "recipe not found" error message.
- `test/functional/features/install.feature`: Added two scenarios: discovery fallback via registry (shfmt resolved from discovery.json, installed via homebrew), and actionable error for truly unknown tools.
- `recipes/discovery.json`: Added `shfmt` entry (homebrew builder, no runtime deps) to enable end-to-end functional testing of the discovery fallback path.

## Key Decisions

- Pre-check recipe existence with `loader.Get()` before the full install flow rather than catching the error after. This prevents the noisy "To create a recipe..." suggestion from printing when discovery is about to try.
- `tryDiscoveryFallback` calls `exitWithCode` on failure rather than returning an error, matching the pattern used by `--from` handling and `runCreate`. This keeps the flow simple.
- Reused the same create pipeline forwarding mechanism from #1337 (set package-level vars, call `runCreate`).

## Trade-offs Accepted

- Discovery runs for every tool that lacks a recipe, adding a small overhead (registry file read). Acceptable since it's a local file operation.
- The `tryDiscoveryFallback` function uses `exitWithCode` which means it doesn't return on failure — same pattern as other install paths but limits composability.

## Test Coverage

- 2 new functional test scenarios (discovery success + unknown tool error)
- All 44 functional scenarios pass
- All unit tests pass
