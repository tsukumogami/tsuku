<!-- decision:start id="project-config-naming-discovery" status="assumed" -->
### Decision: Project Configuration File Naming and Discovery

**Context**

Tsuku needs a per-directory project configuration file so repositories can declare their tool requirements. This file is the foundation for two downstream features: shell environment activation (#1681), which reads it on every prompt hook, and project-aware exec (#2168), which reads it on every command invocation. The file name and discovery strategy affect discoverability for new contributors, monorepo usability, and debugging experience.

The decision covers three axes: file name (dotfile vs. visible), directory search strategy (current-only vs. parent traversal), and multi-file behavior (first-match vs. merge vs. explicit extends). Tsuku's existing files are all non-dotfiles: `$TSUKU_HOME/config.toml`, `recipes/<name>.toml`, `state.json`. Competitive tools split roughly evenly: mise supports both names with merging, asdf uses dotfiles with first-match traversal, devbox uses a visible name with no traversal.

Performance is not a differentiator. TOML parsing takes ~0.1ms for small files, and even worst-case parent traversal (20 stat calls) adds only ~0.2ms -- well within the 50ms shell integration budget regardless of approach.

**Assumptions**

- Most projects will have a single config file at the repository root. Deep nesting with layered configs is uncommon at launch.
- The dominant use case for monorepo support is "shared tools at repo root, individual projects inherit." Per-directory layering (root + subdirectory tools combined) is a secondary use case that can be addressed later.
- Contributors encountering tsuku for the first time benefit more from a visible file than from a clean project root.

**Chosen: `tsuku.toml` with parent traversal (first-match, no merge)**

Use `tsuku.toml` as the single accepted file name. When loading project configuration, walk up from the working directory checking each directory for `tsuku.toml`. Stop at the first match. Do not merge multiple configs. Stop traversal at `$HOME` by default; allow `TSUKU_CEILING_PATHS` environment variable (colon-separated list of directories) to customize the ceiling.

The discovery algorithm:

1. Start at the current working directory.
2. Check if `tsuku.toml` exists in this directory.
3. If found, parse and return it. Done.
4. If not found, move to the parent directory.
5. If the parent is `$HOME` or matches a `TSUKU_CEILING_PATHS` entry, stop. Return no config.
6. Repeat from step 2.

When `tsuku init` creates a new config file, it creates `tsuku.toml` in the current directory. The `tsuku install` (no args) command loads project config via this algorithm and installs all declared tools.

**Rationale**

*Non-dotfile name aligns with tsuku conventions.* Every existing tsuku file uses a visible name: `config.toml`, `state.json`, `recipes/*.toml`. A dotfile would be the sole exception. Using `tsuku.toml` keeps the convention consistent and makes the file immediately visible in `ls` output and file browsers. New contributors see it without knowing to look for hidden files.

*Single name eliminates ambiguity.* Supporting both `.tsuku.toml` and `tsuku.toml` (the mise approach) means every project team must decide which name to use, every doc must mention both, and tsuku must define precedence rules for when both exist. A single name removes this decision entirely.

*Parent traversal enables monorepos without complexity.* Many real projects live inside monorepos. Without traversal, each subdirectory needs its own `tsuku.toml` with duplicated tool declarations. Traversal lets a monorepo root declare shared tools that all subdirectories inherit. The `$HOME` ceiling prevents configs from leaking between unrelated projects.

*First-match (no merge) keeps behavior predictable.* When a `tsuku.toml` exists in the current directory, it completely defines the tools for that directory. There's no need to reason about which parent directories might contribute additional tools or override versions. A developer can read one file and know exactly what tools apply. This matches asdf's model and is simpler than mise's merging.

*Future extensibility preserved.* If demand for per-directory layering emerges, an `extends` field can be added to `tsuku.toml` without breaking backward compatibility. This is the same path volta took: no automatic merging, but explicit `extends` for projects that need it.

**Alternatives Considered**

- **`.tsuku.toml` (dotfile only)**: Keeps the project root clean and matches asdf/direnv conventions. Rejected because it breaks tsuku's non-dotfile convention and reduces discoverability. The "clean root" benefit is marginal -- one visible file among many (Makefile, Dockerfile, README.md, etc.) is not meaningful clutter.

- **Both `.tsuku.toml` and `tsuku.toml`**: Maximum flexibility, matches mise. Rejected because supporting two names creates ambiguity ("which should I use?"), doubles the test and documentation surface, and requires precedence rules. The flexibility doesn't justify the complexity for a new tool without an existing user base to migrate.

- **`tsuku.toml` with no traversal**: Simplest possible approach, matches devbox. Rejected because it forces tool declaration duplication in monorepos, which is a common and growing project structure. Traversal adds minimal complexity (well-understood algorithm, negligible performance cost) for significant monorepo usability.

- **Parent traversal with merging**: Most powerful monorepo support, matches mise. Rejected because merging makes it hard to answer "what tools does this directory get?" without mentally combining multiple files. Debugging version conflicts across merged configs is painful. The power-to-complexity ratio doesn't justify it at launch; `extends` can be added later if needed.

**Consequences**

What becomes easier: New contributors see `tsuku.toml` immediately. Monorepo projects declare tools once at the root. Debugging is straightforward -- one file controls each directory's tools. The `LoadProjectConfig` interface is simple: it returns one `ProjectConfig` or nil.

What becomes harder: Projects that need different tool sets in subdirectories of a monorepo must either duplicate the parent's declarations in a subdirectory `tsuku.toml` or wait for `extends` support. Teams migrating from mise who rely on config merging won't get that behavior automatically.
<!-- decision:end -->
