# Fossil Version Source

## Status

Proposed

## Context and Problem Statement

Tsuku's version providers currently support GitHub, Homebrew, PyPI, npm, and other popular package registries. However, several important open-source projects are hosted on [Fossil SCM](https://fossil-scm.org/), a distributed version control system that provides consistent patterns for version enumeration and source downloads.

Notable Fossil-hosted projects include:

| Project | Repository URL |
|---------|----------------|
| SQLite | https://sqlite.org/src |
| Tcl | https://core.tcl-lang.org/tcl |
| Tk | https://core.tcl-lang.org/tk |
| SpatiaLite | https://www.gaia-gis.it/fossil/libspatialite |
| Fossil | https://fossil-scm.org/home |

Without a Fossil version provider, recipes for these tools must either:
- Use Homebrew as an intermediary (losing the ability to build from source)
- Hardcode version strings and URLs (requiring manual updates)

Adding a Fossil version source enables recipes to build these tools from source with automatic version tracking.

### Scope

**In scope:**
- Fossil version source (`source = "fossil"`)
- Version enumeration from Fossil timeline
- Integration with existing version provider system

**Out of scope:**
- Changes to other version providers
- Fossil-specific download actions (standard `download` action works)

## Decision Drivers

- **Expand coverage**: Support important projects not on GitHub/Homebrew
- **Leverage existing patterns**: Fossil provides consistent, well-documented APIs
- **Complete version history**: Enable version pinning with full historical data
- **Minimal integration effort**: Fits existing version provider interface

### Key Assumptions

1. **Fossil URL patterns are stable**: The `/tarball/TAG/project.tar.gz` pattern is a core Fossil SCM feature.

2. **Timeline structure is consistent**: All Fossil repositories use the same timeline HTML structure.

3. **Version tags are parseable**: Projects use predictable tag naming (e.g., `version-X.Y.Z`).

## Background

### Fossil SCM Capabilities

Fossil repositories provide:

1. **Release timeline**: `{repo}/timeline?t=release&n=all&y=ci` lists all releases with dates and commit hashes

2. **Source tarballs**: `{repo}/tarball/{tag}/{project}.tar.gz` downloads source for any tag

3. **Tag list**: `{repo}/taglist` enumerates all tags

These endpoints are consistent across all Fossil installations and have been stable for years.

### Example: SQLite

SQLite's Fossil repository at `https://sqlite.org/src` provides:
- Timeline with 201+ releases dating back to 2007
- Tarballs for any version: `https://sqlite.org/src/tarball/version-3.51.1/sqlite.tar.gz`
- Tags following `version-X.Y.Z` naming convention

A recipe using the Fossil version source:

```toml
[version]
source = "fossil"
repo = "https://sqlite.org/src"

[[steps]]
action = "download"
url = "https://sqlite.org/src/tarball/version-{version}/sqlite.tar.gz"
```

## Considered Options

### Option A: Fossil Version Source

Add a dedicated Fossil version provider that parses the release timeline.

**Pros:**
- Direct integration with Fossil's native APIs
- Complete version history available
- Works for any Fossil-hosted project
- Consistent with other version providers

**Cons:**
- Requires HTML parsing (timeline page)
- New provider to maintain

### Option B: GitHub Mirror

Some Fossil projects have GitHub mirrors. Use the GitHub provider instead.

**Pros:**
- No new code needed
- JSON API (no HTML parsing)

**Cons:**
- Not all Fossil projects have mirrors
- Mirrors may lag behind or be incomplete
- Doesn't solve the general case

### Option C: Hardcoded URLs

Continue with manually-maintained URLs in recipes.

**Pros:**
- No code changes needed

**Cons:**
- Manual updates for each release
- Defeats the purpose of version providers

## Decision Outcome

**Chosen: Option A - Fossil Version Source**

### Summary

Add a Fossil version provider that parses the release timeline to enumerate versions and integrates with the existing provider system.

### Rationale

1. **Complete coverage**: Works for all Fossil-hosted projects, not just those with GitHub mirrors

2. **Authoritative source**: Uses the project's own repository, not a third-party mirror

3. **Full history**: Timeline provides all historical versions, enabling version pinning

4. **Consistent patterns**: Fossil's URL patterns are standardized across all installations

### Alternatives Rejected

- **Option B (GitHub Mirror)**: Not all Fossil projects have mirrors, and mirrors may be incomplete
- **Option C (Hardcoded URLs)**: Creates ongoing maintenance burden

## Solution Architecture

