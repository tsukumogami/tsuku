---
status: Planned
problem: |
  The auto-update system ignores .tsuku.toml project-level version constraints.
  MaybeAutoApply operates globally on cached check entries with no project
  awareness. A team pinning node = "20.16.0" in their project config expects
  auto-update to leave node alone, but today it updates regardless.
decision: |
  Load .tsuku.toml from CWD ancestry during auto-apply and use project-level
  version constraints as effective pins that override global state.json pins.
  Exact pins suppress auto-update entirely; prefix pins narrow the boundary.
  Project config is passed into MaybeAutoApply as a parameter following the
  established caller-loads-and-passes pattern.
rationale: |
  Project pin override is the only approach that correctly implements PRD R17
  across all pin levels. The alternatives (suppress exact pins only, or
  blocklist classification) both fail the node = "20" with global "latest"
  scenario. Passing config as a parameter matches every existing usage of
  LoadProjectConfig in the codebase and keeps tests filesystem-free.
upstream: docs/prds/PRD-auto-update.md
---

# DESIGN: Project-Level Auto-Update Integration

## Status

Proposed

## Context and Problem Statement

`.tsuku.toml` declares per-project tool version constraints, but the auto-update
system ignores them entirely. `MaybeAutoApply` in `internal/updates/apply.go`
operates globally on cached update check entries with no concept of which project
the user is working in.

