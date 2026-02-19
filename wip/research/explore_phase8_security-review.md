# Security Review: GPU Backend Selection Design

Reviewer: Architect Reviewer (Security Focus)
Document: `docs/designs/DESIGN-gpu-backend-selection.md`
Date: 2026-02-19

---

## Scope of Review

This review focuses on the security implications of the GPU backend selection design for a tool that downloads and executes binaries. It covers:
1. Attack vectors not considered in the design's Security Considerations section
2. Sufficiency of proposed mitigations
3. Residual risk assessment
4. "Not applicable" justification review
5. Fake library injection via library probing
6. `.variant` file TOCTOU and tampering risks
7. Supply chain impact of expanding from 5 to 10 manifest entries

## 1. Attack Vectors Not Considered

### 1a. Library planting to influence variant selection (NEW, SIGNIFICANT)

The design proposes `os.Stat()` calls against hardcoded paths like `/usr/lib/x86_64-linux-gnu/libcuda.so.1` to determine which backend to download. An attacker who can write files to any of the probed paths can influence which binary variant gets downloaded and executed.

**Scenario**: An attacker places a fake `libcuda.so.1` at one of the probed paths. The Go-side detection sees "CUDA available," downloads the CUDA variant, and runs it. If the attacker controls the fake library, the Rust binary may load it at runtime via the dynamic linker (not via Go's `os.Stat`, which is safe -- but the Rust binary's runtime `dlopen`/implicit linking is a different story).

**Assessment**: The Go-side probing itself is safe (read-only `stat()` calls, as the design correctly states). The risk is indirect: the probed file influences which binary is downloaded, and the downloaded binary then interacts with that same library at runtime. However:
- An attacker who can write to `/usr/lib/x86_64-linux-gnu/` already has root or equivalent write access to system library directories, which means the system is already compromised.
- The detection only affects _which_ verified binary gets downloaded. The binary itself is still SHA256-verified against the embedded manifest. The attacker cannot change _what_ binary runs, only _which variant_ runs.
- The Rust binary loading a fake GPU library is a runtime attack on the Rust binary, not on tsuku. This is equivalent to any binary loading a compromised system library.

**Severity**: Low. Requires existing root-equivalent access. The probing influences variant selection, not binary content. The design should note this explicitly in the Security Considerations section: "Library probing uses `os.Stat()` on system library directories. An attacker with write access to these directories could influence variant selection, but the downloaded binary is still SHA256-verified. Such an attacker already has the ability to compromise any binary on the system."

### 1b. Config file injection to force backend selection

The `llm.backend` config override in `$TSUKU_HOME/config.toml` provides a direct path to control which variant is downloaded. An attacker who can write to the config file can force any valid backend name.

**Assessment**: `$TSUKU_HOME/config.toml` is a user-owned file with 0600 permissions (enforced by `userconfig.go:162`). The `loadFromPath` function warns on permissive permissions (`userconfig.go:116-123`). An attacker who can write to this file can already modify any tsuku configuration (providers, secrets, etc.), so this isn't a new attack surface. The design should validate that the `llm.backend` config value is one of the known backend names (cuda, vulkan, cpu, metal) before using it as a manifest lookup key.

**Severity**: Low (pre-existing risk). The config validation is a minor hardening recommendation.

### 1c. Backend name injection in manifest lookup

The `llm.backend` config value is used to look up `platforms[platformKey].variants[backend]`. If the backend name is not validated, and the Go code uses it in path construction (`$TSUKU_HOME/tools/tsuku-llm/<version>/<backend>/tsuku-llm`), a malicious config value like `../../bin` could cause path traversal.

