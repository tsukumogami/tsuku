# Decision C: What test suite proves the guard actually works, and how do we demonstrate each guard is load-bearing?

Everything below was prototyped for real. A scratch suite was written to
`internal/actions/zz_decision_c_scratch_test.go`, `extract.go` and
`app_bundle.go` were patched with `os.OpenRoot`, the suite was run against both
the unpatched and patched trees, twelve defects were injected one at a time, and
all of it was then reverted. Final state is verified clean at the end of the
Evidence section.

## Options

**A. Assert the validation helpers return errors.** Ruled out by the issue
itself, and correctly: `isPathWithinDirectory("dest/b/pwned", "dest")` returns
`true` and `validateSymlinkTarget("a/..", "dest/b", "dest")` returns `nil` for
the exact escape input. A helper-level assertion would pass on both the
vulnerable and the fixed tree, proving nothing.

**B. Assert extraction returns a non-nil error.** Necessary but far too weak.
It cannot distinguish "refused before writing" from "wrote outside the
destination and then errored on a later entry" — which is a real failure mode,
not a hypothetical: mutation M4 below produces exactly that, creating
directories in the destination's parent and *then* returning an error. A
`wantErr`-only suite scores that as a pass.

**C. Assert specific escape paths do not exist.** Better, but it only catches
the escape the test author thought of. It misses a differently-named artifact,
misses directories created as a side effect of a blocked write, and — most
importantly — misses *overwrite* of a file that already existed outside the
destination, since the path is present both before and after.

**D. Fingerprint everything outside the destination before and after, and
diff.** Recommended. Catches creation, deletion and in-place modification of any
path, under any name, of any type, without the test having to predict the
payload. This is what closes the gap in C, and it is what makes mutations M4,
M9 and M12 detectable at all.

## Evidence

### Baseline: the suite on unpatched `main`

31 cases across three extractors. Every escape case fails; every
must-keep-working case passes.

```
$ go test ./internal/actions/ -run 'TestScratch' -v
--- FAIL: TestScratchTarContainment/escape/regular-file-through-staged-symlink
--- FAIL: TestScratchTarContainment/escape/directory-through-staged-symlink
--- FAIL: TestScratchTarContainment/escape/symlink-through-staged-symlink
--- FAIL: TestScratchTarContainment/escape/overwrite-existing-outside-file
--- FAIL: TestScratchTarContainment/escape/chain-to-arbitrary-absolute-path
--- FAIL: TestScratchTarContainment/escape/nested-parent-dirs-created-outside-before-write
--- FAIL: TestScratchTarContainment/escape/final-component-symlink-overwrites-outside-file
--- FAIL: TestScratchTarContainment/escape/final-component-symlink-creates-outside-file
--- PASS: TestScratchTarContainment/legit/write-through-earlier-in-root-symlink
--- PASS: TestScratchTarContainment/legit/bottle-shallow-sibling
--- PASS: TestScratchTarContainment/legit/bottle-up-and-back-down-inside-root
--- PASS: TestScratchTarContainment/legit/bottle-with-strip-dirs-2
--- PASS: TestScratchTarContainment/legit/dangling-symlink-target-created-later
--- PASS: TestScratchTarContainment/legit/benign-sibling-relative-symlink
--- PASS: TestScratchTarContainment/legit/dotdot-inside-entry-name-is-cleaned
--- PASS: TestScratchTarContainment/legit/destination-directory-does-not-exist-yet
--- PASS: TestScratchTarContainment/policy/absolute-symlink-target-rejected
--- PASS: TestScratchTarContainment/policy/upward-relative-symlink-target-rejected
--- PASS: TestScratchTarContainment/policy/entry-name-traversal-diagnostic
--- FAIL: TestScratchZipContainment/zip/escape-through-preexisting-symlink
--- FAIL: TestScratchZipContainment/zip/overwrite-existing-outside-file-through-preexisting-symlink
--- PASS: TestScratchZipContainment/zip/legit-write-through-preexisting-in-root-symlink
--- FAIL: TestScratchZipContainment/zip/nested-parent-dirs-created-outside-before-write
--- FAIL: TestScratchZipContainment/zip/final-component-symlink-overwrites-outside-file
--- PASS: TestScratchZipContainment/zip/plain-nested-file
--- PASS: TestScratchZipContainment/zip/destination-directory-does-not-exist-yet
--- FAIL: TestScratchAppBundleContainment/appbundle/escape-through-staged-symlink
--- FAIL: TestScratchAppBundleContainment/appbundle/symlink-through-staged-symlink
--- FAIL: TestScratchAppBundleContainment/appbundle/nested-parent-dirs-created-outside-before-write
--- FAIL: TestScratchAppBundleContainment/appbundle/final-component-symlink-overwrites-outside-file
--- PASS: TestScratchAppBundleContainment/appbundle/legit-framework-versions-current
```

