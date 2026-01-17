# Ruby Validation Example - Three-Category Model

**Date:** 2026-01-17
**Purpose:** Concrete walkthrough of Tier 2 validation using Ruby as example

## The Three Categories

| Category | Example | What We Do |
|----------|---------|------------|
| **Pure system lib** | `libc.so.6`, `libm.so.6` | Skip entirely (no tsuku recipe exists) |
| **Tsuku-managed** | `libyaml` (built from source) | Validate soname + recurse into its deps |
| **Externally-managed tsuku recipe** | `openssl-system` (via apt) | Validate provides expected soname, but STOP recursion |

## Ruby Example Walkthrough

### Setup

Assume Ruby is built from source by tsuku with these declared dependencies:

```toml
# recipes/ruby.toml
[recipe]
name = "ruby"
type = "tool"
dependencies = ["libyaml", "openssl", "readline", "zlib"]
```

### Step 1: Extract Binary Dependencies

```bash
$ readelf -d ~/.tsuku/tools/ruby-3.3.0/bin/ruby | grep NEEDED
  NEEDED: libc.so.6
  NEEDED: libm.so.6
  NEEDED: libpthread.so.0
  NEEDED: libyaml-0.so.2
  NEEDED: libssl.so.3
  NEEDED: libcrypto.so.3
  NEEDED: libreadline.so.8
  NEEDED: libz.so.1
```

### Step 2: Build Soname Index from state.json

At verification time, we build an in-memory index from installed libraries:

```json
// state.json (simplified)
{
  "libs": {
    "libyaml": {
      "0.2.5": {
        "sonames": ["libyaml-0.so.2"]
      }
    },
    "openssl": {
      "3.2.1": {
        "sonames": ["libssl.so.3", "libcrypto.so.3"]
      }
    },
    "readline": {
      "8.2": {
        "sonames": ["libreadline.so.8"]
      }
    },
    "zlib": {
      "1.3.1": {
        "sonames": ["libz.so.1"]
      }
    }
  }
}
```

**Resulting index:**
```
libyaml-0.so.2   → libyaml
libssl.so.3      → openssl
libcrypto.so.3   → openssl
libreadline.so.8 → readline
libz.so.1        → zlib
```

### Step 3: Classify Each DT_NEEDED Entry

For each binary dependency, we ask TWO questions in order:

1. **Is it in our soname index?** (Do we have a tsuku recipe providing this?)
2. **If not, is it a known system library pattern?**

| DT_NEEDED | In Index? | System Pattern? | Category | Action |
|-----------|-----------|-----------------|----------|--------|
| `libc.so.6` | No | Yes (`libc.so`) | PURE SYSTEM | Skip |
| `libm.so.6` | No | Yes (`libm.so`) | PURE SYSTEM | Skip |
| `libpthread.so.0` | No | Yes (`libpthread.so`) | PURE SYSTEM | Skip |
| `libyaml-0.so.2` | Yes → libyaml | - | TSUKU-MANAGED | Validate + Recurse |
| `libssl.so.3` | Yes → openssl | - | TSUKU-MANAGED | Validate + Recurse |
| `libcrypto.so.3` | Yes → openssl | - | TSUKU-MANAGED | Validate + Recurse |
| `libreadline.so.8` | Yes → readline | - | TSUKU-MANAGED | Validate + Recurse |
| `libz.so.1` | Yes → zlib | - | TSUKU-MANAGED | Validate + Recurse |

**Key insight:** We check the soname index FIRST. If a soname is in our index, we know tsuku provides it. If not in our index, THEN we check system patterns.

### Step 4: Validate Tsuku-Managed Dependencies

For each tsuku-managed dependency found in index:

1. **Check recipe declaration:** Is `libyaml` in Ruby's `dependencies`? YES
2. **Verify installation:** Is `libyaml` actually installed? YES
3. **Verify soname match:** Does installed libyaml provide `libyaml-0.so.2`? YES