**Assessment**: The design includes the backend name in the directory path. If `backend` comes from user config without validation, a value like `../../../etc` would construct a path outside the expected directory. However:
- The manifest lookup (`variants[backend]`) would fail for any backend name that isn't in the manifest's variant map, so the download flow would abort before creating any directory.
- Even if somehow bypassed, the binary is downloaded from a URL specified in the embedded manifest, so the content is controlled.
- The `os.MkdirAll` call in `downloadAddon` (`manager.go:168`) would create directories based on the path, but only after manifest validation.

**Recommendation**: Validate backend names against an allowlist (`cuda`, `vulkan`, `metal`, `cpu`) before using them in path construction. This is defense-in-depth. Even though the manifest lookup provides implicit validation, explicit validation at the input boundary is clearer.

**Severity**: Low (mitigated by manifest lookup), but the explicit validation is worth adding.

### 1d. Environment passthrough to addon binary (PRE-EXISTING)

`lifecycle.go:188` passes `os.Environ()` unsanitized to the addon binary:

```go
cmd.Env = os.Environ()
```

The codebase already has an environment sanitization pattern in `internal/verify/dltest.go:300-327` (`sanitizeEnvForHelper`) that strips dangerous loader variables (`LD_PRELOAD`, `LD_AUDIT`, `DYLD_INSERT_LIBRARIES`, etc.) before spawning helper processes. The addon lifecycle doesn't use this pattern.

An attacker who can set environment variables (e.g., via a malicious shell profile, `.env` file, or parent process) could inject `LD_PRELOAD=/path/to/malicious.so` into the tsuku-llm addon process, causing arbitrary code execution in the GPU inference context.

**Assessment**: This is a pre-existing issue, not introduced by the GPU backend selection design. However, the design adds new binary variants with GPU library dependencies that interact with the dynamic linker, making the environment passthrough slightly more relevant. The GPU variants will `dlopen` CUDA/Vulkan libraries, and `LD_PRELOAD` could intercept these.

**Recommendation**: The design should either:
1. Apply the same `sanitizeEnvForHelper` pattern from `internal/verify/dltest.go` when launching the addon, or
2. Note this as a known limitation with a reference to the pre-existing pattern.

This is not blocking for the GPU backend design specifically, but it's worth calling out since GPU inference with untrusted environment variables is a higher-risk scenario than CPU-only inference.

**Severity**: Medium (pre-existing, not introduced by this design, but compounded by GPU library loading).

### 1e. `file://` URL in manifest (PRE-EXISTING)

The current `manifest.json` contains a `file://` URL:

```json
"linux-amd64": {
  "url": "file:///home/dgazineu/.tsuku/tools/tsuku-llm/0.1.0/tsuku-llm",
  "sha256": "526f663e88f6bdc461f6df51590b39fb0db6ecbbcdea428bdcdd3f5987d979ca"
}
```

Go's `http.DefaultClient` with `http.DefaultTransport` does not support `file://` URLs -- `http.NewRequestWithContext` would construct the request but `http.DefaultClient.Do` would fail with a protocol error. This appears to be a development artifact, not a functional entry.

