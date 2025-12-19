# Documentation Gap Analysis: Milestone M15 (Deterministic Recipe Execution)

## Milestone Overview

**Milestone:** M15 - Deterministic Recipe Execution
**Description:** Implement two-phase installation (eval/exec) for reproducible tool installations
**Design Document:** docs/DESIGN-deterministic-resolution.md
**Closed Issues:** 44

## Analysis Methodology

1. Reviewed milestone design document and all 44 closed issues
2. Examined existing documentation (README.md, guides, help text)
3. Identified features implemented in the milestone
4. Cross-referenced implemented features against documentation coverage
5. Identified gaps, inconsistencies, and missing examples

## Features Implemented in M15

### Core Plan-Based Installation
- ✅ `tsuku eval` command to generate installation plans
- ✅ `tsuku install --plan` to install from pre-computed plans
- ✅ `tsuku plan show` to display stored plans
- ✅ `tsuku plan export` to export plans as JSON files
- ✅ `--fresh` flag to force plan regeneration
- ✅ Plan caching in state.json for reproducible reinstalls
- ✅ Checksum verification during plan execution

### Action Decomposition System
- ✅ Decomposable interface for composite actions
- ✅ Recursive decomposition algorithm
- ✅ Primitive action registry
- ✅ Composite actions decompose to primitives (github_archive, download_archive, github_file, hashicorp_release)
- ✅ Plan validation to ensure only primitives in execution

### Ecosystem Primitives (New Actions)
- ✅ `go_build` with go.sum capture
- ✅ `cargo_build` with Cargo.lock capture
- ✅ `npm_exec` with package-lock.json capture
- ✅ `pip_install` with pip-compile lockfile
- ✅ `gem_exec` with Gemfile.lock capture
- ✅ `nix_realize` with flake.lock
- ✅ `cpan_install` ecosystem primitive
- ✅ `go_install`, `cargo_install`, `npm_install`, `gem_install`, `nix_install` made evaluable via lockfile capture
- ✅ `homebrew_bottle` made evaluable with checksum verification

### Plan Execution Infrastructure
- ✅ Two-phase installation flow (eval/exec)
- ✅ Plan conversion helpers
- ✅ ExecutePlan method with checksum verification
- ✅ Plan loading utilities for external plans
- ✅ Dependency installation steps in plans
- ✅ Deterministic flag in plan schema

## Documentation Coverage Assessment

### ✅ Well-Documented Features

#### 1. Basic Plan-Based Installation (README.md + GUIDE-plan-based-installation.md)
- `tsuku eval` command usage and examples
- `tsuku install --plan` file-based and piped installation
- `--fresh` flag purpose and when to use it
- Plan caching for reproducible installations
- Checksum verification and failure recovery
- Air-gapped deployment workflow
- CI distribution patterns
- Sandbox testing with plans
- Plan format reference with detailed field descriptions

#### 2. CLI Help Text
- `tsuku install --help` includes `--plan` flag with clear examples
- `tsuku eval --help` provides detailed description, use cases, and examples
- Examples show both file-based and piped workflows
- Cross-platform plan generation documented (--os, --arch flags)
- Eval-time dependency installation explained (--yes flag)

#### 3. Plan Commands
- `tsuku plan show` documented with human-readable output format
- `tsuku plan export` documented with stdout and file options
- Help text includes clear examples

### ⚠️ Gaps and Missing Documentation

#### 1. **CRITICAL: Ecosystem Primitives Completely Undocumented for Users**

**Issue:** Milestone M15 implemented 14 new ecosystem primitives and made 6 existing ecosystem actions evaluable through lockfile capture. These are fundamental to the deterministic execution model, but there is **zero user-facing documentation** about them.

**What's Missing:**
- No explanation of what ecosystem primitives are or why they exist
- No documentation on the difference between file operation primitives and ecosystem primitives
- No user guidance on which actions are fully deterministic vs. have residual non-determinism
- No documentation on the `deterministic` flag in plans
- No explanation of when/why plans might be marked as non-deterministic

