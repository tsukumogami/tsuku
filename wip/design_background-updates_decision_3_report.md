<!-- decision:start id="distributed-registry-init-timeout" status="assumed" -->
### Decision: Distributed Registry Initialization Startup Protection

**Context**

When distributed registries are configured, `NewDistributedRegistryProvider` is called for each one in `main.go init()` (lines 133-150). Each call invokes `DiscoverManifest` synchronously, which probes `raw.githubusercontent.com` for a `manifest.json` via up to four sequential HTTP requests (two branches × two manifest paths). The context passed is `context.Background()` with no timeout, so a slow or unavailable GitHub CDN response blocks binary startup indefinitely.

The existing error handling already treats discovery failure as non-fatal: `NewDistributedRegistryProvider` falls back to `recipe.Manifest{Layout: "flat"}` if `DiscoverManifest` returns an error, and the `init()` loop skips the registry with a warning on any error. The missing piece is simply a time bound on how long discovery is allowed to run. A user with two configured distributed registries currently pays up to two unbounded TCP timeouts (potentially minutes each) before any command executes.

The `NewDistributedRegistryProviderWithManifest` constructor already exists for cases where the manifest is known in advance, confirming that manifest discovery is designed as optional infrastructure rather than a hard requirement.

**Assumptions**

- `httputil.NewSecureClient` does not set a client-level `Timeout`. If it does, the worst-case hang is partially mitigated, but adding an explicit context timeout remains the correct approach for deterministic control.
- The number of configured distributed registries is small (typically 1-3). A shared 3-second context deadline across all discovery calls bounds total init blocking to 3 seconds regardless of registry count.
- Users with distributed registries configured accept that those sources may be unavailable on degraded networks and prefer a fast degraded experience over a slow one.

**Chosen: Option A — Add context timeout; skip source on timeout (best-effort)**

Replace `initCtx := context.Background()` with a context carrying a 3-second deadline shared across all distributed registry discovery calls in `init()`. If any `DiscoverManifest` call times out or fails, the existing warning path (`fmt.Fprintf(os.Stderr, "Warning: failed to initialize distributed source %s: %v\n", ...)`) surfaces the condition and the registry is skipped for this run. No other behavior changes.

The 3-second value bounds the worst-case startup addition to 3 seconds total (shared context), accommodating legitimate CDN slowness while keeping startup acceptable. On happy-path networks (GitHub CDN typically responds in 100-300ms), there is no user-visible change.

For logging: the existing `Warning:` stderr message is appropriate. The notice system (`internal/notices`) is designed for persistent tool update failures and should not be used for transient network errors during manifest discovery.

**Rationale**

The existing code already handles discovery failure gracefully — the fallback to flat layout is already implemented. Option A closes the defect with a one-line context replacement and no interface or behavior changes. Option B (fail on timeout) contradicts the feature's design intent: Go `init()` cannot return errors, and restructuring initialization to surface a fatal error would be a breaking change for a best-effort feature. Option C (lazy initialization) defers rather than bounds the latency and requires a new proxy struct with `sync.Once` — more invasive for marginal UX gain, since the user still waits, just at recipe access time rather than startup.

**Alternatives Considered**

- **Option B — Add context timeout; fail command on timeout**: Rejected. Go `init()` cannot return errors; making manifest discovery fatal requires restructuring initialization into `PersistentPreRun`. More importantly, discovery failure is already designed to be non-fatal (flat layout fallback). Surfacing a timeout as a fatal error would break users on degraded networks for an optional best-effort feature.

- **Option C — Lazy initialization**: Rejected. While architecturally clean, it requires a proxy struct with `sync.Once` to protect `RegistryProvider` construction, since the current constructor takes a manifest at creation time. The UX benefit is minimal — users still experience latency, just at recipe-access time instead of startup. The added complexity isn't justified when a context timeout achieves the stated constraint (zero foreground blocking beyond a short bound) with one line of change.

**Consequences**

- Startup blocking from distributed registry discovery is bounded to 3 seconds total regardless of registry count.
- Users on degraded networks see existing warning messages rather than hanging indefinitely.
- No changes to the `RecipeProvider` interface, `DistributedRegistryProvider` struct, or test helpers.
- Tests in `provider_test.go` that exercise `NewDistributedRegistryProvider` use immediate-responding HTTP transports and are unaffected.
- The notice system remains dedicated to tool update failures; transient network conditions during discovery are surfaced inline via stderr warnings.
<!-- decision:end -->
