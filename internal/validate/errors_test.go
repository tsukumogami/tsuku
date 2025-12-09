package validate

import (
	"strings"
	"testing"
)

func TestParseValidationErrorBinaryNotFound(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
		stdout string
	}{
		{
			name:   "command not found",
			stderr: "bash: mytool: command not found",
		},
		{
			name:   "not found suffix",
			stderr: "mytool: not found\n", // Must be at end of line
		},
		{
			name:   "no such file or directory",
			stderr: "/usr/bin/mytool: no such file or directory",
		},
		{
			name:   "cannot find",
			stderr: "cannot find: mytool",
		},
		{
			name:   "executable file not found",
			stderr: "exec: executable file not found in $PATH",
		},
		{
			name:   "exec not found",
			stderr: "exec: \"mytool\": not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseValidationError(tt.stdout, tt.stderr, 127)

			if result.Category != ErrorBinaryNotFound {
				t.Errorf("expected category %v, got %v", ErrorBinaryNotFound, result.Category)
			}
			if len(result.Suggestions) == 0 {
				t.Error("expected suggestions, got none")
			}
		})
	}
}

func TestParseValidationErrorExtractionFailed(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
	}{
		{
			name:   "tar error",
			stderr: "tar: Error opening archive: Failed to open 'file.tar.gz'",
		},
		{
			name:   "tar cannot",
			stderr: "tar: cannot open: No such file or directory",
		},
		{
			name:   "unzip error",
			stderr: "unzip: error extracting file.zip",
		},
		{
			name:   "gzip error",
			stderr: "gzip: stdin: not in gzip format",
		},
		{
			name:   "not in gzip format",
			stderr: "not in gzip format",
		},
		{
			name:   "invalid tar header",
			stderr: "archive/tar: invalid tar header",
		},
		{
			name:   "unexpected EOF",
			stderr: "unexpected EOF while reading archive",
		},
		{
			name:   "corrupt archive",
			stderr: "archive is corrupt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseValidationError("", tt.stderr, 1)

			if result.Category != ErrorExtractionFailed {
				t.Errorf("expected category %v, got %v", ErrorExtractionFailed, result.Category)
			}
			if len(result.Suggestions) == 0 {
				t.Error("expected suggestions, got none")
			}
		})
	}
}

func TestParseValidationErrorVerifyFailed(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
	}{
		{
			name:   "verification failed",
			stderr: "verification failed: checksum does not match",
		},
		{
			name:   "checksum mismatch",
			stderr: "checksum mismatch for downloaded file",
		},
		{
			name:   "sha256 mismatch",
			stderr: "sha256 mismatch: expected abc123 got def456",
		},
		{
			name:   "signature invalid",
			stderr: "signature invalid for package",
		},
		{
			name:   "gpg failed",
			stderr: "gpg: verification failed",
		},
		{
			name:   "expected got",
			stderr: "expected 'abc123' got 'def456'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseValidationError("", tt.stderr, 1)

			if result.Category != ErrorVerifyFailed {
				t.Errorf("expected category %v, got %v", ErrorVerifyFailed, result.Category)
			}
			if len(result.Suggestions) == 0 {
				t.Error("expected suggestions, got none")
			}
		})
	}
}

func TestParseValidationErrorPermissionDenied(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
	}{
		{
			name:   "permission denied",
			stderr: "/usr/local/bin/mytool: Permission denied",
		},
		{
			name:   "operation not permitted",
			stderr: "Operation not permitted",
		},
		{
			name:   "access denied",
			stderr: "Access denied to file",
		},
		{
			name:   "EACCES",
			stderr: "EACCES: permission denied, open '/file'",
		},
		{
			name:   "cannot write",
			stderr: "cannot write to /etc/config",
		},
		{
			name:   "read-only file system",
			stderr: "Read-only file system",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseValidationError("", tt.stderr, 1)

			if result.Category != ErrorPermissionDenied {
				t.Errorf("expected category %v, got %v", ErrorPermissionDenied, result.Category)
			}
			if len(result.Suggestions) == 0 {
				t.Error("expected suggestions, got none")
			}
		})
	}
}

