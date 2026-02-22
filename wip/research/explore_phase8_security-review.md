# Security Review: Library Recipe Generation Design

**Reviewer:** architect-reviewer
**Design:** `docs/designs/DESIGN-library-recipe-generation.md`
**Date:** 2026-02-22

## Scope

This review evaluates the security properties of extending the deterministic recipe generator to handle library-only Homebrew bottles. The design adds scanning of `lib/` and `include/` directories in bottle tarballs and generates `type = "library"` recipes with `install_mode = "directory"`.

## Trust Model Summary

The system has a layered trust model:

1. **GHCR as bottle source**: Bottles are downloaded from `ghcr.io/homebrew/core/`. Authentication uses an anonymous GHCR token. The GHCR manifest provides a blob SHA256 digest.
2. **SHA256 verification at download**: `downloadBottleBlob` (`homebrew.go:1521`) computes SHA256 during download via `io.MultiWriter` and compares against the blob SHA from the manifest. Mismatch causes failure.
3. **Tarball inspection (no execution)**: The generator reads tar headers for file paths. It does not extract file contents to disk during the scanning phase, and never executes anything from the bottle.
4. **Generated recipe as output**: The output is a TOML recipe file. It describes installation steps but does not itself perform installation.
5. **Installation via existing actions**: When the recipe is later used, `install_binaries` with `install_mode = "directory"` copies the directory tree and creates symlinks. Path traversal validation exists in `validateBinaryPath` (`install_binaries.go:262`).

## Findings

### Finding 1: No download size limit on bottle blobs

**Severity: Advisory**

`downloadBottleBlob` (`homebrew.go:1551`) uses unbounded `io.Copy(writer, resp.Body)` to write the bottle to a temp file. Other network responses in the same file have size limits (64KB for tokens, 10MB for manifests, 1MB for formula JSON), but bottle downloads have none.

The design doubles the number of bottle downloads per library package (Linux + macOS). A malicious or misconfigured GHCR response could stream an arbitrarily large response, exhausting disk space on the machine running the generator.

**Risk assessment:** Low in practice. The generator authenticates against `ghcr.io/homebrew/core/` specifically (hardcoded in `downloadBottleBlob`), and the SHA256 verification would reject any response that doesn't match the manifest digest. An attacker would need to compromise GHCR itself or perform a MITM attack. The batch pipeline runs on ephemeral CI runners with limited disk, so disk exhaustion would cause a job failure rather than persistent damage.

**Recommendation:** Consider adding a reasonable size cap (e.g., 500MB) as defense-in-depth, consistent with the limiting pattern used elsewhere in the file. This is pre-existing, not introduced by this design, but the design doubles exposure. Not blocking.

### Finding 2: Tarball path traversal protection during scanning is adequate

**Severity: Not an issue**

The design proposes scanning tarball entries by reading `header.Name` and matching path components (e.g., `parts[2] == "lib"`). This is consistent with the existing `extractBottleBinaries` pattern (`homebrew.go:1593`). The scan only reads tar headers to collect file names -- it does not extract files to disk.

At installation time (not at generation time), the `install_binaries` action validates paths via `validateBinaryPath` (`install_binaries.go:262`), which rejects `..` and absolute paths. The `extract` action has its own `isPathWithinDirectory` check (`extract.go:322`).

A crafted bottle with `../../etc/passwd` as a tar entry name would produce that string in the generated recipe's `outputs` list. When the recipe is later installed, `validateBinaryPath` would reject it. The attack surface is the generation output (a TOML file), which is reviewed before being committed to the registry.

**Conclusion:** The existing path validation at the install layer is the correct defense point. The generator doesn't need its own traversal protection beyond what the tarball scanner implicitly provides (reading paths without extracting). The design correctly identifies this.

### Finding 3: Tarball scan does not filter symlinks in lib/ -- but the design says it does

**Severity: Advisory**

The design states: "The scan filters for regular files only (skipping symlinks and directory entries in the tar)." The existing `extractBottleBinaries` code (`homebrew.go:1594`) checks `header.Typeflag == tar.TypeReg`, which correctly skips symlinks.

