# Platform/Infrastructure Engineer Review: Recipe Registry Separation

## Executive Summary

The design proposes a significant shift from embedded recipes to a hybrid model with community recipes fetched from GitHub registry and golden files stored in Cloudflare R2. The R2 strategy is viable but requires substantially more rigor in implementation planning.

## 1. Scalability Analysis

**Golden File Storage at Scale:**
- 10K recipes × 2.4 variants = 24K files, ~380MB
- R2 storage cost: Within 10GB free tier
- R2 request cost at 500k requests/month: ~$20 (acceptable)

**Gap:** No documented performance testing above 1K recipes. No analysis of concurrent recipe fetches.

## 2. Cost Analysis

| Component | Estimated Cost |
|-----------|-----------------|
| R2 storage | $5-10/month |
| R2 requests | <$1/month |
| **TOTAL** | **<$20/month at 10K recipes** |

**Critical Unknown:** Cloudflare R2→GitHub Actions egress pricing is unclear. If charged, costs could spike.

**Recommendation:** Must clarify pricing with Cloudflare BEFORE Stage 7 implementation.

## 3. Data Consistency Risks

**Problem:** Golden files live in two places (R2 for CI, git for source). No explicit sync protocol.

**How it breaks:**
1. Developer commits recipe update
2. CI generates new golden file, uploads to R2
3. Upload fails silently
4. R2 has stale golden file, validation uses wrong expectation

**Missing:**
- When does upload happen? (On PR merge? After test passes?)
- Failure handling? (Retry policy? Notifications?)
- Rollback? (If recipe is reverted, do we revert R2 golden file?)

**Recommendation:** Add "Sync Protocol" section to R2 storage design document.

## 4. Performance Impact

**Nightly Validation Performance:**
- Sequential fetch of 24K golden files: ~80 minutes (unacceptable)
- Design doesn't address parallelization

**Recommendation:**
- Implement parallel R2 fetch (10 concurrent)
- Target: Fetch all golden files in <5 minutes
- Consider gzip compression (50% reduction)

## 5. Critical Infrastructure Gaps

### Gap 1: Network Failure Modes Not Addressed
- If GitHub is down during nightly validation, workflow fails with no fallback
- If R2 is down during user install, community recipe fails with "404"

### Gap 2: No Fallback for Offline Community Recipes
- Community recipes unavailable if GitHub is down
- Design says "offline installation works" but that's only critical recipes now

### Gap 3: No Disaster Recovery Procedure
- GitHub repository compromised: No security incident playbook
- R2 bucket deleted: No backup mentioned

## Top 5 Critical Concerns (Ranked)

1. **Unvalidated R2 Egress Cost** - Must clarify pricing before Stage 7
2. **Golden File Sync Protocol Missing** - No defined procedure for upload/fallback
3. **Nightly Validation Performance Unquantified** - Need parallel fetch strategy
4. **No Fallback for Offline Community Recipes** - Breaking change to user expectations
5. **Embedded Recipe Migration Decision Is Manual** - Risk of moving critical recipe by accident

## Overall Assessment

- **Design is strategically sound** but **implementation planning is incomplete**
- Cost and scalability are not blockers; operational maturity is
- Before proceeding to Stage 7, must address critical gaps 1, 2, and 3
