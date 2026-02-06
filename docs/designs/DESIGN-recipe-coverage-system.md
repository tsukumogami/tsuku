---
status: Proposed
problem: |
  M47 delivered platform compatibility infrastructure (libc detection, recipe conditionals, coverage analysis) but 0 of 13 library recipes were migrated to support musl/Alpine. The gap wasn't visible until milestone validation ran. We lack systematic monitoring to surface coverage gaps early, no multi-angle visibility into which recipes support which platforms, and no automated enforcement preventing regression. Users on Alpine encounter failures, recipe authors don't know if contributions break platform support, and milestones can be "complete" while features remain non-functional.
decision: |
  Build three interconnected subsystems: (1) Monitoring via internal tooling (cmd/coverage-analytics) and metrics generation across dimensions (libc, architecture, OS, category), (2) Visualization through static website at tsuku.dev/coverage with matrix views and recipe detail pages, and (3) Enforcement via CI checks that block library recipes lacking musl support with automated PR feedback and opt-out mechanism. Includes systematic workflow to migrate 13 M47 library recipes in batches, using system packages (apk) for Alpine support. Follows the same architecture pattern as the existing pipeline dashboard (internal/dashboard, cmd/queue-analytics, website/pipeline).
rationale: |
  This is the only approach that addresses all needs: closes M47 gap immediately (Phase 1 recipe migrations), prevents future gaps (CI enforcement), and provides visibility (dashboard). Alternatives considered: CI-only (no visibility), dashboard-only (no enforcement), external tools (don't understand tsuku's libc conditionals). The comprehensive system scales to additional dimensions and provides both immediate value and long-term infrastructure. Phased implementation delivers value incrementally while managing complexity.
---

# Recipe Coverage Monitoring, Visualization, and Gap-Closing System

## Status

**Proposed**

**Related Designs:**
- [DESIGN-platform-compatibility-verification.md](./current/DESIGN-platform-compatibility-verification.md) - M47: Infrastructure for platform compatibility
- [DESIGN-pipeline-dashboard.md](./current/DESIGN-pipeline-dashboard.md) - Existing dashboard for batch generation monitoring

**Related Milestones:**
- Milestone 47: Platform Compatibility Verification (infrastructure delivered, recipes not migrated)

## Context and Problem Statement

### The M47 Gap

Milestone 47 "Platform Compatibility Verification" delivered infrastructure for Alpine/musl support:
- Libc detection system (glibc vs musl)
- Recipe conditional syntax (`when = { libc = ["glibc"] }`)
- Step-level dependency declarations
- Coverage analysis code (`internal/recipe/coverage.go`)
- Documentation for hybrid libc approach

However, **zero of 13 library recipes** were migrated. Despite all tooling being ready, Alpine/musl support remains non-functional for embedded libraries (libcurl, openssl, zlib, brotli, ncurses, readline, libyaml, sqlite, libffi, gmp, libxml2, libxslt, libiconv).

This gap wasn't visible until milestone validation ran.

### The Broader Problem

Beyond M47, we lack visibility into recipe coverage across dimensions:

**Coverage Dimensions:**
1. Linux family: glibc-based (Debian, Ubuntu, Fedora) vs musl-based (Alpine)
2. Architecture: x86_64, aarch64, arm, i686
3. Operating system: Linux, macOS, BSD variants
4. Recipe category: Libraries (embedded) vs CLI tools vs language runtimes

**Current State:**
- 265 recipes in registry
- 13 explicitly typed as libraries (4.9%)
- Coverage analysis code exists but isn't integrated into PR workflows
- No real-time visibility into gaps
- No automated enforcement preventing regression

**Consequences:**
- Milestones can be "complete" while features remain non-functional
- Users on Alpine encounter unexpected failures
- Recipe authors don't know if contributions break platform support
- Can't prioritize gap-closing work without visibility

## Decision Drivers

