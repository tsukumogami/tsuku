# Security Review: Hardcoded Version Detection Feature

## Executive Summary

This security review examines the Hardcoded Version Detection feature proposed in DESIGN-hardcoded-version-detection.md. The feature adds static analysis validation to detect hardcoded versions in recipe TOML files before installation.

**Overall Assessment: LOW RISK with minor recommendations**

The feature is purely static analysis operating on TOML content before any downloads or execution occur. The security considerations in the design document are accurate, though some nuances warrant additional discussion.

---

## 1. Attack Vector Analysis

### 1.1 Vectors Considered in the Design

| Vector | Coverage | Assessment |
|--------|----------|------------|
| Download verification impact | Addressed | Correct - no impact |
| Execution isolation | Addressed | Correct - not applicable |
| Supply chain risks | Addressed | Correct positive framing |
| User data exposure | Addressed | Correct - not applicable |

### 1.2 Additional Attack Vectors Identified

#### 1.2.1 ReDoS (Regular Expression Denial of Service)

**Risk Level: LOW**

The design proposes using regex patterns to detect version strings:

```go
var versionPatterns = []*regexp.Regexp{
    regexp.MustCompile(`\b[vV]?(\d+)\.(\d+)\.(\d+)(-[a-zA-Z0-9.]+)?(\+[a-zA-Z0-9.]+)?\b`),
    regexp.MustCompile(`\b[vV]?(\d+)\.(\d+)\b`),
    regexp.MustCompile(`\b20\d{2}\.\d{2}(\.\d{2})?\b`),
}
```

**Analysis:**
- Go's `regexp` package uses RE2, which is immune to catastrophic backtracking by design
- RE2 runs in guaranteed linear time O(n) relative to input length
- The proposed patterns are simple and don't contain nested quantifiers

**Verdict: NOT VULNERABLE** - Go's RE2 engine prevents ReDoS attacks that affect PCRE-based engines.

#### 1.2.2 Validation Bypass via Crafted Input

**Risk Level: LOW**

An attacker could craft recipe content to evade detection:

1. **Unicode lookalikes**: Using characters like `1.2.3` where digits are Unicode lookalikes
2. **URL encoding**: `https://example.com/tool-%31.%32.%33.tar.gz`
3. **Template injection**: `{version}1.2.3` mixing placeholders with literals

**Mitigations in Design:**
- The detection is defense-in-depth, not security-critical
- Recipe authors are trusted contributors (PR review process)
- Detection failure results in a warning, not security bypass

**Recommendation:** Document that detection is best-effort and not a security boundary.

#### 1.2.3 False Positive Exploitation

**Risk Level: NEGLIGIBLE**

An attacker could theoretically submit recipes with legitimate-looking but malicious values that trigger false positive warnings, hoping reviewers become desensitized.

**Analysis:**
- This is a social engineering attack, not a technical vulnerability
- PR review process is the mitigation
- The feature doesn't change the security model

**Verdict: NOT A SECURITY CONCERN** - existing PR review process remains the defense.

#### 1.2.4 Memory Exhaustion via Large Input

**Risk Level: LOW**

A maliciously crafted recipe with very large field values could consume memory during regex scanning.

**Analysis:**
- TOML parser already has limits during recipe loading
- Recipe files are typically small (< 10KB)
- Validation runs locally, not in a security-sensitive context

**Recommendation:** No specific mitigation needed; existing TOML parser limits are sufficient.

---

## 2. Mitigation Sufficiency Analysis

### 2.1 Current Mitigations in Design

| Mitigation | Target Risk | Assessment |
|------------|-------------|------------|
| High-confidence patterns first | Over-detection | **SUFFICIENT** - Start narrow, expand based on feedback |
| Clear documentation | Edge cases | **SUFFICIENT** - Explains when static versions are acceptable |
| Recipe-level suppression (future) | Legitimate exceptions | **DEFERRED** - Reasonable to postpone |

### 2.2 Additional Mitigations to Consider

#### 2.2.1 URL Normalization Before Detection

**Current State:** Detection scans raw field values.

**Recommendation:** Consider normalizing URLs (decode percent-encoding) before pattern matching to catch evasion attempts like `%31.%32.%33`.

**Priority: LOW** - Attackers who evade detection still face PR review.

#### 2.2.2 Logging for Audit Trail

**Current State:** Warnings are displayed but not logged.

**Recommendation:** When running in CI, log detections to an audit file for post-mortem analysis of what passed/failed validation.

**Priority: LOW** - Nice-to-have for recipe quality analysis.

---

## 3. Residual Risk Assessment

### 3.1 Risks to Escalate: NONE

After analysis, no risks require escalation. The feature:
- Does not introduce new attack surface
- Does not change the trust model
- Does not access network or execute code
- Does not modify files

### 3.2 Accepted Residual Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Detection bypass | LOW | LOW | PR review remains primary control |
| False positives | MEDIUM | LOW | Clear documentation, suppression option |
| False negatives | MEDIUM | LOW | Iterative pattern improvement |

---

