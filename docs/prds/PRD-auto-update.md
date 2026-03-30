---
status: Draft
problem: |
  Tsuku has manual update and outdated commands, but no automatic update mechanism
  for managed tools or its own binary. The outdated command only checks GitHub-sourced
  tools, and the update command ignores version constraints from install time. Users
  must manually check and apply every update.
goals: |
  Automatically keep tools and tsuku itself current within user-defined version
  boundaries, with safe rollback on failure, configurable notifications, and
  suppression in CI environments.
---

# PRD: Auto-update

## Status

Draft

## Problem statement

Tsuku users install developer tools and then forget about them. Patches ship, security fixes land, minor versions improve things, but nothing happens unless the user remembers to run `tsuku outdated` and then `tsuku update` for each tool. Most don't.

Three specific problems make this worse:

1. **No automatic checking.** There's no background check, no periodic notification, nothing that tells users updates exist. The `outdated` command is explicit and manual.

2. **Blind spots in `outdated`.** The command only checks tools sourced from GitHub releases. Tools from PyPI, npm, crates.io, RubyGems, Homebrew, and other providers are invisible. That's a large chunk of the registry.

3. **`update` ignores version constraints.** Running `tsuku install node@18` records `Requested: "18"` in state, but `tsuku update node` resolves to absolute latest (currently Node 22). The constraint is stored but not used. This makes manual updates risky and automatic updates dangerous.

These problems compound. Users who try to stay current have incomplete information (`outdated` misses tools), and those who do update risk breaking their pin constraints. The result: most tools fall behind.

## Goals

1. Tools and tsuku itself stay current within the boundaries the user set at install time, without manual intervention.
2. Users who pin to a major version get patches and minors automatically. Users who pin to a minor get patches. Exact pins never change.
3. When an auto-update fails, the previous working version is preserved and the failure is reported clearly.
4. CI pipelines and non-interactive environments aren't affected by auto-update behavior.
5. Users can see what's available, control what updates, and roll back when something goes wrong.

## User stories

**US1: Daily developer, automatic patches.** As a developer with 15 tools installed, I want tools to receive updates automatically within my install-time constraints so that I get bug fixes and security patches without running commands manually.

**US2: Daily developer, major version awareness.** As a developer pinned to Node 20, I want to know when Node 22 is available so that I can decide whether to adopt it on my own schedule.

**US3: Daily developer, failed auto-update.** As a developer, I want a failed auto-update to automatically roll back to the previous working version and notify me on next use so that my workflow is never broken by an update, and I can see what went wrong.

**US3a: Daily developer, runtime breakage.** As a developer whose auto-updated kubectl passes installation but crashes at runtime, I want to revert to the previous version in one command so that I can keep working while the upstream bug is fixed.

**US4: CI pipeline, deterministic builds.** As a CI configuration, I want auto-update to be suppressed in non-interactive environments so that my builds don't change behavior without an explicit config change.

**US5: Team lead, shared tooling.** As a team lead maintaining `.tsuku.toml` across repositories, I want version constraints to express both "install this" and "auto-update within this range" so that my team gets patches without me updating the config for every point release.

**US6: Developer, tsuku self-update.** As a tsuku user, I want tsuku to update itself automatically so that I always have the latest features and recipe support without re-running the installer.

## Requirements

### Functional requirements

**R1: Version channel pinning.** The pin level is derived from what the user typed at install time. An empty string or "latest" tracks the latest stable version. "20" pins to major 20 (auto-updates within 20.x.y). "1.29" pins to minor 1.29 (auto-updates within 1.29.z). "1.29.3" is an exact pin (never auto-updates). The number of dot-separated components in the `Requested` field determines the pin level.

**R2: Channel-aware update resolution.** `tsuku update <tool>` must respect the `Requested` field. A tool installed with `tsuku install node@18` updates only within 18.x.y, not to the absolute latest. This replaces the current behavior where `update` ignores constraints.

