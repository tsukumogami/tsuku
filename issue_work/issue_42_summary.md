# Issue #42: Add PyPI Builder for Python Packages

## Summary
Implemented a PyPIBuilder that generates recipes for Python packages from pypi.org, following the same pattern established by CargoBuilder and GemBuilder.

## Changes Made

### New Files
- `internal/builders/pypi.go` - PyPIBuilder implementation
- `internal/builders/pypi_test.go` - Unit tests for PyPIBuilder
- `.github/workflows/pypi-builder-tests.yml` - Integration tests (daily + on file changes)

### Modified Files
- `cmd/tsuku/create.go` - Added pypi to supported ecosystems

## Implementation Details

### PyPIBuilder
The PyPIBuilder:
1. Fetches package metadata from PyPI JSON API (`/pypi/<package>/json`)
2. Discovers executables by:
   - Parsing `project.scripts` from pyproject.toml on GitHub
   - Falling back to `tool.poetry.scripts` for Poetry projects
   - Using package name as executable if scripts cannot be discovered
3. Generates recipes with:
   - `pipx_install` action
   - Version source set to `pypi`
   - Verify command using discovered executable

### Security Measures
- Package name validation against path traversal and injection
- Response size limits to prevent memory exhaustion
- Timeout for pyproject.toml fetching

### Ecosystem Aliases
The `--from` flag accepts: `pypi`, `pypi.org`, `pip`, `python`

## Testing

### Unit Tests
All unit tests pass:
- `TestPyPIBuilder_Name`
- `TestPyPIBuilder_CanBuild` (valid, not found, invalid names)
- `TestPyPIBuilder_Build` (ruff recipe, fallback, not found)
- `TestIsValidPyPIPackageName`
- `TestPyPIBuilder_buildPyprojectURL`

### Manual Testing
```bash
# Create recipe for ruff
./tsuku create ruff --from pypi
# Recipe created with correct executables discovered from pyproject.toml

# Install ruff
./tsuku install ruff
# Installation successful

# Verify
~/.tsuku/tools/current/ruff --version
# ruff 0.14.7
```

### Integration Tests
The GitHub workflow tests:
- Recipe creation for `ruff` package
- Recipe contents verification
- Installation using generated recipe
- Verification that installed tool works

Runs on Linux and macOS daily at 8:00 UTC or on changes to builder files.

## Notes
- PyPI JSON API doesn't expose console_scripts directly, so we fetch pyproject.toml from GitHub
- Only GitHub repositories are supported for pyproject.toml discovery (GitLab, etc. fall back to package name)
