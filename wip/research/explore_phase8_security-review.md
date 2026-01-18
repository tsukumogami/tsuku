# Security Review: dlopen Load Testing Design (Level 3)

**Document Reviewed:** `docs/designs/DESIGN-library-verify-dlopen.md`
**Review Date:** 2026-01-18
**Reviewer:** Security Analysis Agent

---

## Executive Summary

The design proposes downloading a helper binary (`tsuku-dltest`) and using it to execute `dlopen()` on libraries for verification. This involves:

1. Downloading a binary from GitHub releases
2. Verifying it against embedded checksums
3. Executing code from library initialization sections (`.init`, constructors)

The design demonstrates good security awareness and addresses most obvious attack vectors. However, this review identifies several attack vectors that warrant additional consideration, questions some mitigation assumptions, and identifies residual risks that should be escalated.

---

## 1. Attack Vectors Not Adequately Addressed

### 1.1 Time-of-Check to Time-of-Use (TOCTOU) on Helper Binary

**Vector:** The design verifies the helper checksum, then executes it. A local attacker with write access to `$TSUKU_HOME/.dltest/` could replace the binary between verification and execution.

**Current Design Gap:** The document describes "atomic rename" during installation but doesn't address TOCTOU during invocation.

**Recommendation:**
- Execute the binary from a file descriptor obtained during verification (Linux: `fexecve()` or `/proc/self/fd/N`)
- On macOS, use file locking or at minimum verify the binary is on a filesystem that doesn't allow modification while open
- Document the threat model assumption that `$TSUKU_HOME` is trusted (user-controlled, non-shared filesystem)

