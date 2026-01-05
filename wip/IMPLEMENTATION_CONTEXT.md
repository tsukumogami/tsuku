## Goal

Refactor test scripts in `test/scripts/` to use tsuku sandbox container building instead of manually building Dockerfiles with hardcoded apt-get calls.

## Context

Currently, scripts in `test/scripts/` build custom Dockerfiles that hardcode apt-get calls:

```dockerfile
RUN apt-get update && \
    apt-get install -y \
        wget \
        curl \
        ca-certificates \
        patchelf
```

This:
- Doesn't test the sandbox container building functionality (#770)
- Makes tests debian/ubuntu-specific
- Duplicates dependency management logic
- Doesn't validate that recipes correctly declare their dependencies

With #770 complete, we can declare these dependencies in recipes and let tsuku build containers with them.

## Affected Scripts

All scripts in `test/scripts/` that build Dockerfiles with apt-get calls:
- `test-checksum-pinning.sh`
- `test-cmake-provisioning.sh`
- `test-cuda-system-dep.sh`
- `test-docker-system-dep.sh`
- `test-homebrew-recipe.sh`
- `test-readline-provisioning.sh`

## Acceptance Criteria

- [ ] Remove apt-get calls from test Dockerfiles
- [ ] Create test recipes declaring system dependencies (wget, curl, ca-certificates, patchelf, etc.)
- [ ] Update test scripts to use `tsuku install --sandbox` with dependency recipes
- [ ] Verify tests pass using sandbox-built containers
- [ ] Tests work across distro families (debian, rhel) via multi-family recipe actions

## Example Transformation

**Before** (test-cmake-provisioning.sh):
```dockerfile
RUN apt-get update && \
    apt-get install -y \
        wget \
        curl \
        ca-certificates \
        patchelf
```

**After** (testdata/recipes/build-essentials.toml):
```toml
[[steps]]
action = "apt_install"
packages = ["wget", "curl", "ca-certificates", "patchelf"]

[[steps]]
action = "dnf_install"
packages = ["wget", "curl", "ca-certificates", "patchelf"]
```

**After** (test script):
```bash
# Let tsuku build container with dependencies
tsuku install --sandbox build-essentials

# Run tests in that container
...
```

## Benefits

- **Tests actual functionality**: Exercises sandbox container building end-to-end
- **Multi-family support**: Tests work on debian, rhel, etc.
- **Validates recipes**: Ensures recipes declare all needed dependencies
- **Golden plan coverage**: Can verify generated plans are family-specific

## Dependencies

Blocked by: #770 (sandbox container building - completed)
