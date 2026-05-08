---
status: Current
problem: |
  When tsuku runs in background auto-update mode, all warnings and non-fatal events
  from the install engine are silently dropped because the subprocess uses a reporter
  that writes to /dev/null. Users never learn that a version fallback occurred or that
  their tool installed a different version than expected. The fix is a context-aware
  notification routing system: the same reporter call routes to the terminal when
  running interactively, and to the notices inbox when running in the background.
decision: |
  Add InboxReporter — a new progress.Reporter implementation that accumulates
  warnings in memory and writes a single notice to $TSUKU_HOME/notices/<tool>.json
  on Stop(). The background auto-apply subprocess constructs this reporter via
  runInstallWithExternalReporter in cmd_apply_updates.go, replacing the silent
  ttyReporter-against-devnull. Version fallback detection lives in
  GitHubArchiveAction.Decompose, which signals the fallback by calling
  ctx.GetReporter().Warn() via a new Reporter field on EvalContext — mirroring
  the existing ExecutionContext.Reporter pattern. The Kind field in the Notice struct
  becomes the lifecycle routing key. Version fallback for static asset patterns is
  descoped to a follow-on. The interactive path continues to use ttyReporter with
  no inbox writes.
rationale: |
  The progress.Reporter interface is already dependency-injected throughout the
  install engine, making InboxReporter a zero-call-site-change swap at a single
  construction point. Flush-on-Stop accumulation preserves all warnings in one
  atomic write, avoiding the silent information loss of overwrite-on-each-warn.
  Adding Reporter to EvalContext mirrors ExecutionContext.Reporter and handles
  concurrent fallbacks without extra locking. Background-only inbox writing
  preserves the notices inbox's meaning — events the user couldn't see — without
  turning it into a general event log.
---

# DESIGN: Notification Routing

## Status

Current

## Context and Problem Statement

tsuku has two execution contexts for updates: interactive (`tsuku update <tool>`, with a terminal) and background auto-apply (the `apply-updates` subprocess, no terminal). Today, the background path constructs a TTY reporter against `/dev/null`, silently discarding all warnings, non-fatal errors, and progress. Users who rely on background auto-apply have no visibility into events like "version fallback occurred" or "shell init changed" unless an install fails outright.

The notices system (`internal/notices/`) already handles failure persistence: background failures write JSON files to `$TSUKU_HOME/notices/` and surface on the next interactive command via `DisplayNotifications`. But this coverage is narrow — only hard failures reach the inbox. Non-fatal events are lost entirely.

The design addresses three related gaps:

1. **Silent background warnings**: The `apply-updates` subprocess needs a reporter implementation that routes `Warn`/`DeferWarn` calls to the notices inbox rather than a terminal.

2. **Success notices never written**: `renderUnshownNotices` already handles `n.Error == ""` notices (displaying "X updated to Y"), but `MaybeAutoApply` never writes a success notice. The display half exists; the write half doesn't.

3. **Version fallback with no user signal**: When `github_archive` picks a release whose asset doesn't exist and falls back to a previous version, no notice is produced. Users don't know they're running an older version than expected.

The principle: any event worth showing inline during an interactive update is worth recording in the inbox for the background path. The execution channel determines the sink; the same call site works in both contexts.

## Decision Drivers

- **No duplicate logic**: Reporter call sites in the install engine must not branch on "is this interactive?". The routing decision lives at reporter construction time, not at each call site.
- **Backward compatibility**: Existing notice files on disk (no `Kind` field set, `Error` field for lifecycle) must continue to display and clear correctly.
- **Atomic per-tool notice**: The current schema stores one JSON file per tool. Multi-warn events during a single install need a clear accumulation strategy.
- **Fallback correctness**: Version fallback during `Decompose` creates a stale `UpdateCheckEntry.LatestWithinPin` in the background checker's cache. The design must address cache staleness.
- **Lifecycle explicitness**: The current `Error != ""` convention for persistent vs. single-view is fragile. The `Kind` field should become the lifecycle routing key.

## Considered Options

### Decision 1: InboxReporter Warning Accumulation and Notice Schema

The `notices/` package stores one JSON file per tool at `$TSUKU_HOME/notices/<tool>.json`. The `Notice` struct carries `Tool`, `AttemptedVersion`, `Error`, `Timestamp`, `Shown`, and `Kind`. Display logic in `renderUnshownNotices` branches on `n.Error == ""` (success vs. failure); the `Kind` field exists but currently drives no behavior.

