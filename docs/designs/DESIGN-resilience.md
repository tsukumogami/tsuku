---
status: Accepted
upstream: docs/prds/PRD-auto-update.md
spawned_from:
  issue: 2187
  repo: tsukumogami/tsuku
problem: |
  The auto-update system writes a failure notice for every failed update attempt,
  regardless of whether the failure is transient (network blip) or persistent
  (broken recipe). Old version directories accumulate indefinitely after updates.
  And when the network is unavailable, the background checker produces error
  entries that clutter the cache. These rough edges erode trust in auto-update.
decision: |
  Extend the Notice struct with a ConsecutiveFailures counter. MaybeAutoApply
  increments the counter on failure and resets on success. Notices with fewer
  than 3 consecutive failures are suppressed in DisplayNotifications. Add a
  version GC pass in MaybeAutoApply that removes version directories older than
  a configurable retention period (default 7 days), protecting the active and
  rollback versions. The background checker already handles offline gracefully
  by writing error entries that auto-apply skips. Doctor gains checks for
  orphaned staging dirs and notices older than 30 days.
rationale: |
  The counter-on-Notice approach avoids new files or state -- it extends the
  existing per-tool notice pattern. GC in MaybeAutoApply runs at the natural
  point where old versions become irrelevant (after a successful update).
  Offline degradation needs no new code -- the checker's error-entry pattern
  already means auto-apply silently skips tools when checks fail.
---

# DESIGN: Resilience

## Status

Proposed

## Context and Problem Statement

The auto-update system (Features 1-6) handles the happy path well, but three real-world rough edges remain.

**Transient failure noise.** Every failed auto-update writes a notice and displays it to the user. A single network timeout during the background check produces a failure notice that shows up on the next command. Most users will see this as a bug in tsuku rather than a temporary network issue. The PRD (R11) specifies that failures with fewer than 3 consecutive occurrences for the same tool should be treated as transient and suppressed.

**Version directory accumulation.** Each version of a tool lives in `$TSUKU_HOME/tools/<name>-<version>/`. After several auto-updates, old version directories pile up. The multi-version model makes rollback fast (no re-download), but without garbage collection, disk usage grows unbounded. The PRD (R18) requires retaining the previous version for at least one cycle (configurable, default 7 days) and GC'ing older ones.

**Offline noise.** When the network is unavailable, the background checker writes error entries to cache files. These errors are harmless (auto-apply skips entries with non-empty `Error` fields), but `tsuku doctor` should detect stale notices and orphaned staging directories left behind by interrupted operations.

This is Feature 7 of the [auto-update roadmap](../roadmaps/ROADMAP-auto-update.md), implementing PRD requirements R11 (consecutive-failure suppression), R18 (old version retention with GC), and R20 (graceful offline degradation).

## Decision Drivers

- **Existing notice infrastructure.** The `Notice` struct in `internal/notices/notices.go` already tracks per-tool failures. Extending it with a counter is the lowest-friction approach.
- **MaybeAutoApply as the natural GC point.** After a successful auto-apply, the previous version becomes the rollback target and older versions become GC candidates. Running GC here avoids a separate cron or background process.
- **Rollback safety.** GC must never delete the active version or the `PreviousVersion` (rollback target). The `ToolState` struct already tracks `PreviousVersion`.
- **Offline already works.** The background checker writes errors to cache entries when resolution fails. `MaybeAutoApply` skips entries with non-empty `Error` fields. No new code is needed for the core offline path -- just verification and doctor integration.
- **Actionable errors bypass suppression.** R11 specifies that checksum mismatches, disk-full, and recipe incompatibility errors should produce notices immediately, even on the first occurrence. The suppression is only for transient failures.

## Considered Options

### Decision 1: Consecutive-failure tracking

When an auto-update fails, the system needs to decide: is this transient (suppress) or persistent (notify)? The PRD says fewer than 3 consecutive failures = transient. The question is where to store the failure count.

Key assumptions:
- Failures are per-tool, not global. Tool A's network timeout doesn't affect tool B's counter.
- A successful update for a tool resets its counter to zero.
- "Actionable" errors (checksum mismatch, disk full, recipe parse error) skip the counter and always produce a notice.

#### Chosen: Extend Notice with ConsecutiveFailures counter

