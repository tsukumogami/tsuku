# Ruby Validation Example - Three-Category Model (Cross-Platform)

**Date:** 2026-01-17
**Purpose:** Concrete walkthrough of Tier 2 validation using Ruby as example across Linux and macOS, demonstrating all THREE dependency categories and how recursion behavior differs for each.

## The Three Categories

| Category | Definition | Recursion Behavior | Example |
|----------|------------|-------------------|---------|
| **PURE SYSTEM** | Inherently OS-provided libraries (no tsuku recipe exists) | Skip entirely | libc.so.6, libSystem.B.dylib |
| **TSUKU-MANAGED** | Tsuku recipe builds from source | Validate soname + recurse into its deps | libyaml (source build) |
| **EXTERNALLY-MANAGED** | Tsuku recipe delegates to package manager (apt/brew/apk) | Validate soname provided, but STOP recursion | openssl (via apt on Linux) |

**Key insight:** The same logical library may have different management styles on different platforms. For example, openssl might be EXTERNALLY-MANAGED on Linux (via apt) but TSUKU-MANAGED on macOS (built from source because Apple deprecated OpenSSL).

---

## Ruby Recipe (Simple, Correct Form)

The Ruby recipe declares its dependencies but does NOT install them inline. Dependencies are SEPARATE tsuku recipes.

```toml
# recipes/ruby.toml
[recipe]
name = "ruby"
type = "tool"
description = "Dynamic programming language"
dependencies = ["libyaml", "openssl", "readline", "zlib"]

[[steps]]
action = "download"
url = "https://cache.ruby-lang.org/pub/ruby/3.3/ruby-{{version}}.tar.gz"

[[steps]]
action = "extract"
format = "tar.gz"

[[steps]]
action = "configure_make_install"
configure_flags = [
  "--enable-shared",
  "--with-openssl-dir={{openssl.prefix}}",
  "--with-readline-dir={{readline.prefix}}",
  "--with-libyaml-dir={{libyaml.prefix}}",
  "--with-zlib-dir={{zlib.prefix}}"
]
```

**Note:** Build tools (gcc, make, build-essential) are environment prerequisites, not recipe dependencies. Users must have a working build environment.

---

## Dependency Recipes with Different Management Styles

Each dependency has its own recipe with platform-specific management:

### libyaml - TSUKU-MANAGED (all platforms)

Built from source on all platforms. Tsuku controls the entire build.

```toml
# recipes/libyaml.toml
[recipe]
name = "libyaml"
type = "library"
description = "YAML parser and emitter library"

[[steps]]
action = "download"
url = "https://github.com/yaml/libyaml/releases/download/{{version}}/yaml-{{version}}.tar.gz"

[[steps]]
action = "extract"
format = "tar.gz"

[[steps]]
action = "configure_make_install"
```

**Category:** TSUKU-MANAGED on all platforms (source build)
**Recursion:** Validate + recurse into libyaml's dependencies

---

### openssl - EXTERNALLY-MANAGED (Linux) / TSUKU-MANAGED (macOS)

Different management styles per platform:

```toml
# recipes/openssl.toml
[recipe]
name = "openssl"
type = "library"
description = "TLS/SSL and crypto library"

# Linux: Use system OpenSSL via apt/apk (common, well-tested)
[[steps]]
action = "apt_install"
packages = ["libssl-dev"]
when = { os = "linux", distro_family = "debian" }

[[steps]]
action = "apk_add"
packages = ["openssl-dev"]
when = { os = "linux", distro_family = "alpine" }

# macOS: Build from source (Apple deprecated system OpenSSL)
[[steps]]
action = "download"
url = "https://www.openssl.org/source/openssl-{{version}}.tar.gz"
when = { os = "darwin" }

[[steps]]
action = "extract"
format = "tar.gz"
when = { os = "darwin" }

[[steps]]
action = "shell"
script = "./Configure darwin64-arm64-cc --prefix={{prefix}} && make && make install"
when = { os = "darwin" }
```

