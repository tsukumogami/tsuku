# Issue 85 Implementation Plan

## Summary

Add `tsuku config get/set` commands with telemetry as the first configurable setting. Config stored in `~/.tsuku/config.toml`, with env vars taking precedence.

## Approach

Create a user configuration package that reads/writes TOML files, then add CLI commands to interact with it. Integrate with telemetry so config-based opt-out works alongside env var opt-out.

### Key Decisions

1. **TOML format**: Matches Go ecosystem conventions and is human-readable
2. **Single config file**: `~/.tsuku/config.toml` keeps things simple
3. **Env var precedence**: `TSUKU_NO_TELEMETRY=1` overrides config file
4. **Lazy loading**: Config loaded only when needed, not at startup

## Files to Create

- `internal/userconfig/userconfig.go` - Config file operations (load, save, get, set)
- `internal/userconfig/userconfig_test.go` - Unit tests
- `cmd/tsuku/config.go` - `config` parent command with `get` and `set` subcommands

## Files to Modify

- `cmd/tsuku/main.go` - Register config command
- `internal/telemetry/client.go` - Check userconfig in addition to env var
- `internal/telemetry/notice.go` - Update notice text to mention config option

## Implementation Steps

- [ ] Step 1: Create userconfig package
  - Define Config struct with Telemetry bool field
  - Load() reads from config.toml, returns defaults if missing
  - Save() writes to config.toml
  - Get(key) returns value as string
  - Set(key, value) updates and saves
  - Respect TSUKU_HOME for config file location

- [ ] Step 2: Add config commands
  - Create `cmd/tsuku/config.go`
  - `config get <key>` - prints current value
  - `config set <key> <value>` - updates config file
  - Help text documents available settings
  - Register in main.go

- [ ] Step 3: Integrate with telemetry
  - Update NewClient() to check userconfig.Load().Telemetry
  - Env var still takes precedence (checked first)
  - Update notice text to mention `tsuku config set telemetry false`

- [ ] Step 4: Add tests
  - Unit tests for userconfig package
  - Test env var precedence over config file

## Testing Strategy

- Unit tests: Test config load/save, get/set operations
- Integration: Verify telemetry respects config setting
- Manual: Run `tsuku config get telemetry`, `tsuku config set telemetry false`

## Acceptance Criteria from Issue

- [x] `tsuku config get <key>` displays current value
- [x] `tsuku config set <key> <value>` updates config
- [x] Config stored in `~/.tsuku/config.toml`
- [x] Config respects `TSUKU_HOME` environment variable
- [x] `tsuku config set telemetry false` disables telemetry
- [x] Telemetry client checks config file in addition to env var
- [x] Env var takes precedence over config file
- [x] First-run notice updated to mention config option
- [x] Help text documents available settings
- [x] Unit tests for config commands and telemetry integration
