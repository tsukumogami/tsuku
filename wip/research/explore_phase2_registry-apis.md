# Registry API Quality Signals Research

Research into what popularity/quality fields each package registry exposes in their standard lookup endpoints, useful for distinguishing real packages from name-squatters.

## 1. crates.io (Rust)

**Endpoint**: `GET https://crates.io/api/v1/crates/{name}`

### Download/Popularity Metrics
| Field | Location | Description |
|-------|----------|-------------|
| `downloads` | `crate.downloads` | Total all-time downloads |
| `recent_downloads` | `crate.recent_downloads` | Downloads in last 90 days |

### Version/Age Signals
| Field | Location | Description |
|-------|----------|-------------|
| `created_at` | `crate.created_at` | Crate creation timestamp |
| `updated_at` | `crate.updated_at` | Last update timestamp |
| `max_version` | `crate.max_version` | Highest published version string |
| `max_stable_version` | `crate.max_stable_version` | Highest non-prerelease version |
| `versions` | `crate.versions` | Array of version IDs (count = number of releases) |
| `num` | `versions[].num` | Version string per release |
| `created_at` | `versions[].created_at` | Per-version publish date |
| `downloads` | `versions[].downloads` | Per-version download count |
| `yanked` | `versions[].yanked` | Whether version was yanked |

### Verified/Official Flags
- No explicit "verified" or "official" flag.
- `exact_match` (boolean) in crate object indicates whether the name was an exact match in search.

### Additional Quality Fields
- `categories` - array of category slugs
- `keywords` - array of keyword strings
- `repository` - source repo URL
- `homepage` - project homepage
- `documentation` - docs URL
- `description` - crate description

### Name-squatter Heuristics
Strong signals: `recent_downloads` = 0 or very low, `versions` array length = 1, `description` is empty/generic, no `repository` URL.

---

## 2. PyPI (Python)

**Endpoint**: `GET https://pypi.org/pypi/{name}/json`

### Download/Popularity Metrics
- **No download stats in the JSON API.** PyPI removed download counts from the API years ago.
- Download stats are available separately via Google BigQuery (`pypistats.org` or the `pypistats` CLI), but not in the package JSON endpoint.

### Version/Age Signals
| Field | Location | Description |
|-------|----------|-------------|
| `version` | `info.version` | Latest version string |
| `releases` | `releases` (top-level) | Dict keyed by version string; length = number of releases |
| `upload_time` | `releases[ver][].upload_time` | Per-file upload timestamp |
| `upload_time_iso_8601` | `releases[ver][].upload_time_iso_8601` | ISO format upload time |

Age can be derived from the earliest `upload_time` across all releases.

### Verified/Official Flags
- No explicit verified/official flag.
- `maintainer` and `maintainer_email` in `info`
- `author` and `author_email` in `info`
- `classifiers` list may contain development status (e.g., `"Development Status :: 5 - Production/Stable"`)

### Additional Quality Fields
| Field | Location | Description |
|-------|----------|-------------|
| `summary` | `info.summary` | One-line description |
| `description` | `info.description` | Full description (README) |
| `description_content_type` | `info.description_content_type` | text/markdown, etc. |
| `home_page` | `info.home_page` | Homepage URL |
| `project_urls` | `info.project_urls` | Dict of labeled URLs (Source, Documentation, etc.) |
| `license` | `info.license` | License text/name |
| `classifiers` | `info.classifiers` | Trove classifiers list |
| `requires_python` | `info.requires_python` | Python version constraint |
| `requires_dist` | `info.requires_dist` | Dependencies list |
| `yanked` | `info.yanked` | Whether latest is yanked |
| `yanked_reason` | `info.yanked_reason` | Reason if yanked |

### Name-squatter Heuristics
Strong signals: only 1 release, empty `summary`/`description`, no `project_urls`, no `requires_dist`, classifier shows "Development Status :: 1 - Planning".

---

## 3. npm (Node.js)

**Endpoint**: `GET https://registry.npmjs.org/{name}`

### Download/Popularity Metrics
- **Not in the registry endpoint.** Downloads available via separate API:
  - `GET https://api.npmjs.org/downloads/point/last-week/{name}` returns `{"downloads": N, "start": "...", "end": "...", "package": "..."}`
  - Also supports `last-day`, `last-month`, and custom date ranges.

### Version/Age Signals
| Field | Location | Description |
|-------|----------|-------------|
| `time.created` | `time.created` | Package creation timestamp |
| `time.modified` | `time.modified` | Last modified timestamp |
| `time[version]` | `time` | Publish timestamp per version |
| `versions` | `versions` | Object keyed by version string; key count = release count |
| `dist-tags` | `dist-tags` | Named tags (e.g., `latest`, `next`) |