**Category:**
- Linux: EXTERNALLY-MANAGED (apt owns the library and its transitive deps)
- macOS: TSUKU-MANAGED (tsuku builds from source)

**Recursion:**
- Linux: Validate soname provided, STOP (apt owns the dependency tree)
- macOS: Validate soname + recurse into openssl's dependencies

---

### readline - EXTERNALLY-MANAGED (Linux) / TSUKU-MANAGED (macOS)

Similar pattern to openssl:

```toml
# recipes/readline.toml
[recipe]
name = "readline"
type = "library"
description = "GNU readline library"

# Linux: Use system readline
[[steps]]
action = "apt_install"
packages = ["libreadline-dev"]
when = { os = "linux", distro_family = "debian" }

[[steps]]
action = "apk_add"
packages = ["readline-dev"]
when = { os = "linux", distro_family = "alpine" }

# macOS: Build from source (system readline is BSD libedit)
[[steps]]
action = "download"
url = "https://ftp.gnu.org/gnu/readline/readline-{{version}}.tar.gz"
when = { os = "darwin" }

[[steps]]
action = "extract"
format = "tar.gz"
when = { os = "darwin" }

[[steps]]
action = "configure_make_install"
when = { os = "darwin" }
```

**Category:**
- Linux: EXTERNALLY-MANAGED
- macOS: TSUKU-MANAGED

---

### zlib - EXTERNALLY-MANAGED (all platforms)

Very common system library, use system version everywhere:

```toml
# recipes/zlib.toml
[recipe]
name = "zlib"
type = "library"
description = "Compression library"

[[steps]]
action = "apt_install"
packages = ["zlib1g-dev"]
when = { os = "linux", distro_family = "debian" }

[[steps]]
action = "apk_add"
packages = ["zlib-dev"]
when = { os = "linux", distro_family = "alpine" }

[[steps]]
action = "brew_install"
packages = ["zlib"]
when = { os = "darwin" }
```

**Category:** EXTERNALLY-MANAGED on all platforms
**Recursion:** Validate soname provided, STOP on all platforms

---

## Step 0: PT_INTERP Validation (ABI Compatibility Check)

Before extracting and classifying dependencies, we validate the dynamic linker (interpreter) exists. This catches fundamental ABI mismatches that would prevent the binary from running at all.

### What is PT_INTERP?

The PT_INTERP segment in an ELF binary specifies the dynamic linker path. This is the program the kernel invokes to load the binary and resolve its shared libraries. If this interpreter doesn't exist, the binary cannot start.

### Extracting PT_INTERP

```bash
# Linux: readelf shows the interpreter
$ readelf -l $TSUKU_HOME/tools/ruby-3.3.0/bin/ruby | grep interpreter
      [Requesting program interpreter: /lib64/ld-linux-x86-64.so.2]

# Or using file command
$ file $TSUKU_HOME/tools/ruby-3.3.0/bin/ruby
ruby: ELF 64-bit LSB pie executable, x86-64, version 1 (SYSV),
dynamically linked, interpreter /lib64/ld-linux-x86-64.so.2, ...
```

### Common Interpreters by ABI

| ABI | Interpreter Path | Systems |
|-----|------------------|---------|
| glibc x86_64 | `/lib64/ld-linux-x86-64.so.2` | Debian, Ubuntu, Fedora, RHEL |
| glibc aarch64 | `/lib/ld-linux-aarch64.so.1` | Debian ARM, Ubuntu ARM |
| musl x86_64 | `/lib/ld-musl-x86_64.so.1` | Alpine |
| musl aarch64 | `/lib/ld-musl-aarch64.so.1` | Alpine ARM |

### Validation Logic