However, library bottles contain many symlinks. For example, a typical library ships:
- `lib/libfoo.so` (symlink to `libfoo.so.1`)
- `lib/libfoo.so.1` (symlink to `libfoo.so.1.2.3`)
- `lib/libfoo.so.1.2.3` (regular file)

If the implementation strictly filters to `tar.TypeReg`, the generated `outputs` list will only contain the versioned file (`libfoo.so.1.2.3`) and miss the unversioned and major-versioned symlinks. Downstream tools that link against `-lfoo` need the `libfoo.so` symlink to be present.

The existing hand-written library recipes (e.g., `abseil.toml`) list the unversioned library names (`lib/libabsl_base.so`), which are typically the symlinks, not the regular files. This means the design's "skip symlinks" rule would produce recipes that differ from the established pattern.

**Security implication:** None. This is a correctness concern, not a security one. Including symlink entries from the tarball in the outputs list doesn't introduce any new attack surface -- the `install_binaries` action copies the entire directory tree in `directory` mode regardless of what's in the outputs list. The outputs list controls what gets symlinked into `$TSUKU_HOME/libs/`, not what gets extracted.

**Recommendation:** The implementation should include `tar.TypeSymlink` entries from `lib/` in the outputs list, or the generated recipes won't match existing conventions. This needs clarification in the design. Not a security concern, but flagging because the design text is misleading about the behavior.

### Finding 4: Cross-platform bottle download reuses the same trust model

**Severity: Not an issue**

Downloading a macOS bottle on a Linux runner is the core new behavior. The design correctly identifies that this uses the same GHCR token, manifest lookup, and SHA256 verification. The tarball format is identical (gzip + tar) on both platforms -- only the file contents differ.

No new trust boundary is crossed. The GHCR manifest is fetched once and contains entries for all platforms. Each platform's blob has its own SHA256 digest verified independently. The `getBlobSHAFromManifest` function (`homebrew.go:1496`) resolves the correct digest per platform tag.

### Finding 5: GHCR token scoping is appropriately narrow

**Severity: Not an issue**

The token request (`homebrew.go:1304`) scopes to `repository:homebrew/core/{formula}:pull`, which is read-only and formula-specific. The design doesn't change this. Downloading additional platform bottles reuses the same token for the same formula.

### Finding 6: No execution of bottle contents -- claim verified

**Severity: Not an issue**

The design claims the generator "reads tarball contents without executing anything from the bottle." Verified: `extractBottleBinaries` (`homebrew.go:1566-1605`) iterates tar headers with `tr.Next()` but never calls `io.Copy` or writes file contents to disk. The proposed `extractBottleContents` function follows the same pattern. The tarball is written to a temp file by `downloadBottleBlob` for the gzip reader, but no files within the tarball are extracted during the scanning phase.

### Finding 7: `isLibraryFile` regex allows overly broad `.so.` matching

**Severity: Advisory**

The proposed `isLibraryFile` function uses `strings.Contains(name, ".so.")` to match versioned shared objects. This would match a file named `reason.something` or `also.notso.txt` because it's a substring match on `.so.`.

In the context of scanning Homebrew bottles where file paths are `lib/{filename}`, this is unlikely to produce false positives in practice -- Homebrew bottles follow strict naming conventions. But the match is broader than necessary.

**Recommendation:** Use a more precise pattern like checking that the name segment before `.so.` ends at a word boundary, or that the path starts with `lib/lib`. Not blocking -- false positives in the outputs list are a recipe quality issue, not a security issue.

### Finding 8: Generated recipe file paths are never used in shell commands

**Severity: Not an issue (design correctly marks as N/A)**

The generated output is a TOML file with an `outputs` array of strings. These strings are later parsed by the `install_binaries` action, which uses `filepath.Join` and filesystem operations -- never `os/exec`. There is no shell injection vector from the outputs list.

### Finding 9: Temp file cleanup on multi-platform download failure

**Severity: Advisory**

The existing `listBottleBinaries` (`homebrew.go:1478-1485`) creates a temp file and defers `os.Remove`. If the process is killed during the second platform download (the new behavior), the first platform's temp file will have been cleaned up by its defer, but the second platform's temp file may be left behind.

This is pre-existing behavior (single-platform downloads have the same risk). The design doesn't make it worse in a meaningful way -- temp files on CI runners are cleaned on job completion.

