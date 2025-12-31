# Investigation Paths: Linux Distribution Hierarchy

## Context

The current design proposes `target = (platform, distro)` where `distro` is a flat string like `ubuntu`, `debian`, `fedora`, `arch`. User feedback challenges this:

> The first grouping under linux isn't distro, it's family. A linux family has distros, and a linux distro has its own sub-groupings by versions.

This suggests a hierarchy:
```
Family          Distro              Version/Release
-------------------------------------------------------
debian-family   debian              12 (bookworm), 11 (bullseye)
                ubuntu              24.04, 22.04, 20.04
                linuxmint           21, 22
                pop                 22.04

rhel-family     rhel                9, 8, 7
                centos              stream-9, stream-8
                rocky               9, 8
                alma                9, 8
                fedora              40, 39

arch-family     arch                (rolling)
                manjaro             (rolling)
                endeavouros         (rolling)

independent     alpine              3.19, 3.18
                nixos               24.05, 23.11
                void                (rolling)
                gentoo              (rolling)
```

The question is whether our model should recognize this structure explicitly or continue treating it as emergent from `ID_LIKE` chains.

---

## Investigation Path 1: Family Taxonomy and Boundaries

### Question
What defines a Linux "family" and where are the boundaries? Is it package manager, init system, filesystem hierarchy, shared lineage, or something else?

### Why It Matters
If "family" is a meaningful abstraction for tsuku, we need a principled definition. Different criteria produce different groupings:
- By package manager: debian-family (apt), rhel-family (dnf/yum), arch-family (pacman), suse-family (zypper)
- By lineage: Slackware derivatives, Red Hat derivatives, Debian derivatives
- By release model: rolling vs point release
- By philosophy: systemd vs non-systemd, glibc vs musl

The definition affects recipe authoring semantics and what "family compatibility" means.

### Research Methodology
1. **Literature review**: Survey how package managers, configuration management tools (Ansible, Puppet), and containerization platforms (Docker official images) categorize distros
2. **Empirical analysis**: For each proposed family, identify:
   - Common packages/repositories
   - Configuration file locations
   - Init system
   - Library implementations (glibc/musl)
3. **Boundary testing**: Identify distros that don't fit cleanly (e.g., Ubuntu Server vs Ubuntu Desktop, Fedora vs RHEL, Alpine's unique position)

### Deliverables
- Taxonomy decision document: What dimension(s) define family for tsuku's purposes
- Family boundary map: Clear membership for top-10 distros with edge cases noted
- Compatibility matrix: What does "family membership" guarantee in practice

---

## Investigation Path 2: ID_LIKE Chain Reality Check

### Question
How consistently do distros declare their `ID_LIKE` chains? Are there systematic patterns in what gets omitted?

### Why It Matters
Our current design relies on `ID_LIKE` for implicit family membership. If distros systematically omit ancestors (Pop!_OS declaring `ubuntu` but not `debian`), we need to understand the pattern to decide if:
- Recipes should explicitly list all intended distros (current recommendation)
- tsuku should maintain a known-ancestry database
- Family should be a first-class concept independent of `ID_LIKE`

### Research Methodology
1. **Collect os-release samples**: Gather `/etc/os-release` from:
   - Official Docker images (ubuntu, debian, fedora, etc.)
   - VM images from distro downloads
   - Cloud provider images (AWS AMI, Azure Gallery)
   - User-submitted samples from issues/community
2. **Parse and categorize**: Extract ID, ID_LIKE, VERSION_ID for each sample
3. **Build ancestry graph**: Compare declared `ID_LIKE` against known lineage
4. **Quantify incompleteness**: What percentage of derivatives omit ancestors?

### Deliverables
- Database: Structured collection of os-release samples with source metadata
- Gap analysis: Report showing incomplete `ID_LIKE` declarations with examples
- Recommendation: Whether transitive resolution or explicit listing is more practical

