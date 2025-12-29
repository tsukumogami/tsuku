# Implementation Plan: Issue #729

## Overview
Replace outdated recipe copy pattern with `--recipe` flag across all test workflows and scripts.

## Complete Inventory

### Files to Modify
1. `.github/workflows/build-essentials.yml` - 5 occurrences
2. `scripts/test-zig-cc.sh` - 1 occurrence

### Verified Clean (no pattern found)
- All other workflows in `.github/workflows/`
- All test scripts in `test/scripts/`
- `Dockerfile` and `Dockerfile.integration`
- All Go test files (`*_test.go`)
- All documentation files (`*.md`)

## Detailed Changes

### 1. `.github/workflows/build-essentials.yml`

#### Occurrence 1: test-configure-make job (lines 102-112)

**Before:**
```yaml
      # Copy test recipe BEFORE build so it gets embedded
      - name: Copy test recipe to registry
        run: cp testdata/recipes/gdbm-source.toml internal/recipe/recipes/g/

      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install gdbm-source (tests configure_make)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./tsuku install --force gdbm-source
```

**After:**
```yaml
      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install gdbm-source (tests configure_make)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./tsuku install --recipe testdata/recipes/gdbm-source.toml --sandbox
```

**Lines to remove:** 102-104
**Lines to modify:** 109-112

---

#### Occurrence 2: test-meson-build job (lines 139-149)

**Before:**
```yaml
      # Copy test recipe BEFORE build so it gets embedded
      - name: Copy test recipe to registry
        run: cp testdata/recipes/libsixel-source.toml internal/recipe/recipes/l/

      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install libsixel-source (tests meson_build)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./tsuku install --force libsixel-source
```

**After:**
```yaml
      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install libsixel-source (tests meson_build)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./tsuku install --recipe testdata/recipes/libsixel-source.toml --sandbox
```

**Lines to remove:** 139-141
**Lines to modify:** 146-149

---

#### Occurrence 3: test-sqlite-source job (lines 209-219)

**Before:**
```yaml
      # Copy test recipe BEFORE build so it gets embedded
      - name: Copy test recipe to registry
        run: cp testdata/recipes/sqlite-source.toml internal/recipe/recipes/s/

      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install sqlite-source (tests dependency chain: sqlite → readline → ncurses)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./tsuku install --force sqlite-source
```

**After:**
```yaml
      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install sqlite-source (tests dependency chain: sqlite → readline → ncurses)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./tsuku install --recipe testdata/recipes/sqlite-source.toml --sandbox
```

**Lines to remove:** 209-211
**Lines to modify:** 216-219

---

#### Occurrence 4: test-git-source job (lines 246-256)

**Before:**
```yaml
      # Copy test recipe BEFORE build so it gets embedded
      - name: Copy test recipe to registry
        run: cp testdata/recipes/git-source.toml internal/recipe/recipes/g/

      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install git-source (tests multi-dependency chain: git → curl → openssl/zlib + expat)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./tsuku install --force git-source
```

**After:**
```yaml
      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install git-source (tests multi-dependency chain: git → curl → openssl/zlib + expat)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./tsuku install --recipe testdata/recipes/git-source.toml --sandbox
```

**Lines to remove:** 246-248
**Lines to modify:** 253-256

---

#### Occurrence 5: test-no-gcc job (lines 345-365)

**Before:**
```yaml
      # Copy test recipe BEFORE build so it gets embedded
      - name: Copy test recipe to registry
        run: cp testdata/recipes/gdbm-source.toml internal/recipe/recipes/g/

      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install zig (provides compiler)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./tsuku install --force zig

      - name: Build gdbm from source (uses zig cc)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          # Don't add tsuku tools to PATH - the Go code handles zig setup internally
          # and we want configure to detect system make, not the Homebrew make
          echo "Building gdbm-source - should use zig cc since no gcc exists"
          echo "System make: $(which make)"
          ./tsuku install --force gdbm-source
```

**After:**
```yaml
      - name: Build tsuku
        run: go build -o tsuku ./cmd/tsuku

      - name: Install zig (provides compiler)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: ./tsuku install --force zig

      - name: Build gdbm from source (uses zig cc)
        env:
          GITHUB_TOKEN: ${{ secrets.GITHUB_TOKEN }}
        run: |
          # Don't add tsuku tools to PATH - the Go code handles zig setup internally
          # and we want configure to detect system make, not the Homebrew make
          echo "Building gdbm-source - should use zig cc since no gcc exists"
          echo "System make: $(which make)"
          ./tsuku install --recipe testdata/recipes/gdbm-source.toml --sandbox
```

**Lines to remove:** 345-347
**Lines to modify:** 357-365 (specifically line 365)

---

### 2. `scripts/test-zig-cc.sh`

#### Occurrence 1: Dockerfile RUN command (lines 75-87)

