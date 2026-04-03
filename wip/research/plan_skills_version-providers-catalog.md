# Version Providers Catalog

14 version providers via strategy pattern.

## Provider Table

| Source Value | Resolves From | Key Config Fields | Interfaces | Notes |
|---|---|---|---|---|
| `github` / `github_repo` | GitHub releases/tags (REST API) | `github_repo` (owner/repo), `tag_prefix` (optional) | VersionResolver, VersionLister | Auto-detects from github_archive/github_file. Tag prefix filtering. Filters out preview/alpha/beta/rc. |
| `npm` | npm registry (JSON API) | Package name from npm_install action | VersionResolver, VersionLister | Auto-detects from npm_install. Fuzzy matching: "1.2" matches "1.2.3" but not "1.20.0". |
| `pypi` | PyPI JSON API | Package name from pipx_install action | VersionResolver, VersionLister | Auto-detects from pipx_install. Excludes yanked versions. |
| `crates_io` | crates.io API | Crate name from cargo_install action | VersionResolver, VersionLister | Auto-detects from cargo_install. Filters yanked. Semver sort. |
| `rubygems` | RubyGems.org | Gem name from gem_install action | VersionResolver, VersionLister | Auto-detects from gem_install. Fuzzy matching. |
| `goproxy` | proxy.golang.org | `module` field or from go_install action | VersionResolver, VersionLister | Auto-detects from go_install. "v" prefix. Pattern inference for github repos. |
| `metacpan` | MetaCPAN (Perl) | Distribution name from cpan_install | VersionResolver, VersionLister | Auto-detects from cpan_install. Normalizes "v" prefix. |
| `go_toolchain` | go.dev/dl JSON API | None | VersionResolver, VersionLister | Toolchain versions (no "v" prefix). Fuzzy matching. |
| `homebrew` | Homebrew API | `formula` field | VersionResolver, VersionLister | Auto-detects from homebrew action. Current stable + versioned formulae only. |
| `cask` | Homebrew Cask API | `cask` field | VersionResolver only | Platform-specific URL/checksum. arm64_ prefix for Apple Silicon. |
| `tap` | Third-party Homebrew tap (GitHub) | `tap` + `formula` or short form `tap:owner/repo/formula` | VersionResolver only | Fetches formula from GitHub. Parses Ruby for version/bottles. |
| `nixpkgs` | NixOS channels | None | VersionResolver, VersionLister | Channel versions (24.05, unstable). |
| Custom | Registry system | `source = "name"` | VersionResolver only | Catch-all for plugin sources. No ListVersions. |
| `fossil` | Fossil VCS timeline | From fossil_archive action | VersionResolver, VersionLister | Tag prefix stripping, version separator customization. |

## Strategy Priority (Evaluation Order)

- **PriorityKnownRegistry (100)**: Known ecosystem registries (npm, pypi, crates_io, etc.)
- **PriorityExplicitHint (90)**: Explicit `github_repo` field
- **PriorityExplicitSource (80)**: Custom `source = "..."` values
- **PriorityInferred (10)**: Auto-detection from install actions

## Auto-Detection Rules

Most providers auto-detect from actions: npm_install -> npm, cargo_install -> crates_io, pipx_install -> pypi, go_install -> goproxy, gem_install -> rubygems, cpan_install -> metacpan, homebrew -> homebrew, github_archive/github_file -> github.

When auto-detection works, the [version] section can be omitted entirely.

## Template Variables

- `{version}` -- Normalized version (all providers)
- `{version.tag}` -- Original tag as resolved (all providers)
- `{version.url}` -- Download URL (Cask, Tap only)
- `{version.checksum}` -- SHA256 (Cask, Tap only)
- `{version.bottle_url}` -- Platform bottle URL (Tap only)

## ResolveWithinBoundary Logic

- Empty/latest constraint -> ResolveLatest()
- Channel pins -> ResolveVersion()
- VersionLister -> filters cached list by pin boundary
- VersionResolver-only -> fuzzy prefix matching
