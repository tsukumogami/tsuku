# Transparent Development Environment Isolation

**Research Goal:** Find approaches where developers (or agents) can build and run `./tsuku install cmake` without special invocation syntax, while automatically getting isolation so their real `~/.tsuku` installation isn't affected.

**Date:** 2026-01-29

## Overview

This research explores six approaches to transparent development environment isolation, assessing each for transparency, setup requirements, agent compatibility, and support for parallel checkouts.

## Approach 1: direnv

### How It Works

direnv is a shell extension that automatically loads and unloads environment variables based on the current directory. Before each shell prompt, direnv checks for a `.envrc` file in the current and parent directories. If found and authorized, it loads variables into a bash sub-shell and exports them to the current shell.

**Implementation for tsuku:**
```bash
# .envrc in project root
export TSUKU_HOME="$(pwd)/.tsuku-dev"
```

When you `cd` into the project directory, `TSUKU_HOME` is automatically set. When you leave, it's unset.

### Transparency Assessment

**Pros:**
- Completely transparent after initial setup - just `cd` into directory and run `./tsuku install cmake`
- Works with any command or tool invocation
- No wrapper scripts or special build steps needed
- Variables are properly scoped to the directory

**Cons:**
- Requires direnv installation on the system
- First-time setup requires explicit authorization: `direnv allow`
- Requires shell integration (hook in `.bashrc`, `.zshrc`, etc.)

### Setup Requirements

**One-time system setup:**
1. Install direnv: `apt install direnv` (or equivalent)
2. Add hook to shell rc file: `eval "$(direnv hook bash)"`
3. Restart shell or source rc file

**Per-project setup:**
1. Create `.envrc` file in project root
2. Run `direnv allow` to authorize it

### Agent Compatibility

**Good for agents that:**
- Can install system packages (for direnv)
- Can modify shell configuration files
- Use interactive shells (direnv hooks run on prompt)

**Challenges:**
- Agents using non-interactive shells might not trigger direnv hooks
- `go run` from a script might not inherit environment unless the script itself is run in direnv context
- Automated CI/CD environments need direnv installed and configured

### Parallel Checkouts

**Excellent support:**
- Each checkout has its own `.envrc` with its own `TSUKU_HOME` path
- `$(pwd)` in `.envrc` ensures each checkout gets a unique directory
- No collision risk - complete isolation per checkout

**Example:**
```bash
# Checkout 1: /home/dev/tsuku-main/.envrc
export TSUKU_HOME="$(pwd)/.tsuku-dev"  # -> /home/dev/tsuku-main/.tsuku-dev

# Checkout 2: /home/dev/tsuku-feature/.envrc
export TSUKU_HOME="$(pwd)/.tsuku-dev"  # -> /home/dev/tsuku-feature/.tsuku-dev
```

