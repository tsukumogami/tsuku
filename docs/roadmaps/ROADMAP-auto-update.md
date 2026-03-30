---
status: Draft
theme: |
  Auto-update capabilities for tsuku and managed tools. Users install developer
  tools and forget about them; this initiative keeps tools current within
  user-defined version boundaries, with safe rollback and configurable behavior.
  Coordinated sequencing matters because the version resolution model, check
  infrastructure, and apply logic are tightly coupled -- building them in the
  wrong order means rework or shipping unsafe defaults.
scope: |
  Covers the full auto-update system from PRD-auto-update: version channel
  pinning, background update checks, auto-apply with rollback, self-update,
  notifications, and resilience features. Excludes pre-release channels,
  security advisory integration, organization policy files, and scheduled
  updates -- these are separate initiatives that build on the auto-update
  infrastructure.
---

# ROADMAP: Auto-update

## Status

Draft

## Theme

Tsuku users install developer tools and forget about them. Patches, security fixes, and minor improvements ship upstream but nothing happens unless the user manually runs `tsuku outdated` and `tsuku update` for each tool. This roadmap coordinates the features needed to keep tools and tsuku itself current automatically, within version boundaries the user chose at install time.

Sequencing matters here. The version resolution model (how pins work) must be solid before checks can be meaningful, checks must exist before auto-apply makes sense, and rollback must ship alongside auto-apply since it's the default behavior. Getting this order wrong means either shipping an unsafe system or doing significant rework.

## Features

### Feature 1: Channel-aware version resolution
**Needs:** `needs-design` -- the pinning model, Requested field semantics, and version comparison logic need architectural decisions
**Dependencies:** None
**Status:** Not started
**Upstream:** [PRD-auto-update](../prds/PRD-auto-update.md) (R1, R2, R6, R15a)

The foundation. Fix `tsuku update` to respect the `Requested` field so that `install node@18` followed by `update node` stays within 18.x.y. Fix `tsuku outdated` to use ProviderFactory for all version providers (not just GitHub). Cache `ResolveLatest` results. Define pin-level semantics: empty string = latest, "20" = major, "1.29" = minor, "1.29.3" = exact. Everything else depends on this being right.

### Feature 2: Background update check infrastructure
**Needs:** `needs-design` -- layered trigger model (shell hook vs. shim vs. command), cache file format, and background process lifecycle need design
**Dependencies:** Feature 1
**Status:** Not started
**Upstream:** [PRD-auto-update](../prds/PRD-auto-update.md) (R4, R5)

The plumbing. Time-cached update checks with a configurable interval (default 24h). Three trigger entry points: shell activation hook (primary, runs on every prompt), shim invocations (secondary), and tsuku commands (fallback). Staleness detection via a single stat on the cache file. Background process spawns detached and writes results to `$TSUKU_HOME/cache/update-check.json`. Update configuration in `config.toml` `[updates]` section.

### Feature 3: Auto-apply with rollback
**Needs:** `needs-design` -- the apply lifecycle (when to apply, state locking, rollback mechanism) needs design alongside the check infrastructure
**Dependencies:** Feature 1, Feature 2
**Status:** Not started
**Upstream:** [PRD-auto-update](../prds/PRD-auto-update.md) (R3, R9, R10, R11a)

The core behavior. When cached check results show a newer version within pin boundaries, tsuku downloads and installs it during the next tsuku command. If installation fails, the previous version is automatically preserved and a failure notice is written. `tsuku rollback <tool>` switches to the immediately preceding version (one level deep). `tsuku notices` displays failure details. This ships together with auto-apply because auto-apply without rollback is unsafe as the default.

