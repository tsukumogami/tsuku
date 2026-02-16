# Security Review: DESIGN-automated-seeding.md

**Reviewer**: Security analysis agent
**Date**: 2026-02-16
**Design**: Automated Seeding Workflow
**Scope**: Supply chain risk, attack surface, disambiguation gaming, audit log exposure, ecosystem API trust

---

## Executive Summary

The automated seeding design introduces a new automated pathway from external ecosystem APIs into tsuku's package queue. While the design correctly identifies the core supply chain risk (ecosystem API trust) and applies meaningful mitigations (10x threshold, secondary signals, priority-based alerting), there are several gaps that could be exploited by a motivated attacker. The most critical finding is that the priority-based alerting split (manual review for tiers 1-2 only) creates a predictable window where an attacker can manipulate tier-3 packages without human oversight, then wait for those packages to gain adoption and be promoted. Additional findings relate to the PyPI probe's missing download data creating a disambiguation blind spot, the third-party PyPI data source being unverified, and the workflow's direct-to-main push pattern bypassing branch protection.

**Critical findings: 3**
**High findings: 3**
**Medium findings: 4**
**Low findings: 2**

---

## 1. Unconsidered Attack Vectors

### CRITICAL: Tier-3 Promotion Pathway Attack

The design auto-accepts source changes for all 5,230 tier-3 packages. An attacker who identifies a tier-3 package that's gaining popularity (trending on social media, mentioned in conference talks) can:

1. Register the package name on a different ecosystem (e.g., register "zoxide" on npm if it currently routes to cargo)
2. Inflate downloads on the new ecosystem to cross the 10x threshold
3. Wait for the 30-day freshness cycle to trigger re-disambiguation
4. The source change is auto-accepted because the package is tier-3
5. The package eventually gets promoted to tier-2 (based on the `tier2Threshold` of 40K installs in `homebrew.go:16`)

The attack window is widened because tier assignments in the existing codebase (`homebrew.go:128-136`) are determined at discovery time, not recalculated during re-disambiguation. A package could gain significant real-world adoption while remaining tier-3 in the queue.

**Recommendation**: Track a "velocity" metric alongside static tier. Packages whose download counts are increasing rapidly should receive manual review on source changes regardless of current tier. Alternatively, recalculate tier during re-disambiguation based on current download data.

### HIGH: Name Squatting Across Ecosystems

The design adds four new ecosystems as discovery sources. Each ecosystem has independent namespace governance. An attacker can register a popular tool name on a less-popular ecosystem before tsuku discovers it. When seeding runs:

1. The CratesIOSource discovers "popular-tool" on crates.io
2. Disambiguation probes all 8 ecosystems
3. The attacker's npm package "popular-tool" has inflated downloads
4. If the npm downloads exceed 10x the crates.io downloads, the attacker's source wins

The existing name match filter (`ecosystem_probe.go:122`) uses case-insensitive exact match, which is good. But it doesn't validate that the package on different ecosystems refers to the same upstream project (e.g., same GitHub repository).

**Recommendation**: Add a cross-ecosystem repository validation check. When disambiguation finds matches on multiple ecosystems, verify that at least the top candidates share a repository URL. Flag mismatches as high-risk regardless of download ratios.

### HIGH: PyPI Static Dump Poisoning

The design uses `https://hugovk.github.io/top-pypi-packages/top-pypi-packages-30-days.min.json` as the PyPI data source. This is a third-party GitHub Pages site maintained by a single individual. An attacker who compromises this GitHub account can inject arbitrary package names into the "top packages" list, causing tsuku to discover and queue malicious packages.

Unlike the official ecosystem APIs (crates.io, npm registry, RubyGems API), this data source has no organizational security backing, no SLA, and its compromise could go undetected.

**Recommendation**: Either validate PyPI candidates against the official PyPI API before adding to queue (already partially done via classifier check), or use PyPI's BigQuery dataset (which is official Google infrastructure). At minimum, pin the data source to a specific commit hash rather than fetching from the default branch.

### MEDIUM: Workflow-Triggered Denial of Service via Issue Flooding

The `seed-queue.yml` workflow creates GitHub issues for priority 1-2 source changes. An attacker who can cause disambiguation to oscillate (e.g., by alternating download counts between two ecosystems across weekly cycles) could generate repeated issues for the same package, creating noise that masks legitimate alerts.

The workflow's issue creation step has no deduplication. It parses stdout JSON and creates issues unconditionally for `auto_accepted == false` entries.

**Recommendation**: Before creating an issue, check if an open issue with the same title already exists (using `gh issue list --search`). Add a deduplication label and skip creation if a matching open issue exists.

