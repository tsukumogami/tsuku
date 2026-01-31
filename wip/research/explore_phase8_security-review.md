# Security Review: DESIGN-batch-platform-validation.md

**Reviewer**: Security Analysis
**Date**: 2026-01-31
**Design Status**: Proposed

## Executive Summary

The design introduces moderate security risk through expanded attack surface (validation on 5 platforms, macOS runners) but mitigates with existing controls (ephemeral runners, checksum verification, `run_command` gate). No critical vulnerabilities identified.

**Key findings:**
1. **Compromised validation runner** could falsify platform support → mitigated by PR CI defense-in-depth
2. **Artifact poisoning** between jobs → mitigated by GitHub Actions artifact scoping
3. **Resource exhaustion** via malicious recipes → needs additional safeguards (timeouts, quotas)

All attack vectors have acceptable risk with recommended mitigations. No residual risk escalation needed.

---

## 1. Threat Model

### 1.1 Actors and Assets

**Actors:**
- Batch workflow (generates and validates recipes)
- Platform validation jobs (execute `tsuku install` on target platforms)
- Merge job (writes platform constraints, creates PR)
- Upstream package sources (Homebrew, npm, crates.io, GitHub releases)
- GitHub Actions infrastructure (runners, artifact storage)
- tsuku users (install recipes post-merge)

**Assets:**
- Recipe integrity (accurate platform support metadata)
- CI infrastructure (runner availability, budget)
- User systems (install binaries from recipes)
- Repository integrity (PRs, main branch)

### 1.2 Trust Boundaries

```
Untrusted:
- Upstream package sources (could serve malicious binaries)
- User-submitted package names (via priority queue)

Partially Trusted:
- GitHub Actions runners (ephemeral, but could be compromised)
- Batch workflow artifacts (scoped to workflow run, but writable by jobs)

Trusted:
- tsuku CLI code (code review, signed commits)
- Recipe checksum verification (cryptographic hashes)
- GitHub repository permissions (branch protection, required reviews)
```

### 1.3 Attack Objectives

**Attacker goals:**
1. Ship a malicious recipe to users (code execution on user systems)
2. Cause denial of service (exhaust CI budget, block batch pipeline)
3. Inject false platform support (crash users on unsupported platforms)
4. Exfiltrate data from validation runners (secrets, credentials)

---

## 2. Attack Vector Analysis

### 2.1 Malicious Recipe Execution

**Vector**: A batch-generated recipe downloads a malicious binary that executes during validation.

**Scenario**:
1. Upstream package source (e.g., npm registry) is compromised
2. Attacker publishes a malicious package
3. Batch workflow generates a recipe for the package
4. Platform validation runs `tsuku install`, downloading and executing the malicious binary
5. Binary compromises the validation runner

**Impact**:
- **Validation runner**: Full compromise (code execution in runner context)
- **User systems**: If the malicious recipe merges, users install the same binary

**Existing mitigations**:
- Ephemeral runners: Each job runs on a fresh VM, destroyed after completion. Compromise doesn't persist.
- Checksum verification: `tsuku install` verifies download checksums from the recipe. If the recipe specifies a checksum, a modified binary fails verification.
- `run_command` gate: Recipes with arbitrary command execution are excluded from auto-merge.

**Gaps**:
- **Checksum source**: Batch-generated recipes get checksums from the package metadata (e.g., npm package.json, Homebrew formula). If the upstream source is compromised, the checksum matches the malicious binary.
- **No sandboxing**: `tsuku install` executes binaries directly (post-install scripts, version detection). No OS-level isolation.

**Risk assessment**: **MEDIUM**

**Rationale**: Checksum verification protects against MITM and CDN compromise but not against upstream source compromise. If an attacker controls the npm registry entry, they control both the binary and its checksum.

