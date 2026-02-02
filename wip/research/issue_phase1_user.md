# Issue Analysis: Ecosystem Probe Name-Squatting False Positives

**Phase:** Initial Assessment (User Perspective)
**Date:** 2025-02-01

---

## 1. Problem Clarity Assessment: YES - Well Defined

### What the User Reports
The user has a concrete, reproducible problem with specific examples:
- Running `tsuku create prettier` (npm tool) fails because a squatter package exists on crates.io
- Running `tsuku create httpie` (Python tool) fails because a squatter package exists on crates.io
- Running `tsuku create ruff` (Python/Rust tool) fails because a squatter package exists on crates.io

The root cause is clear: crates.io has ecosystem priority 2 (higher than npm at 4 and pypi at 3), so when a placeholder/squatter crate with the exact name exists, it wins the priority-based disambiguation and users get "Cargo is required" errors for tools that have nothing to do with Rust.

### The Pain
Users can't create recipes for legitimate tools when placeholder packages exist in higher-priority registries. This breaks the discovery experience without a workaround (other than using `--from` to explicitly specify the source).

### Clarity Score: 10/10
- Exact error scenario
- Reproducible with named examples
- Root cause identified (priority-based disambiguation without quality signals)
- Impact clearly stated (wrong ecosystem selected, downstream build failures)

---

## 2. Issue Type: FEATURE (with bug characteristics)

### Classification Reasoning

**This is primarily a feature request, not a bug**, because:

1. **Current design is working as intended**: The ecosystem probe correctly implements its specification (DESIGN-ecosystem-probe.md):
   - It finds packages that exist in registries
   - It applies exact name matching (which works: `prettier` exists on crates.io)
   - It uses static priority ranking (crates.io=2 > pypi=3 > npm=4)
   - This behavior is documented and intentional

2. **The design accepts this tradeoff**: The DESIGN document explicitly states:
   > "Without download counts, we can't distinguish between a popular crate and an obscure one. A static priority list is a rough proxy. This is acceptable because disambiguation (#1321) will present options to the user in ambiguous cases."

   The design chose to accept coarse disambiguation in exchange for speed and simplicity.

3. **However**, the design also says:
   > "Mitigated by exact name matching and the curated registry taking precedence."

   So exact name matching was intended as a mitigation. The problem is: exact name matching is **necessary but not sufficient**. It catches typos and partial matches, but placeholder packages with the exact same name slip through.

### Why It Has Bug Characteristics

The user's observation points out a gap in the filtering strategy:
- **Design intent**: "Filter out typosquats and placeholder packages"
- **Current mechanism**: Exact name match + static priority
- **Gap exposed**: Placeholder packages with legit exact names in high-priority registries can auto-select and break the user's install

The name-squatting problem is the **reason** the design doc mentioned needing download counts or age filtering. The document explicitly considered this:

> "The parent design specified >90 days AND >1000 downloads/month as noise reduction. Since download counts aren't available, we need a different approach to reduce false positives from typosquats and placeholder packages."
>
> **Chosen: Static ecosystem priority with name-match validation**

The current priority ranking is too coarse to handle cases where crates.io contains legitimately-named but useless packages.

### Verdict: Feature Request
Ask the user to implement better signals (not just existence checking) to filter placeholder packages:
- Check for actual package metadata (files, binaries, recent updates)
- Apply minimum age threshold to go builder (already available)
- Consider ecosystem-specific confidence signals (e.g., npm has download counts available)

---

## 3. Scope Appropriateness: MARGINAL - Needs Clarification

### Scope as Written: Single Issue
The user's suggestion ("inspect that the package has files, binaries, or real content") is **one** coherent idea that could be one issue. However, there's ambiguity about what "real content" means and how to implement it.

### Scope Concerns

**Too Broad:**
- "Better signals to trust a package can be delivered" could mean:
  - Adding download counts from secondary APIs (rejected by design as fragile)
  - Checking package size or binary availability (API-dependent per ecosystem)
  - Applying age thresholds (only Go provides this)
  - Requiring recent updates (would need additional API calls)
  - Machine learning-based quality scoring (way out of scope)

**Too Vague:**
- "Inspect that the package has files, binaries" — which ecosystems provide this? npm no, PyPI yes (wheel/sdist), crates.io yes (binary crates). Go yes, RubyGems yes.
- "Or real content" — what constitutes "real"? A placeholder on crates.io might have a README, a TOML, metadata fields.

### Breakdown Needed
This should be scoped into a design issue or investigation to:
1. Audit the top 5-10 placeholder packages across ecosystems
2. Identify what signals reliably differentiate them from legitimate packages
3. Determine which signals are available from existing APIs without secondary calls
4. Design and estimate implementation effort per ecosystem builder

### Verdict: UNCLEAR SCOPE
The core problem is clear, but the solution has unspecified complexity. Requires upstream design work before implementation.

---

## 4. Gaps and Ambiguities

### A. Which Signals Are Actually Available?

