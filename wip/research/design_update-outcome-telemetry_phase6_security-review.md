# Phase 6 Security Review: Update Outcome Telemetry

## Methodology

Independent review of DESIGN-update-outcome-telemetry.md and the Phase 5 security findings, cross-referenced against the actual codebase: `internal/telemetry/client.go`, `internal/telemetry/event.go`, `internal/registry/errors.go`, `internal/updates/apply.go`, and `telemetry/src/index.ts`.

---

## Q1: Attack Vectors Not Considered

### 1.1 No Request Body Size Limit on POST /event (NEW)

The worker's `POST /event` handler calls `await request.json()` with no body size check (line 590 of `index.ts`). A malicious client can POST arbitrarily large JSON payloads. While Cloudflare Workers have a platform-level 100MB request body limit, processing multi-megabyte JSON payloads burns CPU time against the 10ms/50ms CPU time limit, and repeated requests could cause throttling or increased costs.

This is not specific to update outcome events -- it affects all existing event types equally. But since this design adds another dispatch branch, it's worth noting as pre-existing technical debt.

**Recommendation:** Add a `Content-Length` check (e.g., reject > 4KB) before `request.json()` in the shared ingestion path. This protects all event types, not just update outcomes.

### 1.2 No Rate Limiting on Event Ingestion (PRE-EXISTING)

The `POST /event` endpoint has no rate limiting. A malicious actor could flood the Analytics Engine dataset with fabricated update outcome events, skewing dashboard metrics. This isn't new -- it affects all event types -- but update outcome events have higher signal value (success rate metrics drive operational decisions), making poisoned data more consequential.

The Phase 5 review states the design "reuses existing rate limiting" but there is no rate limiting in the worker code. The design doc also claims it, but the claim is incorrect.

**Recommendation:** Document this as a known limitation. If update reliability metrics will drive operational decisions (e.g., blocking a recipe version), rate limiting becomes a prerequisite. For dashboard-only use, the risk is acceptable with a note that metrics are best-effort.

### 1.3 Timing Side Channel in Auto-Apply Telemetry (LOW)

Auto-apply fires events in a tight loop over pending entries (apply.go lines 76-108). If an observer can monitor outbound requests from a machine (e.g., network-level adversary), the burst pattern of `update_outcome_*` events reveals the number of tools with pending auto-updates and their success/failure pattern. Combined with the fire-and-forget 2-second timeout, timing between requests leaks the number of tools being updated.

**Severity:** Very low. Requires network-level adversary, and the information gained (tool update count) has minimal value.

**Recommendation:** No action needed. Documenting for completeness.

### 1.4 Analytics Engine Query Injection via Stats Endpoint (LOW)

The `/stats/updates` endpoint will construct Analytics Engine SQL queries. If recipe names or other blob values are interpolated into query strings without parameterization, SQL injection in the Analytics Engine API is possible. The existing `/stats/discovery` endpoint uses string interpolation for the `WHERE blob1 LIKE 'discovery_%'` clause (line 501), but these are hardcoded constants, not user input. The stats query for update outcomes should follow the same pattern (filtering by the `update_outcome_*` action prefix, which is a constant).

**Recommendation:** Ensure all Analytics Engine queries in the new `/stats/updates` handler use only hardcoded filter values, never interpolated blob content. The existing codebase does this correctly; maintain the pattern.

---

## Q2: Are Mitigations Sufficient for Identified Risks?

### 2.1 classifyError() Privacy Boundary -- SUFFICIENT with caveats

The design correctly identifies `classifyError()` as the critical privacy boundary. The mitigation (fixed taxonomy, classification at emission time, unit tests) is sound.

**Caveat:** The existing `classifyError()` in `internal/registry/errors.go` classifies *network* errors for the registry client. The proposed `classifyError()` in `internal/telemetry/event.go` must classify *installation* errors -- a different error domain (extraction failures, permission errors, symlink errors, verification failures). These error types come from different packages (`internal/install`, `internal/builders`, etc.) and may use different error wrapping patterns.

