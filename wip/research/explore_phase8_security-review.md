# Security Review: Pipeline Dashboard Design

**Date**: 2026-02-03
**Design Document**: `docs/designs/DESIGN-pipeline-dashboard.md`
**Status**: Proposed

## Executive Summary

The Pipeline Dashboard security analysis is well-constructed and addresses the primary threat vectors. The design's inherent simplicity (static files, no backend) significantly reduces the attack surface. However, there are a few additional considerations and one minor gap worth addressing.

**Overall Assessment**: The security posture is appropriate for the feature's scope. The identified residual risks are acceptable given the public nature of all displayed data.

---

## Review of Existing Security Analysis

### Download Verification: "Not Applicable"

**Assessment**: Correctly marked as not applicable.

The design only reads from repository-resident JSON files and writes to a repository-resident JSON file. There are no external downloads. The justification is accurate.

**Verdict**: Appropriate.

---

### Execution Isolation

**Assessment**: Accurately described, with one addition.

The analysis correctly identifies:
- File system access scope (read `data/`, write `website/pipeline/dashboard.json`)
- No network access required
- Standard GitHub Actions runner permissions

**Additional consideration**: The shell script processes JSONL files using `jq`. While jq is memory-safe and doesn't execute arbitrary code from input, malformed JSON could cause jq to crash or hang. The design should ensure:
1. Invalid JSON lines in JSONL files are skipped gracefully
2. The script has a reasonable timeout (already implicit in CI job timeout)

**Verdict**: Sufficient with minor hardening recommendation.

---

### Supply Chain Risks

**Assessment**: Well-analyzed.

The analysis correctly identifies that:
- All data sources are written by CI with `GITHUB_TOKEN` permissions
- PRs require review before merge
- Cloudflare Pages deployment requires code review

**Additional considerations**:

1. **GitHub Actions token scope**: The batch-generate workflow uses `secrets.PAT_BATCH_GENERATE` (a Personal Access Token) rather than the default `GITHUB_TOKEN`. This PAT presumably has `contents: write` and `pull-requests: write` permissions. If this PAT were compromised, an attacker could:
   - Commit arbitrary data to the repository
   - Create PRs that bypass branch protection if auto-merge is enabled

   **Mitigation**: The PAT should have minimal scope. Repository settings should require status checks to pass before merge. The design document could note that the PAT scope should be audited periodically.

2. **Dependency on jq**: The script relies on `jq` being available. On GitHub Actions ubuntu-latest runners, jq is pre-installed. This is a trusted system dependency, not a downloaded artifact.

3. **JSONL accumulation**: The design notes that JSONL files grow indefinitely. While not a security issue per se, extremely large files could be used to degrade CI performance (a form of resource exhaustion). The 1MB rotation threshold mentioned in the design is a reasonable mitigation.

**Verdict**: Appropriate with minor recommendations.

---

### User Data Exposure

**Assessment**: Accurate and complete.

The analysis correctly identifies:
- All exposed data (package names, failure messages, timestamps) is public
- No user-identifying information
- No secrets or API keys
- No local filesystem paths

**Verification**: I reviewed the actual failure data in `data/failures/homebrew.jsonl`. The failure messages contain:
- Homebrew formula names (public)
- tsuku CLI output (public)
- Generic error messages (no sensitive information)
- Timestamps (non-identifying)

One edge case worth noting: failure messages could theoretically contain filesystem paths from the CI runner (e.g., `/tmp/action-validator-NNNN`). These are:
- Ephemeral CI paths (not sensitive)
- Not user-identifying
- Standard CI behavior visible in public logs anyway

**Verdict**: Appropriate.

---

### XSS Mitigation

**Assessment**: The claim "Dashboard renders text as textContent, not innerHTML" is a design intention, not yet implemented. This is the correct approach.

**Verification required**: When the HTML dashboard is implemented, the code review must verify:
```javascript
// Correct (safe)
element.textContent = failureMessage;

// Incorrect (XSS vulnerable)
element.innerHTML = failureMessage;
```

**Additional XSS vectors to address**:
1. **Template literals with string interpolation**: If using template literals like `` `<div>${data}</div>` ``, the data is still interpreted as HTML when assigned to innerHTML.
2. **URL injection**: If blocker package names become links (unlikely in current design), they must be validated before use in `href` attributes.

**Verdict**: Appropriate mitigation identified. Implementation must be verified during code review.

---

## Attack Vectors Not Covered

### 1. JSONL Injection via Upstream Data Corruption

**Scenario**: If the batch-generate workflow has a bug that writes malformed data to JSONL files, subsequent dashboard generation could fail or produce unexpected output.

**Risk Level**: Low

**Mitigation**:
- jq naturally fails on malformed JSON (graceful degradation)
- The dashboard JSON schema is simple and well-defined
- Malformed data would be caught during PR review

