# Lead: Trust and security models for third-party sources

## Findings

### Homebrew Taps

**Trust boundary:** The Homebrew maintainer team for official taps; the tap owner for third-party taps.

**User opt-in:** `brew tap foo/bar` (explicit) or `brew install foo/bar/baz` (implicit). Once tapped, formulae are available like any other package. This is classic trust-on-first-use (TOFU).

**Verification:** Official taps go through maintainer code review and CI. Third-party taps have no verification beyond whatever the tap owner does. Formulae are Ruby scripts -- executable code that runs during installation. Historically, there was no cryptographic verification of tap content at all.

**Recent improvements:** Homebrew is adopting Sigstore-based signing and SLSA provenance for bottles. The trust proposition is: "brew tap foo/bar establishes that Sigstore identities tied to github.com/foo/bar can sign bottles for that tap." This upgrades from pure TOFU to TOFU + cryptographic binding.

**Known incidents:**
- A Trail of Bits audit found 25 vulnerabilities including path traversal that could execute code as root during cask installation.
- 20 casks in the official tap downloaded over HTTP with no integrity checks, vulnerable to MITM.
- Attackers have used unofficial taps to distribute malware, particularly targeting macOS users.

**Key takeaway:** Homebrew's model prioritizes convenience (any GitHub repo can be a tap) at the cost of security. The formulae-as-code design means trust in a tap is trust in arbitrary code execution.