---

## 2. Mitigation Sufficiency Assessment

### 10x Threshold + Secondary Signals

The `isClearWinner` function (`disambiguate.go:41-57`) requires three conditions:
1. 10x download ratio
2. Version count >= 3
3. Has repository link

This is a reasonable defense-in-depth approach. However, the secondary signals are easy to game:

- **Version count >= 3**: An attacker can publish 3 trivial patch versions in hours. This threshold is too low. Most legitimate CLI tools have 10+ versions.
- **Has repository**: Creating a GitHub repository is free and instant. This signal only proves the attacker bothered to create a repo, not that the package is legitimate.

The 10x threshold itself is sound for organic detection but vulnerable to download inflation attacks. On npm, download counts can be inflated by automated CI pipelines that install the package. On PyPI, download counts include mirror traffic. On crates.io, download counts include all `cargo build` invocations in CI.

**Assessment**: Individually, each signal is gameable. Together, they raise the bar meaningfully but not prohibitively for a determined attacker. The design's own residual risk section acknowledges this but relies on "batch generation validates recipes on 11 platform environments" as the final backstop. This is correct -- the queue only determines *which* source to try, not what gets installed. The actual installation goes through recipe generation and CI validation.

**Recommendation**: Raise the version count threshold to 5 or higher for automated batch decisions. Consider adding a "time since first version" signal (reject packages where the first version was published within the last 30 days).

### Priority-Based Alerting

The split between manual review (tier 1-2) and auto-accept (tier 3) is pragmatic but creates a clear boundary that attackers can target. The tier 1 list in `homebrew.go:20-33` contains 27 hardcoded package names. An attacker can see exactly which packages require manual review by reading the public source code.

**Assessment**: The tier-1 list is appropriate for the highest-value targets, and the 10x threshold provides baseline protection for everything else. The gap is in the tier-2 threshold (`tier2Threshold = 40000` in `homebrew.go:16`), which is only applied during Homebrew seeding. Packages discovered via cargo/npm/pypi/rubygems don't go through the `assignTier` function at all -- the design doesn't specify how tiers are assigned for non-homebrew sources.

**Recommendation**: Define tier assignment logic for all ecosystem sources, not just Homebrew. The current design only specifies tier assignment for `HomebrewSource`. New sources should assign tiers based on their own download thresholds.

### Curated Entry Protection

The design correctly states that curated entries are never re-disambiguated. This is verified in the code -- the `Merge` function (`queue.go:63-77`) skips existing entries by ID. However, the `confidence` field doesn't exist in the `seed.Package` struct (`queue.go:11-20`). The design references `confidence: "curated"` and `confidence: "review_pending"` but there's no field for this in the current schema.

**Assessment**: This is a gap between the design and the implementation. The design assumes a `confidence` field that doesn't exist in the seed package's `Package` struct. Without it, there's no mechanism to distinguish curated from auto-disambiguated entries, and the "don't re-disambiguate curated entries" logic can't be implemented.

**Recommendation**: Add a `Confidence` field to `seed.Package` before implementing this design. The freshness check logic depends on it.

---

## 3. Residual Risk Escalation

### CRITICAL: Direct Push to Main Branch

The proposed workflow (`seed-queue.yml` in the design, lines 462-524) commits and pushes directly to main:

```yaml
git commit -m "chore(batch): update queue from seeding run"
git push
```

The existing workflow (`.github/workflows/seed-queue.yml`) also pushes directly to main with retry logic. This bypasses branch protection rules and code review. An attacker who can manipulate the seeding output (via ecosystem API compromise, response injection, or DNS hijacking of ecosystem endpoints) gets their changes committed directly to main with no human review.

This is particularly concerning because the seeding workflow runs with `contents: write` permission and a GitHub App token (`TSUKU_BATCH_GENERATOR_APP_ID`), which likely has elevated privileges.

**Assessment**: This is the highest-risk residual item. The workflow has write access to the repository and pushes to main without PR review. While the queue is "just JSON data," it determines which packages get processed by the batch pipeline, which then generates recipes that install binaries on user machines.

**Recommendation**: Change the workflow to create a PR instead of pushing directly. For the weekly cadence, a PR that auto-merges after CI passes would add review opportunity without human bottleneck. At minimum, require that the commit is signed and the diff is validated against a schema.

### HIGH: Ecosystem API Response Injection (MITM)

The design mentions HTTPS and TLS validation, but the `hugovk.github.io` PyPI source is a GitHub Pages site served over HTTPS with a GitHub-managed certificate. If an attacker can perform a MITM attack (e.g., compromised CI runner network), they can inject arbitrary data into any of the ecosystem API responses.

