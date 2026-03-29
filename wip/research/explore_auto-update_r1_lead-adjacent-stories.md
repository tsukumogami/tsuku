# Lead: What adjacent stories should be in scope?

## Findings

### 1. Pre-release / Beta / Nightly Channels

**What mature tools offer:**

- **rustup**: First-class channel model (`stable`, `beta`, `nightly`). Users pin per-toolchain: `rustup default nightly`. Channels are separate release trains, not just version filters.
- **Homebrew**: Versioned formulae (`python@3.11`) act as channel-like pinning. No explicit beta channel, but `--HEAD` installs from source tip. Cask has `--no-quarantine` for pre-release apps.
- **nvm / volta / mise / proto**: Support installing pre-release versions by explicit tag (e.g., `nvm install v21.0.0-rc.1`), but don't have named channels. Version aliases (`lts/*`, `stable`) serve a similar role.
- **Docker**: Tags serve as channels (`latest`, `alpine`, `slim`, specific versions). No formal channel subscription, but `docker pull` defaults to `:latest`.
- **npm / pip**: Pre-release versions exist in the registry but require explicit opt-in (`npm install foo@next`, `pip install --pre`). npm's `dist-tag` system (`latest`, `next`, `canary`) is the closest to named channels.
- **Claude Code**: Has `stable` and `preview` channels. Users switch with `claude update --channel preview`. The channel is sticky -- subsequent updates follow the same channel.

**Tsuku's current state:**

Tsuku already records a `Requested` field in `VersionState` (see `internal/install/state.go:17`) that stores what the user asked for, including `@lts` style tags. The version providers (GitHub, npm, PyPI, etc.) resolve specific versions from these requests. However, there is no concept of a named channel that persists across updates. The `Requested` field is stored but there is no evidence it's used during `tsuku update` to re-resolve within the same constraint.

**Adjacent story: Channel-aware updates.** When a user installs `node@lts`, subsequent `tsuku update node` should re-resolve within the LTS channel, not jump to the latest non-LTS release. This requires:
- Persisting the channel/constraint in state (already partially done via `Requested`)
- Using `Requested` during update resolution (not currently implemented -- `cmd/tsuku/update.go` calls `runInstallWithTelemetry` with empty version)
- Defining which version providers support channel semantics vs. just version prefixes