**Where Documentation Should Exist:**
- README.md should have a section on "Action Types" or "Primitive Actions"
- GUIDE-plan-based-installation.md should explain the deterministic flag
- New guide needed: "Understanding Tsuku Actions" or "Recipe Action Reference"

**Impact:** Users cannot understand:
- Why some tools have `deterministic: false` in their plans
- What guarantees they get with different action types
- How ecosystem primitives handle reproducibility
- What lockfiles are being captured during eval

**Recommended Content:**
```markdown
## Understanding Actions and Determinism

Tsuku uses two types of primitive actions in installation plans:

### File Operation Primitives
Fully deterministic, reproducible operations:
- download: Fetch files with checksum verification
- extract: Decompress archives
- chmod: Set file permissions
- install_binaries: Copy binaries and create symlinks

### Ecosystem Primitives
Operations that delegate to external package managers. These capture
maximum constraint at eval time but may have residual non-determinism:

- go_build: Builds from Go source, locks dependencies via go.sum
- cargo_build: Builds from Rust source, locks via Cargo.lock
- npm_exec: Runs npm commands with package-lock.json
- pip_install: Installs Python packages with hashed requirements
- gem_exec: Runs bundler with Gemfile.lock
- nix_realize: Uses Nix for fully deterministic builds
- cpan_install: Installs Perl modules with carton snapshot

Plans containing ecosystem primitives are marked with `deterministic: false`
to indicate potential variation from compiler versions or native code builds.
```

#### 2. **Action Decomposition Not Explained to Users**

**Issue:** The decomposition system is a core architectural change but users have no visibility into it.

**What's Missing:**
- No explanation that composite actions decompose to primitives during eval
- No documentation on what actions decompose vs. what actions are primitives
- No visibility into the decomposition process (what happened during eval)

**Where This Matters:**
- When comparing plans (`tsuku eval` output) - users see primitives, not the composite actions from recipes
- When troubleshooting plan generation failures
- When understanding why plans are large/complex

**Recommended Addition to GUIDE-plan-based-installation.md:**
```markdown
## How Plans Are Generated

When you run `tsuku eval`, composite actions from recipes are decomposed
into primitive operations:

- github_archive → download + extract + chmod + install_binaries
- download_archive → download + extract
- hashicorp_release → download + extract + install_binaries

This decomposition ensures plans contain only primitive, deterministic
operations that can be executed reproducibly.
```

#### 3. **Plan Schema Version and Compatibility**

**Issue:** Plans have `format_version: 2` but no documentation explains:
- What changed from version 1
- Compatibility guarantees
- What happens if you try to install a plan with wrong format version

**Where Missing:**
- GUIDE-plan-based-installation.md mentions `format_version` in the example but doesn't explain it
- No migration guide for plans created with older tsuku versions

#### 4. **Lockfile Capture Not Explained**

