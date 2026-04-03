# Recipe TOML Format Catalog

Complete struct documentation from internal/recipe/types.go with annotated examples.

## [metadata] Section

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Recipe identifier, kebab-case |
| description | string | Recommended | Tool description |
| homepage | string | Optional | HTTPS-only URL |
| version_format | string | Optional | "raw" (default), "semver", "semver_full", "strip_v" |
| requires_sudo | bool | Optional | Default false |
| type | string | Optional | "tool" (default) or "library" |
| tier | int | Optional | 1=binary, 2=package manager, 3=nix |
| llm_validation | string | Optional | "skipped" or empty |
| supported_os | []string | Optional | ["darwin", "linux"] |
| supported_arch | []string | Optional | ["amd64", "arm64"] |
| supported_libc | []string | Optional | ["glibc"], ["musl"], or both |
| unsupported_platforms | []string | Optional | Exceptions: "os/arch" format |
| unsupported_reason | string | Optional | Explanation for constraints |
| dependencies | []string | Optional | Install-time deps (replaces implicit) |
| runtime_dependencies | []string | Optional | Runtime deps |
| extra_dependencies | []string | Optional | Additional install-time deps (extends implicit) |
| extra_runtime_dependencies | []string | Optional | Additional runtime deps |
| binaries | []string | Optional | Explicit binary paths |
| satisfies | map | Optional | Ecosystem -> package names (e.g., homebrew = ["dav1d"]) |

## [version] Section

| Field | Type | Description |
|-------|------|-------------|
| source | string | Provider name. Empty = infer from actions |
| github_repo | string | "owner/repo" for GitHub providers |
| tag_prefix | string | Prefix to strip from tags (e.g., "v") |
| module | string | Go module path (for goproxy) |
| formula | string | Homebrew formula name |
| cask | string | Homebrew Cask name |
| tap | string | Homebrew tap ("owner/repo") |
| fossil_repo | string | Fossil repo URL |
| project_name | string | Project name for Fossil |
| version_separator | string | Separator in Fossil version numbers |
| timeline_tag | string | Tag filter for Fossil timeline |

**Source Inference:** When source is empty, inferred from actions:
- npm_install -> npm, cargo_install -> crates_io, pipx_install -> pypi
- go_install -> goproxy, gem_install -> rubygems, cpan_install -> metacpan
- homebrew -> homebrew, github_archive/github_file -> github

## [[steps]] Array

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| action | string | Yes | Action name |
| phase | string | No | "install" (default), "post-install", "pre-remove", "pre-update" |
| when | table | No | Platform conditions |
| note | string | No | Human-readable note |
| dependencies | []string | No | Step-level dependencies |
| (other) | varies | No | Action-specific parameters |

## [steps.when] Clause

Platform filtering (mutually exclusive top-level):
- `platform` ([]string): Exact "os/arch" tuples
- `os` ([]string): OS-only filter (any arch)

Refinement dimensions:
- `arch` (string): Architecture
- `linux_family` (string): "debian", "rhel", "alpine", "arch", "suse"
- `libc` ([]string): ["glibc"], ["musl"]
- `gpu` ([]string): ["nvidia"], ["amd"], ["intel"], ["none"]
- `package_manager` (string): Runtime check ("brew", "apt", "dnf")

Logic: empty = match all. Multiple conditions AND together.

## [verify] Section

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| command | string | Yes (except libraries) | Verification command |
| pattern | string | No | Regex with {version} placeholder |
| mode | string | No | "version" (default) or "output" |
| reason | string | If output mode | Why version check isn't possible |
| version_format | string | No | Transform for extracted version |
| exit_code | int | No | Expected exit code (default: 0) |
| additional | []obj | No | Additional verification commands |

## [[resources]] Array (Optional)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| name | string | Yes | Unique resource identifier |
| url | string | Yes | Download URL |
| checksum | string | No | SHA256 |
| dest | string | Yes | Destination directory |

## [[patches]] Array (Optional)

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| url | string | Mutual exclusive with data | Patch URL |
| data | string | Mutual exclusive with url | Inline patch |
| checksum | string | Required for url | SHA256 |
| strip | int | No | -p flag (default 1) |
| subdir | string | No | Subdirectory to apply in |

## Validation Rules Summary

- name: required, kebab-case
- type: "tool", "library", or empty
- homepage: must be https://
- At least one step required
- Each step needs action field
- Path params checked for ../ traversal
- URLs validated for http/https
- SHA256 must be 64 hex chars
- Verify command required except for libraries
- Pattern should have {version} in version mode
- Dangerous patterns warned: rm, curl |, eval, exec, &&, ||

## Annotated Recipe Examples

(See plan_skills_exemplar-recipes-catalog.md for curated examples by category)
