# Findings: Cloud Provider Defaults

## Summary

Cloud providers offer multiple Linux distributions, but defaults vary by provider. AWS emphasizes Amazon Linux, GCP offers multiple options with Container-Optimized OS for Kubernetes, and Azure partners with multiple Linux vendors with Ubuntu featured prominently in quickstarts.

## Cloud Provider Matrix

| Provider | Default VM Image | Recommended for New Projects | Container Service Default |
|----------|-----------------|-----------------------------|-----------------------|
| AWS EC2 | Amazon Linux 2023 | Amazon Linux 2023 | AL2023 (ECS/EKS) |
| GCP Compute | No single default | Debian, Ubuntu, or specialty | Container-Optimized OS (GKE) |
| Azure VMs | No single default | Ubuntu 22.04 LTS (quickstart) | Ubuntu (AKS) |

## AWS (Amazon Web Services)

### EC2 Instances

**Recommended Image:** Amazon Linux 2023 (AL2023)

**AL2023 Details:**
- Based on Fedora
- Requires IMDSv2 by default
- Kernel options: 6.1 (stable) or 6.12 (March 2025)
- Long-term support through 2028
- Quarterly update cadence

**Available AMIs:**
- Amazon Linux 2023 (recommended)
- Amazon Linux 2 (EOL June 30, 2026)
- Ubuntu (via Canonical partnership)
- Debian, RHEL, SUSE, CentOS Stream

**Key Migration Timeline:**
- Amazon Linux 2 EOL: June 30, 2026
- Amazon Linux 1 EOL: December 31, 2023 (already passed)
- No new Amazon Linux versions planned for 2025-2026

### Container Services (ECS/EKS)

**ECS-Optimized AMI:**
- Recommended: Amazon Linux 2023
- AL2 EOL: June 30, 2026

**EKS-Optimized AMI:**
- Default for new clusters (v1.30+): Amazon Linux 2023
- AL2 EKS AMI support ends: November 26, 2025
- SELinux enabled by default
- OpenSSL version 3

**Container Base Images:**
- `public.ecr.aws/amazonlinux/amazonlinux:latest` (AL2023)
- Available on Docker Hub as `amazonlinux`

## GCP (Google Cloud Platform)

### Compute Engine

**No Single Default:** GCP presents multiple options without a clear default

**Available Image Projects:**
| Project | Distribution | License Cost |
|---------|-------------|--------------|
| `debian-cloud` | Debian | Free |
| `ubuntu-os-cloud` | Ubuntu | Free |
| `centos-cloud` | CentOS Stream | Free |
| `rocky-linux-cloud` | Rocky Linux | Free |
| `almalinux-cloud` | AlmaLinux | Free |
| `fedora-coreos-cloud` | Fedora CoreOS | Free |
| `rhel-cloud` | Red Hat Enterprise | Premium |
| `suse-cloud` | SUSE Linux Enterprise | Premium |
| `oracle-linux-cloud` | Oracle Linux | Free |

**Azure-Tuned Kernels:** GCP provides tuned kernels for:
- Debian Cloud Kernel
- Ubuntu Azure-Tuned Kernel (sic - documentation uses "Azure" for historical reasons)

**Image Characteristics:**
- 10 GB default boot disk
- GPT partition table
- EFI partition for UEFI boot
- Regular updates for CVEs

### GKE (Google Kubernetes Engine)

**Default Node Image:** Container-Optimized OS with containerd (cos_containerd)

**Details:**
- Default since GKE 1.19
- GKE Autopilot: Always uses cos_containerd (not configurable)
- Docker node images deprecated in GKE 1.24
- Supports gVisor and Image Streaming

**Alternative:** Ubuntu node images available when:
- XFS filesystem support needed
- CephFS support needed
- Debian packages required

## Azure

### Azure Virtual Machines

**No Single Default:** Azure presents multiple endorsed distributions

**Endorsed Linux Distributions:**
- Ubuntu (Canonical)
- Debian
- Red Hat Enterprise Linux
- SUSE Linux Enterprise
- CentOS Stream
- Oracle Linux
- Flatcar Container Linux

