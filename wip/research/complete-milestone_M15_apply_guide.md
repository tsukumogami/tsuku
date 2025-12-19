# Milestone M15 Documentation Guide Update Summary

## Task Overview

Updated `/home/dgazineu/dev/workspace/tsuku/tsuku-1/public/tsuku/docs/GUIDE-plan-based-installation.md` to address documentation gaps identified in the Milestone 15 validation report.

## Changes Made

### 1. Deterministic Flag Explanation (Lines 251-318)

**What was added:**
- Updated plan format reference to include the `deterministic` field in the example JSON
- Added `Understanding Determinism in Plans` section with detailed explanation of:
  - What the `deterministic` field means (byte-for-byte reproducibility)
  - When it's true (file operation primitives only)
  - When it's false (ecosystem primitives like go_build, npm_exec, etc.)
  - Why some tools can't be fully deterministic (compiler versions, native code builds)

**Key content:**

**Fully Deterministic Plans (`deterministic: true`):**
- Uses only file operation primitives: download, extract, chmod, install_binaries
- Produces byte-for-byte identical results across machines and times
- Examples: kubectl, ripgrep, terraform (with pre-built binaries)

**Non-Deterministic Plans (`deterministic: false`):**
- Contains ecosystem primitives: go_build, cargo_build, npm_exec, pip_install, gem_exec, nix_realize, cpan_install
- Captures maximum constraints at eval time through lockfiles
- May have residual non-determinism from:
  - Compiler version differences producing different machine code
  - LLVM and code generators creating platform-specific variations
  - Runtime environment affecting compilation
  - Floating-point precision variations

**When to use non-deterministic plans:**
- CI/CD pipelines for consistent tool versions
- Development environments for team alignment
- Air-gapped deployments
- Supply chain control via captured dependencies

**Practical guidance:**
- How to check the deterministic flag using `jq`
- Understanding the difference between version consistency and binary reproducibility

### 2. Security Model Section (Lines 338-422)

**What was added:**
- New comprehensive `Security Model` section covering trust boundaries and best practices
- Explains how plans are treated as trusted inputs
- Details what checksum verification protects against (and what it doesn't)
- Provides actionable security best practices

**Key content:**

**Plans as Trusted Inputs:**
- tsuku trusts URLs in the plan
- tsuku verifies checksums match the plan
- tsuku does NOT re-validate against external sources
- tsuku does NOT connect to version providers during execution

**Checksum Verification Coverage Table:**
Shows what checksums protect against:
- ✅ File corruption during download
- ✅ Upstream file modifications (re-tagged releases)
- ✅ Man-in-the-middle attacks
- ❌ Malicious URLs in the plan itself
- ❌ Compromised sources at eval time
- ❌ Pre-eval supply chain attacks

**Best Practices:**
1. Generate plans yourself using `tsuku eval`
2. Review plan content before production use
   - Inspect URLs with `jq`
   - Verify checksums
3. Only use plans from trusted sources (organization releases, official projects)
4. Version control your plans in git for audit trails
5. Regenerate plans with review process for updates

**Regeneration Workflow:**
- Generate new plan
- Review diff against current plan
- Replace after approval
- Commit to version control with clear message

## Document Statistics

- **Lines added:** ~100
- **Sections added:** 2 major sections
- **Subsections added:** 8 subsections
- **Code examples:** 8 practical examples
- **Tables:** 2 (plan fields reference + threat protection matrix)

## Validation Against Requirements

### Requirement 1: Deterministic Flag Explanation ✅
- Explains what it means (true = fully reproducible, false = has residual non-determinism)
- Documents when it's true (file operation primitives only)
- Documents when it's false (ecosystem primitives)
- Explains why some tools can't be fully deterministic

### Requirement 2: Security Model Section ✅
- Plans are treated as trusted inputs
- Checksum verification protects against file corruption and upstream changes
- Recommends users generate plans themselves via `tsuku eval`
- Provides recommendations for external sources
- Clear table distinguishing what checksums protect against vs. what they don't

## Content Quality

**Strengths:**
- Written for external contributors (public repository)
- Uses practical examples and code snippets
- Clear threat model and trust boundaries
- Actionable best practices
- Follows existing documentation style and structure
- Naturally integrated with existing plan format reference

**User Value:**
- Users now understand what reproducibility guarantees they get with different actions
- Clear guidance on plan security and trust models
- Practical workflows for safe plan management
- Better educated decisions about using pre-generated vs. self-generated plans

## Cross-References

The new sections fit naturally within the existing guide:
- Determinism section immediately follows "Plan Format Reference"
- Security model section inserted before "Troubleshooting"
- Both sections reference practical commands users already know (jq, tsuku eval, git)
- Security section references earlier sections on CI distribution and plan generation

## Integration Notes

These additions address the Priority 1 critical gaps identified in the validation report:
1. ✅ Document ecosystem primitives concept (in determinism section)
2. ✅ Explain deterministic flag
3. ✅ Add security model section
4. ✅ Show what checksums protect against

The guide is now ready for users to understand:
- The determinism guarantees of different plan types
- Trust boundaries and security implications
- Best practices for safe plan management
- When to use deterministic vs. non-deterministic plans
