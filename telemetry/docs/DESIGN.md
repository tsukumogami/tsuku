# tsuku-telemetry Design

## Overview

Cloudflare Worker that receives telemetry events from tsuku CLI and stores them in Cloudflare Analytics Engine.

## Architecture

```
┌─────────────┐     ┌──────────────────────────┐     ┌─────────────────┐
│ tsuku CLI   │────▶│ telemetry.tsuku.dev      │────▶│ Analytics Engine│
│             │     │ (Cloudflare Worker)      │     │                 │
└─────────────┘     └──────────────────────────┘     └─────────────────┘
                              │
                              ▼
                    ┌──────────────────┐
                    │ /stats endpoint  │
                    │ (aggregated JSON)│
                    └──────────────────┘
```

## Directory Structure

```
telemetry/
├── src/
│   └── index.ts          # Worker code
├── wrangler.toml         # Cloudflare config
├── package.json
├── tsconfig.json
└── README.md
```

The telemetry service is part of the tsuku monorepo. Deployment is handled by the monorepo CI workflow.

## Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/event` | POST | Receive telemetry events from CLI |
| `/stats` | GET | Return aggregated public statistics |
| `/stats/recipe/:name` | GET | Stats for specific recipe |
| `/health` | GET | Health check |

## Event Schema

### Blob Positions

All events use a consistent blob ordering for Analytics Engine storage:

| Position | Field | Description | Values |
|----------|-------|-------------|--------|
| 0 | action | Event type | `"install"` \| `"update"` \| `"remove"` \| `"create"` \| `"command"` |
| 1 | recipe | Recipe name | Recipe name or `""` for non-recipe actions |
| 2 | version_constraint | User-provided constraint | `"@LTS"`, `">=1.0"`, `""` |
| 3 | version_resolved | Actual version installed | e.g., `"22.0.0"` or `""` |
| 4 | version_previous | Previous version | For update/remove, else `""` |
| 5 | os | Operating system | `"linux"` \| `"darwin"` |
| 6 | arch | Architecture | `"amd64"` \| `"arm64"` |
| 7 | tsuku_version | CLI version | e.g., `"0.3.0"` |
| 8 | is_dependency | Dependency flag | `"true"` \| `"false"` \| `""` |
| 9 | command | Command name | For command action, else `""` |
| 10 | flags | Command flags | For command action, else `""` |
| 11 | template | Template name | For create action, else `""` |
| 12 | schema_version | Schema version | `"1"` |

### Index

- For `install`, `update`, `remove`: Index is the recipe name
- For `create`, `command`: Index is the action type

### TypeScript Interface

```typescript
interface TelemetryEvent {
  action: "install" | "update" | "remove" | "create" | "command";
  recipe?: string;
  version_constraint?: string;
  version_resolved?: string;
  version_previous?: string;
  os: string;
  arch: string;
  tsuku_version: string;
  is_dependency?: boolean;
  command?: string;
  flags?: string;
  template?: string;
}

const SCHEMA_VERSION = "1";
```

### Example Events

**Direct install with version constraint:**
```json
{
  "blobs": ["install", "nodejs", "@LTS", "22.0.0", "", "linux", "amd64", "0.3.0", "false", "", "", "", "1"],
  "indexes": ["nodejs"]
}
```

**Dependency install:**
```json
{
  "blobs": ["install", "nodejs", "", "22.0.0", "", "linux", "amd64", "0.3.0", "true", "", "", "", "1"],
  "indexes": ["nodejs"]
}
```

**Update (version change):**
```json
{
  "blobs": ["update", "nodejs", "", "22.1.0", "22.0.0", "linux", "amd64", "0.3.0", "", "", "", "", "1"],
  "indexes": ["nodejs"]
}
```

**Remove:**
```json
{
  "blobs": ["remove", "nodejs", "", "", "22.0.0", "linux", "amd64", "0.3.0", "", "", "", "", "1"],
  "indexes": ["nodejs"]
}
```

**Recipe creation:**
```json
{
  "blobs": ["create", "my-tool", "", "", "", "linux", "amd64", "0.3.0", "", "", "", "github_release", "1"],
  "indexes": ["create"]
}
```

**Command tracking:**
```json
{
  "blobs": ["command", "", "", "", "", "linux", "amd64", "0.3.0", "", "list", "--json", "", "1"],
  "indexes": ["command"]
}
```

