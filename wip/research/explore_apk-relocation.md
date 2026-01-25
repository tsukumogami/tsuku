# APK Binary Relocation Analysis for tsuku

**Date:** 2026-01-24
**Related Design:** DESIGN-platform-compatibility-verification.md
**Issue:** #1092

---

## Executive Summary

**APK binaries do NOT need relocation** like Homebrew bottles do. Alpine packages install to system paths (`/usr/lib`, `/usr/bin`) by design and have no placeholder strings or problematic RPATH settings. If tsuku extracts APK files to a custom location (`$TSUKU_HOME/libs/`), it would need to run patchelf to set RPATH, but the complexity is significantly lower than Homebrew relocation because there are no text placeholders to replace.

**Recommendation:** Use `apk_install` (system packages) rather than APK extraction. This aligns with the design document's decision and avoids all relocation complexity.

---

## Question 1: Do Alpine Packages Have Hardcoded Paths?

**Answer: Yes, but not like Homebrew.**

### Homebrew Bottles

Homebrew uses placeholder strings that must be replaced at install time:

| Placeholder | Purpose | Example |
|-------------|---------|---------|
| `@@HOMEBREW_PREFIX@@` | Installation root | `/opt/homebrew` or `/home/linuxbrew/.linuxbrew` |
| `@@HOMEBREW_CELLAR@@` | Package storage | `/opt/homebrew/Cellar` |

These placeholders appear in:
- Text configuration files (e.g., `curl-config`)
- Binary RPATH entries
- Script shebangs

The `relocatePlaceholders()` function in `homebrew_relocate.go` handles this by:
1. Scanning all files for placeholder bytes
2. Replacing text placeholders in config files
3. Using patchelf/install_name_tool to fix binary RPATH

### Alpine APK Packages

Alpine packages do NOT use placeholder strings. They are built with standard FHS paths:

| Component | Default Path | Source |
|-----------|--------------|--------|
| Libraries | `/usr/lib/` | Standard prefix=`/usr` |
| Binaries | `/usr/bin/` | Standard prefix=`/usr` |
| Config | `/etc/` | Standard FHS |

Alpine's build system uses `--prefix=/usr` by default, and packages expect to be installed to the root filesystem. There are no `@@ALPINE_PREFIX@@` or similar placeholders.