16 escape cases red, 15 compatibility cases green. That split matters: a
compatibility case that only turns green *after* the fix would be proving
nothing about regression risk.

The failure output shows why option D beats B and C. Note that in every one of
these the extractor also returned `nil`:

```
--- FAIL: .../escape/directory-through-staged-symlink
    extraction wrote outside the destination directory:
      created: pwneddir (dir)
      created: pwneddir/f (file 93fc6ec560dce84a)
    expected extraction to fail, got nil
--- FAIL: .../escape/overwrite-existing-outside-file
    extraction wrote outside the destination directory:
      modified: canary.txt (file 9f2235c7754a56a9 -> file 06c9c46ca6d42f6e)
    expected extraction to fail, got nil
--- FAIL: .../escape/chain-to-arbitrary-absolute-path
    extraction wrote outside the destination directory:
      created: ABSOLUTE_PROOF (file 6597e095990a03ff)
    expected extraction to fail, got nil
```

The `modified:` line is the one option C could never have produced.

### The same suite on the `os.OpenRoot` tree

```
$ go test ./internal/actions/ -run 'TestScratch'
ok  	github.com/tsukumogami/tsuku/internal/actions	0.069s

$ go test ./internal/actions/          # full existing package suite
ok  	github.com/tsukumogami/tsuku/internal/actions	24.018s
```

All 31 pass, and the 873-line existing `extract_test.go` needs no edits.

The `legit/write-through-earlier-in-root-symlink` case (`lib64/` dir, then
`lib -> lib64`, then `lib/foo.so`) is the one an over-strict fix breaks. It
passes both before and after: `os.Root` permits traversal through an in-root
symlink and only refuses when resolution leaves the root.

### The implementation defect this suite caught

The first `os.Root` port failed one case:

```
--- FAIL: TestScratchAppBundleContainment/appbundle/legit-framework-versions-current
    expected extraction to succeed, got:
      mkdirat Foo.app/.../Versions/A: unsupported file mode
```

`app_bundle.go:282` passes `file.Mode()` straight to `MkdirAll`. `os.MkdirAll`
masks the type bits on Unix; `os.Root.MkdirAll` rejects them. The port needs
`file.Mode().Perm()`. A suite that only carried attack archives would have
shipped this.

### Mutation matrix, first attempt — and why it had to be redone

The first version of the suite (attack cases plus the obvious happy paths) was
run against twelve injected defects. **Ten of twelve survived.**

```
### M1 tar regular-file write reverted to os.OpenFile     SURVIVED
### M2 tar symlink creation reverted to os.Symlink        SURVIVED
### M3 tar directory entry reverted to os.MkdirAll        FAIL: escape/directory-through-staged-symlink
### M4 tar parent-dir creation reverted to os.MkdirAll    SURVIVED
### M5 destPath MkdirAll dropped before OpenRoot          SURVIVED
### M6 lexical entry-name pre-check dropped               SURVIVED
### M7 validateSymlinkTarget policy call dropped          FAIL: policy/absolute-symlink-target-rejected
                                                          FAIL: policy/upward-relative-symlink-target-rejected
### M8 zip regular-file write reverted to os.OpenFile     SURVIVED
### M9 zip parent-dir creation reverted to os.MkdirAll    SURVIVED
### M10 app_bundle write reverted to os.OpenFile          SURVIVED
### M11 app_bundle symlink reverted to os.Symlink         SURVIVED
### M12 app_bundle parent-dir reverted to os.MkdirAll     SURVIVED
```

The guards mask each other. Against the canonical `b -> a/..` staged escape, the
write to `b/pwned` is refused by `root.MkdirAll("b")` *before* the file open is
reached — so reverting the file open alone changes nothing observable. A suite
built only from the issue's reproducer gives a completely false picture of which
line is doing the work.

Three case shapes fix this, each isolating a guard the staged escape masks:

- **Deep nesting** (`b/deep/nested/pwned`). The open is still blocked, but a
  non-root-scoped `MkdirAll` has already materialised `sandbox/deep/nested`
  before it. Only the fingerprint diff sees this. Isolates M4/M9/M12.