### Validation Rules

| Action | Required Fields | Optional Fields | Must Be Empty |
|--------|-----------------|-----------------|---------------|
| `install` | recipe, version_resolved, os, arch, tsuku_version | version_constraint, is_dependency | command, flags, template |
| `update` | recipe, version_resolved, version_previous, os, arch, tsuku_version | version_constraint | is_dependency, command, flags, template |
| `remove` | recipe, version_previous, os, arch, tsuku_version | - | version_constraint, version_resolved, is_dependency, command, flags, template |
| `create` | template, os, arch, tsuku_version | - | recipe, version_*, is_dependency, command, flags |
| `command` | command, os, arch, tsuku_version | flags | recipe, version_*, is_dependency, template |

Invalid events return 400 Bad Request and are not written to Analytics Engine.

## Schema Versioning

The `schema_version` field (blob 12) enables future schema evolution:

- **Current version**: `"1"` - the schema defined in this document
- **Forward compatibility**: Reserved blob positions (13-19) for future fields
- **Breaking changes**: Increment schema_version, update Worker to handle both versions during transition
- **Query filtering**: Use `WHERE blob12 = '1'` to ensure consistent field semantics

## Stats Response Format

```json
{
  "generated_at": "2024-11-27T12:00:00Z",
  "period": "last_30_days",
  "total_installs": 15234,
  "recipes": [
    { "name": "nodejs", "installs": 2341, "updates": 123 },
    { "name": "terraform", "installs": 1892, "updates": 89 },
    { "name": "kubectl", "installs": 1654, "updates": 201 }
  ],
  "by_os": {
    "linux": 12000,
    "darwin": 3234
  },
  "by_arch": {
    "amd64": 14000,
    "arm64": 1234
  }
}
```

## Query Examples

**Top recipes by install:**
```sql
SELECT blob1 as recipe, COUNT(*) as installs
FROM tsuku_telemetry
WHERE blob0 = 'install'
GROUP BY blob1
ORDER BY installs DESC
LIMIT 20
```

**Version constraint usage:**
```sql
SELECT blob2 as constraint, COUNT(*) as count
FROM tsuku_telemetry
WHERE blob0 = 'install' AND blob2 != ''
GROUP BY blob2
```

**Dependency vs direct installs:**
```sql
SELECT
  blob1 as recipe,
  SUM(CASE WHEN blob8 = 'false' THEN 1 ELSE 0 END) as direct,
  SUM(CASE WHEN blob8 = 'true' THEN 1 ELSE 0 END) as dependency
FROM tsuku_telemetry
WHERE blob0 = 'install'
GROUP BY blob1
```

**Update frequency by recipe:**
```sql
SELECT blob1 as recipe, COUNT(*) as updates
FROM tsuku_telemetry
WHERE blob0 = 'update'
GROUP BY blob1
ORDER BY updates DESC
```

**OS and architecture distribution:**
```sql
SELECT blob5 as os, blob6 as arch, COUNT(*) as count
FROM tsuku_telemetry
WHERE blob0 = 'install'
GROUP BY blob5, blob6
```

## Cloudflare Configuration

```toml
# wrangler.toml
name = "tsuku-telemetry"
main = "src/index.ts"
compatibility_date = "2024-01-01"

[[analytics_engine_datasets]]
binding = "ANALYTICS"
dataset = "tsuku_telemetry"
```

## Deployment

### GitHub Actions

```yaml
name: Deploy
on:
  push:
    branches: [main]

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: cloudflare/wrangler-action@v3
        with:
          apiToken: ${{ secrets.CLOUDFLARE_API_TOKEN }}
```

### Manual

```bash
npx wrangler deploy
```

## Security Considerations

- No authentication required (public write endpoint)
- Rate limiting handled by Cloudflare automatically
- No PII stored
- Analytics Engine handles data retention (3 months)
- CORS enabled for stats endpoint (browser access)

### Data Minimization

The schema collects only:
- Action type and recipe name (public information)
- Version information (public information)
- OS and architecture (broad categories, not identifying)
- tsuku version (public information)
- Command flags (names only, not values)

**NOT collected:**
- IP addresses
- User identifiers
- File paths
- Environment variables

## Cost

| Tier | Requests/Month | Cost |
|------|----------------|------|
| Free | Up to 100K/day | $0 |
| Paid | 10M included/month, then $0.25/million | $5/month base |
