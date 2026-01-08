# Issue #742: git-source CI Re-enablement Research

## Problem Statement

git-source was failing in CI with: `git: 'remote-https' is not a git command`

## Root Cause Analysis

### Git's curl/HTTPS Support Architecture

Based on [Linux From Scratch](https://www.linuxfromscratch.org/blfs/view/svn/general/git.html) and [git mailing list discussions](https://git.vger.kernel.narkive.com/K4pqTFcN/patch-makefile-honor-no-curl-when-setting-remote-curl-variables):

1. **git-remote-https is a symlink to git-remote-http** ([Atlassian Support](https://support.atlassian.com/bitbucket-data-center/kb/bitbucket-server-repository-import-fails-with-error-remote-https-is-not-a-git-command/))
   - Not a separate binary
   - Lives in `libexec/git-core/`, not `bin/`
   - Git expects to find it via `git --exec-path`

2. **NO_CURL Makefile variable controls building**
   - If configure can't find curl headers, it sets `NO_CURL=YesPlease`
   - This prevents `git-remote-http`, `git-remote-https`, `git-remote-ftp`, `git-remote-ftps` from being built
   - Setting `NO_CURL=` (empty) explicitly enables curl support

3. **Required environment variables for Git configure**
   - `CURL_CONFIG`: Path to curl-config binary
   - `CURLDIR`: Root directory of curl installation (headers in include/, libs in lib/)
   - `NO_CURL=`: Empty to force enable curl support
   - `CPPFLAGS`: Include directories for curl and its dependencies
   - `LDFLAGS`: Library directories with RPATH for curl and its dependencies
   - `PKG_CONFIG_PATH`: For pkg-config to find curl.pc

## What We Fixed

### 1. Library Dependency Installation (cmd/tsuku/install_lib.go)
Fixed `installLibrary()` to actually install dependencies for library recipes.

**Before:** Library recipes listed dependencies but they were never installed.

**After:** Added dependency installation loop before executor creation (lines 119-133).

### 2. Environment Setup (internal/actions/configure_make.go)
The `buildAutotoolsEnv()` function already sets all required environment variables:
- Lines 305-313: Sets `CURL_CONFIG` and `CURLDIR` when libcurl is detected
- Line 358: Sets `NO_CURL=` to explicitly enable curl support
- Lines 316-334: Builds `PKG_CONFIG_PATH`, `CPPFLAGS`, `LDFLAGS` from all dependencies

**CI logs confirm this works:**
```
Debug env: CURL_CONFIG=/Users/runner/.tsuku/libs/libcurl-8.18.0/bin/curl-config
Debug env: CURLDIR=/Users/runner/.tsuku/libs/libcurl-8.18.0
Debug env: NO_CURL=
Debug configure: checking for curl_global_init in -lcurl... yes
Debug configure: checking for curl-config... /Users/runner/.tsuku/libs/libcurl-8.18.0/bin/curl-config
```

### 3. RPATH for Transitive Dependencies (testdata/recipes/git-source.toml)
Added RPATHs for libcurl's transitive dependencies (libnghttp2, libssh2, brotli, etc.) in both:
- `bin/git`: Lines 35-36
- `libexec/git-core/git-remote-https`: Lines 39-41

**Caveat:** Currently using hardcoded versions (e.g., `libnghttp2-1.68.0`). Need to implement transitive dependency version resolution.

### 4. Homebrew Test Signature Updates
Fixed all 9 calls to `relocatePlaceholders()` in `homebrew_test.go` to use new 4-parameter signature.

### 5. macOS dylib RPATH Fixes (internal/actions/homebrew_relocate.go)
Added `fixLibraryDylibRpaths()` function (lines 545-680) to add RPATHs to .dylib files on macOS using install_name_tool. This ensures dylibs can find their transitive dependencies at runtime.

## Bug 1: Directory Mode Binary Auto-Discovery (FIXED)

### Symptom
CI logs showed:
```
🔗 Symlinked 2 binaries: [bin/git bin/git-remote-https]
⚠️  Could not compute binary checksums: failed to resolve binary path bin/git-remote-https:
    lstat /Users/runner/.tsuku/tools/git-source-2.52.0/bin/git-remote-https: no such file or directory
```

### Root Cause
The `ExtractBinaries()` function in `internal/recipe/types.go` was processing `binaries` parameters from ALL steps, including `set_rpath` steps. When it saw `.install/libexec/git-core/git-remote-https` in a set_rpath step, it extracted the basename and added `bin/` prefix, creating `bin/git-remote-https` in the symlink list.

### Fix
Added action type whitelist to `ExtractBinaries()` to only process installation actions (install_binaries, download_archive, github_archive, github_file, npm_install), skipping set_rpath and other modification actions. See `internal/recipe/types.go` lines 315-326.

### Verification
After fix, CI logs show: `🔗 Symlinked 1 binaries: [bin/git]` ✓

---

## Bug 2: Git Can't Find git-remote-https at Runtime (IN PROGRESS)

### Symptom
Even after fixing Bug 1, git clone still fails on all platforms:
```
git: 'remote-https' is not a git command. See 'git --help'.
fatal: remote helper 'https' aborted session
```

### Analysis
1. The directory tree IS copied correctly, including `libexec/git-core/git-remote-https`
2. Git was compiled with `--prefix=/var/folders/.../T/action-validator-.../.install`
3. This prefix is compiled into git as the location to find helper programs
4. When the tree is copied to `$TSUKU_HOME/tools/git-source-2.52.0/`, the compiled-in path no longer exists
5. Git runs via symlink from `$TSUKU_HOME/bin/git` → `$TSUKU_HOME/tools/git-source-2.52.0/bin/git`
6. Git looks for helpers at the compiled-in path (which doesn't exist), not relative to the binary location

### Solution: RUNTIME_PREFIX Make Variable

Git supports building with relocatable prefix via the `RUNTIME_PREFIX` make variable:
- When built with `RUNTIME_PREFIX=1`, Git computes its prefix at runtime based on the executable's location
- This allows Git installations to be moved to arbitrary filesystem locations
- Git strips known directories (like `bin/`) from the executable path to compute the prefix
- For example: if binary is at `/path/to/tools/git-2.52.0/bin/git`, prefix becomes `/path/to/tools/git-2.52.0`

### Implementation (Commit ca57971)
1. Added `make_args` parameter to configure_make action (internal/actions/configure_make.go)
   - Reads `make_args` from recipe step parameters
   - Expands variables in make_args (like configure_args)
   - Appends to commonMakeArgs before running make
2. Updated git-source.toml to pass `make_args = ["RUNTIME_PREFIX=1"]`

### Fix Attempt 1: Remove set_rpath Steps (Commit a5db21a)

**Hypothesis**: Wrapper scripts created by set_rpath were breaking RUNTIME_PREFIX.

**Action**: Removed both set_rpath steps from git-source.toml, leaving only:
- configure_make with RUNTIME_PREFIX=1
- install_binaries in directory mode

**Result**: Still fails with the same error on all platforms.

**Conclusion**: The problem is NOT wrapper scripts. Something else is preventing git-remote-https from being found.

### Current Investigation: Debug Logging (Commit 1e76b21)

Added debug logging to configure_make action to verify:
1. Whether RUNTIME_PREFIX=1 is actually being passed to make
2. Whether libexec/git-core/git-remote-https exists after installation

Logging added:
- Print all make arguments (including RUNTIME_PREFIX=1)
- List contents of libexec/git-core/ directory after make install

**CI Results**: Confirmed RUNTIME_PREFIX=1 is being passed and git-remote-https is being built, but Git still uses compile-time path.

### Root Cause Identified: configure vs RUNTIME_PREFIX Incompatibility

**Problem**: Git's `./configure` script sets absolute paths for gitexecdir, localedir, and perllibdir. RUNTIME_PREFIX requires these to be relative paths to work correctly.

**Evidence from CI**:
```
Debug: git --exec-path (where Git looks for helpers)
/tmp/action-validator-2440619504/.install/libexec/git-core
```

Git is looking at the compile-time path (which no longer exists) instead of computing the path at runtime relative to the binary location.

**Key Findings from Git Documentation**:
1. RUNTIME_PREFIX and autoconf are incompatible - configure sets absolute paths
2. The correct make variable is `RUNTIME_PREFIX=YesPlease`, not `RUNTIME_PREFIX=1`
3. RUNTIME_PREFIX requires relative paths for gitexecdir, localedir, and perllibdir
4. When using configure, it "munges the relative paths" which breaks RUNTIME_PREFIX

**Solution**: Build Git without configure, passing all required variables directly to make.

### Fix Attempt 2: Build Without Configure (IMPLEMENTED)

**Implementation**: Added `skip_configure` parameter to configure_make action.

**Changes**:
1. **internal/actions/configure_make.go**:
   - Added `skip_configure` boolean parameter
   - When true, skips `./configure` execution
   - Passes `prefix=<path>` directly to make instead
   - Updated Preflight to warn if make_args not specified with skip_configure

2. **testdata/recipes/git-source.toml**:
   - Set `skip_configure = true`
   - Updated make_args:
     - `RUNTIME_PREFIX=YesPlease` (correct value, not "1")
     - `gitexecdir=libexec/git-core` (relative path as required)
     - `NO_CURL=` (empty to enable curl support)
   - Removed configure_args (not used when skipping configure)

**How It Works**:
- Environment variables (CURL_CONFIG, CPPFLAGS, LDFLAGS) still set by buildAutotoolsEnv()
- CURLDIR passed as make variable (Git's Makefile requires this as a make var, not env var)
- Make receives all variables needed to build relocatable Git without configure
- RUNTIME_PREFIX with relative paths allows Git to compute prefix at runtime

This approach is used by Git for Windows and should work for relocatable Unix builds.

### Fix Attempt 3: Add CURLDIR as Make Variable (IMPLEMENTED)

**Problem**: Git's Makefile does not read CURLDIR from environment variables when building without configure. It must be passed as a make variable.

**Change**: Added `CURLDIR={libs_dir}/libcurl-{deps.libcurl.version}` to make_args in git-source.toml

**How Git's Makefile Works**:
- `CURLDIR=/foo/bar` tells Git that curl headers are in `/foo/bar/include` and libs in `/foo/bar/lib`
- This overrides CURL_CONFIG and is the recommended approach when building without configure
- Other dependencies (openssl, zlib, expat) are still found via CPPFLAGS/LDFLAGS environment vars

### Fix Attempt 4: Add Libcurl Transitive Dependencies (IMPLEMENTED)

**Problem**: Linker errors showing undefined references to nghttp3, ngtcp2, brotli, etc.:
```
/usr/bin/ld: /home/runner/.tsuku/libs/libcurl-8.18.0/lib/libcurl.so: undefined reference to `ngtcp2_conn_del'
/usr/bin/ld: /home/runner/.tsuku/libs/libcurl-8.18.0/lib/libcurl.so: undefined reference to `nghttp3_conn_block_stream'
```

**Root Cause**: git-source recipe only listed direct dependencies (libcurl, openssl, zlib, expat), but libcurl.so itself depends on many other libraries for HTTP/3 support. When setup_build_env runs, it only adds LDFLAGS for the direct dependencies, not the transitive ones.

**Solution**: Added libcurl's transitive dependencies to git-source recipe:
- brotli (compression)
- libnghttp2 (HTTP/2)
- libnghttp3 (HTTP/3)
- libngtcp2 (QUIC for HTTP/3)
- libssh2 (SSH protocol)
- zstd (compression)

Now setup_build_env will add `-L` paths for all these libraries to LDFLAGS, allowing the linker to resolve the symbols in libcurl.so.

**BUT THIS DIDN'T FIX THE PROBLEM** - The linker still failed with the same errors.

### Fix Attempt 5: Explicitly Pass CURL_LIBCURL to Make (IMPLEMENTED)

**Problem**: Even with transitive dependencies in LDFLAGS, the linker still fails:
```
/usr/bin/ld: libcurl.so: undefined reference to `ngtcp2_conn_del'
/usr/bin/ld: libcurl.so: undefined reference to `nghttp3_conn_block_stream'
```

**Root Cause Analysis**:
- When using `./configure`, Git runs `curl-config --libs` which returns ALL libraries needed
- When using `CURLDIR` without configure, Git's Makefile only sets: `CURL_LIBCURL = -L$(CURLDIR)/lib -lcurl`
- This only links against `-lcurl`, not the transitive dependencies
- The LDFLAGS environment variable contains `-L` paths, but doesn't tell Git's linker which libraries to link

**From Git Makefile Research**:
- Git's Makefile has `CURL_LIBCURL` variable that accumulates all curl-related linker flags
- When configure runs, it does: `CURL_LIBCURL += $(shell $(CURL_CONFIG) --libs)`
- This returns: `-lcurl -lnghttp3 -lngtcp2 -lnghttp2 -lssh2 -lssl -lcrypto -lz -lzstd -lbrotlidec`

**Solution**: Pass `CURL_LIBCURL` explicitly as a make variable with all transitive libraries:
```toml
make_args = [
    "RUNTIME_PREFIX=YesPlease",
    "gitexecdir=libexec/git-core",
    "NO_CURL=",
    "CURLDIR={libs_dir}/libcurl-{deps.libcurl.version}",
    "CURL_LIBCURL=-lcurl -lnghttp3 -lngtcp2 -lnghttp2 -lssh2 -lbrotlidec -lbrotlicommon -lzstd"
]
```

This tells Git's Makefile exactly which libraries to link when building programs that use curl.

### References
- [Git RUNTIME_PREFIX patches](https://www.spinics.net/lists/git/msg90467.html)
- [RUNTIME_PREFIX relocatable Git](https://yhbt.net/lore/all/87vadrc185.fsf@evledraar.gmail.com/T/)
- [Git INSTALL documentation](https://github.com/git/git/blob/master/INSTALL)
- [Git 2.18 RUNTIME_PREFIX discussion](https://public-inbox.org/git/nycvar.QRO.7.76.6.1807082346140.75@tvgsbejvaqbjf.bet/T/)

## Key Learnings

1. **Git is complex**: Multiple helper programs in non-standard locations, symlinked structure
2. **Transitive dependencies matter**: Not just direct dependencies need RPATH, but their dependencies too
3. **Environment variables are critical**: Git's configure needs multiple vars to find curl properly
4. **Directory mode semantics**: Copies full tree but should only symlink specified binaries
5. **macOS dylib dependencies**: Need explicit RPATH fixes for transitive dependencies to load

## References

- [Linux From Scratch - Git 2.52.0](https://www.linuxfromscratch.org/blfs/view/svn/general/git.html)
- [Atlassian - git-remote-https symlink](https://support.atlassian.com/bitbucket-data-center/kb/bitbucket-server-repository-import-fails-with-error-remote-https-is-not-a-git-command/)
- [Git mailing list - NO_CURL Makefile](https://git.vger.kernel.narkive.com/K4pqTFcN/patch-makefile-honor-no-curl-when-setting-remote-curl-variables)
- [JoeQuery - Install Git from Source on OSX](https://joequery.me/guides/install-git-source-osx/)
