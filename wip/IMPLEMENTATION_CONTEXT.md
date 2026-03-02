# Implementation Context: Issue #1968

## Summary
Create `$TSUKU_HOME/env` during `tsuku install` so the env file exists in any context (Docker, CI, scripted installs), not just when the host install script runs. Then simplify the sandbox Dockerfile generator to remove per-dependency PATH workarounds.

## Key Decisions
- Env file content matches `website/install.sh` format: `export PATH="$TSUKU_HOME/bin:$TSUKU_HOME/tools/current:$PATH"`
- The per-dep `ENV PATH` lines in foundation.go (line 127) are no longer needed because #1967 fixed tools/current symlinks
- The global `ENV PATH` at foundation.go line 116 already includes `tools/current` and `bin`, which is sufficient
- Env file creation is idempotent and cheap -- safe to call on every install

## Integration Points
- `internal/install/manager.go` - Add env file write in `InstallWithOptions`
- `internal/config/config.go` - Add `EnvFile()` path method and `EnsureEnvFile()` method
- `internal/sandbox/foundation.go` - Remove per-dep `ENV PATH` lines from `GenerateFoundationDockerfile`
- `internal/sandbox/foundation_test.go` - Update tests to not expect per-dep PATH lines

## Dependencies
- #1967 (closed) - tools/current symlinks must work for env-based PATH to be sufficient

## User Instructions
- Update sandbox mode to use this new mechanism
- Manually test with `tsuku install --sandbox` on recipes from PR #1956 that needed PATH workaround