However, if a custom transport were ever added (or if Go's default behavior changed), `file://` URLs could read arbitrary local files and write them to the temp download path. The SHA256 check would still apply (the content must match the embedded checksum), so this can't be exploited to execute arbitrary files, but it could leak file contents by timing the checksum verification response.

**Assessment**: Pre-existing, not introduced by this design. The `file://` URL in the manifest appears to be a development leftover. Since the manifest is `//go:embed`'d at compile time, this URL would ship in the binary. It should be cleaned up to only use HTTPS URLs.

**Severity**: Low (currently non-functional, but should be cleaned up).

## 2. Mitigation Sufficiency

### 2a. Embedded manifest via `//go:embed` -- SUFFICIENT

The design correctly identifies that the manifest is embedded at compile time and cannot be tampered with without rebuilding the tsuku binary. This is a strong supply chain control. The Go compiler embeds `manifest.json` as a byte slice in the binary, and there's no runtime mechanism to override it.

I verified this in the codebase: `manifest.go:10-11` shows:

```go
//go:embed manifest.json
var manifestData []byte
```

The `cachedManifest` variable (`manifest.go:31`) caches the parsed result. There is no code path that loads a manifest from disk or network. The assumption in the phase 4 review (section F) -- "the manifest is only read from `//go:embed`, never from a cached file" -- is confirmed.

### 2b. SHA256 verification flow -- SUFFICIENT WITH CAVEAT

The download flow (download to temp, verify checksum, chmod, atomic rename) is sound and doesn't change. Each variant gets its own SHA256 checksum in the manifest.

**Caveat**: The design introduces a `.variant` file alongside the binary (see section 6 below). The `.variant` file is not itself checksum-verified. Its content determines which SHA256 checksum is used for verification. This creates a subtle dependency that needs analysis (covered in section 6).

### 2c. Library probing uses `os.Stat()` only -- SUFFICIENT

The design explicitly states: "GPU library probing uses `os.Stat()` calls only, never loads or executes the probed libraries." This is confirmed by the design's detection approach. `os.Stat()` reads file metadata from the filesystem via the `stat` syscall and does not open, read, or execute the file. This is safe even against maliciously crafted files.

### 2d. No new network calls -- SUFFICIENT

The only network activity is the existing download flow from URLs in the embedded manifest. Library probing is entirely local. The config override is read from a local file. No new exfiltration channels are introduced.

## 3. Residual Risk Assessment

### 3a. False positive detection leading to broken addon (MODERATE, ACKNOWLEDGED)

Library file existence doesn't guarantee backend functionality. A system with `libvulkan.so.1` present (from mesa) but no working GPU driver will download the Vulkan variant, which then fails at runtime. The design acknowledges this and defers automatic fallback.

**Residual risk**: Users hit a broken addon and must manually run `tsuku config set llm.backend cpu`. The error message from the Rust side must be clear enough for non-technical users to follow.

**Recommendation**: The design should specify the exact error message format and verify it includes the corrective command. The current design shows a sample error message (lines 377-383) which is good, but the Go side should also detect process exit failure and surface the suggestion, not rely on users reading stderr.

### 3b. No binary signing beyond macOS ad-hoc (MODERATE, PRE-EXISTING)

The release pipeline (`llm-release.yml:142-144`) uses `codesign -s -` for macOS, which is an ad-hoc signature (no identity). For Linux, there is no binary signing. The only integrity guarantee is SHA256 checksums in the embedded manifest.

With 10 variants instead of 5, there are more binaries that rely solely on checksum verification. If the release pipeline's GitHub Actions environment is compromised, an attacker could produce binaries with valid checksums (by computing them after modification) and embed them in the manifest before the Go binary is built.

**Assessment**: This risk exists today and doesn't change qualitatively with more variants. The checksum-only approach is standard for tools distributed via GitHub releases (similar to `goreleaser` projects). Sigstore/cosign would add an independent verification layer, but that's a separate design concern.

**Residual risk**: Accepted. The embedded manifest provides integrity verification against download-time tampering. Build-time tampering requires compromising the CI pipeline itself.

### 3c. Pre-execution verification gap during variant switch (NEW, LOW)

When a user switches `llm.backend` from `vulkan` to `cuda`, the next `EnsureAddon` call will:
1. Read the new backend from config
2. Construct a new path (`<version>/cuda/tsuku-llm`)
3. Not find a binary there
4. Download the CUDA variant
5. Verify and install

The old Vulkan binary remains on disk. If `verifyBinary()` is called with the old path but the new manifest checksum, it would fail and trigger a re-download. The design handles this by including the backend in the path, so old and new variants don't collide.

**Residual risk**: Minimal. The path separation ensures variants don't interfere.

## 4. "Not Applicable" Justification Review

The design's Security Considerations section has four subsections. None are marked "not applicable" explicitly, but several areas are omitted:

### 4a. Code execution of probed libraries -- correctly treated as N/A

The design explicitly says probing uses `os.Stat()` only. This is correct and sufficient.

### 4b. Privilege escalation -- implicitly N/A, CORRECT

The addon runs with the same user permissions as tsuku. No setuid, no capability changes, no privilege boundaries. GPU library access doesn't require elevated privileges (GPU device access is typically via group membership, e.g., `video` or `render` groups, which the user already has or doesn't). This is correctly not addressed because there's no privilege change.

