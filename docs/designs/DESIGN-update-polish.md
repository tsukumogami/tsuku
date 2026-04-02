---
status: Planned
upstream: docs/prds/PRD-auto-update.md
spawned_from:
  issue: 2186
  repo: tsukumogami/tsuku
problem: |
  tsuku outdated only shows versions within the pin boundary, hiding major
  releases outside the pin. There's no proactive notification when out-of-channel
  versions exist. And tsuku update requires one tool name at a time with no
  batch operation.
decision: |
  Add dual-column output to tsuku outdated (within-pin and overall). Add
  tsuku update --all for batch updates within pin boundaries. Add out-of-channel
  notifications in DisplayNotifications with per-tool weekly throttle using
  mtime dotfiles (.ooc-<tool>) and injectable clock via time.Time parameter.
rationale: |
  All three additions reuse existing infrastructure. The mtime dotfile throttle
  matches the proven .notified sentinel pattern. Per-tool files avoid JSON
  parsing and atomic-write concerns. The time.Time parameter avoids global
  mutable state for clock injection.
---

# DESIGN: Update polish

## Status

Planned

## Context and Problem Statement

The auto-update system's core is complete (Features 1-5), but three UX gaps remain. First, `tsuku outdated` only shows the latest version within the pin boundary. Users pinned to `node@20` don't see that Node 22 exists unless they check manually. The PRD calls for dual columns showing both "within pin" and "overall" versions (R15b).

Second, there's no proactive notification when a newer version exists outside the user's pin. A developer pinned to Go 1.21 gets patches automatically but has no idea Go 1.23 shipped months ago. Out-of-channel notifications (R13) solve this, but they need per-tool weekly throttling to avoid nagging. The throttle requires persistent state and an injectable clock for testing.

Third, `tsuku update` takes exactly one tool name. Updating all tools requires running the command once per tool. `tsuku update --all` (R14) is a simple batch operation that iterates installed tools and updates each within its pin boundary.

This is Feature 6 of the [auto-update roadmap](../roadmaps/ROADMAP-auto-update.md), implementing PRD requirements R13, R14, and R15b. All dependencies are complete: Feature 1 (version resolution), Feature 3 (auto-apply), and Feature 5 (notification system).

## Decision Drivers

- **Existing infrastructure.** The `UpdateCheckEntry` cache already has `LatestWithinPin` and `LatestOverall` fields. `outdated.go` resolves within-pin but ignores overall. The notification system (`ShouldSuppressNotifications`, `DisplayNotifications`) is in place.
- **Weekly throttle needs persistence.** Out-of-channel notifications must appear at most once per week per tool (R13). This requires storing the last notification timestamp per tool somewhere that survives across invocations.
- **Testability.** The weekly throttle's time dependency must be injectable for testing. Tests can't wait a week or manipulate system clocks.
- **Minimal new surface.** These are polish features, not new infrastructure. The design should reuse existing patterns (cache files, config keys, notification rendering) rather than introducing new concepts.
- **Backward compatibility.** `tsuku outdated` and `tsuku update` have existing users. New columns and flags shouldn't break scripts that parse the current output. JSON output is the stable contract; text output can change.

## Considered Options

### Decision 1: Out-of-channel throttle persistence and clock injection

When a tool is pinned to a version boundary (e.g., `node@20`), the background checker resolves both the latest within pin and overall. If a newer major version exists outside the pin, tsuku should show an out-of-channel notification -- but at most once per week per tool (R13). This requires persistent per-tool state and an injectable clock for testing.

The existing codebase uses file-based persistence patterns in `$TSUKU_HOME/cache/updates/`: per-tool JSON cache entries, a `.last-check` sentinel using mtime, and a `.notified` sentinel using mtime. Clock injection doesn't exist yet in the updates package.

Key assumptions:
- The number of pinned tools with out-of-channel versions will be small (under 20), so per-tool file I/O is fine.
- The `.notified` sentinel mtime pattern is proven in this codebase for time-based dedup.
- No concurrent writes to throttle state since notifications render in the foreground process.

#### Chosen: Per-tool throttle files with mtime

One dotfile per tool at `$TSUKU_HOME/cache/updates/.ooc-<tool>`, using the file's mtime as the "last notified" timestamp. To check: stat the file; if missing or mtime is older than 7 days, show the notification and touch the file.