---

## Investigation Path 3: Family as First-Class Concept

### Question
Should tsuku's data model include an explicit `family` field, separate from `distro`? What would the semantics be?

### Why It Matters
Two possible models:

**Model A: Flat distro with ID_LIKE fallback (current)**
```toml
[[steps]]
action = "apt_install"
packages = ["docker-ce"]
when = { distro = ["ubuntu", "debian", "pop", "mint"] }  # List all explicitly
```

**Model B: Hierarchical with family**
```toml
[[steps]]
action = "apt_install"
packages = ["docker-ce"]
when = { family = ["debian"] }  # Matches all debian-family distros
```

Model B is more ergonomic but requires tsuku to maintain family membership definitions.

### Research Methodology
1. **User experience study**: Design mock recipe snippets using both models; evaluate readability and error potential
2. **Maintenance analysis**: What happens when a new derivative appears? Who updates family membership?
3. **Edge case exploration**: How do we handle distros in multiple families or with disputed lineage?
4. **Prior art review**: How do Ansible facts, Puppet facter, Chef ohai handle this? What can we learn from their evolution?

### Deliverables
- Decision document: Flat vs hierarchical model with trade-offs
- If hierarchical: Family membership source-of-truth proposal
- Recipe authoring examples showing both models
- Migration path if we start flat and evolve to hierarchical

---

## Investigation Path 4: Version Constraint Semantics

### Question
What version constraint syntax and semantics should tsuku support? How do rolling releases fit?

### Why It Matters
The hierarchy includes versions: `debian-12`, `ubuntu-24.04`, `arch` (no version). Different distros have fundamentally different versioning:
- **Point releases**: Ubuntu 24.04, Debian 12, RHEL 9.3 - clear numeric versions
- **Rolling releases**: Arch, Manjaro, Gentoo - no version number (or `BUILD_ID=rolling`)
- **Codenames**: Debian bookworm, Ubuntu noble - string identifiers
- **LTS vs regular**: Ubuntu 24.04 LTS vs 24.10 - same format, different support window

### Research Methodology
1. **Survey version formats**: Collect VERSION_ID, VERSION_CODENAME from distro samples
2. **Define comparison semantics**: How does `>=22.04` compare across distros? Is `24.04 > 23.10` true?
3. **Rolling release strategy**: What does version constraint mean for rolling releases?
4. **Feature detection alternative**: When is runtime feature detection better than version constraints?

### Deliverables
- Version format catalog: Table of VERSION_ID formats by distro
- Comparison algorithm: Spec for version comparison (or decision to not support)
- Rolling release handling: How constraints apply to Arch, Gentoo, etc.
- Feature detection guidelines: When to use version constraints vs capability detection

---

## Investigation Path 5: Package Manager as Implicit Family

### Question
Is package manager the right proxy for family? Should `when = { package_manager = "apt" }` replace distro-based matching?

### Why It Matters
The goal of distro filtering is often to select the right package manager:
```toml
when = { distro = ["ubuntu", "debian"] }  # Really means: when apt is available
```

Perhaps the model should be:
```toml
when = { package_manager = "apt" }  # Direct intent
```

This sidesteps the family taxonomy problem but introduces package manager detection complexity.

### Research Methodology
1. **Inventory package managers**: List package managers by distro family
2. **Detection mechanisms**: How to detect which package manager is available?
3. **Edge cases**: Distros with multiple managers (Fedora with dnf and flatpak), third-party managers (Homebrew on Linux)
4. **Semantic comparison**: Compare `distro`, `family`, `package_manager` as filtering dimensions

### Deliverables
- Package manager matrix: Which distros use which managers
- Detection algorithm: How to determine available package managers
- Recommendation: Whether package_manager should be a filter dimension
- Interaction design: How distro, family, package_manager would coexist if all were supported

---

## Investigation Path 6: Container Environment Implications