If any check fails → Warning (dep not declared, or declared but not installed, or soname mismatch).

### Step 5: Recursive Validation (--deep)

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

### Example: openssl as Externally-Managed

Suppose on this system, openssl is installed via apt:

```toml
# recipes/openssl.toml
[recipe]
name = "openssl"
type = "library"

[[steps]]
action = "apt_install"
packages = ["libssl-dev"]
```

The `apt_install` action returns `IsExternallyManaged() = true`.

**Validation for openssl:**
1. Is openssl in Ruby's deps? YES
2. Is openssl installed (in state.json)? YES
3. Does it provide `libssl.so.3`? YES (we extracted this at install time)
4. **STOP HERE** - don't recurse into openssl's dependencies

**Why stop?** Because apt manages openssl. Whatever openssl depends on (libz, etc.) is apt's responsibility. If libssl.so.3 works, the system is correctly configured.

### Example: libyaml as Fully Tsuku-Managed

Suppose libyaml is built from source:

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

No `apt_install` or `brew_install` → `IsExternallyManaged() = false`.

**Validation for libyaml:**
1. Is libyaml in Ruby's deps? YES
2. Is libyaml installed? YES
3. Does it provide `libyaml-0.so.2`? YES
4. **RECURSE** into libyaml's binary dependencies

Extract libyaml's DT_NEEDED:
```bash
$ readelf -d ~/.tsuku/libs/libyaml-0.2.5/lib/libyaml-0.so.2 | grep NEEDED
  NEEDED: libc.so.6
```

Classify: `libc.so.6` is PURE SYSTEM → Skip.

libyaml validation complete.

## Complete Flow Diagram

```
                    DT_NEEDED entry
                          │
                          ▼
                ┌─────────────────┐
                │ In soname index? │
                └────────┬────────┘
                    │         │
                   YES        NO
                    │         │
                    ▼         ▼
           ┌────────────┐  ┌──────────────────┐
           │TSUKU-MANAGED│  │System lib pattern?│
           └──────┬─────┘  └────────┬─────────┘
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
        │(apt owns │  │(validate │
        │  the rest)│  │  its deps)│
        └──────────┘  └──────────┘
```

## Example Output

```
$ tsuku verify ruby --deep
Verifying ruby (version 3.3.0)...

  Tier 1: Header validation
    bin/ruby: OK (ELF x86_64)

  Tier 2: Dependency validation
    Binary deps: libc.so.6, libm.so.6, libpthread.so.0, libyaml-0.so.2,
                 libssl.so.3, libcrypto.so.3, libreadline.so.8, libz.so.1

    libc.so.6        → SYSTEM (glibc)
    libm.so.6        → SYSTEM (glibc)
    libpthread.so.0  → SYSTEM (glibc)
    libyaml-0.so.2   → libyaml ✓ (declared, installed, soname matches)
    libssl.so.3      → openssl ✓ (declared, installed, soname matches)
    libcrypto.so.3   → openssl ✓ (declared, installed, soname matches)
    libreadline.so.8 → readline ✓ (declared, installed, soname matches)
    libz.so.1        → zlib ✓ (declared, installed, soname matches)

  Recursive validation (--deep):
    → libyaml: OK (tsuku-managed, no non-system deps)
    → openssl: OK (externally-managed via apt, recursion stopped)
    → readline: OK (externally-managed via apt, recursion stopped)
    → zlib: OK (tsuku-managed, no non-system deps)

ruby verified successfully
```

## Summary

The order of checks matters:

1. **First:** Check if soname is in our index → If yes, it's TSUKU-MANAGED
2. **Second:** If not in index, check system patterns → If matches, it's PURE SYSTEM
3. **Third:** If neither → Warning (unknown dependency)

For tsuku-managed deps, we also check:
- Is the recipe externally managed? → If yes, validate but DON'T recurse
- Is the recipe built from source? → Validate AND recurse