**R3: Automatic update application.** When update checks find a newer version within the tool's pin boundary, tsuku automatically installs it. The lifecycle: a trigger (shell hook, shim, or tsuku command) detects the cache is stale and spawns a background process to check. The background process writes results to a cache file. On a subsequent tsuku command, tsuku reads the cache, downloads and installs pending updates, and notifies the user. The download/install phase runs during tsuku commands only (not during shell hooks or shim invocations) to avoid adding latency to tool execution or prompt rendering.

**R4: Time-cached update checks.** Update checks are cached with a configurable interval (default 24 hours, range 1h-30d). Checks don't repeat within the interval. A force flag (`--check-updates`) bypasses the cache. Staleness is determined by a single stat on the cache file's mtime — no JSON parsing on the hot path.

**R5: Non-blocking checks with layered triggers.** Update checks are triggered by three entry points in priority order:

1. **Shell activation hook** (primary, most frequent): For users with shell integration enabled (`tsuku hook install --activate`), the `hook-env` prompt command stats the update cache file on each prompt. If stale, it spawns a detached background process to check. This runs on every prompt and is the most reliable trigger. The stat check must add <5ms to prompt latency.
2. **Shim invocations** (secondary): For tools installed via recipe shims (`tsuku run` delegation), the shim path can trigger a staleness check before exec'ing the real binary. Not all tools use shims — most are plain symlinks.
3. **Direct tsuku commands** (fallback, least frequent): Any tsuku command (`install`, `list`, `outdated`, etc.) checks staleness and spawns a background check if needed. This is the only trigger for users without shell integration or shims.

In all cases, the check is non-blocking: the entry point stats the cache file, spawns a detached background process if stale, and proceeds immediately. The background process writes results to the cache file and exits. Shell integration should be strongly recommended during `tsuku` installation to ensure timely update checks.

**R6: All-provider support.** Update checks use the ProviderFactory to resolve versions from all supported sources (GitHub, PyPI, npm, crates.io, RubyGems, Homebrew, Go proxy, etc.). This replaces the current GitHub-only implementation in `outdated`.

**R7: Self-update.** `tsuku self-update` updates the tsuku binary using a rename-in-place mechanism. The running binary downloads the new version to a temp file, verifies its checksum, renames the old binary to `$TSUKU_HOME/bin/tsuku.old`, and renames the new binary into place. The backup is kept until the next successful self-update. Self-update always tracks the latest stable release; there's no version pinning for tsuku itself.

**R8: Self-update checks.** Tsuku's own version is included in the periodic update check. When a newer version is available, the notification appears alongside tool update notifications.

**R9: Rollback.** `tsuku rollback <tool>` switches to the immediately preceding active version (one step back) without re-downloading if the version directory is still on disk. Rollback history is one level deep; further rollback requires `tsuku install tool@version`. Rollback doesn't change the `Requested` field, so auto-update may re-apply the update on the next cycle. This is intentional: rollback is a temporary fix for a broken release, not a permanent pin change.

**R10: Automatic rollback on failure.** If an auto-update fails at any point (download, extraction, verification, symlink creation), the previous version remains active. No user intervention is needed. The failure is reported via the notification system.

**R11: Deferred failure reporting.** Failed auto-updates write a notice to `$TSUKU_HOME/notices/`. The notice is displayed on the next tsuku command invocation (stderr, once). Failures with fewer than 3 consecutive occurrences for the same tool are considered transient and suppressed. At 3+ consecutive failures, or on immediately actionable errors (checksum mismatch, disk full, recipe incompatibility), a notice is produced.

**R11a: Failure log.** `tsuku notices` (or `tsuku update-log`) displays all pending and recent update failure details, including the tool name, attempted version, error message, and timestamp. Users can review what went wrong after seeing a deferred failure notification.

**R12: Update notifications.** When updates are available (or have been applied), a stderr notification appears after the primary command's output. Notifications are suppressed when stdout is not a TTY, when `CI=true` is set, when `--quiet` is passed, or when `TSUKU_NO_UPDATE_CHECK=1` is set.

**R13: Out-of-channel notifications.** When a tool has a newer version available outside the user's pin boundary (e.g., pinned to 1.x but 2.0 exists), a separate notification can inform the user. This is configurable via `updates.notify_out_of_channel` (default: true). Out-of-channel notifications appear at most once per week per tool.

