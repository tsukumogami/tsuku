# Recipe Pattern Exemplars Catalog

This catalog identifies the best teaching examples for each recipe pattern category. Each exemplar is selected for clarity, representative patterns, and simplicity as a learning reference.

---

## Category 1: Binary Download (github_archive or github_file)

### Candidates
1. **trivy.toml** - Clean github_archive with standard os/arch mapping, simple binaries
2. **tflint.toml** - github_archive with zip format, standard asset pattern
3. **velero.toml** - github_archive with strip_dirs, common pattern
4. **skaffold.toml** - github_file action, single binary, cleaner than archive
5. **tsuku-dltest.toml** - github_file with binary remapping, shows binaries as array of objects

### Selected: `recipes/t/trivy.toml`
**Why**: Trivy is the clearest exemplar because:
- Demonstrates **github_archive** pattern with tar.gz (most common format)
- Shows proper os_mapping/arch_mapping transformations (darwin/linux, amd64/arm64)
- Includes standard verify pattern with {version} placeholder
- No extra complexity - just the core pattern
- Human-authored (not llm_validation="skipped")
- Perfect length for teaching

**Key Patterns Demonstrated**:
- github_archive action with repo reference
- asset_pattern with platform placeholders
- strip_dirs configuration
- binaries specification
- os/arch mapping for cross-platform assets

---

## Category 2: Homebrew-Backed Installation

### Candidates
1. **zoxide.toml** - Simple homebrew + install_binaries, clean structure
2. **zsh.toml** - Similar pattern, minimal metadata
3. **zstd.toml** - Adds library outputs (showing library variant)
4. **zls.toml** - Straight homebrew + binaries
5. **xz.toml** - Multiple binaries example