Add a `ConsecutiveFailures int` field to the `Notice` struct. When `MaybeAutoApply` encounters a failure:
1. Read the existing notice for the tool (if any)
2. If the error is actionable (checksum, disk, recipe), write the notice with `ConsecutiveFailures = 3` (forces display)
3. Otherwise, increment the counter: `ConsecutiveFailures = existing + 1`
4. Write the notice (always, for logging)
5. Set `Shown = ConsecutiveFailures < 3` (pre-mark as shown if below threshold)

`DisplayNotifications` renders notices where `Shown == false`, which naturally skips suppressed ones. On the 3rd consecutive failure, `Shown` is false and the notice displays.

When a tool updates successfully, remove its notice file (already happens -- `RemoveEntry` after successful apply).

#### Alternatives considered

**Separate counter file per tool.** One `.failures-<tool>` file with just a count. Rejected because the Notice struct already exists per-tool and adding a field is simpler than managing a parallel file set. Two files per tool (notice + counter) adds complexity for no benefit.

**In-memory counter with persistence on shutdown.** Track counts in a map during the process lifetime, write to disk at exit. Rejected because tsuku commands are short-lived processes -- the counter would reset on each invocation. File-based persistence per invocation is the only viable approach.

### Decision 2: Version GC scope and trigger

Old version directories need cleanup. The question is what triggers GC, what's protected, and how the retention period works.

Key assumptions:
- The active version and PreviousVersion (rollback target) must never be GC'd.
- Version directories follow the `<name>-<version>` naming convention in `$TSUKU_HOME/tools/`.
- GC runs during foreground tsuku commands, not in the background checker.

#### Chosen: Post-apply GC in MaybeAutoApply

After a successful auto-apply for a tool, scan `$TSUKU_HOME/tools/` for directories matching `<tool>-*`. For each:
1. Skip if it's the active version directory
2. Skip if it's the PreviousVersion directory
3. Check the directory's mtime against the retention period (configurable via `updates.version_retention`, default 7 days)
4. Remove if older than the retention period

GC runs per-tool after each successful apply, not as a global sweep. This keeps the scope narrow and the duration short. A global `tsuku gc` command is out of scope for this feature.

The retention period is configurable via `updates.version_retention` in config.toml (Go duration format, e.g., "7d" or "168h"). Default is 7 days.

#### Alternatives considered

**Global GC command (`tsuku gc`).** A separate command that sweeps all tools. Rejected for this feature because auto-apply is the natural trigger -- old versions become irrelevant right after a new one installs. A manual command can be added later if users want it.

**Background GC in the checker process.** Run GC during the detached background check. Rejected because the background checker shouldn't mutate the tools directory -- it's a read-only resolution process. Mixing concerns makes the checker harder to reason about.

## Decision Outcome

**Chosen: Notice counter + post-apply GC**

### Summary

Three changes, each targeted at a specific rough edge.

For consecutive-failure suppression, `Notice` gains a `ConsecutiveFailures` field. `MaybeAutoApply` reads the existing notice before writing, increments the counter on transient failures, and pre-marks notices below the threshold as shown. Actionable errors (classified by pattern-matching the error string for "checksum", "disk", "recipe") bypass the counter. `DisplayNotifications` already skips shown notices, so no rendering changes are needed.

For version GC, `MaybeAutoApply` calls a new `GarbageCollectVersions(toolsDir, toolName, activeVersion, previousVersion, retention, now)` function after each successful apply. It lists directories matching `<tool>-*`, skips active and previous, and removes those with mtime older than the retention period. A new `updates.version_retention` config key controls the period (default "168h" = 7 days).

For offline degradation, no new code is needed in the core path. The background checker already writes error entries when resolution fails, and `MaybeAutoApply` already skips entries with errors. The design adds two doctor checks: orphaned staging directories (leftover temp files in `$TSUKU_HOME/tools/` matching `.staging-*`) and stale notices (files in `$TSUKU_HOME/notices/` older than 30 days).

### Rationale

These changes are minimal extensions to existing infrastructure. The Notice counter avoids new file types. Post-apply GC runs at the natural cleanup point. The offline path already works and just needs verification via doctor checks. Each sub-feature is independently shippable.

## Solution Architecture

### Overview

Changes to three existing packages plus a new config key. No new packages.

### Components

