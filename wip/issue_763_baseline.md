# Issue #763 Baseline

## Issue
feat(actions): implement Describe() for documentation generation

## Branch
feature/763-describe-method

## Baseline State
- All tests passing
- Build succeeds
- Main branch: 7b4759a

## Dependencies
- #755 (action structs): CLOSED
- #756 (config/verification structs): CLOSED

## Scope
Implement `Describe() string` method on all typed actions to generate human-readable, copy-pasteable shell commands.
