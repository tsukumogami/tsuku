# Issue 726 Implementation Plan

## Summary

Add testdata recipes for SpatiaLite and its dependencies (GEOS, PROJ, libxml2) to exercise the full build ecosystem. These recipes demonstrate CMake builds, GitLab hosting, and complex dependency chains, completing the fossil_archive capability showcase.

## Research Findings

### Version Resolution

All four projects have Homebrew formulas, enabling dynamic version resolution:

| Project | Homebrew Formula | Current Version | Source |
|---------|-----------------|-----------------|--------|
| GEOS | geos | 3.14.1 | GitHub |
| PROJ | proj | 9.7.1 | GitHub |
| libxml2 | libxml2 | 2.15.1 | GitLab |
| SpatiaLite | libspatialite | 5.1.0 | Fossil |

### Download URL Patterns

| Project | URL Pattern |
|---------|-------------|
| GEOS | `https://github.com/libgeos/geos/archive/{version}.tar.gz` |
| PROJ | `https://github.com/OSGeo/PROJ/archive/{version}.tar.gz` |
| libxml2 | `https://download.gnome.org/sources/libxml2/{major}.{minor}/libxml2-{version}.tar.xz` |
| SpatiaLite | `https://www.gaia-gis.it/fossil/libspatialite/tarball/version-{version}/libspatialite.tar.gz` |

### Build Systems

| Project | Build System | Action |
|---------|-------------|--------|
| GEOS | CMake | `cmake_build` |
| PROJ | CMake | `cmake_build` |
| libxml2 | autoconf | `configure_make` |
| SpatiaLite | autoconf | `configure_make` |

### Dependency Chain

```
spatialite-source
├── sqlite-source (existing)
├── geos-source (new)
├── proj-source (new - depends on sqlite-source)
└── libxml2-source (new)
```

## Established Patterns

From existing testdata recipes:

1. **Naming convention**: `<tool>-source.toml`
2. **Recipe structure**: metadata -> version -> steps -> verify
3. **Version resolution**: Use `source = "homebrew"` with `formula = "<name>"` for reliable version lookup
4. **CMake builds**: Follow `ninja.toml` pattern - `download` + `extract` + `cmake_build` + `install_binaries`
5. **Complex dependencies**: Use `setup_build_env` action before build step (see `tk-source.toml`)
6. **Install binaries**: Always use `install_mode = "directory"` with explicit `binaries = ["bin/<name>"]`

## Files to Create/Modify

### New Files (4)

| File | Description |
|------|-------------|
| `testdata/recipes/geos-source.toml` | GEOS CMake build recipe |
| `testdata/recipes/proj-source.toml` | PROJ CMake build with sqlite dependency |
| `testdata/recipes/libxml2-source.toml` | libxml2 autoconf build recipe |
| `testdata/recipes/spatialite-source.toml` | SpatiaLite fossil_archive + full deps |

### Modified Files (1)

| File | Changes |
|------|---------|
| `docs/BUILD-ESSENTIALS.md` | Add fossil_archive reference in Build System Actions section |

## Implementation Steps

### Step 1: Create geos-source.toml [x]

Recipe pattern:
- Version: Homebrew `geos` formula
- Steps: download + extract + cmake_build + install_binaries
- Verify: `geos-config --version`

```toml
[metadata]
name = "geos-source"
description = "Geometry Engine - Open Source (source build for testing)"
homepage = "https://libgeos.org/"

[version]
source = "homebrew"
formula = "geos"

[[steps]]
action = "download"
url = "https://github.com/libgeos/geos/archive/{version}.tar.gz"
skip_verification_reason = "GitHub source archives don't provide checksums"

[[steps]]
action = "extract"
archive = "{version}.tar.gz"
format = "tar.gz"

[[steps]]
action = "cmake_build"
source_dir = "geos-{version}"
executables = ["geos-config"]
cmake_args = ["-DBUILD_TESTING=OFF"]

[[steps]]
action = "install_binaries"
install_mode = "directory"
binaries = ["bin/geos-config"]

[verify]
command = "geos-config --version"
pattern = "{version}"
```

### Step 2: Create proj-source.toml [x]

Recipe pattern:
- Version: Homebrew `proj` formula
- Dependencies: sqlite-source
- Steps: download + extract + setup_build_env + cmake_build + install_binaries
- Verify: `proj --version`

```toml
[metadata]
name = "proj-source"
description = "Cartographic projections library (source build for testing)"
homepage = "https://proj.org/"
dependencies = ["sqlite-source"]

[version]
source = "homebrew"
formula = "proj"

[[steps]]
action = "download"
url = "https://github.com/OSGeo/PROJ/archive/{version}.tar.gz"
skip_verification_reason = "GitHub source archives don't provide checksums"

[[steps]]
action = "extract"
archive = "{version}.tar.gz"
format = "tar.gz"

[[steps]]
action = "setup_build_env"

[[steps]]
action = "cmake_build"
source_dir = "PROJ-{version}"
executables = ["proj"]
cmake_args = ["-DBUILD_TESTING=OFF", "-DENABLE_TIFF=OFF", "-DENABLE_CURL=OFF"]

[[steps]]
action = "install_binaries"
install_mode = "directory"
binaries = ["bin/proj"]

[verify]
command = "proj"
pattern = "{version}"
```

