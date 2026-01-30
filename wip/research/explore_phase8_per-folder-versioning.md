# Per-Folder Tool Versioning Analysis

**Research Question**: Is the proposed `--env` feature a building block toward per-folder tool versioning (like `.tool-versions`, `.node-version`, `.python-version` files)?

**Context**: The `--env` proposal adds explicit environment isolation via `tsuku --env <name>` or `TSUKU_ENV=<name>`. This analysis examines whether that design positions tsuku to support automatic, per-directory tool version switching similar to asdf, mise, volta, and nvm.

---

## 1. How Per-Folder Versioning Works

### asdf (.tool-versions)

**Mechanism**: Shell hook + config file lookup

```bash
# .tool-versions file (plain text, one line per tool)
nodejs 20.10.0
python 3.12.1
ruby 3.3.0
```

**Activation Flow**:
1. User adds `asdf` shim directory to PATH (`~/.asdf/shims/`)
2. Shell hook runs on every `cd` command (`asdf` function in `.bashrc`/`.zshrc`)
3. Hook walks up directory tree looking for `.tool-versions` file
4. When found, `asdf` rewrites shims to point to the specified versions
5. Running `node` or `python` hits the shim, which execs the correct version binary

**Key Components**:
- **Shim layer**: Thin wrapper scripts in a single directory (`~/.asdf/shims/node`, etc.)
- **Directory walking**: Start at `$PWD`, walk up to `/` until finding `.tool-versions`
- **Shell integration**: Hook on `cd` to detect directory changes
- **Version resolution**: Parse `.tool-versions`, verify versions exist, update shim targets

**Example**:
```bash
$ cd ~/project-a
$ cat .tool-versions
nodejs 18.0.0

$ node --version   # shim execs ~/.asdf/installs/nodejs/18.0.0/bin/node
v18.0.0

$ cd ~/project-b
$ cat .tool-versions
nodejs 20.10.0

$ node --version   # shim now execs ~/.asdf/installs/nodejs/20.10.0/bin/node
v20.10.0
```

### mise (.mise.toml or .tool-versions)

**Mechanism**: Enhanced asdf with native code + broader config

```toml
# .mise.toml (TOML format, more features than .tool-versions)
[tools]
node = "20.10.0"
python = "3.12"
go = { version = "1.21", install = false }  # Don't auto-install

[env]
NODE_ENV = "development"
```

**Activation Flow**: Nearly identical to asdf, but faster (Rust implementation)

1. Shim directory in PATH (`~/.local/share/mise/shims/`)
2. Shell hook on `cd` (optional: can use shims without hook)
3. Directory walking to find `.mise.toml` or `.tool-versions`
4. Shims redirect to versioned binaries

**Differences from asdf**:
- Native Rust binary (faster hook execution)
- TOML config with env vars, tasks, and conditional logic
- Can work without shell hook (shims check `$PWD` on every exec)
- Supports `.node-version`, `.python-version`, etc. (single-tool files)

### volta (.node-version, package.json)

**Mechanism**: Node-specific shim layer + package.json integration

```json
// package.json
{
  "volta": {
    "node": "20.10.0",
    "npm": "10.2.0"
  }
}
```

**Activation Flow**:
1. Install volta (adds `~/.volta/bin/` to PATH)
2. `node`, `npm`, `yarn` are shims in that directory
3. Shims check for `package.json` with `volta` key, or `.node-version` file
4. On each invocation, shim execs the specified version

**Key Difference**: No shell hook needed. Each shim does directory walking on exec.

**Trade-off**: Small overhead per invocation (dir walk + config parse) vs. no shell integration

### nvm (manual activation)

**Mechanism**: Shell function that rewrites PATH

```bash
# .nvmrc file (plain text, just the version)
20.10.0
```

**Activation Flow**:
1. User runs `nvm use` manually in each directory
2. `nvm` function reads `.nvmrc`, prepends version-specific bin dir to PATH
3. No automatic switching on `cd`

**Optional auto-switch**: Some community scripts add a hook to auto-run `nvm use` on `cd`, but it's not built-in.

**Key Difference**: Explicit activation vs. implicit (asdf/mise/volta do it automatically)

---

## 2. Is `--env` Architecturally on the Path to Per-Folder Versioning?

**Answer: Orthogonal, not on the path.**

### Architectural Differences

