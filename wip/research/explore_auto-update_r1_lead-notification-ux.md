# Lead: What UX patterns exist for update notifications?

## Findings

### 1. Suppression Mechanisms Across CLI Tools

CLI tools use a layered approach to notification suppression, combining environment variables, config files, and CLI flags. The pattern is remarkably consistent across the ecosystem:

| Tool | Env Var Suppression | Config File | CLI Flag | Default Check Interval |
|------|-------------------|-------------|----------|----------------------|
| gh | `GH_NO_UPDATE_NOTIFIER` | none | none | 24 hours |
| npm | `NO_UPDATE_NOTIFIER` | `update-notifier=false` in `.npmrc` | `--no-update-notifier` | varies (configurable) |
| pip | `PIP_DISABLE_PIP_VERSION_CHECK` | `disable-pip-version-check = true` in pip.conf | `--disable-pip-version-check` | every invocation |
| Terraform | `CHECKPOINT_DISABLE` | `disable_checkpoint = true` in `.terraformrc` | none | every invocation |
| Homebrew | `HOMEBREW_NO_AUTO_UPDATE` | none | none | every 5 minutes (API check) |
| gcloud | `CLOUDSDK_COMPONENT_MANAGER_DISABLE_UPDATE_CHECK` | via `gcloud config` | `--quiet` | periodic |
| rustup | none (no automatic notification yet) | none | `--no-self-update` | manual only |

**Key pattern**: Every tool that shows update notifications also provides at least one environment variable to suppress them. This is the minimum bar for CI/scripting friendliness.

### 2. Notification Output Patterns

All tools examined write update notifications to **stderr**, not stdout. This is critical for scriptability -- stdout carries machine-readable output while stderr carries advisory messages.

**gh CLI format** (stderr):
```
A new release of gh is available: 2.40.0 -> 2.41.0
To upgrade, run: gh upgrade
https://github.com/cli/cli/releases/tag/v2.41.0
```

**npm update-notifier format** (stderr, boxed):
```
+-----------------------------------------+
|   Update available: 5.0.0 -> 6.0.0     |
|   Run npm install -g npm to update      |
+-----------------------------------------+
```

**pip format** (stderr):
```
WARNING: A new release of pip is available: 23.2 -> 23.3
WARNING: To update, run: pip install --upgrade pip
```

**Terraform format** (stdout, during `terraform version`):
```
Your version of Terraform is out of date! The latest version
is 1.6.0. You can update by downloading from https://...
```

Terraform is the outlier -- it includes an `terraform_outdated` boolean in `terraform version -json` output, making it machine-parseable.

### 3. CI and Non-Interactive Environment Detection

The npm `update-notifier` package (used by hundreds of CLI tools) established a comprehensive detection pattern:

1. **TTY check**: Only notify if stdout is a terminal
2. **CI detection**: Suppress if `CI` env var is set (covers GitHub Actions, GitLab CI, Jenkins, etc.)
3. **npm script detection**: Suppress if running inside `npm run` (configurable via `shouldNotifyInNpmScript`)
4. **NODE_ENV check**: Suppress if `NODE_ENV=test`
5. **Explicit opt-out**: `NO_UPDATE_NOTIFIER` env var

**tsuku's current state**: tsuku already has TTY detection via `term.IsTerminal()` (used in `cmd/tsuku/install.go:342` and `cmd/tsuku/config.go:126`), and environment variable conventions via the `TSUKU_*` prefix pattern. The `--quiet` flag suppresses informational messages via `printInfo`/`printInfof` helpers, and the `--json` flag provides structured output for `outdated` and other commands.

### 4. Structured Output for Scripting

The `tsuku outdated` command already supports `--json` output with a clean schema:

```json
{
  "updates": [
    {"name": "kubectl", "current": "1.28.0", "latest": "1.29.0"}
  ]
}
```

For update notifications (as opposed to the explicit `outdated` command), tools take different approaches:

- **Terraform**: Embeds `terraform_outdated: true` in `terraform version -json`
- **gh**: No structured notification format -- stderr only
- **npm**: No structured format for the notification itself

The pattern that emerges: explicit commands (`outdated`, `version`) should have structured output. Passive notifications (shown after other commands) should be stderr-only and suppressible.

### 5. Out-of-Channel Notification Patterns

This is where existing tools are weakest. Most tools don't have the concept of "channel pinning" with cross-channel awareness:

**rustup** is the closest model. Users pin to channels (stable, beta, nightly) and `rustup check` shows updates within your channel. There's no automatic "nightly has feature X" notification when you're on stable. Cross-channel awareness is entirely manual.

**Homebrew** pins formulae to specific taps but doesn't notify about alternative versions. The `brew livecheck` command checks for newer upstream versions but isn't automatic.

**Docker Desktop** has notification levels in its settings UI: "Always", "Only for new major versions", "Never". This is the closest to what tsuku needs -- but it's a GUI application, not a CLI.

**Node.js/nvm** doesn't notify about new LTS versions when you're on an older LTS. `nvm ls-remote` is the manual check.

**No CLI tool examined implements configurable out-of-channel notifications.** This would be a differentiating feature for tsuku.

### 6. Tsuku's Existing Infrastructure

Relevant code paths for building notification support:

- **Output helpers** (`cmd/tsuku/helpers.go`): `printInfo`/`printInfof` respect `--quiet`, `printWarning` writes to stderr and respects quiet, `printJSON` for structured output
- **Config system** (`internal/userconfig/userconfig.go`): TOML-based config at `$TSUKU_HOME/config.toml` with `Get`/`Set` API, supports boolean, string, duration, and numeric types
- **Environment variables** (`internal/config/config.go`): Established `TSUKU_*` prefix convention; `TSUKU_QUIET`, `TSUKU_VERBOSE`, `TSUKU_DEBUG` already control log levels
- **Telemetry notice** (`internal/telemetry/notice.go`): Existing pattern for one-time stderr notices with marker files -- directly reusable for update notification cadence control
- **TTY detection**: `term.IsTerminal()` used in multiple places
- **Log levels**: slog-based with WARN default, configurable via flags and env vars
- **Deprecation warnings** (`cmd/tsuku/helpers.go`): `checkDeprecationWarning` shows registry deprecation notices on stderr, with `sync.Once` to fire at most once per invocation -- a model for update notifications

### 7. Notification Level Taxonomy

Synthesizing patterns across tools, notification approaches fall into these categories:

| Level | When | Where | Suppressible By |
|-------|------|-------|----------------|
| **Silent** | Never | n/a | default for CI |
| **Passive** | After command completes, at most daily | stderr | env var, config, `--quiet` |
| **Active** | Before command runs, blocks briefly | stderr | config only |
| **Forced** | Security updates, breaking changes | stderr | nothing (always shown) |

Most CLI tools operate at the **Passive** level with env var suppression. None implement **Active** (blocking) notifications for updates. **Forced** notifications are rare and reserved for critical security advisories.

## Implications

1. **Minimum viable notification**: stderr hint after command completion, suppressed by `TSUKU_NO_UPDATE_NOTIFIER` env var and `--quiet` flag. This matches every other CLI tool and is table stakes.

2. **Config-driven notification levels**: tsuku's existing config system (`userconfig`) can support a `notifications.updates` key with values like `off`, `pinned`, `all`. This maps cleanly to the `Get`/`Set` API pattern.

3. **Out-of-channel is novel territory**: No CLI tool does this well. tsuku could define it as: when `notifications.updates = all`, show a low-priority stderr hint about major version availability even when pinned to a minor channel. When `= pinned` (default), only show updates within the pinned channel.

4. **Structured output belongs in explicit commands**: The `tsuku outdated --json` path already handles scripting. Passive notifications should never appear in stdout or JSON output.

