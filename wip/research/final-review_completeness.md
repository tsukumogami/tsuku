# Final Review: Design Document Completeness for Issue #722

## Overview

This review assesses the completeness of two design documents against Issue #722 requirements and design document standards:

- `docs/DESIGN-system-dependency-actions.md` (action vocabulary)
- `docs/DESIGN-structured-install-guide.md` (sandbox container building)

## 1. Issue #722 Acceptance Criteria

Issue #722 title: "docs(design): create design for structured install_guide"

| Criterion | Status | Location | Assessment |
|-----------|--------|----------|------------|
| Design document created at `docs/DESIGN-structured-install-guide.md` | COMPLETE | File exists | The file exists and addresses the problem space |
| Covers structured package format | COMPLETE | Both docs, primarily DESIGN-system-dependency-actions.md "Action Vocabulary" section | Comprehensive coverage: `apt_install`, `apt_repo`, `brew_install`, `brew_cask`, `dnf_install`, `pacman_install` actions with typed parameters |
| Covers minimal container approach | COMPLETE | DESIGN-structured-install-guide.md "Decision 3: Base Container Strategy", "Minimal Base Container" section | Option 3A chosen with clear rationale; Dockerfile example provided |
| Covers sandbox executor changes | COMPLETE | DESIGN-structured-install-guide.md "Sandbox Executor Changes" section | `ExtractPackages()` and `DeriveContainerSpec()` implementations specified |
| Status set to Proposed | COMPLETE | Both docs line 5 | Status: Proposed |
| Ready for review | COMPLETE | Both docs are feature-complete | All required sections present, comprehensive analysis |

**Summary**: All 6 acceptance criteria from Issue #722 are fully addressed.

## 2. Design Document Standards Compliance

Per the `design-doc` skill, every design document MUST have these core sections in order:

### DESIGN-system-dependency-actions.md

| Required Section | Present | Location | Quality Assessment |
|------------------|---------|----------|-------------------|
| 1. Status | YES | Line 3-5 | "Proposed" - correct for draft |
| 2. Context and Problem Statement | YES | Lines 7-43 | Strong: clearly identifies 5 problems with current `require_system` |
| 3. Decision Drivers | PARTIAL | Embedded in "Design Goals" (lines 64-70) | Section titled "Design Goals" not "Decision Drivers" - but content serves same purpose |
| 4. Considered Options (2+ viable) | YES | "Decisions" section (lines 72-238) | 5 decisions (D1-D5) with multiple options each, pros/cons documented |
| 5. Decision Outcome | PARTIAL | Integrated into each Decision section | No consolidated "Decision Outcome" section - decisions are embedded with rationale |
| 6. Solution Architecture | YES | "Action Vocabulary" (lines 241-274), "WhenClause Extension" (lines 354-390), "Example" (lines 392-438) | Comprehensive action catalog and schema definitions |
| 7. Implementation Approach | YES | Lines 440-476 | 4 phases clearly defined |
| 8. Security Considerations | PARTIAL | Moved to "Future Work" (lines 478-525) | Security constraints documented but framed as future work, not current scope |
| 9. Consequences | MISSING | Not present | No explicit Consequences section |

### DESIGN-structured-install-guide.md

| Required Section | Present | Location | Quality Assessment |
|------------------|---------|----------|-------------------|
| 1. Status | YES | Line 5 | "Proposed" - correct |
| 2. Context and Problem Statement | YES | Lines 35-102 | Excellent: identifies 3 problems clearly with code examples |
| 3. Decision Drivers | YES | Lines 104-115 | 5 clear decision drivers |
| 4. Considered Options (2+ viable) | YES | Lines 117-320 | 4 decisions with multiple options each, comprehensive pros/cons |
| 5. Decision Outcome | YES | Lines 322-361 | "Chosen: 1B + 2B + 3A + 4C" with summary and rationale |
| 6. Solution Architecture | YES | Lines 363-550 | Detailed with code examples, action vocabulary reference |
| 7. Implementation Approach | YES | Lines 718-749 | 4 phases clearly defined |
| 8. Security Considerations | YES | Lines 750-825 | Comprehensive - addresses all 4 dimensions |
| 9. Consequences | YES | Lines 894-919 | Positive, negative, and mitigations documented |

### Additional Context-Aware Sections

| Section | Required? | DESIGN-system-dependency-actions.md | DESIGN-structured-install-guide.md |
|---------|-----------|------------------------------------|------------------------------------|
| Upstream Design Reference | If exists | Missing - should reference DESIGN-golden-plan-testing.md | PRESENT (Lines 22-33) |
| Implementation Issues | After /plan | N/A (status is Proposed) | N/A (status is Proposed) |

