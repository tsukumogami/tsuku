---
status: Proposed
problem: Tsuku has 155 registry recipes but thousands of developer tools exist across ecosystems (8K+ Homebrew formulas, 200K+ Rust crates, 11M+ npm packages). Manual recipe creation doesn't scale, and missing system dependencies block many formulas.
decision: Adopt fully deterministic batch generation with structured failure analysis. Failures reveal capability gaps that drive manual fixes. LLM builders remain a user feature, not part of automation.
rationale: A deterministic-only pipeline produces consistent, analyzable results. Failures are valuable data that identify which capabilities to build next. Keeping LLM out of automation ensures predictable costs and enables systematic gap analysis.
---

# DESIGN: Registry Scale Strategy

## Status

Proposed

## Context and Problem Statement

Tsuku has successfully separated embedded recipes (17 core bootstrapping tools) from registry recipes (155 installable tools). The registry separation design enables scaling beyond a few hundred recipes, but the current recipe count is far below the potential: Homebrew alone has 8,170 formulas, and popular vendor taps (HashiCorp, MongoDB, AWS) add hundreds more.

Three challenges prevent scaling to thousands of recipes:

1. **Generation is manual**: Each recipe requires running `tsuku create --from <source>` and validating the output. There's no batch generation or CI pipeline.

2. **System dependencies are incomplete**: Many Homebrew formulas depend on libraries (libpng, sqlite, curl) that tsuku doesn't yet provide. When these deps are missing, recipes fail to install.

3. **Quality assurance at scale**: With hundreds of recipes, how do we ensure they work? Sandbox validation helps for LLM-generated recipes, but deterministic recipes skip sandbox testing.

### Why Now

The recipe registry separation (M30-M32) is nearing completion. The infrastructure exists to host thousands of recipes, but without recipes to host, the infrastructure provides no user value. Scaling the registry is the natural next step to make tsuku useful for real-world development.

### Success Criteria

- **Short term**: 500 validated recipes covering the most-requested developer tools
- **Medium term**: 2,000+ recipes across all major ecosystems (Homebrew, crates.io, npm, PyPI)
- **Quality bar**: <1% installation failure rate for validated recipes

### Scope

**In scope:**
- Automated batch generation of recipes from known sources
- Prioritization criteria for which recipes to generate first
- System dependency backfill strategy
- Quality assurance for generated recipes
- Support for popular Homebrew taps (hashicorp, mongodb, etc.)

**Out of scope:**
- User-submitted recipes (community contributions)
- Recipe versioning and upgrade workflows
- Storage and distribution infrastructure (covered by registry separation design)

## Decision Drivers

- **Deterministic generation preferred**: Ecosystem builders (crates.io, npm, pypi, rubygems) are zero-cost and scale linearly; LLM-based generation costs ~$0.10/recipe
- **Quality over quantity**: Broken recipes damage user trust; every generated recipe must be tested
- **Bottle availability**: Homebrew bottle inspection is ~85-90% deterministic; remaining formulas need LLM analysis
- **System dependencies**: Many tools need libraries; adding deps unlocks entire categories of recipes
- **Popular tools first**: Users need common tools (terraform, kubectl, ripgrep) before obscure ones
- **Tap support matters**: Vendor taps (hashicorp/tap, mongodb/brew) contain actively-maintained formulas

## External Research

### Current Builder State

Analysis of existing tsuku builders reveals which are ready for scale and which have gaps:

| Builder | Status | Deterministic | Ready for Scale | Gap |
|---------|--------|---------------|-----------------|-----|
| Cargo (crates.io) | Active | Yes | Yes | None |
| NPM | Active | Yes | Yes | None |
| PyPI | Active | Yes | Yes | None |
| RubyGems | Active | Yes | Yes | None |
| Go (proxy.golang.org) | Active | Yes | Yes | None |
| CPAN (metacpan.org) | Active | Yes | Yes | None |
| Homebrew | Active | Hybrid (85-90% deterministic) | Partial | LLM fallback for ~10-15% |
| Homebrew Cask | Active | Yes | Yes | None |
| GitHub Release | Active | **No (LLM-only)** | **No** | Major: no deterministic path |

**Key findings:**
- 8 builders are fully deterministic and ready for batch generation
- Homebrew is 85-90% deterministic via bottle inspection; LLM fallback handles edge cases
- **GitHub Release builder is LLM-only** - significant gap since many tools distribute via GitHub releases

