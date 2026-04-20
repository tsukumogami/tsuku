# Lead: How do peer CLI tools handle non-blocking update checks?

## Findings

### Homebrew (brew)

**Mechanism**: Homebrew runs `brew update` (a `git fetch` of the formula tap, or a JSON API download in newer versions) synchronously before `brew install`, `brew upgrade`, and related commands. The staleness threshold is controlled by `HOMEBREW_AUTO_UPDATE_SECS` (default 300s for the JSON API via `HOMEBREW_API_AUTO_UPDATE_SECS`). If the tap was fetched within the threshold, the update is skipped entirely via an mtime check on `.git/FETCH_HEAD` or the downloaded JSON file. A separate optional daemon approach exists as the `homebrew-autoupdate` tap, which uses macOS launchd to run `brew update` on a schedule independent of user commands.

**Communication**: Updates print progress to stdout inline, blocking the user until complete. There's no "checking in background" notice — the update either happens before your command or is skipped.

**Footprint**: No persistent daemon in the default setup. The launchd-based autoupdate tap adds a system service, but it's opt-in.

**Known issues**: This is one of the most-complained-about aspects of Homebrew. Users hitting slow git operations or poor network conditions experience multi-minute delays before their install even starts. Issues #4755, #9285, and #10386 on the Homebrew repo each have dozens of comments requesting purely non-blocking behavior. The workarounds are: set `HOMEBREW_NO_AUTO_UPDATE=1` (disables entirely), increase `HOMEBREW_AUTO_UPDATE_SECS`, or use `HOMEBREW_NO_AUTO_UPDATE` in CI. The community considers this a design flaw rather than a tradeoff.

**Lesson**: Blocking is the wrong default. The time-based skip (staleness threshold) helps but doesn't eliminate the problem when the threshold has expired.

---

### npm / yarn (via `update-notifier`)

**Mechanism**: npm and many Node.js CLIs use the `update-notifier` package. When a command runs, the library checks the mtime of a cached result file. If the result is stale (default interval: 24 hours), it spawns an unref'd, detached child process to query the registry. The parent process continues immediately — it does not wait for the child. The child writes its result to a JSON file in the user's config directory. On the _next_ invocation, the parent reads the cached result and displays a notice if a newer version exists.

**Communication**: Notices appear on stderr at the end of the command output, separated visually (often boxed). The message is "A new release of npm is available: X → Y. Run npm install -g npm to update." This is purely informational; the update is never auto-applied.

**Footprint**: Zero persistent footprint. One short-lived child process per check cycle, which exits after writing its result.

