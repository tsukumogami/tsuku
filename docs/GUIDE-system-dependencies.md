# System Dependencies Guide

This guide explains how tsuku handles system dependencies - packages that require installation via your system's package manager.

## What Are System Dependencies?

Some tools require system-level packages that tsuku cannot install directly because they need:

- Root/sudo privileges
- Kernel modules or drivers
- System service configuration
- Package manager integration (apt, brew, dnf, etc.)

**Examples of system dependencies:**
- Docker (requires kernel modules, system services)
- GPU drivers (requires kernel modules)
- Network tools that need raw socket access
- Tools that integrate with system services

## How Tsuku Handles System Dependencies

When you install a recipe that has system dependencies, tsuku:

1. Detects your platform (macOS, Ubuntu, Fedora, etc.)
2. Filters the relevant instructions for your system
3. Displays step-by-step instructions you can copy and run
4. Exits so you can complete the system setup
5. After you run the commands, you can retry the install

### Example Output

```
This recipe requires system dependencies for Ubuntu/Debian:

  1. sudo apt-get update && sudo apt-get install -y docker-ce docker-ce-cli containerd.io
  2. sudo usermod -aG docker $USER

After completing these steps, run the install command again.
```

## Using the --target-family Flag

By default, tsuku detects your Linux distribution family automatically. You can override this with the `--target-family` flag to see instructions for a different platform.

**Supported families:**
- `debian` - Ubuntu, Debian, Linux Mint, Pop!_OS
- `rhel` - Fedora, RHEL, CentOS, Rocky Linux, AlmaLinux
- `arch` - Arch Linux, Manjaro, EndeavourOS
- `alpine` - Alpine Linux
- `suse` - openSUSE, SLES

**Example: See instructions for Fedora while on Ubuntu:**

```bash
tsuku install docker --target-family rhel
```

**Example: See instructions for Alpine:**

```bash
tsuku install docker --target-family alpine
```

This is useful when:
- Testing recipes across distributions
- Writing documentation for multiple platforms
- Setting up tools in containers

## System Action Types

Recipes use different action types for different package managers and configurations:

### Package Installation

| Action | Package Manager | Platform |
|--------|-----------------|----------|
| `apt_install` | apt-get | Ubuntu/Debian |
| `dnf_install` | dnf | Fedora/RHEL |
| `pacman_install` | pacman | Arch Linux |
| `apk_install` | apk | Alpine |
| `zypper_install` | zypper | openSUSE |
| `brew_install` | Homebrew | macOS, Linux |
| `brew_cask` | Homebrew Cask | macOS |

### Repository Configuration

| Action | Description | Platform |
|--------|-------------|----------|
| `apt_repo` | Add APT repository | Ubuntu/Debian |
| `apt_ppa` | Add PPA repository | Ubuntu |
| `dnf_repo` | Add DNF repository | Fedora/RHEL |

### System Configuration

| Action | Description |
|--------|-------------|
| `group_add` | Add user to a group |
| `service_enable` | Enable a system service |
| `service_start` | Start a system service |

### Verification

| Action | Description |
|--------|-------------|
| `require_command` | Verify a command is available |
| `manual` | Manual installation instructions |

## Recipe Examples

### Docker Recipe (Simplified)

```toml
[metadata]
name = "docker"

# For Ubuntu/Debian
[[steps]]
action = "apt_repo"
params = { name = "docker", url = "https://download.docker.com/linux/ubuntu", key_url = "https://download.docker.com/linux/ubuntu/gpg" }
when = { linux_family = "debian" }

[[steps]]
action = "apt_install"
params = { packages = ["docker-ce", "docker-ce-cli", "containerd.io"] }
when = { linux_family = "debian" }

# For Fedora/RHEL
[[steps]]
action = "dnf_install"
params = { packages = ["docker-ce", "docker-ce-cli", "containerd.io"] }
when = { linux_family = "rhel" }

# For macOS
[[steps]]
action = "brew_cask"
params = { cask = "docker" }
when = { platform = "darwin/*" }

# Common configuration
[[steps]]
action = "group_add"
params = { group = "docker" }
when = { os = "linux" }

# Verification
[[steps]]
action = "require_command"
params = { command = "docker", message = "Docker should now be available" }
```

When you run `tsuku install docker` on Ubuntu, you'll see:

```
This recipe requires system dependencies for Ubuntu/Debian:

  1. Add Docker repository
  2. sudo apt-get update && sudo apt-get install -y docker-ce docker-ce-cli containerd.io
  3. sudo usermod -aG docker $USER

After installation, verify with:
  docker --version

After completing these steps, run the install command again.
```

## Quiet Mode

The `--quiet` flag suppresses system dependency instructions:

```bash
tsuku install docker --quiet
```

In quiet mode, tsuku only shows errors. This is useful for scripted installations where you've already handled system dependencies.

## Troubleshooting

### "Invalid target-family" Error

```
Error: invalid target-family "gentoo", must be one of: debian, rhel, arch, alpine, suse
```

You specified an unsupported family. Use one of the supported values listed above.

### Instructions Don't Match My Distribution

If the displayed instructions don't match your distribution:

1. Check which family your distribution belongs to
2. Use `--target-family` to specify the correct family
3. If your distribution isn't supported, report it as an issue

### Dependencies Already Installed

If you've already installed the system dependencies, simply run the install command again. Tsuku will continue with the non-system parts of the installation.

### Service Won't Start

If a service action fails:

1. Check system logs: `journalctl -xe`
2. Verify the package installed correctly
3. Some services require a reboot or re-login

## Related Documentation

- [Actions and Primitives Guide](GUIDE-actions-and-primitives.md) - Complete list of available actions
- [Library Dependencies Guide](GUIDE-library-dependencies.md) - How tsuku handles library dependencies
- [System Dependency Actions Design](DESIGN-system-dependency-actions.md) - Technical design document

## Summary

System dependencies are packages that require your system's package manager. Tsuku:

- Detects your platform automatically
- Shows platform-specific instructions
- Supports `--target-family` for overriding detection
- Respects `--quiet` to suppress output

After running the displayed commands, retry your installation.
