# Fossil Version Source for Build-from-Source Recipes

## Status

Proposed

## Context and Problem Statement

Some tools use non-standard version formats in their download URLs that are incompatible with simple `{version}` substitution. The canonical example is SQLite:

**SQLite amalgamation URL format:**
- Semantic version: `3.51.1`
- Download URL: `https://sqlite.org/2025/sqlite-autoconf-3510100.tar.gz`
  - Year path component: `2025` (changes with releases)
  - Transformed version: `3510100` (format: `3XXYY00` where X.Y is the minor.patch)

This creates two problems:

1. **Recipe maintenance burden**: Recipes must hardcode URLs and version strings, requiring manual updates for each release
2. **Version provider mismatch**: Even when a version provider (e.g., Homebrew) correctly reports `3.51.1`, the download URL requires a computed transformation

**Key insight:** SQLite is hosted on Fossil SCM, which provides a consistent URL pattern for source tarballs that uses semantic versions directly:

```
https://sqlite.org/src/tarball/version-{version}/sqlite.tar.gz
```

This URL:
- Uses the semantic version directly (`version-3.51.1`, not `version-3510100`)
- Has no year component
- Works for any tagged version in history

By introducing a **Fossil version source**, we can solve the SQLite recipe problem without implementing version transforms at all.

### Scope

**In scope:**
- Fossil version source for tools hosted on Fossil SCM
- Update sqlite-source.toml to use Fossil tarballs
- Version enumeration from Fossil timeline
- Release date metadata from Fossil (for future use)

**Out of scope:**
- Computed version transformations (deferred - no current use case beyond SQLite)
- Year/date URL components (not needed with Fossil approach)
- General-purpose templating language

### Key Trade-off

Fossil tarballs contain **raw source code**, not the pre-processed amalgamation. Building from raw source requires Tcl as a build dependency. This is an acceptable trade-off because:

1. Tsuku can manage Tcl as a recipe dependency
2. The alternative (version transforms + year resolution) adds significant complexity
3. Building from raw source is more aligned with "build from source" philosophy

## Decision Drivers

- **Minimal recipe maintenance**: Version updates should not require recipe edits
- **Leverage existing infrastructure**: Fossil SCM provides consistent URL patterns across all hosted projects
- **Complete version history**: Solution must work for pinned versions, not just "latest"
- **Generic abstraction**: Solution should work for other Fossil-hosted projects (Tcl/Tk, SpatiaLite, Fossil itself)

### Key Assumptions

1. **Fossil URL patterns are stable**: Fossil SCM's `/tarball/TAG/project.tar.gz` pattern is a core feature of the Fossil SCM software and unlikely to change.

2. **Raw source builds are acceptable**: Building from raw source (requiring Tcl) is acceptable for the sqlite-source recipe. Users wanting pre-built binaries should use the Homebrew-based sqlite recipe.

3. **Version tags follow semantic versioning**: Fossil tags like `version-3.51.1` can be mapped to semantic versions for version comparison and selection.

4. **Timeline parsing is reliable**: The Fossil release timeline HTML structure has been stable for years and is consistent across all Fossil installations.

## Background

### Fossil SCM Overview