**Known issues**: On Windows, `spawn({detached: true})` behavior differs and there are known bugs where the child process can prevent the parent from exiting (nodejs/node#5614). npm itself has had complaints that the check can hang when run behind a corporate proxy (npm/npm#20979). Displaying the notice on every subsequent command until the user upgrades is irritating for some users.

**Lesson**: The "check in background, notify next run" pattern separates the network operation entirely from the user's command. The two-phase design (spawn-and-forget now, read-and-display next time) is the clearest model for a non-blocking check.

---

### rustup

**Mechanism**: rustup performs self-update checks during `rustup update` and `rustup toolchain install` operations. The `auto-self-update` configuration variable controls behavior: `enable` (check and apply), `check-only` (report but don't apply), or `disable`. A proposal to add proactive background checking (issue #3688) was discussed: spawn a background check when cargo or rustup runs, wait up to ~100-200ms for a result, and display a notice if it completes in time or defer to the next run if it doesn't. As of 2026, rustup 1.29.0 added concurrent update checking in `rustup check`.

**Communication**: When an update is available, rustup prints a notice on stderr after the command completes. When auto-apply is enabled, it prints the update progress after the primary command output.

**Footprint**: No daemon. `rustup check` is an explicit command. Background checking, where implemented, is a short-lived goroutine or subprocess within the existing process.

**Known issues**: Issue #766 ("rustup update does not terminate while checking self-updates") showed the check could hang indefinitely on poor networks — blocking the command that triggered it. This led to the `--no-self-update` flag and CI-detection logic. The 100-200ms timeout proposal directly addresses the "what if the check is slow?" problem.

**Lesson**: A hard timeout on the background check is a useful guard. If the check can't complete within a perceptible threshold, defer the result rather than block. This preserves responsiveness without abandoning accuracy.

---

### gh (GitHub CLI)

**Mechanism**: `gh` checks for a newer release of itself by querying the GitHub releases API. The check happens asynchronously in a goroutine launched at command startup. A cache file prevents repeat checks within a 24-hour window (mtime-based staleness). The goroutine result is read after the main command completes; if a newer version is found, a notice is printed to stderr.

**Communication**: Notices appear on stderr after command output: "A new release of gh is available: v2.x.x → v2.y.y. To upgrade, run: gh upgrade." The notice is shown once per check cycle, not on every invocation.

**Footprint**: No daemon, no external process. The goroutine shares the process lifetime — it starts when the command starts and the result is collected (with a short deadline) before exit.

**Known issues**: In-process goroutines leak if the result is never collected. Tools that use goroutines for update checks need to ensure they don't delay process exit. `gh` handles this by reading the channel result at PostRun time with a short timeout rather than waiting indefinitely.

**Lesson**: In-process goroutines avoid the cross-platform complexity of detached process spawning. Reading the result at PostRun (after the user's command completes) gives the check time to run without blocking the command itself.

---

### apt / unattended-upgrades

**Mechanism**: `apt-get update` is always blocking when explicitly invoked. Automatic background updates are handled by a separate system service: two `systemd` timer units (`apt-daily.timer` and `apt-daily-upgrade.timer`) trigger at scheduled intervals with a random jitter (0-1800s by default) to avoid mirror stampedes. The CLI itself doesn't trigger background work — the daemon model is entirely decoupled from user commands.

**Communication**: Results from unattended-upgrades are logged to `/var/log/unattended-upgrades/`. Users see no inline notice during `apt install`. The lock file (`/var/lib/dpkg/lock`) blocks concurrent apt operations when the daemon is active.

**Footprint**: A persistent daemon (`snapd`/`systemd` services) plus lock files. This is the highest-footprint approach.

**Known issues**: The dpkg lock frequently blocks provisioning scripts and CI jobs that run `apt install` during startup while the automatic update daemon is also active. This is a well-known pain point in cloud environments.

**Lesson**: Full daemon separation eliminates any CLI latency but introduces lock contention and system-footprint costs that are inappropriate for a developer tool. This model doesn't transfer well to tsuku's context.

---

### snap / snapd

**Mechanism**: Snap uses a persistent background daemon (`snapd`) that checks for updates four times per day independently of user commands. The `snap refresh` command triggers an immediate check. There's no in-CLI update check at all — the daemon owns the update loop entirely.

**Communication**: Users see no inline notice. Snaps are updated silently in the background; running apps may be restarted. There's a "snap changes" command to review history.

**Footprint**: The highest of any tool surveyed. `snapd` is a system daemon that starts at boot. Confinement and update checking are inseparable from the daemon model.

**Known issues**: Users complain that snaps can be force-updated while in use, causing disruption. The daemon uses significant memory and adds boot time. The inability to easily disable automatic updates is a recurring complaint.

**Lesson**: Persistent daemons are overkill for update notification and introduce user-trust and control problems. Avoid.

---

### cargo

**Mechanism**: Cargo does not have a built-in self-update or update-notification mechanism. Users run `cargo install --locked <crate>` explicitly when they want a newer version. The `cargo-update` third-party subcommand adds this functionality. Cargo's main blocking operation is the registry index fetch ("Updating crates.io index"), which was historically a full git clone of the index and could take 75+ seconds on first use. Cargo 1.68 (2023) made the sparse protocol the default, reducing this to ~10 seconds by fetching only the specific crate metadata needed.

**Communication**: "Updating crates.io index" is printed inline and blocks the build. With sparse protocol, the wait is much shorter but still synchronous.

**Footprint**: No daemon. Registry data is cached in `~/.cargo/registry/`.

**Known issues**: Before the sparse protocol, the full-index fetch was the canonical example of a blocking registry operation making tools feel slow. The solution was not to defer the work but to make the work itself much cheaper (fetch only what's needed). This is a different class of fix than background deferral.

**Lesson**: Sometimes the right answer is to reduce the cost of the operation, not just defer it. Cargo's sparse protocol shows that redesigning the data-fetch shape can eliminate the latency problem at its source.

---

### pip

**Mechanism**: pip performs a self-version check synchronously at the end of most commands. It queries PyPI, writes the result to `selfcheck.json` (in the pip cache directory), and prints a notice if a newer pip is available. The check has a 7-day default interval (stored in the JSON file as a timestamp). It can be disabled via `--disable-pip-version-check` or `PIP_DISABLE_PIP_VERSION_CHECK=1`.

**Communication**: Notices appear as `[notice]` lines on stderr after the command output: "[notice] A new release of pip is available: 22.3 → 22.3.1. To update, run: pip install --upgrade pip."

**Footprint**: No daemon. Synchronous inline check.

**Known issues**: In air-gapped or proxy-restricted environments, the check hangs until the network timeout fires, adding 30+ seconds to every pip invocation. There's no graceful timeout. Issue #2269 on pypa/pip documents this. The check was later made non-fatal and given some timeout handling, but it remains synchronous. `pip list` performing a self-update check (issue #11677) was considered a design bug.

**Lesson**: Synchronous checks with no timeout are a reliability hazard on restricted networks. The `[notice]` formatting (lowercase, clearly secondary) is a good pattern for distinguishing update notices from primary output.

---

## Implications

**The dominant non-blocking pattern is two-phase**: spawn or goroutine to run the check concurrently or after the main work completes, write the result to a local file, read and display on the next command invocation. This pattern appears in npm (detached child + JSON file), gh (in-process goroutine + cache file), and is what tsuku's `trigger.go` / `notify.go` already implement for tool update checks.

**Staleness via mtime is the right primitive**: every well-designed tool uses filesystem mtime comparison rather than parsing a timestamp from a file. It's a sub-millisecond check and requires no parsing. Homebrew, npm's `update-notifier`, and tsuku all use this pattern.

**Timeout guards are essential**: rustup's experience (issue #766) and pip's air-gap problem both show that any check that can hit the network must have a hard timeout. The proposed 100-200ms "check in background, display if done, defer if not" model is the right mental frame.

**In-process goroutines vs. detached processes**: `gh` uses in-process goroutines; tsuku's `trigger.go` uses detached subprocess spawning. The goroutine approach avoids cross-platform exec semantics and is tidier for Go, but requires careful collection at process exit. The detached subprocess approach (tsuku's current model) is more resilient to main-process crashes and avoids goroutine leak risk, at the cost of cross-platform spawn behavior.

**Never block a user command on an update check**: Homebrew and pip are the cautionary examples. Both perform synchronous pre-command network operations and both generate consistent user frustration. The lesson is absolute: update checks must not gate the command the user asked for.

**Registry refresh is distinct from self-update notification**: Cargo's sparse-protocol fix is a reminder that if the blocking operation is a data fetch (registry index, recipe cache), the best solution may be redesigning the fetch to be incremental or cheaper, not just deferring it. Tsuku's registry refresh problem may benefit from this lens in addition to deferral.

**Notice placement matters**: pip's `[notice]` suffix pattern, npm's boxed stderr message, and gh's post-command stderr line all make update notices clearly secondary to the user's actual output. Notices that appear before command output (Homebrew's inline update progress) feel like interruptions; notices that appear after feel like footnotes.

---

## Surprises

**Homebrew's approach is universally considered wrong by its own community**: The number of open issues and the intensity of user complaints about blocking updates suggests this isn't an acceptable tradeoff even for a mature tool. Most users discover `HOMEBREW_NO_AUTO_UPDATE=1` and leave it set permanently — meaning the update mechanism fails at its goal anyway.

**npm's update-notifier is broken on Windows in specific configurations**: The `detached: true` + `unref()` pattern that works cleanly on Unix can cause the parent process to hang on Windows (nodejs/node#5614). This is relevant if tsuku targets Windows — the same risk may apply to tsuku's `spawnChecker()` which uses `cmd.Start()` without waiting.

**Cargo's sparse-protocol fix was more impactful than any deferral strategy**: A 75s → 10s improvement on the same synchronous fetch demonstrates that reducing operation cost can matter more than the blocking vs. non-blocking question. If tsuku's registry refresh fetches more than it needs, that's worth examining separately from the scheduling question.

**The rustup 100-200ms timeout proposal is a rarely documented but elegant design point**: Most tools either block fully or defer fully. The hybrid — wait a short time, display immediately if available, otherwise defer — is a middle path that hasn't been widely adopted but addresses the case where network conditions are good and users benefit from seeing the notice on the same run.

---

## Open Questions

1. **Does tsuku's `spawnChecker()` behave correctly on Windows?** The `cmd.Start()` without `SysProcAttr` process group settings may not produce a properly detached process on Windows. This should be verified before tsuku targets that platform.

2. **Is the registry refresh (cache refresh) currently blocking, and is it triggered on the same commands as update checks?** The scope document mentions "waits up to a minute" — if this is registry refresh rather than update notification, the fix may require either deferral (background fetch) or a sparse/incremental fetch redesign, not just extending the existing notification background pattern.

3. **What is the actual p50/p95 latency of tsuku's current blocking operations?** The "up to a minute" figure is worst-case. Understanding the distribution would clarify whether timeout-then-defer (rustup model) is viable or whether the operations must always be fully deferred.

4. **Should the same background execution mechanism handle both tool update checks and registry refresh, or are they different enough to warrant separate designs?** Update checks are low-data (version strings); registry refresh may involve downloading recipe files. The cost and frequency profiles differ.

5. **What happens to the detached `check-updates` subprocess if the user closes their terminal immediately after running a tsuku command?** On Linux/macOS, a detached child in a new process group should survive. On some systems, the child may receive SIGHUP if the session exits before the child starts its own session. This edge case is worth testing.

---

## Summary

The clearest transferable pattern is npm/gh's two-phase model: run the check concurrently (detached subprocess or goroutine), write the result to a cache file, and surface a notice on the next command invocation — which tsuku already implements for tool update notifications but likely hasn't extended to registry refresh. The main tradeoff is that users see the notice one command late, which is acceptable for informational updates but may feel odd if the blocking operation was the registry refresh needed for the command they're running right now. The biggest open question for tsuku is whether the "up to a minute" wait is from the update notification path (already non-blocking in current code) or from a registry/cache refresh that runs synchronously before the actual command work and needs a separate deferral design.
