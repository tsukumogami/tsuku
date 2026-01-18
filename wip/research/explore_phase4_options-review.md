# Phase 4 Options Review

## Review Summary

The design is directionally sound but needs concrete specs on dependency resolution, offline behavior, and security practices.

## Key Findings

### 1. Problem Statement Assessment
The problem is specific and well-motivated (binary bloat, update coupling, CI burden), but lacks quantification. The design mentions "estimated 30-50% recipe content reduction" without baseline measurements.

### 2. Missing Alternatives
- No consideration for lazy-loading embedded recipes (compress selectively)
- No discussion of recipe versioning strategy for community recipes
- Missing: what happens to community recipes when network is unavailable after initial install?

### 3. Unstated Assumptions
- Assumes all action dependencies are captured by current Dependencies() methods (but `homebrew` has a TODO comment #644 about composite actions not properly aggregating dependencies)
- Assumes 15-20 critical recipes, but provides no actual dependency graph analysis
- Assumes GitHub is always available for community recipe fetches (no offline fallback design)

### 4. Critical Recipe List Incompleteness
- Missing: `nix-portable` is auto-bootstrapped but not a recipe
- Missing: potential transitive build tool dependencies
- The table lists actions but doesn't show which recipes provide them

### 5. Testing Strategy Issues
- Option 3A (Hash-Only) is a strawman: catches zero functional issues
- Option 3C (Plan-Only) has practical blind spots for platform-specific download failures
- Current 264 exclusions suggest complexity; splitting testing further increases burden

### 6. Security Gaps
- No mention of GitHub account compromise recovery
- Missing: cache poisoning mitigation
- No audit trail discussion for community recipe changes

## Top Recommendations

1. **Build actual dependency analysis**: Use Dependencies() infrastructure to compute transitive closure at build time

2. **Define community recipe failure modes explicitly**: Document network unavailability, 404s, version conflicts

3. **Option 3C needs enhancement**: Add nightly full test runs for community recipes plus cache invalidation policy