1. **Prevent invisible gaps** - Coverage issues must be visible before milestone closure
2. **Guide recipe authors** - Clear feedback on what platforms their recipe supports
3. **Close existing gaps** - Systematic workflow for migrating M47's 13 library recipes
4. **Prevent regression** - Automated checks ensuring library recipes support musl
5. **Multi-angle visibility** - Slice coverage data by architecture, OS family, recipe type
6. **Actionable insights** - Don't just show gaps, explain blockers and provide guidance
7. **Public transparency** - External users should see what platforms tsuku supports

## Considered Options

### Decision 1: System Scope and Architecture

The core question is how much to build: monitoring-only, visualization-only, or a comprehensive system that does both plus enforcement?

#### Chosen: Comprehensive Three-Subsystem Approach

Build three interconnected subsystems:

1. **Monitoring Subsystem**
   - Internal package (`internal/coverage/`) with analysis logic
   - CLI tool (`cmd/coverage-analytics/`) generating coverage metrics JSON
   - Metrics generator analyzing all 265 recipes across dimensions
   - Historical snapshots for tracking trends
   - Coverage analysis algorithm detecting gaps, classifying blockers, generating recommendations

2. **Visualization Subsystem**
   - Static website at tsuku.dev/coverage with 5 views: overview matrix (recipes × platforms), category breakdown, platform breakdown, gap details, trends over time
   - Recipe detail pages showing per-recipe platform support
   - Static HTML/JS loading coverage.json (matches pipeline dashboard architecture)
   - Public transparency through deployed website

3. **Enforcement Subsystem**
   - CI GitHub Actions workflow using internal/coverage validation
   - Blocks PRs if library recipes lack musl support
   - Automated PR comments with gap analysis and fix recommendations
   - Opt-out mechanism with documented reasons for legitimate blockers
   - Validation rules: Libraries MUST support musl, CLI tools SHOULD

The subsystems work together: monitoring generates data, visualization renders it, enforcement prevents regression. Implementation happens in 3 phases to deliver value incrementally.

#### Alternatives Considered

**CI Check Only**: Add a single CI check that fails if library recipes lack musl support.
Rejected because it provides no visibility into current gaps, no guidance for fixing issues, and doesn't help close M47 gap. Binary pass/fail with no nuance about blockers.

**Dashboard Expansion Only**: Extend existing pipeline dashboard with coverage monitoring tab.
Rejected because it provides visibility but no enforcement. Gaps remain visible but nothing prevents new ones. Doesn't integrate with developer workflow (no PR feedback).

**External Tool Integration**: Use existing tools like GitHub's dependency graph or third-party platforms.
Rejected because recipe coverage is tsuku-specific. External tools don't understand libc conditionals or platform matching logic. Limited customization and vendor dependency.

### Decision 2: Coverage Dimensions to Track

What dimensions should the system monitor? This shapes the data model and determines what gaps we can detect.

#### Chosen: Four Orthogonal Dimensions

Track coverage across these dimensions:

| Dimension | Values | Source | Notes |
|-----------|--------|--------|-------|
| **libc Family** | glibc, musl | Recipe `when` clauses | Primary M47 dimension |
| **Architecture** | x86_64, aarch64, arm, i686 | Recipe `when` clauses | Hardware platform |
| **OS Family** | linux, darwin, bsd | Recipe `when` clauses | Operating system |
| **Recipe Category** | library, cli, runtime | Recipe `[metadata]` | User-facing vs embedded |

The system evaluates recipe `when` clauses for all platform combinations, identifies which platforms have installation paths, detects gaps (library recipes without musl support), classifies blockers (upstream limitations vs tsuku recipe gaps), and generates actionable recommendations.

#### Alternatives Considered

**Libc Only**: Track only glibc vs musl, ignore architecture and OS.
Rejected because it's too narrow. Users need ARM64 support, macOS support, etc. M47 focused on libc but future work needs broader visibility.

**Add Linux Distro as Dimension**: Track Debian vs Ubuntu vs Alpine vs Fedora.
Rejected because tsuku recipes don't use distro-specific conditionals currently. Libc family (glibc/musl) is more fundamental. Can add later if needed.