**Pre-release channels** (beta, nightly, RC) are a separate concern. Most of tsuku's version providers (GitHub releases, Homebrew formulae) already distinguish pre-releases. The question is whether tsuku should:
1. Filter them out by default and require opt-in (like pip's `--pre`)
2. Support named channels that include pre-releases (like rustup)
3. Both

Given tsuku's philosophy of "just works" defaults, filtering pre-releases by default with explicit opt-in seems right. Named channels could come later.

### 2. Organizational / Team Version Locks

**What mature tools offer:**

- **asdf / mise**: `.tool-versions` file at project root pins versions for the team. mise extends this with `.mise.toml`. Both auto-switch when you `cd` into the project.
- **volta**: `package.json` `volta` field pins Node, npm, and yarn versions. Enforced across the team. `volta pin` command to set.
- **nvm**: `.nvmrc` file, but no enforcement. Just a convention.
- **proto**: `.prototools` file for per-project version pinning. Supports global, user, and project scopes.
- **Docker**: `docker-compose.yml` with image digests for exact pinning.
- **npm/pip**: Lockfiles (`package-lock.json`, `Pipfile.lock`) pin transitive dependency versions.

**Tsuku's current state:**

Tsuku already has a project configuration system (`internal/project/config.go`). The `.tsuku.toml` file supports per-project tool pinning:

```toml
[tools]
node = "20.16.0"
python = { version = "3.12" }
```

The `project.Resolver` (`internal/project/resolver.go`) resolves command-to-version mappings using the binary index and project config. This is already integrated with the auto-install system via the `autoinstall.ProjectVersionResolver` interface.

**What's missing for org-level policies:**
- No "organization config" or shared policy file that applies across projects
- No version range constraints (only exact versions in `.tsuku.toml`)
- No mechanism to prevent installing unapproved tools or versions
- No `StrictRegistries`-like enforcement for version policy (though `StrictRegistries` in `userconfig.go:51` exists for registry sources)

**Adjacent stories:**
- **Version range constraints in .tsuku.toml**: Allow `node = ">=20"` or `node = "~20.16"` so project configs can express "at least this version" rather than exact pins. Interacts with auto-update because the update system needs to know whether to respect the project constraint.
- **Organization policy file**: A shared config (fetched from a URL or git repo) that enforces approved tool versions across all projects. Higher complexity, lower immediate value for individual users.
- **Update policy per tool**: In `.tsuku.toml`, specify whether a tool should auto-update, stay pinned, or follow a range. E.g., `node = { version = "20.16.0", update = "minor" }` meaning "auto-update within 20.x but not to 21".

### 3. Update-on-Install Mode (Lazy Updates)

**What mature tools offer:**

- **Homebrew**: `brew install` installs latest by default. `brew upgrade` is a separate explicit action. No "check for updates when you use the tool" mode.
- **mise**: Checks for updates on `mise use` (install) but not on every tool execution. Has `mise outdated` for batch checking.
- **proto**: Checks for updates during `proto use` and shows a notification. Doesn't auto-update without consent.
- **npm**: `npx` always fetches latest unless pinned. `npm install` follows `package.json` constraints.
- **pip**: No auto-update. `pip install --upgrade` is explicit.
- **Claude Code**: Checks for updates at startup, shows notification, doesn't block.

**Tsuku's current state:**

Tsuku has `tsuku update <tool>` (explicit) and `tsuku outdated` (check all). The `outdated` command iterates all installed tools and checks GitHub for newer versions (`cmd/tsuku/outdated.go`). The `update` command reinstalls the tool at latest version. Neither runs automatically.

The version cache (`internal/version/cache.go`) already has TTL-based caching for version lists, which could be reused for update-check caching.

**Adjacent stories:**
- **Check-on-run**: When a shim executes a tool, optionally check for updates in the background (non-blocking) and display a notification next time. This is the "deferred notification" pattern from the core auto-update scope.
- **Update-on-install**: When `tsuku install <tool>` is run for an already-installed tool, check if a newer version matches the requested constraint and offer to update. Low complexity, natural UX.
- **Lazy background checks**: A background process or shell hook that periodically checks for updates. Higher complexity, needs careful resource management.

### 4. Security-Only Updates

**What mature tools offer:**

- **npm**: `npm audit fix` applies only security patches. Separate from `npm update`.
- **Dependabot / Renovate**: Can be configured for security-only updates.
- **apt/yum**: `unattended-upgrades` package supports security-only mode.
- **Docker**: Trivy/Snyk scan for vulnerabilities in images.

**Tsuku's current state:**

No security advisory integration. Tsuku's version providers resolve from upstream sources (GitHub, PyPI, npm, etc.) but don't cross-reference vulnerability databases. The `go vet` and `govulncheck` runs in CI (see `lint_test.go`) check tsuku's own dependencies, not the tools it manages.

**Adjacent story: Security advisory notifications.** When checking for updates, cross-reference the installed version against known vulnerabilities (GitHub Security Advisories, OSV database). Notify users of security-relevant updates with higher urgency than feature updates. This is a significant initiative requiring a vulnerability data source, but the notification infrastructure built for auto-update would be reusable.

**Assessment:** High user value but high complexity. Not a natural extension of core auto-update -- it requires a vulnerability data pipeline. Should be tracked as a separate initiative that depends on the update notification system.

### 5. Update Hooks / Scripts

**What mature tools offer:**

- **Homebrew**: Post-install scripts via `postflight` in Casks. No user-defined hooks.
- **npm**: `preinstall`, `postinstall`, `preupdate`, `postupdate` lifecycle scripts.
- **pip**: No hooks.
- **asdf**: Plugin system with `bin/post-install` hook scripts.
- **Docker**: `HEALTHCHECK` for post-deploy validation.

**Tsuku's current state:**

Tsuku has a hook system (`internal/hook/`) but it's for shell integration (command-not-found, activation), not for install/update lifecycle events. The recipe `verify` section (`internal/recipe/types.go`) runs post-install verification, but this is recipe-defined, not user-defined.

**Adjacent stories:**
- **Pre/post-update hooks in config**: Allow users to define commands that run before or after a tool updates. E.g., "after updating node, run `npm rebuild`." Low complexity if built on existing recipe verify patterns.
- **Recipe-defined migration scripts**: Recipes could define version-specific migration steps (e.g., "when upgrading from 2.x to 3.x, run this migration"). Higher complexity, niche use case.

### 6. Telemetry for Update Success Rates

**What mature tools offer:**

- **Homebrew**: Analytics track install/upgrade events (opt-out).
- **npm**: Downloads tracked per version.
- **Rust**: No public update telemetry.

**Tsuku's current state:**

Tsuku already tracks update events via telemetry (`internal/telemetry/event.go:47-48`). The `NewUpdateEvent` function records recipe name, previous version, and new version. The telemetry client sends these to the Cloudflare Worker (`telemetry/`).

**Adjacent stories:**
- **Update outcome tracking**: Extend telemetry to track success/failure of updates, including rollback events. The deferred error reporting from core auto-update naturally produces these events.
- **Aggregate update health dashboard**: Use telemetry data to detect problematic recipe versions (high failure rate after update). This is a separate analytics initiative.

The telemetry infrastructure is already in place. Adding update outcome tracking is a natural extension of core auto-update with minimal additional complexity.

### 7. Scheduled / Batch Updates

**What mature tools offer:**

- **Homebrew**: `brew upgrade` updates all. No scheduling.
- **apt**: `unattended-upgrades` runs on cron.
- **Flatpak**: Auto-updates on a schedule.
- **Windows Update**: Scheduled update windows.
- **mise**: `mise upgrade` updates all tools at once.

**Tsuku's current state:**

`tsuku update` takes a single tool. `tsuku outdated` checks all tools but doesn't update them. No batch update command. No scheduling.

**Adjacent stories:**
- **`tsuku update --all`**: Batch update all tools (or all outdated tools). Low complexity, high user value. Natural extension of the update command.
- **Scheduled updates**: Cron-like scheduling for unattended updates. Higher complexity, probably overkill for a developer tool manager. Most users would just alias `tsuku update --all` in their own cron.

### 8. Offline Mode / Air-Gapped Environments

**What mature tools offer:**

- **Homebrew**: No offline support. Requires network.
- **npm**: `npm pack` / `npm cache` for offline installs.
- **Docker**: Registry mirrors, `docker save/load`.
- **proto**: No offline mode.

**Tsuku's current state:**

No offline mode. All version resolution and downloads require network. The version cache (`internal/version/cache.go`) provides some resilience for version listing but not for actual installation.

**Adjacent story:** When auto-update checks fail due to network unavailability, the system should gracefully degrade (use cached version info, skip update check) rather than error. This is partially covered by the "time-cached update checks" in core scope, but the graceful-degradation behavior deserves explicit attention.

## Implications

The adjacent stories fall into three categories by their relationship to core auto-update:

**Natural extensions (should be in scope or immediately follow):**
1. **Channel-aware updates** -- The `Requested` field already stores channel info but `tsuku update` doesn't use it. Fixing this is a prerequisite for sensible auto-update behavior. Without it, auto-updating `node@lts` could jump to a non-LTS version.
2. **`tsuku update --all`** -- Trivial once single-tool update works reliably.
3. **Update telemetry (outcome tracking)** -- Small addition to existing telemetry, provides valuable signal for rollback reliability.
4. **Graceful offline degradation** -- Required for the cached update check system to be useful.

**Dependent but separate initiatives:**
5. **Version range constraints in .tsuku.toml** -- Meaningful interaction with auto-update policy, but the project config system needs its own design work.
6. **Pre-release channel opt-in** -- Useful but not blocking. Could default to filtering pre-releases and add later.
7. **Pre/post-update hooks** -- Nice to have, separate design surface.

**Independent initiatives (track separately):**
8. **Security advisory integration** -- Different data pipeline, different UX. Would reuse notification infrastructure.
9. **Organization policy files** -- Enterprise feature, different audience.
10. **Scheduled updates** -- Users can use cron; tsuku doesn't need to reinvent this.

### Critical decision for core scope

The biggest decision this research surfaces is whether **channel-aware updates** belong in the core auto-update design or as a follow-on. The argument for including it: the `Requested` field is already stored but not used during updates, which means the current `tsuku update` is subtly broken for users who installed with version constraints. Auto-update would inherit this bug and make it worse (silently jumping channels). The argument against: it adds complexity to an already large design.

Recommendation: Include channel-aware updates in the core design, at least at the data model level. The `Requested` constraint should be re-resolved during updates. The full channel subscription model (named channels like rustup's stable/beta/nightly) can follow later.

## Surprises

1. **The `Requested` field is already stored but not used during updates.** This is effectively a latent bug: `tsuku install node@18` records `Requested: "18"`, but `tsuku update node` ignores this and resolves to latest (which might be node 22). This means the auto-update system inherits a broken foundation unless channel-aware resolution is addressed.

2. **The project config system (.tsuku.toml) is more mature than expected.** It already has version pinning, command-to-recipe resolution via binary index, and integration with auto-install. This means per-project update policies could build on existing infrastructure rather than starting from scratch.

3. **Tsuku's version providers are diverse but all resolve to exact versions.** There's no concept of "version stream" or "version constraint" at the provider level -- providers return a single `VersionInfo` for "latest" or a specific version. Supporting "latest within constraint" (e.g., "latest 18.x") would require new provider methods or a filter layer on top of the version list.

4. **The `StrictRegistries` config field hints at an organizational control model** that could extend to version policies. The pattern of "restrict what can be installed from where" is a natural companion to "restrict what versions can be updated to."

## Open Questions

1. **Should channel-aware updates be in the core auto-update scope or a prerequisite?** The current `tsuku update` doesn't respect the `Requested` constraint. Should the auto-update design fix this, or should it be a separate issue resolved first?

2. **What does "pre-release" mean across version providers?** GitHub releases have a pre-release flag. npm has dist-tags. PyPI has PEP 440 pre-release markers. Homebrew versioned formulae use `@` suffix. Should tsuku normalize these into a single "pre-release" concept or handle them per-provider?

3. **How should `.tsuku.toml` interact with auto-update?** If a project pins `node = "20.16.0"`, should auto-update:
   - Skip that tool entirely when in that project directory?
   - Update to latest 20.x (interpreting the pin as a minimum)?
   - Only update if the project config is also updated?

4. **Is `tsuku update --all` an MVP requirement or can it follow?** Batch updates are high user value but could be a separate PR after the core update mechanism is solid.

5. **Should version range constraints (semver ranges) in `.tsuku.toml` be designed concurrently with auto-update?** They interact heavily -- an auto-update system that respects project constraints needs to know the constraint language.

## Summary

Mature package managers support roughly ten adjacent features beyond basic auto-update, but only four are natural extensions of the core system: channel-aware updates (using the already-stored `Requested` field), batch update (`--all`), update outcome telemetry, and graceful offline degradation. The most important finding is that tsuku's `Requested` field stores channel/constraint information during install but `tsuku update` ignores it, creating a latent bug that auto-update would amplify -- this should be addressed as part of or before the core auto-update design. The biggest open question is how `.tsuku.toml` project-level version pins should interact with auto-update, since the project config system is more mature than expected and could either constrain or complicate the update model.
