# Design Summary: self-update

## Input Context (Phase 0)
**Source:** GitHub issue #2182 (Feature 4, auto-update roadmap)
**Problem:** No self-update path for tsuku binary; users must re-run installer
**Constraints:** Separate from managed tool system (D5), atomic replacement, checksum verification, integrates with Feature 2 background checks

## Decisions (Phase 2)
1. Direct asset name construction with checksums.txt parsing (high confidence)
2. Same-directory temp with two-rename backup (high confidence, critical tier)
3. Append self-check to RunUpdateCheck with well-known constant (high confidence)

## Cross-Validation (Phase 3)
No conflicts found. All assumptions compatible across decisions.

## Security Review (Phase 5)
**Outcome:** Document considerations
**Summary:** Checksum-only integrity is adequate for launch (matches gh, rustup). Cosign signing tracked as post-launch hardening.

## Architecture Review (Phase 6)
**Outcome:** One blocking finding (version normalization) resolved. Advisory items (HTTP client, parameter naming) noted for implementation.

## Security Review (Phase 6)
**Outcome:** Three pre-launch additions incorporated: downgrade protection, hard-fail on missing checksums, file lock for concurrency. Post-launch items tracked (cosign, .old cleanup).

## Current Status
**Phase:** 6 - Final Review (complete, awaiting approval)
**Last Updated:** 2026-04-01