**Recommendation**: The generation script should validate that output JSON is well-formed before writing.

---

### 2. Denial of Service via Data Volume

**Scenario**: An attacker with write access could commit extremely large data files, causing:
- Slow page loads
- Memory issues in the browser
- CI timeouts during dashboard generation

**Risk Level**: Low (requires write access)

**Mitigation identified**:
- Dashboard truncates display (max 10 blockers, 10 runs)
- Data growth rate is naturally limited (one batch run every 3 hours)

**Recommendation**: Add a file size check in the generation script:
```bash
# Abort if source files are unreasonably large
for f in "$QUEUE_FILE" "$FAILURES_FILE" "$METRICS_FILE"; do
  if [ -f "$f" ] && [ "$(stat -c%s "$f" 2>/dev/null || stat -f%z "$f")" -gt 10485760 ]; then
    echo "Error: $f exceeds 10MB size limit"
    exit 1
  fi
done
```

---

### 3. Race Condition in Dashboard Generation

**Scenario**: If two batch runs overlap (despite concurrency controls), they could write conflicting data to `dashboard.json`.

**Risk Level**: Very Low

**Mitigation already in place**:
- Workflow concurrency group (`concurrency: group: batch-generate`) prevents parallel runs
- GitHub Actions handles race conditions at the commit level

**Verdict**: Already mitigated.

---

### 4. Information Disclosure via Failure Messages

**Scenario**: Failure messages might inadvertently contain sensitive information in edge cases.

**Risk Level**: Very Low

**Analysis**: I reviewed the failure message patterns:
- "no LLM providers available: set ANTHROPIC_API_KEY or GOOGLE_API_KEY" - mentions environment variables but not values
- "recipe already exists at recipes/p/pkgconf.toml" - filesystem path (not sensitive)
- "failed to fetch bottle data for formula X" - API error (not sensitive)

These are all appropriate for public display.

**Recommendation**: If future failure modes might include sensitive data, add a sanitization step. For now, no action needed.

---

## Residual Risk Assessment

| Risk | Likelihood | Impact | Residual Risk | Escalation Required? |
|------|------------|--------|---------------|---------------------|
| Compromised PAT injects bad data | Very Low | Medium | Low | No |
| XSS via failure messages | Low | Medium | Low (with textContent) | No |
| DoS via large JSON files | Low | Low | Very Low | No |
| Malformed JSON crashes dashboard | Low | Low | Very Low | No |

**Conclusion**: No residual risks require escalation. All are within acceptable bounds for a public-facing, read-only dashboard displaying public data.

---

## Recommendations

### Required (before implementation)

1. **Verify XSS mitigation in code review**: Ensure all dynamic content uses `textContent`, not `innerHTML`.

### Recommended (during implementation)

2. **Add input validation to generation script**: Validate that output JSON is well-formed before writing to `dashboard.json`.

3. **Add file size limits**: Abort generation if source files exceed 10MB (prevents resource exhaustion).

4. **Document PAT scope requirements**: Note in workflow comments that `PAT_BATCH_GENERATE` should have minimal scope.

### Optional (future hardening)

5. **Content Security Policy**: Add CSP headers to the dashboard page via Cloudflare Pages `_headers` file:
   ```
   /pipeline/*
     Content-Security-Policy: default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'
   ```

6. **Subresource Integrity**: If the dashboard ever loads external scripts (currently none), use SRI hashes.

---

## Answers to Review Questions

### 1. Are there attack vectors we haven't considered?

Yes, but all are low-risk:
- JSONL injection via upstream bugs (mitigated by jq's JSON parsing)
- DoS via data volume (mitigated by truncation and natural growth limits)
- Race conditions (mitigated by concurrency group)

See "Attack Vectors Not Covered" section for details.

### 2. Are the mitigations sufficient for the risks identified?

Yes. The mitigations are appropriate for the risk level. Key factors:
- All displayed data is already public
- The dashboard is read-only
- The attack surface is minimal (static files, no backend)
- Browser sandbox provides isolation

### 3. Is there residual risk we should escalate?

No. All residual risks are within acceptable bounds:
- "Compromised CI could inject bad data" is inherent to any CI-generated content and mitigated by GitHub's security model and code review requirements.
- XSS risk is mitigated by the specified `textContent` approach (must be verified in implementation).

### 4. Are any "not applicable" justifications actually applicable?

No. The "Download Verification: Not Applicable" is correctly marked. The feature genuinely does not download external artifacts.

---

## Summary

The Pipeline Dashboard design has a sound security posture. The security section in the design document is accurate and complete for the identified threat vectors. This review identified a few additional considerations (JSONL injection, DoS via data volume, PAT scope) but none that change the overall assessment.

**Recommendation**: Proceed with implementation. Ensure code review verifies XSS mitigations and consider the optional hardening recommendations for defense in depth.