**Source:** [Alpine APKBUILD Reference](https://wiki.alpinelinux.org/wiki/APKBUILD_Reference) documents default configure options.

---

## Question 2: Do APK Binaries Have RPATH Set?

**Answer: Generally no, or set to standard system paths.**

### Standard Alpine Package Behavior

Alpine packages are built for installation to `/usr/lib` and `/usr/bin`. Most binaries:
- Have no RPATH set (rely on default search paths)
- Use system library paths when RPATH is present

The musl dynamic linker searches in this order:
1. `LD_LIBRARY_PATH` environment variable
2. RPATH/RUNPATH embedded in binary
3. Default paths from `/etc/ld-musl-$ARCH.path` (or built-in defaults)

**Source:** [musl Manual](https://www.musl-libc.org/doc/1.0.0/manual.html) documents LD_LIBRARY_PATH behavior.

### musl RPATH Behavior

musl's ld.so supports `$ORIGIN` in RPATH, just like glibc:

```bash
# Example: binary with $ORIGIN RPATH
$ readelf -d binary | grep PATH
0x0000000f (RPATH)    Library rpath: [$ORIGIN/../lib]
```

Unlike glibc, musl handles DT_RUNPATH and DT_RPATH identically (no priority differences).

**Source:** [rpath Wikipedia](https://en.wikipedia.org/wiki/Rpath) and [Shared Library Search Paths](https://www.eyrie.org/~eagle/notes/rpath.html)

---

## Question 3: Would patchelf Need to Run on APK-Sourced Binaries?

**Answer: Only if extracting to non-standard location.**

### Scenario A: Using `apk_install` (System Packages)

**No patchelf needed.** Packages install to `/usr/lib` which is in the default library search path. Binaries work out of the box.

### Scenario B: Extracting APK to `$TSUKU_HOME/libs/`

**Yes, patchelf would be needed** to set RPATH so binaries can find sibling libraries:

```bash
# After extracting zlib APK to $TSUKU_HOME/libs/zlib-1.3.1/
patchelf --set-rpath '$ORIGIN' $TSUKU_HOME/libs/zlib-1.3.1/lib/libz.so.1.3.1
```

However, this is simpler than Homebrew relocation because:
1. No text placeholder replacement needed
2. No interpreter path fixup needed (musl binary on musl system)
3. Only RPATH needs modification

### Complexity Comparison

| Task | Homebrew Bottles | Alpine APK Extraction |
|------|------------------|----------------------|
| Text placeholder replacement | Required | Not needed |
| Binary RPATH fixup | Required | Required |
| Interpreter path fixup | Required (glibc bottles) | Not needed |
| Codesign (macOS ARM64) | Required | N/A (Linux only) |
| Mach-O install_name fixup | Required | N/A (Linux only) |

**Estimated LOC for APK RPATH fixup:** ~50-100 lines (vs. ~400+ for Homebrew relocation)

---

## Question 4: Can tsuku-dltest Verify APK-Sourced .so Files?

**Answer: Yes, with caveats.**

### Compatibility Matrix

| Scenario | tsuku-dltest Binary | Library Source | Works? | Notes |
|----------|---------------------|----------------|--------|-------|
| glibc host | glibc-linked | Homebrew bottle | Yes | Current behavior |
| glibc host | glibc-linked | APK package | **No** | libc mismatch |
| musl host | musl-linked | APK package | Yes | Native libraries |
| musl host | musl-linked | Homebrew bottle | **No** | glibc bottle on musl |

### Requirements for APK Verification

1. **tsuku-dltest must be built for musl** when running on Alpine
2. **Library paths must be correct** (`$TSUKU_HOME/libs` added to `LD_LIBRARY_PATH`)
3. **musl's /etc/ld-musl-$ARCH.path** may need configuration for non-standard paths

The current `sanitizeEnvForHelper()` in `dltest.go` already prepends `$TSUKU_HOME/libs` to `LD_LIBRARY_PATH`, which would work for APK-extracted libraries.

### Current dltest.go Behavior

```go
// From sanitizeEnvForHelper():
libsDir := filepath.Join(tsukuHome, "libs")
env = append(env, fmt.Sprintf("LD_LIBRARY_PATH=%s:%s", libsDir, os.Getenv("LD_LIBRARY_PATH")))
```

This is sufficient for APK-sourced libraries if:
1. Libraries are extracted to `$TSUKU_HOME/libs/<package>/`
2. tsuku-dltest is built for musl (on Alpine)

---

## Question 5: Alpine-Specific Loader Behaviors

**Answer: Minor differences, mostly advantageous.**

### musl vs glibc Loader Differences

| Aspect | glibc | musl | Impact on tsuku |
|--------|-------|------|-----------------|
| Library search | Complex (ldconfig, cache) | Simple (path list) | Simpler debugging |
| `$ORIGIN` support | Yes | Yes | No difference |
| RPATH vs RUNPATH | Different precedence | Same precedence | Simpler |
| Unified libc | No (libc, pthread, rt, etc.) | Yes (single libc.so) | Fewer deps |
| Default search path | `/lib:/usr/lib` | From `/etc/ld-musl-*.path` | Must configure |

**Source:** [musl FAQ](https://wiki.musl-libc.org/faq) and [musl Design Concepts](https://wiki.musl-libc.org/design-concepts)

### Key musl Consideration

musl does not have `ldconfig`. Instead, library search paths are configured in `/etc/ld-musl-$ARCH.path`:

```bash
# Example /etc/ld-musl-x86_64.path
/lib
/usr/lib
/usr/local/lib
```

If tsuku extracts libraries to `$TSUKU_HOME/libs`, it would need to either:
1. Rely on `LD_LIBRARY_PATH` (already done in dltest.go)
2. Set RPATH in binaries (via patchelf)
3. Advise users to add path to `/etc/ld-musl-*.path` (requires root)

Option 1 (LD_LIBRARY_PATH) is the simplest and what tsuku already does.

---

## Question 6: Minimal Viable Implementation for APK Relocation

**Answer: No relocation needed if using system packages.**

### Recommended Approach: System Packages

The design document already decided on "self-contained tools, system-managed dependencies." For Alpine:

```toml
# Example openssl recipe
[[steps]]
action = "apk_install"
packages = ["openssl-dev"]
```

This requires:
- No relocation
- No patchelf
- No RPATH fixup
- Works natively on musl

### Alternative Approach: APK Extraction (If Hermetic Required)

If version-pinned libraries are needed, APK extraction is simpler than Homebrew:

**Implementation steps:**
1. Download APK from Alpine CDN (tar.gz format)
2. Extract to `$TSUKU_HOME/libs/<package>-<version>/`
3. Run patchelf to set RPATH to `$ORIGIN` on .so files
4. No text replacement needed

**Estimated complexity:**
- New action: `apk_extract` (~100 LOC)
- RPATH fixup: Reuse `fixElfRpath()` from `homebrew_relocate.go` (~50 LOC borrowed)
- APKINDEX parsing for checksums: ~150 LOC
- **Total: ~300 LOC vs. ~700 LOC for Homebrew relocation**

### Code Reuse from homebrew_relocate.go

| Function | Reusable? | Notes |
|----------|-----------|-------|
| `fixElfRpath()` | Yes | Works for any ELF binary |
| `fixBinaryRpath()` | Partial | Magic detection is reusable |
| `isBinaryFile()` | Yes | Generic binary detection |
| `relocatePlaceholders()` | No | Homebrew-specific |
| `extractBottlePrefixes()` | No | Homebrew-specific |
| `fixMachoRpath()` | N/A | macOS only |

---

## Estimated Complexity

**Using system packages (apk_install):** **Low**
- tsuku already has `apk_install` action in `linux_pm_actions.go`
- Just need to update recipes to use it
- No relocation code needed

**APK extraction with relocation:** **Medium**
- Simpler than Homebrew (no text placeholders)
- patchelf logic can be borrowed from `homebrew_relocate.go`
- Need APKINDEX parsing for checksums
- Alpine CDN has simple, predictable URL structure

**Comparison:**

| Approach | Complexity | Hermetic? | Lines of Code | Maintenance |
|----------|------------|-----------|---------------|-------------|
| System packages | Low | No | 0 (existing) | None |
| APK extraction | Medium | Yes | ~300 new | APKINDEX format |
| Homebrew on musl | N/A | N/A | N/A | Impossible |

---

## Recommendations

### Primary Recommendation

**Use `apk_install` (system packages)** as decided in the design document. This:
- Works immediately with existing code
- Has zero relocation complexity
- Aligns with "self-contained tools, system-managed dependencies" philosophy
- Avoids glibc/musl incompatibility entirely

### If Hermetic Packages Needed Later

APK extraction is a viable future enhancement:
1. Reuse `fixElfRpath()` from homebrew_relocate.go
2. Implement simple APK download (it's just tar.gz)
3. Add APKINDEX parsing for checksums
4. Skip all text placeholder replacement (not needed)

This could be added in a future milestone without blocking current work.

---

## Sources

- [Alpine APKBUILD Reference](https://wiki.alpinelinux.org/wiki/APKBUILD_Reference)
- [musl libc Manual](https://www.musl-libc.org/doc/1.0.0/manual.html)
- [musl FAQ](https://wiki.musl-libc.org/faq)
- [musl Design Concepts](https://wiki.musl-libc.org/design-concepts)
- [Shared Library Search Paths](https://www.eyrie.org/~eagle/notes/rpath.html)
- [rpath Wikipedia](https://en.wikipedia.org/wiki/Rpath)
- [Homebrew Bottles Documentation](https://docs.brew.sh/Bottles)
- [Homebrew Binary Patching Issue #10846](https://github.com/Homebrew/brew/issues/10846)
- [Alpine /usr merge announcement](https://www.alpinelinux.org/posts/2025-10-01-usr-merge.html)
- [Alpine patchelf package](https://pkgs.alpinelinux.org/package/edge/main/armv7/patchelf)
