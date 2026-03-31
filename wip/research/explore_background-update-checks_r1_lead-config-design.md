# Lead: How should the [updates] config section be structured?

## Findings

### Existing Config Patterns

The user config lives in `internal/userconfig/userconfig.go` using `github.com/BurntSushi/toml`. Current sections: top-level (telemetry, auto_install_mode, strict_registries), [llm], [secrets], [registries]. The LLM config uses pointer types for optional values and getter methods that check environment variables first.

The path-management config in `internal/config/config.go` handles env var overrides like TSUKU_API_TIMEOUT, TSUKU_VERSION_CACHE_TTL with duration parsing and range validation.

### Proposed [updates] Structure

```toml
[updates]
enabled = true              # bool, default true
auto_apply = true           # bool, default true (D1 from PRD)
check_interval = "24h"      # duration string, default "24h", range 1h-30d
notify_out_of_channel = true # bool, default true
self_update = true          # bool, default true
```

### Go Struct Design

Follow the LLMConfig pattern with pointer types:

```go
type UpdatesConfig struct {
    Enabled            *bool   `toml:"enabled"`
    AutoApply          *bool   `toml:"auto_apply"`
    CheckInterval      *string `toml:"check_interval"`
    NotifyOutOfChannel *bool   `toml:"notify_out_of_channel"`
    SelfUpdate         *bool   `toml:"self_update"`
}
```

Getter methods check env vars first, then config value, then default:
- `IsEnabled()` checks TSUKU_NO_UPDATE_CHECK first (inverted)
- `GetCheckInterval()` checks TSUKU_UPDATE_CHECK_INTERVAL first
- `IsAutoApplyEnabled()` checks TSUKU_AUTO_UPDATE first (for CI override)

### Env Var Overrides

From PRD:
- `TSUKU_NO_UPDATE_CHECK=1` -- disables all update checking (inverts enabled)
- `TSUKU_UPDATE_CHECK_INTERVAL` -- overrides check_interval
- `TSUKU_AUTO_UPDATE=1` -- overrides CI detection for explicit opt-in
- `CI=true` -- suppresses auto-apply and notifications (unless TSUKU_AUTO_UPDATE=1)

### Precedence Chain

CLI flag > env var > .tsuku.toml > config.toml > default

Enforced at usage sites (command handlers), not in the config layer. The config layer provides getter methods that handle env var > config.toml > default. CLI flags and .tsuku.toml are checked by the calling code.

### Validation

- check_interval: parse as time.Duration, reject if < 1h or > 30d (720h)
- Log warning for out-of-range values, fall back to default
- Match existing patterns from config.go timeout/TTL handling

## Implications

The config surface is straightforward -- follow existing patterns in userconfig.go. The interesting part is the precedence chain: .tsuku.toml version constraints implicitly disable auto-update for exact-pinned tools (handled by PinLevelFromRequested returning PinExact), while the global config controls the overall behavior.

## Surprises

The config and path-management systems are split across two packages (userconfig vs config). The [updates] section belongs in userconfig (TOML-parsed) but the env var override pattern follows config.go's approach.

## Open Questions

1. Should `auto_apply = false` still allow `tsuku update` to work manually? (Yes -- it only disables automatic application.)
2. Should there be a `quiet` config key, or is `--quiet` CLI-only? PRD suggests CLI-only.
3. How does `enabled = false` interact with `tsuku outdated`? Should outdated still work even if background checks are disabled?

## Summary

The [updates] config section should follow the LLMConfig pattern: pointer types for optional values, getter methods checking env vars first, duration strings with range validation. Five keys (enabled, auto_apply, check_interval, notify_out_of_channel, self_update) cover the PRD requirements. The precedence chain is enforced at usage sites, not in the config layer.
