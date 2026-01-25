# Hermetic Package Extraction vs System Packages on Alpine Linux

**Date:** 2026-01-24
**Research Question:** Is hermetic APK extraction worth ~500 LOC on Alpine specifically?

---

## Executive Summary

**Recommendation: No - hermetic APK extraction is not worth the complexity on Alpine.**

The concrete value proposition of hermetic package extraction breaks down when examined against Alpine's specific characteristics:

1. **Alpine doesn't retain old package versions** - pinning to specific versions fails within days/weeks
2. **Library isolation is rarely needed** - tools typically require the same library APIs (not conflicting versions)
3. **APK extraction only works on Alpine** - provides no cross-musl-distro benefit
4. **System packages solve the actual problem** - getting libraries that work on musl

The 500 LOC investment would provide:
- Hermetic version control that breaks anyway when Alpine removes old packages
- Alpine-only benefit with no portability to other musl distros
- Additional maintenance burden for questionable reproducibility gains

---

## 1. Version Pinning with apk

### 1.1 Can `apk add` Pin to Specific Versions?

**Technically yes, practically no.**

Alpine supports version pinning syntax:
```bash
apk add zlib=1.2.13-r0
```

However, this fails within days or weeks because **Alpine does not retain old package versions**.

### 1.2 Alpine's Package Retention Policy

Alpine explicitly does not keep old packages:

