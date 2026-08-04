package shellenv

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// precedenceFixture lays out a $TSUKU_HOME and a competing prefix under one
// temp root, so a test can build a PATH without naming anything real.
type precedenceFixture struct {
	root       string
	tsukuHome  string
	binDir     string
	currentDir string
	otherDir   string
}

func newPrecedenceFixture(t *testing.T) precedenceFixture {
	t.Helper()

	root := t.TempDir()
	f := precedenceFixture{
		root:       root,
		tsukuHome:  filepath.Join(root, "tsuku"),
		binDir:     filepath.Join(root, "tsuku", "bin"),
		currentDir: filepath.Join(root, "tsuku", "tools", "current"),
		otherDir:   filepath.Join(root, "other", "bin"),
	}
	for _, dir := range []string{f.binDir, f.currentDir, f.otherDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll(%s): %v", dir, err)
		}
	}
	return f
}

// dir creates a directory under the fixture root and returns it.
func (f precedenceFixture) dir(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{f.root}, parts...)...)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("MkdirAll(%s): %v", path, err)
	}
	return path
}

// installDir creates a tool's own bin directory under $TSUKU_HOME/tools, the
// shape a real install leaves: tools/<name>-<version>/bin.
func (f precedenceFixture) installDir(t *testing.T, nameVersion string) string {
	t.Helper()
	return f.dir(t, "tsuku", "tools", nameVersion, "bin")
}

func (f precedenceFixture) symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("Symlink(%s -> %s): %v", link, target, err)
	}
}

// writeExecutable creates a runnable file. Windows has no execute bit, so the
// mode is advisory there and existence is what counts.
func writeExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func writeNonExecutable(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("not runnable\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(%s): %v", path, err)
	}
	return path
}

