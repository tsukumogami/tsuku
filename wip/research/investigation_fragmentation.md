# Linux Ecosystem Fragmentation Investigation Paths

This document identifies research paths for understanding real-world Linux distribution fragmentation to inform tsuku's support matrix decisions.

---

## Path 1: Desktop Linux Market Share

### Question
What is the current distribution of Linux distros among desktop users, and what trends indicate growth or decline?

### Why It Matters
Desktop users are a primary audience for developer tools. Understanding which distros dominate helps prioritize:
- Testing environments
- Binary compatibility targets
- Documentation and troubleshooting guides
- glibc/musl version requirements

### Research Methodology
1. Aggregate data from multiple sources (no single source is authoritative):
   - StatCounter GlobalStats
   - Steam Hardware Survey (gaming, but large sample)
   - DistroWatch (popularity, not usage, but indicative of mindshare)
   - Stack Overflow Developer Survey
   - JetBrains Developer Survey
2. Cross-reference to identify consensus distros
3. Track trends over 2-3 years to avoid optimizing for declining platforms

### Deliverables
- Table: Top 10 desktop distros by estimated market share
- Trend analysis: Growing vs declining distros
- Distro family groupings (Debian-based, RHEL-based, Arch-based, etc.)
- Identified gaps in data reliability

---

## Path 2: Server/Cloud Linux Distribution

### Question
What Linux distributions dominate cloud and server deployments, and what do major cloud providers default to or recommend?

### Why It Matters
Many tsuku users will install tools on servers:
- CI/CD runners
- Development VMs
- Production build servers
- Cloud IDE environments (Codespaces, Gitpod, Cloud9)

### Research Methodology
1. Survey cloud provider defaults:
   - AWS EC2 default AMIs
   - GCP Compute Engine default images
   - Azure VM default images
   - DigitalOcean, Linode, Vultr defaults
2. Review cloud-specific distros:
   - Amazon Linux 2/2023
   - Google Container-Optimized OS
   - Azure Linux (CBL-Mariner)
3. Analyze enterprise adoption reports (Red Hat, Canonical)

### Deliverables
- Table: Cloud provider default distros
- Analysis of cloud-specific distros and their base relationships
- Enterprise vs hobbyist server distro split
- LTS release schedule comparison (support windows)

---

## Path 3: CI Provider Default Environments

### Question
What Linux distributions and versions do major CI providers use as their default runners?

### Why It Matters
CI is a critical use case - developers expect tools to "just work" in CI:
- First-run experience is often in CI
- Failures in CI create immediate friction
- CI environments are often more constrained than local dev

### Research Methodology
1. Document default runners for each major CI:
   - GitHub Actions (`ubuntu-latest`, `ubuntu-22.04`, `ubuntu-24.04`)
   - GitLab CI (shared runners)
   - CircleCI (machine executor, docker executor)
   - Travis CI
   - Azure Pipelines
   - Buildkite (hosted agents)
   - Jenkins (common Docker images)
2. Track runner update schedules (when does `ubuntu-latest` change?)
3. Note any non-Ubuntu options and their adoption

### Deliverables
- Table: CI provider to default distro mapping
- Runner version lifecycle documentation
- Recommended CI testing matrix for tsuku
- Edge cases (self-hosted runners, custom images)

---

## Path 4: Container Base Image Ecosystem

### Question
What are the most commonly used base images in the container ecosystem, and what does this imply for binary compatibility?

### Why It Matters
Containers are ubiquitous in modern development:
- Many users will run tsuku inside containers
- Container base images influence glibc expectations
- "Distroless" and Alpine challenge assumptions

### Research Methodology
1. Analyze Docker Hub pull statistics:
   - Official images (ubuntu, debian, alpine, fedora, etc.)
   - Language runtime images (python, node, golang, rust)
   - Common application bases
2. Review Docker Official Images repository for patterns
3. Survey popular Dockerfile bases in open-source projects
4. Examine container-specific distros:
   - Alpine (musl libc!)
   - Distroless (minimal glibc)
   - Wolfi/Chainguard

### Deliverables
- Ranked list: Top 20 base images by pull count
- glibc vs musl distribution in container ecosystem
- Minimum glibc version to cover X% of containers
- Alpine/musl-specific considerations

---

## Path 5: Competitor Analysis - Tool Manager Approaches

### Question
How do existing cross-platform tool managers handle Linux fragmentation, and what lessons can we learn?

