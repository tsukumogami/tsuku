# Design Review: Homebrew Tap Support

**Document:** `docs/designs/DESIGN-tap-support.md`
**Reviewer:** Design Review Agent
**Date:** 2026-01-13

## Overall Assessment: NEEDS_REVISION

The tap support design is well-structured and follows the established cask design patterns. However, there are several gaps that need to be addressed before implementation can proceed. The primary concerns are around the Ruby parsing fragility, incomplete bottle URL resolution, and missing implementation slices dependency tracking.

---

## Section-by-Section Feedback

### 1. Upstream Design Reference

**Status:** PASS

The design correctly references the upstream homebrew and cask designs. The relevant patterns are identified (hybrid approach, GHCR bottle downloading, version provider factory).

---

### 2. Context and Problem Statement

**Status:** PASS

**Strengths:**
- Clear articulation of the gap (third-party taps not accessible)
- Concrete examples of popular taps (hashicorp/tap, github/gh, etc.)
- Good comparison table showing differences between homebrew/core and third-party taps
- Well-defined scope boundaries (in-scope vs out-of-scope)

**Minor suggestion:**
- Consider adding usage statistics or user research data to quantify demand for tap support

---

### 3. Decision Drivers

**Status:** PASS

**Strengths:**
- Identifies key constraints (API rate limits, bottle availability, security model)
- Explicitly calls out Ruby parsing complexity as a known risk
- Links to cask design for consistency

---

### 4. Options Analysis

**Status:** NEEDS_REVISION

**Issues:**

1. **Option 1 pros/cons imbalance**: The chosen option (dedicated `tap` provider) lists "No local storage required" as a pro, but later the Solution Architecture introduces `TapCache` for local storage. This is inconsistent.

2. **Option 4 dismissal reasoning incomplete**: The hybrid approach was rejected as "complex fallback logic" and "inconsistent behavior," but these issues exist in the chosen approach too (GitHub API fallback, caching layer). The comparison should be more rigorous.

3. **Missing Option**: No consideration of using the Homebrew API's extended endpoints. While `formulae.brew.sh` doesn't directly expose third-party taps, some Homebrew-maintained taps (like `homebrew/cask-versions`) are accessible via the API. This could reduce GitHub API dependency for a subset of taps.

**Recommendation:**
- Align Option 1 description with the actual solution (which does include caching)
- Strengthen Option 4 rejection rationale or reconsider hybrid as the chosen approach

---

### 5. Decision Rationale

**Status:** PASS

**Strengths:**
- Clear alignment with cask design principles
- Explicit acknowledgment of trade-offs (rate limiting, Ruby parsing, bottle variability)
- Good separation of concerns justification

---

### 6. Solution Architecture

**Status:** NEEDS_REVISION

**Issues:**

1. **Ruby Parsing Strategy Underspecified**

   The design mentions "targeted regex extraction" but provides only pseudocode. Critical questions unanswered:

   - What specific regex patterns will be used?
   - How will nested `bottle do` blocks be handled?
   - How will conditional version strings (e.g., `version :head`, version from URL inference) be parsed?
   - What happens when the formula uses Ruby metaprogramming (not uncommon)?

   **Recommendation:** Add a detailed section on Ruby parsing strategy with:
   - Specific regex patterns for version, bottle block, checksum extraction
   - Known edge cases and how they'll be handled
   - Fallback behavior when parsing fails
   - Test coverage strategy for parsing robustness

2. **Bottle URL Resolution Incomplete**

   The design shows `bottle_url` as a template variable but doesn't explain how bottle URLs are constructed. Homebrew formulas don't store complete URLs in the `bottle do` block - they store:
   - `sha256` per platform
   - Optional `root_url` (defaults to GHCR for homebrew/core)

   Third-party taps often:
   - Use GitHub Releases (`root_url "https://github.com/{owner}/{repo}/releases/download/v#{version}"`)
   - Use custom S3/CDN URLs
   - Have no bottles at all (source-only)

   **Recommendation:** Add explicit handling for:
   - Parsing `root_url` from formula
   - Constructing full bottle URL from `root_url` + formula name + version + platform tag
   - Detecting and rejecting source-only formulas clearly

3. **Platform Tag Mapping Missing**

   The cask design handles architecture selection (`arm64` vs `x86_64`). The tap design needs equivalent handling for bottle platform tags:
   - `arm64_sonoma`, `arm64_ventura`, etc.
   - `sonoma`, `ventura`, etc. (Intel)
   - `x86_64_linux`

   **Recommendation:** Add platform tag resolution logic similar to cask's architecture handling.

4. **Short Form Syntax Ambiguity**

   Pattern B shows `source = "tap:hashicorp/tap/terraform"` but the parsing rules are unclear:
   - How is this split? Is it `tap:owner/repo/formula` or `tap:tap-name/formula`?
   - Does `hashicorp/tap` mean owner=hashicorp, repo=homebrew-tap?

   **Recommendation:** Document explicit parsing rules for the short form.

---

### 7. Implementation Approach (Slices)

**Status:** NEEDS_REVISION

**Issues:**

