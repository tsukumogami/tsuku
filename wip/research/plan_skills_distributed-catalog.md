# Tsuku Distributed Recipe System Catalog

## Directory Structure (.tsuku-recipes/ layout)

### On-Disk Repository Layout

Each distributed recipe repository must contain:
```
owner/repo/
├── .tsuku-recipes/
│   ├── recipe-one.toml
│   ├── recipe-two.toml
│   └── manifest.json (optional)
```

**Key Points:**
- Required directory: `.tsuku-recipes/` at repository root
- Recipe files: `{recipe-name}.toml` (kebab-case recommended, must match metadata.name in TOML)
- Optional manifest: `.tsuku-recipes/manifest.json` or `recipes/manifest.json` (fallback path)
- No other setup or registration step needed—detection is automatic

### Manifest Format (Optional)

When `manifest.json` is present, it defines repository layout and indexing:

```json
{
  "layout": "flat",
  "index_url": "optional_prebuilt_index_url"
}
```

**Manifest fields:**
- `layout`: `"flat"` (all recipes in `.tsuku-recipes/`) or `"grouped"` (subdirectories per recipe)
- `index_url`: Optional pre-computed recipe index (for performance optimization)

**Manifest discovery:**
- Probes `.tsuku-recipes/manifest.json` first (canonical)
- Falls back to `recipes/manifest.json` (compatibility)
- If neither found: defaults to flat layout with no index
- Branch probing tries: `main`, `master`

---

## Install Syntax (all variants with examples)

### Distributed Install Argument Formats

| Syntax | Example | Behavior |
|--------|---------|----------|
| `owner/repo` | `acme-corp/my-tool` | Installs single recipe or default (repo name as recipe) |
| `owner/repo:recipe` | `acme-corp/tools:deploy-cli` | Installs named recipe from repo |
| `owner/repo@version` | `acme-corp/my-tool@v1.0.0` | Installs specific version of default recipe |
| `owner/repo:recipe@version` | `acme-corp/tools:deploy-cli@2.1.0` | Installs specific version of named recipe |

### Parsing Logic (parseDistributedName)

Recognizes distributed syntax by presence of `/`:
```go
// Order of operations:
1. Check for `@` (version) at the end
2. Check for `:` (recipe name) after owner/repo
3. Default recipe name to repo name if not specified
4. Reject path traversal attempts (`.., /, etc`)
```

### Real-World Examples

```bash
# Single-recipe repo (repo name = recipe name)
tsuku install acme-corp/my-tool
tsuku install acme-corp/my-tool@v1.0.0

# Multi-recipe repo (recipe name required after :)
tsuku install acme-corp/internal-tools:deploy-cli
tsuku install acme-corp/internal-tools:deploy-cli@2.1.0

# With --yes flag to skip confirmation
tsuku install -y acme-corp/internal-tools:deploy-cli

# With --sandbox for testing
tsuku install acme-corp/my-tool --sandbox
```

---

## Discovery Mechanism (branch probing, manifest, caching)

### Detection Flow

```
User runs: tsuku install acme-corp/my-tool:deploy-cli

1. parseDistributedName extracts:
   - Source: "acme-corp/my-tool"
   - RecipeName: "deploy-cli"
   - Version: "" (or version if @specified)

2. ensureDistributedSource:
   - Validates source format (owner/repo only, no path traversal)
   - Checks if provider exists (cached in loader)
   - Loads user config to check registration
   - Confirms if unregistered (unless --yes or strict mode blocks)
   - Auto-registers to config.toml if approved
   - Adds provider to global recipe loader

3. GitHubClient.ListRecipes called via RegistryProvider:
   - Check cache freshness → return if fresh
   - Try GitHub Contents API → cache and return
   - If rate-limited → try cached branch or probe default branches
   - Fallback: probe raw.githubusercontent.com on main/master
```

### GitHub API Client Setup