### Why It Matters
- Avoid reinventing the wheel
- Learn from others' mistakes
- Understand user expectations set by existing tools
- Identify gaps we can fill

### Research Methodology
1. Analyze installation methods for each tool:
   - **rustup**: Official Rust toolchain installer
   - **pyenv**: Python version manager
   - **nvm**: Node version manager
   - **asdf**: Multi-runtime version manager
   - **mise** (formerly rtx): Modern asdf alternative
   - **Homebrew**: Package manager (now supports Linux)
   - **Nix**: Reproducible package manager
2. For each tool, document:
   - Supported distros (explicit vs implicit)
   - Binary distribution strategy (static? dynamic? source?)
   - glibc version requirements
   - musl/Alpine support
   - Installation method (curl|bash, package repos, manual)
3. Review their issue trackers for distro-specific bugs

### Deliverables
- Comparison matrix: Tool vs distro support approach
- Common patterns and anti-patterns
- Identified pain points from issue trackers
- Recommended best practices synthesis

---

## Path 6: The 80/20 Analysis

### Question
What is the minimal set of distros/configurations that covers 80% (or 90%, 95%) of real-world Linux users?

### Why It Matters
- Resource constraints require prioritization
- Defines our official support tier
- Guides CI matrix configuration
- Sets expectations with users

### Research Methodology
1. Synthesize data from Paths 1-4
2. Group by distro family (Debian-based, RHEL-based, etc.)
3. Identify common denominators:
   - glibc version floors
   - Package manager families (apt, dnf/yum, pacman)
   - Init system (systemd near-universal now)
4. Model coverage curves (X distros = Y% coverage)
5. Consider "long tail" - what do we miss at each cutoff?

### Deliverables
- Coverage curve chart
- Recommended tier system:
  - Tier 1: Fully tested, officially supported
  - Tier 2: Expected to work, community-tested
  - Tier 3: Best-effort, may work
- Explicit list of out-of-scope distros with rationale
- glibc version floor recommendation

---

## Path 7: Binary Compatibility Deep Dive

### Question
What are the technical constraints for shipping a single Linux binary that works across distros?

### Why It Matters
- Tsuku philosophy is "self-contained, no system dependencies"
- Binary compatibility determines our distribution strategy
- Trade-offs between static linking, glibc versions, and binary size

### Research Methodology
1. Research glibc symbol versioning and backwards compatibility
2. Investigate static linking options:
   - Fully static with musl
   - Partially static with glibc
   - Go's CGO_ENABLED=0 approach
3. Study how Go, Rust handle cross-distro binaries
4. Test binary compatibility empirically:
   - Build on old glibc, test on new
   - Identify common failure modes
5. Research solutions like:
   - Building on oldest supported distro
   - `zig cc` for cross-compilation
   - Hermetic build environments

### Deliverables
- Technical summary: glibc compatibility rules
- Decision matrix: Static vs dynamic linking trade-offs
- Recommended build environment specification
- Compatibility test matrix

---

## Path 8: User Base Characteristics

### Question
Who are tsuku's target users, and what does their Linux usage look like?

### Why It Matters
- "Linux users" is too broad
- Developer tools users may differ from general Linux users
- Enterprise vs individual contributors have different needs

### Research Methodology
1. Define user personas:
   - Open-source contributor
   - Enterprise developer
   - DevOps/SRE engineer
   - Student/learner
   - Hobbyist
2. Map personas to likely distro choices
3. Survey existing tsuku users (if any) or similar tool users
4. Analyze GitHub stars/traffic demographics if available

### Deliverables
- User persona definitions
- Persona to distro mapping
- Prioritized user segment list
- Distro recommendations per segment

---

## Synthesis and Decision Framework

After completing the above research paths, synthesize into:

1. **Support Matrix**: Official document defining what we support
2. **Test Matrix**: CI configuration covering supported platforms
3. **Build Strategy**: How we produce compatible binaries
4. **Documentation**: Clear communication of support boundaries
5. **Issue Templates**: Structured reporting for distro-specific issues

---

## Research Execution Notes

- Paths 1-4 can be researched in parallel (data gathering)
- Path 5 can run independently (competitor analysis)
- Paths 6-8 depend on results from Paths 1-5
- Total estimated effort: 2-4 hours of research per path
- Recommend time-boxing each path to avoid rabbit holes
