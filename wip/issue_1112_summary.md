# Issue 1112 Summary

## Completed Work

Created `system_dependency` action that checks for system packages and guides users to install them on musl systems (Alpine).

### Files Created
- `internal/actions/system_dependency.go` - Action implementation
- `internal/actions/system_dependency_test.go` - Unit tests

### Files Modified
- `internal/actions/action.go` - Registered the new action

## Key Implementation Decisions

1. **Read-only operation**: The action never runs privileged commands. It only checks if packages are installed and provides installation guidance.

2. **Alpine-only scope**: Initial implementation supports only Alpine Linux (`apk info -e`). The code is structured to be extensible to other families (Debian, RHEL, etc.) in future work.

3. **Root detection**: Uses `os.Getuid()` to detect root. Prefers `doas` over `sudo` when not root (common on Alpine/BSD systems).

4. **Family detection**: Uses `platform.DetectFamily()` to determine the current Linux distribution family.

5. **Structured error type**: `DependencyMissingError` provides fields for CLI aggregation (Library, Package, Command, Family).

## Test Coverage

- Preflight validation (name and packages parameters)
- Error type and message format
- `IsDependencyMissing` / `AsDependencyMissing` helper functions
- `parsePackagesMap` for TOML parameter conversion
- `getInstallCommand` for various families
- Action registration verification

## Out of Scope (Future Work)

Per introspection analysis, these items are handled by downstream issues:
- Plan generator aggregation logic (#1114)
- CLI formatting of aggregated errors (#1114)
- Debian/RHEL/Arch/SUSE package detection
