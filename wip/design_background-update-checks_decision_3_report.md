# Decision 3: Configuration surface

## Question

What is the [updates] configuration surface: struct fields, TOML keys, env var overrides, defaults, validation, and precedence chain implementation?

## Options Considered

### Option A: Flat struct with getter methods (LLMConfig pattern)

Add `UpdatesConfig` as a nested struct in `userconfig.Config` with pointer-typed fields and getter methods on `Config`. Each getter checks env var, then config value, then returns a default. The precedence chain's upper layers (CLI flags, `.tsuku.toml`) are handled at call sites.

```go
type UpdatesConfig struct {
    Enabled            *bool   `toml:"enabled,omitempty"`
    AutoApply          *bool   `toml:"auto_apply,omitempty"`
    CheckInterval      *string `toml:"check_interval,omitempty"`
    NotifyOutOfChannel *bool   `toml:"notify_out_of_channel,omitempty"`
    SelfUpdate         *bool   `toml:"self_update,omitempty"`
}
```

Getter pattern (on `*Config`):
```go
func (c *Config) UpdatesEnabled() bool {
    if os.Getenv("TSUKU_NO_UPDATE_CHECK") == "1" { return false }
    if c.Updates.Enabled == nil { return true }
    return *c.Updates.Enabled
}
```

**Pros:**
- Directly matches LLMConfig, the most recent pattern in the codebase.
- Pointer types distinguish "not set" from "set to default value" for TOML serialization.
- Getter methods are the single place env var overrides live -- callers don't need to know about env vars.
- Integrates into existing `Get`/`Set`/`AvailableKeys` infrastructure with minimal changes.

**Cons:**
- Each field needs its own getter, adding ~5 methods. This is boilerplate but consistent with existing code.
- CLI flag and `.tsuku.toml` precedence still handled outside this layer -- callers must compose.

### Option B: Resolver struct that encapsulates the full precedence chain

Introduce a `ResolvedUpdatesConfig` that takes CLI flags, env vars, project config, and user config as inputs and produces final resolved values. A single `Resolve()` function handles all precedence.

```go
type ResolvedUpdatesConfig struct {
    Enabled            bool
    AutoApply          bool
    CheckInterval      time.Duration
    NotifyOutOfChannel bool
    SelfUpdate         bool
}

func ResolveUpdatesConfig(cli CLIFlags, env EnvVars, project *ProjectConfig, user *UpdatesConfig) *ResolvedUpdatesConfig
```

**Pros:**
- All precedence logic in one function -- easy to test and reason about.
- Callers get final values directly, no further composition needed.

**Cons:**
- No precedent for this pattern anywhere in the codebase. LLMConfig, telemetry, auto_install_mode all use the getter pattern.
- Requires defining input structs (CLIFlags, EnvVars) that don't exist yet.
- Couples config resolution to CLI and project config packages, creating import cycles or requiring interfaces.
- Over-engineers the problem: the existing getter pattern handles env > config > default fine, and CLI/project layers are naturally handled at call sites where those values are already available.

### Option C: Config struct with env var overrides in a separate middleware layer

Store the raw `UpdatesConfig` in userconfig as in Option A, but add a separate `updateconfig` package that wraps it with env var resolution. The middleware reads env vars and overlays them onto the raw config.

```go
// package updateconfig
type EffectiveConfig struct {
    raw *userconfig.UpdatesConfig
}

func (e *EffectiveConfig) Enabled() bool {
    if os.Getenv("TSUKU_NO_UPDATE_CHECK") == "1" { return false }
    if e.raw.Enabled == nil { return true }
    return *e.raw.Enabled
}
```

**Pros:**
- Separates TOML persistence from runtime resolution.
- Could allow testing raw config without env var interference.

**Cons:**
- Splits the same concern across two packages for no practical gain.
- LLMConfig already embeds env var checks in getters on `Config` -- this would be inconsistent.
- Extra indirection: callers need the wrapper, not the raw config.
- Testing is already easy with `os.Setenv` in tests (used throughout the codebase).

