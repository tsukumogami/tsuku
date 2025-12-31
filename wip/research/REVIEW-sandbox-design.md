# Review: DESIGN-structured-install-guide.md (Sandbox Container Building)

## Summary

This review assesses `docs/DESIGN-structured-install-guide.md` for completeness, alignment with the companion actions design, and implementation readiness. The design is well-structured with clear scope boundaries, but several gaps require attention before implementation.

---

## Issues

### Issue 1: Missing Runtime Interface Extension Details

**Severity**: Medium

The design mentions (line 614-617):

> **Implementation note:** The current `Runtime` interface (`internal/validate/runtime.go`) only supports `Run()`. Container building requires adding:
> - `Build(ctx context.Context, dockerfile string, imageName string) error`
> - `ImageExists(ctx context.Context, imageName string) (bool, error)`

**Problem**: The current `Runtime` interface (confirmed via code review) only has:
```go
type Runtime interface {
    Name() string
    IsRootless() bool
    Run(ctx context.Context, opts RunOptions) (*RunResult, error)
}
```

The design does not specify:
1. Where these new methods should be added (extend `Runtime` vs create new interface)
2. Error handling for build failures
3. How the Dockerfile content is passed (string vs file path)
4. Build context handling (what files are included in the build)
5. Whether `Build()` should stream logs or capture output

**Recommendation**: Add a "Runtime Interface Extension" section with concrete interface definitions.

---

### Issue 2: Minimal Base Container Dockerfile is Underspecified

**Severity**: High

The design provides a minimal Dockerfile (lines 526-533):

```dockerfile
FROM scratch
COPY --from=builder /tsuku /usr/local/bin/tsuku
COPY --from=builder /lib/x86_64-linux-gnu/libc.so.6 /lib/x86_64-linux-gnu/
COPY --from=builder /lib64/ld-linux-x86-64.so.2 /lib64/
# ... minimal runtime dependencies for tsuku binary
```

**Problems**:

1. **Multi-stage build undefined**: References `--from=builder` but no builder stage shown
2. **arm64 not addressed**: Only x86_64 paths shown; arm64 uses `/lib/aarch64-linux-gnu/`
3. **Missing dependencies**: Research (`findings_bootstrap-requirements.md`) identified tsuku needs:
   - SSL certificates (`ca-certificates`)
   - DNS resolution (`libnss_dns`, `libnss_files`, `libresolv`)
   - Locale archive (for proper Unicode handling)
   - `/etc/passwd` and `/etc/group` (for user context)
4. **No package manager**: Design says "Container cannot run `apt-get`" but then shows `RUN apt-get update && apt-get install -y docker.io` for derived containers. This is contradictory - the derived container needs apt.

**Recommendation**:
1. Show complete multi-stage Dockerfile for both architectures
2. Define the "builder" base image (e.g., `debian:bookworm-slim`)
3. Clarify that the minimal base is for the tsuku binary, but derived containers use standard distro images for package installation

---

### Issue 3: ExtractPackages Does Not Handle All Action Types

**Severity**: Medium

The `ExtractPackages()` function (lines 560-598) handles:
- `apt_install`
- `apt_repo`
- `brew_install`, `brew_cask`
- `dnf_install`
- `pacman_install`

**Missing from actions doc vocabulary**:
- `apt_ppa` - Adds Ubuntu PPA (should increment hasSystemDeps)
- `apk_install` - Alpine packages (listed in D6 constraints)
- `zypper_install` - openSUSE packages (listed in D6 constraints)
- `dnf_repo` - DNF repository setup

**Recommendation**: Update `ExtractPackages()` example to include all action types from the actions vocabulary.

---

### Issue 4: WhenClause Does Not Yet Support linux_family

**Severity**: High

The design assumes `linux_family` field exists in `WhenClause`, but current codebase shows:

```go
type WhenClause struct {
    Platform       []string `toml:"platform,omitempty"`
    OS             []string `toml:"os,omitempty"`
    PackageManager string   `toml:"package_manager,omitempty"`  // No linux_family!
}
```

This is correctly a Phase 1 dependency in the actions design, but the sandbox design references the field as if it already exists. This could cause confusion about implementation ordering.

**Recommendation**: Add explicit note that Phase 1 of actions design (infrastructure) must be complete before ExtractPackages can use linux_family.

---

### Issue 5: ContainerSpec Base Image Selection Logic Missing

**Severity**: Medium

The design shows `DeriveContainerSpec()` returning:

```go
return &ContainerSpec{
    Base:     MinimalBaseImage,
    Packages: packages,
}
```

**Problems**:
1. `MinimalBaseImage` is undefined - what image name?
2. Package managers require different base images:
   - apt needs Debian/Ubuntu base
   - dnf needs Fedora/RHEL base
   - pacman needs Arch base
3. No logic for selecting base image based on detected packages

**Recommendation**: Add logic to select appropriate base image based on package manager type in the extracted packages.

---

### Issue 6: brew_install/brew_cask Sandbox Strategy Unclear

**Severity**: Medium

The design focuses heavily on apt/dnf sandbox containers but doesn't address Homebrew:

1. Homebrew doesn't work in containers without special setup
2. macOS containers are not widely available
3. Should recipes with only `brew_*` actions skip sandbox testing?

**Recommendation**: Add explicit section on macOS/Homebrew sandbox strategy (likely: skip sandbox for darwin-only recipes, or use Linuxbrew in containers).

---

### Issue 7: Container Cache Cleanup Not Specified

**Severity**: Low

The design mentions "Cache eviction" as future work (line 851) but provides no mechanism for:
1. Detecting cache growth
2. Cleaning up unused images
3. User control over cache size

For local development this could accumulate many images.

