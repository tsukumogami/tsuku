# Ruby Validation Example - Three-Category Model (Cross-Platform)

**Date:** 2026-01-17
**Purpose:** Concrete walkthrough of Tier 2 validation using Ruby as example across Linux (glibc), Linux (musl), and macOS

## The Three Categories

| Category | Example | What We Do |
|----------|---------|------------|
| **Pure system lib** | `libc.so.6` (Linux), `libSystem.B.dylib` (macOS) | Skip entirely (inherently OS-provided) |
| **Tsuku-managed** | `libyaml` (built from source) | Validate soname + recurse into its deps |
| **Externally-managed tsuku recipe** | `openssl-system` (via apt/brew) | Validate provides expected soname, but STOP recursion |

**Why PURE SYSTEM?** Libraries like libc, libm, libpthread are classified as PURE SYSTEM because they are **inherently OS-provided**. The OS kernel and standard runtime require these libraries to function. Pattern matching identifies them, with patterns varying by platform. The absence of tsuku recipes for these libraries is a **consequence** of being OS-provided, not the definition.

---

## Platform Overview

| Aspect | Linux (glibc) | Linux (musl) | macOS (Darwin) |
|--------|--------------|--------------|----------------|
| **Distribution examples** | Debian, Ubuntu, RHEL, Fedora | Alpine, Void Linux | macOS 12+, 13+, 14+ |
| **Binary format** | ELF | ELF | Mach-O |
| **Dependency extraction** | `readelf -d` (DT_NEEDED) | `readelf -d` (DT_NEEDED) | `otool -L` (LC_LOAD_DYLIB) |
| **Standard C library** | glibc | musl-libc | libSystem |
| **Linker** | `ld-linux-x86-64.so.2` | `ld-musl-x86_64.so.1` | `/usr/lib/dyld` |

---

## Ruby Recipe with Platform-Specific Steps

The Ruby recipe uses conditional steps based on the target platform:

```toml
# recipes/ruby.toml
[recipe]
name = "ruby"
type = "tool"
dependencies = ["libyaml", "openssl", "readline", "zlib"]

# Step 1: Download Ruby source (all platforms)
[[steps]]
action = "download"
url = "https://cache.ruby-lang.org/pub/ruby/3.3/ruby-{{version}}.tar.gz"

# Step 2: Install build dependencies (platform-specific)
[[steps]]
action = "apt_install"
packages = ["build-essential", "libffi-dev"]
when = { os = "linux", libc = "glibc" }

[[steps]]
action = "apk_add"
packages = ["build-base", "libffi-dev"]
when = { os = "linux", libc = "musl" }

[[steps]]
action = "brew_install"
packages = ["libffi"]
when = { os = "darwin" }

# Step 3: Build from source (all platforms, uses installed deps)
[[steps]]
action = "configure_make_install"
configure_flags = ["--enable-shared", "--with-openssl-dir={{openssl.prefix}}"]
```

**Key points:**
- The `when` clause controls which steps run on each platform
- Actions like `apt_install`, `apk_add`, `brew_install` imply the platform
- Dependencies are resolved to platform-appropriate library paths

---

## Step 1: Extract Binary Dependencies

Each platform uses different tools and reports dependencies differently:

### Linux (glibc) - e.g., Debian/Ubuntu

```bash
$ readelf -d $TSUKU_HOME/tools/ruby-3.3.0/bin/ruby | grep NEEDED
  NEEDED: libc.so.6
  NEEDED: libm.so.6
  NEEDED: libpthread.so.0
  NEEDED: libdl.so.2
  NEEDED: libyaml-0.so.2
  NEEDED: libssl.so.3
  NEEDED: libcrypto.so.3
  NEEDED: libreadline.so.8
  NEEDED: libz.so.1
```

### Linux (musl) - e.g., Alpine

```bash
$ readelf -d $TSUKU_HOME/tools/ruby-3.3.0/bin/ruby | grep NEEDED
  NEEDED: libc.musl-x86_64.so.1
  NEEDED: libyaml-0.so.2
  NEEDED: libssl.so.3
  NEEDED: libcrypto.so.3
  NEEDED: libreadline.so.8
  NEEDED: libz.so.1
```

