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
Even after fixing Bug 1, git clone still fails:
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

### Implementation
1. Added `make_args` parameter to configure_make action (internal/actions/configure_make.go)
   - Reads `make_args` from recipe step parameters
   - Expands variables in make_args (like configure_args)
   - Appends to commonMakeArgs before running make
2. Updated git-source.toml to pass `make_args = ["RUNTIME_PREFIX=1"]`

### References
- [Git RUNTIME_PREFIX patches](https://www.spinics.net/lists/git/msg90467.html)
- [RUNTIME_PREFIX relocatable Git](https://yhbt.net/lore/all/87vadrc185.fsf@evledraar.gmail.com/T/)
- [Git INSTALL documentation](https://github.com/git/git/blob/master/INSTALL)

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
