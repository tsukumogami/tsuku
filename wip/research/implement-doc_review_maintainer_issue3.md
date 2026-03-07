# Maintainability Review: Issue #3

## Issue

chore(scripts): update generation script to emit integer schema version

## Review Focus

Maintainability -- clarity, readability, duplication

## Diff

Single file changed: `scripts/generate-registry.py`, one line:

```python
# Before
SCHEMA_VERSION = "1.2.0"
# After
SCHEMA_VERSION = 1
```

## Findings

None.

## Overall Assessment

The change is a single constant value swap. The constant name (`SCHEMA_VERSION`) accurately describes its purpose. It's defined once at module scope (line 23) and used once in `generate_json()` (line 282). There's no duplication, no naming confusion, and no implicit contract to misunderstand.

The surrounding code -- `generate_json()` at line 282 -- passes `SCHEMA_VERSION` directly into the output dict, so the type flows naturally from definition to serialization. A future developer reading this will immediately understand what the value is and where it goes.

No blocking or advisory findings.