```go
// Extract interpreter from ELF PT_INTERP segment
interp := getInterpreter(binary)  // e.g., "/lib64/ld-linux-x86-64.so.2"

if interp == "" {
    // No interpreter = statically linked
    return Info("No dynamic dependencies (statically linked)")
}

if !fileExists(interp) {
    // Binary expects a different ABI than this system provides
    return Warning("Binary requires %s which is not present (ABI mismatch?)", interp)
}
// Interpreter exists, proceed with dependency validation
```

### ABI Mismatch Example: glibc Binary on Alpine (musl)

A binary built on Ubuntu (glibc) will NOT run on Alpine (musl):

```
$ tsuku verify ruby
Verifying ruby (version 3.3.0) on linux-musl-x86_64...

  Tier 1: Header validation
    bin/ruby: OK (ELF x86_64)

  ABI Compatibility Check:
    Interpreter: /lib64/ld-linux-x86-64.so.2
    WARNING: Interpreter not found - ABI mismatch detected

    This binary was built for glibc but you appear to be running musl.
    The binary will not run on this system.

    Possible solutions:
    - Use a musl-compatible binary (if available)
    - Run in a glibc-based container
    - Build from source on this system

ruby verification FAILED
```

### Why This Check Comes First

The PT_INTERP check happens before dependency classification because:

1. **Fundamental requirement**: If the interpreter doesn't exist, nothing else matters
2. **Clear error message**: Users get a specific, actionable error instead of confusing "file not found" from the kernel
3. **Fast check**: Single stat() call before more expensive operations
4. **Catches common mistake**: Downloading pre-built binaries for wrong distro type

### macOS Note

macOS uses the Mach-O format instead of ELF, so PT_INTERP doesn't apply. The dynamic linker (`/usr/lib/dyld`) is always present on macOS systems. ABI compatibility on macOS is handled differently through:
- Minimum OS version requirements (LC_VERSION_MIN_MACOSX)
- Architecture checks (x86_64 vs arm64)

---

## Step 1: Extract Binary Dependencies

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

---

## Step 2: Build Soname Index from state.json

The soname index maps library sonames to recipe names:

```json
{
  "libs": {
    "libyaml": {
      "0.2.5": {
        "sonames": {
          "linux-glibc": ["libyaml-0.so.2"],
          "darwin": ["libyaml-0.2.dylib"]
        },
        "management": {
          "linux-glibc": "source",
          "darwin": "source"
        }
      }
    },
    "openssl": {
      "3.2.1": {
        "sonames": {
          "linux-glibc": ["libssl.so.3", "libcrypto.so.3"],
          "darwin": ["libssl.3.dylib", "libcrypto.3.dylib"]
        },
        "management": {
          "linux-glibc": "apt",
          "darwin": "source"
        }
      }
    },
    "readline": {
      "8.2": {
        "sonames": {
          "linux-glibc": ["libreadline.so.8"],
          "darwin": ["libreadline.8.dylib"]
        },
        "management": {
          "linux-glibc": "apt",
          "darwin": "source"
        }
      }
    },
    "zlib": {
      "1.3.1": {
        "sonames": {
          "linux-glibc": ["libz.so.1"],
          "darwin": ["libz.1.dylib"]
        },
        "management": {
          "linux-glibc": "apt",
          "darwin": "brew"
        }
      }
    }
  }
}
```

---

## Step 3: Classify Dependencies

For each binary dependency, we ask:
1. **Is it in our soname index?** -> Which recipe provides it?
2. **If yes, is that recipe externally-managed for this platform?**
3. **If not in index, does it match a pure system pattern?**

### Classification Table - Linux (glibc)