A new `InboxReporter` must implement `progress.Reporter` so the background subprocess can route `Warn()`/`DeferWarn()` calls to disk instead of the current `/dev/null` sink. During a single install, multiple warn calls are realistic: a PATH hint via `DeferWarn`, a version fallback notice via `Warn` during plan generation, and possible shell cache rebuild warnings. With the one-file-per-tool schema, the accumulation strategy determines whether earlier warnings survive to display.

#### Chosen: Flush-on-Stop accumulation

`InboxReporter` holds all `Warn()` and `DeferWarn()` messages in an in-memory slice. On `Stop()`, it writes a single `Notice` to disk via `notices.WriteNotice()`. Deferred messages are flushed into the slice before the write, in order (immediate warns first, then deferred warns). One file write per install run; one notice per tool per run.

The `Notice` struct gains one field:

```go
Messages []string `json:"messages,omitempty"`
```

`Kind` becomes the lifecycle routing key in `renderUnshownNotices`. New Kind constants:

| Constant | Value | Lifecycle |
|----------|-------|-----------|
| `KindUpdateResult` | `""` | Existing: persistent on error, single-view on success |
| `KindAutoApplyResult` | `"auto_apply_result"` | Existing: persistent on error, single-view on success |
| `KindVersionFallback` | `"version_fallback"` | New: single-view regardless of Error |
| `KindShellInitChange` | `"shell_init_change"` | New: single-view |

When a background install succeeds but includes a version fallback warning, `InboxReporter` escalates the Kind from `KindAutoApplyResult` to `KindVersionFallback` at `Stop()` time if any such message was accumulated. Both Kind values share single-view lifecycle; the escalation affects display classification only.

`FlushDeferred()` on `InboxReporter` transfers deferred messages into the accumulation slice without writing to disk. `Stop()` is the write trigger because the caller defers it after `FlushDeferred()` — writing on `FlushDeferred()` would miss messages added in between.

#### Alternatives Considered

**Last-warn-wins overwrite**: Each `Warn()`/`DeferWarn()` call immediately writes/overwrites the notice file. Rejected because earlier warnings are silently discarded when multiple fire in sequence — a version fallback notice would be lost if a PATH hint fires afterward.

**Severity-gated single write**: Write only the highest-priority warning per run, determined by a Kind priority ranking. Rejected because the ranking is a maintained invariant that silently breaks when new Kind values are added without updating the ranking, and it discards actionable information even when all warnings deserve surfacing.

---

### Decision 2: Version Fallback Signal from Decompose

`GitHubArchiveAction.Decompose` returns `([]Step, error)`. When it falls back from version X to X-1 because `FetchReleaseAssets` finds no matching asset, the install succeeds — returning a non-nil error is wrong. Yet the caller needs to know about the fallback to surface it to the user. Three out-of-band mechanisms were evaluated.

The constraint is that `Decompose` can't change its signature, a successful fallback must not be treated as an error, and zero call-site changes are required in `actions/`, `executor/`, and `install/`.

#### Chosen: Reporter field on EvalContext

Add a `Reporter progress.Reporter` field to `EvalContext` and a `GetReporter()` helper (returning `NoopReporter{}` when nil). When `GitHubArchiveAction.Decompose` detects a version fallback, it calls `ctx.GetReporter().Warn(...)`. Callers that want notice routing pass an `InboxReporter`; callers that don't (validation pipelines, eval paths) supply nothing and `GetReporter()` discards the call silently.

This mirrors the existing `ExecutionContext.Reporter` pattern used in `Execute()` methods throughout `internal/actions/`. Multiple fallbacks within a single plan each fire `Warn()` independently and are captured by the accumulation strategy from Decision 1. The field is concurrent-safe at no extra cost since reporter implementations already use a mutex.

#### Alternatives Considered

**PlanConfig.OnWarning callback**: The callback lives in `PlanConfig`, not `EvalContext`. To reach `Decompose`, it must be threaded into `EvalContext` anyway — equivalent structural cost but a weaker string-only API. All callers that currently omit `OnWarning` would need to add closures. Rejected: equivalent change surface, weaker semantics.

