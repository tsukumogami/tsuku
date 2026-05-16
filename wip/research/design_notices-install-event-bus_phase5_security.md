# Security Review: notices-install-event-bus

## Dimension Analysis

### External Artifact Handling
**Applies:** No

The design adds an in-process pub/sub bus and a subscriber that writes/removes
per-tool JSON files under `$TSUKU_HOME/notices/`. It does not download, fetch,
execute, or evaluate any external input. Events are constructed by the
publisher (the `install.Manager` and `updates.CheckAndApplySelf`) from data
already present in the tsuku process — tool names from `state.json`, version
strings already resolved, error values already returned by failing operations.
The renderer is unchanged and only reads the local notice files. No new
artifact ingestion path is introduced.

### Permission Scope
**Applies:** Yes (no escalation)

The notices subscriber needs the same filesystem permissions the existing
ad-hoc `notices.WriteNotice` / `RemoveNotice` calls already require: create
`$TSUKU_HOME/notices/`, write `<tool>.json` atomically (`0644` via
`os.WriteFile` + `os.Rename`), and `os.Remove` the same files.

No additional permissions are requested:
- No network access (the bus is in-process; subscribers run inline).
- No new process spawning (the auto-apply subprocess pre-existed).
- No setuid / privileged operations; `tsuku` runs as the invoking user.
- No new files outside `$TSUKU_HOME/notices/`.

The design removes ad-hoc direct writes from `cmd/tsuku/update.go`,
`internal/updates/apply.go`, and `internal/updates/self.go` and routes them
through one subscriber, which slightly narrows the surface that has to know
the notices directory path.

Escalation risk: none. The bus value is process-local, not a global, and
subscribers are wired explicitly in `cmd/tsuku/events_wiring.go`. A
compromised dependency cannot register a subscriber via package `init()`
because the chosen wiring design rejected `init()`-based registration
(Decision 4 — "Per-package init() self-registration. Rejected.").

### Supply Chain or Dependency Trust
**Applies:** No

The new `internal/installevents` package and the `internal/notices`
subscriber are first-party Go code in the tsuku monorepo. No new third-party
dependencies are introduced (no message-bus library, no serialization
codec). Events are plain structs passed by value within the same process;
they are not serialized over any channel that could be tampered with by a
different trust principal.

### Data Exposure
**Applies:** Yes (low severity — same data as today)

The on-disk notice file already contains: `Tool`, `AttemptedVersion`,
`Error` (the stringified error from a failing install), `Timestamp`,
`Shown`, `ConsecutiveFailures`, `Kind`, and `Messages`. The new flow
preserves exactly the same fields; nothing new is logged or persisted.

`InstallFailed.Err.Error()` is the field most likely to leak path or
network detail (URLs, redacted-but-maybe-not API responses, file system
paths under `$TSUKU_HOME`). This risk pre-exists: today, `apply.go:148-158`
and `self.go:164` write the same error string into the same notice file.
The event bus does not widen the disclosure surface — the same string lands
in the same file at the same `0644` perms.

One new exposure is **self-update failures becoming user-visible** (called
out under "Consequences → Positive"). Today they log silently to the trace
file; with this design they produce a rendered notice. The visible content
is the error string itself; the impact is that an error that previously
sat in a trace file is now printed on next `tsuku` invocation. This is
intended user-facing behavior, not a leak — but the design should note
that `Err.Error()` may include URLs/paths and should not be expanded to
include stack traces or HTTP response bodies without further review.

The bus's logger (used for subscriber-name + panic logging — see
"Information disclosure via logs" below) is the other new write path.

### Untrusted Input Paths
**Applies:** Yes (low severity — pre-existing validation covers the new flow, but with one gap)

`Tool` flows from event payloads into `WriteNotice(dir, &Notice{Tool: ...})`
and `RemoveNotice(dir, tool)`, both of which append `<tool>.json` to the
notices directory.

