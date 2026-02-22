# Security Review: DESIGN-gem-exec-wrappers

## Context

This fix converts bare relative symlinks to bash wrapper scripts inside `executeLockDataMode()` in `internal/actions/gem_exec.go`. The wrapper scripts set `GEM_HOME`, `GEM_PATH`, and `PATH` before calling the bundler-installed gem executable.

This review covers the code paths in `gem_exec.go` (full file) and `gem_install.go` (lines 193-247).

---

## Attack vectors not considered in the design

### 1. Lockfile injection via `lock_data` content (Medium risk, not fully mitigated)

The design says "No change to supply chain risk. The gem packages come from the same source." This is true for what bundler downloads, but `lock_data` is a parameter passed to `executeLockDataMode()` and written directly to disk at line 397:

```go
if err := os.WriteFile(lockPath, []byte(lockData), 0644); err != nil {
```

The `Gemfile.lock` format is a structured text format that bundler parses. A maliciously crafted `lock_data` value could cause bundler to install different gems than intended -- for example, by pointing the `GIT` or `PATH` source sections to attacker-controlled locations. The design does not validate the `lock_data` content structure beyond accepting it as a string.

This is partially mitigated by `BUNDLE_FROZEN=true`, which prevents bundler from modifying the lock. But `BUNDLE_FROZEN` only prevents changes to the lockfile; it does not prevent bundler from following a `GIT` source entry in the existing lockfile. A recipe with a malicious `lock_data` could specify a `GIT` remote pointing to an attacker's repository.

The threat model here is: who controls `lock_data`? If only the recipe author (who is trusted), this is acceptable. If `lock_data` comes from an untrusted external source during decomposition, it is not. The design should state the trust boundary explicitly.

### 2. Wrapper script hardcodes ruby bin dir from install time (Low risk)

The wrapper script embeds the absolute path to the ruby `bin/` directory at install time (derived from `findBundler()`). If tsuku's ruby is updated, the embedded path in existing wrapper scripts will point to the old ruby version's directory, which may no longer exist. The gem then fails to run with a confusing "no such file" error rather than a clear version mismatch message.

This is not a security vulnerability but it is a latent correctness issue that the security section did not flag. The wrapper format (absolute path embedded at install time) is the same approach `gem_install.go` uses, so this is consistent -- but both have this property.

### 3. `BASH_SOURCE[0]` symlink resolution on macOS with readlink (Low risk)

The symlink resolution loop in the wrapper uses `readlink` without `-f`:

```bash
SCRIPT_PATH="$(readlink "$SCRIPT_PATH")"
```

On macOS, `readlink` without `-f` only resolves one level of symlink per iteration. The while loop handles this correctly (it loops until no symlink). However, `readlink` on macOS and GNU/Linux have subtly different behaviors. This is the same code already used in `gem_install.go` and is reported as working, so the risk is contained to the `gem_exec` path adopting the same limitation.

### 4. `environment_vars` parameter passes through to `buildEnvironment` unchecked (Medium risk, pre-existing)

In `executeLockDataMode()` at line 416:

```go
env := a.buildEnvironment(installDir, installDir, true, environmentVars)
```

`buildEnvironment()` appends custom environment variables directly without validating key names or values:

```go
for k, v := range customEnv {
    env = append(env, fmt.Sprintf("%s=%s", k, v))
}
```

An environment variable key containing `=` or null bytes could produce a malformed env entry, and some shells/programs parse env entries differently depending on how many `=` characters appear. The `environment_vars` parameter is recipe-controlled (trusted source), but the lack of validation means a recipe typo (e.g., `"GEM_HOME=bad"` as a key) could silently corrupt the environment. This is pre-existing and not introduced by this change.

---

## Are the mitigations sufficient for the identified risks?

### Input validation (gem name, version, executable names)

These are well-handled. `isValidGemName()` and `isValidGemVersion()` in `gem_install.go` are strict allowlists. The executable name validation in `executeLockDataMode()` (lines 351-361) checks path separators, `..`, `.`, and shell metacharacters. The `command` parameter in source_dir mode checks `ContainsAny(";|&$\`\\")`.

One gap: the executable name check does not check for null bytes (`\x00`). The `gem_install.go` version does (lines 104-107). This is an inconsistency between the two code paths that the design doesn't mention. Null bytes in filenames cause undefined behavior across OS implementations.

### `BUNDLE_FROZEN=true` enforcement

Correct and sufficient for its stated purpose (preventing bundler from modifying the lockfile during install).

### `exec ruby` in wrapper vs. symlink

The wrapper's use of `exec` (replace-not-fork) is correct. There is no persistent shell process after the gem starts.

### `GEM_HOME`/`GEM_PATH` scoping

Setting both to `$INSTALL_DIR` is correct. The design's claim that "the environment is explicitly scoped to the install directory" is accurate -- this is an improvement over bare symlinks.

---

## Residual risk to escalate

### Moderate: `lock_data` trust boundary is unstated

The security section says lock_data introduces "no change to supply chain risk," but this is only true if the trust model for `lock_data` is clearly defined. The design assumes `lock_data` is generated by tsuku's own decomposition process (which runs `bundle lock` on a Gemfile controlled by the recipe). This assumption should be stated explicitly in the security section, and the code should document that `lock_data` is trusted content from the recipe file.

If the decomposition pipeline is ever extended to accept external lockfiles (e.g., from a user's project), this trust boundary becomes load-bearing.

This is worth a one-line code comment in `executeLockDataMode()` where `lock_data` is written to disk.

### Low: Three gem action paths with different binary exposure mechanisms

After this fix, there will still be three distinct gem action implementations:
- `gem_install.go`: wrapper scripts (working)
- `gem_exec.go`: wrapper scripts after fix (proposed)
- `install_gem_direct.go`: bare symlinks from `.gem/bin/` to `tsuku/bin/` (lines 97-113)

`install_gem_direct.go` does not set `GEM_HOME` or `GEM_PATH` in its wrappers, and it creates bare symlinks -- the same problem being fixed in `gem_exec.go`. The design scope explicitly excludes this path, but the security reviewer should flag it as a known gap that will surface when a recipe using `install_gem_direct` is run.

---

## "Not applicable" justifications: are any actually applicable?

### "Download Verification: Not applicable to this change"

Correct. The change only affects what happens after installation.

### "Supply Chain Risks: No change to supply chain risk"

Partially correct, with the caveat noted above. Bundler with `BUNDLE_FROZEN=true` and a lockfile that contains checksums (Gemfile.lock with `CHECKSUMS` section, generated by `bundle lock --add-checksums`) provides strong supply chain guarantees. The gap is the `GIT` source vector in lockfiles, and the trust boundary around `lock_data`.

### "User Data Exposure: No change"

Correct. The wrappers only set local filesystem paths.

---

## Recommended additions to the design's security section

1. State explicitly: "`lock_data` is generated by tsuku's decomposition pipeline from recipe-controlled parameters and is treated as trusted content. External lockfiles must not be passed through this parameter without validation."

2. Note that `install_gem_direct.go` has the same bare-symlink issue and is out of scope for this fix but should be tracked.

3. Add null-byte check to `executeLockDataMode()` executable name validation to match `gem_install.go`'s validation (lines 104-107).
