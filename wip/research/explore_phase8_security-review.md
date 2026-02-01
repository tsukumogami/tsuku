# Security Review: Discovery Registry Bootstrap

## Executive Summary

This review examines the security analysis in DESIGN-discovery-registry-bootstrap.md for a system that maps tool names to GitHub repositories. The design has solid foundational security but contains **critical gaps** in threat modeling, particularly around registry poisoning and the human review process.

**Key Findings:**
- **Critical Gap**: No systematic verification that seed list entries point to the "canonical" repository for a tool
- **Critical Gap**: PR review process not hardened against social engineering or typosquatting
- **Critical Gap**: No plan for detecting compromised-then-legitimate repos (reverse of the transfer scenario)
- **Moderate Gap**: Disambiguation override mechanism needs stronger safeguards
- **Moderate Gap**: Missing monitoring/alerting for suspicious patterns

**Overall Risk Assessment**: MEDIUM-HIGH residual risk that requires additional mitigations before production deployment.

---

## 1. Attack Vectors Not Considered

### 1.1 Initial Seed List Poisoning (CRITICAL)

**Attack Vector**: A malicious contributor submits a PR with a plausible-looking entry that points to a legitimate-appearing but actually malicious repository.

**Example Scenario:**
```json
{"name": "kubectl", "repo": "kubernetes-tools/kubernetes", "binary": "kubectl"}
```

The real kubectl is at `kubernetes/kubernetes`, but `kubernetes-tools/kubernetes` looks legitimate, has release artifacts, passes automated validation, and could bypass human review if the reviewer doesn't independently verify the canonical source.

**Current Mitigations**: "Standard code review process, automated validation"

**Why Insufficient:**
- The automated validation only checks that the repo EXISTS and has releases, not that it's the CORRECT repo
- Human reviewers would need to independently research each tool's canonical source
- No canonical source database to validate against
- The design assumes reviewers will catch "subtle repo name changes" but provides no checklist or verification process

**Attack Surface**: ~500 entries at bootstrap, each requiring independent verification of canonical source.

### 1.2 Typosquatting in Discovery Registry

**Attack Vector**: Add entries for commonly mistyped tool names pointing to malicious repos.

**Example:**
```json
{"name": "kubeclt", "repo": "malicious-org/kubeclt"},  // typo: kubeclt vs kubectl
{"name": "terrafrm", "repo": "malicious-org/terrafrm"}  // typo: terrafrm vs terraform
```

**Current Mitigations**: None explicitly mentioned.

**Why Concerning:**
- Users who mistype `tsuku install kubeclt` would get a malicious binary
- Standard typosquatting protection mechanisms (registries reject similar names) don't apply to a curated list
- The design doesn't discuss policies for rejecting entries with names "too similar" to existing entries

### 1.3 Compromised-Then-Legitimized Repository

**Attack Vector**: The inverse of the "transferred repo" scenario. An attacker:
1. Creates a malicious repo with release artifacts
2. Gets it added to the discovery registry via PR (passes validation: repo exists, has releases)
3. After merge, adds legitimate-looking documentation, stars, activity to avoid detection
4. Waits for users to install the malicious tool

**Current Mitigations**: The freshness check verifies repo still exists and has releases, but doesn't re-evaluate legitimacy.

**Why Insufficient:**
- The weekly freshness check validates technical properties (exists, not archived), not trust properties
- No re-evaluation of whether a repo is the canonical/legitimate source for a tool
- A repo that was malicious at T0 and still malicious at T7 passes all checks

### 1.4 Disambiguation Override Abuse

**Attack Vector**: Submit a disambiguation override that reverses the expected resolution to point users at a malicious ecosystem package.

**Example:**
```json
// In disambiguations.json
{"name": "bat", "builder": "npm", "source": "malicious-user-bat"}
```

The legitimate bat is `sharkdp/bat` on GitHub. A malicious npm package named `bat` could be created, and a disambiguation override could redirect all `tsuku install bat` requests to it.

**Current Mitigations**: "Automated collision detection + human review"

