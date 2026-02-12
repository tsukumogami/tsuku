# Options Analysis Review: DESIGN-Disambiguation

## Review Context

This document reviews the problem statement and options analysis for the Disambiguation design document. The review evaluates whether the problem is well-defined, whether alternatives are fairly considered, and whether any unstated assumptions need to be explicit.

## 1. Problem Statement Evaluation

### Strengths

The problem statement is specific and actionable:

1. **Clear root cause**: "silently selects the highest-priority registry (cask > homebrew > crates.io > ...) without user awareness" - points directly to the code behavior that needs to change.

2. **Concrete example**: The bat example (sharkdp/bat on crates.io vs bat-cli on npm) is a real case that illustrates the security and usability concerns.

3. **Dual problem framing**: Security risk (wrong package) and usability problem (no recourse) are distinct concerns that may require different mitigations.

### Gaps

1. **Missing frequency data**: How often do name collisions actually occur? The design acknowledges this uncertainty ("Collision frequency: Unknown how often tools actually collide across ecosystems in practice") but proceeds with a complex solution without first validating the problem size.

   **Recommendation**: Add a scope qualifier like "Applies to tools where ecosystem probe returns 2+ matches" and note that telemetry will inform whether the solution complexity is justified.

2. **Batch pipeline problem is understated**: The batch context is mentioned ("Batch pipeline compatibility: Must support deterministic selection") but the problem statement focuses on interactive use. The batch pipeline has a fundamentally different requirement: it needs to generate recipes at scale without human intervention, but those recipes should be correct. This deserves equal billing in the problem statement.

3. **Missing: What happens today?** The problem statement says silent selection is bad, but doesn't say what the current user experience is. A reader unfamiliar with the codebase doesn't know whether users see any output, whether they can tell which source was selected, or whether they're surprised post-install.

## 2. Missing Alternatives

### Disambiguation Algorithm (Decision 1)

The document considers:
- Popularity-based auto-select with interactive fallback (chosen)
- Always prompt (rejected)
- LLM disambiguation (rejected)
- Require explicit --from (rejected)

**Missing alternatives:**

1. **Registry-first with ecosystem fallback**: If the tool is in the discovery registry, use that without prompting. Only disambiguate for tools not in the registry. This shifts disambiguation complexity to registry curation.

   **Why it matters**: The registry already exists for ~500 tools and handles disambiguation implicitly. The design should acknowledge that registry entries bypass disambiguation entirely, making the algorithm only relevant for registry misses.

2. **Confidence-weighted selection**: Instead of a fixed 10x threshold, weight by data quality. Homebrew packages are curated (high confidence). npm downloads might be inflated (lower confidence). The algorithm could use ecosystem-specific weights.

   **Why it matters**: A 10x download gap between npm (easily gamed) and crates.io (harder to game) is different from a 10x gap within the same ecosystem.

3. **User preference learning**: Store user's past selections and weight toward their preferred ecosystems. "You usually install Rust tools; prefer crates.io."

   **Why it matters**: This is explicitly out of scope, but should be mentioned as future work since it's a natural extension.

### Typosquatting Detection (Decision 2)

The document considers:
- Edit distance <= 2 against registry entries (chosen)
- Block instead of warn (rejected)
- Phonetic similarity (rejected)
- Check all ecosystem packages (rejected)

**Missing alternatives:**

1. **Edit distance + popularity anomaly**: SpellBound (cited in the research) combines edit distance with popularity checking. A package named "rigrep" with 50 downloads that's edit-distance-1 from "ripgrep" (1M downloads) is suspicious. The combination is stronger than either signal alone.

   **Why it matters**: The design cites SpellBound's 99.4% detection rate but doesn't adopt its approach.

2. **Check against ecosystem-specific popular packages**: Instead of only checking against the ~500 registry entries, check against the top packages per ecosystem. A typosquat on npm is likely targeting a popular npm package.

   **Why it matters**: The registry is cross-ecosystem. Ecosystem-specific typosquatting (npm:lodash vs npm:lodsh) wouldn't be caught unless lodash is in the registry.

3. **Defer to ecosystem probe**: Some registries (npm) already have typosquatting detection. The design could check if the registry flagged the package.

   **Why it matters**: npm's SpellBound is running; duplicating it adds cost without adding coverage.

### Interactive Prompt (Decision 3)

**Missing alternatives:**

1. **Progressive disclosure**: Show the top match first with y/N, then show alternatives only if the user says N. Reduces cognitive load for the common case.

   **Why it matters**: Showing 3+ options with metadata for every ambiguous case is heavyweight. Most users want the popular one.

