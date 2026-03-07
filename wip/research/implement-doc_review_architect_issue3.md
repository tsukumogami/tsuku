# Architecture Review: Issue #3

## Issue

chore(scripts): update generation script to emit integer schema version

## Source Document

docs/plans/PLAN-registry-versioning.md

## Diff Summary

Single file changed: `scripts/generate-registry.py`, line 23.
Change: `SCHEMA_VERSION = "1.2.0"` to `SCHEMA_VERSION = 1`.

## Architecture Assessment

### Design Alignment

The change aligns with the design doc's intent. The generation script (`scripts/generate-registry.py`) is the single source of truth for the `recipes.json` schema version value. The Go CLI's `Manifest.SchemaVersion` field was changed to `int` in Issue 1, and `parseManifest()` now validates against `[MinManifestSchemaVersion, MaxManifestSchemaVersion]` range. This script change makes the producer (Python script) emit the format the consumer (Go CLI) expects.

### Pattern Consistency

The `SCHEMA_VERSION` constant follows the existing pattern in `generate-registry.py`: top-level module constants (`MAX_DESCRIPTION_LENGTH`, `MAX_FILE_SIZE`, etc.) define configuration, and `generate_json()` at line 279-285 assembles the output dict from those constants. No new patterns introduced.

The `generate_json()` function passes `SCHEMA_VERSION` directly into the output dictionary at line 282. Python's `json.dump` serializes `int` as a JSON integer. This is the same serialization path used for other fields -- no special handling needed.

### Cross-Component Compatibility

- **Producer**: `scripts/generate-registry.py` now emits `"schema_version": 1` (JSON integer)
- **Consumer**: `internal/registry/manifest.go` has `SchemaVersion int` with `json:"schema_version"` tag, validated against range `[1, 1]`

The producer and consumer agree on type and value. No contract violation.

### Separation of Concerns

The script remains purely a build-time artifact generator. It doesn't import or depend on CLI internals. The contract between script and CLI is the JSON schema of `recipes.json`, which is now consistent across both sides.

## Findings

None.

## Overall Assessment

The change is structurally sound. It completes the producer side of the string-to-integer migration started in Issue 1. The generation script remains the single point of schema version definition for the registry manifest. No architectural concerns.
