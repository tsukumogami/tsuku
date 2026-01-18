# DevOps/CI Engineer Review: Recipe Registry Separation Design

**Reviewer Role:** DevOps/CI Engineer  
**Date:** 2026-01-18  
**Design:** Recipe Registry Separation (DESIGN-recipe-registry-separation.md)  
**Status:** Critical concerns identified - design needs operational hardening before implementation

---

## Executive Summary

This design introduces a distributed recipe system with three repository locations, split CI workflows, and external Cloudflare R2 storage for golden files. While the architectural direction is sound, the design significantly underestimates operational complexity. Key concerns:

1. **R2 outage cascades to all community recipe validation** - No fallback described
2. **Registry fetch failures will be user-facing** - Cache strategy is underdeveloped
3. **Workflow complexity increases 40-50%** - New CI orchestration burden
4. **No monitoring/alerting infrastructure** - Operational visibility gaps
5. **Cache poisoning and stale data risks** - Security boundaries unclear

**Recommendation:** Proceed with Stage 1-4, but defer Stage 7 (R2 integration) until operational concerns in this review are addressed with concrete implementation details.

---

## Top 5 Critical Concerns

### 1. R2 Outage Scenarios - No Graceful Degradation

**Problem:**
The design assumes Cloudflare R2 will store community golden files. Stage 7 is deferred as a separate design, but the nightly validation workflow (Stage 4, Step 5) depends on downloading golden files from R2:

```
nightly-community-validation.yml
  → Runs validate-all-golden.sh for community recipes
  → Downloads golden files from R2 bucket
  → If R2 is down: entire nightly validation fails
```

**Impact:**
- **Nightly R2 outage**: No community recipe validation for 24 hours. Users deploying community recipes may have broken installations that go undetected.
- **Cascade effect**: If R2 is down during a PR merge, community recipe change validation cannot complete. The PR workflow (Stage 4, Step 1-2) runs locally via GitHub, but validation against R2-stored golden files fails.
- **No incident response**: Design doesn't describe what to do when R2 is unavailable. Do we skip validation? Create noise in GitHub issues? Fail loudly?

**Mitigation Gaps:**
- No fallback to stale git-cached golden files
- No health check before CI starts
- No graceful skip with warning log
- No alerting for R2 unavailability

**Recommended Implementation:**
1. **Health check in CI**: Before nightly validation starts, `curl -I` check R2 bucket. If unreachable, skip validation and create GitHub issue titled "⚠️ Nightly validation skipped: R2 unreachable"
2. **Git-based cache**: Keep last 3 months of golden file changes in git (compressed tarballs in `.github/cache/`) for fallback during R2 outage
3. **Explicit monitoring**: Add CloudFlare API integration to GitHub Actions to proactively detect zone issues
4. **Timeout configuration**: Set 30-second timeout on all R2 requests; fail fast instead of hanging

**Effort:** Medium (new GitHub Actions workflow, R2 API integration, git cache management)

---

### 2. Cache Invalidation Strategy - Incomplete Design

**Problem:**
Stage 5 describes cache policy superficially. Critical gaps:

```
Stage 5 proposes:
- 24-hour default TTL (configurable)
- tsuku update-registry command
- --force flag on install
```

But implementation is missing critical details:

1. **What happens on cache miss + network failure?**
   - User runs `tsuku install fzf` on machine with no internet
   - Local cache expired or cleared
   - GitHub registry unreachable
   - Design says: "User requests recipe... Fetch from registry → found"
   - Reality: What error message? Can they retry? Does it hang for 30s?

2. **Stale cache policy during development:**
   - Developer pushes recipe change to community registry
   - CI workflow validates immediately (uses fresh fetch)
   - User runs `tsuku install` 1 minute later
   - User gets cached old version for 24 more hours
   - User doesn't know why their recipe change isn't visible
   - No mitigation described

3. **Cache location and size limits:**
   - `$TSUKU_HOME/registry/` will accumulate 150+ community recipes
   - No cleanup described. What if user runs `tsuku install` 100 times across different recipes?
   - Unbounded disk usage possible