**EvalContext output field (FallbackInfo struct)**: Mutating a context object as an output channel is unconventional Go. A single field is overwritten on multiple fallbacks (last-write wins). Callers must poll the field after every `Decompose` call — easy to miss when adding new callers. Rejected: unconventional, fragile for multiple fallbacks, error-prone to extend.

---

### Decision 3: Version Fallback for Static Asset Patterns

`github_archive` has two execution paths based on `asset_pattern`. Wildcard patterns trigger `FetchReleaseAssets` in `Decompose`, detecting a missing asset at plan time. Static patterns construct the download URL directly and hit a 404 only at actual download time — well after plan generation completes.

In practice, static pattern recipes use per-platform `when` clauses to select a fixed asset name that's stable across releases (deno, eza, ripgrep, ollama follow this pattern). When a static pattern 404s, the cause is typically a transient release problem, not a structural naming mismatch that fallback would fix.

#### Chosen: Descope static patterns (v1 wildcard-only)

Implement version fallback only for wildcard asset patterns in v1. Static pattern 404s continue to surface as download errors. This is consistent with the "fallback belongs in `Decompose`" constraint: `Decompose` is the only point with both `ctx.Resolver` and asset existence awareness, and static patterns make no asset API call there.

Static patterns make up roughly 5% of the recipe registry. Their 404s are typically transient release problems that the existing retry logic handles once assets are uploaded. Deferring gives the wildcard path time to prove the approach before extending it.

#### Alternatives Considered

**Proactive HEAD check in Decompose**: For static patterns, `Decompose` makes an HTTP HEAD request to verify asset existence before committing to the version. Rejected because this adds a network round-trip to every static-pattern install for a rare failure mode; HEAD responses also don't guarantee body availability on all CDNs.

**Retry on download 404**: The download executor detects 404, signals the install manager to re-plan with X-1, and retries. Rejected because it requires the executor to be aware of version fallback semantics and the install manager to accept mid-execution re-plan requests — violating the zero call-site changes constraint and introducing significant cross-layer complexity.

---

### Decision 4: Interactive vs. Background Inbox Writing

tsuku has two execution paths. The interactive path (`tsuku update <tool>`) runs with a live TTY; `ttyReporter` writes `Warn()` calls to stderr in real time. The background path (`apply-updates` subprocess) has no terminal; its reporter writes to `/dev/null`.

The design's core insight — routing decision at reporter construction time, not at call sites — means whether to persist warnings to the inbox is determined by which reporter is constructed, not by the `reporter.Warn()` call itself.

#### Chosen: Background-only inbox writing

The interactive path continues to use `ttyReporter` only. Inbox writes happen exclusively in the background path via `InboxReporter`. Warnings emitted during explicit `tsuku update` are visible inline at the terminal and are not persisted to `$TSUKU_HOME/notices/`.

The user's framing was precise: background events are "re-routed to the inbox" — replacement, not supplementation. An interactive user sees warnings as they happen and doesn't need them surfaced again via `tsuku notices`. If every interactive `tsuku update` also writes to the inbox, `tsuku notices` becomes a log of things the user already saw, eroding its signal: "events you couldn't see."

This is also the simplest implementation — `ttyReporter` for interactive, `InboxReporter` for background, no fanout types.

#### Alternatives Considered

**fanoutReporter for interactive (always persist)**: Interactive path uses both `ttyReporter` and `InboxReporter` in parallel. Rejected because it inverts the inbox's purpose — the user already saw the warning — and adds implementation complexity for no clear user benefit.

**Selective Kind-based fanout**: Interactive path fans out only for high-value Kinds (`KindVersionFallback`, `KindShellInitChange`). Rejected because it requires the caller to be Kind-aware at reporter construction time, conflicting with the "routing decision not at call site" constraint, and creates ongoing maintenance burden as new Kinds are added.

---

## Decision Outcome

**Chosen: D1 Flush-on-Stop + D2 Reporter-on-EvalContext + D3 Wildcard-only + D4 Background-only**

### Summary

The background `apply-updates` subprocess gains an `InboxReporter` in place of the current ttyReporter-against-devnull. `InboxReporter` implements `progress.Reporter` and accumulates all `Warn()` and `DeferWarn()` calls in an in-memory slice. When `Stop()` is called at the end of the install, it writes one `Notice` to `$TSUKU_HOME/notices/<tool>.json` with a `Messages []string` field containing all accumulated warnings. If a version fallback message was accumulated, the notice Kind is escalated to `KindVersionFallback`; otherwise it defaults to `KindAutoApplyResult`. The swap point is the existing `runInstallWithExternalReporter` entry point in `cmd_apply_updates.go` — no changes to `actions/`, `executor/`, or `install/`.

