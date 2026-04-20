---
status: Proposed
problem: |
  tsuku runs full tool installs synchronously in PersistentPreRun before the
  command the user asked for, blocking even fast read-only commands like `tsuku
  list`. The update check is already non-blocking; the apply step is not. A
  secondary path in main.go init() makes unbounded HTTP calls for distributed
  registry discovery with no timeout, hanging startup on slow networks.
decision: |
  Replace the synchronous MaybeAutoApply call in PersistentPreRun with a
  MaybeSpawnAutoApply function that fires a detached apply-updates subprocess
  (mirroring the existing CheckAndSpawnUpdateCheck pattern). Results are
  delivered via the existing notice system on the next command invocation. The
  Notice struct gains a Kind field for backward-compatible result classification.
  Distributed registry init gets a 3-second shared timeout.
rationale: |
  The detached-subprocess pattern is already proven in production for update
  checks. Extending it to apply requires only a new hidden subcommand and a spawn
  call — no new infrastructure. Option C (PersistentPostRun) still blocks the
  terminal after the user's command. Option B (combine check+apply) violates the
  10-second check-updates timeout budget. The Kind field is the minimal schema
  change that preserves backward compatibility via Go's zero-value JSON semantics.
---

# DESIGN: Background Updates

## Status

Proposed

## Context and Problem Statement

Every tsuku command (except a small skip-list) passes through `PersistentPreRun`
in `cmd/tsuku/main.go`, which calls two functions before the user's command runs:

1. `CheckAndSpawnUpdateCheck` — already non-blocking (<1ms). Spawns a detached
   `tsuku check-updates` subprocess via `cmd.Start()` without `Wait()`. Uses
   sentinel file mtime and non-blocking flock to avoid duplicate spawns.

2. `MaybeAutoApply` — synchronous. Reads cached update entries and, if
   auto-apply is enabled and updates are pending, calls `runInstallWithTelemetry`
   for each one before the user's command runs. A user with three pending
   auto-updates waits through three complete install operations — including
   downloads — before `tsuku list` prints anything.

This is the source of the blocking users experience. It makes tsuku feel broken.

A secondary blocking path exists in `main.go init()`: when distributed registries
are configured, `NewDistributedRegistryProvider` calls `DiscoverManifest`
synchronously with `context.Background()` (no timeout) for each configured source,
adding unbounded HTTP roundtrips at binary startup. The existing error handling
already treats discovery failure as non-fatal (fallback to flat layout), so only
a time bound is missing.

The update check itself (`CheckAndSpawnUpdateCheck` + `trigger.go`) already
implements the right pattern — fire-and-forget subprocess with file-lock dedup
and sentinel freshness — and the notification system (file-backed, pull-per-command
via `$TSUKU_HOME/notices/`) already delivers async results without new IPC.
The design question is how to extend this to the apply step.

## Decision Drivers

- **Zero foreground blocking:** The user's command must start immediately.
  Any update-related work that can be deferred must be.
- **Lighter footprint first:** No persistent daemons, no OS schedulers (cron,
  systemd timers, launchd). The detached-subprocess pattern already in use is
  the starting point.
- **Use existing infrastructure:** The notice system and the `trigger.go`
  subprocess pattern are already proven. Minimize new primitives.
- **Backward-compatible schema changes:** The `Notice` struct is serialized to
  disk files on user machines. Any schema extension must deserialize existing
  files without error.
- **Safe concurrent installs:** Auto-apply running in background and an explicit
  `tsuku install foo` running in foreground must not corrupt tool state. The
  existing `state.json.lock` flock is the gate; the design must account for
  it explicitly.
- **Platform scope:** Linux and macOS only (current GoReleaser targets).
  Windows must not be broken but is not a release target.

## Decisions Already Made

From exploration (Round 1):

- OS schedulers (cron, systemd timers, launchd) are eliminated. They require
  system footprint and lifecycle management that contradicts the project
  philosophy.
- Persistent daemon is eliminated for the same reason.
- The detached-subprocess pattern in `trigger.go` is the confirmed mechanism.
  No new background primitives needed.
- The notice system (file-backed, pull-per-command) is the correct delivery
  channel for background activity results. No new IPC needed.
- "Registry cache refresh" is not the primary blocking concern. Registry refresh
  has no automatic trigger; only `tsuku update-registry` (explicit) or inline
  recipe fetches on cache miss. The blocking is from `MaybeAutoApply`.
- Notices should appear after command output, not before.

## Considered Options

