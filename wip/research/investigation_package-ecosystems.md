# Investigation Paths: Package Manager Ecosystems

This document identifies research paths needed to design typed system dependency actions for tsuku. Each path describes a question, its relevance, methodology, and expected deliverables.

---

## Path 1: Complete Package Manager Inventory

### Question
What is the complete set of Linux package managers in active use, and which distributions use each?

### Why It Matters
We need to know the full scope before deciding which package managers to support. Missing a widely-used manager could exclude significant user populations. Understanding the distro-to-manager mapping helps prioritize which actions to implement first.

### Research Methodology
1. Survey major Linux distributions and their default package managers
2. Identify secondary/alternative package managers (e.g., flatpak, snap, appimage)
3. Categorize by user base size (use distrowatch, surveys, container base image popularity)
4. Map package manager versions and capability differences within same manager family

### Deliverables
- **Inventory table**: Package manager name, distro(s), model type, estimated user base
- **Distro family tree**: Visual showing which distros share package managers
- **Priority ranking**: Ordered list by estimated impact/coverage

---

## Path 2: Imperative vs Declarative Model Classification

### Question
Which package managers follow an "imperative install" model compatible with our action design, and which require fundamentally different approaches?

### Why It Matters
Our current action model assumes `package-manager install <package>` is sufficient. Some systems (Nix, Guix) use declarative configuration files. Others (Portage) compile from source with USE flags. Knowing which managers fit our model determines whether we need alternative action types or should exclude certain ecosystems.

### Research Methodology
1. Document the canonical installation flow for each package manager
2. Identify whether install commands are:
   - Idempotent (safe to re-run)
   - Transactional (atomic success/failure)
   - Persistent (survive system operations like `nix-collect-garbage`)
3. Test representative install commands on each platform
4. Interview or survey users of non-imperative systems about their expectations

### Deliverables
- **Classification matrix**: Manager, model type (imperative/declarative/source-based), idempotency, persistence model
- **Compatibility assessment**: Which managers fit `*_install` action pattern
- **Alternative approaches doc**: For incompatible managers, what would an action look like?

---

## Path 3: Package Naming Divergence Analysis

### Question
How do package names differ across package managers for the same upstream software?

### Why It Matters
If tsuku recipes specify system dependencies, we need to know whether `docker-ce` on apt equals `docker` on dnf. Package name mapping is essential for cross-distro recipes. The alternative is distro-specific recipe variants, which increases maintenance burden.

### Research Methodology
1. Select 20-30 representative packages across categories:
   - Development tools (gcc, clang, cmake, make)
   - Libraries (openssl, libcurl, zlib, sqlite)
   - Runtimes (python, nodejs, ruby, java)
   - Containers (docker, podman)
   - Common CLI tools (git, curl, wget, jq)
2. Look up package names in each manager's repository
3. Identify patterns (prefixes, suffixes, version schemes)
4. Document packages with no equivalent across managers

### Deliverables
- **Package name mapping table**: Upstream name to {apt, dnf, pacman, brew, apk, ...}
- **Pattern analysis**: Common transformations (lib prefix, -dev/-devel suffixes, etc.)
- **Unmappable packages list**: Software not universally available

---

## Path 4: Repository and Key Management

### Question
What repository configuration and GPG key management does each package manager require for third-party sources?

### Why It Matters
Some tools (Docker, Node.js official builds, HashiCorp tools) require adding third-party repositories. If tsuku needs to install from non-default repos, we must understand each manager's repo/key configuration. This affects whether we need `add_repository` actions.

### Research Methodology
1. Document the default repository structure for each manager
2. Identify the process for adding third-party repositories:
   - Configuration file format and location
   - GPG/signing key import process
   - Repository priority/pinning mechanisms
3. Analyze security implications of automated repo addition
4. Survey how other tools (Ansible, Chef, Puppet) handle this

### Deliverables
- **Repository config reference**: Per-manager guide to repo addition
- **Key management comparison**: How each manager handles GPG keys
- **Security considerations doc**: Risks and mitigations for automated repo setup
- **Decision recommendation**: Should tsuku support third-party repos or only defaults?

---

## Path 5: Privilege and Execution Model

### Question
What privilege levels and execution contexts do package managers require?

### Why It Matters
Tsuku's philosophy is "no sudo required." System package managers typically need root. We need to understand:
- Can any system packages be installed without root?
- What's the user experience for privilege escalation?
- How do containerized/rootless scenarios work?

### Research Methodology
1. Test each package manager's root requirements
2. Document privilege escalation mechanisms (sudo, pkexec, polkit)
3. Investigate rootless alternatives (user namespaces, fakeroot)
4. Analyze how this interacts with containerized builds

### Deliverables
- **Privilege requirements matrix**: Manager, root required, escalation method, rootless options
- **UX implications doc**: How privilege requirements affect tsuku's user experience
- **Containerization compatibility**: How each manager works in Docker/Podman builds

---

## Path 6: Dependency Resolution and Conflicts

### Question
How do different package managers handle dependency resolution, and what conflicts can arise?

