# Python venv/virtualenv vs tsuku --env: A Structural Comparison

## Executive Summary

Python's venv creates isolated package environments that share the base interpreter and standard library while isolating third-party packages. The tsuku --env proposal creates isolated tool installations that share the download cache while isolating installed tools and state. Structurally, they're similar isolation mechanisms. The critical difference is audience: venv is a core end-user workflow essential for Python development, while --env is explicitly designed for tsuku contributors and CI. This difference in audience positioning is decisive when evaluating whether the feature belongs in-binary.

venv's history from third-party tool (virtualenv, 2007) to stdlib inclusion (Python 3.3, 2012) demonstrates that isolation became essential enough to warrant first-class support. However, the five-year gap between virtualenv's creation and stdlib adoption suggests the stdlib integration happened after isolation became a proven, universal workflow need. The tsuku --env proposal starts from a more limited scope: contributor tooling, not universal end-user workflow.

## What Python venv Actually Does

### Core Mechanism

Python's venv module creates lightweight virtual environments with their own site-packages directories while sharing the base Python interpreter and standard library. When you create a venv, Python constructs a directory structure that isolates third-party packages without duplicating the entire Python installation.

### Directory Structure

```
myenv/                              # Virtual environment root
├── pyvenv.cfg                      # Config file pointing to base Python
├── bin/                            # (Scripts/ on Windows)
│   ├── python -> /usr/bin/python3  # Symlink to base interpreter
│   ├── pip                         # Environment-specific pip
│   └── activate                    # Shell activation script
├── lib/
│   └── python3.x/
│       └── site-packages/          # Isolated third-party packages
└── include/                        # C headers for package compilation
```

**Key components:**

