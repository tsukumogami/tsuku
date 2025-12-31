# Issue 756 Baseline

## Environment
- Date: 2025-12-31
- Branch: feature/756-config-verification-actions
- Base commit: 3f1dbff (main)

## Test Results
- Total: All packages passed
- Passed: 20 packages (cmd/tsuku, internal/*)
- Failed: 0

## Build Status
- Build succeeded (go build -o tsuku ./cmd/tsuku)

## Pre-existing Issues
None detected.

## Issue Context
Issue #756: feat(actions): define configuration and verification action structs

Goal: Create Go structs for configuration (`group_add`, `service_enable`) and verification (`require_command`, `manual`) actions.

Acceptance Criteria:
- [ ] Configuration actions: `GroupAddAction`, `ServiceEnableAction`, `ServiceStartAction`
- [ ] Verification/fallback: `RequireCommandAction`, `ManualAction`
- [ ] All structs implement `SystemAction` interface
- [ ] All structs have `Validate() error` method
- [ ] Action parsing from TOML step
- [ ] Unit tests for each action struct
