# Security Review: DESIGN-seed-queue-pipeline.md

## Review Metadata

- **Design Document**: `docs/designs/DESIGN-seed-queue-pipeline.md`
- **Status**: Proposed
- **Review Date**: 2026-01-30
- **Reviewer Role**: Security Analysis
- **Threat Model Context**: CI pipeline seeding a priority queue from Homebrew analytics (metadata only, no binaries)

## Executive Summary

The design correctly identifies that this pipeline processes metadata, not executable artifacts, which significantly reduces the attack surface. However, the security analysis is thin in several areas. Most notably: (1) the "not applicable" dismissal of download verification overlooks metadata injection attacks, (2) supply chain risk mitigation relies on downstream validation without considering denial-of-service scenarios, and (3) the direct-commit pattern lacks safeguards against schema validation bypass.

The residual risks are acceptable for a metadata pipeline, but additional guardrails would improve defense-in-depth.

## Review Questions

### 1. Are there attack vectors we haven't considered?

**Overall: YES, 4 attack vectors need analysis**

#### Identified Attack Vectors

The design identifies:
1. **Compromised Homebrew API** injecting misleading package names
2. **HTTPS interception** of API requests
3. **Stale data** after API outage

These are reasonable, but incomplete.

#### Additional Attack Vectors

**A. Malicious Package Name Injection (Typosquatting)**

The design mentions this briefly ("typosquatted names designed to trick the batch pipeline") but underestimates the risk.

**Attack scenario**:
1. Attacker compromises Homebrew analytics API (or MitM the request)
2. Injects fake package entries with names similar to popular tools:
   - `node-js` instead of `node`
   - `postgresq1` (with a "1") instead of `postgresql`
   - `aws--cli` (double dash) instead of `aws-cli`
3. Seed script writes these to `priority-queue.json`
4. Batch pipeline processes them, creates recipes, and possibly installs them for testing
5. Recipes might point to malicious repositories or binaries

**Current mitigation**: "The batch pipeline validates every generated recipe through its own gates (schema validation, sandbox testing)"

**Problem**: The validation happens *after* the batch pipeline has already:
- Cloned potentially malicious git repos (supply chain risk)
- Executed recipe generation logic (could trigger code execution if recipe generator has bugs)
- Wasted CI resources on fake packages (DoS)