### Decision 1: Auto-apply lifecycle and concurrency model

Every tsuku command calls `MaybeAutoApply` in `PersistentPreRun`, which runs full
tool installs synchronously before the user's command. The existing `state.json.lock`
flock (`TryLockExclusive`) is already used as a concurrency gate — if another tsuku
holds the lock, auto-apply skips silently. The apply subprocess inherits this model.

Key assumptions:
- Tool installs take 10–120 seconds; the 10-second check-updates context timeout
  rules out embedding installs in that subprocess.
- The "one command late" tradeoff — background results appear on the next invocation
  — is acceptable. Peer tools (npm, gh) use this model; Homebrew's inline blocking
  is universally considered a flaw.
- `SysProcAttr{Setpgid: true}` must be set on spawned processes. Without it,
  closing the terminal sends SIGHUP to the background process group on some shell
  configurations, interrupting an in-progress install.

#### Chosen: Option A — Spawn a separate apply subprocess from PersistentPreRun

Mirror `CheckAndSpawnUpdateCheck` exactly: add a `MaybeSpawnAutoApply` function
in `internal/updates/trigger.go` that, when auto-apply is enabled and pending
entries exist, spawns a detached `tsuku apply-updates` subprocess via `cmd.Start()`
without `Wait()`. `PersistentPreRun` calls this function alongside
`CheckAndSpawnUpdateCheck`; both return in under 1ms.

The `apply-updates` subcommand (hidden, parallel to `check-updates`):
1. Tries `TryLockExclusive` on `state.json.lock`. If not acquired, exits silently.
2. Reads pending cache entries and applies each update via the install flow.
3. Writes a notice file per tool (success or failure) to `$TSUKU_HOME/notices/`.
4. Removes the consumed cache entry after each tool's apply completes.

Lock contention: if a foreground `tsuku install foo` holds `state.json.lock`, the
apply subprocess exits at the probe. Cache entries persist and are retried on the
next invocation. If the foreground install covered the same version as a pending
entry, `LatestWithinPin == ActiveVersion` after completion, and the entry is
naturally filtered out.

#### Alternatives Considered

**Option B — Combine check and apply in single background subprocess:** Rejected.
Tool installs violate the 10-second `check-updates` context timeout. Embedding
60+ second downloads in a 10-second subprocess either means exceeding the timeout
silently (incomplete applies) or extending it (creating an unpredictably long-lived
background process).

**Option C — Defer to PersistentPostRun:** Rejected. Still blocks the terminal
after the user's command completes, defeating the purpose. `PersistentPostRun`
does not fire on command errors, creating inconsistent apply behavior. A 30-second
timeout that fires repeatedly on every command is worse than the current behavior.

---

### Decision 2: Notice schema extension

The `Notice` struct (`internal/notices/notice.go`) is serialized to per-tool JSON
files on user machines. Background auto-apply needs to write results that the next
command invocation can display. The struct currently has no field to distinguish
background-apply results from other notice types.

Key assumptions:
- Background apply success notices should be shown (one-line confirmation per tool,
  matching self-update behavior today).
- No migration mechanism exists; schema changes must work via zero-value JSON
  semantics alone.
- Future notice kinds are plausible but not scheduled; extensibility is secondary.

#### Chosen: Option A — Add Kind field with backward-compatible JSON unmarshaling

Add `Kind string \`json:"kind,omitempty"\`` to `Notice`. Go's `json.Unmarshal`
leaves missing fields at zero value; existing files on disk deserialize with
`Kind == ""` and render unchanged.

```go
const (
    KindUpdateResult    = ""                // zero value — all existing notices
    KindAutoApplyResult = "auto_apply_result"
)
```

Background apply writes `Kind: KindAutoApplyResult`. Existing paths (self-update,
foreground apply results) continue writing `Kind: ""` with no changes. Rendering
switches on Kind when needed; the default path is unaffected.

Rendering format: plain stderr line matching existing style — `"<tool> updated to
<version>"` — no prefix, no box. Success notices use the existing `Shown: false`
gate; they display once and are marked shown.

#### Alternatives Considered

**Option B — Separate directory for activity notices:** Rejected. Splits one
conceptual entity into two read paths with no compatibility benefit. The `shown`
gate and TTY/CI suppression work identically regardless of directory.

**Option C — Repurpose Error field as generic message:** Rejected. A successful
update is not an error. Encoding a success message in the Error field makes
rendering logic fragile and breaks the existing semantic contract (`Error == ""`
means success).

