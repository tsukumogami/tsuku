# Security Review: DESIGN-curated-recipes.md

**Reviewer**: Security researcher  
**Date**: 2026-04-16  
**Document**: `docs/designs/DESIGN-curated-recipes.md`  
**Status**: Proposed

---

## Review Scope

The Security Considerations section of the design (lines 491-526) makes four claims
and concludes that "no design changes are required." This review examines whether those
claims hold and whether any residual risk is underweighted or missing.

---

## Finding 1: External Artifact Handling — Partially Adequate, One Gap Unaddressed

### What the design says

> "Handcrafted recipes should specify download URLs from official sources... Where the
> upstream provides checksums or signatures, the recipe should verify them using the
> `checksum` field."

### Assessment

The guidance is correct but advisory. The design defers checksum decisions to
implementers ("should"), whereas a stronger statement would tie curation to a minimum
verification level.

The code in `internal/recipe/types.go` already provides `GetChecksumVerification()`
which classifies each recipe into `ChecksumNone`, `ChecksumDynamic`, `ChecksumEcosystem`,
or `ChecksumStatic`. `npm_install` returns `ChecksumEcosystem` — meaning the npm
registry's own integrity mechanism (SHA-512 package integrity hashes and package-lock)
is treated as adequate. This is factually correct: npm packages carry `integrity` fields
in the lock file that npm verifies before extracting. So the design's claim about
`npm_install` is technically sound.

For `github_archive` recipes (kubectl, helm, bat, starship, neovim), the verification
classification is `ChecksumDynamic` when no static checksum is specified. This means
checksums are computed at plan-generation time from the upstream artifact, and baked
into the golden file — a real mitigation against post-release tampering, but not against
a compromise that happens before the plan is generated. The design points to
`terraform.toml` and `golang.toml` as patterns, but those use `ChecksumStatic` (explicit
SHA256 in TOML). **The design does not require curated recipes to use static checksums.**

**Gap**: The distinction between `ChecksumDynamic` and `ChecksumStatic` is meaningful
for supply chain integrity and is not discussed in the Security Considerations section.
For the highest-value curated recipes (those installed by millions of users), static
checksums with upstream signature verification (kubectl publishes SHA256 checksums,
helm publishes checksum files) would be appropriate. The section should acknowledge
this gap rather than implicitly treating dynamic checksums as equivalent.

**Severity**: Medium. Not a showstopper, but an unacknowledged residual risk.

---

## Finding 2: npm Scoped-Package Trust — Characterization Is Mostly Accurate, But Overstated

### What the design says

> "The `@` scope prefix means the package can only be published by the organization
> that controls that npm scope. This is a meaningful supply chain control."

### Assessment

The claim is broadly correct: npm scoped packages (`@anthropic-ai/`) require
authorization to publish within that scope, and the scope itself is owned by a
verified organization. An attacker cannot publish a package named
`@anthropic-ai/claude-code` without compromising Anthropic's npm account or npm itself.

However, there are three gaps the design does not mention:

**Gap 2a — Account compromise**: npm organization accounts can be compromised. Scoped
packages do not protect against a credential leak at the publisher level. The npm
security model does not include mandatory 2FA for all publish operations (though
Anthropic may enforce it internally). The design presents scope as a "meaningful
supply chain control" without noting that this control lives at the account layer, not
the cryptographic layer.

**Gap 2b — No signature verification at install time**: npm's `integrity` field in
`package-lock.json` verifies the package tarball hash, but does not verify that the
tarball was published by a specific key. npm Provenance (introduced 2023) adds
attestation linking a publish to a specific GitHub Actions run, but `npm install` does
not enforce provenance by default. The `npm_exec` action in `executePackageInstall()`
runs `npm ci` with `--ignore-scripts --no-audit --no-fund --prefer-offline`, which is
good hygiene but does not verify provenance.

**Gap 2c — Lockfile bootstrap trust**: The `Decompose()` path in `NpmInstallAction`
runs `npm install --package-lock-only` at eval time to capture a lockfile, which is
then embedded in the golden file. The golden file's lockfile pins the exact content
hashes of all transitive dependencies. When `executePackageInstall()` runs `npm ci`
at install time, it enforces those pinned hashes — this is a meaningful control. The
design does not explain this chain clearly. It mentions "the npm registry's own
integrity mechanism (package lock and signatures)" but doesn't distinguish between the
lockfile bootstrap step (which fetches live from npm) and the deterministic install step
(which enforces pinned hashes). A reader might incorrectly conclude that every install
fetches fresh, unverified content.

