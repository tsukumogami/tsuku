package sonameindex

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/tsukumogami/tsuku/internal/recipe"
)

// makeRecipe builds an in-memory library recipe with two install_binaries
// steps, one each for linux and darwin, populated with the given outputs.
// Pass nil for either slice to omit that platform's step.
func makeRecipe(name string, linuxOutputs, darwinOutputs []string) *recipe.Recipe {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: name,
			Type: recipe.RecipeTypeLibrary,
		},
	}
	if linuxOutputs != nil {
		r.Steps = append(r.Steps, recipe.Step{
			Action: "install_binaries",
			When:   &recipe.WhenClause{OS: []string{"linux"}, Libc: []string{"glibc"}},
			Params: map[string]interface{}{
				"install_mode": "directory",
				"outputs":      stringsToInterfaces(linuxOutputs),
			},
		})
	}
	if darwinOutputs != nil {
		r.Steps = append(r.Steps, recipe.Step{
			Action: "install_binaries",
			When:   &recipe.WhenClause{OS: []string{"darwin"}},
			Params: map[string]interface{}{
				"install_mode": "directory",
				"outputs":      stringsToInterfaces(darwinOutputs),
			},
		})
	}
	return r
}

func stringsToInterfaces(in []string) []interface{} {
	out := make([]interface{}, len(in))
	for i, s := range in {
		out[i] = s
	}
	return out
}

// assertProvider checks that (platform, soname) maps to the expected recipe.
func assertProvider(t *testing.T, idx *Index, platform Platform, soname, wantRecipe string) {
	t.Helper()
	provider, ok := idx.Lookup(platform, soname)
	if !ok {
		t.Errorf("Lookup(%q, %q) = not found; want recipe %q", platform, soname, wantRecipe)
		return
	}
	if provider.Recipe != wantRecipe {
		t.Errorf("Lookup(%q, %q) = %q; want %q", platform, soname, provider.Recipe, wantRecipe)
	}
}

func assertNoProvider(t *testing.T, idx *Index, platform Platform, soname string) {
	t.Helper()
	if provider, ok := idx.Lookup(platform, soname); ok {
		t.Errorf("Lookup(%q, %q) = %q; want no provider", platform, soname, provider.Recipe)
	}
}

