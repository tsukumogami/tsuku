# Security Review: DESIGN Registry Refresh TTL Semantics

## Design Summary

The change makes `tsuku update-registry` always force-fetch every cached recipe from the
registry, rather than skipping recipes whose cache is within the 24-hour TTL. Implicit
cache access (during `tsuku install`, `tsuku info`, `tsuku search`) keeps the TTL. The
`--all` flag, `forceRegistryRefreshAll` helper, and `registryRefreshAll` variable are
removed. The implementation change is confined to `cmd/tsuku/update_registry.go`.

---

## Dimension Analysis

### 1. External artifact handling

**Applies: YES. Risk: LOW (pre-existing, unchanged).**

Recipe TOML files are fetched from `https://raw.githubusercontent.com/tsukumogami/tsuku/main`
via TLS. The transport is already hardened: `NewRegistryHTTPClient` (registry.go:37-51)
sets `DisableCompression: true` to block decompression bombs, applies per-request timeouts
(dial: 10s, TLS handshake: 10s, response headers: 10s, overall: `TSUKU_API_TIMEOUT`,
default 30s), and response bodies are capped at 1 MiB (`io.LimitReader(resp.Body, 1<<20)`
in registry.go:187).

Fetching more frequently does not change these protections. A recipe TOML is still text
parsed at install time, not executed at fetch time. The attack surface for the fetch itself
(MITM, oversized payload, decompression) is identical to the pre-design baseline.

**Mitigation needed: None.** The existing transport constraints handle this dimension.

---

### 2. Permission scope

**Applies: YES. Risk: NEGLIGIBLE.**

The change is confined to `cmd/tsuku/update_registry.go`. Execution writes to
`$TSUKU_HOME/registry/` (TOML files and `.meta.json` sidecars). File permissions on cache
writes use `0644` for content and `0755` for directories (registry.go:339-350,
cache.go:61-70). No new paths are accessed. No privilege escalation is possible: tsuku
does not require or request elevated permissions, and `$TSUKU_HOME` defaults to
`~/.tsuku`.

The manifest fetch (`refreshManifest`) and binary index rebuild (`rebuildBinaryIndex`) are
already called on every `update-registry` invocation; the design does not expand their
scope.

**Mitigation needed: None.**

---

### 3. Supply-chain and dependency trust

**Applies: YES. Risk: POSITIVE CHANGE (design closes a gap).**

Prior security assessments (wip/research/issue_phase1_security.md,
wip/research/issue_phase3_security.md) rated the pre-design state as MEDIUM-HIGH
supply-chain risk. The TTL-only freshness check in `RefreshAll` silently withheld recipe
security fixes from users who cached a recipe within the last 24 hours, even when they
explicitly ran `update-registry`. The 7-day `maxStale` window extended this exposure
further during network degradation.

This design closes that propagation gap: an explicit `update-registry` invocation now
fetches every cached recipe unconditionally. A security-relevant recipe update (corrected
download URL, fixed checksum, replaced distribution source) reaches users the next time
they run `update-registry`, not 24 hours later.

One residual concern: `CacheMetadata.ContentHash` (a SHA-256 of the cached TOML) is
computed on write but still not compared against the fetched content on refresh. The
design doesn't use it. This field could detect silent cache corruption on disk; without
verification, a corrupted local TOML could persist until the next explicit refresh. This
pre-existed the design and is out of scope here, but worth noting as a follow-on.

**Mitigation needed: None for this change.** The design is a net improvement. A follow-on
could compare `ContentHash` on read to detect silent disk corruption.

---

### 4. Data exposure

**Applies: NO.**

Recipe TOMLs are public, version-controlled files on the tsuku GitHub repository. No
credentials are involved in the fetch path. `fetchRemoteRecipe` sends a plain GET with no
authentication headers. The response is a TOML file, not user data.

**Mitigation needed: None.**

---

### 5. Denial of service / rate limiting

**Applies: YES. Risk: LOW-MEDIUM (manageable, worth documenting).**

`update-registry` now sends one HTTPS GET to `raw.githubusercontent.com` per cached
recipe on every invocation. The registry currently contains 1,405 TOML files. A user who
has cached all recipes would trigger 1,405 requests per `update-registry` run.

GitHub's raw content CDN (`raw.githubusercontent.com`) applies rate limits to unauthenticated
requests. The exact limit is undocumented for the raw CDN (unlike the API endpoint, which
is 60/hr unauthenticated). In practice, the sequential fetch loop (no goroutines in
`forceRegistryRefreshAll` or the proposed rewrite) and the 30-second per-request timeout
bound total request rate. With typical recipe response sizes (small TOML files), latency
per request is on the order of tens of milliseconds, so 1,405 sequential requests would
take roughly 30-60 seconds under normal conditions.

There are two concrete risks:

**a. User inadvertently triggers rate limiting.** A user who runs `update-registry` many
times in quick succession (e.g., in a shell loop or CI script) could hit the CDN's rate
limit. The current code handles HTTP 429 (`ErrTypeRateLimit`) and surfaces the error per
recipe, but does not add a backoff delay or abort early. Under a 429, the loop continues
and may make up to N-1 further doomed requests before exiting. The error message says
"Wait a few minutes before trying again" (errors.go:72), which is adequate but per-recipe
errors in a 1,400-recipe refresh will be noisy.