**R14: Batch updates.** `tsuku update --all` updates all outdated tools within their pin boundaries.

**R15a: All-provider outdated.** `tsuku outdated` checks tools from all version providers (via ProviderFactory), not just GitHub. This is a prerequisite fix.

**R15b: Pin-aware outdated display.** `tsuku outdated` shows two columns: "available within pin" and "available overall." This gives users a clear view of both what auto-update would do and what's available if they widen their pin. The `--json` flag provides structured output with both values.

**R16: CI environment detection.** Auto-update checks are suppressed when `CI=true` is set (standard across GitHub Actions, GitLab CI, CircleCI, etc.) or when stdout is not a TTY. Explicit opt-in via `TSUKU_AUTO_UPDATE=1` overrides CI detection.

**R17: Project-level interaction.** When `.tsuku.toml` specifies a version constraint, it takes precedence over global auto-update policy. `node = "20.16.0"` in `.tsuku.toml` means auto-update never touches node in that project context. `node = "20"` allows auto-update within 20.x.y.

**R18: Old version retention.** After a successful auto-update, the previous version directory is kept on disk for at least one auto-update cycle (configurable, default 7 days). A garbage collection mechanism removes older versions. This ensures rollback is fast (no re-download).

### Non-functional requirements

**R19: Zero added latency.** Background update checks must not add measurable latency to any tsuku command. The check goroutine has a 10-second absolute timeout.

**R20: Graceful offline degradation.** When network is unavailable, update checks fail silently. Cached results are used when available. No error output for transient network failures.

**R21: Atomic operations.** All file writes (state, cache, notices) use temp-file-then-rename for atomicity. Auto-update never leaves the system in a partially updated state.

**R22: Update outcome telemetry.** Successful and failed auto-updates are tracked via the existing telemetry system (extending `NewUpdateEvent`). Telemetry respects the existing opt-out mechanism.

## Acceptance criteria

### Version pinning
- [ ] `tsuku install ripgrep` (no version) enables auto-update to any newer version
- [ ] `tsuku install node@20` enables auto-update within 20.x.y only
- [ ] `tsuku install kubectl@1.29` enables auto-update within 1.29.z only
- [ ] `tsuku install terraform@1.6.3` disables auto-update entirely
- [ ] `tsuku update node` (after `install node@18`) resolves within 18.x.y, not absolute latest
- [ ] Pin level is derived from the `Requested` field, no new syntax required

### Automatic updates
- [ ] After the check interval elapses, the next trigger (shell hook, shim, or tsuku command) spawns a background update check
- [ ] The shell activation hook (`hook-env`) triggers staleness checks on every prompt when shell integration is enabled
- [ ] Shim invocations (`tsuku run` delegation) trigger staleness checks for shim-based tools
- [ ] Direct tsuku commands trigger staleness checks as a fallback for users without shell integration
- [ ] The staleness check is a single stat on the cache file (no JSON parsing, no network I/O on the hot path)
- [ ] The background check process is detached and doesn't block the calling process
- [ ] Updates within pin boundaries are downloaded and installed during the next tsuku command (not during shell hooks or tool execution)
- [ ] An auto-applied update is reported via stderr notification
- [ ] Tools with exact pins (3-component version) are never auto-updated
- [ ] When `updates.auto_apply = false`, available updates are reported but not installed
- [ ] The installer recommends enabling shell integration for timely update checks

### Self-update
- [ ] `tsuku self-update` downloads, verifies, and replaces the tsuku binary
- [ ] The old binary is preserved as a backup (`tsuku.old`)
- [ ] Self-update failure leaves the current binary functional
- [ ] Self-update is included in periodic update checks

### Rollback
- [ ] `tsuku rollback <tool>` switches to the immediately preceding active version (one step back)
- [ ] If the previous version directory exists, rollback completes without making network requests
- [ ] Rollback doesn't change the `Requested` field
- [ ] The rollback notification explains that auto-update may re-apply
- [ ] Further rollback beyond one step requires `tsuku install tool@version`

