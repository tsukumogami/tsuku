# Research Spec P1-A: Prior Art Deep Dive

## Objective

Understand how existing tools have solved Linux distribution targeting to avoid reinventing failed approaches and adopt proven patterns.

## Scope

Analyze these tools' approaches to Linux distro handling:

| Tool | Category | Why Relevant |
|------|----------|--------------|
| **Ansible** | Configuration management | Mature distro detection, fact gathering, package abstraction |
| **Puppet** | Configuration management | Facter system, provider abstraction |
| **Chef** | Configuration management | Ohai detection, platform family concept |
| **Nix/Nixpkgs** | Package manager | Declarative model, cross-distro binary distribution |
| **Homebrew/Linuxbrew** | Package manager | Source-based with bottles, Linux adaptation |
| **asdf/mise** | Version manager | Similar to tsuku, multiple tool installation |
| **rustup** | Toolchain manager | Target triple model, musl support |

## Research Questions

For each tool, answer:

### 1. Detection Model
- How does the tool detect the current Linux distribution?
- What fields are extracted? (ID, version, family, package manager?)
- How are edge cases handled? (containers, WSL, unknown distros)

### 2. Hierarchy Model
- Does the tool use a flat distro list or hierarchical model?
- Is "family" a first-class concept?
- How are derivatives handled? (Ubuntu as Debian derivative)

### 3. Targeting Model
- How do users/authors specify distro requirements?
- Is matching exact, pattern-based, or hierarchical?
- How are version constraints expressed?

### 4. Package Abstraction
- How does the tool abstract package installation across distros?
- Is there a "generic package" concept that maps to distro-specific packages?
- How are package name differences handled?

### 5. Failure Modes
- What happens when a distro is unsupported?
- Are there documented failures from their approach?
- What breaking changes have they made over time?

## Methodology

1. **Documentation Review**: Read official docs for each tool's platform handling
2. **Source Code Analysis**: Find detection and matching logic in source
3. **Issue Tracker Mining**: Search for "distro", "platform", "unsupported" issues
4. **Changelog Archaeology**: Find breaking changes related to platform model

## Deliverables

### 1. Comparison Matrix (`findings_prior-art-matrix.md`)

| Aspect | Ansible | Puppet | Chef | Nix | Homebrew | asdf | rustup |
|--------|---------|--------|------|-----|----------|------|--------|
| Detection method | | | | | | | |
| Hierarchy model | | | | | | | |
| Family concept | | | | | | | |
| Version constraints | | | | | | | |
| Package abstraction | | | | | | | |
| Edge case handling | | | | | | | |

### 2. Pattern Catalog (`findings_prior-art-patterns.md`)

Document patterns that work well:
- What abstractions have stood the test of time?
- What detection strategies are most reliable?
- What user-facing syntax is most ergonomic?

### 3. Anti-Pattern Catalog (`findings_prior-art-antipatterns.md`)

Document patterns that failed:
- What approaches caused breaking changes?
- What user complaints recur across tools?
- What maintenance burdens emerged?

### 4. Recommendations (`findings_prior-art-recommendations.md`)

Based on findings:
- Which tool's model is closest to tsuku's needs?
- What should tsuku adopt?
- What should tsuku avoid?

## Output Location

All deliverables go in: `wip/research/`

## Time Box

Focus on breadth over depth. Spend roughly equal time on each tool. If a tool's approach is clearly irrelevant to tsuku, document why and move on.

## Dependencies

None - this track runs independently.

## Handoff

Findings feed into Phase 2 hierarchy model decision.