> "Unfortunately Alpine Linux does not keep old packages. Alpine doesn't have resources to store all built packages indefinitely in their infrastructure. They currently keep only the latest for each stable branch, and has always been like that."
> - [Alpine Forums](https://dev.alpinelinux.org/~clandmeter/other/forum.alpinelinux.org/forum/general-discussion/it-not-possible-install-old-version-packages.html)

This is documented as a known limitation:

> "The Alpine package repository drops packages by design. On 2020 March 10th, gcc 9.2.0-r3 was found on the Alpine package repository under branch 3.11. On 2020 March 23rd, just 13 days later, a Dockerfile failed to run because the package gcc 9.2.0-r3 had been revoked from branch 3.11."
> - [GitLab Issue #9996](https://gitlab.alpinelinux.org/alpine/abuild/-/issues/9996)

### 1.3 Documented Pain Points

Docker users have extensively documented this problem:

> "Since docker build with an exact pinning (e.g. git=2.13.5-r0) fails after a new git version is published to the configured repo (remember, old versions are deleted), this requires updating the Dockerfile every time a new version is published."
> - [Medium: The Problem with Docker and Alpine's Package Pinning](https://medium.stschindler.io/the-problem-with-docker-and-alpines-package-pinning-18346593e891)

Common errors:
```
ERROR: unsatisfiable constraints: postgresql-dev-10.3-r0: breaks: world[postgresql-dev=10.2-r0]
```

**Implication for tsuku:** Even if we extract APK packages with pinned versions, users trying to reproduce an installation 6 months later would face the same problem - the specific APK version may no longer exist on Alpine's CDN.

### 1.4 Comparison with Debian/Fedora

| Distro | Keeps Old Versions | Snapshot Service |
|--------|-------------------|------------------|
| Alpine | No | No |
| Debian | No | Yes (snapshot.debian.org) |
| Ubuntu | No | Yes (snapshot.ubuntu.com) |
| Fedora | Yes (koji archive) | Partial |

Alpine lacks the snapshot infrastructure that enables reproducible builds on Debian.

---

## 2. Reproducibility Challenges

### 2.1 What Happens 6 Months Later?

If a developer needs the same library version 6 months after initial installation:

**With system packages (`apk add`):**
- Likely fails if the exact version was removed
- Workaround: pin to Alpine release version (e.g., `alpine:3.18`) which provides version stability within that release

**With hermetic APK extraction:**
- Same failure mode - the source APK no longer exists on Alpine CDN
- Would need to cache APK files locally (tsuku doesn't do this)
- APKINDEX changes over time, breaking checksum verification

**Neither approach provides true reproducibility on Alpine without additional infrastructure.**

### 2.2 How Alpine Users Handle This Today

Real-world workarounds from the community:

1. **Repository pinning over version pinning:**
   ```dockerfile
   # Instead of:
   RUN apk add jq=1.6-r0  # Fails eventually

   # Use:
   RUN apk add --no-cache --repository=http://dl-cdn.alpinelinux.org/alpine/v3.18/main jq=~1.6
   ```

2. **Accepting "latest within release":**
   ```dockerfile
   FROM alpine:3.18
   RUN apk add --no-cache jq  # Gets whatever 3.18 provides
   ```

3. **Building from source:** Clone aports, switch to desired branch, build with `abuild`

4. **Using Nix on Alpine:** Some users install Nix package manager on Alpine for true reproducibility

> "In the Alpine ecosystem, it is generally not advised to pin versions of packages at all. This is not so much a recommendation as it is a statement of impossibility."
> - [Hacker News Discussion](https://news.ycombinator.com/item?id=39241696)

### 2.3 Nix/Guix on Alpine

Users who need hermetic builds on Alpine often turn to Nix:

> "Mixing package managers doesn't cause anomalies - that's part of the beauty of Nix. Pretty much everything it does lives in /nix and is softlinked into place, meaning it never interferes with your existing package manager."
> - [Nix on Other Distros](https://voidcruiser.nl/rambles/nix-on-other-distros-packagemanagers/)

Nix is available in Alpine's edge/community repository, though setup requires manual steps (Alpine uses OpenRC, not systemd).

**Implication:** Users who truly need hermetic builds on Alpine already have Nix. tsuku trying to replicate this would be competing with a mature solution.

---

## 3. Library Version Isolation

### 3.1 Can Two Tools Require Different Library Versions?

**Theoretically yes, practically rare for tsuku's libraries.**

OpenSSL version conflicts ARE a real problem on Alpine:

> "OpenSSL 3.0 became the default OpenSSL version in Alpine Linux 3.17, but Prisma only supported OpenSSL 1.X at that time. This resulted in errors like: 'Error loading shared library libssl.so.1.1: No such file or directory.'"
> - [Prisma GitHub Issue #16553](https://github.com/prisma/prisma/issues/16553)

However, this is primarily an **application** problem, not a **library dependency** problem:

| Scenario | Example | Real? |
|----------|---------|-------|
| App A needs OpenSSL 1.1, App B needs OpenSSL 3.0 | Prisma + Alpine 3.17+ | Yes |
| Library zlib 1.2 vs zlib 1.3 for different tools | None found | Theoretical |
| libyaml version conflicts | None found | Theoretical |

### 3.2 How This Gets Solved Today

**Containerization:** The dominant solution. Each tool runs in its own container with its own library versions.

**Flatpak's approach:**
> "Flatpak solves software dependency issues by creating isolated environments, known as 'sandboxes,' for each application. If Application A needs Library X version 1.0 and Application B requires Library X version 2.0, Flatpak packages each application with its own required versions."
> - [Flatpak Documentation](https://docs.flatpak.org/en/latest/dependencies.html)

**LD_LIBRARY_PATH manipulation:**
> "Under Alpine Linux, LD_LIBRARY_PATH should accomplish the same as loading the /etc/ld.so.conf.d/nwrfcsdk.conf with ldconfig as you would do in other distros."
> - [SAP Node RFC Issue #148](https://github.com/SAP-archive/node-rfc/issues/148)

### 3.3 Is This a Real Problem for tsuku?

**For tsuku's 4 library dependencies (zlib, openssl, libyaml, gcc-libs): No.**

These are foundational libraries with stable APIs:
- **zlib:** Compression - API hasn't changed substantially in years
- **openssl:** Major version transitions (1.x to 3.x) do cause issues, but tools generally target the system version
- **libyaml:** Parsing - stable API
- **gcc-libs (libstdc++):** ABI is backward compatible within GCC major versions

Tools that depend on these libraries are built against the system's version. The conflict scenario (Tool A needs openssl 1.1, Tool B needs openssl 3.0) typically means:
- Tool A is outdated and should be updated
- Or Tool A should run in a container

tsuku providing isolated library versions wouldn't solve the root cause.

---

## 4. Comparison with Homebrew Value Proposition

### 4.1 Why Does tsuku Use Homebrew Bottles on glibc?

From the codebase research (`DESIGN-platform-compatibility-verification.md`):

**Original assumption (pre-research):**
> "The original assumption was that Homebrew bottles provide: fresher versions than distro packages, hermetic reproducible builds, self-contained installation without system dependencies."

**Research finding:**
> "Homebrew bottles don't provide version freshness - distros often lead. Debian Bookworm has zlib 1.2.13 (same as Homebrew), libyaml 0.2.5 (same)."

The design document concluded that the real value is **"no build tools required"** - users don't need gcc/make. But for library dependencies, this is "unnecessary complexity."

### 4.2 Do Those Reasons Apply to Alpine?

| Homebrew Benefit | Applies to Alpine? | Analysis |
|-----------------|-------------------|----------|
| Fresher versions | No | Alpine edge is often newer |
| No build tools needed | Partial | `apk add` is simpler than building |
| Reproducibility | No | Alpine removes old packages anyway |
| No sudo required | No | APK extraction would need relocation |
| Cross-distro portability | **No** | APK only works on Alpine |

**Key finding from portability research:**
> "APK binaries are not universally portable across musl distros. Chimera Linux uses APKv3 (incompatible with Alpine's APKv2). Void Linux uses xbps. There's no 'musl-universal' binary format."
> - `explore_apk-portability.md`

Homebrew bottles work across glibc distros because:
1. glibc has symbol versioning for backward compatibility
2. Homebrew controls the entire dependency tree
3. RPATHs are patched to point to Homebrew's lib directory

APK packages have none of these properties. They're built for Alpine specifically.

### 4.3 The Real Homebrew Differentiator on glibc

Homebrew bottles work on Debian, Fedora, Arch, and SUSE because glibc provides ABI stability across distributions. A bottle built on Ubuntu works on Fedora.

**This does not exist in the musl world:**
- Alpine uses APKv2
- Chimera uses APKv3
- Void uses xbps
- Each maintains separate repositories

---

## 5. Real-World Failure Scenarios

### 5.1 When Would `apk_install` Fail but Hermetic APK Extraction Succeed?

**Scenario 1: Package version removed**
- `apk add openssl-dev=3.1.4-r0` fails because 3.1.4-r1 is now the only version
- Hermetic extraction... also fails because the APK file is gone from CDN
- **Verdict: Neither approach wins**

**Scenario 2: User lacks sudo**
- `apk add openssl-dev` requires root
- Hermetic extraction to `$TSUKU_HOME` doesn't require root
- **Verdict: Hermetic extraction wins... but only on Alpine**

**Scenario 3: Different Alpine version than expected**
- User on Alpine 3.18, recipe expects 3.19 packages
- `apk add` installs 3.18's version (may differ)
- Hermetic extraction installs 3.19's specific version
- **Verdict: Hermetic extraction provides version consistency... until the 3.19 APK is removed**

### 5.2 Documented Cases of Alpine Package Churn Breaking Tools

**Prisma OpenSSL 3.0 transition (Alpine 3.17):**
Prisma hard-coded openssl 1.x paths. When Alpine moved to 3.0, Prisma broke.

**OpenSSH/Git version mismatch (2025):**
> "OpenSSL version mismatch. Built against 3050003f, you have 30500010"
> - [GitLab Issue #17547](https://gitlab.alpinelinux.org/alpine/aports/-/issues/17547)

**Postfix header mismatch:**
> "run-time library vs. compile-time header version mismatch: OpenSSL 3.2.0 may not be compatible with OpenSSL 3.1.0"
> - [GitLab Issue #15903](https://gitlab.alpinelinux.org/alpine/aports/-/issues/15903)

**Analysis:** These are primarily **Alpine release upgrade** issues, not package churn within a release. Tools built for Alpine 3.17 broke on 3.18 due to OpenSSL major version change.

Hermetic extraction wouldn't prevent this - if a user upgrades Alpine, system OpenSSL changes. The extracted library would conflict with system libraries.

---

## 6. Developer Experience Analysis

### 6.1 UX: "tsuku manages library version" vs "system manages library version"

**System packages (`apk_install`):**
```
$ tsuku install cmake

This recipe requires system dependencies for Alpine:

  sudo apk add openssl-dev

After completing this step, run the install command again.
```

User experience:
- Clear instruction
- Uses familiar package manager
- System handles security updates
- May require sudo (container use case: already root)

**Hermetic APK extraction:**
```
$ tsuku install cmake

Installing openssl from Alpine packages...
  Downloading openssl-3.1.4-r5.apk (2.1 MB)
  Verifying checksum...
  Extracting to $TSUKU_HOME/libs/openssl/3.1.4-r5/
  Setting RPATH...
Done.
```

User experience:
- Automatic, no sudo
- tsuku controls version
- User doesn't know about the library (hidden complexity)
- Security updates require tsuku recipe update, not system update

### 6.2 Is Version Control Valuable for Library Dependencies?

**For tools (what users install):** Yes. Users want `tsuku install go@1.21` not `tsuku install go` and get whatever.

**For libraries (what tools need):** Questionable.

Libraries are implementation details. Users don't care about zlib 1.2.13 vs 1.3.1. They care that cmake works.

The value of version control for libraries:
1. **Debugging:** "This worked yesterday" - but Alpine removes old versions anyway
2. **Reproducibility:** Nix provides this better
3. **Isolation:** Containers provide this better

**tsuku's design philosophy aligns with system packages:**
> "Self-contained tools, system-managed dependencies: Tools remain self-contained binaries, but library dependencies use system package managers rather than embedded Homebrew bottles."
> - `DESIGN-platform-compatibility-verification.md`

---

## 7. Implementation Effort Analysis

### 7.1 What Would ~500 LOC Buy?

From `explore_apk-download.md`:

| Component | Lines | Purpose |
|-----------|-------|---------|
| APKINDEX parser | ~100 | Parse package metadata for checksums |
| APK extraction | ~50 | Handle multi-segment gzip, skip control files |
| AlpineAPKAction | ~200 | Recipe action with Decompose() support |
| Tests | ~150 | Verification |
| **Total** | **~500** | Alpine-only hermetic extraction |

### 7.2 What Would We NOT Get?

- **Cross-musl-distro support:** Still need `xbps_install` for Void, etc.
- **True reproducibility:** Alpine removes old APKs
- **Security update automation:** Manual recipe updates required
- **Conflict resolution:** Extracted libraries could conflict with system libraries

### 7.3 Maintenance Burden

- Track Alpine release cycles
- Update APKINDEX parsing if format changes
- Handle APKv3 when/if Alpine adopts it
- Test across Alpine versions
- Handle architecture variants (x86_64, aarch64)

**For comparison:** `apk_install` is already implemented (0 new LOC), uses Alpine's native package manager, and Alpine handles all the complexity.

---

## 8. Recommendation

### 8.1 Primary Recommendation: Use System Packages

**Do not implement hermetic APK extraction.**

The value proposition doesn't hold:
1. Version pinning fails because Alpine removes old packages
2. Reproducibility requires Nix-level infrastructure tsuku doesn't have
3. Library isolation is better solved by containers
4. APK extraction only works on Alpine, not other musl distros
5. 500 LOC adds maintenance burden for minimal benefit

**Instead:** Rely on `apk_install` which already exists and works.

### 8.2 When Hermetic APK Extraction WOULD Make Sense

If ALL of these were true:
- tsuku cached APK files locally for offline reproducibility
- Users needed to run without sudo in environments where containers aren't available
- Alpine was the only musl target (no Void, Chimera support needed)
- Version consistency mattered more than security updates

**This doesn't match tsuku's current use cases or philosophy.**

### 8.3 Future Considerations

If version-controlled library dependencies become critical:

1. **Consider Nix backend:** Already works on Alpine, provides true reproducibility
2. **Consider static linking:** Universal musl binaries, no library dependency
3. **Consider tool-level isolation:** Container recipes instead of library extraction

---

## 9. Sources

### Alpine Package Management
- [Alpine Package Keeper Wiki](https://wiki.alpinelinux.org/wiki/Alpine_Package_Keeper)
- [Alpine abuild Issue #9996 - Package Retention](https://gitlab.alpinelinux.org/alpine/abuild/-/issues/9996)
- [Medium: The Problem with Docker and Alpine's Package Pinning](https://medium.stschindler.io/the-problem-with-docker-and-alpines-package-pinning-18346593e891)
- [Alpine Linux Install Specific Package Version](https://medium.com/@scythargon/alpine-linux-install-specific-older-package-version-36eadca31fc1)

### Reproducibility
- [Alpine Linux in Docker: Package Management](https://medium.com/viascom/the-alpine-packages-problem-e188f941d04e)
- [Hacker News: Alpine Version Pinning](https://news.ycombinator.com/item?id=39241696)
- [Reproducible Builds Alpine Test Results](https://tests.reproducible-builds.org/alpine/alpine.html)

### Library Conflicts
- [Prisma OpenSSL 3.0 Issue](https://github.com/prisma/prisma/issues/16553)
- [Alpine OpenSSL Mismatch Issues](https://gitlab.alpinelinux.org/alpine/aports/-/issues/17547)
- [Alpine OpenSSL/LibreSSL Conflicts](https://github.com/gliderlabs/docker-alpine/issues/303)

### Cross-Platform Tools
- [Homebrew on Linux Documentation](https://docs.brew.sh/Homebrew-on-Linux)
- [Flatpak Dependencies](https://docs.flatpak.org/en/latest/dependencies.html)
- [Nix on Alpine Gist](https://gist.github.com/danmack/b76ef257e0fd9dda906b4c860f94a591)
- [Nix on Other Distros](https://voidcruiser.nl/rambles/nix-on-other-distros-packagemanagers/)

### musl Portability
- [musl Distribution Guidelines](https://wiki.musl-libc.org/guidelines-for-distributions.html)
- [PEP 656 - musllinux Platform Tag](https://peps.python.org/pep-0656/)

### tsuku Codebase
- `docs/designs/DESIGN-platform-compatibility-verification.md`
- `wip/research/explore_full_synthesis.md`
- `wip/research/explore_apk-synthesis.md`
- `wip/research/explore_apk-portability.md`
- `internal/actions/linux_pm_actions.go`
