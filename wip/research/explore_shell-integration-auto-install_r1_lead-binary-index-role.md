# Research: Binary Index Role in Project-Declared Tool Use Case

## Summary

The binary index (issue #1677) and project config (Block 4) serve separate concerns. The binary index maps `command_name → recipe` for discovery of unknown commands. Project config already knows the recipe name -- it goes directly to install. These are complementary lookup paths, not overlapping. The project config use case does NOT require the binary index. One nuance: when recipe name differs from binary name (e.g., recipe "aws-cli" provides binary "aws"), project config needs to know what binaries a recipe provides to manage PATH symlinks. This data already exists in `VersionState.Binaries` post-install.

---

## Two Distinct Lookup Paths

### Path A: Unknown Command Discovery (Binary Index use case)

```
User types `jq` → command not found → binary index lookup("jq") → recipe "jq" → suggest/install
```

The binary index solves: given a command name the user typed, which recipe provides it?

### Path B: Project-Declared Tool Install (tsuku.toml use case)

```
tsuku.toml: [tools] koto = ">=0.3" → recipe "koto" already known → resolve version → install
```

The project config path already knows the recipe name. No reverse lookup needed.

### Are They the Same Code Path?

No. They diverge at the "how do I know the recipe name?" step:
- Path A: binary index lookup to discover recipe
- Path B: recipe name is declared in tsuku.toml

After that point, both converge on the same install machinery.

---

## One Nuance: Recipe Name vs Binary Name

When recipe name ≠ binary name (e.g., recipe "aws-cli" provides binary "aws"), the project config path needs to know what binaries a recipe provides for PATH symlink management.

This data exists in:
- `VersionState.Binaries` in state.json (post-install, per-version tracking)
- `Recipe.ExtractBinaries()` method (pre-install, from recipe metadata)

The binary index could serve double duty here (maps binary name → recipe AND recipe → binary names), but this creates coupling. A cleaner design keeps the binary index for discovery and uses `ExtractBinaries()` directly for the project config path.

---

## Does Issue #1677 Need to Change?

Issue #1677 is scoped correctly for Track A (command-not-found discovery). It does NOT need to expand to cover the tsuku.toml use case.

However, the binary index design SHOULD clarify:
1. The index serves Track A (unknown command → recipe), not Track B (project config → install)
2. Both paths share the underlying `ExtractBinaries()` data source
3. The binary index is not a prerequisite for implementing Block 4 (project config)

This means Track B (project config) can be implemented in parallel with or before Track A's binary index, as the design intends.

---

## Implications for the Convergence Question

The "optional ProjectConfig parameter" the parent design proposes for Block 3 (Auto-Install) is about:
- When auto-installing a command, check if tsuku.toml specifies a version constraint
- If yes, install that version instead of latest

This is a version-constraint overlay, not a discovery mechanism. The binary index is still needed to find the recipe for an unknown command; the project config adds a version override after the recipe is found.

Complete flow with convergence:
```
User types `koto` (not installed)
→ command-not-found handler (Block 2) calls tsuku
→ binary index (Block 1) finds recipe "koto"
→ auto-install (Block 3) checks tsuku.toml for version constraint
→ installs koto@>=0.3 (from project config) instead of latest
→ executes koto
```

Without command-not-found hook (CI/scripts):
```
tsuku run koto [args]
→ finds recipe "koto" (known from argument, no index needed)
→ checks tsuku.toml for version constraint
→ installs if needed
→ executes koto
```