### 4c. Network exposure -- implicitly N/A, MOSTLY CORRECT

The addon communicates via Unix domain socket, which is local-only. No new network listeners. However, the design doesn't mention that the Unix socket permissions should restrict access to the current user. Looking at `lifecycle.go`, the socket path is `$TSUKU_HOME/llm.sock`. The default socket permissions on Linux are 0755 for the directory, and the socket itself inherits umask. Another user on a shared system could connect to the socket. This is a pre-existing concern, not introduced by this design.

### 4d. Denial of service via detection -- NOT ADDRESSED, LOW RISK

A malicious program could repeatedly create/remove files at the probed library paths to cause different backend selections on each `EnsureAddon` call, potentially causing repeated downloads. This is extremely unlikely in practice (requires write access to system library directories) and bounded by the fact that downloads are expensive (50-200MB) but the checksums prevent execution of wrong binaries.

**Assessment**: Correctly not addressed. This is a theoretical concern with no practical impact.

## 5. Fake Library Injection via Library Probing

This is the specific question: "Can an attacker place a fake `libcuda.so` to influence variant selection?"

### Analysis

**Write access required**: The probed paths are all system library directories:
- `/usr/lib/x86_64-linux-gnu/` -- requires root
- `/usr/lib64/` -- requires root
- `/usr/lib/` -- requires root
- `/usr/local/cuda/lib64/` -- requires root (or CUDA installer permissions)

An unprivileged attacker cannot write to any of these paths. A privileged attacker who can write to these paths has already compromised the system.

**What the attacker gains**: If an attacker plants `libcuda.so.1` at a probed path:
1. Go-side detection reports "CUDA available"
2. CUDA variant is downloaded (from embedded manifest URL, SHA256 verified)
3. CUDA variant binary is executed
4. The Rust binary attempts to use CUDA, loading the fake `libcuda.so.1` via the dynamic linker

Step 4 is where the real risk lies, but it's not specific to tsuku's detection. Any CUDA application on the system would load the fake library. The detection merely caused a different (legitimate, verified) binary to be downloaded.

**Symlink attacks**: The design doesn't mention whether the probed paths are checked via `os.Stat()` (follows symlinks) or `os.Lstat()` (doesn't follow symlinks). If `os.Stat()` is used (the design says `os.Stat()` explicitly), a symlink at a non-probed path couldn't affect detection. But if a symlink is placed at a probed path pointing to a different file, `os.Stat()` would follow it and report the file as existing. Again, this requires write access to system directories.

**Container environments**: In Docker containers with NVIDIA Container Toolkit, GPU libraries are bind-mounted into the container. The mount paths align with the probed paths for Debian-based images. An image author could include fake GPU libraries that influence detection, but the user chose to run that image. This is within the user's trust boundary.

### Verdict

The fake library injection attack requires root-equivalent access and only influences which verified binary is downloaded, not the binary's content. The design's claim that "GPU library probing uses `os.Stat()` calls only, never loads or executes the probed libraries" is the correct security boundary. The subsequent runtime library loading by the Rust binary is outside tsuku's control and is the same risk any GPU application faces.

**No additional mitigation needed.** The design should add one sentence noting that probed paths are system-owned directories requiring privileged write access.

## 6. `.variant` File TOCTOU and Tampering Risks

### The Role of `.variant`

