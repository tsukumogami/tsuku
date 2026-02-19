# Design Review: GPU Backend Selection

Reviewer: Architect Reviewer
Document: `docs/designs/DESIGN-gpu-backend-selection.md`
Date: 2026-02-19

---

## 1. Problem Statement Specificity

The problem statement is strong. It names four concrete failure modes (wrong binary, late detection, CUDA version coupling, Vulkan VRAM zero) and traces each to a specific code location or behavior. The current manifest schema, `PlatformKey()` implementation, and the gap between CI output (10 variants) and manifest capacity (5 entries) are all quantified. This is enough to evaluate solutions against.

One gap: the problem statement says "CUDA driver mismatches fail silently" but later in the design the fix is Vulkan-as-default (avoiding CUDA) rather than fixing silent failure for users who explicitly choose CUDA. The runtime fallback (Decision 4) addresses this, but the problem statement should be explicit that users who force CUDA will still face silent degradation if the driver version check in `backend_available()` doesn't test driver compatibility -- it only checks if the hardware detection matches the compiled backend, not the driver version.

## 2. Missing Alternatives

**Manifest alternatives**: The design considers three manifest schemas but misses a fourth: keep the current flat schema and add a separate backend-resolution endpoint or embedded lookup table. The manifest stays simple (one URL per composite key like `linux-amd64-vulkan`), and a separate `backends.json` maps `GOOS-GOARCH` to ordered backend lists. This separates "what's available" from "where to download it" and avoids the nesting complexity. Not necessarily better, but worth rejecting explicitly.

**Detection alternatives**: The design doesn't consider `ldconfig -p` output parsing on Linux. `ldconfig` is nearly universal on glibc-based systems and provides a complete library path map without hardcoded paths. It would address the container/Nix/non-standard distro concern flagged below.

**Fallback alternatives**: The design doesn't consider keeping the first-downloaded GPU variant and dynamically loading CPU fallback via LD_PRELOAD or similar. This is probably correctly out of scope (too fragile), but the "dynamic backend loading" rejection in Decision 1 is about a different thing (shipping .so files separately).

## 3. Rejection Rationale Assessment

**Flat composite keys (Decision 2)**: The rejection says flat keys make it "impossible to enumerate available backends for a platform without scanning all keys with a prefix match." This overstates the difficulty. Given Go's `strings.HasPrefix`, enumerating backends for `linux-amd64` from keys like `linux-amd64-cuda`, `linux-amd64-vulkan`, `linux-amd64-cpu` is trivial:

```go
func backendsForPlatform(platforms map[string]VariantInfo, prefix string) []string {
    var backends []string
    for key := range platforms {
        if strings.HasPrefix(key, prefix+"-") {
            backends = append(backends, strings.TrimPrefix(key, prefix+"-"))
        }
    }
    return backends
}
```

That said, the nested schema is cleaner for a different reason: it makes the `default` field natural and avoids ambiguity about where the platform key ends and the backend key begins (e.g., a hypothetical platform `linux-amd64-v2` would conflict with `linux-amd64-v2-cuda`). The rejection should be rewritten around schema clarity rather than enumeration impossibility.

**Priority-ordered variant list (Decision 2)**: The rejection that "arrays require linear scan" is weak -- these arrays have at most 3 elements. The real problem is that priority should be determined by the detection system, not baked into the manifest, and the design says this in its second sentence. Lead with the stronger argument.

**Single bundled binary (Decision 1)**: Rejection is fair and well-reasoned. The probe cost argument (tsuku-llm starts on demand, not persistent) is a strong differentiator from Ollama.

**Shell-out to nvidia-smi (Decision 3)**: Rejection is fair. Tool availability is genuinely unreliable.

## 4. Unstated Assumptions