2. **Default timeout auto-select**: After 10 seconds with no input, select the top match. Common in installers.

   **Why it matters**: Unattended scripts that accidentally pipe into tsuku would hang forever without this.

### Batch Integration (Decision 4)

**Missing alternatives:**

1. **Threshold-based gating**: Only auto-merge recipes where disambiguation was unambiguous (10x+ gap). Queue ambiguous recipes for manual review.

   **Why it matters**: This separates "definitely right" from "probably right" at the PR level, not just the metrics level.

2. **Batch-specific overrides**: A `batch-disambiguations.json` that's separate from user-facing disambiguation. Batch operators might want different defaults.

   **Why it matters**: Batch operators have different risk tolerance than interactive users.

### Code Location (Decision 5)

**Missing alternative:**

1. **Standalone disambiguator with ecosystem probe integration**: Create a `Disambiguator` type that ecosystem probe uses, but that could also be used elsewhere (e.g., by LLM discovery when it finds multiple sources).

   **Why it matters**: LLM discovery already has similar ranking logic. The design says "Premature abstraction. If patterns converge, refactor later." - this is reasonable, but the alternative should be stated.

## 3. Rejection Rationale Evaluation

### Decision 1: Disambiguation Algorithm

| Alternative | Rejection Rationale | Fair? |
|-------------|---------------------|-------|
| Always prompt | "too much friction" | Partial. Doesn't acknowledge that this is the safest option. Should note that it was considered and rejected for UX reasons despite being most secure. |
| LLM disambiguation | "adds latency/cost" | Fair. Also correctly notes that LLM is already the fallback for ecosystem probe misses. |
| Require explicit --from | "too disruptive" | Fair. Provides a migration path concern. |

**Issue**: The "always prompt" rejection doesn't engage with the security benefit. The design should acknowledge that always-prompt is the most secure but trades security for convenience, and that the 10x threshold is a calibrated risk acceptance.

### Decision 2: Typosquatting Detection

| Alternative | Rejection Rationale | Fair? |
|-------------|---------------------|-------|
| Block instead of warn | "false positives" | Fair. Correctly notes that legitimate tools may have similar names. |
| Phonetic similarity | "typed not spoken" | Fair. This is correct - typos are keyboard errors, not pronunciation errors. |
| Check all ecosystem packages | "too slow" | Partial. Doesn't quantify "too slow." How many packages? What's the latency? |

**Issue**: "Check all ecosystem packages" rejection needs numbers. Is it 1M packages? Would it add 100ms or 10s? Without data, this reads as a guess.

### Decision 3: Interactive Prompt

| Alternative | Rejection Rationale | Fair? |
|-------------|---------------------|-------|
| Simple y/N for top match | "no alternatives shown" | Partial. This is a valid design that other installers use. Should engage more. |
| JSON output only | "human readability first" | Fair. JSON is for machines, and this is the primary UX. |
| Full GitHub metadata | "requires extra API calls" | Fair. Correctly notes that ecosystem probe doesn't fetch this. |