**Quickstart Default:** Ubuntu Server 22.04 LTS

**Azure-Tuned Kernels Available:**
- CentOS Azure-Tuned (via Virtualization SIG)
- Debian Cloud Kernel
- SUSE Azure-Tuned
- Ubuntu Azure-Tuned
- Flatcar Container Linux

**Image Attributes:**
- Publisher: Canonical, RedHat, SUSE, etc.
- Offer: e.g., 0001-com-ubuntu-server-jammy
- SKU: e.g., 22_04-lts-gen2

### AKS (Azure Kubernetes Service)

**Default:** Ubuntu-based node images

**Supported Versions (Azure Local):**
- Ubuntu 20.04
- Ubuntu 22.04
- Ubuntu 24.04 LTS

## DigitalOcean and Linode

### DigitalOcean

**Available Distributions:**
- Ubuntu (featured prominently)
- Debian
- Fedora
- CentOS
- Rocky Linux

**Documentation Default:** Ubuntu in most tutorials

### Linode (Akamai)

**Available Distributions:**
- Ubuntu
- Debian
- CentOS
- Fedora
- Rocky Linux
- Alpine Linux

**Documentation Default:** Ubuntu in most guides

## Key Observations

### Ubuntu Is Ubiquitous

1. **Azure quickstarts default to Ubuntu**
2. **GCP documentation often shows Ubuntu examples**
3. **AWS offers Ubuntu as an alternative to Amazon Linux**
4. **DigitalOcean and Linode tutorials favor Ubuntu**

### Amazon Linux Is AWS-Specific

- Optimized for AWS services
- May have AWS-specific packages/configurations
- Less transferable knowledge to other environments
- Still requires glibc compatibility

### Container Services Converge on Custom OS

- **GKE:** Container-Optimized OS (minimal, purpose-built)
- **EKS:** Amazon Linux 2023 (Fedora-based, container-optimized)
- **AKS:** Ubuntu with Azure-tuned kernel

### Enterprise vs Developer Defaults

- **Enterprise deployments:** RHEL, SUSE (paid support)
- **Developer/startup deployments:** Ubuntu, Debian (free)

## Implications for tsuku

### Priority Targets

1. **Ubuntu LTS (22.04, 24.04):** Universal across all clouds, CI, documentation
2. **Debian:** Base for many cloud images, container runtimes
3. **Amazon Linux 2023:** Important for AWS-centric users

### Secondary Targets

4. **Fedora:** Cutting edge, AL2023 is Fedora-based
5. **RHEL/Rocky/Alma:** Enterprise deployments

### Container Runtime Considerations

- GKE's Container-Optimized OS is not a general-purpose target
- Users run containers ON these systems, not tsuku
- Focus on what runs INSIDE containers (Ubuntu, Debian, Alpine)

## Sources

- [AWS AL2023 on EC2](https://docs.aws.amazon.com/linux/al2023/ug/ec2.html)
- [AWS Amazon Linux 2023 FAQs](https://aws.amazon.com/linux/amazon-linux-2023/faqs/)
- [AWS ECS-optimized Linux AMIs](https://docs.aws.amazon.com/AmazonECS/latest/developerguide/ecs-optimized_AMI.html)
- [AWS EKS-optimized Amazon Linux AMIs](https://docs.aws.amazon.com/eks/latest/userguide/eks-optimized-ami.html)
- [GCP OS Images](https://cloud.google.com/compute/docs/images)
- [GCP Operating System Details](https://docs.cloud.google.com/compute/docs/images/os-details)
- [GKE Node Images](https://cloud.google.com/kubernetes-engine/docs/concepts/node-images)
- [Azure Endorsed Linux Distributions](https://learn.microsoft.com/en-us/azure/virtual-machines/linux/endorsed-distros)
- [Azure Linux VM Quickstart](https://learn.microsoft.com/en-us/azure/virtual-machines/linux/quick-create-portal)
- [Ubuntu on GCP Documentation](https://documentation.ubuntu.com/gcp/)
