# Security Analysis: Toolchain Dependency Pinning

## Executive Summary

The Toolchain Dependency Pinning feature presents **LOW overall security risk** with adequate mitigations in place. The design correctly identifies that pinning only affects version selection during plan generation, not execution-time security controls. However, this analysis identifies **one additional attack vector** (golden file poisoning) and recommends **one enhancement** to strengthen existing mitigations.

**Verdict: No escalation required. Proceed with implementation after addressing the golden file integrity recommendation.**

---

## Analysis of Design-Stated Security Considerations

### 1. Download Verification: CONFIRMED NOT AFFECTED

**Design claim:** Pinning only affects version selection during plan generation. Checksum verification happens at execution time, unchanged.

**Analysis:** Confirmed correct.

Evidence from code review:

1. **Plan execution verifies checksums** (`internal/executor/executor.go:431-434`):
   ```go
   if step.Action == "download" && step.Checksum != "" {
       if err := e.executeDownloadWithVerification(ctx, execCtx, step, plan); err != nil {
           return fmt.Errorf("step %d (%s) failed: %w", i+1, step.Action, err)
       }
   }
   ```

2. **Checksum verification is independent of version selection** (`executor.go:467-507`):
   - `executeDownloadWithVerification()` computes SHA256 of the downloaded file
   - Compares against `step.Checksum` from the plan
   - Returns `ChecksumMismatchError` if verification fails

3. **Version pinning happens in constraint extraction** (`constraints.go:32-53`):
   - `ExtractConstraintsFromPlan()` only extracts version strings
   - No modification to checksum handling path

**Conclusion:** The execution-time verification chain is completely separate from the version selection path. A pinned older version still has its checksum verified against the golden file's recorded checksum.

### 2. Execution Isolation: CONFIRMED NOT AFFECTED

**Design claim:** This feature changes which version is selected, not how the selected version is executed.

**Analysis:** Confirmed correct.

Evidence from code review:

1. **Resource limits are unchanged** (`executor.go:811-830`):
   - `validateResourceLimits()` checks dependency depth (max 5) and total count (max 100)
   - These limits apply regardless of whether versions are pinned or dynamically resolved

2. **Platform validation is unchanged** (`executor.go:793-809`):
   - `validatePlatform()` still validates OS/arch compatibility
   - No special cases for pinned versions

3. **Action execution unchanged**:
   - `action.Execute(execCtx, step.Params)` is called identically for all versions
   - No bypass of validation for pinned versions

**Conclusion:** Execution isolation controls operate on plans, not on the version resolution process.

### 3. Supply Chain Risks: CONFIRMED MINIMAL WITH ADEQUATE MITIGATION

**Design claim:** Risk exists that older versions might have known vulnerabilities. Mitigated by (a) pinning only during CI validation, (b) periodic golden file regeneration.

**Analysis:** Assessment is accurate but slightly incomplete.

**Risk quantification:**

| Risk Factor | Severity | Likelihood | Notes |
|------------|----------|------------|-------|
| Vulnerable toolchain version | Medium | Low | Version providers typically remove severely compromised versions |
| Dependency confusion | N/A | N/A | Toolchains are first-party (go, python-standalone, nodejs) |
| Typosquatting | N/A | N/A | Toolchain names are hardcoded in recipes |

**Mitigation effectiveness:**

1. **`--pin-from` is CI-only**: Confirmed in design. Users running `tsuku install` are unaffected.
   - Verified: `cfg.Constraints` is only populated when `--pin-from` flag is passed
   - Normal installations (`tsuku install <tool>`) always resolve to latest

2. **Golden file regeneration**: Effective but process not enforced.
   - Recommendation: Document regeneration schedule (e.g., monthly) in CONTRIBUTING.md

**Additional consideration:** The design correctly notes that if version providers aggressively prune old versions, fallback becomes the common path. The graceful degradation (warn + use available version) is appropriate.

**Conclusion:** Supply chain risk is acceptably mitigated given the CI-only scope.

### 4. User Data Exposure: CONFIRMED NOT APPLICABLE

**Design claim:** This feature operates on golden file JSON and recipe TOML only. No user data is accessed.

**Analysis:** Confirmed correct.

Data flow audit:

1. **Input:** Golden file JSON (generated in CI), Recipe TOML (from registry)
2. **Processing:** String parsing, map operations, version comparison
3. **Output:** Modified `EvalConstraints` struct

No file system operations outside work directory. No network calls during constraint extraction. No user home directory access during this phase.

**Conclusion:** Classification as "not applicable" is accurate.

---

## Additional Attack Vectors Identified

### Attack Vector 1: Golden File Poisoning (NEW)

**Description:** A malicious actor with write access to the golden file repository could craft a golden file that pins dependencies to specific vulnerable versions.

**Attack scenario:**
1. Attacker compromises CI or gains PR merge access
2. Attacker modifies `golden/black-26.1a1.json` to pin `python-standalone@<vulnerable-version>`
3. Future CI runs with `--pin-from` use the vulnerable toolchain
4. If the vulnerable version has code execution flaws, CI environment is compromised

