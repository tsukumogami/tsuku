<!-- decision:start id="notification-suppression-gate" status="assumed" -->
### Decision: Notification Suppression Gate

**Context**
Tsuku's auto-update system currently has no coherent notification suppression. `displayUnshownNotices` in internal/updates/apply.go writes to stderr unconditionally, meaning CI pipelines, piped scripts, and users who pass `--quiet` all receive unexpected update failure notices. The PRD (R12, R16) requires layered suppression from five signals: non-TTY stdout, `CI=true`, `--quiet`, `TSUKU_NO_UPDATE_CHECK=1`, and `TSUKU_AUTO_UPDATE=1` as an override.

The existing codebase already has per-subsystem suppression scattered across userconfig methods: `UpdatesEnabled()` checks `TSUKU_NO_UPDATE_CHECK`, `UpdatesAutoApplyEnabled()` checks CI with `TSUKU_AUTO_UPDATE` override. But these control whether update checks and auto-apply run, not whether notifications render. A new gate is needed specifically for notification output.

The key architectural constraint is the cmd/internal package boundary. The `--quiet` flag lives in cmd/tsuku/main.go and can't be imported by internal packages. Any solution must bridge this boundary explicitly.

**Assumptions**
- The notification suppression gate applies to update-related stderr output (failure notices, "new version available" banners), not to error messages from failed operations. If wrong: legitimate errors might be silently swallowed.
- `TSUKU_AUTO_UPDATE=1` overrides CI detection for notifications, not just auto-apply. If wrong: CI users who set `TSUKU_AUTO_UPDATE=1` would get auto-apply but never see what happened.
- The progress package's `IsTerminalFunc` var remains the canonical way to check stdout TTY status. If wrong: the suppression function would need its own TTY check mechanism.

**Chosen: Single ShouldSuppressNotifications function in internal/updates**

A single `ShouldSuppressNotifications(quiet bool) bool` function in `internal/updates/suppress.go` that evaluates all five suppression signals and returns true when notifications should be silenced. The `quiet` parameter bridges the cmd/internal boundary -- callers in cmd/tsuku pass the flag value, and the function handles everything else internally.

Precedence order (evaluated top-to-bottom, first match wins):
1. `TSUKU_AUTO_UPDATE=1` -- explicit opt-in, returns false (never suppress)
2. `TSUKU_NO_UPDATE_CHECK=1` -- explicit opt-out, returns true
3. `CI=true` -- environmental suppression, returns true
4. `quiet` parameter -- user chose silence, returns true
5. Non-TTY stdout -- scripted context, returns true
6. Default -- returns false (show notifications)

Call sites: `displayUnshownNotices` and any future notification renderers call this function before writing to stderr. The function is called at render time, not at startup, so it always reflects current state.

**Rationale**
Option A wins on simplicity and directness. The function has one job -- compose signals into a bool -- and the precedence order is readable in a single function body. The `quiet` parameter is an honest acknowledgment of the package boundary rather than a workaround.

The duplicated env var checks (CI, TSUKU_NO_UPDATE_CHECK are also in userconfig) are acceptable because the responsibilities differ. The userconfig methods answer "should this subsystem run at all?" while `ShouldSuppressNotifications` answers "should output be visible?" These are distinct questions that happen to consult some of the same signals.

Placing the function in internal/updates (not userconfig) keeps it next to its consumers. The primary caller is `displayUnshownNotices` in the same package, and future notification code will likely land here too.

**Alternatives Considered**
- **Extend userconfig with NotificationsEnabled()**: Follows existing patterns, but the method wouldn't use any Config fields -- it would only read env vars and parameters. A Config method that ignores `self` is a code smell. The two required parameters (`quiet`, `stdoutIsTTY`) make the method signature awkward compared to the zero-parameter `UpdatesEnabled()` and `UpdatesAutoApplyEnabled()` next to it.
- **Notification context struct**: Pre-computing suppression into a struct adds a threading requirement (every call site needs the struct parameter) for negligible performance gain. The env var lookups cost nanoseconds. The struct introduces a new concept -- `NotificationContext` -- that doesn't carry enough information to justify its existence. A bool is sufficient.

**Consequences**
- `displayUnshownNotices` gains a suppression check, closing the gap where CI pipelines receive unexpected stderr output.
- The internal/updates package gains a dependency on internal/progress (for `IsTerminalFunc`). This is a one-directional dependency and both packages are in the same layer.
- Future notification paths have a clear pattern: call `ShouldSuppressNotifications(quiet)` before writing to stderr.
- The `quiet` parameter must be plumbed from cmd/tsuku to any code that calls the suppression function. This is a minor cost that makes the dependency explicit rather than hidden.
<!-- decision:end -->