**Overall on npm**: The characterization is defensible but incomplete. The lockfile
hash-pinning in `npm_exec` is a stronger control than the design implies (it's not just
"consistent with industry practice" — it's deterministic reproducibility), and the
residual risk (account compromise, no provenance verification) is not acknowledged.

**Severity**: Low-Medium. The characterization is not wrong enough to block the design,
but the Security Considerations section overstates the protection provided by npm scope
while underexplaining the real protection (pinned hashes in `npm ci`).

---

## Finding 3: CI Permission Scope — Accurate But Incomplete

### What the design says

> "The issue-creation-on-failure step requires the `issues: write` GitHub Actions
> permission, which should be scoped to the nightly job only."

### Assessment

This is accurate. The recommendation to scope `issues: write` to only the nightly
job (not the entire workflow or workflow_call caller) is sound. The existing pattern in
the codebase confirms this approach: `checksum-drift.yaml` grants `issues: write` at
the top-level workflow scope alongside `contents: read`, which is the correct minimal
pattern for a scheduled workflow that creates issues.

**Gap 3a — PR-creation permission from recipe-validation-core.yml**: The design says
the nightly curated test extends the `recipe-validation-core.yml` workflow (or an
equivalent). That workflow already has a "Create pull request" step in its `report` job
(lines 502-537) that calls `gh pr create` after adding platform constraints. That step
uses `GH_TOKEN: ${{ secrets.GITHUB_TOKEN }}` without an explicit `pull-requests: write`
permission declaration in the workflow file — it relies on the default GitHub token
permissions for the repository. If the nightly curated workflow is added to
`recipe-validation-core.yml`, the PR-creation step is inherited too. The design does
not address this: it only mentions `issues: write` and does not consider that
`recipe-validation-core.yml` already has a PR-creation path that may require
`pull-requests: write` and `contents: write`. These would be automatically granted if
the repo's default permissions are permissive. A scoped workflow should declare all
required permissions explicitly.

**Gap 3b — Secrets propagation**: The design does not state whether the nightly curated
test needs `GITHUB_TOKEN` for recipe execution (to avoid GitHub API rate limiting when
downloading from GitHub releases for kubectl, helm, etc.). The existing
`recipe-validation-core.yml` passes `--env GITHUB_TOKEN="$GITHUB_TOKEN"` to the sandbox
for exactly this reason. The curated recipes (kubectl, helm) download from `dl.k8s.io`
and `get.helm.sh` — official CDN mirrors that do not require a GitHub token — so this
is not a direct gap for the Phase 3 recipes. But it should be noted.

**Severity**: Low. The `issues: write` observation is correct; the PR-creation
permission inheritance is an implementation gap rather than a design flaw.

---

## Finding 4: Discovery Entry Trust — Partially Accurate, One Claim Overstated

### What the design says

> "A malicious entry could redirect a tool name to the wrong package. The existing
> registry protects against this for distribution: entries in the central registry are
> code-reviewed on merge."

### Assessment

The characterization of code review as the primary protection is accurate for the
**central registry path** (files in `recipes/discovery/` checked into the repo). A
malicious discovery entry would require a PR that passes review, which is a meaningful
control for a public repository with required reviews.

However, two gaps are not addressed:

**Gap 4a — The fallback path in the data flow description has an inconsistency**:
The design's Data Flow section (lines 404-435) describes two paths. In the normal path
(cache hit), the handcrafted recipe at `recipes/c/claude.toml` is fetched and the
discovery entry is never consulted. In the fallback path (no local cache), the discovery
entry is used and then `runCreate → NpmBuilder.Build` is invoked. The design acknowledges
the `NpmBuilder` gap (it uses `req.Package` not `req.SourceArg`, so auto-generation from
a discovery entry alone would query the wrong package). This is deferred to a separate
PR.

The security implication is: **on cold installs where the central registry is not yet
cached, the fallback path runs `NpmBuilder.Build` with the wrong package name** (it
searches npm for a package named "claude" rather than "@anthropic-ai/claude-code"). The
design treats this as a correctness bug (install fails or returns wrong results), but it
also has a security dimension: if there is a package named "claude" on npm, the fallback
path would attempt to install it. The design correctly notes this produces an
`AmbiguousMatchError`, but if future npm search logic changes, it could silently install
the wrong package. The discovery entry is supposed to prevent this via `ConfidenceRegistry`
short-circuit, but the interaction between discovery-entry lookup and the NpmBuilder gap
is not fully traced in the Security Considerations section.

**Gap 4b — User-local recipe directory trust is mentioned but understated**: The design
says "User-local recipe dirs have always had higher trust." This is a known design
decision, but it means a local attacker who can write to `$TSUKU_HOME/registry/discovery/`
could inject discovery entries that redirect curated tools to malicious packages. This
is a threat model the design accepts without naming it. For the scope of this design
(adding curated recipes), this is reasonable — it's pre-existing, not introduced by the
design. But it should be acknowledged explicitly rather than dismissed in a subordinate
clause.

**Severity**: Low-Medium for Gap 4a (the NpmBuilder interaction has a security dimension
that the design omits), Low for Gap 4b.

---

## Finding 5: "Not Applicable" Justifications

The design has no explicit "not applicable" statements — instead, it implicitly dismisses
several threat categories by saying "no design changes are required." Let's examine the
tacit dismissals:

**Tacit dismissal 1 — TOML injection via curated flag**: The `curated = true` flag is
metadata-only, has no runtime effect, and is parsed into a `bool` field. No injection
risk. Correctly not applicable.

**Tacit dismissal 2 — Registry poisoning via PRs**: The design relies on code review.
For a public repository, this means a compromised maintainer account or a sophisticated
social engineering attack could merge a malicious recipe. This is a residual risk that
exists for all open-source package managers and is not specific to this design. The
design is correct not to address it, but a note about branch protection and required
reviews would strengthen the security posture statement.

**Tacit dismissal 3 — Nightly CI as an attack surface**: The design says the curated
array "adds more recipes to this existing execution context — no new permissions, network
access, or execution scope." This is accurate: the sandbox infrastructure already handles
arbitrary recipe execution. Adding curated recipes increases the number of recipes
executed but not the execution model. Correctly not applicable.

**Tacit dismissal 4 — Version pinning drift**: The design does not discuss what happens
when the nightly test installs `@anthropic-ai/claude-code@latest` and a malicious
version is published between golden file generation and nightly execution. The `npm ci`
path enforces the lockfile hashes — so this is mitigated by the lockfile pinning in
the golden file. However, when the nightly test runs without a pre-generated golden file
(i.e., a fresh `tsuku install claude` rather than `tsuku install --plan golden.json`),
it fetches the live version. Whether the nightly curated test uses golden files or live
installs is not specified in the design. This is an implementation question that affects
the security posture.

---

## Finding 6: Residual Risk Summary

The design's "no design changes are required" conclusion is defensible for what it covers,
but it omits several residual risks that should be stated even if not acted upon:

1. **Static vs. dynamic checksums for curated recipes**: The design recommends following
   `terraform.toml` and `golang.toml` patterns (static checksums) but does not require
   them. The gap between advisory guidance and enforcement is not named.

2. **npm account compromise**: The scoped package protection lives at the account layer.
   Compromise of Anthropic's npm credentials would bypass it. No mitigation is discussed.

3. **npm provenance not enforced**: `npm ci` verifies tarball hashes but not publication
   provenance. This is consistent with industry practice but should be stated, not implied.

4. **NpmBuilder fallback security dimension**: The cold-install fallback path (discovery
   entry → NpmBuilder) is described as a correctness issue but has a security dimension
   (potential to install wrong package). Deferred to a separate PR, but the security
   implication should be noted.

5. **Nightly test install mode**: The design does not specify whether nightly curated
   tests use golden-file plans (deterministic, hash-verified) or live installs
   (fetch latest, verify after). The security posture differs significantly.

---

## Verdict

The Security Considerations section **needs changes**. The section's conclusion ("no
design changes are required") is defensible for the design scope, but the section itself
has material inaccuracies and omissions:

- **Overstated**: npm scope is described as a "meaningful supply chain control" without
  explaining that the real protection is lockfile hash-pinning in `npm ci`, not scope
  ownership.
- **Understated**: The distinction between static and dynamic checksums for curated
  recipes is not discussed; the advisory language ("should") is not flagged as a gap.
- **Missing**: The NpmBuilder fallback path has a security dimension (wrong-package
  install risk on cold installs) that is not acknowledged in the security section.
- **Missing**: Nightly test install mode (golden file vs. live) is unspecified, and
  the security implication of each mode is not discussed.
- **Incomplete**: `issues: write` scoping advice is correct but the inherited
  `pull-requests: write` / `contents: write` from `recipe-validation-core.yml`'s
  PR-creation step is not addressed.

None of these findings block the design. The design is well-constructed and the
implementation (especially `npm_exec` with `--ignore-scripts`, lockfile pinning, and
`npm ci`) is more secure than the Security Considerations section communicates. The
section should be updated to accurately describe the actual protection model and
acknowledge the named residual risks.

### Recommended Changes to Security Considerations Section

1. Replace the npm scope paragraph with a two-part description: (a) scope ownership as
   a trust signal for provenance, (b) `npm ci` with pinned lockfile hashes as the actual
   integrity control, (c) residual risk of account compromise and absence of provenance
   attestation enforcement.

2. Add a sentence distinguishing static checksums (required by `terraform.toml` pattern)
   from dynamic checksums (computed at plan-generation time) and state explicitly which
   curated recipes will use each, and why.

3. Add a note on the NpmBuilder fallback path: on cold installs, the fallback through
   NpmBuilder uses `req.Package` rather than `req.SourceArg`, which means it queries npm
   for "claude" rather than "@anthropic-ai/claude-code". This is a correctness and
   security gap deferred to a separate NpmBuilder fix PR; until that PR lands, the
   handcrafted recipe at `recipes/c/claude.toml` must exist in the central registry to
   ensure the recipe path (not the fallback path) is used on all installs.

4. Clarify that the nightly curated tests should use golden-file plans (not live
   installs) to maintain hash-verified deterministic installs, and that new golden files
   must be regenerated when the version changes.

5. Expand the GitHub Actions permission discussion to include `pull-requests: write`
   and `contents: write` if the curated nightly workflow inherits the PR-creation step
   from `recipe-validation-core.yml`, and confirm these permissions are acceptable for
   a scheduled workflow.
