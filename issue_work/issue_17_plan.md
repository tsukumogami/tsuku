# Issue #17 Implementation Plan

## Overview
Add `--json` flag to commands for machine-readable output suitable for parsing with tools like jq.

## Commands to Update
1. `list` - List installed tools
2. `info` - Show tool details
3. `versions` - List available versions
4. `outdated` - Check for updates
5. `search` - Search for tools

## Design Decisions

### Flag Pattern
Follow the existing `--quiet` pattern:
- Add global `jsonFlag bool` variable in main.go
- Use local `--json` flag per command (not global) since not all commands support it
- Check flag at start of command Run function

### JSON Output Structures

#### list
```json
{"tools": [{"name": "nodejs", "version": "22.11.0", "path": "/path/to/tool"}]}
```

#### info
```json
{
  "name": "nodejs",
  "description": "Node.js runtime",
  "homepage": "https://nodejs.org",
  "version_format": "semver",
  "status": "installed",
  "installed_version": "22.11.0",
  "location": "/path/to/tool",
  "verify_command": "node --version"
}
```

#### versions
```json
{"versions": ["22.11.0", "22.10.0", "21.7.3"]}
```

#### outdated
```json
{"updates": [{"name": "nodejs", "current": "22.10.0", "latest": "22.11.0"}]}
```

#### search
```json
{"results": [{"name": "nodejs", "description": "Node.js runtime", "installed": "22.11.0"}]}
```

### Implementation Approach
1. Add helper function `printJSON(v interface{})` that marshals and prints JSON
2. Add `--json` flag to each command in its init() function
3. At start of Run, check flag and switch to JSON output path
4. Keep existing human-readable output as default

## File Changes
1. `cmd/tsuku/helpers.go` - Add `printJSON` helper
2. `cmd/tsuku/list.go` - Add --json flag and JSON output
3. `cmd/tsuku/info.go` - Add --json flag and JSON output
4. `cmd/tsuku/versions.go` - Add --json flag and JSON output
5. `cmd/tsuku/outdated.go` - Add --json flag and JSON output
6. `cmd/tsuku/search.go` - Add --json flag and JSON output

## Interaction with --quiet
- `--json` and `--quiet` are independent
- When `--json` is set, output is JSON only (no informational messages)
- `--quiet` has no effect when `--json` is used (JSON mode is inherently quiet for non-JSON output)

## Testing
- Manual testing of each command with --json flag
- Verify output is valid JSON with jq
- Verify pipe-ability: `tsuku list --json | jq '.tools[].name'`