The existing `maxResponseBytes` limit (10MB in `homebrew.go:13`) prevents memory exhaustion but doesn't validate response integrity. There's no certificate pinning, no response signature verification, and no consistency checks against known-good data.

**Assessment**: MITM against ecosystem APIs in a GitHub Actions environment is low probability (GitHub controls the runner network) but high impact. The more realistic attack vector is DNS poisoning or BGP hijacking targeting ecosystem API domains.

**Recommendation**: For critical signals like download counts, implement a sanity check: compare the fetched download count against a historical baseline. If a package's downloads change by more than 100x between seeding runs, flag it for review regardless of tier.

---

## 4. "Not Applicable" Justification Review

### Download Verification: "Not applicable for seeding itself"

The design states download verification is not applicable because "the seeding command queries ecosystem APIs for metadata (package names, download counts, version counts) but doesn't download any binaries."

**Assessment**: This justification is **partially valid but misleadingly scoped**. While it's true the seeding command doesn't download binaries, the seeding command determines *which* binaries will eventually be downloaded. A corrupted source assignment (`cargo:malicious-fork` instead of `cargo:legitimate-tool`) persists through the queue and into batch generation. The design should acknowledge this transitive risk rather than dismissing verification as "not applicable."

The design does mention that "batch generation validates recipes on 11 platform environments" but doesn't explain what that validation checks. If the validation only checks that the binary runs successfully (functional testing), it won't catch a supply chain compromise where a backdoored binary still passes functional tests.

**Recommendation**: Reframe this section. Instead of "not applicable," state: "Binary verification is deferred to the batch generation phase, which validates checksums and tests on 11 platform environments. The seeding command's risk is in source routing, not binary integrity, and is mitigated by the disambiguation threshold and source change alerting."

### Execution Isolation: "No new execution surface"

**Assessment**: This is **correctly justified**. The seeding command reads APIs and writes JSON. The GitHub Actions runner is ephemeral. No user-supplied code is executed.

### User Data Exposure: "No user data"

**Assessment**: This is **correctly justified** for the seeding command itself. The one nuance is the User-Agent header, which the design acknowledges. The audit logs contain no user data.

---

## 5. Source Change Alerting Analysis

### Adequacy Against Supply Chain Attacks

The source change alerting has three layers:
1. Curated entries are never auto-updated (strong)
2. Priority 1-2 changes create GitHub issues (strong for known targets)
3. Priority 3 changes are auto-accepted (weak)

**Gap 1: No alerting on initial source assignment.** When a newly discovered package gets its first disambiguation, there's no alert regardless of tier. An attacker who registers a malicious package with a popular name on a less-monitored ecosystem gets it queued without any human review. The first disambiguation is trusted implicitly.

**Gap 2: No aggregate anomaly detection.** If a seeding run suddenly finds 50 new packages from npm that all resolve to the same GitHub organization, there's no alert. Batch registration of packages pointing to a single entity is a common supply chain attack pattern.

**Gap 3: Source changes that don't cross ecosystem boundaries.** If a package moves from `github:original-author/tool` to `github:fork-author/tool` (same ecosystem, different owner), the current alerting doesn't trigger because the ecosystem prefix hasn't changed. This is a significant gap because project takeover through GitHub transfer/fork is a real attack vector.

**Recommendation**:
- Add alerting for first disambiguation of packages with download counts below a threshold (potential new/suspicious packages).
- Add aggregate alerting: if more than N packages in a single seeding run resolve to sources owned by the same entity, flag the entire batch.
- Track the full source identifier (including owner/org), not just the ecosystem prefix, when detecting source changes.

---

## 6. 10x Threshold + Secondary Signals in Batch Context

### Is This Sufficient for Automated Disambiguation?

In interactive mode, a human reviews ambiguous cases. In batch mode with `forceDeterministic: true`, the system falls back to priority ranking when there's no clear winner. The question is whether this combination is safe at scale.

**Analysis of the priority fallback path:**

When `isClearWinner` returns false (no 10x gap), `disambiguate()` in deterministic mode selects the first result by priority ranking (`disambiguate.go:123-131`). The priority ranking in `ecosystem_probe.go:53-61` is:

```
cask: 1, homebrew: 2, crates.io: 3, pypi: 4, npm: 5, rubygems: 6, go: 7, cpan: 8
```

This means that for close calls, Homebrew/Cask always wins over other ecosystems. This is a reasonable default for macOS tools but may be wrong for Linux-first CLI tools. More importantly, this fallback is applied whenever download data is missing from one or both candidates.

