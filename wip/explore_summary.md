# Exploration Summary: R2 Golden Storage

## Problem (Phase 1)

Registry recipes are projected to scale to 10K+, making git-based golden file storage unsustainable. At current per-recipe averages (~2.7 golden files, ~16KB each), 10K recipes would produce ~27K files totaling ~420MB plus git history overhead. This creates slow clones, bloated diffs on version bumps, and unbounded repo growth. The solution must address not just storage, but the complete lifecycle: generation, upload/download, version management, cleanup of orphaned files, testing efficiency, and graceful degradation on failures.

## Decision Drivers (Phase 1)

- **Scalability**: Must handle 10K+ recipes with multiple platforms/versions without repo bloat
- **CI Reliability**: Nightly validation and PR workflows must not fail due to infrastructure issues
- **Automation**: Upload, cleanup, and version management must be fully automated
- **Cost Efficiency**: Stay within reasonable cost bounds (ideally free tier for initial scale)
- **Testing Efficiency**: Minimize redundant validation while maintaining quality
- **Security**: Credential management, access control, and audit logging
- **Graceful Degradation**: Fallback mechanisms when external storage unavailable
- **Migration**: One-time migration of existing golden files with rollback capability
- **Simplicity**: Infrastructure complexity must be maintainable by small team

## Research Findings (Phase 2)

Eight specialist agents analyzed different aspects of the problem:

### R2 Infrastructure
- S3-compatible API, use AWS CLI or rclone for bulk operations
- No OIDC support from Cloudflare; must use API tokens with quarterly rotation
- 10GB storage, 1M Class A ops, 10M Class B ops free tier
- At 30K files with 100 CI runs/month: stays within free tier

### CI Patterns
- Current architecture has 3-layer validation (recipe changes, execution, code changes)
- Dynamic matrix generation from changed files works well
- Append-only with immutable keys recommended for golden files
- Fan-out/fan-in pattern for cross-platform generation

### Data Lifecycle
- Tiered retention: latest always + previous major for 90 days + max 5 versions
- Orphan detection: recipe existence check + platform support check + weekly scan
- Safe cleanup: detection → grace period → soft delete → hard delete
- Recommended key structure: `plans/{category}/{recipe}/v{version}/{platform}.json`

### Testing Efficiency
- Current: 418 files, 60 min timeout for nightly
- Letter-based parallelism (26x) recommended for scale
- Stratified sampling (~1000 recipes) with weekly full validation
- macOS constraint: 10x Linux cost, batch into single jobs

### Security
- Separate read-only and read-write tokens
- Environment protection for write operations
- Bucket locks for WORM protection on release artifacts
- Quarterly credential rotation SOP needed

### Failure Modes
- Three-tier degradation: R2 → Fallback cache → Skip validation
- Health check using sentinel object with 5s timeout
- Git-based fallback cache at ~40MB compressed
- Circuit breaker pattern for R2 operations

### Automation
- Post-merge generation eliminates git bloat
- Weekly version detection workflow needed
- Phased migration: 5 phases over ~9 weeks

### Scalability
- Breaking points: Git at 1GB, matrix at 256 jobs, API at 5000/hr
- LFS viable up to 5K recipes
- R2 recommended for 5K-10K
- Architecture redesign needed at 50K+

## Options (Phase 3)

1. **Full R2 Migration with CI-Generated Golden Files** - CI generates on merge, R2 is sole source, git fallback for resilience. Best scalability, eliminates git bloat, but adds credential management complexity.

2. **Git LFS for Golden Files** - Store golden files in LFS, keep current workflow. Minimal changes, preserves PR review, but limited scale (5K max) and has bandwidth costs.

3. **Hybrid Archive** - Latest in git for review, older versions in R2. Preserves PR review but complex dual-system maintenance.

4. **On-Demand Generation** - Generate at validation time, no storage. Zero storage but prohibitive compute costs at scale (~$300+/mo).

## Decision (Phase 5)

**Problem:** Registry recipes scaling to 10K+ makes git-based golden file storage unsustainable due to repo bloat and slow clones.

**Decision:** Migrate registry golden files to Cloudflare R2 with CI-generated files on merge, two-tier degradation (R2 or skip), and 6-phase rollout.

**Rationale:** R2 eliminates git bloat while two-tier degradation keeps CI simple; CI generation removes contributor friction; free tier covers projected costs.

## Current Status

**Phase:** 5 - Decision Complete
**Last Updated:** 2026-01-24
**Next:** Phase 6 - Architecture (detailed solution design)
