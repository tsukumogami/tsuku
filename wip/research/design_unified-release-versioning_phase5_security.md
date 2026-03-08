# Security Review: unified-release-versioning

## Dimension Analysis

### Download Verification
**Applies:** Yes

This design changes how artifact names are constructed and how recipes resolve download URLs. The `github_file` action downloads binaries from GitHub releases.

**Risks:**
- **Asset pattern mismatch**: If the recipe's `asset_pattern` doesn't match the actual release asset name after naming standardization, the download could fail silently or match an unexpected file. Severity: Medium.
- **Checksum continuity**: The `finalize-release` job generates `checksums.txt` covering all artifacts. Changing artifact names means all checksum entries change. If checksums are generated before all artifacts are uploaded, partial checksums could pass verification. Severity: Low (finalize-release already waits for all builds).

**Mitigations:**
- Integration-test validates binary execution before finalize-release publishes. A name mismatch would cause download failure in integration-test, blocking release.
- checksums.txt is generated after all artifacts are uploaded (existing behavior, unchanged).
- Recipe asset patterns are updated in the same batch as the naming change (Phase 5), ensuring consistency.

### Execution Isolation
**Applies:** Partially

Version pinning adds auto-reinstall behavior: when a version mismatch is detected, the CLI automatically installs the correct version of dltest/llm. This invokes the recipe system, which downloads and installs binaries.

**Risks:**
- **Automatic binary replacement**: A user running tsuku could trigger an automatic download and installation without explicit consent. This already exists for dltest (the pinning pattern is established). Extending it to llm follows the same security model. Severity: Low (consistent with existing behavior).
- **Daemon shutdown**: Version mismatch handling for llm requires stopping a running daemon. If the daemon is serving active requests, shutdown could interrupt inference. Severity: Low (graceful shutdown via gRPC is already implemented).

**Mitigations:**
- Auto-reinstall uses the same recipe system and download verification as manual `tsuku install`. No privilege escalation.
- The `Prompter` interface in `AddonManager` can gate auto-downloads with user confirmation.

### Supply Chain Risks
**Applies:** Yes

This design changes the source of version resolution for tsuku-llm from `tsukumogami/tsuku-llm` (separate repo) to `tsukumogami/tsuku` (main repo). It also consolidates all builds into a single workflow.

**Risks:**
- **Single point of compromise**: Moving all builds into one workflow means a compromise of `release.yml` affects all three binaries. Currently, llm builds are in a separate workflow. Severity: Low (both workflows are in the same repo with the same access controls; consolidation doesn't change the threat surface).
- **Version resolution source change**: Changing the llm recipe from `tsukumogami/tsuku-llm` to `tsukumogami/tsuku` changes which GitHub repo's tags are trusted for version resolution. Since both repos are owned by the same organization, this doesn't change the trust boundary. Severity: Low.

**Mitigations:**
- GitHub Actions workflows are protected by branch protection rules and required reviews.
- checksums.txt provides integrity verification for all artifacts.
- GoReleaser signing (if configured) applies to all builds equally.

### User Data Exposure
**Applies:** No

This design changes release infrastructure and version enforcement. It does not access, transmit, or store user data. The gRPC `addon_version` field in StatusResponse reports the binary version, not user data. Version information is already logged locally.

## Recommended Outcome

**OPTION 2 - Document considerations:**

The design has relevant security dimensions (download verification and supply chain) that warrant brief documentation. The risks are low and mitigated by existing infrastructure, but should be noted for implementers.

## Summary
The design's security profile is consistent with the existing dltest pinning pattern. The main considerations are ensuring artifact naming changes are coordinated with checksum generation and recipe asset patterns, and that auto-reinstall follows the same download verification as manual installation. No design changes needed; considerations are worth documenting briefly.