Sources:
- [Security and the Homebrew contribution model](https://workbrew.com/blog/security-and-the-homebrew-contribution-model)
- [Brew Hijack: Serving Malware Over Homebrew's Core Tap](https://www.koi.ai/blog/brew-hijack-serving-malware)
- [Build Provenance and Code-signing for Homebrew](https://repos.openssf.org/proposals/build-provenance-and-code-signing-for-homebrew.html)
- [Taps (Third-Party Repositories) - Homebrew Documentation](https://docs.brew.sh/Taps)

---

### Cargo Alternative Registries

**Trust boundary:** Each registry is an independent trust domain. crates.io is the default; alternatives must be explicitly configured.

**User opt-in:** Registries must be added to `.cargo/config.toml` with a name and index URL. Dependencies then reference the registry by name in `Cargo.toml` (`registry = "my-registry"`). This is explicit opt-in -- there's no way to accidentally pull from an alternative registry.

**Verification:** Registries serve a git-based or sparse index with package metadata. The registry's `config.json` controls download URLs and API endpoints. Authentication is supported via credential providers (OS keychain integration). Cargo verifies checksums from the index against downloaded crate files.

**Attacks possible:**
- A compromised registry could serve malicious crates, but only for projects that explicitly reference that registry.
- No cross-registry confusion attacks because registry affiliation is explicit per-dependency.
- The registry operator is fully trusted for packages in their registry.

**Key takeaway:** Cargo's model is the most restrictive -- registries are explicitly configured and each dependency declares its registry. This eliminates confusion attacks but adds friction for third-party sources.

Sources:
- [RFC 2141: Alternative Registries](https://rust-lang.github.io/rfcs/2141-alternative-registries.html)
- [RFC 3139: Cargo Alternative Registry Auth](https://rust-lang.github.io/rfcs/3139-cargo-alternative-registry-auth.html)
- [Registries - The Cargo Book](https://doc.rust-lang.org/cargo/reference/registries.html)

---

### Go Module Proxy and Checksum Database (sumdb)

**Trust boundary:** Module authors publish to any VCS host. The checksum database (sum.golang.org) is a global append-only log operated by Google.

**User opt-in:** Enabled by default. All module downloads are verified against sumdb unless the user explicitly opts out via `GONOSUMDB` or `GONOSUMCHECK` for specific modules. Private modules use `GOPRIVATE`.

**Verification:** When `go` downloads a module, it computes the hash of the module content and compares it against the checksum database. The database uses a Merkle tree (transparency log), which means:
- Inclusion proofs verify a specific hash exists in the log.
- Consistency proofs verify the log hasn't been tampered with (append-only property).
- The go command stores known tree state locally, detecting rollback attacks.

**Attacks mitigated:**
- Module proxy compromise: The proxy is outside the trusted computing base. Even a compromised proxy can't serve different content because checksums are verified against sumdb.
- Targeted attacks: Because sumdb is a transparency log, an attacker can't serve different content to different users without being detected (the tree would be inconsistent).
- First-use attacks: Even the first download of a module is verified if any other user has already fetched it.

**Attacks still possible:**
- If you're the first user of a module, sumdb records whatever the VCS host serves. A compromised VCS host could serve malicious content that gets "blessed" by sumdb.
- Private module exclusions (`GOPRIVATE`) bypass all verification.

**Key takeaway:** Go's model is the gold standard for integrity verification. The transparency log means you don't trust the proxy, the CDN, or any intermediary -- only the module author's VCS and the sumdb operator. However, it verifies integrity (content hasn't changed), not trust (content is safe).

Sources:
- [Proposal: Secure the Public Go Module Ecosystem](https://go.googlesource.com/proposal/+/master/design/25530-sumdb.md)
- [Module Mirror and Checksum Database Launched](https://blog.golang.org/module-mirror-launch)
- [Go modules services](https://proxy.golang.org/)

---

### npm Scoped Registries

**Trust boundary:** Each scope (@org/...) can map to a different registry via `.npmrc` configuration.

**User opt-in:** Scopes are configured in `.npmrc` (per-project or per-user). A scope maps to a registry URL: `@myco:registry=https://registry.myco.com`. This is explicit configuration but applies at the scope level, not per-package.

**Verification:** npm verifies package integrity using `integrity` fields in `package-lock.json` (SRI hashes). The registry itself is trusted to serve correct metadata. There's no transparency log or cross-registry verification.

**Attacks possible:**
- Dependency confusion: If a scoped package falls back to the public registry, an attacker can publish a same-named package there. The mitigation is to configure the private registry to not fall back.
- Registry compromise gives full control over all packages in that scope.
- Credential leakage from `.npmrc` files committed to version control.

**Key takeaway:** npm's scoped registries solve namespace isolation (preventing confusion between public and private packages) but don't provide cryptographic verification of registry content. Trust is entirely in the registry operator.

Sources:
- [Securing your software supply chain with scoped registries](https://blog.packagecloud.io/tactics-securing-your-software-supply-chain-with-scoped-registries/)
- [Scope - npm Docs](https://docs.npmjs.com/cli/v11/using-npm/scope/)
- [Dependency Confusion with a private npm registry](https://medium.com/smallcase-engineering/security-dependency-confusion-with-a-private-npm-registry-88cea461f9a5)

---

### Nix Flakes

**Trust boundary:** Each flake input is a git repository (or other fetchable source). nixpkgs (the official package set) is the primary trusted source. Third-party flakes are trusted by the user adding them.

**User opt-in:** Flake inputs are declared in `flake.nix` and pinned via `flake.lock` (content-addressed hashes). Adding a new input requires editing `flake.nix`. Updating pins (`nix flake update`) re-fetches and re-hashes.

**Verification:** Flake lock files contain content hashes (NAR hashes) for all inputs. This pins exact content. However:
- Lock files only protect you if you already have them. On first use or update, you're trusting whatever the source serves.
- `nixConfig` in flakes can configure substitute (binary cache) servers, potentially directing Nix to fetch pre-built binaries from untrusted sources.

**Security mechanisms:**
- Nix has "trusted users" and "trusted substituters" settings.
- Paranoid mode: Nix can prompt before accepting `nixConfig` from flakes.
- Determinate Nix is adding provenance data and signing for store paths.

**Attacks possible:**
- A malicious flake's `nixConfig` can add untrusted binary caches, serving backdoored binaries.
- Flake updates trust the upstream source completely.
- Typosquatting on flake input URLs.

**Key takeaway:** Nix flakes provide strong content-addressing (once pinned, content is verified), but the trust model is weak at the boundaries: first use, updates, and nixConfig. The content-addressing is for integrity, not trust.

Sources:
- [What is nixConfig, Should You Trust It?](https://notashelf.dev/posts/reject-flake-content)
- [Trust model for nixpkgs](https://discourse.nixos.org/t/trust-model-for-nixpkgs/9450)
- [Introducing the Nix Flake Checker](https://determinate.systems/blog/flake-checker/)

---

### Docker Content Trust (DCT)

**Trust boundary:** Image publishers sign images; consumers verify signatures. Uses The Update Framework (TUF) via Notary.

**User opt-in:** Enabled by setting `DOCKER_CONTENT_TRUST=1`. When enabled, only signed images can be pulled. Disabled by default.

**Verification:** Full cryptographic chain:
- Root keys (offline, high security)
- Repository keys (per-image, used for signing)
- Timestamp keys (server-managed, provide freshness)
- Delegation keys (allow teams to manage signing authority)

**Attacks mitigated:** Compromise resilience via key hierarchy. Replay attacks prevented by timestamp metadata. Rollback attacks detectable.

**Key takeaway:** Docker's model is the most sophisticated cryptographically (based on TUF), but adoption has been low due to complexity. DCT is being deprecated (removal March 2028) in favor of Notary v2/Sigstore. The lesson: a security model that's too complex for users to adopt provides no security at all.

Sources:
- [Content trust in Docker](https://docs.docker.com/engine/security/trust/)
- [Docker Content Trust: What It Is and How It Secures Container Images](https://www.trendmicro.com/vinfo/us/security/news/virtualization-and-cloud/docker-content-trust-what-it-is-and-how-it-secures-container-images)
- [Signing container images: Comparing Sigstore, Notary, and Docker Content Trust](https://snyk.io/blog/signing-container-images/)

---

### mise and aqua

**Trust boundary:** mise has a curated registry mapping short names to backends (aqua, asdf plugins, etc.). The aqua backend downloads binaries from GitHub releases with verification.

**User opt-in:** Tools in the mise registry can be installed by short name. Third-party plugins (asdf-style) require explicit trust. mise has a "paranoid mode" where only trusted config files and first-party/mise-org plugins are allowed.

**Verification (aqua backend):**
- Checksum verification (SHA256 against aqua-registry metadata)
- Cosign signature verification (keyless, Sigstore-based)
- SLSA provenance verification
- GitHub Artifact Attestations
- Minisign signatures
- All enabled by default, configurable via environment variables

**Trust layers:**
1. mise registry (curated mapping of tool names to backends) -- maintained by mise team
2. aqua-registry (tool download metadata, checksums, verification config) -- community-maintained, reviewed
3. Tool publisher (actual binary on GitHub) -- verified via checksums/signatures

**Attacks mitigated:**
- Supply chain attacks on tool publishers (cosign/SLSA verification)
- Plugin supply chain attacks (mise-plugins org consolidation)
- Untrusted config injection (paranoid mode, `mise trust` command)

**Key takeaway:** mise/aqua represents the current state of the art for developer tool managers. They layer multiple verification methods, default to the most secure options, and provide escape hatches for corporate/private use. The key insight is separating the "registry" (which tool to install) from "verification" (is the downloaded artifact authentic).

Sources:
- [Registry - mise-en-place](https://mise.jdx.dev/registry.html)
- [Aqua Backend - mise-en-place](https://mise.jdx.dev/dev-tools/backends/aqua.html)
- [Paranoid mode - mise-en-place](https://mise.jdx.dev/paranoid.html)
- [Cosign and SLSA Provenance - aqua](https://aquaproj.github.io/docs/reference/security/cosign-slsa/)
- [Checksum Verification - aqua](https://aquaproj.github.io/docs/reference/security/checksum/)

---

## Implications

### For tsuku's trust model

**The fundamental tension:** tsuku recipes can execute arbitrary installation steps (download, extract, chmod, run scripts). This means trusting a recipe is equivalent to trusting arbitrary code execution -- the same threat level as Homebrew formulae, not the safer level of "just downloading a binary."

**Recommended approach -- layered trust with explicit registry opt-in:**

1. **Official registry is default and curated.** The built-in tsuku recipe registry is the only source enabled out of the box. All recipes go through review. This is the npm/cargo "default registry" model.

2. **Third-party registries require explicit registration.** Like Cargo's `.cargo/config.toml` approach, users must explicitly add a registry with `tsuku registry add <name> <url>`. No implicit trust. No "tap by installing" like Homebrew.

3. **Recipe pinning with content hashes.** Like Nix flake locks and Go's go.sum, record the hash of each recipe at install time. Detect if a recipe changes unexpectedly on update. This catches compromised registries serving modified recipes.

4. **Start without cryptographic signing, plan for it later.** Docker's experience shows that complex signing infrastructure that nobody uses provides no security. Start with the simpler mechanisms (explicit opt-in, content hashing) and add Sigstore/cosign verification when there are actual users and third-party registries to protect against.

5. **Separate recipe trust from artifact trust.** Following mise/aqua's insight: the recipe (which tool, which version, where to download) is one trust domain; the downloaded artifact (is this binary authentic) is another. Recipe checksums protect against recipe tampering. Artifact checksums (already in tsuku recipes) protect against download tampering.

### What NOT to do

- **Don't allow implicit trust** (Homebrew's `brew install foo/bar/baz` automatically tapping). Every third-party source should require a deliberate, separate action.
- **Don't build a complex signing infrastructure before you have users.** Docker Content Trust's deprecation is a cautionary tale.
- **Don't treat all registries as equivalent.** The official registry should have a different (higher) trust level than third-party registries, reflected in the UI (e.g., warnings when installing from third-party sources).

### Recommended trust levels

| Source | Trust Level | Verification | User Action Required |
|--------|------------|--------------|---------------------|
| Official registry | High | Recipe review + checksums | None (default) |
| Registered third-party | Medium | Content hashing, warning on change | `tsuku registry add` |
| Local recipes | User-controlled | None (user wrote them) | File path reference |

## Surprises

1. **Docker Content Trust is being deprecated.** The most cryptographically sophisticated model (TUF-based) failed in practice because the complexity barrier was too high. Simpler approaches (Sigstore keyless signing) are replacing it.

2. **Go's sumdb doesn't verify trust, only integrity.** It guarantees everyone gets the same content for a given module version, but it doesn't verify that the content is safe. The first publisher's content is "blessed" by being recorded. This is a common misconception.

3. **Homebrew is retroactively adding Sigstore signing.** The TOFU model is being upgraded, but the fundamental issue (formulae are executable Ruby) remains. Signing bottles helps, but the formula itself is still trusted code.

4. **mise/aqua's verification stack is more advanced than most OS package managers.** Multiple independent verification methods (cosign, SLSA, GitHub attestations, minisign) all enabled by default is unusual in the developer tooling space.

5. **npm's dependency confusion vulnerability** exists specifically because of the fallback behavior between scoped registries and the public registry. Cargo avoids this entirely by requiring explicit registry declarations per-dependency.

## Open Questions

1. **Should tsuku recipes be allowed to run arbitrary scripts?** The current action system (download, extract, chmod, install_binaries) is relatively constrained compared to Homebrew formulae. Could the action set be restricted enough that recipe trust becomes less critical? Or is arbitrary execution a required feature?

2. **What's the update story?** Go's sumdb handles this well (every version is independently verified). If a third-party registry updates a recipe, how does tsuku detect and handle the change? Is it a new version, or a mutation of an existing version?

3. **Should registries be git repositories (like Homebrew taps and Cargo registries) or API services (like npm)?** Git repos give you content-addressing and history for free. API services are more flexible but harder to verify.

4. **Is there value in a transparency log for tsuku?** Given the small user base, probably not now. But the design should not preclude adding one later (like Go did).

5. **How should registry priority/conflict resolution work?** If the same tool name exists in the official registry and a third-party registry, which wins? Cargo's answer (explicit per-dependency) is safest. Homebrew's answer (last-tapped wins) is convenient but error-prone.

## Summary

Every package manager sits on a spectrum from convenience (Homebrew's implicit taps) to security (Cargo's explicit per-dependency registry declarations), and the most cryptographically ambitious approach (Docker Content Trust) failed due to adoption friction while simpler models like Go's sumdb succeeded. For tsuku, the right starting point is explicit registry registration with content-hash pinning -- simple enough to ship without users, secure enough to prevent the classes of attacks that have hit Homebrew and npm, and extensible toward Sigstore-based signing when third-party registries actually exist. The biggest open question is whether tsuku's recipe action system can be constrained enough to reduce the trust requirement, or whether arbitrary execution capability means recipe trust will always be equivalent to code trust.
