# Panel: Solve Contributor Isolation with a Script Now, Design Per-Directory Versioning Separately

## The Core Problem: Forced Marriage of Unrelated Features

The `--env` flag proposal tries to serve two masters: it needs to solve the immediate contributor isolation problem (developers working on tsuku recipes without polluting their personal install) while also serving as a "stepping stone" to per-directory version activation. This dual purpose compromises both goals.

The contributor problem is trivial to solve TODAY. A 20-line shell script can set `TSUKU_HOME` to a temporary directory, share the download cache to avoid redundant network requests, and provide a clean workspace for testing recipe changes. No binary changes needed. No architectural decisions required. Just a documented pattern that contributors can use immediately. Here's the entire solution:

```bash
#!/usr/bin/env bash
# dev.sh - Isolated tsuku development environment
export TSUKU_HOME="${TSUKU_DEV_HOME:-/tmp/tsuku-dev}"
export TSUKU_CACHE="${TSUKU_CACHE:-$HOME/.tsuku/cache}"  # Share download cache
mkdir -p "$TSUKU_HOME" "$TSUKU_CACHE"
exec tsuku "$@"
```

That's it. Contributors run `./dev.sh install serve` and get complete isolation. The script can live in the repo, be documented in CONTRIBUTING.md, and solve the problem completely without touching the binary. If we discover later that this pattern is so universally useful that it deserves first-class support, we can promote it then—but we'll do so with actual usage data rather than speculation.

## Per-Directory Versioning Deserves Its Own Design Process

Per-directory version activation is a first-class feature that fundamentally changes how users interact with tsuku. It's not a "nice to have" or an "evolution" of environments—it's a completely different mental model. Users need to understand config file precedence, version resolution rules, activation semantics, and interaction with shell integration. This is complex UX territory that deserves dedicated exploration through `/explore`, user research, and careful design iteration.

Trying to shoehorn this into `--env` creates architectural debt from day one. The research already found that mise's environment model is fundamentally different: it uses config-based activation with shared installations, not state isolation. mise reads `.mise.toml` files and activates specific versions via shell hooks—there's no concept of separate "environment directories" with independent state. If we bolt `--env` onto tsuku as a "stepping stone" to per-directory versioning, we're committing to either:

1. **Abandoning the path entirely**: `--env` creates isolated state directories, which is orthogonal to mise's shared-install + config-activation model. We'd need to rip it out and start over when we actually design per-directory versioning.

2. **Compromising the final design**: We'd feel pressure to make per-directory versioning "compatible" with `--env` semantics, even if that leads to a worse UX. The tail would wag the dog.

## The Virtualenv Precedent: External Validation Before Stdlib Adoption

The virtualenv trajectory is instructive here. It started as an external tool (2007), proved its value in the real world for years, and only became part of Python's standard library as `venv` in 2012—five years later, after the design had been battle-tested and refined. The key insight: let the pattern prove itself externally before committing to it in the core tool.

A dev.sh script follows the same philosophy. If isolated environments via `TSUKU_HOME` become a universal pattern that contributors love, and if we see organic demand for first-class support, then we promote it—but we do so knowing it's solving a real problem with proven design decisions. We avoid the trap of adding features speculatively because they "might be useful."

## The Honest Trade-Offs

**Script-first approach pros:**
- Solves contributor problem immediately (no waiting for design/implementation)
- Zero architectural debt or premature abstraction
- Can be documented and shipped in next PR
- Provides real usage data if we ever want first-class support
- Keeps binary focused on core package manager functionality

**Script-first approach cons:**
- Not as discoverable as a built-in flag
- Contributors need to remember to use the script
- Slightly more verbose (`./dev.sh install` vs `tsuku --env dev install`)

**Bundling --env with per-directory goals pros:**
- Single implementation effort (if the designs actually align)
- Feels like progress toward the larger goal

**Bundling --env with per-directory goals cons:**
- Architectural mismatch: `--env` (state isolation) ≠ mise's model (config activation)
- Delays contributor solution until per-directory design is complete
- Creates pressure to compromise per-directory design for `--env` compatibility
- Risks delivering neither feature well

## Recommendation

1. **Immediate action**: Add `dev.sh` to the repo, document it in CONTRIBUTING.md, solve contributor isolation now
2. **Parallel track**: Run `/explore --tactical per-directory-versioning` as a dedicated effort focused entirely on end-user UX, config design, and activation semantics
3. **Future decision**: If dev.sh proves universally useful and we want first-class support, promote it then—but only if it doesn't conflict with the per-directory design that emerges from proper exploration

This approach respects both problems by giving each one the attention it deserves, rather than forcing them into an awkward compromise that serves neither well.