[Fossil SCM](https://fossil-scm.org/) is a distributed version control system used by several significant open-source projects:

| Project | Repository URL | Tarball Pattern |
|---------|----------------|-----------------|
| SQLite | https://sqlite.org/src | `/tarball/version-X.Y.Z/sqlite.tar.gz` |
| Tcl | https://core.tcl-lang.org/tcl | `/tarball/core-X-Y-Z/tcl.tar.gz` |
| Tk | https://core.tcl-lang.org/tk | `/tarball/core-X-Y-Z/tk.tar.gz` |
| SpatiaLite | https://www.gaia-gis.it/fossil/libspatialite | `/tarball/X.Y.Z/libspatialite.tar.gz` |
| Fossil | https://fossil-scm.org/home | `/tarball/version-X.Y.Z/fossil.tar.gz` |

All Fossil repositories provide:
1. **Timeline page**: Lists all releases with dates and commit hashes
2. **Tarball endpoint**: Downloads source for any tag: `{repo}/tarball/{tag}/{project}.tar.gz`
3. **Zipball endpoint**: Same as tarball but `.zip` format

### Current sqlite-source.toml Problem

The current recipe uses hardcoded URLs with SQLite's non-standard version encoding:

```toml
url = "https://sqlite.org/2025/sqlite-autoconf-3510100.tar.gz"
```

This requires manual updates for each release because:
- Version encoding: `3.51.1` → `3510100`
- Year component: `2025/` in path

### Alternative: Fossil Tarball

Using the Fossil tarball URL eliminates both problems:

```toml
url = "https://sqlite.org/src/tarball/version-{version}/sqlite.tar.gz"
```

- Uses semantic version directly: `version-3.51.1`
- No year component needed
- Works for any tagged version

## Considered Options

### Option A: Version Transforms + Year Resolution

Implement a system for transforming versions in URLs:

```toml
[[steps]]
action = "download"
url = "https://sqlite.org/{release_year}/sqlite-autoconf-{version|packed_semver}.tar.gz"
```

This requires:
1. A transform syntax (pipe operator `{version|transform}`)
2. Transform functions in Go (`packed_semver`, etc.)
3. A way to resolve year/date components (metadata parsing, provider extension)

**Pros:**
- Works with amalgamation downloads (simpler build, no Tcl)
- Could apply to non-Fossil projects with similar issues

**Cons:**
- Significant implementation effort (three components)
- Year resolution is complex (metadata parsing, caching, error handling)
- SQLite-specific until we find other tools that need it
- Over-engineered for the immediate problem

### Option B: Fossil Version Source (Recommended)

Use Fossil's native tarball endpoint which requires no transforms:

```toml
[version]
source = "fossil"
repo = "https://sqlite.org/src"

[[steps]]
action = "download"
url = "https://sqlite.org/src/tarball/version-{version}/sqlite.tar.gz"
```

**Pros:**
- Single parameter configuration (just repo URL)
- No version transforms needed (semantic version works directly)
- No year resolution needed (Fossil URLs are version-only)
- Works for any Fossil project (SQLite, Tcl, Tk, SpatiaLite, Fossil)
- Provides full version history for pinned versions
- Avoids building a transform system we may not need

**Cons:**
- Only works for Fossil-hosted projects
- Raw source requires Tcl to build (vs. amalgamation)
- Fossil timeline parsing (HTML, though stable)

### Option C: Keep Hardcoded URLs

Continue with manually-maintained URLs:

```toml
url = "https://sqlite.org/2025/sqlite-autoconf-3510100.tar.gz"
```

**Pros:**
- No code changes needed
- Works today

**Cons:**
- Manual updates for each release
- Defeats the purpose of version providers
- Recipe maintenance burden

### Evaluation Matrix

| Criterion | A (Transforms) | B (Fossil) | C (Hardcoded) |
|-----------|----------------|------------|---------------|
| Implementation effort | High | Medium | None |
| Maintenance burden | Low | Low | High |
| Generality | Medium | Good (Fossil projects) | N/A |
| Solves SQLite | Yes | Yes | Partially |
| Future-proof | Maybe | Yes | No |

## Decision Outcome

**Chosen: Option B - Fossil Version Source**

### Summary

Introduce a Fossil SCM version source that leverages Fossil's consistent URL patterns. This eliminates the need for version transforms or year resolution entirely, since Fossil tarball URLs use semantic versions directly.

### Rationale

The Fossil approach was chosen because:

1. **Eliminates transform complexity**: Fossil tarball URLs like `/tarball/version-3.51.1/sqlite.tar.gz` use the semantic version directly. No `packed_semver` transform needed.

2. **Eliminates year resolution**: Fossil URLs have no date component. No metadata parsing or caching needed.

3. **Generic solution**: Works for any Fossil-hosted project, not just SQLite. The same pattern applies to Tcl, Tk, SpatiaLite, and Fossil itself.

4. **Complete version history**: Fossil timeline provides metadata for all historical versions, enabling version pinning without special handling.

5. **Simpler implementation**: One new version provider vs. three components (transform syntax, transform registry, year resolver).

### Alternatives Rejected

- **Option A (Transforms + Year Resolution)**: Over-engineered for the immediate problem. Building a transform system without clear use cases beyond SQLite is premature. If we encounter non-Fossil tools that need transforms, we can add that system later.

- **Option C (Hardcoded URLs)**: Creates ongoing maintenance burden. Defeats the purpose of having version providers.

### Trade-offs Accepted

1. **Raw source builds require Tcl**: Building from Fossil tarballs requires Tcl as a build dependency. This is acceptable because:
   - Tcl can be managed as a recipe dependency
   - Users wanting pre-built binaries can use the Homebrew-based `sqlite` recipe
   - "Build from source" philosophy aligns with using actual source code

2. **Only Fossil projects benefit**: This solution specifically targets Fossil-hosted projects. Non-Fossil projects with similar version encoding issues would need a different solution. This is acceptable because:
   - We have no concrete non-Fossil examples yet
   - YAGNI: we shouldn't build transforms without clear use cases
   - Fossil hosts several important projects (SQLite, Tcl, Tk, SpatiaLite)

3. **Transform system deferred**: If we later find tools that genuinely need version transforms, we'll design that system then with concrete requirements.

## Deferred Features

The following features were considered during this design but deferred pending concrete use cases. Each should be tracked as a future exploration issue.

### Version Transforms (Future Issue)

A system for transforming version strings in URLs using pipe syntax:

```toml
url = "https://example.com/{version|packed_semver}.tar.gz"
```

**Potential transforms:**
- `packed_semver`: Pack `X.Y.Z` into `XYYZZNN` (e.g., `3.51.1` → `3510100`)
- `strip_v`: Remove leading `v` prefix
- `underscores`: Replace dots with underscores
- `major_minor`: Extract `X.Y` from `X.Y.Z`

**When to implement:** When we encounter non-Fossil tools with non-standard version encodings in download URLs.

### Version Provider Metadata (Future Issue)

Extend version providers to return additional metadata beyond just the version string:

```go
type VersionInfo struct {
    Version     string
    ReleaseDate time.Time  // for year-based URLs
    Checksum    string     // for verification
    DownloadURL string     // canonical download URL
}
```

**When to implement:** When we need release dates for URL construction (year paths) or want providers to supply checksums directly.

## Solution Architecture

### Overview

Add a new Fossil version provider that:
1. Parses the Fossil release timeline to enumerate available versions
2. Extracts version tags and release dates
3. Provides standard version provider interface (`GetLatest`, `GetVersions`)

```
┌─────────────────────────────────────────────────────────────────────────┐
│                           Recipe Definition                              │
├─────────────────────────────────────────────────────────────────────────┤
│  [version]                                                               │
│  source = "fossil"                                                       │
│  repo = "https://sqlite.org/src"                                        │
│  tag_prefix = "version-"   # optional, defaults to "version-"           │
│                                                                          │
│  [[steps]]                                                               │
│  action = "download"                                                     │
│  url = "https://sqlite.org/src/tarball/version-{version}/sqlite.tar.gz" │
└─────────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
                        ┌───────────────────┐
                        │  Fossil Provider  │
                        ├───────────────────┤
                        │ • Parse timeline  │
                        │ • Get versions    │
                        │ • Get latest      │
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

### Key Interfaces

**Fossil Provider Configuration:**

```go
type FossilConfig struct {
    Repo      string // Repository URL (e.g., "https://sqlite.org/src")
    TagPrefix string // Tag prefix to strip (default: "version-")
}
```

**Provider Interface (existing):**

```go
type Provider interface {
    GetLatest(ctx context.Context) (string, error)
    GetVersions(ctx context.Context) ([]string, error)
}
```

### Fossil Timeline Parsing

The Fossil release timeline (`{repo}/timeline?t=release&n=all&y=ci`) contains release entries with:

```html
<span class='timelineSimpleComment'>Version 3.51.1</span>
...
<span class='timelineHash'>281fc0e9af</span>
...
<div class="divider timelineDate">2025-11-28</div>
```

Extraction approach:
1. Fetch timeline HTML
2. Use regex to extract version tags: `version-(\d+\.\d+\.\d+)`
3. Parse dates from `timelineDate` dividers
4. Return versions sorted by date (newest first)

### Data Flow

1. **Recipe Loading**: Parse `[version]` section, recognize `source = "fossil"`
2. **Provider Creation**: Instantiate `FossilProvider` with repo URL and tag prefix
3. **Version Resolution**: Call `GetLatest()` or `GetVersions()` as needed
4. **Timeline Fetch**: Fetch and parse `{repo}/timeline?t=release&n=all&y=ci`
5. **Version Extraction**: Extract versions from tags, strip prefix, sort by date
6. **Standard Flow**: Return version string to recipe executor for URL expansion

### Error Handling

```go
// Example error messages
"fossil: failed to fetch timeline from https://sqlite.org/src: connection timeout"
"fossil: no releases found in timeline"
"fossil: failed to parse version from tag 'release-3.x'"
```

Errors are returned from the provider and propagated to the user with context.

## Implementation Approach

### Phase 1: Fossil Version Provider

Implement the core Fossil provider.

| Task | Description |
|------|-------------|
| Create `fossil.go` | New file `internal/version/fossil.go` with `FossilProvider` |
| Timeline fetching | HTTP client to fetch `{repo}/timeline?t=release&n=all&y=ci` |
| HTML parsing | Regex extraction of version tags and dates |
| Provider interface | Implement `GetLatest()` and `GetVersions()` |
| Unit tests | Test parsing with sample HTML, error cases |

**Dependencies:** None

### Phase 2: Provider Registration

Integrate into the version provider system.

| Task | Description |
|------|-------------|
| Register provider | Add "fossil" to provider registry in `providers.go` |
| Config parsing | Parse `repo` and optional `tag_prefix` from recipe TOML |
| Integration tests | End-to-end test with real Fossil repo |

**Dependencies:** Phase 1

### Phase 3: Recipe Updates

Update sqlite-source.toml to use Fossil.

| Task | Description |
|------|-------------|
| Update sqlite-source.toml | Change to `source = "fossil"` with Fossil tarball URL |
| Add Tcl dependency | Add `tcl` to dependencies (required for raw source build) |
| Test build | Verify sqlite builds successfully from Fossil tarball |
| Documentation | Update recipe authoring guide with Fossil source example |

**Dependencies:** Phase 2, Tcl recipe

### Files to Modify

| File | Changes |
|------|---------|
| `internal/version/fossil.go` | New file: `FossilProvider` implementation |
| `internal/version/providers.go` | Register "fossil" source type |
| `internal/recipe/version.go` | Parse `repo` and `tag_prefix` config fields |
| `testdata/recipes/sqlite-source.toml` | Update to use Fossil source |

### Updated Recipe Example

After implementation, sqlite-source.toml becomes:

```toml
[metadata]
name = "sqlite-source"
description = "SQLite CLI (built from source)"
homepage = "https://sqlite.org/"
dependencies = ["readline", "tcl"]

[version]
source = "fossil"
repo = "https://sqlite.org/src"

[[steps]]
action = "download"
url = "https://sqlite.org/src/tarball/version-{version}/sqlite.tar.gz"
skip_verification_reason = "Fossil tarballs are generated on-demand; no published checksums"

[[steps]]
action = "extract"
archive = "sqlite.tar.gz"
format = "tar.gz"

[[steps]]
action = "setup_build_env"

[[steps]]
action = "run_command"
command = "tclsh configure.tcl"
working_dir = "sqlite"

[[steps]]
action = "configure_make"
source_dir = "sqlite"
configure_args = ["--enable-readline"]
executables = ["sqlite3"]

# ... remaining steps
```

## Consequences

### Positive

- **Enables automated version updates**: sqlite-source.toml can track upstream versions without hardcoding
- **Generic solution**: Works for any Fossil-hosted project (SQLite, Tcl, Tk, SpatiaLite, Fossil)
- **Complete version history**: Supports version pinning with full historical data
- **Simpler than alternatives**: One provider vs. transform system + year resolution
- **Avoids premature abstraction**: No transform system until concrete use cases emerge

### Negative

- **Tcl build dependency**: Building from raw Fossil source requires Tcl
- **HTML parsing**: Timeline parsing depends on Fossil's HTML structure (stable but not guaranteed)
- **Fossil-only**: Non-Fossil projects don't benefit from this solution

### Mitigations

- **Tcl dependency**: Create a Tcl recipe so it can be managed as a dependency
- **HTML parsing fragility**: Fossil's timeline format has been stable for years; add robust error handling and fallback to cached versions if parsing fails
- **Fossil-only scope**: If we encounter non-Fossil tools with similar needs, we can add transforms then with concrete requirements

## Security Considerations

### Download Verification

**Challenge**: Fossil tarballs are generated on-demand by the Fossil server. There are no pre-published checksums for these archives.

**Mitigations:**
- Use HTTPS for all Fossil repository URLs (enforced in provider)
- Fossil servers are first-party (e.g., sqlite.org, not a mirror)
- Post-install verification (`[verify]` section) confirms expected binary behavior
- Future: Layer 3 checksum pinning (#693) can pin checksums after first successful install

**Residual risk:** First download of a version has no checksum verification. Accepted because:
- Source is authoritative (project's own Fossil server)
- Build-from-source users accept more responsibility for verification
- Users can manually verify source after download

### Execution Isolation

**Not applicable** - the Fossil provider only fetches HTML (timeline) and tarballs. It does not execute any content.

The provider:
- Makes HTTP GET requests to Fossil servers
- Parses HTML with regex (no JavaScript execution)
- Returns version strings

No code execution occurs in the provider itself.

### Supply Chain Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Compromised Fossil server | Low | High | HTTPS, first-party servers only |
| Timeline HTML injection | Very Low | Medium | Strict regex patterns, no eval |
| Version list manipulation | Low | Medium | Cache timeline results, log version changes |
| Man-in-the-middle attack | Very Low | High | HTTPS enforced |

**Fossil server compromise:** If sqlite.org were compromised, an attacker could:
- Inject malicious versions into the timeline
- Serve malicious tarballs

This risk exists for any version provider. Mitigations:
- HTTPS prevents MITM
- Fossil servers are well-maintained first-party infrastructure
- Users building from source typically have security awareness

### User Data Exposure

**Not applicable** - the Fossil provider does not access or transmit user data.

The provider only:
- Fetches public timeline pages
- Fetches public source tarballs

No user-specific information (paths, credentials, identifiers) is involved.

### Security Mitigations Summary

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| No checksum verification | HTTPS, first-party servers, post-install verify | First download unverified (accepted) |
| HTML parsing vulnerabilities | Strict regex, no eval/JS | Novel parsing edge cases |
| Server compromise | HTTPS, trusted sources | Sophisticated attack (accepted) |
| User data exposure | No user data accessed | None |

