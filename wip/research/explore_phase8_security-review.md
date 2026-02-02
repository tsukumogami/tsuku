# Security Review: Probe Quality Filtering

**Design:** DESIGN-probe-quality-filtering.md
**Reviewer:** Security Analysis Agent
**Date:** 2026-02-01
**Context:** Tsuku downloads and executes binaries from the internet, requiring analysis across four dimensions: download verification, execution isolation, supply chain risks, and user data exposure.

## Executive Summary

The design introduces quality filtering to prevent tsuku from resolving tool names to squatter packages. The security analysis section correctly identifies this as a supply chain risk mitigation but is incomplete. Critical attack vectors and residual risks are not addressed. The analysis below identifies 8 significant security concerns, 3 of which represent exploitable attack vectors.

**Risk Level:** MEDIUM - The design improves security posture but introduces new attack surfaces that require mitigation.

## Detailed Analysis

### 1. Attack Vectors Not Considered

#### 1.1 Threshold Gaming via Coordinated Install Campaigns

**Severity:** HIGH
**Attack Vector:** A sophisticated attacker can bypass quality filters by artificially inflating metrics.

**Scenario:**
- Attacker publishes `httpie` squatter on crates.io with malicious payload
- Launches coordinated campaign to generate 100+ downloads (threshold: 100)
- Publishes 5+ versions with trivial changes (threshold: 5)
- Package now passes quality filter and resolves ahead of legitimate PyPI package due to priority ordering (crates.io priority 2, PyPI priority 3)

**Feasibility:** HIGH. Generating 100 downloads and 5 versions requires minimal resources:
- 100 installs from distributed VMs or residential proxies: ~$5-10
- Publishing 5 versions: free, automated via CI
- Total campaign cost: <$50, one-time investment
- Payoff: persistent installation vector for users typing `tsuku install httpie`

**Current Mitigation:** None. The design acknowledges "A well-crafted squatter with artificially inflated downloads could still pass the filter" but treats this as acceptable residual risk without quantifying likelihood or impact.

**Recommended Mitigation:**
- Add velocity checks: reject packages where 90%+ of downloads occurred in the last 7 days
- Cross-reference repository activity: reject if GitHub repo has <10 stars or was created in the last 30 days
- Add registry priority penalty for packages that barely exceed thresholds (e.g., 101 downloads = suspicious)

#### 1.2 Time-of-Check Time-of-Use (TOCTOU) Window

**Severity:** MEDIUM
**Attack Vector:** Package quality can change between probe time and install time.

**Scenario:**
- Probe queries npm at T0: package has 10,000 downloads, passes filter
- User runs `tsuku install <package>` at T0 + 5 minutes
- Install command fetches recipe/version at T0 + 5 minutes
- Between T0 and T0+5m, attacker publishes new malicious version
- No re-validation of quality metrics at install time

**Feasibility:** MEDIUM. Requires attacker to:
1. Compromise existing legitimate package (via stolen credentials, malicious maintainer)
2. Time the malicious version publish for the TOCTOU window
3. Hope user installs within window before detection

**Current Mitigation:** None. The design does not specify whether quality checks are repeated at install time or cached from probe time.

**Recommended Mitigation:**
- Cache probe results with TTL (e.g., 1 hour)
- Re-validate quality metrics at install time if cache is stale
- Add metric regression detection: fail install if downloads dropped >50% since probe

#### 1.3 Metadata Injection via Compromised Registry APIs

**Severity:** MEDIUM
**Attack Vector:** Attacker gains temporary control of registry API and returns inflated metrics.

**Scenario:**
- Attacker compromises npm download stats API (api.npmjs.org)
- API returns fake download counts for attacker's squatter package
- Probe accepts inflated metrics, package passes filter
- Users install malicious package based on false quality signal

**Feasibility:** LOW-MEDIUM. Requires compromise of registry infrastructure, which is difficult but not impossible:
- DNS hijacking of api.npmjs.org (temporary, detectable)
- BGP hijacking for MitM (sophisticated, nation-state level)
- Insider threat or stolen API keys (if registry has internal admin APIs)

**Current Mitigation:** None. The design assumes registry APIs are trustworthy.

**Recommended Mitigation:**
- Add anomaly detection: flag packages where reported downloads exceed GitHub stars by >1000x
- Add cross-registry validation: if package exists on multiple registries, compare metrics for consistency
- Log raw API responses for forensic analysis

### 2. Mitigation Sufficiency Assessment

#### 2.1 Supply Chain Risk Mitigation: Incomplete

The design correctly identifies name-squatting as a supply chain risk and proposes quality filtering as mitigation. However:

**Gaps:**
- **No verification of repository authenticity**: `HasRepository` checks existence, not ownership. Attacker can point to a legitimate repo they don't control.
- **No maintainer reputation scoring**: A package with 1000 downloads from a brand-new maintainer account is riskier than 100 downloads from a 5-year-old account.
- **No ecosystem cross-check**: Doesn't verify that the package on Registry A is related to the tool users expect. `prettier` on crates.io could have a GitHub repo at `fake-org/prettier` that has nothing to do with the real prettier.

