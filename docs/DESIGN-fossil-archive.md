# Fossil Archive Action

## Status

Proposed

## Context and Problem Statement

Tsuku supports ecosystem-specific actions like `github_archive` that handle both version resolution and download in a single step. Several important open-source projects are hosted on [Fossil SCM](https://fossil-scm.org/), a distributed version control system with consistent patterns for releases and source downloads.

Notable Fossil-hosted projects include:

| Project | Repository URL |
|---------|----------------|
| SQLite | https://sqlite.org/src |
| Tcl | https://core.tcl-lang.org/tcl |
| Tk | https://core.tcl-lang.org/tk |
| SpatiaLite | https://www.gaia-gis.it/fossil/libspatialite |
| Fossil | https://fossil-scm.org/home |

Adding a `fossil_archive` action enables concise recipes for these projects, following the same pattern as `github_archive`.

### Scope

**In scope:**
- `fossil_archive` action with integrated version resolution
- Automatic tarball URL construction
- Configuration for tag prefix and project name

**Out of scope:**
- Standalone Fossil version provider (not needed with integrated action)
- Fossil-specific build steps (use existing `configure_make`, etc.)

## Decision Drivers

- **Consistency**: Follow the `github_archive` pattern
- **Minimal configuration**: Infer as much as possible from repo URL
- **Expand coverage**: Support Fossil-hosted projects

## Background

### Fossil SCM Capabilities

All Fossil repositories provide:

1. **Release timeline**: `{repo}/timeline?t=release&n=all&y=ci` - lists all releases
2. **Source tarballs**: `{repo}/tarball/{tag}/{project}.tar.gz` - any tagged version

### Comparison with github_archive

| Aspect | github_archive | fossil_archive |
|--------|----------------|----------------|
| Repo format | `owner/repo` | Full URL |
| Version source | GitHub Releases API | Fossil timeline |
| Download URL | GitHub release assets | `{repo}/tarball/{tag}/...` |
| Asset naming | Configurable pattern | Predictable pattern |

## Solution

### Action Definition

```toml
[[steps]]
action = "fossil_archive"
repo = "https://sqlite.org/src"
binaries = ["sqlite3"]
```

### Configuration Options

| Field | Required | Default | Description |
|-------|----------|---------|-------------|
| `repo` | Yes | - | Fossil repository URL |
| `binaries` | Yes | - | Binaries to install |
| `tag_prefix` | No | `"version-"` | Prefix before version in tags |
| `project_name` | No | Last path segment | Name in tarball filename |
| `strip_dirs` | No | `1` | Directories to strip from archive |

### Inference Rules

Given `repo = "https://sqlite.org/src"`:

1. **Project name**: `sqlite` (last path segment, or explicit `project_name`)
2. **Timeline URL**: `https://sqlite.org/src/timeline?t=release&n=all&y=ci`
3. **Tarball URL**: `https://sqlite.org/src/tarball/version-{version}/sqlite.tar.gz`

### Example Recipes

**SQLite:**
```toml
[metadata]
name = "sqlite"
description = "SQLite database engine"
homepage = "https://sqlite.org/"

[[steps]]
action = "fossil_archive"
repo = "https://sqlite.org/src"
binaries = ["sqlite3"]

[verify]
command = "sqlite3 --version"
pattern = "{version}"
```

**Fossil itself:**
```toml
[metadata]
name = "fossil"
description = "Distributed version control system"
homepage = "https://fossil-scm.org/"

[[steps]]
action = "fossil_archive"
repo = "https://fossil-scm.org/home"
binaries = ["fossil"]

[verify]
command = "fossil version"
pattern = "{version}"
```

**Tcl (different tag format):**
```toml
[metadata]
name = "tcl"
description = "Tool Command Language"
homepage = "https://www.tcl.tk/"

[[steps]]
action = "fossil_archive"
repo = "https://core.tcl-lang.org/tcl"
tag_prefix = "core-"
project_name = "tcl"
binaries = ["tclsh"]
```

## Implementation Approach

### Phase 1: Core Action

| Task | Description |
|------|-------------|
| Create `fossil_archive.go` | New action in `internal/actions/` |
| Timeline parsing | Fetch and parse release timeline |
| URL construction | Build tarball URL from config |
| Download + extract | Reuse existing download/extract logic |

### Phase 2: Integration

| Task | Description |
|------|-------------|
| Register action | Add to action registry |
| Unit tests | Test timeline parsing, URL construction |
| Integration tests | Test with real Fossil repos |

### Phase 3: Recipes

| Task | Description |
|------|-------------|
| SQLite recipe | Create recipe using `fossil_archive` |
| Documentation | Add to recipe authoring guide |

### Files to Create/Modify

| File | Changes |
|------|---------|
| `internal/actions/fossil_archive.go` | New action implementation |
| `internal/actions/registry.go` | Register action |
| `internal/recipe/recipes/s/sqlite.toml` | New recipe (or update existing) |

## Consequences

### Positive

- **Concise recipes**: Single action handles version + download
- **Consistent pattern**: Mirrors `github_archive` approach
- **Expanded coverage**: Fossil projects now supported

### Negative

- **New action to maintain**: Additional code surface
- **HTML parsing**: Depends on Fossil timeline structure

### Mitigations

- Timeline format has been stable for years
- Action is self-contained, minimal maintenance

## Security Considerations

### Download Verification

**Challenge**: Fossil tarballs are generated on-demand with no published checksums.

**Mitigations:**
- HTTPS enforced
- First-party servers only
- Post-install verification via `[verify]`
- Layer 3 checksum pinning after first install

### Execution Isolation

**Not applicable** - action only fetches HTML and tarballs.

### Supply Chain Risks

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Compromised server | Low | High | HTTPS, first-party only |
| Timeline injection | Very Low | Medium | Strict regex parsing |

### User Data Exposure

**Not applicable** - only fetches public pages.