**Severity:** Medium (requires local attacker with write access to user's home directory)

### 1.2 Environment Variable Injection

**Vector:** The design explicitly inherits tsuku's environment (`cmd.Env = os.Environ()`). This exposes several attack surfaces:

1. **`LD_PRELOAD`/`DYLD_INSERT_LIBRARIES`**: An attacker who can set environment variables before tsuku runs could inject malicious libraries that execute during the helper's dlopen calls
2. **`LD_LIBRARY_PATH` poisoning**: Could redirect shared library resolution to attacker-controlled locations
3. **`LD_AUDIT`**: Could inject auditing code that intercepts all library loads

**Current Design Gap:** The document acknowledges environment inheritance for legitimate reasons (`LD_LIBRARY_PATH` for `$ORIGIN` dependencies) but doesn't sanitize dangerous variables.

**Recommendation:**
- Create an allowlist of environment variables the helper inherits
- Explicitly strip `LD_PRELOAD`, `LD_AUDIT`, `DYLD_INSERT_LIBRARIES`, `DYLD_PRINT_*` etc.
- Document remaining inherited variables and their purpose

**Severity:** Medium (requires attacker to control tsuku's environment)

### 1.3 Symlink/Path Traversal in Library Paths

**Vector:** The design passes library paths as command-line arguments to the helper. If tsuku doesn't canonicalize paths, a malicious recipe could include paths like:
- `/home/user/.tsuku/libs/foo/../../../etc/malicious.so`
- Symlinks within `$TSUKU_HOME/libs/` pointing outside the directory

**Current Design Gap:** No path validation is specified before passing to the helper.

**Recommendation:**
- Canonicalize all paths using `filepath.EvalSymlinks()` and verify they remain within `$TSUKU_HOME/libs/`
- Add path validation in the helper itself as defense-in-depth
- Document whether symlinks are allowed in library paths

**Severity:** Medium (recipes are already trusted, but this is defense-in-depth)

### 1.4 Resource Exhaustion via Malicious Libraries

**Vector:** A library's initialization code could:
- Allocate unbounded memory (OOM killer affects system)
- Spawn child processes (bypassing the 5-second timeout)
- Create many file descriptors (exhaust system limits)
- Fork bomb within the timeout window

**Current Design Gap:** Only timeout is mentioned as a resource limit.

**Recommendation:**
- Consider using cgroups on Linux to limit memory, CPU, and process count
- Document that macOS lacks equivalent isolation without full sandbox
- On Linux, consider using `RLIMIT_AS`, `RLIMIT_NPROC` before exec
- Accept as residual risk with documentation if sandboxing is too complex

**Severity:** Low-Medium (libraries are user-installed, but supply chain attacks could exploit this)

### 1.5 Signal Handling Race Conditions

**Vector:** The helper binary handles multiple libraries in sequence. If a library's initialization code installs signal handlers (e.g., SIGSEGV handler), subsequent library loads may behave unexpectedly or mask crashes.

**Current Design Gap:** No mention of signal handling strategy.

**Recommendation:**
- Reset signal handlers to default before each `dlopen()` call
- Use `sigprocmask()` to block signals during critical sections
- Consider if helper should ignore SIGTERM/SIGINT during execution

**Severity:** Low (edge case, but could mask real issues)

### 1.6 Helper Binary Persistence as Escalation Vector

**Vector:** Once the helper is installed, it's a privileged binary (from the user's perspective) that any process can invoke. If the helper has vulnerabilities (buffer overflows in path parsing, etc.), it becomes an attack target for other malware on the system.

**Current Design Gap:** No hardening requirements specified for the helper binary.

**Recommendation:**
- Build helper with `-buildmode=pie` (Position Independent Executable)
- Enable stack protector (`-fstack-protector-strong`)
- Consider static linking to avoid glibc-specific attack surfaces
- Document security build flags in goreleaser config

**Severity:** Low (helper is minimal, but good hygiene)

---

## 2. Mitigation Sufficiency Analysis

### 2.1 Process Isolation - Sufficient with Caveats

**Claim:** "Process isolation comes free (separate process means crashes don't affect tsuku)"

**Analysis:** Process isolation is necessary but not sufficient. Shared resources can still be affected:
- Shared memory segments created by library
- Files modified during initialization
- Network connections established
- Named pipes or UNIX sockets created

**Verdict:** Mitigation is adequate for tsuku's stability. The "what the helper can do" section acknowledges residual risk appropriately. However, users may not understand that "process isolation" doesn't mean "sandboxed."

**Recommendation:** Clarify in user-facing documentation that process isolation protects tsuku but doesn't sandbox the library code.

### 2.2 Timeout Protection - Mostly Sufficient

**Claim:** "5-second timeout prevents infinite loops or hangs"

**Analysis:**
- **Sufficient for:** Blocking syscalls, CPU-bound loops
- **Insufficient for:** Fork bombs (children outlive parent), background threads, detached child processes

**Verdict:** Acceptable with documentation. The 5-second window is acknowledged as residual risk.

**Recommendation:** Consider `RTLD_NODELETE` to prevent libraries from starting detached threads that outlive `dlclose()`. Document that forked processes are not killed.

### 2.3 Checksum Verification - Mostly Sufficient

**Claim:** "Embedded checksums, fail-closed verification"

**Analysis:**
- Checksums use SHA256 which is cryptographically secure
- Fail-closed behavior is correct
- Version checking prevents downgrade to older (potentially vulnerable) helper

**Gaps:**
- No signature verification (checksums verify integrity, not authenticity)
- If GitHub Actions is compromised, attacker could publish malicious binary AND update checksums in same PR

**Verdict:** Acceptable given trust model. Users already trust tsuku releases.

**Recommendation:** Consider GPG signing of releases for future hardening. Document that the threat model assumes GitHub and CI are trusted.

### 2.4 User Opt-Out - Sufficient

**Claim:** "`--skip-dlopen` flag allows users to skip Level 3"

**Analysis:** Opt-out is present. Users who want to avoid code execution have a path.

**Verdict:** Sufficient.

**Recommendation:** Consider making Level 3 opt-in for first-time users or in CI environments (via environment variable like `TSUKU_DLOPEN_ENABLED=0`).

---

## 3. Residual Risks Requiring Escalation

### 3.1 CRITICAL: 5-Second Arbitrary Code Execution Window

**Risk:** Any installed library's initialization code runs with user privileges for up to 5 seconds.

**Impact:** This is a significant window for:
- Credential theft (`~/.ssh/`, `~/.aws/`, `~/.kube/`)
- Cryptocurrency wallet theft
- Backdoor installation
- Data exfiltration

**Current Mitigation Assessment:** Process isolation doesn't prevent this. Opt-out requires user awareness.

**Escalation Recommendation:**
1. **Documentation**: Explicitly warn users that `tsuku verify` executes library code
2. **Consent**: Consider requiring explicit opt-in for Level 3 verification (not just opt-out)
3. **CI Defaults**: Default to `--skip-dlopen` in CI environments (detect `CI=true`)
4. **Principle of Least Surprise**: Users running "verify" may not expect code execution

**Suggested User-Facing Warning:**
```
Level 3 verification will load libraries to test if they work. This executes
initialization code from the libraries you installed. Use --skip-dlopen to
skip this step if you're concerned about code execution.
```

### 3.2 HIGH: Supply Chain Attack Amplification

**Risk:** A compromised recipe that installs a malicious library would have its malicious initialization code executed during verification.

**Why This Matters:**
- Users might verify a library before trusting it
- "Verify" implies safety checking, but Level 3 executes code
- This flips the mental model: verification becomes an attack trigger

**Escalation Recommendation:**
1. **Rename consideration**: "Load test" instead of "verify" for Level 3
2. **Clear separation**: Show Levels 1-2 results before prompting for Level 3
3. **Default off**: Consider making Level 3 opt-in entirely

### 3.3 MEDIUM: GitHub Actions as Single Point of Compromise

**Risk:** If GitHub Actions is compromised, attacker can publish malicious helper AND update checksums in the same malicious commit.

**Current Assessment:** This is acknowledged as same trust level as tsuku itself.

**Escalation Recommendation:**
1. **Multi-party signing**: Require release manager to GPG-sign the checksum file separately from CI
2. **Reproducible builds**: Allow users to verify helper binary matches source
3. **Accept and document**: If above are too complex, explicitly document this trust assumption

---

## 4. "Not Applicable" Justifications Review

### 4.1 "Privilege escalation: Helper runs as same user as tsuku - None"

**Review:** This is correctly marked as "None" for privilege escalation risk. The helper cannot gain elevated privileges that tsuku doesn't have.

**Verdict:** Correctly justified.

### 4.2 "Helper crashes tsuku: Separate process, error handling - None"

**Review:** This is correctly marked as "None." Process crash isolation is effective.

**Verdict:** Correctly justified.

### 4.3 "MITM during helper download: HTTPS, checksum verification - None"

**Review:** HTTPS + checksum is defense-in-depth. TLS could have vulnerabilities, but checksum is the real guarantee.

**Concern:** If Go's HTTP client has a certificate validation bug, MITM could serve malicious content. However, checksum verification catches this.

**Verdict:** Correctly justified. Checksum is the ultimate verification.

### 4.4 Missing "Not Applicable" That Should Be Addressed

**Missing Item:** macOS Gatekeeper/notarization

The document mentions "macOS code signing may be needed for good UX" but doesn't address the security implications of running unsigned binaries on macOS.

**Risk:** macOS Gatekeeper will flag the helper binary. Users may:
- Right-click and "Open" to bypass (trained bad behavior)
- Disable Gatekeeper system-wide (security regression)
- `xattr -d com.apple.quarantine` the binary (trained bad behavior)

**Recommendation:**
- Add to Security Considerations section
- Plan for Apple Developer ID signing before macOS release
- Document temporary workaround and its implications

---

## 5. Additional Recommendations

### 5.1 Defense-in-Depth Additions

1. **Seccomp (Linux)**: The helper binary could use seccomp to restrict syscalls it can make. This limits what malicious library code can do.

2. **Sandbox-exec (macOS)**: Consider using macOS sandbox profile to restrict helper's capabilities.

3. **Separate user (advanced)**: For high-security environments, allow running helper as separate user via sudo or capabilities.

### 5.2 Logging and Auditability

1. **Log helper invocations**: Record which libraries were tested and when
2. **Capture stderr**: Library initialization might print warnings/errors to stderr that aid debugging
3. **Record timing**: Log how long each library took to load (helps identify slow/suspicious libraries)

### 5.3 Error Message Security

1. **Sanitize dlerror() output**: Library paths in error messages could leak sensitive path information
2. **Rate limit helper failures**: Prevent using verification as an oracle for probing what libraries exist

---

## 6. Summary of Findings

### Attack Vectors Requiring Action

| Vector | Severity | Recommendation |
|--------|----------|----------------|
| TOCTOU on helper binary | Medium | Use fexecve or document trust assumption |
| Environment variable injection | Medium | Sanitize LD_PRELOAD, LD_AUDIT, etc. |
| Path traversal in library paths | Medium | Canonicalize and validate paths |
| Resource exhaustion | Low-Medium | Consider cgroups/rlimits |
| Signal handling races | Low | Reset handlers between loads |
| Helper binary hardening | Low | Build with PIE, stack protector |

### Mitigations Needing Clarification

| Mitigation | Status | Recommendation |
|------------|--------|----------------|
| Process isolation | Adequate | Clarify doesn't mean sandboxed |
| Timeout protection | Adequate | Document forked processes not killed |
| Checksum verification | Adequate | Consider GPG signing for future |
| User opt-out | Adequate | Consider opt-in for Level 3 |

### Residual Risks to Escalate

| Risk | Priority | Action Required |
|------|----------|-----------------|
| 5-second code execution window | CRITICAL | Warn users, consider opt-in |
| Supply chain attack amplification | HIGH | Clarify "verify" doesn't mean "safe" |
| GitHub Actions single point of trust | MEDIUM | Document or add multi-party signing |

---

## 7. Conclusion

The design demonstrates thoughtful security consideration and addresses the most obvious attack vectors. The embedded checksum pattern, process isolation, and user opt-out are sound mitigations.

However, the fundamental tension remains: **Level 3 verification provides value by executing code, but that same execution creates risk.** Users expecting "verification" to be a safety check may be surprised that it runs potentially untrusted code.

The primary recommendation is to ensure users understand that Level 3 is a **functional test**, not a **security check**, and to consider making it opt-in rather than opt-out.

Secondary recommendations focus on hardening (environment sanitization, path validation) and defense-in-depth (seccomp, build flags).

The design is acceptable for implementation with the recommended clarifications and the understanding that Level 3 represents a deliberate trade-off between verification completeness and code execution risk.