| Dimension | `--env` (proposed) | Per-Folder Versioning |
|-----------|-------------------|----------------------|
| **Activation** | Explicit (`--env dev` or `export TSUKU_ENV=dev`) | Implicit (triggered by `cd` or shim invocation) |
| **Scope** | Session-wide or command-specific | Directory-specific, automatic context switch |
| **State location** | Single state file per environment (`$TSUKU_HOME/envs/dev/state.json`) | Single global state + config files in project dirs |
| **PATH model** | Environment's `bin/` directory in PATH | Shim directory in PATH, shims redirect to versioned bins |
| **Primary use case** | Isolation for testing, CI, dev workflows | Project-specific tool versions for reproducibility |
| **User intent** | "I want a clean slate" | "This project needs Go 1.21, that one needs 1.22" |

### Why They're Orthogonal

1. **`--env` isolates state, not versions**: An environment has one active version of each tool (via `state.json` and `current/` symlinks). Per-folder versioning allows *different* active versions in the same `TSUKU_HOME` depending on `$PWD`.

2. **`--env` requires explicit selection**: You must run `tsuku --env dev install cmake` or `export TSUKU_ENV=dev`. Per-folder versioning activates automatically when you `cd` into a directory with a config file.

3. **`--env` changes state location**: It moves `state.json`, `tools/`, `bin/` to `$TSUKU_HOME/envs/<name>/`. Per-folder versioning keeps a single `tools/` directory with all versions installed, and switches which one is "active" based on context.

### Example Contrast

**With `--env`**:
```bash
$ export TSUKU_ENV=project-a
$ tsuku install go@1.21
$ go version   # 1.21 (from $TSUKU_HOME/envs/project-a/tools/current/go)

$ export TSUKU_ENV=project-b
$ tsuku install go@1.22
$ go version   # 1.22 (from $TSUKU_HOME/envs/project-b/tools/current/go)

$ cd ~/project-a
$ go version   # Still 1.22! TSUKU_ENV=project-b is still set
```

**With per-folder versioning** (hypothetical):
```bash
$ cd ~/project-a
$ cat .tsuku-versions
go 1.21

$ go version   # 1.21 (shim or hook detected .tsuku-versions)

$ cd ~/project-b
$ cat .tsuku-versions
go 1.22

$ go version   # 1.22 (context switched automatically)
```

The `--env` model is session-scoped. Per-folder versioning is directory-scoped.

---

## 3. What Additional Pieces Would Be Needed for Per-Folder Versioning?

To build per-folder versioning, tsuku would need these components (none of which `--env` provides):

### A. Shim Layer or Shell Hook

**Option 1: Shims** (like volta, mise without hook)

- Create thin wrapper scripts in `$TSUKU_HOME/bin/` for every installed binary
- Each shim:
  1. Walks up directory tree from `$PWD` to find `.tsuku-versions` (or similar)
  2. Parses config to determine desired version for this tool
  3. Execs the real binary from `$TSUKU_HOME/tools/<tool>-<version>/bin/<name>`

**Option 2: Shell hook** (like asdf, mise with hook)

- Add function to `.bashrc`/`.zshrc` that runs on every `cd`
- Hook:
  1. Checks for `.tsuku-versions` in current directory or parents
  2. Updates a tsuku-managed state file or env var with active versions
  3. Rewrites symlinks in `$TSUKU_HOME/bin/` to point to correct versions

**Trade-offs**:
- Shims: No shell integration needed, but small exec overhead per command
- Hook: Faster execution, but requires shell config changes and doesn't work in subshells/scripts

**Current tsuku model**: Direct symlinks in `$TSUKU_HOME/bin/` pointing to `tools/current/<tool>`. No shim layer, no hook.

### B. Config File Format

Define a per-directory config file (e.g., `.tsuku-versions`, `.tool-versions`, or reuse existing formats).

**Options**:
1. **Plain text** (asdf `.tool-versions` style):
   ```
   go 1.21.5
   node 20.10.0
   python 3.12
   ```
   Pros: Simple, readable, asdf-compatible
   Cons: No metadata, no environment variables

2. **TOML** (mise `.mise.toml` style):
   ```toml
   [tools]
   go = "1.21.5"
   node = "20.10.0"

   [env]
   GOPATH = "/workspace/go"
   ```
   Pros: Structured, supports env vars and extra config
   Cons: More complex parser

