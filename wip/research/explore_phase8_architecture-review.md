# Architecture Review: PGP Signature Verification

## Executive Summary

The proposed solution architecture for PGP signature verification is **well-designed and implementation-ready**. The design correctly integrates with existing patterns in the codebase, particularly mirroring the `verifyChecksum()` flow. The key choices (gopenpgp v2, fingerprint-based verification) are sound. This review identifies a few refinements and clarifications needed before implementation.

**Overall Assessment**: Ready for implementation with minor refinements.

---

## Detailed Analysis

### 1. Architecture Clarity

**Verdict**: Clear and sufficient for implementation.

The architecture diagram in the design doc clearly shows:
- Where new code integrates (`download.go` calls `verifySignature()`)
- The new module (`signature.go` with `VerifyPGPSignature()` and `PGPKeyCache`)
- The data flow (download -> signature download -> key fetch/cache -> fingerprint validation -> verify)
- The library integration points (`gopenpgp/v2` functions)

**Minor Clarification Needed**: The design mentions calling `verifySignature()` "after checksum verification" but the current `download.go:Execute()` calls `verifyChecksum()` after download at line 271. The implementation should decide:
- Option A: Signature verification replaces checksum verification (mutually exclusive)
- Option B: Both can be specified (belt and suspenders)

Recommendation: **Option A** - keep them mutually exclusive. The design doc mentions this is for tools that "intentionally do not provide checksums." If both were provided, it would create confusion about which verification failed.

### 2. Component Analysis

#### 2.1 Download Action Extension

**Current Structure** (`download.go`):
```go
func (a *DownloadAction) Execute(ctx *ExecutionContext, params map[string]interface{}) error {
    // ... download file ...

    // Verify checksum if provided
    if err := a.verifyChecksum(ctx.Context, ctx, params, destPath, vars); err != nil {
        return fmt.Errorf("checksum verification failed: %w", err)
    }
    // ... cache and return ...
}
```

**Proposed Extension** (lines 174-290 in download.go):

The design correctly identifies where to add signature verification - after the download but before caching. The existing pattern is:

1. Download file (line 266)
2. Verify checksum (line 271) - new signature verification would go here
3. Save to cache (line 277)

**Integration Point**: Add new private method `verifySignature()` following the same pattern as `verifyChecksum()`.

#### 2.2 New signature.go Module

**Proposed Interfaces**:
```go
func VerifyPGPSignature(ctx context.Context, filePath string, signatureURL string, keyURL string, expectedFingerprint string) error

type PGPKeyCache struct {
    cacheDir string
}
func (c *PGPKeyCache) Get(ctx context.Context, fingerprint string, keyURL string) (*crypto.Key, error)
```

**Assessment**: These interfaces are well-designed:
- `VerifyPGPSignature()` is a standalone function (not a method on DownloadAction), enabling unit testing
- `PGPKeyCache` is a simple struct with a single responsibility
- The fingerprint parameter enables cache lookup without network

**Refinement**: The `ExecutionContext` should be extended to include a key cache directory, similar to `DownloadCacheDir`. Proposed addition to `action.go`:
```go
type ExecutionContext struct {
    // ... existing fields ...
    KeyCacheDir string // PGP key cache directory ($TSUKU_HOME/cache/keys/)
}
```

#### 2.3 Preflight Validation

The design mentions updating Preflight but doesn't provide detail. Based on the existing pattern in `download.go:Preflight()` (lines 32-73), the new validation should:

1. **Require all three signature params together**:
```go
sigURL, hasSigURL := GetString(params, "signature_url")
keyURL, hasKeyURL := GetString(params, "signature_key_url")
fingerprint, hasFP := GetString(params, "signature_key_fingerprint")

if hasSigURL || hasKeyURL || hasFP {
    // All three must be present
    if !hasSigURL {
        result.AddError("signature_key_url provided without signature_url")
    }
    if !hasKeyURL {
        result.AddError("signature_url provided without signature_key_url")
    }
    if !hasFP {
        result.AddError("signature_url provided without signature_key_fingerprint")
    }
}
```