### Builder Gaps Requiring Tactical Work

1. **GitHub Release Deterministic Path**: The GitHub Release builder currently requires LLM for every generation (~$0.10/recipe). A deterministic path analyzing release asset naming patterns could handle many common cases (tools that follow `{name}-{version}-{os}-{arch}.tar.gz` conventions).

2. **Homebrew Deterministic Success Rate**: Current 85-90% success rate means 10-15% of formulas still need LLM. Improving bottle inspection heuristics could reduce LLM dependency.

### Ecosystem Scale Analysis

| Source | Total Available | CLI-Relevant | Generation Method | Cost/Recipe |
|--------|----------------|--------------|-------------------|-------------|
| Homebrew formulas | 8,170 | ~3,000 | Bottle inspection or LLM | $0-0.10 |
| Homebrew casks | 7,526 | All (macOS apps) | Deterministic API | $0 |
| Homebrew taps | 1,000+ | 500+ | Same as formulas | $0-0.10 |
| Crates.io | 210,000+ | ~10,000 CLI crates | Deterministic API | $0 |
| npm | 11M+ | ~100K CLI tools | Deterministic API | $0 |
| PyPI | 500K+ | ~50K CLI tools | Deterministic API | $0 |
| RubyGems | 180K+ | ~20K CLI tools | Deterministic API | $0 |

### Popular Vendor Taps

Taps with stable, vendor-maintained formulas:

| Tap | Formulas | Notable Tools |
|-----|----------|---------------|
| hashicorp/tap | 20+ | terraform, vault, consul, nomad, packer |
| mongodb/brew | 10+ | mongodb-community, mongosh |
| aws/tap | 5+ | aws-sam-cli, copilot-cli |
| azure/functions | 3+ | azure-functions-core-tools |
| goreleaser/tap | 2+ | goreleaser |
| buildpacks/tap | 2+ | pack |

These taps follow standard formula structure and work with tsuku's Homebrew builder.

### System Dependency Analysis

Current embedded libraries: 17 (go, rust, nodejs, python, ruby, perl, zig, gcc-libs, openssl, zlib, etc.)

Frequently-required system deps not yet available:
- **Compression**: bzip2, xz, lz4, zstd
- **Graphics**: libpng, libjpeg, freetype, fontconfig
- **Data**: sqlite, libxml2, libxslt
- **Network**: curl, libssh2, nghttp2
- **Crypto**: libsodium, gnutls

Adding ~20 core libraries would unblock hundreds of Homebrew formulas.

## Considered Options

### Decision 1: Generation Strategy

How should we generate recipes at scale?

#### Option 1A: Manual Batch Generation

Run `tsuku create` for prioritized lists of packages, reviewing output manually.

**Pros:**
- Human review catches edge cases
- No new tooling required
- Can start immediately

**Cons:**
- Doesn't scale beyond hundreds
- Developer time is the bottleneck
- Inconsistent quality depends on reviewer attention

#### Option 1B: CI Pipeline with Validation Gates

Automated pipeline that generates recipes, validates them, and creates PRs for review.

**Pros:**
- Scales to thousands of recipes
- Consistent validation criteria
- Parallel generation across ecosystems

**Cons:**
- Significant tooling investment
- Need to handle flaky validation
- LLM costs accumulate at scale

#### Option 1C: Hybrid: Deterministic Auto-Merge, LLM Human Review

Auto-merge deterministic recipes (crates.io, npm, pypi) after validation passes; require human review for LLM-generated recipes.

**Pros:**
- Balances scale with quality
- Focuses human attention where needed
- Zero-cost scaling for ecosystems

**Cons:**
- Two different code paths
- Need clear criteria for "deterministic"
- Deterministic recipes may still select wrong executables or miss platform-specific issues

#### Option 1D: Event-Driven Generation

Generate recipes on-demand when users request missing tools via `tsuku install <unknown-tool>`.

**Pros:**
- Only generates what users actually need
- No upfront cost for unused recipes
- Builds library organically from real demand

**Cons:**
- First user pays the wait cost (LLM generation takes seconds)
- No curated experience for new users
- Harder to market ("we have N recipes" vs "we can generate any recipe")