| DT_NEEDED | Category | Reason | Action |
|-----------|----------|--------|--------|
| `libc.so.6` | **PURE SYSTEM** | Inherently OS-provided, no tsuku recipe | Skip |
| `libm.so.6` | **PURE SYSTEM** | Inherently OS-provided, no tsuku recipe | Skip |
| `libpthread.so.0` | **PURE SYSTEM** | Inherently OS-provided, no tsuku recipe | Skip |
| `libdl.so.2` | **PURE SYSTEM** | Inherently OS-provided, no tsuku recipe | Skip |
| `libyaml-0.so.2` | **TSUKU-MANAGED** | libyaml recipe builds from source | Validate + Recurse |
| `libssl.so.3` | **EXTERNALLY-MANAGED** | openssl recipe uses `apt_install` | Validate, NO recurse |
| `libcrypto.so.3` | **EXTERNALLY-MANAGED** | openssl recipe uses `apt_install` | Validate, NO recurse |
| `libreadline.so.8` | **EXTERNALLY-MANAGED** | readline recipe uses `apt_install` | Validate, NO recurse |
| `libz.so.1` | **EXTERNALLY-MANAGED** | zlib recipe uses `apt_install` | Validate, NO recurse |

**Summary for Linux:**
- 4 PURE SYSTEM libs (skipped entirely)
- 1 TSUKU-MANAGED lib (validate + recurse)
- 4 EXTERNALLY-MANAGED libs (validate, stop recursion)

### Classification Table - macOS (Darwin)

| LC_LOAD_DYLIB | Category | Reason | Action |
|---------------|----------|--------|--------|
| `/usr/lib/libSystem.B.dylib` | **PURE SYSTEM** | Inherently OS-provided, no tsuku recipe | Skip |
| `@rpath/libyaml-0.2.dylib` | **TSUKU-MANAGED** | libyaml recipe builds from source | Validate + Recurse |
| `@rpath/libssl.3.dylib` | **TSUKU-MANAGED** | openssl recipe builds from source on macOS | Validate + Recurse |
| `@rpath/libcrypto.3.dylib` | **TSUKU-MANAGED** | openssl recipe builds from source on macOS | Validate + Recurse |
| `@rpath/libreadline.8.dylib` | **TSUKU-MANAGED** | readline recipe builds from source on macOS | Validate + Recurse |
| `@rpath/libz.1.dylib` | **EXTERNALLY-MANAGED** | zlib recipe uses `brew_install` | Validate, NO recurse |

**Summary for macOS:**
- 1 PURE SYSTEM lib (skipped entirely)
- 4 TSUKU-MANAGED libs (validate + recurse) - more than Linux!
- 1 EXTERNALLY-MANAGED lib (validate, stop recursion)

**Key difference:** openssl and readline are EXTERNALLY-MANAGED on Linux but TSUKU-MANAGED on macOS. This reflects real-world needs (Apple deprecated OpenSSL, macOS "readline" is actually libedit).

---

## Step 4: Recursive Validation (--deep)

Recursion behavior differs by category:

```go
for _, dep := range binaryDeps {
    category := classifyDep(dep)

    switch category {
    case PURE_SYSTEM:
        // Skip entirely - no validation, no recursion
        continue

    case EXTERNALLY_MANAGED:
        // Validate soname is provided, but DON'T recurse
        // The package manager (apt/brew/apk) owns the dependency tree
        validateSonameProvided(dep)
        // STOP - don't look at openssl's deps when apt installed it

    case TSUKU_MANAGED:
        // Validate AND recurse
        validateSonameProvided(dep)
        recursivelyValidate(dep)  // Continue down the tree
    }
}
```

### Recursive Validation on Linux

Only **libyaml** triggers recursion (built from source):

```bash
$ readelf -d $TSUKU_HOME/libs/libyaml-0.2.5/lib/libyaml-0.so.2 | grep NEEDED
  NEEDED: libc.so.6
```

Classification: `libc.so.6` -> PURE SYSTEM -> Skip
Result: libyaml has no tsuku-managed deps, recursion ends cleanly.

**openssl, readline, zlib:** NO recursion. apt installed them, apt owns their dependency trees. We trust apt to have installed their transitive dependencies correctly.

### Recursive Validation on macOS

More libraries trigger recursion (built from source):

