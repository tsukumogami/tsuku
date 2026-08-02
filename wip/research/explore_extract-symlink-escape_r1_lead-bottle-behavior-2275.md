# Lead: On current `main`, which symlink shapes does `ExtractAction.extractTarGz` actually accept, and which does it reject? Do the relative-symlink Homebrew bottles described in tsukumogami/tsuku#2275 extract today?

All results below come from driving real tar.gz archives through the real
`ExtractAction.extractTarGz` in package `actions`, on the worktree at
`/home/dgazineu/dev/niwaw/tsuku/tsuku+extract_symlink_escape-179b07a4/public/tsuku/.claude/worktrees/extract-symlink-escape`
(HEAD `bb146ed2`). Command: `go test ./internal/actions/ -run 'TestProbe' -v`.
Error strings are copied verbatim from the test log.

## Findings

### The answer up front

**#2275-shaped bottles do NOT universally extract today — it depends entirely on
the shape.** The dividing line is whether the symlink's *lexical* resolution
(`filepath.Join(filepath.Dir(linkLocation), linkTarget)`) ends up inside
`destPath`. Relative targets that climb up and come back down are fine *as long
as they don't climb above `destPath`*. The specific shape #2275 quotes —
`libexec/bin/python3.14 -> ../../../../../opt/python@3.14/bin/python3.14` — climbs
above `destPath` and is rejected with exactly the error the issue quotes.

So there are two distinct populations under the "relative symlink that exits and
re-enters" banner:

- **Exits and re-enters *within* `destPath`** (e.g. `pkg/1.0/bin/x ->
  ../../../pkg/1.0/libexec/x`, or `bin/tool -> ../libexec/bin/tool`): extracts
  today, no error. Any fix must not regress these.
- **Exits *above* `destPath` and re-enters a sibling Homebrew tree** (the #2275
  quoted shape): fails today. #2275 is asking for this to be *newly allowed*.

`#2473`'s acceptance criterion "#2275's relative-symlink bottles still extract"
is therefore only satisfiable for the first population. The second population
does not extract today at all, so "still" cannot apply to it.

### Behavior matrix

