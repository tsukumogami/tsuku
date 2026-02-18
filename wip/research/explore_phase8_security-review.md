# Security Review: DESIGN-unified-batch-pipeline

**Reviewer:** Security review agent
**Date:** 2026-02-17
**Status:** Proposed design, pre-implementation
**Context:** tsuku downloads and executes binaries from the internet. The batch pipeline generates recipe files that define how to install tools. This review examines whether the unified pipeline changes introduce or worsen security risks.

---

## 1. Attack Vectors Not Considered

### 1.1. Ecosystem Confusion / Source Spoofing in Mixed Batches

**Risk: Medium**

With single-ecosystem batches, a compromised or malicious entry was confined to one ecosystem's trust boundary. With mixed-ecosystem batches, the orchestrator processes entries from multiple ecosystems in a single run. If an attacker can influence the `Source` field of a queue entry (e.g., by compromising the disambiguation step or injecting entries into the queue file), they could craft a source that appears to be one ecosystem but resolves differently.

Example: An entry with `Source: "github:evil-org/jq"` would pass the `Ecosystem()` extraction as "github" and the rate limit would be GitHub's. But `tsuku create --from github:evil-org/jq` would download binaries from the attacker's repository.

**Current mitigation:** The queue entries have pre-resolved sources set during disambiguation, which includes security checks (`version_count >= 3`, `has_repository` link). The design correctly identifies this.

**Gap:** The design doesn't discuss what happens if a queue entry's `Source` field is modified after disambiguation. The queue file (`priority-queue.json`) is a JSON file in the repository. Anyone with write access to the repo (or the GitHub App token) can modify it. The design should note that queue integrity depends on repository access controls and PR review.

**Recommendation:** No immediate action needed beyond what exists, but the design should explicitly state the trust boundary: "Queue entries are trusted because they're committed through reviewed PRs. Direct pushes to the queue file bypass this trust."

### 1.2. Circuit Breaker State Manipulation

**Risk: Low-Medium**

The design moves circuit breaker reads into the orchestrator via `Config.BreakerState`. The `batch-control.json` file is committed to the repository. An attacker with commit access could set all breakers to "closed" to force processing of ecosystems that should be blocked, or set all breakers to "open" to create a denial-of-service on the pipeline.

**Current mitigation:** `batch-control.json` is modified by the workflow through `update_breaker.sh` and committed by the GitHub App bot. Changes to it appear in git history.

**Gap:** There is no validation that `batch-control.json` hasn't been tampered with between runs. The workflow reads it at the start and trusts it. A malicious PR that modifies `batch-control.json` to close all breakers would be merged through normal review, and the next cron run would honor the modified state.

**Recommendation:** This is an acceptable risk given that the file is reviewed in PRs. No change needed, but the design should document that `batch-control.json` integrity relies on code review.

### 1.3. Rate Limit Exhaustion Through Mixed-Ecosystem Batches

**Risk: Low**

With a single ecosystem per batch, rate limits applied uniformly. With mixed batches, 25 candidates could include entries from 8 different ecosystems. If the batch has 20 cargo entries and 5 github entries, the cargo API gets 20 requests at 1s intervals, while the github API gets 5 requests interspersed among the cargo ones. Since entries are processed sequentially and the rate limit applies per-entry based on its ecosystem, a batch can't exceed the per-ecosystem rate limit.

However, consider this: if entries are ordered `[github, cargo, github, cargo, ...]`, the GitHub API gets requests spaced by `cargo_rate_limit` (1s), not `github_rate_limit`. The per-entry rate limiting as designed sleeps for `ecosystemRateLimits[pkg.Ecosystem()]` before each entry (except the first). This means the sleep duration is determined by the *current* entry's ecosystem, not by the time since the last request to the *same* ecosystem.

**Gap:** Two consecutive entries from different ecosystems with different rate limits could result in one ecosystem being hit faster than its rate limit allows. Example:
- Entry 1: `cargo:a` (sleep 1s)
- Entry 2: `github:b` (sleep 1s -- but the last github request was 0s ago, not 1s ago because entry 2 is the first github request)
- Entry 3: `cargo:c` (sleep 1s)
- Entry 4: `github:d` (sleep 1s -- but the last github request was only 2s ago, which is fine for most APIs)

In practice, this is only a concern for APIs with strict per-second rate limits. The 1-second sleep between each entry means no single API gets hit more than once per second regardless of ecosystem interleaving, because the loop is sequential. So this is actually fine for the current rate limits. It would become a concern if an ecosystem needed a rate limit longer than the batch's total processing time, which isn't the case.

