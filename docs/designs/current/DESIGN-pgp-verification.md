---
status: Current
problem: Tsuku recipes cannot verify downloads for projects that only provide PGP signatures instead of checksums, limiting adoption for security-critical tools like curl.
decision: Use ProtonMail's gopenpgp v2 library with fingerprint-based key management, allowing recipes to specify signature and public key URLs with expected fingerprints for verification.
rationale: This approach provides strong cryptographic verification (fingerprints are version-controlled in recipes), works for any project without requiring tsuku key management, and uses a well-maintained production library. The trade-offs of adding a dependency and requiring recipe authors to obtain fingerprints are acceptable for a security-critical feature.
---

# Design: PGP Signature Verification for Downloads

## Status

Current
- **Issue**: #682
- **Author**: @dangazineu
- **Created**: 2025-12-27

## Context and Problem Statement

Tsuku currently supports checksum-based verification for downloads. The `download` action accepts a `checksum_url` parameter that downloads a checksum file and validates the downloaded content against it. This works for most tools.

**The problem:**

Some projects intentionally do not provide checksums because they consider them insufficient for security. As Daniel Stenberg (curl maintainer) explains: "providing checksums next to the downloads adds almost no extra verification. If someone can tamper with the tarballs, they can probably update the webpage with a fake checksum as well."

Instead, these projects provide cryptographic signatures using PGP/GPG. Signatures are stronger because:

1. **Authenticity**: Signatures prove the file was created by someone with the private key
2. **Offline verification**: The signing key never needs to be on the download server
3. **Tamper resistance**: Attackers cannot forge signatures without the private key

Curl provides `.asc` files (armored detached signatures) for all releases, signed by the maintainer's personal GPG key. Currently, tsuku recipes for curl cannot have upstream verification because the download action only supports `checksum_url`.

**Threat model:**

The primary threat is a compromised download server or CDN that serves malicious files. Checksums do not protect against this because an attacker who controls the server can update both the tarball and the checksum file. PGP signatures prevent this attack because the signing key is stored offline and never accessible to the compromised server.

**Why this matters now:**

Tsuku aims to provide reproducible, verified installations. Forcing recipes to skip verification for tools like curl undermines this goal. Users installing security-sensitive tools like curl deserve the strongest available verification.

### Scope

**In scope:**
- PGP signature verification for downloads (detached signatures)
- Key management for well-known project keys
- Recipe parameters for specifying signature URLs and keys
- Armored signature format (`.asc` files)

**Out of scope:**
- Inline/attached signatures
- Key generation or signing functionality
- Automatic key discovery (e.g., keyserver lookups)
- Minisign or other signature formats
- SLSA provenance attestation

## Decision Drivers

1. **No external dependencies**: tsuku must remain self-contained (no system GPG requirement)
2. **Recipe simplicity**: Common cases should require minimal configuration
3. **Security**: Key management must not introduce new attack vectors
4. **Maintainability**: Use well-maintained libraries, avoid deprecated code
5. **Consistency**: Signature verification should integrate cleanly with existing download flow
6. **Auditability**: Users should be able to inspect and verify the keys tsuku uses

## Implementation Context

### Existing Patterns

**Similar implementations:**
- `download.go:verifyChecksum()` - Downloads checksum file, parses it, validates against file hash
- `util.go:VerifyChecksum()` - Hash computation and comparison
- `util.go:ReadChecksumFile()` - Flexible checksum file parsing

**Current download action flow:**
1. Download file to `WorkDir`
2. If `checksum_url` provided, download checksum file
3. Parse checksum from file (various formats supported)
4. Compute file hash and compare

**Conventions to follow:**
- Actions use `GetString()`, `GetBool()` helpers for parameter extraction
- Error messages are descriptive with context
- Temporary files are cleaned up after use
- HTTPS is enforced for all downloads

**Anti-patterns to avoid:**
- Don't add external binary dependencies (e.g., system GPG)
- Don't add inline static values for dynamic content (use URLs)

### Applicable Specifications

**Standard**: RFC 4880 (OpenPGP Message Format)
**Relevance**: Defines signature packet format, key format, armor encoding
**Source**: https://datatracker.ietf.org/doc/rfc4880/

### Go Library Options