**Include Installation Method**: Track whether recipes use homebrew vs system packages vs source builds.
Kept as derived property, not a first-class dimension. Installation method is a consequence of platform choice, not an independent axis.

### Decision 3: Enforcement Level

How strict should CI validation be? Block all gaps, warn on gaps, or only enforce for certain recipe categories?

#### Chosen: Category-Based Enforcement

Validation rules:
1. **Strict (blocking)**: Library recipes MUST support both glibc and musl
2. **Warning (non-blocking)**: CLI tools SHOULD support both glibc and musl
3. **Opt-out allowed**: Recipes can declare coverage exclusions with documented reason

Libraries are strictly enforced because they're embedded in other tools. If libcurl lacks musl support, any tool depending on it also lacks musl support. CLI tools get warnings because upstream limitations are more common (binary availability varies).

Opt-out mechanism requires explicit `[coverage.exclusions]` section with `reason` field and optional `issue_url`. Code review validates that reasons are substantive (not just "doesn't work"). Maintainers can reject insufficient justifications.

#### Alternatives Considered

**Block Everything**: All recipes must support all platforms or fail CI.
Rejected because some tools genuinely can't support certain platforms (upstream doesn't provide binaries, technical limitations). Would create friction and false positives.

**Warning Only**: CI warns about gaps but doesn't block.
Rejected because warnings get ignored. M47 gap shows that visibility without enforcement isn't enough. Library recipes need strict enforcement.

**Manual Approval**: Gaps require maintainer approval via PR comment.
Rejected because it doesn't scale. Maintainers become bottleneck. Better to require explicit opt-out with documented reason.

### Decision 4: M47 Gap Closure Approach

How should we migrate the 13 library recipes? All at once, one by one, or in batches?

#### Chosen: Batch Migration with Priority Ordering

Migrate in 4 batches by priority:

**Batch 1 (critical)**: openssl, zlib, libcurl - used by almost everything
**Batch 2 (common)**: ncurses, readline, sqlite - terminal UI and databases
**Batch 3 (remaining)**: brotli, libyaml, libffi, gmp, libxml2, libxslt
**Batch 4 (special)**: libiconv - musl has built-in iconv, document this

Process per batch: Create feature branch, migrate using template, test on both glibc (Ubuntu) and musl (Alpine), run coverage validation, create PR, merge after CI passes.

Migration template:
```toml
[[step]]
action = "install_homebrew_package"
package = "libcurl"
when = { libc = ["glibc"] }

[[step]]
action = "install_system_package"
apk = ["curl-dev"]
when = { libc = ["musl"] }
```

All 12 recipes (excluding libiconv) map cleanly to Alpine packages: curl-dev, openssl-dev, zlib-dev, brotli-dev, ncurses-dev, readline-dev, yaml-dev, sqlite-dev, libffi-dev, gmp-dev, libxml2-dev, libxslt-dev.

#### Alternatives Considered

**All at Once**: Single PR migrating all 13 recipes.
Rejected because it's high-risk. If one recipe fails, entire PR blocks. Harder to review. Harder to roll back if issues found.

**One by One**: 13 separate PRs.
Rejected because it's tedious. Creates noise (13 PRs for nearly identical changes). Delays closing M47. Batch approach balances risk and efficiency.

**No Priority**: Migrate in alphabetical or random order.
Rejected because some libraries are more critical. Openssl and zlib are used by many tools. Get high-value migrations done first.

## Decision Outcome

**Chosen: 1 + 2 + 3 + 4** (Comprehensive system + Four dimensions + Category-based enforcement + Batch migration)

### Summary

We're building a complete recipe coverage system with three subsystems working together. Monitoring analyzes all recipes across four dimensions (libc, architecture, OS, category) and generates metrics. Visualization renders multi-angle dashboards showing gaps by category, platform, and recipe. Enforcement blocks PRs that introduce library gaps while allowing opt-outs with documented reasons.

