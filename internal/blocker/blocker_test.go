package blocker

import (
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
