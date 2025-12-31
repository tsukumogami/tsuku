# Investigation: Binary Compatibility Across Linux Distros

## Context

Tsuku downloads pre-built binaries from upstream releases (GitHub, etc.). We assumed "simple" recipes like ripgrep are distro-agnostic, but this assumption needs validation given:
- Alpine uses musl libc (not glibc)
- NixOS has non-standard filesystem layout
- Gentoo can be configured with various libc implementations
- Static vs dynamic linking affects portability

This document identifies research paths to inform our `target = (platform, distro)` design.

---

## Investigation Path 1: C Library Landscape

### Question
Which Linux distributions use glibc vs musl vs other libc implementations, and what is the binary compatibility story between them?

### Why It Matters
If most GitHub releases ship glibc-linked binaries, tsuku needs to know which distros cannot run them. This directly affects whether `distro` is a required dimension in our target model, or whether a simpler `libc` dimension suffices.

### Research Methodology
1. Create a matrix of major distros and their default libc:
   - Survey: Debian, Ubuntu, Fedora, RHEL, Alpine, Void, Gentoo, Arch, NixOS, openSUSE
   - Note which support multiple libc options (e.g., Void has both glibc and musl variants)
2. Test binary compatibility empirically:
   - Take a glibc-linked binary (e.g., ripgrep from GitHub releases)
   - Attempt to run on musl-based Alpine
   - Document failure modes and error messages
3. Research glibc version compatibility:
   - Do binaries compiled on newer glibc run on older?
   - What's the "safe" minimum glibc version for broad compatibility?

### Deliverables
- Distro/libc compatibility matrix (table)
- Empirical test results with specific error messages
- Recommended minimum glibc version for tsuku-distributed binaries
- Decision recommendation: is `libc` sufficient or do we need full `distro`?

---

## Investigation Path 2: Static Binary Portability

### Question
What does "static binary" actually mean in practice, and are statically-linked binaries truly portable across all Linux distros?

### Why It Matters
Many Rust tools (ripgrep, fd, bat) can be compiled as static musl binaries. If static binaries are truly universal, tsuku could prefer them and potentially avoid the distro dimension entirely for many recipes.

### Research Methodology
1. Define "static" precisely:
   - What system calls do static binaries still make?
   - What kernel ABI assumptions exist?
   - Are there edge cases (NSS, DNS resolution, locale)?
2. Survey static binary availability:
   - Sample 20 popular tools from tsuku recipes
   - Check if upstream provides static/musl builds
   - Note naming conventions (x86_64-unknown-linux-musl, etc.)
3. Test static binary portability:
   - Run same static binary on: Ubuntu, Alpine, NixOS, old CentOS 7
   - Test edge cases: DNS resolution, user lookup, locale handling

### Deliverables
- Technical explainer: what "static" means and its limitations
- Survey results: which tools provide static builds
- Portability test matrix
- Recommendation: can tsuku prefer static builds as a strategy?

---

## Investigation Path 3: Filesystem Layout Assumptions

### Question
What filesystem paths do pre-built binaries assume, and which distros violate these assumptions?

### Why It Matters
Even if a binary can execute, it may fail if it expects files in locations that don't exist. NixOS is particularly notable for non-standard paths, but other distros have variations too.

### Research Methodology
1. Identify common hardcoded paths in binaries:
   - `/lib64/ld-linux-x86-64.so.2` (dynamic linker)
   - `/etc/` configuration files
   - `/usr/share/` data files
   - Cert paths (`/etc/ssl/certs/`, `/etc/pki/`)
2. Map distro filesystem variations:
   - NixOS: `/nix/store/` based paths
   - Gentoo: can vary based on profile
   - GoboLinux: completely non-standard
3. Analyze failure modes:
   - What happens when expected paths don't exist?
   - Can tsuku provide shims or environment variables?

### Deliverables
- Common hardcoded path inventory
- Distro filesystem divergence matrix
- Failure mode catalog with solutions
- Design recommendation for path-related recipe configuration

---

## Investigation Path 4: Dynamic Linking Dependencies

### Question
Beyond libc, what other shared library dependencies do common tools have, and how do these vary across distros?

### Why It Matters
A binary might link against libssl, libcurl, libz, etc. Even with matching libc, missing or incompatible versions of these libraries cause failures. This affects whether tsuku needs to track more than just libc.

### Research Methodology
1. Audit dependencies of sample tools:
   - Use `ldd` on binaries from GitHub releases
   - Categorize: libc-only vs additional deps
   - Note which deps are "usually present" vs "often missing"