### Why It Matters
Installing system dependencies might conflict with existing packages or other tsuku-installed tools. Understanding conflict scenarios helps us design appropriate error handling and user guidance.

### Research Methodology
1. Document dependency resolution algorithms (SAT solver, priority-based, etc.)
2. Identify common conflict scenarios:
   - Version conflicts with existing packages
   - File ownership conflicts
   - Conflicts between different package managers on same system
3. Test conflict scenarios and document error messages
4. Research how to detect conflicts before installation

### Deliverables
- **Dependency resolution comparison**: How each manager resolves dependencies
- **Conflict taxonomy**: Types of conflicts and their causes
- **Pre-flight check possibilities**: Can we detect conflicts before attempting install?

---

## Path 7: Cross-Platform Abstraction Patterns

### Question
How have other tools (Ansible, Homebrew Linuxbrew, pkgsrc) solved cross-platform package abstraction?

### Why It Matters
We're not the first to face this problem. Learning from existing solutions can inform our design and help avoid known pitfalls. We might adopt proven patterns or identify anti-patterns to avoid.

### Research Methodology
1. Study Ansible's `package` module and distro-specific modules
2. Analyze how Homebrew/Linuxbrew achieves cross-platform installs
3. Review pkgsrc's approach to portable package management
4. Examine container base image patterns (distroless, Alpine popularity)
5. Look at how language package managers (pip, npm, cargo) handle native dependencies

### Deliverables
- **Pattern catalog**: Cross-platform abstraction approaches with pros/cons
- **Lessons learned**: What worked and what failed in other projects
- **Recommended patterns**: Which approaches best fit tsuku's philosophy

---

## Path 8: Version Specification and Pinning

### Question
How do package managers handle version specification, and can we achieve reproducible installs?

### Why It Matters
Tsuku values reproducibility. If system dependencies are version-sensitive, we need to understand whether pinning is possible and practical across managers.

### Research Methodology
1. Document version syntax for each manager (=, >=, ~, etc.)
2. Test whether specific versions can be installed reliably
3. Investigate repository snapshot/archive services (snapshot.debian.org, etc.)
4. Analyze version availability windows (how long are old versions kept?)

### Deliverables
- **Version syntax reference**: Per-manager version specification formats
- **Pinning feasibility assessment**: Can we achieve reproducible installs?
- **Practical limitations**: What blocks true reproducibility?

---

## Path 9: Detection and Fallback Strategies

### Question
How should tsuku detect the available package manager and handle systems where no supported manager exists?

### Why It Matters
Users run tsuku on diverse systems. We need robust detection logic and graceful degradation when system dependencies can't be installed automatically.

### Research Methodology
1. Inventory detection methods (command existence, /etc/os-release, lsb_release)
2. Test detection reliability across distros and containers
3. Document edge cases (multiple managers installed, minimal containers)
4. Design fallback user experience (manual instructions, skip with warning)

### Deliverables
- **Detection algorithm**: Reliable method to identify package manager
- **Edge case catalog**: Unusual configurations and how to handle them
- **Fallback UX design**: What users see when auto-install isn't possible

---

## Path 10: NixOS/Guix Special Case Analysis

### Question
Should tsuku support NixOS/Guix at all, and if so, what would that look like?

### Why It Matters
NixOS and Guix users have chosen declarative systems deliberately. Imperative install actions may conflict with their workflow. We need to decide whether to support these systems and how.

### Research Methodology
1. Interview NixOS/Guix users about their expectations for tools like tsuku
2. Understand nix-shell and nix develop as alternatives to global install
3. Explore whether tsuku could generate nix expressions instead of installing
4. Analyze whether tsuku itself makes sense on these platforms

### Deliverables
- **User research summary**: What NixOS/Guix users want from tsuku
- **Integration options**: Possible approaches (nix-shell, flakes, explicit skip)
- **Recommendation**: Support strategy for declarative distros

---

## Summary: Research Priority Matrix

| Path | Priority | Effort | Blocking Decisions |
|------|----------|--------|-------------------|
| 1. Package Manager Inventory | High | Low | Which actions to implement |
| 2. Imperative vs Declarative | High | Medium | Action model viability |
| 3. Package Naming Divergence | High | High | Recipe portability |
| 4. Repository/Key Management | Medium | Medium | Third-party source support |
| 5. Privilege Model | High | Low | User experience design |
| 6. Dependency Resolution | Medium | Medium | Error handling design |
| 7. Cross-Platform Patterns | Medium | Medium | Architecture decisions |
| 8. Version Pinning | Low | Medium | Reproducibility scope |
| 9. Detection/Fallback | High | Low | Implementation planning |
| 10. NixOS/Guix Special Case | Low | Medium | Platform scope decision |

---

## Suggested Research Sequence

1. **Phase 1 (Foundational)**: Paths 1, 2, 5 - Establish scope and model compatibility
2. **Phase 2 (Design Informing)**: Paths 3, 7, 9 - Inform architecture decisions
3. **Phase 3 (Detail)**: Paths 4, 6, 8 - Fill in implementation details
4. **Phase 4 (Edge Cases)**: Path 10 - Handle special platforms

Each phase should complete before major design decisions dependent on its findings.
