---
status: Proposed
upstream: docs/prds/PRD-auto-update.md
spawned_from:
  issue: 2184
  repo: tsukumogami/tsuku
problem: |
  Feature 2 produces per-tool cache files showing available updates, but nothing
  acts on them. Users still need to run tsuku update manually for each tool. When
  auto-apply is added, a failed update could leave a tool broken with no fast
  recovery path. Rollback must ship alongside auto-apply since auto-apply is the
  default behavior (PRD D6).
decision: |
  TBD -- pending decision execution.
rationale: |
  TBD -- pending decision execution.
---

# DESIGN: Auto-apply with rollback

## Status

Proposed

## Context and Problem Statement

Feature 2 (background update checks) writes per-tool cache files to `$TSUKU_HOME/cache/updates/<toolname>.json` showing what's available within pin boundaries. But nothing reads those results and acts on them. Users must still manually run `tsuku update <tool>` for each tool.

The auto-update system's core value proposition is that tools stay current without manual intervention. This feature closes the loop: during any tsuku command, if cached check results show a newer version within pin boundaries, tsuku downloads and installs it automatically. The apply phase runs during tsuku commands only (not shell hooks or shim invocations) to avoid adding latency to tool execution or prompt rendering (PRD R3).

Two companion features must ship together:

1. **Auto-rollback on failure** (R10): If an auto-update fails at any point (download, extraction, verification, symlink creation), the previous version remains active with no user intervention.

2. **Manual rollback** (R9): `tsuku rollback <tool>` switches to the immediately preceding version for runtime breakage (tool installs fine but crashes when used). Rollback is one level deep and temporary -- it doesn't change the `Requested` field, so auto-update may re-apply on the next cycle (PRD D7).

3. **Basic failure notices** (R11a): Failed auto-updates write a notice to `$TSUKU_HOME/notices/`. `tsuku notices` displays details. Consecutive-failure suppression (R11) ships in Feature 7.

## Decision Drivers

- **Safety as the default**: Auto-apply is the default behavior (PRD D1). A failed update must never leave the user without a working tool. Auto-rollback is mandatory.
- **Existing install infrastructure**: The install flow (`runInstallWithTelemetry`) already handles downloading, extracting, verifying, and symlinking. Auto-apply should reuse this, not reinvent it.
- **Multi-version directory model**: Tsuku installs each version to `$TSUKU_HOME/tools/<name>-<version>/`. Old version directories remain on disk. This makes rollback cheap (symlink switch, no re-download).
- **Concurrency safety**: Auto-apply mutates state.json and tool directories. It must not run concurrently with other state-mutating commands (install, update, remove).
- **Feature 2 cache as input**: The `UpdateCheckEntry.LatestWithinPin` field from per-tool cache files is the signal for what to install. If the field is empty or matches the active version, no update is needed.
- **Downstream consumers**: Feature 5 (notifications) needs to know what was applied or failed. Feature 7 (resilience) adds consecutive-failure tracking on top of the basic notice system.