**Note:** musl combines libc, libm, libpthread, libdl into a single `libc.musl-*.so.1` library.

### macOS (Darwin)

```bash
$ otool -L $TSUKU_HOME/tools/ruby-3.3.0/bin/ruby
/Users/user/.tsuku/tools/ruby-3.3.0/bin/ruby:
  /usr/lib/libSystem.B.dylib
  @rpath/libyaml-0.2.dylib
  @rpath/libssl.3.dylib
  @rpath/libcrypto.3.dylib
  @rpath/libreadline.8.dylib
  @rpath/libz.1.dylib
```

**Note:** macOS uses `@rpath` prefixes for libraries that should be resolved via the runtime search path, and absolute paths for system libraries.

---

## Step 2: Build Soname Index from state.json

The soname index is built the same way on all platforms, but the recorded sonames differ:

```json
// state.json structure (platform-aware)
{
  "libs": {
    "libyaml": {
      "0.2.5": {
        "sonames": {
          "linux-glibc": ["libyaml-0.so.2"],
          "linux-musl": ["libyaml-0.so.2"],
          "darwin": ["libyaml-0.2.dylib"]
        }
      }
    },
    "openssl": {
      "3.2.1": {
        "sonames": {
          "linux-glibc": ["libssl.so.3", "libcrypto.so.3"],
          "linux-musl": ["libssl.so.3", "libcrypto.so.3"],
          "darwin": ["libssl.3.dylib", "libcrypto.3.dylib"]
        }
      }
    },
    "readline": {
      "8.2": {
        "sonames": {
          "linux-glibc": ["libreadline.so.8"],
          "linux-musl": ["libreadline.so.8"],
          "darwin": ["libreadline.8.dylib"]
        }
      }
    },
    "zlib": {
      "1.3.1": {
        "sonames": {
          "linux-glibc": ["libz.so.1"],
          "linux-musl": ["libz.so.1"],
          "darwin": ["libz.1.dylib"]
        }
      }
    }
  }
}
```

**Resulting index (per platform):**

| Linux (glibc/musl) | macOS (Darwin) |
|-------------------|----------------|
| `libyaml-0.so.2` -> libyaml | `libyaml-0.2.dylib` -> libyaml |
| `libssl.so.3` -> openssl | `libssl.3.dylib` -> openssl |
| `libcrypto.so.3` -> openssl | `libcrypto.3.dylib` -> openssl |
| `libreadline.so.8` -> readline | `libreadline.8.dylib` -> readline |
| `libz.so.1` -> zlib | `libz.1.dylib` -> zlib |

---

## Step 3: Classify Dependencies (Platform-Specific Patterns)

For each binary dependency, we ask TWO questions in order:

1. **Is it in our soname index?** (Is tsuku managing this library?)
2. **If not, does it match a known OS-provided library pattern?**

### Pure System Patterns by Platform

| Platform | Pattern Examples | Rationale |
|----------|-----------------|-----------|
| **Linux (glibc)** | `libc.so.*`, `libm.so.*`, `libpthread.so.*`, `libdl.so.*`, `librt.so.*`, `ld-linux*.so.*` | Core glibc components provided by every glibc-based distribution |
| **Linux (musl)** | `libc.musl-*.so.*`, `ld-musl-*.so.*` | All standard C library functions consolidated in single musl library |
| **macOS (Darwin)** | `/usr/lib/libSystem.B.dylib`, `/usr/lib/libc++.1.dylib`, `/usr/lib/libobjc.A.dylib`, `/System/Library/Frameworks/*` | Apple system libraries and frameworks, always available |

### Classification Tables by Platform

#### Linux (glibc)

| DT_NEEDED | In Index? | System Pattern? | Category | Action |
|-----------|-----------|-----------------|----------|--------|
| `libc.so.6` | No | Yes (`libc.so.*`) | PURE SYSTEM | Skip |
| `libm.so.6` | No | Yes (`libm.so.*`) | PURE SYSTEM | Skip |
| `libpthread.so.0` | No | Yes (`libpthread.so.*`) | PURE SYSTEM | Skip |
| `libdl.so.2` | No | Yes (`libdl.so.*`) | PURE SYSTEM | Skip |
| `libyaml-0.so.2` | Yes -> libyaml | - | TSUKU-MANAGED | Validate + Recurse |
| `libssl.so.3` | Yes -> openssl | - | TSUKU-MANAGED | Validate + Recurse |
| `libcrypto.so.3` | Yes -> openssl | - | TSUKU-MANAGED | Validate + Recurse |
| `libreadline.so.8` | Yes -> readline | - | TSUKU-MANAGED | Validate + Recurse |
| `libz.so.1` | Yes -> zlib | - | TSUKU-MANAGED | Validate + Recurse |