### Question
How does the family/distro/version hierarchy interact with container-based sandbox testing?

### Why It Matters
Our sandbox design builds containers for specific `target = (platform, distro)` tuples:
```go
DeriveContainerSpec(targetDistro string, ...) *ContainerSpec
// Maps: ubuntu -> ubuntu:24.04, debian -> debian:bookworm-slim
```

If we have a hierarchy, questions arise:
- What base image represents `debian-family`?
- Should we test on multiple family members (ubuntu AND debian)?
- How does version affect base image selection?

### Research Methodology
1. **Map distros to images**: Document canonical Docker images for each distro
2. **Test coverage analysis**: What's the minimum set of images to cover each family?
3. **Version pinning**: How do container image tags map to distro versions?
4. **CI matrix design**: How would hierarchical filtering affect CI test matrices?

### Deliverables
- Image selection guide: How to choose base image for family, distro, version
- Test matrix recommendation: Minimum viable distro coverage for CI
- Version pinning strategy: How to specify version constraints in container selection
- Cost-benefit analysis: More granular testing vs CI time/cost

---

## Investigation Path 7: Prior Art Deep Dive

### Question
How have other tools solved this problem? What patterns are proven vs which are cautionary tales?

### Why It Matters
We're not the first to face this. Tools like Ansible, Puppet, Nix, Homebrew, and language package managers all handle platform variance. Understanding their evolution can help us avoid known pitfalls.

### Research Methodology
1. **Tool survey**: Document how these tools handle distro detection and filtering:
   - Ansible: `ansible_facts`, `ansible_distribution`, `ansible_os_family`
   - Puppet: `facter`, `os.family`, `os.distro`
   - Chef: `ohai`, `platform_family`
   - Nix/NixOS: platform detection
   - Homebrew: Linux support
2. **Evolution study**: How have these tools' models changed over time? What broke?
3. **Failure modes**: Document known issues and community complaints
4. **Best practices extraction**: What consensus has emerged?

### Deliverables
- Comparison matrix: How each tool models distro/family/version
- Historical analysis: Changes and deprecations over time
- Failure catalog: Known issues from each tool's approach
- Recommendations: Patterns to adopt, patterns to avoid

---

## Summary: Investigation Priority

| Path | Question | Urgency | Effort | Dependency |
|------|----------|---------|--------|------------|
| 1 | What defines family? | High | Medium | None |
| 2 | ID_LIKE reality | High | High | Path 1 |
| 3 | Family as first-class | High | Medium | Paths 1, 2 |
| 4 | Version semantics | Medium | High | Path 3 decision |
| 5 | Package manager proxy | Medium | Medium | Path 3 decision |
| 6 | Container implications | Medium | Medium | Path 3 decision |
| 7 | Prior art | High | Low | None (can inform all) |

**Recommended sequence:**
1. Path 7 (Prior Art) - Quick survey to learn from others first
2. Path 1 (Family Taxonomy) - Establish definitions
3. Path 2 (ID_LIKE Reality) - Gather data
4. Path 3 (First-Class Family) - Make model decision
5. Paths 4, 5, 6 in parallel after model decision

---

## Open Questions

These questions emerged from framing the investigation and need resolution:

1. **Stability commitment**: If we maintain a family-to-distro mapping, what's our update cadence when new distros appear?

2. **Authority source**: Who decides family membership? Distro maintainers (via ID_LIKE), tsuku maintainers, or community PRs?

3. **Granularity balance**: How fine-grained should targeting be? `debian` vs `debian-bookworm` vs `debian-12` vs `debian-12.5`?

4. **Validation scope**: Should tsuku validate that a recipe's distro claims are accurate (this recipe says apt but targets arch)?

5. **Documentation commitment**: If we support 5 tier-1 distros, do we commit to documenting behavior differences?

6. **Breaking change tolerance**: If we start with flat distro and later add family, is that breaking? Can recipes use both?