The `.variant` file records which backend variant was downloaded (e.g., "vulkan"). It's used by `verifyBinary()` to look up the correct SHA256 checksum from the manifest. Without it, after a config change, verification would use the new backend's checksum against the old binary, causing a false mismatch.

### TOCTOU Analysis

**Race 1: Between writing `.variant` and verifying the binary.**

The download flow is: download to temp -> verify checksum -> chmod -> atomic rename -> write `.variant`. If the process crashes between rename and `.variant` write, the binary exists but the `.variant` file is missing or stale.

**Impact**: On next startup, `verifyBinary()` can't determine which variant to verify against. It would either:
- Fall back to re-detecting the backend (and if the config changed, verify against the wrong checksum, triggering a re-download -- this is safe, just wasteful)
- Fail with an error, prompting re-download

**Recommendation**: Write `.variant` atomically (write to temp, rename) and write it _before_ the final binary rename, or in the same atomic operation. Alternatively, encode the variant in the directory path (which the design already does: `<version>/<backend>/tsuku-llm`), making the `.variant` file redundant -- the directory name IS the variant identifier.

**Wait -- the design DOES encode variant in the path**: `$TSUKU_HOME/tools/tsuku-llm/<version>/<backend>/tsuku-llm`. This means the `.variant` file is redundant. The directory name `vulkan/` already tells `verifyBinary()` which variant to check. The `.variant` file and the directory name would need to agree, creating two sources of truth.

**Recommendation (SIGNIFICANT)**: Drop the `.variant` file entirely. The backend is already encoded in the directory path. `verifyBinary()` can extract the backend name from the binary's parent directory name. This eliminates the TOCTOU risk, the dual-source-of-truth problem, and the `.variant` file write/read code. The design already has the right directory structure; the `.variant` file is unnecessary.

### Tampering Analysis

If `.variant` is kept:

**Scenario 1**: An attacker modifies `.variant` to say "cpu" when the actual binary is the Vulkan variant. On next verification, `verifyBinary()` looks up the CPU checksum and compares it against the Vulkan binary. Checksum mismatch -> re-download. The attacker caused a re-download but can't cause execution of a wrong binary (verification still happens after download).

**Scenario 2**: An attacker replaces both the binary AND `.variant`. The replacement binary must match the SHA256 checksum of the variant named in `.variant`, which comes from the embedded manifest. The attacker can't forge a binary that matches an embedded checksum, so this attack fails.

**Assessment**: `.variant` tampering can cause unnecessary re-downloads (denial of service on bandwidth) but cannot cause execution of unverified binaries. The checksum verification against the embedded manifest is the trust anchor, not the `.variant` file.

### Verdict

The `.variant` file is both redundant (the directory path encodes the same information) and a minor tampering surface. Removing it simplifies the design and eliminates the TOCTOU and dual-source-of-truth concerns with no security cost.

## 7. Supply Chain with 10 Manifest Entries

### Quantitative Impact

Moving from 5 to 10 entries in the embedded manifest doubles the number of:
- SHA256 checksums to manage
- URLs to verify during manifest generation
- Binaries built in CI
- Artifacts in each GitHub release

### CI Pipeline Analysis

Looking at `llm-release.yml`:
- Each variant is built in a separate GitHub Actions job with a specific feature flag
- The `create-release` job generates checksums via `sha256sum`
- The `finalize-release` job verifies all 9 expected artifacts (8 binaries + checksums.txt) are present
- No binary signing beyond macOS ad-hoc codesign

The finalize step (lines 257-282) already lists all 8 binary variants plus checksums.txt. Adding more variants means updating this list. If an artifact is missing, the release fails. This is a good completeness check.

### Manifest Generation Gap

The design doesn't specify how the manifest is updated after a release. Currently, the SHA256 checksums are generated in CI (`sha256sum tsuku-llm-* > checksums.txt`). Somebody must take these checksums and update `manifest.json` in the Go source, then rebuild tsuku.

