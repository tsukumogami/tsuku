# Research Spec P1-D: Ecosystem Analysis

## Objective

Understand real-world Linux distro usage across CI providers, container ecosystems, and cloud environments to identify the 80/20 point - which distros cover the majority of tsuku's target users.

## Scope

Analyze three ecosystems:
1. **CI Providers**: What distros do developers use in CI/CD?
2. **Container Base Images**: What images are most pulled on Docker Hub?
3. **Cloud Defaults**: What do major cloud providers default to?

## Research Questions

### 1. CI Provider Analysis

For each major CI provider:

| Provider | Default Runner OS | Available Linux Options | Custom Image Support |
|----------|------------------|------------------------|---------------------|
| GitHub Actions | | | |
| GitLab CI | | | |
| CircleCI | | | |
| Travis CI | | | |
| Azure Pipelines | | | |
| Jenkins (common configs) | | | |

Questions:
- What is the default Linux runner?
- What distro versions are available?
- Can users bring custom images?
- What percentage use default vs custom?

### 2. Container Base Image Analysis

Survey Docker Hub for most-pulled base images:

| Image | Pulls | Use Case | Package Manager |
|-------|-------|----------|-----------------|
| ubuntu | | General purpose | apt |
| debian | | Minimal | apt |
| alpine | | Minimal, small | apk |
| fedora | | Latest packages | dnf |
| centos/rocky/alma | | Enterprise | dnf/yum |
| amazonlinux | | AWS Lambda, ECS | yum |
| distroless | | Production, minimal | none |

Questions:
- What are the top 20 base images by pull count?
- What's the split between glibc and musl images?
- What's the trend? (Alpine growing? Ubuntu stable?)

### 3. Cloud Provider Defaults

| Provider | Default VM Image | Available Images | Container Service Default |
|----------|-----------------|------------------|--------------------------|
| AWS EC2 | | | ECS/EKS default |
| GCP Compute | | | GKE default |
| Azure VMs | | | AKS default |
| DigitalOcean | | | |
| Linode | | | |

### 4. Developer Tool Ecosystem

Survey what distros other version managers target:

| Tool | Primary Target | Tested Distros | Unsupported Distros |
|------|---------------|----------------|---------------------|
| rustup | | | |
| pyenv | | | |
| nvm | | | |
| asdf | | | |
| mise | | | |
| Homebrew (Linux) | | | |

## Methodology

1. **CI Provider Docs**: Review official documentation for runner specifications
2. **Docker Hub API**: Query pull counts for base images
3. **Cloud Provider Docs**: Check default AMI/image specifications
4. **Tool Documentation**: Review installation docs and CI configs

### Sample Queries

```bash
# Docker Hub pull counts (approximate via API)
curl -s "https://hub.docker.com/v2/repositories/library/ubuntu/" | jq '.pull_count'

# GitHub Actions runner images
# Check: https://github.com/actions/runner-images

# Popular Dockerfiles analysis
# Search GitHub for "FROM ubuntu" vs "FROM alpine" in Dockerfiles
```

## Deliverables

### 1. CI Provider Matrix (`findings_ci-providers.md`)

Complete matrix of CI providers and their Linux offerings.

### 2. Container Ecosystem Report (`findings_container-ecosystem.md`)

- Top 20 base images by popularity
- glibc vs musl split
- Trends over time (if data available)

### 3. Cloud Defaults Summary (`findings_cloud-defaults.md`)

What each major cloud provider uses by default.

### 4. 80/20 Analysis (`findings_80-20-analysis.md`)

**Key deliverable**: Which distros cover 80% of use cases?

| Tier | Distros | Coverage | Rationale |
|------|---------|----------|-----------|
| Tier 1 | Ubuntu LTS | ~50%? | CI default, most popular |
| Tier 1 | Debian | ~15%? | Container base, Ubuntu parent |
| Tier 2 | Alpine | ~15%? | Container optimization |
| Tier 2 | Fedora/RHEL | ~10%? | Enterprise, latest packages |
| Tier 3 | Others | ~10% | Long tail |

### 5. Recommendations (`findings_ecosystem-recommendations.md`)

Based on analysis:
- What should be Tier 1 (full testing, golden files)?
- What should be Tier 2 (supported, limited testing)?
- What should be Tier 3 (community-supported, no guarantees)?
- What should be explicitly unsupported?

## Output Location

All deliverables go in: `wip/research/`

## Time Box

- 1 hour: CI provider documentation review
- 1 hour: Docker Hub and container ecosystem analysis
- 30 mins: Cloud provider defaults
- 30 mins: Developer tool survey
- 30 mins: 80/20 synthesis

## Dependencies

None - this track runs independently.

## Handoff

Findings feed into:
- Support tier decisions
- CI matrix design
- Golden file distro coverage
- Documentation (which distros to highlight)
