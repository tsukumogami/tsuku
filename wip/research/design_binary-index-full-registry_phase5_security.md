# Security Review: binary-index-full-registry

## Dimension Analysis

### External Artifact Handling
**Applies:** Yes

The new `FetchRecipe` path downloads TOML files from the registry URL and passes them to `toml.Unmarshal` inside `Rebuild`. The TOML is parsed into `recipeMinimal`, a narrow struct that reads only `metadata.binaries` and a flat list of step params. Malformed TOML is caught and skipped with a warning rather than aborting the rebuild. No code in the fetch-then-index pipeline executes any field value as a shell command, evaluates URLs, or writes files outside the cache directory.

Two gaps are present in the current design:

1. **No response size cap on `FetchRecipe`.** `fetchRemoteRecipe` calls `io.ReadAll(resp.Body)` with no byte limit. A malicious or misconfigured server could return a multi-megabyte body for a single recipe request. With a semaphore of 10 concurrent fetches and ~1,400 recipes on a cold start, this creates a worst-case in-memory allocation of `10 * response_size` before the data is written to disk. The existing `NewRegistryHTTPClient` sets `DisableCompression: true` (blocking decompression bombs) and a 30-second response timeout, but no explicit `MaxBytesReader` wrapper is used.

2. **No content-type validation.** The client accepts any response body as a recipe TOML regardless of `Content-Type`. If the upstream CDN or proxy serves a redirect page or error document as a 200, `toml.Unmarshal` will either fail (the recipe is skipped) or, in pathological cases, succeed and produce nonsense binary path strings. The second outcome is low-risk because the TOML parser only extracts known string fields; unexpected keys are discarded.

Neither issue allows remote code execution through the index rebuild path alone. Actual code execution comes from a subsequent `tsuku install`, which has its own validation path.

### Permission Scope
**Applies:** Yes

The `Rebuild` path requires:

- **Network egress** to the registry URL (`raw.githubusercontent.com` by default, or a user-supplied override via `TSUKU_REGISTRY_URL`).
- **Read** access to `$TSUKU_HOME/registry/` (existing cached recipes) and `$TSUKU_HOME/cache/manifest.json` (for `ListAll`).
- **Write** access to `$TSUKU_HOME/registry/` (to cache fetched TOMLs via `CacheRecipe`) and `$TSUKU_HOME/cache/binary-index.db`.

This is no escalation over the permissions `update-registry` already holds. The user running `tsuku update-registry` already has network access and owns `$TSUKU_HOME`. The SQLite database and registry cache are both user-owned files. No sudo, no setuid, no inter-process communication beyond the HTTP client.

One scoping note: the `TSUKU_REGISTRY_URL` override accepted in `registry.New` allows the base URL to be set to any value, including `http://` (plaintext). If set to a local path, `isLocalPath` routes to `fetchLocalRecipe`, which reads from the filesystem. This is by design for development and testing, but it means a compromised environment variable can redirect fetches to an attacker-controlled HTTP endpoint. This concern is separate from the design change — it exists today — but the new fetch volume makes it more significant.

### Supply Chain or Dependency Trust
**Applies:** Yes

**TLS verification:** The `http.Transport` in `NewRegistryHTTPClient` uses Go's default TLS configuration. No custom `TLSClientConfig` is set, meaning the system certificate pool is used with full chain and hostname verification. There is no `InsecureSkipVerify` flag anywhere in the transport configuration. Fetches to `raw.githubusercontent.com` receive standard TLS validation.

**Redirect handling:** Go's default `http.Client` follows redirects automatically (up to 10 hops). No custom `CheckRedirect` is set. This means a redirect from `raw.githubusercontent.com` to a different host would be followed transparently and the response body would be treated as a valid recipe. GitHub's CDN does not redirect recipe files in practice, but a compromised or custom registry URL could issue cross-origin redirects.

**Source authenticity:** There is no cryptographic signature verification on recipe TOML content. The trust model relies entirely on HTTPS transport integrity: if the bytes arrive over a valid TLS session to `raw.githubusercontent.com`, they are assumed authoritative. This is the same trust level as `curl | sh` installers, and is consistent with how `GetCached` and the existing `update-registry` flow work today. The design change does not make this worse, but it does increase the attack surface by fetching up to ~1,400 additional files during a cold-start rebuild rather than zero.

A MITM against `raw.githubusercontent.com` is not practical without a forged TLS certificate trusted by the system. A supply chain compromise of the upstream GitHub repository would affect the manifest, cached recipes, and fetched recipes equally — this is not a concern introduced by the design change.

