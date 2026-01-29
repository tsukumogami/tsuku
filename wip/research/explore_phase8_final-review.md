# Final Review: DESIGN-batch-operations.md

**Reviewer**: Senior Architect
**Date**: 2026-01-27
**Design Version**: Proposed

---

## 1. Summary

**Result: PASS with minor recommendations**

The design is thorough, well-structured, and addresses all requirements from the upstream design. The decisions are internally consistent, the implementation details are sufficient for a developer to begin work, and security considerations are comprehensive. The design is ready for approval with minor clarifications recommended.

---

## 2. Completeness Assessment

### Requirements from Upstream Design (DESIGN-registry-scale-strategy.md)

| Requirement | Section | Status | Notes |
|-------------|---------|--------|-------|
| Rollback scripts (Phase 0) | Rollback Script section | **Complete** | Batch ID metadata + git revert approach. Script provided. |
| Emergency stop (Phase 1b) | Decision 2, Workflow Integration | **Complete** | Circuit breaker + control file. Both mechanisms documented. |
| SLI/SLO definitions (Phase 1b) | Decision 4 | **Complete** | Per-ecosystem success rates with severity alerting. |
| Circuit breaker (Phase 1b) | Decision 2, Architecture | **Complete** | 50% threshold, per-ecosystem, state persisted in control file. |
| Cost caps (Phase 2) | Decision 3 | **Complete** | Time-windowed budget with sampling degradation. |
| Post-merge monitoring (Phase 4/Security) | Security Considerations | **Complete** | Checksum drift detection workflow snippet provided. |

### Upstream Design Cross-References

The design correctly references:
- Phase 0: Rollback scripts
- Phase 1b: Emergency stop, SLI/SLO definitions, circuit breaker
- Phase 2: Cost caps (via sampling strategy)
- Security: Post-merge monitoring, incident response