func TestCheckPrecedence(t *testing.T) {
	tests := []struct {
		name string
		// setup places binaries and returns the PATH dirs in order plus the
		// tools to check.
		setup func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries)
		// wantShadowed lists the binary names expected in the result, in order.
		wantShadowed []string
		// wantResolvedIn and wantExpectedIn name the fixture directories the
		// single expected finding should point at. Empty means unchecked.
		wantResolvedIn func(f precedenceFixture) string
		wantExpectedIn func(f precedenceFixture) string
	}{
		{
			name: "shadow ahead of tools/current is reported",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.otherDir, "koto")
				writeExecutable(t, f.currentDir, "koto")
				return []string{f.otherDir, f.binDir, f.currentDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
			wantShadowed:   []string{"koto"},
			wantResolvedIn: func(f precedenceFixture) string { return f.otherDir },
			wantExpectedIn: func(f precedenceFixture) string { return f.currentDir },
		},
		{
			name: "shadow ahead of bin is reported",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.otherDir, "rg")
				writeExecutable(t, f.binDir, "rg")
				return []string{f.otherDir, f.binDir, f.currentDir},
					[]ManagedBinaries{{Tool: "ripgrep", Binaries: []string{"rg"}}}
			},
			wantShadowed:   []string{"rg"},
			wantResolvedIn: func(f precedenceFixture) string { return f.otherDir },
			wantExpectedIn: func(f precedenceFixture) string { return f.binDir },
		},
		{
			name: "tsuku's copy first is not reported",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.currentDir, "koto")
				writeExecutable(t, f.otherDir, "koto")
				return []string{f.binDir, f.currentDir, f.otherDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
		},
		{
			name: "a shim in bin winning over tools/current is not a conflict",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.binDir, "rg")
				writeExecutable(t, f.currentDir, "rg")
				return []string{f.binDir, f.currentDir, f.otherDir},
					[]ManagedBinaries{{Tool: "ripgrep", Binaries: []string{"rg"}}}
			},
		},
		{
			name: "a shim with no tools/current copy is not a conflict",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.binDir, "rg")
				return []string{f.binDir, f.currentDir, f.otherDir},
					[]ManagedBinaries{{Tool: "ripgrep", Binaries: []string{"rg"}}}
			},
		},
		{
			// Found by running doctor against a live machine: a tool's own
			// install directory lands on PATH ahead of tools/current whenever a
			// recipe exports it or tsuku runs the tool with its runtime
			// dependencies. The binary reached is tsuku's, for the active
			// version. Reporting it was a false positive on every such tool.
			name: "a tool's own install directory ahead of tools/current is not a shadow",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				installBin := filepath.Join(f.tsukuHome, "tools", "socat-1.8.1.3", "bin")
				if err := os.MkdirAll(installBin, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				writeExecutable(t, installBin, "socat")
				writeExecutable(t, f.currentDir, "socat")
				return []string{installBin, f.binDir, f.currentDir, f.otherDir},
					[]ManagedBinaries{{Tool: "socat", Binaries: []string{"socat"}}}
			},
		},
		{
			// Linking managed binaries into a directory already on PATH is a
			// normal alternative to putting $TSUKU_HOME/bin on it. The name is
			// outside $TSUKU_HOME; the file it runs is tsuku's own.
			name: "a link from outside into tools/current is not a shadow",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				versionBin := f.installDir(t, "ripgrep-14.1.0")
				writeExecutable(t, versionBin, "rg")
				f.symlink(t, filepath.Join(versionBin, "rg"), filepath.Join(f.currentDir, "rg"))

				localBin := f.dir(t, "home", ".local", "bin")
				f.symlink(t, filepath.Join(f.currentDir, "rg"), filepath.Join(localBin, "rg"))

				return []string{localBin, f.binDir, f.currentDir},
					[]ManagedBinaries{{Tool: "ripgrep", Binaries: []string{"rg"}}}
			},
		},
		{
			// Aimed straight at a version directory rather than through
			// tools/current. Still tsuku's file, so still not a foreign shadow.
			// Whether such a link pins a version that stops tracking updates is
			// a separate diagnostic; the same exposure exists for a PATH entry
			// naming a version directory outright, which this check allows.
			name: "a link from outside into a version directory is not a shadow",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				versionBin := f.installDir(t, "ripgrep-14.1.0")
				writeExecutable(t, versionBin, "rg")
				writeExecutable(t, f.currentDir, "rg")

				localBin := f.dir(t, "home", ".local", "bin")
				f.symlink(t, filepath.Join(versionBin, "rg"), filepath.Join(localBin, "rg"))

				return []string{localBin, f.binDir, f.currentDir},
					[]ManagedBinaries{{Tool: "ripgrep", Binaries: []string{"rg"}}}
			},
		},
		{
			// The guard against over-resolving: following links must not turn a
			// competing installer's own binary into tsuku's.
			name: "a link to a binary outside the home is still a shadow",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				foreign := f.dir(t, "koto-install", "bin")
				writeExecutable(t, foreign, "koto")
				writeExecutable(t, f.currentDir, "koto")

				localBin := f.dir(t, "home", ".local", "bin")
				f.symlink(t, filepath.Join(foreign, "koto"), filepath.Join(localBin, "koto"))

				return []string{localBin, f.binDir, f.currentDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
			wantShadowed: []string{"koto"},
		},
		{
			// The trailing-separator case: a directory whose name merely starts
			// with $TSUKU_HOME is not inside it.
			name: "a sibling directory sharing the home's prefix is a shadow",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				lookalike := f.tsukuHome + "-backup"
				if err := os.MkdirAll(lookalike, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				writeExecutable(t, lookalike, "koto")
				writeExecutable(t, f.currentDir, "koto")
				return []string{lookalike, f.binDir, f.currentDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
			wantShadowed: []string{"koto"},
		},
		{
			name: "a non-executable file earlier on PATH does not shadow",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeNonExecutable(t, f.otherDir, "koto")
				writeExecutable(t, f.currentDir, "koto")
				return []string{f.otherDir, f.binDir, f.currentDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
		},
		{
			name: "a directory earlier on PATH does not shadow",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				if err := os.MkdirAll(filepath.Join(f.otherDir, "koto"), 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				writeExecutable(t, f.currentDir, "koto")
				return []string{f.otherDir, f.binDir, f.currentDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
		},
		{
			name: "a binary that resolves nowhere is not reported",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				return []string{f.binDir, f.currentDir, f.otherDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
		},
		{
			name: "a binary tsuku has no copy of is not reported",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.otherDir, "koto")
				return []string{f.otherDir, f.binDir, f.currentDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
		},
		{
			name: "an uncleaned PATH entry naming a tsuku directory is not a shadow",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.currentDir, "koto")
				// Built by concatenation rather than filepath.Join, which would
				// clean the ".." away before the check ever sees it.
				sep := string(filepath.Separator)
				uncleaned := f.binDir + sep + ".." + sep + "tools" + sep + "current"
				return []string{uncleaned, f.otherDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
		},
		{
			name: "a shadow later on PATH than tsuku's copy is not reported",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.currentDir, "koto")
				writeExecutable(t, f.otherDir, "koto")
				return []string{f.currentDir, f.otherDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
		},
		{
			name: "a binary named differently from its tool is reported under the tool",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.otherDir, "rg")
				writeExecutable(t, f.currentDir, "rg")
				return []string{f.otherDir, f.currentDir},
					[]ManagedBinaries{{Tool: "ripgrep", Binaries: []string{"rg"}}}
			},
			wantShadowed:   []string{"rg"},
			wantResolvedIn: func(f precedenceFixture) string { return f.otherDir },
			wantExpectedIn: func(f precedenceFixture) string { return f.currentDir },
		},
		{
			name: "an empty PATH entry is skipped rather than treated as the working directory",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				// A shell reads an empty PATH entry as the working directory.
				// Honoring that would make the result depend on where doctor
				// was run, so chdir somewhere holding a competing binary and
				// assert it is not picked up.
				cwd := filepath.Join(f.root, "cwd")
				if err := os.MkdirAll(cwd, 0o755); err != nil {
					t.Fatalf("MkdirAll: %v", err)
				}
				writeExecutable(t, cwd, "koto")
				t.Chdir(cwd)

				writeExecutable(t, f.currentDir, "koto")
				return []string{"", f.currentDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}}
			},
		},
		{
			name: "each of a tool's binaries is checked",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.currentDir, "git")
				writeExecutable(t, f.currentDir, "git-lfs")
				writeExecutable(t, f.otherDir, "git-lfs")
				return []string{f.otherDir, f.currentDir},
					[]ManagedBinaries{{Tool: "git", Binaries: []string{"git", "git-lfs"}}}
			},
			wantShadowed: []string{"git-lfs"},
		},
		{
			name: "a binary two tools both claim is reported once",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				writeExecutable(t, f.otherDir, "node")
				writeExecutable(t, f.currentDir, "node")
				return []string{f.otherDir, f.currentDir}, []ManagedBinaries{
					{Tool: "node", Binaries: []string{"node"}},
					{Tool: "nodejs", Binaries: []string{"node"}},
				}
			},
			wantShadowed: []string{"node"},
		},
		{
			name: "an empty binary name is skipped",
			setup: func(t *testing.T, f precedenceFixture) ([]string, []ManagedBinaries) {
				return []string{f.otherDir, f.currentDir},
					[]ManagedBinaries{{Tool: "koto", Binaries: []string{""}}}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if runtime.GOOS == "windows" {
				t.Skip("execute-bit resolution differs on Windows")
			}

			f := newPrecedenceFixture(t)
			pathDirs, tools := tt.setup(t, f)

			got := CheckPrecedence(PrecedenceInput{
				TsukuHome: f.tsukuHome,
				PathDirs:  pathDirs,
				Tools:     tools,
			})

			if len(got) != len(tt.wantShadowed) {
				t.Fatalf("CheckPrecedence returned %d findings %+v, want %d %v",
					len(got), got, len(tt.wantShadowed), tt.wantShadowed)
			}
			for i, want := range tt.wantShadowed {
				if got[i].Binary != want {
					t.Errorf("finding %d: Binary = %q, want %q", i, got[i].Binary, want)
				}
			}
			if len(got) != 1 {
				return
			}
			if tt.wantResolvedIn != nil {
				if dir := filepath.Dir(got[0].Resolved); dir != tt.wantResolvedIn(f) {
					t.Errorf("Resolved = %q, want a path in %q", got[0].Resolved, tt.wantResolvedIn(f))
				}
			}
			if tt.wantExpectedIn != nil {
				if dir := filepath.Dir(got[0].Expected); dir != tt.wantExpectedIn(f) {
					t.Errorf("Expected = %q, want a path in %q", got[0].Expected, tt.wantExpectedIn(f))
				}
			}
		})
	}
}

