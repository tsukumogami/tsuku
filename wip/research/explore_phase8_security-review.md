# Security Review: Ecosystem Probe Design

**Date**: 2026-02-01
**Reviewer**: Security Analysis Agent
**Documents**: DESIGN-ecosystem-probe.md, DESIGN-discovery-resolver.md
**Scope**: Ecosystem probe stage of discovery resolver chain

## Executive Summary

This review identifies **7 critical attack vectors** not adequately addressed in the current design documents, including DNS rebinding, SSRF via registry APIs, response injection attacks, and timing-based reconnaissance. While the design includes basic filtering and validation, the security analysis section significantly underestimates risks specific to parallel HTTP queries against untrusted endpoints.

**Risk Level**: MEDIUM-HIGH (residual risk requires escalation)

**Recommendation**: Add security controls before implementation, particularly around HTTP client configuration, redirect handling, and response validation.

## Attack Vectors Analysis

### 1. DNS Rebinding Attacks ⚠️ CRITICAL GAP

**Threat**: A malicious package registry could use DNS rebinding to make tsuku query internal network resources.

**Scenario**:
1. Attacker registers a package on a legitimate registry (npm, PyPI, crates.io)
2. Package metadata points to attacker-controlled repository URL
3. When ecosystem probe queries registry API, DNS resolution for the registry is manipulated
4. First DNS query resolves to legitimate registry IP
5. Second query (for Cargo.toml, pyproject.toml, etc.) resolves to internal IP (e.g., 169.254.169.254, 10.0.0.1)
6. tsuku queries internal metadata services, cloud instance metadata, or internal admin panels

**Current Mitigation**: NONE identified in design

**Design Impact**:
- Cargo builder fetches Cargo.toml from `raw.githubusercontent.com` (lines 282-284 of cargo.go)
- PyPI builder fetches pyproject.toml from `raw.githubusercontent.com` (lines 302 of pypi.go)
- npm builder parses repository URLs from package metadata (lines 274-303 of npm.go)

All three builders follow repository URLs from untrusted registry data without validating target IP addresses.

**Recommended Mitigations**:
1. **IP address validation**: Before making HTTP requests, resolve hostnames and reject private IP ranges (RFC 1918, RFC 4193, link-local)
2. **Allowlist hostnames**: Only permit requests to known-safe domains (`raw.githubusercontent.com`, `gitlab.com`, etc.)
3. **Separate HTTP client for metadata fetches**: Use a different client with stricter settings for repository metadata fetching
4. **DNS pinning**: Cache DNS resolutions and revalidate before each request

**Residual Risk**: HIGH - DNS rebinding can bypass hostname allowlists if DNS TTL is short enough to change between validation and request.

---

### 2. SSRF via Malicious Registry Responses ⚠️ CRITICAL GAP

**Threat**: Package registry API responses could contain URLs pointing to internal services, causing tsuku to make requests to attacker-chosen endpoints.

**Scenario**:
1. Attacker uploads package to npm with `repository.url = "http://169.254.169.254/latest/meta-data/iam/security-credentials/"`
2. Ecosystem probe queries npm API (safe)
3. npm builder parses repository URL from response (npm.go line 134, 275-291)
4. Builder attempts to fetch `package.json` or metadata from the URL
5. tsuku queries cloud instance metadata service, exposing credentials

**Current Mitigation**: None. URL validation in `cleanRepositoryURL` (npm.go:295-303) only handles protocol prefixes and `.git` suffix. No IP validation.

**Attack Vectors in Current Code**:
- `CargoBuilder.buildCargoTomlURL()` (cargo.go:256) constructs URLs from `repository` field without validation
- `PyPIBuilder.buildPyprojectURL()` (pypi.go:273) constructs URLs from `ProjectURLs` without validation
- `NpmBuilder.extractRepositoryURL()` (npm.go:274) uses repository data directly

**Recommended Mitigations**:
1. **URL parsing and IP validation**: Parse all URLs from registry responses and reject private IP addresses
2. **Hostname allowlist**: Only permit fetches from known code hosting platforms
3. **Protocol allowlist**: Reject non-HTTPS URLs (except localhost in tests)
4. **Path validation**: Reject suspicious paths (`/admin`, `/api/v1/token`, etc.)

**Residual Risk**: MEDIUM - Allowlists may not cover all legitimate code hosting, but restricting to major platforms (GitHub, GitLab, Bitbucket, SourceHut) covers 99%+ of real packages.

