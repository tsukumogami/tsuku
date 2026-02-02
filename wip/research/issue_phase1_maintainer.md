# Ecosystem Probe Quality Signals - Maintainer Analysis

## Issue Summary
The ecosystem probe's Probe() methods across all 7 builders (cargo, pypi, npm, gem, go, cpan, cask) only check if a package name exists on a registry API but don't validate package quality. This causes misidentification of tools because squatted placeholder packages are detected as valid matches.

**Concrete Problem:** "prettier" exists on crates.io with 87 downloads and version 0.1.5, but crates.io has priority 2 in the static map (above pypi at 3 and npm at 4), so the probe incorrectly identifies prettier as a Rust crate instead of an npm package.

---

## Analysis

### 1. Is the problem clearly defined?
**YES** ✓

The issue precisely identifies:
- What: The Probe() methods only check existence (HTTP 200 response)
- Where: All 7 ecosystem builders in `internal/builders/{cargo,pypi,npm,gem,go,cpan,cask}.go`
- Why it matters: Squatted placeholder packages break tool discovery when they have higher priority
- Concrete example: prettier (npm package) misidentified as a Rust crate

The problem is actionable and measurable.

### 2. Type Classification
**BUG** (with quality/heuristics component)

This is fundamentally a **bug** because the current behavior produces incorrect results. The probe returns true for squatted packages that shouldn't be considered valid matches for tool discovery. However, implementing a fix requires adding **quality heuristics** rather than just patching a logic error.

Alternative framing: Could be **feature request** if treating it as "add quality signals to discovery," but that undersells the fact that current behavior is demonstrably wrong.

### 3. Scope Appropriateness
**YES** - Appropriate for single issue, but requires design decisions ✓

**Scope boundaries:**
- IN SCOPE: Enhancing Probe() methods to check quality signals before returning Exists: true
- IN SCOPE: Updating ProbeResult struct if needed to surface quality metrics
- IN SCOPE: Adjusting the ecosystem probe filtering logic to use quality signals
- POTENTIALLY OUT OF SCOPE: Changing the static priority map (crates.io = 2, pypi = 3, npm = 4)

The static priority map is a **separate architectural decision** that could be addressed in a follow-up issue if needed, but the quality signal enhancement is independent and can proceed without changing priorities.

**Why single issue works:** The implementation involves coordinated changes across 7 builders and the ecosystem probe, but they're all addressing the same quality problem with a consistent solution pattern.

### 4. Current Implementation Review

#### ProbeResult Structure
```go
type ProbeResult struct {
	Exists    bool
	Downloads int    // Monthly downloads (0 if unavailable)
	Age       int    // Days since first publish (0 if unavailable)
	Source    string // Builder-specific source arg
}
```

**Observation:** The ProbeResult struct **already has fields for Downloads and Age** (lines 8-9 in probe.go). This suggests quality signals were designed into the interface but not implemented in most Probe() methods.

#### Current Probe() Patterns (Examples)

**Cargo (line 355-364):**
```go
func (b *CargoBuilder) Probe(ctx context.Context, name string) (*ProbeResult, error) {
	_, err := b.fetchCrateInfo(ctx, name)
	if err != nil {
		return &ProbeResult{Exists: false}, nil
	}
	return &ProbeResult{
		Exists: true,
		Source: name,
	}, nil
}
```
- Only checks if fetchCrateInfo() succeeds (HTTP 200)
- Ignores Downloads, Age fields
- No quality filtering

**PyPI (line 386-395):** Same pattern - existence-only check

**NPM (line 325-334):** Same pattern - existence-only check

**Go (line 271-284):** Partial implementation - computes Age from timestamp but ignores Downloads

**Cargo/PyPI/NPM/Gem/CPAN/Cask:** 6 of 7 builders have no quality signal computation

#### What Data is Accessible?

Examining fetchCrateInfo(), fetchPackageInfo() methods:
- **Cargo:** cratesIOCrateResponse has no download count or version count fields (would require additional API call)
- **PyPI:** Response has no download stats (PyPI stopped exposing this in main API; requires stats.pythonhosted.org)
- **NPM:** Response has version history accessible via npmPackageResponse.Versions (can infer version count)
- **Go:** Has timestamp for Age (already partially implemented)
- **Gem (RubyGems):** Response has no download stats (requires separate stats API)
- **CPAN:** Metadata available via MetaCPAN API

**Key Finding:** Data availability varies by registry. Not all registries expose download counts in their primary APIs. Some require secondary API calls or aren't exposed at all.

---

## Gaps and Ambiguities

