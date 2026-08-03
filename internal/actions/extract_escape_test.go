package actions

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Regression tests for the symlink-chain escape: an archive stages a symlink in
// one entry and a later entry is written through it, landing outside the
// destination. The containment guarantee is that no archive entry can cause a
// write outside destPath, so these tests drive real archives through the real
// extractors and assert against the filesystem rather than against a validation
// helper's return value -- both helpers report "valid" for the escaping input.

// archiveEntry is one member of a synthetic archive, serialized as either tar.gz
// or zip so the same case table can drive every extractor.
type archiveEntry struct {
	name string
	typ  byte // tar.TypeReg, tar.TypeDir, tar.TypeSymlink
	link string
	body string
	mode int64 // 0 means the type's default
}

func reg(name, body string) archiveEntry {
	return archiveEntry{name: name, typ: tar.TypeReg, body: body}
}

// regMode is a regular file carrying an explicit mode, for the archives that
// ship setuid binaries.
func regMode(name, body string, mode int64) archiveEntry {
	return archiveEntry{name: name, typ: tar.TypeReg, body: body, mode: mode}
}

func dir(name string) archiveEntry {
	return archiveEntry{name: name, typ: tar.TypeDir}
}

func sym(name, target string) archiveEntry {
	return archiveEntry{name: name, typ: tar.TypeSymlink, link: target}
}

