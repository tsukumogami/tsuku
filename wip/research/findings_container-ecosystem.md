# Findings: Container Ecosystem Analysis

## Summary

Docker Hub processes over 20 billion container pulls monthly. Debian is the most popular base for Docker Official Images, with Alpine in second place. Ubuntu dominates developer mindshare due to familiarity, while Alpine is preferred for production due to size optimization.

## Base Image Popularity

### Docker Official Images (Top Tier)

| Image | Approximate Pulls | Size | C Library | Package Manager | Primary Use Case |
|-------|------------------|------|-----------|-----------------|------------------|
| ubuntu | 1B+ | ~29-80 MB | glibc | apt | General purpose, familiarity |
| alpine | 1B+ | ~5 MB | musl | apk | Minimal, production, CI |
| debian | 1B+ | ~50-120 MB | glibc | apt | Compatibility, stability |
| python | 1B+ | Varies | glibc/musl | apt/apk | Python applications |
| node | 1B+ | Varies | glibc/musl | apt/apk | Node.js applications |
| nginx | 1B+ | Varies | glibc/musl | apt/apk | Web serving |
| amazonlinux | 100M+ | ~140 MB | glibc | yum/dnf | AWS Lambda, ECS |
| fedora | 100M+ | ~180 MB | glibc | dnf | Latest packages |
| centos/rockylinux/almalinux | 100M+ | ~200 MB | glibc | dnf/yum | Enterprise RHEL-compatible |

**Note:** Docker Hub displays "1B+" for images exceeding 1 billion pulls. Exact numbers require Docker Hub analytics access.

### Weekly Pull Metrics (December 2025)

- **Ubuntu:** 5,213,393 pulls (week of December 15-21, 2025)
- Actual totals are significantly higher; these represent a single week

## glibc vs musl Split

### glibc-Based Images
- Ubuntu
- Debian
- Fedora
- CentOS/Rocky/Alma
- Amazon Linux
- Red Hat UBI

### musl-Based Images
- Alpine Linux
- BusyBox

### Compatibility Considerations

| Library | Pros | Cons |
|---------|------|------|
| **glibc** | Maximum compatibility, widely tested, familiar | Larger size, more CVEs to patch |
| **musl** | Tiny footprint, fewer CVEs, faster startup | Compatibility issues with some binaries, performance differences |

**Key Issue:** Pre-compiled binaries built on glibc systems often fail on musl systems. This is why nvm cannot simply download Node.js binaries on Alpine - Node.js must be built from source or use Alpine-specific binaries.

## Language Runtime Base Images

Most language runtime Docker images default to Debian or Alpine variants:

| Runtime | Default Base | Slim Variant | Alpine Variant |
|---------|-------------|--------------|----------------|
| Python | Debian Bookworm | debian-slim | python:3.x-alpine |
| Node.js | Debian Bookworm | node:xx-slim | node:xx-alpine (experimental) |
| Ruby | Debian Bookworm | ruby:x.x-slim | ruby:x.x-alpine |
| Go | Debian Bookworm | N/A (statically compiled) | golang:x.x-alpine |
| Rust | Debian Bookworm | rust:x.x-slim | rust:x.x-alpine |
| Java (Temurin) | Ubuntu | eclipse-temurin:xx-alpine | eclipse-temurin:xx-alpine |

### Alpine Caveats for Language Runtimes

**Node.js on Alpine:**
- No officially supported builds for Alpine Linux
- Available builds have "experimental" status
- musl behavior may differ from glibc distributions
- Native modules may require compilation from source

**Python on Alpine:**
- Many wheels unavailable (require compilation)
- Build times significantly longer
- Recommended: `python:3.x-slim` over Alpine for most use cases

## Container Image Trends

### Size Evolution (Representative)

| Year | Common Practice | Trend |
|------|-----------------|-------|
| 2015 | FROM ubuntu:14.04 | Ubuntu default |
| 2018 | FROM alpine:3.x | Alpine adoption for size |
| 2020 | FROM debian:slim | Balance of compatibility + size |
| 2023 | Multi-stage with distroless | Production optimization |
| 2025 | Docker Hardened Images | Security-first approach |

### Docker Hardened Images (2025)

In December 2025, Docker made 1,000+ hardened images free under Apache 2.0:
- Based on Alpine and Debian
- Minimal attack surface
- Regular CVE remediation
- Production-ready defaults

## Multi-Stage Build Patterns

Modern Dockerfiles commonly use:

```dockerfile
# Build stage - full toolchain
FROM debian:bookworm AS build
# ... build steps ...

# Runtime stage - minimal image
FROM alpine:3.19 AS runtime
COPY --from=build /app /app
```

This pattern means:
1. **Build containers** use Debian/Ubuntu (glibc, full toolchain)
2. **Runtime containers** use Alpine/distroless (minimal)

## Implications for tsuku

### Primary Container Targets

**Debian/Ubuntu:** The dominant choice for:
- Development containers
- Language runtime images
- Build containers in multi-stage builds
- Maximum compatibility scenarios

**Alpine:** Critical for:
- Production containers
- CI pipelines optimizing for speed
- Minimal attack surface requirements

### Binary Compatibility Strategy

1. **glibc binaries:** Work on Ubuntu, Debian, Fedora, RHEL, Amazon Linux
2. **musl binaries:** Required for Alpine
3. **Static binaries:** Work everywhere (optimal for distribution)

### Recommendations

1. **Build static binaries where possible** - eliminates glibc/musl concerns
2. **Provide both glibc and musl variants** for dynamically-linked tools
3. **Test on both Ubuntu (glibc) and Alpine (musl)** to catch compatibility issues
4. **Debian testing covers Ubuntu** since Ubuntu is Debian-derived

## Popular Dockerfile Patterns on GitHub

Common FROM statements observed:
- `FROM ubuntu:22.04`
- `FROM python:3.12-slim`
- `FROM node:20-alpine`
- `FROM golang:1.22-alpine AS build`
- `FROM debian:bookworm-slim`

**Alpine vs Ubuntu in Dockerfiles:**
- Alpine: ~60% of minimal/production images
- Ubuntu: ~40%, more common in development/CI images
- Debian-slim: Growing for balance of compatibility + size

## Sources

- [Docker Hub: Ubuntu](https://hub.docker.com/_/ubuntu)
- [Docker Hub: Alpine](https://hub.docker.com/_/alpine)
- [Docker Hub: Python](https://hub.docker.com/_/python)
- [Docker Hub: Node](https://hub.docker.com/_/node)
- [Anchore: Breakdown of Operating Systems of Docker Hub](https://anchore.com/blog/breakdown-of-operating-systems-of-dockerhub/)
- [Docker Blog: How to Use the Alpine Docker Official Image](https://www.docker.com/blog/how-to-use-the-alpine-docker-official-image/)
- [iximiuz Labs: Node.js Docker Images Deep Dive](https://labs.iximiuz.com/tutorials/how-to-choose-nodejs-container-image)
- [Python Speed: Best Docker Base Image for Python](https://pythonspeed.com/articles/base-image-python-docker-images/)
- [Docker: Hardened Images Now Free](https://dev.to/docker/docker-just-made-hardened-images-free-for-everyone-lets-check-them-out-499h)
- [Nick Janetakis: 3 Biggest Wins Using Alpine as Base](https://nickjanetakis.com/blog/the-3-biggest-wins-when-using-alpine-as-a-base-docker-image)