| Shape | Entries (name → type/linkname) | `strip_dirs` | Result | Escaped `destPath`? |
|---|---|---|---|---|
| #2473 two-hop escape | `a` → symlink `.`; `b` → symlink `a/..`; `b/pwned` → regular file | 0 | **no error** | **YES** — `pwned (file)` in `dest`'s parent |
| Chain to arbitrary absolute path | `h0` → symlink `.`; `h1` → symlink `h0/..`; `h2` → symlink `h1/..`; `h3` → symlink `h2/..`; `h4` → symlink `h3/..`; `h4/tmp/TestProbe02_.../001/ABSOLUTE_PROOF` → regular file | 0 | **no error** | **YES** — file written at the chosen absolute path `/tmp/TestProbe02_ChainToAbsolutePath135025694/001/ABSOLUTE_PROOF` |
| Directory + file through the escape link | `a` → symlink `.`; `b` → symlink `a/..`; `b/pwneddir` → dir; `b/pwneddir/f` → regular file | 0 | **no error** | **YES** — `pwneddir (dir)`, `pwneddir/f (file)` |
| Symlink created through the escape link | `a` → symlink `.`; `b` → symlink `a/..`; `b/outsidelink` → symlink `whatever` | 0 | **no error** | **YES** — `outsidelink (symlink)` |
| Absolute symlink target | `link` → symlink `/etc/passwd` | 0 | error: `absolute symlink targets are not allowed: /tmp/TestProbe03_AbsoluteSymlink2192668665/001/dest/link -> /etc/passwd` | no |
| Simple upward relative | `link` → symlink `../../evil` | 0 | error: `symlink target escapes destination directory: /tmp/TestProbe04_SimpleUpwardRelative401648231/001/dest/link -> ../../evil (resolves to /tmp/TestProbe04_SimpleUpwardRelative401648231/evil)` | no |
| Bottle: shallow sibling | `libexec/bin/tool` → file; `bin/tool` → symlink `../libexec/bin/tool` | 0 | **no error** | no |
| Bottle: up-3-and-back-down, stays inside root | `pkg/1.0/libexec/x` → file; `pkg/1.0/bin/x` → symlink `../../../pkg/1.0/libexec/x` | 0 | **no error** | no |
| Bottle: **#2275 quoted shape** | `libexec/bin/python3.14` → symlink `../../../../../opt/python@3.14/bin/python3.14` | 0 | error: `symlink target escapes destination directory: /tmp/TestProbe07_BottleUpAndBackDownAboveRoot1626098485/001/dest/libexec/bin/python3.14 -> ../../../../../opt/python@3.14/bin/python3.14 (resolves to /tmp/opt/python@3.14/bin/python3.14)` | no |
| Bottle: #2275 shape, **deeper `destPath`** (`dest/deep/deeper/deepest`) | same as above | 0 | error: `symlink target escapes destination directory: /tmp/TestProbe08_..._DeepDest96105004/001/dest/deep/deeper/deepest/libexec/bin/python3.14 -> ../../../../../opt/python@3.14/bin/python3.14 (resolves to /tmp/TestProbe08_..._DeepDest96105004/001/dest/opt/python@3.14/bin/python3.14)` | no |
| Bottle via real homebrew call path, sibling | `python@3.14/3.14.0/libexec/bin/tool` → file; `python@3.14/3.14.0/bin/tool` → symlink `../libexec/bin/tool` | **2** | **no error** | no |
| Bottle via real homebrew call path, #2275 shape | `python@3.14/3.14.0/libexec/bin/tool` → file; `python@3.14/3.14.0/bin/tool` → symlink `../../../../../opt/python@3.14/bin/python3.14` | **2** | error: `symlink target escapes destination directory: /tmp/TestProbe09_BottleWithStripDirs2207453036/002/dest/bin/tool -> ../../../../../opt/python@3.14/bin/python3.14 (resolves to /opt/python@3.14/bin/python3.14)` | no |
| Dangling symlink, target created by a later entry | `bin/tool` → symlink `../libexec/tool`; `libexec/tool` → regular file | 0 | **no error**; link created, then resolves and reads `"created after the link\n"` | no |
| Benign intra-archive relative | `a/file` → file; `b/link` → symlink `../a/file` | 0 | **no error** | no |
| `.`-link then `..` inside the *entry name* | `a` → symlink `.`; `a/../pwned` → regular file | 0 | **no error** | no — `filepath.Join` cleans the `..` out of the entry name before it reaches disk |
| Subdir link, write through it | `real` → dir; `link` → symlink `real`; `link/inside` → file | 0 | **no error** | no |

Notes on the deep-`destPath` row: the resolved path in that error is
`.../001/dest/opt/python@3.14/...`, which *looks* like it is inside `dest` — it is
not inside `destPath`, which is `.../001/dest/deep/deeper/deepest`. This is the
clearest demonstration that the check is purely lexical against `destPath`: the
same archive entry produces a different resolved path depending only on how deep
`destPath` happens to sit, and whether it passes is a function of that depth, not
of anything in the archive.

### Why the escape works, in the code

`validateSymlinkTarget` (`internal/actions/extract.go:39-55`) computes
`resolvedTarget := filepath.Join(filepath.Dir(linkLocation), linkTarget)` and
checks it with `isPathWithinDirectory`, which does `filepath.Abs` on both sides
and a string prefix test. `filepath.Abs` and `filepath.Join` both call
`filepath.Clean`, which collapses `..` textually. Neither function ever calls
`Lstat`, `Readlink`, or `EvalSymlinks`. So `Join(dest, "a/..")` is `dest` —
"inside" — regardless of what `a` actually points at on disk. The same lexical
cleaning happens to the tar entry name at `extract.go:316-321`, which is why
`b/pwned` passes the entry-level containment check while the OS follows the real
`b` symlink and writes to `dest`'s parent.

The `.`-link-then-`..`-in-entry-name variant does *not* escape, because the `..`
appears in the entry name itself and `filepath.Join(destPath, relativePath)`
cleans it away before `os.OpenFile` ever sees it. The escape needs the `..` to
live inside a *symlink target* (which is stored on disk verbatim and resolved by
the kernel later), not inside a path string the extractor cleans.

### How the homebrew action actually calls extract