## 3. Missing Content Analysis

### DESIGN-system-dependency-actions.md

1. **No explicit "Decision Drivers" section**
   - Content exists under "Design Goals" but uses different heading
   - RECOMMENDATION: Rename "Design Goals" to "Decision Drivers" for consistency

2. **No consolidated "Decision Outcome" section**
   - Decisions and outcomes are embedded in the "Decisions" section
   - Each D1-D5 subsection has its chosen option, but no unified summary
   - RECOMMENDATION: Add "Decision Outcome" section summarizing "D1: Option A, D2: Option B..." with rationale for how choices work together

3. **No "Consequences" section**
   - Missing entirely
   - RECOMMENDATION: Add section documenting positive/negative consequences and mitigations

4. **Security in Future Work, not Security Considerations**
   - Security constraints (group allowlisting, tiered consent, etc.) are framed as "Future Work: Host Execution"
   - This is appropriate since current scope excludes host execution
   - However, there should be a "Security Considerations" section documenting current scope security
   - RECOMMENDATION: Add minimal "Security Considerations" section stating that current scope (documentation generation + sandbox) has no privileged host operations, and reference future work for host execution

5. **No Upstream Design Reference**
   - Should reference DESIGN-golden-plan-testing.md blocker section
   - RECOMMENDATION: Add after Status section

### DESIGN-structured-install-guide.md

1. **Largely complete** - this document adheres well to the design-doc skill standards

2. **Minor: No explicit handling of action failure modes in sandbox**
   - What happens if `apt-get install` fails inside container?
   - RECOMMENDATION: Add brief error handling section under "Action Execution in Sandbox"

## 4. Edge Cases and Failure Modes

### Covered Edge Cases

| Edge Case | Coverage | Location |
|-----------|----------|----------|
| Distro detection failure | COVERED | DESIGN-system-dependency-actions.md line 131-134 |
| Unknown package manager | COVERED | `manual` action as fallback |
| Missing /etc/os-release | COVERED | Falls back to skipping distro-filtered steps |
| Content-addressing for external resources | COVERED | DESIGN-structured-install-guide.md lines 469-481 |

### Missing Edge Cases

| Edge Case | Risk | Recommendation |
|-----------|------|----------------|
| **Container build failure** | Medium | What happens when package installation in sandbox fails? Need retry/error handling spec |
| **Hash mismatch for external resources** | Low | Preflight rejects, but no user guidance on resolution |
| **Package conflicts** | Medium | What if two packages conflict in derived container? |
| **Disk space exhaustion** | Low | Container cache can grow unbounded; no eviction policy |
| **Network failure during sandbox build** | Medium | Package downloads may fail; no retry spec |
| **Action ordering violations** | Low | What if `service_enable` comes before `apt_install` for same package? |

### What Happens When Things Fail?

**Current coverage:**
- Preflight validation catches missing parameters and unhashed resources
- Package managers handle "already installed" gracefully (idempotent)

**Missing coverage:**
- No explicit error propagation strategy from sandbox actions
- No rollback mechanism if partial container build fails
- No user-facing error messages for specific failure modes

**RECOMMENDATION**: Add "Error Handling" subsection to Implementation Approach specifying:
1. How action failures are reported to the user
2. Whether partial failures allow continuation or abort
3. Container cleanup on failure (ephemeral, so probably just delete)

## 5. Security Coverage Assessment

The design-doc skill requires 4 security dimensions be addressed:

### DESIGN-system-dependency-actions.md

| Dimension | Status | Location | Assessment |
|-----------|--------|----------|------------|
| Download verification | PARTIAL | Line 500 mentions content-addressing in security constraints | Framed as future work, but applies to current scope |
| Execution isolation | PARTIAL | Not explicitly addressed for current scope | Sandbox execution is in companion doc |
| Supply chain risks | YES | Lines 500-510 mention repository allowlisting | Well analyzed for future host execution |
| User data exposure | NOT ADDRESSED | Missing for current scope | Should note: documentation generation accesses no user data |

**Assessment**: Security is documented but framed as "Future Work" for host execution. Current scope security should be explicit.

### DESIGN-structured-install-guide.md

| Dimension | Status | Location | Assessment |
|-----------|--------|----------|------------|
| Download verification | YES | Lines 765-779 | Content-addressing with SHA256, preflight validation |
| Execution isolation | YES | Lines 781-801 | Container isolation, ephemeral, no host filesystem access |
| Supply chain risks | YES | Lines 803-816 | Package manager trust, recipe review, action vocabulary control |
| User data exposure | YES | Lines 818-825 | No user data mounted, container destroyed after execution |

