package blocker

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputeTransitiveBlockers_directOnly(t *testing.T) {
	blockers := map[string][]string{
		"gmp": {"homebrew:ffmpeg", "homebrew:coreutils"},
	}
	pkgToBare := BuildPkgToBare(blockers)
	memo := make(map[string]int)

	count := ComputeTransitiveBlockers("gmp", blockers, pkgToBare, memo)
	if count != 2 {
		t.Errorf("gmp direct count: got %d, want 2", count)
	}
}

func TestComputeTransitiveBlockers_chain(t *testing.T) {
	// A blocks B, B blocks C, C blocks D
	blockers := map[string][]string{
		"A": {"homebrew:B"},
		"B": {"homebrew:C"},
		"C": {"homebrew:D"},
	}
	pkgToBare := BuildPkgToBare(blockers)
	memo := make(map[string]int)

	count := ComputeTransitiveBlockers("A", blockers, pkgToBare, memo)
	if count != 3 {
		t.Errorf("A transitive count: got %d, want 3 (B+C+D)", count)
	}
}

func TestComputeTransitiveBlockers_deduplication(t *testing.T) {
	blockers := map[string][]string{
		"gmp": {"homebrew:ffmpeg", "homebrew:ffmpeg", "homebrew:coreutils"},
	}
	pkgToBare := BuildPkgToBare(blockers)
	memo := make(map[string]int)

	count := ComputeTransitiveBlockers("gmp", blockers, pkgToBare, memo)
	if count != 2 {
		t.Errorf("gmp count with dupes: got %d, want 2", count)
	}
}

func TestComputeTransitiveBlockers_cycle(t *testing.T) {
	// A blocks homebrew:B, B blocks homebrew:A -- a cycle
	blockers := map[string][]string{
		"A": {"homebrew:B"},
		"B": {"homebrew:A"},
	}
	pkgToBare := BuildPkgToBare(blockers)
	memo := make(map[string]int)

	countA := ComputeTransitiveBlockers("A", blockers, pkgToBare, memo)
	countB := ComputeTransitiveBlockers("B", blockers, pkgToBare, memo)

	// Both should be positive (at least their direct dependent)
	if countA < 1 {
		t.Errorf("A count should be >= 1, got %d", countA)
	}
	if countB < 1 {
		t.Errorf("B count should be >= 1, got %d", countB)
	}
}

func TestComputeTransitiveBlockers_noBlockers(t *testing.T) {
	blockers := map[string][]string{}
	pkgToBare := BuildPkgToBare(blockers)
	memo := make(map[string]int)

	count := ComputeTransitiveBlockers("unknown", blockers, pkgToBare, memo)
	if count != 0 {
		t.Errorf("unknown dep count: got %d, want 0", count)
	}
}

func TestComputeTransitiveBlockers_memoReuse(t *testing.T) {
	// When memo is shared, repeated calls should return cached values
	blockers := map[string][]string{
		"A": {"homebrew:B"},
		"B": {"homebrew:C"},
	}
	pkgToBare := BuildPkgToBare(blockers)
	memo := make(map[string]int)

	count1 := ComputeTransitiveBlockers("A", blockers, pkgToBare, memo)
	count2 := ComputeTransitiveBlockers("A", blockers, pkgToBare, memo)
	if count1 != count2 {
		t.Errorf("memoized call should return same result: %d vs %d", count1, count2)
	}
	if count1 != 2 {
		t.Errorf("A count: got %d, want 2", count1)
	}
}

func TestBuildPkgToBare_withPrefix(t *testing.T) {
	blockers := map[string][]string{
		"gmp": {"homebrew:ffmpeg", "cargo:ripgrep"},
	}
	pkgToBare := BuildPkgToBare(blockers)

	if pkgToBare["homebrew:ffmpeg"] != "ffmpeg" {
		t.Errorf("homebrew:ffmpeg -> %q, want ffmpeg", pkgToBare["homebrew:ffmpeg"])
	}
	if pkgToBare["cargo:ripgrep"] != "ripgrep" {
		t.Errorf("cargo:ripgrep -> %q, want ripgrep", pkgToBare["cargo:ripgrep"])
	}
}

func TestBuildPkgToBare_withoutPrefix(t *testing.T) {
	blockers := map[string][]string{
		"gmp": {"ffmpeg", "imagemagick"},
	}
	pkgToBare := BuildPkgToBare(blockers)

	if pkgToBare["ffmpeg"] != "ffmpeg" {
		t.Errorf("ffmpeg -> %q, want ffmpeg", pkgToBare["ffmpeg"])
	}
	if pkgToBare["imagemagick"] != "imagemagick" {
		t.Errorf("imagemagick -> %q, want imagemagick", pkgToBare["imagemagick"])
	}
}

func TestBuildPkgToBare_empty(t *testing.T) {
	pkgToBare := BuildPkgToBare(nil)
	if len(pkgToBare) != 0 {
		t.Errorf("empty input should produce empty map: got %v", pkgToBare)
	}
}

// writeJSONL is a test helper that writes lines to a JSONL file.
func writeJSONL(t *testing.T, dir, filename string, lines []string) {
	t.Helper()
	content := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(dir, filename), []byte(content), 0644); err != nil {
		t.Fatalf("write JSONL: %v", err)
	}
}