---

### Decision 3: Distributed registry initialization safety

`main.go init()` calls `DiscoverManifest` synchronously with `context.Background()`
for each configured distributed source. Discovery failure is already non-fatal
(fallback to flat layout); only a time bound is missing.

Key assumptions:
- Registry count is typically small (1–3); a shared 3-second deadline bounds
  total init blocking to 3 seconds regardless of count.
- Users with distributed registries prefer fast degraded behavior over hanging.

#### Chosen: Option A — Add context timeout; skip on timeout (best-effort)

Replace `initCtx := context.Background()` with a context carrying a 3-second
shared deadline. The existing warning-and-skip path handles timeout errors without
any other behavior change. The notice system is not used for transient discovery
failures; the existing stderr warning is appropriate.

#### Alternatives Considered

**Option B — Fail command on timeout:** Rejected. `init()` cannot return errors;
restructuring to surface a fatal error contradicts the feature's best-effort design
intent and breaks users on degraded networks for an optional feature.

**Option C — Lazy initialization:** Rejected. Defers rather than bounds latency.
Requires a `sync.Once` proxy struct with no meaningful UX benefit over Option A.

---

### Implicit Decision: Per-tool cache entry removal vs. batch removal

After each tool's apply completes, its cache entry is removed immediately (not
after the whole batch). This makes partial-batch crashes safe: completed tools
have their entries removed, failed tools retain entries for retry. The alternative
— remove all entries after the full batch succeeds — would re-apply every tool in
the batch if any one fails mid-run.

## Decision Outcome

The three decisions combine cleanly:

`PersistentPreRun` calls `MaybeSpawnAutoApply` alongside the existing
`CheckAndSpawnUpdateCheck`. Both return in under 1ms. The user's command executes
immediately. In the background, the `apply-updates` subprocess acquires
`state.json.lock` (or exits if held), installs each pending update, writes a notice
with `Kind: KindAutoApplyResult`, and removes the cache entry. The next command
invocation calls `DisplayNotifications`, which reads unshown notices and prints a
one-line confirmation per updated tool.

The distributed registry timeout fix is independent and additive: `main.go init()`
gets a 3-second shared context deadline, bounding startup for users with distributed
sources on degraded networks.

Both spawned subprocesses (`check-updates` and `apply-updates`) gain
`SysProcAttr{Setpgid: true}` via a shared `spawnDetached` helper, isolating them
from terminal signal propagation.

## Solution Architecture

### Overview

The apply step is decoupled from the foreground command by moving it into a
detached subprocess that runs alongside the user's command rather than before it.
Result delivery uses the existing pull-based notice system. No new IPC, no daemon,
no OS scheduler.

### Components

```
cmd/tsuku/main.go
  PersistentPreRun
    ├── CheckAndSpawnUpdateCheck()    (existing, unchanged)
    ├── MaybeSpawnAutoApply()         (new — mirrors CheckAndSpawnUpdateCheck)
    └── DisplayNotifications()        (existing — reads unshown notices)

internal/updates/trigger.go
    ├── spawnDetached(cmd)            (new helper — sets SysProcAttr{Setpgid:true})
    ├── spawnChecker()                (updated to use spawnDetached)
    └── MaybeSpawnAutoApply()         (new — parallel to CheckAndSpawnUpdateCheck)

internal/updates/spawn_unix.go       (new — sets SysProcAttr{Setpgid: true})
internal/updates/spawn_windows.go    (new stub — no-op for Windows build compat)

cmd/tsuku/cmd_apply_updates.go       (new hidden subcommand — parallel to
                                      cmd_check_updates.go)

internal/notices/notice.go
    Notice struct
        + Kind string `json:"kind,omitempty"`   (new field)
        + KindUpdateResult    = ""               (new constant)
        + KindAutoApplyResult = "auto_apply_result" (new constant)

main.go init()
    context.WithTimeout(context.Background(), 3*time.Second)  (new deadline)
```

### Key Interfaces

**MaybeSpawnAutoApply** (new, `internal/updates/trigger.go`):
```go
func MaybeSpawnAutoApply(cfg *config.Config, userCfg *userconfig.Config) error
```
Returns immediately after `cmd.Start()` or if no spawn is needed. Mirrors
`CheckAndSpawnUpdateCheck` signature.

**spawnDetached** (new helper, `internal/updates/trigger.go`):
```go
// spawnDetached prepares cmd for fire-and-forget execution:
// sets Setpgid (Unix) or no-op (Windows), suppresses stdio, starts without Wait.
func spawnDetached(cmd *exec.Cmd) error
```
Both `spawnChecker` and `MaybeSpawnAutoApply` use this helper.

