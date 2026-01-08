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

## Current Bug: Directory Mode Binary Auto-Discovery

### Symptom
CI logs show:
```
🔗 Symlinked 2 binaries: [bin/git bin/git-remote-https]
⚠️  Could not compute binary checksums: failed to resolve binary path bin/git-remote-https:
    lstat /Users/runner/.tsuku/tools/git-source-2.52.0/bin/git-remote-https: no such file or directory
```

### Analysis
The recipe specifies:
```toml
[[steps]]
action = "install_binaries"
install_mode = "directory"
binaries = ["bin/git"]
```

But the install manager is trying to symlink 2 binaries, including `bin/git-remote-https` which doesn't exist (it's in `libexec/git-core/`).

### Hypothesis
In directory mode, the install manager appears to be auto-discovering binaries from the installed directory tree instead of using only the binaries listed in the recipe. This is causing it to find `git-remote-https` in `libexec/git-core/` and try to symlink it as `bin/git-remote-https`.

### Next Steps
1. Find where the install manager populates the binaries list for directory mode
2. Ensure it uses only the binaries specified in the recipe, not auto-discovered ones
3. The directory tree copy should include everything (including `libexec/git-core/git-remote-https`)
4. But only the explicitly listed binaries should be symlinked to `$TSUKU_HOME/bin/`

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
