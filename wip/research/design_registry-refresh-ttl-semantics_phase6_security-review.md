# Security Review: registry-refresh-ttl-semantics

Phase 6 security review of the TTL semantics and force-refresh design for `update-registry`.

## Scope

This review covers the four areas raised in the design document's Security Considerations section and adds an assessment of attack vectors the document does not mention.

---

## 1. Supply chain (positive impact claim)

**Claim accurate.** Before this change, `RefreshAll` in `cached_registry.go` skips fresh entries (`isFresh` check at line 334). A recipe cached within the TTL window was silently returned even when the caller explicitly invoked `update-registry`. After this change, the `--all` flag routes through `forceRegistryRefreshAll`, which calls `Refresh` unconditionally. Any security fix committed to the registry reaches users on the next explicit `update-registry --all` call, not on the next natural expiry.

One gap: the default invocation (no `--all`) still skips fresh entries. A user running `update-registry` without `--all` who cached a recipe two hours ago against a 24-hour TTL will not receive an emergency security fix. The design document does not surface this nuance. The mitigation (use `--all` for security-sensitive refreshes) should be documented in the command's `Long` help text.

---

## 2. Transport security (unchanged)

**Assessment accurate.** The code confirms all three controls:

- `NewRegistryHTTPClient` sets `DisableCompression: true` explicitly (registry.go:43).
- `io.LimitReader(resp.Body, 1<<20)` applies to both `fetchRemoteRecipe` (registry.go:187) and `FetchRecipe` in `distributed/client.go` (line 214).
- `NewSecureClient` in `httputil/client.go` wraps the transport with `DisableCompression` by default and a redirect checker that blocks HTTP downgrades and validates resolved IPs against private/loopback/link-local ranges (ssrf.go).

One finding: `fetchRemoteDiscoveryEntry` in registry.go (line 284) calls `io.ReadAll(resp.Body)` without a `LimitReader`. Discovery entries are small JSON blobs today, but a malicious or compromised CDN response could allocate unbounded memory. This is a pre-existing gap not introduced by this design, but the design's increased refresh frequency makes it marginally more reachable.

---

## 3. Rate limiting

**Assessment largely accurate.** The analysis correctly identifies ~1,400 sequential requests on a full cache refresh. The mitigations proposed (`--recipe`, `--dry-run`, `TSUKU_REGISTRY_URL`) are real and usable.

Additional findings:

**a. `RefreshAll` does not respect HTTP 429 from the central registry.**
`runRegistryRefreshAll` calls `cachedReg.RefreshAll`, which iterates all cached recipes in a map (non-deterministic order) and treats `ErrTypeRateLimit` as a per-recipe error to accumulate, not as a signal to abort. After a 429, all subsequent requests will also 429 and be logged as errors. The loop produces noisy output and hammers the CDN through the rate-limit window. The design acknowledges this ("does not abort on rate limiting") but does not document the observable behaviour.

**b. The `--all` flag ignores rate-limit errors from distributed sources.**
`refreshDistributedSources` calls `rp.Refresh` per provider and continues on error. `GitHubBackingStore.ForceRefresh` calls `ForceListRecipes`, which on a 429 falls back to stale cache (distributed/client.go:148-157). This is actually safer than the central registry path because it silently degrades rather than emitting per-recipe errors.

**c. No backoff between requests.** Sequential requests to `raw.githubusercontent.com` with no inter-request delay are aggressive. CDN-level token bucket rate limits may activate for CI pipelines that run `update-registry --all` on every job.

**Recommendation:** Add early-exit from `RefreshAll` (or at minimum, `forceRegistryRefreshAll`) after the first `ErrTypeRateLimit` response. Expose this in the help text so operators know to set `TSUKU_REGISTRY_URL` in CI.

---

## 4. TOCTOU / concurrent access (pre-existing, low severity)

**Assessment accurate.** `PutRecipe` in `distributed/cache.go` writes the TOML file with `os.WriteFile` (line 175) then the metadata sidecar with a second `os.WriteFile` (line 185). A concurrent reader between the two writes sees a stale TOML with fresh (or absent) metadata, which causes `isFresh`/`IsRecipeFresh` to return false and trigger a redundant network fetch. This is a benign race: the worst outcome is a wasted request, not corrupted data.

The same pattern applies to the central registry: `CacheRecipe` calls `os.WriteFile` for the TOML (registry.go:350), then `WriteMeta` for the sidecar. Same race, same consequence.

Neither write window is widened by this design change. The assessment of "not worsened" is correct.

---

## 5. Disk integrity (pre-existing gap)