// TestCheckPrecedenceAttributesToolAndPaths pins the content of a finding. The
// resolved path is the thing a user cannot easily discover on their own, so an
// assertion that merely counts findings would not be pinning what matters.
func TestCheckPrecedenceAttributesToolAndPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute-bit resolution differs on Windows")
	}

	f := newPrecedenceFixture(t)
	shadowPath := writeExecutable(t, f.otherDir, "koto")
	tsukuPath := writeExecutable(t, f.currentDir, "koto")

	got := CheckPrecedence(PrecedenceInput{
		TsukuHome: f.tsukuHome,
		PathDirs:  []string{f.otherDir, f.binDir, f.currentDir},
		Tools:     []ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}},
	})

	if len(got) != 1 {
		t.Fatalf("CheckPrecedence returned %d findings, want 1: %+v", len(got), got)
	}
	if got[0].Tool != "koto" {
		t.Errorf("Tool = %q, want %q", got[0].Tool, "koto")
	}
	if got[0].Resolved != shadowPath {
		t.Errorf("Resolved = %q, want %q", got[0].Resolved, shadowPath)
	}
	if got[0].Expected != tsukuPath {
		t.Errorf("Expected = %q, want %q", got[0].Expected, tsukuPath)
	}
}

// TestWithinHomeUnresolvablePath pins the error branch directly, because
// CheckPrecedence cannot reach it: resolveOnPath only hands over paths that
// isExecutableFile already stat'd, so a dangling link never gets this far. A
// path that cannot be resolved has not been shown to be tsuku's, and saying it
// is would silence a real shadow.
func TestWithinHomeUnresolvablePath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink resolution differs on Windows")
	}

	root := t.TempDir()
	home := filepath.Join(root, "tsuku")
	if err := os.MkdirAll(filepath.Join(home, "bin"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	dangling := filepath.Join(root, "elsewhere", "rg")
	if err := os.MkdirAll(filepath.Dir(dangling), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.Symlink(filepath.Join(root, "gone", "rg"), dangling); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	if withinHome(home, dangling) {
		t.Error("a path that cannot be resolved must not be claimed as tsuku's")
	}
}

// TestCheckPrecedenceSymlinkedHome covers a $TSUKU_HOME reached through a
// symlinked parent. PATH names the real location, the configured home names the
// link, and nothing matches unless the home is resolved too -- which would warn
// on every managed tool at once.
func TestCheckPrecedenceSymlinkedHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute-bit resolution differs on Windows")
	}

	root := t.TempDir()
	realParent := filepath.Join(root, "real")
	realCurrent := filepath.Join(realParent, "tsuku", "tools", "current")
	if err := os.MkdirAll(realCurrent, 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(realCurrent, "koto"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	linkedParent := filepath.Join(root, "linked")
	if err := os.Symlink(realParent, linkedParent); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	got := CheckPrecedence(PrecedenceInput{
		TsukuHome: filepath.Join(linkedParent, "tsuku"),
		PathDirs:  []string{realCurrent},
		Tools:     []ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}},
	})

	if len(got) != 0 {
		t.Errorf("a home reached through a symlinked parent must recognize its own files, got %+v", got)
	}
}

