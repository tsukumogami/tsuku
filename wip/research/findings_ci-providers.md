# Findings: CI Provider Linux Runner Analysis

## Summary

All major CI providers default to Ubuntu-based runners. Ubuntu 22.04 LTS and 24.04 LTS dominate the CI landscape, with the `ubuntu-latest` label transitioning to Ubuntu 24.04 across providers in 2025.

## CI Provider Matrix

| Provider | Default Linux Runner | Default Version | Other Available Options | Custom Image Support |
|----------|---------------------|-----------------|------------------------|---------------------|
| GitHub Actions | Ubuntu | 24.04 LTS (`ubuntu-latest`) | 22.04 LTS | Yes (container jobs) |
| GitLab CI SaaS | ruby:3.1 (Debian-based) | Debian (Container-Optimized OS host) | Any Docker image | Yes |
| CircleCI | cimg/base (Ubuntu) | 22.04 LTS (newest LTS after 2 months) | Custom images | Yes |
| Travis CI | Ubuntu | 20.04 LTS (Focal) | 22.04 (Jammy), 24.04 (Noble) | Yes |
| Azure Pipelines | Ubuntu | 24.04 LTS (`ubuntu-latest`) | 22.04 LTS | Yes |

## Detailed Provider Analysis

### GitHub Actions

**Default Image:** `ubuntu-latest` maps to Ubuntu 24.04 LTS (as of January 2025)

**Available Linux Options:**
- `ubuntu-24.04` - Latest LTS
- `ubuntu-22.04` - Previous LTS
- `ubuntu-slim` - Minimal variant

**Key Details:**
- Images updated weekly
- Ubuntu 20.04 brownout began January 2025
- Default Java version is 17 on Ubuntu 24.04
- Images hosted on Azure (kernel version 6.11.0-1018-azure)

**Image Source:** [actions/runner-images](https://github.com/actions/runner-images)

### GitLab CI (SaaS)

**Default Image:** `ruby:3.1` (Debian-based Docker image)

**Infrastructure:**
- Hosted runners run on Google Cloud Compute Engine
- Host VMs use Google Container-Optimized OS (COS)
- Jobs run in Docker containers with docker+machine executor

**Key Details:**
- Changed from Ruby 2.5 to Ruby 3.1 in January 2023
- Runners support Docker-in-Docker (privileged mode)
- Self-hosted runners can use any default image (commonly `alpine:latest`)
- Untagged jobs run on small Linux x86-64 runners

**Documentation:** [GitLab CI Hosted Runners on Linux](https://docs.gitlab.com/ee/ci/runners/hosted_runners/linux.html)

### CircleCI

**Default Image:** `cimg/base` (Ubuntu-based)

**Machine Executor:** Ubuntu 22.04 VM

**Available Tags:**
- `default` - Current stable (newest LTS after 2 months)
- `current` - Updates approximately every 3 months
- `edge` - Preview releases (not for production)

**Key Details:**
- Both AMD64 and ARM64 architectures supported
- `machine: true` is deprecated; must specify image
- Two most recent Ubuntu LTS versions maintained

**Documentation:** [CircleCI cimg/base](https://circleci.com/developer/images/image/cimg/base)

### Travis CI

**Default Image:** Ubuntu 20.04 LTS (Focal)

**Available Distros:**
- `focal` (20.04) - Default
- `jammy` (22.04)
- `noble` (24.04) - May fall back to focal without warning
- Legacy: `xenial`, `bionic`
- Also: `rhel8`, server-2016 (Windows)

**Key Issues:**
- Noble (24.04) may silently fall back to Focal
- Documentation recommends explicitly specifying `dist:` key

**Documentation:** [Travis CI Ubuntu Linux Build Environments](https://docs.travis-ci.com/user/reference/linux/)

### Azure Pipelines

**Default Image:** `ubuntu-latest` maps to Ubuntu 24.04 LTS (transitioning 2025)

**Available vmImages:**
- `ubuntu-latest` (transitioning to 24.04)
- `ubuntu-24.04`
- `ubuntu-22.04`

**Key Details:**
- Ubuntu 20.04 retired August 2025
- .NET 6 removed from images August 2025
- Uses same runner images as GitHub Actions

**Documentation:** [Azure Pipelines Microsoft-hosted agents](https://learn.microsoft.com/en-us/azure/devops/pipelines/agents/hosted)

## Key Insights

### Ubuntu Dominance

1. **Every major CI provider defaults to Ubuntu** - This is the clearest signal in the ecosystem
2. **Ubuntu 22.04 and 24.04 LTS are the only versions that matter** - 20.04 is being retired across all providers
3. **Weekly update cadence** - CI images are kept current with security patches

### Container Runtime Context

- GitLab CI SaaS runs jobs in Docker containers on Container-Optimized OS hosts
- Jobs can use any Docker image (Alpine, Debian, etc.) but Ruby/Debian is the default
- CircleCI's `cimg/base` is purpose-built for CI workloads

### Architecture Support

- x86-64 (AMD64) is universally supported
- ARM64 support is growing (GitHub, CircleCI, Azure all offer ARM runners)
- ARM64 runners typically also use Ubuntu

## Implications for tsuku

### Primary Target

**Ubuntu 22.04 and 24.04 LTS** should be Tier 1 targets because:
- Every CI provider defaults to Ubuntu
- Developers installing tools in CI will be on Ubuntu
- Testing on Ubuntu covers the CI use case

### Secondary Consideration

**Debian** matters because:
- Language runtime Docker images (python, node, ruby) are Debian-based
- GitLab CI default `ruby:3.1` is Debian
- Many CI jobs use language-specific images rather than bare Ubuntu

### Container Jobs

When developers use custom Docker images in CI:
- Alpine is popular for size optimization
- Debian/Debian-slim for compatibility
- Ubuntu for familiarity

## Sources

- [GitHub Actions Runner Images](https://github.com/actions/runner-images)
- [GitHub Actions: ubuntu-latest will use Ubuntu-24.04](https://github.com/actions/runner-images/issues/10636)
- [GitLab CI Hosted Runners on Linux](https://docs.gitlab.com/ee/ci/runners/hosted_runners/linux.html)
- [CircleCI cimg/base](https://circleci.com/developer/images/image/cimg/base)
- [CircleCI Linux VM Support Policy](https://circleci.com/docs/guides/execution-managed/linux-vm-support-policy/)
- [Travis CI Ubuntu Linux Build Environments](https://docs.travis-ci.com/user/reference/linux/)
- [Azure Pipelines Microsoft-hosted agents](https://learn.microsoft.com/en-us/azure/devops/pipelines/agents/hosted)
- [Azure DevOps Blog: Upcoming Updates for Azure Pipelines](https://devblogs.microsoft.com/devops/upcoming-updates-for-azure-pipelines-agents-images/)
