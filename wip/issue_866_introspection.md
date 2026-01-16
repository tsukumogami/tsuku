# Issue 866 Introspection

## Context Reviewed
- Design doc: `docs/designs/DESIGN-cask-support.md`
- Sibling issues reviewed: #862 (walking skeleton), #863 (cask version provider), #864 (DMG extraction), #865 (binary symlinks)
- Prior patterns identified: SessionBuilder interface, DeterministicSession wrapper, builder registration in cmd/tsuku/create.go

## Prior Work Summary

All dependency issues (#863, #864, #865) have been closed with merged PRs:

| Issue | PR | Key Implementation Details |
|-------|-----|----------------------------|
| #862 | #871 | Walking skeleton - established CaskProvider stub, AppBundleAction, dotted-path template expansion |
| #863 | #882 | Full cask version provider - queries `formulae.brew.sh/api/cask/{name}.json`, handles architecture selection, returns VersionInfo with Metadata containing url/checksum |
| #864 | #894 | DMG extraction via hdiutil in app_bundle action |
| #865 | #915 | Binary symlinks to `$TSUKU_HOME/tools/current/`, ~/Applications symlink, `tsuku list --apps` |

## Gap Analysis

### Minor Gaps

1. **File location established**: The issue specifies `internal/builders/cask.go` which aligns with existing builder convention (e.g., `homebrew.go`, `cargo.go`).

2. **Registry pattern established**: Builders are registered in `cmd/tsuku/create.go` line 197-203. CaskBuilder must be added here with `builderRegistry.Register(builders.NewCaskBuilder())`.

3. **SessionBuilder interface requirement**: The issue correctly specifies implementing `SessionBuilder` interface. Since CaskBuilder generates recipes deterministically (no LLM), it should use `DeterministicSession` wrapper like other ecosystem builders.

4. **API endpoint established**: PR #882 established the cask API URL pattern as `https://formulae.brew.sh/api/cask/{name}.json`. The CaskBuilder should reuse this.

5. **Artifact parsing structure**: The issue proposes `caskArtifact` struct for parsing. However, the actual Cask API response has a more complex structure. PR #882's `homebrewCaskInfo` struct does not include `artifacts` parsing. The CaskBuilder will need to extend this.

### Moderate Gaps

1. **Artifact parsing not in existing code**: The cask version provider (#863) only extracts version, URL, and checksum. It does NOT parse the `artifacts` array which contains `app` and `binary` information. The CaskBuilder must add this parsing.

   **Proposed amendment**: Add artifact parsing to CaskBuilder (not to the version provider). The version provider's responsibility is version resolution; artifact parsing is builder-specific for recipe generation.

2. **Recipe format established but needs verification**: PR #915 established the app_bundle action parameters. The generated recipe should use:
   - `app_name`: Required, e.g., "Visual Studio Code.app"
   - `binaries`: Optional array of paths relative to .app bundle (e.g., "Contents/Resources/app/bin/code")
   - `symlink_applications`: Optional boolean, defaults to true

3. **CurrentDir vs BinDir**: PR #915 shows binary symlinks go to `ctx.CurrentDir` (which is `$TSUKU_HOME/tools/current/`), not `$TSUKU_HOME/bin`. The issue's example recipe shows `binaries` correctly, but implementation must ensure the generated recipe uses paths that work with the actual app_bundle action.

4. **Checksum handling**: Some casks use `:no_check` (no checksum). The version provider returns empty string for these. The CaskBuilder should emit a warning for no-checksum casks but still generate valid recipes. The app_bundle action will need to handle empty checksum (or `--allow-no-checksum` flag may be needed).

### Major Gaps

None identified. The issue spec is well-aligned with prior work.

## Recommendation

**Proceed** - with minor documentation of the established patterns from sibling issues.

## Proposed Amendments

The following clarifications should be incorporated into the implementation plan based on prior work:

1. **Builder registration**: Add `builders.NewCaskBuilder()` to `cmd/tsuku/create.go` in the builder registry initialization block (around line 203).

2. **Use DeterministicSession**: Since CaskBuilder does not use LLM, wrap the build function with `DeterministicSession`:
   ```go
   func (b *CaskBuilder) NewSession(ctx context.Context, req BuildRequest, opts *SessionOptions) (BuildSession, error) {
       return NewDeterministicSession(b.build, req), nil
   }
   ```

3. **RequiresLLM() returns false**: This enables sandbox testing to be skipped for ecosystem builders (as per cmd/tsuku/create.go line 300).

4. **Artifact parsing**: The cask API's `artifacts` field is a JSON array with heterogeneous objects. Example:
   ```json
   "artifacts": [
     {"app": ["Visual Studio Code.app"]},
     {"binary": ["{{appdir}}/Visual Studio Code.app/Contents/Resources/app/bin/code"]}
   ]
   ```
   The `{{appdir}}` placeholder should be stripped, and paths adjusted to be relative to the .app bundle root.

5. **Normalizing binary paths**: Binary artifact values like `{{appdir}}/Visual Studio Code.app/Contents/Resources/app/bin/code` need to be converted to paths relative to the .app bundle, e.g., `Contents/Resources/app/bin/code`.

6. **Add "cask" to help text**: Update `createCmd.Long` in cmd/tsuku/create.go to include `cask:name` in the supported sources list.
