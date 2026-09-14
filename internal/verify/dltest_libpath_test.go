package verify

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSanitizeEnvForHelperSearchesEachInstalledLibrary is the regression test
// for tsukumogami/tsuku#1090.
//
// The helper dlopens a tsuku-installed library and reports whether it loaded.
// The environment it runs under used to put only $TSUKU_HOME/libs on the
// loader path — but libraries install to $TSUKU_HOME/libs/<name>-<version>/lib,
// so that directory contains no .so file at all. The loader therefore found the
// system copy first, and `tsuku verify openssl` failed with a symbol-version
// error naming the system libcrypto against tsuku's newer libssl.
//
// The directories must also come before the caller's own value: a verification
// run has to test the library tsuku installed, not whichever copy the machine
// would otherwise prefer.
func TestSanitizeEnvForHelperSearchesEachInstalledLibrary(t *testing.T) {
	t.Setenv("LD_LIBRARY_PATH", "/opt/caller")

	tsukuHome := t.TempDir()
	libsDir := filepath.Join(tsukuHome, "libs")

	openssl := filepath.Join(libsDir, "openssl-3.6.0", "lib")
	zlib := filepath.Join(libsDir, "zlib-1.3.2", "lib")
	staging := filepath.Join(libsDir, ".openssl-3.6.0.staging", "lib")
	for _, dir := range []string{openssl, zlib, staging} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("failed to create %s: %v", dir, err)
		}
	}

	entries := loaderEntries(sanitizeEnvForHelper(tsukuHome), "LD_LIBRARY_PATH")
	if len(entries) != 1 {
		t.Fatalf("LD_LIBRARY_PATH appears %d times, want 1: %v", len(entries), entries)
	}
	got := entries[0]

	for _, dir := range []string{openssl, zlib} {
		if !strings.Contains(got, dir) {
			t.Errorf("LD_LIBRARY_PATH = %q\nis missing %q.\n"+
				"Only the parent libs directory is on the path, which holds no .so "+
				"file, so the loader falls through to the system copy — that is "+
				"tsukumogami/tsuku#1090.", got, dir)
		}
	}

	if strings.Contains(got, staging) {
		t.Errorf("LD_LIBRARY_PATH = %q\nincludes the staging directory %q.\n"+
			"Dot-prefixed entries under libs are an install's work directories: a "+
			"tree still being written, or a superseded copy of an older version. "+
			"Admitting one would put two versions of the same library on the path.", got, staging)
	}

	dirs := strings.Split(got, ":")
	if last := dirs[len(dirs)-1]; last != "/opt/caller" {
		t.Errorf("LD_LIBRARY_PATH = %q: caller's value should come last, got %q last", got, last)
	}
	if first := dirs[0]; first != openssl {
		t.Errorf("LD_LIBRARY_PATH = %q: an installed library's own directory should "+
			"come first, got %q", got, first)
	}
}
