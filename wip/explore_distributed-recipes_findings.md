# Exploration Findings: distributed-recipes

## Core Question

How should tsuku model registries as a unified concept, where embedded recipes,
the central registry, local user recipes, and third-party repo recipes are all
instances of the same abstraction?

## Round 1

### Key Insights

1. **Claude Code's marketplace validates the registry-of-registries model.**
   Marketplaces are git-hosted JSON catalogs with flexible source types and a
   `tool@marketplace` addressing scheme. The `strict` toggle (registry can curate
   or just index) and organizational lockdown are directly applicable. (Lead: claude-marketplace)

2. **tsuku's code has clean seams for a RecipeProvider interface.** The Loader
   uses a hardcoded priority chain but each source is already isolated:
   `loadLocalRecipe()`, `EmbeddedRegistry.Get()`, `Registry.FetchRecipe()`.
   Unification is a refactor, not a rewrite. (Lead: tsuku-registry-code)

3. **The manifest should split into per-recipe and registry-level layers.**
   Per-recipe metadata (name, deps) is universal. Registry envelope (schema
   version, deprecation) only matters for remote sources. A bare `.tsuku-recipes/`
   directory of TOML files could work without a manifest. (Lead: unified-manifest)

4. **One-step fully-qualified install has the lowest friction.** Homebrew and Nix
   allow install-from-source without prior registration. Most other systems require
   explicit `add` first. The two-level `source/package` naming with default-source
   elision is universal. (Lead: package-manager-ux)

5. **Package-to-source binding must be deterministic.** pip's "search all, pick
   highest" caused real supply-chain attacks. Each recipe should resolve from
   exactly one source, no ambiguous fallback. (Lead: package-manager-ux)

6. **Start simple on trust, plan for growth.** Docker Content Trust (complex
   crypto) was deprecated for low adoption. Go's sumdb succeeded by being
   transparent. For tsuku: implicit trust on first use now, content hashing for
   tampering detection, cryptographic signing later. (Lead: trust-models)

### Tensions

- **Convenience vs. security on implicit install.** Most ecosystems require
  explicit registration. Homebrew's implicit tap is considered a security concern.
  But friction matters for adoption, especially pre-launch.

- **Manifest vs. no-manifest for third-party repos.** Claude Code requires a
  manifest. But a bare TOML directory is simpler for tool authors shipping one
  recipe. The PRD needs to decide the minimum viable registry.

- **Unification depth.** Embedded recipes are compiled into the binary -- making
  them a "registry" is conceptual, not just mechanical. Is unification a code
  abstraction (RecipeProvider interface) or a user-facing model, or both?

### Gaps

- Claude Code marketplace adoption data is thin (hard to gauge if proven at scale)
- No research on koto's specific recipe and what moving it would require
- Enterprise/private registry authentication not deeply explored

### User Focus

**Registry UX decision: auto-register by default, strict mode as config.**

The trust model has two modes:

- **Default (open):** `tsuku install owner/repo` auto-registers the source as a
  known registry, then installs. Zero friction. Trust on first use.
- **Strict mode:** A system config option (off by default) blocks auto-registration.
  Unknown sources are rejected with a message to run `tsuku registry add` first.

`tsuku registry add <name> <source>` always exists for:
- Pre-registering sources before first install
- Aliasing (short names for long source URLs)
- Required step in strict mode

This mirrors Claude Code's `strictKnownMarketplaces` pattern: convenient by
default, lockdown-able for enterprise/security-conscious users.

## Decision: Crystallize

## Accumulated Understanding

tsuku should unify all recipe sources (embedded, central, local, distributed)
behind a common RecipeProvider interface. The existing code has clean seams for
this refactor.

For users, `tsuku install ripgrep` continues to work unchanged (default registry).
`tsuku install owner/repo` fetches from a third-party repo's `.tsuku-recipes/`
directory, implicitly trusting and caching the source (Homebrew tap model). The
slash in the name distinguishes "distributed recipe" from "central registry recipe."

The manifest should split into two layers: per-recipe metadata (always present in
TOML) and a registry envelope (optional JSON for remote registries). A third-party
repo could ship a single `.tsuku-recipes/tool.toml` with no manifest overhead, or
a full manifest for multi-recipe registries.

Trust model has two modes: open (default) auto-registers unknown sources on first
install; strict mode (system config, off by default) blocks auto-registration and
requires explicit `tsuku registry add`. Both modes use content-hash pinning for
tampering detection. Cryptographic signing deferred until there are users and
third-party registries to protect.

Key open questions for the PRD:
- What's the minimum viable third-party registry? (Single TOML file? Manifest required?)
- How are recipe name conflicts across sources resolved?
- What does the RecipeProvider interface look like?
- How does `@latest` resolve for distributed recipes? (HEAD, latest tag, latest release?)
- How does state.json track distributed installs?
