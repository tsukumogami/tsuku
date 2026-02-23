# Maintainer Review: Issue #1903 (ci: add Renovate config and drift-check CI job)

## Files Reviewed

- `renovate.json` (new)
- `.github/workflows/drift-check.yml` (new)

## Findings

### Finding 1: Divergent twins in hardcoded-reference search loops

**File**: `.github/workflows/drift-check.yml`, lines 110-142
**Severity**: Advisory

Three identical `while IFS= read -r file` loop bodies search workflow files (`.yml`), Go files (`.go`), and shell scripts (`.sh`) respectively. The only difference between the three loops is the `find` command in the process substitution. The grep/filter/accumulate logic is copied verbatim.

If the next developer needs to change the search logic -- say, adding a new output format or adjusting how exceptions are applied -- they must update all three loops identically. If they update two and miss one, the difference will look accidental.

This could be consolidated into a shell function:

```bash
search_files() {
  while IFS= read -r file; do
    matches=$(grep -nE "$PATTERN" "$file" 2>/dev/null | sed "s|^|$file:|" || true)
    [ -z "$matches" ] && continue
    filtered=$(echo "$matches" | grep -vE "$EXCLUDE_PATTERN" || true)
    if [ -n "$filtered" ]; then
      violations="$violations"$'\n'"$filtered"
      found=1
    fi
  done
}

find .github/workflows -name '*.yml' -type f 2>/dev/null | search_files
find . -name '*.go' -type f -not -path './vendor/*' | search_files
find . -name '*.sh' -type f -not -path './vendor/*' | search_files
```

Not blocking because the current code is correct and the script is a single self-contained block, but the triple duplication is a maintenance trap.

---

### Finding 2: Workflow file search misses `.yaml` extension

**File**: `.github/workflows/drift-check.yml`, line 118
**Severity**: Advisory

The `find` command searches only `*.yml`:

```bash
find .github/workflows -name '*.yml' -type f 2>/dev/null
```

The repo already has at least one `.yaml` file: `.github/workflows/checksum-drift.yaml`. The workflow's `paths` trigger at line 9 uses `.github/workflows/**`, which covers both extensions -- so the trigger fires on `.yaml` changes, but the search doesn't scan them.

Currently none of the `.yaml` files contain hardcoded container image references, so this isn't causing false negatives today. But if someone adds a new workflow as `.yaml` and hardcodes an image, the drift check won't catch it, despite being triggered. The next developer would reasonably assume that since the workflow triggers on all workflow changes, it checks all workflow files.

Fix: change `'*.yml'` to `\( -name '*.yml' -o -name '*.yaml' \)` or use a glob like `'*.y*ml'`.

---

### Finding 3: Renovate regex won't match `opensuse/tumbleweed` (no tag)

**File**: `renovate.json`, line 8
**Severity**: Advisory

The Renovate regex requires a colon between `depName` and `currentValue`:

```
"(?<depName>[a-z][a-z0-9./-]+):\s*(?<currentValue>[a-z0-9][a-z0-9._-]+)"
```

But `container-images.json` has:

```json
"suse": "opensuse/tumbleweed"
```

There's no `:tag` in `opensuse/tumbleweed` -- it's a rolling release image with no version tag. The regex won't capture this entry, so Renovate will silently skip it.

This is probably fine since tumbleweed is rolling and there's nothing to "bump." But there's no comment in `renovate.json` explaining this intentional gap. The next developer looking at the config will wonder why Renovate never proposes updates for the suse image and whether that's a bug.

Add a JSON comment (Renovate supports `//` comments in its JSON config) explaining that `opensuse/tumbleweed` is deliberately unmanaged because it has no versioned tags.

---

### Finding 4: Comment on exception loop is slightly misleading

**File**: `.github/workflows/drift-check.yml`, lines 96-98
**Severity**: Advisory

```bash
# Strip leading whitespace and skip comments
exc=$(echo "$exc" | sed 's/^\s*//')
case "$exc" in '#'*|'') continue;; esac
```

The comment says "skip comments," but the `#` lines in the EXCEPTIONS array are bash comments -- the shell strips them during array construction. No array element will ever start with `#`. The `case` guard is defensive coding against future misuse (someone accidentally adding a `# comment` as a string element), which is fine, but the comment implies active filtering of existing content.

A more accurate comment: `# Strip leading whitespace; skip if empty (guard against future misuse)`. Minor, but the current wording could send a developer on a detour trying to understand which "comments" are being skipped.

---

### Finding 5: Missing `permissions` block

**File**: `.github/workflows/drift-check.yml`
**Severity**: Advisory

The workflow doesn't declare a `permissions` block. Other workflows in the repo (e.g., `checksum-drift.yaml`) set explicit permissions. This workflow only needs `contents: read`. Adding it follows the principle of least privilege and makes the workflow's scope explicit to the next reader.

---

## Overall Assessment

The implementation is clean and well-structured. The two files serve distinct purposes that are clearly communicated: `renovate.json` handles proactive version bump proposals, and `drift-check.yml` catches both stale embedded copies and hardcoded image regressions.

The drift-check script is the more complex piece. Its PATTERN and EXCEPTIONS design is sound -- the exception list is well-documented with per-entry comments explaining why each exclusion exists, and the self-referencing concern (the script matching its own pattern definition) is avoided because the regex metacharacters in the pattern string prevent literal matches. The error output includes GitHub annotations with file/line references, which is a nice touch for debugging.

No blocking findings. The five advisories are all about reducing traps for the next developer: the `.yaml` gap is a latent coverage hole, the triple loop duplication is a maintenance risk, and the Renovate/tumbleweed gap needs a comment so nobody wastes time investigating "missing" Renovate PRs.
