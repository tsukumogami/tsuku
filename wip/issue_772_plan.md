# Issue 772 Implementation Plan

## Summary

Migrate 3 recipes from `require_system` + `install_guide` to typed actions.

## Files to Modify

1. `internal/recipe/recipes/d/docker.toml`
2. `internal/recipe/recipes/c/cuda.toml`
3. `internal/recipe/recipes/t/test-tuples.toml`

## Migration Pattern

Replace `require_system` + `install_guide` with:
- `brew_cask` for macOS (implicit darwin constraint)
- `apt_install` for Linux/Debian (implicit debian constraint)
- `manual` for complex instructions
- `require_command` at end for verification

## Recipe Conversions

### docker.toml

**Current**:
```toml
[[steps]]
action = "require_system"
command = "docker"
[steps.install_guide]
darwin = "brew install --cask docker"
linux = "See https://docs.docker.com/engine/install/..."
```

**Target**:
```toml
[[steps]]
action = "brew_cask"
packages = ["docker"]

[[steps]]
action = "manual"
text = "See https://docs.docker.com/engine/install/ for Linux installation"
when = { os = "linux" }

[[steps]]
action = "require_command"
command = "docker"
```

### cuda.toml

**Current**: Both platforms are manual (CUDA requires NVIDIA hardware/drivers)

**Target**:
```toml
[[steps]]
action = "manual"
text = "CUDA is not supported on macOS. Consider using cloud GPU instances."
when = { os = "darwin" }

[[steps]]
action = "manual"
text = "Visit https://developer.nvidia.com/cuda-downloads for installation"
when = { os = "linux" }

[[steps]]
action = "require_command"
command = "nvcc"
min_version = "11.0"
version_flag = "--version"
version_regex = "release ([0-9.]+)"
```

### test-tuples.toml

**Current**: Uses platform tuples

**Target**:
```toml
[[steps]]
action = "brew_cask"
packages = ["docker"]

[[steps]]
action = "apt_install"
packages = ["docker.io"]

[[steps]]
action = "require_command"
command = "docker"
```

## Validation

After each recipe change:
1. `go vet ./...`
2. `go test ./internal/recipe/...` - verify recipe parses
3. `./tsuku info <recipe>` - verify metadata loads
4. `./tsuku eval <recipe>` - verify plan generation

## Steps

1. Convert docker.toml
2. Convert cuda.toml
3. Convert test-tuples.toml
4. Run full test suite
5. Verify preflight validation passes for all