**Assessment**: FULLY COMPLIANT. All 4 dimensions thoroughly addressed with clear analysis.

### Sandbox vs Host Distinction

| Context | Clear? | Evidence |
|---------|--------|----------|
| Sandbox execution | YES | Explicit in both docs; containers run as root internally |
| Host execution (current) | YES | Both docs state "tsuku does not execute system dependencies on host" |
| Host execution (future) | YES | Explicitly deferred with security constraints outlined |

**Assessment**: The sandbox vs host distinction is exceptionally clear. Both documents carefully scope current behavior (documentation generation + sandbox testing) vs future behavior (host execution).

## 6. Specific Recommendations

### High Priority (Missing Required Sections)

1. **DESIGN-system-dependency-actions.md: Add "Decision Outcome" section**
   ```markdown
   ## Decision Outcome

   **Chosen: D1-A + D2-B + D3-C + D4-A + D5-Hybrid**

   ### Summary
   We replace the polymorphic `require_system` with granular typed actions
   (`apt_install`, `brew_cask`, etc.), using `/etc/os-release` for distro
   detection via `when` clause extension, idempotent installation with
   final `require_command` verification, and separate actions for
   post-install configuration.

   ### Rationale
   [Explain how D1-D5 choices work together...]
   ```

2. **DESIGN-system-dependency-actions.md: Add "Consequences" section**
   ```markdown
   ## Consequences

   ### Positive
   - Typed actions enable static analysis and auditing
   - Consistent platform filtering via `when` clause
   - Machine-readable format enables documentation generation and sandbox building

   ### Negative
   - More verbose than polymorphic `require_system`
   - Requires Go code changes for new actions

   ### Mitigations
   - Verbosity traded for explicit, auditable behavior
   - New actions follow established patterns
   ```

3. **DESIGN-system-dependency-actions.md: Add "Security Considerations" section**
   ```markdown
   ## Security Considerations

   **Current scope (documentation generation + sandbox):**
   - No privileged operations on host
   - Documentation generation accesses only recipe files
   - Sandbox execution is fully isolated in containers

   For future host execution security constraints, see
   [Future Work: Host Execution](#host-execution).
   ```

4. **DESIGN-system-dependency-actions.md: Add "Upstream Design Reference"**
   ```markdown
   ## Upstream Design Reference

   This design addresses part of [DESIGN-golden-plan-testing.md](DESIGN-golden-plan-testing.md).

   **Relevant sections:**
   - Blocker: Structured install_guide for System Dependencies (lines 1465-1515)
   ```

### Medium Priority (Missing Edge Cases)

5. **DESIGN-structured-install-guide.md: Add error handling for sandbox builds**
   Under "Sandbox Executor Changes", add:
   ```markdown
   ### Error Handling

   | Failure Mode | Behavior |
   |--------------|----------|
   | Package installation fails | Report error, destroy container, fail recipe test |
   | Hash mismatch for external resource | Reject at preflight, display expected vs actual hash |
   | Container build timeout | Configurable timeout (default 10min), fail with clear message |
   | Network failure | Retry 3 times with exponential backoff, then fail |
   ```

6. **DESIGN-structured-install-guide.md: Add package conflict handling**
   Under "Sandbox Executor Changes", add:
   ```markdown
   ### Package Conflicts

   If extracted packages conflict (e.g., `docker.io` vs `docker-ce`),
   container build fails with apt/dnf error. Recipe author must resolve
   by ensuring only compatible packages are specified for each platform.
   ```

### Low Priority (Polish)

7. **DESIGN-system-dependency-actions.md: Rename "Design Goals" to "Decision Drivers"**
   For consistency with design-doc skill standards.

8. **Both docs: Add cross-reference index**
   At the end of each doc, add a "Related Documents" section listing the companion design for easier navigation.

## Summary

| Document | Issue #722 Compliance | Design Standard Compliance | Security Coverage | Overall |
|----------|----------------------|---------------------------|-------------------|---------|
| DESIGN-system-dependency-actions.md | COMPLETE | PARTIAL (missing Decision Outcome, Consequences, Security Considerations) | PARTIAL | Needs 3-4 section additions |
| DESIGN-structured-install-guide.md | COMPLETE | COMPLETE | COMPLETE | Ready for review |

**Bottom line**:
- `DESIGN-structured-install-guide.md` is essentially complete and well-structured
- `DESIGN-system-dependency-actions.md` needs structural additions to conform to design-doc standards, but the technical content is thorough

The two documents together provide comprehensive coverage of the problem space. The main gap is structural compliance in the action vocabulary design.