- **Final-component escape.** Parent `d` is a genuine in-root directory, and the
  escaping symlink is the last component: `p -> "."`, `d/` , then
  `d/x -> ../p/../canary.txt`, then a regular file at `d/x`. Its *lexical*
  resolution is `dest/canary.txt` — inside — so `validateSymlinkTarget` accepts
  it; its *real* resolution is `sandbox/canary.txt`. `MkdirAll("d")` succeeds,
  so only the file open can catch it. Isolates M1/M8/M10.
- **Absent destination** and **diagnostic pinning**, isolating M5 and M6.

### Mutation matrix, final — ten of twelve killed, each by one named test

```
### M1 tar regular-file write reverted to os.OpenFile
  FAIL: TestScratchTarContainment/escape/final-component-symlink-creates-outside-file
  FAIL: TestScratchTarContainment/escape/final-component-symlink-overwrites-outside-file
### M2 tar symlink creation reverted to os.Symlink
  SURVIVED -- no test failed
### M3 tar directory entry reverted to os.MkdirAll
  FAIL: TestScratchTarContainment/escape/directory-through-staged-symlink
### M4 tar parent-dir creation reverted to os.MkdirAll
  FAIL: TestScratchTarContainment/escape/nested-parent-dirs-created-outside-before-write
### M5 destPath MkdirAll dropped before OpenRoot (tar)
  FAIL: TestScratchTarContainment/legit/destination-directory-does-not-exist-yet
### M6 lexical entry-name pre-check dropped (tar)
  FAIL: TestScratchTarContainment/policy/entry-name-traversal-diagnostic
### M7 validateSymlinkTarget policy call dropped (tar)
  FAIL: TestScratchTarContainment/policy/absolute-symlink-target-rejected
  FAIL: TestScratchTarContainment/policy/upward-relative-symlink-target-rejected
### M8 zip regular-file write reverted to os.OpenFile
  FAIL: TestScratchZipContainment/zip/final-component-symlink-overwrites-outside-file
### M9 zip parent-dir creation reverted to os.MkdirAll
  FAIL: TestScratchZipContainment/zip/nested-parent-dirs-created-outside-before-write
### M10 app_bundle regular-file write reverted to os.OpenFile
  FAIL: TestScratchAppBundleContainment/appbundle/final-component-symlink-overwrites-outside-file
### M11 app_bundle symlink creation reverted to os.Symlink
  SURVIVED -- no test failed
### M12 app_bundle parent-dir creation reverted to os.MkdirAll
  FAIL: TestScratchAppBundleContainment/appbundle/nested-parent-dirs-created-outside-before-write
```

### Why M2 and M11 survive, and why that is not a coverage gap

Root-scoping the symlink *creation* cannot be independently killed, because
neither `symlink(2)` nor `rename(2)` follows a symlink in the final path
component. Whenever a symlink entry's *parent* escapes, `root.MkdirAll` fires
first; when the parent is in-root, an unscoped `os.Symlink`/`os.Rename` still
cannot escape. Measured directly:

```
os.Symlink onto existing escaping link: symlink whatever /tmp/.../dest/d/x: file exists
os.Symlink to .tmp: <nil>
os.Rename tmp -> x: <nil>
sandbox/victim.txt after = "pristine"
dest/d/x now -> "whatever" (rename replaced the link, did not follow it)
```

Keep the root-scoped symlink write anyway — it costs nothing and removes a
dependency on that kernel semantic — but record in the PR that it is
defence-in-depth, not a killable guard. Claiming otherwise would be false.

### The tempdir trap (sub-question 3)

CI fails on `git status --porcelain` being non-empty
(`.github/workflows/test.yml:77-85`), and these tests write outside their
destination by construction when the fix is absent. The escape from the
canonical chain lands in **`dest`'s parent**, so the parent must itself be
disposable:

```
<t.TempDir()>/          archive.tar.gz lives here, never fingerprinted
  sandbox/              <- the "outside"; escapes land here
    canary.txt          <- pre-existing file an escape can overwrite
    dest/               <- destPath handed to the extractor
```

Two levels, not one. Putting the archive in `sandbox` instead of the temp root
would make it show up in every fingerprint diff as noise. Verified: after
running the full suite against the *unpatched* tree — the run that provokes
every escape — the source tree is untouched:

```
$ go test ./internal/actions/ -run 'TestScratch'   # 16 escapes provoked
$ git status --porcelain
 M internal/actions/app_bundle.go     <- my hand edits
 M internal/actions/extract.go        <- my hand edits
?? internal/actions/zz_decision_c_scratch_test.go
?? wip/...
```

