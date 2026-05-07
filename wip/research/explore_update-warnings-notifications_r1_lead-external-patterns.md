# Lead: External patterns for context-aware notification routing

## Findings

### npm / update-notifier: spawn-and-persist

npm's update notification is extracted into the `update-notifier` package (sindresorhus), used by npm itself and many Node CLI tools. The pattern has three discrete phases:

1. **Check phase** - spawned as an unref'd child process so the parent can exit without waiting. The child writes the result to `~/.config/configstore/update-notifier-<module>.json` with two fields: `optOut` (bool) and `lastUpdateCheck` (unix ms timestamp). The actual update availability result is also stored so the next invocation doesn't need to re-check.

2. **Display phase** - on the next command invocation the parent reads the configstore file, loads the stored update availability into `.update`, and if conditions are met, prints to stderr. Display is suppressed when: no TTY on stdout/stderr, `CI=true`, `NO_UPDATE_NOTIFIER` env var is set, `--no-update-notifier` flag is used, or the user has set `optOut: true`.

3. **Rate limiting** - the first run never shows a notification even if an update is available, to avoid startling users on first install. The `updateCheckInterval` (default 24h) gates when re-checks occur.

Key property: the check and display are fully decoupled across process invocations. The check writes to disk; the display reads from disk. The parent process never blocks on the check.

