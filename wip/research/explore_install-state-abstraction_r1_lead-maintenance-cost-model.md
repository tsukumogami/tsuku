# Lead: What's the maintenance cost of the current shape under realistic future scenarios?

## Findings

### Sub-question 1: Decompose PR #2412's churn

PR #2412 changed 36 files (+3955, -289). I read every diff in `git show 880ed188` and bucketed each file by the reason it changed. Three reasons matter for the cost question: **bus-introduction** (would happen exactly once, for any cross-cutting refactor), **direct-write removal** (would happen anyway once the bus exists — these files get smaller), and **Source threading** (the cross-cutting parameter the issue worries about).

| File | LOC (+/-) | Bucket | Why |
|------|-----------|--------|-----|
| `internal/installevents/bus.go` | +225 / 0 | Bus introduction | New package |
| `internal/installevents/events.go` | +156 / 0 | Bus introduction | New package |
| `internal/installevents/bus_test.go` | +392 / 0 | Bus introduction (test) | New package |
| `internal/notices/subscriber.go` | +139 / 0 | Subscriber | New code |
| `internal/notices/subscriber_test.go` | +287 / 0 | Subscriber (test) | New code |
| `internal/telemetry/subscriber.go` | +93 / 0 | Subscriber | New code |
| `internal/telemetry/subscriber_test.go` | +211 / 0 | Subscriber (test) | New code |
| `cmd/tsuku/events_wiring.go` | +33 / 0 | Bus wiring | New file: per-process bus constructor |
| `internal/install/manager.go` | +167 / -14 | Manager hub | Adds `WithEventBus`, `publishInstallOutcome`, `Rollback`, `Source` on `InstallOptions`. About 40 LOC is Source threading (signature changes on `Install` and `Rollback`, `Source` field on `InstallOptions`, default-source plumbing). The remaining ~120 LOC is event publishing logic that would exist in any consolidated abstraction. |
| `internal/install/remove.go` | +60 / -6 | Manager hub | Adds Source param to `Remove`/`RemoveVersion`/`RemoveAllVersions` plus `publishRemoveOutcome`. About 25 LOC is Source threading; ~35 LOC is publish logic. |
| `internal/install/manager_events_test.go` | +343 / 0 | Manager test | New event-publish tests |
| `internal/install/manager_events_e2e_test.go` | +198 / 0 | Manager test | New end-to-end tests |
| `internal/install/remove_test.go` | +15 / -14 | Test adapt | Existing tests updated for Source param |
| `internal/notices/notices.go` | +21 / 0 | Schema change | `Verb` field added; `RemoveNotice` validation hardened |
| `internal/notices/notices_test.go` | +15 / 0 | Schema change (test) | `RemoveNotice` validation tests |
| `internal/updates/notify.go` | +62 / -16 | Renderer | Verb-aware message formatting (required by schema change) |
| `internal/updates/notify_test.go` | +103 / 0 | Renderer (test) | Per-verb phrasing tests |
| `internal/updates/apply.go` | +20 / -43 | Direct-write removal | Net **shrinks** by 23 LOC: telemetry+notice direct calls removed; subscriber handles them. |
| `internal/updates/apply_test.go` | +19 / -40 | Direct-write removal (test) | Net shrinks by 21 LOC |
| `internal/updates/self.go` | +31 / -13 | Direct-write removal | Direct `notices.WriteNotice` removed; publishes events instead. Net +18 LOC because it now also publishes failure (new behavior). |
| `internal/updates/checker.go` | +8 / -2 | Bus threading | `RunUpdateCheck` signature gains `bus` param |
| `internal/updates/checker_test.go` | +2 / -2 | Bus threading (test) | Test calls updated |
| `cmd/tsuku/update.go` | +17 / -42 | Direct-write removal | Net **shrinks** by 25 LOC: removes telemetry and notice direct calls |
| `cmd/tsuku/install_deps.go` | +23 / -45 | Mixed | Net shrinks by 22 LOC. `runInstallWithTelemetry` renamed to `runInstall` and gains `src` param (Source threading); `clearAndRecordInstallSuccess` becomes a no-op (-30 LOC, direct-write removal). |
| `cmd/tsuku/install_deps_test.go` | +10 / -29 | Test adapt | Net shrinks |
| `cmd/tsuku/cmd_rollback.go` | +9 / -6 | Direct-write removal + Source | Drops the direct `tc.SendUpdateOutcome` call (now in subscriber); adds Source arg to `mgr.Rollback`. Net +3 LOC. |
| `cmd/tsuku/install.go` | +6 / -5 | Source threading only | 6 call sites updated from `runInstallWithTelemetry(...)` to `runInstall(..., installevents.SourceManual)` + 1 import |
| `cmd/tsuku/install_project.go` | +3 / -2 | Source threading only | 2 call sites + import |
| `cmd/tsuku/install_lib.go` | +3 / -2 | Source threading only | Adds `src` param to `installLibrary`; 1 call site forwarding + import |
| `cmd/tsuku/cmd_apply_updates.go` | +2 / -1 | Source threading only | Renames `runInstallWithExternalReporter` to `runInstallWithReporter` and passes `SourceAuto` + import |
| `cmd/tsuku/cmd_check_updates.go` | +6 / -1 | Bus threading | Constructs bus, passes to `RunUpdateCheck` |
| `cmd/tsuku/cmd_run.go` | +2 / -1 | Source threading only | One call site + import |
| `cmd/tsuku/create.go` | +2 / -1 | Source threading only | One call site + import |
| `cmd/tsuku/eval.go` | +2 / -1 | Source threading only | One call site + import |
| `cmd/tsuku/remove.go` | +4 / -3 | Source threading only | 3 call sites + import |
| `docs/designs/current/DESIGN-notices-install-event-bus.md` | +1266 / 0 | Docs | Design doc |