Nothing from the test run itself. The `chain-to-arbitrary-absolute-path` case
deserves a note: it genuinely reaches `/` and then descends to an absolute path
of the archive's choosing. It aims that path back inside `t.TempDir()` purely
for hygiene — the mechanism is unrestricted, as the exploration established.

### Lint constraints (sub-question 4), all measured

`errcheck` **does** lint test files and does **not** exclude the three functions
a fixture builder needs. Probed directly with a throwaway test containing
unchecked calls:

```
zz_lintprobe_test.go:11:14: Error return value of `os.WriteFile` is not checked (errcheck)
zz_lintprobe_test.go:12:13: Error return value of `os.MkdirAll` is not checked (errcheck)
zz_lintprobe_test.go:13:12: Error return value of `os.Symlink` is not checked (errcheck)
```

`.golangci.yaml:85-104` has no `tests: false`, and its `exclude-functions` list
covers only `Close` variants, `os.Remove`, `os.RemoveAll`, `os.Setenv` and
`fmt.Fprintf`. Every fixture write must be checked.

`dupl` at threshold 250 (`.golangci.yaml:81-83`) **does** fire on near-identical
tar and zip test bodies. Probed with two deliberately un-factored ~60-line
drivers:

```
2 issues:
* dupl: 2
```

The recommended suite, which routes all three extractors through one
`runExtractCase` body, lints clean:

```
$ golangci-lint run ./internal/actions/...
0 issues.
```

So the shared-runner shape is not a style preference — it is what keeps the
suite lintable.

**One finding for the implementation, not the tests.** `errcheck` flags the
production code:

```
internal/actions/extract.go:298:18: Error return value of `root.Close` is not checked (errcheck)
internal/actions/extract.go:435:18: Error return value of `root.Close` is not checked (errcheck)
internal/actions/app_bundle.go:277:18: Error return value of `root.Close` is not checked (errcheck)
```

`(io.Closer).Close` in the exclusion list does not match a concrete `*os.Root`.
Adding `- (*os.Root).Close` next to `(*os.File).Close` in
`.golangci.yaml` clears it — verified, `0 issues`. Alternative is
`defer func() { _ = root.Close() }()` at three sites; the config line is
cleaner and matches how the file already treats `*os.File`.

`-short`: passed in **both** branches of the unit-test job
(`.github/workflows/test.yml:65-73`), so gating anything on `testing.Short()`
would skip it on every PR *and* every push to main. Nothing in the recommended
suite is gated, and it runs in 0.07s:

```
$ go test ./internal/actions/ -run 'TestScratch' -short
ok  	github.com/tsukumogami/tsuku/internal/actions	0.050s
```

Note also that `lint_test.go:15-19` skips golangci-lint under `-short`, so the
`unit-tests` job never lints; only the separate `lint-tests` job does
(`test.yml:115-116`).

### Final state

```
$ git status --porcelain
?? wip/design_extract-symlink-escape_coordination.json
?? wip/design_extract-symlink-escape_decision_A_report.md
?? wip/scope_extract-symlink-escape_state.md

$ git diff --stat
(empty)

$ go build ./... && go test ./internal/actions/
ok  	github.com/tsukumogami/tsuku/internal/actions	24.334s
```

`extract.go`, `app_bundle.go` and `.golangci.yaml` are byte-identical to `main`;
the scratch test and both lint probes are deleted. Only `wip/` differs, and two
of those three files belong to other agents.

## Recommendation

Adopt option D: **a pre/post fingerprint diff of everything outside the
destination, asserted on every case whether it expects success or failure.**

### The containment helper

```go
// fingerprintOutside records every filesystem entry under sandbox that is not
// inside dest, keyed by path and valued by a type-and-content fingerprint.
// Comparing two fingerprints therefore catches creation, deletion AND
// in-place overwrite of anything outside the extraction destination.
func fingerprintOutside(t *testing.T, sandbox, dest string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(sandbox, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if p == sandbox {
			return nil
		}
		if p == dest {
			return fs.SkipDir
		}
		rel, relErr := filepath.Rel(sandbox, p)
		if relErr != nil {
			return relErr
		}
		switch {
		case d.Type()&fs.ModeSymlink != 0:
			tgt, rlErr := os.Readlink(p)
			if rlErr != nil {
				return rlErr
			}
			out[rel] = "symlink -> " + tgt
		case d.IsDir():
			out[rel] = "dir"
		default:
			b, rdErr := os.ReadFile(p)
			if rdErr != nil {
				return rdErr
			}
			sum := sha256.Sum256(b)
			out[rel] = "file " + hex.EncodeToString(sum[:8])
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", sandbox, err)
	}
	return out
}

// assertNothingEscaped fails with a readable diff if anything outside dest
// changed between before and after.
func assertNothingEscaped(t *testing.T, before, after map[string]string) {
	t.Helper()
	var diffs []string
	for k, v := range after {
		if old, ok := before[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("  created: %s (%s)", k, v))
		} else if old != v {
			diffs = append(diffs, fmt.Sprintf("  modified: %s (%s -> %s)", k, old, v))
		}
	}
	for k, v := range before {
		if _, ok := after[k]; !ok {
			diffs = append(diffs, fmt.Sprintf("  removed: %s (%s)", k, v))
		}
	}
	if len(diffs) > 0 {
		sort.Strings(diffs)
		t.Errorf("extraction wrote outside the destination directory:\n%s", strings.Join(diffs, "\n"))
	}
}
```

