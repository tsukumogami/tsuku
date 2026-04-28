# Issue #2331: PyPI Version Pinning for pipx_install — Data Flow Analysis

**Research Lead:** L2  
**Task:** Trace data flow from recipe `pipx_install` step through plan generation to `PyPIProvider` construction. Identify whether bundled `python-standalone` version is reachable at provider construction time.

---

## Executive Summary

The `python-standalone` version is **resolved and available at plan generation time** (in `GeneratePlan()`), but it is **NOT automatically passed to PyPIProvider** during construction. The provider is created with only the PyPI package name; the python-standalone version must be plumbed through explicitly if we want PyPI to be aware of Python compatibility.

**Key finding:** `Decompose()` runs at eval-time (inside plan generation) with full access to the dependency tree and python-standalone version. However, `PyPIProvider` is constructed during version resolution (before decomposition), with no knowledge of python-standalone.

---

## Data Flow: Recipe Step → Plan Generation → Provider

### 1. Recipe Step Definition

File: Recipe TOML (e.g., `recipes/a/ansible.toml`)

```toml
[[steps]]
action = "pipx_install"
package = "ansible"
executables = ["ansible"]
```

The step is parsed into `recipe.Step` struct with `Action="pipx_install"` and `Params` map.

---

### 2. Plan Generation Entry Point

**File:** `internal/executor/plan_generator.go:71` — `GeneratePlan()`

Plan generation proceeds in this order:
1. **Version resolution** (line 128-150): Calls `e.resolveVersionWith()` to get `VersionInfo`
2. **EvalContext construction** (line 174-186): Creates context with resolved version, resolver, constraints
3. **Dependency discovery** (line 238-246): Calls `generateDependencyPlans()` to recursively resolve install-time deps
4. **Step resolution** (line 190-226): For each recipe step, calls `e.resolveStep()` which may decompose

---

### 3. Version Resolution (Before PyPIProvider)

**File:** `internal/executor/plan_generator.go:138-140`

```go
versionInfo, err := e.resolveVersionWith(ctx, resolver)
```

This resolves the **main recipe's version** (ansible's version), not the dependency (python-standalone).

At this point, the provider factory is already invoked via `e.resolveVersionWith()`. The PyPIProvider is constructed in `ProviderFactory.ProviderFromRecipe()` without knowledge of python-standalone.

---

### 4. Dependency Resolution (AFTER Version Resolution)

**File:** `internal/executor/plan_generator.go:238-246`

```go
deps, err := generateDependencyPlans(ctx, e.recipe, cfg, processed)
```

This recursively resolves **install-time dependencies** (python-standalone among them).

**Flow:**
- `generateDependencyPlans()` (line 664)
  → `ResolveDependenciesForTarget()` (internal/actions/resolver.go:78) extracts step-level deps
  → Calls `generateSingleDependencyPlan()` for each dep (line 726)
  → Loads dependency recipe via `cfg.RecipeLoader.GetWithContext()` (line 733)
  → Creates plan for dependency's install steps

**Key:** python-standalone version is resolved here, but only **after** the root recipe's version provider was created.

---

### 5. Step Decomposition (Where pipx_install Actually Fails)

**File:** `internal/executor/plan_generator.go:329-350` — `resolveStep()`

For decomposable actions (like `pipx_install`):

```go
if actions.IsDecomposable(step.Action) {
    // Check eval-time dependencies
    evalDeps := actions.GetEvalDeps(step.Action)  // Returns ["python-standalone"]
    missing := actions.CheckEvalDeps(evalDeps)
    
    // Decompose
    primitiveSteps, err := actions.DecomposeToPrimitives(evalCtx, step.Action, step.Params)
```

**At decomposition time:**
- `EvalContext` contains resolver, version info, but **NOT the python-standalone version**
- `evalCtx.Version` is ansible's version (e.g., "2.20")
- The `python-standalone` recipe has already been loaded into dependency plans, but that version is not in `evalCtx`

---

### 6. PyPIProvider Construction

**File:** `internal/version/provider_factory.go:148-178` — `PyPISourceStrategy`

Two paths construct `PyPIProvider`:

#### Path A: Explicit `source = "pypi"` (PyPISourceStrategy, line 150-178)

```go
func (s *PyPISourceStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
    for _, step := range r.Steps {
        if step.Action == "pipx_install" {
            if pkg, ok := step.Params["package"].(string); ok {
                return NewPyPIProvider(resolver, pkg), nil  // Line 173
            }
        }
    }
}
```

**Data available at construction:**
- `pkg` (PyPI package name from step params): ✓
- `resolver` (Resolver instance): ✓
- Python version: ✗ (NOT available)

#### Path B: Inferred (InferredPyPIStrategy, line 250-275)

```go
func (s *InferredPyPIStrategy) Create(resolver *Resolver, r *recipe.Recipe) (VersionProvider, error) {
    for _, step := range r.Steps {
        if step.Action == "pipx_install" {
            if pkg, ok := step.Params["package"].(string); ok {
                return NewPyPIProvider(resolver, pkg), nil  // Line 270
            }
        }
    }
}
```

**Data available:** Same as Path A.

**When construction happens:**
- Both paths are evaluated during `GeneratePlan()` at line 139, **before** decomposition
- `PyPIProvider` exists before `Decompose()` is called

---

### 7. Decompose Calls pip download

**File:** `internal/actions/pipx_install.go:283-359` — `Decompose()`

