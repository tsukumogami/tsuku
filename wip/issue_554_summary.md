# Issue #554 Implementation Summary: Add curl recipe

## Overview
Successfully implemented curl recipe with proper RPATH configuration to link against tsuku-provided openssl and zlib libraries instead of system libraries.

## Problem
The initial implementation of curl was linking against system libraries (e.g., `/lib/x86_64-linux-gnu/libssl.so.3`) instead of tsuku-provided libraries (`~/.tsuku/libs/openssl-3.6.0/lib/libssl.so.3`). This would fail on systems without these dependencies installed.

## Root Cause Analysis
Through multi-agent consultation and investigation, we identified that libtool's relink phase was stripping the RPATH entries that were added via LDFLAGS. The attempted fix of setting `lt_cv_sys_lib_dlsearch_path_spec=""` did not work, so we implemented a post-build RPATH fix using the `set_rpath` action.

## Solution Implemented

### 1. Enhanced Variable Expansion System
**Files Modified:**
- `internal/actions/util.go` - Added `libs_dir` to GetStandardVars()
- All action files - Updated GetStandardVars() calls to include libsDir parameter
- Test files - Updated test calls accordingly

**Changes:**
- Added `libs_dir` variable to standard variable mappings
- Updated function signature: `GetStandardVars(version, installDir, workDir, libsDir string)`
- Enables recipes to reference `{libs_dir}` in RPATH configurations

### 2. Enhanced RPATH Validation
**File:** `internal/actions/set_rpath.go`

**Changes:**
- Updated `validateRpath()` to accept `libsDir` parameter
- Allowed colon-separated multiple RPATH entries
- Allowed absolute paths within `$TSUKU_HOME/libs/` directory
- Added variable expansion for RPATH values before validation
- Maintained security checks to prevent injection attacks

### 3. ExecutionContext Enhancement
**Files Modified:**
- `internal/actions/action.go` - Added LibsDir field
- `internal/executor/executor.go` - Added libsDir field and SetLibsDir() method
- `internal/builders/orchestrator.go` - Added LibsDir to OrchestratorConfig
- `cmd/tsuku/create.go` - Passed LibsDir from config
- `cmd/tsuku/install_deps.go` - **CRITICAL FIX**: Added `exec.SetLibsDir(cfg.LibsDir)` call

**Changes:**
- Propagated LibsDir through entire execution context chain
- Fixed missing SetLibsDir() call in install command (line 390 of install_deps.go)

### 4. Curl Recipe Implementation
**File:** `internal/recipe/recipes/c/curl.toml`

**Configuration:**
```toml
[metadata]
name = "curl"
description = "Command line tool for transferring data with URLs"
homepage = "https://curl.se/"
dependencies = ["openssl", "zlib"]
binaries = ["bin/curl"]

[[steps]]
action = "download_file"
url = "https://curl.se/download/curl-8.11.1.tar.gz"
checksum = "a889ac9dbba3644271bd9d1302b5c22a088893719b72be3487bc3d401e5c4e80"

[[steps]]
action = "extract"
archive = "curl-8.11.1.tar.gz"
format = "tar.gz"

[[steps]]
action = "setup_build_env"

[[steps]]
action = "configure_make"
source_dir = "curl-8.11.1"
configure_args = ["--with-openssl", "--with-zlib", "--disable-silent-rules", "--without-libpsl"]
executables = ["curl"]

[[steps]]
action = "set_rpath"
binaries = [".install/bin/curl", ".install/lib/libcurl.so.4.8.0"]
rpath = "$ORIGIN/../lib:{libs_dir}/openssl-3.6.0/lib:{libs_dir}/zlib-1.3.1/lib"

[[steps]]
action = "install_binaries"
install_mode = "directory"
binaries = ["bin/curl"]

[verify]
command = "curl --version"
pattern = "curl 8.11.1"
```

### 5. Test Enhancement
**File:** `test/scripts/verify-tool.sh`

**Changes:**
- Added `verify_curl()` function to test:
  - Binary execution (`curl --version`)
  - OpenSSL linkage verification
  - zlib linkage verification
  - HTTPS functionality test
- Added curl case to verification switch statement

## Verification Results