// TestCheckPrecedenceRelativeTsukuHome pins the absolutization of the owned
// directories. A relative home compared against the absolute path a PATH walk
// produces matches nothing, which would turn every managed binary into a
// reported shadow -- the loudest possible false positive.
func TestCheckPrecedenceRelativeTsukuHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute-bit resolution differs on Windows")
	}

	f := newPrecedenceFixture(t)
	writeExecutable(t, f.currentDir, "koto")
	t.Chdir(f.root)

	got := CheckPrecedence(PrecedenceInput{
		TsukuHome: "tsuku", // relative to the working directory
		PathDirs:  []string{f.currentDir},
		Tools:     []ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}},
	})

	if len(got) != 0 {
		t.Errorf("a relative TsukuHome must still recognize its own directories, got %+v", got)
	}
}

// TestCheckPrecedenceWithoutTsukuHome guards the zero value. An empty home
// would otherwise absolutize to the working directory, so doctor would compare
// managed binaries against a "bin" and "tools/current" that happen to sit
// wherever it was run from.
func TestCheckPrecedenceWithoutTsukuHome(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("execute-bit resolution differs on Windows")
	}

	f := newPrecedenceFixture(t)
	writeExecutable(t, f.otherDir, "koto")

	// A working directory that looks like a tsuku home, so the guard is what
	// stops the check rather than the absence of a copy to compare against.
	decoy := filepath.Join(f.root, "decoy")
	if err := os.MkdirAll(filepath.Join(decoy, "bin"), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	writeExecutable(t, filepath.Join(decoy, "bin"), "koto")
	t.Chdir(decoy)

	got := CheckPrecedence(PrecedenceInput{
		PathDirs: []string{f.otherDir},
		Tools:    []ManagedBinaries{{Tool: "koto", Binaries: []string{"koto"}}},
	})

	if got != nil {
		t.Errorf("CheckPrecedence with no TsukuHome returned %+v, want nil", got)
	}
}
