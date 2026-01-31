# Security Review: Deterministic-Only Recipe Generation

## Executive Summary

The security analysis in the design document is **fundamentally flawed**. While the immediate change (adding a `--deterministic-only` flag and exit code) appears benign, the justifications miss critical security implications for a package manager. The "not applicable" dismissals are incorrect or incomplete.

**Verdict**: INSUFFICIENT - Requires substantial revision before implementation.

---

## Detailed Analysis

### 1. Download Verification: PARTIALLY APPLICABLE

**Design claim**: "Not applicable. This change affects error handling and control flow, not how artifacts are downloaded or verified."

**Reality**: The flag fundamentally changes the *reliability* of recipe generation, which directly impacts download verification workflows.

#### Attack Vectors Not Considered

1. **Timing-based cache poisoning**
   - If deterministic generation fails (exit 3), users may retry later
   - During retry window, attacker could poison upstream sources (Homebrew formula, GHCR manifests)
   - Without clear guidance on retry safety, users may unknowingly download compromised artifacts

2. **Error message exploitation**
   - Design mentions "formula names" and "failure categories" in stderr
   - What if error messages inadvertently leak partial artifact URLs or checksums?
   - These could be used to fingerprint vulnerable tsuku versions or identify generation strategies to bypass

3. **Fallback behavior confusion**
   - Without `--deterministic-only`: Fails deterministic → tries LLM → succeeds with unverified recipe
   - With `--deterministic-only`: Fails deterministic → exits immediately
   - If users don't understand this distinction, they may disable the flag to "make it work", unknowingly accepting LLM-generated recipes that lack the same verification guarantees

#### Mitigations Needed

- **Retry guidance**: Document safe retry intervals and how to verify upstream sources haven't been compromised
- **Error message sanitization**: Audit stderr output to ensure no partial artifact URLs, checksums, or internal paths leak
- **Flag education**: Clearly document that `--deterministic-only` provides *stronger* guarantees, not just "faster failures"

---

### 2. Execution Isolation: MISLEADING

**Design claim**: "Not applicable. No new code execution paths are introduced."

**Reality**: The flag changes *when* code execution happens, which has isolation implications.

#### Attack Vectors Not Considered

1. **Deferred execution risks**
   - Without flag: Recipe generated (possibly via LLM) → installed → binary executed
   - With flag: Recipe generation fails → user retries later → different recipe generated → different binary executed
   - Time-of-check vs time-of-use (TOCTOU) vulnerability: The deterministic generation that "succeeds" in retry #5 may not be the same artifact available at generation time for retries #1-4

2. **Error handling as side channel**
   - Exit code 3 signals "deterministic failure" to calling processes
   - Malicious wrappers could detect this and substitute their own recipes
   - Without execution isolation at the CLI boundary, this becomes a privilege escalation vector

#### Mitigations Needed

- **TOCTOU protection**: Document that recipes should be versioned/timestamped and re-verified before installation
- **Secure defaults**: Consider making `--deterministic-only` the *default*, requiring explicit opt-in to LLM fallback
- **Exit code documentation**: Warn that exit code 3 should not trigger automatic fallback logic in wrapper scripts

---

### 3. Supply Chain Risks: CRITICALLY INCOMPLETE

**Design claim**: "Not applicable. This change doesn't alter where artifacts come from or how they're verified."

**Reality**: The flag fundamentally changes the *trust model* for recipe generation, which is the entry point to the supply chain.

#### Attack Vectors Not Considered

1. **Deterministic generation dependency on GHCR**
   - Design mentions "SHA256 against GHCR manifest digests"
   - What if GHCR is compromised? What if the manifest itself is poisoned?
   - The "deterministic" label creates false confidence that these are immutable

2. **LLM fallback as supply chain bypass**
   - Standard path: Deterministic generation with GHCR verification
   - Fallback path: LLM generates recipe (what verification happens here?)
   - By *preventing* LLM fallback, the flag exposes a critical question: Are LLM-generated recipes subject to the same verification as deterministic ones?
   - If no: The default (non-`--deterministic-only`) behavior is a massive supply chain hole
   - If yes: Why is this not documented in the security section?

3. **Recipe generation as attack surface**
   - Tsuku downloads and executes binaries based on recipes
   - Recipe generation is therefore the highest-value attack target
   - Any compromise here (deterministic or LLM) means arbitrary code execution on user machines
   - The design doesn't address how recipe integrity is maintained between generation and installation

#### Mitigations Needed

- **GHCR trust boundaries**: Document assumptions about GHCR integrity and what happens if it's compromised
- **LLM verification parity**: Explicitly state whether LLM-generated recipes undergo the same verification as deterministic ones
- **Recipe signing**: Consider signing generated recipes to prevent tampering between generation and installation
- **Transparency**: Provide a way for users to audit/diff recipes before installation (especially LLM-generated ones)

---

### 4. User Data Exposure: INSUFFICIENT ANALYSIS

**Design claim**: "Not applicable. The `--deterministic-only` flag prevents LLM API calls, which means no formula data is sent to external LLM providers."

**Reality**: While the flag *prevents* data leakage, the analysis doesn't address residual risks.

