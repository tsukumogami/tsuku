# Architecture and Security Review: CI Build Essentials Consolidation

**Document reviewed:** `docs/designs/DESIGN-ci-build-essentials-consolidation.md`
**Reviewer role:** Architect Reviewer
**Date:** 2026-02-23

---

## Architecture Review

### 1. Is the architecture clear enough to implement?

**Yes, with minor clarifications needed.**

The design is a mechanical transformation of an existing pattern. The `run_test()` function at lines 189-230 of the design doc is copied nearly verbatim from the actual `test-macos-arm64` job at `/home/dangazineu/dev/workspace/tsuku/tsuku-3/public/tsuku/.github/workflows/build-essentials.yml:460-499`. An implementer can diff the two and produce the Linux variant with minimal ambiguity.

Two implementation details need clarification:

**Detail A: `timeout` wrapping is inconsistent between the design pseudocode and the macOS source pattern.** The design's `run_test()` (line 206) wraps the install command in `timeout 600` for recipe installs and `timeout 300` for non-recipe installs. But the actual macOS `run_test()` at `build-essentials.yml:474-485` does NOT use `timeout` at all -- it relies on the job-level `timeout-minutes: 90`. The existing `test-homebrew-linux` job at `build-essentials.yml:89-93` wraps individual tool tests in `timeout 300` via `timeout 300 bash -c '...'`. The design should note that adding per-test timeouts to `run_test()` is a deliberate improvement over the macOS pattern, not a copy of it. This isn't blocking -- it's a good enhancement -- but the "direct copy" framing in the rationale (lines 143-144) is misleading.

**Detail B: The tls-cacerts handling needs the download cache symlink.** The design's tls-cacerts block at lines 241-248 creates `$TSUKU_HOME` but does not create the `cache/downloads` symlink. The tls-cacerts test installs `ca-certificates` and `curl-source`, both of which download files. Without the cache symlink, these downloads won't benefit from the shared cache and (more importantly) will work fine -- this is purely an optimization miss. Advisory.

### 2. Are there missing components or interfaces?

**One functional gap: `TSUKU_REGISTRY_URL` env var handling.**

Every existing Linux job sets `TSUKU_REGISTRY_URL` as a step-level `env:` to point at the PR branch's raw content (see `build-essentials.yml:71-72`). The macOS jobs also set this at the step level (`build-essentials.yml:453-454`). The design's pseudocode for `run_test()` at lines 189-230 omits this entirely. The design's job skeleton at lines 93-113 also doesn't mention it.

In practice, the implementer will copy the macOS step's `env:` block and this will be handled. But since the design explicitly specifies the job structure (lines 93-113) and the `run_test()` function (lines 189-230), omitting `TSUKU_REGISTRY_URL` creates a gap where an implementer following the design literally would produce a job that resolves recipes from main instead of the PR branch. **Advisory** -- the existing macOS pattern makes the fix obvious, but the design should mention it.

**No structural components are missing.** The change is entirely within a single YAML file. No new packages, no new interfaces, no new dispatch paths.

### 3. Are the implementation phases correctly sequenced?

**Yes.** The three steps (add consolidated job, remove 7 individual jobs, validate) must be done atomically in a single commit to the workflow file. If the consolidated job is added without removing the old ones, CI would run duplicate tests; if the old ones are removed without adding the new one, coverage disappears. The design correctly frames this as "a single-file change" (line 264) and describes the steps as a sequence within one PR, not as separate PRs.

The upstream design (`DESIGN-ci-job-consolidation.md`) explicitly deferred this consolidation (its job topology table at lines 290-301 shows each Linux build essentials test "stays 1"). This design cleanly picks up where the upstream left off. No sequencing conflict.

### 4. Are there simpler alternatives we overlooked?

**No.** The design already evaluates the right alternatives:

- **2-job split by duration** (rejected at line 117): correct rejection -- the maintenance cost of balancing groups outweighs the wall-time savings.
- **Artifact-based binary sharing** (rejected at line 119): correct rejection -- addresses runner-minute waste but not queue pressure, which is the primary problem.
- **Doing nothing**: The upstream design already measured 7-11 minute queue waits. With 7 separate jobs, the waste is 7 x (1.5 min setup + variable queue wait). The consolidation is the obvious fix.

The design is about as simple as it can be: copy an existing pattern from macOS to Linux, adjust for Linux-specific needs (gettext, tls-cacerts script). There's no novel mechanism being introduced.

### Architectural Fit Assessment

**This change respects the existing architecture.** It extends the GHA group serialization pattern already established by the macOS jobs in the same file. It does not introduce a parallel pattern. After this change, both Linux and macOS tool tests follow the same structure: single job, `run_test()` function, `::group::` markers, failure array, shared download cache. The only differences are platform-specific: `CGO_ENABLED=0` for Linux builds, `gettext` for git-source, tls-cacerts as a special case.

No new workflow patterns are introduced. No existing patterns are bypassed.

