# Security Review: Phase 8

## Summary

The design is a CI topology change with low inherent security risk. The attack surface does not meaningfully expand because the consolidation reuses patterns already deployed in production (`test-recipe.yml`, `build-essentials.yml` macOS). The Security Considerations section is mostly accurate, but understates the implications of the read-write volume mount and omits a credential-in-container concern that applies to at least two of the seven migration targets.

Overall assessment: no blocking security issues, two advisory findings that should be addressed in the design document before implementation begins.

## Findings

### 1. Read-write volume mount allows container-to-host writes (Advisory)

**Location:** Design doc line 168 (`-v "$PWD:/workspace"`) and Security Considerations section (lines 316-317).

The document says: "The volume mount (`-v "$PWD:/workspace"`) is read-write -- containers write exit code files and test artifacts back to the workspace." It then dismisses this with "--rm ensures no state leaks between family tests."

This is accurate about inter-container isolation but incomplete about the container-to-host direction. A malicious or compromised container image can modify any file in `$PWD`, which is the full checkout of the repository on the runner. Concrete scenario: if a pulled image (e.g., `archlinux:base`) were compromised at Docker Hub, code in the container could overwrite `test/scripts/verify-tool.sh` or the `tsuku` binary itself, and subsequent test steps on the host runner would execute the modified code.

**However, this is the pre-existing risk profile.** The exact same `-v "$PWD:/workspace"` mount with the same images already exists in `test-recipe.yml` (line 153), `release.yml` (line 155), and `platform-integration.yml` (line 96). The consolidation design does not introduce this risk; it replicates it to more workflows.

**Risk residual:** Low. GitHub-hosted runners are ephemeral. The images used are official Docker Hub images with established provenance. The pre-existing exposure in `test-recipe.yml` has been running without issue.

**Recommendation:** Add a sentence to the Execution Isolation section acknowledging that read-write mounts allow containers to modify host workspace files, and note that this is accepted because (a) the same risk exists in test-recipe.yml today, (b) runners are ephemeral, and (c) the images are official distributions. This documents the risk decision rather than leaving it implicit.

**Why not blocking:** The risk is pre-existing and contained to ephemeral CI runners. The consolidation doesn't amplify it (same images, same mount).

### 2. GITHUB_TOKEN in container loops is unaddressed (Advisory)

**Location:** Design doc Security Considerations (lines 327-328), Container Loop Pattern (lines 150-177).

The document's "User Data Exposure" section says "The only secrets involved are `GITHUB_TOKEN` (for API rate limits) [...] neither of which are affected by job consolidation." This is incorrect for two migration targets.

Currently, `integration-tests.yml` workflows `checksum-pinning` and `homebrew-linux` use `GITHUB_TOKEN` at the runner level, where their test scripts (`test-checksum-pinning.sh`, `test-homebrew-recipe.sh`) handle containerization internally. The checksum script passes `GITHUB_TOKEN` into containers via `docker run --rm -e GITHUB_TOKEN="$GITHUB_TOKEN"` (test-checksum-pinning.sh line 125). The homebrew script uses `tsuku install --sandbox` which builds its own container with the token available on the host.

When these jobs are consolidated into the design's container loop pattern (lines 150-177), the implementer must decide how to handle GITHUB_TOKEN inside each `docker run`. The design's template at line 168 shows:

```yaml
timeout 300 docker run --rm -v "$PWD:/workspace" -w /workspace "$image" sh -c "
  # family-specific package install
  # run actual test
"
```

No `-e GITHUB_TOKEN` is shown. If the implementer follows this template literally, checksum-pinning and homebrew tests will fail because tsuku needs a token to download from GitHub releases without hitting rate limits.

If the implementer adds `-e GITHUB_TOKEN="$GITHUB_TOKEN"` to the docker run invocation, the token becomes available inside the container. This is the same trust level as the current test-checksum-pinning.sh, but the document should acknowledge it explicitly because the Security Considerations currently says secrets are "not affected."

**Risk residual:** Low. `GITHUB_TOKEN` is a short-lived, repo-scoped token that GHA injects automatically. Passing it into ephemeral containers from official images is standard practice and is what the existing test scripts already do. The token has read-only access for PR triggers and expires when the workflow completes.

**Recommendation:** Update the Container Loop Pattern to show the `-e GITHUB_TOKEN` flag for loops that need API access (checksum-pinning, homebrew). Update the "User Data Exposure" section to say that GITHUB_TOKEN is passed into containers in workflows that test download-dependent features, consistent with the existing test scripts.

**Why not blocking:** The token handling is pre-existing in the test scripts. The consolidated version does the same thing with the same trust model. But the document should not claim secrets are unaffected when the implementation must explicitly pass them into containers.

### 3. "Download Verification" dismissal is correct (No issue)

The document says download verification is "not applicable" because test commands remain identical. This is accurate. The consolidation changes orchestration, not what each test does. The tsuku binary still performs its own checksum verification. No finding.

### 4. "Supply Chain Risks" dismissal is correct (No issue)

The document says supply chain risks are "not applicable" because the same Docker images are used. Verified: the images listed in the design (`debian:bookworm-slim`, `fedora:41`, `archlinux:base`, `opensuse/tumbleweed`, `alpine:3.21`) match those already used in `test-recipe.yml` lines 132-133. No new dependencies are introduced. No finding.

### 5. Correlated failure from shared GITHUB_TOKEN rate limit (No issue, already documented)

The Consequences section (line 137) correctly identifies correlated failure risk from shared network state, including GitHub API rate limits. The current per-job model gives each job its own rate-limit window for GITHUB_TOKEN (each job gets a fresh token). After consolidation, all serialized tests in one job share a single token's rate-limit budget.

This is documented in the design under "Trade-offs Accepted" and mitigated by per-test isolation and the observation that most downloads use GitHub releases (CDN, separate from API rate limits). No additional finding needed.

### 6. Container image tag pinning (Out of scope, noted)

The design uses floating tags (`debian:bookworm-slim`, `alpine:3.21`) rather than digest-pinned images (`debian@sha256:...`). This is a supply chain concern, but it's the pre-existing state in `test-recipe.yml` and all other workflows. The consolidation design shouldn't be burdened with fixing a pre-existing practice. Noted for completeness only.

## Recommendations

1. **Update Execution Isolation section** to explicitly acknowledge that read-write volume mounts allow containers to modify host workspace files, and document why this is accepted (ephemeral runners, official images, pre-existing pattern). One to two sentences suffice.

2. **Update Container Loop Pattern and User Data Exposure section** to address GITHUB_TOKEN passing for the checksum-pinning and homebrew migration targets. Show `-e GITHUB_TOKEN="$GITHUB_TOKEN"` in the pattern for workflows that need it. Change "not affected" to "handled consistently with existing test scripts."

3. **No other changes needed.** The "Download Verification" and "Supply Chain Risks" N/A justifications are correct. The correlated-failure risk is already well documented in Trade-offs Accepted.
