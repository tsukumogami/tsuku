# Issue 802 Summary

## What Was Implemented

Migrated `test-checksum-pinning.sh` and `test-homebrew-recipe.sh` to use tsuku sandbox containers instead of hardcoded apt-get Dockerfiles. The scripts now generate family-specific plans using `tsuku eval --install-deps` and run tests in isolated containers built from distribution-appropriate base images.

## Changes Made

- `test/scripts/test-checksum-pinning.sh`:
  - Removed ubuntu-specific apt-get Dockerfile
  - Added family argument (debian, rhel, arch, alpine, suse) defaulting to debian
  - Uses family-specific base images for container builds
  - Replaced jq-based JSON parsing with grep for binary_checksums verification
  - Generates plans using `tsuku eval fzf --linux-family $FAMILY --install-deps`

- `test/scripts/test-homebrew-recipe.sh`:
  - Removed ubuntu-specific apt-get Dockerfile
  - Added family argument for multi-family support
  - Uses `tsuku eval --install-deps` to get plans including patchelf dependency
  - Runs `tsuku install --plan --sandbox --force` directly for sandbox testing

## Key Decisions

- **Keep Docker for checksum tests**: The checksum pinning tests require running arbitrary commands inside the container (checking state.json, tampering with binaries, running verify). The sandbox infrastructure doesn't support custom commands, only installation verification.

- **Direct sandbox for homebrew tests**: The homebrew test only needs to verify installation succeeds, which is what the sandbox infrastructure provides. Converted to direct `tsuku install --sandbox` usage.

- **Replace jq with grep**: Removed jq dependency by using grep to check for "binary_checksums" string in state.json. The test doesn't need to parse JSON structure, just verify the field exists.

## Trade-offs Accepted

- **Still uses Docker directly for checksum tests**: The sandbox API doesn't support post-install command execution. Acceptable because the Docker usage is now family-aware using base images instead of hardcoded apt-get.

## Test Coverage

- No new unit tests required - these are integration test scripts
- Go unit tests continue to pass

## Known Limitations

- Checksum pinning tests build a custom Docker image rather than using sandbox-built containers. This is because the tests need to run custom verification commands after installation.

## Future Improvements

- Could add a `--run` flag to sandbox to execute arbitrary commands after installation, which would allow fully migrating test-checksum-pinning.sh
