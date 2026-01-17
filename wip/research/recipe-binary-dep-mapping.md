# Recipe vs Binary Dependency Mapping

**Date:** 2026-01-17
**Purpose:** Analyze overlap between recipe `requires` and binary DT_NEEDED

## The Mapping Problem

| Level | Example | Format |
|-------|---------|--------|
| Recipe | `dependencies = ["openssl"]` | tsuku recipe name |
| Binary | `DT_NEEDED: libssl.so.3` | soname with version |

**No mapping exists** from soname → recipe name.

## Recipe-Level Dependencies

### Declaration (internal/recipe/types.go)

```go
type MetadataSection struct {
    Dependencies             []string  // Install-time
    RuntimeDependencies      []string  // Runtime
    ExtraDependencies        []string  // Additional install-time
    ExtraRuntimeDependencies []string  // Additional runtime
}
```

### Example (git-source.toml)

```toml
dependencies = ["libcurl", "brotli", "libnghttp2", "openssl", "zlib", "expat"]
```

These are **recipe names** that map to directories like:
- `$TSUKU_HOME/tools/openssl-3.1.0/`
- `$TSUKU_HOME/libs/zlib-1.3.1/`

### State Tracking (state.json)

```json
{
  "installed": {
    "git-source": {
      "install_dependencies": ["libcurl", "openssl", "zlib"],
      "runtime_dependencies": ["openssl"]
    }
  }
}
```

## Binary-Level Dependencies

### Example: Compiled curl binary

```
$ readelf -d ~/.tsuku/tools/curl-8.5.0/bin/curl | grep NEEDED
  0x0001 (NEEDED)  Shared library: [libssl.so.3]
  0x0001 (NEEDED)  Shared library: [libcrypto.so.3]
  0x0001 (NEEDED)  Shared library: [libz.so.1]
  0x0001 (NEEDED)  Shared library: [libc.so.6]
```

## The Gap

To map `libssl.so.3` back to recipe `openssl`, we need:

1. **Library metadata** - Recipe declares what sonames it provides:
   ```toml
   [metadata]
   name = "openssl"
   type = "library"
   provides = ["libssl.so.3", "libcrypto.so.3"]
   ```

2. **Discovery at install** - Scan installed library files:
   ```go
   // After installing openssl, scan lib/ for *.so files
   // Extract sonames and record in state.json
   ```

3. **Reverse lookup table** - Built at verification time:
   ```go
   sonames := map[string]string{
       "libssl.so.3":    "openssl",
       "libcrypto.so.3": "openssl",
       "libz.so.1":      "zlib",
   }
   ```

## Current Limitations

1. **No `provides` field** in recipe metadata
2. **No soname scanning** during installation
3. **No reverse lookup** capability
4. **System libraries** (libc.so.6) never map to recipes

## Implications for Verification

### Without Mapping (Current Design)

Tier 2 can only validate:
- System deps: Pattern match (libc, libm, etc.) → skip
- Path-based: $ORIGIN/libfoo.so → resolve and check exists
- Unknown: Absolute path /usr/lib/libfoo.so → warning

Cannot answer: "Is this dependency provided by a tsuku recipe?"

### With Mapping (Proposed)

Tier 2 could:
- Extract DT_NEEDED from binary
- Look up each soname in reverse table
- If found: Recursively verify that recipe
- If not found + not system: Warn about undeclared dep

## Proposed Solution

### Option 1: Explicit `provides` in Recipes

```toml
[metadata]
name = "openssl"
type = "library"
provides = ["libssl.so.3", "libcrypto.so.3"]
```

**Pros:** Explicit, version-controlled
**Cons:** Manual maintenance, can drift from reality

### Option 2: Auto-Discovery at Install

```go
func recordProvidedLibraries(installDir string, state *LibraryState) {
    // Walk lib/ directory
    // Extract soname from each .so file using readelf
    // Store in state.json under library entry
}
```

**Pros:** Always accurate, no maintenance
**Cons:** Requires parsing binaries, slower install

### Option 3: Hybrid

- Auto-discover at install time
- Store in state.json
- Optional `provides` override in recipe for edge cases

## Recommendation

**Start with auto-discovery (Option 2)** because:
- Library verification already reads binaries (Tier 1)
- No recipe changes needed
- Always accurate
- Can add `provides` override later if needed