`internal/actions/homebrew.go:113-124`: the bottle is extracted with
`format: "tar.gz"`, `strip_dirs: 2`, and no `dest` — so `dest` defaults to `"."`
and `destPath` is `ctx.WorkDir`. That means a bottle's `<formula>/<version>/`
prefix is stripped and everything lands directly in the work dir. A bottle
symlink at `<formula>/<ver>/libexec/bin/python3.14` therefore ends up at
`<workdir>/libexec/bin/python3.14`, only two directories deep, so the five `..`
hops in the #2275 target climb well past `destPath` — confirmed empirically in
the two `strip_dirs=2` rows above.

### Real recipes exercising this

1171 recipes under `recipes/` use `action = "homebrew"`
(`grep -rln 'action = "homebrew"' recipes/ | wc -l` → `1171`), so the bottle path
is heavily used. But there is **no bottle fixture in `testdata/`** — `find . -name
'*.tar.gz' -not -path './website/*'` returns nothing, and there is no
`testdata` directory under `internal/actions`. Bottles are fetched live from GHCR
at install time; nothing exercises real bottle symlinks in the test suite.

The one recipe that records this failure in prose is `recipes/a/awscli.toml:71`:

```
reason = "AWS publishes no versioned macOS zips (only PKG installers); Homebrew bottles fail sandbox extraction due to Python symlinks escaping the install dir. Linux installs are integrity-verified via PGP signature."
```

That is the recipe-level workaround for exactly #2275 — awscli's macOS support was
dropped rather than fixed. It is independent corroboration that the #2275 shape
fails today in production, not just in my synthetic archives.

Recipes mentioning "symlink" at all: `recipes/README.md`, `recipes/a/awscli.toml`,
`recipes/l/liberica.toml`, `recipes/p/pcre2.toml`, `recipes/g/gradle.toml`,
`recipes/o/openjdk.toml`, `recipes/o/ollama.toml`.

### `-short` and existing suite

`-short` makes no difference to any of these probes — nothing in this path is
gated on `testing.Short()`:

```
$ go test ./internal/actions/ -run 'TestProbe' -short
ok  	github.com/tsukumogami/tsuku/internal/actions	0.038s
```

The full existing package suite passes unchanged on this HEAD (I modified no
source):

```
$ go test ./internal/actions/
ok  	github.com/tsukumogami/tsuku/internal/actions	24.081s
```

### Throwaway test source

Written to `internal/actions/zz_throwaway_symlink_probe_test.go`, run, then
deleted. Reproduce by pasting this back into that path.