The system addresses the M47 gap directly: Phase 1 migrates 13 library recipes in 4 priority batches using a standard template (homebrew for glibc, system packages for musl). Internal coverage-analytics tool generates metrics and validates recipes. CI checks block library recipes lacking musl support and post automated comments with fix guidance. Static website at tsuku.dev/coverage shows real-time coverage across all dimensions with drill-down to recipe details.

Coverage dimensions (libc, arch, OS, category) are orthogonal, enabling multi-angle analysis. Library recipes get strict enforcement (MUST support musl) while CLI tools get warnings (SHOULD support musl). Opt-out mechanism allows exceptions for legitimate blockers with substantive reasons.

Implementation happens in 3 phases: Foundation (CLI tools, recipe migrations, M47 closure), Visibility (dashboard, metrics, trends), Enforcement (CI automation, PR feedback). Each phase delivers value independently while building toward the complete system.

### Rationale

The comprehensive system is the only option that addresses all decision drivers: closes M47 gap (recipe migrations), prevents regression (CI enforcement), provides visibility (dashboard), guides contributors (PR comments), and scales to new dimensions.

Alternatives were too narrow: CI-only gives enforcement without visibility, dashboard-only gives visibility without prevention, external tools don't understand tsuku's recipe model. Four dimensions balance current needs (M47 libc focus) with future work (architecture, OS support).

Category-based enforcement (strict for libraries, warnings for tools) reflects reality: libraries are foundational (gaps cascade), tools face genuine upstream constraints. Opt-out prevents false positives while maintaining standards through code review.

Batch migration balances risk and speed: critical libraries first (openssl, zlib, libcurl), then common dependencies, then remainder. One-by-one is too slow, all-at-once is too risky. Batches allow validation between groups.

The phased approach manages complexity: Phase 1 closes M47 with minimal tooling, Phase 2 adds visibility, Phase 3 adds automation. Each phase standalone-valuable while building toward comprehensive system.

## Solution Architecture

### System Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                   Recipe Coverage System                         │
└─────────────────────────────────────────────────────────────────┘
           │                    │                    │
           ▼                    ▼                    ▼
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│   Monitoring     │  │  Visualization   │  │   Enforcement    │
│    Subsystem     │  │    Subsystem     │  │    Subsystem     │
│                  │  │                  │  │                  │
│ • Coverage       │  │ • Dashboard      │  │ • CI checks      │
│   analyzer       │  │ • Matrix views   │  │ • PR comments    │
│ • Metrics        │  │ • Drill-down     │  │ • Blocking gates │
│   collection     │  │ • CLI tool       │  │ • Opt-out        │
│ • Dimension      │  │ • Website        │  │   validation     │
│   tracking       │  │   integration    │  │                  │
└──────────────────┘  └──────────────────┘  └──────────────────┘
           │                    │                    │
           └────────────────────┴────────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │  Coverage Data   │
                    │    Repository    │
                    │                  │
                    │ • Recipe         │
                    │   metadata       │
                    │ • Platform       │
                    │   support matrix │
                    │ • Historical     │
                    │   trends         │
                    └──────────────────┘
```

**Component Interaction:**
1. Monitoring analyzes recipes and publishes metrics to data repository
2. Visualization queries data repository and renders multi-angle views
3. Enforcement blocks PRs introducing gaps, queries monitoring for validation
4. Data repository stores historical snapshots for trend analysis

### Coverage Analysis Algorithm

```go
type CoverageReport struct {
    RecipeName     string
    Category       RecipeCategory // library, cli, runtime

    // Platform support matrix
    SupportedPlatforms map[Platform]SupportStatus

    // Dimension-specific coverage
    LibcCoverage      map[LibcFamily]bool    // glibc, musl
    ArchCoverage      map[Architecture]bool  // x86_64, aarch64, etc.
    OSCoverage        map[OS]bool            // linux, darwin, etc.

    // Gap analysis
    MissingSupport    []Platform
    Blockers          []Blocker

    // Recommendations
    Actions           []string
}

