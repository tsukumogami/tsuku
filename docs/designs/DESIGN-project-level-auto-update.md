---
status: Proposed
problem: |
  The auto-update system ignores .tsuku.toml project-level version constraints.
  MaybeAutoApply operates globally on cached check entries with no project
  awareness. A team pinning node = "20.16.0" in their project config expects
  auto-update to leave node alone, but today it updates regardless.
decision: |
  TBD
rationale: |
  TBD
upstream: docs/prds/PRD-auto-update.md
---

# DESIGN: Project-Level Auto-Update Integration

## Status

Proposed

## Context and Problem Statement

`.tsuku.toml` declares per-project tool version constraints, but the auto-update
system ignores them entirely. `MaybeAutoApply` in `internal/updates/apply.go`
operates globally on cached update check entries with no concept of which project
the user is working in.

This creates a conflict: a team pins `node = "20.16.0"` in their project config
expecting deterministic builds, but auto-update could install a different version
because the global pin (from `state.json`'s `Requested` field) may be broader
(e.g., `"20"` allowing 20.x.y).

PRD requirement R17 is clear: `.tsuku.toml` version constraints take precedence
over global auto-update policy. Exact versions disable auto-update for that tool
in that project context. Prefix versions allow auto-update within the pin.

The core challenge is that auto-apply runs in `PersistentPreRun` before any
command executes, using CWD to detect the project. But CWD can change between
sessions, and a single tsuku installation serves all projects. The design must
reconcile per-project constraints with a global tool installation model.

### Scope

**In scope:**
- How `.tsuku.toml` constraints suppress or allow auto-update per tool
- Where project config is injected into the auto-apply decision
- Pin semantics: exact versions disable, prefix versions allow within pin
- CWD-based project detection and its edge cases
- Any ToolRequirement struct extensions

**Out of scope:**
- New `.tsuku.toml` syntax beyond what's needed for auto-update
- Per-tool auto-update config in `.tsuku.toml` (belongs in `config.toml`)
- Organization-level policy files
- Changes to the update check infrastructure

## Decision Drivers

- `.tsuku.toml` takes precedence over global config (PRD R17)
- Must work with existing `PinLevelFromRequested` / `VersionMatchesPin` semantics
- Zero added latency (R19) -- project config loading must be fast
- MaybeAutoApply runs in PersistentPreRun -- CWD is available but may differ from install time
- Atomic operations (R21) -- no partial state from project config interactions
- Simple mental model: users should predict what auto-update will do