The design should specify how the telemetry `classifyError()` maps errors from installation packages. Pattern-matching on error strings is fragile and risks leaking new error messages if upstream packages change their error text. Prefer `errors.Is()` / `errors.As()` with sentinel errors or typed errors from the install packages, falling back to `"unknown"` for anything unrecognized.

**Recommendation:** The `classifyError()` implementation should have a test that creates an error wrapping a filesystem path (e.g., `fmt.Errorf("failed to extract to /home/jdoe/.tsuku/tools/foo: %w", someErr)`) and verifies the output is a taxonomy string, not the path-containing message. This is already suggested in Phase 5 -- confirming it's necessary.

### 2.2 Opt-Out -- SUFFICIENT

Every `Send*` method in `client.go` checks `c.disabled` as its first operation. The pattern is mechanical and consistent across all five existing methods. Adding `SendUpdateOutcome()` with the same guard is low-risk. A unit test confirming the guard exists is warranted but the risk of regression is minimal given the established pattern.

### 2.3 Worker-Side Validation -- SUFFICIENT if implemented as specified

The design specifies strict validation: action must be one of three values, trigger must be "auto"/"manual", error_type must be from the taxonomy or empty. This matches the validation rigor of existing event types. Max field lengths for recipe/version are specified but need concrete values.

**Recommendation:** Use the existing `MAX_TOOL_NAME_LENGTH = 128` and `TOOL_NAME_PATTERN` for the recipe field (consistent with discovery event validation). Add `MAX_VERSION_LENGTH = 64` for version strings. These limits already exist as informal conventions in the codebase; formalizing them for update outcome events is straightforward.

### 2.4 Dashboard XSS -- SUFFICIENT if implemented as specified

The design specifies HTML-escaping all values from the stats API. The existing dashboard uses `textContent` assignment (not `innerHTML`) for most values, which provides inherent XSS protection. The new dashboard section should follow this pattern.

---

## Q3: "Not Applicable" Justifications

### 3.1 External Artifact Handling: N/A -- CORRECT

The design genuinely does not parse external artifacts. Events are constructed from in-process Go data and sent outbound. The `/stats/updates` endpoint returns computed aggregates. No external input is parsed beyond what the existing `POST /event` handler already handles.

### 3.2 Supply Chain: N/A -- CORRECT

No new dependencies on either side. Verified: the Go struct uses only `runtime` and `buildinfo` (already imported). The worker changes are pure TypeScript using existing Cloudflare Workers APIs.

---

## Q4: Residual Risk for Escalation

### No blocking residual risk.

The most consequential residual risk is the **absence of rate limiting on event ingestion**, which is pre-existing and affects all event types. If update reliability metrics will be used for automated decisions (e.g., auto-disabling a recipe version that shows high failure rates), this becomes a material risk since an attacker could inject false failure events to trigger the disable. However, the design explicitly puts feedback loops out of scope ("Changing auto-apply behavior based on telemetry" is listed as out of scope), so this risk is bounded to dashboard accuracy.

**Summary of residual risks (none blocking):**

| Risk | Severity | Status |
|------|----------|--------|
| No rate limiting on POST /event (pre-existing) | Low for dashboard-only use | Accept, document |
| No request body size check (pre-existing) | Low (Cloudflare platform limits apply) | Nice-to-have fix |
| classifyError() implementation correctness | Medium if done wrong | Mitigated by specified tests |
| Metric poisoning via fabricated events | Low for dashboard-only use | Accept; revisit if metrics drive automation |

---

## Recommendations Summary

1. **Implement `classifyError()` using typed errors / `errors.Is()` / `errors.As()`**, not string matching. Include a test with path-containing error messages.
2. **Add concrete max length constants** for recipe name and version fields in worker validation (`MAX_TOOL_NAME_LENGTH = 128`, `MAX_VERSION_LENGTH = 64`).
3. **Consider adding a Content-Length check** (4KB max) to the shared `POST /event` handler as a defense-in-depth measure. This is a pre-existing gap, not specific to this design.
4. **Document the absence of rate limiting** as a known limitation of the telemetry system. Flag it as a prerequisite before any future work that uses telemetry metrics for automated decisions.
5. **Use `textContent` (not `innerHTML`)** for all dashboard rendering of stats values, consistent with existing dashboard patterns.