| Ecosystem | Has Download Count | Has Age | Has Binary Info | Has File List | Additional Cost |
|-----------|-------------------|---------|-----------------|----------------|-----------------|
| crates.io | No (API doesn't expose) | Yes | Yes (downloads field) | Yes | 0 (included) |
| PyPI | Yes (but not in standard API) | Yes | Partial (wheel vs sdist) | Yes | +1 API call to pypistats |
| npm | Yes | Yes | No | Partial | +1 API call to npm downloads |
| RubyGems | No (in downloads page, not API) | Yes | No | Yes | +1 API call or web scrape |
| Go | Yes (via proxy API) | Yes | No | No | 0 (included) |
| CPAN | No | Yes | No | No | 0 (included) |
| Cask | No | No | No | No | 0 (included) |

**Gap**: The user assumes metadata is available; design doc shows it's sparse. Need to clarify what's actually usable without adding latency.

### B. Placeholder Package Detection Heuristics?

The user suggests checking for "real content" but:
- A placeholder crate might have `PLACEHOLDER` in the README but still be valid TOML
- A crate with 0 downloads might be new and legitimate
- A crate published 2 years ago with no updates might be maintained or abandoned

**Gap**: What is the ground truth? Need examples of actual placeholders vs. legitimate tools to design filtering.

### C. Static Priority vs. Download Counts

Design doc says:
> "The static priority ranking may not match user expectations in all cases. A Go developer probably expects `go install` over `cargo install` for a tool that exists in both."

The current priority puts crates.io (2) above npm (4). This assumes crates.io is more trustworthy. But:
- Is this assumption true for the tools users actually install?
- Should priority vary by ecosystem maturity or content quality?

**Gap**: No data on whether the static priority is correct for real tool distributions.

### D. What About Age Filtering?

The design doc mentions:
> "The age threshold from the parent design (>90 days) can be applied to the Go builder since it exposes `Time`. For other builders, age is unknown and not filtered."

**Gap**: Why not use available age data from crates.io, PyPI, etc.? If crates.io provides publish time in its API response, why isn't it being extracted?

---

## 5. Recommended Title (Conventional Commits)

Based on scope and problem clarity:

### Option A: Focused on Diagnosis
```
feat(discover): improve ecosystem probe filtering for placeholder packages
```
**Rationale:** Acknowledges this is a feature enhancement (improving filtering), not a bug fix. "Placeholder packages" is the specific problem.

### Option B: Focused on Solution Direction
```
feat(discover): add quality signals to ecosystem probe disambiguation
```
**Rationale:** Points toward the solution space (quality signals), which the user and design doc agree on.

### Option C: Issue-as-Investigation
```
feat(discover): investigate ecosystem probe false positives from name squatting
```
**Rationale:** Acknowledges the scope ambiguity; suggests this needs upfront investigation before implementation.

### Recommended: **Option A** or **Option C**
- **Option A** if confident in the solution (add metadata checks to builders)
- **Option C** if the implementation approach is unclear (needs design first)

Given the ambiguities in section 4, **Option C is safer**.

---

## 6. Pre-Filing Clarifications Needed from User

Before creating a GitHub issue, ask:

1. **Priority**: Is this blocking your typical workflow, or just occasional edge cases?
   - If most tools work (only rare name conflicts), it's lower priority
   - If you hit this frequently, it's urgent

2. **Preferred Solution**:
   - Would you accept a `--skip-ecosystem-check` flag to force LLM discovery?
   - Should we expose disambiguation UI (show all 3 matches when multiple ecosystems have the same tool)?
   - Or is better filtering required?

3. **Scope Confirmation**:
   - Should we implement for all ecosystems (high effort) or start with crates.io only (which is causing your problem)?
   - Should we use existing metadata (age, file counts) or accept secondary API calls?

4. **Real-World Examples**:
   - Can you provide a list of 5-10 placeholder packages you've hit?
   - Do they have any common patterns (empty README, no downloads, recent publish)?

---

## Summary

| Aspect | Assessment | Evidence |
|--------|-----------|----------|
| **Problem Clear?** | YES | Reproducible examples, root cause identified |
| **Type** | Feature (enhancement) | Current design is working as intended, but needs improvement |
| **Scope** | MARGINAL | Core problem clear, solution approach unspecified |
| **Gaps** | Multiple | Which signals are available? What heuristics work? Should priority change? |
| **Recommended Title** | `feat(discover): investigate ecosystem probe false positives from name squatting` | Reflects scope ambiguity and need for upfront design |
| **Next Step** | Clarify solution approach with user | Ask about priority, preferred solution, preferred scope (all ecosystems or crates.io only?) |

---

## User Experience Impact

**Current State:**
- Users hit false positives occasionally when legitimate tools have name-squatted crates
- Error message ("Cargo is required") is confusing; doesn't suggest the real problem
- Workaround exists: `tsuku create <tool> --from <correct-ecosystem>`

**With Fix:**
- Ecosystem probe would filter out placeholder/low-quality packages
- Discovery would correctly identify the right ecosystem even if multiple candidates exist
- Builds would succeed without manual override

**Why It Matters:**
Ecosystem probe is the "happy path" for users without LLM keys. False positives undermine confidence in the auto-discovery feature and force users to know (or research) the right ecosystem for each tool.
