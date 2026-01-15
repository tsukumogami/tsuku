# Implementation Context: Issue #889

## Bug

dlv (Delve debugger) golden file execution fails on darwin-arm64 because v1.3.0 predates Apple Silicon support.

## Root Cause

The dlv v1.3.0 golden file can't compile on darwin-arm64 - architecture-specific symbols (`archInst`, `asmDecode`, `resolveCallArg`) are missing for arm64.

## Solution

Regenerate golden files with a newer dlv version that supports darwin-arm64 (arm64 support added ~v1.6.0+).

## Acceptance Criteria

- Golden file execution passes on darwin-arm64
