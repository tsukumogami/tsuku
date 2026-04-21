# Security Review: install-ux-v2

## Dimension Analysis

### External Artifact Handling
**Applies:** No

This design changes only output routing through a `progress.Reporter` interface. It does not modify download mechanisms, archive extraction security (path traversal and symlink validation already exist in `extract.go`), checksum verification, recipe loading, or network call handling. All external artifact validation remains unchanged.

### Permission Scope
**Applies:** No

The reporter writes only to `io.Writer` (stdout/stderr), which are already available to the tsuku process. The design adds no new filesystem operations, does not escalate privileges, does not create new IPC mechanisms, and does not change file permissions on installed tools. The `Manager` receives a reporter instance set by the caller; the caller already possesses stdout access. No new privilege boundaries are crossed.

### Supply Chain or Dependency Trust
**Applies:** No

The Reporter interface and related types (`NoopReporter`, `ttyReporter`) are internal to tsuku and require no new external dependencies. No new imports or libraries are added. Recipe sourcing, embedded registry resolution, and GitHub token handling in `internal/secrets/` are all untouched. Secrets are already isolated and not wired into output paths.

### Data Exposure
**Applies:** No

Reporter methods receive only display strings (tool names, versions, status messages), which are already written to the terminal by existing `fmt.Printf` calls. The design does not add logging of sensitive data, does not capture new data, and preserves the existing contract that callers must not pass values from `internal/secrets/` to any Reporter method. ANSI injection prevention via `SanitizeDisplayString` remains in place.

## Recommended Outcome

**OPTION 3 - N/A with justification:** This design introduces no new security dimensions. It is a refactoring of output routing — replacing direct `fmt.Printf` calls with Reporter method calls. All artifact handling, permission scope, supply chain trust, and data exposure are identical before and after implementation. The changes are confined to output formatting and orchestration, layers already below authentication, download verification, and secret handling.

## Summary

The install-ux-v2 design is a benign output refactoring that poses no new security risks. It consolidates console output through a single interface to improve UX, while maintaining all existing validation, verification, and secret-handling mechanisms unchanged. No external artifacts, permissions, dependencies, or sensitive data flows are affected.
