# Security Review: Sandbox Image Unification Design

**Reviewer:** maintainer-reviewer
**Date:** 2026-02-22
**Document:** `docs/designs/DESIGN-sandbox-image-unification.md`
**Status:** Proposed

## Executive Summary

The design consolidates container image references from three locations (Go source, CI YAML, test scripts) into a single `container-images.json` file. The Security Considerations section correctly identifies the core trade-off (centralization doesn't change the trust model, just the location) and the Renovate auto-update risk. However, the section dismisses several concerns as "not applicable" that deserve deeper treatment, and it misses two attack vectors that the centralization introduces.

The most significant finding is the absence of image digest pinning. The design uses mutable tags (`alpine:3.21`), which means the actual image content pulled at CI time vs. sandbox time vs. next week can differ without any change to `container-images.json`. This is an existing problem, but the design presents itself as the "single source of truth" for images -- a next developer reading that claim will believe that identical JSON content means identical container behavior, which isn't true with tags.

## Question 1: Unconsidered Attack Vectors

### 1A. Single-file tampering amplification (NEW)

**Current state:** An attacker who compromises a single image reference in Go source only affects sandbox testing. An attacker who compromises a CI workflow only affects that workflow's distro. The scattered approach provides accidental isolation -- you'd need to tamper with multiple files to affect everything.

**After this design:** A single-character change to `container-images.json` redirects sandbox testing, all CI validation, and all test scripts simultaneously. The design acknowledges this in passing ("if tampered with, could redirect sandbox testing and CI to malicious container images") but then dismisses it with "this is the same risk that exists today." It isn't the same risk. Today, compromising the alpine image in Go source doesn't affect CI's alpine validation. After this change, it does. The blast radius of a single file edit increases from one consumer to all consumers.

**Mitigation quality:** The existing CODEOWNERS file (`.github/CODEOWNERS`) protects `.github/workflows/**` and `/scripts/**` but does not cover `container-images.json` at the repo root. After this change, modifying `container-images.json` is equivalent in impact to modifying a workflow file, but it won't trigger the same review requirements.

**Recommendation:** Add `container-images.json` to CODEOWNERS with the same protection level as workflow files (`@tsukumogami/core-team @tsukumogami/security-team`). This should be part of Phase 1, not Phase 3. **Blocking.**

### 1B. Build-time desync as a confused deputy (NEW)

The design uses a `go generate` or Makefile copy step to sync the repo-root JSON into `internal/containerimages/`. The design acknowledges this as a "stale copy" risk in the Consequences section but frames it purely as a correctness issue. It's also a security issue.

Scenario: An attacker submits a PR that modifies the repo-root `container-images.json` to point to a malicious image. The CI drift-check (Phase 3) passes because it only checks that all consumers read from `container-images.json` -- which they do. But what if the `go generate` step that copies the file into the Go package applies sanitization or transformation that strips the malicious payload? Or conversely, what if someone directly edits the embedded copy under `internal/containerimages/` without updating the root file?

The two-file design (root file + embedded copy) creates a window where the file seen by CI (`jq` reads the root) and the file seen by Go (`go:embed` reads the copy) can differ. The drift-check job proposed in Phase 3 only checks for hardcoded image strings outside `container-images.json` -- it doesn't verify that the embedded copy matches the root copy.

**Recommendation:** The Phase 3 CI drift-check should also verify `diff container-images.json internal/containerimages/container-images.json` and fail if they differ. **Advisory** (the Makefile handles this in the normal case, but a defense-in-depth check costs one line of shell).

### 1C. Tag mutability (EXISTING, but newly relevant)

Container tags are mutable. `alpine:3.21` today and `alpine:3.21` next month can be different images. The design uses tags exclusively. The current scattered approach has the same problem, but the design's claim of being a "single source of truth" creates a false sense of determinism. A developer reading "all consumers read from the same version of the file" will reasonably conclude that builds are reproducible. They aren't -- the same tag can resolve to different images on different days.

This matters for a package manager. If CI validated recipe X against `alpine:3.21` on Tuesday, and the sandbox pulls `alpine:3.21` on Friday (after a tag update), a user running `--sandbox` might get different results than CI did. The design can't claim parity between sandbox and CI without pinning to digests.

**Recommendation:** Document this limitation explicitly in the Security Considerations. If digest pinning is impractical now (Renovate handles tags more naturally than digests), note it as accepted residual risk. Don't claim "guaranteed to use the same images" without this caveat. **Advisory** for the design document; would be **Blocking** for the implementation if the `DefaultImage()` function's doc comment claims determinism.

## Question 2: Mitigation Sufficiency

### Supply Chain Risk Mitigations

The design's stated mitigation for Renovate-introduced compromised images is: "Renovate PRs go through the normal review process, and CI runs tests against the proposed images before merge."

This is partially effective but has a gap: **CI tests validate that recipes install successfully, not that the base image is trustworthy.** A compromised `alpine:3.21` image could include a backdoor that doesn't interfere with recipe installation. CI would pass. The reviewer would see a one-line version bump and approve. The compromised image would then be used in all sandbox testing and CI validation, potentially exfiltrating `GITHUB_TOKEN` values (which are passed into containers, as visible in `recipe-validation-core.yml` line 169).

Practical mitigation: This is a general supply chain problem with Docker Hub images and is not unique to this design. The design doesn't make it worse. But the Renovate auto-update mechanism does reduce the friction for this attack path compared to manual updates, where a human would at least intentionally choose the new version.

**Recommendation:** Consider requiring that Renovate PRs for `container-images.json` run an extended CI check (e.g., scanning the new image with `trivy` or `grype` before proceeding with recipe validation). This is Phase 3 scope and can be a follow-up. **Advisory.**

### CODEOWNERS Gap

As noted in 1A, `container-images.json` is not covered by CODEOWNERS. Given that the file now controls what code runs in CI containers that have access to `GITHUB_TOKEN`, it should be treated as security-sensitive infrastructure. **Blocking** -- this is a concrete gap that's easy to fix.

## Question 3: Residual Risk to Escalate

1. **Tag mutability** is accepted residual risk. It should be documented but doesn't need escalation -- it's pre-existing and the design doesn't make it materially worse.

2. **Renovate auto-bumping images that receive secrets** is accepted residual risk. The mitigation (review + CI) is standard practice. Worth noting for the team but not escalation-worthy.

3. **`ubuntu:24.04` hardcoded outside the config file** (`container_spec.go` line 134): When a PPA repository is detected, the code overrides the debian family image to `ubuntu:24.04`. After this design, this override would remain hardcoded in Go source rather than living in `container-images.json`. This means the "single source of truth" claim has an exception. A developer updating `container-images.json` won't know about this override. Not a security risk per se, but a maintenance trap that undermines the design's own goals. **Advisory** -- should be called out in the design as a known exception, or the JSON schema should support an `ubuntu` entry.

## Question 4: "Not Applicable" Justifications

### Download Verification -- "Not applicable"

**Verdict: Justified, but missing context.** The design correctly notes it doesn't change how images are pulled. However, the design also doesn't mention that images are pulled by tag without verification of content integrity. For a package manager's own infrastructure, this is a gap worth noting even if it's pre-existing. The `container_spec.go` file even has a comment about this: "Note: This uses the image tag, not digest, so it doesn't catch time-based staleness when the tag is updated. See issue #799 for proper version pinning solution." The Security Considerations should reference issue #799 and note that this design intentionally doesn't address content verification, deferring to that issue.

### Execution Isolation -- "Not applicable"

**Verdict: Justified.** The execution model genuinely doesn't change. Same container, same mounts, same limits. Nothing to flag.

### User Data Exposure -- "Not applicable"

**Verdict: Justified.** The JSON contains only public image names. No issue here.

## Question 5: Centralization and Attack Surface

The centralization changes the attack surface shape, not its size:

**Reduced surface:**
- Fewer files to audit for correct image references
- Drift between consumers is eliminated, removing the risk of testing against one image but running against another
- Single review point for all image changes

**Increased surface:**
- Single point of failure -- one compromised file affects everything
- The repo-root JSON file is more discoverable (an attacker scanning the repo knows exactly where to aim)
- Renovate's auto-update mechanism creates a regular flow of image-change PRs, which could be used as cover for a malicious bump (reviewer fatigue)

**Net assessment:** The centralization is a net positive for security. The reduced drift risk is a real, measurable improvement (the current state has images that are actually wrong, as documented in the drift table). The increased blast radius is theoretical and mitigable with CODEOWNERS. The reviewer fatigue risk from Renovate is real but acceptable given the alternative (manual updates that don't happen, leading to stale images with known CVEs).

## Summary of Recommendations

| # | Finding | Severity | Action |
|---|---------|----------|--------|
| 1 | `container-images.json` not in CODEOWNERS | Blocking | Add to CODEOWNERS with workflow-level protection in Phase 1 |
| 2 | Embedded copy vs. root copy can diverge without detection | Advisory | Add `diff` check to Phase 3 CI drift job |
| 3 | Tag mutability undermines "same images" claim | Advisory | Add caveat to Security Considerations; reference issue #799 |
| 4 | `ubuntu:24.04` PPA override hardcoded outside config | Advisory | Document as known exception or add `ubuntu` to JSON schema |
| 5 | Renovate image bumps should be scanned | Advisory | Consider `trivy`/`grype` scan step in Renovate PR CI |
| 6 | Download Verification "N/A" should reference #799 | Advisory | Add cross-reference to existing issue |

One blocking item: CODEOWNERS coverage for the new config file. Everything else is advisory-level documentation or defense-in-depth improvements.