```go
package actions

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// ---- throwaway probe harness (DELETE ME) ----

type probeEntry struct {
	name     string // tar header name
	typ      byte   // tar.TypeReg / TypeDir / TypeSymlink
	linkname string // for symlinks
	body     string // for regular files
}

func probeBuildTarGz(t *testing.T, entries []probeEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	for _, e := range entries {
		h := &tar.Header{
			Name:     e.name,
			Typeflag: e.typ,
			Mode:     0644,
			Linkname: e.linkname,
		}
		switch e.typ {
		case tar.TypeDir:
			h.Mode = 0755
		case tar.TypeReg:
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatalf("WriteHeader(%s): %v", e.name, err)
		}
		if e.typ == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("Write(%s): %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// probeOutside walks root and returns every path that is NOT under dest.
func probeOutside(t *testing.T, root, dest string) []string {
	t.Helper()
	var out []string
	_ = filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if p == root {
			return nil
		}
		if p == dest {
			return filepath.SkipDir
		}
		// Ancestors of dest (when dest is nested) are not escapes.
		if strings.HasPrefix(dest, p+string(os.PathSeparator)) {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		kind := "file"
		if info.Mode()&os.ModeSymlink != 0 {
			kind = "symlink"
		} else if info.IsDir() {
			kind = "dir"
		}
		out = append(out, fmt.Sprintf("%s (%s)", rel, kind))
		return nil
	})
	sort.Strings(out)
	return out
}

// probeRun extracts entries into <root>/dest (plus optional extra depth) and
// reports the error plus anything that landed outside dest.
func probeRun(t *testing.T, entries []probeEntry, stripDirs int, extraDepth []string) (root, dest string, err error) {
	t.Helper()
	root = t.TempDir()
	parts := append([]string{root, "dest"}, extraDepth...)
	dest = filepath.Join(parts...)
	if mkErr := os.MkdirAll(dest, 0755); mkErr != nil {
		t.Fatal(mkErr)
	}
	archive := filepath.Join(root, "probe.tar.gz")
	if wErr := os.WriteFile(archive, probeBuildTarGz(t, entries), 0644); wErr != nil {
		t.Fatal(wErr)
	}
	a := &ExtractAction{}
	err = a.extractTarGz(archive, dest, stripDirs, nil)
	return root, dest, err
}

func probeReport(t *testing.T, label string, root, dest string, err error) {
	t.Helper()
	if err != nil {
		t.Logf("[%s] ERROR: %v", label, err)
	} else {
		t.Logf("[%s] OK (no error)", label)
	}
	outside := probeOutside(t, root, dest)
	// probe.tar.gz always sits in root; filter it out of the escape report.
	var real []string
	for _, o := range outside {
		if strings.HasPrefix(o, "probe.tar.gz") {
			continue
		}
		real = append(real, o)
	}
	if len(real) == 0 {
		t.Logf("[%s] ESCAPED: no", label)
	} else {
		t.Logf("[%s] ESCAPED: YES -> %v", label, real)
	}
}

// ---- Case 1: the #2473 two-hop escape ----

func TestProbe01_Escape2473(t *testing.T) {
	entries := []probeEntry{
		{name: "a", typ: tar.TypeSymlink, linkname: "."},
		{name: "b", typ: tar.TypeSymlink, linkname: "a/.."},
		{name: "b/pwned", typ: tar.TypeReg, body: "OWNED\n"},
	}
	root, dest, err := probeRun(t, entries, 0, nil)
	probeReport(t, "2473-two-hop", root, dest, err)
}

// ---- Case 2: longer chain reaching an arbitrary absolute path ----

func TestProbe02_ChainToAbsolutePath(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "dest")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatal(err)
	}

	// Number of hops needed to walk dest all the way up to "/".
	depth := len(strings.Split(strings.Trim(filepath.Clean(dest), "/"), "/"))
	t.Logf("dest = %s (depth from / = %d)", dest, depth)

	entries := []probeEntry{{name: "h0", typ: tar.TypeSymlink, linkname: "."}}
	for i := 1; i <= depth; i++ {
		entries = append(entries, probeEntry{
			name:     fmt.Sprintf("h%d", i),
			typ:      tar.TypeSymlink,
			linkname: fmt.Sprintf("h%d/..", i-1),
		})
	}
	// h<depth> now resolves to "/". Descend back down to an absolute path of
	// our choosing -- kept inside the same t.TempDir() so nothing real is touched.
	absTarget := filepath.Join(root, "ABSOLUTE_PROOF")
	entries = append(entries, probeEntry{
		name: fmt.Sprintf("h%d%s", depth, absTarget), // e.g. "h7/tmp/TestX/001/ABSOLUTE_PROOF"
		typ:  tar.TypeReg,
		body: "arbitrary absolute write\n",
	})

	archive := filepath.Join(root, "probe.tar.gz")
	if err := os.WriteFile(archive, probeBuildTarGz(t, entries), 0644); err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Logf("  entry: %-40s type=%c link=%s", e.name, e.typ, e.linkname)
	}
	a := &ExtractAction{}
	err := a.extractTarGz(archive, dest, 0, nil)
	probeReport(t, "chain-to-absolute", root, dest, err)
	if _, statErr := os.Lstat(absTarget); statErr == nil {
		t.Logf("[chain-to-absolute] CONFIRMED write at arbitrary absolute path %s", absTarget)
	} else {
		t.Logf("[chain-to-absolute] no file at %s (%v)", absTarget, statErr)
	}
}

// ---- Case 3: absolute symlink target ----

func TestProbe03_AbsoluteSymlink(t *testing.T) {
	entries := []probeEntry{
		{name: "link", typ: tar.TypeSymlink, linkname: "/etc/passwd"},
	}
	root, dest, err := probeRun(t, entries, 0, nil)
	probeReport(t, "absolute-etc-passwd", root, dest, err)
}

// ---- Case 4: simple upward relative target ----

func TestProbe04_SimpleUpwardRelative(t *testing.T) {
	entries := []probeEntry{
		{name: "link", typ: tar.TypeSymlink, linkname: "../../evil"},
	}
	root, dest, err := probeRun(t, entries, 0, nil)
	probeReport(t, "up-two-evil", root, dest, err)
}

// ---- Case 5: Homebrew bottle shapes (#2275) ----

func TestProbe05_BottleShallowSibling(t *testing.T) {
	// bin/tool -> ../libexec/bin/tool  (stays inside the bottle root)
	entries := []probeEntry{
		{name: "libexec/bin/tool", typ: tar.TypeReg, body: "real\n"},
		{name: "bin/tool", typ: tar.TypeSymlink, linkname: "../libexec/bin/tool"},
	}
	root, dest, err := probeRun(t, entries, 0, nil)
	probeReport(t, "bottle-shallow-sibling", root, dest, err)
}

func TestProbe06_BottleUpAndBackDownInsideRoot(t *testing.T) {
	// <pkg>/<ver>/bin/x -> ../../../<pkg>/<ver>/libexec/x
	// Climbs 3 levels (to dest) then comes back down. Never leaves dest.
	entries := []probeEntry{
		{name: "pkg/1.0/libexec/x", typ: tar.TypeReg, body: "real\n"},
		{name: "pkg/1.0/bin/x", typ: tar.TypeSymlink, linkname: "../../../pkg/1.0/libexec/x"},
	}
	root, dest, err := probeRun(t, entries, 0, nil)
	probeReport(t, "bottle-up3-back-down-inside", root, dest, err)
}

func TestProbe07_BottleUpAndBackDownAboveRoot(t *testing.T) {
	// The exact shape quoted in #2275:
	// libexec/bin/python3.14 -> ../../../../../opt/python@3.14/bin/python3.14
	entries := []probeEntry{
		{name: "libexec/bin/python3.14", typ: tar.TypeSymlink,
			linkname: "../../../../../opt/python@3.14/bin/python3.14"},
	}
	root, dest, err := probeRun(t, entries, 0, nil)
	probeReport(t, "bottle-2275-quoted-shape", root, dest, err)
}

func TestProbe08_BottleUpAndBackDownAboveRoot_DeepDest(t *testing.T) {
	// Same shape but destPath is deeper on disk -- proves the check is purely
	// lexical relative to destPath, not to the filesystem root.
	entries := []probeEntry{
		{name: "libexec/bin/python3.14", typ: tar.TypeSymlink,
			linkname: "../../../../../opt/python@3.14/bin/python3.14"},
	}
	root, dest, err := probeRun(t, entries, 0, []string{"deep", "deeper", "deepest"})
	probeReport(t, "bottle-2275-deep-dest", root, dest, err)
}

func TestProbe09_BottleWithStripDirs2(t *testing.T) {
	// How the homebrew action actually calls extract: strip_dirs = 2.
	// Bottle layout: <formula>/<version>/...
	cases := []struct {
		label string
		link  string
	}{
		{"strip2-sibling-inside", "../libexec/bin/tool"},
		{"strip2-2275-escape", "../../../../../opt/python@3.14/bin/python3.14"},
	}
	for _, c := range cases {
		entries := []probeEntry{
			{name: "python@3.14/3.14.0/libexec/bin/tool", typ: tar.TypeReg, body: "real\n"},
			{name: "python@3.14/3.14.0/bin/tool", typ: tar.TypeSymlink, linkname: c.link},
		}
		root, dest, err := probeRun(t, entries, 2, nil)
		probeReport(t, c.label, root, dest, err)
	}
}

// ---- Case 6: dangling symlink, target created by a LATER entry ----

func TestProbe10_DanglingThenCreated(t *testing.T) {
	entries := []probeEntry{
		{name: "bin/tool", typ: tar.TypeSymlink, linkname: "../libexec/tool"},
		{name: "libexec/tool", typ: tar.TypeReg, body: "created after the link\n"},
	}
	root, dest, err := probeRun(t, entries, 0, nil)
	probeReport(t, "dangling-then-created", root, dest, err)
	lp := filepath.Join(dest, "bin", "tool")
	if tgt, rErr := os.Readlink(lp); rErr == nil {
		t.Logf("[dangling-then-created] link exists -> %s", tgt)
	} else {
		t.Logf("[dangling-then-created] readlink failed: %v", rErr)
	}
	if b, rErr := os.ReadFile(lp); rErr == nil {
		t.Logf("[dangling-then-created] resolves and reads: %q", string(b))
	} else {
		t.Logf("[dangling-then-created] read through link failed: %v", rErr)
	}
}

// ---- Case 7: benign intra-archive relative symlink ----

func TestProbe11_BenignIntraArchive(t *testing.T) {
	entries := []probeEntry{
		{name: "a/file", typ: tar.TypeReg, body: "hello\n"},
		{name: "b/link", typ: tar.TypeSymlink, linkname: "../a/file"},
	}
	root, dest, err := probeRun(t, entries, 0, nil)
	probeReport(t, "benign-sibling", root, dest, err)
}

// ---- Case 8: minimal variants of the staged-symlink trick ----

func TestProbe12_StagedVariants(t *testing.T) {
	variants := []struct {
		label   string
		entries []probeEntry
	}{
		{
			// Single symlink to "." then traverse with ".." in the file entry.
			// filepath.Join cleans the "..", so the entry name never survives.
			label: "dot-link-then-dotdot-in-entry",
			entries: []probeEntry{
				{name: "a", typ: tar.TypeSymlink, linkname: "."},
				{name: "a/../pwned", typ: tar.TypeReg, body: "x\n"},
			},
		},
		{
			// Symlink to a subdirectory, then write through it (no escape).
			label: "subdir-link-write-through",
			entries: []probeEntry{
				{name: "real", typ: tar.TypeDir},
				{name: "link", typ: tar.TypeSymlink, linkname: "real"},
				{name: "link/inside", typ: tar.TypeReg, body: "x\n"},
			},
		},
		{
			// Directory entry written through the staged escape link.
			label: "2473-dir-through-link",
			entries: []probeEntry{
				{name: "a", typ: tar.TypeSymlink, linkname: "."},
				{name: "b", typ: tar.TypeSymlink, linkname: "a/.."},
				{name: "b/pwneddir", typ: tar.TypeDir},
				{name: "b/pwneddir/f", typ: tar.TypeReg, body: "x\n"},
			},
		},
		{
			// Symlink created THROUGH the escape link -- validation uses the
			// lexical target path, so the symlink itself lands outside.
			label: "2473-symlink-through-link",
			entries: []probeEntry{
				{name: "a", typ: tar.TypeSymlink, linkname: "."},
				{name: "b", typ: tar.TypeSymlink, linkname: "a/.."},
				{name: "b/outsidelink", typ: tar.TypeSymlink, linkname: "whatever"},
			},
		},
	}
	for _, v := range variants {
		root, dest, err := probeRun(t, v.entries, 0, nil)
		probeReport(t, v.label, root, dest, err)
	}
}
```