Tool name provenance under the new design is narrower than today's
free-form callers:
- `Manager.Install` / `Activate` / `RemoveVersion` / `RemoveAllVersions` —
  the `Tool` field is the same string used as a `map[string]ToolState` key
  in `state.json`. It has already been accepted by recipe lookup
  (`internal/recipe`) and recipe names are constrained to kebab-case in
  the registry.
- `updates.CheckAndApplySelf` — emits `Activated{Tool: "tsuku"}` /
  `InstallFailed{Tool: "tsuku"}`. Hard-coded.
- Auto-apply iterates entries that originated from the same `state.json`.

So in practice, `Tool` is a recipe-name or `"tsuku"`. None of these paths
introduce untrusted input that wasn't already implicitly trusted by the
state-mutating code.

`WriteNotice` already defends against path-separator injection
(`internal/notices/notices.go:57-59`) and rejects `..`. The event-based
flow does not bypass this — the subscriber calls the same `WriteNotice`
exported function.

**One small gap worth noting in the design:** `RemoveNotice`
(`notices.go:144-151`) does **not** perform the same validation as
`WriteNotice`. Today the only direct `RemoveNotice` callers
(`apply.go:149`, `notify.go:106`, `update.go:201, 386`) pass either
`entry.Tool` (from state.json, trusted) or `tool.Name` (also from state).
With the new subscriber, `RemoveNotice(s.dir, e.Tool)` runs on every
`Activated` (remove-then-write) and `Removed` event. The event `Tool`
field is also trusted (same provenance), so this is not exploitable in
practice. But the design crosses a boundary where `RemoveNotice` is now
called from one more entry point, and a defense-in-depth audit would
suggest **applying the same `..` / path-separator check inside
`RemoveNotice`**, or factoring the check into a shared validator that
both functions call. This is a minor hardening recommendation, not a
blocking issue.

### Denial of Service / Robustness
**Applies:** Yes (low severity)

Three sub-concerns from the brief:

1. **Unbounded queue growth from re-entrant Publish.**
   Decision 2 specifies that re-entrant `Publish` from inside a `Handle`
   is queued and flushed after the current event finishes. A malicious or
   buggy subscriber that publishes a new event for each event it handles
   would loop indefinitely and grow the queue without bound. In practice
   the only subscriber today is `internal/notices.Subscriber`, which
   does not call back into the bus. But the design as written has no
   max-depth or queue-size guard.

   **Severity:** low. The bus is in-process, subscribers are first-party
   code under code review, and a runaway loop manifests as a hung CLI
   process the user can SIGINT. A bounded queue or a small recursion-depth
   limit (e.g., refuse to enqueue beyond depth N, log a warning) would
   harden against future subscriber bugs at trivial cost. The design
   already commits to logging recovered panics; a similar log line for
   "re-entrancy depth exceeded" would be consistent.

2. **`recover()` containment sufficiency.**
   `defer recover()` in Go catches panics from the deferred goroutine
   only; since the bus is synchronous and `Handle` runs inline,
   per-subscriber recover is sufficient. A few edge cases to confirm
   during implementation:
   - `runtime.Goexit()` is not recoverable. A subscriber that calls
     `t.FailNow()` or similar in a test path would still unwind. This is
     fine for production code; test wiring should be aware.
   - Panics propagated as out-of-memory or stack-overflow may abort the
     process regardless of `recover()`. These are not actionable.
   - The design says "errors and recovered panics are logged" — the
     logger itself must not panic. The implementation should defensively
     guard the log call (or use a logger guaranteed non-panicking on
     formatting input).

3. **Subscriber-induced state damage.**
   A subscriber that succeeds on `WriteNotice` for `Activated` then panics
   before the queued flush could leave the on-disk store in a transient
   state — but since `WriteNotice` uses `os.Rename` atomic-write, a panic
   between write and Publish-return doesn't corrupt the file. The store
   is eventually consistent with the last event the subscriber observed.

Overall: robustness is fine for the stated subscriber set. A bounded
re-entrancy depth (default 8 or 16) would future-proof the design for
subscribers that haven't been written yet.