**Recommended Implementation:**
1. **Network failure fallback**:
   ```
   Fetch from GitHub
     → Success: return, cache with timestamp
     → Timeout (30s): Check local cache
       → Cache exists: return with warning "Recipe may be stale (cached X hours ago)"
       → Cache missing: fail with "No internet access and no cached recipe available"
   ```

2. **Stale cache notification**: Add metadata to cached recipes including source commit hash. Suggest `tsuku update-registry` if local hash differs from latest.

3. **Cache size management**:
   - Implement LRU cache with 500MB default limit (configurable)
   - Periodically prune old entries (`tsuku cache-cleanup` subcommand)
   - Warn user when cache approaches limit

4. **Cache verification**: When cache is used due to network failure, verify checksums of downloaded tools against recipe hash (add `verify_hash` field to cached recipe metadata)

**Effort:** Medium-High (error handling, manifest formats, cleanup logic)

---

### 3. CI Workflow Complexity Increases 40-50% Without Operational Framework

**Problem:**
Current state: 4 workflows + integration test matrix = moderate complexity  
Post-design state: 5-6 workflows + split golden file directories + R2 integration = significant complexity

New operational burden:

```
Current Workflows:
1. test-changed-recipes.yml
2. validate-golden-recipes.yml
3. validate-golden-code.yml
4. validate-golden-execution.yml
Plus: test-matrix.json

New Workflows (with this design):
1. test-changed-recipes.yml (updated)
2. validate-golden-recipes.yml (updated)
3. validate-golden-code.yml (scoped to critical only - new logic)
4. validate-golden-execution.yml (updated for dual directories)
5. nightly-community-validation.yml (new)
6. (Future) r2-golden-file-sync.yml (Stage 7)
Plus: test-matrix.json (updated)
Plus: Separate critical/community golden file directories
Plus: New exclusions structure
```

**Orchestration Gaps:**
1. **Workflow interdependency not defined**: What if test-changed-recipes.yml fails but validate-golden-execution.yml succeeds? Do we block merge?
2. **Nightly failure recovery**: Nightly validation runs at 2 AM UTC and fails. Who gets paged? What's the runbook?
3. **Golden file sync race condition**: PR merges community recipe → triggers test-changed-recipes.yml → generates new golden file → needs R2 upload. But R2 upload workflow not defined until Stage 7. Incomplete.
4. **No workflow health dashboard**: How do we know nightly validation is running? When did it last succeed?

**Recommended Implementation:**
1. **Workflow dependency graph**: Document in `.github/workflows/README.md`:
   ```
   PR Workflow:
   - test-changed-recipes.yml (parallel, required for merge)
   - validate-golden-recipes.yml (parallel, required for merge)
   - validate-golden-code.yml (only for critical, required for merge)
   
   Completion Workflow (post-merge):
   - nightly-community-validation.yml (informational, creates issue on failure)
   - (Stage 7) r2-golden-file-sync.yml (parallel)
   ```

2. **Nightly failure notification**: Update GitHub branch protection rules to alert on nightly workflow failures via Slack or email

3. **Workflow status dashboard**: Add `.github/workflows/status.md` file that updates with latest workflow runs (automated via Actions)

4. **Workflow timeout management**:
   - Critical recipes validation: 10 min timeout
   - Community recipes validation: 30 min timeout (nightly only, less urgent)
   - R2 operations: 5 min timeout with fallback

**Effort:** Medium (documentation, monitoring setup, timeout tuning)

---

### 4. Monitoring Gaps - Operational Blindness Post-Launch

**Problem:**
Design provides zero monitoring/alerting strategy. Questions with no answers:

1. **How do we know nightly validation is running?**
   - If GitHub Actions fails silently (out of disk, secret rotation issue), who finds out?
   - Answer: Nobody until a user complains about broken community recipe
   - No SLA defined for community recipe availability

