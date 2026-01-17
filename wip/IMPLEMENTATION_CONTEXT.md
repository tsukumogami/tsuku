# Implementation Context for Issue #973

## Goal

Consolidate `verify-relocation.sh` and `verify-no-system-deps.sh` into a single `verify-binary.sh` script that performs both RPATH/relocation checks and dependency resolution checks in one binary iteration pass.

## Shared Infrastructure

Both scripts share:
- Tool directory discovery: `find "$TSUKU_HOME/tools" -maxdepth 1 -type d -name "${TOOL_NAME}-*"`
- Binary iteration: `find "$TOOL_DIR" -type f \( -perm -u+x -o -name "*.so" -o -name "*.dylib" \) -print0`
- Platform detection: `uname -s` with Linux/Darwin branching
- File type detection: `file` command to check ELF vs Mach-O
- Reporting: PASS/FAIL with issue count pattern

## Key Differences

- `verify-relocation.sh`: Uses `readelf -d` (Linux) and checks `strings` output for forbidden patterns
- `verify-no-system-deps.sh`: Uses `ldd` (Linux) / `otool -L` (macOS) to check runtime resolution

## CI Usage

Scripts called 8 times in `build-essentials.yml` - consolidation reduces duplicate binary iteration.