| Library | Maintenance | Notes |
|---------|-------------|-------|
| `golang.org/x/crypto/openpgp` | Deprecated | Security fixes only, no new features |
| `github.com/ProtonMail/gopenpgp/v2` | Active | High-level API, used by Proton products |
| `github.com/ProtonMail/go-crypto` | Active | Fork of x/crypto/openpgp with fixes |

ProtonMail's gopenpgp provides a clean API for signature verification:
```go
sig, _ := crypto.NewPGPSignatureFromArmored(ascContent)
keyRing := crypto.NewKeyRing(publicKey)
err := keyRing.VerifyDetached(message, sig, verifyTime)
```

### Research Summary

**Patterns to follow:**
- Mirror `verifyChecksum()` structure: download verification file, parse, validate
- Use existing download infrastructure for fetching `.asc` files
- Store keys in a discoverable location for auditability

**Specifications to comply with:**
- RFC 4880 for signature format
- Armored ASCII format for `.asc` files

**Implementation approach:**
- Add `signature_url` and `signature_key` parameters to download action
- Create a key registry in `$TSUKU_HOME/keys/` or embedded in binary
- Use ProtonMail's gopenpgp for pure Go implementation

## Considered Options

This design addresses two independent decisions:

### Decision 1: OpenPGP Library

Which Go library should tsuku use for PGP signature verification?

#### Option 1A: ProtonMail gopenpgp

Use `github.com/ProtonMail/gopenpgp/v2` for signature verification.

**Pros:**
- Actively maintained by Proton (security-focused company)
- High-level API designed for common use cases
- Supports RFC 9580 (latest OpenPGP standard)
- Used in production by Proton Mail products
- Well-documented with examples