### Option D: Non-pointer fields with sentinel values

Use plain types instead of pointers. Booleans default to false (Go zero value), so use a custom type or "unset" sentinels to detect missing config.

```go
type UpdatesConfig struct {
    Enabled            bool   `toml:"enabled"`
    AutoApply          bool   `toml:"auto_apply"`
    CheckInterval      string `toml:"check_interval"`
    NotifyOutOfChannel bool   `toml:"notify_out_of_channel"`
    SelfUpdate         bool   `toml:"self_update"`
}
```

**Pros:**
- Simpler field access (no nil checks).

**Cons:**
- Cannot distinguish "user set false" from "user didn't set anything." For `enabled`, the default is `true`, so a zero-value `false` would mean "disabled" when the user may not have written anything -- wrong behavior.
- TOML encoder would always emit all fields, even unset ones, cluttering `config.toml`.
- Breaks the pattern established by LLMConfig.
- Would require `DefaultConfig()` to pre-populate `true` values, but TOML decode overwrites the struct entirely for the section.

## Chosen

Option A: Flat struct with getter methods (LLMConfig pattern)

## Rationale

Option A is the right choice because it matches the existing codebase patterns exactly. LLMConfig already solves the same problem (pointer types for optional TOML values, getter methods with env var precedence, integration with `Get`/`Set`/`AvailableKeys`). Following this pattern means:

1. Contributors familiar with LLMConfig immediately understand UpdatesConfig.
2. The `tsuku config get/set updates.*` commands work through the existing `Get`/`Set` infrastructure.
3. Env var overrides live in getter methods, matching `LLMIdleTimeout()`.
4. CLI flag and `.tsuku.toml` precedence are handled at call sites where those values are already in scope, avoiding package coupling.

Option B's full resolver is a better abstraction in theory, but it doesn't exist anywhere in tsuku and would be the first of its kind. The getter pattern is proven and well-understood here. Option C splits one concern across two packages for no benefit. Option D can't distinguish unset from false.

The `check_interval` field stores a duration string (like `LLMConfig.IdleTimeout`) rather than a `time.Duration` because TOML doesn't have a native duration type and the BurntSushi encoder needs a string.

## Configuration Table

| TOML Key | Go Field | Type | Default | Env Override | CLI Override |
|----------|----------|------|---------|--------------|-------------|
| `updates.enabled` | `Enabled` | `*bool` | `true` | `TSUKU_NO_UPDATE_CHECK=1` (inverted: disables) | `--check-updates` (force check) |
| `updates.auto_apply` | `AutoApply` | `*bool` | `true` | `TSUKU_AUTO_UPDATE=1` (overrides CI=true suppression) | none |
| `updates.check_interval` | `CheckInterval` | `*string` | `"24h"` | `TSUKU_UPDATE_CHECK_INTERVAL` | none |
| `updates.notify_out_of_channel` | `NotifyOutOfChannel` | `*bool` | `true` | none | none |
| `updates.self_update` | `SelfUpdate` | `*bool` | `true` | none | none |

**CI interaction:** When `CI=true` is set, auto-apply and notifications are suppressed unless `TSUKU_AUTO_UPDATE=1` is also set. This logic lives in the `UpdatesAutoApplyEnabled()` getter, not in `UpdatesEnabled()`. Update checks themselves are still controlled by `TSUKU_NO_UPDATE_CHECK`.

## Go Struct