type Blocker struct {
    Platform    Platform
    Reason      string  // e.g., "Upstream binary not available for musl"
    CanFix      bool    // true if tsuku can address, false if external
    Workaround  string  // Alternative approach if CanFix is true
}
```

Algorithm:
1. Parse recipe TOML
2. Evaluate `when` clauses for all platform combinations
3. Identify which platforms have installation paths
4. Detect gaps (library recipes without musl support)
5. Classify blockers (upstream limitations vs tsuku recipe gaps)
6. Generate actionable recommendations

### Data Model

**Recipe Metadata (Enhanced):**

```toml
[metadata]
name = "libcurl"
category = "library"  # NEW
description = "Command line tool and library for transferring data with URLs"

# NEW: Coverage declarations
[coverage]
required_platforms = ["linux-x86_64-glibc", "linux-x86_64-musl"]

# NEW: Explicit opt-out for specific platforms
[[coverage.exclusions]]
platform = "linux-x86_64-musl"
reason = "Upstream Homebrew does not provide musl binaries"
issue_url = "https://github.com/tsukumogami/tsuku/issues/1234"

[[step]]
action = "install_homebrew_package"
package = "libcurl"
when = { libc = ["glibc"] }

[[step]]
action = "install_system_package"
apk = ["curl-dev"]
when = { libc = ["musl"] }
```

**Coverage Metrics (New):**

```json
{
  "generated_at": "2026-02-06T12:00:00Z",
  "total_recipes": 265,
  "by_category": {
    "library": {"total": 13, "musl_support": 0, "glibc_support": 13, "coverage_pct": 0.0},
    "cli": {"total": 180, "musl_support": 150, "glibc_support": 180, "coverage_pct": 83.3}
  },
  "by_platform": {
    "linux-x86_64-glibc": {"supported": 265, "total": 265, "pct": 100.0},
    "linux-x86_64-musl": {"supported": 215, "total": 265, "pct": 81.1}
  },
  "gaps": [{
    "recipe": "libcurl",
    "category": "library",
    "missing_platforms": ["linux-x86_64-musl"],
    "blocker": "Recipe uses Homebrew which only provides glibc binaries",
    "can_fix": true,
    "recommendation": "Add system package step with 'when = { libc = [\"musl\"] }'"
  }]
}
```

## Implementation Approach

### Phase 1: Foundation (2-3 weeks)

**Goal:** Close M47 gap with minimal tooling.

**Components:**
1. Coverage analysis package (`internal/coverage/`)
2. Coverage analytics tool (`cmd/coverage-analytics/`)
3. Recipe metadata schema (`[coverage]` section with exclusions)
4. Migrate 13 library recipes in 4 batches
5. Contribution guide

**Tool Usage:**
```bash
# Generate coverage metrics (run by CI or manually)
coverage-analytics --recipes recipes/ --output website/coverage/coverage.json