**The PyPI blind spot is critical here.** The PyPI prober (`pypi.go:401-405`) returns `Downloads: 0` because PyPI's API doesn't expose download counts. This means any package that exists on PyPI *and* another ecosystem will never have PyPI win via the 10x threshold. PyPI can only win via priority fallback if it's ranked higher than the other ecosystem -- but PyPI is ranked 4th, below cask, homebrew, and crates.io.

For packages discovered by the PyPISource, disambiguation will probe all ecosystems. If the package also exists on crates.io with even 100 downloads, crates.io will win because it has downloads > 0 while PyPI has 0. This may be incorrect for packages like `httpie` or `black` that are genuinely Python tools.

**Recommendation**: For the PyPISource specifically, consider fetching download counts from PyPI Stats (`pypistats.org/api/`) or BigQuery during disambiguation. Alternatively, when a package is discovered via PyPI and the probe returns 0 downloads, use the discovery source's download data (from the static dump) as a fallback signal rather than relying solely on the probe.

### Batch-Specific Risks

Running 260+ disambiguations per week in batch mode introduces timing-based risks:

- **Ecosystem API lag**: If crates.io's download counts are cached and npm's are real-time, a package could have stale cargo downloads and fresh npm downloads, skewing the comparison.
- **Probe timeout inconsistency**: The EcosystemProbe uses a shared timeout (`ecosystem_probe.go:85`). If one prober is slow (e.g., crates.io rate-limited), its results may be dropped, making the remaining probers' results look like a "single match" that gets auto-selected without threshold checks.

**Recommendation**: Log which probers timed out or errored for each disambiguation. If a high-priority ecosystem prober (crates.io, npm) fails, mark the disambiguation as provisional and re-run it in the next cycle rather than treating the remaining results as definitive.

---

## 7. Ecosystem API Trust Assumptions

### Are They Reasonable?

The design assumes ecosystem APIs return accurate metadata. Let's evaluate each:

**crates.io**: Run by the Rust Foundation. API is well-documented and stable. Download counts are server-side (can't be easily inflated by CI). Category system is curated. **Trust level: High.**

**npm**: Run by GitHub/Microsoft. Download counts include all `npm install` invocations, including CI. The npm registry has a history of typosquatting and malicious packages. The search API's `popularity` weight is a black box. **Trust level: Medium.** Download counts are inflatable via CI.

**PyPI (static dump)**: Third-party GitHub Pages site. No SLA, no organizational backing. The underlying data (PyPI download counts) comes from BigQuery, which is reliable, but the intermediary is not. **Trust level: Low for the data source, Medium for the underlying data.**

**PyPI (official API)**: Run by the Python Software Foundation. No download counts in the API. Package metadata is reliable. **Trust level: High for metadata, N/A for downloads.**

**RubyGems**: Run by Ruby Together. API is documented. Download counts are server-side. The `executables` field for CLI filtering is self-reported by gem authors. **Trust level: Medium.** The `executables` field can be set arbitrarily.

**Homebrew (existing)**: Run by Homebrew maintainers. Analytics are aggregate and opt-in. Formula existence implies some curation. **Trust level: High.**

### Cross-Ecosystem Download Count Incomparability

A fundamental assumption in the disambiguation algorithm is that download counts across ecosystems are comparable. They aren't:

| Ecosystem | What "downloads" means | Inflatable? |
|-----------|----------------------|-------------|
| crates.io | `cargo build` invocations (recent) | Moderate (CI builds) |
| npm | `npm install` invocations (weekly) | High (CI, scripts) |
| Homebrew | `brew install` invocations (365-day) | Low (requires macOS) |
| RubyGems | Total gem downloads (lifetime) | Moderate (CI) |
| PyPI | N/A (not available from API) | N/A |

The 10x threshold implicitly assumes that a 10x difference in "downloads" across ecosystems represents a genuine popularity gap. But npm weekly downloads of 100K vs. crates.io recent downloads of 10K doesn't mean the npm package is 10x more popular -- npm's denominator is different.

**Recommendation**: Add per-ecosystem download normalization. Either convert all download counts to a common unit (e.g., estimated monthly unique users, normalized by ecosystem size) or apply ecosystem-specific multipliers to make the comparison fairer. At minimum, document this limitation as a known inaccuracy.

---

## 8. Audit Log Risks

### Data Manipulation

Audit files are written to `data/disambiguations/audit/<name>.json` and committed to the repository. Since the workflow pushes directly to main, audit log integrity depends on the workflow not being compromised. An attacker who can modify the workflow (via PR to `.github/workflows/`) could alter audit logs to hide evidence of manipulation.

**Mitigation already present**: GitHub's branch protection (if enabled) would require review for workflow changes. The `CODEOWNERS` file (if present) could require specific reviewers for workflow modifications.

**Gap**: The design doesn't mention any integrity verification for audit logs. Since they're plain JSON files in git, any commit can modify them. Historical audit data can be rewritten in a force push.

**Recommendation**: Consider adding a running hash/checksum across audit entries (each new entry includes a hash of the previous entry). This creates a tamper-evident chain without requiring external infrastructure. Alternatively, since git itself provides commit hashing, ensure branch protection prevents force pushes to main.

### Reconnaissance Value

The audit logs contain:
- Full probe results with download counts per ecosystem
- Selection rationale (which signals were decisive)
- Whether the disambiguation was "high risk"
- Previous source assignments

This data is valuable for an attacker planning a supply chain attack:

1. **Download thresholds are visible**: An attacker can read audit logs to see exactly how many downloads they need to cross the 10x threshold for a specific target package.
2. **High-risk flags reveal weak spots**: Packages marked `high_risk: true` (priority fallback) are the easiest targets because their disambiguation was uncertain.
3. **Probe failure patterns**: If audit logs show that certain ecosystem probers consistently timeout, an attacker knows which ecosystem data to manipulate (the one that responds faster will dominate).

**Assessment**: Since tsuku is an open-source project, the disambiguation algorithm is already public. The audit logs add specific per-package data (current download counts, ratios) but don't reveal algorithmic secrets. The risk is moderate -- the data makes targeted attacks *easier* but not *possible* where they weren't before.

**Recommendation**: Accept this as residual risk. The transparency benefits (debugging, community auditing) outweigh the reconnaissance risk. However, consider not publishing exact download counts in audit logs -- instead publish download "bands" (e.g., "10K-100K") which are sufficient for debugging but less useful for calculating exact attack thresholds.

---

## Finding Summary Table

| # | Severity | Finding | Recommendation |
|---|----------|---------|----------------|
| 1 | CRITICAL | Tier-3 promotion pathway: attacker manipulates tier-3 package before it gains adoption | Track download velocity; recalculate tier during re-disambiguation |
| 2 | CRITICAL | Direct push to main bypasses code review for queue changes | Switch to PR-based workflow with auto-merge |
| 3 | CRITICAL | `confidence` field missing from `seed.Package` struct; curated protection can't be implemented | Add `Confidence` field before implementation |
| 4 | HIGH | Cross-ecosystem name squatting with no repository validation | Add cross-ecosystem repository URL comparison |
| 5 | HIGH | PyPI static dump from third-party GitHub Pages site | Pin to commit hash or use official BigQuery source |
| 6 | HIGH | PyPI probe returns Downloads=0, creating systematic bias against PyPI sources | Fetch download counts from pypistats.org or use discovery data as fallback |
| 7 | MEDIUM | No alerting on initial source assignment for new packages | Alert when newly discovered packages have suspiciously low download counts |
| 8 | MEDIUM | No aggregate anomaly detection for batch source assignments | Flag batches where many packages resolve to same entity |
| 9 | MEDIUM | Source change detection doesn't track owner changes within same ecosystem | Compare full source identifiers including owner/org |
| 10 | MEDIUM | Download counts are incomparable across ecosystems | Document limitation; consider normalization |
| 11 | LOW | Issue flooding via disambiguation oscillation | Add deduplication check before creating GitHub issues |
| 12 | LOW | Audit logs provide reconnaissance data for targeted attacks | Consider publishing download bands instead of exact counts |

---

## Conclusion

The design demonstrates solid security awareness by identifying the core supply chain risk and applying the 10x threshold with secondary signals. The priority-based alerting for tier 1-2 packages is appropriate. However, three items should be addressed before implementation:

1. **The `confidence` field gap** (finding #3) is a blocking implementation issue that will prevent curated entry protection from working.

2. **The direct-push-to-main pattern** (finding #2) is the most architecturally significant risk. Converting to a PR-based workflow with auto-merge adds minimal latency but provides a review checkpoint and audit trail that's separate from the audit logs.

3. **The PyPI download blind spot** (finding #6) will systematically misroute Python-native tools. Since PyPISource is one of the four new sources being added, this should be fixed in the same design rather than deferred.

The remaining findings are improvements that can be implemented incrementally as the seeding workflow matures. None of them individually break the security model, but together they represent a meaningful attack surface expansion that should be tracked.