func buildTarGzEntries(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Typeflag: e.typ, Linkname: e.link, Mode: 0644}
		switch e.typ {
		case tar.TypeDir:
			// Real tar writers store directory names with a trailing slash,
			// which is what makes strip_dirs produce an empty relative path.
			hdr.Name = strings.TrimSuffix(e.name, "/") + "/"
			hdr.Mode = 0755
		case tar.TypeSymlink:
			hdr.Mode = 0777
		case tar.TypeReg:
			hdr.Size = int64(len(e.body))
		}
		if e.mode != 0 {
			hdr.Mode = e.mode
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write tar header %s: %v", e.name, err)
		}
		if e.typ == tar.TypeReg {
			if _, err := tw.Write([]byte(e.body)); err != nil {
				t.Fatalf("write tar body %s: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return buf.Bytes()
}

// buildZipEntries encodes symlinks the way real zip tooling does: the mode
// carries fs.ModeSymlink and the member body holds the link target. That is
// what app_bundle.go's extractZIP reads.
func buildZipEntries(t *testing.T, entries []archiveEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.name, Method: zip.Deflate}
		body := e.body
		switch e.typ {
		case tar.TypeDir:
			hdr.Name = strings.TrimSuffix(e.name, "/") + "/"
			hdr.SetMode(fs.ModeDir | 0755)
		case tar.TypeSymlink:
			hdr.SetMode(fs.ModeSymlink | 0777)
			body = e.link
		default:
			hdr.SetMode(0644)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("create zip header %s: %v", e.name, err)
		}
		if e.typ != tar.TypeDir {
			if _, err := w.Write([]byte(body)); err != nil {
				t.Fatalf("write zip body %s: %v", e.name, err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}

// fingerprintOutside records every filesystem entry under sandbox that is not
// inside dest, keyed by path and valued by a type-and-content fingerprint.
// Comparing two fingerprints catches creation, deletion AND in-place overwrite
// of anything outside the extraction destination -- an error-only assertion
// would miss the side-effect directories and silent overwrites that several of
// these cases turn on.
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

// newSandbox builds the two-level layout the escape cases need:
//
//	<t.TempDir()>/          archive lives here
//	  sandbox/              the "outside" the guard must protect
//	    canary.txt          pre-existing file an escape could overwrite
//	    dest/               destPath handed to the extractor
//
// Keeping "outside" inside t.TempDir() matters: when the guard is absent these
// cases write outside dest by construction, and CI fails the run if the working
// tree is dirty afterwards.
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

type extractCase struct {
	name string
	// build returns the archive members. sandbox and dest are passed so a case
	// can compute how many hops it needs to reach a chosen absolute path.
	build func(t *testing.T, sandbox, dest string) []archiveEntry
	// setup plants state in dest before extraction, for the zip cases where the
	// extractor cannot create symlinks itself.
	setup     func(t *testing.T, dest string)
	stripDirs int
	wantErr   bool
	// wantErrContains pins the diagnostic, not merely the failure -- it is what
	// makes the lexical pre-filter's contribution testable.
	wantErrContains string
	// destAbsent removes dest before extraction, so the extractor must create it.
	destAbsent bool
	// wantInside reads through any symlinks the archive shipped, so the
	// compatibility cases assert the link resolves to the right content rather
	// than merely that a link exists.
	wantInside map[string]string
}

func runExtractCase(
	t *testing.T,
	tc extractCase,
	ext string,
	build func(*testing.T, []archiveEntry) []byte,
	extract func(archivePath, destPath string, stripDirs int) error,
) {
	t.Helper()
	archiveDir, sandbox, dest := newSandbox(t)
	if tc.destAbsent {
		if err := os.RemoveAll(dest); err != nil {
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

// stagedEscape is the canonical chain: "a" points at the destination, "b" walks
// through it and up one level, so anything written under "b/" lands in dest's
// parent. Every entry passes both lexical checks.
func stagedEscape(extra ...archiveEntry) []archiveEntry {
	return append([]archiveEntry{sym("a", "."), sym("b", "a/..")}, extra...)
}

// --- tar ---

func TestExtractTarGz_SymlinkChainEscape(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}

	tests := []extractCase{
		{
			name: "escape/regular-file-through-staged-symlink",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return stagedEscape(reg("b/pwned", "owned"))
			},
			wantErr: true,
		},
		{
			name: "escape/directory-through-staged-symlink",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return stagedEscape(dir("b/pwneddir"), reg("b/pwneddir/f", "owned"))
			},
			wantErr: true,
		},
		{
			name: "escape/symlink-through-staged-symlink",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return stagedEscape(sym("b/outsidelink", "whatever"))
			},
			wantErr: true,
		},
		{
			name: "escape/overwrite-existing-outside-file",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return stagedEscape(reg("b/canary.txt", "TAMPERED"))
			},
			wantErr: true,
		},
		{
			name: "escape/nested-parent-dirs-created-outside-before-write",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return stagedEscape(reg("b/deep/nested/pwned", "owned"))
			},
			wantErr: true,
		},
		{
			// Lengthening the chain walks up one level per hop, so a long
			// enough chain reaches an arbitrary absolute path.
			name: "escape/chain-to-arbitrary-absolute-path",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				hops := strings.Count(filepath.Clean(dest), string(os.PathSeparator)) + 1
				entries := []archiveEntry{sym("h0", ".")}
				for i := 1; i <= hops; i++ {
					entries = append(entries, sym(fmt.Sprintf("h%d", i), fmt.Sprintf("h%d/..", i-1)))
				}
				target := filepath.Join(fmt.Sprintf("h%d", hops), filepath.Join(sandbox, "abs-pwned"))
				return append(entries, reg(target, "owned"))
			},
			wantErr: true,
		},
		{
			// The escaping component is the final one, so a guard that only
			// checks parent directories misses it.
			name: "escape/final-component-symlink-overwrites-outside-file",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{
					sym("p", "."),
					dir("d"),
					sym("d/x", "../p/../canary.txt"),
					reg("d/x", "TAMPERED"),
				}
			},
			wantErr: true,
		},
		{
			name: "escape/final-component-symlink-creates-outside-file",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{
					sym("p", "."),
					dir("d"),
					sym("d/x", "../p/../NEWFILE"),
					reg("d/x", "owned"),
				}
			},
			wantErr: true,
		},
		{
			// The case an over-strict fix breaks: a later entry written through
			// an earlier symlink that stays inside the destination.
			name: "legit/write-through-earlier-in-root-symlink",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{dir("lib64"), sym("lib", "lib64"), reg("lib/foo.so", "ELF")}
			},
			wantInside: map[string]string{"lib64/foo.so": "ELF"},
		},
		{
			name: "legit/bottle-shallow-sibling",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{
					reg("libexec/bin/tool", "real"),
					sym("bin/tool", "../libexec/bin/tool"),
				}
			},
			wantInside: map[string]string{"bin/tool": "real"},
		},
		{
			name: "legit/bottle-up-and-back-down-inside-root",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{
					reg("pkg/1.0/libexec/x", "real"),
					sym("pkg/1.0/bin/x", "../../../pkg/1.0/libexec/x"),
				}
			},
			wantInside: map[string]string{"pkg/1.0/bin/x": "real"},
		},
		{
			// strip_dirs makes the top-level entry's relative path empty, which
			// is the shape every Homebrew bottle and node tarball produces.
			name: "legit/bottle-with-strip-dirs-2",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{
					dir("python@3.14"),
					dir("python@3.14/3.14.0"),
					reg("python@3.14/3.14.0/libexec/bin/tool", "real"),
					sym("python@3.14/3.14.0/bin/tool", "../libexec/bin/tool"),
				}
			},
			stripDirs:  2,
			wantInside: map[string]string{"bin/tool": "real"},
		},
		{
			name: "legit/dangling-symlink-target-created-later",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{
					sym("bin/tool", "../libexec/tool"),
					reg("libexec/tool", "real"),
				}
			},
			wantInside: map[string]string{"bin/tool": "real"},
		},
		{
			name: "legit/benign-sibling-relative-symlink",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{reg("a/file", "content"), sym("b/link", "../a/file")}
			},
			wantInside: map[string]string{"b/link": "content"},
		},
		{
			name: "legit/destination-directory-does-not-exist-yet",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{reg("bin/tool", "real")}
			},
			destAbsent: true,
			wantInside: map[string]string{"bin/tool": "real"},
		},
		{
			name: "policy/absolute-symlink-target-rejected",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{sym("link", "/etc/passwd")}
			},
			wantErr: true,
		},
		{
			name: "policy/upward-relative-symlink-target-rejected",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{sym("link", "../../evil")}
			},
			wantErr: true,
		},
		{
			// The root would refuse this too, but the pre-filter is what names
			// the offending entry, so pin the message rather than the failure.
			name: "policy/entry-name-traversal-rejected",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{reg("../../../etc/pwned", "owned")}
			},
			wantErr:         true,
			wantErrContains: "archive entry escapes destination directory",
		},
		{
			// Tar stores setuid in the unix layout, which the root rejects as an
			// unsupported file mode unless the permission bits are masked out.
			name: "legit/setuid-binary-mode-is-accepted",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{regMode("bin/sudo-like", "ELF", 04755)}
			},
			wantInside: map[string]string{"bin/sudo-like": "ELF"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runExtractCase(t, tc, ".tar.gz", buildTarGzEntries,
				func(archivePath, destPath string, stripDirs int) error {
					return action.extractTarGz(archivePath, destPath, stripDirs, nil)
				})
		})
	}
}

