# Security Review: self-update

## Dimension Analysis

### External Artifact Handling

**Applies:** Yes

The design downloads two artifacts from GitHub: `checksums.txt` and the platform binary (`tsuku-{os}-{arch}`). Both arrive over HTTPS (GitHub's release CDN). The binary is verified against a SHA256 hash extracted from `checksums.txt`.

**Risks:**

1. **Checksums.txt has no independent verification.** The checksum file itself is downloaded from the same GitHub release as the binary. If an attacker compromises the GitHub release (stolen maintainer token, compromised CI pipeline, or GitHub infrastructure breach), they can replace both the binary and `checksums.txt` simultaneously. The checksum verification then passes on a malicious binary. This is integrity verification (detects corruption and CDN errors) but not authenticity verification (does not prove the binary was produced by a trusted party).

   *Severity: Medium.* The attack requires compromising the GitHub release, which is a high-barrier prerequisite. But the consequence -- executing an attacker-controlled binary as the user -- is severe. The net severity is medium because the attack surface (a single GitHub repo's release permissions) is narrow but the impact is full code execution.

2. **No signature verification.** The existing recipe system already supports GPG signature verification (`signature_url`, `signature_key_url`, `signature_key_fingerprint` parameters in `DownloadAction`). The self-update design omits this entirely. GoReleaser supports signing releases with cosign or GPG, but the current `.goreleaser.yaml` has no signing configuration.

   *Severity: Medium.* This is the root cause of risk #1. Without cryptographic signatures from a key not hosted alongside the artifacts, there is no way to verify that the release was produced by an authorized build pipeline.

3. **TOCTOU between checksums.txt download and binary download.** The design downloads `checksums.txt` first, parses the expected hash, then downloads the binary in a separate request. A release could theoretically be re-published (deleted and recreated with different assets) between these two fetches. In practice, GitHub does not allow re-uploading assets to an existing release without deleting the release first, so this window is extremely narrow and requires release admin access.

   *Severity: Low.* The window requires the same access level as risk #1, but with a timing constraint that makes it impractical. Both files are fetched within seconds of each other.

4. **Checksums.txt parsing.** The file is line-oriented (`{hash}  {filename}`). If the parser does not strictly validate the format, a malformed `checksums.txt` could cause unexpected behavior. For example, a filename containing path separators or the hash field being the wrong length.

   *Severity: Low.* The parser is simple (split line, match filename, extract hash) and operates on a small, well-structured file. Implementation should validate hash length (64 hex chars for SHA256) and reject lines that don't match the expected format.

**Mitigations:**

- **Short term (document):** Add a Security Considerations section acknowledging that checksum verification provides integrity but not authenticity. Note this matches the threat model of gh, rustup, and similar self-updating CLIs that rely on GitHub release integrity.
- **Medium term (implement):** Enable GoReleaser's cosign signing. Add cosign signature verification to the self-update flow. This is the standard supply chain hardening step and aligns with Sigstore's keyless signing model (signatures are tied to the GitHub Actions OIDC identity, not a long-lived key).
- **Implementation detail:** Validate SHA256 hash format (exactly 64 lowercase hex characters) during `checksums.txt` parsing. Reject any line that doesn't conform.

### Permission Scope

**Applies:** Yes

The self-update replaces the running binary. This requires write permission to the directory containing the binary and write permission to create a temp file in that same directory.

**Risks:**

1. **Temp file permissions before chmod.** Between `os.CreateTemp` (step 3) and `os.Chmod` (step 5), the temp file has the OS default permissions (typically 0600 on Linux). The downloaded binary content is written during this window. This is not a vulnerability -- the file is more restrictive than needed, not less.

   *Severity: None.* The temp file starts restrictive and is opened to match the existing binary's permissions before being renamed into place.

2. **The .old backup retains the original binary's permissions.** `os.Rename` preserves the source file's permissions. The `.old` file is a valid executable that persists until the next self-update. If the binary directory is world-readable (e.g., `/usr/local/bin/`), the `.old` file is also world-readable, which is identical to the current binary's exposure.

   *Severity: None.* The `.old` file has exactly the same permissions as the binary it came from. No escalation.

3. **Symlink following via `filepath.EvalSymlinks`.** The design resolves symlinks to find the real binary path, then operates on that path. If an attacker can create a symlink race (replacing the symlink target between resolution and temp file creation), the temp file could be created in an attacker-controlled directory. However, this requires write access to the directory containing the binary, which is the same privilege needed to attack the binary directly.

   *Severity: Low.* The attacker already needs the same permissions they'd gain from the attack.

4. **Binary in root-owned directory.** If the binary is at `/usr/local/bin/tsuku` (owned by root), `os.CreateTemp` fails because the user can't write to that directory. The design correctly identifies this as an early failure with a clear error message. No partial state is left.

   *Severity: None (by design).* The failure is clean and early.

**Mitigations:**

- The design already handles this well. The same-directory temp file strategy and early failure on permission errors are correct. No additional mitigations needed for permission scope.

### Supply Chain or Dependency Trust

**Applies:** Yes

The binary is fetched from `tsukumogami/tsuku` GitHub releases. The trust chain is: GitHub account security -> CI pipeline integrity -> GoReleaser build -> release assets.

**Risks:**

1. **Single point of trust: GitHub release integrity.** The entire authenticity guarantee rests on GitHub's access controls for the `tsukumogami/tsuku` repository. A compromised maintainer account, leaked `GITHUB_TOKEN`, or malicious PR that modifies the CI pipeline could produce a poisoned release. This is the standard risk for any tool distributed via GitHub releases.

   *Severity: Medium.* Same severity assessment as External Artifact Handling risk #1 -- this is the same underlying issue viewed from the supply chain perspective.

2. **No build reproducibility or transparency log.** There is no mechanism for users to verify that a given binary was produced from a specific commit. The ldflags embed version and commit hash, but these are self-reported by the build and can be forged.

   *Severity: Low.* Build reproducibility is a defense-in-depth measure. Its absence is common in the ecosystem and does not create a direct vulnerability.

3. **Version resolution trusts GitHub API.** `GitHubProvider.ResolveLatest()` queries the GitHub API to determine the latest version. If an attacker can intercept or spoof this response (MITM on the API call), they could direct the client to download a specific malicious tag. However, the GitHub API uses HTTPS with certificate pinning in Go's standard library.

   *Severity: Low.* HTTPS provides strong protection against MITM. An attacker would need to compromise a trusted CA or the user's trust store.

**Mitigations:**

- **Short term (document):** Acknowledge the GitHub-release trust model in the Security Considerations section.
- **Medium term:** Enable cosign signing in the GoReleaser pipeline (same mitigation as External Artifact Handling). Cosign with keyless signing ties the signature to the GitHub Actions OIDC identity, which means even a compromised maintainer account can't produce a valid signature unless they can also trigger a CI build.
- **Long term:** Consider publishing a Sigstore transparency log entry for each release, enabling independent verification.

### Data Exposure

**Applies:** Yes (minor)

**Risks:**

1. **GitHub API calls reveal user information.** The `GitHubProvider.ResolveLatest()` call hits the GitHub API. If the user has a `GH_TOKEN` or `GITHUB_TOKEN` set (common for developers), this token is sent with the request. The API call reveals the user's IP address and, if authenticated, their GitHub identity to GitHub's servers. This is standard behavior for any GitHub API consumer and not specific to self-update.

   *Severity: Low.* No data beyond what GitHub already collects from normal git/API usage is exposed.

2. **Binary download reveals IP and platform.** The download URL encodes the OS and architecture (`tsuku-linux-amd64`). Combined with the IP address, this tells GitHub (and any network observer of the HTTPS connection metadata) that a specific IP is using tsuku on a specific platform. Again, this is standard for any binary download.

   *Severity: Low.* The information is minimal and comparable to what any package manager reveals during updates.

3. **No local data transmitted.** The self-update command does not send any local state, installed tool list, or usage data to any endpoint. It only makes GET requests to resolve the version and download artifacts.

   *Severity: None.* No telemetry or data exfiltration risk in the self-update path.

**Mitigations:**

- No mitigations needed. The data exposure is minimal and inherent to downloading software from GitHub. The design does not introduce any novel data transmission.

## Recommended Outcome

**OPTION 2 - Document considerations:**

The design should fill in the "Security Considerations" section (currently "TBD (Phase 5)") with the following content:

---

### Security Considerations

**Integrity verification.** Downloaded binaries are verified against SHA256 checksums from the release's `checksums.txt`. This detects corruption during transfer and CDN-level tampering. The checksum parser validates hash format (64 hex characters) and rejects malformed lines.

**Authenticity limitations.** The checksum file is hosted alongside the binary in the same GitHub release. If an attacker gains write access to the release (compromised maintainer credentials, CI pipeline injection), they can replace both artifacts and the checksum verification passes. This is the same trust model used by gh, rustup, cargo-binstall, and other self-updating CLIs that distribute via GitHub releases.

**Future hardening: cosign signatures.** GoReleaser supports Sigstore cosign signing with keyless OIDC-based identity. Adding cosign verification to the self-update flow would tie binary authenticity to the GitHub Actions build identity, not just the release's contents. This is tracked as a follow-up enhancement, not a launch blocker, because the current trust model matches industry standard practice.

**Transport security.** All downloads use HTTPS. Go's `net/http` client validates TLS certificates against the system trust store. No HTTP fallback is supported.

**Permission safety.** The temp file is created in the same directory as the target binary, so `os.CreateTemp` fails early if the user lacks write permission -- before any modification to the existing binary. The `.old` backup retains the original binary's permission bits via `os.Rename`.

**No data transmission.** The self-update command only makes GET requests to GitHub (API for version resolution, CDN for artifact download). No local state, tool list, or usage data is transmitted.

---

## Summary

The design's security posture is adequate for initial release. The primary gap is the absence of cryptographic signature verification -- `checksums.txt` proves integrity (the bytes match what was uploaded) but not authenticity (the upload came from an authorized build). This matches the trust model of every major self-updating CLI that distributes via GitHub releases. Adding cosign signature verification is the clear next step for hardening but is not a launch blocker given the industry-standard baseline. The binary replacement sequence, permission handling, and data exposure profile are all sound.
