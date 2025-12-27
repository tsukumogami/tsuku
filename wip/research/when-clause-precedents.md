# When Clause Precedents: Conditional Filtering/Execution Patterns

**Research Date**: 2025-12-27
**Purpose**: Determine whether additive (independent matching) or exclusive (precedence-based) semantics are more common for conditional execution in configuration files, particularly TOML configs.

## Executive Summary

**Key Finding**: The ecosystem is **mixed** but trends toward **additive semantics** in most modern tools:

- **Additive (all matching conditions execute)**: Cargo, Homebrew, systemd, Ansible, GitHub Actions, Nix
- **Sequential/First-match (exclusive-like)**: GitLab CI rules
- **Mutual exclusivity by design**: Poetry (enforces non-overlapping conditions), Bazel (requires unambiguous matches)
- **Context-dependent**: Docker Compose (override files are additive, platform attributes are exclusive)

**Recommendation for Tsuku**: **Additive semantics** align better with:
1. Industry precedent (Cargo, Homebrew, systemd)
2. Composability and predictability
3. TOML as a configuration format (declarative, not imperative)
4. User expectations from similar package managers

## 1. Cargo (Rust) - Platform-Specific Dependencies

### Behavior: **ADDITIVE**

Multiple target sections all contribute dependencies when their conditions match. All matching `[target.'cfg(...)'.dependencies]` sections are combined.

### Syntax

```toml
[target.'cfg(windows)'.dependencies]
winhttp = "0.4.0"

[target.'cfg(unix)'.dependencies]
openssl = "1.0.1"

[target.'cfg(target_arch = "x86")'.dependencies]
native-i686 = { path = "native/i686" }

[target.'cfg(target_arch = "x86_64")'.dependencies]
native-x86_64 = { path = "native/x86_64" }
```

Or using full target triples:

```toml
[target.x86_64-pc-windows-gnu.dependencies]
winhttp = "0.4.0"

[target.i686-unknown-linux-gnu.dependencies]
openssl = "1.0.1"
```

### How It Works

- **Additive**: All matching target sections contribute their dependencies
- **Feature unification**: When the same dependency appears in multiple targets, features are unified (merged)
- **Resolver v2**: Can prevent cross-target feature unification with `resolver = "2"`

### Known Behaviors

