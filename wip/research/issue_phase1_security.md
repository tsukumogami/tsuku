# Security Issue Analysis: Ecosystem Probe Name-Squatting Risk

**Date**: 2026-02-01
**Context**: Assessment of potential GitHub issue about name-squatting vulnerability in tsuku's ecosystem probe
**Scope**: Evaluate problem clarity, issue type, scope appropriateness, gaps, and recommendation

---

## Executive Summary

The issue describes a legitimate but narrow security concern: the ecosystem probe (DESIGN-ecosystem-probe.md, fully implemented and merged) accepts packages from any registry without quality gates beyond exact name matching. A malicious actor could register a package name on a higher-priority ecosystem (e.g., crates.io) to intercept a tool installation intended for another ecosystem (e.g., npm). The design document acknowledged this as "residual risk" but proposed no specific mitigations.

**Assessment**: The problem is clearly defined and narrowly scoped. However, the concern is somewhat overstate given the design's actual safeguards, and it represents an edge case rather than a critical gap. The issue is suitable for a single, focused task with appropriate severity (low-to-medium).

---

## Detailed Analysis

### 1. Problem Definition: YES (Clear and Well-Specified)

**What the issue correctly identifies:**

- The ecosystem probe queries registries and returns the first match based on static priority ranking
- Exact name matching is enforced (per ecosystem_probe.go line 92: `strings.EqualFold(outcome.result.Source, toolName)`)
- Only existence is checked; no download counts, age thresholds, or content verification occurs
- The design doc (DESIGN-ecosystem-probe.md) explicitly lists this as "Residual risk" under "Trade-offs Accepted" (lines 170-173):
  > "No age filtering for most ecosystems: Only Go provides publish dates. Young typosquat packages on npm or PyPI won't be filtered by age. Mitigated by exact name matching and the curated registry taking precedence."

**What the concern describes as a gap:**

The issue states the design "didn't propose mitigations beyond exact name matching." This is partially true:

1. **Exact name matching** is implemented (ecosystem_probe.go:92)
2. **Static priority ranking** favors ecosystems less prone to squatting (cask > crates.io > pypi > npm > rubygems > go > cpan)
3. **Curated registry precedence**: The chain resolver (ChainResolver in chain.go) checks the curated registry (~500 entries) before the ecosystem probe, so popular tools never reach the probe
4. **User override**: Users can always specify `--from` to force a specific source
5. **No implicit execution**: The probe only checks existence; binary build/verification happens later with existing checks (signature verification, checksum validation)

The issue's framing that mitigations "didn't [go] beyond exact name matching" understates these safeguards.

---

### 2. Issue Type: FEATURE/ENHANCEMENT or DESIGN GAP (Not a bug)

**Classification**: This is NOT a bug; it's a design question about additional safeguards.

The ecosystem probe functions as designed:
- It queries registries in priority order (ecosystem_probe.go:28-36)
- It enforces exact name matching (ecosystem_probe.go:92)
- It returns the best match without additional filtering (ecosystem_probe.go:119-129)

The question is whether the current safeguards are sufficient or if additional mitigations are needed. Options:

1. **DESIGN GAP**: Document a new design for enhanced name-squatting detection
2. **FEATURE**: Add optional quality gates (e.g., download count thresholds, age checks, reputation scoring)
3. **CHORE**: Add documentation clarifying the name-squatting risk and existing mitigations

Given the design doc already acknowledged this and the implementation includes multiple layers of defense, this is closer to **CHORE** (documentation/clarification) or a **FEATURE** (enhanced detection) rather than a critical bug.

---

### 3. Scope Appropriateness: YES (Single Issue Scope)

**Is this a single, implementable task?** Mostly yes.

The problem is narrow: "How should we mitigate name-squatting in ecosystem probe?"

A single issue could address:
- Research name-squatting frequency on real package registries
- Evaluate the likelihood of an attack (how likely is a malicious actor to register a high-priority ecosystem package with the exact name of a popular tool?)
- Assess the impact if it occurs (installation fails during build, not a silent compromise)
- Recommend specific mitigations (e.g., add optional reputation scoring, implement ecosystem-specific heuristics)

