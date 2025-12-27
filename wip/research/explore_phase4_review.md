# Phase 4 Review: PGP Signature Verification Design

## Problem Statement Analysis

### Strengths

The problem statement is well-articulated with clear motivation:

1. **Concrete example**: Curl is used as a primary motivator, with a direct quote from Daniel Stenberg explaining why checksums are insufficient. This grounds the problem in reality rather than abstract security concerns.

2. **Clear technical gap**: The current `checksum_url` parameter limitation is explicitly stated, making it clear what capability is missing.

3. **Security rationale is sound**: The three-point explanation (authenticity, offline verification, tamper resistance) accurately describes why signatures are stronger than checksums.

4. **Scope boundaries are explicit**: The in-scope/out-of-scope lists help prevent scope creep and set expectations.

### Weaknesses

1. **Missing threat model**: The problem statement assumes signatures are necessary but does not articulate the specific threat being mitigated. Who is the attacker? What are they capable of? A web host compromise? A MITM attack? This affects design choices (e.g., where keys should be stored).

2. **No quantification of the problem**: How many recipes currently use `skip_verification_reason`? How many tools provide only PGP signatures? This would help prioritize the feature.

3. **"Why now" is weak**: The statement says tsuku "aims to provide reproducible, verified installations" but does not explain why this is urgent or what triggered the investigation.

### Recommendations

- Add explicit threat model section defining attacker capabilities
- Quantify the number of affected recipes/tools
- Consider whether the problem warrants the complexity of PGP verification or if alternative approaches (like SLSA attestation for future-proofing) should also be evaluated

---

## Missing Alternatives Check

### Decision 1: OpenPGP Library

The three options presented (gopenpgp v2, go-crypto fork, x/crypto/openpgp) are the main Go library options for OpenPGP. However, there are some missing considerations:

**Potentially Missing Options:**

1. **gopenpgp v3**: The design mentions v3 is in development but does not evaluate it. v3 is now stable (v3.3.0) and offers a cleaner API with RFC 9580 support. Should be explicitly considered or dismissed with rationale.

2. **Sequoia-PGP (sqv/rpgp)**: While not Go-native, Sequoia provides a Rust library (rpgp) that could be used via CGO. This is worth dismissing explicitly given tsuku's "no external dependencies" requirement.

3. **Vendor GPG binary**: Bundle a stripped-down GPG binary. This should be dismissed but currently is not mentioned.

4. **Minisign support**: While explicitly out of scope, the design does not explain why minisign is excluded when some projects (especially Go projects like age, mkcert) use it. This omission may frustrate recipe authors later.

### Decision 2: Key Management

The four options (embedded, external directory, key URL, hybrid) cover the main approaches but miss some nuances:

**Potentially Missing Options:**

1. **Per-recipe inline key**: Allow recipes to embed the public key directly in TOML:
   ```toml
   signature_key_inline = """
   -----BEGIN PGP PUBLIC KEY BLOCK-----
   ...
   """
   ```
   This is similar to 2A but at recipe granularity rather than in tsuku binary. Could be useful for less common keys.

2. **Web of Trust / Key verification via HTTPS**: Use TLS certificate chain to bootstrap trust when fetching keys. Some package managers (notably apt) use this model where the initial key is trusted because it came over HTTPS from a trusted domain.

3. **Key fingerprint verification**: Recipe specifies expected key fingerprint rather than full key. Key is fetched from URL but verified against fingerprint. Combines flexibility of 2C with some security of 2A/2D.

4. **Keyserver lookup (explicitly dismissed)**: The scope says "out of scope: automatic key discovery" but does not say why. Keyservers are largely abandoned, but a brief rationale would help.

---

## Options Fairness Review

### Library Options

| Option | Assessment |
|--------|------------|
| **1A (gopenpgp)** | Fair. Pros and cons are balanced. Dependency concern is valid but minor for a security feature. |
| **1B (go-crypto)** | Fair. "Less documentation than gopenpgp" is subjective but generally true. |
| **1C (x/crypto/openpgp)** | **Potentially understated cons**. Beyond deprecation, there are known security issues (e.g., weak key generation defaults, signature stripping attacks). The con "known issues that won't be fixed" should be more specific. |

**Concern**: Option 1C is presented fairly but its security issues are understated. This could lead to someone choosing it to avoid dependencies when it has real security problems.

### Key Management Options

| Option | Assessment |
|--------|------------|
| **2A (Embedded)** | Fair. The con "binary size increases with each key" is valid but impact is minimal (~1-2KB per key). |
| **2B (External directory)** | Fair. "TOCTOU issues" is a sophisticated concern that most designs overlook. Good inclusion. |
| **2C (Key URL)** | **Potentially missing con**: Key URLs could be blocked by corporate proxies/firewalls, causing installation failures. The "Vulnerable if key URL is compromised" con is stated but understates the risk - this is essentially TOFU (Trust On First Use) with no verification, which is arguably no better than checksum verification from a compromised server. |
| **2D (Hybrid)** | Fair. "More complex implementation" is honest. The precedence question is important and correctly flagged. |