From [GitHub issue #6121](https://github.com/rust-lang/cargo/issues/6121):
- If the same crate appears in different `[target.*.dependencies]` sections, features merge by default
- Resolver v2 prevents feature unification across targets not being built

### Complex Conditions

Cargo supports boolean operators in cfg expressions:

```toml
[target.'cfg(all(unix, target_arch = "x86_64"))'.dependencies]
native = { path = "native/unix-x86_64" }
```

Operators: `not`, `any`, `all`

### Limitations

- Cannot use feature-based conditions: `[target.'cfg(feature = "fancy")'.dependencies]` is NOT supported

### Documentation

- [Specifying Dependencies - The Cargo Book](https://doc.rust-lang.org/cargo/reference/specifying-dependencies.html)
- [Features - The Cargo Book](https://doc.rust-lang.org/cargo/reference/features.html)
- [RFC 1361: cfg-based dependencies](https://rust-lang.github.io/rfcs/1361-cargo-cfg-dependencies.html)

---

## 2. Poetry (Python) - Platform Markers

### Behavior: **MUTUAL EXCLUSIVITY ENFORCED**

Poetry uses PEP 508 environment markers. Multiple specifications for the same package **must have non-overlapping conditions** or Poetry raises an error.

### Syntax

```toml
[tool.poetry.dependencies]
pathlib2 = { version = "^2.2", markers = "python_version <= '3.4' or sys_platform == 'win32'" }
```

For different versions based on Python version:

```toml
[tool.poetry.dependencies]
foo = [
    {version = "<=1.9", python = ">=3.6,<3.8"},
    {version = "^2.0", python = ">=3.8"}
]
```

Platform-specific dependencies:

```toml
[tool.poetry.dependencies.psycopg2]
version = "^2.9.3"
markers = "sys_platform == 'linux'"

[tool.poetry.dependencies.psycopg2-binary]
version = "^2.9.3"
markers = "sys_platform == 'darwin'"
```

### How It Works

- **Not additive**: Each specification applies only when its condition matches
- **Mutual exclusivity required**: Overlapping conditions cause errors during resolution
- **Design rationale**: Avoids ambiguity about which version to install

### Documentation Quote

> "The constraints **must** have different requirements (like `python`) otherwise it will cause an error when resolving dependencies."

### Common Marker Variables

- `sys_platform` (e.g., 'linux', 'darwin', 'win32')
- `platform_machine` (e.g., 'arm64', 'x86_64')
- `python_version`
- `platform_python_implementation`

### Poetry 2.0 Changes

Poetry 2.0 supports `project.dependencies` with inline PEP 508 markers:

```toml
[project]
dependencies = [
    "pathlib2 (>=2.2,<3.0) ; python_version <= '3.4' or sys_platform == 'win32'"
]
```

### Documentation

- [Dependency specification - Poetry](https://python-poetry.org/docs/dependency-specification/)
- [Platform based dependencies in poetry](https://www.whatsdoom.com/posts/2022/07/24/platform-based-dependencies-in-poetry/)

---

## 3. GitLab CI - `rules`, `only`, `except`

### Behavior: **SEQUENTIAL FIRST-MATCH**

GitLab CI evaluates `rules` sequentially and stops at the first match. This creates exclusive-like behavior but based on order, not precedence.

### Syntax

```yaml
job:
  script: echo "Hello"
  rules:
    - if: '$CI_PIPELINE_SOURCE == "merge_request_event"'
      when: never
    - if: '$CI_COMMIT_BRANCH == "main"'
      when: always
    - when: on_success
```

### How It Works

- **Sequential evaluation**: Rules evaluated top-to-bottom
- **First match wins**: Stops at first matching rule
- **Default if no match**: Jobs with no rules default to `except: merge_requests`

### Precedence

**Workflow rules take precedence over job rules:**

> "workflow:rules are evaluated before jobs and take precedence over the job rules. For example, if a job has rules that allow it to run against a specific branch, but the workflow rules set jobs running against the branch to `when: never`, the jobs will not run."

### Cannot Mix with `only`/`except`

> "`only` and `except` should not be used with `rules` because this can lead to unexpected behavior."

### Expression Precedence

- Parentheses take precedence over `&&` and `||`
- Expressions in parentheses evaluate first

### Documentation

- [Specify when jobs run with rules - GitLab Docs](https://docs.gitlab.com/ci/jobs/job_rules/)
- [GitLab CI Rules – Change Pipeline Workflow](https://www.bitslovers.com/gitlab-ci-rules/)

---

## 4. GitHub Actions - `if` Conditions

### Behavior: **ADDITIVE (all matching steps run)**

Each step with a matching `if` condition executes. Steps are independent unless explicitly chained.

### Syntax

```yaml
steps:
  - name: Run on Linux
    if: runner.os == 'Linux'
    run: echo "Running on Linux"

  - name: Run on macOS
    if: runner.os == 'macOS'
    run: echo "Running on macOS"

  - name: Run on both
    if: runner.os == 'Linux' || runner.os == 'macOS'
    run: echo "Running on Unix-like"
```

### Multi-line Conditions

```yaml
- name: Running all tests
  if: |
    startsWith(github.event.comment.body, '/run-e2e-all') ||
    startsWith(github.event.comment.body, '/run-e2e-first-test') ||
    startsWith(github.event.comment.body, '/run-e2e-second-test')
  run: npm test
```

### How It Works

- **All matching steps execute**: No first-match behavior
- **Independent evaluation**: Each step's `if` evaluated separately
- **Default status check**: `success()` applied unless overridden

### Status Check Functions

```yaml
- name: The demo step has failed
  if: ${{ failure() && steps.demo.conclusion == 'failure' }}
  run: echo "Handling failure"
```

### Job-Level Conditionals

```yaml
job-b:
  needs: job-a
  if: needs.job-a.result == 'success' || needs.job-a.result == 'skipped'
```

### Composite Actions

Composite actions (YAML-based actions) support `if` conditionals on individual steps.

### Documentation

- [Using conditions to control job execution - GitHub Docs](https://docs.github.com/en/actions/writing-workflows/choosing-when-your-workflow-runs/using-conditions-to-control-job-execution)
- [Evaluate expressions in workflows - GitHub Docs](https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/evaluate-expressions-in-workflows-and-actions)
- [Using conditionals in GitHub Actions](https://www.brycewray.com/posts/2023/04/using-conditionals-github-actions/)

---

## 5. Ansible - `when` Clauses

### Behavior: **ADDITIVE (all conditions in list must be true)**

Multiple `when` conditions in list format create an implicit AND (all must be true). Tasks execute independently based on their conditions.

### Syntax

List format (implicit AND):

```yaml
tasks:
  - name: "shut down CentOS 6 systems"
    command: /sbin/shutdown -t now
    when:
      - ansible_facts['distribution'] == "CentOS"
      - ansible_facts['distribution_major_version'] == "6"
```

Explicit boolean operators:

```yaml
- name: Install on Debian or Ubuntu
  apt:
    name: nginx
  when: (ansible_distribution == "Debian") or (ansible_distribution == "Ubuntu")
```

AND condition:

```yaml
- name: Reboot proxy server
  reboot:
  when: (reboot_file.stat.exists) and (inventory_hostname == 'aws-proxy-server')
```

### How It Works

- **List = AND**: All conditions in list must be true
- **Tasks are independent**: Each task evaluated separately
- **Supports Jinja2**: Full expression support with tests and filters

### Import vs Include Behavior

**Imports (additive to all tasks):**
> "When you add a conditional to an import statement, Ansible applies the condition to all tasks within the imported file. This behavior is the equivalent of tag inheritance."

**Includes (applies only to include task):**
> "When a conditional is used with `include_*` tasks, it is applied only to the include task itself and not to any other tasks within the included file(s)."

### Documentation

- [Conditionals - Ansible Documentation](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_conditionals.html)
- [How to define multiple when conditions in Ansible](https://www.cyberciti.biz/faq/how-to-define-multiple-when-conditions-in-ansible/)
- [Ansible Conditionals - Spacelift](https://spacelift.io/blog/ansible-when-conditional)

---

## 6. Homebrew - Platform Blocks

### Behavior: **ADDITIVE (all matching blocks execute)**

All matching `on_*` blocks contribute their declarations. Blocks can be nested for multiple conditions.

### Syntax

```ruby
class MyFormula < Formula
  desc "Example formula"
  homepage "https://example.com"

  on_linux do
    depends_on "gcc"
  end

  on_macos do
    depends_on "llvm"
  end

  on_arm do
    depends_on "special-arm-lib"
  end

  # Nested for multiple conditions
  on_macos do
    on_arm do
      depends_on "gettext" => :build
    end
  end
end
```

Version-specific blocks:

```ruby
on_sequoia :or_newer do
  depends_on "gettext" => :build
end

on_ventura :or_older do
  depends_on "old-lib"
end
```

### How It Works

- **Additive**: All matching blocks execute and contribute dependencies/patches/resources
- **Nesting for AND**: Nest blocks to require multiple conditions
- **Version modifiers**: `:or_newer`, `:or_older` create ranges

### Inside `install` and `test` Methods

Don't use `on_*` blocks inside methods. Use conditionals instead:

```ruby
def install
  if OS.mac?
    # macOS-specific installation
  elsif OS.linux?
    # Linux-specific installation
  end

  if Hardware::CPU.arm?
    # ARM-specific configuration
  end
end
```

### Available Conditionals

- `OS.mac?` and `OS.linux?`
- `Hardware::CPU.arm?` and `Hardware::CPU.intel?`
- `MacOS.version` with comparison operators

### Known Issue: `keg_only` in Platform Blocks

From [Issue #11398](https://github.com/Homebrew/brew/issues/11398):
> "`keg_only` only supports supplying one reason, which affects both platforms. However on some occasions we will need to make a formula keg-only on macOS and Linux for different reasons."

Currently working around by choosing one reason, but ideally would support platform-specific reasons.

### Documentation

- [Formula Cookbook - Homebrew](https://docs.brew.sh/Formula-Cookbook)
- [Pull Request #8907: add on_linux and on_macos blocks](https://github.com/Homebrew/brew/pull/8907)

---

## 7. Docker Compose - Platform Configuration

### Behavior: **HYBRID (override files are additive, platform attribute is exclusive)**

Docker Compose has two distinct mechanisms with different semantics:

### Override Files (Additive)

Multiple compose files merge/overlay:

```bash
# Reads docker-compose.yml + docker-compose.override.yml
docker-compose up

# Specify multiple files
docker-compose -f docker-compose.yml -f docker-compose.dev.yml up
```

**Behavior**: Later files override/supplement earlier files. All matching services, networks, volumes are merged.

### Platform Attribute (Exclusive)

```yaml
services:
  frontend:
    platform: "linux/amd64"
    build:
      context: "."
```

**Behavior**: Single platform per service. Setting `DOCKER_DEFAULT_PLATFORM` with explicit `platform:` causes an error.

### Known Issues

From [Issue #9853](https://github.com/docker/compose/issues/9853):
> "Using a compose file that has a service which specifies platform: and also supplying DOCKER_DEFAULT_PLATFORM will result in an error. The application appears to be requesting two platforms to be targeted."

**Workaround**: Move platform specification to Dockerfile's FROM statement.

### Best Practices

- Base `docker-compose.yml`: Common configuration across environments
- `docker-compose.override.yml`: Local development overrides
- `docker-compose.prod.yml`, `docker-compose.staging.yml`: Environment-specific configurations

### Documentation

- [What is Docker Compose Override - GeeksforGeeks](https://www.geeksforgeeks.org/devops/docker-compose-override/)
- [Tips and Tricks for Docker Compose - DEV Community](https://dev.to/aless10/tips-and-tricks-for-docker-compose-leveraging-the-override-feature-4hj0)

---

## 8. systemd - Condition Directives

### Behavior: **ADDITIVE (all conditions must be true)**

Multiple `Condition*=` directives create an implicit AND. All must be satisfied for the unit to start.

### Syntax

```ini
[Unit]
Description=Example Service
ConditionPathExists=/etc/example.conf
ConditionVirtualization=!container
ConditionHost=production-server
ConditionMemory=>4G

[Service]
ExecStart=/usr/bin/example-daemon
```

### Available Condition Directives

- `ConditionPathExists=` - File exists
- `ConditionPathExistsGlob=` - At least one file matches glob
- `ConditionPathIsDirectory=`
- `ConditionPathIsMountPoint=`
- `ConditionPathIsReadWrite=`
- `ConditionFileIsExecutable=`
- `ConditionDirectoryNotEmpty=`
- `ConditionVirtualization=` - Running in specific virtualization
- `ConditionHost=` - Hostname matches
- `ConditionKernelCommandLine=`
- `ConditionKernelVersion=`
- `ConditionMemory=` - Available memory check
- `ConditionCPUs=` - CPU count check
- `ConditionArchitecture=`
- `ConditionSecurity=` - Security features available

### Negation

Prefix with `!` to negate:

```ini
ConditionPathExists=!/etc/disable-service
ConditionVirtualization=!container
```

### How It Works

- **All must be true**: All `Condition*=` directives must succeed
- **Graceful skip**: If conditions fail, unit is skipped (not failed)
- **vs Assert**: `Assert*=` directives cause failure (not skip) when false

### Condition vs Assert

From the documentation:
> "Condition directives allow the administrator to test certain conditions prior to starting the unit. This can be used to provide a generic unit file that will only be run on appropriate systems. If the condition is not met, the unit is gracefully skipped."

> "Assert directives are similar to Condition directives - they check for different aspects of the running environment to decide whether the unit should activate. However, unlike the Condition directives, a negative result causes a failure with this directive."

### Nested Virtualization

> "If multiple virtualization technologies are nested, only the innermost is considered."

### Documentation

- [systemd.unit - freedesktop.org](https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html)
- [Understanding Systemd Units - DigitalOcean](https://www.digitalocean.com/community/tutorials/understanding-systemd-units-and-unit-files)

---

## 9. Bazel - `select()` and Platform-Specific Dependencies

### Behavior: **UNAMBIGUOUS MATCH REQUIRED (exclusive-like)**

Bazel requires that when multiple conditions match, either all resolve to the same value or one is strictly more specific than others.

### Syntax

```python
cc_library(
    name = "mylib",
    srcs = select({
        "@platforms//os:linux": ["linux_impl.cc"],
        "@platforms//os:macos": ["macos_impl.cc"],
        "@platforms//os:windows": ["windows_impl.cc"],
        "//conditions:default": ["generic_impl.cc"],
    }),
    deps = select({
        "@platforms//cpu:x86_64": [":x86_deps"],
        "@platforms//cpu:aarch64": [":arm_deps"],
    }),
)
```

### Unambiguous Specialization

When multiple conditions match, one of two scenarios must be true:

1. **All resolve to same value**:
```python
select({
    "@platforms//os:linux": "Hello",
    "@platforms//cpu:x86_64": "Hello",
})
# OK on linux x86_64 - both return "Hello"
```

2. **One is strictly more specific**:
```python
select({
    "@platforms//cpu:x86": "x86 build",
    ":x86_dbg": "x86 debug build",  # More specific
})

config_setting(
    name = "x86_dbg",
    values = {"cpu": "x86", "compilation_mode": "dbg"}
)
# OK - :x86_dbg is more specific than cpu:x86
```

### Known Issue with Constraint Values

From [Issue #14604](https://github.com/bazelbuild/bazel/issues/14604):

There's a bug where constraint value specialization doesn't work correctly:

```python
# This SHOULD work but doesn't:
select({
    "@platforms//os:ios": "ios",
    ":ios-aarch64": "ios-aarch64",  # More specific
})

config_setting(
    name = "ios-aarch64",
    constraint_values = [
        "@platforms//os:ios",
        "@platforms//cpu:aarch64",
    ]
)
# ERROR: Illegal ambiguous match (even though ios-aarch64 is more specific)
```

**Workaround**: Use `values` parameter instead of `constraint_values` for specialization.

### OR Logic with `selects.with_or()`

```python
load("@bazel_skylib//lib:selects.bzl", "selects")

cc_library(
    name = "mylib",
    deps = selects.with_or({
        ("@platforms//os:linux", "@platforms//os:macos"): [":posix_impl"],
        "@platforms//os:windows": [":windows_impl"],
    }),
)
```

### AND Logic

Chain nested `select()` statements or use `config_setting` with multiple conditions.

### Best Practices

> "Projects can `select()` on `constraint_value` targets but not complete platforms. This is intentional so that `select()`s support as wide a variety of machines as possible."

### Documentation

- [Configurable Build Attributes - Bazel](https://bazel.build/docs/configurable-attributes)
- [Platforms - Bazel](https://bazel.build/extending/platforms)

---

## 10. Nix - Conditionals and Platform-Specific Dependencies

### Behavior: **ADDITIVE (conditionals compose)**

Nix uses functional programming with conditional expressions. Multiple conditionals compose additively through function application.

### Syntax

Basic conditional:

```nix
if stdenv.isDarwin then darwinDeps else linuxDeps
```

Platform-specific dependencies:

```nix
buildInputs = [
  commonDep1
  commonDep2
] ++ lib.optionals stdenv.isDarwin [
  darwinOnlyDep
] ++ lib.optionals stdenv.isLinux [
  linuxOnlyDep
];
```

Using `lib.mkIf`:

```nix
config = lib.mkIf cfg.enable {
  services.myservice.enable = true;
};
```

### How It Works

- **Functional composition**: Conditionals are expressions that return values
- **List concatenation**: `++` combines lists from multiple conditions
- **`lib.optionals`**: Returns list if condition true, empty list otherwise

### Platform Detection

- `stdenv.isDarwin` - macOS
- `stdenv.isLinux` - Linux
- `stdenv.hostPlatform.isLinux`
- `stdenv.hostPlatform.isAarch64`

### Home-Manager Cross-Platform Challenges

From [Issue #1906](https://github.com/nix-community/home-manager/issues/1906):

Home-manager conditionally defines modules based on platform, making cross-platform configurations awkward:

```nix
# This fails on Darwin because the option doesn't exist:
config = lib.mkIf pkgs.stdenv.hostPlatform.isLinux {
  services.lorri.enable = true;
};
```

**Issue**: Can't reference Linux-only options even in conditional blocks on Darwin.

**Workaround**: Separate configuration files per platform.

### Assertions

```nix
assert sslSupport -> openssl != null;
```

Assertions check requirements between features and dependencies. Unlike conditions, they cause build failures.

### Build Inputs Cross-Compilation

- `nativeBuildInputs` - Tools that run on build platform
- `buildInputs` - Libraries for target platform
- Miscategorization breaks cross-compilation

### Documentation

- [Conditional configuration - NixOS Blog](https://tsawyer87.github.io/posts/conditional_configuration/)
- [Language Constructs - Nix Manual](https://nix.dev/manual/nix/2.13/language/constructs)

---

## Comparison Table

| Project | TOML/YAML | Semantics | Multiple Matches | Precedence Rules | Notes |
|---------|-----------|-----------|------------------|------------------|-------|
| **Cargo** | TOML | Additive | All sections apply | None (union) | Features merge across targets by default |
| **Poetry** | TOML | Mutual Exclusion | Error if overlap | N/A | Enforces non-overlapping conditions |
| **GitLab CI** | YAML | Sequential First-Match | First match wins | Top-to-bottom order | Workflow rules override job rules |
| **GitHub Actions** | YAML | Additive | All steps execute | None | Each step independent |
| **Ansible** | YAML | Additive (AND) | All must be true | None | List format = implicit AND |
| **Homebrew** | Ruby DSL | Additive | All blocks execute | None | Nesting creates AND conditions |
| **Docker Compose** | YAML | Hybrid | Overlay (files) / Exclusive (platform) | File order | Override files merge |
| **systemd** | INI | Additive (AND) | All must be true | None | Graceful skip vs assert failure |
| **Bazel** | Python DSL | Unambiguous Required | Must be same or specialized | Specialization hierarchy | Error on ambiguity |
| **Nix** | Nix Lang | Functional | Composition | None | List concatenation with optionals |

---

## Analysis: Additive vs Exclusive Patterns

### Additive Pattern Characteristics

**Used by**: Cargo, GitHub Actions, Ansible, Homebrew, systemd, Nix

**Advantages**:
1. **Predictable**: All matching conditions contribute
2. **Composable**: Easy to reason about combined effects
3. **Extensible**: Add new conditions without changing existing ones
4. **Declarative**: Describes "what should be included" rather than control flow

**Disadvantages**:
1. **Potential conflicts**: Multiple matches might provide conflicting values
2. **Order-independent**: Can't express "prefer X over Y"
3. **Verbose**: Might require negative conditions to exclude scenarios

**Best for**:
- Dependency declarations (union of all needed deps)
- Feature flags (multiple features can be active)
- Platform-specific additions (additive patches, resources)

### Exclusive Pattern Characteristics

**Used by**: GitLab CI (sequential), Bazel (unambiguous)

**Advantages**:
1. **Unambiguous**: Only one branch executes
2. **Control flow**: Express priorities and fallbacks
3. **Simpler mental model**: "First to match wins"

**Disadvantages**:
1. **Order-dependent**: Fragile to reordering
2. **Less declarative**: More like imperative control flow
3. **Hard to compose**: Can't easily combine conditions

**Best for**:
- Workflow decisions (run or skip)
- Override behavior (most specific wins)
- Imperative scripts

### Mutual Exclusivity Pattern

**Used by**: Poetry, Bazel

**Advantages**:
1. **Explicit conflict detection**: Errors prevent ambiguity
2. **Clear semantics**: Each condition owns its scope
3. **Validation**: Catches configuration mistakes

**Disadvantages**:
1. **Rigid**: Can't express overlapping valid scenarios
2. **Complex conditions**: Need carefully crafted non-overlapping predicates
3. **Limited reuse**: Hard to share conditions across declarations

**Best for**:
- Version resolution (only one version per package)
- Single-value selections (choose one implementation)

---

## Implications for Tsuku `when` Clauses

### Context: Tsuku Recipe Actions

Tsuku recipes define installation actions with optional `when` clauses:

```toml
[[action]]
type = "install_binaries"
bins = ["tsuku"]

[[action]]
type = "install_binaries"
bins = ["tsuku-helper"]
when.arch = "x86_64"

[[action]]
type = "setup_env"
when.os = "linux"
```

### Question: What if multiple actions could match?

```toml
[[action]]
type = "install_binaries"
bins = ["common-tool"]

[[action]]
type = "install_binaries"
bins = ["x86-tool"]
when.arch = "x86_64"

[[action]]
type = "install_binaries"
bins = ["linux-tool"]
when.os = "linux"
```

On `linux/x86_64`, should:
- **Additive**: Install all three (common, x86, linux)
- **Exclusive**: Install most specific (needs precedence rules)

### Recommendation: **Additive Semantics**

**Rationale**:

1. **Ecosystem precedent**: Cargo (closest analog) uses additive
2. **Declarative nature**: Recipes describe "what to install," not "how to decide"
3. **Composability**: Easy to express "always install X, plus Y on linux, plus Z on arm"
4. **Predictability**: Users can mentally "union" all matching conditions
5. **TOML philosophy**: TOML is for configuration (declarative), not scripts (imperative)

**Implementation approach**:

```
For each action in recipe:
  if action.when is empty OR all conditions in action.when match:
    execute action
```

Each action independently evaluated. All matching actions execute in order.

### Alternative: Exclusive with Precedence

If exclusive semantics were needed, would require:

```toml
[[action]]
type = "install_binaries"
bins = ["common-tool"]
# No when = lowest precedence

[[action]]
type = "install_binaries"
bins = ["x86-tool"]
when.arch = "x86_64"
# Single condition = medium precedence

[[action]]
type = "install_binaries"
bins = ["linux-x86-tool"]
when.os = "linux"
when.arch = "x86_64"
# Multiple conditions = highest precedence
```

**Problems**:
- Implicit precedence hard to document
- Order-dependent behavior confusing
- Against TOML declarative nature

### Hybrid Approach: Explicit Priority

Could support optional priority field:

```toml
[[action]]
type = "install_binaries"
bins = ["fallback-tool"]
priority = 1

[[action]]
type = "install_binaries"
bins = ["preferred-tool"]
when.os = "linux"
priority = 10
```

Highest priority action that matches wins.

**Problems**:
- More complex
- Rare use case
- Can achieve with negative conditions in additive model

---

## Recommendations for Tsuku

### Primary Recommendation: Additive Semantics

Implement `when` clauses with additive semantics where all matching actions execute:

**Pros**:
- Aligns with Cargo, Homebrew, systemd, Ansible, GitHub Actions
- Natural for dependency-style declarations
- Easier to reason about
- Fits TOML's declarative nature
- Simpler implementation

**Cons**:
- Can't express "prefer X over Y" without restructuring recipe
- Requires negative conditions for mutual exclusion

**Mitigation for cons**:
- Document best practices for common patterns
- Provide examples of negative conditions
- Recipe validation can detect common mistakes

### When to Use Additive

✅ **Good fit**:
```toml
# Install base tool everywhere, plus platform-specific helpers
[[action]]
type = "install_binaries"
bins = ["kubectl"]

[[action]]
type = "install_binaries"
bins = ["kubectl-macos-helper"]
when.os = "darwin"

[[action]]
type = "setup_env"
vars = { "KUBECTL_PLATFORM" = "unix" }
when.os = ["linux", "darwin"]
```

❌ **Requires care** (mutual exclusion):
```toml
# Don't want both installed - use negative conditions
[[action]]
type = "install_binaries"
bins = ["psycopg2"]
when.os = ["linux", "windows"]  # Not darwin

[[action]]
type = "install_binaries"
bins = ["psycopg2-binary"]
when.os = "darwin"
```

### Alternative Considered: Poetry-Style Mutual Exclusion

Enforce non-overlapping `when` clauses for actions of the same type.

**Rejected because**:
- Too restrictive for common patterns (base + platform-specific)
- Complex validation logic
- Error messages confusing for users
- Against Tsuku's "easy to write recipes" goal

### Alternative Considered: GitLab-Style Sequential

First matching action wins.

**Rejected because**:
- Order-dependent behavior hard to reason about
- Fragile to reordering during edits
- Against TOML's declarative philosophy
- No precedent in package managers (GitLab is CI/CD workflow)

---

## Documentation Strategy

If implementing additive semantics, documentation should:

1. **Clearly state the model**:
   > "When multiple actions have `when` clauses that match the current platform, all matching actions execute in the order they appear in the recipe."

2. **Show common patterns**:
   - Base + platform-specific (additive)
   - Mutual exclusion (using negative conditions or non-overlapping predicates)
   - Nested conditions (all must match)

3. **Provide examples**:
```toml
# Pattern 1: Base + Platform-Specific Additions
[[action]]
type = "install_binaries"
bins = ["main-tool"]

[[action]]
type = "install_binaries"
bins = ["linux-only-helper"]
when.os = "linux"

# Pattern 2: Mutual Exclusion with Negative Conditions
[[action]]
type = "install_binaries"
bins = ["unix-impl"]
when.os = ["linux", "darwin"]

[[action]]
type = "install_binaries"
bins = ["windows-impl"]
when.os = "windows"

# Pattern 3: Specific Override Pattern
# (Use separate actions for different configurations)
[[action]]
type = "install_binaries"
bins = ["default-binary"]
when.arch = "x86_64"

[[action]]
type = "install_binaries"
bins = ["arm-binary"]
when.arch = ["arm64", "aarch64"]
```

4. **Recipe validation warnings**:
   - Detect overlapping `install_binaries` actions (potential conflict)
   - Warn if same binary name in multiple actions with overlapping conditions
   - Suggest using non-overlapping conditions

---

## Summary

**Key Finding**: Additive semantics dominate the ecosystem, especially in package managers and configuration systems (Cargo, Homebrew, systemd, Nix). Sequential/exclusive patterns appear primarily in workflow/CI systems (GitLab CI).

**Recommendation**: Implement additive semantics for Tsuku `when` clauses. All matching actions execute. This aligns with:
- Industry precedent from similar tools (Cargo, Homebrew)
- TOML's declarative nature
- User expectations from package managers
- Simplicity of implementation and mental model

**Trade-offs accepted**: Users must use negative conditions or non-overlapping predicates for mutual exclusion scenarios. This is well-precedented (Cargo, systemd, Ansible) and can be documented with clear examples.

---

## Sources

### Cargo
- [Specifying Dependencies - The Cargo Book](https://doc.rust-lang.org/cargo/reference/specifying-dependencies.html)
- [Features of dependencies merge between different targets - Issue #6121](https://github.com/rust-lang/cargo/issues/6121)
- [Features - The Cargo Book](https://doc.rust-lang.org/cargo/reference/features.html)
- [RFC 1361: cargo-cfg-dependencies](https://rust-lang.github.io/rfcs/1361-cargo-cfg-dependencies.html)

### Poetry
- [Dependency specification - Poetry Documentation](https://python-poetry.org/docs/dependency-specification/)
- [Platform based dependencies in poetry](https://www.whatsdoom.com/posts/2022/07/24/platform-based-dependencies-in-poetry/)

### GitLab CI
- [Specify when jobs run with rules - GitLab Docs](https://docs.gitlab.com/ci/jobs/job_rules/)
- [GitLab CI Rules – Change Pipeline Workflow](https://www.bitslovers.com/gitlab-ci-rules/)

### GitHub Actions
- [Using conditions to control job execution - GitHub Docs](https://docs.github.com/en/actions/writing-workflows/choosing-when-your-workflow-runs/using-conditions-to-control-job-execution)
- [Evaluate expressions in workflows - GitHub Docs](https://docs.github.com/en/actions/writing-workflows/choosing-what-your-workflow-does/evaluate-expressions-in-workflows-and-actions)
- [Using conditionals in GitHub Actions](https://www.brycewray.com/posts/2023/04/using-conditionals-github-actions/)

### Ansible
- [Conditionals - Ansible Documentation](https://docs.ansible.com/ansible/latest/playbook_guide/playbooks_conditionals.html)
- [How to define multiple when conditions in Ansible](https://www.cyberciti.biz/faq/how-to-define-multiple-when-conditions-in-ansible/)
- [Ansible Conditionals - Spacelift](https://spacelift.io/blog/ansible-when-conditional)

### Homebrew
- [Formula Cookbook - Homebrew Documentation](https://docs.brew.sh/Formula-Cookbook)
- [Pull Request #8907: add on_linux and on_macos blocks](https://github.com/Homebrew/brew/pull/8907)
- [Issue #11398: keg_only inside on_macos/on_linux blocks](https://github.com/Homebrew/brew/issues/11398)

### Docker Compose
- [What is Docker Compose Override - GeeksforGeeks](https://www.geeksforgeeks.org/devops/docker-compose-override/)
- [Tips and Tricks for Docker Compose: Leveraging the override Feature](https://dev.to/aless10/tips-and-tricks-for-docker-compose-leveraging-the-override-feature-4hj0)

### systemd
- [systemd.unit - freedesktop.org](https://www.freedesktop.org/software/systemd/man/latest/systemd.unit.html)
- [Understanding Systemd Units and Unit Files - DigitalOcean](https://www.digitalocean.com/community/tutorials/understanding-systemd-units-and-unit-files)

### Bazel
- [Configurable Build Attributes - Bazel](https://bazel.build/docs/configurable-attributes)
- [Platforms - Bazel](https://bazel.build/extending/platforms)
- [Issue #14604: select unambiguous specialization with constraint_values](https://github.com/bazelbuild/bazel/issues/14604)

### Nix
- [Conditional configuration - NixOS Blog](https://tsawyer87.github.io/posts/conditional_configuration/)
- [Language Constructs - Nix Reference Manual](https://nix.dev/manual/nix/2.13/language/constructs)
- [Issue #1906: Conditionally-defined modules - home-manager](https://github.com/nix-community/home-manager/issues/1906)