**Recommendation:** Not actionable. Standard behavior for temp files in Go.

## "Not Applicable" Justifications Review

The design's security section makes several implicit "not applicable" claims. Evaluating each:

### "User data exposure: No user data is accessed or transmitted"

**Verdict: Correctly N/A.** The generator reads from a public registry (GHCR for Homebrew) and writes recipe files locally. No user credentials, paths, or installation state are involved in the generation process.

### Shell injection via generated recipes

**Verdict: Correctly N/A.** The generated recipes contain only `homebrew` and `install_binaries` actions. Neither action passes recipe parameters through a shell. The `homebrew` action uses `formula` as a GHCR URL component (with `url.PathEscape`). The `install_binaries` action uses `outputs` as filesystem paths (with `validateBinaryPath`).

### Symlink attacks at install time

**Verdict: Correctly handled by existing code.** The `install_binaries` action in `directory` mode copies the full directory tree to `$TSUKU_HOME/libs/{name}-{version}/`, validates output paths against traversal, and uses atomic symlink creation (`createSymlink` at `install_binaries.go:279`). A malicious recipe with `../../../etc/foo` in outputs would be caught by `validateBinaryPath`.

## Attack Vectors Not Addressed in the Design

### 1. GHCR manifest manipulation (TOCTOU between manifest fetch and blob download)

If an attacker could modify the GHCR manifest between the manifest fetch and the blob download, they could serve a different blob SHA that matches a malicious bottle. However, this requires MITM against HTTPS to ghcr.io, which is outside the threat model for an anonymous public registry client. The SHA256 verification ensures that whatever blob is downloaded matches what the manifest claimed.

This is the same trust model as downloading any package from a public registry. Not a gap.

### 2. Homebrew bottle with malicious library names that exploit downstream consumers

A bottle could ship a library like `lib/libcrypto.so` that shadows a system library. When installed to `$TSUKU_HOME/libs/`, a tool that sets `LD_LIBRARY_PATH` or `RPATH` to include `$TSUKU_HOME/libs/{name}-{version}/lib/` could load this instead of the system's `libcrypto.so`.

This is an inherent risk of any library installation mechanism, not specific to this design. The trust boundary is the Homebrew bottle itself -- if you trust `ghcr.io/homebrew/core/bdw-gc` to ship legitimate libraries, you trust its file names. The design doesn't introduce a new attack surface here; it automates what recipe authors already do manually.

### 3. Denial of service via very large output lists

A bottle with thousands of files in `lib/` and `include/` would produce a very large `outputs` array. This could result in a recipe file that's unwieldy but not a security issue. The TOML file is committed to the registry via PR, where a human reviews it.

**Recommendation:** Consider adding a sanity limit (e.g., warn if outputs exceeds 200 entries) as a signal that the bottle may not be a "pure library" and might need manual review.

## Residual Risk Assessment

| Risk | Likelihood | Impact | Residual After Mitigations |
|------|-----------|--------|---------------------------|
| Compromised GHCR serving malicious bottles | Very Low | High | Same as all Homebrew users; SHA256 ties blob to manifest |
| Library name shadowing in $TSUKU_HOME/libs/ | Low | Medium | Not new; inherent to any library installation |
| Stale outputs list missing new files | Medium | Low | Functional, not security; files not symlinked but also not executed |
| Unbounded bottle download size | Very Low | Low | Pre-existing; SHA256 would reject mismatched content |

**None of these residual risks warrant escalation.** They are either pre-existing (and this design doesn't materially change the exposure) or functional issues rather than security issues.

## Summary

The design's security model is sound. It extends an existing, well-secured download-and-inspect pipeline without introducing new trust boundaries, execution paths, or privilege escalation vectors. The SHA256 verification on bottle downloads, path validation in `install_binaries`, and the generator's read-only tarball scanning all carry forward correctly.

The main actionable items are:
1. **Clarify symlink handling in the design**: The design says "skip symlinks" but existing library recipes list symlink names. This is a correctness issue, not security.
2. **Consider a bottle download size limit**: Pre-existing gap, but the design doubles exposure. Defense-in-depth, not blocking.
3. **Consider an outputs count sanity check**: Practical quality gate, not security-critical.