### Failure handling
- [ ] A failed auto-update automatically rolls back to the previous working version with no user intervention
- [ ] The deferred notice for a failed auto-update says the rollback happened, names the tool and versions, and points to `tsuku notices` for details
- [ ] `tsuku notices` displays full failure details (tool, attempted version, error, timestamp) for all recent failures
- [ ] Transient network failures (1-2 consecutive) don't produce notices
- [ ] Persistent failures (3+) or actionable errors produce one notice
- [ ] `tsuku doctor` detects orphaned staging directories and stale notices

### Notification and suppression
- [ ] Update notifications are written to stderr after command output
- [ ] Notifications are suppressed when `CI=true` is set
- [ ] Notifications are suppressed when stdout is not a TTY (indicating piped or scripted usage)
- [ ] Notifications are suppressed with `--quiet` or `TSUKU_NO_UPDATE_CHECK=1`
- [ ] When both `CI=true` and `TSUKU_AUTO_UPDATE=1` are set, update checks run (explicit opt-in overrides CI detection)
- [ ] Out-of-channel notifications appear at most once per week per tool (requires injectable clock for testing)
- [ ] Out-of-channel notifications can be disabled via config

### Configuration
- [ ] `config.toml` `[updates]` section supports: `enabled`, `auto_apply`, `check_interval`, `notify_out_of_channel`, `self_update`
- [ ] `TSUKU_NO_UPDATE_CHECK=1` disables all update checking
- [ ] `TSUKU_UPDATE_CHECK_INTERVAL` overrides the check interval
- [ ] `.tsuku.toml` version constraints take precedence over global auto-update policy
- [ ] Precedence order: CLI flag > env var > .tsuku.toml > config.toml > default

### Provider coverage
- [ ] `tsuku outdated` checks tools from all version providers, not just GitHub
- [ ] `tsuku outdated` shows "within pin" and "overall" columns
- [ ] `tsuku outdated --json` includes both values in structured output

### Offline and resilience
- [ ] When network is unavailable, update checks fail silently with no stderr output
- [ ] Cached check results are displayed when the network is unavailable
- [ ] Two concurrent `tsuku update <tool>` invocations result in one success and one clean failure, with no state corruption

### Batch and project
- [ ] `tsuku update --all` updates all tools with available versions within their pin boundaries
- [ ] A tool with an exact version in `.tsuku.toml` (e.g., `node = "20.16.0"`) is never auto-updated, even if global config has `updates.enabled = true`
- [ ] A tool with a prefix version in `.tsuku.toml` (e.g., `node = "20"`) allows auto-update within 20.x.y

## Out of scope

- **Pre-release channel opt-in** (beta/nightly): tools filter pre-releases by default. Named channels are a separate initiative.
- **Version range constraints in .tsuku.toml**: semver range syntax (e.g., `>=1.29, <2`) is more complexity than needed. Prefix-level pinning is sufficient.
- **Pre/post-update hooks**: user-defined scripts that run around updates. Separate feature surface.
- **Security advisory integration**: cross-referencing versions against vulnerability databases requires a data pipeline that doesn't exist. Reuses notification infrastructure when built.
- **Organization policy files**: shared version constraints across teams. Enterprise feature.
- **Scheduled/cron updates**: users who want unattended updates can `crontab tsuku update --all`.
- **Per-tool update configuration in config.toml**: global config sets defaults; per-tool overrides belong in `.tsuku.toml` only, avoiding two places to configure per-tool behavior.
- **Windows self-update**: tsuku targets Unix (Linux and macOS). Windows binary replacement has different constraints and can be addressed when Windows support is added.

## Known limitations

1. **Runtime breakage detection.** Tsuku can verify checksums (binary matches what upstream published) but can't detect runtime regressions in upstream releases. If an auto-updated tool is broken at runtime, the user must roll back manually. A future enhancement could add recipe-defined health checks.

2. **Concurrent same-tool updates.** Two tsuku processes updating the same tool simultaneously could race on the staging directory rename. The atomic rename means one wins and the other fails with an error, but no corruption occurs. Per-tool locking could be added later if this becomes a real problem.