**Why Insufficient:**
- Disambiguation entries have a `disambiguation: true` flag for "extra review attention," but no checklist for what "extra attention" means
- No verification that the chosen builder/source is the one with higher popularity/trust
- The design says "fully automated resolution by popularity" was rejected, but provides no alternative mechanism for verifying the correct choice

### 1.5 Seed List Injection via Supply Chain

**Attack Vector**: Compromise the development environment or CI pipeline to inject malicious entries into seed lists before they're committed.

**Example:**
- Attacker gains access to a contributor's machine
- Modifies `data/discovery-seeds/dev-tools.json` to add malicious entries
- Contributor commits the file without reviewing every line (500 entries is a lot)
- Malicious entries get merged

**Current Mitigations**: None explicitly mentioned beyond "standard code review."

**Why Concerning:**
- Seed lists are static JSON files that contributors edit directly
- No integrity checking or signing of seed list files
- No diff-based review tooling to highlight entries added/changed

### 1.6 CI Freshness Check Compromise

**Attack Vector**: An attacker who can modify the freshness workflow could:
1. Disable the freshness check entirely
2. Modify it to skip certain entries (allowlist malicious repos)
3. Modify it to auto-merge stale entry removals without human review

**Current Mitigations**: Standard GitHub repository protections (branch protection, required reviews).

**Why Concerning:**
- The design doesn't specify that workflow files should be part of CODEOWNERS or require specific reviewers
- A compromised maintainer account could modify the workflow

### 1.7 GitHub API Response Manipulation (Low Probability)

**Attack Vector**: An attacker with MITM position or GitHub API access could return fake validation responses.

**Example:**
- Validation queries GitHub API for `malicious-org/fake-tool`
- Attacker returns 200 OK with fake release data
- Tool passes validation

**Current Mitigations**: "HTTPS transport, standard API authentication"

**Why Low Priority:**
- Requires GitHub infrastructure compromise or MITM (difficult)
- But the design lists this as "GitHub infrastructure compromise" residual risk without acknowledging the MITM vector

---

## 2. Mitigation Sufficiency Analysis

### 2.1 Registry Poisoning via PR

**Design Mitigation**: "Standard code review process, automated validation"

**Residual Risk**: "Reviewer oversight for subtle repo name changes"

**Assessment**: **INSUFFICIENT**

**Gaps:**
- "Standard code review" is not defined. What are reviewers expected to check?
- No verification process documented (e.g., "for each entry, independently verify the canonical source")
- No tooling to assist reviewers (e.g., automated check against a trusted source like Homebrew formulas)
- No requirement for multi-reviewer approval on seed list changes
- No CODEOWNERS configuration requiring specific reviewers for `data/discovery-seeds/`

**Recommended Enhancements:**
1. **Canonical Source Verification**: Build a verification step that cross-references seed entries against trusted sources:
   - Homebrew formulas (most CLI tools have Homebrew formulas pointing to canonical repos)
   - Awesome-lists (github/awesome, etc.)
   - Official tool documentation
2. **Review Checklist**: Document a mandatory review checklist for seed list PRs:
   - [ ] Verified tool exists via independent search
   - [ ] Confirmed repo is official/canonical via tool's website or documentation
   - [ ] Checked repo owner is legitimate organization/maintainer
   - [ ] Verified no typosquatting (similar names to existing entries)
3. **CODEOWNERS**: Require review from security-focused maintainers for seed list changes
4. **Automated Diff Highlighting**: CI job that comments on PRs with a clear diff of added/changed/removed entries
5. **Popularity Threshold**: Require entries to meet minimum popularity criteria (stars, downloads, etc.) to reduce risk of obscure malicious tools

### 2.2 Stale Entry to Transferred Repo

**Design Mitigation**: "Weekly freshness check with ownership comparison"

**Residual Risk**: "7-day window between transfer and detection"

**Assessment**: **PARTIALLY SUFFICIENT**