1. **Slice 3 Dependency Inconsistency**

   Slice 3 says: "Dependencies: Slice 1, requires cask support (#862) for template infrastructure"

   But the cask design shows #862 as a walking skeleton, with template infrastructure (#863) being a separate issue. This dependency should be on the specific template infrastructure, not the entire cask walking skeleton.

   **Recommendation:** Clarify which specific cask slice provides the template infrastructure needed.

2. **Missing Slice for Formula File Layout Variations**

   The design assumes formulas are at `Formula/{name}.rb`, but taps can have:
   - `Formula/{name}.rb` (standard)
   - `Casks/{name}.rb` (if the tap contains casks)
   - `HomebrewFormula/{name}.rb` (legacy)
   - Aliases and symlinks

   **Recommendation:** Add a slice or extend Slice 1 to handle formula file discovery.

3. **Missing Testing Slice**

   The cask design includes golden file testing mentions. The tap design should have a similar testing strategy slice that covers:
   - Mocked GitHub API responses
   - Formula parsing test fixtures
   - Integration tests with real taps (hashicorp/tap as canonical example)

---

### 8. Consequences

**Status:** PASS

**Strengths:**
- Honest assessment of negative consequences
- Practical mitigations proposed

**Minor suggestion:**
- Add consequence: "Maintenance burden for formula parser as Homebrew DSL evolves"

---

### 9. Security Considerations

**Status:** PASS

**Strengths:**
- All four dimensions covered (Download Verification, Execution Isolation, Supply Chain Risks, User Data Exposure)
- Clear trust model diagram showing third-party differences
- Practical mitigation table

**Minor gap:**
- Consider adding a note about GitHub token security (don't log tokens, use fine-grained permissions)

---

## Cask Design Alignment Check

| Aspect | Cask Design | Tap Design | Aligned? |
|--------|-------------|------------|----------|
| Version provider pattern | Yes (`cask` provider) | Yes (`tap` provider) | Yes |
| Reuses existing action | Yes (`app_bundle`) | Yes (`homebrew`) | Yes |
| Template variables | `{{version.url}}`, `{{version.checksum}}` | `{{version.bottle_url}}`, `{{version.checksum}}` | Yes |
| Explicit API source | `formulae.brew.sh/api/cask` | GitHub raw content | Yes |
| Cache layer | Not explicitly mentioned | `TapCache` | Tap adds caching (appropriate) |
| Implementation slices | 5 slices with dependencies | 4 slices with dependencies | Yes |
| Security dimensions | All 4 covered | All 4 covered | Yes |
| Short form syntax | `cask:name` | `tap:owner/repo/formula` | Yes (consistent pattern) |

**Alignment verdict:** The tap design follows the cask design patterns well. The main deviation (adding a cache layer) is appropriate given GitHub API rate limits.

---

## Specific Issues Summary

### Critical (Must Fix)

1. **Ruby Parsing Strategy Underspecified** - Without detailed regex patterns and edge case handling, implementation will be blocked or produce a fragile parser.

2. **Bottle URL Construction Missing** - The design doesn't explain how to go from formula file contents to a complete bottle URL.

### Major (Should Fix)

3. **Platform Tag Mapping** - Need explicit handling for Homebrew's platform tags.

4. **Slice 3 Dependency Confusion** - Clarify the actual dependency on cask template infrastructure.

5. **Formula File Discovery** - Handle variations in tap directory structure.

### Minor (Consider)

6. **Option 1 Storage Inconsistency** - Remove "no local storage" pro or explain the distinction.

7. **Short Form Parsing Rules** - Document explicitly.

8. **Testing Slice** - Add explicit testing strategy.

---

## Recommendations

1. **Add a "Ruby Parsing" subsection** under Solution Architecture that:
   - Lists the exact regex patterns for version, bottle block, root_url, sha256
   - Documents known formula patterns that cannot be parsed (and error messaging)
   - Provides 2-3 concrete formula examples with expected parse output

2. **Add a "Bottle URL Resolution" subsection** that:
   - Shows the formula for constructing bottle URLs: `{root_url}/{formula}--{version}.{platform}.bottle.tar.gz`
   - Lists known `root_url` patterns (GitHub Releases, S3, GHCR)
   - Handles missing bottles explicitly

3. **Add platform tag mapping table** similar to cask's architecture handling:
   ```
   | runtime.GOOS | runtime.GOARCH | macOS Version | Platform Tag |
   |--------------|----------------|---------------|--------------|
   | darwin | arm64 | 14 | arm64_sonoma |
   | darwin | amd64 | 14 | sonoma |
   | linux | amd64 | - | x86_64_linux |
   ```

4. **Clarify Slice 3 dependency** - Reference specific cask issue number for template infrastructure, or define the template infrastructure in this design if it's expected to land first.

5. **Add Slice 0: Formula Parsing Test Fixtures** - Create a comprehensive test suite before implementing the parser.

---

## Conclusion

The tap support design demonstrates strong alignment with tsuku's architecture and the cask design patterns. The problem statement is clear, the options analysis is reasonable, and the security considerations are thorough. However, the implementation-critical details around Ruby parsing and bottle URL construction need to be fleshed out before this design is ready for implementation.

**Next Steps:**
1. Address Critical and Major issues listed above
2. Update Implementation Approach with corrected dependencies
3. Re-review after revisions