**Recommendation:** No change needed. The sequential processing inherently spaces requests.

### 1.4. Failure File Injection Through Ecosystem Name

**Risk: Low**

The `WriteFailures()` function uses the ecosystem string in the filename: `fmt.Sprintf("%s-%s.jsonl", ecosystem, timestamp)`. The ecosystem is extracted from `entry.Source` via `strings.Index(entry.Source, ":")`. If an entry's `Source` is `"../../etc/passwd:exploit"`, the `Ecosystem()` method returns `"../../etc/passwd"` and the failure file would be written to `data/failures/../../etc/passwd-2026-02-17T12-00-00Z.jsonl`, which is a path traversal.

**Current mitigation:** `QueueEntry.Validate()` checks that Source is not empty but does not validate its format beyond that. The `QueueEntry.Ecosystem()` method extracts everything before the first colon without sanitization.

**Gap:** The `Source` field has no format validation. While queue entries are created through controlled code paths (disambiguation, seeding), the lack of validation means a malformed entry could cause path traversal in failure file writes.

**Recommendation:** Add validation to `QueueEntry.Validate()` ensuring the ecosystem prefix doesn't contain path separators:

```go
eco := e.Ecosystem()
if strings.ContainsAny(eco, "/\\..") {
    errs = append(errs, fmt.Sprintf("source ecosystem contains invalid characters: %q", eco))
}
```

This is a defense-in-depth measure. The risk is low because queue entries are generated by trusted code, but validation at the data layer is sound practice.

---

## 2. Are the Mitigations Sufficient for the Risks Identified?

### 2.1. Supply Chain Risk Mitigation: Adequate

The design identifies that mixed-ecosystem batches increase the exposure surface per run. The mitigation correctly identifies that:

- Per-entry circuit breakers isolate ecosystem-level failures
- `tsuku create` performs checksum verification, binary discovery, and platform validation
- Queue entries have pre-resolved sources from disambiguation with security checks

**Assessment:** The mitigations are sufficient. The existing `tsuku create` validation pipeline is the real security boundary, and this design doesn't weaken it. The mixed-batch change only affects *which* entries are selected, not *how* they're processed.

### 2.2. Missing Mitigation: GitHub rate limit for `github:` ecosystem

The `ecosystemRateLimits` map has no `github` entry. With 261 re-routed packages (many likely using `github:` sources), the pipeline could send rapid unauthenticated requests to the GitHub API, which has a 60 requests/hour limit for unauthenticated callers and 5000/hour for authenticated ones.

The `tsuku create` command likely uses the GitHub API to resolve releases and download URLs. Without rate limiting, a batch of 25 github-sourced entries would send 25+ API requests in quick succession, potentially hitting rate limits and causing cascading failures.

**Assessment:** This is operationally relevant, not a security vulnerability per se, but could trigger rate limiting that masks real failures and prevents the circuit breaker from functioning correctly.

**Recommendation:** Add `"github": 2 * time.Second` to `ecosystemRateLimits`. GitHub's API returns rate limit headers; the 2-second spacing keeps well within limits for batches of 25.

---

## 3. Residual Risk Assessment

### Residual risks that should be acknowledged

| Risk | Severity | Residual after mitigations | Action |
|------|----------|---------------------------|--------|
| Compromised upstream ecosystem serves malicious binaries | High | Mitigated by `tsuku create` checksum validation and multi-platform validation. Residual: if attacker controls the canonical release, checksums match the malicious binary. | Accept -- this is inherent to any package manager and out of scope for this design |
| Queue file tampered with between runs | Low | Mitigated by git history and PR review. Residual: the GitHub App has write access and could be compromised. | Accept -- standard supply chain risk |
| Circuit breaker manipulation | Low | Mitigated by PR review. Residual: someone closes all breakers intentionally. | Accept |
| Failure file path traversal | Low | Not currently mitigated. Residual: requires malformed queue entry. | Fix -- add ecosystem validation |
| Missing GitHub rate limit | Medium | Not currently mitigated. Could cause operational disruption. | Fix -- add rate limit entry |

### Risks that do not need escalation

None of the identified risks represent a critical vulnerability that requires stopping the design process. The path traversal and rate limit gaps should be addressed during implementation but don't change the design's overall approach.

---

## 4. "Not Applicable" Justification Review

### 4.1. "Download Verification: Not applicable"

**Assessment: Justified.** The design correctly states it doesn't change download, extraction, or verification mechanisms. The `generate()` method passes `pkg.Source` to `tsuku create --from`, which is unchanged from the current behavior. The only change is *which* entries are selected, not how they're processed. The download verification path in `tsuku create` is unaffected.

