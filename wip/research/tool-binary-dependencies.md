# Tool Binary Dependencies Analysis

**Date:** 2026-01-17
**Purpose:** Analyze whether tool binaries need DT_NEEDED validation

## Current State

### Tool Verification (4 Steps)

1. **Execution verification** - Run recipe's verify command
2. **PATH check** - Verify $TSUKU_HOME/current is in PATH
3. **Binary resolution** - Check `which` finds correct binary
4. **Integrity check** - Compare SHA256 checksums

**Missing:** No DT_NEEDED extraction or validation for tools.

### Library Verification (4 Tiers)

1. **Header validation** - Parse ELF/Mach-O headers ✅ Implemented
2. **Dependency checking** - Validate DT_NEEDED entries ❌ Not implemented
3. **dlopen testing** - Actually load library ❌ Not implemented
4. **Integrity check** - Verify checksums ✅ Implemented

## Reusable Infrastructure

The existing `verify.ValidateHeader()` function can parse tool binaries:

```go
// internal/verify/header.go
func ValidateHeader(path string) (*HeaderInfo, error) {
    // Returns HeaderInfo with Dependencies []string
    // Works for any ELF or Mach-O binary (tool or library)
}
```

This extracts DT_NEEDED/LC_LOAD_DYLIB entries from any binary.

## Current Assumption

**"If `tool --version` succeeds, dependencies are satisfied."**

Rationale:
- The dynamic linker validates all DT_NEEDED entries before execution
- If execution succeeds, the linker found everything
- Recipe authors test tools before publishing

## Gap Analysis

This assumption fails in these scenarios:

1. **Lazy loading** - Some deps only loaded on specific code paths
2. **Conditional features** - `--enable-feature` may load additional deps
3. **Symlink rot** - Deps exist but symlinks are broken
4. **Version mismatch** - libfoo.so.1 exists but binary needs libfoo.so.2

## Example: Real Tool Dependencies

A typical Go binary (statically linked):
```
readelf -d /path/to/gh
# Often shows NO DT_NEEDED entries (static linking)
```

A typical C binary:
```
readelf -d /path/to/curl
# DT_NEEDED: libssl.so.3, libcrypto.so.3, libz.so.1, libc.so.6
```

## Recommendation

**Tool verification should optionally validate DT_NEEDED entries.**

Two approaches:

### Option A: Unified Verification
Use same logic for tools and libraries:
- Extract DT_NEEDED from tool binary
- Classify each as system/tsuku-managed
- Validate tsuku-managed deps exist

### Option B: Execution-First + Warning
Keep execution as primary verification but add:
- Extract DT_NEEDED entries
- Compare against recipe's `runtime_dependencies`
- Warn if binary deps not declared in recipe

## Files Referenced

- `cmd/tsuku/verify.go` - Main verify command
- `internal/verify/header.go` - Binary parsing (reusable)
- `internal/verify/types.go` - Error categories
