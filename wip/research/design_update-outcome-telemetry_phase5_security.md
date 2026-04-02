# Security Review: update-outcome-telemetry

## Dimension Analysis

### External Artifact Handling

**Applies:** No

This design does not download, execute, or process external artifacts. The telemetry feature only *observes* the outcomes of update operations that are already handled by existing code paths (the auto-apply loop, manual update, and rollback commands). The new code constructs event structs from in-process data and sends them outbound. The `/stats/updates` endpoint returns aggregate JSON computed from Analytics Engine queries -- no external input is parsed or executed on the worker side beyond what the existing `POST /event` handler already does.

The `POST /event` path already exists and handles untrusted input. The new `update_outcome_*` events flow through the same ingestion path with the same validation and rate limiting. No new parsing surface is introduced for external data.

### Permission Scope

**Applies:** Yes (minimal, no change from baseline)

**Filesystem:** The design reads no new files. The `classifyError()` function inspects Go error objects already in memory. The telemetry client reads `config.toml` for the opt-out check, but this is existing behavior -- `NewClient()` already does this for every event category.

**Network:** One outbound HTTP POST per update outcome event, to `https://telemetry.tsuku.dev/event`. This matches the existing pattern exactly. The 2-second timeout and fire-and-forget goroutine are reused from `sendJSON()`. No new endpoints are contacted.

**Process:** No new processes are spawned. No shell commands are executed. The goroutine model is identical to all five existing send methods.

**Risk assessment:** Low. The permission footprint is identical to what every existing telemetry event already uses. No privilege escalation, no new file access, no new network destinations.

### Supply Chain or Dependency Trust

**Applies:** No

The design adds no new Go modules, npm packages, or external dependencies. On the CLI side, `UpdateOutcomeEvent` uses only `runtime` and `buildinfo` (both already imported). On the worker side, the new dispatch branch and stats endpoint use only Cloudflare Workers APIs (`AnalyticsEngineDataset`, `D1Database`) that are already in the dependency graph. The dashboard changes are vanilla HTML/JS with no new libraries.

The `classifyError()` function maps Go error types to a fixed string enum entirely within the `telemetry` package. No external classifier, error-parsing library, or regex engine is introduced.

### Data Exposure

**Applies:** Yes (the primary security-relevant dimension)

**What is transmitted:**

| Field | Content | Sensitivity |
|-------|---------|-------------|
| action | One of 3 fixed strings | None |
| recipe | Tool name (e.g., "terraform") | None -- public registry data |
| version_previous | Semver string | None |
| version_target | Semver string | None |
| trigger | "auto" or "manual" | None |
| error_type | One of 9 taxonomy values or empty | None |
| os | "linux" or "darwin" | Low -- broad category |
| arch | "amd64" or "arm64" | Low -- broad category |
| tsuku_version | CLI version string | None |
| schema_version | "1" | None |

**What is NOT transmitted:**

- Raw error messages (which may contain filesystem paths, usernames, or temp directory names)
- Machine identifiers, IP addresses (beyond what Cloudflare sees at the network layer)
- Tool installation paths
- Configuration file contents
- Any field that could identify a specific user or machine

**Risk 1: Error string leakage.** The `classifyError()` function is the critical privacy boundary. If a developer adds a new error type and maps it incorrectly, or if the catch-all `unknown` case accidentally passes through a wrapped error string instead of the literal `"unknown"`, raw errors could leak.

*Severity:* Medium. Filesystem paths in error messages could reveal usernames (e.g., `/home/jdoe/.tsuku/`).

*Mitigation:* The design already addresses this by classifying errors in Go before transmission. Additional safeguards:
- Unit tests for `classifyError()` should verify that every code path returns exactly one of the 9 taxonomy values and that the function signature returns `string` (not `error`).
- A test should pass an error wrapping a filesystem path and confirm the output is a taxonomy value, not the raw message.
- Consider making the taxonomy values a Go `const` block with a validation function, so the compiler or linter catches typos.

**Risk 2: Recipe name as fingerprint.** If a user installs an unusual combination of tools and their update patterns are distinctive, the recipe + os + arch + version combination could theoretically narrow down identity across events. However, events have no session ID, no machine ID, and no timestamp correlation beyond what Analytics Engine assigns internally. The `/stats/updates` endpoint returns only aggregate counts, not individual events. The risk of deanonymization from aggregate counts is negligible.

*Severity:* Low.

*Mitigation:* Already handled. The stats endpoint returns only sums and distributions, never individual data points.

**Risk 3: Worker-side blob injection.** A malicious client could POST crafted events with unexpected field values (e.g., an `error_type` containing HTML for XSS on the dashboard, or extremely long strings to abuse storage).

*Severity:* Low. The design specifies server-side validation: action must be one of three values, trigger must be "auto" or "manual", error_type must be from the taxonomy or empty. Invalid events return 400.

*Mitigation:* Already addressed in the design. Implementation should additionally:
- Enforce maximum field lengths (recipe name, version strings) to prevent storage abuse.
- Ensure the dashboard HTML-escapes all values rendered from the stats API (standard XSS prevention).

**Risk 4: Opt-out bypass.** If `SendUpdateOutcome()` doesn't check the `disabled` flag, events could be sent despite user opt-out.

*Severity:* Medium (trust violation).

*Mitigation:* The design states it uses the same `disabled` check. Looking at the codebase, every `Send*` method in `client.go` (lines 98-190) checks `c.disabled` as its first operation. A unit test should confirm `SendUpdateOutcome()` follows this pattern. This is low-risk given the consistent codebase pattern, but worth a test.

## Recommended Outcome

**OPTION 1: No blocking security concerns.**

The design introduces no new attack surface, no new permissions, no new dependencies, and no new data categories beyond what the existing telemetry system already handles. The error classification boundary (`classifyError()`) is the one area requiring careful implementation and testing, but the design already identifies this and specifies the right approach (fixed taxonomy, classification at emission time, raw strings never transmitted).

## Summary

This design has no blocking security issues. It follows established telemetry patterns with identical permission scope and opt-out mechanisms. The primary security-relevant aspect is data exposure, where the critical control is `classifyError()` -- the function that maps Go errors to a fixed 9-value taxonomy before any data leaves the machine. The design handles this correctly by specifying client-side classification with raw error strings never transmitted. Implementation should include unit tests verifying that `classifyError()` always returns a taxonomy value (never a raw error string) and that `SendUpdateOutcome()` respects the disabled flag.
