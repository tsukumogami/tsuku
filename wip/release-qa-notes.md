# Release QA Notes: v0.4.0

Features and changes since v0.3.1, with usage examples for QA validation.

## 1. Platform Compatibility (glibc/musl)

tsuku now supports both glibc (Debian, Fedora, Arch) and musl (Alpine) Linux. This affects how library recipes install dependencies.

### 1.1 Libc Detection

tsuku detects the system's C library at runtime by inspecting `/bin/sh`'s ELF interpreter.

**Validation:**

```bash
# On glibc (Debian/Ubuntu/Fedora)
tsuku install zlib
tsuku verify zlib
# Expected: uses Homebrew bottles, full dependency resolution

# On musl (Alpine)
apk add zlib-dev
tsuku install zlib
tsuku verify zlib
# Expected: uses system packages via apk
```

### 1.2 Hybrid Library Recipes

Library recipes now have platform-conditional steps. On glibc Linux, they use Homebrew bottles for hermetic version control. On musl Linux, they use system packages (`apk add`).

**Validation:**

```bash
# On Alpine (musl)
docker run --rm -it alpine:3.19 sh
apk add curl
# Install tsuku, then:
apk add openssl-dev zlib-dev
tsuku install cmake
cmake --version
# Expected: cmake installs successfully using system OpenSSL/zlib

# On Debian (glibc)
docker run --rm -it debian:bookworm-slim bash
apt-get update && apt-get install -y curl
# Install tsuku, then:
tsuku install cmake
cmake --version
# Expected: cmake installs via Homebrew bottles, resolves openssl/zlib as tsuku deps
```

### 1.3 Libc Platform Constraint

Recipes can declare `supported_libc` to restrict which platforms they support.

**Validation:**

```bash
# A recipe with supported_libc = ["glibc"] should fail on Alpine
# Check: tsuku install <glibc-only-tool> on Alpine produces a clear error message
# with the unsupported_reason from the recipe.
```

### 1.4 Step-Level Dependencies

Dependencies declared on individual steps are only resolved when that step's `when` clause matches the platform. On musl, steps using `apk_install` typically have no tsuku-level dependencies because apk handles transitive deps.

**Validation:**

```bash
# On musl: install a tool that depends on libraries
# Dependencies should NOT be resolved by tsuku (apk handles them)
tsuku install cmake  # on Alpine with system packages pre-installed
# Should not download Homebrew bottles for openssl, zlib, etc.
```

---

## 2. Library Verification

### 2.1 Four-Tier Verification

`tsuku verify` for libraries now runs up to four tiers of checks:

| Tier | Check | Flag |
|------|-------|------|
| 1 | Header validation (ELF/Mach-O format, architecture) | Always |
| 2 | Dependency validation (DT_NEEDED entries resolved) | Always |
| 3 | dlopen load testing (actual library loading) | `--skip-dlopen` to skip |
| 4 | Integrity verification (SHA256 checksums) | `--integrity` to enable |

**Validation:**

```bash
# Install a library with dependencies
tsuku install openssl

# Tier 1-3 (default)
tsuku verify openssl
# Expected: Tier 1 header OK, Tier 2 deps resolved, Tier 3 dlopen loads

# Skip dlopen
tsuku verify openssl --skip-dlopen
# Expected: Tier 3 shows "skipped"

# Enable integrity check
tsuku verify openssl --integrity
# Expected: Tier 4 shows "N files verified"
```

### 2.2 Integrity Verification (Tier 4)

Compares SHA256 checksums of installed library files against values stored at install time. Detects post-installation tampering.

**Validation:**

```bash
tsuku install zlib
tsuku verify zlib --integrity
# Expected: "N files verified"

# Tamper with a file and re-verify
# (modify a .so file in $TSUKU_HOME/libs/zlib-*/lib/)
tsuku verify zlib --integrity
# Expected: "MODIFIED" status with mismatched checksums
```

### 2.3 Batch Processing and Timeouts

dlopen verification processes libraries in batches of 50 with a 5-second timeout per batch. Crashes trigger retry with halved batch size.

**Validation:**