```go
// UpdatesConfig holds auto-update settings stored in the [updates] section of config.toml.
type UpdatesConfig struct {
    // Enabled controls whether update checks run at all.
    // Default is true. Overridden by TSUKU_NO_UPDATE_CHECK=1 (disables).
    Enabled *bool `toml:"enabled,omitempty"`

    // AutoApply controls whether discovered updates are installed automatically.
    // Default is true. Suppressed when CI=true unless TSUKU_AUTO_UPDATE=1.
    AutoApply *bool `toml:"auto_apply,omitempty"`

    // CheckInterval is the minimum time between update checks.
    // Accepts Go duration format (e.g., "24h", "12h", "30m").
    // Default is "24h". Range: 1h-720h (30d).
    // Overridden by TSUKU_UPDATE_CHECK_INTERVAL env var.
    CheckInterval *string `toml:"check_interval,omitempty"`

    // NotifyOutOfChannel controls whether users are notified about versions
    // outside their pin boundary (e.g., pinned to 1.x but 2.0 exists).
    // Default is true.
    NotifyOutOfChannel *bool `toml:"notify_out_of_channel,omitempty"`

    // SelfUpdate controls whether tsuku checks for and applies updates to itself.
    // Default is true.
    SelfUpdate *bool `toml:"self_update,omitempty"`
}
```

Added to the top-level Config struct:

```go
type Config struct {
    // ... existing fields ...

    // Updates contains auto-update configuration.
    Updates UpdatesConfig `toml:"updates,omitempty"`
}
```

## Getter Methods

```go
const (
    // DefaultCheckInterval is the default time between update checks.
    DefaultCheckInterval = 24 * time.Hour

    // MinCheckInterval is the minimum allowed check interval.
    MinCheckInterval = 1 * time.Hour

    // MaxCheckInterval is the maximum allowed check interval (30 days).
    MaxCheckInterval = 30 * 24 * time.Hour

    // EnvNoUpdateCheck disables all update checking when set to "1".
    EnvNoUpdateCheck = "TSUKU_NO_UPDATE_CHECK"

    // EnvAutoUpdate overrides CI detection for explicit opt-in when set to "1".
    EnvAutoUpdate = "TSUKU_AUTO_UPDATE"

    // EnvUpdateCheckInterval overrides the check interval from config.
    EnvUpdateCheckInterval = "TSUKU_UPDATE_CHECK_INTERVAL"

    // EnvCI is the standard CI environment variable.
    EnvCI = "CI"
)

// UpdatesEnabled returns whether update checks are enabled.
// TSUKU_NO_UPDATE_CHECK=1 disables regardless of config.
// Returns true if not explicitly set.
func (c *Config) UpdatesEnabled() bool {
    if os.Getenv(EnvNoUpdateCheck) == "1" {
        return false
    }
    if c.Updates.Enabled == nil {
        return true
    }
    return *c.Updates.Enabled
}

// UpdatesAutoApplyEnabled returns whether updates should be installed automatically.
// Suppressed in CI environments (CI=true) unless TSUKU_AUTO_UPDATE=1 overrides.
// Returns true if not explicitly set and not in CI.
func (c *Config) UpdatesAutoApplyEnabled() bool {
    // Explicit env override takes highest precedence
    if os.Getenv(EnvAutoUpdate) == "1" {
        return true
    }
    // CI suppresses auto-apply
    if strings.EqualFold(os.Getenv(EnvCI), "true") {
        return false
    }
    if c.Updates.AutoApply == nil {
        return true
    }
    return *c.Updates.AutoApply
}

// UpdatesCheckInterval returns the minimum time between update checks.
// TSUKU_UPDATE_CHECK_INTERVAL env var takes precedence over config.
// Returns DefaultCheckInterval (24h) if not configured.
// Out-of-range values are clamped with a stderr warning.
func (c *Config) UpdatesCheckInterval() time.Duration {
    // Check env var first
    if envVal := os.Getenv(EnvUpdateCheckInterval); envVal != "" {
        if d, err := time.ParseDuration(envVal); err == nil {
            return clampCheckInterval(d, EnvUpdateCheckInterval)
        }
        fmt.Fprintf(os.Stderr, "Warning: invalid %s value %q, using default %v\n",
            EnvUpdateCheckInterval, envVal, DefaultCheckInterval)
        return DefaultCheckInterval
    }

    // Check config value
    if c.Updates.CheckInterval != nil {
        if d, err := time.ParseDuration(*c.Updates.CheckInterval); err == nil {
            return clampCheckInterval(d, "updates.check_interval")
        }
    }

    return DefaultCheckInterval
}

// clampCheckInterval enforces the 1h-30d range, logging warnings for out-of-range values.
func clampCheckInterval(d time.Duration, source string) time.Duration {
    if d < MinCheckInterval {
        fmt.Fprintf(os.Stderr, "Warning: %s too low (%v), using minimum %v\n",
            source, d, MinCheckInterval)
        return MinCheckInterval
    }
    if d > MaxCheckInterval {
        fmt.Fprintf(os.Stderr, "Warning: %s too high (%v), using maximum %v\n",
            source, d, MaxCheckInterval)
        return MaxCheckInterval
    }
    return d
}

// UpdatesNotifyOutOfChannel returns whether out-of-channel version notifications are shown.
// Returns true if not explicitly set.
func (c *Config) UpdatesNotifyOutOfChannel() bool {
    if c.Updates.NotifyOutOfChannel == nil {
        return true
    }
    return *c.Updates.NotifyOutOfChannel
}

// UpdatesSelfUpdate returns whether tsuku should check for and apply self-updates.
// Returns true if not explicitly set.
func (c *Config) UpdatesSelfUpdate() bool {
    if c.Updates.SelfUpdate == nil {
        return true
    }
    return *c.Updates.SelfUpdate
}
```