**Gaps:**
- The design mentions "ownership comparison" but doesn't specify how this works
- What if ownership legitimately changes (e.g., project moves to a foundation)?
- No plan for notifying users who already installed the tool before transfer detection
- No rollback mechanism if a transferred repo is detected

**Recommended Enhancements:**
1. **Ownership Fingerprinting**: Store the expected owner in seed lists as a verification field:
   ```json
   {"name": "kubectl", "repo": "kubernetes/kubernetes", "expected_owner": "kubernetes"}
   ```
2. **Ownership Change Alerts**: When ownership changes detected:
   - Immediate issue creation (not just weekly)
   - Notify all users via security advisory if tool is popular
   - Temporarily disable the discovery entry until manual review
3. **Transfer Legitimacy Check**: Maintain a list of "expected transfers" (e.g., projects moving to CNCF) to reduce false positives
4. **Historical Validation**: On first addition, check repo ownership history (did it recently transfer?) to catch pre-poisoned repos

### 2.3 Wrong Disambiguation

**Design Mitigation**: "Automated collision detection + human review"

**Residual Risk**: "Obscure collisions not in seed lists"

**Assessment**: **INSUFFICIENT**

**Gaps:**
- "Human review" process not defined for disambiguation decisions
- No criteria documented for "correct" choice when collision exists
- No verification that chosen source is more popular/trustworthy
- Disambiguation entries bypass the "skip if recipe exists" logic (preserved even with recipe), increasing attack surface

**Recommended Enhancements:**
1. **Disambiguation Policy**: Document clear criteria for choosing the canonical source:
   - Prioritize official tool websites and documentation
   - Use popularity metrics (GitHub stars for GitHub tools, npm downloads for npm tools)
   - Default to the tool that matches the primary use case (CLI tools favor GitHub releases over npm libraries)
2. **Popularity Verification**: Require disambiguation PRs to include evidence of relative popularity:
   ```json
   {
     "name": "bat",
     "builder": "github",
     "source": "sharkdp/bat",
     "disambiguation": true,
     "justification": "sharkdp/bat (28k stars, CLI tool) vs npm:bat (200k downloads, testing framework). CLI use case."
   }
   ```
3. **Disambiguation Review Board**: Require 2+ approvals on disambiguation entries from different maintainers
4. **Periodic Re-evaluation**: Disambiguation entries should be re-validated annually (popularity can shift)

### 2.4 GitHub API Compromise

**Design Mitigation**: "HTTPS transport, standard API authentication"

**Residual Risk**: "GitHub infrastructure compromise"

**Assessment**: **ACCEPTABLE BUT INCOMPLETE**

**Gaps:**
- Doesn't acknowledge MITM attack vector (certificate pinning could help)
- Doesn't consider compromised GitHub token (CI uses `secrets.GITHUB_TOKEN`)
- No fallback validation mechanism if GitHub API is suspected compromised

**Recommended Enhancements:**
1. **Multi-Source Validation**: For critical tools, cross-validate against multiple sources:
   - GitHub API
   - Homebrew formula repo (another trust anchor)
   - Archive.org snapshots of official tool websites
2. **Certificate Pinning**: Pin GitHub's certificate in the validation tool (defense-in-depth)
3. **Token Rotation**: Use short-lived GitHub tokens for CI validation
4. **Validation Caching**: Store hashes of validation responses to detect inconsistencies over time

---

## 3. Residual Risk Escalation

### 3.1 Risks Requiring Escalation

#### High Priority: Pre-Deployment Blockers

**Risk: Unverified Canonical Sources**
- **Impact**: Users install malicious tools thinking they're legitimate
- **Likelihood**: Medium (requires successful malicious PR merge, but only needs to succeed once among 500 entries)
- **Combined Risk**: HIGH
- **Recommendation**: BLOCK deployment until canonical source verification is implemented

**Risk: Disambiguation Override Abuse**
- **Impact**: Users systematically redirected to malicious tools for common names
- **Likelihood**: Low-Medium (requires malicious PR merge + bypass of "extra review," but high impact targets are obvious)
- **Combined Risk**: MEDIUM-HIGH
- **Recommendation**: Implement disambiguation policy and multi-reviewer requirement before deployment