However, if the scope expands to "implement multiple mitigation strategies," this should be split into:
- #N1: Research and design enhanced name-squatting detection
- #N2: Implement reputation scoring from package registries
- #N3: Add ecosystem-specific heuristics

---

### 4. Gaps and Ambiguities

**Critical gaps in the issue description:**

1. **Attack likelihood not quantified**: How many tools exist in multiple ecosystems with exact name matches? Is this a realistic threat?
   - Example: `bat` exists on crates.io (file viewer) and npm (unrelated package)
   - Counter-example: How many tools published on both crates.io AND npm with exact name match?
   - The curated registry (~500 tools) covers the most popular ones, reducing probe frequency

2. **Impact understated**: The issue says "it's still a supply chain concern" but doesn't describe the actual impact:
   - If the wrong binary is downloaded, the build would fail (Cargo.toml parsing, missing binaries)
   - The user would see an error, not a silent compromise
   - The attack surface is limited to tools NOT in the curated registry

3. **Existing mitigations not acknowledged**: The issue frames this as undefended, but misses:
   - The curated registry is checked first (chain resolver stage 1)
   - Exact name matching filters out most false positives
   - Static priority ranking favors reliable ecosystems
   - The build will fail if the wrong package is used (no silent compromise)

4. **No consideration of user-provided overrides**: Users can specify `--from crates.io::bat` to force a specific ecosystem source, bypassing the probe

5. **No risk vs. benefit analysis**: The probe is valuable (free, fast, deterministic tool discovery). Adding aggressive filtering (e.g., requiring >1000 downloads) would increase false negatives

**Ambiguous assumptions:**

- The issue assumes downloading the wrong binary is a serious risk (true) but doesn't acknowledge that the build would fail (mitigating factor)
- It treats all ecosystems as equally vulnerable to squatting (npm/PyPI are more vulnerable than crates.io/Homebrew)
- It doesn't ask whether this is a real problem in practice (has anyone reported this occurring?)

---

### 5. Design Document Review

**DESIGN-ecosystem-probe.md acknowledgment of this risk:**

The design explicitly addresses name confusion in the "Supply Chain Risks" section (lines 336-347):

> "The primary risk is name confusion: a tool named `bat` exists on both crates.io (the popular file viewer) and npm (an unrelated package). If the probe returns the wrong one, the user installs something they didn't intend."

**Mitigations already in place:**

