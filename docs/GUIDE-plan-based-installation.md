# Plan-Based Installation Guide

This guide explains how to use tsuku's two-phase installation for reproducible deployments, air-gapped environments, and CI distribution.

## Overview

tsuku supports a two-phase installation architecture:

1. **Eval phase**: Generate an installation plan with resolved URLs and checksums
2. **Exec phase**: Execute the plan to install the tool

This separation enables:

- **Reproducibility**: Same plan produces identical installations
- **Air-gapped deployments**: Generate plans online, execute offline
- **CI distribution**: Pre-compute plans, distribute to build agents
- **Auditability**: Inspect exactly what will be installed before execution

## Basic Usage

### File-Based Installation

Generate a plan, optionally inspect it, then install:

```bash
# Generate plan to a file
tsuku eval rg > rg-plan.json

# Inspect the plan (optional)
cat rg-plan.json

# Install from the plan
tsuku install --plan rg-plan.json
```

### Piped Installation

For immediate installation without saving the plan:

```bash
tsuku eval rg | tsuku install --plan -
```

The `-` tells tsuku to read the plan from stdin.

### Tool Name Validation

You can optionally specify the tool name for validation:

```bash
# Validates that plan.json is for "rg"
tsuku install rg --plan plan.json
```

This catches errors like accidentally using the wrong plan file.

## Air-Gapped Deployment

Air-gapped environments have no internet access. Use plan-based installation to pre-fetch everything needed.

### Step 1: Generate Plan (Online Machine)

On a machine with internet access:

```bash
tsuku eval kubectl@1.29.0 > kubectl-plan.json
```

### Step 2: Download Assets (Online Machine)

The plan contains download URLs. Extract and download them:

```bash
# Extract URLs from plan
jq -r '.steps[] | select(.params.url) | .params.url' kubectl-plan.json > urls.txt

# Download all assets
mkdir -p assets
cd assets
while read url; do
  curl -LO "$url"
done < ../urls.txt
```

### Step 3: Transfer to Air-Gapped Machine

Copy the plan file and downloaded assets to the target machine:

```bash
# Package everything
tar -czf kubectl-bundle.tar.gz kubectl-plan.json assets/

# Transfer via USB, secure copy, etc.
scp kubectl-bundle.tar.gz airgapped-host:/tmp/
```

### Step 4: Install (Air-Gapped Machine)

On the air-gapped machine:

```bash
# Extract bundle
tar -xzf kubectl-bundle.tar.gz

# Serve assets locally (simple HTTP server)
cd assets && python3 -m http.server 8000 &

# Modify plan URLs to point to local server
sed -i 's|https://.*kubernetes.*|http://localhost:8000/|g' kubectl-plan.json

# Install from modified plan
tsuku install --plan kubectl-plan.json
```

## CI Distribution

Pre-computing plans eliminates network variability in CI builds.

### Release Workflow

During release, generate plans for all supported platforms:

```bash
#!/bin/bash
# generate-plans.sh - Run during release

TOOLS="kubectl terraform gh"
PLATFORMS="linux-amd64 linux-arm64 darwin-amd64 darwin-arm64"

for tool in $TOOLS; do
  for platform in $PLATFORMS; do
    os=${platform%-*}
    arch=${platform#*-}
    tsuku eval $tool --os $os --arch $arch > plans/${tool}-${platform}.json
  done
done
```

Store plans as release artifacts alongside binaries.

### CI Installation

In CI jobs, download and use pre-computed plans:

```yaml
# .github/workflows/build.yml
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - name: Download tool plans
        run: |
          curl -LO https://releases.example.com/plans/kubectl-linux-amd64.json
          curl -LO https://releases.example.com/plans/terraform-linux-amd64.json

      - name: Install tools from plans
        run: |
          tsuku install --plan kubectl-linux-amd64.json
          tsuku install --plan terraform-linux-amd64.json
```

### Benefits for CI

- **Deterministic**: Every build uses identical tool versions
- **Fast**: Skips version resolution (already computed)
- **Reliable**: No external API calls during builds
- **Auditable**: Plans document exactly what was installed

## Sandbox Testing with Plans

Combine plan-based installation with sandbox testing to validate installations in isolation.

### Testing Plans in Containers