func TestParseValidationErrorDownloadFailed(t *testing.T) {
	tests := []struct {
		name   string
		stderr string
	}{
		{
			name:   "download failed",
			stderr: "download failed: network error",
		},
		{
			name:   "connection refused",
			stderr: "Connection refused",
		},
		{
			name:   "connection timed out",
			stderr: "Connection timed out",
		},
		{
			name:   "could not resolve host",
			stderr: "Could not resolve host: example.com",
		},
		{
			name:   "404 not found",
			stderr: "404 Not Found",
		},
		{
			name:   "curl error",
			stderr: "curl: (7) error connecting to host",
		},
		{
			name:   "wget error",
			stderr: "wget: error downloading file",
		},
		{
			name:   "HTTP 500",
			stderr: "HTTP 500 Internal Server Error",
		},
		{
			name:   "HTTP 403",
			stderr: "HTTP 403 Forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ParseValidationError("", tt.stderr, 1)

			if result.Category != ErrorDownloadFailed {
				t.Errorf("expected category %v, got %v", ErrorDownloadFailed, result.Category)
			}
			if len(result.Suggestions) == 0 {
				t.Error("expected suggestions, got none")
			}
		})
	}
}

func TestParseValidationErrorUnknown(t *testing.T) {
	result := ParseValidationError("stdout content", "some generic error", 42)

	if result.Category != ErrorUnknown {
		t.Errorf("expected category %v, got %v", ErrorUnknown, result.Category)
	}
	if result.Details["exit_code"] != "42" {
		t.Errorf("expected exit_code '42', got %q", result.Details["exit_code"])
	}
	if len(result.Suggestions) == 0 {
		t.Error("expected suggestions, got none")
	}
}

func TestParseValidationErrorEmptyOutput(t *testing.T) {
	result := ParseValidationError("", "", 1)

	if result.Category != ErrorUnknown {
		t.Errorf("expected category %v, got %v", ErrorUnknown, result.Category)
	}
	if result.Message != "No error output captured" {
		t.Errorf("expected 'No error output captured', got %q", result.Message)
	}
}

func TestParseValidationErrorPriority(t *testing.T) {
	// When multiple patterns could match, the first matched category wins
	// Binary not found should be checked before others
	stderr := "command not found: mytool\npermission denied"
	result := ParseValidationError("", stderr, 127)

	if result.Category != ErrorBinaryNotFound {
		t.Errorf("expected category %v (first match), got %v", ErrorBinaryNotFound, result.Category)
	}
}

func TestParseValidationErrorMessageTruncation(t *testing.T) {
	// Generate very long output
	longOutput := strings.Repeat("x", 1000)
	result := ParseValidationError(longOutput, "", 1)

	if len(result.Message) > 550 { // 500 + some overhead for "..."
		t.Errorf("expected message to be truncated, got length %d", len(result.Message))
	}
}

func TestParseValidationErrorStdoutFallback(t *testing.T) {
	// Error in stdout should also be detected
	result := ParseValidationError("command not found: mytool", "", 127)

	if result.Category != ErrorBinaryNotFound {
		t.Errorf("expected category %v from stdout, got %v", ErrorBinaryNotFound, result.Category)
	}
}

func TestErrorCategoryStrings(t *testing.T) {
	categories := []ErrorCategory{
		ErrorBinaryNotFound,
		ErrorExtractionFailed,
		ErrorVerifyFailed,
		ErrorPermissionDenied,
		ErrorDownloadFailed,
		ErrorUnknown,
	}

	for _, cat := range categories {
		if string(cat) == "" {
			t.Errorf("category %v has empty string value", cat)
		}
	}
}

func TestParsedErrorHasSuggestions(t *testing.T) {
	// Every category should have suggestions
	categories := map[ErrorCategory]string{
		ErrorBinaryNotFound:   "command not found: test",
		ErrorExtractionFailed: "tar: Error opening archive",
		ErrorVerifyFailed:     "verification failed",
		ErrorPermissionDenied: "permission denied",
		ErrorDownloadFailed:   "download failed",
		ErrorUnknown:          "random error",
	}

	for category, stderr := range categories {
		result := ParseValidationError("", stderr, 1)
		if len(result.Suggestions) == 0 {
			t.Errorf("category %v should have suggestions", category)
		}
	}
}