**Cons:**
- Adds a new dependency to tsuku
- Pulls in transitive dependencies (Proton's go-crypto fork)
- API is opinionated - may not map perfectly to our use case

#### Option 1B: ProtonMail go-crypto

Use `github.com/ProtonMail/go-crypto/openpgp` (their fork of x/crypto).

**Pros:**
- Drop-in replacement for deprecated x/crypto/openpgp
- Lower-level API offers more control
- Actively maintained with security fixes
- Smaller dependency footprint than gopenpgp

**Cons:**
- Lower-level API requires more code
- Less documentation than gopenpgp
- Must handle armor parsing, keyring management manually

#### Option 1C: golang.org/x/crypto/openpgp

Use the standard library's openpgp package.

**Pros:**
- No new dependencies
- API is familiar to Go developers
- Part of the extended standard library

**Cons:**
- Officially deprecated (security fixes only, explicitly marked for removal)
- Missing AEAD support for encrypted data
- Default key generation uses weak parameters
- No RFC 9580 support (latest OpenPGP standard)
- Known vulnerabilities will not be fixed
- Not recommended for any new code

### Decision 2: Key Management

How should tsuku store and manage PGP public keys for verification?

#### Option 2A: Embedded Key Registry

Embed well-known project keys in the tsuku binary as Go code.

```go
var KnownKeys = map[string]string{
    "curl": `-----BEGIN PGP PUBLIC KEY BLOCK-----
    ...
    -----END PGP PUBLIC KEY BLOCK-----`,
}
```

**Pros:**
- No additional files to manage
- Keys ship with tsuku, always available
- Tamper-resistant (binary is verified)
- Simple to use in recipes (`signature_key = "curl"`)

**Cons:**
- Requires tsuku release to add/update keys
- Binary size increases with each key
- Keys cannot be updated independently
- Limited to keys we choose to embed

#### Option 2B: External Key Directory

Store keys in `$TSUKU_HOME/keys/` as `.asc` files.

```
~/.tsuku/keys/
├── curl.asc
├── gnupg.asc
└── user-keys/     # User-provided keys
    └── custom.asc
```

**Pros:**
- Keys can be updated via `tsuku update-keys` command
- Users can add custom keys without tsuku release
- Keys are inspectable (just text files)
- Separation of concerns (code vs. data)

**Cons:**
- Additional bootstrap step (download keys before first use)
- Keys must be secured (file permissions)
- More complex error handling (missing key file)
- Potential TOCTOU issues with key files

#### Option 2C: Recipe-Specified Key URL

Recipes provide a URL to fetch the key, cached locally.

```toml
[[steps]]
action = "download"
url = "https://curl.se/download/curl-{version}.tar.gz"
signature_url = "https://curl.se/download/curl-{version}.tar.gz.asc"
signature_key_url = "https://daniel.haxx.se/mykey.asc"
```

**Pros:**
- No key management in tsuku core
- Keys always come from the authoritative source
- Works for any project without tsuku changes
- Natural fit with existing URL-based parameters

**Cons:**
- Key must be fetched on first install (adds latency)
- Key URL could change or become unavailable
- No trust anchor - how do we verify the key itself?
- **Security limitation**: Provides minimal improvement over checksums - an attacker who compromises the download server can likely also compromise the key URL, serving both malicious files and malicious keys
- Essentially Trust On First Use (TOFU) with TLS, not true cryptographic verification

#### Option 2D: Hybrid Approach

Embed critical keys, allow external keys, support key URLs as fallback.

```toml
# Use embedded key
signature_key = "curl"

# Or reference external key file
signature_key_file = "custom.asc"

# Or fetch from URL (cached)
signature_key_url = "https://example.com/key.asc"
```

**Pros:**
- Best of all worlds: convenience for common keys, flexibility for others
- Embedded keys provide trust anchor for well-known projects
- Users can add custom keys without waiting for tsuku release
- URL fallback works for any project

**Cons:**
- More complex implementation
- Multiple code paths to test
- Documentation must explain all options
- Precedence rules needed (which key source wins?)

#### Option 2E: Fingerprint-Based Verification

Recipe specifies the key fingerprint; key is fetched from URL and validated against fingerprint.

```toml
[[steps]]
action = "download"
url = "https://curl.se/download/curl-{version}.tar.gz"
signature_url = "https://curl.se/download/curl-{version}.tar.gz.asc"
signature_key_url = "https://daniel.haxx.se/mykey.asc"
signature_key_fingerprint = "27EDEAF22F3ABCEB50DB9A125CC908FDB71E12C2"
```

**Pros:**
- Strong security: fingerprint in recipe is version-controlled and auditable
- Flexible: key can be fetched from any URL
- No need to embed keys in tsuku binary
- Fingerprints are small and stable (don't change with key metadata updates)
- Works for any project without tsuku changes

**Cons:**
- Recipe authors must obtain and verify fingerprints manually
- Fingerprints are long and error-prone to transcribe
- Key must still be fetched on first use (network dependency)
- Fingerprint verification adds complexity to implementation

### Evaluation Against Decision Drivers

| Option | No Dependencies | Recipe Simplicity | Security | Maintainability | Consistency | Auditability |
|--------|-----------------|-------------------|----------|-----------------|-------------|--------------|
| **Library** | | | | | | |
| 1A: gopenpgp | Fair (new dep) | Good | Good | Good | Good | Good |
| 1B: go-crypto | Fair (new dep) | Fair | Good | Good | Good | Good |
| 1C: x/crypto | Good (existing) | Fair | Poor | Poor | Good | Good |
| **Key Mgmt** | | | | | | |
| 2A: Embedded | Good | Good | Good | Fair | Good | Poor |
| 2B: External | Good | Fair | Fair | Good | Good | Good |
| 2C: Key URL | Good | Good | Poor | Good | Good | Good |
| 2D: Hybrid | Good | Good | Good | Fair | Fair | Good |
| 2E: Fingerprint | Good | Fair | Good | Good | Good | Good |

### Uncertainties

- **Performance impact**: Signature verification adds computation and one HTTP request. Impact on install time is unknown but expected to be negligible.
- **gopenpgp version**: v2 is stable and widely deployed. v3 is now also stable (v3.3.0) with RFC 9580 support and cleaner API. Need to evaluate whether v3's benefits outweigh being newer.
- **Key rollover**: How do projects handle key expiration or rotation? We may need to support multiple valid keys per project.
- **Key expiration handling**: Should tsuku fail on expired keys, warn, or ignore expiration? Many legitimate old releases are signed with now-expired keys.
- **Signature formats**: Some projects may use non-armored signatures (binary `.sig` files) or inline-signed files. Testing against real-world signatures is needed.

## Decision Outcome

**Chosen: 1A (gopenpgp v2) + 2E (Fingerprint-Based Verification)**

### Summary

We will use ProtonMail's gopenpgp v2 library for signature verification, with fingerprint-based key management. Recipes specify a key fingerprint and URL; tsuku fetches the key and validates it against the fingerprint before verifying the signature. This provides strong security (fingerprint is version-controlled in the recipe) with flexibility (any project can be supported without tsuku changes).

### Rationale

**Library choice (1A: gopenpgp v2):**
- Best addresses maintainability driver: actively maintained by security-focused team
- High-level API reduces implementation complexity and risk of misuse
- Well-documented with production deployment experience
- v2 over v3: v2 is more mature with broader adoption; v3 can be considered in a future iteration

**Key management choice (2E: Fingerprint-Based):**
- Best addresses security driver: fingerprint provides cryptographic binding to the correct key
- Best addresses auditability driver: fingerprint in recipe is inspectable and version-controlled
- Best addresses maintainability driver: no embedded keys means no tsuku releases for key updates
- Addresses recipe simplicity driver adequately: slightly more complex than embedded keys, but works for any project

**Alternatives rejected:**

| Option | Reason |
|--------|--------|
| 1B: go-crypto | Lower-level API increases implementation risk without clear benefits |
| 1C: x/crypto/openpgp | Deprecated with security issues; unsuitable for new code |
| 2A: Embedded | Poor auditability; requires tsuku releases for key updates |
| 2B: External directory | Additional bootstrap complexity; file permission concerns |
| 2C: Key URL only | Provides minimal security improvement over checksums (TOFU model) |
| 2D: Hybrid | Added complexity without clear benefit over fingerprint-based approach |

### Trade-offs Accepted

By choosing this option, we accept:

1. **New dependency**: gopenpgp adds ~2MB to binary size and introduces transitive dependencies. This is acceptable because signature verification is a security-critical feature that benefits from a well-maintained library.

2. **Recipe complexity**: Authors must obtain and verify key fingerprints. This is acceptable because it's a one-time effort per tool, and incorrect fingerprints will fail obviously during testing.

3. **Network dependency for key fetch**: First installation requires fetching the key. This is acceptable because we already fetch the file and signature; one more fetch is negligible, and keys are cached after first use.

4. **No embedded keys for convenience**: Recipe authors can't just write `signature_key = "curl"`. This is acceptable because fingerprint-based verification provides stronger security and doesn't require tsuku to maintain a key registry.

## Solution Architecture

### Overview

Signature verification extends the existing download action with three new optional parameters. When provided, tsuku downloads the signature file, fetches and validates the public key against the fingerprint, then verifies the signature before accepting the download.

### Recipe Parameters

```toml
[[steps]]
action = "download"
url = "https://curl.se/download/curl-{version}.tar.gz"
signature_url = "https://curl.se/download/curl-{version}.tar.gz.asc"
signature_key_url = "https://daniel.haxx.se/mykey.asc"
signature_key_fingerprint = "27EDEAF22F3ABCEB50DB9A125CC908FDB71E12C2"
```

| Parameter | Required | Description |
|-----------|----------|-------------|
| `signature_url` | Yes (for PGP) | URL to detached signature file (`.asc`) |
| `signature_key_url` | Yes (for PGP) | URL to armored public key |
| `signature_key_fingerprint` | Yes (for PGP) | Expected SHA-1 fingerprint of the key (40 hex chars) |

**Note**: `signature_url` and `checksum_url` are mutually exclusive. A recipe should use one verification method, not both. If both are provided, Preflight validation returns an error.

### Preflight Validation

The `download` action's Preflight method will be extended with these new checks:

| Condition | Result |
|-----------|--------|
| `signature_url` without `signature_key_url` or `signature_key_fingerprint` | Error: all three signature params required together |
| `signature_url` AND `checksum_url` both provided | Error: mutually exclusive verification methods |
| `signature_key_fingerprint` not 40 hex chars | Error: invalid fingerprint format |
| No `checksum_url` and no `signature_url` | Warning: no upstream verification (unchanged from current behavior) |
| `signature_url` but URL has no placeholders | Warning: should use download_file instead |

### Components

```
┌──────────────────────────────────────────────────────────────────┐
│                       download.go                                 │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │                    Execute()                                 │ │
│  │  1. Download file                                            │ │
│  │  2. If checksum_url: verifyChecksum()                        │ │
│  │  3. If signature_url: verifySignature() ◄── NEW              │ │
│  └─────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│                    signature.go (NEW)                             │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │  VerifySignature(file, sigURL, keyURL, fingerprint)          │ │
│  │    1. Download signature file                                │ │
│  │    2. Fetch or load cached key                               │ │
│  │    3. Validate key fingerprint                               │ │
│  │    4. Verify signature using gopenpgp                        │ │
│  └─────────────────────────────────────────────────────────────┘ │
│  ┌─────────────────────────────────────────────────────────────┐ │
│  │  KeyCache                                                    │ │
│  │    - Store: $TSUKU_HOME/cache/keys/<fingerprint>.asc         │ │
│  │    - Lookup by fingerprint                                   │ │
│  │    - Validate fingerprint on load                            │ │
│  └─────────────────────────────────────────────────────────────┘ │
└──────────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────────┐
│              github.com/ProtonMail/gopenpgp/v2                    │
│    - NewKeyFromArmored()                                          │
│    - KeyRing.VerifyDetached()                                     │
│    - Key.GetFingerprint()                                         │
└──────────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **Download phase**: File is downloaded to `WorkDir` (existing behavior)

2. **Signature verification phase** (new, after checksum verification):
   ```
   signature_url ──► Download .asc file to temp
                         │
                         ▼
   signature_key_url ──► Fetch key (or load from cache)
                         │
                         ▼
   signature_key_fingerprint ──► Validate key fingerprint
                         │
                         ▼
   gopenpgp.VerifyDetached(file, signature, key)
                         │
                         ▼
                    Success or Error
   ```

3. **Key caching**: Keys are stored in `$TSUKU_HOME/cache/keys/` by fingerprint:
   - Cache key: `<fingerprint>.asc`
   - On cache hit: validate fingerprint matches before use
   - On cache miss: fetch from URL, validate, then cache

### Key Interfaces

```go
// VerifyPGPSignature verifies a file's signature using a PGP public key.
// The key is fetched from keyURL and validated against expectedFingerprint.
// Returns nil on success, error with details on failure.
func VerifyPGPSignature(
    ctx context.Context,
    filePath string,
    signatureURL string,
    keyURL string,
    expectedFingerprint string,
) error

// PGPKeyCache manages cached public keys.
type PGPKeyCache struct {
    cacheDir string // $TSUKU_HOME/cache/keys/
}

// Get retrieves a key by fingerprint, fetching from URL if not cached.
// Returns the armored key and validates it matches the expected fingerprint.
func (c *PGPKeyCache) Get(
    ctx context.Context,
    fingerprint string,
    keyURL string,
) (*crypto.Key, error)
```

## Implementation Approach

### Phase 1: Core Signature Verification

Add the signature verification capability:

1. Add gopenpgp/v2 dependency to go.mod
2. Create `internal/actions/signature.go`:
   - `VerifyPGPSignature()` function
   - `PGPKeyCache` for key management
   - Fingerprint validation helper
3. Extend `download.go`:
   - Add `signature_url`, `signature_key_url`, `signature_key_fingerprint` parameters
   - Call `VerifyPGPSignature()` after download completes
4. Update Preflight validation in `download.go`:
   - If any signature param is provided, require all three together
   - Validate `signature_key_fingerprint` is exactly 40 hex characters (case-insensitive)
   - Error if both `checksum_url` and `signature_url` provided (mutually exclusive)
   - Update "no verification" warning to accept either `checksum_url` OR `signature_url`
   - Warn if signature params provided but URL has no placeholders (should use download_file with static fingerprint instead)
5. Add key size limit (~100KB) and fetch timeout to signature.go
6. Add `KeyCacheDir` field to `ExecutionContext` struct

**Dependencies**: None

### Phase 2: Recipe Integration

Update recipes and validation:

1. Update curl recipe to use signature verification
2. Add validator check for signature parameters
3. Add tests:
   - Unit tests for fingerprint validation, key caching
   - Integration test with real curl signature
   - Test fingerprint mismatch rejection
   - Test signature for wrong file rejection

**Dependencies**: Phase 1 complete

### Phase 3: Documentation and Polish

1. Update recipe authoring docs with signature verification guide:
   - How to find and verify key fingerprints
   - Key rotation process for recipe maintainers
2. Add troubleshooting section for common PGP errors
3. Add `tsuku verify-signature` debug command (optional)

**Dependencies**: Phase 2 complete

## Security Considerations

### Download Verification

This feature directly improves download verification by adding cryptographic signature support.

**Current state**: Tsuku verifies downloads using SHA-256 checksums from `checksum_url`. This protects against transmission errors and mirrors serving outdated files, but not against a compromised upstream server.

**With this feature**: Downloads can be verified using PGP signatures, which protect against:
- Compromised download servers or CDNs
- Modified checksums on a compromised server
- Man-in-the-middle attacks (signature verification is independent of TLS)

**Verification failure behavior**: If signature verification fails, the download is rejected and an error is displayed. The file is deleted from the work directory. Users cannot bypass verification without modifying the recipe.

### Execution Isolation

**File system access**:
- Keys are cached in `$TSUKU_HOME/cache/keys/` with restricted permissions (0600)
- Signature files are downloaded to temp and deleted after verification
- No new directories or elevated permissions required

**Network access**:
- Fetches signature files from `signature_url` (already trusted domain)
- Fetches public keys from `signature_key_url` (potentially different domain)
- All fetches use HTTPS (enforced by existing download infrastructure)

**Privilege escalation**: None. Signature verification runs with the same permissions as the existing download action.

### Supply Chain Risks

**Key trust model**:
- Trust is established via fingerprint, which is checked into the recipe repository
- Recipe changes (including fingerprint changes) go through code review
- Fingerprint acts as a cryptographic commitment to a specific key

**Key URL compromise**:
- If `signature_key_url` is compromised, the attacker cannot forge a valid key
- The fetched key must match the fingerprint in the recipe
- Attacker would need to control both the key URL AND modify the recipe

**Upstream key rotation**:
- If a project rotates their signing key, the recipe fingerprint must be updated
- Old releases remain verifiable with old fingerprint
- Version-specific fingerprints could be supported via `{version}` in fingerprint (future)

**gopenpgp supply chain**:
- gopenpgp is maintained by Proton, a security-focused company
- Library is used in Proton Mail and other production systems
- Dependency is pinned in go.mod with checksum in go.sum

### User Data Exposure

**Data accessed**:
- Downloaded file content (read for signature verification)
- Public keys (cached, no sensitive data)
- No user credentials, environment variables, or personal data accessed

**Data transmitted**:
- HTTP requests to signature URL (typically same domain as download)
- HTTP requests to key URL (specified in recipe)
- No user-identifying information beyond standard HTTP headers

**Privacy implications**: None. This feature accesses only files and URLs explicitly specified in the recipe.

### Mitigations

| Risk | Mitigation | Residual Risk |
|------|------------|---------------|
| Malicious key at `signature_key_url` | Fingerprint validation rejects keys that don't match | Attacker with recipe write access could update fingerprint |
| Cache poisoning with wrong key | Keys cached by fingerprint; re-validated on load | None - fingerprint is cryptographic hash |
| Signature stripping (remove sig, claim unsigned) | Recipe requires signature; verification is mandatory when specified | None - can't skip without recipe modification |
| Expired signing key | Verify signature math only, not key expiration | Old keys with poor crypto could be weak (mitigated by fingerprint) |
| gopenpgp vulnerability | Use stable v2, monitor for security advisories | Zero-day in library |
| Key URL unavailable | Cache keys after first successful fetch | First install of new tool requires network |

## Consequences

### Positive

- **Stronger verification**: Recipes can use cryptographic signatures instead of (or in addition to) checksums
- **Curl support**: The curl recipe can now have upstream verification
- **Future-proof**: Framework supports any project that uses PGP signatures
- **Auditable**: Fingerprints in recipes are version-controlled and inspectable
- **No central registry**: tsuku doesn't need to maintain a list of trusted keys

### Negative

- **Increased binary size**: gopenpgp adds ~2MB to the tsuku binary
- **Recipe complexity**: Authors must find and verify key fingerprints
- **Network dependency**: Key fetch required on first install of each tool
- **Error complexity**: PGP errors can be cryptic; need good error messages

### Mitigations

- **Binary size**: Acceptable trade-off for security feature; 2MB is negligible for a CLI tool
- **Recipe complexity**: Provide documentation and examples; fingerprint is a one-time effort
- **Network dependency**: Keys are cached; subsequent installs use cache
- **Error complexity**: Wrap gopenpgp errors with user-friendly messages; include troubleshooting guide