### 1. Quality Signal Definition
**GAP:** The issue doesn't specify which signal(s) to use:
- Download count threshold? (e.g., < 50 downloads = squatter?)
- Version count? (e.g., < 3 versions = placeholder?)
- Content inspection? (Empty or tiny source code?)
- Combination of signals?
- Different thresholds per ecosystem?

**Recommendation:** Issue should clarify the heuristic strategy. Some ecosystems may need different criteria.

### 2. Data Availability
**GAP:** Issue doesn't address that quality signals aren't equally accessible:
- **Download counts:** PyPI discontinued in main API; Cargo doesn't expose; RubyGems doesn't expose; Go/NPM/CPAN could provide
- **Version history:** Most registries have this, but Go doesn't version at release level
- **Source code inspection:** Would require cloning repos (expensive, already done partially for Cargo/PyPI discovery)

**Recommendation:** Clarify which signal(s) are feasible and what falls back to if unavailable.

### 3. API Cost/Performance
**GAP:** Implementing quality checks may require additional API calls:
- Cargo: Would need separate download stats endpoint (new API call)
- PyPI: Would need stats.pythonhosted.org (external domain, new API call)
- RubyGems: Would need stats API (new API call)
- Cask: Limited stats available

The probers run in parallel with a 5-second timeout total. Additional blocking calls could hit timeout budgets.

**Recommendation:** Should specify whether parallel calls are acceptable or if serial composition is preferred.

### 4. Placeholder Package Identification
**GAP:** No clear definition of what constitutes a "placeholder" vs. "legitimate early-stage package":
- Should a package with 0 downloads be rejected? (Might be a newly published real tool)
- Should a package with 1 version be rejected? (Could be unmaintained but functional)
- Should we check if there's actual code? (metadata-only packages are common)

**Recommendation:** The issue mentions "checking if package has actual files/binaries" which is more specific than download count, but this needs elaboration.

### 5. Backwards Compatibility
**GAP:** Changing Probe() to return false for previously discovered packages will affect:
- Cached discovery results (may become stale)
- User scripts relying on ecosystem probe for tool discovery
- Error messages users see

**Recommendation:** Should clarify deprecation/migration strategy.

---

## Recommended Title (Conventional Commits)

**Primary Option:**
```
fix(discover): enhance ecosystem probe quality signals
```

**Alternative Options:**
1. `fix(discover): reject squatted packages in ecosystem probe`
2. `fix(discover): add quality heuristics to ecosystem package detection`
3. `feat(discover): implement package quality filtering in probes`

**Rationale:**
- Primary title emphasizes this is a bug fix (incorrect behavior) not a new feature
- "enhance quality signals" is clear and actionable
- Scope: `discover` is correct (affects EcosystemProbe + builders)
- Could use `feat` if preferred, but current behavior is objectively wrong for the prettier case, making `fix` more accurate

---

## Implementation Considerations

### Minimal Starting Point
Focus on what's actionable without perfect data:

1. **Version count heuristic** (available across all ecosystems):
   - Reject packages with < 2-3 releases
   - Simple to implement, catches most placeholder packages

2. **Age heuristic** (available across all ecosystems):
   - Reject packages < 7 days old
   - Prevents brand-new empty packages

3. **Description/metadata check** (already fetched):
   - Reject if name exactly matches but description is empty or generic
   - Already have this data in all builders

### What to Clarify with Submitter
1. Which quality signal(s) to prioritize given data availability constraints
2. Acceptable performance impact (additional API calls?)
3. Specific thresholds (e.g., min 2 versions, min 7 days old)
4. How to handle ecosystem-specific limitations (e.g., some registries don't expose downloads)
5. Whether to update the static priority map separately

---

## Assessment Summary

| Criterion | Rating | Notes |
|-----------|--------|-------|
| Problem clarity | ✓ Clear | Concrete example (prettier), specific file locations |
| Type classification | ✓ Bug | Current behavior is objectively wrong |
| Single issue scope | ✓ Yes | Coordinated changes across 7 builders, single purpose |
| Implementation feasibility | ⚠️ Medium | Requires design decisions on quality signals and thresholds |
| Data availability | ⚠️ Variable | Not all registries expose download counts equally |
| Gaps requiring closure | 3-5 | See section above |

---

## Recommended Next Steps

1. **Ask submitter** for clarification on:
   - Priority heuristics (version count? age? description? downloads?)
   - Acceptable thresholds
   - Performance constraints

2. **If proceeding without clarification**, default to:
   - Version count check (available everywhere, easy to implement)
   - Skip registries that don't have version data
   - Keep changes minimal and testable

3. **Consider separate follow-up** for:
   - Adjusting static priority map if version count heuristic still produces wrong results
   - Implementing download count checks (requires secondary API calls)
   - Cask-specific handling (limited stats availability)