**Recommended mitigations**:
1. **Sandbox validation** (#1287 already planned): Run validation in containers with no network access (except tsuku downloads) and no volume mounts. Limits blast radius.
2. **Post-install script blocking**: During validation, disable post-install scripts via a flag: `tsuku install --no-post-install`. Many package managers (npm, pip) run arbitrary code post-install.
3. **Dependency pinning**: For ecosystems that support lockfiles (npm, pip), generate lockfiles during recipe creation and validate that downloads match lockfile hashes.

### 2.2 Compromised Validation Runner

**Vector**: A GitHub Actions runner is compromised (0-day in runner software, supply chain attack on runner image).

**Scenario**:
1. Attacker gains control of a macOS runner via a vulnerability
2. Validation job runs on the compromised runner
3. Attacker modifies platform validation results to report false passes
4. Merge job writes platform constraints based on falsified results
5. Broken recipes ship to users

**Impact**:
- **Recipe integrity**: Users on affected platforms get broken recipes (download failures, crashes)
- **User trust**: Erosion of confidence in tsuku quality

**Existing mitigations**:
- Ephemeral runners: Compromises don't persist across workflow runs
- PR CI defense-in-depth: `test-changed-recipes.yml` validates the final PR on separate runners. If batch validation is falsified, PR CI catches it.
- Artifact scoping: Artifacts are scoped to workflow run ID. An attacker can't inject artifacts from a different run.

**Gaps**:
- **No result integrity**: Platform validation results are plain JSON. No cryptographic signature to verify they came from the expected job.
- **Runner provenance**: No verification that the runner is genuine (not an attacker-controlled VM spoofing the runner label).

**Risk assessment**: **LOW**

**Rationale**: GitHub Actions runner compromise is a low-probability event (GitHub has strong supply chain controls). PR CI provides defense-in-depth. Impact is limited to recipe quality, not code execution.

**Recommended mitigations**:
1. **Result checksumming**: Merge job validates that platform result artifacts match expected schema and contain plausible data (e.g., recipe count matches passing-recipes count).
2. **Anomaly detection**: Log platform validation results to a separate system (e.g., S3, external monitoring). Alert if validation pass rates drop suddenly (could indicate falsified results).
3. **Require PR CI**: Make `test-changed-recipes.yml` a required status check. Even if batch validation is compromised, PR CI must pass before merge.

### 2.3 Artifact Poisoning

**Vector**: An attacker injects malicious content into workflow artifacts that get consumed by downstream jobs.

**Scenario**:
1. Generation job produces `passing-recipes` artifact with a list of recipe paths
2. Attacker modifies the artifact to include a path to a malicious recipe not in the repository
3. Platform jobs download the poisoned artifact and validate the malicious recipe
4. Merge job includes the malicious recipe in the PR

**Impact**:
- **Recipe integrity**: Malicious recipe ships to users

**Existing mitigations**:
- Artifact scoping: GitHub Actions artifacts are scoped to workflow run. Only jobs in the same run can access them. An attacker would need to compromise a job in the same run to inject artifacts.
- Path validation: If platform jobs validate that recipe paths are within the `recipes/` directory, out-of-tree recipes are rejected.

**Gaps**:
- **No artifact signing**: Artifacts are not cryptographically signed. A job with artifact write access can modify any artifact.
- **Job compromise prerequisite**: Exploiting this vector requires compromising a job in the workflow (generation job or a platform job). This is a higher bar than external attackers.

**Risk assessment**: **LOW**

**Rationale**: Requires compromising a workflow job, which is already a high-value target (see 2.1, 2.2). Artifact poisoning doesn't add significant risk beyond the initial compromise.

**Recommended mitigations**:
1. **Path validation**: Platform jobs validate that recipe paths in `passing-recipes` are relative paths starting with `recipes/` and don't contain `..` (no directory traversal).
2. **Artifact checksum**: Generation job computes a checksum of the `passing-recipes` artifact and writes it to `$GITHUB_STEP_SUMMARY`. Merge job verifies the checksum. This detects tampering but doesn't prevent it (attacker with job access can also modify the checksum).

### 2.4 Resource Exhaustion

**Vector**: A malicious recipe or crafted batch run exhausts CI resources (runner time, disk space, network bandwidth).

**Scenario 1: Infinite Retry Loop**:
1. A recipe has a flaky download that always returns exit code 5 (ExitNetwork)
2. Platform validation retries indefinitely (or up to a very high limit)
3. Job runs until GitHub Actions timeout (6 hours)

**Scenario 2: Disk Filling**:
1. A recipe downloads a 100GB binary
2. Platform job runs out of disk space
3. Job fails, but has consumed excessive resources

**Scenario 3: macOS Budget Exhaustion**:
1. Attacker submits 500 packages to the priority queue
2. Batch workflow validates all 500 on macOS (5000 minutes = ~83 hours)
3. Weekly budget (1000 minutes) exhausted in a single run
4. Legitimate macOS CI jobs blocked for the rest of the week

**Impact**:
- **CI availability**: Legitimate workflows can't run (out of budget, queue backed up)
- **Cost**: Excessive GitHub Actions billing (if on a paid plan)

**Existing mitigations**:
- Progressive validation: Only recipes that pass Linux are promoted to macOS. This filters out most broken recipes.
- Skip flags: Operators can disable macOS validation to conserve budget.
- Retry limit: Design specifies 3 retries (section 6.1 of arch review).

**Gaps**:
- **No per-recipe timeout**: A single recipe could run for 6 hours if `tsuku install` hangs.
- **No total time budget**: A 500-recipe batch could run for 30+ hours on macOS.
- **No disk quota**: No limit on download size per recipe.

**Risk assessment**: **MEDIUM**

**Rationale**: Resource exhaustion is easy to trigger (via priority queue submissions) and has high impact (blocks CI pipeline). Existing mitigations reduce likelihood but don't eliminate risk.

**Recommended mitigations**:
1. **Per-recipe timeout**: Wrap `tsuku install` in `timeout 5m`. If a recipe hangs, it's killed and logged as a failure.
2. **Total batch timeout**: Fail the platform job if total runtime exceeds 60 minutes (for Linux) or 120 minutes (for macOS). This prevents runaway batches.
3. **Download size limit**: Reject recipes with binaries larger than 500MB during generation (before platform validation).
4. **Queue rate limiting**: Limit priority queue submissions to 100 packages per day per submitter (prevents bulk abuse).

### 2.5 Supply Chain Tampering

**Vector**: An attacker compromises an upstream package source (Homebrew, npm, crates.io) and serves malicious binaries.

**Scenario**:
1. Attacker gains control of an npm package maintainer account
2. Publishes a malicious version of a popular package
3. Batch workflow generates a recipe for the malicious version
4. Platform validation downloads and runs the malicious binary (see 2.1)
5. Recipe merges and ships to users

**Impact**:
- **User systems**: Code execution when users run `tsuku install <package>`

**Existing mitigations**:
- Checksum verification: Protects against MITM, not against upstream compromise (see 2.1)
- `run_command` gate: Blocks recipes with arbitrary commands from auto-merge
- Ephemeral validation runners: Limits blast radius during validation

**Gaps**:
- **No package vetting**: Batch pipeline generates recipes automatically without manual review
- **No upstream integrity checks**: tsuku trusts that Homebrew, npm, etc. are not compromised

**Risk assessment**: **HIGH** (impact), **LOW** (likelihood) → **MEDIUM** (combined)

**Rationale**: Upstream source compromise is rare but has happened (event-stream npm package, ua-parser-js npm package). Impact is severe (code execution on user systems). Likelihood is low because major ecosystems have security teams and monitoring.

**Recommended mitigations**:
1. **Package popularity threshold**: Only generate recipes for packages with >1000 downloads/month. High-popularity packages are more likely to be monitored.
2. **Version lag**: Only generate recipes for package versions that are at least 7 days old. This gives time for community vetting and upstream security scans.
3. **Anomaly detection**: Flag packages for manual review if:
   - Binary size increased >50% between versions
   - New maintainer added in the last 30 days
   - Package was unpublished then re-published
4. **User warnings**: Add a `--trust` flag to `tsuku install` that users must acknowledge when installing batch-generated recipes: "This recipe was auto-generated. Review the source before installing."

### 2.6 Privilege Escalation via Platform Constraints

**Vector**: An attacker crafts a recipe with incorrect platform constraints to bypass security checks.

**Scenario**:
1. A recipe contains a privilege escalation exploit that only works on macOS
2. Attacker ensures the recipe fails macOS validation (e.g., via download timeout)
3. Merge job writes `supported_os = ["linux"]`, excluding macOS
4. Recipe merges without macOS testing
5. Attacker manually edits the recipe post-merge to remove constraints
6. Users on macOS install the exploit

**Impact**:
- **User systems**: Privilege escalation on macOS

**Existing mitigations**:
- Branch protection: Direct commits to main are blocked. Changes require PR + review.
- Commit signing: All commits must be signed (if configured). Unsigned commits are rejected.
- PR CI: `test-changed-recipes.yml` validates recipes on macOS. Manual edits trigger re-validation.

**Gaps**:
- **Post-merge modification**: If an attacker compromises a maintainer account with merge permissions, they can modify recipes on main.

**Risk assessment**: **LOW**

**Rationale**: Requires compromising a maintainer account, which is a high-value target beyond this design's scope. Existing repository security controls (2FA, branch protection) mitigate this.

**Recommended mitigations**:
1. **Constraint immutability**: Add a CI check that fails if a batch-generated recipe's platform constraints are modified in a subsequent PR. Only allow constraint relaxation (e.g., adding support for a new platform) with manual review.
2. **Audit log**: Log all recipe modifications to a tamper-proof log (e.g., AWS CloudTrail, GitHub audit log). Alert on constraint changes.

### 2.7 Denial of Service via Skipped Platforms

**Vector**: An attacker forces all platform validations to be skipped, allowing broken recipes to merge.

**Scenario**:
1. Attacker gains access to trigger `workflow_dispatch` (e.g., compromised GitHub token)
2. Triggers batch run with `skip_arm64=true`, `skip_musl=true`, `skip_macos=true`
3. All platform validations skipped; merge job uses only Linux x86_64 results
4. Recipes that fail on other platforms merge with no constraints
5. Users on macOS, ARM64, musl get broken recipes

**Impact**:
- **Recipe quality**: Broken recipes ship to users on non-Linux platforms
- **User trust**: Erosion of confidence in tsuku reliability

**Existing mitigations**:
- Workflow permissions: `workflow_dispatch` requires write access to the repository. Only maintainers have this.
- PR CI: `test-changed-recipes.yml` validates the PR on macOS, catching some failures.

**Gaps**:
- **No skip flag audit**: Skip flags are boolean inputs. No log of why they were set or who set them.
- **Partial PR CI coverage**: `test-changed-recipes.yml` doesn't test ARM64 or musl (same gap as batch validation).

**Risk assessment**: **LOW**

**Rationale**: Requires maintainer access. Impact is limited to recipe quality (not code execution). PR CI provides partial mitigation.

**Recommended mitigations**:
1. **Skip flag justification**: Require a text input `skip_reason` when skip flags are set. Log it to workflow summary and PR description.
2. **Skip flag restrictions**: Only allow skipping platforms for pre-release runs (e.g., tag workflow runs with `--prerelease` flag and only allow skips for those).
3. **Mandatory macOS validation**: Make macOS validation required for production batches. Only allow skipping in dev/test runs.

---

## 3. Security Controls Assessment

### 3.1 Existing Controls (from Design)

| Control | Effectiveness | Gaps |
|---------|--------------|------|
| Checksum verification | **Good** against MITM, CDN tampering | Doesn't protect against upstream source compromise |
| Ephemeral runners | **Good** against persistence, lateral movement | Doesn't prevent in-job compromise |
| `run_command` gate | **Good** against arbitrary command execution | Only blocks auto-merge; manual review can bypass |
| PR CI defense-in-depth | **Fair** - validates final recipes | Doesn't test ARM64 or musl |
| Artifact scoping | **Good** against cross-run injection | Doesn't prevent in-run tampering |

### 3.2 Proposed Additional Controls

| Control | Purpose | Priority |
|---------|---------|----------|
| Per-recipe timeout (5 min) | Prevent resource exhaustion | **CRITICAL** |
| Total batch timeout (60-120 min) | Prevent runaway batches | **CRITICAL** |
| Download size limit (500MB) | Prevent disk exhaustion | **HIGH** |
| Result checksum validation | Detect artifact tampering | **MEDIUM** |
| Path validation (no `..`) | Prevent directory traversal | **HIGH** |
| Package popularity threshold | Reduce supply chain risk | **MEDIUM** |
| Version lag (7 days) | Allow community vetting | **MEDIUM** |
| Sandbox validation containers | Isolate malicious binaries | **HIGH** (already planned #1287) |
| Post-install script blocking | Prevent code execution during validation | **HIGH** |
| Skip flag justification | Audit platform skipping | **LOW** |

---

## 4. Mitigations Sufficiency Analysis

### 4.1 Attack Vector 2.1 (Malicious Recipe Execution)

**Existing mitigations**: Ephemeral runners, checksum verification, `run_command` gate

**Sufficiency**: **INSUFFICIENT**

**Reasoning**: Checksum verification doesn't protect against upstream compromise. An attacker controlling an npm package can ship both a malicious binary and a matching checksum.

**Required improvements**:
- Sandbox validation (#1287): **CRITICAL** - Isolates validation from runner
- Post-install script blocking: **HIGH** - Prevents code execution paths
- Package popularity threshold: **MEDIUM** - Reduces attack surface

**Residual risk**: Even with sandboxing, a sophisticated attacker could exploit sandbox escapes. Acceptable residual risk given defense-in-depth (PR CI, user discretion).

### 4.2 Attack Vector 2.2 (Compromised Validation Runner)

**Existing mitigations**: Ephemeral runners, PR CI defense-in-depth, artifact scoping

**Sufficiency**: **SUFFICIENT**

**Reasoning**: PR CI provides independent validation on separate runners. Even if batch validation is compromised, PR CI must pass before merge.

**Optional improvements**:
- Result checksumming: **MEDIUM** - Adds detection capability
- Anomaly detection: **LOW** - Helps identify systematic compromise

**Residual risk**: If both batch validation and PR CI runners are compromised simultaneously, broken recipes could merge. Extremely low probability.

### 4.3 Attack Vector 2.3 (Artifact Poisoning)

**Existing mitigations**: Artifact scoping, job isolation

**Sufficiency**: **SUFFICIENT** with path validation

**Reasoning**: Artifact poisoning requires compromising a workflow job. If an attacker has that level of access, they can already modify code, recipes, or artifacts directly. Path validation prevents directory traversal exploits.

**Required improvements**:
- Path validation: **HIGH** - Prevents `../../etc/passwd` style attacks

**Residual risk**: Acceptable. Artifact poisoning doesn't add risk beyond job compromise.

### 4.4 Attack Vector 2.4 (Resource Exhaustion)

**Existing mitigations**: Progressive validation, skip flags, retry limit

**Sufficiency**: **INSUFFICIENT**

**Reasoning**: No timeouts or quotas. A malicious recipe can consume excessive resources.

**Required improvements**:
- Per-recipe timeout: **CRITICAL** - Prevents hanging recipes
- Total batch timeout: **CRITICAL** - Prevents runaway batches
- Download size limit: **HIGH** - Prevents disk exhaustion

**Residual risk**: After mitigations, acceptable. An attacker can still consume CI budget by submitting many small recipes, but can't exhaust resources with a single recipe.

### 4.5 Attack Vector 2.5 (Supply Chain Tampering)

**Existing mitigations**: Checksum verification, `run_command` gate, ephemeral runners

**Sufficiency**: **PARTIALLY SUFFICIENT**

**Reasoning**: Existing controls limit blast radius (ephemeral runners, auto-merge gate) but don't prevent the attack. Supply chain compromise is a systemic risk beyond this design's scope.

**Optional improvements**:
- Package popularity threshold: **MEDIUM** - Reduces attack surface
- Version lag: **MEDIUM** - Allows community vetting
- User warnings: **LOW** - Shifts some responsibility to users

**Residual risk**: Supply chain compromise of major ecosystems (npm, Homebrew) is a low-probability, high-impact event. tsuku inherits the risk of the ecosystems it supports. This is an acceptable trade-off for functionality.

### 4.6 Attack Vector 2.6 (Privilege Escalation via Platform Constraints)

**Existing mitigations**: Branch protection, commit signing, PR CI

**Sufficiency**: **SUFFICIENT**

**Reasoning**: Repository security controls prevent post-merge tampering. Constraint modification triggers PR CI re-validation.

**Optional improvements**:
- Constraint immutability check: **MEDIUM** - Adds detection capability

**Residual risk**: Acceptable. Requires compromising a maintainer account, which is out of scope.

### 4.7 Attack Vector 2.7 (Denial of Service via Skipped Platforms)

**Existing mitigations**: Workflow permissions, PR CI

**Sufficiency**: **SUFFICIENT**

**Reasoning**: Only maintainers can skip platforms. Impact is limited to recipe quality.

**Optional improvements**:
- Skip flag justification: **LOW** - Audit trail for skipping
- Mandatory macOS validation: **LOW** - Policy enforcement

**Residual risk**: Acceptable. Trusted maintainers can make quality trade-offs.

---

## 5. "Not Applicable" Justifications Review

### 5.1 Security Considerations Section (Lines 335-358)

The design includes a "Security Considerations" section with four subsections:

#### 5.1.1 Download Verification

**Claim**: "Checksum verification is sufficient because recipes specify expected checksums."

**Analysis**: **PARTIALLY INCORRECT**

**Issue**: Checksum verification is effective against MITM and CDN tampering but not upstream source compromise. If an attacker controls the npm registry entry, they control both the binary and its checksum. The design should acknowledge this limitation.

**Recommendation**: Revise to:
> Checksum verification protects against man-in-the-middle attacks and CDN tampering. However, it does not protect against upstream source compromise, where an attacker controls both the binary and its published checksum. This residual risk is mitigated by package popularity thresholds and version lag (see Phase 4 mitigations).

#### 5.1.2 Execution Isolation

**Claim**: "Ephemeral runners provide sufficient isolation."

**Analysis**: **CORRECT** for the threat model, but incomplete.

**Issue**: Ephemeral runners prevent persistence and lateral movement but don't prevent in-job compromise. The design should acknowledge the need for sandboxing (already planned in #1287).

**Recommendation**: Revise to:
> Ephemeral runners prevent persistence across workflow runs and lateral movement to other systems. However, they do not prevent compromise during the validation job. Issue #1287 (sandbox validation) will add OS-level isolation to contain malicious binaries during validation.

#### 5.1.3 Supply Chain Risks

**Claim**: "PR CI provides defense-in-depth and catches compromised runners."

**Analysis**: **CORRECT**

**Verification**: If batch validation falsely reports a pass (due to compromised runner), PR CI re-validates on independent runners. This is a valid defense-in-depth strategy.

#### 5.1.4 User Data Exposure

**Claim**: "Not applicable; validation runs on synthetic environments with no user data."

**Analysis**: **CORRECT**

**Verification**: Platform validation doesn't access user data. Only pass/fail results are recorded.

### 5.2 Implicit "Not Applicable" Assumptions

The design doesn't explicitly list threats it considers out of scope. Reviewing for omissions:

**Omitted threat: Compromised tsuku CLI**

**Is it applicable?**: **YES**

**Analysis**: If the `tsuku` binary used for validation is compromised (e.g., via supply chain attack on Go toolchain), it could falsify validation results or exfiltrate data.

**Recommendation**: Add to security considerations:
> The platform validation jobs depend on the integrity of the `tsuku` CLI binary. If the binary is compromised (e.g., via a supply chain attack on the Go toolchain or GitHub release process), validation results may be falsified. Mitigations:
> - Build `tsuku` from source in the workflow (don't download pre-built binaries)
> - Verify Go toolchain checksums via `actions/setup-go`
> - Use reproducible builds (future enhancement)

**Omitted threat: GitHub Actions workflow modification**

**Is it applicable?**: **YES**

**Analysis**: An attacker with write access could modify `batch-generate.yml` to skip validation or disable security checks.

**Recommendation**: Add to security considerations:
> The batch workflow YAML file is a trust root. Modifications to the workflow can disable security checks. Mitigations:
> - Require PR review for `.github/workflows/` changes
> - Use CODEOWNERS to require security team approval for workflow changes
> - Enable "Restrict modifications to workflows" in repository settings (prevents PRs from modifying workflows)

---

## 6. Residual Risk Assessment

### 6.1 Risks Accepted (With Mitigations)

| Risk | Likelihood | Impact | Residual Likelihood | Residual Impact | Acceptance Rationale |
|------|-----------|--------|-------------------|----------------|---------------------|
| Malicious recipe execution (2.1) | Medium | High | Low (with sandbox) | Medium (isolated) | Sandbox limits blast radius; PR CI provides defense-in-depth |
| Resource exhaustion (2.4) | High | Medium | Low (with timeouts) | Low (bounded) | Timeouts prevent runaway resource usage |
| Supply chain tampering (2.5) | Low | High | Low | High | Inherent risk of package ecosystems; mitigated by popularity threshold, version lag |

### 6.2 Risks Accepted (Without Additional Mitigations)

| Risk | Likelihood | Impact | Acceptance Rationale |
|------|-----------|--------|---------------------|
| Compromised validation runner (2.2) | Very Low | Medium | PR CI provides independent validation; impact limited to recipe quality |
| Artifact poisoning (2.3) | Very Low | Medium | Requires prior job compromise; path validation prevents exploitation |
| Privilege escalation via constraints (2.6) | Very Low | High | Requires maintainer account compromise; repository security controls mitigate |
| Denial of service via skipped platforms (2.7) | Very Low | Low | Requires maintainer access; impact limited to quality |

### 6.3 Risks Requiring Escalation

**NONE**

All identified risks have acceptable residual risk after applying recommended mitigations. No risks require escalation to senior security review.

---

## 7. Compliance and Policy Considerations

### 7.1 Open Source Security Best Practices

**Applicable standards**:
- [OpenSSF Best Practices Badge](https://bestpractices.coreinfrastructure.org/)
- [SLSA Framework](https://slsa.dev/) (Supply Chain Levels for Software Artifacts)

**Current compliance**:
- ✅ **Automated testing**: PR CI validates all changes
- ✅ **Static analysis**: golangci-lint runs on all Go code
- ⚠️ **Build provenance**: No SLSA provenance for `tsuku` binaries
- ⚠️ **Dependency pinning**: Go modules pinned, but GitHub Actions use `@v4` (not commit hash)

**Recommendations**:
1. Pin GitHub Actions to commit hashes: `uses: actions/checkout@a1b2c3d...` (prevents supply chain attacks on actions)
2. Generate SLSA provenance for `tsuku` releases (enables verification of build integrity)
3. Sign workflow artifacts (enables merge job to verify platform results came from expected jobs)

### 7.2 GitHub Actions Security Hardening

**Applicable guidance**: [GitHub Actions security hardening](https://docs.github.com/en/actions/security-guides/security-hardening-for-github-actions)

**Current compliance**:
- ✅ **Minimal token permissions**: Not specified in design; verify in implementation
- ✅ **Ephemeral runners**: Jobs use fresh VMs
- ⚠️ **Secrets handling**: Design doesn't use secrets (validation doesn't need auth)
- ⚠️ **Third-party actions**: Uses `actions/upload-artifact@v4`, `actions/download-artifact@v4` (official, but not pinned)

**Recommendations**:
1. Set explicit workflow permissions: `permissions: contents: read` (deny write by default)
2. Pin third-party actions to commit hashes
3. Use `GITHUB_TOKEN` with minimal scopes (read-only for most jobs)

---

## 8. Security Testing Recommendations

### 8.1 Fuzzing

**Target**: Platform constraint derivation algorithm

**Approach**:
- Generate randomized platform pass/fail combinations
- Verify constraints are valid (at least one platform matches)
- Verify constraints are minimal (no over-constraining)

**Tooling**: Go's built-in fuzzing (`go test -fuzz`)

### 8.2 Penetration Testing

**Targets**:
1. Artifact tampering: Attempt to inject malicious recipes via artifact poisoning
2. Resource exhaustion: Submit recipes that consume excessive CPU, memory, disk
3. Constraint bypass: Craft recipes with incorrect constraints that pass validation

**Approach**: Red team exercise with constrained scope (don't compromise production infra)

### 8.3 Static Analysis

**Targets**:
- Workflow YAML: Use [actionlint](https://github.com/rhysd/actionlint) to detect security anti-patterns
- Go code: Use `gosec` to detect common security issues (path traversal, command injection)
- Recipe validation: Use custom linter to detect recipes with suspicious patterns (e.g., `run_command` with user-controlled input)

---

## 9. Incident Response Plan

### 9.1 Detection

**Signals of compromise**:
- Sudden drop in platform validation pass rates (possible runner compromise)
- Recipes with `run_command` action in batch PRs (should be excluded by merge job)
- User reports of broken recipes on platforms that passed validation
- High CI budget consumption (possible resource exhaustion attack)

**Monitoring**:
- GitHub Actions audit log: Monitor for unexpected workflow triggers, permission changes
- Workflow run history: Alert on failed jobs, long-running jobs, high retry counts
- PR review: Flagged by `run_command` gate in merge job

### 9.2 Response

**Scenario 1: Compromised validation runner**

**Actions**:
1. Immediately disable platform validation (set `ENABLE_PLATFORM_VALIDATION: false`)
2. Review recent batch PRs for anomalous validation results
3. Re-run validation on known-good runners
4. Contact GitHub support to investigate runner compromise
5. Re-enable platform validation after confirmation of clean runners

**Scenario 2: Malicious recipe in production**

**Actions**:
1. Identify affected recipes via failure JSONL or user reports
2. Create hotfix PR to remove malicious recipes from `recipes/`
3. Deploy hotfix via emergency merge (skip normal review if time-critical)
4. Notify users via GitHub release notes, tsuku CLI warning
5. Post-mortem: Analyze how the recipe passed validation; improve detection

**Scenario 3: Resource exhaustion**

**Actions**:
1. Cancel running workflow
2. Review priority queue for suspicious submissions
3. Remove offending packages from queue
4. Implement rate limiting on queue submissions (if not already present)
5. Re-run batch with corrected queue

---

## 10. Long-Term Security Roadmap

### 10.1 Phase 1 (With This Design)

- ✅ Ephemeral runners
- ✅ Checksum verification
- ✅ `run_command` gate
- ✅ PR CI defense-in-depth
- 🔲 Per-recipe timeout (5 min)
- 🔲 Total batch timeout (60-120 min)
- 🔲 Download size limit (500MB)
- 🔲 Path validation (no `..`)

### 10.2 Phase 2 (Planned)

- 🔲 Sandbox validation (#1287) - **CRITICAL**
- 🔲 Post-install script blocking - **HIGH**
- 🔲 Package popularity threshold - **MEDIUM**
- 🔲 Version lag (7 days) - **MEDIUM**

### 10.3 Phase 3 (Future Enhancements)

- 🔲 SLSA provenance for `tsuku` releases
- 🔲 Workflow artifact signing
- 🔲 Anomaly detection for validation results
- 🔲 Dependency pinning (actions to commit hashes)
- 🔲 Reproducible builds
- 🔲 User warnings for batch-generated recipes

---

## 11. Recommendations Summary

### Critical (Implement Before Launch)

1. **Per-recipe timeout** (5 min): Prevents hanging recipes (2.4)
2. **Total batch timeout** (60-120 min): Prevents runaway batches (2.4)
3. **Path validation**: Prevents directory traversal in artifact paths (2.3)
4. **Sandbox validation** (#1287): Isolates malicious binaries (2.1) - **already planned**

### High Priority (Implement in Phase 1)

5. **Download size limit** (500MB): Prevents disk exhaustion (2.4)
6. **Post-install script blocking**: Prevents code execution during validation (2.1)
7. **Result checksum validation**: Detects artifact tampering (2.3)

### Medium Priority (Implement in Phase 2)

8. **Package popularity threshold**: Reduces supply chain risk (2.5)
9. **Version lag** (7 days): Allows community vetting (2.5)
10. **Workflow permissions hardening**: Minimal token scopes (7.2)

### Low Priority (Post-Launch)

11. **Skip flag justification**: Audit trail for platform skipping (2.7)
12. **SLSA provenance**: Build integrity verification (7.1)
13. **Dependency pinning**: Pin GitHub Actions to commit hashes (7.1, 7.2)

---

## 12. Conclusion

The design introduces moderate security risk through expanded attack surface but provides adequate mitigations for the threat model. Key findings:

**Strengths**:
- Ephemeral runners limit persistence and lateral movement
- PR CI provides defense-in-depth against falsified batch validation
- `run_command` gate prevents auto-merge of high-risk recipes

**Weaknesses**:
- No timeouts or quotas (resource exhaustion risk)
- Checksum verification doesn't protect against upstream compromise
- No sandboxing for validation (planned in #1287)

**Critical mitigations required before launch**:
- Per-recipe timeout (5 min)
- Total batch timeout (60-120 min)
- Path validation for artifact paths

**Residual risk**: All residual risks are acceptable with recommended mitigations. No escalation needed.

**Overall security assessment**: **APPROVED** with critical mitigations implemented.