2. Test cross-distro library availability:
   - Compare library versions across Ubuntu LTS, Fedora, Alpine, Arch
   - Identify common version mismatches
3. Research bundling strategies:
   - Do some projects bundle dependencies?
   - What's the binary size vs portability tradeoff?

### Deliverables
- Dependency audit for 20 sample tools
- Cross-distro library availability matrix
- Categorization: "safe" deps vs "problematic" deps
- Recommendation for recipe metadata about dependencies

---

## Investigation Path 5: GitHub Release Binary Survey

### Question
What binary variants do popular projects actually ship in their GitHub releases, and is there a common pattern?

### Why It Matters
Our target model should reflect reality. If 90% of projects ship only glibc x86_64, that's different from a world where musl variants are common. This directly informs recipe authoring guidelines.

### Research Methodology
1. Survey GitHub releases for 30+ popular tools:
   - Record all Linux binary variants offered
   - Note naming conventions
   - Check for static/musl options
2. Analyze patterns:
   - What percentage offer musl builds?
   - What percentage offer ARM64?
   - Do Rust vs Go vs C++ projects differ?
3. Check download statistics:
   - Which variants are actually downloaded?
   - Is there signal about real-world distro usage?

### Deliverables
- Survey data spreadsheet (tool, variants, naming)
- Statistical summary of variant availability
- Naming convention guide for recipe authors
- Recommendation for default variant selection logic

---

## Investigation Path 6: Distro Detection Mechanisms

### Question
How can tsuku reliably detect which distro (and libc) it's running on, and what are the edge cases?

### Why It Matters
If our target model includes `distro`, tsuku needs to detect it at runtime. Containers, WSL, and custom systems complicate this.

### Research Methodology
1. Inventory detection methods:
   - `/etc/os-release` parsing
   - `ldd --version` for libc detection
   - Package manager presence
   - Filesystem markers
2. Test edge cases:
   - Docker containers (may lack os-release)
   - WSL (reports as Ubuntu but has quirks)
   - Custom/minimal installs
   - NixOS (os-release exists but paths are unusual)
3. Research existing solutions:
   - How do other tools detect distro?
   - What does rustup/golang installers do?

### Deliverables
- Detection algorithm pseudocode
- Edge case catalog with handling strategies
- Comparison with other installers' approaches
- Recommended detection implementation for tsuku

---

## Investigation Path 7: Real-World Failure Analysis

### Question
What actual failures do users experience when running pre-built binaries on "incompatible" distros, and how severe are they?

### Why It Matters
Theoretical incompatibility may not matter if failures are rare or have easy workarounds. Conversely, common failures justify complexity in our model.

### Research Methodology
1. Search issue trackers:
   - ripgrep, fd, bat, delta issues mentioning Alpine/musl
   - Look for "glibc not found" type errors
   - Categorize: blocker vs cosmetic vs workaround-exists
2. Test empirically:
   - Attempt to install glibc binaries on Alpine via tsuku
   - Document exact failure messages
   - Try potential workarounds (gcompat, etc.)
3. Survey user expectations:
   - Do Alpine/NixOS users expect tools to "just work"?
   - What's acceptable UX for incompatibility?

### Deliverables
- Issue tracker analysis report
- Empirical failure documentation
- Workaround catalog (gcompat, static builds, etc.)
- UX recommendation for handling incompatibility

---

## Summary: Research Priority

| Path | Priority | Effort | Impact |
|------|----------|--------|--------|
| 1. C Library Landscape | High | Medium | Foundational for target model |
| 2. Static Binary Portability | High | Medium | Could simplify model significantly |
| 5. GitHub Release Survey | High | Low | Grounds model in reality |
| 4. Dynamic Linking Deps | Medium | Medium | Affects edge cases |
| 6. Distro Detection | Medium | Low | Implementation-focused |
| 3. Filesystem Layout | Medium | High | NixOS-specific mostly |
| 7. Failure Analysis | Low | Medium | Validates other findings |

### Recommended Approach

1. **Start with Path 5** (GitHub Release Survey) - quick data gathering
2. **Then Path 1** (C Library) - establish theoretical foundation
3. **Then Path 2** (Static Binaries) - explore simplification strategy
4. **Remaining paths** as needed based on initial findings

### Key Decision Points

After research, we should be able to answer:

1. **Do we need `distro` or just `libc` in our target model?**
2. **Can we prefer static binaries and reduce complexity?**
3. **What's the minimum viable target model for 90% of use cases?**
4. **What UX should tsuku provide when no compatible binary exists?**