Version fallback detection is added inside `GitHubArchiveAction.Decompose` (wildcard patterns only). When `FetchReleaseAssets` returns no matching asset for the requested version, `Decompose` retries with preceding versions via `ctx.Resolver.ListGitHubVersions`, selects the first version with a matching asset, and calls `ctx.GetReporter().Warn(...)` with a structured fallback message. This requires one new field on `EvalContext` — `Reporter progress.Reporter`, with a `GetReporter()` nil-safe accessor — mirroring `ExecutionContext.Reporter`. Callers that generate plans for the background install path pass the same `InboxReporter` through both the executor and the plan generator; other callers (validation pipelines, eval paths) are unaffected because `GetReporter()` returns a `NoopReporter{}` when nil.

The `Kind` field in `Notice` becomes the lifecycle routing key in `renderUnshownNotices` and `RemoveNotice` calls. `KindVersionFallback` and `KindShellInitChange` are single-view (cleared after first display via `RemoveNotice`); `KindAutoApplyResult` with `Error == ""` is also single-view, preserving backward compatibility. Existing notice files with no `Kind` field continue to use the `Error != ""` convention. `MaybeAutoApply` gains a success-notice write on the success branch (currently missing despite `renderUnshownNotices` already handling `n.Error == ""` notices). The `warnShellInitChanges` function is migrated from direct `fmt.Fprintf(os.Stderr, ...)` to `reporter.Warn(...)` to route correctly in both contexts.

Static asset patterns (non-wildcard `github_archive` recipes) are out of scope for v1 fallback. Their 404s continue to surface as download errors. Version fallback for static patterns is documented as a known gap.

### Rationale

`progress.Reporter` is the right seam because it's already dependency-injected through the entire install stack; swapping the implementation at one construction point requires no changes anywhere else. Flush-on-Stop is the right accumulation strategy because it produces one atomic write per run and preserves all warnings without silent information loss. Reporter-on-EvalContext mirrors `ExecutionContext.Reporter`, handles multiple concurrent fallbacks without extra locking, and requires no closure wiring at call sites. Background-only inbox writing preserves `tsuku notices` as a "missed events" surface rather than a general event log.

The cache staleness tradeoff — `UpdateCheckEntry.LatestWithinPin` records X, but X-1 was installed — is accepted for this design. The next background check will see X still as "latest within pin" and attempt X again. If X still has no assets, `Decompose` will fall back again and a new notice will fire. This creates a benign loop until upstream fixes the release; it's preferable to fetching asset lists during the background check phase, which would add significant latency to every background check run.

## Solution Architecture

### Overview

Five existing components are extended; one new component is added. The routing decision is encoded entirely at reporter construction time — the install engine calls `reporter.Warn(...)` identically in both execution contexts, and the reporter implementation determines the sink.

```
Interactive path:
  tsuku update <tool>
    └─ ttyReporter ──> stderr (visible at terminal)

Background path:
  apply-updates subprocess
    └─ InboxReporter ──> $TSUKU_HOME/notices/<tool>.json (surfaced on next interactive command)
```

### Components

**`internal/progress/inbox_reporter.go`** (new)

`InboxReporter` implements `progress.Reporter`. It accumulates `Warn()` and `DeferWarn()` messages in memory and writes a single `Notice` to disk on `Stop()`. `Status()` and `Log()` are no-ops (no terminal, no log sink). `FlushDeferred()` moves deferred messages to the immediate slice without writing to disk.

```go
type InboxReporter struct {
    toolName   string
    noticesDir string
    mu         sync.Mutex
    immediate  []string
    deferred   []string
}

func (r *InboxReporter) Stop() {
    r.mu.Lock()
    msgs := append(r.immediate, r.deferred...)
    r.immediate = nil
    r.deferred = nil
    r.mu.Unlock()

    if len(msgs) == 0 {
        return  // no warnings: nothing to write; success notice is written separately
    }
    kind := KindAutoApplyResult
    for _, m := range msgs {
        if strings.Contains(m, "version_fallback:") {  // structured prefix
            kind = KindVersionFallback
            break
        }
    }
    _ = notices.WriteNotice(r.noticesDir, &notices.Notice{
        Tool:      r.toolName,
        Timestamp: time.Now(),
        Shown:     false,
        Kind:      kind,
        Messages:  msgs,
    })
}
```

