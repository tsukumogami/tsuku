# Distributed Recipe Reference

How to publish tsuku recipes from your own GitHub repository, outside the
central registry.

---

## Directory Setup

Create a `.tsuku-recipes/` directory at your repository root and add TOML files:

```
my-repo/
├── .tsuku-recipes/
│   ├── my-tool.toml
│   └── another-tool.toml      # multi-recipe repos
├── src/
└── README.md
```

No configuration file or registration step is required. tsuku discovers
recipes by probing the `.tsuku-recipes/` directory via GitHub's API.

### File Naming

- Use kebab-case filenames: `deploy-cli.toml`, `my-tool.toml`
- The filename (without .toml) must match the `metadata.name` field inside the file
- Mismatch causes confusion: tsuku uses the filename for discovery but the name field for state tracking

### Branch

Push recipes to `main` or `master`. tsuku probes these branches in order
when discovering recipes.

---

## Install Syntax

| Syntax | Example | Behavior |
|--------|---------|----------|
| `owner/repo` | `acme/my-tool` | Installs the default recipe (repo name as recipe name) |
| `owner/repo:recipe` | `acme/tools:deploy-cli` | Installs a named recipe from a multi-recipe repo |
| `owner/repo@version` | `acme/my-tool@v1.0.0` | Installs a specific version |
| `owner/repo:recipe@version` | `acme/tools:deploy-cli@2.1` | Named recipe at a specific version |

### Single-Recipe Repos

If your repo contains one recipe and the recipe name matches the repo name,
users install with just `owner/repo`:

```bash
tsuku install acme/my-tool
```

### Multi-Recipe Repos

When the repo contains multiple recipes, users specify which one after a colon:

```bash
tsuku install acme/internal-tools:deploy-cli
tsuku install acme/internal-tools:build-helper
```

---

## Optional Manifest (manifest.json)

For repos with many recipes or non-standard layouts, add a manifest:

```
.tsuku-recipes/
├── manifest.json
├── deploy-cli.toml
└── build-helper.toml
```

```json
{
  "layout": "flat",
  "index_url": "https://example.com/recipe-index.json"
}
```

| Field | Values | Description |
|-------|--------|-------------|
| `layout` | `"flat"` (default), `"grouped"` | `flat`: all TOMLs in `.tsuku-recipes/`. `grouped`: subdirectories per recipe. |
| `index_url` | URL | Pre-computed recipe index for faster discovery (optional) |

tsuku probes `.tsuku-recipes/manifest.json` first, then `recipes/manifest.json`
as a fallback. If neither exists, it defaults to flat layout.

---

## Trust Model

### First-Install Confirmation

Installing from an unregistered source prompts the user:

```
Install from unregistered source 'acme/internal-tools'? [y/N]
```

If approved, the source is auto-registered in `$TSUKU_HOME/config.toml`.
The `-y` flag skips the prompt.

### Pre-Registering Sources

```bash
tsuku registry add acme/internal-tools
```

Registered sources install without prompts. List registered sources with
`tsuku registry list`.

### Strict Mode

For CI, shared machines, or locked-down environments:

```toml
# $TSUKU_HOME/config.toml
strict_registries = true
```

When enabled:
- Unregistered sources are rejected immediately (no prompt)
- `-y` flag doesn't bypass the restriction
- Users must explicitly `tsuku registry add` before installing

### Removing a Source

```bash
tsuku registry remove acme/internal-tools
```

This removes the registry entry but doesn't uninstall tools already installed
from that source.

---

## Testing

### Validate Locally

```bash
tsuku validate .tsuku-recipes/my-tool.toml
tsuku validate --strict .tsuku-recipes/my-tool.toml
```

### Sandbox Install

Test in an isolated container without affecting the system:

```bash
# From a local recipe file
tsuku install --recipe .tsuku-recipes/my-tool.toml --sandbox

# From the distributed source (after pushing)
tsuku install acme/my-tool --sandbox
```

### Verify Cache Behavior

```bash
# First install: downloads recipe
tsuku install acme/my-tool

# Second install: uses cached recipe (1-hour TTL)
tsuku install acme/my-tool

# Force refresh
tsuku update-registry
```

---

## Cache Behavior

### TTL

- Directory listing (which recipes exist): 1 hour default
- Individual recipe files: 1 hour default
- Incomplete listings (from rate-limit fallback): 5 minutes
- Override with: `TSUKU_RECIPE_CACHE_TTL=30m`

### Cache Location

```
$TSUKU_HOME/cache/distributed/{owner}/{repo}/
├── _source.json          # directory listing metadata
├── {recipe}.toml         # cached recipe content
└── {recipe}.meta.json    # ETag/Last-Modified for conditional requests
```

### Updating Published Recipes

1. Edit the TOML file in `.tsuku-recipes/`
2. Push to main/master
3. Users pick up changes after the cache TTL expires or after running
   `tsuku update-registry`

### Rate Limiting

tsuku uses GitHub's API for recipe discovery. Unauthenticated requests have
low rate limits (60/hour). For better limits, users should set `GITHUB_TOKEN`.

When rate-limited, tsuku falls back to branch probing via raw.githubusercontent.com
and serves stale cache when available.

---

## Max Cache Size

The distributed recipe cache caps at 20 MB. When exceeded, tsuku evicts the
oldest repo directory (LRU by last access time).
