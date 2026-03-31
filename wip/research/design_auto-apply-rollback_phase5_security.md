# Security Review: Auto-apply with Rollback

## Scope

Review of DESIGN-auto-apply-rollback.md across five security dimensions, compared against the precedent set in DESIGN-background-update-checks.md (Feature 2).

---

## Dimension 1: External Artifact Handling

**Threat**: Auto-apply downloads and installs binaries based on cached version data. A poisoned cache entry (LatestWithinPin field) could direct the install flow to fetch an arbitrary version string.

**Severity**: Medium. The version string is passed to `runInstallWithTelemetry`, which resolves download URLs through the recipe and provider systems. The attack requires either (a) filesystem write access to `$TSUKU_HOME/cache/updates/` to inject a fake cache entry, or (b) a compromised upstream provider that returned a malicious version during the background check phase.

**Design doc coverage**: The "State mutation from cached data" paragraph addresses this and correctly notes that the install flow's validation (checksums for recipes that define them) bounds the risk. It also correctly states this is equivalent to manual `tsuku update`.

**Gap identified**: The design does not mention that auto-apply changes the *timing* model. Manual updates require user presence and intent; auto-apply acts without user awareness during PersistentPreRun. A poisoned cache entry gets consumed automatically on the next command, reducing the window for the user to notice something wrong (e.g., a compromised provider returning a bad version that gets retracted minutes later). This is a minor gap -- the Feature 2 design already flagged the "automated query cadence" aspect for checks, but Feature 3 extends this to automated *installation*, which deserves an explicit note.

**Suggested addition**: A sentence noting that auto-apply reduces the time-to-compromise window compared to manual updates, and that the opt-out (`auto_apply = false`) or pin constraints are the user's controls.

---

## Dimension 2: Permission Scope

**Threat**: Auto-apply modifies `$TSUKU_HOME/tools/` directories, `state.json` (including the new PreviousVersion field), symlinks in `$TSUKU_HOME/bin/`, cache entries, and notice files. All mutations happen under the user's own account.

**Severity**: Low. No privilege escalation. tsuku operates entirely within `$TSUKU_HOME`, which is user-owned. Auto-apply uses the same `runInstallWithTelemetry` code path as manual install -- no new filesystem locations are written.

**Design doc coverage**: Adequate. The same-user permission model is referenced in multiple threat paragraphs. The TryLock gate prevents concurrent state mutations.

**No gap**.

---

## Dimension 3: Supply Chain Trust

**Threat**: The auto-apply pipeline trusts version info that flowed through: upstream provider -> background check -> cache file -> auto-apply -> install. Each hop is a trust boundary.

**Severity**: Medium-High for recipes without checksums. Feature 2's security section already flagged this clearly: "Recipes with dynamic URL templates and no checksums have zero integrity protection in the full check-then-apply pipeline." Auto-apply completes this pipeline, making the Feature 2 warning fully realized.

**Design doc coverage**: The auto-apply design's "State mutation from cached data" paragraph says "checksums for recipes that define them" but does not re-emphasize the no-checksum gap that Feature 2 called out. This is the most significant coverage gap in the security section.

**Suggested addition**: Explicitly restate that for recipes without per-version checksums, the full automated pipeline (check -> cache -> apply) has no integrity verification beyond HTTPS transport security. Reference Feature 2's more detailed treatment. This doesn't require a new mitigation (it's a known limitation of the recipe model), but the security section should not leave readers thinking checksums are universal.

---

## Dimension 4: Data Exposure

**Threat**: Notice files at `$TSUKU_HOME/notices/<toolname>.json` contain tool names, attempted version strings, error messages, and timestamps. These are readable by any process running as the user.

**Severity**: Low. The data is informational and already implied by the directory structure of `$TSUKU_HOME/tools/`. Error messages could theoretically leak internal paths or provider URLs, but this is bounded by the same-user permission model.

**Design doc coverage**: The "Notice files are informational only" paragraph covers this adequately and correctly notes they contain no executable content.

**Minor observation**: Error messages serialized into notice files could contain upstream HTTP response bodies if the install flow propagates them. If an upstream provider returns a verbose error (e.g., including authentication tokens in error responses), that would be persisted to disk. This is a general Go HTTP client concern, not specific to this design. No action needed in this design doc.

**No gap**.

---

## Dimension 5: Process Lifecycle

**Threat**: The TryLock gate, auto-rollback on failure, and notice writing form a multi-step lifecycle with several failure modes.

**Severity**: Low-Medium.

Sub-analysis:

**5a. TryLock DoS**: An attacker holding `state.json.lock` prevents auto-apply. The design correctly notes this is bounded by same-user permissions and matches Feature 2's flock DoS treatment. Adequate.

**5b. Auto-rollback reliability**: On install failure, `Activate(tool, previousVersion)` is called. If Activate itself fails (e.g., broken symlink, permission issue), both the new and old versions could be in a degraded state. The "Resilience to corruption" section covers "PreviousVersion pointing to a deleted directory" but not "Activate() itself failing during rollback."

**Suggested addition**: Note what happens if the rollback Activate() call fails. The likely answer is: the tool remains on whatever version was partially installed (or no working version), and a notice is written. This should be stated explicitly since it's the worst-case failure mode of the safety feature.

**5c. Interrupted auto-apply**: The design's "Resilience to corruption" section states "The install flow uses atomic staging directories. An interrupted install leaves either the old version active or a complete new version -- never a partial state." This is good coverage.

**5d. Lock scope coupling**: The design explicitly identifies the locking coupling between MaybeAutoApply's TryLock and runInstallWithTelemetry's internal locking. This is a correctness issue rather than a security issue, but deadlock would manifest as a hung command. The design flags it for implementation-phase resolution. Adequate.

---

## Overall Assessment of Security Considerations Section

The existing security section covers four threats with appropriate severity calibration. It correctly applies the same-user permission model as a bounding assumption and avoids overstating risks.

**Gaps to address** (in priority order):

1. **Supply chain trust for no-checksum recipes**: The most significant omission. Feature 2 explicitly warned about zero integrity protection for recipes without checksums. Auto-apply completes that attack path. The auto-apply design should reference this.

2. **Rollback failure mode**: What happens when Activate() fails during auto-rollback? The design should state the degraded-state behavior explicitly, since auto-rollback is the primary safety mechanism.

3. **Reduced user-awareness window**: Minor. Auto-apply acts without user intent, reducing the time between a compromised version appearing upstream and being installed locally.

None of these gaps change the fundamental security posture (same-user model bounds everything), but they would make the security section more complete and consistent with Feature 2's level of detail.

---

## Recommendation

**OPTION 1: Accept with minor revisions.**

The design is sound. The security model is correctly bounded by same-user permissions, and the auto-rollback mechanism provides genuine safety. The three gaps identified above are documentation completeness issues, not architectural flaws. Adding 3-4 sentences to the Security Considerations section would bring it to parity with Feature 2's treatment.