**Likelihood:** Low (requires repository write access)

**Impact:** Medium (CI environment compromise only, not user installations)

**Current mitigations:**
- PR review process
- CI runs in sandboxed environment
- Golden files are committed (visible in git history)

**Recommended enhancement:**
Consider adding integrity verification for golden files:
```go
// In ExtractConstraints()
if cfg.VerifyGoldenIntegrity {
    expectedHash := loadGoldenHashFromRegistry(planPath)
    actualHash := sha256(data)
    if expectedHash != actualHash {
        return nil, fmt.Errorf("golden file integrity check failed")
    }
}
```

**Assessment:** This is an **acceptable residual risk** given:
1. Existing PR review controls
2. CI-only impact scope
3. Git history provides auditability

No immediate action required, but consider for future hardening.

### Attack Vector 2: First-Encountered-Wins Version Conflict (Examined, Not a Risk)

**Description:** The design uses "first-encountered wins" for version conflicts during depth-first traversal.

**Analysis:** This is NOT an attack vector because:
1. Conflict resolution only affects which pinned version is used
2. All candidate versions come from the same trusted golden file
3. Checksum verification still applies regardless of which version wins

**Conclusion:** Design decision is sound.

### Attack Vector 3: Version String Injection (Examined, Not a Risk)

**Description:** Could a malicious version string in the golden file cause command injection?

**Analysis:** No, because:
1. Version strings are used in `NewWithVersion()` which passes to version providers
2. Version providers use HTTP APIs (GitHub, PyPI, etc.) with proper URL encoding
3. No shell execution with version strings

Evidence from `executor.go:60-67`:
```go
func NewWithVersion(r *recipe.Recipe, version string) (*Executor, error) {
    exec, err := New(r)
    if err != nil {
        return nil, err
    }
    exec.reqVersion = version  // Stored, not executed
    return exec, nil
}
```

**Conclusion:** No injection risk.

---

## Review of Existing Security Infrastructure

The codebase demonstrates strong security practices that remain effective with this feature:

### SSRF Protection (version/security_test.go)
- Link-local IP blocking (AWS metadata: 169.254.169.254)
- Private IP blocking (10.x, 172.16.x, 192.168.x)
- Loopback blocking (127.x, ::1)
- IPv4-mapped IPv6 validation
- Redirect chain limits (max 5)

**Relevance to this feature:** Version providers use this hardened HTTP client. Pinned versions are resolved through the same secure path.

### Package Name Validation
- Unicode homoglyph protection
- Control character rejection
- Path traversal prevention
- Length limits (214 chars for npm)

**Relevance to this feature:** Dependency names from golden files pass through same validation.

### Response Size Limits
- 50MB limit on responses
- Compression bomb protection (reject compressed responses)

**Relevance to this feature:** Applies to any version provider calls, including resolving pinned versions.

---

## Answers to Review Questions

### Q1: Are there attack vectors we haven't considered?

**Answer:** One additional vector identified: **Golden File Poisoning**. However, it has:
- Low likelihood (requires repo write access)
- Medium impact (CI-only)
- Existing mitigations (PR review, git history)

Assessment: Acceptable residual risk. No design changes required.

### Q2: Are the mitigations sufficient for the risks identified?

**Answer:** Yes. The mitigations are appropriate for the risk level:

| Risk | Mitigation | Sufficiency |
|------|------------|-------------|
| Vulnerable old version | CI-only scope, periodic regeneration | Adequate |
| Golden file poisoning | PR review, git history, CI sandbox | Adequate |
| Version conflicts | First-encountered-wins is deterministic | N/A (not a risk) |
| Version string injection | No shell execution | N/A (not a risk) |

### Q3: Is there residual risk we should escalate?

**Answer:** No. All identified risks are within acceptable bounds:
- User installations are completely unaffected
- CI environment has defense-in-depth (sandbox, review, audit)
- Graceful degradation prevents brittle failures

No escalation required.

### Q4: Are any "not applicable" justifications actually applicable?

**Answer:** No. The "User Data Exposure: Not applicable" classification is accurate:
- Feature operates only on JSON/TOML files from the registry
- No user credentials, preferences, or personal data involved
- No home directory access during constraint extraction

---

## Recommendations

### Required (before merge)
None. The design is sound.

### Recommended (future hardening)
1. **Document golden file regeneration schedule** in CONTRIBUTING.md (e.g., monthly refresh)
2. **Consider golden file integrity hashes** in version-sources.toml (low priority)

### Optional (defense-in-depth)
1. Add log entry when using fallback version (already designed, ensure implementation)
2. Consider warning in CI output when golden file is older than 90 days

---

## Conclusion

The Toolchain Dependency Pinning feature has been designed with appropriate security boundaries. The core insight - that pinning affects version selection but not execution security controls - is correct and well-implemented. The existing security infrastructure (SSRF protection, checksum verification, resource limits) remains fully effective.

**Security verdict: APPROVED for implementation.**
