# Test Plan: system-lib-backfill

Generated from: docs/designs/DESIGN-system-lib-backfill.md
Issues covered: 3 (1865, 1866, 1867; 1864 already completed)
Total scenarios: 10

---

## Scenario 1: generate-registry.py passes after satisfies backfill
**ID**: scenario-1
**Category**: infrastructure
**Testable after**: #1865
**Commands**:
- `python3 scripts/generate-registry.py`
**Expected**: Exit code 0 with no validation errors. The script validates satisfies entries for correct format (lowercase alphanumeric), no duplicate claims across recipes, and no conflicts with canonical recipe names. A non-zero exit code or any "satisfies" error in output means the backfill introduced invalid metadata.
**Status**: pending

---

## Scenario 2: all library recipes have satisfies metadata or documented exclusion
**ID**: scenario-2
**Category**: infrastructure
**Testable after**: #1865
**Commands**:
- For each recipe listed in issue #1865 (abseil, brotli, cairo, cuda-runtime, expat, gettext, geos, giflib, gmp, jpeg-turbo, libcurl, libnghttp3, libngtcp2, libpng, libssh2, libxml2, mesa-vulkan-drivers, proj, readline, vulkan-loader, zstd): `grep -l 'satisfies' recipes/<letter>/<name>.toml`
- For recipes where no homebrew formula exists (cuda-runtime, mesa-vulkan-drivers, vulkan-loader): verify a TOML comment explains why no satisfies entry applies
**Expected**: Every recipe in the list either contains a `[metadata.satisfies]` section or has a comment explaining why none applies. The three system/NVIDIA packages (cuda-runtime, mesa-vulkan-drivers, vulkan-loader) are the expected exclusions.
**Status**: pending

---

## Scenario 3: libngtcp2 satisfies resolves homebrew "ngtcp2" alias
**ID**: scenario-3
**Category**: use-case
**Testable after**: #1865
**Commands**:
- Verify `recipes/l/libngtcp2.toml` contains `homebrew = ["ngtcp2"]` in its `[metadata.satisfies]` section
- `python3 scripts/generate-registry.py` and check the generated registry JSON includes `"satisfies": {"homebrew": ["ngtcp2"]}` for the libngtcp2 entry
**Expected**: The Homebrew formula name "ngtcp2" resolves to the tsuku recipe "libngtcp2" via satisfies metadata. This is the key mismatch case: Homebrew uses "ngtcp2" but the tsuku canonical name is "libngtcp2". The registry JSON must include the mapping so the pipeline resolves the dependency correctly.
**Status**: pending

---

## Scenario 4: abseil satisfies resolves homebrew "abseil-cpp" alias
**ID**: scenario-4
**Category**: use-case
**Testable after**: #1865
**Commands**:
- Verify `recipes/a/abseil.toml` contains `homebrew = ["abseil-cpp"]` in its `[metadata.satisfies]` section
- `python3 scripts/generate-registry.py` and check the registry includes the mapping
**Expected**: The Homebrew formula name "abseil-cpp" resolves to the tsuku recipe "abseil". This is another key mismatch. Without this satisfies entry, any tool depending on Homebrew's abseil-cpp formula would produce a missing_dep failure even though the library exists under the name "abseil".
**Status**: pending

---

## Scenario 5: no duplicate satisfies claims across the full recipe set
**ID**: scenario-5
**Category**: infrastructure
**Testable after**: #1865
**Commands**:
- `python3 scripts/generate-registry.py 2>&1`
**Expected**: Exit code 0. No output lines containing "duplicate satisfies entry". The generate-registry script performs cross-recipe validation to ensure no two recipes claim the same ecosystem package name. The satisfies backfill must not introduce any duplicates with existing entries (e.g., libnghttp2 already claims "nghttp2").
**Status**: pending

---

## Scenario 6: ranked library list exists with required format
**ID**: scenario-6
**Category**: infrastructure
**Testable after**: #1866
**Commands**:
- `test -f docs/library-backfill-ranked.md && echo "exists"`
- Verify the file contains a markdown table with columns: Library, Block Count, Status, Category
- Verify the table is sorted descending by block count
**Expected**: The file `docs/library-backfill-ranked.md` exists, contains a markdown table with the four required columns, and rows are sorted by block count (highest first). The table must have at least the 14 known blockers from the design doc plus any additional blockers found during the discovery run.
**Status**: pending

---

## Scenario 7: discovery list includes known historical blockers
**ID**: scenario-7
**Category**: use-case
**Testable after**: #1866
**Commands**:
- Check `docs/library-backfill-ranked.md` for entries matching the known blockers: gmp, libgit2, bdw-gc, pcre2, oniguruma, dav1d, tree-sitter, libevent, libidn2, glib, gettext, ada-url, notmuch
- For each, verify the Status column correctly reflects the current state (has recipe / needs satisfies / no recipe)
**Expected**: All 13 known blockers from the design doc table (excluding openssl@3 which already has satisfies) appear in the ranked list. Status column values match reality: gmp and gettext should show "has recipe, needs satisfies" (or "recipe exists" after #1865 adds satisfies). The remaining should show "no recipe" unless the discovery run found they already have recipes.
**Status**: pending

---

## Scenario 8: new library recipes have type=library and satisfies metadata
**ID**: scenario-8
**Category**: infrastructure
**Testable after**: #1867
**Commands**:
- For each new recipe created (at minimum: libgit2, bdw-gc, pcre2, oniguruma, dav1d, tree-sitter, libevent, libidn2, glib, ada-url, notmuch): verify the TOML file exists, contains `type = "library"`, and contains `[metadata.satisfies]`
- `python3 scripts/generate-registry.py`
**Expected**: Every new library recipe file exists under `recipes/<letter>/<name>.toml`, declares `type = "library"` in metadata, and includes a `[metadata.satisfies]` section with the appropriate Homebrew formula alias. The generate-registry script exits 0 with no validation errors.
**Status**: pending

---

## Scenario 9: tree-sitter recipe includes versioned alias
**ID**: scenario-9
**Category**: use-case
**Testable after**: #1867
**Commands**:
- Verify `recipes/t/tree-sitter.toml` contains `tree-sitter@0.25` in its satisfies homebrew array
- `python3 scripts/generate-registry.py` exits 0
**Expected**: The tree-sitter recipe resolves the versioned alias "tree-sitter@0.25" from Homebrew. Without this, tools that depend on Homebrew's `tree-sitter@0.25` formula would fail with missing_dep even though a tree-sitter recipe exists. The `@` character is valid per the NAME_PATTERN (`^[a-z0-9@.-]+$`) used in generate-registry.py.
**Status**: pending

---

## Scenario 10: test-recipe workflow validates new library recipes across platforms
**ID**: scenario-10
**Category**: use-case
**Environment**: manual (requires GitHub Actions runners)
**Testable after**: #1867
**Commands**:
- For each library recipe PR, trigger the `test-recipe.yml` workflow on the PR branch
- Check workflow run results across all platforms: Linux x86_64 (debian, rhel, arch, alpine, suse), Linux arm64 (debian, rhel, suse, alpine), macOS arm64, macOS x86_64
**Expected**: The test-recipe workflow runs to completion for each library recipe. Platforms where the recipe fails have corresponding `when` filters added to the recipe TOML before merge. The workflow summary shows pass/fail per platform. This scenario cannot be validated locally -- it requires GitHub Actions runners and macOS/arm64 infrastructure. The tester should verify the workflow ran and review platform results in the GHA job summaries after each library recipe PR is opened.
**Status**: pending
