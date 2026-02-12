# Security Analysis: Disambiguation Design Document

## Executive Summary

The Disambiguation design addresses a real security gap (silent package selection across registries) with reasonable mitigations for the identified risks. However, the analysis reveals **three attack vectors not fully addressed**, **one mitigation with significant gaps**, and **one area of residual risk** that warrants documentation rather than escalation.

**Recommendation**: The design is acceptable for implementation with modifications to address the gaps identified below.

---

## 1. Attack Vectors Analysis

### 1.1 Documented Attack Vectors (Covered by Design)

| Attack Vector | Mitigation in Design | Assessment |
|---------------|---------------------|------------|
| Typosquatting | Edit-distance <=2 against registry entries | Adequate for common typos |
| Wrong package (silent selection) | 10x threshold, interactive prompt, `--from` override | Good layered defense |
| Batch pipeline hijacking | Deterministic selection + tracking for review | Reasonable given constraints |

### 1.2 Attack Vectors NOT Addressed

#### A. Popularity Gaming (CRITICAL GAP)

**Attack**: An attacker registers a package with the same name as a popular tool on a higher-priority ecosystem, then artificially inflates download counts to exceed the 10x threshold.

**Example**: Attacker registers `prettier` on crates.io, uses CI scripts to download it 50,000 times per week. The 10x threshold is met, the package is auto-selected over npm's legitimate prettier.

**Current mitigation gap**: The design relies on popularity thresholds for auto-selection but doesn't address artificial inflation. The quality filtering design (DESIGN-probe-quality-filtering.md) notes this explicitly: "A motivated attacker could farm ~100 downloads for under $50, passing our thresholds."

**Impact**: HIGH. Auto-selection without user interaction means the attack succeeds silently.

**Recommendation**:
1. Add version count as a secondary signal for auto-selection (not just for quality filtering). A package with 10x downloads but only 2 versions should not auto-select.
2. Consider requiring repository presence for auto-selection (squatters rarely link to legitimate repos).
3. Document this as a known limitation in the design's security section.

#### B. Priority Order Exploitation (MODERATE GAP)

**Attack**: When popularity data is unavailable (common for PyPI and Go), the fallback to static ecosystem priority can be exploited. An attacker registers a package in a higher-priority ecosystem (e.g., crates.io at priority 2) knowing it will beat the legitimate package in a lower-priority ecosystem (e.g., PyPI at priority 3).

**Current mitigation**: Quality filtering rejects packages with low version counts, but threshold is permissive (>=5 versions for crates.io).

**Impact**: MODERATE. The quality filter catches obvious squatters, but a patient attacker can create 5 empty versions over time.

**Recommendation**:
1. When falling back to priority ordering, always prompt the user (never auto-select).
2. Alternative: Add "priority fallback" as a selection reason that triggers review in batch mode.

#### C. Registry Entry Poisoning via Typosquat (MODERATE GAP)

**Attack**: An attacker registers `ripgerp` (transposition typo) which passes the distance-2 check against `ripgrep`, but the warning only appears if the user types `ripgerp`. If the user types `ripgrep` and an attacker has registered `ripgrep` on a different ecosystem, the typosquat check doesn't fire (exact name match).

**Current mitigation**: Edit-distance checking only catches typos in user input, not deliberate same-name registration across ecosystems.

**Impact**: MODERATE. This is actually covered by the disambiguation prompt, but the design doesn't connect these two mechanisms.

**Recommendation**: Clarify in the design that edit-distance checking addresses typos in user input, while disambiguation handles cross-ecosystem name collisions. These are complementary defenses.

---

## 2. Mitigation Assessment

### 2.1 Typosquatting Detection via Edit Distance

**Status**: ADEQUATE with caveats

**Strengths**:
- Distance <=2 is research-backed (catches 45% of historical typosquats)
- Checks against the registry (~500 popular tools) which are the actual typosquatting targets
- Warning shows both packages with popularity data

**Gaps**:
- Short package names (e.g., "go", "fd", "rg") have many legitimate neighbors at distance <=2
- The design acknowledges this uncertainty but doesn't specify how to handle it

**Recommendation**: Add package length awareness. For names <=3 characters, consider:
- Reducing threshold to distance <=1, OR
- Always prompting (never auto-select), OR
- Adding to the registry as explicit disambiguation entries

### 2.2 10x Popularity Threshold for Auto-Select

**Status**: PARTIALLY ADEQUATE

**Strengths**:
- Based on LLM discovery precedent
- Provides reasonable automation for clear cases
- Fallback to prompt when data is unavailable

**Gaps**:
- Threshold can be gamed with download inflation
- Cross-ecosystem comparisons are apples-to-oranges (npm weekly vs crates.io 90-day)
- Design acknowledges this as an uncertainty but proposes no mitigation

**Recommendation**:
1. Add secondary signals (version count, repository presence) as requirements for auto-select.
2. Consider ecosystem-normalized thresholds rather than raw counts.
3. Track auto-select decisions in telemetry to detect anomalies.

### 2.3 Non-Interactive Mode Error Handling

**Status**: ADEQUATE

The design correctly errors on ambiguity rather than silently selecting. This is the right security posture for CI/CD pipelines.

### 2.4 Batch Pipeline Tracking

**Status**: ADEQUATE with monitoring dependency

**Strengths**:
- Deterministic selection prevents flaky behavior
- Tracking enables human review
- `disambiguations.json` can be updated when issues found

**Gaps**:
- Effectiveness depends on actual human review of tracked selections
- No automation to detect suspicious patterns

**Recommendation**: Add batch metrics dashboard showing:
- Selection reasons distribution
- Packages selected with close popularity ratios
- First-time disambiguations (new since last run)