**Issue:** Multiple issues (#608-613) made ecosystem actions "evaluable via lockfile capture" but this mechanism is not documented.

**What's Missing:**
- No explanation that tsuku captures lockfiles during eval
- No documentation on where lockfiles are stored in plans
- No visibility into what lockfiles were captured
- No guidance on inspecting lockfile content from plans

**Example Missing Content:**
```markdown
## Lockfile Capture

For ecosystem-based installations (npm, cargo, go, etc.), tsuku captures
dependency lockfiles during plan generation:

- npm_install: Captures package-lock.json
- cargo_install: Captures Cargo.lock
- go_install: Captures go.sum
- gem_install: Captures Gemfile.lock
- nix_install: Captures flake.lock

These lockfiles are embedded in the plan and used during execution to
ensure exact dependency versions.

To inspect captured lockfiles:
tsuku plan show mytool --json | jq '.steps[] | select(.action=="npm_install") | .params.lockfile'
```

#### 5. **Dependency Installation in Plans**

**Issue:** Issue #621 "Plans should include dependency installation steps" was closed but no documentation explains this feature.

**What's Missing:**
- No explanation that dependencies are now included in plans
- No documentation on how dependency chains are represented
- No guidance on what happens during plan execution with dependencies

#### 6. **Plan Validation and Errors**

**Issue:** Plans are validated to contain only primitives (#441) but validation errors are not documented.

**What's Missing:**
- No list of plan validation errors and their meanings
- No troubleshooting guide for plan validation failures
- No explanation of what makes a plan "valid"

**Recommended Section for GUIDE-plan-based-installation.md:**
```markdown
## Plan Validation

Plans must meet these requirements:
- Contain only primitive actions (no composite actions)
- Have valid checksums for all downloads
- Match the target platform (os/arch)
- Use supported plan format version

Common validation errors:
- "plan contains non-primitive action X": Recipe failed to decompose properly
- "plan format version unsupported": Plan created with incompatible tsuku version
- "platform mismatch": Plan is for different os/arch than current system
```

#### 7. **Sandbox Testing with Plans**

**Issue:** While README.md mentions `tsuku install --plan - --sandbox`, there's no documentation on:
- Network configuration for sandboxed plan execution
- How sandbox mode handles eval-time dependencies
- Differences between sandboxing eval vs exec phases

#### 8. **Recipe Migration Guide Missing**

**Issue:** The decomposition system changed how recipes work but there's no guide for recipe authors on:
- Which actions are now composite vs. primitive
- How to test recipes with the new decomposition system
- What changed in recipe semantics
- How to verify decomposition works correctly

**Recommended New Document:** `docs/GUIDE-recipe-migration-m15.md`

#### 9. **Performance Implications Not Documented**

**Issue:** Plan caching improves reinstall performance but this is not quantified or explained.

**What's Missing:**
- No explanation of when cached plans are used vs. regenerated
- No performance comparison (eval+exec vs direct install)
- No guidance on when to use --fresh vs. trusting cache

#### 10. **Security Model Not Fully Explained**

**Issue:** README.md mentions checksum verification detects supply chain attacks but doesn't explain:
- Threat model for plan-based installation
- Attack vectors and mitigations
- Trust boundaries (who generates plans vs. who executes them)
- Implications of using pre-generated plans from untrusted sources

**Recommended Addition to GUIDE-plan-based-installation.md:**
```markdown
## Security Considerations

### Plan Trust Model
Plans are trusted inputs. When you install from a plan file, tsuku:
- Trusts the URLs in the plan
- Verifies checksums match what's in the plan
- Does NOT re-validate URLs or checksums against external sources

### Recommendations
- Generate plans yourself using `tsuku eval`
- Verify plan content before execution in production
- Only use plans from trusted sources
- Review plans for unexpected URLs or dependencies

### Checksum Verification
During execution, downloaded files are verified against plan checksums.
This protects against:
- File corruption during download
- Upstream file modifications (re-tagged releases)
- Man-in-the-middle attacks during download

It does NOT protect against:
- Malicious plans with attacker-controlled URLs
- Compromised sources at eval time
```

## Technical Documentation Completeness

### Design Documents
- ✅ DESIGN-deterministic-resolution.md: Comprehensive strategic design
- ✅ DESIGN-decomposable-actions.md: Detailed decomposition architecture
- ✅ DESIGN-plan-based-installation.md: Implementation details
- ✅ DESIGN-installation-plans-eval.md: Eval phase specifics
- ✅ Seven ecosystem-specific design docs (ecosystem_*.md) in docs/deterministic-builds/

**Note:** These are excellent internal/developer docs but are not user-facing.

### Missing Technical Documentation
1. No API reference for plan JSON structure
2. No migration guide from pre-M15 recipes
3. No troubleshooting guide for decomposition failures

## Inconsistencies and Outdated Content

### 1. README.md Claims vs. Reality
**Inconsistency:** README.md line 18 says "Package manager integration: npm_install action for npm tools (pip/cargo pending)"

**Reality:** M15 implemented:
- npm_install (evaluable), npm_exec (primitive)
- cargo_install (evaluable), cargo_build (primitive)
- pip_install (both evaluable and primitive)
- go_install (evaluable), go_build (primitive)
- gem_install (evaluable), gem_exec (primitive)
- nix_install (evaluable), nix_realize (primitive)
- cpan_install (primitive)

**Impact:** Users don't know that pip/cargo/go/gem/nix/cpan are fully supported.

**Fix Required:** Update README.md line 18 to:
```markdown
- **Ecosystem integration**: npm, cargo, go, pip, gem, nix, and cpan package managers with lockfile-based reproducibility
```

### 2. No recipes/ Directory Content
**Issue:** The recipes/ directory is empty except for CLAUDE.local.md. All the ecosystem primitives were implemented but there are no example recipes using them.

**Impact:** Users cannot learn by example how to use new primitives.

**Recommendation:** Add example recipes or clarify that recipes are distributed separately.

## Documentation Quality Assessment

### Strengths
1. **Excellent user-facing guides**: GUIDE-plan-based-installation.md is comprehensive, well-structured, and includes practical examples
2. **Clear CLI help text**: All new commands have helpful descriptions and examples
3. **Good troubleshooting sections**: Common errors are documented with recovery steps
4. **Practical workflows**: Air-gapped and CI distribution patterns are well-explained
5. **Strong design documentation**: Internal architecture is well-documented for developers

### Weaknesses
1. **Ecosystem primitives invisible to users**: Major feature with zero user docs
2. **Action decomposition unexplained**: Core architectural change not surfaced
3. **Determinism guarantees unclear**: Users can't understand what reproducibility they get
4. **No recipe examples**: Can't learn new primitives by example
5. **Security model incomplete**: Trust boundaries not fully explained

## Recommendations

### Priority 1: Critical Gaps (Must Address)
1. **Document ecosystem primitives** in README.md and new guide
2. **Explain deterministic flag** in GUIDE-plan-based-installation.md
3. **Update README.md line 18** to reflect current ecosystem support
4. **Add security model section** to plan-based installation guide

### Priority 2: Important Gaps (Should Address)
5. **Document lockfile capture** mechanism and inspection
6. **Explain action decomposition** in user terms
7. **Add plan validation errors** reference
8. **Document dependency installation** in plans

### Priority 3: Nice to Have
9. **Create recipe migration guide** for M15 changes
10. **Add performance characteristics** documentation
11. **Provide example recipes** using new primitives
12. **Document plan format versioning** and compatibility

## Summary

**Overall Assessment:** Documentation is **partially complete** with significant gaps.

**Strengths:**
- Basic plan-based installation workflow is well-documented
- CLI help text is comprehensive
- Air-gapped and CI workflows are practical and clear

**Critical Gaps:**
- Ecosystem primitives (14 new actions) are completely undocumented for users
- Determinism model and guarantees are not explained
- No examples of new primitives in practice
- README.md has outdated claims about ecosystem support

**Risk:**
- Users cannot understand what reproducibility guarantees they get
- No visibility into new ecosystem primitive capabilities
- Can't learn how to use new features without reading source code

**Recommendation:** Address Priority 1 items before milestone can be considered fully complete. The implementation is excellent, but users need documentation to understand and use the new capabilities.

## Appendix: Documentation File Inventory

### User-Facing Documentation
- ✅ README.md: Overview, installation, basic usage
- ✅ docs/GUIDE-plan-based-installation.md: Plan-based installation workflows
- ✅ docs/GUIDE-recipe-verification.md: Recipe verification (unrelated to M15)
- ✅ CLI help text: install, eval, plan commands

### Design/Technical Documentation
- ✅ docs/DESIGN-deterministic-resolution.md: Strategic design
- ✅ docs/DESIGN-decomposable-actions.md: Decomposition architecture
- ✅ docs/DESIGN-plan-based-installation.md: Implementation details
- ✅ docs/DESIGN-installation-plans-eval.md: Eval phase
- ✅ docs/deterministic-builds/: 7 ecosystem-specific design docs

### Missing Documentation
- ❌ Guide: Understanding Tsuku Actions and Primitives
- ❌ Guide: Recipe Migration for M15
- ❌ Reference: Plan JSON Schema and API
- ❌ Reference: Plan Validation Errors
- ❌ Examples: Recipes using ecosystem primitives
