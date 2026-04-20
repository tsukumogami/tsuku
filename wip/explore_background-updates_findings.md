# Exploration Findings: background-updates

## Core Question

Tsuku's auto-update and cache-refresh operations block commands the user actually
asked for, making the tool feel broken. We need to understand the full shape of the
blocking problem and what approach — background processes, deferred work, scheduling,
or something else — best eliminates the wait without sacrificing reliability or
adding system footprint.

## Round 1

### Key Insights

- **The blocker is `MaybeAutoApply`, not the update check.** `CheckAndSpawnUpdateCheck`
  is already non-blocking (<1ms, detached subprocess). `MaybeAutoApply` in
  `PersistentPreRun` runs full tool installs synchronously before every non-excluded
  command — including `tsuku list`. A user with three pending auto-updates waits through
  three complete installs before their command runs. (Lead: blocking-operations,
  notification-system, async-patterns)

- **The architecture already has the right pattern.** `internal/updates/trigger.go`
  implements fire-and-forget subprocess spawning with file-lock dedup and sentinel
  freshness checks. This is proven, working, and used on every command. Extending it
  to auto-apply is a natural fit — not a new mechanism. (Lead: async-patterns,
  platform-implications)

- **The notification system is the right delivery channel.** Notices are pull-based
  and file-backed: background processes write JSON to `$TSUKU_HOME/notices/`, and
  `DisplayNotifications` reads them on the next command. Self-update already uses this
  pattern. The existing infrastructure handles dedup, TTY suppression, and atomic
  writes. A schema extension (a `Kind` field) would make it support new notice types
  cleanly. (Lead: notification-system)

- **Secondary blocker: distributed registry `init()`.** `main.go init()` calls
  `DiscoverManifest` synchronously with `context.Background()` (no timeout) for each
  configured distributed registry. This adds one or more unbounded HTTP roundtrips at
  binary startup for users with distributed registries. (Lead: blocking-operations)

- **Peer tools confirm: blocking is the wrong default.** Homebrew's pre-command
  blocking is universally considered a design flaw. npm/gh's two-phase model
  (spawn-or-goroutine → write result → display on next command) is the accepted
  pattern. Notices should appear after command output, not before. (Lead: peer-patterns)

- **Platform story is straightforward.** Linux and macOS are the only release targets.
  The existing detached subprocess approach works on both. The only gap is missing
  `SysProcAttr{Setpgid: true}`, which would prevent SIGHUP propagation on terminal
  close — fixable with two small build-tagged files. No daemon, no OS scheduler
  needed. (Lead: platform-implications)

### Tensions

- **"One command late" UX.** Moving auto-apply to a background subprocess means
  results appear on the next command invocation, not the current one. This is
  acceptable for general updates but feels odd when the user's command is blocked
  because an unrelated tool is being updated — even with the fix, the "applied" notice
  arrives a run late. The peer-patterns research suggests this is the accepted
  tradeoff; Homebrew's inline blocking is worse.

- **Auto-apply vs. explicit install conflict.** If `foo` has a pending auto-update and
  the user runs `tsuku install foo`, the current code runs the auto-update first, then
  the explicit install. Backgrounding auto-apply resolves the blocking but surfaces a
  race: will the background process and the explicit install step on each other? The
  existing file lock (`state.json.lock`) should prevent this, but the behavior needs
  explicit design.

- **Registry "cache refresh" is not the same problem as auto-apply.** The user framed
  the issue as "cache refresh" blocking, but the code shows registry refresh has no
  automatic trigger — only `tsuku update-registry` (explicit) or inline recipe fetches
  on cache miss. The actual blocking the user experienced is almost certainly
  `MaybeAutoApply` running installs, not a registry refresh. This is a simpler problem
  than initially framed.

### Gaps

- No real-world timing data on `MaybeAutoApply`. The code shows it can block for
  download time × pending updates, but how often users have pending updates in practice
  is unknown.
- How much the `update-registry` command's synchronous behavior (distinct from
  `MaybeAutoApply`) contributes to user frustration. It's user-invoked and intentionally
  synchronous, but could benefit from progress reporting.

### Decisions

- Scope narrowed: OS schedulers (cron, systemd timers, launchd) are off the table.
  They require system footprint and lifecycle management that contradicts the project
  philosophy. No evaluation needed.
- Persistent daemon is off the table for the same reason.
- The detached subprocess pattern in `trigger.go` is the confirmed mechanism.
  No need to evaluate alternatives.

### User Focus

Auto mode — no user narrowing input this round. Findings are highly convergent across
all five leads; gap coverage is sufficient to proceed to crystallize.

## Accumulated Understanding

The blocking users experience comes from `MaybeAutoApply` executing full tool installs
in `PersistentPreRun` before every command. The update check itself is already
non-blocking. The solution direction is clear: move `MaybeAutoApply` to the same
detached-subprocess model already used for the update check, surfacing results via the
existing notice system on the next command invocation.

Secondary work: add a timeout to distributed registry initialization in `main.go init()`
to prevent unbounded startup hangs for users with distributed registries configured.

The notice system needs a schema extension (a `Kind` field) to distinguish update
results from future notice categories, but the underlying delivery mechanism requires
no new infrastructure.

The platform story is: Linux and macOS only (current release targets), detached
subprocess already proven, `SysProcAttr{Setpgid: true}` gap is minor but worth fixing
to prevent SIGHUP on terminal close.

## Decision: Crystallize
