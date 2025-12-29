# Issue 725 Implementation Plan

## Goal
Add testdata recipes for fossil-source, tcl-source, and tk-source demonstrating fossil_archive with different configurations.

## Recipes to Create

### 1. fossil-source.toml
- Builds Fossil SCM from https://fossil-scm.org/home
- Uses default tag_prefix ("version-") and version_separator (".")
- Reference: Fossil uses `version-X.Y` tags

### 2. tcl-source.toml  
- Builds Tcl from https://core.tcl-lang.org/tcl
- Uses `tag_prefix = "core-"` and `version_separator = "-"` for `core-X-Y-Z` tags
- Build from unix/ subdirectory

### 3. tk-source.toml
- Builds Tk from https://core.tcl-lang.org/tk
- Dependencies: ["tcl-source"]
- Same tag format as Tcl

## Pattern to Follow
Follow sqlite-source.toml structure:
1. [metadata] with name, description, homepage, dependencies (if any)
2. [version] with fossil_repo, project_name, tag_prefix, version_separator
3. [[steps]] for fossil_archive
4. [[steps]] for configure_make
5. [[steps]] for install_binaries
6. [verify] command

## Design Doc Update
Update docs/DESIGN-fossil-archive.md to mark #724 and #725 as done in the dependency graph.

## Testing
Build and test each recipe locally before committing.
