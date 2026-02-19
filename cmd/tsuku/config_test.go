package main

import (
	"io"
	"strings"
	"testing"
)

func TestIsKnownSecret(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"anthropic_api_key", true},
		{"google_api_key", true},
		{"github_token", true},
		{"tavily_api_key", true},
		{"brave_api_key", true},
		{"nonexistent_key", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isKnownSecret(tt.name)
			if got != tt.want {
				t.Errorf("isKnownSecret(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestReadSecretFromStdin(t *testing.T) {
	// Save originals and restore after test.
	origReader := stdinReader
	origIsTerminal := stdinIsTerminal
	defer func() {
		stdinReader = origReader
		stdinIsTerminal = origIsTerminal
	}()

	tests := []struct {
		name       string
		input      string
		isTerminal bool
		wantValue  string
		wantErr    bool
	}{
		{
			name:       "piped value with newline",
			input:      "test-secret-value\n",
			isTerminal: false,
			wantValue:  "test-secret-value",
		},
		{
			name:       "piped value without newline (EOF)",
			input:      "test-secret-value",
			isTerminal: false,
			wantValue:  "test-secret-value",
		},
		{
			name:       "piped value with CRLF",
			input:      "test-secret-value\r\n",
			isTerminal: false,
			wantValue:  "test-secret-value",
		},
		{
			name:       "empty input",
			input:      "\n",
			isTerminal: false,
			wantErr:    true,
		},
		{
			name:       "EOF with no content",
			input:      "",
			isTerminal: false,
			wantErr:    true,
		},
		{
			name:       "terminal input with newline",
			input:      "my-api-key\n",
			isTerminal: true,
			wantValue:  "my-api-key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdinReader = strings.NewReader(tt.input)
			stdinIsTerminal = func() bool { return tt.isTerminal }

			value, err := readSecretFromStdin("test_key")
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if value != tt.wantValue {
				t.Errorf("got %q, want %q", value, tt.wantValue)
			}
		})
	}
}

func TestReadSecretFromStdinReadError(t *testing.T) {
	origReader := stdinReader
	origIsTerminal := stdinIsTerminal
	defer func() {
		stdinReader = origReader
		stdinIsTerminal = origIsTerminal
	}()

	stdinReader = &errorReader{}
	stdinIsTerminal = func() bool { return false }

	_, err := readSecretFromStdin("test_key")
	if err == nil {
		t.Error("expected error from broken reader")
	}
	if !strings.Contains(err.Error(), "failed to read from stdin") {
		t.Errorf("expected 'failed to read from stdin' in error, got %q", err.Error())
	}
}

// errorReader always returns an error on Read.
type errorReader struct{}

func (e *errorReader) Read(p []byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
