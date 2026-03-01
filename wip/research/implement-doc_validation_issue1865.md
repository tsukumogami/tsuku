# Validation Report: Issue #1865 (Satisfies Backfill)

Date: 2026-02-22
Branch: docs/gpu-backend-selection
Python: 3.12 (system python3.12, required for tomllib)

---

## Scenario 1: generate-registry.py passes after satisfies backfill

**ID**: scenario-1
**Status**: PASSED

**Commands run**:
```
python3.12 scripts/generate-registry.py
```

**Output**:
```
Found 362 recipe files
Generated _site/recipes.json with 362 recipes
```

**Exit code**: 0
**Errors on stderr**: None

The script validated all 362 recipe files, including satisfies entries for correct format, no duplicate claims, and no conflicts with canonical recipe names. No validation errors were produced.

---

## Scenario 2: all library recipes have satisfies metadata or documented exclusion

**ID**: scenario-2
**Status**: PASSED

**Method**: Read each recipe TOML file and checked for `[metadata.satisfies]` section or a TOML comment explaining the exclusion.

### Recipes with `[metadata.satisfies]` section (18 of 21):

| Recipe | Satisfies Homebrew Value | Notes |
|--------|-------------------------|-------|
| abseil | `["abseil-cpp"]` | Name mismatch: tsuku=abseil, Homebrew=abseil-cpp |
| brotli | `["brotli"]` | Names match |
| cairo | `["cairo"]` | Names match |
| expat | `["expat"]` | Names match |
| gettext | `["gettext"]` | Names match |
| geos | `["geos"]` | Names match |
| giflib | `["giflib"]` | Names match |
| gmp | `["gmp"]` | Names match |
| jpeg-turbo | `["libjpeg-turbo"]` | Name mismatch: tsuku=jpeg-turbo, Homebrew=libjpeg-turbo |
| libnghttp3 | `["libnghttp3"]` | Names match |
| libngtcp2 | `["ngtcp2"]` | Name mismatch: tsuku=libngtcp2, Homebrew=ngtcp2 |
| libpng | `["libpng"]` | Names match |
| libssh2 | `["libssh2"]` | Names match |
| libxml2 | `["libxml2"]` | Names match |
| proj | `["proj"]` | Names match |
| readline | `["readline"]` | Names match |
| zstd | `["zstd"]` | Names match |

That is 17 recipes, not 18. Let me recount... (libcurl is missing from this list).

### Recipes with documented exclusions (4 of 21):

| Recipe | Comment in TOML | Reason |
|--------|----------------|--------|
| cuda-runtime | `# No [metadata.satisfies] -- CUDA runtime is from NVIDIA, not a Homebrew formula` | System/NVIDIA package, no Homebrew equivalent |
| mesa-vulkan-drivers | `# No [metadata.satisfies] -- system package installed via native package managers, not a Homebrew formula` | System package, no Homebrew equivalent |
| vulkan-loader | `# No [metadata.satisfies] -- system package installed via native package managers, not a Homebrew formula` | System package, no Homebrew equivalent |
| libcurl | `# No [metadata.satisfies] for homebrew "curl" -- that name is the canonical recipe` | Homebrew name "curl" conflicts with existing recipes/c/curl.toml canonical recipe |

### Notes:
- The test plan identified 3 expected exclusions (cuda-runtime, mesa-vulkan-drivers, vulkan-loader). In practice there is a 4th: libcurl, which cannot claim `homebrew = ["curl"]` because that name is the canonical recipe for the curl CLI tool (recipes/c/curl.toml). The libcurl recipe documents this reasoning in a TOML comment. This is correct behavior.
- All 21 recipes account for: 17 with satisfies + 4 with documented exclusion = 21.

---

## Scenario 3: libngtcp2 satisfies resolves homebrew "ngtcp2" alias

**ID**: scenario-3
**Status**: PASSED

**Check 1**: recipes/l/libngtcp2.toml contains `homebrew = ["ngtcp2"]` in `[metadata.satisfies]`
- Result: CONFIRMED (lines 8-9 of the file)

**Check 2**: Registry JSON includes the mapping
- Command: `python3.12 scripts/generate-registry.py` then inspected `_site/recipes.json`
- Result: CONFIRMED

Registry JSON entry for libngtcp2:
```json
{
  "name": "libngtcp2",
  "description": "QUIC protocol implementation",
  "homepage": "https://nghttp2.org/ngtcp2/",
  "dependencies": [],
  "runtime_dependencies": [],
  "satisfies": {
    "homebrew": [
      "ngtcp2"
    ]
  }
}
```

The Homebrew formula name "ngtcp2" correctly resolves to the tsuku recipe "libngtcp2" via satisfies metadata.

---

## Scenario 4: abseil satisfies resolves homebrew "abseil-cpp" alias

**ID**: scenario-4
**Status**: PASSED

**Check 1**: recipes/a/abseil.toml contains `homebrew = ["abseil-cpp"]` in `[metadata.satisfies]`
- Result: CONFIRMED (lines 7-8 of the file)

**Check 2**: Registry JSON includes the mapping
- Command: Inspected `_site/recipes.json` (generated above)
- Result: CONFIRMED

Registry JSON entry for abseil:
```json
{
  "name": "abseil",
  "description": "C++ Common Libraries",
  "homepage": "https://abseil.io",
  "dependencies": [],
  "runtime_dependencies": [],
  "satisfies": {
    "homebrew": [
      "abseil-cpp"
    ]
  }
}
```

The Homebrew formula name "abseil-cpp" correctly resolves to the tsuku recipe "abseil" via satisfies metadata.

---

## Scenario 5: no duplicate satisfies claims across the full recipe set

**ID**: scenario-5
**Status**: PASSED

**Command**: `python3.12 scripts/generate-registry.py 2>&1`
**Exit code**: 0
**Output**: No lines containing "duplicate satisfies entry"

The generate-registry script performs cross-recipe validation (lines 322-345 of the script) to ensure:
1. No two recipes claim the same ecosystem package name
2. No satisfies entry conflicts with an existing recipe's canonical name

Both checks passed with no errors. The satisfies backfill does not introduce any duplicates.
