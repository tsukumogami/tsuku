# Issue 726 Introspection

## Context Reviewed
- Design doc: `docs/DESIGN-fossil-archive.md`
- Sibling issues reviewed: #724, #725
- Prior patterns identified:
  - Recipe structure (metadata, version, steps, verify)
  - Action sequence: `fossil_archive` or `download` -> build action -> `install_binaries`
  - Testdata recipe naming: `<tool>-source.toml`
  - Version resolution: Either `fossil_repo` block or `source = "homebrew"` fallback
  - Dependency declaration: `dependencies = ["tool-source"]`

## Gap Analysis

### Minor Gaps

1. **Recipe pattern established in #725**: `tk-source.toml` uses `setup_build_env` action before `configure_make` for proper dependency environment. SpatiaLite recipe should follow this pattern given its complex dependency chain.

2. **Install binaries pattern**: All sibling recipes use `install_mode = "directory"` with explicit `binaries = ["bin/<executable>"]` pattern. Issue acceptance criteria references just `spatialite` but the actual binary install path should follow this pattern.

3. **Version resolution approach for non-Fossil sources**:
   - GEOS and PROJ are on GitHub - should use `source = "github"` or `source = "homebrew"` for version resolution
   - libxml2 is on GitLab - needs explicit version pinning or Homebrew version source
   - Design doc suggests `github_archive` action but this action does not exist. Should use `download` + `extract` pattern instead (as shown in `ninja.toml`).

4. **cmake_build action exists**: The cmake_build action is available in `internal/actions/cmake_build.go` and has well-defined parameters (source_dir, executables, cmake_args, build_type).

5. **No github_archive action**: Design doc mentions `github_archive` action but grep shows no such action exists. GEOS and PROJ recipes should use `download` + `extract` + `cmake_build` pattern instead (like `ninja.toml`).

6. **Documentation update scope**: BUILD-ESSENTIALS.md should add `fossil_archive` to the list of actions/usage patterns. The document currently only covers `configure_make`, `cmake_build`, and `setup_build_env`.

### Moderate Gaps

None identified. All gaps are minor and can be resolved by following patterns from existing recipes.

### Major Gaps

None identified.

## Recommendation

**Proceed**

## Proposed Amendments

Incorporating minor gaps into implementation approach:

1. **GEOS and PROJ recipes**: Use `download` + `extract` + `cmake_build` pattern (not `github_archive` which doesn't exist). Example from ninja.toml shows the correct pattern.

2. **libxml2 recipe**: Use `download` + `extract` + `configure_make` pattern with either Homebrew version resolution or pinned version.

3. **SpatiaLite recipe**: Include `setup_build_env` action (like tk-source.toml) before configure_make to ensure proper dependency environment configuration.

4. **All recipes**: Follow the established `install_binaries` pattern with `install_mode = "directory"`.

## Implementation Notes

The recipes should be structured as:

### geos-source.toml
```toml
[version]
source = "github"
owner = "libgeos"
repo = "geos"

[[steps]]
action = "download"
url = "https://github.com/libgeos/geos/archive/{version}.tar.gz"

[[steps]]
action = "extract"
...

[[steps]]
action = "cmake_build"
source_dir = "geos-{version}"
executables = ["geos-config"]  # or appropriate executables
```

### proj-source.toml
```toml
[version]
source = "github"
owner = "OSGeo"
repo = "PROJ"
dependencies = ["sqlite-source"]

# Similar download + extract + cmake_build pattern
```

### libxml2-source.toml
```toml
[version]
source = "homebrew"
formula = "libxml2"

[[steps]]
action = "download"
url = "https://download.gnome.org/sources/libxml2/{major}.{minor}/libxml2-{version}.tar.xz"

# Use configure_make pattern
```

### spatialite-source.toml
```toml
dependencies = ["sqlite-source", "geos-source", "proj-source", "libxml2-source"]

[[steps]]
action = "fossil_archive"
...

[[steps]]
action = "setup_build_env"  # Important for dependency chain

[[steps]]
action = "configure_make"
...
```