---

## 3. Security Section Review

### 3.1 Download Verification

**Stated**: "Disambiguation doesn't change download verification"

**Assessment**: CORRECT. Disambiguation is purely a selection mechanism. The selected package flows through the existing verification pipeline which includes:
- Checksum verification during download (Layer 1)
- Version verification post-install (Layer 2)
- Binary checksum pinning (Layer 3)
- Sandbox testing for recipe validation

### 3.2 Execution Isolation

**Stated**: "No change to execution isolation. Disambiguation is a selection mechanism, not an execution mechanism."

**Assessment**: CORRECT but incomplete. While disambiguation itself doesn't execute code, **misdirection has equivalent security impact to execution**. Selecting the wrong package leads to executing different code. The design should acknowledge this implicit trust boundary.

**Recommendation**: Add to security section: "While disambiguation doesn't execute code, misdirection leads to installing and executing unintended software. The defenses here are part of tsuku's supply chain security."

### 3.3 Supply Chain Risks

**Stated**: Three risks covered (typosquatting, wrong package, batch pipeline)

**Assessment**: PARTIALLY COMPLETE. The documented risks are valid, but the mitigations section should be more explicit about:
- What the design protects against (casual squatters, common typos, silent misdirection)
- What it doesn't protect against (well-resourced attackers with popularity gaming, compromised legitimate packages, MITM on registry APIs)

### 3.4 User Data Exposure

**Stated**: "No user data is accessed or transmitted"

**Assessment**: CORRECT for disambiguation logic itself. However, note that the ecosystem probe does transmit tool names to all registries in parallel. This is existing behavior documented in DESIGN-ecosystem-probe.md.

---

## 4. "Not Applicable" Justification Review

The design doesn't explicitly mark any standard security sections as "Not Applicable" - it addresses each one. This is appropriate.

However, the following security considerations are implicit and could be made explicit:

| Consideration | Status | Recommendation |
|---------------|--------|----------------|
| TLS/Transport Security | Implicit (ecosystem APIs use HTTPS) | Document that registry probes use HTTPS only |
| Rate Limiting | Not addressed | Add note about respecting registry rate limits |
| Input Validation | Partial (edit-distance check) | Document Unicode normalization before disambiguation |
| Logging/Audit | Partial (batch tracking) | Consider logging interactive selections too |

---

## 5. Residual Risk Assessment

### 5.1 Risks Requiring Documentation (Not Escalation)

| Risk | Severity | Justification for Acceptance |
|------|----------|------------------------------|
| Popularity gaming with artificial downloads | Medium | Quality filtering + version count provides some defense; sophisticated attackers can bypass any client-side check. Defense in depth via sandbox validation. |
| Short package name false positives | Low | Affects few tools; can add to registry as workaround. |
| Cross-ecosystem download count incomparability | Low | 10x threshold is conservative enough to absorb noise. |

### 5.2 Risks Requiring Additional Mitigation

| Risk | Recommendation | Priority |
|------|----------------|----------|
| Auto-select on priority fallback | Prompt instead of auto-select when no popularity data | High |
| Auto-select on gamed downloads | Add version count + repo presence as secondary requirements | Medium |
| Execution isolation framing | Add explicit note about misdirection = execution risk | Low (documentation) |

---

## 6. Comparison with Related Designs

### vs. DESIGN-ecosystem-probe.md

The ecosystem probe design explicitly states: "The primary risk is **name confusion**" and lists mitigations including curated registry, exact name matching, static priority ranking, and disambiguation prompting. The disambiguation design builds correctly on this foundation.

**Gap identified**: The ecosystem probe design mentions "Sandbox validation catches recipes with unexpected behavior" as defense in depth. The disambiguation design should reference this.

### vs. DESIGN-probe-quality-filtering.md

The quality filtering design acknowledges: "Download counts can be artificially inflated. A motivated attacker could farm ~100 downloads for under $50." The disambiguation design should reference this limitation rather than assuming download counts are authoritative.

### vs. DESIGN-discovery-resolver.md (parent)

The parent design states: "Filtering is noise reduction, not a security boundary." This philosophy is correct and should be echoed in the disambiguation design to set appropriate expectations.

---

## 7. Recommendations Summary

### Must Address Before Implementation

1. **Add secondary signals for auto-select**: Version count and repository presence should be required alongside the 10x download threshold.

2. **Prompt on priority fallback**: When popularity data is unavailable and selection uses static priority, prompt the user instead of auto-selecting.

3. **Document limitations explicitly**: The design should state what it protects against (casual squatters, typos) and what it doesn't (well-resourced attackers, registry compromise).

### Should Address (Enhancement)

4. **Short package name handling**: Add length-aware edit distance or explicit registry entries for short names.

5. **Batch metrics dashboard**: Track selection patterns to detect anomalies.

6. **Reference sandbox validation**: Add defense-in-depth note about sandbox catching malicious recipes.

### Nice to Have

7. **Ecosystem-normalized popularity comparison**: Consider normalizing download counts per ecosystem before comparison.

8. **Audit logging for interactive selections**: Track what users select for future analysis.

---

## 8. Conclusion

The disambiguation design addresses a genuine security gap with a layered defense approach. The core decision (10x threshold for auto-select, prompt for close matches, error in non-interactive mode) is sound.

However, the design assumes download counts are trustworthy signals, which is a weak assumption for a security-critical decision. The recommendations above add secondary signals to reduce reliance on easily-gamed popularity metrics.

**Overall assessment**: APPROVE WITH MODIFICATIONS. Implement recommendations #1-3 before shipping. Track recommendations #4-6 as follow-up issues.
