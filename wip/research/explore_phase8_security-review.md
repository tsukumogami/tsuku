# Security Review: DESIGN-discovery-resolver.md

## Overview

This review examines the security posture of the three-stage discovery resolver (embedded registry, parallel ecosystem probe, LLM fallback) that maps tool names to `{builder, source}` pairs for installation.

## 1. Attack Vectors Not Considered

### 1.1 Registry Override Poisoning via update-registry

The design states `tsuku update-registry` can override the embedded registry with a fetched version. The integrity mechanism is listed as "TBD: checksum or signature." This is a critical gap. If the sync channel uses plain HTTPS without content signing, a network-level attacker (compromised CDN, corporate MITM proxy, DNS hijack) can replace the entire registry. The embedded registry's signed-binary integrity guarantee is nullified the moment a user runs `update-registry`.

**Recommendation**: Block this design from shipping without a concrete integrity scheme. Content-addressable hashes pinned in the binary (hash of the registry at embed time) with a threshold -- the synced registry must be signed by a key embedded in the binary, or the binary must ship with a hash of the expected signing key.

### 1.2 Ecosystem Account Takeover (Name Squatting on Abandoned Packages)

The age threshold (>90 days) actually helps attackers who plan ahead. A patient attacker can register `stripe-cli` on PyPI, wait 90 days, and accumulate downloads via dependency confusion or CI bots. The download threshold of 1000/month is achievable through automated means. The design's thresholds filter out naive typosquatting but not deliberate, patient supply-chain attacks.

**Recommendation**: Document this as accepted residual risk. The registry override for the top 500 tools is the real defense. For ecosystem-only tools, the disambiguation UX showing multiple results is the primary mitigation.

### 1.3 Homoglyph and Unicode Tool Names

The design doesn't address homoglyph attacks where tool names use visually similar characters (e.g., Cyrillic 'а' vs Latin 'a'). Ecosystem registries have varying levels of protection against this. A user typing a tool name from a web page with hidden Unicode could trigger discovery of a malicious package.

**Recommendation**: Normalize tool names to ASCII before querying. Reject or warn on non-ASCII input.

### 1.4 Time-of-Check-to-Time-of-Use (TOCTOU) on Ecosystem Results

Discovery resolves `{builder, source}` at probe time, but the actual download happens later in the build pipeline. Between resolution and download, a package could be compromised (maintainer account takeover, new malicious version published). This is inherent to any package manager but worth noting because discovery adds a conceptual gap between "user decided to install X" and "X is actually fetched."

**Recommendation**: This is standard package manager risk. No additional mitigation needed beyond existing builder-level verification.

### 1.5 LLM Tool Use Exploitation

The LLM has a web search tool. If the LLM can be manipulated into searching for attacker-controlled queries, the search results themselves become an injection vector. The design mentions HTML stripping but doesn't address the case where the LLM's web search tool returns results that contain structured data designed to manipulate the LLM's reasoning.

**Recommendation**: The LLM should be given a constrained system prompt that instructs it to return only a `{builder, source}` pair. Post-process the LLM output with strict parsing -- don't allow freeform URLs, only patterns matching known builder source formats (e.g., `owner/repo` for GitHub). Reject any output that doesn't match expected patterns.

## 2. Mitigation Sufficiency

### 2.1 Ecosystem Probe Filtering (Threshold-Based)

The 90-day age + 1000 downloads/month thresholds are a reasonable starting point but provide weak security guarantees:
- Downloads are gameable
- Age only stops impatient attackers
- No verification that the package actually produces a binary tool (vs. a library)

The mitigations are sufficient for **reducing noise** but insufficient as a **security boundary**. The real security comes from the disambiguation UX and sandbox validation.

### 2.2 LLM Prompt Injection Defenses

The listed defenses (HTML stripping, URL validation, user confirmation) are necessary but incomplete:
- HTML stripping doesn't catch injection in plain text content, markdown, or JavaScript-rendered pages
- URL validation patterns aren't specified
- User confirmation is the strongest defense but users develop "click fatigue"

**Gap**: No mention of constraining the LLM's system prompt to limit output format, no mention of output validation beyond URL pattern matching.

### 2.3 Sandbox Validation

Mentioned as "defense in depth" but not detailed. The effectiveness depends entirely on what the sandbox checks. If the sandbox only verifies the recipe builds successfully, a malicious binary that behaves normally during build but exfiltrates data at runtime would pass.

**Note**: This is out of scope for this design (sandbox is existing infrastructure), but the security section should not lean on it without clarifying what it actually catches.

## 3. Residual Risk Escalation

The following residual risks warrant explicit acknowledgment in the design:

| Risk | Severity | Notes |
|------|----------|-------|
| update-registry without integrity verification | **High** | TBD is not acceptable for a supply chain component. Must be resolved before shipping. |
| Patient typosquatting bypassing thresholds | Medium | Accepted risk, mitigated by registry overrides for popular tools |
| LLM manipulation via search result poisoning | Medium | User confirmation is the backstop, but novel techniques evolve |
| Sandbox validation scope unclear | Medium | Design leans on sandbox without specifying what it catches |

The **update-registry integrity gap** is the only item I'd escalate as a blocker. Everything else is residual risk that can be documented and accepted.