**Concern**: Option 2C's security model is not sufficiently criticized. If an attacker compromises curl.se, they can provide both a malicious tarball AND a malicious key at the key URL. This option provides very limited security improvement over checksums for the curl threat model.

---

## Unstated Assumptions

### Technical Assumptions

1. **Signatures are always detached**: The design assumes `.asc` files are detached signatures. Some projects provide inline-signed files (e.g., OpenSSL historically). This may break for some tools.

2. **One key per project**: The current design assumes each project has one signing key. In reality:
   - Projects may have multiple valid signers
   - Keys expire and rotate
   - Old releases may be signed with old keys

3. **Armored format only**: Only ASCII-armored `.asc` signatures are considered. Binary `.sig` files exist but are less common.

4. **HTTPS is sufficient for key download**: Option 2C assumes fetching keys over HTTPS provides adequate security. This is TOFU with TLS, which may or may not meet the security bar.

### Business Assumptions

1. **Recipe authors will maintain keys**: If keys are embedded (2A) or directory-based (2B), someone must update them when keys rotate. Who maintains this?

2. **Users trust tsuku's key selection**: Embedded keys mean users trust tsuku developers to vet and include correct keys. This centralizes trust.

3. **Signature verification is optional**: The design assumes recipes can still skip verification. If the goal is security, should verified recipes be the default/required?

### Missing Context

1. **Performance impact**: No benchmarks or estimates for signature verification overhead.

2. **Error handling UX**: What happens when verification fails? Is the error message user-friendly? Can users override?

3. **Offline behavior**: Can users install tools offline if they have cached downloads but need to verify signatures?

---

## Strawman Detection

### Assessment

None of the options appear to be deliberate strawmen designed to fail. However:

1. **Option 1C (x/crypto/openpgp)** is effectively a non-starter due to deprecation. While not a strawman (it's a real option some might consider), it exists mainly to dismiss the "use stdlib" argument. This is acceptable design practice.

2. **Option 2C (Key URL)** has significant security weaknesses that make it unsuitable as a standalone solution. It exists to address "what if a project doesn't have an embedded key" but its inclusion may give false confidence that it provides security. It should be more clearly positioned as a fallback with limited security properties.

### Recommendation

The document should be more explicit that:
- 1C should only be used if there's a compelling reason to avoid any dependencies
- 2C provides very limited security improvement and should only be used as a last resort or in combination with fingerprint verification

---

## Recommendations

### Immediate Changes

1. **Add threat model**: Define what attack is being prevented. Recommended wording:
   > **Threat**: A compromised download server (or CDN) serves malicious files. PGP signatures prevent this because the signing key is offline and not on the compromised server.

2. **Strengthen 1C cons**: Add specific security issues:
   > - Missing AEAD support for encrypted data
   > - Default key generation uses weak parameters
   > - No RFC 9580 support (latest standard)

3. **Clarify 2C limitations**: Add a clear statement:
   > Key URL verification provides limited security improvement over checksums because both can be compromised by an attacker who controls the download server. Use only as a fallback when embedded keys are unavailable.

4. **Consider gopenpgp v3**: Evaluate v3 explicitly. It's now stable and offers cleaner API with better defaults.

### Additional Options to Consider

1. **Fingerprint-based verification**: Recipe specifies key fingerprint, key is fetched from URL and validated against fingerprint. This provides strong security (fingerprint in recipe is version-controlled) with flexibility (key can be fetched from anywhere).

   ```toml
   signature_url = "https://curl.se/download/curl-{version}.tar.gz.asc"
   signature_key_url = "https://daniel.haxx.se/mykey.asc"
   signature_key_fingerprint = "27EDEAF22F3ABCEB50DB9A125CC908FDB71E12C2"
   ```

2. **Key expiration handling**: Consider how to handle key expiration. Options:
   - Ignore expiration (signatures from expired keys still verify)
   - Warn on expiration
   - Fail on expiration (too strict for many scenarios)

### Decision Matrix Update

The evaluation matrix should include a "Security Model" row:

| Option | Security Model |
|--------|----------------|
| 2A Embedded | Trust tsuku developers to vet keys |
| 2B External | Trust file system integrity + key update process |
| 2C Key URL | Trust On First Use (TOFU) with TLS |
| 2D Hybrid | Layered: embedded for critical tools, TOFU for others |

---

## Summary

The design document is well-structured and covers the main decision points. The problem statement could be strengthened with an explicit threat model. The library options are fair, though x/crypto/openpgp's security issues should be more prominent. The key management options miss the fingerprint-based approach which provides a good balance of security and flexibility.

Key recommendations:
1. Add explicit threat model
2. Consider gopenpgp v3
3. Add fingerprint-based key verification as an option
4. Clarify that Key URL (2C) provides minimal security improvement
5. Address key expiration and rotation in design