### Step 3: Create libxml2-source.toml [x]

Recipe pattern:
- Version: Homebrew `libxml2` formula
- Steps: download + extract + configure_make + install_binaries
- Uses GNOME download server with major.minor path structure
- Verify: `xmllint --version`

```toml
[metadata]
name = "libxml2-source"
description = "GNOME XML library (source build for testing)"
homepage = "https://gitlab.gnome.org/GNOME/libxml2"

[version]
source = "homebrew"
formula = "libxml2"

[[steps]]
action = "download"
url = "https://download.gnome.org/sources/libxml2/2.15/libxml2-{version}.tar.xz"
skip_verification_reason = "GNOME FTP mirrors don't provide checksums for dynamic versions"

[[steps]]
action = "extract"
archive = "libxml2-{version}.tar.xz"
format = "tar.xz"

[[steps]]
action = "configure_make"
source_dir = "libxml2-{version}"
executables = ["xmllint"]
configure_args = ["--without-python", "--without-lzma", "--without-icu"]

[[steps]]
action = "install_binaries"
install_mode = "directory"
binaries = ["bin/xmllint"]

[verify]
command = "xmllint --version"
pattern = "{version}"
```

### Step 4: Create spatialite-source.toml [x]

Recipe pattern:
- Version: Homebrew `libspatialite` formula
- Dependencies: Full dependency chain (sqlite-source, geos-source, proj-source, libxml2-source)
- Steps: fossil_archive + setup_build_env + configure_make + install_binaries
- Verify: `spatialite --version` or similar

```toml
[metadata]
name = "spatialite-source"
description = "SpatiaLite spatial extensions for SQLite (source build for testing)"
homepage = "https://www.gaia-gis.it/fossil/libspatialite"
dependencies = ["sqlite-source", "geos-source", "proj-source", "libxml2-source"]

[version]
source = "homebrew"
formula = "libspatialite"

[[steps]]
action = "fossil_archive"
repo = "https://www.gaia-gis.it/fossil/libspatialite"
project_name = "libspatialite"
strip_dirs = 1

[[steps]]
action = "setup_build_env"

[[steps]]
action = "configure_make"
source_dir = "."
executables = ["spatialite"]
configure_args = ["--disable-freexl", "--disable-minizip", "--disable-gcp", "--disable-rttopo"]

[[steps]]
action = "install_binaries"
install_mode = "directory"
binaries = ["bin/spatialite"]

[verify]
command = "spatialite -version"
pattern = "{version}"
```

### Step 5: Update BUILD-ESSENTIALS.md [x]

Add `fossil_archive` to the Build System Actions section after `setup_build_env`:

```markdown
### fossil_archive

Downloads and extracts source archives from Fossil SCM repositories (https://fossil-scm.org/).

**Action:** `fossil_archive`

**Parameters:**
- `repo` (required): Fossil repository URL (must be HTTPS)
- `project_name` (required): Name used in tarball filename
- `tag_prefix` (optional): Prefix before version in tags (default: "version-")
- `version_separator` (optional): Separator in version numbers (default: ".")
- `strip_dirs` (optional): Directories to strip from archive (default: 1)

**Example recipe:**
```toml
[[steps]]
action = "fossil_archive"
repo = "https://sqlite.org/src"
project_name = "sqlite"
strip_dirs = 1
```

**URL construction:**
The action builds tarball URLs using the pattern: `{repo}/tarball/{tag}/{project_name}.tar.gz`

For example, with version `3.46.0` and `tag_prefix = "version-"`:
`https://sqlite.org/src/tarball/version-3.46.0/sqlite.tar.gz`

**See also:** [DESIGN-fossil-archive.md](DESIGN-fossil-archive.md) for detailed implementation
```

## Testing Strategy

### Unit Testing

Existing unit tests should pass (recipes are testdata, not production):
- `go test ./...`

### Integration Testing

Manual validation (CI can add this later):
```bash
# Build tsuku
go build -o tsuku ./cmd/tsuku

# Test individual recipes
./tsuku install geos-source
./tsuku install proj-source
./tsuku install libxml2-source
./tsuku install spatialite-source

# Verify installations
geos-config --version
proj
xmllint --version
spatialite -version
```

## Risk Assessment

### Low Risk
- Recipe structure follows established patterns
- All version sources (Homebrew) are proven reliable
- Build actions (cmake_build, configure_make) are well-tested

### Medium Risk
- **Complex dependency chain**: SpatiaLite depends on 4 other source builds
  - Mitigation: setup_build_env action handles environment configuration
- **libxml2 URL structure**: Uses major.minor in path, may need adjustment
  - Mitigation: Can fall back to static version if needed

### Blocking Questions

None. All patterns are established and all information is available.

## Summary

- **Files to create**: 4 new recipe files
- **Files to modify**: 1 documentation file
- **Approach**: Follow established patterns from ninja.toml (CMake) and tk-source.toml (fossil_archive + dependencies)
- **Implementation steps**: 5 sequential steps
- **Estimated effort**: Small to medium