#### Medium Priority: Post-Deployment Enhancements

**Risk: Typosquatting Entry Injection**
- **Impact**: Users who mistype tool names install malicious tools
- **Likelihood**: Low (requires user typo + malicious entry existence)
- **Combined Risk**: MEDIUM
- **Recommendation**: Implement fuzzy-match detection in PR review automation (warn on entries similar to existing)

**Risk: Compromised-Then-Legitimized Repos**
- **Impact**: Users install initially malicious tools that evade detection
- **Likelihood**: Low (requires long-term attacker commitment, success against code review)
- **Combined Risk**: MEDIUM
- **Recommendation**: Implement popularity/trust scoring on initial addition, re-validate trust on schedule

### 3.2 Risks Acceptable As-Is

**Risk: GitHub API Compromise**
- **Impact**: Validation bypassed, malicious entries pass checks
- **Likelihood**: Very Low (requires GitHub infrastructure breach or sophisticated MITM)
- **Combined Risk**: LOW-MEDIUM
- **Recommendation**: Accept residual risk, implement multi-source validation in future iteration

**Risk: CI Freshness Check Compromise**
- **Impact**: Stale/malicious entries not detected
- **Likelihood**: Low (requires maintainer account compromise)
- **Combined Risk**: MEDIUM
- **Recommendation**: Accept residual risk, implement CODEOWNERS for workflow files

---

## 4. "Not Applicable" Justification Review

The design lists several standard security categories as "not applicable." Let's verify:

### 4.1 Download Verification

**Design Justification**: "The seed-discovery tool doesn't download binaries. It queries the GitHub API... No artifacts are downloaded or executed during the bootstrap process."

**Review**: **CORRECT - NOT APPLICABLE**

The tool genuinely doesn't download or execute binaries during bootstrap. However, this is misleading because the OUTPUT of the bootstrap (discovery.json) directly controls what binaries users download. The design should acknowledge this transitive risk more clearly.

**Recommendation**: Add note: "Download verification is not applicable to the bootstrap tool itself, but the discovery entries it produces control what binaries end users download. Entry accuracy is therefore a supply chain security concern equivalent to download verification."

### 4.2 Execution Isolation

**Design Justification**: "The tool runs locally or in CI. It makes read-only GitHub API calls and writes a JSON file. No code execution, no sandbox needed, no elevated permissions required."

**Review**: **CORRECT - NOT APPLICABLE**

The tool's operations are genuinely low-risk. But:

**Potential Issue**: If the tool is extended to validate binaries in the future (e.g., download and checksum verify that releases actually contain usable binaries), execution isolation WOULD become applicable.

**Recommendation**: Add note: "Not applicable for current read-only API validation. If future enhancements include downloading/inspecting release artifacts, re-evaluate execution isolation requirements."

### 4.3 User Data Exposure

**Design Justification**: "The bootstrap tool sends tool names and repo identifiers to the GitHub API. This is equivalent to browsing GitHub. No user data is collected or transmitted."

**Review**: **CORRECT - NOT APPLICABLE**

But the design misses a related concern:

**Gap**: The published discovery.json is a CURATED LIST of tools. This list reveals what tools tsuku users are likely to install, which could be valuable intelligence for attackers (e.g., "focus malicious efforts on tools in the discovery registry since those are popular").

**Recommendation**: Add note: "No user data exposure during bootstrap. The published registry is a curated list of popular tools (public information). Attack surface consideration: the registry acts as a 'top 500 targets' list for attackers."

---

## 5. Recommended Security Enhancements

### Priority 1: Pre-Deployment (MUST HAVE)

1. **Canonical Source Verification Database**
   - Build a seed list verification step that cross-references against Homebrew formulas or official sources
   - Require each entry to include a `verification_source` field documenting where canonical repo was confirmed
   - Automate cross-check where possible (e.g., parse Homebrew formula repo for `url` fields)