**libyaml:**
```bash
$ otool -L $TSUKU_HOME/libs/libyaml-0.2.5/lib/libyaml-0.2.dylib
  /usr/lib/libSystem.B.dylib
```
-> PURE SYSTEM -> Skip. Done.

**openssl (macOS builds from source!):**
```bash
$ otool -L $TSUKU_HOME/libs/openssl-3.2.1/lib/libssl.3.dylib
  /usr/lib/libSystem.B.dylib
  @rpath/libcrypto.3.dylib
```
-> libSystem: PURE SYSTEM -> Skip
-> libcrypto: TSUKU-MANAGED (same openssl recipe) -> Already validated. Done.

**readline (macOS builds from source!):**
```bash
$ otool -L $TSUKU_HOME/libs/readline-8.2/lib/libreadline.8.dylib
  /usr/lib/libSystem.B.dylib
  /usr/lib/libncurses.5.4.dylib
```
-> libSystem: PURE SYSTEM -> Skip
-> libncurses: PURE SYSTEM (macOS system lib) -> Skip. Done.

**zlib:** NO recursion. brew installed it, brew owns its dependency tree.

---

## Complete Flow Diagram

```
                    DT_NEEDED / LC_LOAD_DYLIB entry
                              |
                              v
                    +-------------------+
                    | In soname index?  |
                    +--------+----------+
                         |         |
                        YES        NO
                         |         |
                         v         v
              +---------------+  +------------------------+
              | Tsuku recipe  |  | System lib pattern?    |
              | found         |  | (platform-specific)    |
              +-------+-------+  +-----------+------------+
                      |                  |         |
                      v                 YES        NO
          +------------------------+     |         |
          | Recipe externally      |     v         v
          | managed for platform?  | +--------+ +----------+
          +-----------+------------+ |  PURE  | | WARNING  |
                  |         |        | SYSTEM | | (unknown)|
                 YES        NO       | (skip) | +----------+
                  |         |        +--------+
                  v         v
          +------------+  +-------------+
          | EXTERNALLY |  | TSUKU       |
          | MANAGED    |  | MANAGED     |
          +-----+------+  +------+------+
                |                |
                v                v
          +------------+  +-------------+
          | Validate   |  | Validate    |
          | soname     |  | soname      |
          | STOP       |  | RECURSE     |
          | (apt/brew  |  | (validate   |
          | owns tree) |  | its deps)   |
          +------------+  +-------------+
```

---

## Example Output

### Linux (glibc) - Mixed Categories

```
$ tsuku verify ruby --deep
Verifying ruby (version 3.3.0) on linux-glibc-x86_64...

  Tier 1: Header validation
    bin/ruby: OK (ELF x86_64)

  ABI Compatibility:
    Interpreter: /lib64/ld-linux-x86-64.so.2 [OK]

  Tier 2: Dependency validation
    Binary deps: libc.so.6, libm.so.6, libpthread.so.0, libdl.so.2,
                 libyaml-0.so.2, libssl.so.3, libcrypto.so.3,
                 libreadline.so.8, libz.so.1

    libc.so.6        -> SYSTEM (skip)
    libm.so.6        -> SYSTEM (skip)
    libpthread.so.0  -> SYSTEM (skip)
    libdl.so.2       -> SYSTEM (skip)
    libyaml-0.so.2   -> libyaml [OK] (tsuku-managed, source build)
    libssl.so.3      -> openssl [OK] (externally-managed via apt)
    libcrypto.so.3   -> openssl [OK] (externally-managed via apt)
    libreadline.so.8 -> readline [OK] (externally-managed via apt)
    libz.so.1        -> zlib [OK] (externally-managed via apt)

  Recursive validation (--deep):
    -> libyaml: recursing into tsuku-managed library...
       libc.so.6 -> SYSTEM (skip)
       libyaml OK (only system deps)
    -> openssl: externally-managed via apt, recursion stopped
    -> readline: externally-managed via apt, recursion stopped
    -> zlib: externally-managed via apt, recursion stopped

  Summary:
    PURE SYSTEM: 4 (skipped)
    TSUKU-MANAGED: 1 (validated, recursed)
    EXTERNALLY-MANAGED: 4 (validated, no recursion)

ruby verified successfully
```