**A. Library presence implies library functionality.** The `probeVulkan()` function checks `os.Stat("/usr/lib/x86_64-linux-gnu/libvulkan.so.1")`. Finding the file doesn't mean Vulkan works. A system could have `libvulkan.so.1` installed (from mesa) but no GPU driver that supports it. The design acknowledges this risk in the "runtime fallback" section but doesn't state it as an explicit assumption in the detection section.

**B. Hardcoded library paths are sufficient.** The probe paths are Debian/Ubuntu-specific:
- `/usr/lib/x86_64-linux-gnu/` -- Debian multiarch layout
- `/usr/local/cuda/lib64/` -- NVIDIA CUDA toolkit default
- No Fedora/RHEL paths (`/usr/lib64/`)
- No Arch Linux paths (`/usr/lib/`)
- No NixOS paths (`/nix/store/...`)
- No container paths (NVIDIA Container Toolkit mounts to `/usr/lib/x86_64-linux-gnu/` inside the container, but only if `--gpus` is passed to `docker run`)
- No Flatpak/Snap paths

This is a meaningful gap. Missing Fedora paths means `probeVulkan()` returns false on a Fedora system with Vulkan installed, causing a CPU download instead. The design's own "Consequences > Negative" section mentions containers but not mainstream distros with different lib layouts.

**C. ARM64 library paths are the same pattern.** The code probes `/usr/lib/aarch64-linux-gnu/` which is correct for Debian ARM64 but the same Fedora/Arch/Nix gap applies.

**D. The Rust binary exits before gRPC starts on backend failure.** The design says exit code 78 is used when the backend fails. But looking at the proposed Rust code, `backend_available()` is called early in `main()`, before the gRPC server starts. This means the Go-side `waitForReady()` will time out (10 seconds), then check the exit code. The 10-second timeout is unnecessary latency in the failure case. The design should consider having the Go side wait on the process exit rather than waiting for the full timeout.

**E. `vulkaninfo` parses reliably.** The Vulkan VRAM fix shells out to `vulkaninfo --summary` and parses its text output. The output format of `vulkaninfo` is not a stable API. Different versions produce different formatting. This is acknowledged as a risk but the assumption that the parsing will work across vulkaninfo versions should be explicit.

**F. Manifest version bump is sufficient for backward compatibility.** The design says the schema change is "fine because the manifest is embedded at compile time." This is true -- old tsuku binaries ship their own old manifest. But if a user has a new manifest in a cached registry location that gets read by an old binary, the parse would fail. The design should confirm that the manifest is only read from `//go:embed`, never from a cached file.

## 5. Strawman Check

No option appears to be a strawman. Each alternative is presented with genuine advantages and rejected on specific grounds. The "user configuration only" option (Decision 3) is the weakest contender, but its inclusion is warranted because it establishes that auto-detection is a design choice, not an inevitability.

## 6. Decision Drivers Completeness

The decision drivers are solid. Two architectural concerns are missing:

**Testability.** The design proposes `probeBackends()` as a platform-specific function using build tags (`detect_linux.go`, `detect_darwin.go`). But the detection is based on `os.Stat()` calls to hardcoded paths. Unit testing requires either:
- Injecting the file system (an interface wrapping `os.Stat`)
- Using build tags for test doubles
- Testing only via integration tests with mock files

The design doesn't address this. The existing codebase uses `t.TempDir()` and `t.Setenv()` for test isolation (visible in `manager_test.go`), but hardcoded absolute paths like `/usr/lib/x86_64-linux-gnu/libvulkan.so.1` can't be redirected with environment variables. This needs a testing strategy.

**Config key registration.** The design proposes `llm.backend` as a new config key in `$TSUKU_HOME/config.toml`. Looking at the existing `userconfig.go`, every config key must be registered in three places: the `Get()` switch statement (line 275), the `Set()` switch statement (line 315), and the `AvailableKeys()` map (line 387). The design should note this as an implementation detail to avoid the new key being settable but not gettable (or vice versa). This is a minor point but it's a pattern that the design should follow.

