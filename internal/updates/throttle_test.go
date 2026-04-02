package updates

import (
	"os"
	"testing"
	"time"
)

func TestIsOOCThrottled_MissingFile(t *testing.T) {
	dir := t.TempDir()
	if IsOOCThrottled(dir, "node", time.Now()) {
		t.Error("missing file should not be throttled")
	}
}

func TestIsOOCThrottled_FreshFile(t *testing.T) {
	dir := t.TempDir()
	if err := TouchOOCThrottle(dir, "node"); err != nil {
		t.Fatal(err)
	}

	// File just created, should be throttled
	if !IsOOCThrottled(dir, "node", time.Now()) {
		t.Error("freshly touched file should be throttled")
	}
}

func TestIsOOCThrottled_ExpiredFile(t *testing.T) {
	dir := t.TempDir()
	if err := TouchOOCThrottle(dir, "node"); err != nil {
		t.Fatal(err)
	}

	// Check with time 8 days in the future
	future := time.Now().Add(8 * 24 * time.Hour)
	if IsOOCThrottled(dir, "node", future) {
		t.Error("file older than 7 days should not be throttled")
	}
}

func TestIsOOCThrottled_ExactBoundary(t *testing.T) {
	dir := t.TempDir()
	if err := TouchOOCThrottle(dir, "node"); err != nil {
		t.Fatal(err)
	}

	// Check at exactly 7 days (should not be throttled -- duration is strict <)
	boundary := time.Now().Add(OOCThrottleDuration)
	if IsOOCThrottled(dir, "node", boundary) {
		t.Error("file at exactly 7 days should not be throttled")
	}
}

func TestTouchOOCThrottle_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	if err := TouchOOCThrottle(dir, "ripgrep"); err != nil {
		t.Fatal(err)
	}

	path := dir + "/" + OOCFilePrefix + "ripgrep"
	if _, err := os.Stat(path); err != nil {
		t.Errorf("throttle file should exist: %v", err)
	}
}

func TestIsOOCThrottled_PerTool(t *testing.T) {
	dir := t.TempDir()

	// Touch throttle for node but not ripgrep
	if err := TouchOOCThrottle(dir, "node"); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	if !IsOOCThrottled(dir, "node", now) {
		t.Error("node should be throttled")
	}
	if IsOOCThrottled(dir, "ripgrep", now) {
		t.Error("ripgrep should not be throttled")
	}
}
