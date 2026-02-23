package executor

import "testing"

func TestCheckPlanVerification_ExitCodeMatch_EmptyPattern(t *testing.T) {
	t.Parallel()

	if !CheckPlanVerification(0, "some output", 0, "") {
		t.Error("Expected true when exit code matches and pattern is empty")
	}
}

func TestCheckPlanVerification_ExitCodeMismatch(t *testing.T) {
	t.Parallel()

	if CheckPlanVerification(1, "some output", 0, "") {
		t.Error("Expected false when exit code does not match")
	}
}

func TestCheckPlanVerification_PatternMatch(t *testing.T) {
	t.Parallel()

	if !CheckPlanVerification(0, "ruff 0.4.1", 0, "ruff") {
		t.Error("Expected true when exit code matches and pattern found in output")
	}
}

func TestCheckPlanVerification_PatternMismatch(t *testing.T) {
	t.Parallel()

	if CheckPlanVerification(0, "some other output", 0, "ruff") {
		t.Error("Expected false when exit code matches but pattern not found")
	}
}

func TestCheckPlanVerification_NonDefaultExpectedExitCode(t *testing.T) {
	t.Parallel()

	// Verify command that intentionally exits with code 2
	if !CheckPlanVerification(2, "expected output", 2, "expected") {
		t.Error("Expected true when non-default exit code matches and pattern found")
	}

	// Wrong exit code when expecting non-default
	if CheckPlanVerification(0, "expected output", 2, "expected") {
		t.Error("Expected false when exit code 0 does not match expected 2")
	}
}

func TestCheckPlanVerification_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		verifyExitCode   int
		output           string
		expectedExitCode int
		pattern          string
		want             bool
	}{
		{
			name:             "exit code 0, no pattern",
			verifyExitCode:   0,
			output:           "",
			expectedExitCode: 0,
			pattern:          "",
			want:             true,
		},
		{
			name:             "exit code 0, pattern found",
			verifyExitCode:   0,
			output:           "tool v1.2.3",
			expectedExitCode: 0,
			pattern:          "v1.2.3",
			want:             true,
		},
		{
			name:             "exit code 0, pattern not found",
			verifyExitCode:   0,
			output:           "tool v1.2.3",
			expectedExitCode: 0,
			pattern:          "v2.0.0",
			want:             false,
		},
		{
			name:             "exit code mismatch, no pattern",
			verifyExitCode:   1,
			output:           "",
			expectedExitCode: 0,
			pattern:          "",
			want:             false,
		},
		{
			name:             "exit code mismatch, pattern would match",
			verifyExitCode:   1,
			output:           "tool v1.2.3",
			expectedExitCode: 0,
			pattern:          "v1.2.3",
			want:             false,
		},
		{
			name:             "non-zero expected, matching",
			verifyExitCode:   42,
			output:           "some output",
			expectedExitCode: 42,
			pattern:          "some",
			want:             true,
		},
		{
			name:             "pattern in multiline output",
			verifyExitCode:   0,
			output:           "line1\nline2\ntool v1.0\nline4",
			expectedExitCode: 0,
			pattern:          "tool v1.0",
			want:             true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CheckPlanVerification(tt.verifyExitCode, tt.output, tt.expectedExitCode, tt.pattern)
			if got != tt.want {
				t.Errorf("CheckPlanVerification(%d, %q, %d, %q) = %v, want %v",
					tt.verifyExitCode, tt.output, tt.expectedExitCode, tt.pattern, got, tt.want)
			}
		})
	}
}
