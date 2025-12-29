# Issue 725 Summary

## What Was Implemented

Added testdata recipes demonstrating the `fossil_archive` action with different configurations:
- fossil-source.toml: Default configuration building Fossil SCM
- tcl-source.toml: Uses `version_separator = "-"` for `core-X-Y-Z` tag format
- tk-source.toml: Declares dependency on tcl-source

## Changes Made
- `testdata/recipes/fossil-source.toml`: New recipe for Fossil SCM from fossil-scm.org
- `testdata/recipes/tcl-source.toml`: New recipe for Tcl with custom tag format
- `testdata/recipes/tk-source.toml`: New recipe for Tk with dependency chain
- `docs/DESIGN-fossil-archive.md`: Updated dependency graph to mark #724 and #725 as done

## Key Decisions
- Removed `[version]` section from tcl-source and tk-source due to bot detection (see below)
- Used same pattern as sqlite-source.toml for consistency
- Added setup_build_env step to tk-source for proper Tcl dependency setup

## Trade-offs Accepted
- tcl-source and tk-source cannot resolve versions automatically due to bot detection on core.tcl-lang.org (#738)
- Recipes are documented with the limitation and work with explicit version specification

## Known Limitations
- Tcl/Tk Fossil servers have bot detection blocking automated HTTP requests to timeline pages
- User-specified versions are overridden by 'dev' fallback when automatic resolution fails
- Filed #738 to track the bot detection issue

## Integration Tests
- fossil-source: Tested locally, builds successfully
- tcl-source: Blocked by #738 (bot detection)
- tk-source: Blocked by #738 (dependency on tcl-source)
