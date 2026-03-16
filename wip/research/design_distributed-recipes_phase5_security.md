# Security Review: distributed-recipes

## Dimension Analysis

### Download Verification

**Applies:** Yes

**Context:** The distributed provider fetches recipe TOML files from GitHub via two
mechanisms: (1) the GitHub Contents API (`api.github.com`) for directory listing,
and (2) `raw.githubusercontent.com` for individual file content. The design
specifies using `httputil.NewSecureClient` for these requests.

**Risks:**

1. **Recipe TOML is fetched without integrity verification (Severity: Medium).**
   The design fetches `.tsuku-recipes/*.toml` from HEAD of the default branch.
   There is no checksum, signature, or content-hash pinning on the recipe file
   itself. The PRD explicitly lists "Cryptographic signing" and "Content-hash
   pinning" as out of scope. This means a compromised GitHub account, a
   man-in-the-middle on the raw content CDN, or a malicious force-push can
   silently alter a recipe between install and update.

   This is distinct from the tool binary verification that already exists. The
   existing `download` action (in `internal/actions/download.go`) supports
   `checksum_url` and PGP signature verification for the *binaries* a recipe
   points to. But the recipe itself -- which defines *which* URLs to download
   from, *which* checksums to expect, and *which* actions to run -- has no
   integrity check. A tampered recipe can redirect binary downloads to attacker-
   controlled URLs with attacker-provided checksums, making the binary-level
   verification useless.

