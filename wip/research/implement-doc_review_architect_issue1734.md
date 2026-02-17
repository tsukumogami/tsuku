# Architecture Review: #1734 (feat(userconfig): add secrets section with atomic writes and 0600 permissions)

## Review Scope

Files changed (based on HEAD~1 diff scope -- reviewing the current state of files modified for this issue):

- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/userconfig/userconfig.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/userconfig/userconfig_test.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/secrets/secrets.go`
- `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/secrets/secrets_test.go`

## Design Doc Alignment Check

The design doc specifies that #1734 should:
1. Extend `userconfig.Config` with a `Secrets` map -- **done** (line 31, `Secrets map[string]string`)
2. Switch all config writes to atomic temp+rename with unconditional 0600 -- **done** (lines 146-185, `saveToPath`)
3. Add a permission warning on read -- **done** (lines 114-124, `loadFromPath`)
4. Wire `secrets.Get()` to fall through to config file on env var miss -- **done** (lines 79-85 of `secrets.go`)
5. Handle `secrets.*` key prefix in `Get()`/`Set()` -- **done** (lines 262-273, 303-313)

All design doc requirements for this issue are implemented.

## Findings

### Finding 1: Package-level mutable state with sync.Once in secrets.go

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/secrets/secrets.go`, lines 35-60
**Severity**: advisory

The `secrets` package uses package-level `sync.Once` plus `cachedCfg`/`configError` global variables with a `ResetConfig()` function that replaces the `sync.Once`. This pattern is consistent with what the design doc specifies (lazy loading via `sync.Once`), and the `ResetConfig()` is test-only.

However, this creates implicit global state that couples `secrets.Get()` to the process lifetime. If the user runs `tsuku config set secrets.foo bar` and then immediately calls `secrets.Get("foo")` in the same process, they get the stale cached value. This is acceptable for the current CLI architecture (each command is a separate process invocation), but it's worth noting that the cached config is never invalidated.

The design doc acknowledges this: "The internal/secrets package is a read-only resolution layer. All writes go through userconfig (via the CLI)." Since writes happen via `userconfig.Save()` and reads happen in separate invocations, this is fine for the current architecture.

**Impact**: None for the current CLI model. Could cause confusion if the architecture evolves toward long-running processes or in-process config mutation.

**Suggestion**: No change needed now. If long-running use cases emerge, the `ResetConfig()` function provides the escape hatch.

### Finding 2: Dependency direction is clean and follows existing patterns

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/secrets/secrets.go`, line 20
**Severity**: no issue (positive observation)

The dependency graph is:
- `secrets` -> `userconfig` -> `config` (and `log`)
- `userconfig` does NOT import `secrets` (no circular dependency)
- `secrets` acts as a higher-level resolution layer that composes env var lookups with userconfig data

This matches the architecture diagram in the design doc. The `secrets` package is a read-only consumer of `userconfig`, and `userconfig` has no knowledge of secrets beyond providing the raw `Secrets` map.

### Finding 3: userconfig.Set() accepts arbitrary secret names without validation

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/userconfig/userconfig.go`, lines 303-313
**Severity**: advisory

The `Set("secrets.foo", "bar")` method accepts any secret name. There's no validation against the `knownKeys` table in the `secrets` package. This means a user could do `tsuku config set secrets.typo_api_key value` and it would be silently stored but never resolved by `secrets.Get()`.

This is a deliberate separation-of-concerns decision: `userconfig` is a general-purpose config store that shouldn't depend on `secrets` (which would create a circular dependency). Validation against known keys belongs in the CLI layer (#1737), which can import both packages.

**Impact**: Low. Users can store orphaned secret keys in config. The CLI integration issue (#1737) is the right place to add validation by cross-referencing `secrets.KnownKeys()` during `tsuku config set secrets.*`.

**Suggestion**: No change needed in this issue. Confirm that #1737 adds validation at the CLI layer.

### Finding 4: AvailableKeys() excludes secrets -- correct boundary

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/userconfig/userconfig.go`, lines 387-398
**Severity**: no issue (positive observation)

`AvailableKeys()` correctly does not include secrets. Secret metadata is surfaced via `secrets.KnownKeys()` instead. This keeps `userconfig` as a generic config layer and avoids duplicating the key registry. The test at line 846-853 of the test file explicitly enforces this boundary.

### Finding 5: Atomic write implementation is well-structured

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/userconfig/userconfig.go`, lines 146-185
**Severity**: no issue (positive observation)

The atomic write implementation follows best practices:
- Temp file created in the same directory (guarantees same filesystem for rename)
- `defer os.Remove(tmpPath)` for cleanup on any error path
- Explicit `Chmod(0600)` after `CreateTemp` (doesn't rely on umask)
- File closed before rename (required on some platforms, as documented)
- Parent directory creation is done upfront

This is consistent with the design doc's specification and follows the pattern used by similar tools.

### Finding 6: config.go still uses direct os.Getenv("GITHUB_TOKEN") -- expected

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/cmd/tsuku/config.go`, line 153
**Severity**: no issue (out of scope)

`cmd/tsuku/config.go` still uses `os.Getenv("GITHUB_TOKEN")` directly in the display output. This is expected -- migration of callers to `secrets.Get()` is planned for #1735/#1736. The current implementation correctly stages the infrastructure without migrating callers.

### Finding 7: omitempty tag on Secrets map preserves backward compatibility

**File**: `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/internal/userconfig/userconfig.go`, line 31
**Severity**: no issue (positive observation)

The `toml:"secrets,omitempty"` tag means existing config files without a `[secrets]` section won't get an empty section written on save. This preserves backward compatibility -- a round-trip of load+save on an existing config file won't add unexpected sections.

## Summary

The implementation is well-aligned with the design doc and follows clean architectural patterns. No blocking issues found.

The dependency direction (`secrets` -> `userconfig` -> `config`) is correct and avoids circular dependencies. The `secrets` package is properly positioned as a higher-level resolution layer that composes environment variable lookups with config file data, without leaking its concerns into `userconfig`.

The atomic write implementation is solid and the permission enforcement is unconditional as specified. The `Secrets` map integration into `userconfig.Config` is minimal and non-intrusive, using the existing `Get()`/`Set()` pattern with prefix-based routing.

The one advisory worth tracking: `userconfig.Set("secrets.*")` accepts any key name without validating against known keys. This validation should happen at the CLI layer in #1737 to maintain the current separation of concerns.