### Information Disclosure via Logs
**Applies:** Yes (low severity)

The bus's logger writes subscriber name + recovered panic value when a
subscriber fails. Two concerns:

1. **Recovered panic value.** A panic from `WriteNotice` would carry the
   error message (e.g., the rejected tool name, a path under
   `$TSUKU_HOME`, or an OS-level errno string). Tool names and
   `$TSUKU_HOME` paths are not sensitive in tsuku's threat model, but the
   panic value is arbitrary `interface{}` — a future subscriber that
   panics with a string built from environment variables or secrets
   would leak them via the log.

   Mitigation: document that subscribers must not panic with
   secret-bearing values; if subscribers need to surface failure to the
   user, they should do so via the notice file (which is the contract)
   rather than via panic.

2. **Logger destination.** The design references a `Logger` but does not
   pin where logs go (stderr? trace file? both?). If the bus log path
   routes to stderr by default, error messages with file paths print on
   every install. If it routes to the trace file only, disclosure is
   localized to a file already created with user-only perms. The
   implementation in Phase 1 should confirm the logger writes to the
   same destination as other tsuku diagnostic logs (`$TSUKU_HOME/log/`
   or equivalent), not to stderr, to avoid surprising the user with
   internal subscriber details on success paths.

3. **Subscriber names.** The chosen wiring uses human-readable names
   (`"notices"`). These are static and contain no user data; safe to log.

### Trust Boundary Inside the Process
**Applies:** No

Publisher and subscriber are both first-party packages in the tsuku
binary, compiled together, distributed together, signed together. They
share the same process, same address space, same on-disk credentials.
There is no meaningful trust boundary between `internal/install`,
`internal/installevents`, and `internal/notices`. If any of them is
compromised, all of them are.

The design correctly avoids treating subscribers as untrusted (no
sandboxing, no rate limiting, no permission scoping). The `recover()`
in `Publish` is robustness against bugs, not a security boundary.

A future world where third-party subscribers register via plugins would
change this assessment. The design's Decision 4 explicitly rejects
plugin-style `init()` registration; the wiring helper in `cmd/tsuku` is
the only place subscribers are added, which preserves the no-boundary
property and makes auditing trivial.

### Cross-Process Implications
**Applies:** Yes (no new risk)

The foreground tsuku command and the background auto-apply subprocess are
both tsuku binaries running as the same user. They synchronize via
`$TSUKU_HOME/state.json` (the install state) and `$TSUKU_HOME/notices/`
(the notice files). The event bus is process-local and does not introduce
any new IPC channel — each process has its own bus that disappears at
exit.

The TOCTOU question: could the new flow create a race where the
foreground process reads a notice file the background process is in the
middle of writing or removing?

- `WriteNotice` uses `os.WriteFile(tmp)` + `os.Rename(tmp, dst)`. `Rename`
  on POSIX is atomic for files in the same directory, so a reader sees
  either the old file or the new file, never a partial write. This is
  unchanged from today.
- `RemoveNotice` calls `os.Remove`, which is atomic. Readers see either
  the file or `ENOENT`. `ReadAllNotices` already tolerates parse failures
  and missing files (lines 96-104, 88-93).
- The renderer's `MarkShown` path reads → mutates → writes (`notices.go:133-140`).
  If the background subscriber writes a fresh `Shown:false` notice
  between the renderer's read and write, the renderer's write will
  clobber it with `Shown:true`. This race exists **today** with the
  ad-hoc writes; the event bus does not introduce it. The window is
  small (microseconds) and the only consequence is one missed notice.

No new locks or fsync are required by the design, and the design does not
weaken existing atomicity guarantees. The bus is in-process only, so two
processes' buses cannot interfere — they only see each other's effects
through atomic file operations.