1. **pyvenv.cfg**: Contains `home = /path/to/base/python` pointing to the base installation
2. **bin/python**: Symlink or copy of the Python interpreter binary
3. **site-packages/**: Isolated directory for third-party packages (`pip install`)
4. **activate script**: Modifies `$PATH` to prepend the venv's bin/ directory

### What's Isolated

- **Third-party packages**: All packages installed via pip go into the venv's site-packages
- **Package versions**: Different venvs can have different versions of the same package
- **pip state**: Each venv tracks its own installed packages independently
- **Installed binaries**: Command-line tools installed by packages live in the venv's bin/

### What's Shared

- **Python interpreter binary**: The actual python executable is shared (via symlink or copy)
- **Standard library**: All stdlib modules (os, sys, json, etc.) come from the base installation
- **Python version**: A venv can't use a different Python version than its base interpreter

### Activation Model

venvs require explicit activation to take effect:

```bash
# Create venv
python3 -m venv myenv

# Activate (modifies $PATH for the session)
source myenv/bin/activate

# Now 'python' and 'pip' resolve to the venv's versions
python --version
pip install requests

# Deactivate (restores original $PATH)
deactivate
```

Activation is stateful and session-scoped. Once activated, all subsequent commands in that shell session use the venv's packages.

## Structural Comparison: venv vs tsuku --env

### Directory Layout

**Python venv:**
```
myenv/
├── pyvenv.cfg          # Config pointing to base Python
├── bin/                # Isolated executables
├── lib/site-packages/  # Isolated packages
└── [shared interpreter via symlink]
```

**tsuku --env:**
```
$TSUKU_HOME/envs/dev/
├── state.json          # Isolated installation state
├── tools/              # Isolated tool installations
├── bin/                # Isolated binary symlinks
├── registry/           # Isolated registry cache
└── [shared download cache via config-level override]
```

Both create a parallel directory tree that isolates "what's installed" while sharing expensive-to-duplicate content (interpreter binary vs download cache).

### What's Isolated vs Shared

| Aspect | Python venv | tsuku --env |
|--------|-------------|-------------|
| **Isolated** | Third-party packages, pip state, installed binaries | Installed tools, state.json, tool versions, binary symlinks |
| **Shared** | Base interpreter, standard library | Download cache (bottles, tarballs) |
| **Why shared** | Interpreter is large and identical across venvs | Downloaded files are content-addressed and identical across envs |
| **Sharing mechanism** | Symlink to base python binary | Config-level override of DownloadCacheDir |

### Activation Model

**Python venv:**
- Requires explicit activation (`source venv/bin/activate`)
- Modifies shell environment ($PATH, $VIRTUAL_ENV)
- Session-scoped: active until deactivate or shell exit
- Visual indicator via shell prompt modification

**tsuku --env:**
- Per-command flag (`tsuku --env dev install cmake`)
- Or session-wide env var (`export TSUKU_ENV=dev`)
- No PATH modification, operates at config layer
- Visual indicator via command output (`[env: dev] Installing cmake`)

venv's activation is more ceremonial and persistent (you're "in" a venv for the whole session). tsuku --env can be per-command or per-session depending on flag vs env var usage.

### Discovery and Management

**Python venv:**
- No built-in discovery (venvs are just directories)
- Management is manual: `rm -rf myenv/` to delete
- Activation script provides user feedback

**tsuku --env:**
- Discovery via `tsuku env list` (shows all named environments)
- Management via `tsuku env clean <name>` (locked cleanup)
- `tsuku env info <name>` shows disk usage and installed tools

tsuku --env is more discoverable because it's managed by the CLI. venvs are opaque directories from Python's perspective.

## Key Differences

### 1. Audience and Use Case

**Python venv:**
- **Audience**: All Python developers, from beginners to experts
- **Use case**: Isolating project dependencies to avoid conflicts
- **Frequency**: Used constantly, every Python project needs at least one venv
- **User type**: End users, not Python core developers

**tsuku --env:**
- **Audience**: tsuku contributors and CI workflows (explicitly stated)
- **Use case**: Testing CLI changes and recipes without breaking real installation
- **Frequency**: Used during development and testing, not daily user workflow
- **User type**: Contributors, not end users

This is the decisive difference. venv is a fundamental part of the Python development workflow. Every Python developer uses venvs to manage dependencies. tsuku --env is explicitly positioned as a contributor tool, not a core user workflow.

### 2. Maturity and Adoption Trajectory

**Python venv/virtualenv timeline:**
- 2007: Ian Bicking creates virtualenv as third-party tool
- 2007-2012: virtualenv becomes essential, "rapidly became an essential utility"
- 2012: PEP 405 proposes adding venv to Python 3.3 stdlib
- 2012-present: venv is the official recommendation, virtualenv continues for advanced features

**tsuku --env timeline:**
- Currently: Proposal stage, not yet implemented
- Use case: Already solved ad-hoc by contributors via manual TSUKU_HOME export
- CI use case: Already solved by Build Essentials tests via temp directories

virtualenv existed as a third-party tool for five years before stdlib inclusion. During that time, it proved its value across the entire Python ecosystem. The stdlib adoption came after isolation became a universal workflow need, not a contributor-only convenience.

tsuku --env is being proposed before the workflow has been proven outside ad-hoc manual solutions. There's no established external tool that tsuku would be "adopting" into the binary.

### 3. In-Binary vs External Tool

**Python venv:**
- Lives in stdlib (in-binary equivalent for an interpreter)
- Rationale per PEP 405: "The utility of Python virtual environments has already been well established by the popularity of existing third-party virtual-environment tools"
- Decision: Add to stdlib because it's universally needed and reduces friction

**tsuku --env:**
- Proposed as in-binary feature (--env flag + TSUKU_ENV)
- Rationale: Low ceremony, automatic cache sharing, discoverability
- Alternative: External wrapper script or manual TSUKU_HOME export

The question for tsuku --env is whether contributor convenience justifies in-binary complexity when external solutions exist.

### 4. Scope of Isolation

**Python venv:**
- Isolates the entire dependency graph of a project
- Prevents global pollution (no `sudo pip install`)
- Enables reproducible builds across machines
- Security boundary: different projects can have conflicting dependencies

**tsuku --env:**
- Isolates installed tools for testing purposes
- Prevents polluting contributor's working installation
- Enables parallel CI jobs without lock contention
- Development boundary: separate what you're testing from what you use daily

venv provides a security and reproducibility boundary. tsuku --env provides a development convenience boundary.

## Does the Analogy Strengthen or Weaken the Case for In-Binary?

### Arguments Strengthened

1. **Proven Pattern**: venv demonstrates that isolation-while-sharing is a well-understood architecture. The structural similarity (isolated state + shared expensive content) suggests tsuku --env isn't novel or risky.

2. **Low Ceremony Matters**: PEP 405's rationale explicitly calls out reducing friction as a reason for stdlib inclusion. The tsuku --env design doc makes the same argument: manual TSUKU_HOME export is tedious and error-prone.

3. **Discoverability**: venv's inclusion in stdlib made virtual environments discoverable via `python -m venv`. Similarly, `--env` makes environments discoverable via `tsuku --help` and `tsuku env list`. External wrappers are invisible.

4. **Cache Sharing**: venv shares the interpreter to avoid duplication. tsuku --env shares the download cache. Both solve "don't make me download/store the same thing N times."

### Arguments Weakened

1. **Audience Gap**: venv is for all Python users. tsuku --env is for contributors. This is the critical difference. PEP 405 justified stdlib inclusion because virtual environments were "already widely used for dependency management." tsuku --env is explicitly not a general-purpose dependency management feature.

2. **Five-Year Proof Period**: virtualenv existed as an external tool from 2007 to 2012 before stdlib adoption. It proved universal value before being integrated. tsuku --env hasn't been proven as an external tool first. There's no "tsuku-env" wrapper that the community is clamoring to upstream.

3. **External Alternative Exists**: Python's stdlib decision assumed no external tool would always be available. tsuku runs as a standalone binary, so a wrapper script is viable. Contributors could use a `tsuku-dev` script that sets TSUKU_HOME to a temp directory and symlinks the cache. This is higher ceremony but demonstrates the workflow before committing to in-binary support.

4. **Scope Creep Risk**: venv isolation is core to Python's value prop (avoiding dependency conflicts). Tool isolation for tsuku is a testing convenience, not a fundamental user need. Adding it in-binary sets a precedent that contributor QoL features belong in the binary, not in external tooling.

### Net Effect

The analogy is structurally supportive (the architecture is proven) but strategically neutral-to-negative (the audience and maturity differences are significant).

If tsuku --env were for end users managing project-specific toolchains, the venv analogy would be strong. As a contributor-focused feature, the analogy highlights what's missing: the five-year proof period and universal adoption that justified venv's stdlib inclusion.

## What venv's Adoption Trajectory Tells Us

### Timeline and Rationale

1. **2007**: Ian Bicking creates virtualenv to solve a real, urgent problem (dependency conflicts in Python projects)
2. **2007-2012**: virtualenv becomes the de facto standard, used across the ecosystem
3. **2012**: PEP 405 proposes stdlib inclusion, citing "well established" utility and "widespread" use
4. **Result**: venv added to Python 3.3 stdlib as a simplified subset of virtualenv

### Key Insights

**Prove value externally first.** virtualenv wasn't proposed for stdlib inclusion immediately. It lived as a third-party tool for five years, during which it proved indispensable to the community. The stdlib adoption was recognition of existing universal usage, not speculation about future utility.

**Universal need, not niche convenience.** PEP 405's rationale emphasizes "dependency management," "ease of installing packages without system-administrator access," and "automated testing across multiple Python versions." These are broad, ecosystem-wide needs, not contributor-only conveniences.

**Subset adoption, not full feature parity.** Python's venv is a simplified version of virtualenv. The stdlib didn't adopt virtualenv's full feature set (e.g., creating venvs for arbitrary Python versions). It adopted the 80% use case that applied universally.

**Reduced external dependency.** Before venv, every Python developer had to `pip install virtualenv` as a prerequisite for real work. After venv, it's built-in. This matters for beginner onboarding and universal availability.

### Applying to tsuku --env

**Prove externally first:** There's no `tsuku-env` wrapper tool that contributors use and love. The current solution is manual `TSUKU_HOME` export, which contributors tolerate but don't evangelize. A wrapper script could prove the workflow before in-binary integration.

**Universal vs niche:** The design doc explicitly states "the primary audience is tsuku contributors and CI workflows." This is the opposite of venv's universal audience. If --env were essential for project-specific tool isolation (e.g., "ensure my CI uses cmake 3.25 exactly"), it would match venv's scope. As a development convenience, it doesn't.

**Subset adoption:** If tsuku were to adopt environment support, what's the 80% use case? The design doc proposes `--env` flag + `TSUKU_ENV` + `tsuku env list/clean/info`. That's a full management interface, not a minimal subset. venv didn't add `python -m venv list` or `python -m venv clean`. It added creation and activation, leaving management to the filesystem.

**Reduced external dependency:** Contributors already have tsuku (they're developing it). Adding --env doesn't reduce an external dependency; it internalizes a convenience feature. The analogy to venv's "no more pip install virtualenv" doesn't apply.

## Structural Alignment vs Strategic Misalignment

### Structural Alignment

The tsuku --env proposal is architecturally sound. It follows the same isolation-while-sharing pattern as venv, uses well-established UX patterns (--context flags, env vars, list/clean commands), and solves real friction (manual TSUKU_HOME juggling, CI lock contention).

If you asked "can this be built like venv?" the answer is yes. The design is coherent and the implementation is straightforward.

### Strategic Misalignment

The tsuku --env proposal is strategically misaligned with venv's adoption trajectory. venv was added to Python after five years of external validation, universal ecosystem adoption, and demonstrated indispensability. tsuku --env is being proposed before external validation, for a contributor-only audience, and as a convenience layer over an existing (if tedious) solution.

The design doc's rejection rationale for Option 2 (standalone TSUKU_HOME + config) is telling:

> "Option 2 was rejected because it formalizes the status quo rather than solving the problem. The only addition (a config key for shared downloads) doesn't remove enough friction."

This frames the decision as friction reduction vs status quo, not essential workflow vs nice-to-have. venv wasn't about reducing friction; it was about making Python development viable at all. Without venv (or virtualenv), Python projects would have unsolvable dependency conflicts. Without tsuku --env, contributors have a tedious workflow.

## Alternative Trajectories

### Path 1: External Wrapper Script (Prove First)

Create a `tsuku-dev` wrapper script in the repository:

```bash
#!/bin/bash
# tsuku-dev: Run tsuku in a throwaway environment

ENV_NAME="${1:-dev}"
TSUKU_HOME="${TSUKU_DEV_HOME:-$HOME/.tsuku-dev}"
ENV_DIR="$TSUKU_HOME/envs/$ENV_NAME"

mkdir -p "$ENV_DIR/cache"
ln -sf "$HOME/.tsuku/cache/downloads" "$ENV_DIR/cache/downloads"

TSUKU_HOME="$ENV_DIR" ./tsuku "${@:2}"
```

**Usage:**
```bash
./tsuku-dev dev install cmake
./tsuku-dev ci-test list
```

**Advantages:**
- Proves the workflow before in-binary commitment
- Contributors can iterate on the UX externally
- Zero binary complexity for non-contributors
- If widely adopted, becomes candidate for upstreaming (like virtualenv)

**Disadvantages:**
- Higher ceremony than a flag
- Requires shell scripting knowledge to customize
- Not cross-platform (Windows needs different script)

### Path 2: In-Binary, Limited Scope (venv Subset Model)

Adopt only the minimal --env flag, skip management commands:

```bash
tsuku --env dev install cmake  # Creates envs/dev/ automatically
tsuku --env dev list           # Uses envs/dev/state.json
rm -rf ~/.tsuku/envs/dev       # Manual cleanup
```

No `tsuku env list/clean/info` commands. Environments are opaque directories, cleaned up manually.

**Advantages:**
- Lower binary complexity than full proposal
- Matches venv's minimal scope (creation only)
- Solves the core problem (isolation + cache sharing)

**Disadvantages:**
- Less discoverable than full management interface
- Still adds in-binary complexity for a contributor feature
- Doesn't fully leverage the opportunity if you're adding environments anyway

### Path 3: Full In-Binary (As Proposed)

Implement the full design: --env flag, TSUKU_ENV, tsuku env list/clean/info.

**Advantages:**
- Best UX for contributors and CI
- Matches kubectl/terraform patterns completely
- Automatic cache sharing, no manual setup
- Discoverable via --help and env list

**Disadvantages:**
- Most binary complexity
- Sets precedent for contributor QoL features in-binary
- No external proof period
- May be over-engineered for a testing convenience

## Conclusion

Python's venv is a strong structural analogy for tsuku --env. Both isolate mutable state while sharing expensive immutable content. Both use session-scoped or command-scoped activation. Both solve "don't make me duplicate everything" problems.

However, the strategic analogy is weak. venv is a universal end-user workflow that was validated externally for five years before stdlib inclusion. tsuku --env is a contributor convenience feature being proposed for immediate in-binary inclusion.

The venv trajectory suggests that if tsuku --env is truly essential, it should start as an external wrapper script, be adopted by contributors, and then be upstreamed if it proves indispensable. The five-year gap between virtualenv (2007) and venv (2012) wasn't wasted time; it was proof-of-concept at ecosystem scale.

The design doc's focus on "low ceremony" and "eliminating manual TSUKU_HOME juggling" frames this as a UX improvement, not a fundamental capability. That's fine, but it weakens the case for in-binary inclusion. If the primary benefit is convenience, an external script is a viable alternative that avoids permanent binary complexity.

If tsuku --env were positioned as "project-specific tool isolation" for end users (e.g., "pin cmake 3.25 for this project"), the venv analogy would be much stronger. As a contributor-focused testing tool, the analogy highlights what's missing: universal need and external validation.

### Recommendation Implied by the Analogy

Start with Path 1 (external wrapper script). Use it in CI and contributor workflows for 6-12 months. If it becomes indispensable and contributors actively choose it over manual TSUKU_HOME export, then revisit in-binary inclusion with real usage data. This follows venv's trajectory: prove value externally, then integrate.

If the wrapper script is too tedious and nobody uses it, that's signal that the feature isn't solving a critical problem. If contributors embrace it and advocate for in-binary inclusion, that's the same signal virtualenv sent before PEP 405.

The venv analogy doesn't say "add --env to the binary." It says "prove it first."