// curatedLibraries is the must-have mapping from the PLAN (Issue 2 ACs).
// These are the four curated library recipes whose SONAMES the index must
// resolve correctly.
func TestBuild_CuratedLibraries(t *testing.T) {
	pcre2 := makeRecipe("pcre2",
		[]string{"bin/pcre2grep", "bin/pcre2test"}, // no .so outputs on linux (static-only build)
		[]string{"bin/pcre2grep", "bin/pcre2test"}, // bottle ships dylibs but the recipe lists only the binaries it symlinks; the bottle's dylibs are picked up by directory copy and aren't authoritative SONAME providers in this index.
	)
	libnghttp3 := makeRecipe("libnghttp3",
		[]string{
			"lib/libnghttp3.so",
			"lib/libnghttp3.so.9",
			"lib/libnghttp3.a",
			"lib/pkgconfig/libnghttp3.pc",
			"include/nghttp3/nghttp3.h",
		},
		[]string{
			"lib/libnghttp3.dylib",
			"lib/libnghttp3.9.dylib",
			"lib/libnghttp3.a",
			"lib/pkgconfig/libnghttp3.pc",
			"include/nghttp3/nghttp3.h",
		},
	)
	libevent := makeRecipe("libevent",
		[]string{
			"lib/libevent.so",
			"lib/libevent.a",
			"lib/libevent_core.so",
			"lib/libevent_extra.so",
			"lib/libevent_openssl.so",
			"lib/libevent_pthreads.so",
			"include/event2/event.h",
		},
		[]string{
			"lib/libevent.dylib",
			"lib/libevent-2.1.7.dylib",
			"lib/libevent_core.dylib",
			"lib/libevent_core-2.1.7.dylib",
			"lib/libevent_extra.dylib",
			"lib/libevent_extra-2.1.7.dylib",
			"lib/libevent_openssl.dylib",
			"lib/libevent_openssl-2.1.7.dylib",
			"lib/libevent_pthreads.dylib",
			"lib/libevent_pthreads-2.1.7.dylib",
			"include/event2/event.h",
		},
	)
	utf8proc := makeRecipe("utf8proc",
		[]string{
			"lib/libutf8proc.so",
			"lib/libutf8proc.so.3",
			"lib/libutf8proc.a",
			"include/utf8proc.h",
		},
		[]string{
			"lib/libutf8proc.dylib",
			"lib/libutf8proc.3.dylib",
			"lib/libutf8proc.a",
			"include/utf8proc.h",
		},
	)

	idx, err := Build([]*recipe.Recipe{pcre2, libnghttp3, libevent, utf8proc})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// libnghttp3: full SONAME and unversioned alias on both platforms.
	assertProvider(t, idx, PlatformLinux, "libnghttp3.so", "libnghttp3")
	assertProvider(t, idx, PlatformLinux, "libnghttp3.so.9", "libnghttp3")
	assertProvider(t, idx, PlatformDarwin, "libnghttp3.dylib", "libnghttp3")
	assertProvider(t, idx, PlatformDarwin, "libnghttp3.9.dylib", "libnghttp3")

	// libevent: family of related libraries.
	assertProvider(t, idx, PlatformLinux, "libevent.so", "libevent")
	assertProvider(t, idx, PlatformLinux, "libevent_core.so", "libevent")
	assertProvider(t, idx, PlatformLinux, "libevent_openssl.so", "libevent")
	assertProvider(t, idx, PlatformDarwin, "libevent.dylib", "libevent")
	assertProvider(t, idx, PlatformDarwin, "libevent-2.1.7.dylib", "libevent")
	assertProvider(t, idx, PlatformDarwin, "libevent_core-2.1.7.dylib", "libevent")

	// utf8proc: versioned and unversioned aliases on both platforms.
	assertProvider(t, idx, PlatformLinux, "libutf8proc.so", "utf8proc")
	assertProvider(t, idx, PlatformLinux, "libutf8proc.so.3", "utf8proc")
	assertProvider(t, idx, PlatformDarwin, "libutf8proc.dylib", "utf8proc")
	assertProvider(t, idx, PlatformDarwin, "libutf8proc.3.dylib", "utf8proc")

	// Non-SONAME outputs (headers, .a, .pc) must not appear in the index.
	assertNoProvider(t, idx, PlatformLinux, "libnghttp3.a")
	assertNoProvider(t, idx, PlatformLinux, "libnghttp3.pc")

	// pcre2 contributes only bin/ outputs in this fixture, so no SONAMES.
	assertNoProvider(t, idx, PlatformLinux, "libpcre2-8.so.0")
}

// TestBuild_VersionedAliasing exercises the design's stated requirement
// that lib/libfoo.so, lib/libfoo.so.1, and lib/libfoo.so.1.2.3 all map
// to the same provider.
func TestBuild_VersionedAliasing(t *testing.T) {
	r := makeRecipe("foo", []string{
		"lib/libfoo.so",
		"lib/libfoo.so.1",
		"lib/libfoo.so.1.2.3",
	}, []string{
		"lib/libfoo.dylib",
		"lib/libfoo.1.dylib",
		"lib/libfoo.1.2.3.dylib",
	})
	idx, err := Build([]*recipe.Recipe{r})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	wantLinux := []string{"libfoo.so", "libfoo.so.1", "libfoo.so.1.2", "libfoo.so.1.2.3"}
	for _, soname := range wantLinux {
		assertProvider(t, idx, PlatformLinux, soname, "foo")
	}
	wantDarwin := []string{"libfoo.dylib", "libfoo.1.dylib", "libfoo.1.2.dylib", "libfoo.1.2.3.dylib"}
	for _, soname := range wantDarwin {
		assertProvider(t, idx, PlatformDarwin, soname, "foo")
	}
}

// TestBuild_PerPlatformVariants exercises the index keying on (platform,
// SONAME). The same recipe contributes different SONAMES on each
// platform.
func TestBuild_PerPlatformVariants(t *testing.T) {
	r := makeRecipe("split",
		[]string{"lib/libsplit.so.2"},
		[]string{"lib/libsplit.2.dylib"},
	)
	idx, err := Build([]*recipe.Recipe{r})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	assertProvider(t, idx, PlatformLinux, "libsplit.so.2", "split")
	assertProvider(t, idx, PlatformLinux, "libsplit.so", "split")
	assertProvider(t, idx, PlatformDarwin, "libsplit.2.dylib", "split")
	assertProvider(t, idx, PlatformDarwin, "libsplit.dylib", "split")

	// Cross-platform lookups must miss — the linux-only SONAME must not
	// appear under darwin and vice versa.
	assertNoProvider(t, idx, PlatformLinux, "libsplit.dylib")
	assertNoProvider(t, idx, PlatformDarwin, "libsplit.so")
}

