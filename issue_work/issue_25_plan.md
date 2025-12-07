# Issue 25 Implementation Plan

## Goal
Document all environment variables supported by tsuku.

## Environment Variables Found

### Core Configuration

| Variable | Default | Description | Source |
|----------|---------|-------------|--------|
| `TSUKU_HOME` | `~/.tsuku` | Base directory for all tsuku data | `internal/config/config.go:12` |
| `TSUKU_API_TIMEOUT` | `30s` | Timeout for API requests (1s-10m range) | `internal/config/config.go:15` |
| `TSUKU_REGISTRY_URL` | GitHub raw URL | Override registry URL for recipes | `internal/registry/registry.go:24` |

### Telemetry

| Variable | Default | Description | Source |
|----------|---------|-------------|--------|
| `TSUKU_NO_TELEMETRY` | (unset) | Disable telemetry when set to any value | `internal/telemetry/client.go:17` |
| `TSUKU_TELEMETRY_DEBUG` | (unset) | Print telemetry events to stderr instead of sending | `internal/telemetry/client.go:20` |

### Development/Debugging

| Variable | Default | Description | Source |
|----------|---------|-------------|--------|
| `TSUKU_DEBUG` | (unset) | Enable verbose debug output from actions | Used in `gem_install.go`, `go_install.go`, etc. |
| `GITHUB_TOKEN` | (unset) | GitHub API token for higher rate limits | `internal/version/resolver.go:156` |

## Implementation

Create `docs/ENVIRONMENT.md` with:
1. Overview of environment variables
2. Detailed documentation for each variable
3. Examples of usage
4. Reference to where defaults can be changed (config.toml for telemetry)

## Files to Create
- `docs/ENVIRONMENT.md` - Main environment variables documentation