**Aggregated by bucket:**

| Bucket | File count | LOC (net) | Counts toward "would happen anyway"? |
|--------|------------|-----------|---------------------------------------|
| Bus introduction (package + wiring file) | 4 | +806 | Yes — one-time cost of any consolidation |
| Subscribers (notices + telemetry) | 4 | +730 | Yes — would happen exactly once |
| Manager hub additions (publish logic) | 2 | +173 net | Yes — would happen once in any abstraction |
| Schema change (`Verb` on notices) | 2 | +36 | Could have been a separate PR; not Source-threading related |
| Renderer (Verb-aware messages) | 2 | +149 | Required by schema change; not Source-threading |
| Direct-write removal (pure shrinkage) | 5 | -78 net | Yes — would happen anyway, net negative |
| **Source threading (pure)** | **7** | **+22 net** | **No — this is the cross-cutting cost** |
| Bus threading (closely related to Source) | 4 | +21 net | Partial — one-shot for this PR |
| Manager-side Source threading inside hub files | (overlap with hub) | ~65 LOC inside manager.go and remove.go | The cross-cutting cost inside the Manager itself |
| Test adapt / test new | 7 | +1101 | Mix; most would happen anyway |
| Docs | 1 | +1266 | One-time |

**The headline number: pure Source-threading touched 7 cmd/tsuku files for a net +22 LOC.** Add the Manager-side threading (about 65 LOC inside `manager.go` and `remove.go`) and the bus-threading on the `RunUpdateCheck`/`CheckAndApplySelf` signatures (~30 LOC across 4 files). **Total cross-cutting cost: 11 files, roughly 120 LOC of pure parameter plumbing.**

The design predicted "~10 call sites for Source threading." Reality was 7 cmd/tsuku files with 12 distinct `runInstall*` call sites updated (install.go has 6 alone), plus the function signature changes. The prediction was directionally right; the absolute count is in the 7-12 range depending on whether you count files or call sites.

### Sub-question 2: Cost model — adding `--dry-run`

What `--dry-run` needs to touch in the current shape:

1. CLI flag definitions on `tsuku install`, `tsuku update`, `tsuku remove`, `tsuku rollback`, `tsuku update --all`, and `tsuku apply-updates`. That's **6 cmd/tsuku files** (flag wiring at the cobra command level).
2. Propagation through the call chain. The chain looks like: cobra `Run` → `runInstall` → `installWithDependencies` → `installLibrary` → `mgr.InstallWithOptions`. Each link is a function signature change. That's `cmd/tsuku/install_deps.go` (3 internal helpers), `cmd/tsuku/install_lib.go` (1 helper), `cmd/tsuku/cmd_apply_updates.go` (calls `runInstallWithReporter`), `cmd/tsuku/cmd_rollback.go` (calls `mgr.Rollback`), `cmd/tsuku/remove.go` (calls `mgr.Remove*`).
3. Inside the Manager: `InstallWithOptions` (or an addition to `InstallOptions`), `Rollback`, `RemoveVersion`, `RemoveAllVersions`, `Remove`. Plus the short-circuit logic inside each. That's `internal/install/manager.go` + `internal/install/remove.go`.
4. Subscribers: telemetry must NOT send a real event on dry-run (or must send a `dry-run` flag). Notices must NOT write to disk (or must write to a side channel). Likely add a `DryRun` field on every event in `internal/installevents/events.go` and gate the subscribers on it. That's `internal/installevents/events.go` + both subscribers.

**File count estimate: 13-15 files, roughly 150-200 LOC.** The shape of the change mirrors PR #2412 closely: same set of files (cmd entry points + helpers + Manager + subscribers).

The work splits identically to Source threading. Every file that took a `Source` param now takes a `Source, DryRun bool` pair (or, more likely, gets refactored to a struct that carries both — at which point the per-concern cost drops sharply).

### Sub-question 3: Cost model — adding `context.Context` for cancellation

Today the Manager does NOT accept `ctx`:
- `Manager.Install(name, version, workDir, src)` — no ctx
- `Manager.InstallWithOptions(name, version, workDir, opts)` — no ctx
- `Manager.Rollback(name, toVersion, src)` — no ctx
- `Manager.RemoveVersion(name, version, src)` — no ctx
- `Manager.RemoveAllVersions(name, src)` — no ctx
- `Manager.Remove(name, src)` — no ctx
- `Manager.Activate(name, version)` — no ctx

Adding `ctx context.Context` as the first parameter:

1. **Manager methods**: 7 signatures in `internal/install/manager.go` + `internal/install/remove.go`. Each needs ctx threading down to whatever blocking ops happen inside (file I/O via `os.Rename`, network calls in actions). Internal helpers like `createSymlink`, `createBinarySymlink`, `createSymlinksForBinaries`, `createWrappersForBinaries`, `createBinaryWrapper`, `collectLibraryPaths`, `executeCleanupActions` — about 7 more internal methods. So 14 method signatures inside the Manager.
2. **Call sites of `install.New(...).Method(...)`**: I counted earlier — `mgr.Install` in `plan_install.go`, `cmd_shim.go`, `install_deps.go`; `mgr.InstallWithOptions` in `plan_install.go`, `install_deps.go`; `mgr.Rollback` in `cmd_rollback.go`; `mgr.RemoveVersion`, `mgr.RemoveAllVersions`, `mgr.Remove` in `remove.go`; `mgr.Activate` in `apply.go` and `cmd_rollback.go`. That's roughly **8 cmd/tsuku files + 2 internal/updates files = 10 call sites**.
3. **The wrappers above the Manager**: `runInstall`, `runInstallWithReporter`, `installWithDependencies`, `installLibrary`, `installFn` in `apply.go`. Each needs a ctx param.
4. **Tests**: Every test that calls a Manager method needs `context.Background()` (or a cancellable ctx for cancellation tests). Roughly 5-10 test files.

**File count estimate: 15-20 files, 150-250 LOC.** Higher than Source threading because ctx penetrates deeper into Manager internals (file I/O loops, sub-process calls in actions), not just the public surface.

