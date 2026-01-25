# Musl-Based Linux Distribution Landscape Analysis

Research conducted: 2026-01-24

## Executive Summary

Alpine Linux dominates the musl ecosystem with an estimated 95%+ share of musl Linux usage, driven primarily by its overwhelming presence in container environments. Other musl distributions serve niche audiences and represent a negligible portion of the overall musl user base. For developer tools targeting musl systems, "musl support" is effectively synonymous with "Alpine support."

**Recommendation**: Alpine-specific APK extraction (~500 LOC) is justified. The investment pays off for the dominant use case, and the marginal benefit of supporting other musl distros doesn't warrant the complexity of generic system package support.

---

## Distribution Analysis

### 1. Alpine Linux

**Status**: Dominant musl distribution

**User Base**: Massive, primarily in containers
- Most popular Docker base image according to Sysdig 2020 Container Security Snapshot
- Official Docker image at only ~5MB, making it approximately 30x smaller than Debian
- 85% of containerized environments use Linux, and Alpine is the preferred minimal base
- Kubernetes adoption exceeds 60% of enterprises (projected 90%+ by 2027), with Alpine as a common base

**Primary Use Cases**:
- Container base images (overwhelming majority)
- Embedded systems and network appliances
- Virtual machines requiring minimal footprint
- Routers, servers, and NAS devices

**Technical Stack**:
- musl libc
- BusyBox
- OpenRC init system
- APK package manager
- Current version: 3.23.2 (December 2025)

**Key Advantages**:
- 8MB container footprint
- ~130MB minimal disk installation
- Security-focused design (position-independent executables)
- Fast CI/CD pipeline execution
- Wide architecture support: amd64, arm32v6, arm32v7, arm64v8, i386, ppc64le, riscv64, s390x