### 1. RPATH Verification
```bash
$ readelf -d ~/.tsuku/tools/curl-dev/bin/curl | grep RPATH
0x000000000000000f (RPATH)  Library rpath: [$ORIGIN/../lib:/home/dgazineu/.tsuku/libs/openssl-3.6.0/lib:/home/dgazineu/.tsuku/libs/zlib-1.3.1/lib]
```
✅ RPATH correctly set with DT_RPATH (not RUNPATH)
✅ Contains all three required paths

### 2. Library Linkage
```bash
$ ldd ~/.tsuku/tools/curl-dev/bin/curl | grep -E "libssl|libcrypto|libz"
libz.so.1 => /home/dgazineu/.tsuku/libs/zlib-1.3.1/lib/libz.so.1
libssl.so.3 => /home/dgazineu/.tsuku/libs/openssl-3.6.0/lib/libssl.so.3
libcrypto.so.3 => /home/dgazineu/.tsuku/libs/openssl-3.6.0/lib/libcrypto.so.3
```
✅ All dependencies resolved to tsuku-provided libraries
✅ No system library dependencies

### 3. Functionality Tests
```bash
$ curl --version
curl 8.11.1 (x86_64-pc-linux-gnu) libcurl/8.11.1 OpenSSL/3.6.0 zlib/1.3.1

$ curl -sI https://example.com | head -1
HTTP/1.1 200 OK
```
✅ Binary executes correctly
✅ Shows tsuku-provided library versions
✅ HTTPS functionality works

### 4. Test Suite Results
```bash
$ go test ./...
ok   github.com/tsukumogami/tsuku   11.030s
# ... (all 22 packages passed)

$ test/scripts/verify-tool.sh curl
=== PASS: Tool verification succeeded ===
```
✅ All Go tests pass
✅ Tool verification script passes

## Files Changed

### Core Implementation
1. `internal/actions/util.go` - Variable expansion system
2. `internal/actions/set_rpath.go` - RPATH validation and expansion
3. `internal/actions/action.go` - ExecutionContext LibsDir field
4. `internal/executor/executor.go` - Executor LibsDir field and setter
5. `internal/builders/orchestrator.go` - OrchestratorConfig LibsDir
6. `cmd/tsuku/create.go` - LibsDir propagation for create command
7. `cmd/tsuku/install_deps.go` - **LibsDir propagation for install command**

### Action Updates
8. `internal/actions/run_command.go`
9. `internal/actions/download.go`
10. `internal/actions/install_binaries.go`
11. `internal/actions/set_env.go`
12. `internal/actions/chmod.go`
13. `internal/actions/text_replace.go`
14. `internal/actions/extract.go`

### Test Updates
15. `internal/actions/util_test.go`
16. `internal/actions/download_test.go`
17. `test/scripts/verify-tool.sh` - Added curl verification

### Recipe
18. `internal/recipe/recipes/c/curl.toml` - New curl recipe

## Technical Decisions

### Why Post-Build RPATH Fix?
The initial attempt to prevent libtool from stripping RPATH (via `lt_cv_sys_lib_dlsearch_path_spec=""`) did not work. Using the `set_rpath` action post-build with patchelf provides a reliable, deterministic solution that:
- Works with any autotools-based build system
- Allows precise control over RPATH entries
- Uses DT_RPATH instead of DT_RUNPATH for better security

### Why Both $ORIGIN and Absolute Paths?
```
$ORIGIN/../lib:/home/dgazineu/.tsuku/libs/openssl-3.6.0/lib:/home/dgazineu/.tsuku/libs/zlib-1.3.1/lib
```
- `$ORIGIN/../lib`: For libcurl.so that curl depends on
- Absolute paths: For dependency libraries (openssl, zlib) in libs directory
- Colon-separated: Standard RPATH format for multiple search paths

### Security Considerations
- RPATH validation prevents injection attacks
- Absolute paths restricted to `$TSUKU_HOME/libs/` only
- Relative paths must use platform-specific prefixes ($ORIGIN, @executable_path, etc.)
- Binary name validation prevents shell injection in wrapper scripts

## Lessons Learned
1. **Critical**: Always set all required context fields in both create AND install commands
2. The missing `exec.SetLibsDir()` call in install_deps.go was the root cause of empty libs_dir
3. Variable expansion must happen before validation
4. Post-build RPATH modification is more reliable than trying to control autotools/libtool behavior

## Issue Resolution
✅ Curl recipe implemented and tested
✅ Links against tsuku-provided openssl and zlib
✅ HTTPS functionality verified
✅ All tests pass
✅ Tool verification script enhanced

Issue #554 is resolved and ready for PR.