### 4.2. "Execution Isolation: Not applicable"

**Assessment: Justified, with a note.** The design doesn't change what gets executed or how. The batch pipeline runs in GitHub Actions runners (Ubuntu, macOS) and validates recipes inside Docker containers. This is unchanged.

However, there is a subtle expansion: previously, a single cron-triggered batch only executed binaries from one ecosystem. Now, a single batch may execute binaries from multiple ecosystems. If a compromised cargo binary escapes the Docker container and modifies the filesystem, subsequent validation runs for homebrew entries in the same batch (which run on the same runner, in sequential Docker containers with `-v "$PWD:/workspace"`) could be affected.

**Gap:** The Docker volume mount `-v "$PWD:/workspace"` shares the host workspace between all validation containers in a job. A malicious binary could write to `/workspace/.tsuku-exit-code` or `/workspace/.tsuku-output.json` before the expected tsuku binary runs, poisoning the validation result.

**Mitigation:** This risk exists in the current single-ecosystem design too -- a compromised homebrew binary could affect later homebrew validation runs in the same job. The mixed-ecosystem change doesn't meaningfully increase this risk because the attack requires escaping the Docker container, at which point the attacker already has host access.

**Recommendation:** The "Not applicable" justification is acceptable but could be strengthened by noting: "Execution isolation between entries within a batch relies on Docker container boundaries, which is unchanged."

### 4.3. "User Data Exposure: Not applicable"

**Assessment: Justified.** The batch pipeline processes package metadata and produces TOML recipe files. No user data is involved. The dashboard displays operational metrics. This is unchanged by the design.

---

## 5. Additional Security Observations

### 5.1. `run_command` Security Gate

The workflow's merge step (line 905-909) checks for `run_command` actions in generated recipes and excludes them:

```bash
if grep -q 'action.*=.*"run_command"' "$RECIPE_FILE" 2>/dev/null; then
    echo "EXCLUDE: $recipe has run_command action"
    rm -f "$RECIPE_FILE"
```

This is a sound security gate -- `run_command` actions execute arbitrary shell commands, and auto-generated recipes with this action should be reviewed manually. This gate is unaffected by the unified pipeline change.

**Note:** The grep pattern `action.*=.*"run_command"` is case-sensitive and requires exact quoting. A recipe with `action = 'run_command'` (single quotes) or `action = "run_Command"` (different casing) would bypass this check. TOML allows both single and double-quoted strings. The pattern should be case-insensitive and handle both quote styles:

```bash
if grep -qi 'action.*=.*["\x27]run_command["\x27]' "$RECIPE_FILE" 2>/dev/null; then
```

This is a pre-existing issue, not introduced by this design, but worth noting.

### 5.2. Path Traversal Check in Validation Steps

The validation steps include:

```bash
case "$recipe_path" in
    *..*) echo "SKIP: path traversal in $recipe_path"; continue ;;
esac
```

This is a defense-in-depth check against path traversal in recipe paths. It's present in all validation jobs. Good practice, unaffected by this design.

### 5.3. GitHub App Token Scope

The workflow uses a GitHub App token with `contents: write` and `pull-requests: write` permissions. The App token is generated per-run and expires in 60 minutes. This is sound security practice and is unaffected by the design change.

---

## Summary of Security Findings

| # | Finding | Severity | Action Required |
|---|---------|----------|-----------------|
| 1 | Queue entry `Source` field lacks path-safety validation in `Ecosystem()` | Low | Add validation in `QueueEntry.Validate()` to reject ecosystems with path separators |
| 2 | Missing `github` rate limit could cause API exhaustion | Medium | Add `github` to `ecosystemRateLimits` map |
| 3 | "Not applicable" justifications are accurate | Info | No change, but strengthen execution isolation note |
| 4 | `run_command` grep pattern is case-sensitive and single-quote-blind | Low | Pre-existing issue; fix opportunistically |
| 5 | Docker volume mount shares workspace between validation runs | Low | Pre-existing issue; document as accepted risk |
| 6 | Supply chain mitigations are sufficient for the risk level | Info | No change needed |

**Overall assessment:** The design does not introduce significant new security risks. The changes are primarily about selection logic (which entries to process), not processing logic (how entries are handled). The existing security controls in `tsuku create`, recipe validation, Docker isolation, and the `run_command` gate remain effective. The two actionable items (ecosystem validation and GitHub rate limiting) are defense-in-depth improvements rather than critical fixes.