This creates a conflict: a team pins `node = "20.16.0"` in their project config
expecting deterministic builds, but auto-update could install a different version
because the global pin (from `state.json`'s `Requested` field) may be broader
(e.g., `""` = latest, allowing any version).

PRD requirement R17 is clear: `.tsuku.toml` version constraints take precedence
over global auto-update policy. Exact versions disable auto-update for that tool
in that project context. Prefix versions allow auto-update within the pin.

The core challenge is that auto-apply runs in `PersistentPreRun` before any
command executes, using CWD to detect the project. But CWD can change between
sessions, and a single tsuku installation serves all projects. The design must
reconcile per-project constraints with a global tool installation model.

### Scope

**In scope:**
- How `.tsuku.toml` constraints suppress or allow auto-update per tool
- Where project config is injected into the auto-apply decision
- Pin semantics: exact versions disable, prefix versions allow within pin
- CWD-based project detection and its edge cases
- Any ToolRequirement struct extensions

**Out of scope:**
- New `.tsuku.toml` syntax beyond what's needed for auto-update
- Per-tool auto-update config in `.tsuku.toml` (belongs in `config.toml`)
- Organization-level policy files
- Changes to the update check infrastructure

## Decision Drivers

- `.tsuku.toml` takes precedence over global config (PRD R17)
- Must work with existing `PinLevelFromRequested` / `VersionMatchesPin` semantics
- Zero added latency (R19) -- project config loading must be fast
- MaybeAutoApply runs in PersistentPreRun -- CWD is available but may differ from install time
- Atomic operations (R21) -- no partial state from project config interactions
- Simple mental model: users should predict what auto-update will do

## Considered Options

### Decision 1: Suppression Model

`.tsuku.toml` declares tool version constraints using the same pin semantics as
`state.json`'s `Requested` field: `""` = latest, `"20"` = major, `"1.29"` = minor,
`"1.29.3"` = exact. The question is how these constraints interact with auto-apply's
filtering of cached update entries.

The motivating scenario: a project declares `node = "20"` but the user's global pin
is `"latest"`. The update cache has node 22.0.0 as `LatestWithinPin`. Should
auto-apply install it?

Key assumptions:
- The cached `LatestWithinPin` was resolved against the global pin, not the project pin
- `VersionMatchesPin(version, constraint)` is a fast string-prefix check
- `LoadProjectConfig(cwd)` is a lightweight directory walk (2-5 stat calls)

#### Chosen: Project pin overrides global pin during auto-apply filtering

When a `.tsuku.toml` exists in the CWD ancestry, its version constraints replace the
cached `Requested` field for auto-apply filtering. For each pending update:

1. If the tool is declared in `.tsuku.toml`, use the project's `Version` as the
   effective pin instead of the cached `Requested`.
2. If the effective pin is `PinExact`, skip the entry entirely -- exact pins never
   auto-update.
3. If the effective pin is `PinMajor` or `PinMinor`, check
   `VersionMatchesPin(entry.LatestWithinPin, projectVersion)`. If the cached
   candidate doesn't satisfy the project's narrower boundary, skip it.
4. If the tool is NOT declared in `.tsuku.toml`, use the cached `Requested` as
   today -- no behavioral change.

This handles all edge cases correctly:

- **No `.tsuku.toml`**: auto-apply proceeds with global pins (no change from today).
- **Tool declared but not installed**: no cached entry exists, nothing to filter.
- **Tool installed but not in `.tsuku.toml`**: global pin applies.
- **Broader project pin than global pin** (project says `"20"`, user installed with
  `"20.16"`): the cached candidate was computed under the narrower `"20.16"` pin,
  so it's already within `"20"`. The update proceeds. A broader project pin never
  blocks an update the global pin already allowed.
- **Narrower project pin than global pin** (project says `"20"`, global is `"latest"`):
  `VersionMatchesPin("22.0.0", "20")` returns false, blocking the update. This is
  the scenario R17 is designed to prevent.
- **Channel pins** (project says `"@stable"`): `VersionMatchesPin` returns false for
  `@`-prefixed values, which conservatively suppresses auto-update. This is safe --
  channel resolution requires provider-specific logic that auto-apply doesn't invoke.

#### Alternatives Considered

**Exact pins suppress, prefix pins are advisory**: Only exact pins in `.tsuku.toml`
block auto-apply. Prefix pins are informational. Rejected because it violates PRD R17:
a project declaring `node = "20"` would still allow a node 22.x update if the global
pin is `"latest"`. This is the exact scenario R17 exists to prevent.

**Allowlist/blocklist classification**: Classify declared tools as blocked (exact pin)
or allowed (anything else). The global pin governs when allowed. Rejected for the same
reason as above -- functionally equivalent to "exact pins suppress" with extra
abstraction. Prefix pins in `.tsuku.toml` must have enforcement power.

### Decision 2: Integration Point

MaybeAutoApply needs access to project config to implement the suppression model. The
question is how the config reaches the function.

Key assumptions:
- Every existing usage of `LoadProjectConfig` follows a caller-loads-and-passes pattern
- `MaybeAutoApply` already handles nil parameters gracefully (`userCfg == nil`)
- There is exactly one call site (main.go PersistentPreRun)

#### Chosen: Pass project config as a parameter

Add `*project.ConfigResult` to MaybeAutoApply's signature. The caller loads it from
CWD and passes it in, following the established pattern used by `cmd_run.go`,
`cmd_shim.go`, `shellenv/activate.go`, and `install_project.go`.

```go
func MaybeAutoApply(cfg *config.Config, userCfg *userconfig.Config,
    projectCfg *project.ConfigResult, installFn InstallFunc,
    tc *telemetry.Client)
```

A nil `*project.ConfigResult` means "no project context" -- MaybeAutoApply applies
global behavior unchanged. The caller change in main.go is three lines:

```go
projCfg, _ := project.LoadProjectConfig(cwd)
updates.MaybeAutoApply(cfg, userCfg, projCfg, installFn, tc)
```

**ToolRequirement needs no changes.** The existing `Version` string combined with
`PinLevelFromRequested` and `VersionMatchesPin` already distinguishes exact pins
(suppress) from prefix pins (allow within boundary). An `AutoUpdate` bool would
contradict the mental model and is explicitly out of scope.

#### Alternatives Considered

**Load project config inside MaybeAutoApply**: Call `LoadProjectConfig(os.Getwd())`
internally. Rejected because it breaks the caller-loads-and-passes pattern used by
every other consumer of `LoadProjectConfig` in the codebase, and it hurts testability
by requiring filesystem setup for `.tsuku.toml` files in tests.

**Inject a filter function**: Add an optional filter callback. Rejected because the
filter would need enough pin-semantics context (LatestWithinPin, project version) that
it essentially leaks update internals to the caller. Over-engineered for a single use
case with one caller and one filtering concern.

## Decision Outcome

**Chosen: Project pin override + parameter passing**

### Summary

When `MaybeAutoApply` runs during PersistentPreRun, it receives the project config
loaded from CWD. For each pending cached update entry, it checks whether the tool
is declared in `.tsuku.toml`. If declared, the project's version constraint becomes
the effective pin, overriding the global `Requested` field from state.json.

Exact project pins (`"20.16.0"`) suppress auto-update entirely for that tool -- the
entry is skipped with no install attempt. Prefix project pins (`"20"` or `"1.29"`)
narrow the boundary: `VersionMatchesPin(entry.LatestWithinPin, projectVersion)` checks
whether the cached candidate satisfies the project's constraint. If not, the entry is
skipped. If the tool isn't declared in `.tsuku.toml`, or no project config exists,
behavior is unchanged from today.

The filtering happens after loading cached entries and before the install loop, using
the same `PinLevelFromRequested` and `VersionMatchesPin` functions that already power
version resolution elsewhere. No new version checks or network calls are needed -- the
filtering operates entirely on cached data and string comparisons.

CWD-based detection means the project config applies only when the user is "in" the
project (their shell's working directory is within the project tree). Running tsuku
from a different directory uses global pins. This matches the mental model of tools
like `.nvmrc` and `.tool-versions`.

### Rationale

Project pin override is the only approach that correctly implements PRD R17 for both
exact and prefix pins. The "exact-only" alternatives fail the motivating scenario
(`node = "20"` with global `"latest"`). The parameter-passing approach follows the
established codebase pattern for project config and keeps tests simple.

The main trade-off is that a narrower project pin can silently suppress an update
that the global pin would have allowed. This is the correct behavior -- the project
constraint takes precedence -- but there's no notification when this happens. A future
enhancement could log a debug message for visibility.

## Solution Architecture

### Components

```
main.go (PersistentPreRun)
  |
  | LoadProjectConfig(cwd)
  v
MaybeAutoApply(cfg, userCfg, projectCfg, installFn, tc)
  |
  | For each pending entry:
  |   1. Look up tool in projectCfg.Config.Tools
  |   2. If found: effectivePin = project Version
  |   3. If not found: effectivePin = entry.Requested (global)
  |   4. If PinExact(effectivePin): skip
  |   5. If !VersionMatchesPin(LatestWithinPin, effectivePin): skip
  |   6. Otherwise: proceed with install
  v
applyUpdate(entry, installFn)
```

### Data Flow

1. PersistentPreRun calls `project.LoadProjectConfig(cwd)` -- walks up from CWD
   looking for `.tsuku.toml`, returns `*ConfigResult` or nil
2. Passes result to `MaybeAutoApply` alongside existing parameters
3. MaybeAutoApply reads cached update entries from `$TSUKU_HOME/cache/updates/`
4. For each entry, checks `projectCfg.Config.Tools[entry.Tool]`
5. Applies pin-level filtering using existing `PinLevelFromRequested` and
   `VersionMatchesPin` from `internal/install/pin.go`
6. Entries that pass filtering proceed to `applyUpdate` as today

### Files Modified

| File | Change |
|------|--------|
| `internal/updates/apply.go` | Add `*project.ConfigResult` parameter, add project-aware filtering before install loop |
| `internal/updates/apply_test.go` | Add tests for project pin suppression (exact, prefix narrower, prefix broader, undeclared tool, nil config) |
| `cmd/tsuku/main.go` | Load project config from CWD, pass to MaybeAutoApply |

### New Function

```go
// effectivePin returns the version pin to use for auto-apply filtering.
// Project config takes precedence when the tool is declared.
func effectivePin(tool string, entry UpdateCheckEntry, projectCfg *project.ConfigResult) string {
    if projectCfg != nil && projectCfg.Config != nil {
        if req, ok := projectCfg.Config.Tools[tool]; ok {
            return req.Version
        }
    }
    return entry.Requested
}
```

## Implementation Approach

### Single phase

This is a focused change to 3 files with no new packages or infrastructure:

1. Add `effectivePin()` helper to `apply.go`
2. Modify `MaybeAutoApply` to accept `*project.ConfigResult` and use `effectivePin()`
   in the filtering loop
3. Update `main.go` to load project config and pass it through
4. Add tests covering: exact pin suppression, prefix pin narrowing, prefix pin
   broadening (no-op), undeclared tool passthrough, nil project config

## Security Considerations

**No new attack surface.** The change reads `.tsuku.toml` from the filesystem using
the existing `LoadProjectConfig` function, which already has ceiling detection
(`$HOME`, `TSUKU_CEILING_PATHS`) to prevent walking above the user's home directory.
No new file paths, network calls, or permission changes are introduced.

**Symlink traversal.** `LoadProjectConfig` already calls `filepath.EvalSymlinks` before
directory traversal. No change needed.

**Malicious .tsuku.toml.** A `.tsuku.toml` can only suppress or narrow auto-updates,
never broaden them beyond what the global pin allows. The worst case is that a
malicious config suppresses all updates (by exact-pinning every tool), which is a
denial of updates rather than an escalation.

**Input validation.** For defense in depth, `effectivePin()` should call
`ValidateRequested` on the project version string before using it as a pin
constraint. `VersionMatchesPin` is safe with arbitrary strings, but validation
catches malformed values early and keeps the behavior consistent with how
`Requested` is validated elsewhere.

## Consequences

### Positive

- Teams can pin exact tool versions in project configs and trust that auto-update
  won't override them
- Prefix pins in project configs genuinely constrain auto-update boundaries, matching
  user expectations
- The mental model is consistent: `.tsuku.toml` is always the effective constraint
  when you're in the project directory

### Negative

- A narrower project pin can silently suppress updates the global pin would allow.
  No notification when this happens (debug logging is the future mitigation).
- Adds one more parameter to MaybeAutoApply (now 5 parameters). Functional but
  approaching the threshold where an options struct would be cleaner.

### Mitigations

- Silent suppression is the correct behavior per R17. A debug log message provides
  visibility for users who investigate.
- The parameter count is manageable for a function with one call site. If further
  parameters are needed in the future, refactoring to an options struct is
  straightforward.