3. **JSON** (package.json-style):
   ```json
   {
     "tsuku": {
       "go": "1.21.5",
       "node": "20.10.0"
     }
   }
   ```
   Pros: Embeds in existing files (package.json, go.mod metadata)
   Cons: Less readable for simple cases

**Recommendation**: Start with plain text (asdf-compatible), add TOML later for advanced use cases.

### C. Directory Walking Logic

Function to search for config file starting at `$PWD` and walking up to `/`:

```go
func FindVersionFile(startDir string) (path string, err error) {
    dir := startDir
    for {
        candidate := filepath.Join(dir, ".tsuku-versions")
        if _, err := os.Stat(candidate); err == nil {
            return candidate, nil
        }

        parent := filepath.Dir(dir)
        if parent == dir {
            return "", fmt.Errorf("no .tsuku-versions found")
        }
        dir = parent
    }
}
```

This runs on every shim invocation (or every `cd` if using shell hook).

### D. Multi-Version State Management

Currently, tsuku's state model supports multiple installed versions per tool (added in multi-version support design), but only one "active" version globally (symlinked in `tools/current/`).

Per-folder versioning needs:
- All versions installed in `$TSUKU_HOME/tools/` (already supported)
- No single "active" version (remove or deprecate `tools/current/`)
- Shims or hooks dynamically select version based on context

**State changes**:
- Keep `state.json` tracking all installed versions
- Remove `ActiveVersion` field (or make it fallback for when no config file exists)
- Symlink management becomes shim/hook responsibility

### E. Fallback Behavior

What happens when:
1. No config file found in directory tree?
   - **Option A**: Use global default version from `state.json` or `~/.tsuku/config.toml`
   - **Option B**: Error out ("no version specified for tool X")

2. Config file specifies a version not installed?
   - **Option A**: Auto-install the version (mise does this with `mise install`)
   - **Option B**: Error with helpful message ("go@1.21.5 not installed, run `tsuku install go@1.21.5`")

3. Config file in parent directory, different in subdirectory?
   - Walk up until first match, stop there (asdf behavior)

---

## 4. Does Building `--env` Now Help or Hinder Future Per-Folder Versioning?

### Neutral to Slightly Helpful

**`--env` does not block per-folder versioning**, but it also doesn't advance it much. They solve different problems:

- `--env`: "I need isolated test environments for development and CI"
- Per-folder: "I need different tool versions per project directory"

### Minor Synergies

1. **State isolation experience**: Implementing `--env` teaches tsuku how to manage multiple independent state files and tool directories. This experience informs multi-version state design (already implemented).

2. **PATH management precedent**: `--env` creates environment-specific `bin/` directories. Per-folder versioning needs dynamic PATH switching (via shims or hooks), which is a related but distinct problem.

3. **Testing infra**: With `--env`, developers can test per-folder versioning prototypes in isolated environments without affecting their main installation.

### Where `--env` Doesn't Help

1. **Shim layer**: `--env` doesn't create shims or hooks. It still uses direct symlinks in `envs/<name>/bin/`.

2. **Directory-aware execution**: `--env` is session-scoped, not directory-scoped. It doesn't introduce the "check `$PWD` on every exec" pattern.

3. **Config file parsing**: `--env` doesn't parse per-directory config files like `.tool-versions`.

4. **Automatic activation**: `--env` is explicit. Per-folder is implicit/automatic.

### Could They Coexist?

Yes, but the interaction needs design:

**Scenario**: User has `TSUKU_ENV=dev` set and `cd`s into a directory with `.tsuku-versions`.

**Option 1: Environment wins** (env overrides per-folder config)
- When `TSUKU_ENV` is set, ignore `.tsuku-versions` entirely
- Use case: Testing per-folder config in isolation ("I want to test this config in env X without affecting my real setup")

**Option 2: Per-folder wins** (config overrides env)
- Even with `TSUKU_ENV` set, `.tsuku-versions` determines active versions
- Use case: Environments are for state isolation, per-folder is for version selection

**Option 3: Hybrid** (env sets base, config overlays)
- Environment provides fallback versions
- `.tsuku-versions` overrides for tools it specifies
- Complex but flexible