`WalkDir` does not follow symlinks, so an escaped symlink is reported as a
symlink rather than silently traversed. `SkipDir` at `dest` is applied
symmetrically in both snapshots, so a destination that did not exist beforehand
does not register as a change.

### The sandbox

```go
// newSandbox builds the two-level layout the escape tests need:
//
//	<t.TempDir()>/            archive lives here, never inspected
//	  sandbox/                the "outside" the guard must protect
//	    canary.txt            pre-existing file an escape could overwrite
//	    dest/                 destPath handed to the extractor
func newSandbox(t *testing.T) (archiveDir, sandbox, dest string) {
	t.Helper()
	archiveDir = t.TempDir()
	sandbox = filepath.Join(archiveDir, "sandbox")
	dest = filepath.Join(sandbox, "dest")
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sandbox, "canary.txt"), []byte("pristine"), 0644); err != nil {
		t.Fatalf("write canary: %v", err)
	}
	return archiveDir, sandbox, dest
}
```

### The runner — one body, three extractors

Sharing this is what keeps `dupl` quiet. The format-specific parts are the
serialiser and the extract call, both injected.

```go
type extractCase struct {
	name string
	// build returns the archive members; dest is passed so the absolute-chain
	// case can compute how many hops it needs.
	build func(t *testing.T, sandbox, dest string) []archiveEntry
	// setup plants state in dest before extraction (pre-existing symlinks).
	setup     func(t *testing.T, dest string)
	stripDirs int
	wantErr   bool
	// wantErrContains pins the diagnostic, not just the failure.
	wantErrContains string
	// destAbsent removes dest before extraction, so the extractor must create it.
	destAbsent bool
	// wantInside are paths relative to dest that must exist afterwards, and
	// for regular files, their expected contents.
	wantInside map[string]string
}

func runExtractCase(t *testing.T, tc extractCase, ext string,
	build func(*testing.T, []archiveEntry) []byte,
	extract func(archivePath, destPath string, stripDirs int) error,
) {
	t.Helper()
	archiveDir, sandbox, dest := newSandbox(t)
	if tc.destAbsent {
		if err := os.Remove(dest); err != nil {
			t.Fatalf("remove dest: %v", err)
		}
	}
	if tc.setup != nil {
		tc.setup(t, dest)
	}
	archivePath := filepath.Join(archiveDir, "archive"+ext)
	if err := os.WriteFile(archivePath, build(t, tc.build(t, sandbox, dest)), 0644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	before := fingerprintOutside(t, sandbox, dest)
	err := extract(archivePath, dest, tc.stripDirs)
	after := fingerprintOutside(t, sandbox, dest)

	assertNothingEscaped(t, before, after)

	if tc.wantErr && err == nil {
		t.Errorf("expected extraction to fail, got nil")
	}
	if !tc.wantErr && err != nil {
		t.Errorf("expected extraction to succeed, got: %v", err)
	}
	if tc.wantErrContains != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErrContains)) {
		t.Errorf("error = %v, want it to mention %q", err, tc.wantErrContains)
	}
	for rel, want := range tc.wantInside {
		got, readErr := os.ReadFile(filepath.Join(dest, rel))
		if readErr != nil {
			t.Errorf("expected %s inside dest: %v", rel, readErr)
			continue
		}
		if string(got) != want {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}
```

`wantInside` reads *through* the symlinks the archive shipped, which is what
makes `legit/bottle-shallow-sibling` and `legit/dangling-symlink-target-created-later`
meaningful rather than decorative — they assert the link resolves to the right
content, not merely that a link exists.

