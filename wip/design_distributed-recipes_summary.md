# Design Summary: distributed-recipes

## Input Context (Phase 0)
**Source PRD:** docs/prds/PRD-distributed-recipes.md
**Problem (implementation framing):** The Loader's hardcoded priority chain, source-unaware state tracking, and central-registry-coupled caching prevent clean addition of distributed recipe sources. A RecipeProvider abstraction, source-tracked state, and multi-origin caching are needed.

## Approaches Investigated (Phase 1)
- **RecipeProvider Interface**: Extract a Go interface all sources implement. Eliminates duplicated chain logic, aligns with version provider pattern. Medium complexity.
- **Extended Registry**: Grow Registry to a list of instances. Minimal conceptual change but doesn't unify local/embedded, assumes bucketed directory layout.
- **URL Resolver**: Resolve owner/repo to GitHub raw URL. Lowest ceremony but accumulates tech debt, no abstraction, fragile URL dependency.

## Selected Approach (Phase 2)
RecipeProvider Interface. It's the only approach that delivers the PRD's unified abstraction goal. The pattern already exists implicitly in the codebase and explicitly in the version provider system. It reduces Loader complexity rather than increasing it, and each provider controls its own URL construction and caching, avoiding layout assumption conflicts.

## Investigation Findings (Phase 3)
- **Interface design**: Three methods (Get, List, Source) plus two optional interfaces (SatisfiesProvider, RefreshableProvider). Four Loader methods collapse into one chain-walking function. In-memory cache stays as Loader concern (stores parsed recipes, not bytes). RequireEmbedded becomes source-tag filter. update-registry uses type-assertion escape hatch.
- **HTTP fetching**: Two-tier strategy -- Contents API (1 rate-limited call) for directory listing, then raw.githubusercontent.com URLs (unlimited) for file content. Auth via GITHUB_TOKEN raises limit to 5000/hr. Separate CacheManager for distributed sources. Use httputil.NewSecureClient for SSRF protection.
- **State & registry**: Add top-level Source field to ToolState with lazy migration (default "central"). Store registered sources in config.toml alongside other user preferences. Source-directed loading for update/verify/outdated (bypass chain, use recorded source). Last-install-wins for name collisions with confirmation prompt.

## Security Review (Phase 5)
**Outcome:** Option 2 -- Document considerations
**Summary:** Trust model matches go install/cargo install (user trusts the source). Main risk is recipe mutation without detection (deferred to content-hash pinning). Three low-cost mitigations added: trust warning on first install, recipe hash recording in state.json, HTTPS validation for Contents API download_url fields.

## Current Status
**Phase:** 5 - Security
**Last Updated:** 2026-03-15
