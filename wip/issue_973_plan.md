# Issue 973 Implementation Plan

## Approach

Create a unified `verify-binary.sh` script that combines both verification types, iterating over binaries once instead of twice.

## Structure of New Script

```
verify-binary.sh
├── Shared infrastructure (from both scripts)
│   ├── Argument parsing
│   ├── TSUKU_HOME resolution
│   ├── Tool directory discovery
│   ├── Binary iteration loop
│   └── Platform detection
├── Relocation checks (from verify-relocation.sh)
│   ├── FORBIDDEN_PATTERNS array
│   ├── check_relocation_linux() - readelf + strings
│   └── check_relocation_macos() - otool forbidden patterns
├── Dependency checks (from verify-no-system-deps.sh)
│   ├── ALLOWED_LINUX array
│   ├── ALLOWED_MACOS array
│   ├── check_deps_linux() - ldd
│   └── check_deps_macos() - otool -L
└── Unified reporting
    ├── Per-binary: show relocation + dependency issues together
    └── Summary: PASS/FAIL with issue counts
```

## Files to Modify

| File | Change |
|------|--------|
| `test/scripts/verify-binary.sh` | CREATE - combined script |
| `test/scripts/verify-relocation.sh` | DELETE |
| `test/scripts/verify-no-system-deps.sh` | DELETE |
| `.github/workflows/build-essentials.yml` | UPDATE - 6 call sites |

## Implementation Steps

1. Create `verify-binary.sh` combining:
   - Shared header and argument parsing
   - Both FORBIDDEN_PATTERNS and ALLOWED_* arrays
   - Single binary iteration loop calling both check types
   - Combined check function for each platform

2. Update `build-essentials.yml`:
   - Replace two separate calls with single `verify-binary.sh` call
   - Update loop variables from `verify_relocation`/`verify_no_system_deps` to single `verify_binary`

3. Delete old scripts

4. Test locally

## Key Differences to Preserve

### verify-relocation.sh specifics
- Uses `readelf -d` for RPATH/RUNPATH on Linux
- Uses `strings` to check for forbidden patterns in binary
- On macOS: checks for absolute paths that should use @rpath

### verify-no-system-deps.sh specifics
- Uses `ldd` on Linux for runtime resolution
- Uses `otool -L` with `tail -n +2` on macOS
- Checks for "not found" dependencies (ERROR)
- Checks against allowlist (WARNING for non-matching)

### Binary find pattern differences
- relocation: includes `*.a` files
- no-system-deps: excludes `*.a` files

Decision: Include `*.a` files (superset) - static libraries may have embedded paths too.

## Testing Strategy

1. Run new script on a known tool (make, pkg-config)
2. Compare output format to old scripts
3. Verify CI workflow syntax is valid
4. Run short tests to ensure no Go code was broken
