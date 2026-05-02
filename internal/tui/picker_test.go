package tui

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

func TestPick_EnterAtFirstRow(t *testing.T) {
	choices := []Choice{
		{Name: "openjdk", Description: "platform default"},
		{Name: "temurin", Description: "Adoptium"},
	}
	stdin := bytes.NewReader([]byte{'\r'}) // Enter
	var stderr bytes.Buffer

	idx, err := pick(stdin, &stderr, "Pick a JDK:", choices)
	if err != nil {
		t.Fatalf("pick: unexpected error: %v", err)
	}
	if idx != 0 {
		t.Errorf("pick: idx = %d, want 0", idx)
	}
	out := stderr.String()
	if !strings.Contains(out, "Pick a JDK:") {
		t.Errorf("expected prompt in stderr; got: %q", out)
	}
	if !strings.Contains(out, "openjdk") || !strings.Contains(out, "temurin") {
		t.Errorf("expected both choice names in stderr; got: %q", out)
	}
}

func TestPick_DownThenEnterAdvancesCursor(t *testing.T) {
	choices := []Choice{
		{Name: "openjdk"},
		{Name: "temurin"},
		{Name: "corretto"},
	}
	// Down arrow (ESC [ B), then Enter
	stdin := bytes.NewReader([]byte{'\x1b', '[', 'B', '\r'})
	var stderr bytes.Buffer

	idx, err := pick(stdin, &stderr, "Pick:", choices)
	if err != nil {
		t.Fatalf("pick: unexpected error: %v", err)
	}
	if idx != 1 {
		t.Errorf("pick: idx = %d, want 1 (one Down advanced from 0 to 1)", idx)
	}
}

func TestPick_DownTwiceThenUpThenEnter(t *testing.T) {
	choices := []Choice{
		{Name: "openjdk"},
		{Name: "temurin"},
		{Name: "corretto"},
		{Name: "microsoft-openjdk"},
	}
	// Down, Down, Up, Enter
	keys := bytes.NewReader([]byte{
		'\x1b', '[', 'B', // Down
		'\x1b', '[', 'B', // Down
		'\x1b', '[', 'A', // Up
		'\r', // Enter
	})
	// pick reads 3 bytes per call but our buffer is contiguous; we need
	// a reader that returns one full ESC sequence per Read. bytes.Reader
	// honors Read's len(p) cap, so explicitly chunk via a small wrapper.
	chunked := &chunkedReader{src: keys, chunk: 3}

	var stderr bytes.Buffer
	idx, err := pick(chunked, &stderr, "Pick:", choices)
	if err != nil {
		t.Fatalf("pick: unexpected error: %v", err)
	}
	// Final cursor: 0 -> 1 -> 2 -> 1.
	if idx != 1 {
		t.Errorf("pick: idx = %d, want 1", idx)
	}
}

func TestPick_CtrlCReturnsErrCancelled(t *testing.T) {
	choices := []Choice{
		{Name: "openjdk"},
		{Name: "temurin"},
	}
	stdin := bytes.NewReader([]byte{0x03}) // Ctrl-C
	var stderr bytes.Buffer

	idx, err := pick(stdin, &stderr, "Pick:", choices)
	if !errors.Is(err, ErrCancelled) {
		t.Fatalf("pick: err = %v, want ErrCancelled", err)
	}
	if idx != 0 {
		t.Errorf("pick: idx = %d on cancel, want 0 (sentinel)", idx)
	}
}

func TestPick_DownAtBottomBoundsIsClamped(t *testing.T) {
	choices := []Choice{
		{Name: "a"},
		{Name: "b"},
	}
	// Three downs on a 2-choice list; cursor should stop at 1.
	chunked := &chunkedReader{
		src: bytes.NewReader([]byte{
			'\x1b', '[', 'B',
			'\x1b', '[', 'B',
			'\x1b', '[', 'B',
			'\r',
		}),
		chunk: 3,
	}
	var stderr bytes.Buffer
	idx, err := pick(chunked, &stderr, "Pick:", choices)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if idx != 1 {
		t.Errorf("pick: idx = %d, want 1 (clamped)", idx)
	}
}

func TestPick_UpAtTopBoundsIsClamped(t *testing.T) {
	choices := []Choice{
		{Name: "a"},
		{Name: "b"},
	}
	chunked := &chunkedReader{
		src: bytes.NewReader([]byte{
			'\x1b', '[', 'A', // Up at 0 — no effect
			'\x1b', '[', 'A', // Up at 0 — no effect
			'\r',
		}),
		chunk: 3,
	}
	var stderr bytes.Buffer
	idx, err := pick(chunked, &stderr, "Pick:", choices)
	if err != nil {
		t.Fatalf("pick: %v", err)
	}
	if idx != 0 {
		t.Errorf("pick: idx = %d, want 0 (clamped)", idx)
	}
}

func TestPick_RejectsEmptyChoices(t *testing.T) {
	_, err := Pick("Pick:", nil)
	if err == nil {
		t.Fatal("expected error for empty choices list")
	}
}

func TestPick_DescriptionWithAnsiEscapeIsStripped(t *testing.T) {
	choices := []Choice{
		{Name: "evil", Description: "\x1b[2J\x1b[Hmalicious"},
		{Name: "good", Description: "ok"},
	}
	stdin := bytes.NewReader([]byte{'\r'})
	var stderr bytes.Buffer

	if _, err := pick(stdin, &stderr, "Pick:", choices); err != nil {
		t.Fatalf("pick: %v", err)
	}
	out := stderr.String()
	// The picker uses ANSI for its OWN frame (cursor positioning, line
	// clearing). What we want to confirm is that the recipe-sourced
	// "\x1b[2J\x1b[H" sequences inside the description are stripped
	// and the literal "malicious" still appears.
	if !strings.Contains(out, "malicious") {
		t.Errorf("expected sanitized description text; got: %q", out)
	}
	// The dangerous "[2J" (clear screen) sequence should NOT appear in
	// the rendered output. Picker's own ANSI bytes don't contain "[2J".
	if strings.Contains(out, "[2J") {
		t.Errorf("expected [2J sequence stripped from description; got: %q", out)
	}
}

func TestSanitizeDisplayString(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"\x1b[2Jclear", "clear"},
		{"\x1b[31mred\x1b[0m", "red"},
		{"\x1b]0;title\x07leftover", "leftover"},
		{"a\x1bb", "ab"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got := SanitizeDisplayString(tt.in)
			if got != tt.want {
				t.Errorf("SanitizeDisplayString(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// chunkedReader returns at most `chunk` bytes per Read so the picker's
// 3-byte-per-iteration loop sees one ESC sequence at a time.
type chunkedReader struct {
	src   *bytes.Reader
	chunk int
}

func (r *chunkedReader) Read(p []byte) (int, error) {
	max := r.chunk
	if len(p) < max {
		max = len(p)
	}
	tmp := make([]byte, max)
	n, err := r.src.Read(tmp)
	copy(p, tmp[:n])
	return n, err
}