## Implications

**#2473's acceptance criterion "#2275's relative-symlink bottles still extract"
is asking for something narrower than it sounds.** Only the inside-`destPath`
population extracts today. Those cases pass because their *lexical* resolution
stays inside — and, crucially, their *real* resolution also stays inside, because
none of the intermediate path components in a well-formed bottle are symlinks
pointing outward. A resolving check (`EvalSymlinks` on the parent, or
`openat`-based descent) would therefore accept all of them unchanged. The fix
does not need to make an exception for bottles; it needs to not break the
already-passing ones, and a correct resolving implementation gets that for free.

**The #2275 population is out of scope for a #2473 fix and should stay out.**
Those symlinks are, by construction, pointers outside the extraction root — they
are asking the extractor to allow exactly the class of thing #2473 wants to stop.
Allowing them safely requires a separate, narrower mechanism (an allowlisted
Homebrew prefix, which is what #2275 itself proposes) layered on top of a correct
containment check, and it has to happen *after* `homebrew_relocate` rewrites
`@@HOMEBREW_PREFIX@@` — which today runs after extraction, so extraction fails
first and relocation never gets a chance. That ordering problem is real and
independent of #2473.

**The escape is worse than the two-hop example suggests.** Beyond writing a
regular file, the staged chain also lets an archive create *directories* and
*symlinks* outside `destPath` (rows 3 and 4 of the matrix). The
`2473-symlink-through-link` case is notable: `validateSymlinkTarget` runs against
the lexical location `dest/b/outsidelink`, decides it is fine, and then
`atomicSymlink` writes the link at the *real* location outside `dest`. So the
symlink validator's own output can land outside the directory it is validating
against. A fix that only hardens the regular-file write path would leave both of
these open.

**A fix must preserve dangling-symlink tolerance.** The
`dangling-then-created` case passes today — the link is created before its target
exists and resolves correctly once the later entry lands. Any `EvalSymlinks`-based
approach must resolve the *parent directory* of the entry, not the entry itself,
or it will start rejecting ordinary archives that list links before targets. This
is a real regression risk in the "run `EvalSymlinks` on each entry" suggested
direction in #2473 and is worth calling out in the design.

**There is no regression safety net for bottles.** With 1171 recipes on the
homebrew path and zero bottle fixtures in the repo, a change to symlink handling
has nothing local that would catch a break. If the fix ships, it should come with
a synthetic bottle-shaped fixture (the `strip_dirs=2` rows above are a starting
point) rather than relying on live GHCR fetches.

## Surprises

The awscli recipe already documents #2275 as a production failure in a
`verify.reason` field — macOS awscli support was *dropped* over this. That is a
stronger signal of impact than the issue text alone conveys, and it means the
"does it work today" question was already answered in the repo, just not
anywhere anyone would look.

The deep-`destPath` result surprised me: the *same* archive entry with the *same*
symlink target either passes or fails depending purely on how many directories
deep `destPath` happens to be. For a bottle extracted at `destPath` five or more
levels deep, `../../../../../opt/...` would land back inside `destPath` and be
accepted. So whether a #2275 bottle extracts today is partly a function of where
tsuku happens to put its work dir — which makes "do #2275 bottles extract" not
even a stable property of the archive.

`atomicSymlink` writes to `linkPath + ".tmp"` then renames. Through the escape
chain, both the temp file and the final rename target land outside `destPath`,
so the "atomic" TOCTOU protection is operating entirely outside the directory it
believes it is protecting.

The `.`-link-plus-`..`-in-entry-name variant does not escape. That is a useful
negative: it confirms the vulnerability specifically requires the `..` to be
stored in a symlink target on disk, and that hardening the entry-name path alone
(which already works) is not where the fix goes.

## Open Questions

Does the real Homebrew bottle population actually contain the #2275 shape at the
frequency the issue implies, or is `python@`-style cross-cellar linking confined
to a handful of formulas? I could not check without fetching live bottles from
GHCR. This matters for sizing: if it is a dozen formulas, a recipe-level
workaround may beat a code change.

Where should the Homebrew-prefix allowlist live if #2275 is eventually fixed?
Extraction currently runs before `homebrew_relocate`, so at extract time the
placeholder has not been rewritten and the extractor has no idea what the target
prefix will be. Either extraction needs to learn the prefix, or the symlinks need
to be deferred and created post-relocation. Neither is decided.

Does the zip path have the same staged-symlink exposure? `extractZip`
(`extract.go:390-462`) has the same lexical `isPathWithinDirectory` check but no
`tar.TypeSymlink` equivalent — Go's `archive/zip` does not create symlinks in this
code, so an archive cannot stage the link. But a zip extracted into a directory
where a *previous* action already created a symlink would still be traversable. I
did not test cross-action staging.

## Summary

The #2275 Homebrew shape does not extract today: the exact quoted symlink
`libexec/bin/python3.14 -> ../../../../../opt/python@3.14/bin/python3.14` fails
with `symlink target escapes destination directory`, and `recipes/a/awscli.toml`
already records that macOS awscli was dropped for this reason — while shallower
bottle symlinks that climb up and back down *without* passing above `destPath`
extract fine, so #2473's "bottles still extract" criterion only binds on that
second, already-working population. Meanwhile the #2473 escape is confirmed and
broader than reported: the two-hop chain writes a file outside `destPath` with no
error, a longer chain reaches an arbitrary absolute path, and the same trick also
plants directories and symlinks outside, meaning a resolving fix must cover every
entry type rather than just regular files. The biggest open question is where a
Homebrew-prefix exception would even live, since extraction runs before
`homebrew_relocate` rewrites the placeholder and so cannot know the legitimate
target prefix at the time it must decide.