2. **Validate fingerprint format**:
```go
if hasFP {
    cleaned := strings.ReplaceAll(strings.ToUpper(fingerprint), " ", "")
    if len(cleaned) != 40 || !isHexString(cleaned) {
        result.AddError("signature_key_fingerprint must be 40 hexadecimal characters")
    }
}
```

3. **Warn if both checksum_url and signature_url are provided**:
```go
if hasChecksumURL && hasSigURL {
    result.AddWarning("both checksum_url and signature_url provided; signature verification is stronger")
}
```

### 3. Missing Components

#### 3.1 Signature File Parsing

The design assumes `.asc` files are armored PGP signatures. gopenpgp handles this with:
```go
sig, err := crypto.NewPGPSignatureFromArmored(ascContent)
```

However, some projects may use binary `.sig` files. The design explicitly lists this as out of scope, which is appropriate for the first implementation. The code should validate the signature format and provide a clear error if non-armored signatures are encountered.

#### 3.2 Error Messages

The design mentions "good error messages" but doesn't specify them. Key error scenarios:

| Scenario | Error Message |
|----------|---------------|
| Signature download fails | "failed to download signature from {url}: {error}" |
| Key download fails | "failed to download public key from {url}: {error}" |
| Fingerprint mismatch | "key fingerprint mismatch: expected {expected}, got {actual}" |
| Signature verification fails | "signature verification failed: file may have been tampered with" |
| Invalid signature format | "signature file is not in armored ASCII format; binary signatures are not supported" |
| Key parsing fails | "failed to parse public key: {error}" |

#### 3.3 HTTP Download Reuse

The design mentions using "existing download infrastructure" for fetching `.asc` files, but doesn't clarify how. The current `downloadFile()` method is private to `DownloadAction`.

**Recommendation**: Extract the HTTP download logic to a shared package-level function:
```go
// downloadFileHTTP already exists in download_file.go (line 127)
// It can be reused directly since it's package-level
```

Actually, looking at the code, `downloadFileHTTP()` is already a package-level function in `download_file.go:127`. This can be reused for downloading signature and key files.

### 4. Implementation Phase Sequencing

**Current Phases**:
1. Core Signature Verification (gopenpgp dep, signature.go, download.go extension, Preflight)
2. Recipe Integration (curl recipe, validator, integration test)
3. Documentation and Polish

**Assessment**: Phases are correctly sequenced with appropriate dependencies.

**Refinement**: Phase 1 should be split to enable incremental testing:

**Phase 1a: Infrastructure**
- Add gopenpgp/v2 to go.mod
- Create signature.go with `VerifyPGPSignature()` and `PGPKeyCache`
- Unit tests for signature.go (using test keys)

**Phase 1b: Integration**
- Extend download.go with new parameters
- Update Preflight validation
- Integration test with mock server

**Phase 2**: Recipe Integration (unchanged)

**Phase 3**: Documentation (unchanged)

### 5. Simpler Alternatives Analysis

The design considered several alternatives. Let me evaluate if any simpler option was overlooked:

#### 5.1 Shell Out to GPG

**Not considered in design**. Would be simpler but violates "no system dependencies" driver. Correctly rejected by design philosophy.

#### 5.2 Minisign (simpler than PGP)

Listed as out of scope. This is a reasonable deferral - PGP has broader adoption among projects like curl, GnuPG, etc. Minisign could be added later as a separate feature.

#### 5.3 Embedded Keys + Fingerprint Fallback

A hybrid where well-known keys are embedded (for convenience) but fingerprint verification is always performed (for security). This wasn't fully explored but would add complexity without clear benefit over pure fingerprint-based approach.

**Conclusion**: No simpler alternatives were overlooked that would meet all decision drivers.

### 6. Security Analysis

The design's security considerations are thorough. A few additional points:

#### 6.1 Key Rotation Handling

