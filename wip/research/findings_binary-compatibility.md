# Alpine/musl Compatibility Test Results

Testing whether "generic Linux" binaries work on Alpine Linux (musl libc).

## Test Environment

- Host: Ubuntu (glibc)
- Container: `alpine:latest` (musl libc 1.2.x)
- Method: `docker run --rm -v $(pwd):/test alpine /test/<binary> --version`

## Test Results

| Binary | Libc Type | Static? | Alpine Works? | Notes |
|--------|-----------|---------|---------------|-------|
| fzf | Go static | Yes | **YES** | `0.67.0 (2ab923f3)` |
| gh | Go static | Yes | **YES** | `gh version 2.83.2` |
| ripgrep-musl | musl | Yes | **YES** | `ripgrep 15.1.0` with PCRE2 |
| delta-glibc | glibc | No | **NO** | `no such file or directory` |
| gum | Go static | Yes | **YES** | `gum version v0.17.0` |
| deno-glibc | glibc | No | **NO** | `no such file or directory` |
| jq | glibc static | Yes | **YES** | `jq-1.7.1` |
| direnv | Go static | Yes | **YES** | `2.36.0` |
| bun-glibc | glibc | No | **NO** | `no such file or directory` |
| bun-musl | musl | No | **NO** | Missing `libstdc++.so.6`, `libgcc_s.so.1` |

## Detailed Analysis

### Category 1: Works Everywhere (Go Static Binaries)

**Tools: fzf, gh, gum, direnv, jq**

These binaries are statically linked with no external library dependencies:

```
$ file fzf-glibc
ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, Go BuildID=...

$ file gh-glibc
ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked, BuildID=...
```

Go's runtime is self-contained. Even though these are built on glibc systems, they don't actually depend on glibc at runtime.

**Conclusion:** Go static binaries work on ANY Linux regardless of libc.

### Category 2: Works Everywhere (musl Static Binaries)

**Tools: ripgrep-musl**

```
$ file rg-musl
ELF 64-bit LSB pie executable, x86-64, version 1 (SYSV), static-pie linked, stripped
```

Statically linked musl binaries work everywhere because they include the libc implementation.

**Conclusion:** Static musl binaries work on ANY Linux.

### Category 3: Fails on Alpine (glibc Dynamic Binaries)

**Tools: delta-glibc, deno-glibc, bun-glibc**

These binaries require glibc and fail with the cryptic error:
```
exec /test/delta-glibc: no such file or directory
```

This error means the dynamic linker (`/lib64/ld-linux-x86-64.so.2`) doesn't exist on Alpine.

```
$ file delta-glibc
ELF 64-bit LSB pie executable, x86-64, version 1 (SYSV), dynamically linked,
interpreter /lib64/ld-linux-x86-64.so.2, ...

$ ldd delta-glibc
linux-vdso.so.1
libz.so.1 => /lib/x86_64-linux-gnu/libz.so.1
libgcc_s.so.1 => /lib/x86_64-linux-gnu/libgcc_s.so.1
libpthread.so.0 => /lib/x86_64-linux-gnu/libpthread.so.0
libm.so.6 => /lib/x86_64-linux-gnu/libm.so.6
libdl.so.2 => /lib/x86_64-linux-gnu/libdl.so.2
libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6
/lib64/ld-linux-x86-64.so.2
```

**Conclusion:** glibc dynamic binaries DO NOT work on Alpine without glibc-compat.

### Category 4: Fails on Alpine (musl Dynamic Binaries)

**Tools: bun-musl**

Even though bun-musl is linked against musl, it fails because it depends on libstdc++:

```
$ file bun-musl
ELF 64-bit LSB executable, x86-64, version 1 (SYSV), dynamically linked,
interpreter /lib/ld-musl-x86_64.so.1, ...

Error loading shared library libstdc++.so.6: No such file or directory
Error loading shared library libgcc_s.so.1: No such file or directory
```

The musl libc itself would load, but C++ standard library is missing.

**Conclusion:** C++ musl binaries may need additional libraries installed.

### jq: Special Case (glibc Static)

jq shows that even C programs can be statically linked:

```
$ file jq-glibc
ELF 64-bit LSB executable, x86-64, version 1 (SYSV), statically linked,
BuildID=..., for GNU/Linux 3.2.0, stripped
```

Despite being built for glibc and having `for GNU/Linux` in the file info, it works on Alpine because it's statically linked.

## Summary Table

| Linking Type | Example | Works on Alpine? |
|--------------|---------|------------------|
| Go static | fzf, gh, gum | YES |
| Rust musl static | ripgrep-musl | YES |
| C static | jq | YES |
| Rust glibc dynamic | delta | NO |
| Zig/LLVM glibc dynamic | deno | NO |
| C++ musl dynamic | bun-musl | NO (needs libstdc++) |

## Key Findings

1. **Static linking is the key**, not glibc vs musl
2. **Go tools just work** - they're always statically linked
3. **Rust musl builds work** - when statically linked
4. **glibc dynamic binaries fail** - missing `/lib64/ld-linux-x86-64.so.2`
5. **C++ complicates musl** - libstdc++ dependency persists

## Implications for tsuku

1. **For Go tools**: No libc detection needed - single binary works everywhere
2. **For Rust tools**: Prefer musl variant when available
3. **For C/C++ tools**: May need to detect glibc vs musl and select appropriately
4. **Error message**: `no such file or directory` for an existing file = wrong libc
