package autoinstall

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/tsukumogami/tsuku/internal/config"
)

// mockProjectVersionResolver is a test double for ProjectVersionResolver.
type mockProjectVersionResolver struct {
	versions map[string]string // command -> version
	err      error
}

func (m *mockProjectVersionResolver) ProjectVersionFor(_ context.Context, command string) (string, bool, error) {
	if m.err != nil {
		return "", false, m.err
	}
	v, ok := m.versions[command]
	return v, ok, nil
}

// newTestRunner creates a Runner with captured stdout/stderr for testing.
func newTestRunner(t *testing.T) (*Runner, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cfg := &config.Config{
		CacheDir: t.TempDir(),
	}
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	r := NewRunner(cfg, stdout, stderr)
	return r, stdout, stderr
}

func TestNewRunner(t *testing.T) {
	r, _, _ := newTestRunner(t)
	if r == nil {
		t.Fatal("NewRunner returned nil")
	}
	if r.cfg == nil {
		t.Fatal("Runner.cfg is nil")
	}
}

func TestRunStubReturnsNotImplemented(t *testing.T) {
	r, _, _ := newTestRunner(t)
	err := r.Run(context.Background(), "jq", nil, ModeConfirm, nil)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

func TestParseModeValid(t *testing.T) {
	tests := []struct {
		input string
		want  Mode
	}{
		{"confirm", ModeConfirm},
		{"suggest", ModeSuggest},
		{"auto", ModeAuto},
	}
	for _, tt := range tests {
		m, ok := ParseMode(tt.input)
		if !ok {
			t.Errorf("ParseMode(%q) returned ok=false", tt.input)
		}
		if m != tt.want {
			t.Errorf("ParseMode(%q) = %v, want %v", tt.input, m, tt.want)
		}
	}
}

func TestParseModeInvalid(t *testing.T) {
	_, ok := ParseMode("invalid")
	if ok {
		t.Error("ParseMode(\"invalid\") returned ok=true")
	}
}

func TestModeString(t *testing.T) {
	tests := []struct {
		mode Mode
		want string
	}{
		{ModeConfirm, "confirm"},
		{ModeSuggest, "suggest"},
		{ModeAuto, "auto"},
	}
	for _, tt := range tests {
		if got := tt.mode.String(); got != tt.want {
			t.Errorf("Mode(%d).String() = %q, want %q", tt.mode, got, tt.want)
		}
	}
}

func TestMockProjectVersionResolver(t *testing.T) {
	resolver := &mockProjectVersionResolver{
		versions: map[string]string{"jq": "1.7.1"},
	}

	v, ok, err := resolver.ProjectVersionFor(context.Background(), "jq")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected ok=true for pinned command")
	}
	if v != "1.7.1" {
		t.Errorf("got version %q, want %q", v, "1.7.1")
	}

	_, ok, err = resolver.ProjectVersionFor(context.Background(), "curl")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok=false for unpinned command")
	}
}