### Overview

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Recipe Definition                              │
├─────────────────────────────────────────────────────────────────────────┤
│  [version]                                                               │
│  source = "fossil"                                                       │
│  repo = "https://sqlite.org/src"                                        │
│  tag_prefix = "version-"   # optional, defaults to "version-"           │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                        ┌───────────────────┐
                        │  Fossil Provider  │
                        ├───────────────────┤
                        │ • Fetch timeline  │
                        │ • Parse versions  │
                        │ • Sort by date    │
                        └───────────────────┘
                                    │
                                    ▼
                        ┌───────────────────┐
                        │ Version: "3.51.1" │
                        └───────────────────┘
```

### Components

| Component | Location | Purpose |
|-----------|----------|---------|
| Fossil Provider | `internal/version/fossil.go` | Fetch versions from Fossil timeline |
| Provider Registration | `internal/version/providers.go` | Register "fossil" source type |

### Configuration

```go
type FossilConfig struct {
    Repo      string // Repository URL (required)
    TagPrefix string // Tag prefix to strip (default: "version-")
}
```

### Provider Interface

The Fossil provider implements the existing `Provider` interface:

```go
type Provider interface {
    GetLatest(ctx context.Context) (string, error)
    GetVersions(ctx context.Context) ([]string, error)
}
```

### Timeline Parsing

The release timeline (`{repo}/timeline?t=release&n=all&y=ci`) contains:

```html
<div class="divider timelineDate">2025-11-28</div>
...
<span class='timelineSimpleComment'>Version 3.51.1</span>
...
<span class='timelineHash'>281fc0e9af</span>
```

Extraction:
1. Fetch timeline HTML
2. Extract version tags via regex: `Version (\d+\.\d+\.\d+)`
3. Parse dates from `timelineDate` dividers
4. Return versions sorted by date (newest first)

### Error Handling

```go
// Example error messages
"fossil: failed to fetch timeline: connection timeout"
"fossil: no releases found in timeline"
"fossil: failed to parse version from 'Version 3.x'"
```

## Implementation Approach

### Phase 1: Core Provider

| Task | Description |
|------|-------------|
| Create `fossil.go` | New file with `FossilProvider` struct |
| Timeline fetching | HTTP GET to `{repo}/timeline?t=release&n=all&y=ci` |
| HTML parsing | Regex extraction of versions and dates |
| Provider methods | Implement `GetLatest()` and `GetVersions()` |
| Unit tests | Test parsing with sample HTML |

### Phase 2: Integration

| Task | Description |
|------|-------------|
| Register provider | Add "fossil" to provider registry |
| Config parsing | Parse `repo` and `tag_prefix` from TOML |
| Integration tests | Test with real Fossil repositories |

### Phase 3: Recipe Examples

| Task | Description |
|------|-------------|
| SQLite recipe | Create/update sqlite recipe using Fossil |
| Documentation | Add Fossil source to recipe authoring guide |

### Files to Modify

| File | Changes |
|------|---------|
| `internal/version/fossil.go` | New file: `FossilProvider` |
| `internal/version/providers.go` | Register "fossil" source |
| `internal/recipe/version.go` | Parse `repo` and `tag_prefix` |

### Recipe Example

```toml
[metadata]
name = "sqlite"
description = "SQLite database engine"
homepage = "https://sqlite.org/"

[version]
source = "fossil"
repo = "https://sqlite.org/src"

[[steps]]
action = "download"
url = "https://sqlite.org/src/tarball/version-{version}/sqlite.tar.gz"
skip_verification_reason = "Fossil tarballs are generated on-demand"

[[steps]]
action = "extract"
archive = "sqlite.tar.gz"
format = "tar.gz"

# ... build steps
```

## Consequences

### Positive

- **Expanded coverage**: Recipes can now track Fossil-hosted projects
- **Automatic updates**: No manual version maintenance for Fossil projects
- **Version pinning**: Full version history enables installing specific versions
- **Consistent interface**: Same provider pattern as GitHub, Homebrew, etc.

### Negative

- **HTML parsing**: Depends on Fossil's timeline HTML structure
- **Maintenance**: Another provider to maintain

### Mitigations

- **HTML fragility**: Fossil's timeline format has been stable for years; add robust error handling
- **Maintenance**: Provider is simple (one HTTP call, regex parsing)

## Security Considerations

### Download Verification

**Challenge**: Fossil tarballs are generated on-demand. No pre-published checksums exist.

**Mitigations:**
- HTTPS enforced for all Fossil URLs
- First-party servers only (e.g., sqlite.org, not mirrors)
- Post-install verification via `[verify]` section
- Layer 3 checksum pinning (#693) can pin after first install

**Residual risk:** First download has no checksum. Accepted because source is authoritative.

### Execution Isolation

**Not applicable** - provider only fetches HTML and returns version strings.

### Supply Chain Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Compromised Fossil server | Low | High | HTTPS, first-party only |
| Timeline HTML injection | Very Low | Medium | Strict regex patterns |
| MITM attack | Very Low | High | HTTPS enforced |

### User Data Exposure

**Not applicable** - provider only fetches public timeline pages.