# CI validation (called from GitHub Actions)
coverage-analytics --validate --strict --recipes recipes/
```

**Recipe Migration:**
- Batch 1: openssl, zlib, libcurl
- Batch 2: ncurses, readline, sqlite
- Batch 3: brotli, libyaml, libffi, gmp, libxml2, libxslt
- Batch 4: libiconv (special case documentation)

Each batch: feature branch, apply template, test glibc + musl, validate, PR, merge.

**Success Criteria:**
- All 13 M47 library recipes support musl
- `tsuku coverage validate --strict` passes
- M47 can be closed as complete

### Phase 2: Visibility (3-4 weeks)

**Goal:** Make coverage visible through dashboard and metrics.

**Components:**
1. Metrics generator (batch analysis, historical snapshots)
2. Coverage dashboard (5 views: matrix, category, platform, gaps, trends)
3. Website coverage page (public visibility)

**Dashboard Views:**
- Overview matrix: recipes × platforms heatmap
- Category breakdown: libraries 40%, CLI 83%, runtimes 90%
- Platform breakdown: which platforms have worst coverage
- Gap details: drillable list with fix guides
- Trends: coverage improving over time

**Implementation:**
- Create static HTML in `website/coverage/` (matches `website/pipeline/` structure)
- Use vanilla JS + static HTML (matches existing pipeline dashboard)
- Load data from `website/coverage/coverage.json` (generated by coverage-analytics)
- Deploy alongside existing website at tsuku.dev/coverage

**Success Criteria:**
- Dashboard shows real-time coverage data
- Historical trend tracking operational
- Public visibility into platform support

### Phase 3: Enforcement (2-3 weeks)

**Goal:** Prevent coverage regression through automation.

**Components:**
1. CI coverage check (GitHub Actions workflow)
2. Automated PR comments with gap analysis
3. Opt-out validation

**Workflow:** `.github/workflows/coverage-check.yml`
- Trigger: PRs modifying `recipes/**/*.toml`
- Build: `go build -o coverage-analytics ./cmd/coverage-analytics`
- Run: `coverage-analytics --validate --strict --recipes recipes/`
- On failure: Post PR comment with fix guidance

**Validation Rules:**
- Library recipes MUST support both glibc and musl (blocking)
- CLI tools SHOULD support both (warning)
- Exclusions must have `reason` field and pass code review

**Success Criteria:**
- PRs blocked if library recipes lack musl support
- Automated PR comments with gap analysis
- Opt-out mechanism functional

### Phase 4: Enhancement (Future)

- Automated issue creation for coverage gaps
- Coverage badges for README
- Per-recipe coverage tracking over time
- Integration with release notes generation

## Required Tactical Designs

| Design | Target Repo | Purpose |
|--------|-------------|---------|
| DESIGN-coverage-analysis-package.md | tsuku | internal/coverage/ package with analysis logic |
| DESIGN-coverage-analytics-tool.md | tsuku | cmd/coverage-analytics/ tool for generating JSON |
| DESIGN-coverage-website.md | tsuku | website/coverage/ static HTML/JS for visualization |
| DESIGN-ci-coverage-enforcement.md | tsuku | GitHub Actions workflow and validation |
| DESIGN-recipe-coverage-metadata.md | tsuku | TOML schema for [coverage] section |
| DESIGN-m47-library-migration.md | tsuku | Systematic migration of 13 libraries |
| DESIGN-coverage-contribution-guide.md | tsuku | Documentation for contributors |

## Security Considerations

### Download Verification

Not applicable - this design doesn't download external binaries. Coverage system analyzes existing recipes and generates metrics from source files.

### Execution Isolation

The coverage analyzer parses TOML files and evaluates `when` clauses but doesn't execute recipe steps. Read-only analysis with no privilege escalation.

Risk: Maliciously crafted recipe could attempt code injection via TOML.
Mitigation: Use established TOML parser (golang.org/x/toml), don't eval recipe content.

### Supply Chain Risks

**Risk:** Malicious actors could manipulate coverage metrics to hide gaps.
**Mitigation:**
- Metrics generated from source recipes (not user-supplied data)
- CI regenerates metrics on every push to main
- Historical snapshots stored in git (tamper-evident)

**Risk:** Contributors could add exclusions without valid reasons to bypass CI.
**Mitigation:**
- Code review required for all exclusions
- Exclusions must include substantive `reason` field
- CI validates exclusion format
- Maintainers can reject insufficient justifications

**Risk:** Contributors might try to bypass coverage checks.
**Mitigation:**
- Coverage check is required status check (cannot be bypassed)
- Only maintainers can override
- Failed checks visible in PR history

### User Data Exposure

Coverage dashboard shows which recipes support which platforms. No sensitive data exposed - this is public information about recipe capabilities.

Dashboard runs as static site (no server-side processing), loads metrics from JSON file. No user authentication or session management.

## Consequences

### Positive

1. **Visible gaps** - Coverage issues immediately apparent in dashboard and CI
2. **Prevents regression** - Automated checks block PRs removing platform support
3. **Closes M47 gap** - Systematic workflow migrates 13 library recipes
4. **Guides contributors** - Clear feedback on what platforms need support
5. **Public transparency** - External users see what tsuku supports
6. **Prevents future invisible gaps** - Validation is continuous
7. **Scales to new dimensions** - Architecture supports adding dimensions (BSD, new architectures)

### Negative

1. **Implementation effort** - 8 tactical designs across 3 phases
2. **Maintenance burden** - Dashboard, CI checks, metrics need ongoing updates
3. **Possible false positives** - CI might block legitimate cases where platform support is impossible
4. **Additional recipe complexity** - Contributors must think about coverage when adding recipes
5. **Storage overhead** - Historical snapshots increase repo size over time

### Neutral

1. **Opt-out mechanism required** - Some tools genuinely can't support all platforms
2. **Coverage targets need definition** - What % coverage is "good enough"? (Proposal: libraries 100%, CLI 80%, runtimes 90%)
3. **Blocker classification is subjective** - "Can we fix this?" judgments may vary
4. **Dashboard vs CLI trade-offs** - Some users prefer visual dashboards, others prefer CLI tools

## References

- [M47 Design: Platform Compatibility Verification](./current/DESIGN-platform-compatibility-verification.md)
- [Pipeline Dashboard Design](./current/DESIGN-pipeline-dashboard.md)
- Recipe Format Specification (../RECIPE-FORMAT.md if it exists)
- Contributing Guide (../../CONTRIBUTING.md)

## Appendices

### M47 Library Recipes Requiring Migration

| Recipe | Current Coverage | Alpine Package | Difficulty |
|--------|------------------|----------------|------------|
| libcurl | glibc-only | curl-dev | Easy |
| openssl | glibc-only | openssl-dev | Easy |
| zlib | glibc-only | zlib-dev | Easy |
| brotli | glibc-only | brotli-dev | Easy |
| ncurses | glibc-only | ncurses-dev | Easy |
| readline | glibc-only | readline-dev | Easy |
| libyaml | glibc-only | yaml-dev | Easy |
| sqlite | glibc-only | sqlite-dev | Easy |
| libffi | glibc-only | libffi-dev | Easy |
| gmp | glibc-only | gmp-dev | Easy |
| libxml2 | glibc-only | libxml2-dev | Easy |
| libxslt | glibc-only | libxslt-dev | Easy |
| libiconv | glibc-only | (built-in) | Special case |

### Example Tool Output

```bash
$ coverage-analytics --analyze libcurl

