package discover

import (
	"bytes"
	"errors"
	"testing"
)

func TestNotFoundError(t *testing.T) {
	err := &NotFoundError{Tool: "mytool"}
	if err.Error() != "could not find 'mytool'" {
		t.Errorf("Error() = %q", err.Error())
	}
	suggestion := err.Suggestion()
	if suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestConfigurationError(t *testing.T) {
	tests := []struct {
		reason      string
		wantErr     string
		wantSuggest string
	}{
		{"no_api_key", "no match for", "ANTHROPIC_API_KEY"},
		{"deterministic_only", "no deterministic source", "--deterministic-only"},
		{"other", "configuration error", ""},
	}

	for _, tt := range tests {
		t.Run(tt.reason, func(t *testing.T) {
			err := &ConfigurationError{Tool: "mytool", Reason: tt.reason}
			errStr := err.Error()
			if !bytes.Contains([]byte(errStr), []byte(tt.wantErr)) {
				t.Errorf("Error() = %q, want to contain %q", errStr, tt.wantErr)
			}
			suggest := err.Suggestion()
			if tt.wantSuggest != "" && !bytes.Contains([]byte(suggest), []byte(tt.wantSuggest)) {
				t.Errorf("Suggestion() = %q, want to contain %q", suggest, tt.wantSuggest)
			}
		})
	}
}

func TestBuilderRequiresLLMError(t *testing.T) {
	err := &BuilderRequiresLLMError{Tool: "mytool", Builder: "github", Source: "owner/repo"}
	errStr := err.Error()
	if errStr == "" {
		t.Error("expected non-empty error message")
	}
	suggestion := err.Suggestion()
	if suggestion == "" {
		t.Error("expected non-empty suggestion")
	}
}

func TestAmbiguousMatchError_Error(t *testing.T) {
	err := &AmbiguousMatchError{
		Tool: "serve",
		Matches: []DiscoveryMatch{
			{Builder: "npm", Source: "serve"},
			{Builder: "crates.io", Source: "serve"},
		},
	}
	errStr := err.Error()
	if !bytes.Contains([]byte(errStr), []byte("Multiple sources")) {
		t.Errorf("Error() = %q, want to contain 'Multiple sources'", errStr)
	}
	if !bytes.Contains([]byte(errStr), []byte("--from")) {
		t.Errorf("Error() = %q, want to contain '--from'", errStr)
	}
}

func TestIsFatalError(t *testing.T) {
	t.Run("ambiguous is fatal", func(t *testing.T) {
		err := &AmbiguousMatchError{Tool: "x"}
		if !isFatalError(err) {
			t.Error("AmbiguousMatchError should be fatal")
		}
	})

	t.Run("generic error is not fatal", func(t *testing.T) {
		err := errors.New("generic")
		if isFatalError(err) {
			t.Error("generic error should not be fatal")
		}
	})
}