2. **What metrics do we track?**
   - Community recipe fetch latency? (impacts user install speed)
   - Cache hit ratio? (indicates cache strategy effectiveness)
   - R2 upload/download success rate? (operational health indicator)
   - Community recipe test coverage (% with passing nightly validation)?

3. **Where do alerts go?**
   - Nightly validation fails: GitHub issue created, but is anyone monitoring?
   - R2 bucket runs low on quota: No alerting described
   - Community recipe fetch timeouts spike: No trend analysis

4. **No SLO defined:**
   - What's the acceptable delay for a community recipe to be available after PR merge?
   - What's the downtime tolerance for community recipe registry?
   - What's acceptable cache staleness?

**Recommended Implementation:**
1. **Monitoring points to instrument**:
   - Workflow completion (nightly-community-validation.yml status)
   - Recipe fetch latency (add timing to loader)
   - Cache hit/miss ratio (prometheus metrics)
   - R2 API error rates
   - Golden file test pass rate by category (critical vs community)

2. **Alert conditions**:
   - Nightly validation fails 2 times in a row → Slack alert
   - Recipe fetch timeout > 10 seconds → Log event (detect external slowness)
   - R2 quota > 80% → Email alert
   - Community recipe nightly coverage < 95% → Warning in PR

3. **Dashboard**: Add GitHub action that updates `.github/community-recipes-status.md` after nightly run:
   ```markdown
   # Community Recipe Validation Status
   Last nightly run: 2025-01-18 02:00 UTC
   Passed: 145/150
   Failed: 5 (links to issues)
   R2 Bucket: 185MB / 1GB
   ```

4. **Post-mortem template**: Create `.github/nightly-failure-runbook.md`:
   ```
   1. Check nightly-community-validation.yml run logs
   2. Identify which recipes failed (sorted by alphabetical)
   3. Check if it's transient (R2, network) or persistent (recipe broken)
   4. Create GitHub issue for each persistent failure
   5. Notify maintainers in #tsuku-alerts Slack channel
   ```

**Effort:** Medium (instrumentation, dashboard automation, alerting integration)

---

### 5. Secrets and Authentication - R2 Credentials in CI

**Problem:**
Design mentions R2 storage but completely omits authentication details. Critical questions:

1. **How do GitHub Actions authenticate to R2?**
   - Option A: API token stored in `GITHUB_TOKEN` + R2 credentials
   - Option B: Temporary credentials via OIDC from GitHub
   - Option C: Cloudflare API token stored as GitHub secret
   - Design mentions none of these

2. **Who has R2 write access?**
   - If any GitHub Actions workflow can write, a compromise of tsuku repo grants R2 write access
   - Should only scheduled nightly workflow write? PR workflows only read?
   - Design doesn't describe access control model

3. **Credential rotation**:
   - When should R2 API keys rotate? (current best practice: every 90 days)
   - Who manages rotation? (ops team? automated?)
   - Design doesn't describe

4. **Audit logging**:
   - When golden files are uploaded to R2, who did it? When? Which version?
   - No S3 API logging strategy described
   - Risk: if R2 is compromised, we won't have audit trail

**Recommended Implementation:**
1. **Authentication method**: Use GitHub OIDC with Cloudflare API tokens:
   ```yaml
   - name: Get R2 credentials via OIDC
     uses: aws-actions/configure-aws-credentials@v2
     with:
       role-to-assume: arn:cloudflare:...
       aws-region: auto
   ```

2. **Access control**:
   - `nightly-community-validation.yml`: READ-ONLY from R2 (download golden files)
   - `r2-golden-file-sync.yml` (Stage 7): WRITE to R2 (upload after PR merge)
   - Regular PR workflows: NO direct R2 access (reduced blast radius)

3. **Credential rotation process**:
   - Document in CONTRIBUTING.md: "Rotate R2 credentials quarterly (Jan/Apr/Jul/Oct)"
   - Create GitHub issue reminder 2 weeks before
   - Store previous keys for 30 days (allows gradual migration)