**Issue**: "Simple y/N" is dismissed too quickly. Many package managers use this pattern successfully (e.g., apt's "Do you want to continue? [Y/n]"). The design should explain why showing alternatives is worth the additional complexity.

### Decision 4: Batch Integration

| Alternative | Rejection Rationale | Fair? |
|-------------|---------------------|-------|
| Pause for ambiguous tools | "growing backlog" | Fair. Batch needs to complete runs. |
| Require all tools in disambiguations.json | "inverts workflow" | Fair. The common case should be handled automatically. |

These rejections are well-reasoned.

### Decision 5: Code Location

| Alternative | Rejection Rationale | Fair? |
|-------------|---------------------|-------|
| New resolver stage | "unnecessary complexity" | Fair. Disambiguation is part of ecosystem probe, not a separate stage. |
| Separate file | "adds indirection" | Partial. The implementation approach creates `disambiguation.go` anyway, contradicting this. |

**Issue**: The rejection of a separate file contradicts the implementation approach, which creates `internal/discover/disambiguate.go`. The "Chosen" section says "Integrate into ecosystem_probe.go" but Phase 1 says "Files to create: internal/discover/disambiguate.go". This is internally inconsistent.

## 4. Unstated Assumptions

### Assumption 1: Download counts are comparable across ecosystems

The 10x threshold treats downloads from different ecosystems as equivalent. But npm packages often have inflated download counts from CI pipelines, while crates.io downloads are more indicative of actual usage.

**Impact**: A 10x gap on npm might be less meaningful than a 10x gap on crates.io.

**Recommendation**: Acknowledge this assumption and consider ecosystem-specific adjustments in future iterations.

### Assumption 2: The registry is the source of truth for typosquatting

Typosquatting detection checks against registry entries (~500 tools). This assumes the registry contains all high-value targets.

**Impact**: Popular tools not in the registry are unprotected from typosquatting.

**Recommendation**: Note that registry coverage limits typosquatting detection effectiveness. Consider checking against the priority queue as well (which has thousands of popular tools).

### Assumption 3: Users will read the prompt

The design assumes users will examine the metadata and make informed decisions. Research on installer behavior suggests many users blindly accept defaults.

**Impact**: The careful metadata display may not improve decision quality.

**Recommendation**: Note this assumption and consider whether the default selection should be even more conservative.

### Assumption 4: Non-interactive mode is rare

The design treats non-interactive errors as an edge case ("Non-interactive: Error with numbered list of options and --from suggestion"). But CI/CD pipelines are common.

**Impact**: Many users may hit non-interactive errors frequently.

**Recommendation**: Consider whether non-interactive mode should auto-select the top match (with a warning) rather than error. This is what the batch pipeline does.

### Assumption 5: Batch and interactive have the same disambiguation needs

The design uses the same algorithm for both, with different handling of the prompt. But batch cares about reproducibility (same run should produce same result), while interactive cares about user intent.

**Impact**: The 10x threshold might be wrong for batch (too aggressive) or interactive (not aggressive enough).

**Recommendation**: Make this assumption explicit and note that thresholds may diverge based on telemetry.

## 5. Strawman Analysis

**Definition**: A strawman is an alternative designed to fail, making the chosen option look better by comparison.

### Candidates for strawman status:

1. **"LLM disambiguation"**: This alternative is somewhat strawman-ish. The design correctly notes that LLM discovery is already the fallback, so adding LLM to ecosystem probe would be redundant. But the alternative as stated is easy to reject. A fairer framing would be "Use LLM only for truly ambiguous cases where no popularity signal exists."

2. **"Require explicit --from for all ambiguous tools"**: This is the most strawman-like. No one would seriously propose breaking existing workflows. A fairer alternative would be "Require --from only when downloads are within 2x" (narrower scope than chosen option).

3. **"Block instead of warn"** (typosquatting): This is easy to reject but represents a real design point. Security-focused tools do block. The rejection should engage with why warning is appropriate for a developer tool (developers are trusted to make decisions) vs. why blocking might be appropriate for end-user software.

### Verdict

The alternatives are mostly fair, but some are framed in their most extreme form to make rejection easier. The design would be stronger if it engaged with weaker versions of the rejected alternatives.

## 6. Recommendations

### High Priority

1. **Resolve the code location inconsistency**: The design says "integrate into ecosystem_probe.go" but the implementation creates a separate file. Pick one and update both sections.

2. **Add collision frequency data**: Before committing to this solution, measure how often the ecosystem probe returns 2+ matches. If it's rare (<5% of queries), the complexity may not be justified.

3. **Strengthen the "always prompt" rejection**: Acknowledge this is the secure default, explain why UX wins, and note the 10x threshold as a calibrated risk.

### Medium Priority

4. **Clarify batch vs. interactive divergence**: State explicitly that batch and interactive use the same algorithm today, but may diverge based on telemetry.

5. **Address the non-interactive UX**: Consider whether erroring is the right default for CI/CD, or whether a warning + auto-select is more appropriate.

6. **Quantify "too slow"**: Add numbers to the typosquatting rejection for checking all ecosystem packages.

### Low Priority

7. **Note ecosystem-specific download scaling**: Acknowledge that 10x on npm is different from 10x on crates.io.

8. **Consider edit distance + popularity**: The SpellBound approach is cited but not adopted. Either adopt it or explain why edit distance alone is sufficient.

## Summary

The disambiguation design is well-structured with clear decision drivers and reasonable choices. The main issues are:

1. **Internal inconsistency** between Decision 5 (integrate into ecosystem_probe.go) and the implementation approach (create disambiguate.go)
2. **Missing frequency data** - the solution complexity isn't validated against problem frequency
3. **Weak engagement with "always prompt"** - the secure default is dismissed without acknowledging its merit
4. **Unstated assumptions** about download comparability and user behavior

The alternatives are mostly fair, though some are framed in extreme forms. No alternatives are pure strawmen, but a few (LLM disambiguation, require explicit --from) are easier to reject than necessary.

Overall, the design makes sound choices. The recommendations above would strengthen the rationale and reduce risk of building a solution larger than the problem requires.