**Recommendation**: Add a **package name allowlist or validation step** in the seed script:
- Maintain a list of known-good Homebrew formula names (or fetch it from Homebrew's official formula repo, not the analytics API)
- Filter out package names that don't match expected patterns (e.g., reject names with typo-common substitutions)
- Add a "confidence score" field to queue entries so the batch pipeline can prioritize validated packages

**B. Queue File Poisoning via Script Bug**

The design says schema validation prevents bad data from landing, but what if the seed script has a bug that generates schema-valid but semantically malicious data?

**Attack scenario**:
1. Bug in merge logic causes status fields to flip (e.g., marks completed packages as pending, wasting CI time)
2. Bug in tier assignment puts low-priority packages in tier 1 (batch pipeline prioritizes junk)
3. Bug in timestamp handling causes `updated_at` to be set to a date in the past (monitoring thinks queue is stale)

**Current mitigation**: Schema validation (prevents malformed JSON, not semantic bugs)

**Problem**: Schema validation is necessary but not sufficient. A schema can't encode "tier 1 must contain <50 packages" or "status transitions must follow this state machine."

**Recommendation**: Add **semantic validation** to `validate-queue.sh`:
- Check package counts per tier (tier 1 should be ~35 packages, tier 2 should be hundreds)
- Verify status transitions (if a package was "success" before, it can't become "pending" again)
- Sanity-check timestamps (updated_at must be recent, added_at can't be in the future)
- Fail if >10% of packages change status in a single update (indicates merge bug)

**C. Direct Commit Without Review Creates Backdoor Risk**

The design accepts direct commits to main as a trade-off: "acceptable for schema-validated data files."

**Attack scenario**:
1. Attacker compromises a maintainer's GitHub account (or gains Actions write permissions via another vulnerability)
2. Modifies the seed workflow to inject malicious entries into `priority-queue.json`
3. Schema validation still passes (data is structurally valid)
4. Commit lands on main without review
5. Batch pipeline processes malicious entries

**Current mitigation**: Workflow runs with `GITHUB_TOKEN` (standard permissions), schema validation before commit

**Problem**: Once an attacker has workflow edit permissions, they can:
- Modify the workflow to skip validation
- Modify the validation script to always pass
- Modify the seed script to inject data while keeping schema valid

**Recommendation**: Add **workflow integrity checks**:
- Use GitHub's required status checks to prevent workflow files from being modified without review
- Consider requiring PR reviews for `.github/workflows/*.yml` changes (separate branch protection rule)
- Add a checksum or signature verification step: the workflow verifies `seed-queue.sh` and `validate-queue.sh` haven't been modified from known-good versions

**D. API Response Size Explosion (DoS)**

The Homebrew analytics API returns thousands of packages. What if an attacker (or a bug) causes the API to return millions of entries?

**Attack scenario**:
1. Homebrew API is compromised or buggy
2. Returns a 1GB JSON response with 10M package entries
3. Seed script attempts to parse and merge this data
4. Workflow runs out of memory or times out
5. If retry logic is naive, subsequent runs repeat the failure, blocking automation

**Current mitigation**: None specified. The script has `--limit` but the workflow sets it to 100 by default (doesn't protect against malicious API responses).

**Recommendation**: Add **response size limits** to the seed script:
- Reject API responses >10MB (Homebrew's analytics data shouldn't be that large)
- Reject responses with >100K package entries (Homebrew has ~6K formulae)
- Add these checks *before* parsing JSON (check `Content-Length` header or stream size)

### 2. Are the mitigations sufficient for the risks identified?

**Overall: PARTIALLY, with gaps in defense-in-depth**

#### Risk 1: Misleading Package Names

**Identified mitigation**: "Batch pipeline validates all recipes independently"

**Sufficiency**: Partial. This catches some attacks (malformed recipes, unsafe binaries) but not all (typosquatted names that point to real but malicious packages).

**Gap**: No validation at the queue seeding stage. The seed script trusts the Homebrew API completely.

**Recommendation**: Add package name validation (see attack vector A above).

#### Risk 2: API Response Interception

**Identified mitigation**: "All API calls use HTTPS. GitHub Actions runners connect through GitHub's network."

**Sufficiency**: Adequate for most scenarios. HTTPS prevents passive eavesdropping and basic MitM attacks.

**Gap**: Doesn't protect against compromised CA or state-level attackers. However, this is an accepted baseline risk for any CI system pulling external data.

**Additional consideration**: The script uses `curl` with default settings. Should it explicitly verify certificates or pin Homebrew's certificate?

**Recommendation**: Add a note in the security section: "Certificate pinning is not used because Homebrew's infrastructure may change. If Homebrew API compromise is a concern, consider fetching formula metadata from the official Homebrew repository (git clone) instead of the analytics API."

#### Risk 3: Direct Push Without Review

**Identified mitigation**: "Schema validation before commit"

**Sufficiency**: Weak. Schema validation is a syntactic check, not a semantic or security check.

**Gap**: As noted in attack vector C, an attacker who can modify the workflow can bypass validation.

**Recommendation**: Add workflow integrity checks (see attack vector C).

#### Risk 4: Stale Data After API Outage

**Identified mitigation**: "Workflow fails and retries on next run"

**Sufficiency**: Adequate. Stale data is a reliability issue, not a security issue. The mitigation (retry on next scheduled run) is reasonable.

**Additional consideration**: If the queue becomes very stale (e.g., 30+ days), the batch pipeline might waste time on packages that are no longer popular. Consider adding a staleness warning to the batch pipeline.

**Recommendation**: No changes needed for this risk.

### 3. Is there residual risk we should escalate?

**Overall: YES, 1 significant residual risk**

#### Residual Risk 1: Malicious Data in Queue Wastes CI Resources (DoS)

**Description**: Even with all proposed mitigations, an attacker who compromises the Homebrew API can inject fake package names that pass validation (because they're structurally valid and match expected patterns). The batch pipeline will waste CI time processing them.

**Severity**: Medium. This doesn't compromise security directly (batch pipeline has its own sandboxing), but it wastes resources and could be used to delay processing of legitimate packages.

**Mitigation strategy**:
1. **Rate limiting**: Batch pipeline processes N packages per run, so a few bad entries won't block everything
2. **Circuit breaker**: If the batch pipeline encounters multiple consecutive failures (e.g., 5 packages in a row fail validation), pause processing and alert operators
3. **Monitoring**: Track batch pipeline success rate and alert if it drops below a threshold (e.g., <80%)

**Should this be escalated?**: No, but it should be documented in the design's "Consequences" section. Add a negative consequence: "Compromised Homebrew API can inject bad entries that waste CI time."

#### Residual Risk 2: Schema Evolution Breaks Merge Logic

**Description**: When `priority-queue.schema.json` is updated (e.g., adds a required field), the seed script's merge logic might produce output that fails validation because old entries lack the new field.

**Severity**: Low. This is a reliability issue, not a security issue, but it could cause the automation to break silently.

**Mitigation strategy**: Add schema version checking to the merge logic (see architecture review).

**Should this be escalated?**: No. Document as a consequence: "Schema changes require updating the seed script's merge logic."

#### Residual Risk 3: Attacker with Workflow Edit Permissions Has Full Control

**Description**: If an attacker gains the ability to modify `.github/workflows/seed-queue.yml`, they can bypass all mitigations (validation, schema checks, etc.) and inject arbitrary data.

**Severity**: High in isolation, but this risk exists for all CI workflows. It's not specific to this design.

**Mitigation strategy**: Standard GitHub security practices:
- Require PR reviews for workflow changes
- Use branch protection rules
- Enable audit logging
- Limit who has write access to the repository

**Should this be escalated?**: No, but add a note in the security section: "This design assumes standard GitHub repository security practices are in place (branch protection, review requirements, audit logging)."

### 4. Are any "not applicable" justifications actually applicable?

**Overall: YES, 2 dismissals are too broad**

#### "Download Verification: Not applicable"

**Justification in design**: "The seed script fetches popularity metadata (package names, download counts) from the Homebrew analytics API. It doesn't download any binaries or executable artifacts."

**Problem with justification**: While it's true that no binaries are downloaded, the dismissal misses the point. "Download verification" in a security context means "verify the authenticity and integrity of fetched data," not just "check binary signatures."

**What should be verified**:
1. **API response authenticity**: Are we really talking to `formulae.brew.sh` or a MitM proxy?
2. **API response integrity**: Has the response been tampered with in transit?
3. **API response freshness**: Is the data recent or stale?

**Current state**: HTTPS provides (1) and (2) at the transport layer. (3) is not verified (the seed script doesn't check if the API response includes a timestamp).

**Recommendation**: Change the section title to "Supply Chain Verification" and add:
- "HTTPS provides transport-layer authenticity and integrity. Certificate pinning is not used because Homebrew's infrastructure may change."
- "API response freshness is not verified. The workflow assumes data returned by the API is current."
- "If Homebrew API compromise is a concern, consider fetching metadata from the official Homebrew formula repository (git clone) and computing popularity metrics locally."

#### "User Data Exposure: Not applicable"

**Justification in design**: "The script reads from a public API and writes to a file in the repository. No user data is accessed, collected, or transmitted."

**Problem with justification**: This is mostly correct, but there's a subtle issue. The Homebrew analytics API returns *aggregate* user data (installation counts). While this isn't PII, it's still user-derived data.

**Nuance**: If tsuku later adds telemetry or user feedback to the priority queue (e.g., "users who installed this tool also requested these recipes"), that would be user data. The "not applicable" dismissal might discourage thinking about this.

**Current state**: Truly not applicable for the current design.

**Recommendation**: Add a future-proofing note: "Currently not applicable. If user telemetry or feedback is added to the priority queue in the future, this section must be revisited to address privacy and data retention concerns."

## Additional Security Considerations

### Missing: Input Validation on Workflow Parameters

The workflow accepts a `limit` input (number, default 100). Questions:

1. What happens if `limit` is set to -1, 0, or 999999999?
2. Can this be used to DoS the API or the workflow?

**Recommendation**: Add input validation to the workflow:

```yaml
inputs:
  limit:
    type: number
    required: false
    default: 100
    min: 1
    max: 10000  # Reasonable upper bound
```

If GitHub Actions doesn't support min/max validation (it doesn't as of 2025), add validation in the workflow steps:

```bash
if [ "$LIMIT" -lt 1 ] || [ "$LIMIT" -gt 10000 ]; then
  echo "Error: limit must be between 1 and 10000"
  exit 1
fi
```

### Missing: Audit Logging

The design doesn't mention how to audit who triggered the workflow or what data was committed.

**Recommendation**: Add an "Audit Logging" subsection:
- "Workflow runs are logged in GitHub Actions history (viewable at `https://github.com/<org>/<repo>/actions`)"
- "Commits include the `github-actions[bot]` user, making them distinguishable from manual commits"
- "To audit changes to `priority-queue.json`, use `git log -- data/priority-queue.json`"

### Missing: Incident Response Plan

If the seed workflow is compromised or injects bad data, what's the response plan?

**Recommendation**: Add an "Incident Response" subsection:
- "If malicious data is detected in `priority-queue.json`, revert the commit immediately and disable the workflow"
- "If the Homebrew API is suspected of compromise, pause the workflow and investigate manually"
- "If the workflow itself is modified unexpectedly, review the change history and verify all `.github/workflows/*.yml` files"

## Security Gaps Summary

| Gap | Severity | Recommendation |
|-----|----------|----------------|
| No package name validation | High | Add allowlist or pattern-based filtering |
| No semantic validation | Medium | Check tier counts, status transitions, timestamps |
| No workflow integrity checks | High | Verify script checksums, protect workflow files |
| No API response size limits | Medium | Reject responses >10MB or >100K packages |
| "Download verification" dismissal too narrow | Low | Reframe as "supply chain verification" |
| "User data exposure" needs future-proofing | Low | Add note about telemetry/feedback |
| No input validation on workflow params | Medium | Add min/max bounds for `limit` |
| No audit logging documentation | Low | Document how to audit workflow runs |
| No incident response plan | Medium | Add section on responding to compromise |

## Threat Model Summary

### Assumed Attacker Capabilities

The design implicitly assumes attackers can:
1. Compromise the Homebrew analytics API (supply chain risk)
2. Perform HTTPS MitM on GitHub Actions runners (low probability)
3. Gain write access to the repository (via compromised maintainer account)

The design does NOT consider:
- Attackers who can compromise GitHub Actions infrastructure itself (out of scope)
- Attackers who can exploit bugs in jq or bash (out of scope, reliance on standard tooling)

### Attack Surface

**External inputs**:
1. Homebrew analytics API response (untrusted, validated only by schema)
2. Workflow input parameters (untrusted, not validated)

**Internal inputs**:
1. Existing `priority-queue.json` file (trusted, assumed to have been validated previously)
2. Schema file (`priority-queue.schema.json`) (trusted, in repository)

**Outputs**:
1. Committed `priority-queue.json` file (consumed by batch pipeline)

**Trust boundaries**:
- Homebrew API → seed script (trust boundary, needs validation)
- Seed script → validation script (internal, trusted)
- Validation script → git commit (internal, trusted)

## Overall Assessment

**Status: APPROVED with security enhancements required before cron graduation**

The design is acceptable for the manual `workflow_dispatch` phase because operators can review each run. However, graduating to automated cron runs requires addressing the identified gaps, especially:

1. Package name validation (prevent typosquatting)
2. Semantic validation (catch merge logic bugs)
3. Workflow integrity checks (prevent workflow tampering)

### Strengths

1. **Correct risk identification**: The design acknowledges that metadata is lower-risk than binaries
2. **Schema validation**: Prevents structurally invalid data from landing
3. **Retry logic**: Handles transient failures gracefully
4. **HTTPS for all requests**: Baseline transport security

### Weaknesses

1. **Thin defense-in-depth**: Relies heavily on downstream (batch pipeline) validation
2. **No input validation**: Trusts the Homebrew API completely
3. **Direct commits without integrity checks**: Vulnerable to workflow tampering
4. **Incomplete security section**: Missing several standard considerations (audit logging, incident response)

### Recommendation

**For manual `workflow_dispatch` phase**: Proceed as designed. The risk is acceptable because humans review each run.

**Before graduating to cron**: Implement:
1. Package name validation (allowlist or pattern matching)
2. Semantic validation (tier counts, status transition rules)
3. API response size limits
4. Input validation on workflow parameters

**Nice-to-haves** (not blockers):
1. Workflow integrity checks (script checksums)
2. Audit logging documentation
3. Incident response plan

With these enhancements, the residual risk is acceptable for an automated metadata pipeline.
