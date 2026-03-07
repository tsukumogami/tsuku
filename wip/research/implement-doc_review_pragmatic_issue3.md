# Pragmatic Review: Issue #3

## Issue: chore(scripts): update generation script to emit integer schema version

## Review Summary

No findings. The change is a single constant value update from `"1.2.0"` (string) to `1` (integer) at `scripts/generate-registry.py:23`. Python's `json.dump` serializes `int` as a JSON integer, so the generated `recipes.json` will contain `"schema_version": 1` as required.

No new abstractions, no dead code, no speculative generality. This is the simplest correct approach.

## Findings

None.