4. **Audit logging**:
   - Enable Cloudflare R2 audit logs to separate storage (S3, GCS)
   - Keep 90-day retention
   - Alert if API calls spike (potential exfiltration attempt)

5. **Secret scanning**:
   - Add `.github/workflows/secret-scan.yml` to detect exposed credentials before merge
   - Use `truffleHog` or `gitleaks` on every PR

**Effort:** Medium (OIDC setup with Cloudflare, audit logging integration, secret scanning)

---

## Secondary Concerns

### 6. testdata/recipes/ Duplication Maintenance Risk

**Problem:**
Stage 3 proposes creating 6 new recipes in `testdata/recipes/` for integration testing:
- netlify-cli (tests npm_install)
- ruff (tests pipx_install)
- cargo-audit (tests cargo_install)
- bundler (tests gem_install)
- ack (tests cpan_install)
- gofumpt (tests go_install)

These are simplified versions of production recipes. Maintenance risk:

1. **Recipe drift**: When production netlify-cli recipe updates, testdata/recipes/netlify-cli.toml doesn't automatically update. Over time, test diverges from reality.
2. **Action behavior changes**: If npm_install action changes, test recipes need updating. Who updates them? Who tracks this?
3. **No cross-reference**: No documentation linking production ↔ test recipes

**Recommended Implementation:**
1. **Single-source-of-truth**: Add `source_recipe` metadata to test recipes:
   ```toml
   [metadata]
   name = "netlify-cli"
   source_recipe = "recipes/n/netlify-cli.toml"  # Link to production
   ```

2. **Validation check**: Add CI step that verifies test recipes haven't diverged from production:
   ```bash
   diff <(jq .actions testdata/recipes/netlify-cli.toml) \
        <(jq .actions recipes/n/netlify-cli.toml)
   ```

3. **Update changelog**: When production recipe changes, require PR author to update test recipe (or mark `[skip-test-sync]` if intentional)

**Effort:** Low (metadata field, CI validation, PR template update)

---

### 7. Community Recipe Test Coverage Gap

**Problem:**
Nightly validation only runs for community recipes with golden files in R2. But:

1. **What about new community recipes?** After PR merge, before first nightly run:
   - Recipe exists in registry
   - No golden file yet (nightly hasn't run)
   - Users can install it, but no CI validation
   - If broken, users will report it before CI catches it

2. **Golden file generation workflow undefined**: When does the nightly job create and upload golden files? Design assumes they exist but doesn't describe bootstrap.

3. **PR validation gap**: Community recipe changes are validated (Stage 4, Step 1-2), but only against local golden files (if they exist). If recipe is brand new, no golden file to validate against.

**Recommended Implementation:**
1. **Bootstrap workflow**: On PR merge of new community recipe:
   - Trigger `generate-golden-files.yml` workflow (separate from nightly)
   - Generates golden files for all platforms
   - Uploads to R2
   - PR validation runs against newly generated files

2. **PR validation for new recipes**: 
   ```yaml
   - name: Validate new community recipes
     run: |
       for recipe in $(git diff --name-only HEAD~1 | grep ^recipes/); do
         tsuku plan --recipe "$recipe" --pin-from <(last-golden-file)
       done
   ```

3. **Golden file initialization**: Document in CONTRIBUTING.md that new recipes need at least one plan generation run before they're "production ready"

**Effort:** Low (documentation, workflow trigger update)

---

### 8. Security: Cache Poisoning Attack Surface

**Problem:**
The caching strategy introduces a new attack vector. Current model (all embedded):
- Attack requires: PR approval + merge + new release + user update
- Time window: weeks to months

New model (community recipes cached locally):
- Attack sequence:
  1. Compromise GitHub account / repository
  2. Merge malicious community recipe update
  3. User runs `tsuku install` within 24 hours
  4. User's machine compromised before PR is reverted
- Time window: minutes to hours
- Detection: Nightly validation catches it, but 24 hours after impact

Additionally:

1. **Cache persistence**: If user clears cache after incident, when does new recipe fetch? Immediately? Or wait 24h?
2. **No recipe signature verification**: Design explicitly says "no signing" (line 788). This means:
   - Cache poisoning + GitHub compromise = user compromise without cryptographic detection
   - Users can't verify recipe integrity independently

**Recommended Implementation:**
1. **Incident response SOP**: 
   - Document in `.github/SECURITY.md`: "If repository compromised, run `tsuku cache-clear` to remove stale recipes"
   - Create security advisory template
   - Test incident response quarterly

2. **Future enhancement (not in this design)**: Recipe signing with COSE/Sigstore
   - Maintain recipe signatures in separate git branch
   - Verify recipes on download
   - Protects against GitHub compromise

3. **Audit trail**: Log all recipe fetches to `~/.tsuku/audit.log`:
   ```
   2025-01-18 10:30:45 FETCH recipes/f/fzf.toml commit=abc123
   2025-01-18 10:30:46 FETCH recipes/f/fzf.toml from CACHE (age 2h)
   ```
   Users can inspect what changed when incident occurs

**Effort:** Low-Medium (documentation, logging)

---

### 9. Network Dependency Increases User Surface Area

**Problem:**
Current state: First install of critical recipes fails offline, but community recipes don't exist yet  
New state: First install of ANY community recipe requires network

This changes the value proposition. Design doesn't address:

1. **Offline-first capability**: If design emphasizes "self-contained package manager", how is recipe registry compatible?
2. **User expectation mismatch**: New user installs tsuku, tries `tsuku install fzf` offline, gets "network required" error. They don't know if fzf is critical or community.
3. **Latency impact**: Every first install of community recipe adds ~500ms-2s network latency. Design doesn't measure or discuss.

**Recommended Implementation:**
1. **Documentation**: Add to `--help` output and README.md:
   ```
   Critical recipes (Go, Rust, etc.) work offline.
   Community recipes require network access (cached after first install).
   ```

2. **Error messages**: Make it clear whether network failure is expected:
   ```
   Fetching community recipe 'fzf' from registry...
     (This recipe requires network access; future installs will use cache)
   
   Error: Network unreachable
   Retry with: tsuku install --force fzf
   Or run: tsuku update-registry (to pre-cache all recipes)
   ```

3. **Latency measurement**: Instrument recipe fetch timing and report in `tsuku info`:
   ```
   $ tsuku info fzf
   Category: Community (cached: 2 days ago)
   Fetch latency: 0.8s (median from last 10 fetches)
   ```

**Effort:** Low (documentation, instrumentation)

---

### 10. Binary Size Impact Unvalidated

**Problem:**
Design claims "estimated 30-50% recipe content reduction" (line 838) but provides zero measurement:

1. **Current binary size**: Not measured in design
2. **Expected post-separation size**: Not calculated
3. **Which recipes account for most bloat**: Unknown (Go? Rust? Homebrew?)
4. **Actual impact on users**: Unknown (how many users on slow networks?)

This is critical because binary size is a key selling point of the separation.

**Recommended Implementation:**
1. **Baseline measurement**: Before implementation:
   ```bash
   go build -o tsuku ./cmd/tsuku
   ls -lh tsuku  # Capture baseline
   ```

2. **Per-recipe size analysis**:
   ```bash
   # Calculate size of each embedded recipe
   for recipe in internal/recipe/recipes/**/*.toml; do
     size=$(wc -c < "$recipe")
     echo "$recipe: $size bytes"
   done | sort -k2 -rn | head -20
   ```

3. **Post-implementation validation**: After Stage 1, re-measure binary size and document:
   - Before: X MB
   - After: Y MB
   - Reduction: Z%
   - Top 5 critical recipes by size

4. **Performance impact**: Measure install latency:
   - Critical recipe (embedded): install latency
   - Community recipe (fetched): install latency
   - Report in telemetry for trend tracking

**Effort:** Low (measurement, documentation)

---

## Implementation Readiness Checklist

**Before starting implementation:**

- [ ] Define R2 fallback behavior (in writing, decision doc)
- [ ] Document cache invalidation policy with explicit error handling
- [ ] Create CI workflow dependency graph in `.github/workflows/README.md`
- [ ] Design monitoring/alerting infrastructure (what metrics, where alerts go)
- [ ] Define R2 authentication method and access control model
- [ ] Establish incident response procedures for:
  - [ ] R2 outage
  - [ ] Repository compromise
  - [ ] Community recipe breakage (nightly failure)
- [ ] Create golden file bootstrap workflow
- [ ] Validate binary size impact (establish baseline before Stage 1)
- [ ] Document cache behavior and offline limitations in user-facing docs

**Estimate:** 1-2 weeks additional design work (before implementation begins)

---

## Recommendations by Implementation Stage

| Stage | Recommendation | Rationale |
|-------|---|---|
| 1 (Recipe Migration) | **Proceed** | Low risk, additive only. Requires cache policy documentation first. |
| 2 (Golden File Reorg) | **Proceed with notes** | Update `.github/workflows/README.md` with new golden file structure before migration. |
| 3 (Integration Tests) | **Proceed** | Requires testdata/recipe sync policy (concern #6). Document single-source-of-truth linking. |
| 4 (CI Workflow Updates) | **Proceed with caution** | HIGH DEPENDENCY: All monitoring gaps (concern #4) and workflow orchestration (concern #3) must be addressed in this stage. |
| 5 (Cache Policy) | **Implement fully** | Currently underdeveloped (concern #2). Expand Stage 5 with network failure handling, size management, verification. |
| 6 (Documentation) | **Proceed** | Must include all security considerations (concern #8) and offline limitations (concern #9). |
| 7 (R2 Integration) | **Defer pending design** | BLOCKING: All R2 concerns (1, 4, 5) require separate tactical design BEFORE starting this stage. Requires: authentication design, monitoring design, fallback design, incident response procedures. |

---

## Open Questions for Design Team

1. **R2 Fallback**: Should nightly skip validation or use compressed git cache when R2 is down?
2. **Cache Eviction**: When community recipe cache exceeds 500MB, which recipes should be evicted? (LRU? Least-used? Manual?)
3. **Offline Workflows**: Should there be a `tsuku update-registry` command to pre-cache all recipes before going offline?
4. **SLOs**: What's the acceptable time for a merged community recipe to be available for users? (1 hour? 24 hours?)
5. **Nightly Failure Escalation**: If nightly validation fails 3+ times, should we automatically create a P1 incident or just a GitHub issue?
6. **Binary Size Target**: What's the success metric for "smaller binary"? 30%? 50%? Should this be measured in release notes?

---

## Risk Summary

| Risk | Severity | Mitigation Owner | Status |
|------|----------|------------------|--------|
| R2 outage → no validation for 24h | HIGH | DevOps | *Not Addressed* |
| Network failure → user-visible breakage | HIGH | Backend/CLI | *Partially Addressed* |
| Workflow orchestration complexity | MEDIUM | DevOps | *Not Addressed* |
| Cache poisoning attack surface | MEDIUM | Security | *Documented, not mitigated* |
| Operational monitoring blindness | MEDIUM | DevOps | *Not Addressed* |
| Recipe test duplication drift | LOW | Backend | *Not Addressed* |
| R2 authentication/secrets | MEDIUM | DevOps/Security | *Not Addressed* |

---

## Conclusion

The architectural vision is sound: separating critical and community recipes enables CLI binary optimization and independent recipe updates. However, the operational design is premature and introduces new failure modes that aren't adequately addressed.

**Recommendation:** Proceed with Stages 1-4 (recipe migration and CI restructuring), but:

1. Complete monitoring/alerting design before Stage 4 goes live
2. Defer Stage 7 (R2 integration) pending separate tactical design addressing all R2/authentication/fallback concerns
3. Expand Stage 5 (cache policy) with comprehensive error handling
4. Add operational documentation in CONTRIBUTING.md and `.github/` before any PR merge

The 1-2 weeks of additional design work will prevent 2-3 months of operational incidents post-launch.