Recipe Coverage Analysis
========================

Recipe: libcurl (library)
Platforms:
  ✓ linux-x86_64-glibc   (homebrew)
  ✗ linux-x86_64-musl    MISSING
  ✓ linux-aarch64-glibc  (homebrew)
  ✗ linux-aarch64-musl   MISSING
  ✓ darwin-x86_64        (homebrew)
  ✓ darwin-aarch64       (homebrew)

Gap Analysis:
  - Missing musl support for Alpine Linux users
  - Blocker: Homebrew provides glibc-only binaries
  - Fix: Add system package step (apk: curl-dev) with libc conditional

Recommendation:
  Add this step to recipes/l/libcurl.toml:

    [[step]]
    action = "install_system_package"
    apk = ["curl-dev"]
    when = { libc = ["musl"] }
```

### Glossary

- **Coverage** - The set of platforms (OS, architecture, libc family) a recipe supports
- **Gap** - A platform that should be supported but isn't
- **Blocker** - Reason why a platform isn't supported (upstream limitation, technical constraint)
- **Exclusion** - Explicit declaration that a platform isn't supported, with documented reason
- **Dimension** - Axis along which coverage is measured (libc, architecture, OS, category)
- **Platform** - Combination of OS + architecture + libc (e.g., linux-x86_64-musl)
- **Hybrid libc approach** - M47 pattern where recipes conditionally install from different sources based on libc