**Before:**
```dockerfile
# Copy gdbm-source test recipe
RUN cp testdata/recipes/gdbm-source.toml internal/recipe/recipes/g/

# Rebuild tsuku with test recipe embedded
RUN go build -o tsuku ./cmd/tsuku

# Add tsuku tools to PATH for build dependencies
# ~/.tsuku/tools/current contains symlinks to installed binaries
ENV PATH="/root/.tsuku/tools/current:$PATH"

# The actual test: build gdbm from source
# This MUST use zig cc since no system compiler exists
RUN ./tsuku install --force gdbm-source
```

**After:**
```dockerfile
# Add tsuku tools to PATH for build dependencies
# ~/.tsuku/tools/current contains symlinks to installed binaries
ENV PATH="/root/.tsuku/tools/current:$PATH"

# The actual test: build gdbm from source
# This MUST use zig cc since no system compiler exists
RUN ./tsuku install --recipe testdata/recipes/gdbm-source.toml --sandbox
```

**Lines to remove:** 75-76, 78-79
**Lines to modify:** 87

**Note:** Lines 81-83 (PATH setup) and 85-86 (comments) remain but move up.

---

## Testing Strategy

### Pre-flight Checks
1. Verify `--recipe` flag is implemented in `tsuku install`
2. Verify `--sandbox` flag works with `--recipe`
3. Confirm test recipes exist:
   - `testdata/recipes/gdbm-source.toml`
   - `testdata/recipes/libsixel-source.toml`
   - `testdata/recipes/sqlite-source.toml`
   - `testdata/recipes/git-source.toml`

### Validation Steps

#### Local Testing
1. Build tsuku: `go build -o tsuku ./cmd/tsuku`
2. Test each recipe directly:
   ```bash
   ./tsuku install --recipe testdata/recipes/gdbm-source.toml --sandbox
   ./tsuku install --recipe testdata/recipes/libsixel-source.toml --sandbox
   ./tsuku install --recipe testdata/recipes/sqlite-source.toml --sandbox
   ./tsuku install --recipe testdata/recipes/git-source.toml --sandbox
   ```

#### Docker Testing
Run the test-zig-cc script locally:
```bash
./scripts/test-zig-cc.sh
```

This will build a Docker container and validate the zig cc fallback.

#### CI Testing
1. Push changes to branch
2. Verify all jobs in `build-essentials.yml` pass:
   - `test-configure-make` (3 platforms)
   - `test-meson-build` (3 platforms)
   - `test-sqlite-source` (3 platforms)
   - `test-git-source` (3 platforms)
   - `test-no-gcc` (1 platform)
3. Confirm tests complete in similar time (no performance regression)

### Expected Outcomes
- All tests pass with identical results
- No embedded recipes needed during build
- Cleaner workflow files (3 lines removed per occurrence)
- More maintainable test pattern

---

## Implementation Order

1. **Phase 1: Workflow file**
   - Edit `.github/workflows/build-essentials.yml`
   - Make all 5 changes in a single atomic commit
   - Test via CI push

2. **Phase 2: Script file**
   - Edit `scripts/test-zig-cc.sh`
   - Test locally with Docker
   - Commit separately for clarity

3. **Phase 3: Verification**
   - Run full CI suite
   - Verify all platforms pass
   - Confirm no regressions

---

## Risk Assessment

### Low Risk
- The `--recipe` flag is well-tested and production-ready
- Test recipes are stable and version-pinned
- Changes are purely in test infrastructure (no production code)

### Potential Issues
1. **Sandbox flag requirement**: If `--recipe` requires `--sandbox`, ensure all tests use it
2. **Path resolution**: Relative paths in CI should work (`testdata/recipes/...`)
3. **Docker context**: In `test-zig-cc.sh`, ensure testdata/ is copied to Docker context

### Rollback Plan
If tests fail:
1. Revert commits immediately
2. Investigate failure in isolation
3. Consider using eval+install pattern as alternative:
   ```bash
   ./tsuku eval --recipe testdata/recipes/gdbm-source.toml | ./tsuku install --plan - --sandbox
   ```

---

## Success Criteria

- [ ] All 6 copy steps removed
- [ ] All 6 install commands use `--recipe testdata/recipes/<name>.toml --sandbox`
- [ ] All 5 "Copy test recipe BEFORE build so it gets embedded" comments removed
- [ ] CI build-essentials workflow passes on all platforms
- [ ] `scripts/test-zig-cc.sh` Docker test passes
- [ ] No embedded recipe references in test code
- [ ] Total lines removed: ~18 (3 lines per occurrence)

---

## Notes

### Why `--sandbox`?
The `--recipe` flag requires `--sandbox` for security - it prevents untrusted recipes from affecting the host system. While test recipes are trusted, using `--sandbox` is best practice and ensures consistent behavior.

### Alternative Pattern
If `--sandbox` is not desired, use the eval+install pattern:
```bash
./tsuku eval --recipe <path> | ./tsuku install --plan - --sandbox
```

This was mentioned in the issue but is more verbose. The direct `--recipe` approach is preferred.

### Documentation Impact
No documentation changes needed - this is purely internal test infrastructure. The user-facing docs already promote the `--recipe` pattern (see `README.md`, `cmd/tsuku/install.go`, etc.).