```bash
# Generate and test a plan in a sandbox
tsuku eval kubectl > kubectl-plan.json
tsuku install --plan kubectl-plan.json --sandbox

# Or pipe directly
tsuku eval kubectl | tsuku install --plan - --sandbox
```

This is useful for:
- **Pre-production validation**: Test plans before distributing to production
- **Recipe development**: Verify local recipe changes in isolation
- **CI/CD pipelines**: Validate installations without affecting the host

### CI Integration with Sandbox Testing

```yaml
# .github/workflows/build.yml
jobs:
  test-tools:
    runs-on: ubuntu-latest
    steps:
      - name: Generate and test plan
        run: |
          ./tsuku eval kubectl > kubectl-plan.json
          ./tsuku install --plan kubectl-plan.json --sandbox

      - name: Install for real (after sandbox validation)
        run: ./tsuku install --plan kubectl-plan.json
```

The sandbox step ensures the plan is valid before actual installation.

## Plan Format Reference

Installation plans are JSON files with this structure:

```json
{
  "format_version": 2,
  "tool": "rg",
  "version": "14.1.0",
  "platform": {
    "os": "linux",
    "arch": "amd64"
  },
  "deterministic": true,
  "steps": [
    {
      "action": "download",
      "params": {
        "url": "https://github.com/.../ripgrep-14.1.0-x86_64.tar.gz"
      },
      "checksum": "sha256:abc123...",
      "evaluable": true
    },
    {
      "action": "extract",
      "params": {
        "archive": "ripgrep-14.1.0-x86_64.tar.gz"
      },
      "evaluable": true
    }
  ]
}
```

### Key Fields

| Field | Description |
|-------|-------------|
| `format_version` | Plan schema version (currently 2) |
| `tool` | Tool name |
| `version` | Resolved version string |
| `platform.os` | Target operating system |
| `platform.arch` | Target architecture |
| `deterministic` | Whether this plan is fully reproducible (see Determinism section below) |
| `steps` | Ordered list of installation actions |
| `steps[].action` | Action type (download, extract, chmod, etc.) |
| `steps[].params` | Action-specific parameters |
| `steps[].checksum` | SHA256 checksum for download verification |
| `steps[].evaluable` | Whether this step can be pre-computed |

## Understanding Determinism in Plans

The `deterministic` field indicates whether an installation plan will produce byte-for-byte identical results across different machines and at different times.

### Fully Deterministic Plans (`deterministic: true`)

Plans marked as deterministic use only **file operation primitives**:

- `download`: Fetch files with checksums
- `extract`: Decompress archives
- `chmod`: Set file permissions
- `install_binaries`: Copy binaries and create symlinks

These operations are fully reproducible because they:
- Don't depend on system compilers or build toolchains
- Download pre-built binaries with verified checksums
- Perform only file operations that produce identical results every time

**Example:** Tools like `kubectl`, `ripgrep`, `terraform` (when pre-built binaries are available) produce deterministic plans.

### Non-Deterministic Plans (`deterministic: false`)

Plans marked as non-deterministic contain **ecosystem primitives** that delegate to external package managers or compilers:

- `go_build`: Builds from Go source code
- `cargo_build`: Builds from Rust source code
- `npm_exec`: Runs npm commands
- `pip_install`: Installs Python packages
- `gem_exec`: Runs Ruby bundler
- `nix_realize`: Uses Nix for builds
- `cpan_install`: Installs Perl modules

While these actions capture maximum constraint at eval time (through lockfiles and dependency snapshots), they may still have residual non-determinism due to:

- **Compiler version differences**: Different C/C++ compiler versions may produce different machine code, even with identical source
- **Native code generation**: LLVM and other code generators can produce platform-specific variations
- **Runtime environment**: System libraries and tools may affect compilation output
- **Floating-point precision**: Some calculations may vary slightly across platforms

### When to Use Non-Deterministic Plans

Non-deterministic plans are still highly reproducible and suitable for:

- **CI/CD pipelines**: Ensures consistent tool versions across builds
- **Development environments**: Guarantees team members have matching toolchain versions
- **Air-gapped deployments**: Pre-computes everything needed without network access
- **Supply chain control**: Captures exact dependencies at plan generation time

The key difference from fully deterministic plans is that byte-for-byte identical binaries are not guaranteed, but the captured constraints ensure version consistency.

### Checking Plan Determinism