Fixture builders: one `[]archiveEntry` type (`name`, `typ`, `link`, `body`),
serialised as tar.gz by `buildTarGzEntries` and as zip by `buildZipEntries`.
The zip builder encodes symlinks the way real tooling does —
`hdr.SetMode(fs.ModeSymlink | 0777)` with the target as the member body — which
is what `app_bundle.go`'s `file.Mode()&os.ModeSymlink` branch reads.

### The test table

`E` = must fail. `K` = must keep working (green before *and* after the fix).
`P` = policy layer preserved.

#### tar — `TestExtractTar_Containment`, driving `extractTarGz`

| # | Case | Entries | Expect | Asserted |
|---|---|---|---|---|
| 1 | `escape/regular-file-through-staged-symlink` | `a`→sym `.`; `b`→sym `a/..`; `b/pwned` file | E | error; nothing outside `dest` changed |
| 2 | `escape/directory-through-staged-symlink` | + `b/pwneddir` dir; `b/pwneddir/f` file | E | error; no dir created outside |
| 3 | `escape/symlink-through-staged-symlink` | + `b/outsidelink`→sym `whatever` | E | error; no symlink created outside |
| 4 | `escape/overwrite-existing-outside-file` | + `b/canary.txt` file `"TAMPERED"` | E | error; `canary.txt` fingerprint unchanged |
| 5 | `escape/chain-to-arbitrary-absolute-path` | `h0`→sym `.`; `h1..hN`→sym `h<i-1>/..` until `/` is reached; then `hN/<abs path>` file | E | error; nothing at the chosen absolute path |
| 6 | `escape/nested-parent-dirs-created-outside-before-write` | staged escape + `b/deep/nested/pwned` file | E | error **and** no `deep/`, `deep/nested/` outside |
| 7 | `escape/final-component-symlink-overwrites-outside-file` | `p`→sym `.`; `d` dir; `d/x`→sym `../p/../canary.txt`; `d/x` file `"TAMPERED"` | E | error; `canary.txt` unchanged |
| 8 | `escape/final-component-symlink-creates-outside-file` | same with `../p/../NEWFILE` | E | error; no `NEWFILE` outside |
| 9 | `legit/write-through-earlier-in-root-symlink` | `lib64` dir; `lib`→sym `lib64`; `lib/foo.so` file | K | success; `dest/lib64/foo.so` == `"ELF"` |
| 10 | `legit/bottle-shallow-sibling` | `libexec/bin/tool` file; `bin/tool`→sym `../libexec/bin/tool` | K | success; `dest/bin/tool` reads `"real"` |
| 11 | `legit/bottle-up-and-back-down-inside-root` | `pkg/1.0/libexec/x` file; `pkg/1.0/bin/x`→sym `../../../pkg/1.0/libexec/x` | K | success; link resolves |
| 12 | `legit/bottle-with-strip-dirs-2` | `python@3.14/3.14.0/libexec/bin/tool` file; `.../bin/tool`→sym `../libexec/bin/tool`, `strip_dirs=2` | K | success; `dest/bin/tool` reads `"real"` |
| 13 | `legit/dangling-symlink-target-created-later` | `bin/tool`→sym `../libexec/tool`; then `libexec/tool` file | K | success; link resolves after the fact |
| 14 | `legit/benign-sibling-relative-symlink` | `a/file` file; `b/link`→sym `../a/file` | K | success; resolves |
| 15 | `legit/dotdot-inside-entry-name-is-cleaned` | `a`→sym `.`; `a/../pwned` file | K | success; lands at `dest/pwned` |
| 16 | `legit/destination-directory-does-not-exist-yet` | `bin/tool` file, `dest` removed first | K | success; extractor creates `dest` |
| 17 | `policy/absolute-symlink-target-rejected` | `link`→sym `/etc/passwd` | P | error |
| 18 | `policy/upward-relative-symlink-target-rejected` | `link`→sym `../../evil` | P | error |
| 19 | `policy/entry-name-traversal-diagnostic` | `../../../etc/passwd` file | P | error mentioning `archive entry escapes destination directory` |

Cases 10–13 are the "#2275 bottles still extract" criterion as it can actually
be satisfied — the population that works today, per the exploration finding that
the exact shape #2275 quotes has never extracted. Say so plainly in the PR.

Six tar compressions share `extractTarReader`, so driving `.tar.gz` covers all
of them. Add one thin case that runs the canonical escape through `extractTar`
(uncompressed) if reviewers want the shared-loop claim asserted rather than
argued.

#### zip via `extract.go` — `TestExtractZip_Containment`, driving `extractZip`