### Sources
- [Discovering direnv | Redowan's Reflections](https://rednafi.com/misc/direnv/)
- [direnv.net official site](https://direnv.net/)
- [Never worry about environment variables again with direnv](https://dev.to/turck/never-worry-about-environment-variables-again-with-direnv-1h17)

## Approach 2: Build-Time Defaults via -ldflags

### How It Works

Go's `-ldflags` flag allows setting string variable values at compile time. You can override a variable's default value when building the binary.

**Implementation for tsuku:**
```go
// internal/config/config.go
package config

import "os"

var defaultHome = "~/.tsuku"  // Production default

func GetTsukuHome() string {
    if home := os.Getenv("TSUKU_HOME"); home != "" {
        return home
    }
    return defaultHome  // Can be overridden at build time
}
```

**Build commands:**
```bash
# Development build - defaults to ./.tsuku-dev
go build -ldflags="-X 'github.com/tsuku/internal/config.defaultHome=./.tsuku-dev'" -o tsuku ./cmd/tsuku

# Production build - defaults to ~/.tsuku
go build -o tsuku ./cmd/tsuku
```

### Transparency Assessment

**Pros:**
- Completely transparent once built - just run `./tsuku install cmake`
- No runtime overhead or wrapper scripts
- Binary itself knows whether it's a dev or prod build
- `TSUKU_HOME` environment variable still works to override

**Cons:**
- Different binaries for dev vs prod (could accidentally ship dev binary)
- Need to remember different build commands or use Makefile
- Relative paths (`./.tsuku-dev`) could be problematic if binary is run from different directory
- Less transparent than direnv because you need to use the right build command

### Setup Requirements

**Per-project setup:**
1. Add Makefile or build script with correct ldflags
2. Build using the dev target: `make build-dev` or `./build-dev.sh`

**Example Makefile:**
```makefile
.PHONY: build build-dev

build:
	go build -o tsuku ./cmd/tsuku

build-dev:
	go build -ldflags="-X 'github.com/tsuku/internal/config.defaultHome=$$(pwd)/.tsuku-dev'" -o tsuku ./cmd/tsuku
```

### Agent Compatibility

**Excellent for agents:**
- Standard Go toolchain, no additional dependencies
- Agents can run `make build-dev` as a standard build step
- Works in any environment (interactive or non-interactive)
- Compatible with `go run` if you pass ldflags: `go run -ldflags="..." ./cmd/tsuku`

**Challenges:**
- Agents need to know to use dev build command
- `go run` without ldflags uses production defaults

### Parallel Checkouts

**Good with absolute paths:**
```bash
# Build in each checkout with absolute path
cd /home/dev/tsuku-main
go build -ldflags="-X 'config.defaultHome=/home/dev/tsuku-main/.tsuku-dev'" -o tsuku ./cmd/tsuku

cd /home/dev/tsuku-feature
go build -ldflags="-X 'config.defaultHome=/home/dev/tsuku-feature/.tsuku-dev'" -o tsuku ./cmd/tsuku
```

**Poor with relative paths:**
- If using `./.tsuku-dev`, binaries are not portable across directories
- Moving the binary breaks the relative path assumption

### Sources
- [Using ldflags to Set Version Information for Go Applications | DigitalOcean](https://www.digitalocean.com/community/tutorials/using-ldflags-to-set-version-information-for-go-applications)
- [Build-Time Variables in Go | belief driven design](https://belief-driven-design.com/build-time-variables-in-go-51439b26ef9/)
- [Setting Go variables from the outside](https://blog.cloudflare.com/setting-go-variables-at-compile-time/)

## Approach 3: Wrapper Script

### How It Works

Create a shell script named `tsuku` that sets `TSUKU_HOME` and then executes the real binary.

**Implementation:**
```bash
#!/bin/bash
# tsuku (wrapper script in project root)

# Set TSUKU_HOME to dev directory
export TSUKU_HOME="$(dirname "$0")/.tsuku-dev"

# Execute the real binary with all arguments
exec "$(dirname "$0")/tsuku-real" "$@"
```

**Build process:**
```bash
# Build the real binary
go build -o tsuku-real ./cmd/tsuku

# Make wrapper executable
chmod +x tsuku
```

### Transparency Assessment

**Pros:**
- Transparent to use - just run `./tsuku install cmake`
- Simple to implement and understand
- Easy to add additional dev-specific setup (logging, debugging flags, etc.)

**Cons:**
- Creates an extra file (`tsuku-real`)
- Slightly less clean than a single binary
- Could accidentally ship wrapper instead of real binary
- Small performance overhead of shell script execution (negligible)

### Setup Requirements

**Per-project setup:**
1. Create wrapper script
2. Modify build process to create `tsuku-real` instead of `tsuku`
3. Make wrapper executable

**Example Makefile:**
```makefile
.PHONY: build

build:
	go build -o tsuku-real ./cmd/tsuku
	chmod +x tsuku
	@echo "Built tsuku wrapper (points to .tsuku-dev)"
```

### Agent Compatibility

**Excellent for agents:**
- No special dependencies beyond standard shell
- Works in any environment
- Agents can create wrapper script as part of setup
- Compatible with all test frameworks and tools

**Example agent workflow:**
```bash
git clone tsuku
cd tsuku
make build          # Creates tsuku-real and tsuku wrapper
./tsuku install gh  # Automatically uses .tsuku-dev
```

### Parallel Checkouts

**Excellent support:**
- Each checkout has its own wrapper that uses `$(dirname "$0")`
- Automatically resolves to the correct directory per checkout
- No collision possible

### Sources
- [Shell Script Wrapper Examples | nixCraft](https://www.cyberciti.biz/tips/unix-linux-bash-shell-script-wrapper-examples.html)
- [Shell-Script Wrappers - Solaris 64-bit Developer's Guide](https://docs.oracle.com/cd/E18752_01/html/816-5138/dev-env-13.html)
- [Shell Wrappers | Advanced Bash-Scripting Guide](https://tldp.org/LDP/abs/html/wrapper.html)

## Approach 4: Runtime .git Detection

### How It Works

The tsuku binary detects at runtime whether it's running from a git repository (by checking for `.git` directory). If so, it automatically uses a dev directory instead of the production home.

**Implementation:**
```go
package config

import (
    "os"
    "path/filepath"
)

func GetTsukuHome() string {
    // Explicit env var takes precedence
    if home := os.Getenv("TSUKU_HOME"); home != "" {
        return home
    }

    // Check if we're in a development environment (git repo)
    if isDevEnvironment() {
        execPath, _ := os.Executable()
        execDir := filepath.Dir(execPath)
        return filepath.Join(execDir, ".tsuku-dev")
    }

    // Production default
    return "~/.tsuku"
}

func isDevEnvironment() bool {
    execPath, err := os.Executable()
    if err != nil {
        return false
    }

    dir := filepath.Dir(execPath)

    // Walk up directory tree looking for .git
    for {
        gitPath := filepath.Join(dir, ".git")
        if _, err := os.Stat(gitPath); err == nil {
            return true
        }

        parent := filepath.Dir(dir)
        if parent == dir {
            break  // Reached root
        }
        dir = parent
    }

    return false
}
```

### Transparency Assessment

**Pros:**
- Completely transparent - no special build steps, scripts, or env vars needed
- Single binary works for both dev and prod
- Automatic detection, zero configuration
- Just run `./tsuku install cmake` and it works

**Cons:**
- "Magic" behavior might be surprising
- Could cause issues if someone has a git repo in their home directory
- Less explicit than other approaches
- Might detect .git in unexpected places

### Setup Requirements

**No setup required:**
- Just build with `go build -o tsuku ./cmd/tsuku`
- Binary automatically detects environment

### Agent Compatibility

**Perfect for agents:**
- Zero setup beyond building the binary
- Works automatically when agents clone repos
- No environment variables to set
- No shell hooks to configure
- Completely portable

**Example agent workflow:**
```bash
git clone tsuku
cd tsuku
go build -o tsuku ./cmd/tsuku
./tsuku install gh  # Automatically uses .tsuku-dev because .git exists
```

### Parallel Checkouts

**Excellent support:**
- Each checkout has its own `.git` directory
- Each binary automatically uses its own directory
- Complete isolation with zero configuration

### Considerations

**When .git detection fails:**
- Binary installed to `/usr/local/bin` won't find `.git` - correctly uses prod default
- Git worktrees might have `.git` file instead of directory - needs handling
- Shallow clones work fine - still have `.git` directory

**Enhancement - check for build tags:**
```go
// Use build tags to force dev mode
//go:build dev

var forceDev = true
```

Build with: `go build -tags dev -o tsuku ./cmd/tsuku`

### Sources
- [Git - git-config Documentation](https://git-scm.com/docs/git-config)
- [Customize git for all projects in a directory](https://alysivji.com/multiple-gitconfig-files.html)

## Approach 5: devcontainer

### How It Works

Visual Studio Code Dev Containers use a `.devcontainer/devcontainer.json` file to define a complete development environment in a Docker container. The configuration specifies the Docker image, environment variables, extensions, and settings.

**Implementation for tsuku:**
```json
// .devcontainer/devcontainer.json
{
    "name": "Tsuku Development",
    "image": "golang:1.22",
    "containerEnv": {
        "TSUKU_HOME": "/workspaces/tsuku/.tsuku-dev"
    },
    "mounts": [
        "source=${localWorkspaceFolder}/.tsuku-dev,target=/workspaces/tsuku/.tsuku-dev,type=bind"
    ],
    "postCreateCommand": "go build -o tsuku ./cmd/tsuku",
    "customizations": {
        "vscode": {
            "extensions": ["golang.go"]
        }
    }
}
```

### Transparency Assessment

**Pros:**
- Environment is completely isolated from host system
- Consistent across all developers and platforms
- Variables are automatically set when container starts
- All dependencies can be pre-installed in container
- Can run `./tsuku install cmake` normally inside container

**Cons:**
- Requires VS Code or compatible editor
- Requires Docker installation
- Not transparent for command-line only workflows
- Container overhead (startup time, disk space)
- Only works when using the devcontainer

### Setup Requirements

**One-time system setup:**
1. Install Docker
2. Install VS Code with Dev Containers extension

**Per-project setup:**
1. Create `.devcontainer/devcontainer.json` configuration
2. Open project in VS Code
3. Click "Reopen in Container"

### Agent Compatibility

**Mixed compatibility:**

**Works well for:**
- Claude Code with VS Code integration
- GitHub Codespaces (cloud-based devcontainers)
- Agents that can work within VS Code environment

**Challenges:**
- Command-line only agents can't use devcontainer directly
- Requires Docker and VS Code, which limits portability
- Some agents might not be able to "Reopen in Container"
- Can use Docker directly but loses transparency benefits

**Alternative for CLI agents:**
```bash
# Agent can build and run container manually
docker build -t tsuku-dev .devcontainer
docker run -it -v $(pwd):/workspace tsuku-dev bash
# Inside container:
./tsuku install cmake  # Uses TSUKU_HOME from container env
```

### Parallel Checkouts

**Good support with containers:**
- Each checkout can have its own container instance
- Containers are isolated from each other
- Mount points ensure each container uses its checkout's directory

**Challenges:**
- Higher resource usage (multiple containers)
- Need to manage container lifecycle for each checkout
- Port conflicts if services listen on same ports

### Sources
- [Developing inside a Container | VS Code](https://code.visualstudio.com/docs/devcontainers/containers)
- [Ultimate Guide to Dev Containers](https://www.daytona.io/dotfiles/ultimate-guide-to-dev-containers)
- [Development containers - Claude Code Docs](https://code.claude.com/docs/en/devcontainer)
- [How to Standardize Your Development Environment with devcontainer.json](https://www.freecodecamp.org/news/standardize-development-environment-with-devcontainers/)

## Approach 6: Makefile with eval Pattern

### How It Works

Use a Makefile to wrap common development commands with environment variables. This requires using a special invocation pattern like `make dev-install ARGS="cmake"`.

**Implementation:**
```makefile
# Makefile
TSUKU_DEV_HOME := $(shell pwd)/.tsuku-dev

.PHONY: dev-install dev-remove dev-list

dev-install:
	TSUKU_HOME=$(TSUKU_DEV_HOME) ./tsuku install $(ARGS)

dev-remove:
	TSUKU_HOME=$(TSUKU_DEV_HOME) ./tsuku remove $(ARGS)

dev-list:
	TSUKU_HOME=$(TSUKU_DEV_HOME) ./tsuku list

# Shell helper for manual commands
.PHONY: dev-env
dev-env:
	@echo "export TSUKU_HOME=$(TSUKU_DEV_HOME)"
```

**Usage:**
```bash
# Via make targets
make dev-install ARGS="cmake"

# Via eval pattern
eval $(make dev-env)
./tsuku install cmake  # Now transparent
```

### Transparency Assessment

**Pros:**
- Familiar pattern for developers
- Can add additional dev-specific logic in Makefile
- Works with standard tools (make is widely available)

**Cons:**
- **Not transparent** - requires special invocation: `make dev-install ARGS="cmake"`
- **eval pattern is awkward** - `eval $(make dev-env)` is not intuitive
- **Shell-specific** - eval pattern doesn't persist across shell sessions
- Passing arguments through make is clunky
- Can't just run `./tsuku install cmake`

### Setup Requirements

**Per-project setup:**
1. Create Makefile with dev targets
2. Remember to use `make dev-install` instead of `./tsuku install`

### Agent Compatibility

**Moderate compatibility:**
- Agents can run make commands
- Standard tool, no special dependencies
- But requires agents to know to use make targets instead of direct invocation

**Challenges:**
- Documentation needs to tell agents "use make dev-install, not ./tsuku install"
- Less discoverable than other approaches
- Eval pattern requires agent to understand shell evaluation

### Parallel Checkouts

**Good support:**
- Each checkout has its own Makefile
- `$(shell pwd)` resolves to correct directory per checkout
- No collision risk

### Sources
- [Variables/Recursion (GNU make)](https://www.gnu.org/software/make/manual/html_node/Variables_002fRecursion.html)
- [Makefile Tutorial by Example](https://makefiletutorial.com/)
- [Understanding and Using Makefile Variables | Earthly Blog](https://earthly.dev/blog/makefile-variables/)

## Comparison Matrix

| Approach | Transparency | Setup Complexity | Agent Compatible | Parallel Checkouts | Production Risk |
|----------|--------------|------------------|------------------|-------------------|----------------|
| **direnv** | ⭐⭐⭐⭐⭐ Transparent | Medium (system install + shell hook) | ⭐⭐⭐ Good (if shell integration works) | ⭐⭐⭐⭐⭐ Excellent | Low (requires .envrc) |
| **Build-time ldflags** | ⭐⭐⭐⭐ Good | Low (just build flags) | ⭐⭐⭐⭐⭐ Excellent | ⭐⭐⭐⭐ Good | Medium (could ship dev binary) |
| **Wrapper script** | ⭐⭐⭐⭐⭐ Transparent | Low (create script) | ⭐⭐⭐⭐⭐ Excellent | ⭐⭐⭐⭐⭐ Excellent | Medium (could ship wrapper) |
| **Runtime .git detection** | ⭐⭐⭐⭐⭐ Transparent | None (zero config) | ⭐⭐⭐⭐⭐ Excellent | ⭐⭐⭐⭐⭐ Excellent | Very Low (auto-detects) |
| **devcontainer** | ⭐⭐⭐ OK (inside container) | High (Docker + VS Code) | ⭐⭐ Limited | ⭐⭐⭐ Good | Low (separate environment) |
| **Makefile + eval** | ⭐ Poor | Low (create Makefile) | ⭐⭐⭐ Moderate | ⭐⭐⭐⭐ Good | Low (requires make command) |

## Lessons from Other Ecosystems

### Rust Cargo: CARGO_TARGET_DIR

Rust's Cargo respects `CARGO_TARGET_DIR` environment variable to control where build artifacts go. This is similar to `TSUKU_HOME` but for build outputs rather than installed tools.

**Key insight:** Environment variables are the standard way to override defaults. Cargo doesn't try to auto-detect or use wrapper scripts - it relies on explicit configuration through env vars or config files.

**Source:** [Environment Variables - The Cargo Book](https://doc.rust-lang.org/cargo/reference/environment-variables.html)

### Python Poetry: .venv in Project

Poetry can be configured to create virtual environments in the project directory (`.venv`) instead of a global cache directory.

**Configuration:**
```bash
poetry config virtualenvs.in-project true
```

**Key insight:** Python ecosystem doesn't auto-activate environments either. Developers must:
- Run `poetry shell` to activate manually
- Run commands with `poetry run` prefix
- Use direnv or similar tools for auto-activation

Tools like direnv are commonly used in Python projects for this exact use case.

**Sources:**
- [Managing environments | Poetry](https://python-poetry.org/docs/managing-environments/)
- [Configuration | Poetry](https://python-poetry.org/docs/configuration/)

## Top Recommendations

### 1st Choice: Runtime .git Detection (Best Overall)

**Why:**
- Zero setup required - completely automatic
- Perfect transparency - just run `./tsuku install cmake`
- Excellent agent compatibility - works immediately after `go build`
- Perfect parallel checkout support
- Lowest production risk - automatically detects environment
- Single binary for both dev and prod

**Implementation:**
```go
func GetTsukuHome() string {
    if home := os.Getenv("TSUKU_HOME"); home != "" {
        return home  // Explicit override always wins
    }

    if isInGitRepo() {
        // Development mode: use .tsuku-dev in binary's directory
        execPath, _ := os.Executable()
        return filepath.Join(filepath.Dir(execPath), ".tsuku-dev")
    }

    // Production mode: use ~/.tsuku
    return filepath.Join(os.Getenv("HOME"), ".tsuku")
}
```

**Concerns:**
- "Magic" behavior could be surprising to users
- Mitigation: Document clearly, make TSUKU_HOME override obvious
- Edge case: Git worktrees use `.git` file not directory
- Mitigation: Check if `.git` is a file and read it to find real git directory

### 2nd Choice: Wrapper Script (Best for Explicit Control)

**Why:**
- Complete transparency in usage
- Very simple to implement and understand
- Excellent agent compatibility
- Perfect parallel checkout support
- Easy to debug and modify

**When to prefer over .git detection:**
- You want explicit, visible dev environment setup
- You need to add other dev-specific configuration
- You want to avoid "magic" behavior
- You want to be able to grep for "TSUKU_HOME" and see where it's set

**Implementation:**
```bash
#!/bin/bash
# tsuku (wrapper script)
export TSUKU_HOME="$(dirname "$0")/.tsuku-dev"
exec "$(dirname "$0")/tsuku-real" "$@"
```

### 3rd Choice: direnv (Best for Multi-Tool Projects)

**Why:**
- Industry standard for project-scoped environment variables
- Works for all tools, not just tsuku
- Very transparent once set up
- Good documentation and community support

**When to prefer:**
- Project needs many environment variables (database URLs, API keys, etc.)
- Team is already using direnv for other projects
- You want environment isolation for multiple tools simultaneously
- You're comfortable with the setup requirements

**Concerns:**
- Requires system installation and shell configuration
- First-time setup friction for new developers/agents
- Mitigation: Provide clear setup instructions in README

## Recommendation Summary

For tsuku specifically, **runtime .git detection** is the best approach because:

1. It requires zero setup - clone, build, run
2. It's completely transparent - no special commands
3. It's perfect for agents - they can just `go build && ./tsuku install tool`
4. It supports parallel checkouts automatically
5. It has the lowest production risk - automatically uses prod defaults when not in a git repo

The wrapper script is a close second and might be preferred if you want more explicit control or need to avoid "magic" behavior.

Direnv is excellent but requires more setup. It's better suited for complex projects that need many environment variables, not just `TSUKU_HOME`.

## Implementation Notes

Whichever approach is chosen, document it clearly:

```markdown
## Development

When you run tsuku from within a git repository, it automatically uses
`.tsuku-dev` in the project directory instead of `~/.tsuku`. This means
you can test changes without affecting your real installation.

To override this behavior, set TSUKU_HOME explicitly:
    TSUKU_HOME=~/.tsuku ./tsuku install cmake

To force development mode outside a git repo:
    TSUKU_HOME=./.tsuku-dev ./tsuku install cmake
```
