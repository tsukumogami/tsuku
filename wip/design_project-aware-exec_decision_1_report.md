<!-- decision:start id="project-aware-exec-design" status="assumed" -->
### Decision: How should tsuku exec work?

**Context**

Block 6 of the shell integration design bridges Track A (command interception and auto-install) with Track B (project configuration). The `autoinstall.Runner.Run` method already accepts a `ProjectVersionResolver` interface, but `tsuku run` passes nil -- it never consults `.tsuku.toml` for version pins. The gap shows up in CI pipelines, shell scripts, and non-interactive environments where shell hooks aren't available.

The central tension is the command-to-recipe mapping. `.tsuku.toml` declares tools by recipe name (e.g., `ripgrep = "14.1.0"`), but users type command names (`rg`). The resolver needs to bridge that gap without introducing new infrastructure.

The existing binary index already maps `rg -> ripgrep` via its SQLite lookup. The question is whether to reuse that mapping or build something new, and whether the exec capability belongs in a new command or inside the existing `tsuku run`.

**Assumptions**

- The binary index is available when `tsuku exec` runs. First-time users who haven't run `tsuku update-registry` will get a clear error message directing them to build the index. This matches the existing `tsuku run` behavior.
- `.tsuku.toml` tool keys are always recipe names, never command aliases. The config format is already implemented and tested with this convention.
- This decision was made in --auto mode without interactive confirmation.

**Chosen: Separate tsuku exec with index-backed resolver**

A new `tsuku exec` command that constructs a `ProjectVersionResolver` backed by both `project.LoadProjectConfig` and the binary index's `LookupFunc`. The flow:

1. `tsuku exec <command> [args]` loads `.tsuku.toml` via `LoadProjectConfig(cwd)`
2. The resolver receives a command name (e.g., "rg")
3. Resolver queries the binary index to find the recipe name ("ripgrep")
4. Resolver checks `.tsuku.toml` for a version pin on that recipe
5. If found, returns the pinned version; if not, returns `!ok` (falls through to latest)
6. `Runner.Run` handles install-if-needed and process replacement as usual

The implementation consists of:

- `cmd/tsuku/cmd_exec.go`: Cobra command, wires resolver into `Runner.Run`
- `internal/project/resolver.go`: `ProjectVersionResolver` implementation
- Resolver constructor: `NewResolver(config *ConfigResult, lookup LookupFunc) ProjectVersionResolver`
- Default consent mode is `auto` (vs `confirm` for `tsuku run`) since `tsuku exec` targets CI/script use

`tsuku run` stays unchanged -- it keeps passing nil for the resolver. The two commands serve different audiences: `tsuku run` is for ad-hoc interactive use with consent prompts; `tsuku exec` is for deterministic, project-pinned execution without prompts.

**Rationale**

Alternative 1 wins because it composes existing, tested infrastructure without modification. The binary index already solves command-to-recipe mapping. `LoadProjectConfig` already solves `.tsuku.toml` discovery. `Runner.Run` already accepts the resolver interface. The only new code is a thin struct that connects these pieces -- roughly 30-50 lines of Go.

Keeping `tsuku exec` separate from `tsuku run` follows the parent design specification and avoids behavioral regressions. The commands have different default modes (auto vs confirm), different target audiences (CI vs interactive), and different resolution strategies (project-pinned vs latest). Merging them would require flag-gated behavior that complicates the codebase for marginal ergonomic gain.

The index-backed resolver (over the name-only approach) correctly handles the common case where command names differ from recipe names. `tsuku exec rg` should respect the `ripgrep` pin in `.tsuku.toml`. Skipping this case would make the project config feel incomplete.

**Alternatives Considered**

- **Enhance tsuku run with project awareness**: Auto-detecting `.tsuku.toml` in `tsuku run` risks surprising users who expect it to always use the latest version. It also changes the semantics of an existing, documented command. The behavioral change would need careful migration and could break scripts that depend on current `tsuku run` behavior. Rejected because clean separation is safer than behavioral mutation.

- **Separate tsuku exec with name-only resolver**: Skips the binary index and only matches when command name equals recipe name. This handles `go`, `node`, `python` but misses `rg` (from ripgrep), `fd` (from fd-find), and similar tools. The partial coverage would confuse users who pin a tool in `.tsuku.toml` but find the pin ignored at exec time. Rejected because incomplete coverage undermines trust in the feature.

- **Separate tsuku exec with cached reverse map from recipes**: Builds a parallel command-to-recipe cache by scanning recipe TOML files. This duplicates what the binary index already provides and adds another cache artifact to maintain, invalidate, and debug. Rejected because it adds complexity without benefit over the existing index.

**Consequences**

What becomes easier:
- CI pipelines can `tsuku exec go build` and get the project-pinned Go version with zero shell setup
- Makefiles and scripts gain deterministic tool versions via `tsuku exec`
- The `ProjectVersionResolver` interface gets its first real implementation, validating the design from Block 3

What becomes harder:
- Users need to know when to use `tsuku exec` vs `tsuku run` (documentation must clarify)
- The resolver depends on the binary index being built -- cold-start scenarios need clear error messages
- Adding a new command increases the CLI surface area by one entry point
<!-- decision:end -->