func TestLoadBlockerMap_legacyBatchFormat(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "batch.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["gmp"]},{"package_id":"homebrew:coreutils","category":"missing_dep","blocked_by":["gmp","zlib"]}]}`,
	})

	blockers, err := LoadBlockerMap(dir)
	if err != nil {
		t.Fatalf("LoadBlockerMap: %v", err)
	}

	if len(blockers["gmp"]) != 2 {
		t.Errorf("gmp should block 2 packages, got %d: %v", len(blockers["gmp"]), blockers["gmp"])
	}
	if len(blockers["zlib"]) != 1 {
		t.Errorf("zlib should block 1 package, got %d: %v", len(blockers["zlib"]), blockers["zlib"])
	}
}

func TestLoadBlockerMap_perRecipeFormat(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "per-recipe.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","recipe":"curl","category":"missing_dep","blocked_by":["openssl"]}`,
		`{"schema_version":1,"ecosystem":"homebrew","recipe":"wget","category":"missing_dep","blocked_by":["openssl","zlib"]}`,
	})

	blockers, err := LoadBlockerMap(dir)
	if err != nil {
		t.Fatalf("LoadBlockerMap: %v", err)
	}

	if len(blockers["openssl"]) != 2 {
		t.Errorf("openssl should block 2 packages, got %d: %v", len(blockers["openssl"]), blockers["openssl"])
	}
	if len(blockers["zlib"]) != 1 {
		t.Errorf("zlib should block 1 package, got %d: %v", len(blockers["zlib"]), blockers["zlib"])
	}
	// Verify package IDs are constructed correctly
	found := false
	for _, pkg := range blockers["openssl"] {
		if pkg == "homebrew:curl" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected homebrew:curl in openssl blockers, got %v", blockers["openssl"])
	}
}

func TestLoadBlockerMap_perRecipeDefaultEcosystem(t *testing.T) {
	dir := t.TempDir()
	// Ecosystem field omitted -- should default to "homebrew"
	writeJSONL(t, dir, "no-eco.jsonl", []string{
		`{"schema_version":1,"recipe":"curl","category":"missing_dep","blocked_by":["openssl"]}`,
	})

	blockers, err := LoadBlockerMap(dir)
	if err != nil {
		t.Fatalf("LoadBlockerMap: %v", err)
	}

	if len(blockers["openssl"]) != 1 {
		t.Fatalf("expected 1 package blocked by openssl, got %d", len(blockers["openssl"]))
	}
	if blockers["openssl"][0] != "homebrew:curl" {
		t.Errorf("expected homebrew:curl, got %s", blockers["openssl"][0])
	}
}

func TestLoadBlockerMap_mixedFormats(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "legacy.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["gmp"]}]}`,
	})
	writeJSONL(t, dir, "per-recipe.jsonl", []string{
		`{"schema_version":1,"ecosystem":"homebrew","recipe":"curl","category":"missing_dep","blocked_by":["openssl"]}`,
	})

	blockers, err := LoadBlockerMap(dir)
	if err != nil {
		t.Fatalf("LoadBlockerMap: %v", err)
	}

	if len(blockers["gmp"]) != 1 {
		t.Errorf("gmp from legacy format: expected 1 blocked, got %d", len(blockers["gmp"]))
	}
	if len(blockers["openssl"]) != 1 {
		t.Errorf("openssl from per-recipe format: expected 1 blocked, got %d", len(blockers["openssl"]))
	}
}

func TestLoadBlockerMap_noFiles(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadBlockerMap(dir)
	if err == nil {
		t.Fatal("expected error for empty directory, got nil")
	}
	if !strings.Contains(err.Error(), "no failure files") {
		t.Errorf("expected 'no failure files' error, got: %v", err)
	}
}

func TestLoadBlockerMap_malformedLinesSkipped(t *testing.T) {
	dir := t.TempDir()
	writeJSONL(t, dir, "mixed.jsonl", []string{
		`not valid json at all`,
		`{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:ffmpeg","category":"missing_dep","blocked_by":["gmp"]}]}`,
		`{truncated`,
	})

	blockers, err := LoadBlockerMap(dir)
	if err != nil {
		t.Fatalf("LoadBlockerMap: %v", err)
	}

	// Only the valid line should contribute
	if len(blockers["gmp"]) != 1 {
		t.Errorf("gmp should have 1 blocked package (from valid line), got %d", len(blockers["gmp"]))
	}
}

func TestLoadBlockerMap_largeLines(t *testing.T) {
	dir := t.TempDir()

	// Build a JSONL record with a blocked_by array large enough to exceed
	// the default 64KB bufio.Scanner limit.
	var deps []string
	for i := 0; i < 2000; i++ {
		deps = append(deps, `"dep-`+strings.Repeat("x", 30)+`-`+string(rune('a'+i%26))+`"`)
	}
	line := `{"schema_version":1,"ecosystem":"homebrew","failures":[{"package_id":"homebrew:big-pkg","category":"missing_dep","blocked_by":[` + strings.Join(deps, ",") + `]}]}`

	// Verify the line actually exceeds 64KB
	if len(line) < 64*1024 {
		t.Fatalf("test line is only %d bytes, need > 65536 to exercise large line handling", len(line))
	}

	writeJSONL(t, dir, "large.jsonl", []string{line})

	blockers, err := LoadBlockerMap(dir)
	if err != nil {
		t.Fatalf("LoadBlockerMap: %v", err)
	}

	// Should have parsed all 2000 deps
	totalBlockers := 0
	for _, pkgs := range blockers {
		totalBlockers += len(pkgs)
	}
	if totalBlockers != 2000 {
		t.Errorf("expected 2000 total blocker entries, got %d", totalBlockers)
	}
}
