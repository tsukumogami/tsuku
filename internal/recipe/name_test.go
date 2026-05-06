package recipe

import "testing"

// TestIsValidRecipeName covers the minimal set of rejections the helper
// enforces: empty, contains '/', '\\', '..', or a null byte. Anything
// else passes — strict validation (e.g., the runtime_dependencies
// pattern) layers on top in the validator.
func TestIsValidRecipeName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"plain", "openssl", true},
		{"underscore", "libatomic_ops", true},
		{"dotted", "python3.11", true},
		{"hyphen", "jpeg-turbo", true},
		{"single_char", "a", true},
		{"empty", "", false},
		{"slash", "foo/bar", false},
		{"backslash", "foo\\bar", false},
		{"path_traversal", "..", false},
		{"path_traversal_embedded", "foo..bar", false},
		{"null_byte", "foo\x00bar", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsValidRecipeName(tt.in); got != tt.want {
				t.Errorf("IsValidRecipeName(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}
