# Issue 508 Baseline

## Environment
- Date: 2025-12-13
- Branch: docs/508-plan-installation-workflow
- Base commit: cdb937178d4d9b3de8b53e2a8ad9d36f5af3e2d0

## Issue Type
Documentation - no code changes expected

## Current State
- PR #512 merged (--plan flag implementation)
- `tsuku install --help` already shows --plan flag
- Design doc at docs/DESIGN-plan-based-installation.md

## Acceptance Criteria from Issue
- [x] `tsuku install --help` includes `--plan` flag description (already done in #507)
- [ ] Examples show file-based workflow: `tsuku install --plan plan.json`
- [ ] Examples show piping workflow: `tsuku eval tool | tsuku install --plan -`
- [ ] Air-gapped deployment workflow documented
- [ ] CI distribution workflow documented