```bash
# Install a library with many .so files
tsuku install openssl
tsuku verify openssl
# Expected: completes successfully; batch processing is transparent
```

### 2.4 Environment Sanitization

Before invoking the dlopen test helper, tsuku removes dangerous environment variables (`LD_PRELOAD`, `DYLD_INSERT_LIBRARIES`, etc.) and validates all library paths are within `$TSUKU_HOME/libs/`.

**Validation:**

```bash
# Set a dangerous env var and verify it doesn't affect verification
LD_PRELOAD=/tmp/fake.so tsuku verify openssl
# Expected: verification succeeds; LD_PRELOAD is sanitized before dlopen testing
```

---

## 3. Cache Management

### 3.1 TTL-Based Cache Expiration

Cached recipes expire after 24 hours by default. Expired recipes trigger a network refresh.

**Configuration:**

```bash
export TSUKU_RECIPE_CACHE_TTL=12h   # Custom TTL
```

**Validation:**

```bash
tsuku install fzf     # Caches recipe
tsuku cache info      # Shows freshness info
# Wait 24h or set short TTL
export TSUKU_RECIPE_CACHE_TTL=1m
# Wait 1 minute
tsuku install fzf     # Should attempt network refresh
```

### 3.2 LRU Cache Size Management

Cache has a configurable size limit (default 50MB). When usage exceeds 80%, least-recently-used entries are evicted to 60%.

**Configuration:**

```bash
export TSUKU_RECIPE_CACHE_SIZE_LIMIT=100MB
```

**Validation:**

```bash
tsuku cache info
# Shows "Limit: 50.00 MB (X% used)"
```

### 3.3 Stale-If-Error Fallback

When a recipe's TTL expires and the network is unavailable, tsuku serves the stale cache (up to 7 days old) with a warning.

**Configuration:**

```bash
export TSUKU_RECIPE_CACHE_MAX_STALE=7d         # Max staleness (default)
export TSUKU_RECIPE_CACHE_STALE_FALLBACK=false  # Disable fallback
```

**Validation:**

```bash
# Cache some recipes, then go offline
tsuku install fzf          # Works, caches recipe
# Disconnect network
export TSUKU_RECIPE_CACHE_TTL=1s  # Force immediate expiry
sleep 2
tsuku install fzf          # Should serve stale cache with warning to stderr
```

### 3.4 `tsuku cache cleanup`

Removes old cache entries by age or enforces size limits.

**Validation:**

```bash
# Preview cleanup (no deletion)
tsuku cache cleanup --dry-run
# Expected: lists entries that would be removed

# Remove entries not accessed in 7 days
tsuku cache cleanup --max-age 7d

# Enforce size limit via LRU
tsuku cache cleanup --force-limit

# Check result
tsuku cache info
```

### 3.5 `tsuku cache info` with Registry Statistics

Shows entry counts, sizes, freshness, and stale count for all caches.

**Validation:**

```bash
tsuku cache info
# Expected output:
#   Downloads: N entries, X MB
#   Versions: N entries, X KB
#   Registry: N entries, X KB
#     Oldest: <name> (cached N days ago)
#     Newest: <name> (cached N hours ago)
#     Stale: N entries
#     Limit: 50.00 MB (X% used)

# JSON output for scripting
tsuku cache info --json
# Expected: valid JSON with downloads, versions, registry objects
```

### 3.6 `tsuku update-registry` Enhancements

Selective and force refresh of cached recipes.

**Validation:**

```bash
# Refresh only expired recipes
tsuku update-registry

# Preview what would refresh
tsuku update-registry --dry-run

# Refresh a single recipe
tsuku update-registry --recipe fzf

# Force refresh all
tsuku update-registry --all
```

---

## 4. Developer Environment Isolation

### 4.1 `make build` (Dev Binary)

Produces a binary that defaults to `.tsuku-dev/` in the current directory instead of `~/.tsuku`.

**Validation:**

```bash
git clone https://github.com/tsukumogami/tsuku.git
cd tsuku
make build
./tsuku install fzf
ls .tsuku-dev/
# Expected: tools installed into .tsuku-dev/, not ~/.tsuku
```

### 4.2 `tsuku shellenv`