### Feature 4: Self-update
**Needs:** `needs-design` -- binary replacement mechanism (rename-in-place vs. alternatives) and version check integration need design
**Dependencies:** None (uses Feature 2's check infrastructure when available, but can ship independently)
**Status:** Not started
**Upstream:** [PRD-auto-update](../prds/PRD-auto-update.md) (R7, R8)

Independent from tool auto-update. `tsuku self-update` downloads the latest tsuku binary, verifies its checksum, renames the old binary to `tsuku.old`, and renames the new one into place. Self-update always tracks latest (no pinning for tsuku itself). When Feature 2's check infrastructure exists, tsuku's own version is included in the periodic check. Can ship before or after the tool auto-update features.

### Feature 5: Notification system
**Needs:** `needs-design` -- notification timing, suppression layers, and configuration surface need design
**Dependencies:** Feature 2 (needs check results to display), Feature 3 (needs apply results to report)
**Status:** Not started
**Upstream:** [PRD-auto-update](../prds/PRD-auto-update.md) (R12, R16)

Cross-cutting. Stderr notifications after command output for available or applied updates. Suppression layers: non-TTY, `CI=true`, `--quiet`, `TSUKU_NO_UPDATE_CHECK=1`. `TSUKU_AUTO_UPDATE=1` overrides CI detection for explicit opt-in. The notification format and suppression logic are shared across tool updates and self-update.

### Feature 6: Update polish
**Dependencies:** Feature 1, Feature 3, Feature 5
**Status:** Not started
**Upstream:** [PRD-auto-update](../prds/PRD-auto-update.md) (R13, R14, R15b)

Refinements that build on the core system. Pin-aware `tsuku outdated` with dual columns ("within pin" and "overall"). Out-of-channel notifications when a newer version exists outside the pin boundary (configurable, at most weekly per tool). `tsuku update --all` for batch updates within pin boundaries.

### Feature 7: Resilience
**Dependencies:** Feature 3 (extends auto-apply with failure handling)
**Status:** Not started
**Upstream:** [PRD-auto-update](../prds/PRD-auto-update.md) (R11, R18, R20)

Hardening for real-world conditions. Deferred failure reporting with consecutive-failure suppression (fewer than 3 consecutive = transient, suppressed). Old version retention with configurable period (default 7 days) and garbage collection. Graceful offline degradation using cached results when network is unavailable.

### Feature 8: Project-level integration
**Needs:** `needs-design` -- interaction between `.tsuku.toml` version constraints and global auto-update policy needs design
**Dependencies:** Feature 1, Feature 3
**Status:** Not started
**Upstream:** [PRD-auto-update](../prds/PRD-auto-update.md) (R17)

`.tsuku.toml` version constraints take precedence over global auto-update policy. Exact versions in project config (e.g., `node = "20.16.0"`) disable auto-update for that tool in that project context. Prefix versions (e.g., `node = "20"`) allow auto-update within the pin. The project config's `ToolRequirement` struct may need extension.

### Feature 9: Update telemetry
**Dependencies:** Feature 3 (needs update events to track)
**Status:** Not started
**Upstream:** [PRD-auto-update](../prds/PRD-auto-update.md) (R22)

Extend the existing telemetry system (`NewUpdateEvent`) with success/failure/rollback outcomes for auto-updates. Respects the existing opt-out mechanism. Low priority but valuable for understanding update reliability at scale.

## Sequencing rationale

The order is driven by three constraints:

**Technical dependency chain.** Version resolution (Feature 1) must be correct before checks (Feature 2) are meaningful, and checks must produce results before auto-apply (Feature 3) can act on them. This is a hard dependency -- you can't build them in a different order without stub infrastructure.

**Safety pairing.** Auto-apply (Feature 3) and rollback ship together because auto-apply is the default behavior. Shipping auto-apply without rollback means users have no fast recovery path when an upstream release is broken. This is a design decision from the PRD, not a technical dependency.

**Independent tracks.** Self-update (Feature 4) has no code-level dependency on the tool auto-update chain. It can ship before, during, or after Features 1-3. The notification system (Feature 5) depends on having check/apply results to display but is otherwise independent in its design surface. Features 6-9 extend the core system and can be delivered in any order once the foundation is in place.

The split between Phase 1 (Features 1-5) and Phase 2 (Features 6-9) reflects a natural delivery boundary: Phase 1 delivers a complete, safe auto-update experience. Phase 2 adds polish, resilience, and integration. Phase 2 features are independently shippable and can be parallelized.

## Progress

| Feature | Status | Artifact |
|---------|--------|----------|
| 1. Channel-aware version resolution | Not started | Needs design |
| 2. Background update check infrastructure | Not started | Needs design |
| 3. Auto-apply with rollback | Not started | Needs design |
| 4. Self-update | Not started | Needs design |
| 5. Notification system | Not started | Needs design |
| 6. Update polish | Not started | -- |
| 7. Resilience | Not started | -- |
| 8. Project-level integration | Not started | Needs design |
| 9. Update telemetry | Not started | -- |