2. **Seed List Review Checklist**
   - Document mandatory checklist for PR reviewers
   - Include: canonical source verification, typosquatting check, popularity threshold, official documentation confirmation
   - Add checklist template to PR template when seed files are modified

3. **CODEOWNERS Configuration**
   - Require at least 2 reviewers for `data/discovery-seeds/**` changes
   - Require at least 1 security-focused maintainer approval

4. **Disambiguation Policy**
   - Document criteria for choosing canonical source when collisions exist
   - Require justification field in disambiguation entries
   - Require 2+ approvals for disambiguation overrides

### Priority 2: Launch Window (SHOULD HAVE)

5. **Automated Diff Highlighting**
   - CI job that posts PR comment with clear table of added/changed/removed entries
   - Include metrics: GitHub stars, release count, last release date
   - Flag entries with <100 stars or <5 releases for extra scrutiny

6. **Typosquatting Detection**
   - Automated check for entries with Levenshtein distance <3 from existing entries
   - Warn on PR if potential typosquatting detected

7. **Ownership Change Monitoring**
   - Store expected owner in seed list entries
   - Freshness check compares current owner to expected owner
   - Immediate alert (not just weekly) on ownership change

8. **Transfer Legitimacy Allowlist**
   - Maintain list of expected/legitimate transfers (e.g., `kubernetes-sigs/* -> kubernetes/*`)
   - Reduce false positive rate on ownership checks

### Priority 3: Post-Launch (NICE TO HAVE)

9. **Popularity Re-validation**
   - Periodic (quarterly?) re-check that entries still meet popularity thresholds
   - Remove entries for abandoned/archived projects

10. **Multi-Source Validation**
    - For top 50 critical tools, cross-validate repo identity against multiple trust anchors
    - E.g., GitHub API + Homebrew formula + official website parsing

11. **User Notification System**
    - When stale/transferred entry detected, notify users who have the tool installed
    - Could be via telemetry callback or registry update with warning

12. **Security Audit Log**
    - Track all changes to seed lists with detailed justifications
    - Periodic review of audit log for suspicious patterns

---

## 6. Implementation Recommendations

### 6.1 Enhanced Seed List Schema

```json
{
  "category": "dev-tools",
  "description": "General development CLI tools",
  "entries": [
    {
      "name": "ripgrep",
      "repo": "BurntSushi/ripgrep",
      "expected_owner": "BurntSushi",
      "verification_source": "https://github.com/BurntSushi/ripgrep (official repo per https://github.com/BurntSushi/ripgrep#readme)",
      "verified_by": "maintainer-username",
      "verified_date": "2026-01-15",
      "min_stars": 1000,
      "disambiguation": false
    },
    {
      "name": "bat",
      "repo": "sharkdp/bat",
      "expected_owner": "sharkdp",
      "verification_source": "https://github.com/sharkdp/bat (official repo, 28k stars vs npm:bat 200k downloads but npm package is testing framework)",
      "verified_by": "maintainer-username",
      "verified_date": "2026-01-15",
      "min_stars": 1000,
      "disambiguation": true,
      "disambiguation_justification": "GitHub release is CLI tool (primary use case), npm package is testing framework"
    }
  ]
}
```

### 6.2 PR Review Checklist Template

```markdown
## Seed List Entry Review Checklist

For each new or modified entry, verify:

- [ ] **Canonical Source**: Confirmed repo is official via tool's website/documentation
- [ ] **Verification Source**: Entry includes `verification_source` URL documenting proof
- [ ] **Popularity**: Repo has minimum 100 stars OR is official tool from known org (e.g., HashiCorp, Kubernetes)
- [ ] **Activity**: Repo has release within last 12 months
- [ ] **Ownership**: `expected_owner` matches current GitHub owner
- [ ] **Typosquatting**: No similar names in existing entries (Levenshtein distance check)
- [ ] **Disambiguation**: If `disambiguation: true`, justification is clear and evidence-based

For disambiguation entries, ADDITIONALLY verify:
- [ ] **Multi-source verification**: Confirmed canonical source from 2+ independent sources
- [ ] **Popularity comparison**: Documented why this source is preferred over alternatives
- [ ] **Second reviewer approval**: At least 2 maintainers have approved

Security escalation: If entry is for a tool with >10k stars or commonly used in CI/CD pipelines, require security-focused maintainer approval.
```