### Selected: `recipes/z/zoxide.toml`
**Why**: Zoxide is ideal because:
- **Simplest homebrew pattern** - just formula + install_binaries
- Shows practical workflow: homebrew retrieves package, install_binaries registers the binary
- Has clear comments showing intent (despite llm_validation="skipped", it's well-structured)
- Single binary example (easiest to understand)
- No platform conditionals to distract from core pattern

**Key Patterns Demonstrated**:
- homebrew action with formula parameter
- install_binaries step following homebrew
- Binary path specification (bin/zoxide)
- Standard verify pattern

---

## Category 3: Source Build with Dependencies (configure_make or cmake_build)

### Candidates
1. **curl.toml** - configure_make with dependencies (openssl, zlib), set_rpath, signature verification
2. **libcurl-source.toml** - configure_make with extensive configure_args, library outputs
3. **ncurses.toml** - configure_make with multiple configure_args, library outputs
4. **apr.toml** - configure_make with platform-conditional source build + skip_verification_reason
5. **pcre2.toml** - configure_make in musl branch only (shows conditional sourcing)

### Selected: `recipes/c/curl.toml`
**Why**: Curl is the best exemplar because:
- **Core configure_make pattern** with real-world dependencies
- Shows **dependency resolution**: configure_args includes --with-openssl, --with-zlib
- Demonstrates **set_rpath action** (advanced but important for libs)
- Includes signature verification (GPG key fingerprint) - security best practice
- Single action (not split by platform) makes it uncluttered
- Comments explain the rpath strategy
- Human-authored, clear intent

**Key Patterns Demonstrated**:
- download + extract workflow
- setup_build_env for build prerequisites
- configure_make with configure_args
- set_rpath for dependency linking
- Signature verification with keys
- install_binaries with verify pattern

---

## Category 4: Platform-Conditional with when Clauses (libc Splits)

### Candidates
1. **pcre2.toml** - Best example: glibc/musl split with different strategies (homebrew vs source)
2. **glib.toml** - glibc/musl/darwin split, shows apk_install for Alpine
3. **libidn2.toml** - Clean glibc/musl/darwin pattern
4. **apr.toml** - Similar three-way split
5. **tree-sitter.toml** - Clean three-way split with library outputs

### Selected: `recipes/p/pcre2.toml`
**Why**: PCRE2 is the exemplar for platform conditionals because:
- **Most instructive conditional structure**: shows different strategies per platform
  - glibc Linux: uses homebrew bottle (fast path)
  - musl Linux: builds from source (because homebrew unavailable)
  - macOS: uses homebrew
- Demonstrates **when clauses with os + libc** - the exact pattern to teach
- Shows that same binary can have different install paths
- Library with outputs (bin/pcre2grep, bin/pcre2test)
- Comments explain the reasoning ("glibc Linux: Homebrew bottle", "musl Linux: compile from source")
- Comprehensive without being overwhelming

**Key Patterns Demonstrated**:
- when = { os = ["linux"], libc = ["glibc"] } for first branch
- when = { os = ["linux"], libc = ["musl"] } for source build branch
- when = { os = ["darwin"] } for macOS
- Multiple steps within same when clause
- download + extract + setup_build_env + configure_make workflow for musl
- install_binaries with outputs across branches

---

## Category 5: Ecosystem-Delegated (cargo_install, npm_install, pipx_install, go_install, gem_install)

### Candidates for cargo_install
1. **try-rs.toml** - Simple cargo_install with crate name
2. (Most cargo recipes are auto-generated)

### Candidates for npm_install
1. **zx.toml** - Simple npm_install with executables
2. **yarn.toml** - npm_install with multiple executables (yarn, yarnpkg)

### Candidates for pipx_install
1. **ruff.toml** - Clean pipx_install, includes comment about runtime_dependencies
2. **poetry.toml** - Simple pipx_install with custom verify pattern
3. **httpie.toml** - Multiple executables (http, https)
4. **black.toml** - Multiple executables (black, blackd)

### Candidates for go_install
1. **staticcheck.toml** - Simple go_install with module path

### Candidates for gem_install
1. **jekyll.toml** - gem_install with special verify using {install_dir} and zig dependency

### Selected by type:
**cargo_install: `recipes/t/try-rs.toml`**
- Simple, clean cargo_install pattern
- Shows crate parameter and executables specification

**npm_install: `recipes/z/zx.toml`** 
- Clearest npm pattern, single executable
- Shows how npm packages resolve to CLI tools

**pipx_install: `recipes/r/ruff.toml`**
**Why Ruff is best for pipx**:
- Clear pipx_install action
- Includes crucial comment about runtime_dependencies (compiled binary, no Python needed)
- Shows the pattern of distributing compiled binaries via PyPI
- Clean verify pattern with version extraction

**go_install: `recipes/s/staticcheck.toml`**
- Perfect go_install exemplar: module path -> executable

**gem_install: `recipes/j/jekyll.toml`**
- Shows gem_install with {install_dir} variable in verify command
- Demonstrates runtime dependency (zig) for native extensions
- Realistic pattern for Ruby tools with compiled components

---

## Category 6: Library with Outputs and rpath

### Candidates
1. **curl.toml** (binary but uses set_rpath) - Good for showing rpath in context
2. **ncurses.toml** - Multiple library outputs, binaries, cross-platform
3. **libcurl-source.toml** - Library variant with extensive outputs, signature verification
4. **glib.toml** - Platform-conditional library with multiple .so/.dylib variants
5. **tree-sitter.toml** - Clean library outputs across platforms

### Selected: `recipes/l/libcurl-source.toml`
**Why**: libcurl-source is the best exemplar because:
- **Dedicated library recipe** (not primary binary like curl.toml)
- Shows extensive **library outputs**: libcurl.so, libcurl.a, .pc file, headers
- Demonstrates **signature verification** with GPG key fingerprint
- Shows **configure_args for library builds**: --enable-shared, --enable-static, --without-* for minimal deps
- **Clear comments**: explains why (Debian container OPENLDAP symbol mismatch), when to use
- install_binaries step with install_mode="directory" for library structure
- Not platform-conditional, so focuses purely on library pattern

**Key Patterns Demonstrated**:
- Library type in metadata
- Download with signature verification
- configure_make with library-specific options
- install_binaries with multiple outputs (binaries, .so, .a, .pc, headers)
- Library dependencies (openssl, zlib)

**Advanced rpath reference**: See `recipes/c/curl.toml` for set_rpath in action

---

## Category 7: Custom Verification (Non-Standard Verify Pattern)

### Candidates
1. **jekyll.toml** - Uses {install_dir} variable in verify command (custom because Ruby gems install differently)
2. **apr.toml** - Has skip_verification_reason (shows verification override)
3. **curl.toml** - Signature verification in download (advanced)
4. **skaffold.toml** - Uses `skaffold version` instead of typical `--version` flag
5. **poetry.toml** - Custom pattern match: "Poetry (version {version})" (parenthetical output)

### Selected: `recipes/j/jekyll.toml`
**Why**: Jekyll exemplifies custom verification because:
- **Uses {install_dir} variable** - necessary because gem binaries install to a non-standard location
- **Demonstrates the problem**: "ensure we verify the installed gem, not any system version" (comment explains the WHY)
- Shows **absolute path requirement** in verify command (critical for correctness)
- Realistic real-world case: Ruby gems don't follow standard bin/ layout
- Pattern is clear and easy to understand

**Secondary exemplars for other custom patterns**:
- **apr.toml**: Shows skip_verification_reason when verification isn't possible
- **poetry.toml**: Shows custom pattern matching for non-standard output format

---

## Bonus: Advanced Exemplars (Multiple Patterns Combined)

### Candidate 1: `recipes/p/pcre2.toml` (Reviewed above in Category 4)
**Additional Advanced Pattern**: Combines platform-conditional + source build + library outputs + verification
- Shows how one recipe can have 3+ completely different execution paths
- Musl path includes full: download -> extract -> setup_build_env -> configure_make workflow
- glibc/darwin paths use package managers but still define outputs explicitly
- This is a **masterclass in conditional recipe design**

### Candidate 2: `recipes/t/tsuku-llm.toml`
**Advanced Pattern**: gpu-conditional github_file with runtime dependencies
- **Multi-dimensional conditionals**: when = { os = ["linux"], arch = "amd64", gpu = ["nvidia"] }
- Shows **gpu parameter** (beyond just os/libc)
- **Multiple gpu variants**: cuda vs vulkan
- Each branch specifies different dependencies
- Demonstrates **extensibility beyond os/arch** (can add gpu, architecture variants, etc.)
- Real-world use case: same tool, different binary for different hardware

### Candidate 3: `recipes/c/curl.toml`
**Advanced Pattern**: Binary + dependencies + signatures + rpath + cross-platform
- Combines: source build + dependency resolution + signature verification + rpath handling
- Comments explain complex decisions (rpath strategy, platform differences)
- Shows the **full production recipe pattern**

---

## Summary by Pattern Coverage

| Pattern | Exemplar | File |
|---------|----------|------|
| Binary Download (github_archive) | Trivy | recipes/t/trivy.toml |
| Binary Download (github_file) | Skaffold | recipes/s/skaffold.toml |
| Homebrew-Backed | Zoxide | recipes/z/zoxide.toml |
| Source Build (configure_make) | Curl | recipes/c/curl.toml |
| Platform-Conditional (libc) | PCRE2 | recipes/p/pcre2.toml |
| Cargo Install | Try-rs | recipes/t/try-rs.toml |
| Npm Install | Zx | recipes/z/zx.toml |
| Pipx Install | Ruff | recipes/r/ruff.toml |
| Go Install | Staticcheck | recipes/s/staticcheck.toml |
| Gem Install | Jekyll | recipes/j/jekyll.toml |
| Library + rpath | Libcurl-source | recipes/l/libcurl-source.toml |
| Custom Verify | Jekyll | recipes/j/jekyll.toml |
| Advanced: Multi-platform | PCRE2 | recipes/p/pcre2.toml |
| Advanced: GPU-Conditional | Tsuku-llm | recipes/t/tsuku-llm.toml |
| Advanced: Production Build | Curl | recipes/c/curl.toml |

---

## Teaching Recommendations

### For Beginners: Start with these in order
1. **Trivy** (github_archive) - Simplest distribution pattern
2. **Zoxide** (homebrew) - How delegation works
3. **Ruff** (pipx_install) - Ecosystem leveraging
4. **PCRE2** (when clauses) - Conditional logic

### For Intermediate: Add these
5. **Curl** (source build + deps) - Full compilation workflow
6. **Jekyll** (custom verify) - Real-world edge cases
7. **Staticcheck** (go_install) - Different ecosystem patterns

### For Advanced: Study these
8. **Curl** (with rpath) - Binary linking and runtime paths
9. **Libcurl-source** (library outputs) - Package structure
10. **Tsuku-llm** (gpu-conditional) - Extended conditionals

### Patterns NOT YET exemplified with ideal recipes:
- **cmake_build**: No cmake recipes found in sampled 1400 (recipes use homebrew for cmake projects)
- **meson_build**: Only reference in discovery metadata, no actual meson recipes