**One subtlety worth a comment in the design:** the `Activated` reaction
is "RemoveNotice then WriteNotice" (subscriber.go pseudocode in the
design). Between those two calls, a concurrent reader sees no notice for
this tool. This is fine semantically (the prior notice for an older
version is gone; the new notice arrives shortly), but it changes the
timing relative to today's "write overwrites" pattern in
`apply.go:153-158`. If the renderer happens to run between Remove and
Write, it'll print nothing for that tool on this invocation. Probably
not user-visible (the rendered notice was for the prior version anyway),
but worth a note in the implementation PR. **Alternative**: skip
`RemoveNotice` and rely on `WriteNotice`'s overwrite semantics. The
atomic rename in `WriteNotice` already replaces the prior file.

## Recommended Outcome

**OPTION 2 - Document considerations:**

Insert the following as the body of the `## Security Considerations`
section (which the design currently lists as "_Filled in during Phase 5._"):

> The event bus is an in-process Go value with no new external inputs,
> no new network access, and no new dependencies. Publishers and the
> single notices subscriber are first-party packages compiled into the
> same binary; there is no meaningful trust boundary between them.
> Filesystem effects are limited to `$TSUKU_HOME/notices/<tool>.json`
> writes (atomic via `os.Rename`) and removes — the same paths
> `notices.WriteNotice` and `notices.RemoveNotice` already touch today.
>
> **Path-traversal defense.** Event `Tool` fields originate from
> `state.json` keys or hard-coded literals (`"tsuku"` for self-update),
> not from user-controlled string input. `notices.WriteNotice` validates
> against path-separator and `..` injection. As a defense-in-depth
> hardening, the implementation should extend the same validation to
> `notices.RemoveNotice` — today only `WriteNotice` checks. This is a
> small, isolated change in `internal/notices/notices.go` and is not
> exploitable in practice given current callers, but it removes an easy
> footgun for future call sites.
>
> **Re-entrancy bounds.** The bus queues nested `Publish` calls and
> flushes them after the current event. The notices subscriber does not
> call back into the bus, but a future subscriber that does — bugged or
> malicious — could grow the queue without bound. The implementation
> should cap re-entrancy depth (e.g., 16) and log + drop further events
> at that limit, matching the bus's "log subscriber error, do not
> propagate" stance.
>
> **Panic containment and logging.** Per-subscriber `defer recover()`
> catches panics from `Handle`. The recovered value and subscriber name
> are logged. Subscriber implementations must not panic with values that
> embed secrets; if the notice file isn't the right channel for some
> future signal, that signal should be added as a typed return or new
> event, not a panic message. The bus's logger should route to the same
> destination as other tsuku diagnostic output (the trace file under
> `$TSUKU_HOME/log/`), not to stderr, to avoid leaking subscriber detail
> on success paths.
>
> **Information disclosure.** `InstallFailed.Err.Error()` lands in the
> notice file and is rendered to the user, unchanged from today's
> behavior. Self-update failures become user-visible for the first time
> (previously silent); the rendered error string may include URLs or
> filesystem paths and should not be expanded to include HTTP response
> bodies or stack traces without further review.
>
> **Cross-process atomicity.** The foreground command and the background
> auto-apply subprocess each construct their own process-local bus. They
> synchronize through atomic file operations (`os.Rename`, `os.Remove`),
> identical to today. No new TOCTOU window is introduced. The
> subscriber's "RemoveNotice then WriteNotice" sequence on `Activated`
> events opens a microsecond gap where a concurrent reader would see no
> notice; the implementation may prefer to rely solely on `WriteNotice`'s
> atomic-rename overwrite semantics and skip the explicit `RemoveNotice`.

## Summary

The design is security-benign: an in-process event bus with no new
external inputs, no new dependencies, no new permissions, no new IPC.
All filesystem effects route through `notices.WriteNotice` /
`RemoveNotice`, which already exist and already write `0644` files under
`$TSUKU_HOME/notices/`. The two concrete hardening items worth noting in
the design's Security Considerations section are (a) extending the
`WriteNotice` path-separator validation to `RemoveNotice` for
defense-in-depth, and (b) capping bus re-entrancy depth to prevent
unbounded queue growth from a future buggy subscriber. Neither is a
design-blocking change.
