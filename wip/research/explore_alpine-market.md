# Alpine Linux Market Research

Research conducted: January 2024

## Executive Summary

Alpine Linux occupies a unique position in the Linux ecosystem as a lightweight, security-focused distribution that dominates the container market while remaining largely invisible in desktop usage metrics. Approximately **20% of all Docker containers use Alpine** as their base image, and the distribution powers "hundreds of millions of running containers" globally. This makes Alpine a **first-class citizen in the developer tools ecosystem**, particularly for containerized environments.

For a developer tool package manager like tsuku, Alpine support is strategically important given the prevalence of Alpine-based CI/CD pipelines, container images, and developer environments.

---

## 1. Docker Hub Market Share

### Alpine vs Debian/Ubuntu Comparison

| Base Image | Approximate Size | Notes |
|------------|------------------|-------|
| Alpine | ~5 MB | Smallest option with package manager |
| Ubuntu minimal | ~29 MB | Expands to 65-80 MB for full variants |
| Ubuntu full | ~188 MB | "Most downloaded OS image" but also largest |
| Debian | ~75 MB | Most extensive package repository (59,000+) |
| Distroless static | ~2.5 MB | No shell, no package manager |

**Key statistics:**
- Approximately **20% of all Docker containers are based on Alpine Linux** ([Coin.host](https://coin.host/blog/the-advantages-of-using-alpine-linux-in-docker-images))
- Over **100 million container image downloads per month**, with "a significant portion" using Alpine as base ([Coin.host](https://coin.host/blog/the-advantages-of-using-alpine-linux-in-docker-images))
- Alpine is roughly **30x smaller than Debian** ([Nick Janetakis](https://nickjanetakis.com/blog/the-3-biggest-wins-when-using-alpine-as-a-base-docker-image))
- Many Docker Official Images offer Alpine variants, typically "even smaller than slim variants" ([Docker Hub](https://hub.docker.com/_/alpine))

### Official Image Support

Most major Docker official images provide Alpine variants:
- Node.js: `node:20-alpine` (181 MB vs ~900 MB full)
- Python: `python:3.x-alpine`
- Go: `golang:alpine`
- Rust: Alpine-based musl builds available
- Ruby, PHP, and most other languages

---

## 2. Adoption Trends (2021-2025)

### Growth Indicators

- Alpine Linux "has surged in popularity within the Docker ecosystem" ([TurnKey Linux](https://www.turnkeylinux.org/blog/alpine-vs-debian))
- Twitter polls showed Alpine "usage is going through the roof" ([Nick Janetakis](https://nickjanetakis.com/blog/benchmarking-debian-vs-alpine-as-a-base-docker-image))
- Kubernetes production deployment reached **80% in 2024** (up from 66% in 2023), increasing demand for lightweight container images ([CNCF Annual Survey 2024](https://www.cncf.io/reports/cncf-annual-survey-2024/))
- **93% of organizations** now use, pilot, or evaluate Kubernetes ([CNCF](https://www.cncf.io/reports/cncf-annual-survey-2024/))

### Competing Trends

Some movement away from Alpine due to musl compatibility issues:
- **~25% of respondents** in polls who started with Alpine later moved away ([iximiuz](https://iximiuz.com/en/posts/containers-making-images-better/))
- Primary reasons: DNS issues, Node.js native extension problems, glibc compatibility
- Some teams "switched to Debian-based images because of intermittent DNS-related woes"

**Net assessment:** Alpine remains dominant for containers, though some workloads migrate to distroless or Debian for compatibility. The 20% market share appears stable.

---

## 3. CI/CD Environment Usage

### GitHub Actions

- **No official Alpine support** for self-hosted runners ([GitHub issue #801](https://github.com/actions/runner/issues/801))
- Feature request has 134+ reactions but "not prioritized"
- Workarounds exist:
  - [jirutka/setup-alpine](https://github.com/jirutka/setup-alpine): Chroot-based Alpine environment on Ubuntu runners
  - Container jobs using Alpine images work fully
  - Self-hosted runners on Alpine possible but unsupported

### GitLab CI

- **Official Alpine-based runner image** available: `gitlab/gitlab-runner:alpine`
- Ubuntu variant: ~800 MB, Alpine variant: ~460 MB ([GitLab Docs](https://docs.gitlab.com/runner/install/docker/))
- Alpine is explicitly supported as a first-class option

### Container-Based CI Patterns

Most CI systems execute jobs in containers, making Alpine support implicit:
- CircleCI: Alpine images work
- Travis CI: Docker-based builds support Alpine
- Jenkins: Kubernetes executors commonly use Alpine
- Buildkite: Container-native, Alpine-friendly

**Assessment:** Alpine is well-supported in CI/CD through containerization, even where the host runner doesn't natively support it.

---

## 4. Developer Tool Projects and Alpine Support

### Package Managers

| Tool | Alpine/musl Support | Notes |
|------|---------------------|-------|
| **Homebrew** | **Not supported** | Depends on glibc; fails on Alpine ([GitHub #8130](https://github.com/Homebrew/brew/issues/8130)) |
| **mise** | **Native support** | Available via `apk add mise`, musl binaries provided ([mise docs](https://mise.jdx.dev/installing-mise.html)) |
| **asdf** | Partial | Plugins vary; some require compilation |
| **nix** | Works | Nix builds hermetically; works on musl |
| **apk** | Native | Alpine's native package manager |

### Key Findings

- **mise** explicitly provides `linux-x64-musl`, `linux-arm64-musl`, `linux-armv6-musl`, `linux-armv7-musl` binaries
- mise has an `all_compile` setting specifically for "Alpine that does not use glibc"
- mise is in Alpine's community repository: `apk add mise`
- Homebrew explicitly does not support Alpine due to glibc dependency

### Language Runtimes

- **Node.js**: Official musl builds available via unofficial-builds project
- **Go**: Excellent musl support; static binaries work everywhere
- **Rust**: First-class musl target (`x86_64-unknown-linux-musl`)
- **Python**: Works but some C extensions may fail

---

## 5. Why Developers Choose Alpine

### Primary Motivations

1. **Size** (cited most frequently)
   - 5 MB base vs 75-188 MB alternatives
   - Faster CI pipelines (less download time)
   - Lower storage costs at scale
   - Faster container startup

2. **Security**
   - "All userland binaries compiled as Position Independent Executables (PIE) with stack smashing protection" ([Alpine about](https://alpinelinux.org/about/))
   - Minimal attack surface (fewer packages = fewer vulnerabilities)
   - Alpine was immune to ShellShock (no Bash by default)

3. **Simplicity**
   - "Designed with security in mind" from the start
   - Clean, minimal base to build upon
   - Fast boot times

### Common Pain Points

1. **musl vs glibc incompatibility**
   - "The special thing about Alpine is that it uses a different C standard lib - musl - instead of glibc. This essentially means everything has to be recompiled" ([Chainguard](https://edu.chainguard.dev/chainguard/chainguard-images/about/images-compiled-programs/glibc-vs-musl/))
   - Some precompiled binaries fail silently or behave unexpectedly

2. **DNS issues** in containerized environments

3. **Limited packages** compared to Debian/Ubuntu (though still "much more complete than other BusyBox based images")

---

## 6. Comparison with Lightweight Alternatives

### Distroless

| Aspect | Alpine | Distroless |
|--------|--------|------------|
| Size | 5 MB (with apk) | 2-30 MB (no package manager) |
| Shell | BusyBox shell | None |
| Package manager | apk | None |
| Debugging | Possible in container | Requires debug image variant |
| libc | musl | glibc (Debian-based) |

**Key insight:** Distroless has better glibc compatibility but no runtime package management. Alpine offers a middle ground.

### Scratch

- 0 bytes base (literally empty)
- Only works for fully static binaries
- No runtime dependencies or debugging capability

### BusyBox

- Smaller than Alpine (~1 MB)
- Fewer packages available
- Less complete package repository

---

## 7. Use Case Breakdown

### Container/Microservices (PRIMARY)
- **20% of Docker containers** use Alpine
- Ideal for: Go services, static binaries, minimal runtimes
- Companies use for: Production microservices, API servers

### CI/CD Pipelines
- Fast image pulls critical for pipeline speed
- Many CI systems use Alpine for runner images
- Build caching benefits from smaller base layers

### Embedded/IoT
- Alpine originated as an embedded distribution for routers
- Used in: "routers, gateways, and other IoT devices"
- postmarketOS (mobile Linux) based on Alpine
- "Enterprise routers" commonly use Alpine

### Desktop
- **Not significant** - ranks poorly on DistroWatch
- "While RHEL finds itself in a humble 57th place on DistroWatch rankings, Alpine Linux is powering hundreds of millions of running containers" ([TecMint](https://www.tecmint.com/top-most-popular-linux-distributions/))
- Desktop users face DRM and proprietary software challenges

### Server/Cloud
- AWS Marketplace offers Alpine Linux images
- Used for "mail server containers, embedded switches, PVRs, storage controllers"

---

## 8. Container Orchestration Context

### Kubernetes

- **80% production deployment in 2024** ([CNCF](https://www.cncf.io/reports/cncf-annual-survey-2024/))
- **92% market share** in container orchestration
- **5.6 million developers** actively use Kubernetes
- Alpine images well-suited for pod resource limits
- Fast scaling benefits from small image size

### Docker Compose

- Development environments commonly use Alpine
- Multi-stage builds: "use Ubuntu for building (tool-rich) and Alpine for running (lean and fast)"

---

## 9. Strategic Recommendation

### Is Alpine a First-Class Citizen?

**Yes, for containers and CI/CD.** Alpine is arguably THE first-class citizen:
- 20% of all Docker containers
- Native support in GitLab CI runners
- mise (closest competitor to tsuku) explicitly supports Alpine
- Go/Rust ecosystems treat musl as first-class target
- Kubernetes ecosystem implicitly depends on Alpine's prevalence

**No, for desktop/general use.** This is irrelevant for tsuku's use case.

### Recommendation for tsuku

**Supporting Alpine is strategically important.** Consider:

1. **User base:** 20% container market share means substantial potential users
2. **Competitive landscape:** mise provides native Alpine support
3. **CI/CD alignment:** Many users will run tsuku in Alpine CI containers
4. **Technical feasibility:** Custom APK extraction (~500 LOC) is reasonable

### Trade-offs

**Custom APK extraction pros:**
- Works in environments without `apk` installed (docker multi-stage builds)
- Hermetic installation without system package manager
- Consistent with tsuku's philosophy of self-contained installation

**Just using `apk install` pros:**
- Zero maintenance burden
- Leverages system package manager
- Simpler implementation

**Recommendation:** If tsuku aims to be a comprehensive developer tool manager that works seamlessly in containers (including minimal/distroless-style images), custom APK extraction is worth the investment. If tsuku only targets environments where users have system package manager access, `apk install` suffices.

---

## Sources

### Market Statistics
- [Coin.host: Why Alpine Linux Dominates Docker Landscapes](https://coin.host/blog/the-advantages-of-using-alpine-linux-in-docker-images)
- [CNCF Annual Survey 2024](https://www.cncf.io/reports/cncf-annual-survey-2024/)
- [Command Linux: Container & Kubernetes Statistics](https://commandlinux.com/statistics/linux-container-kubernetes-adoption-statistics/)
- [Octopus: Kubernetes Statistics 2025](https://octopus.com/devops/ci-cd-kubernetes/kubernetes-statistics/)

### Technical Comparisons
- [Nick Janetakis: Benchmarking Debian vs Alpine](https://nickjanetakis.com/blog/benchmarking-debian-vs-alpine-as-a-base-docker-image)
- [TurnKey Linux: Alpine vs Debian](https://www.turnkeylinux.org/blog/alpine-vs-debian)
- [iximiuz: In Pursuit of Better Container Images](https://iximiuz.com/en/posts/containers-making-images-better/)
- [Chainguard: glibc vs musl](https://edu.chainguard.dev/chainguard/chainguard-images/about/images-compiled-programs/glibc-vs-musl/)

### CI/CD
- [GitHub Actions Runner Alpine Issue #801](https://github.com/actions/runner/issues/801)
- [GitLab Runner Documentation](https://docs.gitlab.com/runner/install/docker/)
- [setup-alpine GitHub Action](https://github.com/jirutka/setup-alpine)

### Developer Tools
- [mise Installation](https://mise.jdx.dev/installing-mise.html)
- [Homebrew Alpine Issue #8130](https://github.com/Homebrew/brew/issues/8130)
- [asdf-nodejs musl proposal](https://github.com/asdf-vm/asdf-nodejs/issues/190)

### Static Binary Compilation
- [rust-musl-builder](https://github.com/emk/rust-musl-builder)
- [muslrust](https://github.com/clux/muslrust)
- [Homebrew musl-cross](https://github.com/FiloSottile/homebrew-musl-cross)

### General Alpine Information
- [Alpine Linux Official](https://alpinelinux.org/about/)
- [Docker Hub Alpine](https://hub.docker.com/_/alpine)
- [Alpine Linux Wikipedia](https://en.wikipedia.org/wiki/Alpine_Linux)