**`internal/notices/notices.go`** (modified)

`Notice` struct gains `Messages []string`. New Kind constants:

```go
const (
    KindUpdateResult   = ""                  // existing: lifecycle via Error != ""
    KindAutoApplyResult = "auto_apply_result" // existing: lifecycle via Error != ""
    KindVersionFallback = "version_fallback"  // new: always single-view
    KindShellInitChange = "shell_init_change" // new: always single-view
)
```

`renderUnshownNotices` (in `internal/updates/notify.go`) gains Kind-based dispatch. For Kind values with "always single-view" lifecycle, it calls `notices.RemoveNotice` after display instead of `notices.MarkShown`. For backward-compatible zero-value Kinds, the existing `Error != ""` convention continues.

**`internal/actions/composites.go`** (modified)

`GitHubArchiveAction.Decompose` gains a fallback loop for wildcard patterns. After `FetchReleaseAssets` returns no matching asset for the resolved version:

1. Call `ctx.GetReporter().Warn("version_fallback: installed %s instead of %s (no asset for %s)", fallback, requested, requested)` — the `version_fallback:` prefix is detected by `InboxReporter.Stop()` for Kind escalation.
2. Retry via `ctx.Resolver.ListGitHubVersions` to find the most recent previous version with a matching asset.
3. Return the fallback version's steps; set `AttemptedVersion` on the steps to the originally-requested version.
4. If no fallback is available, return the original error.

Static patterns (no wildcard in `asset_pattern`) are not affected in v1.

**`internal/executor/eval.go`** (modified)

`EvalContext` gains a `Reporter` field:

```go
type EvalContext struct {
    Resolver   VersionResolver
    PlanConfig PlanConfig
    Reporter   progress.Reporter  // nil-safe: use GetReporter() to access
    // ... existing fields
}

func (ctx EvalContext) GetReporter() progress.Reporter {
    if ctx.Reporter == nil {
        return progress.NoopReporter{}
    }
    return ctx.Reporter
}
```

Callers that generate plans for the background install path set `EvalContext.Reporter` to the same `InboxReporter` used for the install. Other callers (validation pipelines, `tsuku eval`) omit the field; `GetReporter()` returns `NoopReporter{}`.

**`cmd/tsuku/cmd_apply_updates.go`** (modified)

Constructs `InboxReporter` instead of `ttyReporter`:

```go
reporter := progress.NewInboxReporter(toolName, notices.NoticesDir(cfg.HomeDir))
return runInstallWithExternalReporter(ctx, cfg, entry, reporter)
```

The `InboxReporter` is passed as the `Reporter` on the `EvalContext` constructed inside `runInstallWithExternalReporter`. `Stop()` is deferred by the existing pattern in `runInstallWithTelemetry`.

**`internal/updates/apply.go`** (modified)

`MaybeAutoApply` writes a success notice on the success branch (line 149 currently only calls `RemoveNotice` for failure cleanup):

```go
_ = notices.RemoveNotice(noticesDir, entry.Tool)
_ = notices.WriteNotice(noticesDir, &notices.Notice{
    Tool:             entry.Tool,
    AttemptedVersion: entry.LatestWithinPin,
    Timestamp:        time.Now(),
    Shown:            false,
    Kind:             notices.KindAutoApplyResult,
})
```

This write happens inside `runInstallWithExternalReporter`, before the deferred `reporter.Stop()`. The ordering is deliberate: `InboxReporter.Stop()` runs after and overwrites this notice only if warnings accumulated. If no warnings fired, `Stop()` returns early and the success notice from apply.go is preserved. This means warnings always win (they include `Messages`), and the success-only path gets the simple success notice. Both notices set `AttemptedVersion` so the installed version is always visible.

**`cmd/tsuku/update.go`** (modified)

`warnShellInitChanges` migrated from `fmt.Fprintf(os.Stderr, ...)` to `reporter.Warn(...)`. The `reporter` is already passed through to the update path; this is a drop-in replacement that routes correctly through the reporter's sink.

### Data Flow