**Assessment accurate.** `CacheMetadata.ContentHash` is written but never read. `GetCached` and `GetRecipe` return file contents without verifying the hash. A modified on-disk TOML (e.g., via a local symlink attack or malicious disk write) is silently trusted.

The threat model is low: `$TSUKU_HOME` is user-owned and not shared. The gap is noted for completeness.

---

## 6. Attack vectors not considered

### 6a. Token leakage via `Authorization` header on redirect

`authTransport.RoundTrip` attaches a `Bearer` token to all requests whose hostname is `api.github.com` (distributed/client.go:51). The redirect checker in `httputil` validates redirect targets for SSRF, but does not strip the `Authorization` header before following a redirect. If `api.github.com` redirects to a different host (unlikely but possible if GitHub ever issues a cross-origin redirect for the Contents API), the token would be forwarded.

Go's `http.Client` does strip `Authorization` headers by default when a redirect changes the host (stdlib behaviour since Go 1.16). The risk is therefore mitigated by the runtime, not by tsuku code. However, this is an implicit dependency on stdlib behaviour that is not documented in `authTransport`. A comment should make this explicit.

### 6b. Recipe name injection via `_source.json` `Files` map

`SourceMeta.Files` maps recipe names to download URLs. These names come from the Contents API response and are written to disk without sanitisation beyond `validateDownloadURL` on the URL. `validateRecipeName` in `distributed/cache.go` (line 63) catches `..` and `/`, but a recipe name containing shell metacharacters (`;`, `$`, backtick) is accepted and written to disk as a TOML filename. The recipe name reaches `os.WriteFile` as part of a `filepath.Join`, which is safe, and is not passed to a shell. No injection risk exists in the current code paths.

However, the name is later displayed in CLI output (`detail.Name`), used in error messages, and could appear in log files. Names with ANSI escape sequences (e.g., `\x1b[31m`) could corrupt terminal output. This is a cosmetic risk, not a security risk, but worth noting.

### 6c. Cache eviction as a denial-of-service primitive

`evictOldest` in `distributed/cache.go` removes the oldest entire repo cache directory when the distributed cache exceeds 20 MB. An attacker who can publish many small recipes to many GitHub repositories could pollute the cache, causing legitimate repos' caches to be evicted repeatedly. The 20 MB cap limits total disk use but not the churn rate. In practice, the distributed source list is configured by the user, limiting the attack surface to repos the user has added.

### 6d. `TSUKU_REGISTRY_URL` pointing to a local path

The design recommends `TSUKU_REGISTRY_URL=<local path>` for CI. `isLocalPath` in registry.go (line 70) accepts any non-HTTP string, including paths like `/etc/passwd` or `../../sensitive`. `fetchLocalRecipe` constructs the path as `r.recipeURL(name)` which produces `<baseURL>/recipes/<letter>/<name>.toml`. If `baseURL` is an attacker-controlled path traversal, `os.ReadFile` reads an arbitrary file. The threat actor would need to set the environment variable, which requires code execution or control of the CI environment. For CI pipelines that set `TSUKU_REGISTRY_URL` from untrusted input (e.g., a PR-submitted workflow file), this is a meaningful risk. Recommend documenting that `TSUKU_REGISTRY_URL` must be a trusted path.

---

## 7. Summary of findings

| Finding | Severity | New or Pre-existing | Recommendation |
|---------|----------|---------------------|----------------|
| Default `update-registry` (no `--all`) skips fresh entries, missing security fixes within TTL | Medium | New (design gap) | Document in `--help`; advise `--all` for security refreshes |
| `fetchRemoteDiscoveryEntry` missing `LimitReader` | Low | Pre-existing | Add `io.LimitReader` to the discovery fetch path |
| No early-exit on HTTP 429 in `RefreshAll` | Low | Pre-existing, worsened by higher request volume | Add abort-on-429 to `forceRegistryRefreshAll` |
| No inter-request delay for large cache refreshes | Low | Pre-existing | Document CI guidance for `TSUKU_REGISTRY_URL` |
| `Authorization` header stripping on redirect implicit (relies on stdlib) | Low | Pre-existing | Add comment to `authTransport` |
| `TSUKU_REGISTRY_URL` accepts arbitrary filesystem paths | Low | Pre-existing | Document that the variable must point to a trusted source |
| TOCTOU on cache writes | Low | Pre-existing | Acknowledged, no action needed |
| `ContentHash` not verified on read | Low | Pre-existing | Out of scope, acknowledged |

No critical or high-severity findings. No issues require escalation. The design's positive supply chain claim is accurate for `--all` refreshes but should be qualified for the default (TTL-skipping) path.
