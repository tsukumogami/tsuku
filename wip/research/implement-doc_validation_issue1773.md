# Validation Report: Issue #1773 (GPU Vendor Detection via PCI sysfs)

## Environment
- Platform: Linux 6.14.0-37-generic, amd64
- Go: standard toolchain
- Date: 2026-02-19

---

## Scenario 1: GPU detection returns valid value on Linux
**ID**: scenario-1
**Status**: PASSED

**Command**: `go test ./internal/platform/ -run 'TestDetectGPU$' -v -count=1`

**Output**:
```
=== RUN   TestDetectGPU
--- PASS: TestDetectGPU (0.00s)
PASS
ok  github.com/tsukumogami/tsuku/internal/platform  0.002s
```

**Analysis**: `DetectGPU()` runs on the real system and returns a value from `ValidGPUTypes`. On this CI-like Linux host it returned successfully. The test validates the return value is one of the five valid strings ("nvidia", "amd", "intel", "apple", "none").

---

## Scenario 2: GPU detection with mock sysfs for each vendor
**ID**: scenario-2
**Status**: PASSED

**Command**: `go test ./internal/platform/ -run 'TestDetectGPUWithRoot' -v -count=1`

**Output**:
```
=== RUN   TestDetectGPUWithRoot_Nvidia
--- PASS: TestDetectGPUWithRoot_Nvidia (0.00s)
=== RUN   TestDetectGPUWithRoot_AMD
--- PASS: TestDetectGPUWithRoot_AMD (0.00s)
=== RUN   TestDetectGPUWithRoot_Intel
--- PASS: TestDetectGPUWithRoot_Intel (0.00s)
=== RUN   TestDetectGPUWithRoot_NvidiaIntel
--- PASS: TestDetectGPUWithRoot_NvidiaIntel (0.00s)
=== RUN   TestDetectGPUWithRoot_AMDIntel
--- PASS: TestDetectGPUWithRoot_AMDIntel (0.00s)
=== RUN   TestDetectGPUWithRoot_None
--- PASS: TestDetectGPUWithRoot_None (0.00s)
=== RUN   TestDetectGPUWithRoot_EmptyRoot
--- PASS: TestDetectGPUWithRoot_EmptyRoot (0.00s)
PASS
ok  github.com/tsukumogami/tsuku/internal/platform  0.002s
```

**Analysis**: All 7 mock sysfs test cases pass:
- `nvidia/` -> "nvidia"
- `amd/` -> "amd"
- `intel/` -> "intel"
- `nvidia-intel/` dual GPU -> "nvidia" (discrete wins)
- `amd-intel/` dual GPU -> "amd" (discrete wins)
- `none/` no display controller -> "none"
- nonexistent root -> "none"

Testdata directory structure confirmed at `internal/platform/testdata/gpu/` with all 6 scenarios.

---

## Scenario 3: platform.Target carries GPU value
**ID**: scenario-3
**Status**: PASSED

**Command**: `go test ./internal/platform/ -run 'TestTarget.*GPU' -v -count=1`

**Output**:
```
=== RUN   TestTarget_GPU
=== RUN   TestTarget_GPU/nvidia_gpu
=== RUN   TestTarget_GPU/amd_gpu
=== RUN   TestTarget_GPU/intel_gpu
=== RUN   TestTarget_GPU/apple_gpu_on_darwin
=== RUN   TestTarget_GPU/no_gpu
=== RUN   TestTarget_GPU/empty_gpu
--- PASS: TestTarget_GPU (0.00s)
    --- PASS: TestTarget_GPU/nvidia_gpu (0.00s)
    --- PASS: TestTarget_GPU/amd_gpu (0.00s)
    --- PASS: TestTarget_GPU/intel_gpu (0.00s)
    --- PASS: TestTarget_GPU/apple_gpu_on_darwin (0.00s)
    --- PASS: TestTarget_GPU/no_gpu (0.00s)
    --- PASS: TestTarget_GPU/empty_gpu (0.00s)
=== RUN   TestTarget_SetGPU
--- PASS: TestTarget_SetGPU (0.00s)
PASS
ok  github.com/tsukumogami/tsuku/internal/platform  0.002s
```

**Analysis**:
- `NewTarget("linux/amd64", "debian", "glibc", "nvidia")` creates a Target whose `GPU()` returns "nvidia" -- confirmed.
- All 5 valid GPU values tested, plus empty string.
- `SetGPU()` helper works correctly: returns a new Target with updated GPU, original unchanged (value semantics).

---

## Scenario 4: Matchable interface includes GPU on both implementations
**ID**: scenario-4
**Status**: PASSED

**Commands**:
```
go build ./...  (exit code 0, no output)
go vet ./...    (exit code 0, no output)
```

**Analysis**:
- Full codebase builds with no errors, confirming all callsites for `NewTarget()` (4-arg) and `NewMatchTarget()` (5-arg) have been updated.
- `go vet` passes with no warnings.
- Verified via `go doc`:
  - `Matchable` interface includes `GPU() string`
  - `platform.Target.GPU()` method exists
  - `recipe.MatchTarget.GPU()` method exists (via `NewMatchTarget` with 5th `gpu` parameter)
  - `DetectGPU()` function is exported

All constructor callsites compile, meaning the cascade update across ~76 callsites succeeded.
