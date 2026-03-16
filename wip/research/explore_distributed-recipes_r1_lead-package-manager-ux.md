# Lead: Package manager UX patterns for third-party sources

## Findings

### 1. Homebrew Taps

**Adding a source:**
```bash
brew tap user/repo          # clones github.com/user/homebrew-repo
brew tap user/repo <url>    # custom Git URL
```
The `homebrew-` prefix is auto-added/stripped -- `brew tap user/foobar` maps to `github.com/user/homebrew-foobar`. This is a nice UX touch that keeps commands short while enforcing a naming convention on the hosting side.

**Installing from a source:**
```bash
brew install vim                 # from homebrew/core (default)
brew install user/repo/vim       # fully qualified from a tap
```
One-step install is possible: `brew install user/repo/formula` auto-taps if needed. The two-step flow (tap then install) is also supported but not required.

**Trust/security:**
- No signature verification on tap contents
- Name collisions with core formulae are explicitly discouraged but not prevented
- Taps are full Git clones, consuming significant disk space (the `.git` folder for homebrew-core alone is >1GB)
- No sandboxing of formula execution

**Friction points:**
- Storage bloat from Git clones of tap repos
- Name collision between taps and core (can't install both side-by-side)
- Documentation gaps around tap creation and maintenance
- Formulae must be in a tap -- local formula files outside taps are rejected
- `brew update` complains about deleted taps; users must manually `untap`
- Auto-update behavior changed (removed `--force-auto-update` in 4.2.13) with poor migration docs

### 2. Cargo Alternative Registries

**Adding a source:**
Configured in `.cargo/config.toml`:
```toml
[registries]
my-registry = { index = "https://my-intranet:8080/index" }
```
No CLI command to add a registry -- it's config-file-only. Authentication goes in `.cargo/credentials.toml` (separate file with tighter permissions).

**Installing from a source:**
```bash
cargo install --registry my-registry my-crate
cargo install --index https://example.com/index my-crate
```
The `--registry` flag uses a named alias from config. The `--index` flag uses a URL directly. Dependencies in `Cargo.toml` use `registry = "my-registry"` per-dependency.

**Trust/security:**
- Registries can require authentication (`auth-required = true` in registry config.json)
- Credential management is split across files with different permissions
- No built-in signature verification on registry contents
- The default (crates.io) has its own trust model independent of alternative registries

**Friction points:**
- Config-file-only setup (no `cargo registry add` command)
- `cargo install` ignores `--registry` for transitive dependencies (issue #12076) -- major footgun
- No fallback chain between registries for the same crate
- Authentication config split across two files is confusing

### 3. Go Module Proxies

**Adding a source:**
```bash
export GOPROXY=https://proxy.example.com,https://proxy.golang.org,direct
export GOPRIVATE=*.corp.example.com
export GONOSUMDB=*.corp.example.com
```
Environment-variable-only configuration. The GOPROXY value is a comma-separated fallback chain. `direct` means fetch from the VCS directly.

**Installing from a source:**
```bash
go get github.com/foo/bar@v1.2.3   # module path IS the source identifier
go install github.com/foo/bar@latest
```
Go's distinctive trait: the module path embeds the source location. There's no separate "add registry" step. The proxy is transparent -- it intercepts fetches for all modules.

**Trust/security:**
- `sum.golang.org` provides a transparency log for checksums
- GOPRIVATE/GONOSUMDB bypass the checksum database for private modules
- The proxy sees all your dependency fetches (privacy concern that led to GOPRIVATE)
- Proxy can return 302 redirects to Google Cloud Storage, creating hidden network dependencies

**Friction points:**
- "All or nothing" proxy behavior -- if any dependency isn't on the proxy, the build fails
- Three separate environment variables (GOPROXY, GOPRIVATE, GONOSUMDB) for what's conceptually one concern
- GOPRIVATE glob patterns are error-prone
- Hidden redirect behavior (proxy -> GCS) is poorly documented and breaks in restricted networks
- No way to pin to a specific proxy for specific modules (it's global)

### 4. npm Scoped Registries

**Adding a source:**
```bash
npm config set @myorg:registry https://npm.myorg.com/
npm config set '//npm.myorg.com/:_authToken' "TOKEN"
```
Or in `.npmrc`:
```ini
@myorg:registry=https://npm.myorg.com/
//npm.myorg.com/:_authToken=TOKEN
```

**Installing from a source:**
```bash
npm install @myorg/my-package    # scope routes to configured registry
```
The `@scope` prefix automatically routes to the right registry. Unscoped packages always go to the default registry.

**Trust/security:**
- Scopes provide namespace isolation, preventing dependency confusion for scoped packages
- Unscoped packages remain vulnerable to confusion attacks when using multiple registries
- Auth tokens in .npmrc are a common secret leak vector

**Friction points:**
- Unscoped packages can't be routed to specific registries (source of dependency confusion)
- `.npmrc` config is fragile and easy to misconfigure
- No CLI flag override for scoped registry (issue #10117)
- Multiple `.npmrc` files (project, user, global) create precedence confusion

### 5. pip / PyPI

**Adding a source:**
```bash
pip install --index-url https://private.pypi.org/simple/ my-package
pip install --extra-index-url https://private.pypi.org/simple/ my-package
```
Or in `pip.conf` / `pyproject.toml`.

**Installing from a source:**
Same as above -- the source is specified inline or via config. No named registries.

**Trust/security:**
- `--extra-index-url` is the canonical dependency confusion vector. pip checks ALL indexes and installs the highest version from any of them. This enabled supply chain attacks against Apple, Microsoft, PayPal.
- `--index-url` (without "extra") replaces the default, which is safer
- PEP 708 proposes mitigations but isn't widely adopted yet
- Pipenv deprecated `--extra-index-urls` in favor of index-restricted packages

**Friction points:**
- The `--extra-index-url` footgun is well-known but persists in the default behavior
- No package-to-index binding (any package can come from any index)
- No named registries -- only URLs
- Configuration scattered across pip.conf, pyproject.toml, requirements.txt, and CLI flags

### 6. Nix Flakes

**Adding a source:**
Flake references use a URL-like scheme:
```
github:owner/repo          # GitHub shorthand
github:owner/repo/ref      # with branch/tag
git+https://example.com/repo.git
path:/local/path
```
A global registry maps short names to full flake refs:
```bash
nix registry add myflake github:owner/repo
```

**Installing/running from a source:**
```bash
nix run github:owner/repo#package
nix profile install github:owner/repo#package
nix run myflake#package               # via registry alias
```

**Trust/security:**
- Content-addressed store means builds are reproducible
- Flake lock files pin exact input revisions (content hashes)
- Registry is convenience-only (CLI shorthand); `flake.lock` in repos uses full refs
- System/user registries intentionally not used in `flake.nix` inputs (prevents ambient config from changing builds)

**Friction points:**
- Still marked "experimental" after years, creating ecosystem uncertainty
- The flakeref syntax is powerful but complex (many URL schemes)
- Registry vs. lock file distinction confuses newcomers
- DeterminateSystems (major Nix vendor) is deprecating registry entries in `flake.nix` inputs

### 7. mise / aqua

**Adding a source (aqua):**
In `aqua.yaml`:
```yaml
registries:
  - type: standard    # built-in default
  - name: custom
    type: github_content
    repo_owner: myorg
    repo_name: aqua-registry
    ref: v1.0.0
    path: registry.yaml
  - name: local
    type: local
    path: ./registry.yaml
```

**Installing from a source (mise):**
```bash
mise use aqua:owner/repo          # explicit backend
mise use owner/repo               # auto-resolved from registry
mise install node@20              # from default registry
```

**Trust/security:**
- aqua: only Standard Registry allowed by default; custom registries require explicit Policy
- Cosign signature verification for standard registry
- Checksum verification for all downloads
- mise: aqua backend is preferred because it offers checksums without requiring plugins

**Friction points:**
- aqua's policy system for allowing custom registries adds setup overhead
- mise's registry lookup order (aqua > github > plugins) is implicit
- No unified cross-backend install syntax in mise (need `aqua:`, `github:`, etc. prefixes)

## Implications

### Shared patterns across systems

1. **Two-level naming**: Every system uses `source/package` in some form. Homebrew: `user/repo/formula`. npm: `@scope/package`. Go: `host/owner/repo`. Cargo: `--registry name crate`. The slash-separated hierarchy is universal.

2. **Default source elision**: All systems let users omit the source for the default/central registry. `brew install vim`, `npm install lodash`, `cargo install ripgrep` -- no source qualifier needed. This is table stakes for tsuku.

3. **Config-file vs. CLI registration**: Systems split between config-file-only (Cargo, npm, Go env vars) and CLI-based (Homebrew `tap`, Nix `registry add`). The CLI approach has lower friction for first-time setup.

4. **Fully-qualified install as one-step**: Homebrew and Nix allow install-from-source without prior registration. `brew install user/repo/formula` auto-taps. `nix run github:owner/repo#pkg` just works. This is the pattern tsuku's proposed syntax follows.

5. **Fallback chains**: Go's GOPROXY supports comma-separated fallbacks. Nix flake inputs can override registry entries. pip's `--extra-index-url` adds (dangerously) to the search set. Fallback is useful but must be ordered and deterministic to avoid confusion attacks.

### For tsuku's install syntax

The proposed syntax `tsuku install owner/repo:recipe@tag` maps well to established patterns:
- It's closest to Nix flakes (`github:owner/repo#output`) and Go modules (`host/path@version`)
- The `:recipe` separator (vs. `/` or `#`) is a reasonable choice to distinguish "repo" from "recipe within repo"
- Omitting the host to default to GitHub matches Go's convention and is intuitive given GitHub's dominance

**Key design decisions that fall out of this research:**

1. **One-step install without prior "tap"**: Follow Homebrew and Nix -- `tsuku install owner/repo:recipe@v1.0` should work without a separate registration step. Registration can still exist for aliasing/caching.

2. **Package-to-source binding must be explicit**: pip's `--extra-index-url` disaster shows that ambiguous source resolution (highest version wins across all sources) is a security hazard. Each recipe should resolve from exactly one source.

3. **Named aliases for convenience, full refs for reproducibility**: Like Nix's registry (CLI convenience) vs. flake.lock (deterministic builds). tsuku could support `tsuku install myalias:tool` where `myalias` maps to a full GitHub ref.

4. **Trust defaults should be restrictive**: aqua's "standard registry only by default, policy for custom" is the right model. Third-party registries should require explicit opt-in.

## Surprises

1. **pip's dependency confusion is still unfixed in default behavior** (2026). Despite being disclosed in 2021 and causing real supply-chain attacks, `--extra-index-url` still searches all indexes and picks the highest version. PEP 708 exists but adoption is slow. This underscores how hard it is to change defaults once shipped.

2. **Go's environment-variable-only configuration** for proxy/private modules is surprisingly awkward for a modern tool. Three separate variables (GOPROXY, GOPRIVATE, GONOSUMDB) for one concept. Enterprise teams report ongoing friction configuring these correctly.

3. **Homebrew requires formulae to be in a tap** -- you can't just point at a local Ruby file. This seems unnecessarily rigid and is a recurring complaint. tsuku should support `tsuku install ./path/to/recipe.toml` as a first-class citizen.

4. **aqua's "policy" system for custom registries** is a good security model but adds friction. It's an interesting tradeoff that tsuku should study -- maybe a `tsuku trust owner/repo` one-time command that's simpler than a YAML policy file.

5. **Cargo's `--registry` flag is ignored for transitive dependencies** of installed crates. The binary comes from your registry but its deps resolve from crates.io. This is a fundamental design gap that tsuku won't face (recipes are self-contained, not dependency trees).

## Open Questions

1. **Discovery**: How should users find recipes in third-party registries? Homebrew has no cross-tap search. npm search only covers the default registry. Should tsuku support `tsuku search --registry owner/repo query`?

2. **Recipe name conflicts**: What happens when `owner-a/repo:tool` and `owner-b/repo:tool` both exist and the user runs `tsuku install tool`? Homebrew warns but doesn't prevent this. The current issue doesn't address resolution priority for recipes with identical names across sources.

3. **Registry metadata caching**: Homebrew clones entire tap repos. Go proxies cache transparently. What's the right caching granularity for tsuku? Cloning whole repos is heavy; fetching individual recipe files is light but loses atomicity.

4. **Version provider interaction**: If a third-party registry recipe uses a version provider (e.g., GitHub releases), does that version provider configuration come from the registry or from tsuku's core? This affects how self-contained third-party recipes can be.

5. **Authentication**: Several systems (npm, Cargo, Go) have fragmented auth stories. How will tsuku handle private GitHub repos as registries? Should it reuse `gh auth` tokens, or have its own credential store?

6. **Lockfile/pinning for managed environments**: If a team uses `tsuku.toml` to define their toolset, should it pin registry refs (like Nix's flake.lock) or float to latest?

## Summary

Every major package manager uses a two-level `source/package` naming scheme with default-source elision, but they diverge sharply on whether sources require pre-registration (Cargo, npm) or support one-step fully-qualified install (Homebrew, Nix) -- and the one-step pattern has significantly lower friction. The critical security lesson from pip's ongoing dependency confusion disaster is that package-to-source binding must be deterministic and explicit, never "search all sources and pick highest version." The biggest open question for tsuku is how to handle recipe name conflicts across registries and what the default trust model should be when a user first references a third-party source.