1. Curated registry precedence (stage 1 of chain resolver)
2. Exact name matching (filters partial matches)
3. Static priority ranking (favors CLI-heavy ecosystems)
4. Disambiguation UI (#1321) for close matches
5. User override with `--from`

**Acknowledged trade-off (lines 170-173):**

> "No age filtering for most ecosystems: Only Go provides publish dates. Young typosquat packages on npm or PyPI won't be filtered by age. Mitigated by exact name matching and the curated registry taking precedence."

The design doc considered download count + age filtering (original parent design) but rejected it because:
- Registry APIs don't expose download counts (decision analysis, lines 76-78)
- Adding secondary API calls for stats would double latency and add fragile dependencies (lines 112-114)
- Static priority is simpler, faster, and deterministic (lines 166-167)

---

### 6. Current Implementation Review

**ecosystem_probe.go behavior:**

```go
// Line 92: Exact name match filter (case-insensitive)
if !strings.EqualFold(outcome.result.Source, toolName) {
    continue
}

// Lines 105-117: Priority-based ranking
sort.Slice(matches, func(i, j int) bool {
    pi := p.priority[matches[i].builderName]
    pj := p.priority[matches[j].builderName]
    if pi == 0 { pi = 999 }      // Unknown builders rank lowest
    if pj == 0 { pj = 999 }
    return pi < pj
})

// Line 119: Returns best match
best := matches[0]
```

**Priority mapping (lines 28-36):**

```go
"cask":      1,          // Homebrew: curated, high-trust
"crates.io": 2,          // Rust: CLI tools, less namespace pollution
"pypi":      3,          // Python: many CLI tools
"npm":       4,          // Largest but noisiest (most squatting risk)
"rubygems":  5,          // Declining
"go":        6,          // Requires module path
"cpan":      7,          // Legacy
```

**Assessment:**

- Priority ranking is reasonable (cask and crates.io are curated/trust-heavy)
- However, crates.io could still be targeted by a malicious actor with resources
- No content analysis, reputation checks, or cross-registry validation occurs

---

### 7. Test Coverage

**From probe_test.go and ecosystem_probe_test.go:**

Tests exist for:
- Single result matching ✓
- Multiple results with priority ranking ✓
- Name mismatch filtering ✓
- Case-insensitive name matching ✓
- Partial failures (soft errors) ✓
- Timeouts ✓
- API errors ✓

**Missing security tests:**

- No test for exact-name-match vs. substring match (e.g., "bat" vs. "batcat")
- No test verifying priority ranking when multiple ecosystems have exact match
- No scenario testing typosquat packages (e.g., "git-hub" instead of "github")
- No testing of ecosystem-specific validation (e.g., verifying crate actually contains binaries)

---

## Recommended Actions

### Option 1: Document the Risk and Existing Mitigations (Minimal)

**Title**: `docs(security): clarify ecosystem probe name-squatting mitigations`

**Scope**: Update DESIGN-ecosystem-probe.md and add a security.md guide explaining:
- Why the design prioritizes certain ecosystems
- How exact name matching filters partial matches
- Why the curated registry reduces probe frequency
- How users can force a specific ecosystem with `--from`
- Known limitations and acceptable risk

**Effort**: 2-4 hours
**Risk**: Low (documentation only)
**Impact**: Reduces confusion, sets expectations for users and contributors

---

### Option 2: Research Name-Squatting Frequency (Tactical Research)

**Title**: `research: quantify name-squatting risk across package registries`

**Scope**: Single task to investigate:
- How many tools exist on multiple ecosystems with exact name match?
- What's the distribution (popular tools vs. obscure)?
- Are there documented cases of cross-registry squatting?
- How often does the probe fire (registry hit rate)?

**Effort**: 4-6 hours
**Risk**: Low (research only, no code changes)
**Impact**: Informs whether additional mitigations are warranted

---

### Option 3: Add Enhanced Heuristics (Feature Implementation)

**Title**: `feat(discover): add enhanced name-squatting detection for ecosystem probe`

**Scope**: Implement one or more of:
- Ecosystem-specific validation (e.g., verify crate has binary)
- Cross-ecosystem reputation checking (does package exist on multiple ecosystems as a signal?)
- Age filtering when available (Go, and potentially npm via secondary API)
- Configurable quality gates (users can set `--min-downloads` threshold)

**Effort**: 8-16 hours (depends on which heuristics)
**Risk**: Medium (changes discovery behavior, could reduce match rate)
**Impact**: Higher confidence in matched packages, but may reduce usability for niche tools

---

## Recommended Title (Conventional Commits Format)

Based on assessment, the issue should be reframed as one of:

1. **If pursuing Option 1**: `docs(security): document ecosystem probe name-squatting mitigations`
2. **If pursuing Option 2**: `research(discover): assess cross-registry name-squatting risk`
3. **If pursuing Option 3**: `feat(discover): add reputation-based filtering to ecosystem probe`

The current framing ("Name-squatting vulnerability in ecosystem probe") overstates the issue's severity and understates existing safeguards. A more precise title would acknowledge it's a known trade-off being reconsidered for additional mitigations.

---

## Summary

| Aspect | Assessment |
|--------|-----------|
| Problem clarity | YES - clearly defined, but frames existing design decisions as gaps |
| Issue type | CHORE/FEATURE (not a bug; a design enhancement) |
| Scope appropriateness | YES, single task if narrowly scoped |
| Existing awareness | YES - design doc explicitly acknowledged as "residual risk" |
| Gaps in issue | Yes: lacks attack likelihood data, understates existing mitigations, no risk analysis |
| Actionable recommendation | Document existing safeguards + research real-world frequency + optionally implement enhanced heuristics |
| Suggested title | `docs(security): document ecosystem probe name-squatting mitigations` (minimal) or `research(discover): assess cross-registry name-squatting risk` (investigative) |

The issue is valid but represents a known design trade-off, not a discovered vulnerability. Appropriate response depends on organizational risk appetite and actual attack frequency in the wild.