Importantly: a context.Context cannot ride the event bus. Cancellation is a request-scoped value that flows top-down; the bus is a bottom-up notification channel. **The bus does not eliminate the ctx cost.** This is the most expensive cross-cutting concern of the three.

### Sub-question 4: Cost model — adding structured audit logging

A JSON line per state mutation to `$TSUKU_HOME/log/audit.log`.

**Via the bus (free):** Write a third subscriber, `internal/audit/subscriber.go`, that handles the same events the notices and telemetry subscribers already handle. Wire it in `cmd/tsuku/events_wiring.go::newEventBus`. Total cost: **2 files, ~150 LOC** (subscriber + tests), one of which is new. **Zero changes to Manager, zero changes to wrappers, zero changes to cmd/tsuku entry points.**

**Via direct threading (high cost):** An `AuditLogger` field threaded through every place a state mutation happens. Plus calls at every mutation site. Roughly **10-15 files** — the same shape as the original direct-write problem the bus solved.

This is the clearest case where the bus has already eliminated the cross-cutting cost. Telemetry + notices were the original ride-along passengers; audit logging is a free third one. **If audit logging is on the roadmap, it's a 2-file change.**

### Sub-question 5: Compare against the "do nothing" baseline

If no new cross-cutting concerns appear in the next year, the cost of the current shape is **zero**. There is no ongoing maintenance burden from the existing Source param or bus wiring — they're stable code paths.

The cost model only matters under the assumption that 3-5 plausible cross-cutting concerns will show up. Lead 2 is enumerating plausibility; I can name the candidates I see in this repo:

- **`--dry-run`** — plausible. Useful for `tsuku update --all` preview and project-install previews.
- **`context.Context` for cancellation** — plausible. The codebase already accepts ctx at many entry points but stops at the Manager.
- **Audit logging** — speculative. No issue I found requests it.
- **Quiet / verbose flag propagation** — partially done via `progress.Reporter`; the cross-cutting cost is already paid in a different way.
- **Future `Source` values** (e.g., `SourceReinstall`, `SourceRecover`) — plausible. The enum was designed to extend; new values add zero per-call-site cost.

Three of these are plausible; one is speculative; one is already handled. **The base rate of cross-cutting concerns in this codebase appears to be 1-2 per year, not 3-5.**

### Sub-question 6: 5-concern projection

Assume 5 cross-cutting concerns over the next year, each shaped like Source threading (worst case for the current shape):