**Recommendation**: Start with Option 1 (environment isolation is primary use case for `--env`, don't muddy it with auto-switching). Later, if demand exists, allow environments to opt into per-folder config.

---

## 5. Key Architectural Difference: Explicit vs. Implicit Activation

This is the fundamental divergence:

### `--env`: Explicit Activation

**Philosophy**: Isolation is a deliberate choice. You opt into an environment for testing, CI, or parallel work.

**User experience**:
```bash
# Explicit every time
tsuku --env dev install cmake

# Or session-wide with visible env var
export TSUKU_ENV=dev
tsuku install cmake
tsuku list
```

**Visibility**: Environment name can be shown in prompt, in `tsuku` output, in command history.

**Predictability**: State doesn't change unless you change the env var or flag.

### Per-Folder: Implicit Activation

**Philosophy**: Tool versions are project metadata. The system should "just know" which versions to use based on where you are.

**User experience**:
```bash
# No explicit tsuku command needed
cd ~/project-a
go version   # Automatically uses version from .tsuku-versions

cd ~/project-b
go version   # Automatically switches
```

**Visibility**: Version might not be obvious unless you check the config file or the tool's `--version` output. Some tools (mise, asdf) can show active versions in shell prompt.

**Predictability**: State changes on every `cd`, which is powerful but can surprise users who don't realize they switched context.

### Why This Matters

These are **incompatible design philosophies** for the same user-facing feature space. You can implement both (asdf has environments via `ASDF_DATA_DIR`, mise has `mise env`), but they don't naturally compose.

`--env` optimizes for:
- Developer testing and CI (explicit, bounded)
- Parallel execution without interference
- Discoverability (list all environments)

Per-folder optimizes for:
- Project reproducibility (automatic, seamless)
- Zero-config version switching
- Team consistency (commit `.tsuku-versions` to repo)

---

## Summary and Recommendation

### Findings

1. **Per-folder versioning** (asdf/mise/volta pattern) relies on:
   - Shim layer or shell hook to intercept binary execution
   - Directory walking to find `.tool-versions` or similar config
   - Implicit activation when `cd`ing into directories
   - Single global tool installation directory with dynamic version selection

2. **`--env`** (proposed) provides:
   - Explicit environment selection via flag or env var
   - Isolated state directories under `$TSUKU_HOME/envs/`
   - Session-scoped or command-scoped isolation
   - No shim layer, no directory walking, no automatic switching

3. **Architectural relationship**: Orthogonal, not sequential. `--env` solves isolation for testing/CI. Per-folder solves version selection for project reproducibility. They address different problems with incompatible activation models (explicit vs. implicit).

4. **Building `--env` now**: Neutral impact on per-folder versioning. Minor synergies (multi-version state experience), but no direct progress toward shims, hooks, or directory-based config. Does not block future per-folder work.

5. **Coexistence**: Possible but requires design decisions about precedence (env overrides config, config overrides env, or hybrid).

### Recommendation

**If per-folder versioning is a near-term goal**: Consider designing it first, then layering `--env` on top as an explicit override mechanism. The shim/hook layer is foundational for per-folder and harder to retrofit.

**If `--env` is needed for immediate CI/dev workflows**: Build it as proposed. It won't conflict with per-folder versioning later, but it also won't get you closer to it. Treat them as independent features.

**If both are long-term goals**: Build `--env` first (it's simpler and immediately useful for contributors). Design per-folder versioning separately, possibly starting with a shim-only approach (no shell hook) to avoid shell integration complexity. When both exist, make `TSUKU_ENV` disable per-folder config lookup to keep the isolation model clean.

---

## Concrete Next Steps if Pursuing Per-Folder Versioning

(Not part of the `--env` design, but included for completeness)

1. **Phase 1: Shim prototype**
   - Create shim generator that wraps all installed binaries
   - Shims do directory walking to find `.tsuku-versions`
   - Parse simple plain-text format (asdf-compatible)
   - Exec versioned binary from `tools/<tool>-<version>/bin/`

2. **Phase 2: State integration**
   - Update `state.json` to track available versions
   - Remove global "active version" concept (or make it fallback)
   - Add `tsuku versions <tool>` to list installed versions

3. **Phase 3: Auto-install**
   - When config specifies a missing version, prompt to install
   - Optional `tsuku sync` command to install all versions from config

4. **Phase 4: Shell hook** (optional, for performance)
   - Generate hook function for bash/zsh/fish
   - Pre-compute version mapping on `cd`, avoid shim overhead

5. **Phase 5: TOML config** (optional, for advanced users)
   - Support `.mise.toml` format for env vars, tasks, etc.
   - Remain compatible with `.tool-versions` for simplicity