### 6.3 Enhanced cmd/seed-discovery Validation

```go
// Validation steps to add:

// 1. Canonical Source Cross-Reference
func validateCanonicalSource(entry SeedEntry) error {
    // Cross-reference against Homebrew formulas
    homebrewRepo := fetchHomebrewFormula(entry.name)
    if homebrewRepo != "" && homebrewRepo != entry.repo {
        return fmt.Errorf("repo mismatch: seed=%s, homebrew=%s", entry.repo, homebrewRepo)
    }

    // Verify verification_source is documented
    if entry.VerificationSource == "" {
        return fmt.Errorf("missing verification_source field")
    }

    return nil
}

// 2. Typosquatting Detection
func checkTyposquatting(entry SeedEntry, existing []SeedEntry) error {
    for _, e := range existing {
        distance := levenshtein(entry.name, e.name)
        if distance < 3 && entry.name != e.name {
            return fmt.Errorf("potential typosquatting: %s too similar to existing entry %s (distance=%d)",
                entry.name, e.name, distance)
        }
    }
    return nil
}

// 3. Popularity Threshold
func checkPopularity(entry SeedEntry, repo *github.Repository) error {
    if entry.MinStars == 0 {
        entry.MinStars = 100 // default threshold
    }

    if repo.StargazersCount < entry.MinStars {
        // Allow exceptions for official tools from known orgs
        if !isKnownOrg(repo.Owner.Login) {
            return fmt.Errorf("below popularity threshold: %d stars (min %d)",
                repo.StargazersCount, entry.MinStars)
        }
    }

    return nil
}

// 4. Ownership Verification
func checkOwnership(entry SeedEntry, repo *github.Repository) error {
    if entry.ExpectedOwner != "" && repo.Owner.Login != entry.ExpectedOwner {
        return fmt.Errorf("ownership mismatch: expected=%s, actual=%s",
            entry.ExpectedOwner, repo.Owner.Login)
    }
    return nil
}
```

---

## 7. Comparison to Industry Standards

### 7.1 Similar Systems

How do other package managers handle discovery/registry security?

**Homebrew:**
- Formula PRs require manual review by maintainers
- Formulas point to official sources (verified via tool websites)
- Community reports suspicious formulas
- No automated canonical source verification

**npm:**
- Package namespace is first-come-first-served (typosquatting is rampant)
- No central "canonical source" verification
- Relies on user vigilance and post-compromise detection

**apt/yum:**
- Packages signed by distribution maintainers
- Centralized trust model (distro maintainers are trust anchor)
- Package maintainers verify upstream sources

**Homebrew Cask:**
- Similar to Homebrew formulas
- Requires manual verification of official sources
- CODEOWNERS for popular casks

**Tsuku Discovery Registry:**
- Most similar to Homebrew formulas (curated list, manual review)
- Gap: Homebrew has years of community trust and established review processes
- Gap: Tsuku has smaller maintainer pool (higher risk of reviewer oversight)