```go
func IsOOCThrottled(cacheDir, toolName string, now time.Time) bool
func TouchOOCThrottle(cacheDir, toolName string) error
```

Clock injection uses a `now time.Time` parameter rather than a package-level variable. This matches the pattern in batch/seed packages (`calculateNextRetryAt`, `expireBackoff`) and avoids global mutable state that can leak between parallel tests.

The dotfile prefix (`.ooc-`) keeps throttle files invisible to `ReadAllEntries`, which already skips files starting with `.`.

#### Alternatives considered

**Single JSON throttle file.** A single `.out-of-channel.json` mapping tool names to timestamps. Rejected because it introduces JSON serialization for what's just a per-key timestamp, requires atomic read-modify-write, and the package-level `var NowFunc` approach for clock injection risks test pollution.

**Extend UpdateCheckEntry.** Add `OutOfChannelNotifiedAt` to the existing cache JSON. Rejected because it mixes background checker state (what versions exist) with foreground UI state (when we last told the user). Cache refresh could reset the throttle, causing unintended re-notification.

## Decision Outcome

**Chosen: Per-tool mtime throttle + existing infrastructure for outdated/batch**

### Summary

Three additions to the existing auto-update system, each building on what's already there.

`tsuku outdated` gains a second resolution pass per tool. After resolving `LatestWithinPin` via `ResolveWithinBoundary`, it also calls `provider.ResolveLatest` to get the overall latest. The text output adds an "OVERALL" column. The JSON output adds a `latest_overall` field alongside the existing `latest` field. The `--json` contract changes are additive (new field, no removed fields). Exact-pinned tools are still skipped.

`tsuku update --all` accepts `--all` as a flag instead of a positional argument. When set, it iterates all installed tools (excluding exact-pinned), resolves within-pin for each, and calls the existing `runInstallWithTelemetry` flow. It reuses the same pin-aware resolution from `tsuku update <tool>`. Failures for individual tools are reported but don't stop the batch. A summary line shows "Updated N/M tools" at the end.

Out-of-channel notifications render in `DisplayNotifications` (from the notification system, Feature 5). After showing apply results and unshown notices, `DisplayNotifications` checks each cache entry where `LatestOverall` differs from both `LatestWithinPin` and `ActiveVersion`. For each such tool, it calls `IsOOCThrottled(cacheDir, tool, time.Now())`. If not throttled, it prints a one-line notification (e.g., `node 22.0.0 available (pinned to 20.x)`) and touches the throttle file. The throttle uses per-tool dotfiles (`.ooc-<tool>`) in the cache directory, with mtime as the timestamp.

### Rationale

All three additions reuse existing code paths. The outdated display extends the same resolution loop that already runs. Batch update wraps the same single-tool flow. Out-of-channel notifications fit into the existing `DisplayNotifications` pipeline with the same suppression gate. The per-tool mtime throttle matches the `.notified` sentinel pattern already in use. No new packages, no new config file formats, no new persistence mechanisms.

## Solution Architecture

### Overview

Three changes to existing files, plus two new functions in `internal/updates/`.

### Components

```
cmd/tsuku/outdated.go (MODIFIED)
  +-- Adds ResolveLatest call for overall version
  +-- Adds "OVERALL" column to text output
  +-- Adds "latest_overall" to JSON output

cmd/tsuku/update.go (MODIFIED)
  +-- Adds --all flag
  +-- Iterates installed tools when --all is set

internal/updates/throttle.go (NEW)
  +-- IsOOCThrottled(cacheDir, toolName, now) bool
  +-- TouchOOCThrottle(cacheDir, toolName) error

internal/updates/notify.go (MODIFIED)
  +-- renderOutOfChannelNotifications(cacheDir, userCfg, now)
  +-- Called from DisplayNotifications when notify_out_of_channel is enabled
```

### Key interfaces

**Throttle** (`internal/updates/throttle.go`):

```go
const OOCThrottleDuration = 7 * 24 * time.Hour
const OOCFilePrefix = ".ooc-"

// IsOOCThrottled returns true if the tool's out-of-channel notification
// was shown within the last 7 days.
func IsOOCThrottled(cacheDir, toolName string, now time.Time) bool

// TouchOOCThrottle creates or updates the throttle file for the tool.
func TouchOOCThrottle(cacheDir, toolName string) error
```

