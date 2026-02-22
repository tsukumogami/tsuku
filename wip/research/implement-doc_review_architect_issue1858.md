# Architect Review: Issue #1858

## Issue

fix(ci): align batch workflow category names with canonical taxonomy

## Review Focus

Architecture (design patterns, separation of concerns)

## Diff Scope

Single file changed: `.github/workflows/batch-generate.yml`, line 893.

Old jq mapping:
```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 then "network" elif .exit_code == 124 or .exit_code == 137 then "timeout" else "deterministic" end)
```

New jq mapping:
```jq
category: (if .exit_code == 8 then "missing_dep" elif .exit_code == 5 or .exit_code == 124 or .exit_code == 137 then "network_error" else "generation_failed" end)
```

Design doc status markers (Mermaid diagram) updated from `blocked` to `ready` for #1858 and #1859.

## Architectural Assessment

### 1. Alignment with canonical taxonomy (no finding)

The CI workflow's jq now produces category strings that match the orchestrator's `categoryFromExitCode()` in `internal/batch/orchestrator.go:491-508`. Verified:

| Exit code | Orchestrator (Go) | CI workflow (jq) | Match? |
|-----------|-------------------|------------------|--------|
| 3 | `recipe_not_found` | (not produced by jq -- generate path doesn't encounter this) | N/A |
| 5 | `network_error` | `network_error` | Yes |
| 6 | `install_failed` | `generation_failed` (falls through to else) | See note |
| 7 | `verify_failed` | `generation_failed` (falls through to else) | See note |
| 8 | `missing_dep` | `missing_dep` | Yes |
| 9 | `generation_failed` | `generation_failed` | Yes |
| 124/137 | `generation_failed` (falls through to default) | `network_error` | Divergence |
| default | `generation_failed` | `generation_failed` | Yes |

**Exit codes 124 and 137 (timeout signals).** The jq maps these to `network_error`. The orchestrator maps them to `generation_failed` via the default case. This is an intentional divergence documented in the design: the CI workflow uses shell-level timeout exit codes (124 from `timeout`, 137 from SIGKILL) that the orchestrator doesn't encounter because it uses Go's `exec.CommandContext` which returns exit code 5 for timeouts. The jq handles a different execution environment. This is architecturally sound -- the same semantic event (network timeout) gets the same category regardless of which code path encounters it.

**Exit codes 6 and 7.** The jq's else branch maps these to `generation_failed`, while the orchestrator distinguishes `install_failed` (6) and `verify_failed` (7). This is fine for the generate path -- the generate workflow doesn't run install or verify steps, so exit codes 6 and 7 would only appear from unexpected failures, where `generation_failed` is the correct catch-all.

### 2. No parallel pattern introduced (no finding)

The design doc establishes the orchestrator as the authority for pipeline categories (Decision 1). The CI jq is the third and final producer being aligned. After this change, all three producers (orchestrator `categoryFromExitCode()`, CLI-to-orchestrator translation in `parseInstallJSON()`, and CI workflow jq) produce consistent category strings. The jq is not introducing a new classification path -- it's aligning an existing one.

### 3. No state contract impact (no finding)

The change modifies category *values* written to JSONL, not the schema. The `category` field already exists in `data/schemas/failure-record.schema.json` as a string without enum constraints, so new string values don't require schema changes. The `subcategory` field was added in #1857; this issue doesn't touch it.

### 4. No residual old category names (no finding)

Searched `.github/workflows/` and `scripts/` for old category strings (`"deterministic"`, `"timeout"`, `"network"` as standalone categories). No matches. The only "deterministic" references in scripts are about the `--deterministic-only` mode flag, which is unrelated.

### 5. Dashboard backward compatibility (out of scope, noted)

The dashboard (#1859, not yet implemented) will need a category remap for old JSONL records that still contain the pre-canonical names. This is the design's intended sequencing -- #1858 and #1859 are independent siblings both depending on #1857. The CI workflow can ship first without dashboard changes.

## Findings

### Blocking

None.

### Advisory

None.

## Overall Assessment

The change is structurally clean. It completes the third leg of the category alignment described in the design doc (Phase 3). The jq expression produces category strings consistent with the orchestrator's `categoryFromExitCode()` for all exit codes relevant to the generate path. The intentional divergence on exit codes 124/137 (shell timeout signals vs Go timeout handling) is architecturally appropriate -- different execution environments mapping the same semantic event to the same category. No new patterns, no contract violations, no dependency concerns.
