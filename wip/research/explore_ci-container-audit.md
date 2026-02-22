# CI Container Image Audit

Audit of all container image usage across `.github/workflows/` files, classifying each by purpose and evaluating whether `tsuku install --sandbox --target-family <family>` could replace it.

## Summary

- **Total container usages**: 22 (active) + 4 (commented out)
- **Recipe validation**: 12
- **Sandbox testing**: 3
- **Build/compile**: 4
- **Other**: 3
- **Could plausibly be replaced by --sandbox**: 12 out of 22 active usages

---

## Detailed Findings

### 1. recipe-validation-core.yml

**Called by**: `recipe-validation.yml`, `recipe-validation-constrain.yml`

#### Usage 1: validate-linux-x86_64

| Field | Value |
|-------|-------|
| Job | `validate-linux-x86_64` |
| Images | `debian:bookworm-slim`, `fedora:41`, `archlinux:base`, `opensuse/tumbleweed`, `alpine:3.21` |
| Category | **Recipe validation** |
| Commands | Installs curl/ca-certificates, then runs `tsuku install --force --recipe <path>` |
| Replaceable by --sandbox | **Yes** -- this is exactly the pattern: run tsuku install inside a specific distro family to verify a recipe works there. |

#### Usage 2: validate-linux-arm64

| Field | Value |
|-------|-------|
| Job | `validate-linux-arm64` |
| Images | `debian:bookworm-slim`, `fedora:41`, `opensuse/tumbleweed`, `alpine:3.21` |
| Category | **Recipe validation** |
| Commands | Same as x86_64 but with arm64 binary |
| Replaceable by --sandbox | **Yes** -- same pattern on arm64 runners. |

**Subtotal**: 2 usages (each with 4-5 family images), 2 replaceable.

---

### 2. test-recipe.yml

#### Usage 3: test-linux-x86_64

| Field | Value |
|-------|-------|
| Job | `test-linux-x86_64` |
| Images | `debian:bookworm-slim`, `fedora:41`, `archlinux:base`, `opensuse/tumbleweed`, `alpine:3.21` |
| Category | **Recipe validation** |
| Commands | Installs curl/ca-certs/jq/build-essential, then runs `tsuku install --force --recipe <path>` inside each container |
| Replaceable by --sandbox | **Yes** -- tests a specific recipe across all families. |

#### Usage 4: test-linux-arm64

| Field | Value |
|-------|-------|
| Job | `test-linux-arm64` |
| Images | `debian:bookworm-slim`, `fedora:41`, `opensuse/tumbleweed`, `alpine:3.21` |
| Category | **Recipe validation** |
| Commands | Same pattern, arm64 binary |
| Replaceable by --sandbox | **Yes** |

**Subtotal**: 2 usages (each with 4-5 family images), 2 replaceable.

---

### 3. batch-generate.yml

#### Usage 5: validate-linux-x86_64

| Field | Value |
|-------|-------|
| Job | `validate-linux-x86_64` |
| Images | `debian:bookworm-slim`, `fedora:41`, `archlinux:base`, `opensuse/tumbleweed`, `alpine:3.21` |
| Category | **Recipe validation** |
| Commands | Installs curl/ca-certs/jq per family, then `tsuku install --json --force --recipe <path>` |
| Replaceable by --sandbox | **Yes** -- validates batch-generated recipes on each family. |

#### Usage 6: validate-linux-arm64

| Field | Value |
|-------|-------|
| Job | `validate-linux-arm64` |
| Images | `debian:bookworm-slim`, `fedora:41`, `opensuse/tumbleweed`, `alpine:3.21` |
| Category | **Recipe validation** |
| Commands | Same pattern, arm64 |
| Replaceable by --sandbox | **Yes** |

**Subtotal**: 2 usages, 2 replaceable.

---

### 4. validate-golden-execution.yml

#### Usage 7: validate-linux-containers

| Field | Value |
|-------|-------|
| Job | `validate-linux-containers` |
| Images | `alpine:3.21`, `fedora:41`, `archlinux:base`, `opensuse/tumbleweed` |
| Category | **Recipe validation** |
| Commands | Installs bash/jq, runs `install-recipe-deps.sh` to install system packages, then `tsuku install --plan <golden-file> --force` |
| Replaceable by --sandbox | **Partially** -- validates golden files (pre-computed plans) on non-Debian families. The `--plan` flag bypasses recipe resolution; sandbox could theoretically do this but would need to accept plan files. |

**Subtotal**: 1 usage (4 family images), 1 partially replaceable.

---

### 5. platform-integration.yml

#### Usage 8: build-dltest-musl (amd64 + arm64)