#### Linux (musl)

| DT_NEEDED | In Index? | System Pattern? | Category | Action |
|-----------|-----------|-----------------|----------|--------|
| `libc.musl-x86_64.so.1` | No | Yes (`libc.musl-*.so.*`) | PURE SYSTEM | Skip |
| `libyaml-0.so.2` | Yes -> libyaml | - | TSUKU-MANAGED | Validate + Recurse |
| `libssl.so.3` | Yes -> openssl | - | TSUKU-MANAGED | Validate + Recurse |
| `libcrypto.so.3` | Yes -> openssl | - | TSUKU-MANAGED | Validate + Recurse |
| `libreadline.so.8` | Yes -> readline | - | TSUKU-MANAGED | Validate + Recurse |
| `libz.so.1` | Yes -> zlib | - | TSUKU-MANAGED | Validate + Recurse |

**Note:** Fewer system libraries appear because musl consolidates them.

#### macOS (Darwin)

| LC_LOAD_DYLIB | In Index? | System Pattern? | Category | Action |
|---------------|-----------|-----------------|----------|--------|
| `/usr/lib/libSystem.B.dylib` | No | Yes (`/usr/lib/libSystem*`) | PURE SYSTEM | Skip |
| `@rpath/libyaml-0.2.dylib` | Yes -> libyaml | - | TSUKU-MANAGED | Validate + Recurse |
| `@rpath/libssl.3.dylib` | Yes -> openssl | - | TSUKU-MANAGED | Validate + Recurse |
| `@rpath/libcrypto.3.dylib` | Yes -> openssl | - | TSUKU-MANAGED | Validate + Recurse |
| `@rpath/libreadline.8.dylib` | Yes -> readline | - | TSUKU-MANAGED | Validate + Recurse |
| `@rpath/libz.1.dylib` | Yes -> zlib | - | TSUKU-MANAGED | Validate + Recurse |

**Note:** `@rpath` prefixes are stripped when looking up in the soname index.

---

## Step 4: Validate Tsuku-Managed Dependencies

For each tsuku-managed dependency found in index:

1. **Check recipe declaration:** Is `libyaml` in Ruby's `dependencies`? YES
2. **Verify installation:** Is `libyaml` actually installed? YES
3. **Verify soname match:** Does installed libyaml provide the expected soname for this platform? YES

If any check fails -> Warning (dep not declared, or declared but not installed, or soname mismatch).

---

## Step 5: Recursive Validation (--deep)

For each validated tsuku-managed dependency, we check if it's externally managed:

```go
// Simplified logic
for _, dep := range tsukuManagedDeps {
    recipe := LoadRecipe(dep.RecipeName)

    if recipe.IsExternallyManagedFor(currentTarget) {
        // Category: EXTERNALLY-MANAGED TSUKU RECIPE
        // Validate it provides expected soname, but DON'T recurse
        validateSonameProvided(dep)
    } else {
        // Category: TSUKU-MANAGED
        // Validate AND recurse
        validateSonameProvided(dep)
        recursivelyValidate(dep)  // Continue down the tree
    }
}
```

### Externally-Managed Recipes by Platform

The same logical recipe can be externally managed on one platform but built from source on another:

```toml
# recipes/openssl.toml
[recipe]
name = "openssl"
type = "library"

# On Linux glibc systems: use system openssl via apt
[[steps]]
action = "apt_install"
packages = ["libssl-dev"]
when = { os = "linux", libc = "glibc" }

# On Alpine (musl): use system openssl via apk
[[steps]]
action = "apk_add"
packages = ["openssl-dev"]
when = { os = "linux", libc = "musl" }

# On macOS: use Homebrew
[[steps]]
action = "brew_install"
packages = ["openssl@3"]
when = { os = "darwin" }
```