5. **CI auto-detection should supplement env vars**: Checking `CI=true` (standard across GitHub Actions, GitLab, etc.) in addition to `TSUKU_NO_UPDATE_NOTIFIER` reduces friction for CI users who forget to set tool-specific vars.

6. **The telemetry notice pattern is reusable**: The marker-file approach in `internal/telemetry/notice.go` (check marker, show notice to stderr, create marker) maps directly to time-cached update notifications. Replace the boolean marker with a timestamp marker.

7. **Deprecation warning pattern is the code template**: `checkDeprecationWarning` with `sync.Once` is already the right structure for "show at most once per invocation." Extending this to "show at most once per 24 hours" requires adding a timestamp file check.

## Surprises

1. **rustup has no automatic update notifications at all.** There have been proposals to add them (checking on every `cargo` invocation) but they've been deferred due to performance concerns. This suggests that non-blocking, background check approaches matter.

2. **Homebrew actively discourages suppressing auto-update.** They added a nag message when `HOMEBREW_NO_AUTO_UPDATE` is set, telling users to reconsider. This is an anti-pattern for CI -- tsuku should never nag about suppression.

3. **Terraform puts update information in stdout** (not stderr) when running `terraform version`, but includes a machine-readable `terraform_outdated` field in JSON output. This hybrid approach is worth considering for `tsuku version --json`.

4. **No tool examined separates "check frequency" from "notification frequency."** gh checks every 24 hours and notifies on every run if outdated. There's no concept of "check daily but only remind weekly." This could reduce notification fatigue for tsuku.

5. **The `CI` environment variable is nearly universal** as a signal for non-interactive environments. GitHub Actions, GitLab CI, CircleCI, Travis CI, Jenkins (usually), and Azure Pipelines all set it. Checking this one variable covers the majority of CI systems.

## Open Questions

1. **Should out-of-channel notifications include a migration guide link?** For example, "kubectl 2.0 is available (you're pinned to 1.x). See https://tsuku.dev/migrate/kubectl-2" -- this requires recipe metadata that doesn't currently exist.

2. **What's the right default for notification level?** `pinned` (only in-channel) is less noisy, but `all` helps users discover major upgrades they might want. The npm ecosystem defaults to showing everything; gh defaults to showing everything. The conservative choice is `all` with easy suppression.

3. **Should tsuku detect CI environments automatically?** The `CI=true` check is cheap, but it creates implicit behavior that can confuse users debugging CI pipelines. An explicit `TSUKU_NO_UPDATE_NOTIFIER=1` in CI config is more transparent.

4. **How should notifications interact with shell integration?** If tsuku adds shell hooks (prompt integration), update notifications in the hook path must be zero-cost or they'll slow down every prompt. The `cd` hook pattern from the shell integration work (blocks 4-6 in recent commits) needs to be considered.

5. **Should check frequency and notification frequency be separate config values?** Checking can happen silently in the background (writing to a cache file), while notification cadence controls how often the user sees a message. This two-phase approach prevents redundant network calls while reducing notification fatigue.

## Summary

CLI tools universally use stderr for update notifications with environment variable suppression (`GH_NO_UPDATE_NOTIFIER`, `NO_UPDATE_NOTIFIER`, `CHECKPOINT_DISABLE`), TTY detection to auto-suppress in non-interactive contexts, and time-cached checks (typically 24 hours) -- tsuku's existing `printWarning`, `term.IsTerminal()`, and telemetry marker-file patterns provide the building blocks for all of these. No CLI tool implements configurable out-of-channel notifications (e.g., "you're pinned to 1.x but 2.0 exists"), making this a differentiating feature if tsuku adds a `notifications.updates` config key with levels like `off`/`pinned`/`all`. The biggest open question is whether to auto-detect CI environments via the `CI` env var or require explicit `TSUKU_NO_UPDATE_NOTIFIER` -- the former reduces friction but adds implicit behavior that can surprise users debugging pipeline output.