## 4. "Not Applicable" Justification Review

The design marks several security considerations as "Not applicable." Let's validate each:

### 4.1 Download Verification - "Not directly impacted"

**Assessment: ACCURATE**

The design correctly states that detection runs during validation, before any downloads occur. The validation workflow is:

```
Recipe TOML -> ValidateFile() -> [includes hardcoded detection] -> Warnings
                                             |
                                             v
                                    (No downloads yet)
```

Evidence from codebase:
- `cmd/tsuku/validate.go` calls `recipe.ValidateFile()` which operates on TOML bytes
- No HTTP calls are made during validation
- Download actions are only executed during `tsuku install`

**Verdict: CORRECT - Not applicable**

### 4.2 Execution Isolation - "Not applicable"

**Assessment: ACCURATE**

The feature is pure static analysis:
- Regex matching on string fields
- No subprocess spawning
- No file system writes
- No network access

Evidence from codebase:
- `ValidateFile()` returns a `ValidationResult` struct
- All validation functions are side-effect-free
- The `Preflight` interface contract explicitly forbids side effects

**Verdict: CORRECT - Not applicable**

### 4.3 Supply Chain Risks - "Positive impact"

**Assessment: ACCURATE with nuance**

The design correctly identifies the positive impact: detecting hardcoded versions helps prevent recipes from being locked to potentially malicious versions.

**Nuance to add:** The detection also helps with version transparency. A hardcoded version in a PR is more visible for review than a dynamic `{version}` that might resolve to a suspicious version at runtime.

**Verdict: CORRECT - Positive impact confirmed**

### 4.4 User Data Exposure - "Not applicable"

**Assessment: ACCURATE**

The feature:
- Reads recipe TOML content only
- Does not access user environment variables
- Does not read user home directory
- Does not transmit any data

**Verdict: CORRECT - Not applicable**

---

## 5. Integration with Existing Security Controls

The codebase already has robust security controls that this feature integrates with:

### 5.1 Existing Download Security

- HTTPS enforcement in `download.go` (line 303-305): `if !strings.HasPrefix(url, "https://")`
- SSRF protection in `httputil/ssrf.go`: IP validation for redirects
- Checksum verification in download workflow

### 5.2 Existing Validation Security

- Path traversal prevention in `extract.go`: `isPathWithinDirectory()`
- Symlink validation: `validateSymlinkTarget()`
- Dangerous pattern detection in verify commands: `validateDangerousPatterns()`

### 5.3 Existing Recipe Security

- URL scheme validation in `validator.go`: Only http/https allowed
- Path parameter validation: Rejects `..` traversal
- Checksum format validation: SHA256 hex format enforced

**The hardcoded version detection feature aligns with these existing controls and adds another layer of recipe quality assurance.**

---

## 6. Recommendations Summary

### Must-Have (Pre-merge)

None. The feature is low-risk as designed.

### Should-Have (Post-initial implementation)

1. **Document detection limitations**: Add to CONTRIBUTING.md that detection is best-effort and reviewers should still manually verify version patterns.

2. **Test with adversarial inputs**: Add test cases with Unicode lookalikes, URL encoding, and edge cases to verify detection behavior.

### Nice-to-Have (Future iterations)

1. **URL normalization**: Decode percent-encoding before pattern matching.

2. **Audit logging in CI**: Log detection results for recipe quality metrics.

3. **Pattern tuning metrics**: Track false positive/negative rates to improve patterns over time.

---

## 7. Conclusion

The Hardcoded Version Detection feature is a low-risk addition to the tsuku validation pipeline. The security considerations in the design document are accurate and complete. The feature:

- **Does not introduce new attack surface**: Pure static analysis on trusted input
- **Does not change the security model**: PR review remains the primary control
- **Provides defense-in-depth**: Catches common recipe mistakes before merge
- **Has positive supply chain impact**: Prevents version-locked recipes

**Recommended action: APPROVE with minor documentation additions.**

---

## Appendix: Codebase Evidence

### A.1 Validation is Side-Effect Free

From `internal/recipe/validate.go`:
```go
// ValidateStructural performs fast, structural validation without external dependencies.
// This is suitable for parse-time validation in the loader.
```

From `internal/actions/preflight.go`:
```go
// CONTRACT: Preflight MUST NOT have side effects (no filesystem, no network).
```

### A.2 Existing Security Controls

From `internal/actions/download.go`:
```go
// SECURITY: Enforce HTTPS for all downloads
if !strings.HasPrefix(url, "https://") {
    return fmt.Errorf("download URL must use HTTPS for security, got: %s", url)
}
```

From `internal/actions/extract.go`:
```go
// SECURITY: Validate that target path is within destPath (prevents path traversal)
if !isPathWithinDirectory(target, destPath) {
    return fmt.Errorf("archive entry escapes destination directory: %s", header.Name)
}
```

### A.3 Go RE2 Safety

Go's `regexp` package documentation:
> The package uses RE2 syntax. The key property of RE2 is that it runs in time linear in the size of the input, making it safe to use with untrusted inputs.