All three platform variants return `IsExternallyManaged() = true`, so validation stops recursion for openssl on all platforms.

### Recursive Validation of libyaml (Built from Source)

libyaml is built from source on all platforms:

```toml
# recipes/libyaml.toml
[recipe]
name = "libyaml"
type = "library"

[[steps]]
action = "download"
url = "https://github.com/yaml/libyaml/releases/..."

[[steps]]
action = "configure_make_install"
```

**Recursive validation extracts libyaml's dependencies:**

#### Linux (glibc)
```bash
$ readelf -d $TSUKU_HOME/libs/libyaml-0.2.5/lib/libyaml-0.so.2 | grep NEEDED
  NEEDED: libc.so.6
```
Classification: `libc.so.6` -> PURE SYSTEM -> Skip. Validation complete.

#### Linux (musl)
```bash
$ readelf -d $TSUKU_HOME/libs/libyaml-0.2.5/lib/libyaml-0.so.2 | grep NEEDED
  NEEDED: libc.musl-x86_64.so.1
```
Classification: `libc.musl-x86_64.so.1` -> PURE SYSTEM -> Skip. Validation complete.

#### macOS (Darwin)
```bash
$ otool -L $TSUKU_HOME/libs/libyaml-0.2.5/lib/libyaml-0.2.dylib
  /usr/lib/libSystem.B.dylib
```
Classification: `/usr/lib/libSystem.B.dylib` -> PURE SYSTEM -> Skip. Validation complete.

---

## Complete Flow Diagram

```
                    DT_NEEDED / LC_LOAD_DYLIB entry
                              │
                              ▼
                    ┌─────────────────┐
                    │ In soname index? │
                    └────────┬────────┘
                        │         │
                       YES        NO
                        │         │
                        ▼         ▼
               ┌────────────┐  ┌──────────────────────┐
               │TSUKU-MANAGED│  │System lib pattern?   │
               └──────┬─────┘  │(platform-specific)   │
                      │        └────────┬─────────────┘
                      │              │         │
                      │             YES        NO
                      │              │         │
                      │              ▼         ▼
                      │        ┌──────────┐  ┌─────────┐
                      │        │PURE SYSTEM│  │ WARNING │
                      │        │  (skip)   │  │(unknown)│
                      │        └──────────┘  └─────────┘
                      ▼
            ┌─────────────────────┐
            │Validate soname match│
            └──────────┬──────────┘
                       │
                       ▼
            ┌─────────────────────────┐
            │Is recipe externally     │
            │managed for this target? │
            └────────────┬────────────┘
                    │          │
                   YES         NO
                    │          │
                    ▼          ▼
            ┌──────────┐  ┌──────────┐
            │  STOP    │  │ RECURSE  │
            │(apt/brew │  │(validate │
            │ owns it) │  │ its deps)│
            └──────────┘  └──────────┘
```

---

## Example Output by Platform

### Linux (glibc) - Debian/Ubuntu

```
$ tsuku verify ruby --deep
Verifying ruby (version 3.3.0) on linux-glibc-x86_64...

  Tier 1: Header validation
    bin/ruby: OK (ELF x86_64)

  Tier 2: Dependency validation
    Binary deps: libc.so.6, libm.so.6, libpthread.so.0, libdl.so.2,
                 libyaml-0.so.2, libssl.so.3, libcrypto.so.3,
                 libreadline.so.8, libz.so.1

    libc.so.6        -> SYSTEM (glibc)
    libm.so.6        -> SYSTEM (glibc)
    libpthread.so.0  -> SYSTEM (glibc)
    libdl.so.2       -> SYSTEM (glibc)
    libyaml-0.so.2   -> libyaml [OK] (declared, installed, soname matches)
    libssl.so.3      -> openssl [OK] (declared, installed, soname matches)
    libcrypto.so.3   -> openssl [OK] (declared, installed, soname matches)
    libreadline.so.8 -> readline [OK] (declared, installed, soname matches)
    libz.so.1        -> zlib [OK] (declared, installed, soname matches)

  Recursive validation (--deep):
    -> libyaml: OK (tsuku-managed, only system deps)
    -> openssl: OK (externally-managed via apt, recursion stopped)
    -> readline: OK (externally-managed via apt, recursion stopped)
    -> zlib: OK (tsuku-managed, only system deps)

ruby verified successfully
```