**Recommendation**: Add basic cleanup strategy for MVP (e.g., `tsuku sandbox prune` command or cleanup on version upgrade).

---

## Suggestions

### Suggestion 1: Add Sequence Diagram for Sandbox Execution

A sequence diagram showing the flow:
1. Recipe loaded
2. Plan filtered for target platform
3. `ExtractPackages()` called
4. Container spec computed
5. Image built or retrieved from cache
6. Container executed
7. Results captured

This would clarify the interaction between components.

---

### Suggestion 2: Define ContainerSpec Schema

The design references `ContainerSpec` without defining it:

```go
type ContainerSpec struct {
    Base     string              // Base image name
    Packages map[string][]string // manager -> packages
    Repos    []Repository        // Custom repositories to add
    // What about apt_ppa, dnf_repo, etc.?
}
```

A complete schema definition would clarify what information flows from recipes to container building.

---

### Suggestion 3: Clarify Relationship to Golden Files

The design mentions issue #745 (enforce golden files for all recipes) as blocked by this work, but doesn't explain how:
1. Golden files are generated from sandbox execution
2. Golden files differ with/without system dependencies
3. Container images affect golden file content

Adding a subsection would connect the designs.

---

### Suggestion 4: Add Implementation Example for apt_repo

The `apt_repo` action is complex (GPG key download, sources.list modification). The design should show how `ExtractPackages()` handles it:

```go
case "apt_repo":
    hasSystemDeps = true
    // Don't add to packages, but mark that we need apt infrastructure
    // Actual repo setup happens during container build
    repos = append(repos, RepoSpec{
        URL:       step.Params["url"].(string),
        KeyURL:    step.Params["key_url"].(string),
        KeySHA256: step.Params["key_sha256"].(string),
    })
```

---

### Suggestion 5: Consider Build Caching Beyond Image Name

The design hashes package lists for image naming, but doesn't consider:
1. Base image version changes
2. Package version updates upstream
3. Repository configuration changes

For CI stability, consider adding timestamp-based cache invalidation or upstream version tracking.

---

## Questions

### Q1: What happens when sandbox testing is unavailable?

If a developer doesn't have Docker/Podman:
- Should preflight skip sandbox-dependent validation?
- Should CI be the only place sandbox tests run?
- How does this affect the migration from current base images?

### Q2: How are action execution errors surfaced?

When `apt-get install` fails inside the container:
- Is the full output captured?
- Are network errors distinguished from package-not-found?
- How does error reporting differ from current `require_system` errors?

### Q3: Should derived container Dockerfiles be persisted?

For debugging and reproducibility:
- Are Dockerfiles written to `$TSUKU_HOME/cache/containers/`?
- Can users inspect/rebuild manually?
- Should `tsuku sandbox show <recipe>` print the Dockerfile?

### Q4: How does this interact with the existing sandbox timeout?

Current `ResourceLimits.Timeout` is 2 minutes. Container builds (especially with package downloads) may exceed this. Should build time be separate from run time limits?

### Q5: What base image should be used for "universal" testing?

Some actions (like `require_command`) don't have implicit platform constraints. If a recipe has:
```toml
[[steps]]
action = "require_command"
command = "git"
```

What container tests this? The current debian base? The minimal base?

---

## Alignment Verification

### Actions Doc Reference Check

| Reference | Status | Notes |
|-----------|--------|-------|
| D2 (linux_family detection) | Correct | Properly references detection mechanism |
| D6 (hardcoded when clauses) | Correct | Design respects implicit constraints |
| Action vocabulary table | Partially aligned | Missing `apk_install`, `zypper_install` in code example |
| Describe() interface | Correct | Properly references actions doc |
| Documentation generation | Correct | Defers to actions doc |

### Options Analysis Accuracy

The options analysis remains accurate post-linux_family adoption:
- Option 1A vs 1B correctly identifies platform keys in parameter vs when clause
- Option 3A (minimal container) rationale is sound
- Option 4C (extensible core) is the right choice

### Implementation Phase Sequencing

Phases are properly ordered:
1. Phase 1 (Adopt Action Vocabulary) depends on actions design phases 1-3
2. Phase 2 (Documentation Generation) can proceed in parallel with Phase 3
3. Phase 3 (Sandbox Container Building) correctly separated from action vocabulary

However, Phase 1 step 4 ("Migrate existing recipes") references `docker.toml`, `cuda.toml`, `test-tuples.toml` but grep search found no `require_system` actions in current recipes. These files may not exist or may have been removed.

**Recommendation**: Verify which recipes need migration or update the design to reflect current state.

### Future Work Relevance

All Future Work items remain relevant and are not duplicated with actions doc:
1. Host Execution - correctly deferred with cross-reference
2. Tiered Extension Model - appropriate for post-MVP
3. Automatic Action Analysis - useful for migration at scale
4. Platform Version Constraints - correctly deferred
5. Container Cache Optimization - appropriate for post-MVP
6. Privilege Escalation Paths - correctly references actions doc

---

## Conclusion

The design is well-conceived and properly scoped. The primary issues are:

1. **Minimal base container needs more specification** (multi-arch, dependencies)
2. **Runtime interface extension needs concrete definition**
3. **WhenClause linux_family dependency should be more explicit**
4. **Homebrew sandbox strategy needs clarification**

These gaps are addressable within the existing structure. The design correctly identifies the boundary between action vocabulary (actions doc) and container building (this doc), and maintains appropriate cross-references.

**Recommended Priority**:
1. Address Issue 2 (base container) - blocks implementation
2. Address Issue 1 (Runtime interface) - blocks implementation
3. Address Issue 4 (linux_family dependency) - clarifies ordering
4. Address remaining issues and suggestions