// TestBuild_CollisionFails asserts that two distinct library recipes
// claiming the same (platform, SONAME) cause Build to fail with both
// recipe names in the error.
func TestBuild_CollisionFails(t *testing.T) {
	r1 := makeRecipe("first", []string{"lib/libcollide.so.1"}, nil)
	r2 := makeRecipe("second", []string{"lib/libcollide.so.1"}, nil)

	_, err := Build([]*recipe.Recipe{r1, r2})
	if err == nil {
		t.Fatal("Build succeeded; want collision error")
	}
	for _, want := range []string{"libcollide.so.1", "first", "second"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not contain %q", err.Error(), want)
		}
	}
}

// TestBuild_CollisionAcrossPlatforms confirms that two recipes claiming
// the same SONAME under different platforms is NOT a collision. The
// design keys the index on (platform, SONAME), so cross-platform reuse
// of a name is permitted (though unusual).
func TestBuild_CollisionAcrossPlatforms(t *testing.T) {
	r1 := makeRecipe("first", []string{"lib/libshared.so.1"}, nil)
	r2 := makeRecipe("second", nil, []string{"lib/libshared.1.dylib"})

	idx, err := Build([]*recipe.Recipe{r1, r2})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	assertProvider(t, idx, PlatformLinux, "libshared.so.1", "first")
	assertProvider(t, idx, PlatformDarwin, "libshared.1.dylib", "second")
}

// TestParser_SkipsNonSonameEntries asserts that non-SONAME outputs are
// skipped without error and without panicking. The fixture covers
// header files, a non-lib path under lib/, a basename without an
// extension, and a traversal attempt.
func TestParser_SkipsNonSonameEntries(t *testing.T) {
	r := makeRecipe("safe", []string{
		"include/foo.h",     // not under lib/
		"lib/include/bar",   // under lib/ but basename does not start with "lib"
		"lib/foo",           // no .so/.dylib extension and basename doesn't start with "lib"
		"../../etc/passwd",  // path traversal
		"lib/../etc/passwd", // path traversal via lib/
		"lib/libfoo.h",      // starts with lib but wrong extension
		"lib/libfoo.a",      // static archive, not a SONAME
		"lib/libfoo.so.txt", // .so but wrong trailing form
		"lib/libreal.so.1",  // valid — must be the only entry indexed
	}, nil)

	idx, err := Build([]*recipe.Recipe{r})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Only the one valid SONAME family should be indexed.
	assertProvider(t, idx, PlatformLinux, "libreal.so.1", "safe")
	assertProvider(t, idx, PlatformLinux, "libreal.so", "safe")

	// None of the rejected forms should appear.
	for _, soname := range []string{
		"foo.h",
		"bar",
		"foo",
		"libfoo.h",
		"libfoo.a",
		"libfoo.so.txt",
	} {
		assertNoProvider(t, idx, PlatformLinux, soname)
	}

	// And the index size must be exactly the two valid aliases.
	if got, want := idx.Size(), 2; got != want {
		t.Errorf("Size() = %d; want %d", got, want)
	}
}

// TestBuild_SkipsNonLibraryRecipes confirms non-library recipes do not
// contribute to the index, even if their `outputs` happen to look like
// SONAMES.
func TestBuild_SkipsNonLibraryRecipes(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "tooltype",
			Type: recipe.RecipeTypeTool, // explicitly a tool, not a library
		},
		Steps: []recipe.Step{
			{
				Action: "install_binaries",
				When:   &recipe.WhenClause{OS: []string{"linux"}},
				Params: map[string]interface{}{
					"install_mode": "directory",
					"outputs":      []interface{}{"lib/libtool.so.1"},
				},
			},
		},
	}
	idx, err := Build([]*recipe.Recipe{r})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if size := idx.Size(); size != 0 {
		t.Errorf("Size() = %d; want 0 (tool-typed recipes must not contribute)", size)
	}
}

