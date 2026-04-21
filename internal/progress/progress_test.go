package progress

import (
	"bytes"
	"testing"
)

func TestProgressWriterReset(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	callCount := 0
	pw := NewProgressWriter(&buf, 1000, func(written, total int64) {
		callCount++
	})

	if _, err := pw.Write(make([]byte, 200)); err != nil {
		t.Fatalf("Write failed: %v", err)
	}
	if pw.written != 200 {
		t.Errorf("written = %d, want 200", pw.written)
	}

	pw.Reset()
	if pw.written != 0 {
		t.Errorf("after Reset written = %d, want 0", pw.written)
	}
}

func TestProgressWriterRetryNeverExceeds100(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	total := int64(500 * 1024) // above small-file threshold
	var maxPct float64

	pw := NewProgressWriter(&buf, total, func(written, _ int64) {
		pct := float64(written) / float64(total) * 100
		if pct > maxPct {
			maxPct = pct
		}
	})

	// First attempt: write full amount
	if _, err := pw.Write(make([]byte, int(total))); err != nil {
		t.Fatal(err)
	}

	// Reset and simulate retry: write again
	pw.Reset()
	if _, err := pw.Write(make([]byte, int(total))); err != nil {
		t.Fatal(err)
	}

	if maxPct > 100 {
		t.Errorf("percentage exceeded 100: %.1f%%", maxPct)
	}
}

func TestProgressWriterFormatWithTotal(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	total := int64(200 * 1024) // 200 KiB, above small-file threshold
	var callWritten, callTotal int64
	called := false

	pw := NewProgressWriter(&buf, total, func(written, tot int64) {
		callWritten = written
		callTotal = tot
		called = true
	})

	if _, err := pw.Write(make([]byte, 100*1024)); err != nil {
		t.Fatal(err)
	}

	if !called {
		t.Fatal("callback was not called for large file with total")
	}
	if callWritten != 100*1024 {
		t.Errorf("callback written = %d, want %d", callWritten, 100*1024)
	}
	if callTotal != total {
		t.Errorf("callback total = %d, want %d", callTotal, total)
	}
}

func TestProgressWriterFormatNoTotal(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	var lastWritten int64

	pw := NewProgressWriter(&buf, 0, func(written, total int64) {
		lastWritten = written
		if total != 0 {
			t.Errorf("expected total=0 for unknown-size download, got %d", total)
		}
	})

	if _, err := pw.Write(make([]byte, 512)); err != nil {
		t.Fatal(err)
	}

	if lastWritten != 512 {
		t.Errorf("lastWritten = %d, want 512", lastWritten)
	}
}

func TestProgressWriterSmallFile(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	callCount := 0

	// total < 100 KB → callback must be suppressed
	pw := NewProgressWriter(&buf, 50*1024, func(written, total int64) {
		callCount++
	})

	if _, err := pw.Write(make([]byte, 50*1024)); err != nil {
		t.Fatal(err)
	}

	if callCount != 0 {
		t.Errorf("callback called %d times for small file, want 0", callCount)
	}
}

func TestShouldShowProgress(t *testing.T) {
	// Save original function
	origFunc := IsTerminalFunc
	defer func() { IsTerminalFunc = origFunc }()

	// Test when terminal
	IsTerminalFunc = func(fd int) bool { return true }
	if !ShouldShowProgress() {
		t.Error("ShouldShowProgress() = false when terminal, want true")
	}

	// Test when not terminal
	IsTerminalFunc = func(fd int) bool { return false }
	if ShouldShowProgress() {
		t.Error("ShouldShowProgress() = true when not terminal, want false")
	}
}
