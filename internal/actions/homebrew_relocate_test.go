package actions

import (
	"debug/elf"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/progress"
)

// -- homebrew_relocate.go: Dependencies, extractBottlePrefixes --

func TestHomebrewRelocateAction_Dependencies(t *testing.T) {
	t.Parallel()
	action := HomebrewRelocateAction{}
	deps := action.Dependencies()
	if len(deps.LinuxInstallTime) != 1 || deps.LinuxInstallTime[0] != "patchelf" {
		t.Errorf("Dependencies().LinuxInstallTime = %v, want [patchelf]", deps.LinuxInstallTime)
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	content := []byte(`some text /tmp/action-validator-abc12345/.install/libyaml/0.2.5/lib/libyaml.so more text
another line /tmp/action-validator-abc12345/.install/libyaml/0.2.5/include/yaml.h end`)

	prefixMap := make(map[string]string)
	action.extractBottlePrefixes(content, prefixMap)

	if len(prefixMap) != 2 {
		t.Errorf("extractBottlePrefixes() found %d entries, want 2", len(prefixMap))
	}

	expectedPrefix := "/tmp/action-validator-abc12345/.install/libyaml/0.2.5"
	for fullPath, prefix := range prefixMap {
		if prefix != expectedPrefix {
			t.Errorf("prefix for %q = %q, want %q", fullPath, prefix, expectedPrefix)
		}
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes_NoMatch(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}
	prefixMap := make(map[string]string)
	action.extractBottlePrefixes([]byte("no bottle paths here"), prefixMap)
	if len(prefixMap) != 0 {
		t.Errorf("extractBottlePrefixes() found %d entries for no-match content, want 0", len(prefixMap))
	}
}

func TestHomebrewRelocateAction_ExtractBottlePrefixes_NoInstallSegment(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}
	// Has the marker but no /.install/ segment
	content := []byte("/tmp/action-validator-abc12345/other/path")
	prefixMap := make(map[string]string)
	action.extractBottlePrefixes(content, prefixMap)
	if len(prefixMap) != 0 {
		t.Errorf("extractBottlePrefixes() found %d entries for no-install content, want 0", len(prefixMap))
	}
}

// -- findPatchelf discovery tests --

func TestFindPatchelf_ExecPaths(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	// Create a temporary bin dir with a fake patchelf
	tmpDir := t.TempDir()
	binDir := filepath.Join(tmpDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakePatchelf := filepath.Join(binDir, "patchelf")
	if err := os.WriteFile(fakePatchelf, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		ExecPaths: []string{binDir},
	}

	got, err := action.findPatchelf(ctx)
	if err != nil {
		t.Fatalf("findPatchelf() returned error: %v", err)
	}
	if got != fakePatchelf {
		t.Errorf("findPatchelf() = %q, want %q", got, fakePatchelf)
	}
}

func TestFindPatchelf_NotFound_ReturnsError(t *testing.T) {
	// Cannot use t.Parallel() with t.Setenv()
	action := &HomebrewRelocateAction{}

	// Override PATH so system patchelf isn't found
	t.Setenv("PATH", t.TempDir())

	tmpDir := t.TempDir()
	ctx := &ExecutionContext{
		ToolsDir:   filepath.Join(tmpDir, "tools"),
		CurrentDir: filepath.Join(tmpDir, "tools", "current"),
	}

	_, err := action.findPatchelf(ctx)
	if err == nil {
		t.Fatal("findPatchelf() should return error when patchelf not found anywhere")
	}
	if !strings.Contains(err.Error(), "patchelf not found") {
		t.Errorf("error message = %q, want it to contain 'patchelf not found'", err.Error())
	}
}

// -- findPatchelfInToolsDir tests (test glob/current fallback directly) --

func TestFindPatchelfInToolsDir_Glob(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	// Simulate $TSUKU_HOME/tools/patchelf-0.18.0/bin/patchelf
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	patchelfBinDir := filepath.Join(toolsDir, "patchelf-0.18.0", "bin")
	if err := os.MkdirAll(patchelfBinDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakePatchelf := filepath.Join(patchelfBinDir, "patchelf")
	if err := os.WriteFile(fakePatchelf, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := action.findPatchelfInToolsDir(toolsDir, filepath.Join(toolsDir, "current"))
	if err != nil {
		t.Fatalf("findPatchelfInToolsDir() returned error: %v", err)
	}
	if got != fakePatchelf {
		t.Errorf("findPatchelfInToolsDir() = %q, want %q", got, fakePatchelf)
	}
}

func TestFindPatchelfInToolsDir_CurrentDir(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	// Simulate $TSUKU_HOME/tools/current/patchelf (no versioned dir)
	tmpDir := t.TempDir()
	currentDir := filepath.Join(tmpDir, "tools", "current")
	if err := os.MkdirAll(currentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	fakePatchelf := filepath.Join(currentDir, "patchelf")
	if err := os.WriteFile(fakePatchelf, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := action.findPatchelfInToolsDir(filepath.Join(tmpDir, "tools"), currentDir)
	if err != nil {
		t.Fatalf("findPatchelfInToolsDir() returned error: %v", err)
	}
	if got != fakePatchelf {
		t.Errorf("findPatchelfInToolsDir() = %q, want %q", got, fakePatchelf)
	}
}

func TestFindPatchelfInToolsDir_PicksLatestVersion(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	// Create two patchelf versions
	tmpDir := t.TempDir()
	toolsDir := filepath.Join(tmpDir, "tools")
	for _, ver := range []string{"0.17.0", "0.18.0"} {
		binDir := filepath.Join(toolsDir, "patchelf-"+ver, "bin")
		if err := os.MkdirAll(binDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(binDir, "patchelf"), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	got, err := action.findPatchelfInToolsDir(toolsDir, filepath.Join(toolsDir, "current"))
	if err != nil {
		t.Fatalf("findPatchelfInToolsDir() returned error: %v", err)
	}
	// Should pick 0.18.0 (last in lexicographic order)
	want := filepath.Join(toolsDir, "patchelf-0.18.0", "bin", "patchelf")
	if got != want {
		t.Errorf("findPatchelfInToolsDir() = %q, want %q (latest version)", got, want)
	}
}

func TestFindPatchelfInToolsDir_NotFound(t *testing.T) {
	t.Parallel()
	action := &HomebrewRelocateAction{}

	tmpDir := t.TempDir()
	_, err := action.findPatchelfInToolsDir(filepath.Join(tmpDir, "tools"), filepath.Join(tmpDir, "tools", "current"))
	if err == nil {
		t.Fatal("findPatchelfInToolsDir() should return error when patchelf not found")
	}
}

// -- fixDylibRpathChain: path computation, defense-in-depth --

// TestComputeChainRpaths_ToolBinaryWithRuntimeDeps checks that a tool recipe
// (binary in bin/) with non-empty RuntimeDependencies gets the expected
// @loader_path-relative entries pointing at $LibsDir/<dep>-<v>/lib.
func TestComputeChainRpaths_ToolBinaryWithRuntimeDeps(t *testing.T) {
	t.Parallel()

	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")
	workDir := filepath.Join(tsukuHome, "work")
	binDir := filepath.Join(workDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dep := range []struct{ name, version string }{
		{"libevent", "2.1.12"}, {"utf8proc", "2.9.0"},
	} {
		if err := os.MkdirAll(filepath.Join(libsDir, dep.name+"-"+dep.version, "lib"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	binaryPath := filepath.Join(binDir, "tmux")
	loaderDir := filepath.Dir(binaryPath)
	entries := []chainEntry{
		{name: "libevent", version: "2.1.12"},
		{name: "utf8proc", version: "2.9.0"},
	}

	rpaths, err := computeChainRpaths(loaderDir, libsDir, entries, "tmux")
	if err != nil {
		t.Fatalf("computeChainRpaths returned error: %v", err)
	}
	if len(rpaths) != len(entries) {
		t.Fatalf("got %d rpaths, want %d", len(rpaths), len(entries))
	}

	// Every emitted entry must start with @loader_path/ and resolve back to
	// $libsDir/<dep>-<v>/lib when joined with loaderDir.
	wantSuffixes := []string{
		"libs/libevent-2.1.12/lib",
		"libs/utf8proc-2.9.0/lib",
	}
	for i, rp := range rpaths {
		if !strings.HasPrefix(rp, "@loader_path/") {
			t.Errorf("rpath %d = %q, want @loader_path/ prefix", i, rp)
		}
		// Re-resolve: filepath.Join(loaderDir, strip "@loader_path/" prefix)
		// must equal the expected dep lib dir.
		rel := strings.TrimPrefix(rp, "@loader_path/")
		joined := filepath.Clean(filepath.Join(loaderDir, filepath.FromSlash(rel)))
		if !strings.HasSuffix(filepath.ToSlash(joined), wantSuffixes[i]) {
			t.Errorf("rpath %d resolved to %q, expected suffix %q", i, joined, wantSuffixes[i])
		}
	}
}

// TestComputeChainRpaths_LibraryDylibWithRuntimeDeps checks the parameterized
// "library type" case: dylib in lib/ with non-empty RuntimeDependencies. The
// chain function (post-rename) treats library and tool recipes uniformly via
// the lifted Type gate; the dep entries land via @loader_path-relative paths
// in both cases.
//
// (Pre-rename, fixLibraryDylibRpaths handled this code path. The library
// install-time chain — driven by the legacy `dependencies` field — lives in
// fixLibraryInstallTimeChain and now also emits @loader_path-relative
// paths; see TestComputeChainRpaths_LibraryInstallTimeIsRelative below for
// the lock on that shape.)
func TestComputeChainRpaths_LibraryDylibWithRuntimeDeps(t *testing.T) {
	t.Parallel()

	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")
	workDir := filepath.Join(tsukuHome, "work")
	libSubDir := filepath.Join(workDir, "lib")
	if err := os.MkdirAll(libSubDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(libsDir, "openssl-3.5.0", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}

	dylibPath := filepath.Join(libSubDir, "libcurl.dylib")
	loaderDir := filepath.Dir(dylibPath)
	entries := []chainEntry{{name: "openssl", version: "3.5.0"}}

	rpaths, err := computeChainRpaths(loaderDir, libsDir, entries, "libcurl.dylib")
	if err != nil {
		t.Fatalf("computeChainRpaths returned error: %v", err)
	}
	if len(rpaths) != 1 {
		t.Fatalf("got %d rpaths, want 1", len(rpaths))
	}
	if !strings.HasPrefix(rpaths[0], "@loader_path/") {
		t.Errorf("rpath = %q, want @loader_path/ prefix", rpaths[0])
	}
	rel := strings.TrimPrefix(rpaths[0], "@loader_path/")
	joined := filepath.Clean(filepath.Join(loaderDir, filepath.FromSlash(rel)))
	want := filepath.Clean(filepath.Join(libsDir, "openssl-3.5.0", "lib"))
	if joined != want {
		t.Errorf("rpath resolved to %q, want %q", joined, want)
	}
}

// TestComputeChainRpaths_EmptyEntriesIsNoOp checks that zero entries produces
// zero rpaths (no-op path through the chain function).
func TestComputeChainRpaths_EmptyEntriesIsNoOp(t *testing.T) {
	t.Parallel()
	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")
	loaderDir := filepath.Join(tsukuHome, "work", "bin")
	if err := os.MkdirAll(loaderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	rpaths, err := computeChainRpaths(loaderDir, libsDir, nil, "tool")
	if err != nil {
		t.Fatalf("computeChainRpaths(nil) = %v, want nil error", err)
	}
	if len(rpaths) != 0 {
		t.Errorf("computeChainRpaths(nil) = %v, want empty slice", rpaths)
	}
}

// TestComputeChainRpaths_EscapingEntryIsRejected checks the defense-in-depth
// post-check. An entry whose constructed dep lib path collapses upward (e.g.,
// a dep name containing "..") must fail the install with a clear error before
// any install_name_tool invocation. Dep names are validated upstream at recipe
// load time (Issue 1's validator), so this is a belt-and-braces check.
func TestComputeChainRpaths_EscapingEntryIsRejected(t *testing.T) {
	t.Parallel()
	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")
	loaderDir := filepath.Join(tsukuHome, "work", "bin")
	if err := os.MkdirAll(loaderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Construct an entry whose Join collapses out of libs/. Even though the
	// validator should never let "..foo" through, the chain layer must still
	// reject it as defense-in-depth: filepath.Join(libsDir, "../etc-1.0/lib")
	// resolves to $tsukuHome/etc-1.0/lib — outside libs/.
	entries := []chainEntry{{name: "../etc", version: "1.0"}}

	rpaths, err := computeChainRpaths(loaderDir, libsDir, entries, "tool")
	if err == nil {
		t.Fatalf("computeChainRpaths returned no error; got rpaths %v, want escape error", rpaths)
	}
	if !strings.Contains(err.Error(), "escapes libs dir") {
		t.Errorf("error = %q, want it to mention 'escapes libs dir'", err.Error())
	}
}

// TestFixDylibRpathChain_NonDarwinIsNoOp checks that the chain function is a
// no-op on non-darwin platforms (the macOS-specific install_name_tool path).
// On darwin runners this test still passes because the chain finds no
// binaries to patch under a non-existent work dir.
func TestFixDylibRpathChain_NonDarwinIsNoOp(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "darwin" {
		t.Skip("non-darwin guard test")
	}

	action := &HomebrewRelocateAction{}
	tsukuHome := t.TempDir()
	ctx := &ExecutionContext{
		WorkDir: filepath.Join(tsukuHome, "work"),
		LibsDir: filepath.Join(tsukuHome, "libs"),
		Dependencies: ResolvedDeps{
			Runtime:             map[string]string{"libfoo": "1.2.3"},
			RuntimeDependencies: []string{"libfoo"},
		},
	}

	// Calling on Linux returns nil immediately; the function does nothing.
	err := action.fixDylibRpathChain(ctx, "/unused", progress.NoopReporter{})
	if err != nil {
		t.Fatalf("fixDylibRpathChain on non-darwin = %v, want nil", err)
	}
}

// TestFixDylibRpathChain_EmptyRuntimeDepsIsNoOp checks that an empty
// RuntimeDependencies list produces no chain entries (no-op) regardless of
// platform — the early-return covers that case before any binary walk.
func TestFixDylibRpathChain_EmptyRuntimeDepsIsNoOp(t *testing.T) {
	t.Parallel()

	action := &HomebrewRelocateAction{}
	tsukuHome := t.TempDir()
	workDir := filepath.Join(tsukuHome, "work")
	if err := os.MkdirAll(filepath.Join(workDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir:      workDir,
		LibsDir:      filepath.Join(tsukuHome, "libs"),
		Dependencies: ResolvedDeps{}, // RuntimeDependencies is nil
	}

	err := action.fixDylibRpathChain(ctx, "/unused", progress.NoopReporter{})
	if err != nil {
		t.Fatalf("fixDylibRpathChain with empty RuntimeDeps = %v, want nil", err)
	}
}

// -- fixLibraryInstallTimeChain: @loader_path-relative emit, golden lock --

// TestComputeChainRpaths_LibraryInstallTimeIsRelative locks the RPATH shape
// for the library install-time chain. Pre-Issue 4, the helper emitted
// absolute paths like "/Users/.../libs/openssl-3.5.0/lib"; post-Issue 4
// it emits "@loader_path/..." entries computed via filepath.Rel over
// EvalSymlinks. The library helper reuses computeChainRpaths, so this test
// also serves as the canary for any future regression to absolute paths.
//
// The expected RPATH is derived from the loader/lib structure (not a
// hardcoded $TSUKU_HOME path) so the test is portable across hosts.
func TestComputeChainRpaths_LibraryInstallTimeIsRelative(t *testing.T) {
	t.Parallel()

	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")
	workLibDir := filepath.Join(tsukuHome, "work", "lib")
	if err := os.MkdirAll(workLibDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(libsDir, "openssl-3.5.0", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}

	// Mirror the libevent shape: a Type == "library" recipe with
	// dependencies = ["openssl"] feeds InstallTime = {"openssl": "3.5.0"}.
	dylibPath := filepath.Join(workLibDir, "libevent.dylib")
	loaderDir := filepath.Dir(dylibPath)
	entries := []chainEntry{{name: "openssl", version: "3.5.0"}}

	rpaths, err := computeChainRpaths(loaderDir, libsDir, entries, "libevent.dylib")
	if err != nil {
		t.Fatalf("computeChainRpaths returned error: %v", err)
	}
	if len(rpaths) != 1 {
		t.Fatalf("got %d rpaths, want 1", len(rpaths))
	}

	// Golden: RPATH must be @loader_path-relative, not absolute.
	rp := rpaths[0]
	if !strings.HasPrefix(rp, "@loader_path/") {
		t.Errorf("rpath = %q, want @loader_path/ prefix (regression to absolute path?)", rp)
	}
	if strings.HasPrefix(rp, "/") {
		t.Errorf("rpath = %q starts with /; library install-time chain must emit @loader_path-relative entries", rp)
	}
	if strings.Contains(rp, tsukuHome) {
		t.Errorf("rpath = %q leaks the absolute test tsuku home %q; the emit form must be relative", rp, tsukuHome)
	}

	// Lock the exact relative form. In this synthetic layout the dylib is
	// at $tsukuHome/work/lib and the dep is at $tsukuHome/libs/openssl-3.5.0/lib,
	// so the @loader_path-relative hop is ../../libs/openssl-3.5.0/lib. The
	// real install layout ($TSUKU_HOME/libs/<recipe>-<v>/lib reaching
	// $TSUKU_HOME/libs/<dep>-<v>/lib) yields ../../<dep>-<v>/lib — same
	// computeChainRpaths machinery, different number of "..", which is
	// exactly what filepath.Rel handles. The golden lock here is on
	// "@loader_path/-prefix and no leaked absolute path", which is portable
	// across host paths.
	wantRel := filepath.ToSlash(filepath.Join("..", "..", "libs", "openssl-3.5.0", "lib"))
	wantRpath := "@loader_path/" + wantRel
	if rp != wantRpath {
		t.Errorf("rpath = %q, want %q (golden shape for libevent-style chain)", rp, wantRpath)
	}

	// Re-resolve: the relative path must point back at the dep lib dir.
	rel := strings.TrimPrefix(rp, "@loader_path/")
	joined := filepath.Clean(filepath.Join(loaderDir, filepath.FromSlash(rel)))
	want := filepath.Clean(filepath.Join(libsDir, "openssl-3.5.0", "lib"))
	if joined != want {
		t.Errorf("rpath resolved to %q, want %q", joined, want)
	}
}

// TestFixLibraryInstallTimeChain_NonDarwinIsNoOp checks the macOS-only
// guard: the helper returns nil immediately on non-darwin platforms. (The
// Linux ELF chain ships in Issue 5.)
func TestFixLibraryInstallTimeChain_NonDarwinIsNoOp(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "darwin" {
		t.Skip("non-darwin guard test")
	}

	action := &HomebrewRelocateAction{}
	tsukuHome := t.TempDir()
	ctx := &ExecutionContext{
		WorkDir: filepath.Join(tsukuHome, "work"),
		LibsDir: filepath.Join(tsukuHome, "libs"),
		Dependencies: ResolvedDeps{
			InstallTime: map[string]string{"openssl": "3.5.0"},
		},
	}

	err := action.fixLibraryInstallTimeChain(ctx, "/unused", progress.NoopReporter{})
	if err != nil {
		t.Fatalf("fixLibraryInstallTimeChain on non-darwin = %v, want nil", err)
	}
}

// TestFixLibraryInstallTimeChain_EmptyInstallTimeIsNoOp checks that an
// empty InstallTime map produces no patching (no-op). Library recipes
// without declared dependencies (pcre2, libnghttp3, utf8proc) take this
// path.
func TestFixLibraryInstallTimeChain_EmptyInstallTimeIsNoOp(t *testing.T) {
	t.Parallel()

	action := &HomebrewRelocateAction{}
	tsukuHome := t.TempDir()
	workLibDir := filepath.Join(tsukuHome, "work", "lib")
	if err := os.MkdirAll(workLibDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Drop a fake dylib in lib/ to confirm the helper still returns nil
	// when there are no entries to add (empty InstallTime is the no-op
	// gate, independent of whether dylibs are present).
	if err := os.WriteFile(filepath.Join(workLibDir, "libfoo.dylib"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir:      filepath.Join(tsukuHome, "work"),
		LibsDir:      filepath.Join(tsukuHome, "libs"),
		Dependencies: ResolvedDeps{}, // InstallTime is nil
	}

	err := action.fixLibraryInstallTimeChain(ctx, "/unused", progress.NoopReporter{})
	if err != nil {
		t.Fatalf("fixLibraryInstallTimeChain with empty InstallTime = %v, want nil", err)
	}
}

// TestFixLibraryInstallTimeChain_EscapingEntryIsRejected checks that the
// defense-in-depth post-check applies to the library install-time chain
// too. An InstallTime entry whose name escapes ctx.LibsDir must fail the
// install before any install_name_tool invocation. Dep names are
// validated upstream at recipe load time, so this is a belt-and-braces
// check that mirrors TestComputeChainRpaths_EscapingEntryIsRejected.
func TestFixLibraryInstallTimeChain_EscapingEntryIsRejected(t *testing.T) {
	t.Parallel()

	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")
	loaderDir := filepath.Join(tsukuHome, "work", "lib")
	if err := os.MkdirAll(loaderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// computeChainRpaths is the shared post-check for both chain helpers,
	// so this exercises the same defense path the library helper hits when
	// it encounters an escaping InstallTime entry.
	entries := []chainEntry{{name: "../etc", version: "1.0"}}
	_, err := computeChainRpaths(loaderDir, libsDir, entries, "libfoo.dylib")
	if err == nil {
		t.Fatalf("computeChainRpaths returned no error for escaping entry; want escape error")
	}
	if !strings.Contains(err.Error(), "escapes libs dir") {
		t.Errorf("error = %q, want it to mention 'escapes libs dir'", err.Error())
	}
}

// TestFixLibraryInstallTimeChain_DeterministicOrder checks that the chain
// helper emits RPATHs in deterministic order (sorted by dep name) when the
// underlying InstallTime map iteration order would otherwise be random.
// Without the sort, golden-fixture diffs would be noisy and the install
// would produce a different RPATH order across runs of the same inputs.
func TestFixLibraryInstallTimeChain_DeterministicOrder(t *testing.T) {
	t.Parallel()

	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")
	loaderDir := filepath.Join(tsukuHome, "work", "lib")
	if err := os.MkdirAll(loaderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dep := range []string{"alpha-1.0", "bravo-2.0", "charlie-3.0"} {
		if err := os.MkdirAll(filepath.Join(libsDir, dep, "lib"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	// Sorted-name entries: alpha, bravo, charlie.
	entries := []chainEntry{
		{name: "alpha", version: "1.0"},
		{name: "bravo", version: "2.0"},
		{name: "charlie", version: "3.0"},
	}
	rpaths, err := computeChainRpaths(loaderDir, libsDir, entries, "libfoo.dylib")
	if err != nil {
		t.Fatalf("computeChainRpaths returned error: %v", err)
	}

	wantOrder := []string{"alpha-1.0", "bravo-2.0", "charlie-3.0"}
	if len(rpaths) != len(wantOrder) {
		t.Fatalf("got %d rpaths, want %d", len(rpaths), len(wantOrder))
	}
	for i, rp := range rpaths {
		if !strings.Contains(rp, wantOrder[i]) {
			t.Errorf("rpath %d = %q, want it to contain %q (sort-by-name order)", i, rp, wantOrder[i])
		}
	}
}

// -- fixElfRpathChain (Linux $ORIGIN-relative chain via patchelf) --

// TestComputeChainRpaths_ELFOriginPrefix is the Linux mirror of
// TestComputeChainRpaths_ToolBinaryWithRuntimeDeps. It exercises the same
// path-computation machinery but with the "$ORIGIN" anchor (the prefix used
// by the ELF chain) instead of "@loader_path". The relative-path portion
// must match what the macOS path produces — only the anchor token differs.
func TestComputeChainRpaths_ELFOriginPrefix(t *testing.T) {
	t.Parallel()

	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")
	workDir := filepath.Join(tsukuHome, "work")
	binDir := filepath.Join(workDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dep := range []struct{ name, version string }{
		{"libevent", "2.1.12"}, {"utf8proc", "2.9.0"},
	} {
		if err := os.MkdirAll(filepath.Join(libsDir, dep.name+"-"+dep.version, "lib"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	binaryPath := filepath.Join(binDir, "tmux")
	loaderDir := filepath.Dir(binaryPath)
	entries := []chainEntry{
		{name: "libevent", version: "2.1.12"},
		{name: "utf8proc", version: "2.9.0"},
	}

	rpaths, err := computeChainRpathsWithPrefix(loaderDir, libsDir, entries, "tmux", "$ORIGIN")
	if err != nil {
		t.Fatalf("computeChainRpathsWithPrefix returned error: %v", err)
	}
	if len(rpaths) != len(entries) {
		t.Fatalf("got %d rpaths, want %d", len(rpaths), len(entries))
	}

	// Every emitted entry must start with $ORIGIN/ (not @loader_path/) and
	// resolve back to $libsDir/<dep>-<v>/lib when joined with loaderDir.
	wantSuffixes := []string{
		"libs/libevent-2.1.12/lib",
		"libs/utf8proc-2.9.0/lib",
	}
	for i, rp := range rpaths {
		if !strings.HasPrefix(rp, "$ORIGIN/") {
			t.Errorf("rpath %d = %q, want $ORIGIN/ prefix", i, rp)
		}
		// Re-resolve via the same path the runtime linker would: drop the
		// anchor token, join against the loader dir, clean.
		rel := strings.TrimPrefix(rp, "$ORIGIN/")
		joined := filepath.Clean(filepath.Join(loaderDir, filepath.FromSlash(rel)))
		if !strings.HasSuffix(filepath.ToSlash(joined), wantSuffixes[i]) {
			t.Errorf("rpath %d resolved to %q, expected suffix %q", i, joined, wantSuffixes[i])
		}
	}
}

// TestComputeChainRpaths_ELFEscapingEntryIsRejected mirrors
// TestComputeChainRpaths_EscapingEntryIsRejected for the ELF anchor. The
// post-check must fire regardless of which anchor prefix is in use — the
// escape check operates on the resolved-path side, before the anchor is
// prepended.
func TestComputeChainRpaths_ELFEscapingEntryIsRejected(t *testing.T) {
	t.Parallel()

	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")
	loaderDir := filepath.Join(tsukuHome, "work", "bin")
	if err := os.MkdirAll(loaderDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(libsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	entries := []chainEntry{{name: "../etc", version: "1.0"}}
	rpaths, err := computeChainRpathsWithPrefix(loaderDir, libsDir, entries, "tool", "$ORIGIN")
	if err == nil {
		t.Fatalf("computeChainRpathsWithPrefix returned no error; got rpaths %v, want escape error", rpaths)
	}
	if !strings.Contains(err.Error(), "escapes libs dir") {
		t.Errorf("error = %q, want it to mention 'escapes libs dir'", err.Error())
	}
}

// TestFixElfRpathChain_NonLinuxIsNoOp checks that the chain function is a
// no-op on non-Linux platforms (the Linux-specific patchelf path). On Linux
// runners this test is skipped because the function does run there.
func TestFixElfRpathChain_NonLinuxIsNoOp(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "linux" {
		t.Skip("non-linux guard test")
	}

	action := &HomebrewRelocateAction{}
	tsukuHome := t.TempDir()
	ctx := &ExecutionContext{
		WorkDir: filepath.Join(tsukuHome, "work"),
		LibsDir: filepath.Join(tsukuHome, "libs"),
		Dependencies: ResolvedDeps{
			Runtime:             map[string]string{"libfoo": "1.2.3"},
			RuntimeDependencies: []string{"libfoo"},
		},
	}

	err := action.fixElfRpathChain(ctx, "/unused", progress.NoopReporter{})
	if err != nil {
		t.Fatalf("fixElfRpathChain on non-linux = %v, want nil", err)
	}
}

// TestFixElfRpathChain_EmptyRuntimeDepsIsNoOp checks that an empty
// RuntimeDependencies list short-circuits before any patchelf lookup,
// regardless of platform. Mirror of TestFixDylibRpathChain_EmptyRuntimeDepsIsNoOp.
func TestFixElfRpathChain_EmptyRuntimeDepsIsNoOp(t *testing.T) {
	t.Parallel()

	action := &HomebrewRelocateAction{}
	tsukuHome := t.TempDir()
	workDir := filepath.Join(tsukuHome, "work")
	if err := os.MkdirAll(filepath.Join(workDir, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}

	ctx := &ExecutionContext{
		WorkDir:      workDir,
		LibsDir:      filepath.Join(tsukuHome, "libs"),
		Dependencies: ResolvedDeps{}, // RuntimeDependencies is nil
	}

	err := action.fixElfRpathChain(ctx, "/unused", progress.NoopReporter{})
	if err != nil {
		t.Fatalf("fixElfRpathChain with empty RuntimeDeps = %v, want nil", err)
	}
}

// TestIsELFBinary_MagicBytes checks the ELF magic-bytes sniff used by the
// chain walk to pick out actual ELF files (so a script in bin/ doesn't make
// patchelf bail). Three cases: real ELF magic, wrong magic, empty file.
func TestIsELFBinary_MagicBytes(t *testing.T) {
	t.Parallel()

	action := &HomebrewRelocateAction{}
	dir := t.TempDir()

	cases := []struct {
		name    string
		content []byte
		want    bool
	}{
		{"elf", []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01}, true},
		{"text", []byte("#!/bin/sh\necho hello\n"), false},
		{"empty", []byte{}, false},
		{"macho", []byte{0xcf, 0xfa, 0xed, 0xfe}, false},
	}

	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(dir, c.name)
			if err := os.WriteFile(path, c.content, 0o644); err != nil {
				t.Fatal(err)
			}
			if got := action.isELFBinary(path); got != c.want {
				t.Errorf("isELFBinary(%q) = %v, want %v", c.name, got, c.want)
			}
		})
	}
}

// TestFixElfRpathChain_WritesDTRpath is the integration test: on a Linux
// host with patchelf available, the chain function must write DT_RPATH
// entries (not DT_RUNPATH) so the runtime linker resolves chained libs
// without the subtle DT_RUNPATH semantics that break some tools (e.g.,
// wget's libunistring). The test patches a real ELF binary copied from
// /bin/true and inspects the dynamic section via debug/elf to assert
// DT_RPATH contains the chain entry and DT_RUNPATH stays empty.
//
// Skipped on non-Linux hosts and on Linux hosts without patchelf (the
// chain helper relies on patchelf and the per-binary fixElfRpath pass
// is what keeps the binary alive on Linux today; see
// TestFindPatchelf_NotFound_ReturnsError for the discovery contract).
func TestFixElfRpathChain_WritesDTRpath(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("DT_RPATH integration test runs on Linux only")
	}
	patchelfPath, err := exec.LookPath("patchelf")
	if err != nil {
		t.Skip("patchelf not on PATH; skipping DT_RPATH integration test")
	}

	// Find a real dynamically-linked ELF binary to copy. /bin/true is the
	// smallest portable choice; if it doesn't exist or isn't ELF, fall back
	// to /bin/ls.
	srcCandidates := []string{"/bin/true", "/bin/ls"}
	var srcBin string
	for _, c := range srcCandidates {
		if data, err := os.ReadFile(c); err == nil && len(data) >= 4 && string(data[:4]) == "\x7fELF" {
			srcBin = c
			break
		}
	}
	if srcBin == "" {
		t.Skip("no suitable ELF binary in /bin to use as test fixture")
	}

	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")
	workDir := filepath.Join(tsukuHome, "work")
	binDir := filepath.Join(workDir, "bin")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(libsDir, "libevent-2.1.12", "lib"), 0o755); err != nil {
		t.Fatal(err)
	}

	dstBin := filepath.Join(binDir, "tmux")
	if err := copyTestFile(srcBin, dstBin, 0o755); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}

	action := &HomebrewRelocateAction{}
	ctx := &ExecutionContext{
		WorkDir: workDir,
		LibsDir: libsDir,
		// ExecPaths makes findPatchelf return the path we just resolved
		// rather than searching the system again.
		ExecPaths: []string{filepath.Dir(patchelfPath)},
		Dependencies: ResolvedDeps{
			Runtime:             map[string]string{"libevent": "2.1.12"},
			RuntimeDependencies: []string{"libevent"},
		},
	}

	if err := action.fixElfRpathChain(ctx, "/unused", progress.NoopReporter{}); err != nil {
		t.Fatalf("fixElfRpathChain returned error: %v", err)
	}

	// Inspect the patched binary via debug/elf. Assert:
	//   - DT_RPATH contains an entry pointing at libevent's lib dir.
	//   - DT_RUNPATH is empty (the patchelf invocation must not have
	//     written the RUNPATH variant).
	f, err := elf.Open(dstBin)
	if err != nil {
		t.Fatalf("elf.Open(%s): %v", dstBin, err)
	}
	defer func() { _ = f.Close() }()

	rpaths, err := f.DynString(elf.DT_RPATH)
	if err != nil {
		t.Fatalf("DynString(DT_RPATH): %v", err)
	}
	runpaths, err := f.DynString(elf.DT_RUNPATH)
	if err != nil {
		t.Fatalf("DynString(DT_RUNPATH): %v", err)
	}

	if len(runpaths) != 0 {
		t.Errorf("DT_RUNPATH = %v, want empty (the chain must write DT_RPATH only)", runpaths)
	}
	if len(rpaths) == 0 {
		t.Fatalf("DT_RPATH is empty; want an entry pointing at libevent's lib dir")
	}

	// patchelf joins entries with ":" and stores them in a single string.
	joined := strings.Join(rpaths, ":")
	if !strings.Contains(joined, "libevent-2.1.12/lib") {
		t.Errorf("DT_RPATH = %q, want it to contain 'libevent-2.1.12/lib'", joined)
	}
	if !strings.Contains(joined, "$ORIGIN") {
		t.Errorf("DT_RPATH = %q, want it to contain '$ORIGIN' (anchor for relative resolution)", joined)
	}
}

// copyTestFile is a small test helper that copies src to dst with the given
// mode. Used by TestFixElfRpathChain_WritesDTRpath to stage a real ELF
// binary in the work dir.
func copyTestFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
