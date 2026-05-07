<!-- decision:start id="notification-routing-interactive-vs-background" status="confirmed" -->
### Decision: Inbox writing is background-only; interactive path uses ttyReporter only

**Context**

tsuku has two execution paths for updates. The interactive path (`tsuku update <tool>`) runs with a live TTY and uses `ttyReporter`, which writes `Warn()` and `DeferWarn()` calls to stderr in real time. The background path (`apply-updates` subprocess) has no terminal; its `ttyReporter` writes to `/dev/null`, silently discarding all events.

The design introduces `InboxReporter` — a `progress.Reporter` implementation that routes `Warn()`/`DeferWarn()` calls to `notices.WriteNotice()` instead of stderr. The question is whether the interactive path should also write to the inbox, either universally (fanout for all warnings) or selectively (fanout only for high-value Kinds like `KindVersionFallback` and `KindShellInitChange`).

The routing decision lives at reporter construction time, not at the call site. This is a fixed constraint from the overall design: `reporter.Warn()` is called identically in both paths; only the Reporter implementation differs.

**Assumptions**

- `InboxReporter` is constructed once per execution context (background subprocess). The interactive path constructs `ttyReporter` as it does today.
- `warnShellInitChanges` in `update.go` will be migrated from `fmt.Fprintf(os.Stderr, ...)` to `reporter.Warn(...)` as a prerequisite — this migration is required regardless of which option is chosen, and its routing then follows from this decision.
- The `tsuku notices` command's intended UX is "events you couldn't see at the terminal," not "a log of all events."

**Chosen: Inbox-only for background (Option A)**

The interactive path continues to use `ttyReporter` only. The inbox (`$TSUKU_HOME/notices/`) is written exclusively by the background path via `InboxReporter`. Warnings emitted during explicit `tsuku update` are visible inline on the terminal and are not persisted. The routing decision is: background replaces the terminal sink; it does not supplement it.

**Rationale**

The user's own framing was precise: background events are "re-routed to the inbox," not "also written to the inbox." Re-routing is replacement, not duplication. An interactive user sees warnings as they happen — version fallback ("installed X-1 instead of X") and shell init changes are visible in the terminal output and require no further surfacing.

Writing to the inbox on the interactive path would invert the inbox's value proposition. `tsuku notices` is meant to surface events the user couldn't see. If every interactive `tsuku update` also writes to the inbox, `tsuku notices` becomes a log of things the user already saw — eroding its signal-to-noise ratio and making it harder to distinguish "missed background event" from "thing you watched happen."

Selective fanout (Option C) requires Kind-awareness at reporter construction time, which partially bleeds the routing logic back into the caller — exactly what the "routing decision not at call site" constraint is meant to prevent. It also requires ongoing judgment about which Kinds "deserve" persistence on the interactive path, adding maintenance surface.

Option A is the simplest implementation: `ttyReporter` for interactive, `InboxReporter` for background. No fanout types needed. The separation is clean, testable, and matches the user's intent.

**Alternatives Considered**

- **fanoutReporter for interactive (Option B)**: Interactive path uses both `ttyReporter` and `InboxReporter` in parallel; all warnings visible inline are also persisted. Rejected because it muddies the inbox's "missed events" semantics — the user already saw the warning at the terminal. It also adds implementation complexity (a fanout type) with no clear user benefit for the interactive case.

- **Selective Kind-based fanout (Option C)**: Interactive path fans out only for high-value Kinds (`KindVersionFallback`, `KindShellInitChange`), leaving routine warnings TTY-only. Rejected because it requires the caller to be Kind-aware at construction time, which conflicts with the "routing decision not at call site" constraint. It also creates a maintenance burden: deciding which Kinds warrant interactive persistence is a judgment call that will need revisiting as new Kinds are introduced.

**Consequences**

- `InboxReporter` is used only in the background path (`cmd_apply_updates.go`). The interactive path needs no changes to its reporter construction.
- `warnShellInitChanges` must be refactored to call `reporter.Warn()` instead of writing to `os.Stderr` directly. This is a prerequisite for correct routing regardless of this decision, and it's the natural cleanup that makes the architecture consistent.
- `tsuku notices` retains clear semantics: it shows events that happened while the user wasn't watching (background auto-apply results, version fallbacks during background updates, shell init changes detected during background updates).
- If a version fallback or shell init change occurs during interactive `tsuku update`, the user sees it inline. No notice is written — the event is considered acknowledged.
- Future additions: if a new Kind warrants interactive persistence (e.g., a security advisory), a fanout type can be introduced at that point with a concrete use case. The architecture doesn't foreclose it; it just doesn't add the complexity speculatively.
<!-- decision:end -->
