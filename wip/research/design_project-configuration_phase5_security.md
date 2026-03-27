# Security Review: project-configuration

## Dimension Analysis

### External Artifact Handling

**Applies:** Yes

**Analysis:** The `tsuku.toml` file is an external artifact when it lives inside a cloned repository. Running `tsuku install` (no args) in a cloned repo parses that file and triggers tool downloads and installations. The file itself is parsed by `BurntSushi/toml`, a well-tested TOML parser, so parsing-level exploits are unlikely. The real concern is what happens after parsing: recipe names from the file are passed to the existing install pipeline, which downloads binaries, verifies checksums, and extracts archives.

**Risks:**

1. **Untrusted repo installs unwanted tools (Medium).** A developer clones a repository and runs `tsuku install` without inspecting `tsuku.toml`. The file could declare dozens of tools the developer didn't expect, consuming bandwidth, disk, and install time. More importantly, if any declared recipe has a post-install step with side effects (e.g., modifying shell config), those execute with user privileges.

2. **Recipe name confusion (Low).** A malicious `tsuku.toml` could reference recipe names that sound benign but install unexpected software. This depends on the registry containing a recipe by that name -- tsuku's curated registry limits this risk, but it's worth noting.

**Mitigations already in design:**
- `tsuku install` requires explicit invocation -- no auto-install on clone or directory entry.
- The `--dry-run` flag lets users preview what would be installed.

**Suggested additional mitigations:**
- Print the full tool list and require confirmation before batch install when no `--yes` flag is present (similar to `apt install` behavior). This is especially important because the tool list comes from a file the user may not have authored.
- Consider a `--trust` or `--non-interactive` flag for CI contexts where confirmation would block, making the default interactive path safer for humans.

### Permission Scope

**Applies:** Yes

**Analysis:** The design operates entirely within user-space permissions. Tools install to `$TSUKU_HOME` (default `~/.tsuku/`), no sudo is required, and no system directories are modified. The parent directory traversal algorithm is bounded by `$HOME` and `TSUKU_CEILING_PATHS`.

**Risks:**

1. **Parent traversal crossing trust boundaries (Medium).** The traversal walks up from the working directory to `$HOME`. In shared hosting or multi-user environments, a `tsuku.toml` placed in a shared parent directory (e.g., `/home/shared/projects/tsuku.toml`) could influence installs for all users working in subdirectories. The `$HOME` ceiling mitigates the worst case (traversal never leaves the user's home), but directories between the project and `$HOME` are still in scope.

2. **Symlink traversal (Low).** If the working directory contains symlinks, the traversal could follow them to unexpected locations. The design doesn't mention symlink handling.

**Mitigations already in design:**
- Traversal stops at `$HOME` by default.
- `TSUKU_CEILING_PATHS` allows users to restrict traversal further.

**Suggested additional mitigations:**
- Resolve symlinks before traversal (use `filepath.EvalSymlinks` on the start directory) to prevent symlink-based misdirection.
- When the discovered `tsuku.toml` is not in the same directory as `.git` (or the current directory), print its path prominently so the user knows which file is governing their install. This makes "surprising" parent configs visible.

### Supply Chain or Dependency Trust

**Applies:** Yes

**Analysis:** The `tsuku.toml` file specifies recipe names and version constraints. These map to recipes in tsuku's curated registry, which are then fetched and installed through the existing pipeline. The trust chain is: `tsuku.toml` (authored by repo maintainer) -> recipe registry (curated by tsuku) -> upstream artifact (from tool author). This design doesn't change the registry trust model, but it does change who initiates installs.

**Risks:**

1. **Delegated install authority (Medium).** Before this design, users explicitly chose which tools to install. With project config, the repo author effectively chooses. A compromised or malicious repo could declare tools that the user wouldn't normally install, expanding their attack surface. The tools themselves go through tsuku's verification pipeline, but the decision of which tools to install shifts from the user to the config file author.

2. **Version pinning as a vector (Low).** A `tsuku.toml` pinning to a specific version could target a known-vulnerable version of a tool. This is a stretch since tsuku installs pre-built binaries from upstream sources, but it's worth considering for tools where specific versions have known issues.

3. **"latest" keyword risk (Low-Medium).** Using `version = "latest"` or `version = ""` means the resolved version depends on when `tsuku install` runs. A recipe update between two developers' installs could produce different tool versions, undermining the reproducibility goal. More concerning: if the registry is ever compromised, "latest" resolution would immediately pull the compromised version.

**Mitigations already in design:**
- Existing checksum verification on downloaded artifacts.
- Curated registry limits what recipe names can resolve to.

**Suggested additional mitigations:**
- Warn when `tsuku.toml` uses "latest" or empty version strings, since these undermine reproducibility and increase exposure to supply chain timing attacks.
- Consider a future lock file mechanism (acknowledged as out of scope) that records resolved versions, so `tsuku install` is deterministic after first resolution. The design's schema already accommodates this without breaking changes.

### Data Exposure

**Applies:** No

**Analysis:** This design reads a local TOML file and passes tool names and versions to the existing install pipeline. It does not introduce new network calls, telemetry, or data transmission. The `tsuku.toml` file itself contains only tool names and version strings -- no credentials, tokens, or user data. The existing telemetry system (if enabled) already reports install events; this design doesn't change what data is collected, only the trigger path.

The file discovery algorithm reads directory names during traversal, but this information stays local and is used only for the stat-based file search. No new data leaves the machine.

## Recommended Outcome

**OPTION 1: Approve with amendments.**

The design is sound and the core security properties hold: no auto-install on clone, explicit user invocation required, existing verification pipeline reused. The risks identified are real but manageable with minor additions:

1. Print the discovered config file path and tool list before batch install, requiring confirmation by default (no `--yes`).
2. Resolve symlinks before parent traversal.
3. Warn on "latest" or empty version constraints in project configs (informational, not blocking).

These are small changes that fit naturally into the existing design without altering the architecture.

## Summary

The project configuration design has a solid security posture because it requires explicit user invocation (`tsuku install`) rather than triggering installs automatically on clone or directory entry. The primary risk is that cloning an untrusted repository gives its author influence over which tools get installed when a user runs `tsuku install` without inspecting the config. Parent directory traversal adds a secondary risk of unexpected config discovery. Both risks are mitigated by adding a confirmation prompt for batch installs (showing the source file and tool list) and resolving symlinks during traversal. No changes to the core architecture are needed.