**Outdated output** (extended `updateInfo` struct):

```go
type updateInfo struct {
    Name         string `json:"name"`
    Current      string `json:"current"`
    Latest       string `json:"latest"`
    LatestOverall string `json:"latest_overall,omitempty"`
}
```

**Batch update** (new flag on updateCmd):

```go
var updateAllFlag bool
// updateCmd.Flags().BoolVar(&updateAllFlag, "all", false, "Update all tools within pin boundaries")
```

### Data flow

1. Background checker already writes `LatestWithinPin` and `LatestOverall` to cache entries
2. `tsuku outdated` reads both fields from cache or resolves live, displays dual columns
3. `tsuku update --all` iterates tools, resolves within-pin for each, installs
4. `DisplayNotifications` reads cache entries, checks `LatestOverall` against active version and pin, calls `IsOOCThrottled`, renders if not throttled, touches throttle file

## Implementation Approach

### Phase 1: Pin-aware outdated display

Add overall version resolution to `outdated.go`. Second `ResolveLatest` call per tool. Add "OVERALL" column to text output, `latest_overall` to JSON. The background checker already populates `LatestOverall` in cache entries, but `outdated` resolves live rather than reading cache (for freshness).

Deliverables:
- Edit `cmd/tsuku/outdated.go`
- Tests for new JSON output format

### Phase 2: Batch update

Add `--all` flag to `tsuku update`. When set, iterate installed tools (same list as `outdated`), skip exact-pinned, resolve within-pin, install. Report per-tool success/failure and a summary line.

Deliverables:
- Edit `cmd/tsuku/update.go`
- Edit `cmd/tsuku/update_test.go`

### Phase 3: Out-of-channel notifications

Add `IsOOCThrottled` and `TouchOOCThrottle` in `internal/updates/throttle.go`. Extend `DisplayNotifications` in `notify.go` to call `renderOutOfChannelNotifications` when `updates.notify_out_of_channel` is enabled. Read cache entries, filter for tools where `LatestOverall` differs from both within-pin and active, check throttle, render, touch.

Deliverables:
- `internal/updates/throttle.go` (new)
- `internal/updates/throttle_test.go` (new)
- Edit `internal/updates/notify.go`
- Edit `internal/updates/notify_test.go`
- Functional test scenarios

## Security Considerations

These are UI-only changes to existing data flows. No new external inputs, network calls, or permission changes. The throttle files are dotfiles in a user-owned directory. Out-of-channel notifications display version numbers that are already in the cache -- no new data exposure.

## Consequences

### Positive

- Users see the full version landscape, not just what's within their pin
- `tsuku update --all` replaces manual per-tool update loops
- Out-of-channel notifications are proactive but not annoying (weekly throttle)
- All three features reuse existing infrastructure with no new packages or patterns

### Negative

- `tsuku outdated` makes a second version resolution call per tool (doubles network requests for live checks)
- The `cache/updates/` directory gains dotfiles that could accumulate if tools are uninstalled without cleanup
- Text output of `tsuku outdated` changes layout (wider table), which could break fragile text-parsing scripts

### Mitigations

- The double resolution is bounded by the tool count and runs in the foreground (user expects to wait). Cache entries from the background checker often have `LatestOverall` pre-populated, so the second call may hit cache.
- `tsuku remove` can clean up `.ooc-<tool>` files alongside cache entries. Orphans are harmless (small dotfiles, stat-only access).
- JSON output is the stable contract (`--json`). Text output is explicitly not guaranteed stable. The PRD specifies the dual-column format.

## Implementation Issues

PLAN: `docs/plans/PLAN-update-polish.md` (single-pr mode)

| Issue | Dependencies | Tier |
|-------|--------------|------|
| [#2186: update polish](https://github.com/tsukumogami/tsuku/issues/2186) | [#2181](https://github.com/tsukumogami/tsuku/issues/2181), [#2184](https://github.com/tsukumogami/tsuku/issues/2184), [#2185](https://github.com/tsukumogami/tsuku/issues/2185) | testable |
| _Pin-aware outdated display, batch update --all, and out-of-channel notifications with weekly per-tool throttle. Single-pr implementation with 3 internal issues tracked in PLAN doc._ | | |