### macOS (Darwin) - More TSUKU-MANAGED

```
$ tsuku verify ruby --deep
Verifying ruby (version 3.3.0) on darwin-arm64...

  Tier 1: Header validation
    bin/ruby: OK (Mach-O arm64)

  ABI Compatibility:
    (macOS uses dyld - always present)

  Tier 2: Dependency validation
    Binary deps: /usr/lib/libSystem.B.dylib, @rpath/libyaml-0.2.dylib,
                 @rpath/libssl.3.dylib, @rpath/libcrypto.3.dylib,
                 @rpath/libreadline.8.dylib, @rpath/libz.1.dylib

    /usr/lib/libSystem.B.dylib -> SYSTEM (skip)
    @rpath/libyaml-0.2.dylib   -> libyaml [OK] (tsuku-managed, source build)
    @rpath/libssl.3.dylib      -> openssl [OK] (tsuku-managed, source build)
    @rpath/libcrypto.3.dylib   -> openssl [OK] (tsuku-managed, source build)
    @rpath/libreadline.8.dylib -> readline [OK] (tsuku-managed, source build)
    @rpath/libz.1.dylib        -> zlib [OK] (externally-managed via brew)

  Recursive validation (--deep):
    -> libyaml: recursing into tsuku-managed library...
       libSystem.B.dylib -> SYSTEM (skip)
       libyaml OK (only system deps)
    -> openssl: recursing into tsuku-managed library...
       libSystem.B.dylib -> SYSTEM (skip)
       libcrypto.3.dylib -> openssl (already validated)
       openssl OK
    -> readline: recursing into tsuku-managed library...
       libSystem.B.dylib -> SYSTEM (skip)
       libncurses.5.4.dylib -> SYSTEM (skip)
       readline OK (only system deps)
    -> zlib: externally-managed via brew, recursion stopped

  Summary:
    PURE SYSTEM: 1 (skipped)
    TSUKU-MANAGED: 4 (validated, recursed)
    EXTERNALLY-MANAGED: 1 (validated, no recursion)

ruby verified successfully
```

---

## Summary: Why Platform Management Differs

| Library | Linux Management | macOS Management | Why Different? |
|---------|-----------------|------------------|----------------|
| libyaml | TSUKU-MANAGED | TSUKU-MANAGED | Consistent: always build from source for control |
| openssl | EXTERNALLY-MANAGED (apt) | TSUKU-MANAGED | macOS deprecated system OpenSSL; must build |
| readline | EXTERNALLY-MANAGED (apt) | TSUKU-MANAGED | macOS "readline" is libedit; need real GNU readline |
| zlib | EXTERNALLY-MANAGED (apt) | EXTERNALLY-MANAGED (brew) | Common system lib, safe to use system version |
| libc/libSystem | PURE SYSTEM | PURE SYSTEM | Always OS-provided, never a tsuku recipe |

### Key Takeaways

1. **PURE SYSTEM** = No tsuku recipe exists (inherently OS-provided). Pattern-matched and skipped.

2. **TSUKU-MANAGED** = Tsuku recipe builds from source. We validate AND recurse because we control the entire dependency tree.

3. **EXTERNALLY-MANAGED** = Tsuku recipe delegates to package manager. We validate the soname is provided but STOP recursion because apt/brew/apk owns the transitive dependencies.

4. **Same library, different management per platform.** The recipe's `when` clauses determine management style. This reflects real-world constraints (Apple deprecations, BSD vs GNU differences).

5. **Recursion stops at external boundaries.** When apt installs openssl, apt also installed openssl's dependencies (libcrypto, etc.). We trust the package manager within its domain.
