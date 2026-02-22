---
status: Proposed
problem: |
  Pipeline failure subcategories are derived by parsing error message text
  with substring matching in the dashboard generator. This is fragile: the
  word "verify" in a suggestion like "Verify the recipe name is correct"
  triggers verify_pattern_mismatch for recipe_not_found errors. The existing
  observability design doc acknowledges this should be replaced with
  structured output from the CLI.
decision: |
  Add a subcategory field to the CLI's --json error output, determined
  alongside the existing category in classifyInstallError(). The orchestrator
  passes it through to FailureRecord, which writes it to JSONL. The
  dashboard prefers the structured field when present and falls back to
  heuristic parsing for older records that lack it.
rationale: |
  The classification logic already exists at the CLI level (exit codes map
  to categories via errors.As()), and extending it to subcategories keeps
  the source of truth where the error context is richest. Adding the field
  at the orchestrator level would mean reimplementing error classification
  outside the CLI. Keeping heuristic parsing indefinitely accepts known
  false positives as permanent. The fallback-with-preference approach
  handles backward compatibility without a migration.
---

# DESIGN: Structured Error Subcategories

## Status

**Proposed**

## Context and Problem Statement

The pipeline dashboard classifies failures into categories (like `recipe_not_found`, `validation_failed`) and subcategories (like `verify_pattern_mismatch`, `no_bottle`). Categories come from structured data: the CLI's `--json` output includes a `category` field, and the orchestrator parses it via `parseInstallJSON()`. Subcategories, by contrast, are extracted from error message text using substring matching in the dashboard generator (`extractSubcategory()` in `internal/dashboard/failures.go`).

This text parsing is fragile. The `recipe_not_found` error for `berkeley-db@5` gets subcategorized as `verify_pattern_mismatch` because its suggestion text contains the word "Verify" ("Verify the recipe name is correct"). The existing observability design doc (DESIGN-dashboard-observability, Decision 2) acknowledges this limitation and notes that "the CLI should add a `subcategory` field to its JSON output so the classification is authoritative rather than heuristic."

The current three-level extraction strategy (bracketed tags, substring matching, exit code fallback) was the right tradeoff when the dashboard shipped: it classified existing data without requiring CLI or orchestrator changes. But as the failure corpus grows and more error messages include incidental trigger words, the false positive rate will increase. The fix for `"verify"` matching suggestion text (tightening to `"failed to verify"`) is a band-aid that doesn't address the root cause.

### Scope

**In scope:**
- Adding a `subcategory` field to the CLI's `--json` error output
- Propagating the field through `FailureRecord` to JSONL files
- Updating the dashboard to prefer the structured field over heuristic parsing
- Backward compatibility with existing JSONL records that lack the field