**Verification with backend segment.** The `verifyBinary()` method in `manager.go` currently calls `GetCurrentPlatformInfo()` which returns a single `PlatformInfo`. With the new schema, verification needs to know which backend variant is installed. The `verifyBinary` method (and `VerifyBeforeExecution`) will need the backend as a parameter, or the manager needs to track which backend was downloaded. This is a subtle contract change that the design doesn't address.

## 7. Scope Assessment

Scope is appropriate. The inclusions are tightly coupled: you can't expand the manifest without detection, and detection without fallback leaves a gap. The Vulkan VRAM fix is borderline -- it's a Rust-side bug fix that could ship independently -- but including it avoids the scenario where the design works correctly (detects Vulkan, downloads Vulkan variant) but then picks the wrong model due to zero VRAM.

The out-of-scope items are reasonable. Windows GPU, multi-CUDA, and dynamic backend loading are all genuinely separate concerns.

One thing that should be in scope but isn't explicitly mentioned: **the `AddonPath()` legacy function and `NewServerLifecycleWithManager()`**. The legacy `AddonPath()` function in `manager.go:232` returns a path without a backend segment. After this change, it'll point to the wrong location. The lifecycle manager's `NewServerLifecycleWithManager()` calls `AddonPath()` at construction time (`lifecycle.go:71`). Both need updating. The design's "Solution Architecture" section shows the `EnsureAddon` flow changing but doesn't mention these callers.

## 8. Specific Concerns Raised in the Review Request

### Nested manifest vs. flat composite keys

As discussed in section 3, the flat key rejection rationale is overstated. String prefix matching works fine for 3-5 entries. However, the nested schema is still the better choice because:
1. The `default` field is a natural part of the platform entry, not something you'd reconstruct from flat keys.
2. No ambiguity about key boundaries (platform vs. backend separator).
3. The Go type system makes the nesting free -- `PlatformEntry` is a clean type, not a string convention.

The recommendation is to keep the nested schema but rewrite the rejection of flat keys to focus on schema clarity rather than enumeration cost.

### Vulkan-over-CUDA as NVIDIA default

The design acknowledges this is unvalidated: "We haven't benchmarked Vulkan vs CUDA performance for the models we ship." This is listed as a risk but not treated as a blocker. For 1-3B parameter models (the sizes shipped), the performance difference between Vulkan and CUDA on NVIDIA hardware is typically 10-30% for inference (not training). Whether this matters depends on the use case: recipe generation latency where the user is waiting, or batch processing.

The design's reasoning (avoid CUDA driver coupling, broader hardware support) is sound for a default. But the risk section should be stronger: if benchmarks show >20% degradation, the default should flip to CUDA with Vulkan as fallback, and the detection logic should check CUDA driver version compatibility before selecting CUDA. The current design has no CUDA driver version check -- `probeCUDA()` only checks if `libcuda.so` exists, which is the same problem the design is trying to solve.

Recommendation: Add a decision gate -- benchmark Vulkan vs CUDA on NVIDIA before shipping. If the delta is <15%, Vulkan default is fine. If >25%, reconsider. Between 15-25%, add a note to the config override documentation.

### Library file probing in containers/Nix/non-standard distros

This is the design's weakest point. The hardcoded paths cover Debian/Ubuntu only. Specific gaps:

| Distribution | libvulkan.so location | Covered? |
|---|---|---|
| Debian/Ubuntu x86_64 | `/usr/lib/x86_64-linux-gnu/libvulkan.so.1` | Yes |
| Fedora/RHEL x86_64 | `/usr/lib64/libvulkan.so.1` | No |
| Arch Linux x86_64 | `/usr/lib/libvulkan.so.1` | No |
| NixOS | `/nix/store/<hash>-vulkan-loader-<ver>/lib/libvulkan.so.1` | No |
| Docker with NVIDIA CTK | Mounted at `/usr/lib/x86_64-linux-gnu/` | Yes (if Debian-based image) |
| Flatpak | Sandboxed, different paths | No |