## 4. "Not Applicable" Justifications

The design doesn't explicitly mark anything as N/A, but there are implicit omissions:

### 4.1 Authentication and Authorization

No mention of authenticating ecosystem API responses. Public API responses from PyPI, npm, crates.io are served over HTTPS, which provides transport security but not content authenticity. If an ecosystem registry is compromised, the probe will trust poisoned results.

**Assessment**: Acceptable. Tsuku is no worse off than pip, npm, or cargo themselves. This is ecosystem-level trust, not tsuku-specific.

### 4.2 Rate Limiting of Outbound Requests

The design mentions "API-side rate limiting" as residual risk but doesn't implement client-side rate limiting. An attacker who can trigger repeated `tsuku install` calls (e.g., in a CI pipeline) could use tsuku as an amplifier for API abuse against ecosystem registries.

**Assessment**: Low severity. Worth adding basic client-side rate limiting (e.g., debounce repeated probes for the same tool within a time window).

## 5. Trust Model for Discovery Registry

The trust model is partially defined but has gaps:

**What's clear:**
- The embedded registry is trusted because it ships with the signed binary
- Registry entries override ecosystem results (good -- prevents hijacking of known tools)
- ~500 entries scoped to GitHub-release tools and disambiguation overrides

**What's missing:**
- **Who can modify registry entries?** The design doesn't specify the review/approval process for registry changes. If a single maintainer can push a registry change, that's a single point of compromise.
- **How is the synced registry authenticated?** Marked as TBD. This is a critical gap (see section 1.1).
- **What happens when a registry entry's upstream repo changes ownership?** GitHub repos can be transferred. A registry entry pointing to `alice/tool` could be transferred to `mallory/tool` without the registry updating.
- **Freshness checks scope**: The design mentions "verifying repos still exist and aren't archived" but not ownership verification.

**Recommendation**: Add repo ownership verification to freshness checks. Pin the expected GitHub owner in registry entries and alert on ownership changes.

## 6. LLM Prompt Injection Defenses

### Current Defenses
1. Strip hidden HTML elements
2. Validate URLs against expected patterns
3. Require user confirmation
4. Sandbox validation as defense in depth

### Gaps

**HTML stripping is insufficient.** Prompt injection can be embedded in:
- Visible text that looks like normal content to humans but steers the LLM
- SEO-optimized pages that rank highly for tool names
- GitHub README files (which the web search tool will likely find)
- Comment sections, issue trackers

**URL validation patterns are unspecified.** "Expected patterns like `github.com/owner/repo`" is too vague. An attacker could create `github.com/attacker/legitimate-looking-name`.

**No output schema enforcement.** The LLM should return structured output that is parsed mechanically, not interpreted freeform.

### Recommendations
1. Define a strict JSON output schema for LLM responses
2. Validate the returned `owner/repo` against GitHub API (existence, age, stars) before presenting to user
3. Consider a denylist of suspicious patterns (repos created recently, repos with names mimicking popular tools)
4. Show the user the full GitHub URL, not just the tool name, in the confirmation prompt
5. Log LLM discovery results for post-hoc analysis of injection attempts

## 7. Ecosystem Probe Typosquatting Defenses

### Current Defense
- Age >90 days
- Downloads >1000/month

### Assessment

These thresholds provide minimal typosquatting defense. Specific gaps:

1. **No edit-distance checking**: The probe doesn't check if a result name is suspiciously similar to a popular tool. `rigrep` on PyPI would pass thresholds if it existed long enough.

2. **No cross-ecosystem correlation**: If `bat` exists on 5 ecosystems but one of them was registered 91 days ago with exactly 1001 downloads, that's suspicious. The probe doesn't compare metadata patterns across ecosystems.

3. **Download count verification**: Not all ecosystems report download counts in the same way or at all. The design's "Uncertainties" section acknowledges this but the threshold filtering assumes it's available.

4. **No description/homepage validation**: Typosquats often have empty descriptions or point to unrelated homepages. This metadata could be used as an additional signal.

### Recommendations
1. Add Levenshtein distance checking against the registry's 500 known tool names. If an ecosystem result is within edit distance 2 of a known tool but doesn't match exactly, flag it.
2. For ecosystems that don't report download counts, fall back to requiring the package to have a non-empty description and a valid homepage URL.
3. Accept that threshold filtering is a noise reduction mechanism, not a security boundary. Document this clearly.

## Summary of Key Findings

1. **Blocker**: The `update-registry` sync mechanism has no integrity verification (marked TBD). This undermines the registry's supply chain guarantees.
2. **High priority**: LLM output needs strict schema enforcement and mechanical validation, not freeform interpretation.
3. **Medium priority**: Threshold-based ecosystem filtering provides noise reduction but is not a security boundary. The design should explicitly acknowledge this distinction.
4. **Medium priority**: Add edit-distance checking against known tool names to catch typosquats that pass age/download thresholds.
5. **Medium priority**: Registry trust model needs ownership pinning and change-of-ownership detection for upstream repos.
6. **Low priority**: Normalize tool name input to ASCII to prevent homoglyph attacks.
7. **Low priority**: Add client-side rate limiting for ecosystem API probes.