```
internal/notices/notices.go (MODIFIED)
  +-- Notice gains ConsecutiveFailures field

internal/updates/apply.go (MODIFIED)
  +-- MaybeAutoApply: read existing notice, increment counter, classify errors
  +-- MaybeAutoApply: call GarbageCollectVersions after successful apply

internal/updates/gc.go (NEW)
  +-- GarbageCollectVersions(toolsDir, toolName, active, previous, retention, now)

internal/userconfig/userconfig.go (MODIFIED)
  +-- UpdatesVersionRetention() config accessor

cmd/tsuku/doctor.go (MODIFIED)
  +-- Check for orphaned staging dirs
  +-- Check for stale notices
```

### Key interfaces

```go
// internal/updates/gc.go
func GarbageCollectVersions(toolsDir, toolName, activeVersion, previousVersion string, retention time.Duration, now time.Time) error

// internal/notices/notices.go (extended struct)
type Notice struct {
    Tool                string    `json:"tool"`
    AttemptedVersion    string    `json:"attempted_version"`
    Error               string    `json:"error"`
    Timestamp           time.Time `json:"timestamp"`
    Shown               bool      `json:"shown"`
    ConsecutiveFailures int       `json:"consecutive_failures,omitempty"`
}

// internal/updates/apply.go (new helper)
func isActionableError(err error) bool  // pattern-matches checksum, disk, recipe errors

// internal/userconfig/userconfig.go
func (c *Config) UpdatesVersionRetention() time.Duration
```

### Data flow

1. Background checker runs, writes cache entries (errors on network failure = offline degradation)
2. `MaybeAutoApply` reads entries, skips those with errors (offline path)
3. For each pending update, attempts install
4. On failure: reads existing notice, increments counter (or forces display for actionable errors), writes notice
5. On success: removes notice, calls `GarbageCollectVersions` for the tool
6. `DisplayNotifications` renders notices where `Shown == false` (counter >= 3 or actionable)
7. `tsuku doctor` checks for `.staging-*` dirs and notices older than 30 days

## Implementation Approach

### Phase 1: Consecutive-failure suppression

Add `ConsecutiveFailures` field to `Notice`. Update `MaybeAutoApply` to read existing notice before writing, increment counter on transient failures, set `Shown = true` for counter < 3. Add `isActionableError` helper. Tests for counter logic and actionable error classification.

Deliverables:
- Edit `internal/notices/notices.go`
- Edit `internal/updates/apply.go`
- `internal/updates/apply_test.go` (new test cases)

### Phase 2: Version GC

Add `GarbageCollectVersions` in `internal/updates/gc.go`. Add `updates.version_retention` config key. Call GC from `MaybeAutoApply` after successful apply. Tests for GC with active/previous protection and retention logic.

Deliverables:
- `internal/updates/gc.go` (new)
- `internal/updates/gc_test.go` (new)
- Edit `internal/userconfig/userconfig.go`
- Edit `internal/updates/apply.go`

### Phase 3: Doctor integration and functional tests

Add doctor checks for orphaned staging dirs and stale notices. Add functional test scenarios for the complete resilience features.

Deliverables:
- Edit `cmd/tsuku/doctor.go`
- `test/functional/features/resilience.feature` (new)

## Security Considerations

These are hardening changes with no new external inputs. GC deletes directories that tsuku itself created, scoped to `$TSUKU_HOME/tools/`. The deletion path validates the directory name matches `<tool>-<version>` format before removing. No new network access, permissions, or data exposure.

## Consequences

### Positive

- Users don't see noise from transient network failures (3-strike rule filters most)
- Disk usage stays bounded as old versions are automatically cleaned up
- `tsuku doctor` catches leftover artifacts from interrupted operations
- The offline path silently degrades without new code -- existing error-entry pattern is correct

### Negative

- The ConsecutiveFailures counter adds a JSON field to every notice file (backward compatible via `omitempty`)
- GC removes old versions, making manual rollback to versions older than PreviousVersion impossible without reinstalling
- The 7-day default retention might be too short for users who rollback infrequently

### Mitigations

- The `omitempty` tag means existing notice files without the field parse correctly (counter defaults to 0, treated as first failure)
- The retention period is configurable. Users who need longer retention can set `updates.version_retention = "720h"` (30 days)
- `tsuku doctor` warns about conditions, it doesn't auto-fix them