**AuthTransport:**
- Adds `Authorization: Bearer {GITHUB_TOKEN}` header only to api.github.com requests
- Token source: `GITHUB_TOKEN` environment variable (via secrets.Get)
- Separate authenticated and unauthenticated HTTP clients
  - apiClient: authenticated, for Contents API
  - rawClient: unauthenticated, for raw downloads (allows aggressive caching)

**API Endpoints:**
```
Contents API: https://api.github.com/repos/{owner}/{repo}/contents/.tsuku-recipes/
Raw Downloads: https://raw.githubusercontent.com/{owner}/{repo}/{branch}/.tsuku-recipes/{file}
```

**Rate Limiting Handling:**
- Detects HTTP 429 (Too Many Requests) or 403 (Forbidden)
- Parses headers: X-RateLimit-Remaining, X-RateLimit-Reset
- Falls back to stale cache if available
- If no cache: probes default branches via raw URLs (lossy listing)
- Error includes: remaining count, reset timestamp, token status hint

### Branch Probing

When Contents API is rate-limited and no cache exists:
```
1. If cached branch exists → try cached branch first
2. Try default branches: main, master
3. HEAD request to raw.githubusercontent.com/.tsuku-recipes/ directory
4. If branch exists → return incomplete SourceMeta (Files: nil)
```

**SourceMeta.Incomplete flag:**
- Set to `true` when directory listing obtained via branch probing
- Uses 5-minute TTL (vs 1-hour for normal) to refresh once API resets
- Files map is `nil` (unknown recipes, will populate on next API success)

### SourceMeta Caching

**SourceMeta struct:**
```go
type SourceMeta struct {
    Branch     string            // Git branch containing recipes
    Files      map[string]string // recipe name → download URL
    FetchedAt  time.Time         // timestamp for TTL checks
    Incomplete bool              // true if from branch probing (lossy)
}
```

**Storage:**
```
$TSUKU_HOME/cache/distributed/{owner}/{repo}/_source.json
```

**Cache Validation:**
- IsSourceFresh(meta): checks (time.Now() - FetchedAt) < TTL
- Normal TTL: 1 hour (distributed.DefaultCacheTTL)
- Incomplete TTL: 5 minutes (forces re-fetch after API recovery)

### Recipe Download Caching

**RecipeMeta struct:**
```go
type RecipeMeta struct {
    ETag         string    // HTTP ETag header
    LastModified string    // HTTP Last-Modified header
    FetchedAt    time.Time // timestamp for TTL checks
}
```

**Storage:**
```
$TSUKU_HOME/cache/distributed/{owner}/{repo}/{recipe}.toml
$TSUKU_HOME/cache/distributed/{owner}/{repo}/{recipe}.meta.json (metadata sidecar)
```

**HTTP Caching:**
- Conditional requests: If-None-Match (ETag), If-Modified-Since
- 304 Not Modified returns cached content
- 1MB per-recipe size limit (io.LimitReader)

**Cache Eviction:**
- Max cache size: 20 MB (defaultMaxCacheSize)
- LRU eviction when exceeded (removes oldest repo directory by _source.json mtime)

---

## Trust Model (first-install confirmation, registries, strict mode)

### First-Install Confirmation Flow

```
User: tsuku install acme-corp/internal-tools:deploy-cli

1. ensureDistributedSource checks:
   - Is source already registered in config.toml registries?
   - If yes → proceed silently
   - If no → check strict_registries mode

2. Strict mode (config.toml: strict_registries = true):
   - REJECT unregistered source immediately
   - Error: "source X is not registered and strict_registries is enabled"
   - User must run: tsuku registry add acme-corp/internal-tools

3. Normal mode (strict_registries = false, default):
   - Prompt user: "Install from unregistered source 'acme-corp/internal-tools'? [y/N]"
   - --yes flag or -y skips prompt
   - User declines → cancel install
   - User approves → auto-register to config.toml and proceed

4. Auto-Registration:
   - Adds entry to $TSUKU_HOME/config.toml:
     [[registries]]
     source = "acme-corp/internal-tools"
   - AutoRegistered flag set to true (for cleanup UI)
   - Dynamically loads provider into recipe loader for same session
```

