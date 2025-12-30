# Issue 750 Implementation Plan

## Summary

Extend `DetectRedundantVersion()` to detect redundant homebrew version configuration.

## Approach

1. Add `"homebrew": "homebrew"` to `actionInference` map
2. Add formula matching logic (similar to go_install module matching)
3. Add test cases for:
   - Redundant: `source = "homebrew"` + `action = "homebrew"` with same formula
   - Not redundant: `source = "homebrew"` + different action (ninja.toml pattern)
   - Not redundant: `source = "homebrew"` + `action = "homebrew"` with different formula

## Files to Modify

1. `internal/version/redundancy.go` - Add homebrew detection logic
2. `internal/version/redundancy_test.go` - Add test cases

## Post-Implementation

Run `tsuku validate --strict` on all recipes and fix redundant configurations.