**Background path (with version fallback):**

```
apply-updates subprocess
  │
  ├─ construct InboxReporter(toolName, noticesDir)
  │
  └─ runInstallWithExternalReporter(reporter)
       │
       ├─ GeneratePlan(EvalContext{Reporter: reporter})
       │    └─ GitHubArchiveAction.Decompose
       │         ├─ FetchReleaseAssets → no match for vX
       │         ├─ ListGitHubVersions → try vX-1
       │         ├─ FetchReleaseAssets(vX-1) → match found
       │         └─ ctx.GetReporter().Warn("version_fallback: vX-1 instead of vX")
       │              └─ InboxReporter.Warn() → appends to r.immediate
       │
       ├─ Execute(plan) → install succeeds
       │
       ├─ apply.go: RemoveNotice(tool) + WriteNotice(success, Kind=KindAutoApplyResult)
       │
       └─ defer reporter.Stop()
            └─ InboxReporter.Stop()
                 ├─ msgs = ["version_fallback: vX-1 instead of vX"]
                 ├─ kind = KindVersionFallback (escalated)
                 └─ WriteNotice(tool, Kind=KindVersionFallback, Messages=[...])
                      └─ overwrites success notice from apply.go (correct)

Next interactive command:
  DisplayNotifications → renderUnshownNotices
    └─ KindVersionFallback notice → display + RemoveNotice (single-view)
```

**Background path (success, no warnings):**

```
  └─ InboxReporter.Stop() → msgs=[] → early return (no write)
  └─ apply.go success notice preserved on disk
  └─ Next interactive command: "tool updated to vX" displayed + RemoveNotice
```

## Implementation Approach

### Phase 1: Notice Schema and Kind Taxonomy

Lay the foundation: extend the `Notice` struct and formalize Kind-based lifecycle dispatch. No behavior changes to the install engine.

Deliverables:
- `internal/notices/notices.go`: add `Messages []string` field; add `KindVersionFallback` and `KindShellInitChange` constants
- `internal/updates/notify.go`: update `renderUnshownNotices` to route single-view Kinds to `RemoveNotice` after display; preserve `Error != ""` convention for `KindUpdateResult`
- Unit tests: verify backward-compatible deserialization (no `kind` field, no `messages` field)

### Phase 2: InboxReporter

Implement the new reporter. At this point it can be unit-tested in isolation; it's not yet wired into the background path.

Deliverables:
- `internal/progress/inbox_reporter.go`: `InboxReporter` struct, all six `Reporter` interface methods, flush-on-Stop logic, Kind escalation via structured `version_fallback:` prefix
- Unit tests: verify multi-warn accumulation, FlushDeferred ordering, Stop() early-return on empty slice, Kind escalation

### Phase 3: EvalContext.Reporter and Version Fallback in Decompose

Wire the signal path for fallback detection. Depends on Phase 2 (needs InboxReporter) and Phase 1 (needs `KindVersionFallback`).

Deliverables:
- `internal/executor/eval.go` (or wherever `EvalContext` is defined): add `Reporter progress.Reporter` field and `GetReporter()` helper
- `internal/actions/composites.go`: fallback retry loop in `GitHubArchiveAction.Decompose` for wildcard patterns; call `ctx.GetReporter().Warn(...)` on fallback; return fallback version's steps
- Tests: synthetic release scenario where vX has no asset but vX-1 does; verify fallback succeeds and Warn fires

### Phase 4: Background Wiring and Success Notice

Connect all pieces to the background path. Depends on Phase 2 and Phase 3.

Deliverables:
- `cmd/tsuku/cmd_apply_updates.go`: construct `InboxReporter` and pass to `runInstallWithExternalReporter`; pass reporter to `EvalContext` inside plan generation
- `internal/updates/apply.go`: write success notice on success branch in `MaybeAutoApply`
- `cmd/tsuku/update.go`: migrate `warnShellInitChanges` from `fmt.Fprintf(os.Stderr)` to `reporter.Warn(...)`
- Integration test: end-to-end background apply path produces expected notice on disk

## Security Considerations

### Tool Name Path Validation (Medium)

`WriteNotice` constructs the output path as `filepath.Join(noticesDir, notice.Tool+".json")` without validating `notice.Tool` for path separators. The new `InboxReporter.Stop()` write path inherits this gap. If a malformed tool name were written to the update cache (e.g., `tool = "../bin/evil"`), `filepath.Join` would resolve it outside `$TSUKU_HOME/notices/`.