### Registry Management Commands

**tsuku registry list:**
- Shows all registered distributed sources
- Displays URL (default: https://github.com/{owner}/{repo})
- Marks auto-registered entries
- Shows strict_registries mode status

**tsuku registry add owner/repo:**
- Validates source format (owner/repo, no path traversal)
- Idempotent: no-op if already registered
- Adds to config.toml with AutoRegistered: false

**tsuku registry remove owner/repo:**
- Removes from config.toml registries section
- Does NOT uninstall tools (they continue to work from cache)
- Lists tools still installed from that source (advisory message)

### Registry Entry Structure

```toml
# In $TSUKU_HOME/config.toml
[[registries]]
source = "acme-corp/internal-tools"  # or use shortened key format
url = "https://github.com/acme-corp/internal-tools"
auto_registered = false  # true if added by tsuku during install
```

**Registries map:**
- Keyed by source name (owner/repo)
- RegistryEntry contains: URL, AutoRegistered flag

### Strict Mode Configuration

**Enable strict registries:**
```toml
# $TSUKU_HOME/config.toml
strict_registries = true
```

**Behavior when enabled:**
- Installs only from: central registry + explicitly listed distributed sources
- Unregistered sources rejected immediately (no prompt)
- CI-safe: prevents accidental installation from untrusted sources
- Useful for shared machines, team environments, CI pipelines

**Interaction with auto-registration:**
- When strict_registries = true: auto-registration is blocked
- ensureDistributedSource returns error requesting manual registry add
- --yes flag does not bypass strict mode (security by design)

---

## Testing Workflow (validate, sandbox for distributed recipes)

### Recipe Validation

**tsuku validate .tsuku-recipes/my-tool.toml:**
- Checks TOML syntax
- Validates required fields: metadata.name, steps, verify.command
- Validates action types and parameters
- Security checks: URL schemes (HTTPS), path traversal

**tsuku validate --strict:**
- Treats warnings as errors
- Enforces best practices (e.g., explicit version handling)

**tsuku validate --check-libc-coverage:**
- Validates glibc/musl coverage for library recipes
- Errors: libraries without musl support
- Warnings: tools with library deps missing musl path

### Sandbox Testing

**tsuku install --recipe .tsuku-recipes/my-tool.toml --sandbox:**
- Runs installation in isolated container
- Tests without affecting system
- Validates verification script works
- Sandbox still requires container runtime

**Via distributed source:**
```bash
tsuku install myorg/recipes:mytool --sandbox
```

### Before Publishing

1. Validate locally:
   ```bash
   tsuku validate .tsuku-recipes/my-tool.toml
   tsuku validate --strict .tsuku-recipes/my-tool.toml
   ```

2. Test sandbox installation:
   ```bash
   tsuku install --recipe .tsuku-recipes/my-tool.toml --sandbox
   tsuku install myorg/recipes:mytool --sandbox
   ```

3. Check caching works:
   ```bash
   tsuku install myorg/recipes:mytool        # First install (downloads)
   tsuku install myorg/recipes:mytool@latest # Second install (uses cache)
   tsuku update-registry                      # Force refresh
   ```

---

## Cache Behavior (TTL, refresh, what authors need to know)

### Cache Directories

```
$TSUKU_HOME/cache/distributed/
├── acme-corp/
│   └── my-tool/
│       ├── _source.json           # Directory listing metadata
│       ├── deploy-cli.toml        # Recipe TOML (from Contents API)
│       └── deploy-cli.meta.json   # ETag/Last-Modified for 304 caching
```

### TTL Values

**SourceMeta (directory listing):**
- Normal: 1 hour (distributed.DefaultCacheTTL)
- Incomplete (from branch probing): 5 minutes
- Env override: `TSUKU_RECIPE_CACHE_TTL` (e.g., `TSUKU_RECIPE_CACHE_TTL=30m`)

**RecipeMeta (individual recipe file):**
- Same as SourceMeta (default 1 hour)
- Fresh check: time since FetchedAt < TTL

### Cache Invalidation

**Manual refresh:**
```bash
tsuku update-registry              # Refresh all caches
tsuku update-registry --distributed # Refresh only distributed sources
```

**Automatic refresh triggers:**
- Installation detects stale cache → re-fetches
- TTL expired → next access triggers refresh
- HTTP 304 Not Modified → uses cached content

### What Recipe Authors Need to Know

1. **Publishing new versions:**
   - Update `.tsuku-recipes/recipe.toml` in your repo
   - Push to main/master branch
   - Users will pick up changes on next install or after TTL expires

2. **Recipe naming consistency:**
   - Filename must match metadata.name
   - Example: `deploy-cli.toml` → `metadata.name = "deploy-cli"`
   - Mismatch causes confusion (filename used for discovery, name for state)

3. **Testing with caching:**
   - First install downloads recipe (cache miss)
   - Subsequent installs use cache (1-hour default)
   - Force refresh with: `tsuku update-registry`
   - Clear cache manually: `rm -rf $TSUKU_HOME/cache/distributed/{owner}/{repo}/`

4. **Rate limiting impact:**
   - If repo is heavily used: users will hit rate limits
   - Recommend users set GITHUB_TOKEN for higher limits
   - Cache mitigates by falling back to branch probing
   - Full listing recovers after rate limit reset

5. **Manifest benefits:**
   - Optional but recommended for multi-recipe repos
   - Enables pre-built index optimization
   - Allows custom layout (grouped subdirectories)
   - Without it: flat layout with auto-discovery

### Environment Variables for Authors/Developers

```bash
# Override cache TTL (distributeddefault: 1 hour)
TSUKU_RECIPE_CACHE_TTL=30m tsuku install acme-corp/my-tool

# GitHub token for higher rate limits
GITHUB_TOKEN=ghp_xxx tsuku install acme-corp/my-tool

# Clear cache between test iterations
rm -rf ~/.tsuku/cache/distributed

# Test branch probing fallback (simulate rate limit)
# (Not directly testable; use live testing or integration tests)
```

---

## Related Implementation Files

**Core distributed code:**
- `/internal/distributed/client.go` - GitHubClient, Contents API, branch probing
- `/internal/distributed/provider.go` - Manifest discovery, DistributedRegistryProvider
- `/internal/distributed/backing_store.go` - GitHubBackingStore adapter
- `/internal/distributed/cache.go` - CacheManager, SourceMeta, RecipeMeta
- `/internal/distributed/errors.go` - Error types (ErrRateLimited, ErrNoRecipeDir, etc)

**CLI commands:**
- `/cmd/tsuku/install.go` - parseDistributedName, ensureDistributedSource, checkSourceCollision
- `/cmd/tsuku/install_distributed.go` - Distributed install workflow, auto-registration
- `/cmd/tsuku/registry.go` - Registry management (add/remove/list)
- `/cmd/tsuku/update_registry.go` - Cache refresh, including refreshDistributedSources
- `/cmd/tsuku/validate.go` - Recipe validation (works for distributed recipes too)

**Configuration:**
- `/internal/userconfig/userconfig.go` - Config struct, Registries map, StrictRegistries flag
- `/internal/config/config.go` - Cache TTL environment variables (TSUKU_RECIPE_CACHE_TTL)

**Documentation:**
- `/docs/GUIDE-distributed-recipes.md` - User guide (install, registry commands, trust model)
- `/docs/GUIDE-distributed-recipe-authoring.md` - Author guide (setup, testing, publishing)