With 10 entries, the manual step of copying checksums has more room for error (wrong checksum for wrong variant, transposed entries). The design should address whether manifest generation will be automated.

**Recommendation**: Automate manifest generation as part of the release pipeline. The CI already computes checksums; it should produce a `manifest.json` that can be committed directly. Manual checksum copying with 10 entries is error-prone.

### Risk of Partial Release

With 5 entries, a CI failure on one platform blocks the release. With 10 entries, a failure on `linux-arm64-cuda` (for example) could block the entire release even though 9 other variants built successfully. The `fail-fast: false` setting in the CI matrix means all builds complete regardless of individual failures, but `finalize-release` checks for all expected artifacts.

**Assessment**: The all-or-nothing release approach is correct for security (no partial manifests with missing checksums). The risk is availability, not security. If a GPU variant fails to build, the release is delayed but no insecure state results.

### No Increased Signing Risk

The design states: "The release pipeline produces signed binaries with checksums." Looking at the actual pipeline, this is slightly inaccurate: only macOS binaries get ad-hoc codesign. Linux binaries have checksums only.

The security posture is the same regardless of whether there are 5 or 10 entries: SHA256 checksums embedded at compile time, with no external signature verification. The risk doesn't scale with the number of entries because the trust anchor (embedded manifest) is the same.

### Verdict

The supply chain story is sound at 10 entries. The primary risk is operational (manual checksum management at 2x scale) rather than security. Automating manifest generation would address this. The integrity verification model (embedded checksums, no runtime manifest fetching) is unchanged and sufficient.

## Summary of Findings

### Findings That Should Be Addressed in the Design

1. **Drop the `.variant` file** (Section 6). The directory structure already encodes the backend name (`<version>/<backend>/tsuku-llm`). The `.variant` file creates a dual-source-of-truth with TOCTOU risk on crash recovery. Extract the backend from the directory path instead. This is a simplification, not additional work.

2. **Validate `llm.backend` config values against an allowlist** (Section 1c). Before using the backend name in path construction or manifest lookup, check it against `{cuda, vulkan, metal, cpu}`. Defense-in-depth against path traversal even though the manifest lookup provides implicit validation.

3. **Note the library path privilege requirement** (Section 5). Add one sentence to Security Considerations: probed paths are system-owned directories requiring privileged write access, so library planting requires an already-compromised system.

4. **Automate manifest generation** (Section 7). With 10 entries, manual checksum copying is error-prone. This is an operational risk, not a security risk per se, but checksum errors could cause download verification failures in the field.

### Pre-Existing Issues Worth Escalating (Not Introduced by This Design)

5. **Unsanitized environment passthrough to addon binary** (Section 1d). `lifecycle.go:188` passes `os.Environ()` directly. The codebase has an established `sanitizeEnvForHelper` pattern in `internal/verify/dltest.go` that strips `LD_PRELOAD`, `LD_AUDIT`, `DYLD_INSERT_LIBRARIES`, etc. The addon lifecycle should use the same pattern. This becomes slightly more relevant with GPU variants because the Rust binary will load GPU libraries via the dynamic linker.

6. **`file://` URL in manifest.json** (Section 1e). The current manifest contains a `file://` URL that appears to be a development artifact. It's non-functional (Go's `http.DefaultClient` doesn't support `file://`), but it shouldn't ship in the embedded manifest.

### Accepted Residual Risks

7. **False positive detection** (Section 3a). Library exists but backend doesn't work. Mitigated by manual `llm.backend` override and clear error messages. Automatic fallback deferred to follow-up.

8. **No binary signing on Linux** (Section 3b). Checksums only, no Sigstore/cosign. Standard for the distribution model. Would require a separate design to add.

9. **Detection accuracy on non-Debian distros** (covered in phase 4 review). Missing Fedora/Arch paths is a UX issue, not a security issue -- wrong detection leads to CPU download, which is safe.
