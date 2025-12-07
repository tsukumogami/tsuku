# Issue 25 Summary

## What was done

Created comprehensive documentation for all environment variables supported by tsuku.

## Files Created

- `docs/ENVIRONMENT.md` - Environment variables documentation

## Environment Variables Documented

| Variable | Default | Purpose |
|----------|---------|---------|
| `TSUKU_HOME` | `~/.tsuku` | Base directory for all tsuku data |
| `TSUKU_API_TIMEOUT` | `30s` | HTTP API request timeout |
| `TSUKU_REGISTRY_URL` | GitHub raw URL | Override remote registry URL |
| `TSUKU_NO_TELEMETRY` | (unset) | Disable telemetry when set |
| `TSUKU_TELEMETRY_DEBUG` | (unset) | Print telemetry to stderr instead of sending |
| `TSUKU_DEBUG` | (unset) | Enable verbose debug output |
| `GITHUB_TOKEN` | (unset) | GitHub API token for higher rate limits |

## Testing

- Build: Pass
- Tests: 17 packages, all pass (cached)

## Notes

- Documentation follows existing patterns in `docs/` directory
- Includes directory structure diagram for `$TSUKU_HOME`
- Provides usage examples and default values for each variable
- Explains the relationship between `TSUKU_NO_TELEMETRY` and config.toml