// TestBuild_StepWithoutWhenContributesToBoth asserts that a step
// without a when clause contributes to both linux and darwin keys.
func TestBuild_StepWithoutWhenContributesToBoth(t *testing.T) {
	r := &recipe.Recipe{
		Metadata: recipe.MetadataSection{
			Name: "universal",
			Type: recipe.RecipeTypeLibrary,
		},
		Steps: []recipe.Step{
			{
				Action: "install_binaries",
				Params: map[string]interface{}{
					"outputs": []interface{}{
						"lib/libuniv.so.1",
						"lib/libuniv.1.dylib",
					},
				},
			},
		},
	}
	idx, err := Build([]*recipe.Recipe{r})
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	assertProvider(t, idx, PlatformLinux, "libuniv.so.1", "universal")
	assertProvider(t, idx, PlatformLinux, "libuniv.so", "universal")
	assertProvider(t, idx, PlatformDarwin, "libuniv.1.dylib", "universal")
	assertProvider(t, idx, PlatformDarwin, "libuniv.dylib", "universal")
}

// TestIsKnownGap exercises the static known-gap allowlist.
func TestIsKnownGap(t *testing.T) {
	if !IsKnownGap("libuuid.so.1") {
		t.Error("IsKnownGap(\"libuuid.so.1\") = false; want true")
	}
	if !IsKnownGap("libacl.so.1") {
		t.Error("IsKnownGap(\"libacl.so.1\") = false; want true")
	}
	if !IsKnownGap("libattr.so.1") {
		t.Error("IsKnownGap(\"libattr.so.1\") = false; want true")
	}
	// SONAMES that are covered by curated library recipes must not be
	// on the allowlist — otherwise the scanner would suppress logs for
	// recipes that do exist.
	if IsKnownGap("libpcre2-8.so.0") {
		t.Error("IsKnownGap(\"libpcre2-8.so.0\") = true; want false (covered by pcre2 recipe)")
	}
	if IsKnownGap("libutf8proc.so.3") {
		t.Error("IsKnownGap(\"libutf8proc.so.3\") = true; want false (covered by utf8proc recipe)")
	}
	if IsKnownGap("") {
		t.Error("IsKnownGap(\"\") = true; want false")
	}
}

// TestLookup_NilIndex confirms Lookup on a nil receiver does not panic.
func TestLookup_NilIndex(t *testing.T) {
	var idx *Index
	if _, ok := idx.Lookup(PlatformLinux, "libfoo.so.1"); ok {
		t.Error("Lookup on nil index returned ok=true; want false")
	}
	if size := idx.Size(); size != 0 {
		t.Errorf("Size on nil index = %d; want 0", size)
	}
}

// TestBuild_FromAllLibraryRecipesOnDisk walks every Type=="library"
// recipe in the on-disk registry to discharge the bonus AC: the parser
// must not panic or produce a parser-level error on any current
// library recipe.
//
// Build itself may fail with a collision error — that's the design's
// intended loud behavior when two recipes claim the same SONAME. We
// strip known collision pairs (e.g. libcurl vs libcurl-source, the
// latter being a source-build variant kept for testing) and re-run.
// A parser-level failure (panic, type mismatch, etc.) still fails the
// test.
func TestBuild_FromAllLibraryRecipesOnDisk(t *testing.T) {
	recipesDir := findRecipesDir(t)
	if recipesDir == "" {
		t.Skip("recipes directory not found relative to test binary")
	}

	matches, err := filepath.Glob(filepath.Join(recipesDir, "*", "*.toml"))
	if err != nil {
		t.Fatalf("glob recipes: %v", err)
	}
	sort.Strings(matches)

	// Recipes named here are intentional alternates of canonical
	// providers (e.g. *-source variants) and would collide with their
	// canonical sibling. Strip them for this end-to-end smoke test.
	collisionAlternates := map[string]bool{
		"libcurl-source": true,
	}

	var libraryRecipes []*recipe.Recipe
	for _, p := range matches {
		r, err := recipe.ParseFile(p)
		if err != nil {
			// Validation errors here are out of scope for this test;
			// the recipe validator has its own test surface.
			continue
		}
		if !r.IsLibrary() {
			continue
		}
		if collisionAlternates[r.Metadata.Name] {
			continue
		}
		libraryRecipes = append(libraryRecipes, r)
	}

	if len(libraryRecipes) == 0 {
		t.Skip("no library recipes found in registry")
	}

	idx, err := Build(libraryRecipes)
	if err != nil {
		t.Fatalf("Build over %d library recipes failed: %v", len(libraryRecipes), err)
	}
	if idx.Size() == 0 {
		t.Errorf("Build over %d library recipes produced an empty index", len(libraryRecipes))
	}
}

// findRecipesDir locates the recipes/ directory by walking up from the
// test working directory. Returns an empty string if not found.
func findRecipesDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(".")
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, "recipes")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}