pnpm initially used `update-notifier` but dropped it (issue #2752) because pnpm already had the registry client code internally. Rather than take on the external dependency, they built their own state persistence mechanism - the same architectural shape, just owned internally.

### npm/pnpm TTY suppression specifics

Both tools suppress notifications when stdout is not a TTY. The reasoning: if stdout is being piped (e.g., `$(npm pack --dry-run)`) the advisory text would corrupt the captured output. Stderr is the chosen channel for advisory output, but even stderr gets suppressed in non-interactive sessions to avoid breaking scripts that redirect stderr.

This is exactly the problem Homebrew hit and fixed in PR #10501: `update-report` was writing to stdout regardless of TTY state. The fix was: if not a TTY, redirect to stderr; otherwise use stdout. This kept command substitutions clean.

### Homebrew: collect-then-flush for caveats

Homebrew uses a deferred collection pattern for caveats (PR #4361). During a multi-formula install or upgrade, each formula's caveats are:
1. Shown inline during that formula's installation step (so users notice them while scrolling)
2. Also collected into a `Messages` class array

At the end of the entire build, all collected caveats are re-emitted together in a consolidated `==> Caveats` section. This "dual display" approach ensures nothing is lost in long scroll-back while also giving users an end-of-run summary.

The homebrew-autoupdate tap (the official background auto-update tool) takes a different approach: it uses macOS launchd to run updates and delivers results via native macOS notification banners (formerly via `terminal-notifier`, now via AppleScript applets). There is no concept of "defer to next interactive command" - the OS notification layer handles surfacing results asynchronously. On Linux/non-macOS this pattern is unavailable.

### rustup: proposed two-phase timeout with TTY gate

rustup issue #3688 (February 2024) describes a proposed but not yet shipped design for background update checking. The proposal:

- When `rustup` or `cargo` is invoked, start a background check with a short timeout (~100-200ms)
- If the check completes within the timeout: display the notice immediately before the command exits
- If the check times out: store the result and display it "on the next invocation"
- Rate limit: show at most once per hour
- TTY gate: show notices only "when executed directly by the user in a terminal", not when "executed programmatically"
- For cargo specifically: the proposal acknowledges that printing from the rustup proxy after `cargo output` requires coordination - one option is a special env var or CLI flag to route notices through cargo's output machinery rather than printing directly, to avoid notices getting buried mid-output

This is the closest existing design to what tsuku needs: explicit TTY detection, explicit "defer to next invocation" path, and acknowledgment that the display problem for cargo (a tool that wraps another tool) is harder than the display problem for rustup itself.

### dnf-automatic: pluggable emitter registry

dnf-automatic has the most formal "sink" abstraction of any tool examined. Its `[emitters]` configuration section defines a list of named emitters specified in `emit_via`:

- `stdio` - print to standard output
- `email` - send via SMTP
- `motd` - write to `/etc/motd`
- `command` - pipe to a custom shell command
- `command_email` - send email via a command

The `emit_via` field accepts a comma-separated list, enabling simultaneous delivery to multiple sinks. This is the most explicit "event routing" model found: the same update result event can be sent to stdio AND email AND motd in a single run. A `send_error_messages` flag additionally controls whether the error path activates all emitters or only some.

The `motd` emitter is particularly relevant as an analog to tsuku's notices file: it writes background-process results to a location that gets surfaced to users on their next login (via the system MOTD mechanism). This is the exact pattern tsuku uses - write to file in background, display on next interactive invocation.

### Go slog / zerolog: handler composition

Go 1.21's `slog` package establishes a clean interface for this class of problem. Every logger is backed by a `slog.Handler` interface with three methods: `Enabled()`, `Handle()`, `WithAttrs()`. Routing to different sinks is done by composing handlers. Third-party libraries like `slog-multi` provide a `FanOutHandler` that dispatches each record to all registered child handlers.

zerolog's `MultiLevelWriter` sends records to multiple writers simultaneously. For level-specific routing (errors to file, info to stdout) the standard approach is to wrap writers with level filters before passing them to `MultiLevelWriter`.

The key abstraction transfer: Go's slog `Handler` interface is essentially what tsuku's `progress.Reporter` interface already is - a pluggable sink. The difference is that `Reporter` is scoped to operational output (progress, log, warn), while the notification routing problem needs a sink that can route some events to the terminal and others to the file-based notices inbox.

### Tsuku's current state for comparison

Tsuku already has:
- `progress.Reporter` interface with `NoopReporter` (discard all output) and `ttyReporter` (TTY-aware terminal output)
- `DeferWarn` / `FlushDeferred` on `ttyReporter` - queues warnings for end-of-operation display
- `ShouldSuppressNotifications()` in `updates/suppress.go` with a precise precedence chain: explicit opt-in env var > explicit opt-out env var > CI env > quiet flag > non-TTY stdout
- `notices/` package with file-based inbox: `WriteNotice`, `ReadUnshownNotices`, `MarkShown`, `RemoveNotice`
- Background spawn via `spawnDetached()`: stdin/stdout/stderr all set to nil, new process group via `Setpgid: true`, so no terminal connection
- Two-phase display in `DisplayNotifications()`: auto-apply results (in-memory, same invocation) → unshown notices (from disk, prior runs) → available summary (sentinel-gated)

The gap: when a warning-worthy event occurs inside the version resolver or installer during a background subprocess, there is no path from `reporter.Warn(...)` to the notices inbox. The background subprocess uses `NoopReporter` (or no reporter at all), so warnings are silently dropped.

## Implications

1. **The file-based inbox pattern is well-validated.** npm, dnf-automatic (via motd), and tsuku's own existing notice system all converge on the same idea: background processes write to a durable store, interactive sessions consume from that store. No need to invent a new approach.

2. **TTY detection is the right routing gate.** Every ecosystem examined uses TTY presence as the primary indicator of interactive context. The suppression chain tsuku already has in `ShouldSuppressNotifications()` matches the consensus. The missing piece is applying that same gate inside the install/version machinery where warnings currently only go to `reporter.Warn()`.

3. **The Reporter interface is the right seam for an inbox sink.** Rather than a separate "notification API," the cleanest path is an `InboxReporter` implementation of `progress.Reporter` that routes `Warn()` and `DeferWarn()` calls to `notices.WriteNotice()` instead of stderr. Callers that already use the reporter interface would gain inbox routing by receiving a different implementation - no call-site changes needed.

4. **dnf-automatic's `emit_via` list is overkill here.** The list-of-emitters approach is useful when operator-configurable delivery channels matter (email vs. MOTD vs. command hook). For tsuku the two sinks (terminal and inbox) are determined by context (interactive vs. background), not by user configuration. A simpler `switch(context)` on reporter construction is sufficient.

5. **Homebrew's dual-display (inline + deferred summary) is worth noting.** tsuku's `DeferWarn` / `FlushDeferred` already implements this for the interactive path. The same mechanism could let an `InboxReporter` also queue deferred writes that are flushed as notice files at the end of the operation rather than inline.

6. **Rustup's post-command display challenge is real.** For tsuku's background auto-apply path, notices are surfaced on the _next_ command after the background process finishes - not during the in-progress command. This is the correct design: the background subprocess has no terminal to write to, and the parent is already past the point where it could display anything.

7. **The `NoopReporter` problem.** The background subprocess currently passes `NoopReporter` (or nothing) to the install flow. An `InboxReporter` would replace `NoopReporter` in that context, turning silently dropped warnings into persisted notices.

## Surprises

1. **npm's update-notifier intentionally delays the first notification.** Even when an update is available on the first run, it doesn't show until the next check cycle. The rationale is avoiding user confusion on first install. tsuku's existing rate limiting (once-per-cycle via sentinel) already matches this behavior.

2. **Homebrew doesn't have a general-purpose "background-to-interactive" notification bridge.** It relies entirely on the macOS notification center for background results, which is OS-specific and irrelevant to a cross-platform CLI. The notices-file pattern (which tsuku already uses) is actually the more portable and pragmatic solution.

3. **Go slog's handler composition is clean but not directly applicable.** slog routes _log records_; tsuku's notice taxonomy (persistent error vs. single-view vs. transient noise) adds lifecycle semantics that slog doesn't have. The Reporter interface is a better fit than slog because it already encodes the distinction between transient (`Status`/`Log`) and deferred (`DeferWarn`/`FlushDeferred`) output.

4. **pnpm's decision to ditch update-notifier is instructive.** The reason wasn't technical - it was dependency hygiene. pnpm already had the registry client; adding an external library for a persistence+display concern it could own itself was unnecessary. tsuku is in the same position: it already has the notices infrastructure. Extending it (rather than adding a new abstraction layer) is the right move.

5. **dnf-automatic separates "what to do" from "how to notify."** It has three distinct systemd timer units: notify-only, download-only, and download-and-install. The emitter system is decoupled from the action system. tsuku's current check-updates / apply-updates / background auto-apply three-phase model already mirrors this separation - the emitter question is the missing piece.

## Open Questions

1. **What's the right lifecycle for version-fallback notices specifically?** A "skipped version X.Y.Z because no asset matched your platform" event might be single-view (once the user has seen it they don't need reminding), or it might be persistent-until-actioned (the tool may never get a working asset, so the warning should recur). The notice taxonomy design (lead 3) needs to answer this before the first use case can be implemented.

2. **Should `InboxReporter` write one notice file per warning, or accumulate multiple warnings into a single notice for a tool?** The current `Notice` struct has a single `Error` string. If version fallback generates multiple skipped-version warnings during a single install, the file format needs to support that (either a list field, or multiple notices per operation keyed differently than by tool name alone).

3. **How does the inbox sink handle warnings that occur during the interactive path?** If a user runs `tsuku install foo` interactively and the version resolver skips an asset, the `ttyReporter` shows the warning inline - but should it also write to the inbox so the user can review it later via `tsuku notices`? Or only persist warnings from the background path? The scope document says "interactive mode routes to terminal (and optionally also to the inbox for events that warrant persistence)" - but that "optionally" needs a concrete rule.

4. **What happens when the inbox sink itself fails (disk full, permissions error)?** The current notice write is best-effort (`_ = notices.WriteNotice(...)`). An `InboxReporter` should have the same swallow-on-error behavior, but this means a background-path warning that fails to persist is silently lost - which is the same problem the investigation started with. Is there a fallback (e.g., log to slog debug level) that provides an audit trail without blocking?

5. **Does the background `apply-updates` subprocess need its own reporter at all, or only the inner install flow?** `MaybeAutoApply()` in `apply.go` currently returns `[]ApplyResult` which the parent process renders. The gap is in what happens _inside_ `applyUpdate()` → `installFn()` - warnings from the version resolver and action executor that aren't errors but aren't surfaced in `ApplyResult`. Understanding exactly which code paths call `reporter.Warn()` and `reporter.DeferWarn()` (lead 1) is needed to scope the fix.

## Summary

The ecosystem consensus is clear: file-based "inbox" persistence for background-path notifications, TTY detection as the routing gate, and a reporter-interface abstraction as the seam where the routing decision lives. tsuku already has all three pieces individually - the notices package, the TTY detection in `ShouldSuppressNotifications()`, and the `Reporter` interface - but they aren't connected: the background subprocess uses `NoopReporter`, silently dropping warnings that should route to the inbox instead. The main implication is that an `InboxReporter` implementation of the existing `Reporter` interface is the minimal, low-risk path to wiring these pieces together. The biggest open question is notice lifecycle taxonomy: specifically whether version-fallback warnings are single-view or persistent-until-actioned, since that determines the `Shown` / `RemoveNotice` semantics for the new notice type.