2. **No pinning to git ref (Severity: Low-Medium).** The design always fetches
   from HEAD. The PRD acknowledges this as a known limitation ("No recipe change
   detection"). Between an initial install and a subsequent update, the recipe
   content could change arbitrarily without any signal to the user.

**Existing mitigations in the codebase:**

- `httputil.NewSecureClient` provides SSRF protection (private/loopback/link-local
  IP blocking in `internal/httputil/ssrf.go`), HTTPS-only redirects, DNS rebinding
  guards, and decompression bomb protection. Verified in source.
- The `download` action enforces HTTPS-only (`strings.HasPrefix(url, "https://")`
  at line 352 of `internal/actions/download.go`).
- Binary downloads support checksum and PGP signature verification.

**Recommendations:**

- The design should note that recipe-level integrity is deferred but is a
  prerequisite for any use in enterprise or high-security contexts.
- Consider recording a content hash of the recipe TOML at install time in
  `state.json`. On update, warn if the recipe content changed. This is low-cost
  and doesn't require cryptographic signing infrastructure.
- The implementer should ensure the distributed provider also enforces HTTPS-only
  for both the Contents API and raw content fetches (not just relying on the
  hardcoded `api.github.com` and `raw.githubusercontent.com` prefixes -- validate
  the `download_url` field returned by the Contents API before following it).

### Execution Isolation

**Applies:** Yes

**Context:** Distributed recipes use the same TOML format and action system as
central registry recipes. They execute the same actions (`download`, `extract`,
`chmod`, `install_binaries`, etc.) with the same permissions as the tsuku process.

**Risks:**

1. **Distributed recipes execute with full user privileges (Severity: Medium).**
   This is the same trust model as central registry recipes, but the trust
   boundary is different. Central registry recipes are reviewed via PR to the
   monorepo. Distributed recipes come from arbitrary GitHub repositories with
   no review process. A malicious distributed recipe could use actions like
   `shell_command` (if it exists) or exploit path traversal in `install_binaries`
   to write outside `$TSUKU_HOME`.

2. **Auto-registration weakens the trust decision (Severity: Medium).** With
   `strict_registries` off (the default), `tsuku install owner/repo` auto-
   registers the source and executes the recipe in a single command. There is
   no confirmation prompt. The user's only signal is the `owner/repo` prefix
   in the command they typed. This is comparable to `curl | sh` -- the user
   has decided to trust the source, but there's no intermediate review step.

**Existing mitigations:**

- `strict_registries` mode blocks auto-registration and requires explicit
  `tsuku registry add`. This is the right escape hatch for security-conscious
  users and teams.
- File system scope is limited to `$TSUKU_HOME` by convention (actions write to
  `WorkDir` and `InstallDir`), though this isn't enforced by a sandbox.
- No `setuid`, no privilege escalation -- tsuku runs as the invoking user.

**Recommendations:**

- The design should require that the first install from a new distributed source
  prints a clear trust warning (e.g., "Installing from owner/repo for the first
  time. This will execute recipe-defined actions with your user permissions.").
  This is cheap and makes the trust decision visible.
- Document that `strict_registries = true` is the recommended setting for teams
  and CI environments.

### Supply Chain Risks

**Applies:** Yes

This is the most significant security dimension for this design.

**Risks:**

1. **Unreviewed recipe execution from arbitrary sources (Severity: High).** The
   central registry has a PR-based review process. Distributed recipes bypass
   this entirely. A recipe is a program: it specifies URLs to download, commands
   to run, and files to install. Anyone with a GitHub account can create a repo
   with `.tsuku-recipes/` and the user executes it with `tsuku install owner/repo`.

2. **Recipe mutation without detection (Severity: High).** Since recipes are
   fetched from HEAD with no content pinning, a compromised upstream can:
   - Change the download URL to point to a malicious binary
   - Change the checksum to match the malicious binary
   - Add new actions (e.g., a post-install script)
   - All of this happens silently on `tsuku update`

   The PRD's "Known Limitations" section acknowledges this: "A malicious registry
   could silently change a recipe's content." The stated mitigation ("explicit
   trust decision via auto-register or strict mode") doesn't address the mutation
   case -- the user trusted the source at install time, not at update time.

3. **Typosquatting (Severity: Medium).** `tsuku install org/tool` vs
   `tsuku install 0rg/tool`. The design has no verification that the `owner/repo`
   is the canonical source for a given tool. R5 (central registry priority for
   unqualified names) prevents name confusion between central and distributed,
   but not between two distributed sources.

4. **GitHub account compromise (Severity: Medium).** If `owner`'s GitHub account
   is compromised, all users who installed from `owner/repo` will get malicious
   recipes on their next update. This is the same risk as any GitHub-hosted
   software, but tsuku's auto-update mechanism (`tsuku update`) automates the
   execution path.

**Existing mitigations:**

- R5 prevents distributed sources from shadowing central registry names.
- `strict_registries` allows locking down sources.
- The binary download path supports checksum and PGP verification, but only
  if the recipe specifies them -- and the recipe is the untrusted input.

**Recommendations:**

- Add a "Security Considerations" section to the design doc (see below).
- Content-hash pinning should be prioritized as a fast-follow, not deferred
  indefinitely. Record `sha256(recipe_toml)` in `state.json` at install time.
  On update, if the hash changed, show a diff summary and require `--force` or
  `--accept-recipe-changes`.
- Consider a `tsuku audit` or `tsuku verify --recipe` command that shows what
  a distributed recipe will do before executing it (dry-run mode).

### User Data Exposure

**Applies:** Yes, but low severity.

**Risks:**

1. **GitHub token sent to GitHub APIs (Severity: Low).** The design uses
   `GITHUB_TOKEN` via the `secrets` package for Contents API calls to raise rate
   limits. The token is sent to `api.github.com` -- the intended recipient. The
   raw content fetches don't use auth. The `secrets` package resolves tokens from
   env vars or `config.toml` (verified in `internal/secrets/secrets.go`).

2. **Telemetry may include distributed source identifiers (Severity: Low).** The
   existing telemetry `Event` struct includes `Recipe` (recipe name). If
   distributed installs send `owner/repo` as the recipe name, the telemetry
   backend receives information about which distributed sources users install from.
   This reveals user preferences but not PII. The telemetry system is opt-out
   (`Telemetry: true` by default in `internal/userconfig/userconfig.go`).

3. **Registry configuration stored locally (Severity: Low).** The list of
   registered distributed sources in `config.toml` reveals which external
   sources a user trusts. This is local-only and not transmitted anywhere
   (unless telemetry is extended).

**Existing mitigations:**

- Telemetry is opt-out via `tsuku config set telemetry false`.
- `config.toml` is saved with 0600 permissions (verified in
  `internal/userconfig/userconfig.go`, `saveToPath` method).
- The `Sanitizer` in `internal/validate/sanitize.go` redacts credentials and
  home paths from strings sent to LLM APIs.
- `GITHUB_TOKEN` is only sent over HTTPS to GitHub's API.

**Recommendations:**

- Decide whether telemetry events for distributed installs should include the
  full `owner/repo` source or just a flag indicating "distributed". The current
  telemetry schema doesn't have a `source` field, so this is a design choice
  for the implementer.

## Recommended Outcome

**OPTION 2 - Document considerations:**

The design is sound architecturally. The security risks are real but acknowledged
in the PRD's "Known Limitations" and "Out of Scope" sections. No design changes
are strictly needed for v1, but the implementer needs clear guidance on what to
build defensively. Here is a draft Security Considerations section for the design
doc:

---

### Security Considerations

**Trust model.** Distributed recipes execute with the same permissions as central
registry recipes but without the central registry's PR-based review process. Users
who run `tsuku install owner/repo` are trusting that repository's maintainers with
arbitrary file system access under their user account. This is the same trust model
as `go install`, `cargo install`, or `pip install` from arbitrary sources.

**Recipe integrity.** v1 does not verify recipe content integrity. Recipes are
fetched from HEAD over HTTPS, which protects against network-level tampering but
not against upstream compromise (account takeover, malicious force-push). Binary
integrity is protected by checksum/signature verification defined in the recipe,
but if the recipe itself is tampered, those verification parameters are also
compromised.

Implementer requirements:
- Validate that `download_url` values returned by the GitHub Contents API use
  HTTPS before following them.
- Record `sha256(recipe_toml_bytes)` in `state.json` alongside the `Source` field.
  This doesn't block mutation but creates an audit trail for a future
  `--accept-recipe-changes` flow.
- Print a one-time trust warning on first install from a new distributed source.

**Strict mode.** Teams and CI environments should set `strict_registries = true`
to prevent auto-registration. Document this in the `tsuku registry` help text and
in the security section of the website.

**Token handling.** `GITHUB_TOKEN` is sent only to `api.github.com` over HTTPS.
Raw content fetches do not include authentication headers. The token is resolved
through the existing `secrets` package, which checks environment variables and
`config.toml` (stored with 0600 permissions).

**Telemetry.** Decide whether distributed install events include the full
`owner/repo` source identifier or an opaque "distributed" tag. Full identifiers
enable usage analytics but reveal user-source relationships to the telemetry
backend.

---

## Summary

The design's security posture is appropriate for a v1 that targets individual
developers (the same audience comfortable with `go install github.com/...`). The
main risk is recipe mutation without detection -- a compromised upstream can
silently alter what gets installed on the next `tsuku update`. This risk is
acknowledged in the PRD and deferred via "content-hash pinning" in Out of Scope.
The recommended mitigations (trust warning, recipe hash recording, strict mode
documentation) are low-cost additions that don't change the design's architecture
but meaningfully improve the security baseline.