// --- zip (extract.go) ---
//
// extractZip never creates symlinks, so the escape cases plant the staged
// symlink in the destination first and check the extractor refuses to write
// through it.

func plantStagedEscape(t *testing.T, dest string) {
	t.Helper()
	if err := os.MkdirAll(dest, 0755); err != nil {
		t.Fatalf("mkdir dest: %v", err)
	}
	if err := os.Symlink("..", filepath.Join(dest, "b")); err != nil {
		t.Fatalf("plant symlink: %v", err)
	}
}

func plantFinalComponentEscape(t *testing.T, dest string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dest, "d"), 0755); err != nil {
		t.Fatalf("mkdir d: %v", err)
	}
	if err := os.Symlink(".", filepath.Join(dest, "p")); err != nil {
		t.Fatalf("plant p: %v", err)
	}
	if err := os.Symlink("../p/../canary.txt", filepath.Join(dest, "d", "x")); err != nil {
		t.Fatalf("plant d/x: %v", err)
	}
}

func plantInRootSymlink(t *testing.T, dest string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dest, "lib64"), 0755); err != nil {
		t.Fatalf("mkdir lib64: %v", err)
	}
	if err := os.Symlink("lib64", filepath.Join(dest, "lib")); err != nil {
		t.Fatalf("plant lib: %v", err)
	}
}

func zipEscapeCases() []extractCase {
	return []extractCase{
		{
			name:  "zip/escape-through-preexisting-symlink",
			setup: plantStagedEscape,
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{reg("b/pwned", "owned")}
			},
			wantErr: true,
		},
		{
			name:  "zip/overwrite-existing-outside-file-through-preexisting-symlink",
			setup: plantStagedEscape,
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{reg("b/canary.txt", "TAMPERED")}
			},
			wantErr: true,
		},
		{
			name:  "zip/nested-parent-dirs-created-outside-before-write",
			setup: plantStagedEscape,
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{reg("b/deep/nested/pwned", "owned")}
			},
			wantErr: true,
		},
		{
			name:  "zip/final-component-symlink-overwrites-outside-file",
			setup: plantFinalComponentEscape,
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{reg("d/x", "TAMPERED")}
			},
			wantErr: true,
		},
		{
			name:  "zip/legit-write-through-preexisting-in-root-symlink",
			setup: plantInRootSymlink,
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{reg("lib/foo.so", "ELF")}
			},
			wantInside: map[string]string{"lib64/foo.so": "ELF"},
		},
		{
			name: "zip/plain-nested-file",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{reg("dir/sub/file.txt", "content")}
			},
			wantInside: map[string]string{"dir/sub/file.txt": "content"},
		},
		{
			name: "zip/destination-directory-does-not-exist-yet",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{reg("bin/tool", "real")}
			},
			destAbsent: true,
			wantInside: map[string]string{"bin/tool": "real"},
		},
	}
}