**Recommended Enhancements:**
- Parse repository URL and verify it matches known canonical repos for popular tools (e.g., `prettier` must resolve to `prettier/prettier` on GitHub)
- Add maintainer tenure check: require account age >90 days for packages near threshold
- Add description/README similarity check: reject if description is <10 words or contains spam patterns

#### 2.2 User Data Exposure: Underassessed

The analysis states "No user-identifying information is transmitted beyond what the existing Probe() calls already send (package name + IP address)."

**Issue:** This understates the privacy implication:
- The npm downloads API call reveals user intent (what tool they're trying to install) to npm, even if they ultimately install from a different registry
- IP address + timestamp + package name = user behavior tracking across registries
- Example: User types `tsuku install prettier`, npm sees the query even though the package ultimately comes from crates.io (in the buggy current state) or npm (after this fix)

**Is this acceptable?** Probably yes, but should be documented:
- Probe queries already leak intent to all 7 registries in parallel
- This design doesn't make it worse; it adds one more query to npm
- Users who care about privacy can use `TSUKU_TELEMETRY=0` (though that doesn't disable probe queries)

**Recommended Enhancement:**
- Document in privacy policy that tool name queries are sent to multiple registries
- Consider adding a privacy mode that disables ecosystem probing and relies only on embedded/LLM discovery

### 3. Residual Risks Requiring Escalation

#### 3.1 Threshold Gaming (HIGH)

As detailed in 1.1, the threshold values are low enough that coordinated gaming is economically feasible. This is a **strategic risk** that requires escalation:

- **Likelihood:** LOW-MEDIUM (requires attacker motivation + resources)
- **Impact:** HIGH (users execute malicious code)
- **Mitigation cost:** MEDIUM (requires additional API calls, repo checks, velocity analysis)

**Escalation Recommendation:** Accept risk for v1, add telemetry to detect anomalies (e.g., track install attempts for packages that barely pass thresholds), plan enhanced validation for v2.

#### 3.2 Priority Ordering Interaction (MEDIUM)

The design states "Changing the static priority ranking system is out of scope." This is a mistake. The quality filter + priority ranking interaction creates a combined attack surface:

- Attacker targets high-priority registries (crates.io, npm) with quality-passing squatters
- Legitimate package on lower-priority registry gets shadowed even if it has better quality metrics
- Example: Real `httpie` on PyPI has 10M downloads; squatter on crates.io has 101 downloads. Squatter wins due to priority.

**Current Mitigation:** The quality filter prevents obvious squatters, but any squatter that passes the filter benefits from priority-based shadowing.

**Escalation Recommendation:** Link this design to a future design that revisits priority ordering. Propose "quality-weighted priority" where a low-priority match with 10000x better metrics can override a high-priority match.

#### 3.3 npm Latency Attack (LOW)

The design adds a parallel HTTP call to npm's downloads API. An attacker who controls network infrastructure could:

- Delay the downloads API response to timeout
- Force the npm probe to rely on version count alone (weaker signal)
- Make npm probes less reliable, biasing results toward other registries

**Likelihood:** LOW (requires network-level compromise)
**Impact:** LOW (degrades to version-count-only filtering, which is still better than nothing)

**Escalation Recommendation:** Accept risk. The design already handles timeout gracefully (falls back to version count).

### 4. "Not Applicable" Justification Review

#### 4.1 Download Verification: Correctly N/A

> "Not applicable. This design doesn't change how binaries are downloaded or verified. It only affects which registry a tool name resolves to."

**Assessment:** CORRECT. This design is about discovery, not installation. No changes to checksum validation, signature verification, or download process.

#### 4.2 Execution Isolation: Incorrectly N/A

> "Not applicable. No new code execution paths are introduced. The quality filter is a pure data comparison."

**Assessment:** PARTIALLY INCORRECT. While the quality filter itself is pure data comparison, the design's outcome directly affects what gets executed:

- If the filter fails and allows a squatter through, malicious code executes
- "Pure data comparison" is true, but the security relevance is not about the filter's implementation—it's about whether the filter correctly prevents malicious package selection

**Recommended Revision:** Change "Not applicable" to "Relevant" with text:
> "The quality filter does not introduce new execution contexts, but its correctness directly determines whether tsuku directs users to install malicious packages. Filter bypass = code execution. This design's security effectiveness should be measured by false positive rate (legitimate packages blocked) and false negative rate (squatters accepted)."

### 5. Sophisticated Attacker Threshold Gaming Analysis

#### 5.1 Can an Attacker Beat the Filter?

**Short Answer:** Yes, with moderate effort.

**Attack Playbook:**
1. **Target selection:** Choose a popular tool name (e.g., `httpie`, `prettier`) on a high-priority registry (crates.io, npm)
2. **Metric farming:**
   - Downloads: Launch 100-200 installs from distributed IPs (cost: $10-20)
   - Versions: Publish 5-10 versions over 2-3 weeks (free, looks organic)
   - Repository: Create GitHub repo with stolen/AI-generated README (free)
3. **Timing:** Wait 30 days after first publish to avoid "too new" detection (if added)
4. **Payload:** Include malicious code in installation script or binary
5. **Activation:** Package now resolves for `tsuku install <tool>`, executes malicious payload

**Cost:** <$50
**Effort:** Low-Medium (mostly automated)
**Detection Likelihood:** Low (unless tsuku adds anomaly detection)

#### 5.2 Defense Against Sophisticated Attackers

The design's thresholds are calibrated to block **casual squatters** (no effort, zero downloads). They are not calibrated to block **motivated attackers** (small effort, minimal cost).

**Recommendations to Harden:**

1. **Add canonical package registry:**
   - Maintain a mapping of well-known tools → expected registry + repository
   - Example: `prettier → npm + github.com/prettier/prettier`
   - For tools in this list, ignore ecosystem probe entirely or use it only as fallback
   - Covers top 100-200 tools, where squatting is most valuable to attackers

2. **Add repository verification:**
   - Parse `HasRepository` URL and fetch GitHub repo metadata
   - Check: stars >100, not archived, last commit <6 months
   - Reject packages where reported downloads >> GitHub stars (e.g., 1000 downloads but 2 stars = suspicious)

3. **Add registry consensus:**
   - If a package name exists on multiple registries, compare metadata
   - If one registry reports 10M downloads and another reports 100, flag the outlier

4. **Add user warnings:**
   - If a package barely passes thresholds (e.g., 101 downloads, threshold 100), show warning: "This package has minimal adoption. Verify authenticity before installing."
   - If no repository link present, warn: "No source repository found. Proceed with caution."

### 6. Additional Security Considerations

#### 6.1 Logging and Forensics

The design does not specify logging requirements. Recommendations:

- Log all probe results (including rejected packages) to structured log file
- Include: tool name, registry, downloads, version count, timestamp, accepted/rejected, rejection reason
- Purpose: Post-incident forensics, anomaly detection, threshold tuning

#### 6.2 Fallback Behavior

What happens if all probes return packages that fail quality checks?

- **Current behavior (inferred):** All rejected, discovery resolver returns "not found"
- **Risk:** Legitimate new tools (e.g., a real package with 50 downloads) become uninstallable
- **Recommendation:** If all candidates fail quality checks, return the highest-quality failure with a warning instead of complete failure

#### 6.3 Quality Metric Manipulation via Package Bundling

Attacker could boost version count by:
- Publishing a legitimate package with many versions
- Adding malicious dependency that tsuku installs transitively
- The dependency itself might be low-quality, but the primary package passes quality checks

**Mitigation:** Out of scope for this design (requires dependency graph analysis), but worth noting as future work.

## Summary of Findings

| # | Finding | Severity | Current Status | Recommended Action |
|---|---------|----------|----------------|-------------------|
| 1 | Threshold gaming via coordinated campaigns | HIGH | Not addressed | Add velocity checks, repo verification |
| 2 | TOCTOU window between probe and install | MEDIUM | Not addressed | Cache with TTL, re-validate at install |
| 3 | Metadata injection via compromised API | MEDIUM | Not addressed | Add anomaly detection, log raw responses |
| 4 | No repository authenticity verification | MEDIUM | Acknowledged | Add canonical package mapping for top tools |
| 5 | User data exposure understated | LOW | Acknowledged | Document in privacy policy |
| 6 | Priority ordering + quality filter interaction | MEDIUM | Out of scope | Link to future priority-rebalancing design |
| 7 | npm latency attack vector | LOW | Mitigated | Accept risk (falls back to version count) |
| 8 | "Not applicable" for execution isolation | LOW | Incorrect framing | Revise to acknowledge relevance |

## Final Recommendations

### Must Fix Before Implementation
1. Add logging for all probe results (accepted + rejected)
2. Add TOCTOU mitigation (cache probe results with TTL, re-validate at install if stale)
3. Revise "Execution Isolation" security consideration to acknowledge relevance

### Should Fix in v1
4. Add canonical package registry for top 50-100 tools (hardcoded mapping)
5. Add repository URL verification (fetch GitHub metadata, check stars/activity)
6. Document user data exposure in privacy policy

### Can Defer to v2
7. Add velocity checks for download count (reject if 90% of downloads in last 7 days)
8. Add cross-registry consensus validation
9. Add maintainer reputation scoring
10. Link to priority-rebalancing design to address shadowing risk

### Accept as Residual Risk
11. Sophisticated attackers can game thresholds with moderate effort (<$50) - Accept for v1, monitor via telemetry
12. npm latency attack can degrade npm filtering to version-only - Already mitigated by fallback behavior

## Conclusion

The design materially improves tsuku's security posture by blocking casual name-squatting. However, it is vulnerable to threshold gaming by motivated attackers. The thresholds are set low enough that a $50 campaign could bypass them.

For a v1 release, this is acceptable if:
1. Telemetry is added to detect anomalies (packages that barely pass thresholds)
2. A canonical package mapping is added for the most commonly installed tools (top 50-100)
3. TOCTOU mitigation is implemented to prevent time-of-check/time-of-use attacks

For production hardening (v2+), additional validation layers (repository verification, velocity checks, maintainer reputation) should be added to raise the cost of successful squatter campaigns from $50 to $5000+.