The design notes this as an uncertainty. For the curl recipe specifically, Daniel Stenberg's key fingerprint is `27EDEAF22F3ABCEB50DB9A125CC908FDB71E12C2`. This key doesn't expire (no expiration set). However, if a project rotates keys, the recipe would need updating.

**Future Enhancement**: Support version-specific fingerprints:
```toml
signature_key_fingerprint = "{version_fingerprint}"
[version_fingerprints]
"8.0.0" = "NEW_KEY_FINGERPRINT"
"*" = "27EDEAF22F3ABCEB50DB9A125CC908FDB71E12C2"
```

This is out of scope for initial implementation but worth noting as a design consideration.

#### 6.2 Cache Invalidation

Keys are cached by fingerprint, which is cryptographically bound to the key content. This is secure but creates a question: what if a key URL changes but fingerprint stays the same (e.g., different mirror)?

**Answer**: This is fine - the fingerprint is the trust anchor, not the URL. If the fingerprint matches, the key is valid regardless of where it was downloaded from.

#### 6.3 Time-of-Check-Time-of-Use (TOCTOU)

The design verifies the signature then the caller proceeds to use the file. If an attacker could modify the file between verification and use, the signature would pass but malicious content would execute.

**Mitigation**: This is already handled by tsuku's architecture - downloaded files are immediately extracted/installed in the same execution flow. There's no window for external modification.

### 7. Test Strategy

The design doesn't detail the test strategy. Recommended approach:

#### Unit Tests (signature_test.go)

```go
func TestVerifyPGPSignature_ValidSignature(t *testing.T)
func TestVerifyPGPSignature_InvalidSignature(t *testing.T)
func TestVerifyPGPSignature_FingerprintMismatch(t *testing.T)
func TestVerifyPGPSignature_MalformedKey(t *testing.T)
func TestVerifyPGPSignature_MalformedSignature(t *testing.T)

func TestPGPKeyCache_CacheMiss(t *testing.T)
func TestPGPKeyCache_CacheHit(t *testing.T)
func TestPGPKeyCache_FingerprintValidation(t *testing.T)
```

Test fixtures needed:
- A test keypair (can be generated once and committed)
- A signed test file
- An invalid signature
- A key with wrong fingerprint

#### Integration Tests

```go
func TestDownloadAction_WithSignatureVerification(t *testing.T)
func TestDownloadAction_SignatureVerificationFails(t *testing.T)
```

These would use httptest.NewTLSServer to serve test files, signatures, and keys.

---

## Recommendations

### Must Have (Blockers)

1. **Clarify mutual exclusivity**: Decide whether `checksum_url` and `signature_url` are mutually exclusive or can coexist. Recommend mutually exclusive.

2. **Add KeyCacheDir to ExecutionContext**: The execution context needs to know where to cache keys.

3. **Specify error messages**: Add the error message table to the design doc for consistency.

### Should Have (Improvements)

4. **Split Phase 1**: Separate infrastructure from integration for incremental testing.

5. **Add test strategy section**: Document the unit and integration test approach.

6. **Document fingerprint format flexibility**: Allow spaces and mixed case in fingerprints (normalize before comparison).

### Nice to Have (Future)

7. **Version-specific fingerprints**: Design for future support of key rotation.

8. **Minisign support**: Document as future enhancement for projects using Ed25519 signatures.

---

## Conclusion

The solution architecture is well-designed and ready for implementation. The fingerprint-based verification approach is the correct choice for security without requiring tsuku to maintain a key registry. The gopenpgp v2 library is the right dependency choice.

The main refinements needed are:
- Clarifying the relationship between checksum and signature verification
- Adding the key cache directory to ExecutionContext
- Documenting test strategy

With these minor clarifications, the design provides a clear roadmap for implementation.

**Implementation Estimate**:
- Phase 1: 4-6 hours (including tests)
- Phase 2: 2-3 hours (recipe + integration test)
- Phase 3: 1-2 hours (documentation)

Total: ~8-11 hours of implementation work.