When you run `tsuku eval`, the output plan includes the `deterministic` field. Inspect it to understand what guarantees the plan provides:

```bash
# Generate a plan and check determinism
tsuku eval myool > plan.json

# View the deterministic flag
jq '.deterministic' plan.json
```

### Checksum Verification

Download steps include checksums computed during eval. During exec:

- Downloaded files are verified against the plan's checksums
- Mismatches cause installation to fail
- This protects against supply chain attacks and detects upstream changes

If verification fails:

```bash
# Regenerate the plan to get updated checksums
tsuku eval rg > rg-plan.json

# Review changes before re-installing
diff old-plan.json rg-plan.json
```

## Security Model

Plan-based installation has important security implications. Understanding the trust boundaries helps you use plans safely.

### Plans as Trusted Inputs

When you execute a plan with `tsuku install --plan <file>`, tsuku treats the plan as a trusted input:

- tsuku **trusts the URLs** listed in the plan
- tsuku **verifies checksums** match what's in the plan
- tsuku **does NOT** re-validate URLs or checksums against external sources
- tsuku **does NOT** connect to version providers during execution

This design enables offline execution in air-gapped environments but means the plan itself must be trustworthy.

### Checksum Verification

During plan execution, downloaded files are verified against the checksums embedded in the plan. This protects against:

| Threat | Protected Against? | Notes |
|--------|-------------------|-------|
| **File corruption during download** | ✅ Yes | Checksums catch accidental corruption |
| **Upstream file modifications** | ✅ Yes | If files are re-released, checksums change and installation fails |
| **Man-in-the-middle (MITM) attacks** | ✅ Yes | Attacker would need to control original source AND bypass HTTPS |
| **Malicious URLs in the plan** | ❌ No | Plans must be trusted; checksums verify content of URLs in the plan |
| **Compromised sources at eval time** | ❌ No | Checksums capture state at plan generation time only |
| **Supply chain attacks pre-eval** | ❌ No | If sources are compromised before `tsuku eval`, the plan reflects that |

### Best Practices for Plan Security

**Generate plans yourself:**

```bash
# Recommended: Generate plans in-house
tsuku eval kubectl > our-kubectl-plan.json
tsuku install --plan our-kubectl-plan.json
```

**Review plans before using them in production:**

```bash
# Inspect plan content for unexpected URLs
jq '.steps[] | select(.params.url) | .params.url' plan.json

# Compare against expected checksums
jq '.steps[] | select(.checksum) | .checksum' plan.json
```

**Only use plans from trusted sources:**

- Plans from your organization's release process: ✅ Safe
- Plans from official project releases: ✅ Generally safe (but verify checksums are listed on their website)
- Plans from untrusted third parties: ⚠️ Risky (could contain malicious URLs)
- Plans from the internet without verification: ❌ Not recommended

**Version control your plans:**

```bash
# Store generated plans in version control
git add kubectl-plan.json
git commit -m "update kubectl plan to 1.29.0"

# This creates an audit trail of what was installed
```

### Regenerating Plans with Updated Constraints

If you need to update a tool version or re-validate checksums:

```bash
# Regenerate the plan
tsuku eval kubectl@1.30.0 > kubectl-plan-new.json

# Review changes
diff kubectl-plan.json kubectl-plan-new.json

# Once satisfied, replace the plan
mv kubectl-plan-new.json kubectl-plan.json

# Commit the update
git add kubectl-plan.json
git commit -m "update kubectl plan to 1.30.0"
```

This workflow ensures that every plan change is reviewed and tracked before deployment.

## Troubleshooting

### Plan Validation Errors

```
Error: plan is for linux-amd64, but this system is darwin-arm64
```

Use a plan generated for your platform, or generate a new one.

### Checksum Mismatch

```
Error: checksum mismatch for file.tar.gz
  expected: sha256:abc123...
  got:      sha256:def456...
```

The upstream file changed. Regenerate the plan with `tsuku eval` and review changes.

### Invalid JSON from Stdin

```
Error: failed to parse plan from stdin
Hint: Save plan to a file first for debugging
```

When piping plans, JSON parsing errors are harder to debug. Save the plan to a file first to inspect it.

## See Also

- [Reproducible Installations](../README.md#reproducible-installations) - Plan caching overview
- [Recipe Verification Guide](GUIDE-recipe-verification.md) - How tsuku verifies installations