| Concern | Current shape (files) | Consolidated shape (files, assuming 50% reduction) |
|---------|----------------------|----------------------------------------------------|
| `--dry-run` | 13-15 | 6-8 |
| `context.Context` | 15-20 | 8-10 (ctx penetrates deeper; bus can't carry it) |
| Audit logging | 2 (free via bus) | 2 (no improvement) |
| New `Source` value | 1-2 (just adds enum value + call sites pass it) | 1-2 (no improvement) |
| One unknown (rate-limit, retry policy, etc.) | 13-15 (guess based on shape) | 6-8 |

**Current-shape total: ~45-55 file touches across 5 concerns.**
**Consolidated-shape total: ~23-30 file touches.**

The 50% reduction assumption is generous. The bus has already eliminated the cost for one of the five concerns (audit). For the other four:
- Source enum extension is already cheap.
- `--dry-run` and a hypothetical fifth concern are the only ones where consolidation would actually help.
- `context.Context` is the most expensive and the consolidation doesn't help much because ctx flows top-down through call paths, not bottom-up through event hubs.

**The honest projection: consolidation helps for 2 of 5 plausible concerns, saving roughly 10-20 file touches over a year.**

## Implications

The issue body says "PR #2412 touched too many files." The data partly contradicts this framing:

- 16 files changed for things that **would happen exactly once for any abstraction** (bus introduction, subscribers, schema change, renderer, design doc).
- 5 files changed in the **direction of less code** (direct-write removal — these files shrank by 78 LOC net).
- 11 files changed because of the **actual cross-cutting cost** (Source threading + bus threading). Net cost: about 120 LOC of plumbing.

If you discount the one-time costs (which are by definition not recurring), the cross-cutting cost of PR #2412 is closer to **11 files / 120 LOC**, not 36 files / 3955 LOC.

The issue's premise has a kernel of truth — Source threading really did touch 7 cmd files for trivial reasons (import + replace `runInstallWithTelemetry(...)` with `runInstall(..., installevents.SourceManual)`). But under a 5-concerns-per-year assumption, the lifetime saving from consolidating is roughly 10-20 file touches. Against the cost of designing, reviewing, and migrating to a new abstraction (which is itself ~15-20 files, comparable to PR #2412 again), the ROI is marginal unless the concern rate is much higher than the data suggests.

**Pushback on the issue's premise: the file count is real but the marginal cost per concern is small (~7 file touches for a Source-shaped concern, ~10-15 for a deeper one). The big PR sizes are dominated by one-time abstraction costs, not by cross-cutting plumbing.**

## Surprises

1. **Net LOC was negative for 5 of the 36 files.** `cmd/tsuku/update.go` shrank by 25 LOC. `internal/updates/apply.go` shrank by 23 LOC. `cmd/tsuku/install_deps.go` shrank by 22 LOC. `cmd/tsuku/install_deps_test.go` shrank by 19 LOC. The bus PR removed more code from the direct-write paths than it added in Source threading.
2. **The bus already eliminates the audit logging cost.** A future audit subscriber is a 2-file change. This is the clearest evidence that the bus is doing useful cross-cutting work — at least one future concern is now free.
3. **`context.Context` is the expensive scenario, and the bus can't help it.** Cancellation flows top-down; subscribers can't intercept it. If ctx threading is a real near-term need, that's an independent design problem that consolidation doesn't solve.
4. **Source threading in cmd/tsuku is genuinely cheap per file.** Six of the seven cmd/tsuku Source-threading files had net deltas of +2 to +6 lines (just import + one or two arg additions). The annoyance factor of touching 7 files is larger than the LOC cost of those touches.
5. **The design doc itself is 1266 lines.** The doc dominated the PR's LOC count more than any code file.

## Open Questions

1. **What's the actual rate of cross-cutting concerns per year?** I projected 1-2 based on what I can see in the repo. Past data (number of PRs that touched 10+ files for a single cross-cutting reason) would calibrate this. Needs a human's read on the roadmap.
2. **Is `context.Context` threading on the near-term roadmap?** If yes, the cost is high regardless of consolidation. If no, the audit logging precedent is the strongest argument for consolidation.
3. **Is the issue's complaint about file count actually about cognitive load / review effort, not about LOC?** Reviewing a 7-file Source threading change is annoying even if each file changed 2 lines. A consolidated abstraction might be valued for the smaller cognitive footprint of future changes, not for LOC saved. I can't measure that from code.
4. **Would the existing `InstallOptions` struct (already in the codebase) absorb future cross-cutting concerns at lower cost?** `InstallOptions` already has 7 fields. Adding `DryRun`, `Ctx`, `AuditTag` to it is a 1-file Manager change; the caller-side cost is unchanged. This is a cheaper consolidation than a wholesale state abstraction.

## Summary

PR #2412's 36-file count decomposes into 16 one-time-cost files (bus, subscribers, schema, renderer, docs), 5 net-negative direct-write-removal files, and only 11 files representing the actual cross-cutting Source-threading cost — about 120 LOC of plumbing. The bus has already eliminated the per-concern cost for one future cross-cutting need (audit logging is now a 2-file subscriber, not a 10-file thread); but the most expensive plausible concern (`context.Context`) cannot ride the bus, so a state abstraction wouldn't help it much, and over 5 plausible concerns the lifetime saving is roughly 10-20 file touches against an upfront design+migration cost comparable to PR #2412 itself. The biggest open question is whether the issue's complaint is really about LOC (which the numbers don't support as a strong case) or about cognitive load of touching many files for trivial parameter additions — a question only a human roadmap-holder can answer.