---

### 3. HTTP Redirect-Based SSRF ⚠️ CRITICAL GAP

**Threat**: HTTP redirects from registry APIs could point to internal services, bypassing initial URL validation.

**Scenario**:
1. Attacker registers package with repository URL pointing to attacker-controlled server
2. tsuku validates URL (appears to be `https://attacker.com/repo.git`)
3. HTTP client follows redirect to `http://localhost:6379/CONFIG SET dir /var/spool/cron`
4. Redis commands executed, or other internal services probed

**Current Mitigation**: Default Go HTTP client follows up to 10 redirects automatically. No redirect policy documented in builders.

**Recommended Mitigations**:
1. **Disable redirects entirely** for ecosystem metadata fetches:
   ```go
   client := &http.Client{
       CheckRedirect: func(req *http.Request, via []*http.Request) error {
           return http.ErrUseLastResponse
       },
   }
   ```
2. **Validate redirect targets**: If redirects must be followed, validate each redirect URL against allowlists
3. **Limit redirect chains**: Maximum 2 redirects for legitimate CDN/hosting changes

**Residual Risk**: LOW - Disabling redirects is safe for registry APIs, which should return direct responses.

---

### 4. Response Injection / Content-Type Confusion ⚠️ MEDIUM

**Threat**: Registry API returns malicious content-type header or response body crafted to exploit JSON parser vulnerabilities.

**Scenario**:
1. Attacker compromises registry API or performs MITM attack
2. API returns `Content-Type: text/html` with JSON body containing `<script>` tags
3. JSON parser processes body, potentially triggering parser bugs
4. OR: Response contains billion-laughs XML entity expansion if TOML parser has vulnerabilities

**Current Mitigation**:
- Partial: Content-type validation exists (cargo.go:206, npm.go:204, pypi.go:214)
- Gap: Validation only checks prefix (`strings.HasPrefix`), doesn't reject charset attacks
- Gap: TOML parsing (Cargo.toml, pyproject.toml) has no content-type validation

**Attack Surface**:
- `json.NewDecoder()` used throughout builders - generally safe but relies on io.LimitReader
- `toml.Unmarshal()` used for Cargo.toml and pyproject.toml parsing - potential TOML bomb attacks

**Recommended Mitigations**:
1. **Strict content-type validation**: Reject responses with unexpected `Content-Type` or charset parameters
2. **TOML bomb protection**: Validate TOML structure before parsing (max nesting depth, key count)
3. **Response body validation**: Check for HTML/XML markers before parsing as JSON/TOML
4. **Parser sandboxing**: Consider using restricted parsing context or timeout per parse operation

**Residual Risk**: LOW - Modern JSON/TOML parsers are well-tested, but new vulnerabilities appear periodically.

---

### 5. Timing-Based Reconnaissance ⚠️ LOW-MEDIUM

**Threat**: Malicious package in ecosystem could use response times to probe internal network topology.

**Scenario**:
1. Attacker registers 1000 packages with repository URLs pointing to `10.0.0.1`, `10.0.0.2`, ... `10.0.3.255`
2. User runs `tsuku install attacker-probe-1`
3. Ecosystem probe queries package, builder attempts to fetch Cargo.toml from `10.0.0.1`
4. HTTP timeout reveals whether host exists (2-3s timeout vs instant connection refused)
5. Attacker observes which packages get installed or error messages to map internal network

**Current Mitigation**: None. Timeout values are per-builder HTTP client (60s default) with no consideration of timing side-channels.