The fallback chain means this isn't catastrophic -- wrong detection leads to CPU download, then runtime works. But Fedora is a major distribution; getting CPU instead of GPU on Fedora by default would be a bad experience.

Mitigation options:
1. Add Fedora/Arch paths to the probe list (simple, covers 90%+ of Linux users).
2. Use `ldconfig -p | grep libvulkan` to find the library regardless of path (covers all glibc systems).
3. Accept the gap and rely on `llm.backend` config override for non-Debian users.

Recommendation: At minimum, add `/usr/lib64/` and `/usr/lib/` to the probe list. The `ldconfig` approach is more correct but adds subprocess spawning, which the design rejected for `vulkaninfo`/`nvidia-smi`. The difference is that `ldconfig` is part of glibc and is effectively always present, unlike `vulkaninfo`.

### EX_CONFIG exit code race condition

There is no race condition in the strict sense. The Go side starts the process, then calls `waitForReady()` which polls the socket. If the Rust process exits before the socket is created, the poll loop runs until timeout (10 seconds), then fails. The design proposes checking the exit code after the timeout.

The issue isn't a race condition -- it's unnecessary latency. The Rust process exits with code 78 almost immediately (before starting the gRPC server), but the Go side waits 10 seconds before checking. The fix is straightforward: start a goroutine that waits on `cmd.Wait()` and sends the exit status on a channel. The `waitForReady` loop then selects on both the socket poll ticker and the process exit channel.

The design's `ServerLifecycle.EnsureRunning()` already monitors the process in a background goroutine (lifecycle.go:201-211), but `waitForReady()` doesn't check whether the process has exited. This is a pre-existing issue that the GPU backend change would make worse (because backend init failure is a new, expected failure mode where the process exits quickly).

Recommendation: Address this in the design. Either modify `waitForReady` to also watch for process exit, or accept the 10-second latency as a known cost of the fallback path.

## Summary of Findings

### Blocking (must address before implementation)

1. **Hardcoded probe paths miss major distros.** Adding Fedora (`/usr/lib64/`) and Arch (`/usr/lib/`) paths is a few lines of code and prevents CPU-only downloads for a significant user population. Without this, the detection is Ubuntu-specific, not Linux-generic.

2. **`verifyBinary()` contract change not addressed.** The current `verifyBinary` calls `GetCurrentPlatformInfo()` which returns a single checksum. With multiple variants per platform, verification needs to know which variant's checksum to use. The design must specify how the manager tracks the installed variant's checksum.

3. **Legacy `AddonPath()` and `NewServerLifecycleWithManager()` not updated.** These functions construct paths without a backend segment and will break after the directory layout change.

### Advisory (should fix, won't compound)

4. **Flat key rejection rationale is inaccurate.** "Impossible to enumerate" is wrong -- it's trivial with prefix matching. The nested schema is still the right call, but the rationale should focus on schema clarity and `default` field ergonomics.

5. **10-second timeout on backend failure.** The `waitForReady` loop doesn't watch for process exit, causing unnecessary delay when the Rust binary exits immediately with code 78. The fix (select on process exit channel) is contained to `lifecycle.go`.

6. **Vulkan vs CUDA benchmark gap.** The design acknowledges this risk but doesn't specify a decision gate. Add explicit criteria for when to reconsider the Vulkan default.

7. **`vulkaninfo` output parsing fragility.** The text format isn't a stable API. Consider using the Vulkan C API directly (via `ash` crate which is likely already a dependency of llama.cpp's Vulkan backend) instead of shelling out to a tool that may not be installed.

### Out of Scope for This Review

- Whether llama.cpp's Vulkan backend is production-ready (correctness concern, not architecture)
- CI pipeline changes for manifest generation (implementation detail)
- Error message wording (readability concern)