**apply-updates subcommand** (new, hidden):
Entry point: `cmd/tsuku/cmd_apply_updates.go`. Not shown in `tsuku --help`.
Behavior: acquire `TryLockExclusive` on `state.json.lock` → iterate pending
cache entries → `runInstallWithTelemetry` per tool → `WriteNotice` (Kind:
KindAutoApplyResult) → `RemoveEntry` → release lock implicitly on exit.

**Notice.Kind** (new field):
Zero value (`""`) preserves all existing behavior. `"auto_apply_result"` marks
results from background apply runs. Rendering in `renderUnshownNotices` can switch
on Kind for future differentiation; the current rendering path is unchanged because
the existing success branch (`n.Error == ""`) already handles auto-apply results
correctly.

### Data Flow

```
User: tsuku list
  └─ PersistentPreRun
       ├─ CheckAndSpawnUpdateCheck   → (if stale) spawn check-updates [background]
       ├─ MaybeSpawnAutoApply        → (if pending) spawn apply-updates [background]
       └─ DisplayNotifications       → render unshown notices from $TSUKU_HOME/notices/
  └─ list command executes → output

Background: apply-updates subprocess
  ├─ TryLockExclusive(state.json.lock) → exit if held
  ├─ for each pending cache entry:
  │    ├─ runInstallWithTelemetry(tool, version, ...)
  │    ├─ WriteNotice(Kind: KindAutoApplyResult, tool, version, err)
  │    └─ RemoveEntry(tool)
  └─ exit (lock released automatically)

Next command: tsuku info foo
  └─ PersistentPreRun
       └─ DisplayNotifications → renders "foo updated to 1.2.0\n" to stderr
                                  marks notice Shown: true
```

**State files involved:**

| File | Role |
|------|------|
| `$TSUKU_HOME/cache/updates/<tool>.json` | Update check cache; consumed by apply-updates |
| `$TSUKU_HOME/cache/updates/.last-check` | Sentinel for check freshness; unchanged |
| `$TSUKU_HOME/cache/updates/.lock` | Dedup lock for check-updates spawn; unchanged |
| `$TSUKU_HOME/notices/<tool>.json` | Notice written by apply-updates; read by next command |
| `$TSUKU_HOME/state.json.lock` | Exclusive lock gating all install operations |

## Implementation Approach

### Phase 1: Notice schema extension

Deliverables:
- `internal/notices/notice.go`: add `Kind string \`json:"kind,omitempty"\`` and
  `KindUpdateResult`/`KindAutoApplyResult` constants
- Verify existing notice rendering tests pass with no changes (zero-value Kind)
- Add a test: notice file with no `kind` field deserializes with `Kind == ""`

Dependencies: none.

### Phase 2: Spawner hardening

Deliverables:
- `internal/updates/spawn_unix.go`: `func setSysProcAttr(cmd *exec.Cmd)` that
  sets `cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}`
- `internal/updates/spawn_windows.go`: no-op stub with matching signature
- `internal/updates/trigger.go`: add `spawnDetached(cmd *exec.Cmd) error` helper;
  update `spawnChecker` to use it

Dependencies: Phase 1 not required; can be done in parallel.

### Phase 3: apply-updates subcommand

Deliverables:
- `cmd/tsuku/cmd_apply_updates.go`: hidden subcommand that acquires lock, iterates
  cache entries, installs, writes notices (`Kind: KindAutoApplyResult`), removes
  entries. Must set a top-level 5-minute context deadline at entry to prevent
  indefinite hangs from stalled network connections.
- `internal/updates/trigger.go`: `MaybeSpawnAutoApply` function using
  `spawnDetached`. Must use a dedicated probe lock (separate from the check-updates
  lock) to deduplicate spawns, mirroring `CheckAndSpawnUpdateCheck`. Without it,
  rapid successive commands could spawn multiple apply-updates processes.
- `cmd/tsuku/main.go`: replace `MaybeAutoApply` call with `MaybeSpawnAutoApply`;
  remove `results` parameter from `DisplayNotifications` if it becomes unused

Dependencies: Phase 1 (Kind constant), Phase 2 (spawnDetached helper).

### Phase 4: Distributed registry init timeout