func TestExtractZip_SymlinkChainEscape(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}
	for _, tc := range zipEscapeCases() {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runExtractCase(t, tc, ".zip", buildZipEntries,
				func(archivePath, destPath string, stripDirs int) error {
					return action.extractZip(archivePath, destPath, stripDirs, nil)
				})
		})
	}
}

// --- zip (app_bundle.go) ---
//
// extractZIP does create symlinks from zip entries, so it carries the staged
// chain in the archive itself, and it is the path macOS .app bundles take.

func TestExtractZIP_AppBundle_SymlinkChainEscape(t *testing.T) {
	t.Parallel()

	tests := []extractCase{
		{
			name: "appbundle/escape-through-staged-symlink",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return stagedEscape(reg("b/pwned", "owned"))
			},
			wantErr: true,
		},
		{
			name: "appbundle/symlink-through-staged-symlink",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return stagedEscape(sym("b/outsidelink", "whatever"))
			},
			wantErr: true,
		},
		{
			name: "appbundle/nested-parent-dirs-created-outside-before-write",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return stagedEscape(reg("b/deep/nested/pwned", "owned"))
			},
			wantErr: true,
		},
		{
			name: "appbundle/final-component-symlink-overwrites-outside-file",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				return []archiveEntry{
					sym("p", "."),
					dir("d"),
					sym("d/x", "../p/../canary.txt"),
					reg("d/x", "TAMPERED"),
				}
			},
			wantErr: true,
		},
		{
			// A real .app bundle: Versions/Current is a symlink and the
			// framework's top-level entries resolve through it.
			name: "appbundle/legit-framework-versions-current",
			build: func(t *testing.T, sandbox, dest string) []archiveEntry {
				const fw = "Foo.app/Contents/Frameworks/Bar.framework"
				return []archiveEntry{
					dir(fw + "/Versions"),
					dir(fw + "/Versions/A"),
					reg(fw+"/Versions/A/Bar", "BINARY"),
					reg(fw+"/Versions/A/Resources", "RES"),
					sym(fw+"/Versions/Current", "A"),
					sym(fw+"/Bar", "Versions/Current/Bar"),
					sym(fw+"/Resources", "Versions/Current/Resources"),
				}
			},
			wantInside: map[string]string{
				"Foo.app/Contents/Frameworks/Bar.framework/Bar":       "BINARY",
				"Foo.app/Contents/Frameworks/Bar.framework/Resources": "RES",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			runExtractCase(t, tc, ".zip", buildZipEntries,
				func(archivePath, destPath string, _ int) error {
					return extractZIP(archivePath, destPath)
				})
		})
	}
}

// TestExtractTarGz_DestinationPathIsAttackerPlantedSymlink covers the two-archive
// variant: one archive plants a symlink inside the work directory, and a later
// extract step whose dest names that symlink would otherwise be anchored outside.
// The recipe here is entirely innocent -- it just extracts into a subdirectory.
func TestExtractTarGz_DestinationPathIsAttackerPlantedSymlink(t *testing.T) {
	t.Parallel()
	action := &ExtractAction{}

	root := t.TempDir()
	work := filepath.Join(root, "work")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(work, 0755); err != nil {
		t.Fatalf("mkdir work: %v", err)
	}
	if err := os.MkdirAll(outside, 0755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	// Archive #1 plants the symlink. Creating it is allowed: link targets are
	// policy, and this one is rejected by the policy layer only because it is
	// upward-relative, so plant it directly to isolate the destination question.
	if err := os.Symlink("../outside", filepath.Join(work, "sub")); err != nil {
		t.Fatalf("plant sub: %v", err)
	}

	archive := filepath.Join(root, "second.tar.gz")
	if err := os.WriteFile(archive, buildTarGzEntries(t, []archiveEntry{reg("planted", "owned")}), 0644); err != nil {
		t.Fatalf("write archive: %v", err)
	}

	// Archive #2 extracts into work/sub, which resolves outside work.
	err := action.extractTarGzInWorkDir(work, "sub", archive, 0, nil)
	if err == nil {
		t.Error("expected extraction into a symlinked destination to fail, got nil")
	}
	if _, statErr := os.Lstat(filepath.Join(outside, "planted")); statErr == nil {
		t.Error("extraction wrote through a symlinked destination into the parent directory")
	}
}