**Assessment**: All upstream requirements are addressed. The design appropriately scopes to what was requested (#1187).

---

## 3. Consistency Check

### Architecture vs Decisions Alignment

| Decision | Architecture Component | Consistent? |
|----------|----------------------|-------------|
| D1: Batch ID + Git Revert | Rollback script uses `git log --grep` | Yes |
| D2: Circuit Breaker + Control File | Control file schema includes `circuit_breaker` object | Yes |
| D3: Time-Windowed + Sampling | Control file includes `budget` object | Yes |
| D4: Per-Ecosystem SLIs | D1 schema has `ecosystem` column | Yes |
| D5: Hybrid Storage | Control file (repo) + D1 (metrics) | Yes |

### Control File Schema Completeness

The schema covers:
- Master enable/disable (`enabled`)
- Per-ecosystem disable (`disabled_ecosystems`)
- Incident tracking (`reason`, `incident_url`, `disabled_by`, `disabled_at`, `expected_resume`)
- Circuit breaker state per ecosystem
- Budget tracking

**Minor gap**: The schema does not include a `sampling_enabled` field to indicate when sampling mode has been activated due to budget pressure. This is implied by the 80%/95% thresholds but not explicitly tracked in state.

### D1 Schema vs SLI Requirements

The D1 schema supports:
- Per-ecosystem success rates (`ecosystem` column, `success_rate` column)
- Time-based queries (`started_at` index)
- Per-batch queries (`batch_id` index)
- Error categorization (`error_category` in `recipe_results`)

**Assessment**: Schema is sufficient for SLI calculations. The `success_rate` column in `batch_runs` allows direct retrieval without requiring computation on every query.

---

## 4. Implementation Concerns

### Workflow YAML Snippets

**Pre-flight checks**: Valid YAML. Logic is correct for reading control file and filtering ecosystems.

**Circuit breaker check**: Valid YAML. Date comparison logic is correct (`date -d` works on Linux runners).

**Post-batch update**: Valid YAML. Uses `jq` correctly for JSON manipulation.

**Potential issue**: The post-batch update step uses `git push || true` which silently swallows push failures. In a concurrent environment with multiple ecosystem jobs, this could lead to lost state updates. Consider:
- Using `git pull --rebase && git push` with retry logic
- Or accepting that control file updates may occasionally fail (state eventually consistent on next run)

### Rollback Script Robustness

The script at `scripts/rollback-batch.sh`:

**Strengths**:
- Uses `set -euo pipefail` for safety
- Validates input
- Shows files before removal (operator review)
- Creates a branch for PR-based review

**Edge cases not handled**:
1. **Empty batch ID result with non-matching grep**: If `git log --grep` returns commits but `grep '^recipes/'` filters all of them out, `$files` will be empty but the script will continue. The empty check exists but only catches when `git log` itself returns nothing.
2. **Partial rollback**: If `git rm` fails for some files (e.g., already deleted in another commit), the script exits. Consider `git rm --ignore-unmatch`.
3. **Branch already exists**: If `rollback-batch-$BATCH_ID` branch already exists, `git checkout -b` fails.

**Recommendation**: These are minor robustness issues that can be addressed during implementation.

### Missing Edge Cases

1. **Concurrent circuit breaker updates**: Multiple ecosystem jobs may try to update the control file simultaneously. The workflow uses `|| true` on push, but race conditions could cause state loss.

2. **Budget reset timing**: The `week_start` field in budget tracking doesn't specify what triggers a reset. Should the workflow reset `macos_minutes_used` to 0 when `week_start` is older than 7 days?

3. **Half-open circuit breaker behavior**: The design mentions half-open state but doesn't specify:
   - How many requests to allow through in half-open
   - What success threshold closes the circuit from half-open

---

## 5. Security Review

### Attack Vectors Coverage

| Vector | Addressed? | Mitigation Adequate? |
|--------|-----------|---------------------|
| Malicious control file modification | Yes | Git audit trail + branch protection recommendation |
| Circuit breaker bypass | Yes | State transitions validated, anomaly detection mentioned |
| Batch ID spoofing | Yes | CI-generated IDs, format validation, PR review |
| D1 data tampering | Yes | Write access restricted to CI, advisory-only |
| DoS via budget exhaustion | Yes | Scheduled runs only, time-windowed budget |

### Access Control Model

The access control table is clear and follows principle of least privilege:
- Operators get write (not admin)
- Admin reserved for nuclear options
- D1 access separate from control file

**Minor clarification needed**: The design mentions "API token" for D1 query access but doesn't specify who has this token or how it's rotated.

### Audit Trail Sufficiency

Audit requirements are well-defined:
- Git history for control file changes
- PR references for rollbacks
- Circuit breaker trips logged with timestamps
- D1 metrics retained 90 days

**Gap**: No mention of alerting when control file changes occur. The design mentions "Alert on control file changes via GitHub notification" as a mitigation but doesn't specify how this is implemented.

### Post-Merge Monitoring Security

The checksum drift detection workflow snippet is well-designed:
- Re-fetches checksums for recently merged recipes
- Creates security issue on drift
- Labels appropriately

**Potential improvement**: Consider adding a 24-hour delay before checking to allow for legitimate upstream re-releases (some projects re-tag releases to fix minor issues). This could be a configurable parameter.

---

## 6. Recommended Changes

### Required (Must address before approval)

None. The design meets all requirements.

### Strongly Recommended (Should address)

1. **Add `sampling_active` field to control file schema**: Track when budget-triggered sampling is active to aid operator visibility.

2. **Clarify half-open circuit breaker behavior**: Specify:
   - Number of requests allowed through (suggest: 1)
   - Success count to close (suggest: 1 success = closed, 1 failure = back to open)

3. **Address concurrent control file update race**: Either:
   - Document that occasional state loss is acceptable (self-healing on next run)
   - Or add retry logic with exponential backoff

### Nice to Have (Consider for implementation)

1. **Budget reset mechanism**: Add a workflow step or document when/how `week_start` resets.

2. **Rollback script improvements**:
   - Use `git rm --ignore-unmatch` for partial rollback resilience
   - Check if branch exists before creating
   - Handle case where grep filters all files

3. **Control file change alerting**: Document how GitHub notification for control file changes is configured (repository settings, branch protection rules, or external tool).

4. **D1 access token documentation**: Specify who holds the API token and rotation policy.

---

## 7. Conclusion

This design is comprehensive and ready for implementation. It successfully translates the upstream strategic requirements into actionable operational procedures. The combination of automatic response (circuit breaker) with manual override (control file) provides appropriate flexibility for incident response.

The security model is well-thought-out, particularly the repository-primary decision that avoids network dependencies for critical control paths. The D1 metrics backend is appropriately positioned as advisory rather than authoritative.

The design demonstrates strong operational thinking: runbook structure is defined, incident classification is clear, and audit trails are comprehensive. Operators will have the tools they need to respond to incidents effectively.

**Approval recommendation**: Approve with the understanding that strongly recommended changes will be addressed during implementation review.