This path never creates symlinks, so the escape must be staged by state already
on disk — a symlink left by an earlier step in the same work dir, which is
reachable whenever a recipe extracts twice into one directory.

| # | Case | Setup / entries | Expect | Asserted |
|---|---|---|---|---|
| 20 | `zip/escape-through-preexisting-symlink` | pre-create `dest/b`→sym `..`; entry `b/pwned` | E | error; nothing outside |
| 21 | `zip/overwrite-existing-outside-file-through-preexisting-symlink` | same; entry `b/canary.txt` | E | error; `canary.txt` unchanged |
| 22 | `zip/nested-parent-dirs-created-outside-before-write` | same; entry `b/deep/nested/pwned` | E | error; no dirs outside |
| 23 | `zip/final-component-symlink-overwrites-outside-file` | pre-create `p`→sym `.`, `d` dir, `d/x`→sym `../p/../canary.txt`; entry `d/x` | E | error; `canary.txt` unchanged |
| 24 | `zip/legit-write-through-preexisting-in-root-symlink` | pre-create `dest/lib64` dir, `dest/lib`→sym `lib64`; entry `lib/foo.so` | K | success; `dest/lib64/foo.so` == `"ELF"` |
| 25 | `zip/plain-nested-file` | entry `dir/sub/file.txt` | K | success |
| 26 | `zip/destination-directory-does-not-exist-yet` | entry `bin/tool`, `dest` removed | K | success |

#### zip via `app_bundle.go` — `TestExtractZIP_Containment`, driving `extractZIP`

This one *does* create symlinks from mode bits, so a single crafted `.app` zip
stages its own escape. `extractZIP` is a package-level function, so it is
callable from a Linux test even though `AppBundleAction.Execute` is darwin-gated.

| # | Case | Entries | Expect | Asserted |
|---|---|---|---|---|
| 27 | `appbundle/escape-through-staged-symlink` | `a`→sym `.`; `b`→sym `a/..`; `b/pwned` file | E | error; nothing outside |
| 28 | `appbundle/symlink-through-staged-symlink` | + `b/outsidelink`→sym `whatever` | E | error; no symlink outside |
| 29 | `appbundle/nested-parent-dirs-created-outside-before-write` | staged escape + `b/deep/nested/pwned` | E | error; no dirs outside |
| 30 | `appbundle/final-component-symlink-overwrites-outside-file` | `p`→sym `.`; `d` dir; `d/x`→sym `../p/../canary.txt`; `d/x` file | E | error; `canary.txt` unchanged |
| 31 | `appbundle/legit-framework-versions-current` | `Foo.app/Contents/Frameworks/Bar.framework/Versions/A` dir; `.../Versions/A/Bar` file; `.../Versions/Current`→sym `A`; `.../Bar`→sym `Versions/Current/Bar`; `.../Versions/Current/Resources` file | K | success; all three resolve to the right content |

Case 31 is the macOS `Versions/Current` idiom — a real symlink-heavy bundle
layout with a later entry written through an earlier in-root symlink. It is the
case that caught the `file.Mode()` / `Perm()` defect.

### Mutation list for the PR body

Inject one at a time, run `go test ./internal/actions/ -run 'Test.*_Containment'`,
confirm the named test goes red, revert.

| # | Defect to inject | Test that must fail |
|---|---|---|
| M1 | `root.OpenFile(relativePath, …)` → `os.OpenFile(target, …)` in the tar `TypeReg` branch | `escape/final-component-symlink-{overwrites,creates}-outside-file` |
| M2 | `atomicSymlinkInRoot(root, …)` → `atomicSymlink(…)` in the tar `TypeSymlink` branch | *(none — see note)* |
| M3 | `root.MkdirAll(relativePath, …)` → `os.MkdirAll(target, …)` in the tar `TypeDir` branch | `escape/directory-through-staged-symlink` |
| M4 | `root.MkdirAll(filepath.Dir(relativePath), …)` → `os.MkdirAll(filepath.Dir(target), …)` in the tar `TypeReg` branch | `escape/nested-parent-dirs-created-outside-before-write` |
| M5 | delete the `os.MkdirAll(destPath, …)` that precedes `os.OpenRoot` in `extractTarReader` | `legit/destination-directory-does-not-exist-yet` |
| M6 | delete the `isPathWithinDirectory(target, destPath)` pre-check in the tar loop | `policy/entry-name-traversal-diagnostic` |
| M7 | delete the `validateSymlinkTarget` call in the tar `TypeSymlink` branch | `policy/absolute-symlink-target-rejected`, `policy/upward-relative-symlink-target-rejected` |
| M8 | `root.OpenFile(relativePath, …)` → `os.OpenFile(target, …)` in `extractZip` | `zip/final-component-symlink-overwrites-outside-file` |
| M9 | `root.MkdirAll(filepath.Dir(relativePath), …)` → `os.MkdirAll(filepath.Dir(target), …)` in `extractZip` | `zip/nested-parent-dirs-created-outside-before-write` |
| M10 | `root.OpenFile(relPath, …)` → `os.OpenFile(targetPath, …)` in `extractZIP` | `appbundle/final-component-symlink-overwrites-outside-file` |
| M11 | `root.Symlink(linkTarget, relPath)` → `os.Symlink(linkTarget, targetPath)` in `extractZIP` | *(none — see note)* |
| M12 | `root.MkdirAll(filepath.Dir(relPath), …)` → `os.MkdirAll(filepath.Dir(targetPath), …)` in `extractZIP` | `appbundle/nested-parent-dirs-created-outside-before-write` |

