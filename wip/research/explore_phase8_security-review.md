# Security Review: Container Image Digest Pinning

## Executive Summary

The design is sound for what it covers. Digest pinning is the right call for a tool that downloads and executes binaries inside containers. The main concern isn't what the design does -- it's what it leaves unpinned. Three hardcoded `ubuntu:22.04` and `debian:bookworm-slim` image constants remain outside `container-images.json`, outside CODEOWNERS protection, and outside Renovate's update scope. The design acknowledges this as out-of-scope, but the security gap is real today and should be explicitly tracked as residual risk.

---

## 1. Attack Vectors

### 1.1 Covered by the Design

**Registry tag poisoning.** An attacker who compromises a registry account (or exploits a registry vulnerability) pushes malicious content under a trusted tag like `alpine:3.21`. With digest pinning, the container runtime rejects the pull because the manifest digest doesn't match. This is the primary attack the design mitigates, and it does so correctly.

**Silent supply chain drift.** Without digests, a tag can resolve to different content over time. With the Renovate + CODEOWNERS flow, every content change requires a reviewed PR. The audit trail is good.

**Cache poisoning via stale images.** The `ContainerImageName()` hash now includes the digest, so cache entries become content-addressed. A tag update that doesn't change the digest (same manifest, same cache key) is correct behavior, not a vulnerability.

### 1.2 Not Covered -- Requires Attention

**Attack Vector: Unpinned hardcoded images (residual risk, HIGH)**

Three image constants remain outside `container-images.json`:

| Constant | Location | Value |
|----------|----------|-------|
| `SourceBuildSandboxImage` | `internal/sandbox/requirements.go:16` | `ubuntu:22.04` |
| `DefaultValidationImage` | `internal/validate/executor.go:20` | `debian:bookworm-slim` |
| `SourceBuildValidationImage` | `internal/validate/source_build.go:19` | `ubuntu:22.04` |

These are not protected by CODEOWNERS (the CODEOWNERS file only guards `/container-images.json`, `/.github/workflows/**`, and `/scripts/**`). They are not tracked by Renovate. They are not covered by digest pinning. A malicious or careless change to any of these constants goes through normal PR review without security team approval.

The drift-check workflow's `hardcoded-references` job already has an explicit exception for `DefaultValidationImage`:

```
'internal/validate/executor\.go:.*DefaultValidationImage'
```

This means the CI safety net deliberately lets this hardcoded reference through. The design document correctly marks these as out-of-scope (sandbox image unification tracks them), but the security posture is inconsistent: five images get digest pinning + CODEOWNERS + Renovate, while three images with identical risk profiles get none of those protections.

**Recommendation:** The sandbox image unification design should be prioritized as a security follow-up, not just a cleanup. At minimum, consider adding `internal/sandbox/requirements.go` and `internal/validate/*.go` files containing image constants to CODEOWNERS before the unification lands.

**Attack Vector: Renovate PR auto-merge**

The design relies on Renovate PRs reviewed by CODEOWNERS. If Renovate is ever configured with auto-merge for digest-only updates (a common pattern to reduce noise), the CODEOWNERS gate is bypassed. The design doesn't mention auto-merge policy.

**Recommendation:** Explicitly state that Renovate auto-merge MUST NOT be enabled for `container-images.json` updates. Document this as an invariant.

**Attack Vector: Compromised Renovate bot**

Renovate has write access to open PRs with arbitrary content. A compromised Renovate instance could propose a PR that changes a digest to point at a malicious manifest. The CODEOWNERS review catches this, but reviewers would need to verify the digest independently (e.g., run `crane digest` themselves) rather than trusting Renovate's output.

**Recommendation:** This is a known supply chain risk with any dependency bot. The CODEOWNERS gate is the correct mitigation. Consider documenting a verification step in the review process: "For digest-only updates, spot-check at least one digest with `crane digest <image>` before approving."

**Attack Vector: Multi-arch manifest confusion**

The design says to use manifest list digests (architecture-independent). However, if an attacker can push a manifest list where the `linux/amd64` entry points to a malicious image while other architectures remain clean, the digest of the manifest list itself changes -- so pinning does catch this. But the attack surface exists if someone pins a platform-specific manifest digest and a different platform pulls a different manifest.

The design handles this correctly by specifying manifest list digests. No action needed, but worth calling out that the choice matters.

### 1.3 Not Applicable -- Correctly Excluded