### Verified/Official Flags
- No explicit verified/official flag in the registry API.
- `maintainers` array with `{name, email}` objects.
- npm has "provenance" attestations on some packages but these aren't in the basic registry response.

### Additional Quality Fields
| Field | Location | Description |
|-------|----------|-------------|
| `description` | `description` | Package description |
| `keywords` | `keywords` | Array of keyword strings |
| `repository` | `repository` | `{type, url}` object |
| `homepage` | `homepage` | Homepage URL |
| `license` | `license` | License identifier |
| `author` | `author` | Author object |
| `maintainers` | `maintainers` | Array of `{name, email}` |
| `readme` | `readme` | Full README text |
| `users` | `users` | Legacy star-count object |

### Name-squatter Heuristics
Strong signals: `time` has only `created`/`modified` + one version, empty `description`, no `repository`, last-week downloads near zero, no `readme` content.

---

## 4. RubyGems

**Endpoint**: `GET https://rubygems.org/api/v1/gems/{name}.json`

### Download/Popularity Metrics
| Field | Location | Description |
|-------|----------|-------------|
| `downloads` | `downloads` | Total all-time downloads (all versions) |
| `version_downloads` | `version_downloads` | Downloads for latest version |

### Version/Age Signals
| Field | Location | Description |
|-------|----------|-------------|
| `version` | `version` | Latest version string |
| `version_created_at` | `version_created_at` | Publish date of latest version |

Version count is not directly in this endpoint. Available via `GET /api/v1/versions/{name}.json` (returns array of all versions).

### Verified/Official Flags
- `yanked` (boolean) - whether latest version is yanked
- `metadata.rubygems_mfa_required` - "true" indicates MFA enforcement (trust signal)
- No explicit "verified" flag.

### Additional Quality Fields
| Field | Location | Description |
|-------|----------|-------------|
| `info` | `info` | Description text |
| `authors` | `authors` | Author string |
| `licenses` | `licenses` | Array of license identifiers |
| `homepage_uri` | `homepage_uri` | Homepage |
| `source_code_uri` | `source_code_uri` | Source repo |
| `documentation_uri` | `documentation_uri` | Docs URL |
| `changelog_uri` | `changelog_uri` | Changelog URL |
| `bug_tracker_uri` | `bug_tracker_uri` | Issue tracker |
| `funding_uri` | `funding_uri` | Funding page |
| `dependencies` | `dependencies` | Runtime and dev deps |

### Name-squatter Heuristics
Strong signals: `downloads` very low, `info` empty/placeholder, no `source_code_uri`, no `homepage_uri`, `version` is "0.0.1" or "0.0.0".

---

## 5. Go Modules (proxy.golang.org / pkg.go.dev)

**Endpoints**:
- `GET https://proxy.golang.org/{module}/@v/list` - list of versions (plain text, one per line)
- `GET https://proxy.golang.org/{module}/@latest` - latest version info
- `GET https://pkg.go.dev/{module}` - web only, no public JSON API for package metadata

### Download/Popularity Metrics
- **No download stats available.** The Go module proxy does not expose download counts.
- pkg.go.dev shows "importers" count on the web UI but has no public API for it.

### Version/Age Signals
| Field | Location | Description |
|-------|----------|-------------|
| `Version` | `@latest` response | Latest version tag |
| `Time` | `@latest` response | Publish timestamp |
| version list | `@v/list` response | Plain text list of all versions (line count = version count) |

The `@latest` response also includes `Origin` with VCS info:
- `Origin.VCS` - version control system (e.g., "git")
- `Origin.URL` - repository URL
- `Origin.Hash` - commit hash
- `Origin.Ref` - git ref (tag)

### Verified/Official Flags
- No verified/official flag.
- The `Origin` block provides VCS provenance but it's informational.

### Name-squatter Heuristics
Go modules are tied to import paths (domain-based), so name-squatting is structurally less common. Signals: only `v0.0.1` or `v0.0.0-` pseudo-versions, very recent `Time`, no real code at the repository URL.

---

## 6. MetaCPAN (Perl/CPAN)

**Endpoints**:
- `GET https://fastapi.metacpan.org/v1/distribution/{name}` - distribution metadata
- `GET https://fastapi.metacpan.org/v1/release/{name}` - latest release details

