# Recommendations for tsuku Binary Selection

Based on the survey of 50+ developer tools and Alpine compatibility testing.

## Executive Summary

**tsuku CAN assume most "linux-amd64" binaries work everywhere**, with the following caveats:

1. **Go static binaries** (56% of tools): Work on any Linux - no detection needed
2. **Rust musl binaries** (when available): Work on any Linux - prefer these
3. **glibc dynamic binaries** (15% of tools): Need glibc - detection needed for full compatibility

## Recommendation 1: Binary Selection Strategy

### Tier 1: Always Prefer Static Binaries
If a tool ships static binaries (detectable from asset name or build metadata), use them unconditionally.

**Detection heuristics:**
- Go tools: Usually no libc indicator in name = static
- Musl in name with no other libs = usually static
- `.AppImage` files are self-contained

### Tier 2: Prefer musl When Both Available
For Rust tools that ship both glibc and musl variants:
```
# Prefer musl over glibc
ripgrep-15.1.0-x86_64-unknown-linux-musl.tar.gz   # Prefer
ripgrep-15.1.0-x86_64-unknown-linux-gnu.tar.gz    # Fallback
```

Musl static binaries work on both glibc and musl systems.

### Tier 3: Fall Back to glibc for Dynamic Binaries
For tools that only ship glibc (deno, neovim, helix):
- These work on most systems (Ubuntu, Debian, Fedora, etc.)
- They fail on Alpine/musl without glibc-compat
- This is acceptable for initial implementation

## Recommendation 2: Recipe Asset Selection Logic

```go
// Pseudocode for asset selection
func selectLinuxAsset(assets []string, arch string) string {
    // Priority order for x86_64:
    patterns := []string{
        // Static musl (highest priority)
        "{name}-*-x86_64-unknown-linux-musl.tar.gz",
        "{name}*linux*musl*amd64*.tar.gz",

        // Go-style static (no libc indicator)
        "{name}_*_linux_amd64.tar.gz",
        "{name}-*-linux-amd64.tar.gz",
        "{name}*linux*amd64*",

        // glibc fallback (lowest priority)
        "{name}-*-x86_64-unknown-linux-gnu.tar.gz",
        "{name}*linux*gnu*amd64*",
    }

    for _, pattern := range patterns {
        if match := findMatch(assets, pattern); match != "" {
            return match
        }
    }
    return ""
}
```

## Recommendation 3: Libc Detection (Deferred)

For v1, tsuku can skip libc detection because:

1. **Most tools work everywhere** (Go static, Rust musl)
2. **glibc-only tools work on most user systems** (Ubuntu, Debian, Fedora, Arch)
3. **Alpine users are power users** who can troubleshoot

### Future Enhancement: Libc Detection
If/when needed, detection is simple:
```bash
# Check for musl
if ldd --version 2>&1 | grep -q musl; then
    LIBC="musl"
else
    LIBC="glibc"
fi
```

Or in Go:
```go
// Check if /lib/ld-musl-* exists
if _, err := os.Stat("/lib/ld-musl-x86_64.so.1"); err == nil {
    return "musl"
}
return "glibc"
```

## Recommendation 4: Recipe Schema Additions

Consider adding optional libc hints to recipes:

```toml
[download]
url = "https://github.com/..."

# Optional: specify asset preference
[download.linux]
prefer = "musl"  # or "glibc", "static"

# Optional: explicit variants
[download.linux.variants]
musl = "ripgrep-{version}-x86_64-unknown-linux-musl.tar.gz"
glibc = "ripgrep-{version}-x86_64-unknown-linux-gnu.tar.gz"
```

This allows recipe authors to encode binary selection knowledge.

## Recommendation 5: Error Handling

When a binary fails to run, provide helpful error messages:

```
Error: ripgrep failed to execute
The binary may be incompatible with your system's libc.

Your system: musl (Alpine Linux)
Binary type: glibc (dynamically linked)

Try: tsuku install ripgrep --variant=musl
Or:  Install glibc compatibility layer
```

## Implementation Priority

### Phase 1 (Current)
- Use simple URL templates
- Prefer musl when obviously available in asset name
- Accept that some tools won't work on Alpine

### Phase 2 (Future)
- Add libc detection
- Automatic variant selection
- Better error messages for incompatible binaries

### Phase 3 (Far Future)
- Recipe-level variant specifications
- Community-contributed compatibility matrix
- Automatic testing of binaries on multiple libc types

## Tool-Specific Notes

### Tools That Need musl Variant
If targeting Alpine/musl, these tools require the musl-specific build:
- delta (no musl build available - glibc only, x86_64)
- deno (no musl build - glibc only)
- helix (no musl build)
- neovim (glibc only, consider AppImage)

### Tools That Just Work Everywhere
These tools ship static binaries - no variant selection needed:
- All Go tools: gh, fzf, yq, helm, terraform, vault, k9s, dive, etc.
- jq (static C build)
- btop, just, zoxide, xsv, xh (musl-only but static)

### Tools With Excellent Variant Support
These tools ship both glibc and musl, well-labeled:
- All sharkdp tools: fd, bat, hyperfine, hexyl, vivid, pastel, diskus
- ripgrep, sd, dust, starship, nushell, lsd, mdBook, bun

## Conclusion

The landscape is more compatible than expected:

1. **56% of tools are statically linked** - zero compatibility issues
2. **41% ship musl variants** - can be made fully portable
3. **Only ~15% are glibc-only dynamic** - the only potential problem

For tsuku's initial implementation:
- **Don't overthink it** - most binaries just work
- **Prefer musl when visible in filename**
- **Accept glibc fallback for remaining tools**
- **Add detection/selection later if users report issues**
