# Release Engineer Review: Recipe Registry Separation

## Executive Summary

This design separates recipes into critical (embedded in CLI) and community (fetched from registry) categories. While the approach is sound in principle, the design has **5 critical gaps** that will cause release and operational problems if not addressed.

## Critical Concern #1: Undefined Migration Path for Existing Users

**Problem:**
- Current behavior: Users have recipes embedded in their CLI binary (no expiration, no network dependency)
- New behavior: Community recipes must be fetched on first use (new network dependency)
- **Missing:** What happens when a user with the v0.X binary (all embedded) upgrades to v0.Y+ (split recipes)?

**Mitigations Required:**
- Document in release notes that first `tsuku install` after upgrade fetches recipes (may be slow)
- Add upgrade scenario test: install tools with v0.9, upgrade to v0.10, verify tools still work
- Add verbose log message during first community recipe fetch

## Critical Concern #2: Rollback Scenarios Completely Undefined

**Problem:** No guidance on what happens if a community recipe update breaks users in production.

**Scenario:**
1. Community recipe PR merged with broken download URL
2. PR passes plan validation but not execution
3. 50,000 users cache broken recipe (TTL = 24 hours)
4. How does tsuku roll this back?

**Mitigations Required:**
- Add recipe-specific revocation system
- Implement cache invalidation trigger
- Add CI gate: community recipe changes must pass execution validation BEFORE merge
- Define emergency release process

## Critical Concern #3: Cache TTL Strategy is Incomplete

**Problem:** The design mentions cache TTL but doesn't address:
- What happens when TTL expires but network is unavailable?
- No staleness check mechanism defined

**Mitigations Required:**
- Explicit cache TTL design section
- Implement `tsuku update-registry --force-refresh` command
- Add telemetry to track cache hit rate and staleness

## Critical Concern #4: Critical Recipe Dependency Analysis is Unvalidated

**Problem:**
- Design estimates 15-20 critical recipes but provides NO ANALYSIS
- No dependency graph shown
- Acknowledgment that "Dependencies() infrastructure has known gaps (#644)"

**Mitigations Required:**
- BEFORE implementation: Run build-time script to extract all `Dependencies()` returns
- Generate definitive critical recipe list with dependency graph
- Add CI gate: code change to action's `Dependencies()` triggers validation

## Critical Concern #5: Community Recipe Breakage Not Detected At Release Time

**Problem:** A PR can merge that passes plan validation, then nightly runs 8+ hours later and finds 30 broken recipes.

**Mitigations Required:**
- Execution validation must run on community recipe PRs BEFORE merge
- Block releases if nightly validation is pending

## Top 5 Release-Critical Risks (Prioritized)

1. **Undefined Rollback for Broken Community Recipes** - No mechanism to revoke broken recipes after merge
2. **Unvalidated Critical Recipe Dependency Set** - 15-20 estimate unproven
3. **Community Recipe Breakage Goes Undetected for 12+ Hours** - Nightly validation runs after release is cut
4. **Undefined Migration Path for Existing Users** - What happens when v0.9 user upgrades to v0.10?
5. **Cache TTL Implementation is Deferred** - Critical gap in rollback strategy