Prints PATH export commands for the current tsuku home directory.

**Validation:**

```bash
make build
./tsuku shellenv
# Expected output (with dev build):
#   export PATH="/path/to/.tsuku-dev/bin:/path/to/.tsuku-dev/tools/current:$PATH"

eval $(./tsuku shellenv)
./tsuku install fzf
which fzf
# Expected: points to .tsuku-dev/tools/current/fzf (or .tsuku-dev/bin/fzf)
```

### 4.3 `tsuku doctor`

Verifies the environment is correctly configured: home directory exists, bin/ and tools/current are in PATH, state file is accessible.

**Validation:**

```bash
# Without PATH configured
make build
./tsuku doctor
# Expected: FAIL on "tools/current in PATH" and "bin in PATH"

# With PATH configured
eval $(./tsuku shellenv)
./tsuku doctor
# Expected: all checks pass, "Everything looks good!"

# Exit code for scripting
./tsuku doctor && echo "OK" || echo "FAIL"
```

### 4.4 TSUKU_HOME Precedence

The environment variable takes priority over the build-time default.

**Validation:**

```bash
make build
# Dev binary defaults to .tsuku-dev
./tsuku shellenv
# Shows .tsuku-dev paths

# Override with env var
export TSUKU_HOME=/tmp/custom-tsuku
./tsuku shellenv
# Expected: shows /tmp/custom-tsuku paths instead
unset TSUKU_HOME
```

---

## 5. Recipe Builders

### 5.1 Go Builder

Generates recipes for Go CLI tools from Go module paths.

**Validation:**

```bash
tsuku create --from go go.uber.org/mock
# Expected: generates recipe with go_install action
cat recipes/m/mockgen.toml  # or wherever it lands
```

### 5.2 CPAN Builder

Generates recipes for Perl tools from CPAN distributions.

**Validation:**

```bash
tsuku create --from cpan perlcritic
# Expected: generates recipe with cpan_install action
cat recipes/p/perlcritic.toml
```

---

## 6. Bug Fixes

### 6.1 Bash Shell Configuration (#1066)

`install.sh` now configures both `.bashrc` and `.bash_profile` for bash users. Previously, only one was updated, causing PATH to not persist in all session types.

**Validation:**

```bash
# Fresh install on a system with bash
bash -c "$(curl -fsSL https://get.tsuku.dev)"
# Check both files
grep tsuku ~/.bashrc
grep tsuku ~/.bash_profile
# Expected: both contain `. "$HOME/.tsuku/env"` line
```

### 6.2 Dynamic OpenSSL Version in cmake RPATH (#1226)

cmake's recipe now resolves the OpenSSL version dynamically instead of hardcoding it, preventing RPATH breakage when OpenSSL updates.

**Validation:**

```bash
tsuku install cmake
cmake --version
# Expected: works regardless of which OpenSSL version is installed
```

### 6.3 install.sh TSUKU_HOME Export Fix

The env file created by install.sh no longer exports `TSUKU_HOME`, which was overriding build-time defaults. It now uses inline fallback syntax.

**Validation:**

```bash
cat ~/.tsuku/env
# Expected content:
#   export PATH="${TSUKU_HOME:-$HOME/.tsuku}/bin:${TSUKU_HOME:-$HOME/.tsuku}/tools/current:$PATH"
# NOT:
#   export TSUKU_HOME="..."
```

---

## Environment Variables Reference

| Variable | Default | Purpose |
|----------|---------|---------|
| `TSUKU_HOME` | `~/.tsuku` | Override tsuku home directory |
| `TSUKU_RECIPE_CACHE_TTL` | `24h` | Recipe cache freshness duration |
| `TSUKU_RECIPE_CACHE_SIZE_LIMIT` | `50MB` | Max cache size before LRU eviction |
| `TSUKU_RECIPE_CACHE_MAX_STALE` | `7d` | Max staleness for offline fallback |
| `TSUKU_RECIPE_CACHE_STALE_FALLBACK` | `true` | Enable/disable stale fallback |
| `TSUKU_INSTALL_DIR` | `$HOME/.tsuku` | Custom install location (install.sh) |
