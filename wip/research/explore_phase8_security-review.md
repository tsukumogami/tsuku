# Security Review: DESIGN-homebrew-deterministic-mode

## Context

Tsuku downloads and executes binaries. The Homebrew builder fetches bottle tarballs from GHCR, inspects them, and generates recipe TOML files that later drive binary installation. This design adds a deterministic-only mode that suppresses LLM fallback.

## 1. Are there attack vectors we haven't considered?

**The design covers the main vectors well.** The security considerations section addresses download verification (SHA256), execution isolation (no execution in deterministic path), supply chain (GHCR content-addressable digests), and user data exposure (no data sent to LLM in deterministic mode).

**Additional vectors to consider:**

### GHCR manifest/digest confusion attack
The design says SHA256 checksums are compared against manifest digests. If an attacker can serve a valid manifest with altered digests (e.g., via MITM on the anonymous token exchange or a compromised CDN edge), the verification would pass against the wrong content. This is mitigated by TLS, but worth noting that the anonymous token endpoint is a trust anchor. The design correctly classifies full GHCR compromise as out of scope.

### Category enumeration as oracle
In batch mode, the failure category is written to JSONL files. If an attacker can influence which formulas are processed (e.g., by creating Homebrew formulas designed to trigger specific failure categories), they could use the structured output as an oracle to probe tsuku's internal capabilities. **Risk: Low.** The categories are coarse-grained and don't reveal implementation details beyond what's already public in the schema.

### Tarball path traversal
The deterministic path inspects bottle tarball contents to find binaries in `bin/`. If the tarball contains path traversal entries (e.g., `../../etc/something`), and the code lists rather than extracts, there's no direct risk. However, if any code path extracts files based on tarball paths, traversal is possible. **This is an existing concern, not introduced by this design.** Worth verifying the `listBottleBinaries` implementation only reads metadata, not extracts.

## 2. Are the mitigations sufficient for the risks identified?

**Yes, for the risks this design introduces.** Key assessment:

| Risk | Mitigation | Sufficient? |
|------|-----------|-------------|
| Compromised bottle on GHCR | SHA256 digest verification | Yes -- content-addressable storage makes targeted attacks require GHCR compromise |
| GHCR token scope creep | Anonymous token, read-only | Yes -- no write capability |
| Failure category leaking internals | Enum values only, generic messages | Yes -- but see recommendation below |
| LLM fallback in batch mode | Explicit `DeterministicOnly` flag | Yes -- compile-time safe, can't accidentally fall through |

**Recommendation:** The design notes "Message could reveal internal paths; keep messages generic" as residual risk. The implementation should enforce this by constructing `DeterministicFailedError.Message` from a fixed set of templates rather than passing through raw error strings from internal functions. Internal errors should go in the `Err` field (for logging), not the `Message` field (for failure records).

## 3. Is there residual risk we should escalate?

**No escalation needed.** The residual risks are:

- GHCR compromise (out of scope, industry-wide)
- Internal path leakage in error messages (low, mitigated by code review)
- Tarball path traversal (pre-existing, not introduced here)

None of these warrant blocking the design.

## 4. Are any "not applicable" justifications actually applicable?

The design doesn't explicitly mark anything as "not applicable." However, reviewing the security considerations:

- **Execution Isolation** says "the deterministic path inspects tarball contents without executing anything." This is correct for the deterministic path itself. However, the generated recipe TOML will later be used to download and execute binaries. The design correctly notes that sandbox validation is "the batch pipeline's responsibility" in deterministic-only mode. This delegation is appropriate since this design's scope is the builder, not the pipeline.

- **No user data exposure** is accurate. Switching from LLM fallback to deterministic-only strictly reduces data exposure (no formula data sent to LLM providers).

**All justifications hold up under scrutiny.**

## Summary

The security posture of this design is sound. It reduces attack surface (no LLM calls in batch mode) without introducing new vectors. Two recommendations:

1. Construct `DeterministicFailedError.Message` from fixed templates, keeping raw internal errors in the `Err` field only.
2. Verify that `listBottleBinaries` only reads tarball metadata without extracting files (pre-existing concern, not a blocker for this design).