**User data exposure.** The design correctly marks this as not affected. The images are public base images from public registries. No credentials or user data flow through `container-images.json`. Confirmed by code review: `ImageForFamily()` returns a string used in `FROM` directives and cache key hashing, nothing more.

**Execution isolation.** Correctly not affected. Digest pinning changes identification, not execution. The sandbox isolation code (`podmanRuntime.buildArgs`, `dockerRuntime.buildArgs`) applies `--network=none`, `--ipc=none`, memory/CPU/PID limits, and read-only mounts regardless of how the image is referenced.

---

## 2. Mitigation Sufficiency

### 2.1 CODEOWNERS Protection

`container-images.json` requires approval from both `@tsukumogami/core-team` and `@tsukumogami/security-team`. This is correct and matches the protection level for workflow files. The rationale in CODEOWNERS is explicit about why:

> A single edit redirects every consumer at once, so it needs the same protection as workflow files.

Sufficient for the images it covers. Insufficient for the three hardcoded constants it doesn't cover.

### 2.2 Renovate Regex

The proposed regex:

```
"(?<depName>[a-z][a-z0-9./-]+):(?<currentValue>[a-z0-9][a-z0-9._-]+)(@(?<currentDigest>sha256:[a-f0-9]+))?"
```

This is correct. The digest capture group is optional, which allows a gradual rollout (Renovate can match entries before digests are added). The `autoReplaceStringTemplate` correctly reconstructs the full reference.

One minor note: the regex uses `[a-f0-9]+` for the digest hex, which is unbounded. In practice, SHA256 digests are always 64 hex chars. A stricter `[a-f0-9]{64}` would reject malformed digests, but Renovate validates digests through the Docker datasource anyway, so this is cosmetic.

### 2.3 Cache Key Integrity

`ContainerImageName()` at `container_spec.go:368` hashes the full `BaseImage` string including the digest suffix. This means:

- Same tag, different digest = different cache key (correct)
- Same digest, different tag = different cache key (correct, tags are for readability)
- Digest removed = different cache key (correct, falls back to tag-only)

The hash uses SHA256 with first 16 hex chars (64 bits). Collision probability is negligible for the number of distinct image configurations in practice.

---

## 3. Residual Risk Summary

| Risk | Severity | Mitigated By | Residual |
|------|----------|--------------|----------|
| Registry tag poisoning | High | Digest pinning | Low (for pinned images only) |
| Unpinned hardcoded images | High | Nothing (out of scope) | **High** |
| Renovate auto-merge bypass | Medium | CODEOWNERS (if not auto-merged) | Medium (policy-dependent) |
| Compromised Renovate bot | Medium | CODEOWNERS review | Low |
| Multi-arch manifest confusion | Low | Manifest list digest choice | Negligible |
| Stale cache hits | Medium | Digest in cache key hash | Negligible |

**Items to escalate:**

1. The three unpinned hardcoded image constants should get interim CODEOWNERS protection before sandbox image unification lands. This is a one-line CODEOWNERS addition per file and closes the gap without waiting for the full unification.

2. Renovate auto-merge policy for `container-images.json` should be documented as "never auto-merge" or the CODEOWNERS protection is effectively decorative.

---

## 4. Review of "Not Applicable" Justifications

| Section | Justification | Verdict |
|---------|---------------|---------|
| Execution Isolation | "Not affected" -- digest changes identification, not execution | **Correct.** Confirmed by code review of `buildArgs()` in both runtimes. |
| User Data Exposure | "Not affected" -- public images, no user data | **Correct.** No secrets or user data in the image reference flow. |

Both "not applicable" justifications hold up against the code.

---

## 5. Findings Summary

1. **HIGH: Unpinned hardcoded images.** Three image constants (`SourceBuildSandboxImage`, `DefaultValidationImage`, `SourceBuildValidationImage`) have the same attack surface as `container-images.json` entries but lack digest pinning, CODEOWNERS protection, and Renovate tracking. Add interim CODEOWNERS rules for these files.

2. **MEDIUM: No auto-merge policy documented.** The design's security model depends on human review of Renovate PRs. If auto-merge is enabled for digest updates, the CODEOWNERS gate becomes ineffective. Document the policy.

3. **LOW: No reviewer verification guidance.** Reviewers approving digest-only Renovate PRs should verify at least one digest independently. Consider adding a review checklist note.

4. **No issues with the design's core approach.** Inline `tag@digest` format, Renovate regex updates, cache key behavior, and consumer transparency are all correct and appropriately scoped.