```go
func (a *PipxInstallAction) Decompose(ctx *EvalContext, params map[string]interface{}) ([]Step, error) {
    packageName, _ := GetString(params, "package")
    version := ctx.Version  // ansible version
    
    pythonPath := ResolvePythonStandalone()  // Line 318
    if pythonPath == "" {
        return nil, fmt.Errorf("python-standalone not found")
    }
    
    lockedRequirements, hasNativeAddons, err := generateLockedRequirements(
        ctx, pythonPath, packageName, version, tempDir)  // Line 331
}
```

**Critical line (call stack):**
- `generateLockedRequirements()` (line 404)
  → `runPipDownloadWithHashes()` (line 461)
  → Runs `pip download` command (line 483-494)

This is where **pip download fails for ansible 2.20** — pip doesn't know which Python version it's running with, so it downloads wheels incompatible with the bundled python-standalone version.

**Data available in Decompose:**
- `ctx.Version`: ansible's version ✓
- `ctx.Resolver`: the resolver instance ✓
- `ResolvePythonStandalone()`: calls OS directory listing, returns path ✓
- Python version: ONLY via `ResolvePythonStandalone()` filesystem check ✗ (no version metadata)

---

## Where python-standalone Version Lives

### Installation Path

**File:** `internal/actions/util.go:353-386` — `ResolvePythonStandalone()`

```go
func ResolvePythonStandalone() string {
    // Looks for $TSUKU_HOME/tools/python-standalone-* directories
    // Returns: /path/to/tools/python-standalone-20240511/bin/python3
}
```

This resolves the **installed binary** but not its version metadata.

### Recipe Definition

**File:** `internal/recipe/recipes/python-standalone.toml`

```toml
[metadata]
name = "python-standalone"
version_format = "custom"
```

The recipe has `version_format = "custom"`, meaning it uses the github_archive asset pattern which includes version in the asset tag (e.g., `cpython-*+20240511-x86_64-unknown-linux-gnu-install_only.tar.gz`).

---

## Data Available at Each Call Site

| Call Site | Data Available |
|-----------|-----------------|
| `GeneratePlan()` entry (plan_gen.go:71) | Target OS/arch, recipe, plan config |
| Version resolution (plan_gen.go:138) | Root recipe, resolver, target platform |
| PyPIProvider construction (provider_factory.go:173) | PyPI package name, resolver **ONLY** |
| Dependency discovery (plan_gen.go:238) | Full dependency tree (python-standalone version resolved here) |
| `resolveStep()` for pipx (plan_gen.go:320) | EvalContext with ansible version, resolver |
| `Decompose()` (pipx_install.go:283) | EvalContext, package name, ansible version, resolver |
| `pip download` command (pipx_install.go:483) | pythonPath from filesystem, no version metadata |

---

## Dependencies() Method

**File:** `internal/actions/pipx_install.go:20-28`

```go
func (PipxInstallAction) Dependencies() ActionDeps {
    return ActionDeps{
        InstallTime: []string{"python-standalone"},
        Runtime:     []string{"python-standalone"},
        EvalTime:    []string{"python-standalone"},  // <-- This is KEY
    }
}
```

**Declared:** pipx_install declares python-standalone as:
- InstallTime: ✓ (needed before running)
- EvalTime: ✓ (needed for Decompose to run pip download)
- Runtime: ✓ (needed during Execute)

**Where checked:**
- EvalTime deps checked at `resolveStep()` line 331 in plan_generator.go
- Checked via `CheckEvalDeps()` (eval_deps.go:19) — just filesystem check, no version

---

## The Plumbing Problem

### Current State: Version NOT Available at Provider Construction

1. Version resolution happens BEFORE dependency discovery
2. PyPIProvider is created with only package name, no Python version context
3. Dependency tree (including python-standalone version) is resolved AFTER provider creation
4. At `Decompose()` time, we have:
   - The resolver (which could theoretically query for python-standalone version)
   - The python-standalone binary path (from filesystem)
   - **But NO mechanism to tell the resolver "use Python 3.X compatibility"**

### What Would Be Needed for "Auto-Resolve"

To make PyPI aware of python-standalone version, we would need either:

**Option 1:** Pass python version to PyPIProvider at construction (plumbing issue)
- Requires moving dependency discovery before provider construction
- Or: plumb python version into provider post-construction (Option 2 below)
- Or: use cached python version from filesystem/config (Option 3)

**Option 2:** Pass python version via EvalContext at Decompose time
- PyPIProvider could be queried inside Decompose() for compatible versions
- Requires storing python-standalone version in a location accessible during decomposition

**Option 3:** Make pip download use `--python-requires` or similar constraints
- Doesn't fix the root issue; still needs to know Python version
- Would be set in Decompose(), not at provider construction

---

## Conclusion

**Can python-standalone version reach PyPIProvider construction time?**

Current answer: **No, not without surgery.**

- PyPIProvider is constructed during version resolution (line 139 of plan_generator.go)
- python-standalone version is resolved during dependency discovery (line 238 of plan_generator.go)
- These happen in opposite order, with no plumbing between them

**Earliest reachable point for python-standalone version:**
- Inside `Decompose()` at line 318 (via filesystem query)
- At dependency plan generation (line 238, but this is for the dependency plan, not readable by root recipe's Decompose)

**For the design "auto-resolve a Python-compatible version":**
- Must happen inside `Decompose()`, not at provider construction
- Can access resolver, package name, and python-standalone path
- Would need to either:
  1. Query resolver for "latest version compatible with Python X.Y"
  2. Store python version metadata somewhere accessible (config, env var, or computed from binary)
  3. Use pip's `--python-requires` to constrain resolution at download time
