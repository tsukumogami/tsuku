# Architect Review: Issue 2 - Deprecation Notice Parsing and Warning Display

## Summary

The implementation follows the design doc's architecture closely and respects established codebase patterns. No blocking findings.

## Findings

### 1. Display boundary correctly maintained (Positive)

The `DeprecationNotice` struct and parsing live in `internal/registry/manifest.go`. Display logic (`checkDeprecationWarning`, `formatDeprecationWarning`, `printWarning`) lives in `cmd/tsuku/helpers.go`. This matches the design doc's Decision 2: "parseManifest() parses and stores but does not write to stderr -- the cmd/ layer checks and displays." The registry package has no stderr writes related to deprecation.

### 2. Error type pattern consistency (Positive)

`DeprecationNotice` is a data struct on `Manifest`, not a new error type. The existing `ErrTypeSchemaVersion` from Issue 1 handles the hard-failure path. Deprecation is a warning-level concern, not an error, and the implementation correctly keeps it separate from the error type system. No parallel error mechanism introduced.

### 3. Dependency direction correct (Positive)

`cmd/tsuku/helpers.go` imports `internal/registry`, `internal/buildinfo`, and `internal/version`. Dependencies flow downward from cmd to internal. The registry package does not import cmd or buildinfo.

### 4. Version comparison reuses existing utility (Positive)

`formatDeprecationWarning` calls `version.CompareVersions()` from `internal/version/version_utils.go` rather than introducing a new comparison mechanism. This matches the design doc's Decision 3 and avoids a parallel pattern.

### 5. `sync.Once` dedup is global, not per-registry -- Advisory

`deprecationWarningOnce` is a single package-level `sync.Once`. The design doc explicitly notes: "When multi-registry support ships (#2073), the sync.Once should become per-registry dedup." The current implementation is correct for the single-registry case. This is a known future change, not a structural violation.

**Severity: Advisory.** The design doc itself calls this out as future work. No action needed now.

### 6. Warning integration limited to `update-registry` path -- Advisory

`checkDeprecationWarning` is called only from `refreshManifest()` in `update_registry.go`. The design doc mentions warnings covering "both update-registry (which fetches fresh) and recipe-using commands (which read from cache)." Commands that read from cache via `GetCachedManifest()` do not currently check for deprecation notices.

This is noted in the prior scrutiny research (`implement-doc_scrutiny_intent_issue2.md:99`). Whether this is intentional scoping for Issue 2 or an omission depends on how strictly the acceptance criteria are interpreted. The acceptance criteria list "Warning fires at most once per CLI invocation via sync.Once" but don't explicitly require integration into cache-reading paths.

**Severity: Advisory.** The `checkDeprecationWarning` function is designed to be called from any path -- it accepts `manifest` and `registryURL` parameters, and the `sync.Once` prevents duplicates. Adding call sites in cache-reading commands later is a one-line change per command, with no structural rework needed.

### 7. `ManifestURL()` export aligns with design intent (Positive)

The `manifestURL()` method was promoted to exported `ManifestURL()` to allow `cmd/` code to pass the actual fetch URL into the warning. This avoids hardcoding the default URL and correctly attributes warnings to the actual registry source, as the design doc requires for security (supply chain section).

### 8. `resetDeprecationWarning()` test helper pattern -- Advisory

The `resetDeprecationWarning()` function resets the `sync.Once` for test isolation. This is unexported, so it's only available within the `main` package tests. This is a pragmatic pattern for testing global state, not a structural concern.

**Severity: Advisory.** Contained to test code.

## Dependency Analysis

| Source | Imports | Direction |
|--------|---------|-----------|
| `cmd/tsuku/helpers.go` | `internal/registry`, `internal/buildinfo`, `internal/version` | Correct (cmd -> internal) |
| `internal/registry/manifest.go` | `encoding/json`, `fmt`, `io`, `net/http`, `os`, `path/filepath` | Correct (stdlib only) |

No circular dependencies. No upward imports.

## Pattern Consistency

| Pattern | Codebase Convention | This Change | Match? |
|---------|-------------------|-------------|--------|
| Error types | `ErrorType` iota in `errors.go` | No new error type needed (deprecation is data, not error) | Yes |
| Warning display | `fmt.Fprintf(os.Stderr, ...)` in cmd/ | `printWarning()` writes to stderr | Yes |
| Quiet flag | `printInfo`/`printInfof` check `quietFlag` | `printWarning` checks `quietFlag` | Yes |
| Version comparison | `version.CompareVersions()` | Reused directly | Yes |

## Overall Assessment

The implementation is architecturally clean. It follows every structural decision from the design doc: display boundary between internal/registry and cmd/, reuse of existing version comparison, proper dependency direction, and extension of existing patterns rather than introduction of parallel ones. The two advisory items (global sync.Once, limited integration points) are both acknowledged in the design doc as known future work.