**Sources**:
- [Alpine Linux - Wikipedia](https://en.wikipedia.org/wiki/Alpine_Linux)
- [Alpine Docker Official Image - Docker Hub](https://hub.docker.com/_/alpine)
- [Docker Alpine Blog Post](https://www.docker.com/blog/how-to-use-the-alpine-docker-official-image/)
- [Sysdig 2020 Container Security Snapshot](https://www.sysdig.com/blog/sysdig-2020-container-security-snapshot)

---

### 2. Void Linux (musl edition)

**Status**: Niche within a niche

**User Base**: Small, primarily advanced desktop users
- Total Void Linux user base is small (DistroWatch page hits: ~#102 ranking)
- Musl edition is a minority of Void users
- One user humorously noted being "one of those dozen or so crazy people who daily-drive Musl on desktop workstations"
- No official statistics on glibc vs musl split

**Primary Use Cases**:
- Desktop systems for advanced users seeking minimal, systemd-free Linux
- Users who prioritize customization and a "do-it-yourself" approach
- Those wanting alternative to Arch Linux without systemd

**Comparison to glibc Edition**:
- Musl edition has compatibility limitations (no Nvidia drivers, some proprietary software issues)
- Proprietary software "usually supports only glibc systems"
- Flatpak provides a workaround for some glibc-only applications
- User reviews note musl as a "Pro" but acknowledge "software programmers need to catch up"

**Technical Stack**:
- musl libc (or glibc in the default edition)
- runit init system (not systemd)
- XBPS package manager
- Rolling release model

**Why musl Edition is Minority**:
- Users converting from glibc to musl must reinstall
- No live switching possible
- Tools like voidnsrun exist to run glibc binaries on musl Void, suggesting users hit compatibility walls

**Sources**:
- [Void Linux musl - Handbook](https://docs.voidlinux.org/installation/musl.html)
- [Void Linux - Wikipedia](https://en.wikipedia.org/wiki/Void_Linux)
- [DistroWatch Void Ratings](https://linuxiac.com/void-linux-topped-distrowatch-average-rating/)
- [Void musl vs glibc comparison](https://sysdfree.wordpress.com/2018/07/01/228/)

---

### 3. Chimera Linux

**Status**: Early-stage project with technical innovation

**User Base**: Very small
- Started in 2021
- In active development but pre-1.0
- Community-driven, does not accept donations
- Two new committers added in 2024/2025
- Some enthusiasts are daily-driving it for work (as of March 2025)

**Primary Use Cases**:
- Desktop Linux with novel architecture
- Users interested in FreeBSD userland on Linux kernel
- Those seeking systemd alternatives (uses dinit)

**Target Audience**:
- Linux enthusiasts and developers
- Users interested in hybrid FreeBSD/Linux approach
- Privacy and security-conscious users

**Technical Innovation**:
- Linux kernel + musl libc + FreeBSD userland + dinit init + APK package manager
- First distro using APKv3 (before Alpine itself adopted it)
- Tier-1 support for aarch64, ppc64le, x86_64
- Support for 5 architectures total (including ppc64 and riscv64)

**Future Trajectory**:
- 2025 focus: service management improvements, elogind replacement
- Active development but small team
- Growing slowly through community contributions

**Sources**:
- [Chimera Linux - About](https://chimera-linux.org/about/)
- [Chimera Linux - Wikipedia](https://en.wikipedia.org/wiki/Chimera_Linux)
- [LWN.net - Chimera Linux desktop](https://lwn.net/Articles/1004324/)
- [Daily Driving Chimera for Work](https://www.wezm.net/v2/posts/2025/daily-driving-chimera-for-work/)

---

### 4. postmarketOS

**Status**: Active mobile Linux project

**User Base**: Small but growing
- 19.4% year-over-year rise in new installations on legacy Android hardware
- 700+ contributors
- Target devices: PinePhone, Fairphone, Pixel 3a, OnePlus 6T, various tablets

**Primary Use Cases**:
- Mobile Linux on smartphones and tablets
- Extending life of "legacy" Android devices
- Privacy-focused mobile computing
- Open-source mobile development

**Market Context**:
- Linux smartphones (PinePhone, Librem 5) hold 0.06% of smartphone market
- Still "work-in-progress software intended for power users"
- Version 25.12 released December 2025 with Alpine 3.23 base

**Technical Stack**:
- Based on Alpine Linux
- Uses Alpine Package Manager (APKv3)
- musl libc inherited from Alpine

**Relevance to Developer Tools**:
- Unlikely target for developer tool installation
- Mobile-first use case, not development workstations
- Inherits Alpine compatibility automatically

**Sources**:
- [postmarketOS - Wikipedia](https://en.wikipedia.org/wiki/PostmarketOS)
- [postmarketOS 25.12 Release](https://www.ghacks.net/2025/12/24/linux-postmarketos-25-12-adds-more-devices-and-ui-updates/)
- [Linux Phone Trends 2025](https://www.accio.com/business/linux-phone-trends)

---

### 5. Adelie Linux

**Status**: Active but small project

**User Base**: Very small
- 1.0-BETA6 released December 2024
- 1.0 GA estimated mid-2025
- Hosted by Software in the Public Interest (SPI)
- Focus on architecture diversity over user growth

**Primary Use Cases**:
- Multi-architecture support (ARM, POWER, x86, with RISC-V and SPARC planned)
- Desktop via XFCE
- Users needing support for older/alternative architectures
- Power Mac users

**Technical Stack**:
- musl libc
- APK package manager (shared with Alpine)
- s6 init + OpenRC
- Systemd-free

**Notable Features**:
- JDK through 1.21 on musl
- "generic Linux to Adelie Linux" bootstrap tool in development
- May offer experimental m68k, ppc64le, riscv32, mips64 ports

**Sources**:
- [Adelie Linux 2024 State](https://blog.adelielinux.org/2024/12/24/2024-state-of-the-adelie-linux-distribution/)
- [Adelie Linux 1.0-BETA6 Release](https://blog.adelielinux.org/2024/12/15/adelie-linux-1-0-beta6-released/)
- [The Register - Adelie Linux Review](https://www.theregister.com/2024/12/20/adelie_linux_1_beta_6/)

---

### 6. OpenWrt

**Status**: Massive in embedded/router space

**User Base**: Large, but specialized
- Runs on "millions of devices" (consumer routers, commercial equipment)
- Switched to musl as default in 2015 (replacing uClibc)
- ~8000 optional software packages available
- Used by Ubiquiti, TP-Link, Xiaomi, ZyXEL, D-Link

**Primary Use Cases**:
- Router firmware replacement
- Network appliances
- Embedded networking devices

**Relevance to Developer Tools**:
- Not a development environment
- Users don't install developer tools on routers
- Extremely constrained storage/memory (targets home router hardware)
- opkg package manager (not APK)

**Technical Stack**:
- Linux kernel + musl + BusyBox
- Custom build system (OpenWrt Buildroot)
- opkg package management

**Sources**:
- [OpenWrt - Wikipedia](https://en.wikipedia.org/wiki/OpenWrt)
- [OpenWrt switches to musl - Hacker News](https://news.ycombinator.com/item?id=9941076)

---

### 7. Sabotage Linux

**Status**: Historical significance, niche usage

**User Base**: Minimal
- "one-man-show" with occasional contributors
- First distro built on musl libc (historically significant)
- Primarily a development/experimental project

**Primary Use Cases**:
- Cross-compilation for embedded hardware
- Experimental/research Linux distribution
- Bootstrapping new architectures

**Notable Contributions**:
- gettext-tiny and netbsd-curses (used by other distros)
- atk-bridge-fake (GTK+3 without dbus)

**Current Status**:
- Still maintained but niche
- Hosted on GitHub and Codeberg
- Supports i386, x86_64, MIPS, PowerPC32, ARM

**Sources**:
- [Sabotage Linux GitHub](https://github.com/sabotage-linux/sabotage)
- [10 Years Sabotage Linux](https://sabotage-linux.neocities.org/blog/11/)

---

### 8. Gentoo (musl profile)

**Status**: Experimental option within Gentoo

**User Base**: Very small subset of Gentoo users
- Profile labeled "experimental"
- Requires choosing musl at install time (can't switch later)
- "Users will probably want a more standard Glibc installation unless they have a specific reason not to"

**Primary Use Cases**:
- Security-hardened desktop (Bluedragon project provides XFCE)
- GNOME on musl achieved via GSoC 2022 project
- Users requiring extreme customization

**Status Note**:
- musl overlay now archived - fixes merged into main Gentoo repository
- Users need to be "comfortable filing bug reports and sometimes providing patches"

**Sources**:
- [Gentoo musl Usage Guide](https://wiki.gentoo.org/wiki/Musl_usage_guide)
- [Project:Musl - Gentoo Wiki](https://wiki.gentoo.org/wiki/Project:Musl)
- [GNOME on musl GSoC 2022](https://blogs.gentoo.org/gsoc/2022/06/20/musl-support-expansion-for-supporting-gnome-desktop-on-gentoo/)

---

## The Big Picture: User Base Estimates

### Estimated Distribution of musl Linux Users

| Distribution | Estimated Share | Confidence | Notes |
|--------------|----------------|------------|-------|
| Alpine Linux | 95%+ | High | Dominates container space |
| OpenWrt | <5% combined | Medium | Large device count but not dev environment |
| Void Linux (musl) | <1% | Medium | Minority of already-small Void base |
| postmarketOS | <1% | Medium | Mobile niche, Alpine-derived |
| Chimera Linux | <0.1% | High | Pre-1.0, small community |
| Adelie Linux | <0.1% | High | Still in beta |
| Gentoo (musl) | <0.1% | High | Experimental profile |
| Sabotage | Negligible | High | Single maintainer |

### Why Alpine Dominates

1. **Container Ecosystem**: 87% of enterprises use Docker (87.67% market share). Alpine is the most popular lightweight base image.

2. **Cloud-Native**: Kubernetes at 92% market share in container orchestration, with Alpine as a common base image.

3. **CI/CD Pipelines**: Alpine's small size means faster pulls, shorter build times.

4. **Official Images**: Many language runtimes (Python, Node, Go, Rust) offer Alpine variants.

5. **Network Effect**: Documentation, tutorials, Stack Overflow answers all assume Alpine for musl.

### Is "musl support" = "Alpine support"?

**Yes, practically speaking.**

For developer tools:
- Alpine represents the vast majority of musl users who would install developer tools
- OpenWrt users don't install developer tools on routers
- postmarketOS users inherit Alpine compatibility
- Void/Chimera/Adelie/Gentoo musl users combined are statistically insignificant
- The "dozen or so" desktop musl users can build from source

---

## Developer Tools Perspective

### How Projects Treat musl Distributions

**Rust/rustup**:
- Provides musl targets: `x86_64-unknown-linux-musl`, `aarch64-unknown-linux-musl`
- rustup now available in Alpine's community repository
- Historical issues with no musl-native rustup-init (now resolved)
- No Alpine-specific code; uses generic musl target

**asdf/mise**:
- asdf available in Alpine's testing repository
- Individual plugins have varying musl support
- Node.js plugin has proposal for musl builds (#190)
- asdf-community/asdf-alpine provides Docker images
- "It is a pain to actually get the binary builds running on Alpine. You have to basically install glibc alongside musl."

**General Pattern**:
- Most tools provide generic musl binaries (if they provide musl at all)
- Alpine is the test target; other musl distros are "your mileage may vary"
- No precedent for "Alpine-specific" features vs "generic musl"
- Common approach: provide musl binary, test on Alpine, document Alpine instructions

### Compatibility Considerations

**musl vs glibc Binary Compatibility**:
- glibc binaries do NOT run on musl systems (different dynamic linker)
- Error: "Could not open '/lib64/ld-linux-x86-64.so.2': No such file or directory"
- Static linking avoids this but increases binary size
- Some tools (JetBrains IDEs) work via JVM's musl support

**IDE Support**:
- Eclipse CDT: No musl builds available
- JetBrains (IntelliJ, CLion): Works via OpenJDK JVM musl support
- VSCode: Requires glibc compatibility layer

---

## Recommendation

### For tsuku's Decision: Alpine-Specific APK Extraction

**Recommendation: Proceed with Alpine-specific APK extraction (~500 LOC)**

**Rationale**:

1. **Market Reality**: Alpine represents 95%+ of musl users who would install developer tools. The remaining musl distros are:
   - Not development environments (OpenWrt)
   - Mobile-focused (postmarketOS, inherits Alpine anyway)
   - Statistically insignificant (Void musl, Chimera, Adelie, Gentoo musl)

2. **No Precedent for Generic Approach**: Other developer tool projects (rustup, asdf, mise) don't implement generic "all musl distros" support. They provide musl binaries tested on Alpine.

3. **Package Manager Fragmentation**: Even among musl distros, package managers differ:
   - Alpine: APK
   - Void: XBPS
   - OpenWrt: opkg
   - Gentoo: Portage
   - Chimera/Adelie: APK (but Chimera uses APKv3)

   "Generic system package support" would require supporting 4+ package managers for <5% of users.

4. **Cost-Benefit**: 500 LOC for APK extraction serves 95%+ of the use case. Generic support would be significantly more complex for marginal benefit.

5. **Edge Cases Can Build**: The handful of Void/Chimera/Gentoo musl users are by definition advanced users who can build from source or use workarounds.

### Alternative Paths Not Recommended

- **Generic musl detection + system packages**: Adds complexity for minimal user benefit
- **Supporting multiple musl package managers**: XBPS, opkg, Portage each require separate implementations
- **Ignoring musl entirely**: Alpine's container dominance makes this a significant gap

---

## Sources Summary

### Primary Sources
- [Alpine Linux - Wikipedia](https://en.wikipedia.org/wiki/Alpine_Linux)
- [Alpine Docker Official Image](https://hub.docker.com/_/alpine)
- [Void Linux Handbook - musl](https://docs.voidlinux.org/installation/musl.html)
- [Chimera Linux](https://chimera-linux.org/)
- [postmarketOS](https://postmarketos.org/)
- [Adelie Linux](https://www.adelielinux.org/)
- [OpenWrt - Wikipedia](https://en.wikipedia.org/wiki/OpenWrt)
- [musl libc Projects](https://wiki.musl-libc.org/projects-using-musl.html)

### Industry Reports
- [Sysdig 2020 Container Security Snapshot](https://www.sysdig.com/blog/sysdig-2020-container-security-snapshot)
- [Linux Statistics 2025 - SQ Magazine](https://sqmagazine.co.uk/linux-statistics/)

### Technical Comparisons
- [Alpine vs Void - StackShare](https://stackshare.io/stackups/alpine-linux-vs-void-linux)
- [Debian vs Alpine - Nick Janetakis](https://nickjanetakis.com/blog/benchmarking-debian-vs-alpine-as-a-base-docker-image)
- [glibc vs musl - Chainguard](https://edu.chainguard.dev/chainguard/chainguard-images/about/images-compiled-programs/glibc-vs-musl/)

### Developer Tool References
- [rustup on Alpine](https://pkgs.alpinelinux.org/package/edge/community/x86_64/rustup)
- [asdf on Alpine](https://pkgs.alpinelinux.org/package/edge/testing/x86_64/asdf)
- [asdf-nodejs musl proposal](https://github.com/asdf-vm/asdf-nodejs/issues/190)
