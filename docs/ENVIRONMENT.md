# Environment Variables

This document describes all environment variables recognized by tsuku.

## Core Configuration

### TSUKU_HOME

Base directory for all tsuku data.

- **Default:** `~/.tsuku`
- **Example:** `export TSUKU_HOME=/opt/tsuku`

When set, tsuku stores all data (installed tools, cached recipes, configuration) under this directory instead of `~/.tsuku`.

Directory structure:
```
$TSUKU_HOME/
├── tools/          # Installed tools
│   └── current/    # Symlinks to active versions
├── apps/           # Installed macOS applications (.app bundles)
├── libs/           # Shared libraries
├── recipes/        # Local recipe overrides
├── registry/       # Cached recipes from remote registry
└── config.toml     # User configuration
```

### TSUKU_API_TIMEOUT

Timeout for HTTP API requests to version providers (GitHub, PyPI, crates.io, etc.).

- **Default:** `30s`
- **Valid range:** `1s` to `10m`
- **Format:** Go duration string (e.g., `30s`, `1m`, `2m30s`)
- **Example:** `export TSUKU_API_TIMEOUT=60s`

If the value is invalid, too low, or too high, a warning is printed and the appropriate bound is used.

### TSUKU_REGISTRY_URL

Override the URL for fetching recipes from the remote registry.

- **Default:** `https://raw.githubusercontent.com/tsukumogami/tsuku/main/internal/recipe`
- **Example:** `export TSUKU_REGISTRY_URL=https://example.com/registry`

Useful for:
- Testing custom recipe repositories
- Using a mirror or fork of the official registry
- Air-gapped environments with a local registry

## Telemetry

Tsuku collects anonymous usage telemetry to help improve the tool. See `tsuku telemetry` for more information.

### TSUKU_NO_TELEMETRY

Disable telemetry collection.

- **Default:** (unset - telemetry enabled)
- **Example:** `export TSUKU_NO_TELEMETRY=1`

When set to any non-empty value, tsuku will not send any telemetry data. This takes precedence over the `telemetry` setting in `config.toml`.

### TSUKU_TELEMETRY_DEBUG

Enable telemetry debug mode.

- **Default:** (unset)
- **Example:** `export TSUKU_TELEMETRY_DEBUG=1`

When set, telemetry events are printed to stderr instead of being sent to the telemetry server. Useful for seeing what data would be collected without actually sending it.

## Development and Debugging

### TSUKU_DEBUG

Enable verbose debug output.

- **Default:** (unset)
- **Example:** `export TSUKU_DEBUG=1`

When set, tsuku prints additional debug information during operations, such as:
- Output from package manager commands (go install, cargo install, etc.)
- Wrapper script generation details
- Other internal debugging information

### GITHUB_TOKEN

GitHub personal access token for API requests.

- **Default:** (unset - anonymous requests)
- **Example:** `export GITHUB_TOKEN=ghp_xxxxxxxxxxxxxxxxxxxx`

GitHub's API has rate limits:
- Anonymous: 60 requests/hour
- Authenticated: 5,000 requests/hour

If you're installing many tools or hitting rate limits, set this variable to a personal access token with no special permissions (public repo access only is sufficient).

To create a token:
1. Go to https://github.com/settings/tokens
2. Click "Generate new token (classic)"
3. Select no scopes (public repo access is default)
4. Copy the token and set the environment variable

## Summary Table

| Variable | Default | Description |
|----------|---------|-------------|
| `TSUKU_HOME` | `~/.tsuku` | Base directory for tsuku data |
| `TSUKU_API_TIMEOUT` | `30s` | HTTP API request timeout |
| `TSUKU_REGISTRY_URL` | GitHub | Remote registry URL |
| `TSUKU_NO_TELEMETRY` | (unset) | Disable telemetry when set |
| `TSUKU_TELEMETRY_DEBUG` | (unset) | Print telemetry to stderr |
| `TSUKU_DEBUG` | (unset) | Enable verbose debug output |
| `GITHUB_TOKEN` | (unset) | GitHub API token |