**Note on M2 and M11.** These survive by construction: `symlink(2)` fails with
`EEXIST` rather than following an existing link, and `rename(2)` replaces a
symlink rather than following it, so an unscoped symlink write cannot escape
once the parent guard is in place. Root-scope them anyway for consistency and
to avoid depending on that semantic — but list them here as *expected
survivors* rather than pretending they are covered. Measured, not assumed.

### Two implementation notes this exercise produced

1. `app_bundle.go:282` must become `root.MkdirAll(relPath, file.Mode().Perm())`.
   `os.Root.MkdirAll` rejects the type bits that `os.MkdirAll` silently masked.
2. Add `- (*os.Root).Close` to `errcheck.exclude-functions` in
   `.golangci.yaml`, beside the existing `(*os.File).Close`. Without it,
   `defer root.Close()` fails lint at three sites.

## Consequences

The suite is roughly 350 lines of test code across three top-level functions
plus about 200 lines of shared fixture and assertion helpers. It runs in 0.07s
and needs no `testing.Short()` gate, no fixture files in `testdata/`, and no
network. `extract_test.go` is already 873 lines, so this roughly doubles it —
worth flagging to reviewers up front so the diff size is not a surprise.

The fingerprint assertion is strictly stronger than what the codebase does
today, and it will occasionally fail for reasons unrelated to containment (a
test that legitimately writes to the sandbox, a helper that leaves a temp file
in the wrong place). That is a feature, but it means the failure message has to
be good — hence the `created:` / `modified:` / `removed:` diff rather than a
bare boolean.

Two guards ship without independent test coverage (M2, M11), and the PR should
say so rather than imply twelve-for-twelve. The honest claim is: ten of twelve
injected defects are caught by a specifically named test, and the other two
cannot escape given the syscall semantics, verified by direct probe.

The mutation list is manual and will rot. It belongs in the PR body as evidence
for reviewers, not in the repo as a maintained artifact — there is no mutation
tooling here to keep it honest, and a stale checklist is worse than none.

Finally, none of this is gated by the CI signals that would normally catch an
extraction regression. `extract.go` is on `validate-golden-code.yml`'s explicit
exclusion list and no Homebrew recipe is in `test-matrix.json`, so these unit
tests are the whole safety net for the bottle population. That argues for
keeping cases 10–13 and 31 even though they pass before and after the fix: they
are regression coverage for a path nothing else covers.

## Summary

The strongest assertion is a pre/post fingerprint of everything under the
destination's *parent* — type plus content hash, diffed — asserted on every
case including the ones expected to succeed; it catches created files,
side-effect directories and silent overwrites that an error-only or
named-path assertion all miss, and it is what makes the parent-directory guard
detectable at all. Prototyped for real: 31 cases across the tar loop, the zip
path and `app_bundle.go`'s `extractZIP`, with 16 escapes red on `main` and green
under `os.OpenRoot` while all 15 compatibility cases stay green throughout, run
inside a two-level `t.TempDir()/sandbox/dest` layout that keeps the escape
target disposable and leaves `git status --porcelain` clean. The mutation
exercise is the load-bearing result: a suite built only from the issue's
reproducer let **ten of twelve** injected defects survive because the guards
mask each other, and only after adding deep-nesting and final-component-escape
shapes did ten of twelve die to a specifically named test — the remaining two
(root-scoping the symlink write) cannot escape given `symlink(2)`/`rename(2)`
semantics, verified by probe, and should be listed as expected survivors rather
than papered over.