### Download/Popularity Metrics (distribution endpoint)
| Field | Location | Description |
|-------|----------|-------------|
| `river.bucket` | `river.bucket` | River of CPAN bucket (0-5 scale, higher = more depended upon) |
| `river.bus_factor` | `river.bus_factor` | Number of active contributors |
| `river.immediate` | `river.immediate` | Number of direct dependents |
| `river.total` | `river.total` | Total (transitive) dependents |
| `bugs.rt.active` | `bugs.rt` | RT bug tracker stats (active, closed, new, etc.) |

### Version/Age Signals (release endpoint)
| Field | Location | Description |
|-------|----------|-------------|
| `date` | `date` | Release date |
| `version` | `version` | Version string |
| `stat` | `stat` (if present) | Release statistics |
| `status` | `status` | Release status (e.g., "latest") |

### Verified/Official Flags
| Field | Location | Description |
|-------|----------|-------------|
| `authorized` | release endpoint | Boolean - whether release is authorized by the package owner |

### Additional Quality Fields (release endpoint)
- `author` - CPAN author ID (PAUSE ID)
- `abstract` - short description
- `license` - array of license identifiers
- `dependency` - array of dependency objects
- `changes_file` - whether a changelog exists

### Additional Quality Fields (distribution endpoint)
- `external_package.debian` - Debian package name (indicates real adoption)
- `external_package.fedora` - Fedora package name

### Name-squatter Heuristics
Strong signals: `river.total` = 0, `river.immediate` = 0, `river.bucket` = 0, `authorized` = false, no `external_package` entries.

---

## 7. Homebrew Cask (formulae.brew.sh)

**Endpoint**: `GET https://formulae.brew.sh/api/cask/{name}.json`

(Also `GET https://formulae.brew.sh/api/formula/{name}.json` for formulae)

### Download/Popularity Metrics
| Field | Location | Description |
|-------|----------|-------------|
| `analytics.install.30d` | `analytics` | Install count over 30 days |
| `analytics.install.90d` | `analytics` | Install count over 90 days |
| `analytics.install.365d` | `analytics` | Install count over 365 days |

### Version/Age Signals
| Field | Location | Description |
|-------|----------|-------------|
| `version` | `version` | Current version |
| `installed` | `installed` | Local install info |
| `outdated` | `outdated` | Whether update available |

Homebrew doesn't expose historical version lists or creation dates via this API. The tap is a git repo, so history is in git.

### Verified/Official Flags
| Field | Location | Description |
|-------|----------|-------------|
| `deprecated` | `deprecated` | Whether cask is deprecated |
| `disabled` | `disabled` | Whether cask is disabled |
| `autobump` | `autobump` | Whether version tracking is automated |
| `tap` | `tap` | Which tap it belongs to (homebrew/cask = official) |

### Additional Quality Fields
- `homepage` - project homepage
- `desc` - description
- `url` - download URL
- `sha256` - checksum
- `depends_on` - system requirements
- `conflicts_with` - conflicting casks

### Name-squatter Heuristics
Less relevant for Homebrew since all casks are reviewed via PR before merging into the official tap. The `tap` field distinguishes official from third-party taps.

---

## Summary Comparison

| Registry | Downloads in API | Version Count | Age/Dates | Verified Flag | Best Squatter Signal |
|----------|-----------------|---------------|-----------|---------------|---------------------|
| crates.io | `downloads`, `recent_downloads` | Version array length | `created_at`, `updated_at` | None | `recent_downloads` = 0 |
| PyPI | None (separate BigQuery) | `releases` dict length | `upload_time` per release | None | Single release, empty description |
| npm | Separate downloads API | `versions` object length | `time.created`, per-version timestamps | None | Near-zero weekly downloads |
| RubyGems | `downloads`, `version_downloads` | Separate endpoint | `version_created_at` | None | Low `downloads`, no source URL |
| Go proxy | None | `@v/list` line count | `Time` in `@latest` | None | Domain-based paths deter squatting |
| MetaCPAN | `river.*` (dependents, not downloads) | Via release search | `date` per release | `authorized` boolean | `river.total` = 0, unauthorized |
| Homebrew | `analytics.install.*` (30/90/365d) | None | None | `tap` = official | Curated tap = low risk |

### Registries Ranked by Signal Richness

1. **crates.io** - Best built-in signals: all-time + recent downloads, version history, dates, all in one endpoint.
2. **RubyGems** - Good: total + per-version downloads directly in response.
3. **MetaCPAN** - Unique: `river` dependency graph metrics + `authorized` flag + OS package presence.
4. **Homebrew** - Good install analytics (30/90/365d) but curated so squatting is moot.
5. **npm** - Requires second API call for downloads; registry endpoint has good date/version data.
6. **PyPI** - No downloads in API at all; must rely on version count and metadata completeness.
7. **Go proxy** - Minimal metadata; squatting mitigated by domain-based naming.