---

## Security Review

### Context

This is a CI workflow refactoring. tsuku is a package manager that downloads and executes binaries. The security-relevant question is: does restructuring CI test orchestration change the attack surface or weaken any existing defenses?

### 1. Are there attack vectors we haven't considered?

**No novel attack vectors.** The consolidation does not change:
- What binaries are downloaded (same tools, same sources)
- How binaries are verified (same `verify-tool.sh`, same `verify-binary.sh`)
- What secrets are available (same `GITHUB_TOKEN` exposure)
- What code executes (same tsuku binary, same test scripts)
- Container isolation for sandbox tests (unchanged, those jobs stay separate)

One theoretical concern: **correlated failure masking.** If an attacker could compromise one download (e.g., substituting a malicious binary for one tool), in the parallel model only that tool's job would show unusual behavior. In the serial model, the attacker's binary runs on the same runner as subsequent tests. However:

- The existing macOS jobs already accept this exact risk (8 tests on one runner).
- Each test uses a fresh `$TSUKU_HOME`, so one tool's files don't contaminate another's installation.
- The binaries run inside `$TSUKU_HOME/tools/current/` -- they don't get system-wide persistence.
- CI runners are ephemeral -- they're destroyed after the workflow completes.

This is not a new risk introduced by the design. It's an inherent property of serialized testing that already exists in the macOS jobs. **Not actionable.**

### 2. Are the mitigations sufficient for the risks identified?

**Yes.** The design identifies three risk areas:

**Download verification (line 288):** Correctly marked as not applicable. The `tsuku install` command handles verification. The workflow change doesn't alter the verification path. Confirmed by reading `build-essentials.yml` -- the install commands are identical between current separate jobs and the proposed consolidated job.

**Execution isolation (lines 293-294):** Correctly analyzed. Per-test `$TSUKU_HOME` isolation is preserved. The shared download cache is write-then-read (each test writes its own downloads, subsequent tests may read cached files). The cache sharing is identical to the existing `test-homebrew-linux` pattern at `build-essentials.yml:76-86`.

**Supply chain risks (lines 296-298):** Correctly marked as not applicable. No new dependencies or sources. The `gettext` package via `apt-get` was already installed by `test-git-source-linux` -- it's moving from one job to another, not being added.

### 3. Is there residual risk we should escalate?

**No.** The residual risks are:
- Correlated failure masking (addressed above -- already accepted in macOS jobs)
- Hanging test blocking subsequent tests (mitigated by per-test timeouts)
- Network issues affecting multiple tests (inherent to serialization, same as macOS)

None of these are novel or escalation-worthy.

### 4. Are any "not applicable" justifications actually applicable?

Reviewed all three "not applicable" claims:

**Download Verification (line 288):** Genuinely not applicable. The workflow topology change doesn't touch the download or verification code paths. The same `tsuku install` commands run with the same flags.

**Supply Chain Risks (line 296):** Genuinely not applicable. No new Docker images, no new package sources, no new binaries. The `gettext` package source (Ubuntu's apt repository) is unchanged.

**User Data Exposure (line 299):** Genuinely not applicable. CI workflows operate on test fixtures and ephemeral installations. No user data is involved.

All three "not applicable" assessments are correct.

---

## Findings Summary

### Blocking

None.

### Advisory

| # | Location | Finding |
|---|----------|---------|
| 1 | Design doc lines 189-230 | `run_test()` pseudocode omits `TSUKU_REGISTRY_URL` env var. The implementer will likely copy from the macOS step's env block, but the design's explicit job skeleton and function definition create a gap if followed literally. Mention the env var. |
| 2 | Design doc lines 143-144 | Rationale claims per-test `timeout` is "a direct copy of the macOS implementation" but the macOS `run_test()` does not use per-test timeouts. The timeouts are an improvement -- frame them as such. |
| 3 | Design doc lines 241-248 | The tls-cacerts block creates `$TSUKU_HOME` but does not create the download cache symlink. Functional but misses the shared cache optimization. |

### Out of Scope

- The `run_test()` function is duplicated between macOS arm64 and macOS Intel (lines 460-499 and 551-589 of `build-essentials.yml`). The Linux consolidation will create a third copy. A future design could extract this into a shared script (`test/scripts/run-build-test.sh`). Not blocking because the duplication is contained within one file and the macOS jobs already live with it.
- The `test-homebrew-linux` job (line 87) sets `PATH` to include `$TSUKU_HOME/bin`, but `verify-tool.sh` line 22 already does this internally. The consolidated job should follow the macOS pattern (don't set PATH in `run_test()`) since the verify scripts handle it. This is a minor cleanup, not a structural issue.

---

## Recommendation

**Approve for implementation.** The design is a clean application of an existing pattern to an obvious gap. It reduces job count from 12 to 6 with no coverage change and no architectural deviation. The three advisory findings are documentation improvements that can be addressed during implementation.