Deliverables:
- `cmd/tsuku/main.go init()`: replace `context.Background()` with
  `context.WithTimeout(context.Background(), 3*time.Second)` shared across all
  `NewDistributedRegistryProvider` calls

Dependencies: none; can be done in parallel with Phases 1–3.

### Phase 5: Cleanup and tests

Deliverables:
- Remove `MaybeAutoApply` synchronous call and its `results []ApplyResult` path
  from `PersistentPreRun` (if not already done in Phase 3)
- Update or remove unit tests for `MaybeAutoApply` that test synchronous behavior
- Add integration test: run a command with pending auto-apply entries; verify the
  command returns immediately; verify notice files are written after the subprocess
  exits; verify next command displays the notices

Dependencies: Phases 1–4 complete.

## Security Considerations

The apply-updates subprocess runs with the same user permissions as the parent
tsuku process. It reads install parameters from cache files in
`$TSUKU_HOME/cache/updates/` and writes results to `$TSUKU_HOME/notices/`. Both
directories are owned by the running user (mode 0755); world-writable `$TSUKU_HOME`
configurations are unsupported.

**Subprocess spawning.** The subprocess command is `tsuku apply-updates` with no
user-controlled arguments. Install parameters are read from the cache directory,
not passed on the command line, eliminating argument injection vectors. Spawn
deduplication via a dedicated probe lock prevents multiple concurrent apply-updates
processes from racing when commands run in rapid succession.

**Cache file integrity.** Cache entries are not cryptographically signed. They are
trusted as same-user-written files under the assumption that `$TSUKU_HOME` is not
writable by other users. A local attacker with write access to the cache directory
could craft an entry to trigger installation of a specific version string; the
recipe executor's checksum validation provides the last line of defense.

**Notice file integrity.** Notice file content is rendered as display text only.
No notice field is interpreted as code or used to construct filesystem paths. The
`Kind` field is validated against a closed set of known values at read time to
prevent unexpected display behavior from crafted entries.

**Orphaned processes.** The apply-updates subprocess sets a 5-minute top-level
context deadline, preventing indefinite hangs from stalled network connections.
The per-request HTTP client timeout guards individual requests; the top-level
deadline guards the overall install session.

**Telemetry.** The apply-updates subprocess may transmit telemetry events (tool
name, version, outcome, error classification) consistent with the foreground
install path. No credentials, file paths, or environment variables are included
in telemetry payloads.

## Consequences

### Positive

- All tsuku commands return in under 1ms for the pre-run hook overhead, regardless
  of pending updates. `tsuku list`, `tsuku info`, and other read-only commands are
  fully unblocked.
- No new infrastructure: the design extends an existing, production-proven pattern.
  The `apply-updates` subcommand is structurally identical to `check-updates`.
- `SysProcAttr{Setpgid: true}` makes both `check-updates` and `apply-updates`
  independent of terminal signal propagation — a correctness improvement for the
  existing spawner as well.
- The `Kind` field makes notice classification explicit, enabling future rendering
  differentiation without schema migration.
- Distributed registry initialization is bounded: users on degraded networks no
  longer hang at startup.

### Negative

- **"One command late" for apply results.** Users see update notices on the next
  command after the background apply completes, not the command that triggered it.
  A tool could be updated while the user is mid-workflow without their knowledge
  until the next invocation.
- **Silent background activity.** There is no "updating foo..." progress indicator
  while the apply subprocess runs. Users who want to monitor updates must check
  `tsuku notices` or observe the next-command notice.
- **Background apply is not cancellable.** Once spawned, the apply subprocess runs
  to completion (or until killed externally). There is no mechanism to abort a
  background install after it starts.
- **Startup add on degraded networks remains.** The 3-second timeout for distributed
  registry discovery means startup can still take up to 3 seconds (down from
  unbounded) for users with misconfigured or unreachable distributed sources.

### Mitigations

- "One command late" is the accepted industry pattern (npm, gh, rustup). The notice
  is clearly labeled and appears before the user's command output on the next run.
- Silent background: the `apply-updates` subprocess inherits the same failure-notice
  behavior as the synchronous path. Failures are written to notices and displayed
  on the next invocation; the consecutive-failure threshold (3) suppresses transient
  noise.
- Non-cancellable: the install flow's staging + atomic rename ensures partial installs
  leave no inconsistent state. Users can interrupt via SIGKILL if needed; the tool's
  prior version remains active until the new one is fully staged and renamed.
- 3-second startup cap: documented behavior. Users who want zero startup latency can
  remove their distributed registry sources or set `updates.enabled = false`.