3. **State-directory consistency gap.** If a tool directory is created but the subsequent `state.json` write fails, an orphaned directory exists without a state entry. `tsuku doctor` should detect this. The window is small but exists.

4. **No automatic rollback for runtime failures.** Auto-rollback handles installation failures (download, extraction, verification). It doesn't handle "installed successfully but broken when run." That requires the user to invoke `tsuku rollback`.

## Decisions and trade-offs

**D1: Auto-apply within pin boundaries (not check-and-notify).** The default behavior is to automatically install updates within the user's pin level, not just notify. This matches the "just works" philosophy. Users who want notification-only can set `updates.auto_apply = false` in config.toml. Alternative considered: notify-only by default (like gh, npm). Rejected because it adds a manual step that most daily developers won't take.

**D2: Pin level inferred from Requested field (not explicit field).** The number of version components the user typed at install time determines the pin level. No new `pin_level` field in state. This avoids schema migration and matches user intuition ("I typed 20, so I'm on the 20 track"). Alternative: explicit `PinLevel` field in state with possible values `major`/`minor`/`patch`/`exact`. Rejected for added complexity without clear benefit. Edge case with calver versions is acceptable.

**D3: Channel-aware resolution designed together with auto-update (not prerequisite fix).** The `Requested` field bug, pin-level semantics, and update policy are tightly coupled. Designing them as one system avoids rework. The alternative (ship the bug fix separately) was considered but rejected because the fix's behavior depends on auto-update design decisions.

**D4: Prerequisites included in auto-update scope (not separate PRs).** The Requested field bug fix and the outdated/ProviderFactory fix are part of the auto-update work. They could ship separately as bug fixes, but combining them avoids coordinating across multiple PRs.

**D5: Self-update as a separate code path (not self-as-managed-tool).** The tsuku binary uses rename-in-place, not the managed tool system. This avoids a bootstrap problem where a broken updater can't fix itself. Two update mechanisms to maintain, but the self-update path is simple (~30 lines of Go) and well-understood. Alternative: treat tsuku as its own managed tool (like proto). Rejected due to bootstrap risk.

**D6: Rollback moves to Phase 1 alongside auto-apply.** Since auto-apply is the default, users need a fast revert path from day one. Without rollback, a broken auto-update would require `tsuku install tool@previous-version` (which works but requires knowing the previous version). The multi-version directory model makes rollback cheap. Alternative: keep rollback in Phase 2 and rely on explicit install for reversion. Rejected because auto-apply without rollback is too risky for the default behavior.

**D7: Rollback is temporary, explicit install is permanent.** `tsuku rollback` switches back but doesn't change `Requested`, so auto-update will retry. `tsuku install tool@version` changes `Requested`, creating a permanent pin. This distinction lets users handle "broken release" (rollback, wait for fix) and "I need this exact version" (explicit install) differently.

## Phasing

### Phase 1: foundation + MVP

Prerequisite fixes and core auto-update in one delivery:

- Fix `tsuku update` to respect the `Requested` field (R2)
- Fix `tsuku outdated` to use ProviderFactory for all providers (R6, R15a)
- Cache `ResolveLatest` results (supports R4)
- Version channel pinning semantics (R1)
- Time-cached background update checks (R4, R5)
- Auto-apply updates within pin boundaries (R3)
- Rollback command and auto-rollback on failure (R9, R10, R11a)
- Basic notification UX with CI suppression (R12, R16)
- Update configuration in config.toml (R4 configurability)
- Self-update binary mechanism and checks (R7, R8)
- `tsuku self-update` command

### Phase 2: polish and resilience

- Pin-aware outdated display with dual columns (R15b)
- Deferred failure reporting with consecutive-failure suppression (R11)
- Out-of-channel notifications (R13)
- Batch update `--all` (R14)
- Old version retention and garbage collection (R18)
- Project-level interaction with .tsuku.toml (R17)
- Graceful offline degradation (R20)
- Update outcome telemetry (R22)