**Out of scope:**
- Changing the existing category taxonomy (those are correct)
- Modifying the human-readable error output (non-JSON stderr)
- Reworking the exit code system
- Removing heuristic parsing entirely (needed for old records)
- Refactoring error paths that bypass `handleInstallError()` (e.g., `tsuku verify` calls `exitWithCode()` directly and won't produce subcategories in JSON output)

## Decision Drivers

- **Accuracy**: Subcategories should be determined where the error context is richest (at the CLI level), not inferred from text downstream
- **Backward compatibility**: Hundreds of existing JSONL records lack a subcategory field and still need classification
- **Minimal coupling**: Changes should flow naturally through the existing data pipeline without adding new interfaces
- **Incremental adoption**: The system should work correctly with a mix of old (no subcategory) and new (has subcategory) records

## Considered Options

### Decision 1: Where to Classify Subcategories

The subcategory needs to be determined somewhere in the pipeline. The CLI has the richest error context (it knows the exact error type via `errors.As()`), the orchestrator sees the CLI's exit code and JSON output, and the dashboard only sees what's stored in JSONL.

#### Chosen: Add Subcategory to CLI `--json` Output

Extend `classifyInstallError()` in `cmd/tsuku/install.go` to also produce a subcategory string. Add a `Subcategory` field to the `installError` struct so the `--json` output includes it. The orchestrator extracts it via `parseInstallJSON()` and stores it in `FailureRecord`. The dashboard reads it from the JSONL field.

The CLI is where the error type is most precisely known. `classifyInstallError()` already uses `errors.As()` to check error types (e.g., `*registry.RegistryError` with specific error type constants). Subcategory classification at this level can distinguish between "recipe not found" and "version not found" without parsing text.

The batch pipeline's `generate()` path (which runs `tsuku create`, not `tsuku install`) doesn't go through `handleInstallError()`, so it won't get CLI-level subcategories. Records from this path continue using `extractSubcategory()`'s bracketed tag parsing as a fallback, which is already the highest-confidence classification for those errors.

#### Alternatives Considered

**Classify at the orchestrator level**: Have the orchestrator determine subcategories after the CLI exits, using exit codes and output parsing.
Rejected because the orchestrator doesn't have access to the typed error information that the CLI has. It would need to reparse error messages, which is the same fragile approach we're replacing. The orchestrator should pass through what the CLI provides, not reinvent classification.

**Keep heuristic parsing, make patterns more specific**: Continue with text matching but tighten patterns (e.g., `"failed to verify"` instead of `"verify"`).
Rejected because each fix is reactive: we discover a false positive, patch the regex, and wait for the next one. The pattern list will grow and become harder to maintain. This approach also can't classify errors where the message text doesn't contain distinctive keywords.

### Decision 2: How to Handle Backward Compatibility

Existing JSONL records don't have a subcategory field. The dashboard needs to classify both old and new records.

#### Chosen: Prefer Structured Field, Fall Back to Heuristic

When loading failure records, check for the `subcategory` field first. If present and non-empty, use it directly. If absent (old records), run the existing `extractSubcategory()` logic as a fallback.

This means the heuristic code stays but gets exercised less over time as old records age out of the 200-record cap. No migration of existing data is needed.

#### Alternatives Considered

**Backfill old records with correct subcategories**: Write a migration script to add subcategory fields to existing JSONL files.
Rejected because JSONL files are committed to the repo and modified by CI workflows. A bulk migration creates a large diff, risks merge conflicts with in-flight batch runs, and the old records age out naturally within a few weeks anyway.

**Drop heuristic parsing immediately**: Only use the new structured field, leave old records without subcategories.
Rejected because the dashboard would temporarily lose subcategory information for all existing records until they're replaced by new runs. This creates a visible regression in the dashboard's usefulness.

## Decision Outcome

**Chosen: 1A + 2A**

### Summary

Add a `subcategory` field to the CLI's `installError` JSON struct, populated by extending `classifyInstallError()` to return both an exit code and a subcategory string. The subcategory maps error types to values like `not_found`, `version_not_found`, `verify_failed`, `network_timeout`, and `missing_dep`. When no specific subcategory applies, the field is empty.

The orchestrator's `parseInstallJSON()` extracts the new field alongside the existing `category` and `missing_recipes`. `FailureRecord` gains a `Subcategory string` field, and `WriteFailures()` includes it in the JSONL output. The `generate()` path (which runs `tsuku create`, not `tsuku install`) doesn't produce structured subcategories; records from that path continue using the dashboard's heuristic fallback.

The dashboard's `loadFailureDetailsFromFile()` reads the `subcategory` field from JSONL records when present. The `extractSubcategory()` function is still called, but only for records where the field is empty. This means new install-path records get authoritative subcategories from the CLI, generate-path records use the existing bracketed tag parsing, and old records without the field use heuristic matching until they age out.

The subcategory taxonomy stays the same as what `extractSubcategory()` currently produces. The values don't change -- only the source of truth does.

### Rationale

Putting classification at the CLI level works because that's where typed error information is available. The CLI already distinguishes between `ErrTypeNotFound`, `ErrTypeNetwork`, `ErrTypeDNS`, and others via `errors.As()`. Subcategories are a natural extension of this existing mechanism. The orchestrator already parses the CLI's JSON output, so adding a field to the struct requires minimal changes. And the fallback approach means we don't need to coordinate a flag day where everything switches at once.

## Solution Architecture

### Overview

Three components change: the CLI (produces subcategories), the orchestrator (passes them through), and the dashboard (prefers them over heuristics).

### Data Flow

```
CLI: classifyInstallError(err) → (exitCode, subcategory)
  ↓
CLI: installError{Category, Subcategory, Message, ExitCode} → JSON stdout
  ↓
Orchestrator: parseInstallJSON() → extracts category + subcategory + blockedBy
  ↓
Orchestrator: FailureRecord{Category, Subcategory, Message, ...} → JSONL
  ↓
Dashboard: loadFailureDetailsFromFile()
  ↓
  if record.Subcategory != "" → use it
  else → extractSubcategory(category, message, exitCode)  [fallback]
```

### Key Changes

**`cmd/tsuku/install.go`**

```go
type installError struct {
    Status         string   `json:"status"`
    Category       string   `json:"category"`
    Subcategory    string   `json:"subcategory,omitempty"`  // NEW
    Message        string   `json:"message"`
    MissingRecipes []string `json:"missing_recipes"`
    ExitCode       int      `json:"exit_code"`
}
```

`classifyInstallError()` returns a subcategory alongside the exit code. The subcategory is derived from the same `errors.As()` checks already in place. For example, `ErrTypeNotFound` maps to subcategory `"not_found"`, while `ErrTypeNetwork` can be further split into `"timeout"`, `"dns"`, `"tls"`, or `"connection"` based on the error type constant.

**`internal/batch/results.go`**

```go
type FailureRecord struct {
    PackageID   string    `json:"package_id"`
    Category    string    `json:"category"`
    Subcategory string    `json:"subcategory,omitempty"`  // NEW
    BlockedBy   []string  `json:"blocked_by,omitempty"`
    Message     string    `json:"message"`
    Timestamp   time.Time `json:"timestamp"`
}
```

**`internal/batch/orchestrator.go`**

`parseInstallJSON()` extracts the subcategory field from the CLI's JSON output. The `validate()` function stores it in `FailureRecord`. The `generate()` path doesn't change; records from that path have an empty subcategory and rely on the dashboard's heuristic fallback.

**`internal/dashboard/failures.go`**

`loadFailureDetailsFromFile()` reads the `subcategory` field from per-recipe format records. `loadFailureDetailRecords()` only calls `extractSubcategory()` for records where `Subcategory` is empty after loading.

### Subcategory Taxonomy

The CLI's `classifyInstallError()` currently produces exit codes 3, 5, 6, and 8. Subcategories are derived from the typed error information available at classification time:

| Exit Code | Error Type | Subcategory |
|-----------|-----------|-------------|
| 3 | ErrTypeNotFound | `not_found` |
| 5 | ErrTypeTimeout | `timeout` |
| 5 | ErrTypeDNS | `dns_error` |
| 5 | ErrTypeTLS | `tls_error` |
| 5 | ErrTypeConnection | `connection_error` |
| 6 | (general install failure) | `install_failed` |
| 8 | (dependency failed) | `dependency_failed` |

Exit codes 4 (`ExitVersionNotFound`), 7 (`ExitVerifyFailed`), and 9 (`ExitDeterministicFailed`) are defined but not produced by `classifyInstallError()`. They're used by other commands (`verify`, `create`) that have their own error paths. If those commands gain `--json` output in the future, the taxonomy can be extended then.

Values from the `generate()` path (via heuristic fallback on bracketed tags): `no_bottles`, `api_error`, `complex_archive`, `no_binaries`.

## Implementation Approach

### Phase 1: CLI Subcategory Output

Modify `classifyInstallError()` to return `(exitCode int, subcategory string)`. Update `handleInstallError()` to include the subcategory in the JSON output. Add tests for the new subcategory values.

Files: `cmd/tsuku/install.go`, `cmd/tsuku/exitcodes.go` (if needed for constants)

### Phase 2: Orchestrator Passthrough

Add `Subcategory` field to `FailureRecord`. Update `parseInstallJSON()` and `installResult` to extract the field. Update `validate()` to store it. Update `data/schemas/failure-record.schema.json` to allow the new field (the schema uses `additionalProperties: false`).

Files: `internal/batch/results.go`, `internal/batch/orchestrator.go`, `internal/batch/orchestrator_test.go`, `data/schemas/failure-record.schema.json`

### Phase 3: Dashboard Preference

Update `loadFailureDetailsFromFile()` to read the `subcategory` field from JSONL records. Change `loadFailureDetailRecords()` to only call `extractSubcategory()` when the loaded subcategory is empty. Also update dashboard deserialization structs (`FailureRecord` in `dashboard.go`) to include the field so `json.Unmarshal` doesn't silently drop it. Update tests.

Files: `internal/dashboard/failures.go`, `internal/dashboard/failures_test.go`, `internal/dashboard/dashboard.go`

## Security Considerations

### Download Verification

Not applicable. This change modifies error classification metadata, not the download or verification pipeline. No artifacts are downloaded or verified differently.

### Execution Isolation

Not applicable. The subcategory field is a classification label added to JSON output. It doesn't change what the CLI executes or what permissions it requires.

### Supply Chain Risks

Subcategory values flow into JSONL records consumed by CI pipeline scripts (batch workflows, requeue scripts). The values must remain a closed enumeration of hardcoded strings derived from typed error checks. Deriving subcategories from error message text or user-controlled input would create an injection vector. This design moves in the right direction: CLI-level classification from typed errors is inherently a closed set, unlike the heuristic parsing it replaces (which operates on arbitrary message text that could contain crafted content from upstream APIs).

### User Data Exposure

The `--json` output gains a `subcategory` field. This contains the same kind of information already present in `category` and `exit_code`: a classification label derived from the error type. No new user data, file paths, or sensitive information is exposed. The `message` field (which can contain paths) is unchanged.

## Consequences

### Positive

- Subcategory classification becomes authoritative rather than heuristic
- False positives from text matching (like "Verify" in suggestion text) stop occurring for new records
- The dashboard shows consistent subcategories regardless of how error messages are worded
- Existing records continue to work via the fallback path

### Negative

- Two classification paths coexist until old records age out (structured field vs heuristic parsing)
- The `subcategory` field in `--json` output becomes a machine-readable API contract; changing values later would be a breaking change for consumers that parse it

### Mitigations

- The heuristic fallback is already tested and working. It doesn't need changes; it just gets called less often as new records replace old ones.
- The `subcategory` field uses `omitempty`, so consumers that don't expect it are unaffected. The field is additive, not a breaking change.
