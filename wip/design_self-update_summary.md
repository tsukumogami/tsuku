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
**Outcome:** Pending
**Summary:** Awaiting security researcher

## Current Status
**Phase:** 5 - Security (in progress)
**Last Updated:** 2026-03-31