The implementation must add an explicit check before the `filepath.Join` call in `WriteNotice`:

```go
if strings.ContainsAny(notice.Tool, "/\\") || notice.Tool == ".." {
    return fmt.Errorf("invalid tool name for notice path: %q", notice.Tool)
}
```

This closes a theoretical path-traversal window. Recipe names are validated kebab-case when added to the registry, so the practical exposure is low, but the guard is a one-line addition with no downside.

### ANSI Sanitization in InboxReporter (Low-Medium)

Existing `progress.Reporter` implementations pass output through `SanitizeDisplayString()` before writing to the terminal. `InboxReporter` writes to a JSON file rather than a terminal, so skipping sanitization at accumulation time seems harmless. However, messages stored in the notice file are later rendered by `renderUnshownNotices` via `fmt.Fprintf(os.Stderr, ...)`. Without sanitization, recipe-sourced strings containing ANSI sequences survive through the notice file to the terminal on the next interactive command.

The implementation must call `progress.SanitizeDisplayString` on each message before accumulation in `InboxReporter.Warn()` and `InboxReporter.DeferWarn()`, consistent with other reporter implementations.

### Secrets Exclusion Contract (Informational)

The existing `progress.Reporter` contract prohibits passing secrets (API tokens, credentials) to any reporter method. `InboxReporter` persists messages to disk, making this requirement more important than for TTY reporters where output is transient. The Warn call sites in `GitHubArchiveAction.Decompose` pass only version strings and asset names — no secrets are involved. The `InboxReporter` implementation must include a code comment preserving this constraint so future Warn call sites don't inadvertently pass sensitive values that end up in notice files.

### Message Accumulation Bound (Low)

`InboxReporter` has no bound on accumulated messages. An unusual install emitting many warnings could write an oversized notice file. Cap accumulated messages at a reasonable limit (50 messages, each truncated to 512 characters) to prevent this edge case from producing a notice file that is expensive to display or store.

## Consequences

### Positive

- Background auto-apply warnings (version fallback, shell init changes) are now surfaced to users via `tsuku notices` instead of being silently dropped.
- Success notices confirm that background auto-apply ran and installed a new version — previously missing from the inbox.
- `Kind` becomes a meaningful field that routes display and lifecycle behavior, enabling future notice types without touching the `Error` convention.
- Zero call-site changes in the install engine. All `actions/`, `executor/`, and `install/` code is unaffected; routing is transparent to business logic.
- The `warnShellInitChanges` migration makes shell-init-change detection consistent across execution contexts.

### Negative

- `InboxReporter.Stop()` writes a notice only when warnings accumulated. If the success notice from `apply.go` and the InboxReporter both write, the InboxReporter's `WriteNotice` overwrites the apply.go notice. If ordering is wrong (apply.go writes after InboxReporter.Stop()), the success notice clobbers warnings. This requires careful ordering discipline.
- Cache staleness: `LatestWithinPin` records version X, but X-1 was installed. The next background check will queue X again. If the release is permanently broken, users see a version fallback notice on every background check cycle.
- Static pattern recipes get no fallback in v1. Users of deno, eza, ripgrep, and similar static-pattern tools won't benefit from fallback detection.
- The `version_fallback:` structured prefix in `Warn()` messages is an informal convention. If another Warn message happens to contain that string, `InboxReporter.Stop()` will incorrectly escalate the Kind.

### Mitigations

- Ordering discipline: `apply.go` success-notice write executes inside `runInstallWithExternalReporter` before the deferred `Stop()`. The defer guarantee ensures InboxReporter.Stop() always runs after, with its overwrite semantics producing the correct result. This is documented explicitly in the implementation.
- Cache staleness loop is benign: once upstream fixes the release (uploads the missing asset), the next background check finds a healthy asset and installs it without fallback. The loop terminates naturally.
- Static pattern limitation is documented as a known gap. The wildcard path proves the approach; follow-on work can add HEAD checks for static patterns once the real-world failure frequency justifies it.
- The structured prefix convention is internal to `InboxReporter` and not part of the public `Reporter` interface. It can be replaced with a typed warning mechanism (e.g., `WarnKind(kind, format, ...)`) in a follow-on without breaking call sites.