### Linux (musl) - Alpine

```
$ tsuku verify ruby --deep
Verifying ruby (version 3.3.0) on linux-musl-x86_64...

  Tier 1: Header validation
    bin/ruby: OK (ELF x86_64)

  Tier 2: Dependency validation
    Binary deps: libc.musl-x86_64.so.1, libyaml-0.so.2, libssl.so.3,
                 libcrypto.so.3, libreadline.so.8, libz.so.1

    libc.musl-x86_64.so.1 -> SYSTEM (musl)
    libyaml-0.so.2        -> libyaml [OK] (declared, installed, soname matches)
    libssl.so.3           -> openssl [OK] (declared, installed, soname matches)
    libcrypto.so.3        -> openssl [OK] (declared, installed, soname matches)
    libreadline.so.8      -> readline [OK] (declared, installed, soname matches)
    libz.so.1             -> zlib [OK] (declared, installed, soname matches)

  Recursive validation (--deep):
    -> libyaml: OK (tsuku-managed, only system deps)
    -> openssl: OK (externally-managed via apk, recursion stopped)
    -> readline: OK (externally-managed via apk, recursion stopped)
    -> zlib: OK (tsuku-managed, only system deps)

ruby verified successfully
```

### macOS (Darwin)

```
$ tsuku verify ruby --deep
Verifying ruby (version 3.3.0) on darwin-arm64...

  Tier 1: Header validation
    bin/ruby: OK (Mach-O arm64)

  Tier 2: Dependency validation
    Binary deps: /usr/lib/libSystem.B.dylib, @rpath/libyaml-0.2.dylib,
                 @rpath/libssl.3.dylib, @rpath/libcrypto.3.dylib,
                 @rpath/libreadline.8.dylib, @rpath/libz.1.dylib

    /usr/lib/libSystem.B.dylib -> SYSTEM (Darwin)
    @rpath/libyaml-0.2.dylib   -> libyaml [OK] (declared, installed, soname matches)
    @rpath/libssl.3.dylib      -> openssl [OK] (declared, installed, soname matches)
    @rpath/libcrypto.3.dylib   -> openssl [OK] (declared, installed, soname matches)
    @rpath/libreadline.8.dylib -> readline [OK] (declared, installed, soname matches)
    @rpath/libz.1.dylib        -> zlib [OK] (declared, installed, soname matches)

  Recursive validation (--deep):
    -> libyaml: OK (tsuku-managed, only system deps)
    -> openssl: OK (externally-managed via brew, recursion stopped)
    -> readline: OK (externally-managed via brew, recursion stopped)
    -> zlib: OK (tsuku-managed, only system deps)

ruby verified successfully
```

---

## Summary

### Platform Consistency

The validation logic works **consistently across all platforms**:

1. **First:** Check if soname is in our index -> If yes, it's TSUKU-MANAGED
2. **Second:** If not in index, check system patterns -> If matches, it's PURE SYSTEM
3. **Third:** If neither -> Warning (unknown dependency)

### What Differs by Platform

| Aspect | What Changes |
|--------|-------------|
| **Extraction tool** | `readelf` (Linux) vs `otool` (macOS) |
| **Dependency format** | DT_NEEDED sonames vs LC_LOAD_DYLIB paths |
| **System patterns** | glibc libs vs musl libs vs Darwin libs |
| **Soname conventions** | `libfoo.so.N` vs `libfoo.N.dylib` |
| **Path prefixes** | None (Linux) vs `@rpath/`, `/usr/lib/` (macOS) |
| **External managers** | apt (glibc), apk (musl), brew (macOS) |

### Why This Works

The three-category model abstracts platform differences:

- **PURE SYSTEM** patterns are platform-specific but serve the same purpose: identify OS-provided libraries that need no validation
- **TSUKU-MANAGED** libraries record platform-appropriate sonames at install time, so index lookups work consistently
- **EXTERNALLY-MANAGED** recipes specify platform via `when` clauses, making `IsExternallyManaged()` correct per-platform

The core algorithm remains identical; only the data (patterns, sonames, extraction commands) differs.
