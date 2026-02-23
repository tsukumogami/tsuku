# Security Review: DESIGN-pipeline-dashboard-overhaul

## Scope of Review

Reviewed the design document, the existing code it modifies (`internal/batch/orchestrator.go`, `internal/dashboard/dashboard.go`, `internal/dashboard/failures.go`), all 12 pipeline HTML pages, and the `check_breaker.sh` script.

---

## 1. Attack Vectors

### 1.1 Circuit Breaker Probe Selection: Denial of Throughput

**Risk: Low. No new vector introduced.**

The design's pending-first probe selection changes *which* entry is picked for a half-open probe, but the entry still goes through the same `generate()` and `validate()` execution path. The backoff bypass only affects queue scheduling, not execution privileges. An attacker who could manipulate `priority-queue.json` (committed to the repo, requires write access) could already cause arbitrary recipe generation, which is the more fundamental threat surface. The probe selection change doesn't widen that.

The design document correctly notes the trade-off: "The pending-first probe selection might mask ecosystem-level failures by always picking untried entries." This is an operational concern, not a security one. The circuit breaker can't be forced into a permanently open state by the probe selection logic since a successful probe closes it and a failed probe keeps it half-open (allowing retry on next `check_breaker.sh` cycle).

### 1.2 Dashboard JSON Data: Information Disclosure

**Risk: Negligible. Already public data.**

The new `by_ecosystem` field in `dashboard.json` exposes per-ecosystem entry counts. As the design states, this is a derived view of already-public data in `priority-queue.json` (committed to the repo). Package names, ecosystem sources, and queue status are all public. No secrets or user data are involved.

### 1.3 XSS via Dashboard HTML

**Risk: Low. Existing mitigations are consistent.**

All 12 pipeline HTML pages define an `esc()` function that creates a DOM text node and extracts `innerHTML`, providing HTML entity encoding. All user-controlled and data-controlled values pass through `esc()` before insertion into the DOM via `innerHTML`.

**Specific patterns verified:**
- URL parameters (from `URLSearchParams`) that appear in error messages go through `esc()` via `renderError()` in all pages: `failure.html`, `run.html`, `package.html`, `blocked.html`.
- Data from `dashboard.json` (package names, ecosystem names, categories, messages) all go through `esc()`.
- `encodeURIComponent()` is used for URL construction (query params, href attributes).
- `target="_blank"` links include `rel="noopener"`.

**One nuance worth noting**: `failure.html:372` uses `esc(f.workflow_url)` as an `href` attribute value. The `esc()` function HTML-encodes the string, which prevents breaking out of the attribute, but does NOT prevent `javascript:` URLs. However, the `workflow_url` field is:
  1. Currently never populated (it's defined in the struct but never set in `loadFailureDetailsFromFile`)
  2. If it were populated, it would come from JSONL data committed to the repo

Since `workflow_url` is never set today and the data source is the repo itself, this is a theoretical concern. If the design adds population of this field from GitHub Actions (e.g., `$GITHUB_SERVER_URL/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID`), the URL would be safe. **No action needed now**, but worth noting for future changes.

### 1.4 DOM-based XSS via `document.title`

**Risk: None.**

`failure.html:433` sets `document.title = failure.package + ' - Failure Detail - tsuku Pipeline'`. While `failure.package` comes from data, setting `document.title` is a safe sink -- browsers do not parse HTML in the title property. Same pattern in `run.html:438` and `package.html:629`.

### 1.5 Supply Chain / Dependency Injection

**Risk: None introduced.**

The design adds no new dependencies. Dashboard pages use vanilla JS. Go changes add a field to an existing struct. The `esc()` function is hand-rolled but correct for its use case (HTML entity encoding via DOM API).

### 1.6 Command Injection via Orchestrator

**Risk: Pre-existing, unchanged by this design.**

The orchestrator passes `pkg.Name` and `pkg.Source` to `exec.Command()` as separate arguments (not shell-interpreted). The `isValidDependencyName()` function rejects path traversal characters in extracted dependency names. The design's probe selection change doesn't alter the command execution path.

### 1.7 Data Integrity of `batch-control.json`

**Risk: Pre-existing, unchanged by this design.**

`check_breaker.sh` writes to `batch-control.json` using `jq` with `mv` for atomicity. The file is committed to the repo and modified by GitHub Actions. The design doesn't change how this file is read or written. The new probe selection logic only reads the `BreakerState` map that was already populated from this file.

---

## 2. Mitigation Sufficiency

### 2.1 XSS Mitigations

**Sufficient.** The `esc()` function consistently applied across all pages provides HTML encoding. URL parameters are encoded via `encodeURIComponent()`. The data source (static JSON file generated from repo-committed data) limits the attack surface to someone with commit access, at which point they could modify the HTML directly.

### 2.2 Circuit Breaker Mitigations

**Sufficient.** The backoff bypass is scoped to half-open probes only. Normal closed-state selection still enforces backoff. The pending-first preference is a strictly better probe strategy than the current approach (which can deadlock). The batch size limit and per-ecosystem probe limit (1) constrain the scope of the change.

### 2.3 JSON Schema Backward Compatibility

**Sufficient.** The `by_ecosystem` field is additive. Existing dashboard pages that don't consume it will ignore it. No existing fields are renamed or removed.

---

## 3. Residual Risk Assessment

### 3.1 No Escalation Needed

All identified risks are low or negligible. The design operates on public data with no authentication, no server-side execution, and no user data handling. The circuit breaker change is a logic fix to existing functionality with well-bounded effects.

### 3.2 Items to Watch (Not Blocking)

1. **`workflow_url` href sink**: If this field gets populated in the future, validate it starts with `https://github.com/`. Low urgency since the field is currently empty.

2. **ET timezone as implicit disclosure**: The design acknowledges this. Not a meaningful risk since CI scheduling is already visible in public logs.

3. **Probe selection as oracle**: A sophisticated observer could potentially infer ecosystem health state from which packages the batch pipeline attempts (visible via commit history of failed/successful recipes). The pending-first selection makes the ordering more predictable. This is a theoretical information leak about operational state, not about user data. Not actionable.

---

## 4. "Not Applicable" Justification Review

### 4.1 Download Verification: "Not applicable"

**Correctly justified.** The design modifies probe selection (which queue entry) and dashboard display. No download URLs, checksums, or binary verification logic is changed. The orchestrator's `generate()` and `validate()` functions are unchanged.

### 4.2 Execution Isolation: "Not applicable"

**Correctly justified.** The dashboard is a static site with client-side rendering only. The circuit breaker change affects entry *selection*, not execution. The same `exec.Command()` path runs the same `tsuku create` and `tsuku install` commands in the same environment.

### 4.3 Supply Chain Risks: "Not applicable"

**Correctly justified.** Zero new dependencies. Vanilla HTML/CSS/JS. No npm, no build tools, no new Go imports.

---

## 5. Summary

| Finding | Severity | Action |
|---------|----------|--------|
| XSS mitigations (esc function) consistently applied | OK | None |
| `workflow_url` as href without scheme validation | Advisory | Note for future; field is currently empty |
| Circuit breaker probe bypass is well-scoped | OK | None |
| New `by_ecosystem` field is additive only | OK | None |
| All "not applicable" justifications are correct | OK | None |
| No user data, no auth, no server-side execution | OK | None |

**Overall: No blocking security concerns. The design's security section is accurate.**
