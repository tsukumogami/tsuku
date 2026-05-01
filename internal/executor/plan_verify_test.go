package executor

import "testing"

func TestCheckPlanVerification_ExitCodeMatch_EmptyPattern(t *testing.T) {
	t.Parallel()

	if !CheckPlanVerification(0, "some output", 0, nil) {
		t.Error("Expected true when exit code matches and patterns is empty")
	}
}

func TestCheckPlanVerification_ExitCodeMismatch(t *testing.T) {
	t.Parallel()

	if CheckPlanVerification(1, "some output", 0, nil) {
		t.Error("Expected false when exit code does not match")
	}
}

func TestCheckPlanVerification_PatternMatch(t *testing.T) {
	t.Parallel()

	if !CheckPlanVerification(0, "ruff 0.4.1", 0, []string{"ruff"}) {
		t.Error("Expected true when exit code matches and pattern found in output")
	}
}

func TestCheckPlanVerification_PatternMismatch(t *testing.T) {
	t.Parallel()

	if CheckPlanVerification(0, "some other output", 0, []string{"ruff"}) {
		t.Error("Expected false when exit code matches but pattern not found")
	}
}

func TestCheckPlanVerification_NonDefaultExpectedExitCode(t *testing.T) {
	t.Parallel()

	// Verify command that intentionally exits with code 2
	if !CheckPlanVerification(2, "expected output", 2, []string{"expected"}) {
		t.Error("Expected true when non-default exit code matches and pattern found")
	}

	// Wrong exit code when expecting non-default
	if CheckPlanVerification(0, "expected output", 2, []string{"expected"}) {
		t.Error("Expected false when exit code 0 does not match expected 2")
	}
}

func TestCheckPlanVerification_MultiPatternAllMatch(t *testing.T) {
	t.Parallel()

	output := "openjdk 25.0.3\nOpenJDK Runtime Environment Microsoft-13877136 (build 25.0.3+9-LTS)"
	if !CheckPlanVerification(0, output, 0, []string{"Microsoft", "openjdk 25"}) {
		t.Error("Expected true when both vendor and version patterns match")
	}
}

func TestCheckPlanVerification_MultiPatternOneMissing(t *testing.T) {
	t.Parallel()

	output := "openjdk 25.0.3\nOpenJDK Runtime Environment Microsoft-13877136"
	// Wrong major — should fail.
	if CheckPlanVerification(0, output, 0, []string{"Microsoft", "openjdk 26"}) {
		t.Error("Expected false when one of multiple patterns is missing")
	}
}

func TestCheckPlanVerification_TableDriven(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name             string
		verifyExitCode   int
		output           string
		expectedExitCode int
		patterns         []string
		want             bool
	}{
		{
			name:             "exit code 0, no patterns",
			verifyExitCode:   0,
			output:           "",
			expectedExitCode: 0,
			patterns:         nil,
			want:             true,
		},
		{
			name:             "exit code 0, pattern found",
			verifyExitCode:   0,
			output:           "tool v1.2.3",
			expectedExitCode: 0,
			patterns:         []string{"v1.2.3"},
			want:             true,
		},
		{
			name:             "exit code 0, pattern not found",
			verifyExitCode:   0,
			output:           "tool v1.2.3",
			expectedExitCode: 0,
			patterns:         []string{"v2.0.0"},
			want:             false,
		},
		{
			name:             "exit code mismatch, no patterns",
			verifyExitCode:   1,
			output:           "",
			expectedExitCode: 0,
			patterns:         nil,
			want:             false,
		},
		{
			name:             "exit code mismatch, pattern would match",
			verifyExitCode:   1,
			output:           "tool v1.2.3",
			expectedExitCode: 0,
			patterns:         []string{"v1.2.3"},
			want:             false,
		},
		{
			name:             "non-zero expected, matching",
			verifyExitCode:   42,
			output:           "some output",
			expectedExitCode: 42,
			patterns:         []string{"some"},
			want:             true,
		},
		{
			name:             "pattern in multiline output",
			verifyExitCode:   0,
			output:           "line1\nline2\ntool v1.0\nline4",
			expectedExitCode: 0,
			patterns:         []string{"tool v1.0"},
			want:             true,
		},
		{
			name:             "two patterns both match",
			verifyExitCode:   0,
			output:           "vendor X version 1.0",
			expectedExitCode: 0,
			patterns:         []string{"vendor X", "version 1.0"},
			want:             true,
		},
		{
			name:             "two patterns, second missing",
			verifyExitCode:   0,
			output:           "vendor X version 1.0",
			expectedExitCode: 0,
			patterns:         []string{"vendor X", "version 2.0"},
			want:             false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := CheckPlanVerification(tt.verifyExitCode, tt.output, tt.expectedExitCode, tt.patterns)
			if got != tt.want {
				t.Errorf("CheckPlanVerification(%d, %q, %d, %v) = %v, want %v",
					tt.verifyExitCode, tt.output, tt.expectedExitCode, tt.patterns, got, tt.want)
			}
		})
	}
}
