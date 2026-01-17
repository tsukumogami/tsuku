# UX Analysis: Tier 2 Dependency Validation

**Date:** 2026-01-17
**Role:** UX Agent
**Verdict:** CONCERNING - needs safeguards

## Summary

The proposal creates significant UX friction around recipe authoring, output verbosity, and migration. Recommend implementing as opt-in with helper tooling.

## Recipe Author Burden

### HIGH FRICTION for Static Binaries

Go binaries have NO `DT_NEEDED` entries (statically linked by default). Recipe authors won't see dependencies to declare.

Example: gh, terraform, and most modern tools are Go binaries. Authors will either:
- Declare system libs they assume (error-prone)
- Run `readelf -d` to check (requires tooling knowledge)
- Skip declaring deps and get confusing warnings

### Dynamic Linking Variance

Cross-platform recipes can't declare exact paths:
- Ruby on Fedora: `/lib64/libc.so.6`
- Ruby on Ubuntu: `/lib/x86_64-linux-gnu/libc.so.6`

## Verification Output

### Verbosity Cliff

Current `tsuku verify` for tools: 4 steps, manageable output.

With recursive validation:
```
Verifying ruby (version 3.3.0)...
  Tier 1: Header validation...
    ruby: OK
    ext/psych/psych.so: OK
  Tier 2: Dependency checking...
    ruby depends on: libc.so.6, libm.so.6, libz.so.1, libyaml.so.0
      libc.so.6: SYSTEM (skipped)
      libz.so.1: WARNING - Not declared in recipe
      libyaml.so.0: INSTALLED, verifying recursively...
        libyaml deps: libc.so.6, libm.so.6
        [continues...]
  Summary: 1 warning, 12 transitive deps validated
```

Users see warnings they can't act on (they didn't write the recipe).

## Warning vs Error

### Migration Path Undefined

- 1000+ existing recipes don't declare deps
- Day 1: `tsuku verify` shows warnings for all unresolved deps
- No guidance on who fixes it

### Warning Fatigue

If warnings appear frequently without failures, users ignore them. Then real missing deps get overlooked.

## Recommendations

### 1. Opt-in Recursion

```bash
tsuku verify ruby                # Only verify ruby (fast)
tsuku verify ruby --deep         # Verify ruby + direct deps
tsuku verify ruby --deep=2       # 2 levels of deps
tsuku verify ruby --deep=all     # Complete tree (slow)
```

### 2. Dry-Run Mode First

```bash
tsuku verify <lib> --report-deps  # Print analysis, don't warn
```

Let users understand their recipes before warnings become default.

### 3. Author Tooling

```bash
tsuku analyze-deps <binary>      # Extract DT_NEEDED, suggest declarations
```

Makes it easy for authors to populate declarations.

### 4. Static Binary Handling

Recipe declares:
```toml
[metadata]
static_binary = true  # Skip Tier 2 for this recipe
```

Avoids "no dependencies found" confusion for Go tools.

### 5. Soname-to-Recipe Display

Output must show: "libz.so.1 (provided by zlib recipe)" not just "libz.so.1: WARNING"

Without bidirectional mapping, warnings aren't actionable.

## Output Mockup (Recommended)

```
$ tsuku verify ruby --deep
Verifying ruby (version 3.3.0)...

  Binary dependencies:
    libc.so.6      → SYSTEM (glibc)
    libm.so.6      → SYSTEM (glibc)
    libz.so.1      → zlib (tsuku recipe) ✓
    libyaml.so.0   → libyaml (tsuku recipe) ✓

  Recipe declares: zlib, libyaml
  Binary needs:    libz.so.1, libyaml.so.0

  ✓ All declared dependencies satisfied

Recursive validation:
  → zlib: OK (no tsuku deps)
  → libyaml: OK (no tsuku deps)

ruby is working correctly
```

Clear, actionable, not overwhelming.