### Decision 2: Prioritization Strategy

Which recipes should we generate first?

#### Option 2A: Popularity-Based (Downloads/Stars)

Generate recipes for the most-downloaded or starred packages first.

**Pros:**
- Maximum user value per recipe
- Clear, objective criteria
- Data available from ecosystem APIs

**Cons:**
- Popular tools may be complex (more deps)
- Metrics favor established tools over rising ones
- Some popular tools already have recipes

#### Option 2B: Dependency-Driven

Generate dependency libraries first, then tools that need them.

**Pros:**
- Unblocks entire categories at once
- Topological ordering ensures deps exist
- Reduces failed installations

**Cons:**
- Users don't directly benefit from libs
- Slower initial visible progress
- Dependency extraction infrastructure is incomplete (see #644)

#### Option 2C: Ecosystem Sweep

Complete coverage of one ecosystem before moving to next.

**Pros:**
- Clear progress metric
- Simplifies tooling (one builder at a time)
- Ecosystem-specific issues addressed together

**Cons:**
- Delays high-value tools in other ecosystems
- Some ecosystems have diminishing returns
- Ignores cross-ecosystem popularity

### Decision 3: System Dependency Handling

How do we handle recipes that need system libraries?

#### Option 3A: Library Recipes First

Create tsuku recipes for common system libraries before generating dependent tools.

**Pros:**
- Full tsuku control over libraries
- Works on any platform
- No system package manager needed

**Cons:**
- Significant work to create lib recipes
- Some libs are complex to build
- Duplicates distro work

#### Option 3B: Skip Tools Requiring System Dependencies

Don't generate recipes for tools that require system libraries tsuku doesn't provide. Focus on self-contained tools first.

**Pros:**
- No dependency complexity
- Faster initial coverage of ecosystem
- All recipes work on all platforms

**Cons:**
- Excludes many popular tools (e.g., imagemagick, ffmpeg)
- May frustrate users expecting comprehensive coverage
- Delays value for users who need those tools

#### Option 3C: Hybrid: Prefer Tsuku, Fallback to System

Try tsuku-provided libs first; fall back to system packages if unavailable.

**Pros:**
- Graceful degradation
- Works today with existing deps
- Gradual migration path

**Cons:**
- Complex resolution logic
- Behavior varies by platform
- Hard to test all paths

### Uncertainties

- **LLM reliability at scale**: Will Claude/Gemini maintain quality for 1000+ sequential generations?
- **Bottle inspection coverage**: ~85-90% of formulas have usable bottle metadata; the rest need LLM fallback
- **Library complexity**: Some libs (fontconfig, mesa) may be too complex for tsuku
- **User demand signal**: Which tools do users actually want vs which are "popular"?
- **API rate limits**: Ecosystem APIs (crates.io, npm) have rate limits that may throttle batch generation
- **Version drift**: Recipes generated today may break as upstream tools update

## Decision Outcome

**Chosen: Deterministic-Only Pipeline + 2A (Popularity-Based) + 3B (Skip System Deps Initially)**

### Summary

Adopt a fully deterministic batch generation pipeline. All failures are recorded with structured data for analysis. LLM builders remain a user-facing feature for manual recipe creation, not part of automation. Prioritize by popularity to maximize user value per recipe. Initially skip tools requiring system libraries tsuku doesn't provide, building out library recipes as a separate workstream.

### Rationale

**Generation Strategy (Deterministic-Only)**: The automated pipeline uses only deterministic builders. When a tool can't be generated deterministically, it's recorded as a failure with structured metadata. This produces:
- Predictable costs ($0 for the pipeline itself)
- Consistent, analyzable results
- Clear signal about which capabilities to build next

LLM builders (GitHub Release, Homebrew fallback) remain available for users running `tsuku create` interactively, but are excluded from batch automation.

**Prioritization (2A)**: Popularity-based ordering aligns with "popular tools first" and "quality over quantity" drivers. Users get terraform, kubectl, and ripgrep before obscure tools. Popularity data is readily available from ecosystem APIs without building complex dependency analysis infrastructure.

**System Dependencies (3B)**: Skipping tools requiring missing system libs initially is pragmatic. It avoids the complexity of Option 3C while not blocking progress. The library backfill becomes a parallel workstream that gradually expands coverage. This aligns with "quality over quantity" since recipes that install will work reliably.

### Alternatives Rejected

- **1A (Manual)**: Doesn't scale to thousands; developer time is finite
- **1B (Full CI with LLM)**: LLM costs are unpredictable at scale; failures become expensive instead of informative
- **1C (Hybrid with LLM fallback)**: Mixing LLM into automation obscures capability gaps; we want failures to be data, not fallback triggers
- **1D (Event-Driven)**: Delays curated experience; harder to market
- **2B (Dependency-Driven)**: Requires dependency infrastructure that doesn't exist (#644)
- **2C (Ecosystem Sweep)**: Ignores cross-ecosystem popularity; delays high-value tools
- **3A (Library First)**: Blocks recipe generation on library work; slows initial progress
- **3C (Hybrid Fallback)**: Complex resolution logic; inconsistent user experience

### Trade-offs Accepted

By choosing this approach, we accept:
- **Lower initial coverage**: Tools that fail deterministic generation won't be in the registry automatically
- **Manual work for complex tools**: GitHub releases, complex Homebrew formulas require manual `tsuku create`
- **Popularity bias**: Rising tools may be underrepresented vs established ones

These are acceptable because:
- Failures produce actionable data (capability gap reports) instead of LLM costs
- Manual creation with LLM remains available as a user feature
- The pipeline becomes a forcing function to improve deterministic builders
- Popularity metrics can be augmented with user requests over time

## Solution Architecture

### Overview

The registry scale strategy is a **fully deterministic pipeline** that generates recipes and records failures for analysis. LLM builders are excluded from automation.

**Automated pipeline workstreams:**

1. **Ecosystem Recipe Generation**: Deterministic builders (Cargo, NPM, PyPI, RubyGems, Go, CPAN, Cask) auto-generate and validate recipes, merging automatically on success
2. **Homebrew Recipe Generation**: Bottle inspection only (deterministic path). Formulas that fail bottle inspection are recorded as failures, not sent to LLM
3. **Failure Collection**: All failures recorded with structured metadata for gap analysis
4. **Library Backfill**: Separate workstream adding tsuku recipes for common system libraries

**Not part of automation:**
- GitHub Release builder (LLM-only) - users can run `tsuku create --from github:org/repo` manually
- Homebrew LLM fallback - users can run `tsuku create --from homebrew:formula` manually for complex formulas

### Components

```
┌──────────────────────────────────────────────────────────────────────────┐
│                   Automated CI Pipeline (Deterministic Only)              │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  ┌────────────────────────────────────┐  ┌─────────────────────────────┐ │
│  │       Deterministic Builders        │  │     Homebrew (bottle only)  │ │
│  │                                     │  │                             │ │
│  │  - Cargo ✓    - Go ✓               │  │  Bottle inspection only     │ │
│  │  - NPM ✓      - CPAN ✓             │  │  (~85-90% of formulas)      │ │
│  │  - PyPI ✓     - Cask ✓             │  │                             │ │
│  │  - RubyGems ✓                      │  │  Failures → analysis        │ │
│  └──────────────┬─────────────────────┘  └──────────────┬──────────────┘ │
│                 │                                       │                 │
│                 ▼                                       ▼                 │
│  ┌───────────────────────────────────────────────────────────────┐       │
│  │                    Validation Gates                            │       │
│  │  - Recipe schema validation                                    │       │
│  │  - Sandbox install test                                        │       │
│  │  - Binary execution check                                      │       │
│  └───────────────────────────────────────────────────────────────┘       │
│                 │                                       │                 │
│          ┌──────┴──────┐                         ┌──────┴──────┐         │
│          ▼             ▼                         ▼             ▼         │
│    ┌──────────┐  ┌──────────┐              ┌──────────┐  ┌──────────┐    │
│    │ Success  │  │ Failure  │              │ Success  │  │ Failure  │    │
│    │ → Merge  │  │ → Record │              │ → Merge  │  │ → Record │    │
│    └──────────┘  └──────────┘              └──────────┘  └──────────┘    │
│                       │                                       │          │
│                       └───────────────┬───────────────────────┘          │
│                                       ▼                                  │
│                          ┌─────────────────────┐                         │
│                          │   Failure Analysis  │                         │
│                          │   (JSONL reports)   │                         │
│                          └─────────────────────┘                         │
└──────────────────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────────────────┐
│                    Manual/Interactive (User Feature)                      │
├──────────────────────────────────────────────────────────────────────────┤
│  Users can run these manually to create recipes for tools that failed:   │
│                                                                           │
│  - tsuku create --from github:org/repo     (LLM-based)                   │
│  - tsuku create --from homebrew:formula    (LLM fallback for failures)   │
└──────────────────────────────────────────────────────────────────────────┘
```

### Priority Queue

The pipeline maintains a priority queue of packages to generate, ordered by:

1. **Popularity score**: Downloads/stars normalized across ecosystems
2. **Dependency availability**: Penalize tools requiring unavailable system libs
3. **Request count**: User requests via telemetry or issues

### Failure Analysis System

The batch pipeline runs in **deterministic-only mode** (no LLM fallback). Failures are valuable data that reveal capability gaps.

**Design principles:**
- Failures are a forcing function, not errors to suppress
- Collect structured data for systematic analysis
- Prioritize capabilities by "popularity-weighted impact" (what capability would unblock the most popular tools?)

**Failure categories mapped to capabilities:**

| Category | Capability Needed | Example |
|----------|-------------------|---------|
| `no_bottles` | Homebrew source fallback | formulas without bottles |
| `build_from_source` | Source build action | tools requiring compilation |
| `no_platform_assets` | Platform matrix expansion | tools missing Linux/macOS builds |
| `missing_dependency` | System library recipes | tools needing libpng, sqlite |
| `binary_not_found` | Improved executable discovery | non-standard binary locations |
| `complex_archive` | Advanced archive inspection | nested or unusual structures |

**Storage:** JSONL files per batch run, enabling:
- `tsuku batch analyze` to generate capability gap reports
- Feedback loop to re-prioritize the queue (skip structural failures, retry transient ones)
- Historical tracking of gap closure over time

**Required flag:** Add `--deterministic-only` to `tsuku create` to disable LLM fallback. This enables fully deterministic batch runs where failures produce analyzable data rather than triggering expensive LLM calls.

### Generation Flow

```
Priority Queue → Select Package → Route by Source
                                        │
              ┌─────────────────────────┴─────────────────────────┐
              ▼                                                   ▼
      Ecosystem Builder                                   Homebrew Builder
      (cargo, npm, pypi,                                  (bottle inspection
       rubygems, go, cpan, cask)                           ONLY - no LLM)
              │                                                   │
              ▼                                                   ▼
      Deterministic Recipe                                Deterministic Recipe
              │                                            OR Failure Record
              ▼                                                   │
      Validation Gates ◄──────────────────────────────────────────┘
              │
       ┌──────┴──────┐
       ▼             ▼
   Success       Failure
   → Merge       → Record
                     │
                     ▼
              Failure Analysis
              (capability gaps)
                     │
                     ▼
              Manual fixes using
              LLM builders (user feature)
```

**Note**: GitHub Release packages are NOT in the automated queue. Users create these manually via `tsuku create --from github:org/repo`.

## Implementation Approach

This is a strategic design. Implementation details are delegated to tactical designs.

### Phase 0: Prerequisites

Add the `--deterministic-only` flag to `tsuku create` to enable batch mode:

1. **DESIGN-deterministic-only-flag.md**: Flag disables LLM fallback, ensuring failures are recorded instead of triggering LLM

This is required before batch generation can run.

### Phase 1: Batch Generation Infrastructure

1. **DESIGN-priority-queue.md**: Define scoring algorithm and data sources
2. **DESIGN-batch-recipe-generation.md**: Build the CI pipeline

### Phase 2: Parallel Workstreams

1. **DESIGN-system-lib-backfill.md**: Add library recipes to unblock more tools
2. **Homebrew deterministic improvements**: Increase bottle inspection success rate

### Milestones

- **M-DeterministicFlag**: Add `--deterministic-only` flag to `tsuku create`
- **M-Priority**: Priority queue implementation (scoring, data ingestion, API)
- **M-BatchGen**: CI pipeline for batch generation (scheduler, validation, PR creation)
- **M-FailureAnalysis**: Failure collection and gap analysis tooling
- **M-LibBackfill**: First 20 library recipes added (compression, data, crypto categories)

## Required Tactical Designs

### Builder Configuration

| Design | Target Repo | Purpose |
|--------|-------------|---------|
| DESIGN-deterministic-only-flag.md | tsuku | Add `--deterministic-only` flag to disable LLM fallback for batch runs |

**Note**: GitHub Release deterministic path is a nice-to-have, not a prerequisite. GitHub releases are handled manually via user-facing LLM builder.

### Failure Analysis Infrastructure

| Design | Target Repo | Purpose |
|--------|-------------|---------|
| DESIGN-batch-failure-analysis.md | tsuku | Structured failure collection and capability gap analysis |

### Batch Generation Infrastructure

| Design | Target Repo | Purpose |
|--------|-------------|---------|
| DESIGN-batch-recipe-generation.md | tsuku | CI pipeline for automated recipe generation |
| DESIGN-priority-queue.md | tsuku | Criteria and data sources for prioritization |
| DESIGN-system-lib-backfill.md | tsuku | Strategy for adding common library recipes |

## Security Considerations

### Download Verification

Recipes generated from Homebrew bottles inherit bottle checksums verified by Homebrew's CI. Ecosystem-generated recipes (crates.io, npm, pypi) include checksums from those registries' APIs. All generated recipes go through tsuku's existing checksum verification at install time.

**Not changing**: This design doesn't alter tsuku's download verification - it uses existing mechanisms.

### Execution Isolation

Batch generation runs in GitHub Actions CI. Generated recipes are validated in sandboxed containers before merge. The CI environment is ephemeral and doesn't have access to production systems or user data.

**Risk**: Malicious recipe could escape sandbox during validation.

**Mitigation**: Sandboxes run in isolated containers with no network access during binary execution tests. Sandbox escape is a defense-in-depth layer; primary protection is upstream ecosystem integrity.

### Supply Chain Risks

**Risk 1**: Compromised upstream package auto-generated as tsuku recipe.

**Mitigation**: Only generate from established ecosystems (Homebrew, crates.io, npm, PyPI, RubyGems) that have their own supply chain protections. These ecosystems have malware scanning, maintainer verification, and incident response processes.

**Risk 2**: Auto-merge introduces vulnerable recipe without human oversight.

**Mitigation**: Auto-merge only applies to deterministic ecosystem recipes where tsuku just packages upstream artifacts. The vulnerability would exist in the upstream ecosystem regardless of tsuku. LLM-based recipe creation is a manual user action, not part of automated pipeline.

### User Data Exposure

**Not applicable**: Batch generation runs in CI with no access to user data. Generated recipes don't contain user-specific information. Telemetry for tool request signals is opt-in and anonymized.

### Additional Risks Identified During Review

**Risk 3**: Typosquatted packages in ecosystems (npm, PyPI, crates.io).

**Mitigation**: Tactical design should include popularity/age gates - require human review for packages with <1000 downloads or <90 days old.

**Risk 4**: `run_command` actions execute arbitrary shell commands at install time with user privileges.

**Mitigation**: This is an existing tsuku risk, not specific to batch generation. Tactical design should consider a 24-72 hour cooldown before auto-merge to allow community detection of malicious recipes.

### Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Compromised upstream package | Rely on ecosystem's own protections | Ecosystem-level compromise bypasses all defenses |
| Sandbox escape | Container isolation, no network | Container runtime vulnerability |
| Recipe enables privilege escalation | Sandbox validation catches obvious cases | Sophisticated attacks may pass validation |
| Typosquatted packages | Popularity/age gates (tactical design) | New popular packages could still be malicious |
| `run_command` abuse | Cooldown period before auto-merge | Sophisticated time-delayed attacks |

**Note**: LLM-related risks (prompt injection, malicious generated recipes) are not applicable to the automated pipeline since LLM is excluded from automation.

## Consequences

### Positive

- **Scale**: Infrastructure to reach 5,000+ recipes
- **User value**: Common developer tools become available
- **Ecosystem coverage**: Multiple language ecosystems covered
- **Forcing function**: Identifies rough edges in generation/validation

### Negative

- **Tooling investment**: CI pipeline and monitoring infrastructure needed
- **Quality risk**: Automated generation may miss edge cases
- **Dependency complexity**: System libs add maintenance burden

### Neutral

- **Cost**: LLM generation costs are bounded by prioritization
- **Time**: Generating 5,000 recipes takes weeks, not days