**Recommendation**: Adopt Homebrew-like review rigor, but enhance with automated canonical source verification (Homebrew doesn't have this).

### 7.2 Security Frameworks

**SLSA (Supply Chain Levels for Software Artifacts):**
- Level 1: Documentation of build process (tsuku has this)
- Level 2: Version control + build service (tsuku has this via CI)
- Level 3: Source and build platform hardening (PARTIAL - CI hardening needed)
- Level 4: Hermetic builds + two-party review (NOT MET - two-party review for seed lists needed)

**Recommendation**: Target SLSA Level 3 for discovery registry by adding:
- Two-party review requirement for seed lists
- Audit logging of all seed list changes
- Automated provenance verification (canonical source checks)

---

## 8. Conclusion

### 8.1 Risk Summary

| Risk Category | Current State | Target State | Gap Severity |
|---------------|---------------|--------------|--------------|
| Canonical Source Verification | Manual review only | Automated + manual | CRITICAL |
| Typosquatting Prevention | None | Automated detection | HIGH |
| Disambiguation Integrity | Human review (undefined process) | Policy + multi-approval | HIGH |
| Ownership Change Detection | Weekly check planned | Enhanced with historical validation | MEDIUM |
| Supply Chain Injection | Standard code review | CODEOWNERS + automated diff | MEDIUM |
| GitHub API Compromise | HTTPS only | Multi-source validation | LOW |

### 8.2 Go/No-Go Recommendation

**Current State**: NOT READY for production deployment

**Blockers**:
1. No canonical source verification process
2. Disambiguation policy undefined
3. No CODEOWNERS or enhanced review process for seed lists

**Minimum Viable Security (MVS)**:
To proceed with deployment, implement:
- Enhanced seed list schema with verification_source field
- PR review checklist for seed list changes
- CODEOWNERS requirement (2+ reviewers)
- Disambiguation policy and justification requirement
- Automated diff highlighting in CI

**Timeline Estimate**: 2-3 weeks to implement MVS enhancements

### 8.3 Final Recommendations

1. **Immediate Actions** (before any seed list population):
   - Implement canonical source verification
   - Document PR review checklist
   - Set up CODEOWNERS for seed directories
   - Define disambiguation policy

2. **Pre-Launch Actions** (before discovery resolver goes live):
   - Enhance cmd/seed-discovery with security validations
   - Implement ownership change monitoring
   - Set up automated diff highlighting

3. **Post-Launch Actions** (continuous improvement):
   - Multi-source validation for critical tools
   - Quarterly re-validation of all entries
   - User notification system for stale entries
   - Security audit logging

4. **Process Changes**:
   - Require 2+ approvals for all seed list PRs
   - Monthly security review of discovery.json changes
   - Incident response plan for compromised entries

---

## Appendix A: Attack Scenarios

### Scenario 1: Targeted Typosquatting Attack

**Attacker Goal**: Get users to install malicious kubectl

**Attack Steps**:
1. Create repo `kubernetes-cli/kubernetes` with release artifacts containing backdoored kubectl
2. Submit PR adding seed entry: `{"name": "kubeclt", "repo": "kubernetes-cli/kubernetes", "binary": "kubectl"}`
3. PR passes automated validation (repo exists, has releases)
4. Human reviewer misses the typo (kubeclt vs kubectl) and repo name mismatch
5. Entry merged, published in discovery.json
6. Users who type `tsuku install kubeclt` get malicious binary

**Current Defenses**: PR review

**Weaknesses**: Easy to miss single-character typo in 500 entries

**Enhanced Defenses**: Typosquatting detection + canonical source verification + CODEOWNERS

### Scenario 2: Disambiguation Override Hijack

**Attacker Goal**: Redirect bat users to malicious npm package

**Attack Steps**:
1. Publish malicious npm package named `bat`
2. Submit PR with disambiguation entry: `{"name": "bat", "builder": "npm", "source": "bat", "disambiguation": true}`
3. Justify as "npm version is more popular" (cite download counts vs GitHub stars)
4. PR reviewer doesn't independently verify which source matches primary use case
5. Entry merged, all `tsuku install bat` requests go to npm malicious package

**Current Defenses**: "Extra review attention" for disambiguation entries

**Weaknesses**: No defined process for "extra attention," no verification criteria

**Enhanced Defenses**: Disambiguation policy + justification requirement + multi-approval + popularity comparison automation

### Scenario 3: Slow-Burn Legitimate-Looking Malware

**Attacker Goal**: Distribute malware via discovery registry with minimal suspicion

**Attack Steps**:
1. Create repo `dev-tools-cli/serve` with polished documentation, legitimate-looking code
2. Publish release with backdoored binary (exfiltrates environment variables to attacker server)
3. Submit PR: `{"name": "serve", "repo": "dev-tools-cli/serve"}`
4. Justification: "faster alternative to vercel/serve"
5. Passes validation (repo exists, has releases, looks professional)
6. PR reviewer doesn't independently verify this is the canonical/popular serve implementation
7. Entry merged
8. Users install, malware exfiltrates secrets

**Current Defenses**: PR review, weekly freshness check

**Weaknesses**:
- Freshness check doesn't re-validate trust/popularity
- No requirement to verify entry is canonical source
- Small/new repos can look legitimate with enough effort

**Enhanced Defenses**: Popularity threshold + canonical source verification + cross-reference against Homebrew + security audit of high-risk tools

---

## Appendix B: Recommended Policies

### Policy 1: Canonical Source Verification

All discovery registry entries MUST be verified as pointing to the official/canonical source for the tool:

**Verification Process**:
1. Search for tool's official website or documentation
2. Verify GitHub repo is linked from official source
3. Cross-reference against at least one additional trust anchor:
   - Homebrew formula
   - Tool's package manager listing (npm, crates.io, etc.)
   - Official installation documentation
4. Document verification source in seed list entry

**Exceptions**:
- Tools from known organizations (HashiCorp, Kubernetes, AWS, etc.) where org ownership is verified
- Tools explicitly flagged as "community forks" with justification

### Policy 2: Disambiguation Resolution

When a tool name exists in multiple ecosystems, the discovery registry MAY include a disambiguation override:

**Resolution Criteria** (in priority order):
1. **Official Source**: If one source is the "official" tool distribution per official docs, choose that
2. **Primary Use Case**: For CLI tool discovery, prefer GitHub releases over language-specific packages unless the latter is explicitly the canonical distribution
3. **Popularity**: If both sources are legitimate, choose the more popular (GitHub stars for releases, downloads for ecosystem packages)
4. **Maintainer Intent**: If tool maintainer has documented preferred installation method, respect that

**Documentation Requirement**:
All disambiguation entries MUST include:
- `disambiguation_justification` field explaining the choice
- Evidence of relative popularity or official source status
- Date of verification

**Review Requirement**:
Disambiguation entries require 2+ maintainer approvals from different individuals

### Policy 3: Popularity Thresholds

To reduce risk of obscure/malicious tools:

**Minimum Requirements** (one must be met):
- GitHub repo has ≥100 stars, OR
- GitHub repo is owned by known organization (kubernetes, hashicorp, aws, etc.), OR
- Tool is explicitly requested by user via issue with justification

**Exceptions**:
- Niche tools that are canonical sources for specific use cases (documented justification required)

### Policy 4: Typosquatting Prevention

Entries with names too similar to existing entries are REJECTED unless:

**Acceptance Criteria**:
- Levenshtein distance ≥3 from all existing entries, OR
- Entry is a legitimate alternate name for an existing tool (documented as alias), OR
- Entry is disambiguating a collision (with disambiguation policy compliance)

**Automation**:
CI MUST flag potential typosquatting (distance <3) on all PRs

### Policy 5: Ownership Stability

All entries MUST include `expected_owner` field:

**Monitoring**:
- Weekly freshness check compares current owner to expected owner
- Ownership changes trigger immediate investigation
- Entry is disabled until ownership change is verified as legitimate

**Legitimate Transfer Allowlist**:
Known legitimate transfers (e.g., project moving to foundation) are pre-approved

### Policy 6: Seed List Review

All PRs modifying `data/discovery-seeds/**` MUST:

**Review Requirements**:
- Include completed review checklist in PR description
- Have ≥2 approvals from CODEOWNERS
- Pass automated validation (canonical source, typosquatting, popularity)
- For disambiguation entries: ≥2 approvals, one from security-focused maintainer

**CI Requirements**:
- Automated diff table posted as PR comment
- Flagging of entries below popularity thresholds
- Typosquatting detection
- Canonical source cross-reference (where automatable)