**b. CI / scripted use.** The design notes that `tsuku update-registry` is a user-facing
command. If a CI pipeline calls it against a full local cache, 1,400+ requests per run
may be impractical and could trigger rate limits. The `TSUKU_REGISTRY_URL` override
already lets CI point at a local path (used in `.github/workflows/test.yml:494`), which
bypasses the CDN entirely. This is a workable mitigation for CI contexts.

**Mitigations to document:**
- Note in command help that `--dry-run` can preview what would be fetched.
- Note that `--recipe <name>` refreshes a single recipe for targeted updates.
- Note that `TSUKU_REGISTRY_URL` can point to a local directory to avoid network requests
  in offline or CI contexts.
- A follow-on could add early-exit on repeated 429s (abort after N consecutive rate-limit
  errors), but this is not required to ship the fix.

---

### 6. TOCTOU / cache poisoning

**Applies: YES. Risk: LOW.**

There is a brief window between `cachedReg.Registry().ListCached()` enumerating recipe
names from disk and the subsequent per-recipe `cachedReg.Refresh(ctx, name)` writing the
fetched content. This window is not new (it existed in `forceRegistryRefreshAll` before
this design). Possible concerns:

**a. Race between fetch and install.** If `tsuku install <tool>` runs concurrently with
`tsuku update-registry`, the install command reads from the cache (via `loader.GetRecipe`
or `CachedRegistry.GetRecipe`) while the refresh loop is writing. The cache writes use
`os.WriteFile` (registry.go:350) which on Linux is not atomic across file + sidecar.
However, the worst-case outcome of a partial read is that install uses a slightly stale
TOML -- functionally identical to the pre-design behavior. There's no path for this race
to inject content that wasn't already in the cache or fetched from the authenticated CDN.
No new vulnerability is introduced.

**b. Symlink attacks on `$TSUKU_HOME/registry/`.** The cache directory is created under
`~/.tsuku/registry/` (user-owned). Subdirectory creation uses `os.MkdirAll(dir, 0755)`
and file writes use `os.WriteFile(path, data, 0644)`. On Linux, `WriteFile` does not
follow symlinks in the path components, but a symlink placed at the target path before the
write could redirect the write. This is a pre-existing issue with the cache layer and is
not introduced by this design. Exploiting it requires write access to `$TSUKU_HOME`, at
which point the attacker already controls the user's tool installation environment.

**c. Content substitution during the fetch window.** A MITM on the TLS connection to
`raw.githubusercontent.com` is required to inject content. Go's `net/http` validates TLS
certificates by default, and the HTTP client in `NewRegistryHTTPClient` does not disable
certificate verification. Short of a CA compromise, this attack is not feasible.

**Mitigation needed: None beyond existing controls.** The TOCTOU surface is pre-existing,
low-severity, and not worsened by this design.

---

## Security Considerations (draft text for design document)

The following is draft text for the Security Considerations section of the design
document:

---

**Supply chain (positive impact).** Making explicit `update-registry` invocations
force-fetch all cached recipes closes the propagation gap identified in the prior security
review. Before this change, a recipe security fix (corrected download URL, fixed
checksum) would not reach users who cached that recipe within the TTL window, even if
they explicitly ran `update-registry`. After this change, any explicit invocation
delivers the current registry state.

**Transport security (unchanged).** Recipe fetches use TLS with certificate verification,
a 1 MiB response body cap, and `DisableCompression` to block decompression attacks. These
controls apply equally to the new force-fetch path.

**Rate limiting.** Force-fetching all cached recipes sends one request per cached TOML.
At the current registry size (~1,400 recipes), a full cache refresh generates ~1,400
sequential requests to `raw.githubusercontent.com`. Users should use `--recipe <name>`
for targeted refreshes and `--dry-run` to preview what would be fetched. Scripts and CI
pipelines should set `TSUKU_REGISTRY_URL` to a local path to avoid CDN requests
entirely. HTTP 429 responses are surfaced as per-recipe errors; the refresh loop does not
abort on rate limiting, which may produce repeated errors if the CDN rate-limits the
session. A follow-on could add early-exit after consecutive 429 responses.

**TOCTOU / concurrent access (pre-existing, low severity).** The cache write path uses
`os.WriteFile`, which is not atomic with respect to the metadata sidecar. A concurrent
`tsuku install` reading the cache during a refresh may observe a stale TOML until the
write completes. This is a pre-existing condition not worsened by the change, and the
worst case is the behavior that existed before the fix: install uses a slightly older
recipe.

**Disk integrity (pre-existing gap).** `CacheMetadata.ContentHash` (SHA-256 of the
cached TOML) is computed on write but not verified on read. Silent disk corruption or
file tampering within `$TSUKU_HOME/registry/` is not detected. A future improvement
could verify the hash on every cache read. This is out of scope for this change.

---

## Recommended Outcome

**Option 2: Document considerations.**

The design does not introduce new exploitable vulnerabilities. It is a net security
improvement by closing the recipe-fix propagation gap. The two considerations worth
documenting are: (a) the rate-limiting behavior under large caches or scripted use, with
mitigations (`--recipe`, `--dry-run`, `TSUKU_REGISTRY_URL`), and (b) the pre-existing
TOCTOU and disk-integrity gaps, which are not worsened by this change but should be noted
for completeness.

No design changes are required before implementation.

---

## Summary

This design improves the security posture of `update-registry` by ensuring that explicit
invocations always deliver current registry content, closing a gap that could delay
security-relevant recipe fixes by up to 24 hours (or 7 days via stale fallback). The
only new risk worth documenting is rate-limit behavior under large caches or automated
use, which is manageable via existing env-var and flag mechanisms. No design changes are
needed before implementation.