## Validation

Validation happens at two points:

**1. On load (getter methods):** Invalid duration strings in `check_interval` silently fall back to the default (24h). This matches `LLMIdleTimeout()` behavior -- parse errors return the default without failing the load.

**2. On set (`Set` method):** The `Set` method validates values before storing:

```go
case "updates.enabled", "updates.auto_apply", "updates.notify_out_of_channel", "updates.self_update":
    b, err := strconv.ParseBool(value)
    if err != nil {
        return fmt.Errorf("invalid value for %s: must be true or false", lowerKey)
    }
    // set the appropriate pointer field

case "updates.check_interval":
    d, err := time.ParseDuration(value)
    if err != nil {
        return fmt.Errorf("invalid value for updates.check_interval: must be a duration (e.g., 24h, 12h)")
    }
    if d < MinCheckInterval || d > MaxCheckInterval {
        return fmt.Errorf("invalid value for updates.check_interval: must be between %v and %v", MinCheckInterval, MaxCheckInterval)
    }
    c.Updates.CheckInterval = &value
```

**Range validation:**
- `check_interval`: parsed as `time.Duration`, must be >= 1h and <= 720h (30d). Out-of-range values in env vars or config are clamped with warnings. Out-of-range values via `tsuku config set` are rejected with an error.
- Boolean fields: accept standard Go `strconv.ParseBool` values (true/false, 1/0, t/f, yes/no).

**TOML decode errors:** If the `[updates]` section has invalid TOML syntax, the entire config load fails (existing behavior for all sections). Individual fields with wrong types (e.g., `enabled = "maybe"`) cause a TOML decode error.

## Assumptions

1. The `Config` struct in `userconfig.go` is the right home for `UpdatesConfig`. No new package is needed.
2. `TSUKU_NO_UPDATE_CHECK=1` is a kill switch that disables all checking. It does not affect `tsuku update` (explicit manual command) or `tsuku outdated`.
3. `TSUKU_AUTO_UPDATE=1` only overrides CI suppression of auto-apply. It doesn't force-enable updates if the user set `enabled = false` in config.
4. CLI flags (`--quiet`, `--check-updates`) are handled at command call sites, not in the config layer. The getter methods don't know about CLI flags.
5. `.tsuku.toml` project-level constraints are resolved at the update application layer, not in the config layer. The config layer provides global policy; per-tool decisions happen elsewhere.
6. The `clampCheckInterval` pattern (warn and clamp) follows `config.go`'s approach for `GetAPITimeout`, `GetVersionCacheTTL`, etc.