**Recommended Mitigations**:
1. **Constant-time failures**: All network errors return after same timeout (don't distinguish connection refused from timeout)
2. **Rate limiting**: Limit ecosystem probe queries per minute to prevent rapid scanning
3. **Error message sanitization**: Don't expose network-level errors (connection refused, no route to host) to users

**Residual Risk**: MEDIUM - Difficult to fully prevent timing attacks, but rate limiting reduces practical impact.

---

### 6. Malicious Registry Response Influences Disambiguation ⚠️ MEDIUM

**Threat**: Registry API returns crafted metadata to bias disambiguation toward attacker's package.

**Scenario**:
1. Legitimate package `bat` exists on crates.io (popular file viewer)
2. Attacker publishes package `bat` on npm with fake download counts in API response
3. Ecosystem probe queries both registries in parallel
4. Attacker's npm package claims 10M downloads/month (vs legitimate 45K)
5. Disambiguation ranks npm higher, user installs wrong package

**Current Mitigation**:
- Design specifies filtering (>90 days, >1000 downloads/month) but admits download counts aren't available (lines 16, 42-54)
- Static priority ranking (Homebrew > crates.io > PyPI > npm) mitigates but doesn't eliminate risk
- "Chosen: Existence-first with optional metadata" (line 45) means download counts won't be used anyway

**Analysis**: This threat is largely mitigated by the decision NOT to use download counts from APIs (since they're not available). The static priority ranking is the actual disambiguation mechanism, which cannot be influenced by registry responses.

**Residual Risk**: LOW - Static priority ranking is immune to response injection. However, if design changes to add secondary API calls for stats (rejected alternative, line 51), this becomes HIGH risk.

---

### 7. Package Age Manipulation (Go Builder Only) ⚠️ LOW

**Threat**: Go module could manipulate `Time` field in proxy response to appear older than it is.

**Scenario**:
1. Attacker publishes Go module with backdoor
2. Compromises proxy.golang.org response or performs MITM attack
3. Sets `Time` field to 2-year-old timestamp
4. Passes >90 day age threshold filter
5. Gets auto-selected if it's the only match

**Current Mitigation**: None. Go builder trusts `Time` field from proxy (go.go:25, probe design lines 206-211).

**Recommended Mitigations**:
1. **Cross-validate age**: Check multiple sources (GitHub API for repo creation date, etc.)
2. **Age threshold as noise reduction, not security**: Document that age filter is not cryptographically secure
3. **User confirmation for young packages**: Prompt if package is <90 days old even if registry says otherwise

**Residual Risk**: LOW - Attacker needs to compromise proxy.golang.org or MITM connection. Age filter is noise reduction, not security boundary (design acknowledges this on line 221).

---

## Review of Existing Security Section

The design's "Security Considerations" section (lines 247-293) addresses:

✅ **Download Verification**: Correctly notes that probe doesn't download binaries (line 251)
✅ **Execution Isolation**: Correctly notes read-only queries, no code execution (line 254)
✅ **Supply Chain Risks - Name Confusion**: Addressed with exact matching and priority ranking (line 259)
✅ **User Data Exposure**: Acknowledges tool names sent to registries (line 271)

❌ **Missing**: DNS rebinding, SSRF, redirect handling, timing attacks, response validation
❌ **Insufficient**: "Execution Isolation" section doesn't address SSRF as a form of execution
❌ **Vague**: "Residual risk: A malicious package..." (line 268) doesn't quantify how LIKELY this is given current API landscape

### Mitigations Table Analysis (lines 595-604)

| Risk | Design Mitigation | Gap in Mitigation |
|------|-------------------|-------------------|
| Discovery misdirection | Registry + edit distance + sandbox | Doesn't prevent SSRF/DNS rebinding during probe |
| Typosquatting via ecosystem | Age/download thresholds | **Thresholds don't exist** (APIs don't expose data) |
| LLM prompt injection | N/A (out of scope) | Correct |
| Registry staleness | update-registry, freshness checks | No implementation detail for freshness checks |
| Ecosystem API abuse | 3-second timeout | No rate limiting, no IP validation |

---

## Risk Assessment by Attack Vector

| Attack Vector | Likelihood | Impact | Combined Risk | Escalate? |
|---------------|------------|--------|---------------|-----------|
| DNS Rebinding | MEDIUM | HIGH | **HIGH** | YES |
| SSRF via Registry Response | MEDIUM | HIGH | **HIGH** | YES |
| HTTP Redirect SSRF | LOW | HIGH | **MEDIUM** | YES |
| Response Injection | LOW | MEDIUM | LOW | NO |
| Timing Reconnaissance | LOW | LOW | LOW | NO |
| Disambiguation Manipulation | LOW | LOW | LOW | NO |
| Age Manipulation | LOW | LOW | LOW | NO |

**Escalation Recommended**: DNS rebinding and SSRF via registry responses pose material risk to users running tsuku in cloud environments or corporate networks with internal services.

---

## False Negatives in "Not Applicable" Justifications

The design correctly marks "Download Verification" as not applicable (line 250), but the reasoning is incomplete:

> "The probe only queries registry APIs to check whether a package exists; it doesn't download binaries."

**Why This Is Insufficient**: While the probe doesn't download binaries, it DOES:
1. Make HTTP requests to URLs derived from registry data (SSRF risk)
2. Parse TOML files from arbitrary GitHub URLs (parser vulnerability risk)
3. Follow repository links that could point to internal services (DNS rebinding risk)

The probe's lack of binary downloads doesn't eliminate supply chain risk—it shifts it to the metadata layer.

---

## Recommendations

### Immediate (Before Implementation)

1. **HTTP Client Hardening**:
   - Disable redirects for ecosystem API clients
   - Add DNS resolution validation (reject private IPs)
   - Separate client for metadata fetches with stricter allowlists

2. **URL Validation Layer**:
   - Parse all URLs from registry responses
   - Validate hostname against allowlist (GitHub, GitLab, Bitbucket, SourceHut)
   - Reject private IP addresses and localhost

3. **Update Security Section**:
   - Add DNS rebinding and SSRF to threat model
   - Document redirect policy
   - Clarify that metadata fetching has same SSRF risk as download

### Near-Term (Before Beta Release)

4. **Response Validation**:
   - Strict content-type checking with charset validation
   - TOML bomb protection (depth/key limits)
   - Error message sanitization

5. **Rate Limiting**:
   - Limit ecosystem probes per minute per tool name
   - Prevent timing-based reconnaissance

6. **Monitoring**:
   - Log all external HTTP requests with destinations
   - Alert on private IP access attempts
   - Telemetry for probe timeout patterns

### Long-Term (Post-Launch)

7. **Formal Security Audit**:
   - Penetration testing of ecosystem probe in cloud environment
   - DNS rebinding proof-of-concept
   - SSRF boundary testing

8. **Security Hardening**:
   - Consider sandboxing ecosystem probe in separate process
   - Add optional security mode that disables metadata fetching entirely
   - Implement certificate pinning for known registries

---

## Answers to Specific Questions

### 1. Are there attack vectors we haven't considered?

**YES**: DNS rebinding, SSRF via registry responses, HTTP redirect attacks, timing-based reconnaissance, and response injection are not addressed in the security section.

### 2. Are the mitigations sufficient for the risks identified?

**NO**: The mitigations address supply chain risks at the package selection level (name confusion, popularity) but ignore network-level risks (SSRF, DNS rebinding) during the probe phase.

### 3. Is there residual risk we should escalate?

**YES**: DNS rebinding and SSRF pose material risk to users in cloud environments (AWS, GCP, Azure) where instance metadata services are accessible at link-local addresses. This should be escalated to design review before implementation.

### 4. Are any "not applicable" justifications actually applicable?

**YES**: "Download Verification" is marked N/A because no binaries are downloaded, but this misses that SSRF and DNS rebinding ARE forms of supply chain attack that occur during the probe phase, before download.

### 5. What about DNS rebinding, SSRF, or response injection from malicious registry responses?

**See Attack Vectors 1-4 above**: These are critical gaps. DNS rebinding and SSRF could allow attackers to use tsuku as a proxy to internal services. Response injection is lower risk but should still be mitigated.

### 6. Could a malicious package registry response influence the probe's decision in dangerous ways?

**YES**:
- **Redirect to internal services** (SSRF): High impact, medium likelihood
- **Fake download counts**: Low risk—design doesn't use API download counts
- **Malicious repository URLs**: High impact, medium likelihood (DNS rebinding)
- **Content-type confusion**: Low impact, low likelihood (parser robustness)

The most dangerous influence is via **repository URL fields** that cause metadata fetches from attacker-controlled or internal endpoints.

---

## Conclusion

The ecosystem probe design is sound from a **functional** perspective but has significant **security gaps** in its HTTP client configuration and URL validation. The security section underestimates network-level risks (SSRF, DNS rebinding) while overemphasizing package-level risks (typosquatting) that are already mitigated by static priority ranking.

**Primary Risk**: Users running tsuku in cloud environments could have their instance metadata exposed to attackers via DNS rebinding or SSRF, potentially leaking IAM credentials or API keys.

**Recommendation**: Implement HTTP client hardening (redirect disable, IP validation, hostname allowlist) before proceeding with ecosystem probe implementation. Update security section to accurately reflect network-level risks.

**Risk Acceptance**: If DNS rebinding and SSRF mitigations cannot be implemented before M59 deadline, consider adding a `--disable-probe` flag to allow security-conscious users to skip ecosystem probe stage and fall through to LLM discovery directly.