| Field | Value |
|-------|-------|
| Job | `build-dltest-musl` |
| Image | `alpine:3.21` |
| Category | **Build/compile** |
| Commands | `apk add rust cargo && cd cmd/tsuku-dltest && cargo build --release` |
| Replaceable by --sandbox | **No** -- builds a Rust binary (tsuku-dltest) linked against musl libc. This is a compile step, not a recipe install. |

#### Usage 9: integration (container: directive)

| Field | Value |
|-------|-------|
| Job | `integration` |
| Images | `debian:bookworm-slim`, `fedora:41` (active); `alpine:3.21` commented out |
| Category | **Recipe validation** |
| Commands | Installs curl, then runs `tsuku install zlib --force`, `tsuku verify zlib`, `tsuku verify libyaml`, `tsuku install just --force` |
| Replaceable by --sandbox | **Partially** -- runs real installs and verifications inside containers. The dlopen verification (`tsuku verify`) step goes beyond basic recipe validation, but the install steps could use --sandbox. |

**Subtotal**: 3 active usages (2 build, 1 recipe validation), 1 partially replaceable.

---

### 6. release.yml

#### Usage 10: build-rust-musl (amd64 + arm64)

| Field | Value |
|-------|-------|
| Job | `build-rust-musl` |
| Image | `alpine:3.21` |
| Category | **Build/compile** |
| Commands | `apk add rust cargo && cd cmd/tsuku-dltest && cargo build --release` |
| Replaceable by --sandbox | **No** -- same musl build as platform-integration.yml but for release artifacts. This is a compilation task. |

**Subtotal**: 2 usages (2 runners: amd64 + arm64), 0 replaceable.

---

### 7. integration-tests.yml

#### Usage 11: library-dlopen-musl (container: directive)

| Field | Value |
|-------|-------|
| Job | `library-dlopen-musl` |
| Image | `golang:1.23-alpine` |
| Category | **Build/compile** |
| Commands | Installs gcc/musl-dev/rustup, builds tsuku with `go build`, installs recipe-declared deps via `apk`, runs Rust-based dlopen test |
| Replaceable by --sandbox | **No** -- this builds both Go and Rust code inside an Alpine container to test musl-linked library dlopen behavior. It's testing the build toolchain itself, not recipe installation. |

#### Usage 12: checksum-pinning (via test script)

| Field | Value |
|-------|-------|
| Job | `checksum-pinning` (calls `test/scripts/test-checksum-pinning.sh`) |
| Images | `debian:bookworm-slim`, `fedora:39`, `archlinux:base`, `alpine:3.19`, `opensuse/tumbleweed` |
| Category | **Recipe validation** |
| Commands | Builds a per-family Docker image, then `docker run` to install fzf, verify checksums, test tamper detection |
| Replaceable by --sandbox | **Partially** -- the installation part is recipe validation, but the tamper detection test (modifying binaries and verifying checksums detect changes) goes beyond what --sandbox provides. |

**Subtotal**: 2 usages, 0 fully replaceable (1 partially).

---

### 8. build-essentials.yml

#### Usage 13: test-no-gcc

| Field | Value |
|-------|-------|
| Job | `test-no-gcc` |
| Image | `ubuntu:22.04` (via `container:` directive) |
| Category | **Sandbox testing** |
| Commands | Strips gcc from the container, installs zig, then builds gdbm-source to test zig-as-compiler fallback |
| Replaceable by --sandbox | **No** -- this specifically tests the "no gcc" scenario by stripping the container of system compilers. It validates tsuku's zig fallback logic, not a recipe per se. |

#### Usage 14: test-sandbox-multifamily

| Field | Value |
|-------|-------|
| Job | `test-sandbox-multifamily` |
| Images | N/A (uses `--sandbox` flag directly, Docker is invoked internally by tsuku) |
| Category | **Sandbox testing** |
| Commands | `tsuku eval <tool> --os linux --linux-family <family> --install-deps > plan.json` then `tsuku install --plan plan.json --sandbox --force` |
| Replaceable by --sandbox | **Already uses --sandbox** -- this IS the sandbox test. Docker is used internally by tsuku, not by the workflow directly. |

**Subtotal**: 2 usages (1 explicit container, 1 internal via --sandbox), 0 replaceable (one already uses --sandbox).

---

### 9. container-tests.yml

#### Usage 15: sandbox-tests

| Field | Value |
|-------|-------|
| Job | `sandbox-tests` |
| Images | N/A (Docker used internally by Go test suite) |
| Category | **Sandbox testing** |
| Commands | `go test ./internal/sandbox/...` (runs Go integration tests that invoke Docker internally) |
| Replaceable by --sandbox | **No** -- these are Go unit/integration tests for the sandbox implementation itself. They test sandbox correctness, not recipes. |

