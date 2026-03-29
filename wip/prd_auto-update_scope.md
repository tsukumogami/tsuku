# /prd Scope: auto-update

## Problem Statement

Tsuku provides manual `update` and `outdated` commands for managing tool versions, but has no automatic update mechanism for either managed tools or its own binary. Users must manually check for and apply updates, the `outdated` command only covers GitHub-sourced tools (missing all other providers), and the existing `update` command ignores version constraints established at install time -- meaning `tsuku install node@18` followed by `tsuku update node` can silently jump to Node 22. An auto-update system would keep tools and tsuku itself current within user-defined boundaries, with safe rollback on failure.

## Initial Scope
### In Scope
- Self-update mechanism for the tsuku binary (rename-in-place pattern)
- Automatic update of managed tools within version pin boundaries
- Version channel pinning at major, minor, or patch granularity (prefix-level model)
- Channel-aware resolution that respects the `Requested` field during updates (fixing the current bug where constraints are ignored)
- Time-cached update checks with configurable intervals (default 24h)
- Force-check command for on-demand update discovery
- Automatic rollback to previous version on update failure
- Deferred failure reporting (file-based notice queue, displayed on next invocation)
- Configurable update notifications with levels (off/pinned/all)
- Out-of-channel notification (e.g., "you're on 1.x but 2.0 is available")
- CI/non-interactive environment detection (TTY check, CI env var, explicit suppression)
- Listing all tools with available upgrades (fixing `outdated` to use ProviderFactory)
- `tsuku update --all` for batch updates
- Update outcome telemetry (extending existing telemetry infrastructure)
- Graceful offline degradation (use cached results when network unavailable)

### Out of Scope
- Pre-release channel opt-in (beta/nightly) -- separate initiative
- Version range constraints in `.tsuku.toml` -- separate design surface
- Pre/post-update hooks -- separate feature
- Security advisory integration -- requires vulnerability data pipeline
- Organization policy files -- enterprise feature, different audience
- Scheduled/cron updates -- users can use system cron with `tsuku update --all`

## Research Leads
1. **User stories and acceptance criteria**: The exploration captured behavioral requirements but not formal user stories. The PRD needs to define who the users are (developer using tsuku daily, CI pipeline, team lead managing shared tooling) and what success looks like for each.
2. **Priority ordering and phasing**: The scope is large. The PRD should establish which capabilities are MVP (minimum to ship) vs. follow-on. Self-update, basic pinning, and cached checks are likely MVP; out-of-channel notifications and batch update could follow.
3. **Configuration surface design**: Multiple config touchpoints emerged (config.toml, environment variables, CLI flags, .tsuku.toml). The PRD needs to define the configuration model holistically -- which settings live where and what takes precedence.
4. **Edge cases and failure scenarios**: What happens when the user is offline, when disk is full, when two tsuku processes update concurrently, when a recipe changes format between versions? These need acceptance criteria.

## Coverage Notes
The exploration thoroughly covers technical feasibility, existing infrastructure, and patterns from other tools. What it does NOT cover:
- Formal user personas and their specific needs
- Priority/sequencing of capabilities within the feature
- Configuration precedence rules (env var vs. config file vs. CLI flag vs. project file)
- Interaction between project-level pinning (.tsuku.toml) and global auto-update behavior
- Windows-specific self-update considerations (not relevant if tsuku only targets Unix)

## Decisions from Exploration
- Channel-aware resolution (respecting the `Requested` field) will be designed together with auto-update, not as a prerequisite fix. The Requested field, pin-level semantics, and update policy are tightly coupled -- designing them as one system avoids rework.