#### Attack Vectors Not Considered

1. **Error messages as information disclosure**
   - "Formula names (public Homebrew data)" - but what if users generate recipes for private/internal tools?
   - "Failure categories (enum values)" - could these leak information about tsuku's internals useful for exploit development?

2. **Local data persistence**
   - Where are failed generation attempts logged?
   - Do logs contain full formulas or just names?
   - Are logs sanitized before being written to disk?

3. **Default behavior data exposure**
   - Without `--deterministic-only`, formula data IS sent to LLM providers
   - How is user consent obtained?
   - Is there a privacy policy covering this?
   - Are users warned that default behavior sends data externally?

#### Mitigations Needed

- **Error message review**: Audit all stderr output for potential information disclosure
- **Log sanitization**: Ensure no sensitive formula data persists in logs
- **Privacy controls**: Make data exposure opt-in (i.e., LLM fallback requires explicit consent)
- **Documentation**: Clearly state what data leaves the user's machine under what circumstances

---

## New Security Considerations

### 5. Availability and Denial of Service

**Missing from design**: How does `--deterministic-only` affect availability?

#### Attack Vectors

1. **Targeted DoS via deterministic failures**
   - Attacker identifies formulas that deterministic generation struggles with
   - Encourages users to generate recipes for these (e.g., via fake tutorials)
   - Users hit exit code 3 repeatedly, blame tsuku, abandon tool

2. **Dependency on external services**
   - Deterministic generation depends on GHCR availability
   - If GHCR is down, all `--deterministic-only` requests fail
   - This could be weaponized: DDoS GHCR → disable tsuku for security-conscious users

#### Mitigations Needed

- **Fallback strategy**: Document recommended fallback when deterministic generation is unavailable
- **Caching**: Consider caching GHCR manifests to reduce dependency on external availability
- **Monitoring**: Provide visibility into deterministic generation success rates

---

### 6. Downgrade Attacks

**Missing from design**: Can the flag be bypassed?

#### Attack Vectors

1. **Environment variable override**
   - If `--deterministic-only` can be overridden by environment variables, attackers can disable it
   - Malicious installers could set `TSUKU_ALLOW_LLM=1` to bypass the protection

2. **Config file manipulation**
   - If tsuku has a config file, can it override CLI flags?
   - Malicious software could edit config to disable deterministic-only mode

#### Mitigations Needed

- **Flag precedence**: Document and enforce that CLI flags take precedence over env vars and config
- **Immutable mode**: Consider a "paranoid mode" where LLM fallback is permanently disabled at build time

---

## Risk Matrix

| Risk Category | Severity | Likelihood | Current Mitigation | Recommended Action |
|--------------|----------|------------|-------------------|-------------------|
| GHCR compromise | **Critical** | Low | SHA256 verification | Add manifest signing |
| LLM recipe tampering | **Critical** | Medium | None documented | Document verification parity |
| TOCTOU in retries | **High** | Medium | None | Add recipe versioning |
| Error message leakage | **Medium** | Medium | None | Audit stderr output |
| Data exposure (default) | **High** | High | Opt-out only | Make opt-in |
| Availability (GHCR down) | **Medium** | Low | None | Add caching layer |
| Config-based bypass | **Medium** | Low | Unknown | Document precedence |

---

## Residual Risks to Escalate

1. **LLM-generated recipe verification gap**: If LLM fallback is available by default and lacks equal verification to deterministic generation, this is a critical supply chain hole that cannot be fixed by this design alone.

2. **Upstream source trust**: The entire security model assumes GHCR and Homebrew are trustworthy. If they're compromised, `--deterministic-only` provides no additional protection. This should be escalated as a systemic risk.

3. **User education failure**: If users don't understand that disabling `--deterministic-only` weakens security, they'll do it anyway. The design needs a strategy for making the secure path the easy path.

---

## Recommendations

### Immediate (Block Implementation)

1. **Rewrite security section**: Remove all "not applicable" justifications and address each category properly
2. **Add LLM verification documentation**: Explicitly state how LLM-generated recipes are verified
3. **Document GHCR trust model**: State assumptions and what happens if violated

### Short-term (Before Release)

1. **Audit error messages**: Ensure no information disclosure in stderr
2. **Add privacy controls**: Make LLM fallback opt-in, not opt-out
3. **Document retry safety**: Provide guidance on safe retry intervals

### Long-term (Future Work)

1. **Recipe signing**: Add cryptographic signatures to generated recipes
2. **Transparency mode**: Allow users to review recipes before installation
3. **Secure defaults**: Make `--deterministic-only` the default behavior

---

## Conclusion

The design's security analysis is dangerously dismissive. For a tool that downloads and executes binaries, **every change has security implications**. The "not applicable" justifications conflate "this PR doesn't touch those code paths" with "this change has no security impact", which is incorrect.

The design should not proceed until:
1. All "not applicable" sections are rewritten with actual analysis
2. LLM recipe verification is documented
3. User data exposure is made opt-in
4. A threat model specific to recipe generation is developed

**Current security posture: INSUFFICIENT for production deployment.**