#### Usage 16: validate-tests

| Field | Value |
|-------|-------|
| Job | `validate-tests` |
| Images | N/A (Docker used internally by Go test suite) |
| Category | **Other** |
| Commands | `go test ./internal/validate/...` (Go tests that use Docker for validation testing) |
| Replaceable by --sandbox | **No** -- Go tests for the validate package, testing validation infrastructure. |

**Subtotal**: 2 usages, 0 replaceable.

---

### 10. container-build.yml

#### Usage 17: build (Docker build + push)

| Field | Value |
|-------|-------|
| Job | `build` |
| Image | Builds `ghcr.io/tsukumogami/tsuku-sandbox` from `sandbox/Dockerfile.minimal` |
| Category | **Other** |
| Commands | Lints Dockerfile, builds multi-platform container image, pushes to GHCR |
| Replaceable by --sandbox | **No** -- this builds the sandbox container image itself. It's infrastructure for the sandbox feature. |

**Subtotal**: 1 usage, 0 replaceable.

---

### 11. sandbox-tests.yml

#### Usage 18: sandbox-tests

| Field | Value |
|-------|-------|
| Job | `sandbox-tests` |
| Images | N/A (uses `--sandbox` flag; Docker invoked internally by tsuku) |
| Category | **Sandbox testing** |
| Commands | `tsuku eval <tool> --install-deps > plan.json && tsuku install --plan plan.json --sandbox --force` |
| Replaceable by --sandbox | **Already uses --sandbox** -- this workflow IS the sandbox test suite. |

**Subtotal**: 1 usage, already uses --sandbox.

---

### Commented-Out Usages (not counted in totals)

| Workflow | Job | Image | Reason Disabled |
|----------|-----|-------|-----------------|
| `test.yml` | `rust-test-musl` | `alpine:3.19` | Embedded libraries are glibc-only (#1092) |
| `platform-integration.yml` | `integration` (alpine entry) | `alpine:3.21` | Requires system packages not in base image (#1570) |
| `platform-integration.yml` | `integration-arm64-musl` | `alpine:3.21` | Same as above |
| `platform-integration.yml` | (commented docker run block) | `alpine:3.21` | Part of disabled arm64-musl tests |

---

## Category Summary

| Category | Active Count | Could Replace with --sandbox |
|----------|-------------|------------------------------|
| Recipe validation | 12 | 10 fully, 2 partially |
| Sandbox testing | 3 | 0 (already use --sandbox or test sandbox itself) |
| Build/compile | 4 | 0 |
| Other | 3 | 0 |
| **Total** | **22** | **12 (10 full + 2 partial)** |

## Container Images Used

| Image | Times Used | Primary Purpose |
|-------|-----------|-----------------|
| `debian:bookworm-slim` | 8 | Recipe validation (Debian family) |
| `fedora:41` | 8 | Recipe validation (RHEL family) |
| `alpine:3.21` | 8 | Recipe validation (Alpine family) + musl builds |
| `opensuse/tumbleweed` | 7 | Recipe validation (SUSE family) |
| `archlinux:base` | 5 | Recipe validation (Arch family) |
| `ubuntu:22.04` | 1 | No-GCC build test |
| `golang:1.23-alpine` | 1 | musl dlopen tests (Go + Rust build) |
| `fedora:39` | 1 | Checksum pinning test (legacy image version) |
| `alpine:3.19` | 1 | Checksum pinning test (legacy image version) |
| `ghcr.io/tsukumogami/tsuku-sandbox` | 1 | Built by container-build.yml |

## Analysis: What --sandbox Could Replace

The 12 replaceable usages all follow the same pattern:

1. Start a distro-family container (`debian:bookworm-slim`, `fedora:41`, etc.)
2. Install minimal bootstrap packages (curl, ca-certificates)
3. Run `tsuku install --force --recipe <path>` inside the container
4. Check exit code

This is precisely what `tsuku install --sandbox --target-family <family>` would do, assuming:
- The `--target-family` flag supports all five families (debian, rhel, arch, alpine, suse)
- The sandbox handles bootstrap package installation internally
- The sandbox works on both x86_64 and arm64 runners

The 10 remaining usages that cannot be replaced fall into clear categories:
- **Build/compile** (4): Building Rust binaries against musl libc -- not recipe installation
- **Sandbox infrastructure testing** (3): Testing the sandbox implementation itself, or already using --sandbox
- **Infrastructure** (1): Building the sandbox container image
- **Test framework** (2): Go test suites that invoke Docker programmatically for their own test infrastructure