**Name enumeration from manifest:** `ListAll` reads recipe names from the locally cached `manifest.json`. The manifest is fetched from `tsuku.dev/recipes.json` (a separate origin from `raw.githubusercontent.com`). A compromise of either origin independently could inject recipe names into the manifest or inject recipe content at the corresponding TOML path. The two-origin model adds a small redundancy: an attacker must control both to inject a functional recipe name and matching TOML body. However, both origins are ultimately controlled by the same GitHub organization, so this is not a meaningful security boundary.

### Data Exposure
**Applies:** Yes

Each `FetchRecipe` call issues an HTTP GET to a URL of the form:

```
https://raw.githubusercontent.com/tsukumogami/tsuku/main/recipes/{letter}/{name}.toml
```

The information revealed per request:

- The recipe name being indexed (one per request).
- The client IP address.
- The `User-Agent` header — Go's default is `Go-http-client/2.0` (or `/1.1`). No tsuku version string is added.
- Timing correlations: 10 concurrent fetches interleaved across ~1,400 recipes produces a recognizable traffic pattern. An observer on the network or at GitHub's CDN could infer that the client is running `tsuku update-registry` on a clean machine.

No authentication credentials, no tool inventory, and no system identifiers are sent. The recipe names themselves are public. The client IP is inherent to any HTTP request.

On a warm machine (recipes already cached), `Rebuild` calls `GetCached` for all names returned by `ListAll` and skips `FetchRecipe` for cached ones. No network requests are made in that case.

The concurrent fetch burst (up to 10 simultaneous connections to `raw.githubusercontent.com`) could trigger GitHub's anonymous rate limiting. The design notes this risk and proposes a semaphore of 10. If rate limiting occurs, individual fetches fail with `ErrTypeRateLimit` and are skipped — the index is built from the subset that succeeded. This degrades gracefully but can produce an incomplete index on a cold machine with a slow connection.

---

## Recommended Outcome

[OPTION 2 - Document considerations]

The design is acceptable to ship. No critical vulnerabilities are introduced. The items below should be addressed in documentation and a follow-up issue rather than blocking the implementation.

---

### Security Considerations

**Recipe fetch integrity**

Recipe TOML files are fetched over HTTPS from `raw.githubusercontent.com` using Go's default TLS stack with full certificate chain and hostname verification. No custom TLS configuration is applied; the system trust store is used. Content is not cryptographically signed — authenticity relies on HTTPS transport integrity.

**Response size**

`FetchRecipe` reads the full response body with `io.ReadAll` and no byte limit. It is recommended to wrap `resp.Body` with `io.LimitReader` (suggested limit: 1 MiB per recipe) before calling `io.ReadAll`. This caps per-fetch memory allocation and prevents an oversized response from exhausting memory during a cold-start rebuild with up to 10 concurrent fetches.

**Redirect behavior**

Go's `http.Client` follows cross-origin redirects by default. If the registry URL is overridden via `TSUKU_REGISTRY_URL` to an HTTP endpoint, redirects are followed without downgrade protection. For production use the default `https://raw.githubusercontent.com` URL is unaffected, but custom registry deployments should use HTTPS.

**Registry URL override**

Setting `TSUKU_REGISTRY_URL` to an attacker-controlled endpoint redirects all recipe fetches to that endpoint. This is an existing concern, not introduced by this design, but the new fetch volume amplifies its impact. Users deploying tsuku in shared environments should treat `TSUKU_REGISTRY_URL` as a sensitive configuration value.

**Network traffic fingerprinting**

A cold-start `update-registry` issues up to ~1,400 concurrent GET requests (semaphore-bounded to 10) to `raw.githubusercontent.com`, revealing the set of recipe names being indexed and the client IP. No personal identifiers or installed tool inventory are included in these requests.

**Rate limiting**

Anonymous requests to `raw.githubusercontent.com` are subject to GitHub's rate limiting. Failed fetches are skipped with a warning and the index is built from successfully fetched recipes. A subsequent `update-registry` retries missing recipes. Consider adding an `Retry-After` backoff for `ErrTypeRateLimit` responses in a future iteration.

---

## Summary

The `FetchRecipe`-during-`Rebuild` path introduces no privilege escalation and no new authentication surface. The main gaps are the absence of a per-response byte cap on `io.ReadAll` in `fetchRemoteRecipe` (a memory-exhaustion risk during cold-start burst fetches) and the lack of cross-origin redirect restrictions on `http.Client`. Both are mitigations worth adding — the byte cap as a code change before merge, the redirect policy as a documented recommendation — but neither blocks the design from shipping.
