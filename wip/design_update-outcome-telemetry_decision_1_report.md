# Decision 1: Update Outcome Event Structure

## Question

How should update outcome events be structured? Covers Go struct design, Analytics Engine blob layout, error classification taxonomy, and schema version.

## Option Analysis

### Option A: Separate UpdateOutcomeEvent struct

A new `UpdateOutcomeEvent` struct with its own `SendUpdateOutcome()` method, `update_outcome_*` action prefix, and dedicated blob layout.

| Criterion | Assessment |
|-----------|------------|
| Pattern alignment | Strong. Every event category in the codebase (LLM, Discovery, VerifySelfRepair, BinaryNameRepair) uses a separate struct, dedicated send method, and action prefix. This is the established convention -- five for five. |
| Extensibility | Good. Dedicated blob layout means all 20 blobs are available for future fields (e.g., duration, retry count, channel). No contention with other event types. |
| Backend complexity | Moderate. Requires a new action prefix check block in `index.ts`, a new validate function, and a new `writeDataPoint` call. But this is copy-paste from existing patterns -- the Discovery and LLM blocks are direct templates. |
| Migration risk | None. New action prefix means old CLIs never emit these events, new CLIs send them to a fresh namespace. No schema version bump needed on existing events. |
| Error classification | Clean fit. Dedicated `error_type` blob with no reuse concerns. |

### Option B: Extend existing Event struct

Add optional `outcome`, `error_type`, and `trigger` fields to the existing `Event` struct. Bump schema version to "2". Reuse `Send()` with new action values.

| Criterion | Assessment |
|-----------|------------|
| Pattern alignment | Weak. No existing event category shares a struct across conceptually different operations. LLM events didn't extend Event; Discovery events didn't extend Event. This would be the first exception. |
| Extensibility | Constrained. The existing Event blob layout uses 13 of 20 blobs. Adding outcome (blob13), error_type (blob14), and trigger (blob15) leaves only 4 blobs for future growth across all event types sharing this struct. |
| Backend complexity | Higher than it appears. The worker's `validateEvent()` switch already handles 5 action types with field-presence rules. Adding "update_success", "update_failure", "update_rollback" means 3 more cases, each with different required/forbidden field combinations. The function becomes harder to maintain. |
| Migration risk | Real. Schema version "2" means the worker must handle both "1" and "2" layouts simultaneously during rollout. Old CLIs send "1", new CLIs send "2". The worker can't distinguish an old "update" event from a new one without checking schema_version first, then interpreting blobs differently. |
| Error classification | Works but awkward. `error_type` would be empty for success events and required for failure events -- yet both share the same struct, so the field is always present in the JSON payload. |

### Option C: New action values on existing struct, repurpose empty blobs

Use action values "auto_update_success"/"auto_update_failure"/"auto_update_rollback" on the existing `Event` struct. Put outcome data in blob9-11 (currently command/flags/template, unused for updates). No schema version change.

| Criterion | Assessment |
|-----------|------------|
| Pattern alignment | Poor. Repurposing blobs to mean different things based on action violates the principle that blob positions have fixed semantics within a layout. The worker queries (e.g., `getStats()`) assume blob1=recipe, blob5=os across all actions. Giving blob9 dual meaning (command for "command" actions, error_type for auto-update actions) creates confusion. |
| Extensibility | Bad. If future event types need blob9-11 for their intended purpose, the dual meaning creates conflicts. |
| Backend complexity | Deceptively low. No new dispatch block needed, but query logic becomes action-dependent. `WHERE blob1 = 'auto_update_failure'` works, but blob9 means "error_type" only for these rows and "command" for others. Anyone writing SQL queries must know this. |
| Migration risk | Low for the CLI, medium for the worker. New action values need to be added to `validActions` and `validateEvent()`. But existing queries that filter on `blob1 IN ('install', 'update', 'remove')` won't accidentally pick up the new events -- which is good for isolation but means auto-update successes won't appear in the existing update stats without query changes. |
| Error classification | Technically works but semantically misleading. The blob named "command" in the layout now holds error types for some events. |

## Recommendation: Option A (Separate UpdateOutcomeEvent struct)

Option A is the clear winner. It follows the pattern used by every other event category in the codebase, avoids migration risk, and provides a clean blob namespace.

The "more backend code" cost is real but small -- it's mechanical copy-paste of the Discovery event dispatch block, adapted for different fields. The codebase already has five examples of this pattern.

### Proposed struct

```go
type UpdateOutcomeEvent struct {
    Action          string `json:"action"`           // "update_outcome_success", "update_outcome_failure", "update_outcome_rollback"
    Recipe          string `json:"recipe"`           // Tool name
    VersionPrevious string `json:"version_previous"` // Version before update attempt
    VersionTarget   string `json:"version_target"`   // Version the update tried to reach
    Trigger         string `json:"trigger"`          // "auto", "manual"
    ErrorType       string `json:"error_type"`       // Error taxonomy value (empty on success)
    OS              string `json:"os"`
    Arch            string `json:"arch"`
    TsukuVersion    string `json:"tsuku_version"`
    SchemaVersion   string `json:"schema_version"`   // "1"
}
```

### Proposed blob layout (10 of 20 blobs)

| Blob | Field | Notes |
|------|-------|-------|
| blob0 | action | `update_outcome_success`, `update_outcome_failure`, `update_outcome_rollback` |
| blob1 | recipe | Tool name, used as index |
| blob2 | version_previous | Version before attempt |
| blob3 | version_target | Target version |
| blob4 | trigger | "auto" or "manual" |
| blob5 | error_type | From taxonomy below; empty on success |
| blob6 | os | Operating system |
| blob7 | arch | CPU architecture |
| blob8 | tsuku_version | CLI version |
| blob9 | schema_version | "1" |

10 blobs used, 10 reserved for future use (duration, channel, retry count, etc.).

## Error Type Taxonomy

Fixed set of values -- no raw error strings cross the wire.

| Value | When used |
|-------|-----------|
| `download_failed` | HTTP error or timeout fetching the artifact |
| `checksum_failed` | SHA256/signature verification mismatch |
| `extraction_failed` | Archive extraction error (tar, zip, gzip) |
| `permission_failed` | chmod or filesystem permission error |
| `symlink_failed` | Failed to create or update symlinks in `$TSUKU_HOME/bin` |
| `verification_failed` | Post-install verification command returned non-zero |
| `state_failed` | Could not read or write `state.json` |
| `version_resolve_failed` | Version provider couldn't resolve target version |
| `unknown` | Catch-all for errors not matching above categories |

The CLI maps `error` values to this taxonomy at emission time, before sending. This keeps the classification logic in Go (where the errors originate) rather than in the worker.

## Confidence

High. The pattern is established by five precedents in the codebase. The blob budget is comfortable. The error taxonomy covers all failure modes visible in the auto-update and rollback code paths.
