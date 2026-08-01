package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/tsukumogami/tsuku/internal/sandbox"
)

func TestSandboxJSONOutput_Serialization(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		output   sandboxJSONOutput
		wantNull bool // whether error field should be null
	}{
		{
			name: "passed result",
			output: sandboxJSONOutput{
				Tool:            "ruff",
				Passed:          true,
				Verified:        true,
				InstallExitCode: 0,
				VerifyExitCode:  0,
				DurationMs:      4523,
				Error:           nil,
			},
			wantNull: true,
		},
		{
			name: "failed result",
			output: sandboxJSONOutput{
				Tool:            "ruff",
				Passed:          false,
				Verified:        false,
				InstallExitCode: 1,
				VerifyExitCode:  -1,
				DurationMs:      1234,
				Error:           strPtr("installation failed with exit code 1"),
			},
			wantNull: false,
		},
		{
			name: "skipped result",
			output: sandboxJSONOutput{
				Tool:            "ruff",
				Passed:          false,
				Verified:        false,
				InstallExitCode: -1,
				VerifyExitCode:  -1,
				DurationMs:      5,
				Error:           strPtr("no container runtime available (install Podman or Docker)"),
			},
			wantNull: false,
		},
		{
			name: "passed but verify failed",
			output: sandboxJSONOutput{
				Tool:            "ruff",
				Passed:          true,
				Verified:        false,
				InstallExitCode: 0,
				VerifyExitCode:  1,
				DurationMs:      3000,
				Error:           nil,
			},
			wantNull: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(tt.output)
			if err != nil {
				t.Fatalf("Failed to marshal: %v", err)
			}

			// Parse back to verify field names and types
			var parsed map[string]interface{}
			if err := json.Unmarshal(data, &parsed); err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			// Check required fields exist
			requiredFields := []string{"tool", "passed", "verified", "install_exit_code", "verify_exit_code", "duration_ms", "error"}
			for _, field := range requiredFields {
				if _, ok := parsed[field]; !ok {
					t.Errorf("Missing required field %q in JSON output", field)
				}
			}

			// Check error null/non-null
			if tt.wantNull {
				if parsed["error"] != nil {
					t.Errorf("Expected error to be null, got %v", parsed["error"])
				}
			} else {
				if parsed["error"] == nil {
					t.Error("Expected error to be non-null")
				}
			}

			// Check tool name preserved
			if parsed["tool"] != tt.output.Tool {
				t.Errorf("tool = %v, want %v", parsed["tool"], tt.output.Tool)
			}

			// Check passed boolean
			if parsed["passed"] != tt.output.Passed {
				t.Errorf("passed = %v, want %v", parsed["passed"], tt.output.Passed)
			}

			// Check verified boolean
			if parsed["verified"] != tt.output.Verified {
				t.Errorf("verified = %v, want %v", parsed["verified"], tt.output.Verified)
			}

			// Check duration_ms is a number
			if _, ok := parsed["duration_ms"].(float64); !ok {
				t.Errorf("duration_ms should be a number, got %T", parsed["duration_ms"])
			}
		})
	}
}

func TestSandboxJSONOutput_ErrorFieldNullEncoding(t *testing.T) {
	t.Parallel()

	// Verify that nil Error pointer serializes to JSON null (not omitted)
	out := sandboxJSONOutput{
		Tool:            "test-tool",
		Passed:          true,
		Verified:        true,
		InstallExitCode: 0,
		VerifyExitCode:  0,
		DurationMs:      100,
		Error:           nil,
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	// The error field should be present as null, not omitted
	if !strings.Contains(jsonStr, `"error":null`) && !strings.Contains(jsonStr, `"error": null`) {
		t.Errorf("Expected JSON to contain error:null, got %s", jsonStr)
	}
}

func TestSandboxJSONOutput_ErrorFieldStringEncoding(t *testing.T) {
	t.Parallel()

	errMsg := "something went wrong"
	out := sandboxJSONOutput{
		Tool:            "test-tool",
		Passed:          false,
		Verified:        false,
		InstallExitCode: 1,
		VerifyExitCode:  -1,
		DurationMs:      200,
		Error:           &errMsg,
	}

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	jsonStr := string(data)
	if !strings.Contains(jsonStr, `"error":"something went wrong"`) && !strings.Contains(jsonStr, `"error": "something went wrong"`) {
		t.Errorf("Expected JSON to contain error string, got %s", jsonStr)
	}
}

func TestEmitSandboxJSON_PassedResult(t *testing.T) {
	t.Parallel()

	result := &sandbox.SandboxResult{
		Passed:         true,
		ExitCode:       0,
		Verified:       true,
		VerifyExitCode: 0,
		DurationMs:     5000,
	}

	out := buildSandboxJSONOutput("ruff", result, nil)

	if out.Tool != "ruff" {
		t.Errorf("Tool = %q, want %q", out.Tool, "ruff")
	}
	if !out.Passed {
		t.Error("Passed should be true")
	}
	if !out.Verified {
		t.Error("Verified should be true")
	}
	if out.InstallExitCode != 0 {
		t.Errorf("InstallExitCode = %d, want 0", out.InstallExitCode)
	}
	if out.VerifyExitCode != 0 {
		t.Errorf("VerifyExitCode = %d, want 0", out.VerifyExitCode)
	}
	if out.DurationMs != 5000 {
		t.Errorf("DurationMs = %d, want 5000", out.DurationMs)
	}
	if out.Error != nil {
		t.Errorf("Error should be nil, got %q", *out.Error)
	}
}

func TestEmitSandboxJSON_FailedResult(t *testing.T) {
	t.Parallel()

	result := &sandbox.SandboxResult{
		Passed:         false,
		ExitCode:       1,
		Verified:       false,
		VerifyExitCode: -1,
		DurationMs:     3000,
	}

	out := buildSandboxJSONOutput("ruff", result, nil)

	if out.Passed {
		t.Error("Passed should be false")
	}
	if out.Error == nil {
		t.Fatal("Error should be non-nil for failed result")
	}
	if !strings.Contains(*out.Error, "exit code 1") {
		t.Errorf("Error should mention exit code, got %q", *out.Error)
	}
}

func TestEmitSandboxJSON_SkippedResult(t *testing.T) {
	t.Parallel()

	result := &sandbox.SandboxResult{
		Skipped:    true,
		DurationMs: 10,
	}

	out := buildSandboxJSONOutput("ruff", result, nil)

	if out.Passed {
		t.Error("Passed should be false for skipped result")
	}
	if out.Verified {
		t.Error("Verified should be false for skipped result")
	}
	if out.InstallExitCode != -1 {
		t.Errorf("InstallExitCode = %d, want -1 for skipped", out.InstallExitCode)
	}
	if out.VerifyExitCode != -1 {
		t.Errorf("VerifyExitCode = %d, want -1 for skipped", out.VerifyExitCode)
	}
	if out.Error == nil {
		t.Fatal("Error should be non-nil for skipped result")
	}
	if !strings.Contains(*out.Error, "no container runtime") {
		t.Errorf("Error should mention missing runtime, got %q", *out.Error)
	}
}

func TestEmitSandboxJSON_ErrorResult(t *testing.T) {
	t.Parallel()

	result := &sandbox.SandboxResult{
		Passed:         false,
		ExitCode:       -1,
		Error:          fmt.Errorf("container failed to start"),
		Verified:       false,
		VerifyExitCode: -1,
		DurationMs:     500,
	}

	out := buildSandboxJSONOutput("ruff", result, nil)

	if out.Passed {
		t.Error("Passed should be false for error result")
	}
	if out.Error == nil {
		t.Fatal("Error should be non-nil for error result")
	}
	if *out.Error != "container failed to start" {
		t.Errorf("Error = %q, want %q", *out.Error, "container failed to start")
	}
	if out.DurationMs != 500 {
		t.Errorf("DurationMs = %d, want 500", out.DurationMs)
	}
}

func TestEmitSandboxJSON_PassedNoVerifyCommand(t *testing.T) {
	t.Parallel()

	// When no verify command exists, Verified is true (vacuously) and
	// VerifyExitCode is -1.
	result := &sandbox.SandboxResult{
		Passed:         true,
		ExitCode:       0,
		Verified:       true,
		VerifyExitCode: -1,
		DurationMs:     2000,
	}

	out := buildSandboxJSONOutput("serve", result, nil)

	if !out.Verified {
		t.Error("Verified should be true when no verify command exists")
	}
	if out.VerifyExitCode != -1 {
		t.Errorf("VerifyExitCode = %d, want -1", out.VerifyExitCode)
	}
	if out.Error != nil {
		t.Errorf("Error should be nil on success, got %q", *out.Error)
	}
}

func TestEmitSandboxJSON_AllFieldsRoundTrip(t *testing.T) {
	t.Parallel()

	// Test that JSON output round-trips correctly through marshal/unmarshal
	errMsg := "test error"
	original := sandboxJSONOutput{
		Tool:            "my-tool",
		Passed:          false,
		Verified:        false,
		InstallExitCode: 127,
		VerifyExitCode:  -1,
		InstallOutput:   "cargo: not found",
		VerifyOutput:    "my-tool: command not found",
		DurationMs:      99999,
		Error:           &errMsg,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	var roundTripped sandboxJSONOutput
	if err := json.Unmarshal(data, &roundTripped); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	if roundTripped.Tool != original.Tool {
		t.Errorf("Tool = %q, want %q", roundTripped.Tool, original.Tool)
	}
	if roundTripped.Passed != original.Passed {
		t.Errorf("Passed = %v, want %v", roundTripped.Passed, original.Passed)
	}
	if roundTripped.Verified != original.Verified {
		t.Errorf("Verified = %v, want %v", roundTripped.Verified, original.Verified)
	}
	if roundTripped.InstallExitCode != original.InstallExitCode {
		t.Errorf("InstallExitCode = %d, want %d", roundTripped.InstallExitCode, original.InstallExitCode)
	}
	if roundTripped.VerifyExitCode != original.VerifyExitCode {
		t.Errorf("VerifyExitCode = %d, want %d", roundTripped.VerifyExitCode, original.VerifyExitCode)
	}
	if roundTripped.InstallOutput != original.InstallOutput {
		t.Errorf("InstallOutput = %q, want %q", roundTripped.InstallOutput, original.InstallOutput)
	}
	if roundTripped.VerifyOutput != original.VerifyOutput {
		t.Errorf("VerifyOutput = %q, want %q", roundTripped.VerifyOutput, original.VerifyOutput)
	}
	if roundTripped.DurationMs != original.DurationMs {
		t.Errorf("DurationMs = %d, want %d", roundTripped.DurationMs, original.DurationMs)
	}
	if roundTripped.Error == nil || *roundTripped.Error != *original.Error {
		t.Errorf("Error = %v, want %v", roundTripped.Error, original.Error)
	}
}

func TestBuildSandboxJSONOutput_InstallFailureIncludesOutput(t *testing.T) {
	t.Parallel()

	result := &sandbox.SandboxResult{
		Passed:         false,
		ExitCode:       6,
		Stdout:         "    AR libgit.a\n    CARGO target/release/libgitcore.a\n",
		Stderr:         "/bin/sh: 1: cargo: not found\nmake: *** [Makefile:3021: target/release/libgitcore.a] Error 127\n",
		Verified:       false,
		VerifyExitCode: -1,
		DurationMs:     420000,
	}

	out := buildSandboxJSONOutput("git-source", result, nil)

	if out.InstallOutput == "" {
		t.Fatal("InstallOutput should be populated when the install fails")
	}
	for _, want := range []string{"CARGO target/release/libgitcore.a", "cargo: not found", "Error 127"} {
		if !strings.Contains(out.InstallOutput, want) {
			t.Errorf("InstallOutput missing %q, got:\n%s", want, out.InstallOutput)
		}
	}
}

func TestBuildSandboxJSONOutput_InstallOutputSerializesUnderExpectedKey(t *testing.T) {
	t.Parallel()

	// build-essentials.yml reads `.install_output` with jq. Renaming the tag
	// would compile and pass every struct-level test while silently returning
	// the job log to exit-code-only.
	out := buildSandboxJSONOutput("git-source", &sandbox.SandboxResult{
		ExitCode: 6,
		Stderr:   "cargo: not found\n",
	}, nil)

	data, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("Unmarshal failed: %v", err)
	}

	got, ok := parsed["install_output"].(string)
	if !ok {
		t.Fatalf("install_output missing or not a string in %s", data)
	}
	if !strings.Contains(got, "cargo: not found") {
		t.Errorf("install_output = %q, want the container output", got)
	}
}

func TestBuildSandboxJSONOutput_RunErrorIncludesOutput(t *testing.T) {
	t.Parallel()

	result := &sandbox.SandboxResult{
		Passed:         false,
		ExitCode:       -1,
		Stderr:         "Error: image pull failed\n",
		Error:          fmt.Errorf("container execution failed"),
		Verified:       false,
		VerifyExitCode: -1,
		DurationMs:     500,
	}

	out := buildSandboxJSONOutput("ruff", result, nil)

	if !strings.Contains(out.InstallOutput, "image pull failed") {
		t.Errorf("InstallOutput should carry container output on run error, got %q", out.InstallOutput)
	}
}

func TestBuildSandboxJSONOutput_PassedOmitsOutput(t *testing.T) {
	t.Parallel()

	result := &sandbox.SandboxResult{
		Passed:         true,
		ExitCode:       0,
		Stdout:         "Installed ruff 0.1.0\n",
		Verified:       true,
		VerifyExitCode: 0,
		DurationMs:     4000,
	}

	out := buildSandboxJSONOutput("ruff", result, nil)

	if out.InstallOutput != "" {
		t.Errorf("InstallOutput should be empty on success, got %q", out.InstallOutput)
	}
}

func TestBuildSandboxJSONOutput_SkippedOmitsOutput(t *testing.T) {
	t.Parallel()

	result := &sandbox.SandboxResult{
		Skipped:    true,
		DurationMs: 10,
	}

	out := buildSandboxJSONOutput("ruff", result, nil)

	if out.InstallOutput != "" {
		t.Errorf("InstallOutput should be empty when the sandbox is skipped, got %q", out.InstallOutput)
	}
}

func TestBuildSandboxJSONOutput_VerifyFailureOmitsInstallOutput(t *testing.T) {
	t.Parallel()

	// The install succeeded, so the build log is noise -- verify_output is the
	// field that matters here.
	result := &sandbox.SandboxResult{
		Passed:         false,
		ExitCode:       0,
		Stdout:         "Installed ruff 0.1.0\n",
		Verified:       false,
		VerifyExitCode: 1,
		VerifyOutput:   "ruff: command not found\n",
		DurationMs:     4000,
	}

	out := buildSandboxJSONOutput("ruff", result, nil)

	if out.InstallOutput != "" {
		t.Errorf("InstallOutput should be empty when only verification failed, got %q", out.InstallOutput)
	}
	if !strings.Contains(out.VerifyOutput, "command not found") {
		t.Errorf("VerifyOutput = %q, want it to carry the verify failure", out.VerifyOutput)
	}
}

func TestSandboxFailureOutput_KeepsTailAndMarksTruncation(t *testing.T) {
	t.Parallel()

	// A build log far larger than the cap: the failing command is at the end,
	// so the tail is what must survive.
	var sb strings.Builder
	for i := 0; i < 20000; i++ {
		fmt.Fprintf(&sb, "    CC builtin/file-%d.o\n", i)
	}
	sb.WriteString("cargo: not found\n")

	got := sandboxFailureOutput(&sandbox.SandboxResult{Stdout: sb.String()}, nil)

	if want := maxSandboxOutputBytes + len(sandboxTruncationNotice) + 1; len(got) > want {
		t.Errorf("output length %d exceeds the cap plus the notice (%d)", len(got), want)
	}
	if !strings.Contains(got, "cargo: not found") {
		t.Error("truncation dropped the tail, which is where the failing command is")
	}
	if !strings.Contains(got, sandboxTruncationNotice) {
		t.Errorf("truncated output should say so, got prefix %q", got[:80])
	}
	if strings.Contains(got, "file-0.o") {
		t.Error("head of the log should have been dropped, not the tail")
	}
}

func TestSandboxFailureOutput_ShortOutputNotTruncated(t *testing.T) {
	t.Parallel()

	got := sandboxFailureOutput(&sandbox.SandboxResult{Stderr: "make: *** Error 2\n"}, nil)

	if strings.Contains(got, sandboxTruncationNotice) {
		t.Errorf("short output should not be marked truncated, got %q", got)
	}
	if got != "make: *** Error 2" {
		t.Errorf("got %q, want the trimmed stderr", got)
	}
}

func TestSandboxFailureOutput_MasksSecrets(t *testing.T) {
	t.Parallel()

	// The Build Essentials workflow forwards GITHUB_TOKEN into the sandbox and
	// publishes this JSON to a public job log.
	result := &sandbox.SandboxResult{
		Stdout: "fetching with token=ghp_abcdef1234567890\n",
		Stderr: "Authorization: Bearer ghp_abcdef1234567890\n",
	}

	got := sandboxFailureOutput(result, []string{"ghp_abcdef1234567890"})

	if strings.Contains(got, "ghp_abcdef1234567890") {
		t.Errorf("secret survived masking: %q", got)
	}
	if strings.Count(got, "[REDACTED]") != 2 {
		t.Errorf("both occurrences should be masked, got %q", got)
	}
}

func TestSandboxFailureOutput_MasksSecretsAnywhereInTheLine(t *testing.T) {
	t.Parallel()

	// A token most often escapes inside a clone URL or a bare echo, not as a
	// tidy KEY=VALUE pair. Exact-value masking catches it either way.
	secret := "ghp_abcdef1234567890"
	result := &sandbox.SandboxResult{
		Stdout: "cloning https://x-access-token:" + secret + "@github.com/o/r\n",
		Stderr: secret + "\n",
	}

	got := sandboxFailureOutput(result, []string{secret})

	if strings.Contains(got, secret) {
		t.Errorf("secret survived masking: %q", got)
	}
	if !strings.Contains(got, "https://x-access-token:[REDACTED]@github.com/o/r") {
		t.Errorf("masking should leave the rest of the URL readable, got %q", got)
	}
}

func TestBuildSandboxJSONOutput_VerifyOutputIsMasked(t *testing.T) {
	t.Parallel()

	// verify_output is printed in the same job log as install_output, so it
	// gets the same treatment.
	secret := "ghp_abcdef1234567890"
	result := &sandbox.SandboxResult{
		Passed:         false,
		ExitCode:       0,
		Verified:       false,
		VerifyExitCode: 1,
		VerifyOutput:   "auth failed for " + secret + "\n",
	}

	out := buildSandboxJSONOutput("ruff", result, []string{secret})

	if strings.Contains(out.VerifyOutput, secret) {
		t.Errorf("secret survived masking in VerifyOutput: %q", out.VerifyOutput)
	}
	if !strings.Contains(out.VerifyOutput, "auth failed for [REDACTED]") {
		t.Errorf("VerifyOutput = %q, want the masked message", out.VerifyOutput)
	}
}

func TestSandboxFailureOutput_MasksBeforeTruncating(t *testing.T) {
	t.Parallel()

	// A secret near the head would otherwise be cut mid-string by truncation,
	// leaving a fragment that exact-match masking can no longer find.
	secret := strings.Repeat("s3cr3t", 8)
	var sb strings.Builder
	sb.WriteString("leaked " + secret + "\n")
	for i := 0; i < 6000; i++ {
		fmt.Fprintf(&sb, "    CC builtin/file-%d.o\n", i)
	}

	got := sandboxFailureOutput(&sandbox.SandboxResult{Stdout: sb.String()}, []string{secret})

	if strings.Contains(got, "s3cr3t") {
		t.Errorf("a fragment of the secret survived: %q", got[:200])
	}
}

func TestSandboxFailureOutput_LeavesBuildDiagnosticsIntact(t *testing.T) {
	t.Parallel()

	// Regression guard: pattern-based redaction used to rewrite "foo.c:12:3:"
	// to "foo.c[IP]3:" and "unexpected token: X" to "unexpected [REDACTED]",
	// destroying the diagnostics this field exists to carry.
	log := strings.Join([]string{
		"foo.c:12:3: error: unexpected token: ')'",
		"checking for basic C types... yes",
		"/bin/sh: 1: cargo: not found",
		"make: *** [Makefile:3021: target/release/libgitcore.a] Error 127",
	}, "\n")

	got := sandboxFailureOutput(&sandbox.SandboxResult{Stdout: log}, nil)

	if got != log {
		t.Errorf("build output was altered:\ngot:  %q\nwant: %q", got, log)
	}
}

func TestSandboxSecretValues(t *testing.T) {
	// No t.Parallel: t.Setenv is incompatible with parallel tests.
	t.Setenv("TSUKU_TEST_HOST_SECRET", "host-side-credential")

	got := sandboxSecretValues([]string{
		"GITHUB_TOKEN=ghp_abcdef1234567890",
		"DEBUG=1",     // too short to mask without wrecking the log
		"EMPTY=",      // no value to mask
		"NOEQUALSIGN", // KEY-only, resolves from the host (unset -> empty)
		"TSUKU_TEST_HOST_SECRET",
	})

	want := []string{"ghp_abcdef1234567890", "host-side-credential"}
	if len(got) != len(want) {
		t.Fatalf("got %q, want %q", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestSandboxFailureOutput_NoNewlineTailStaysValidUTF8(t *testing.T) {
	t.Parallel()

	// One enormous line with no newline to cut on: the byte slice can land
	// mid-rune, and invalid UTF-8 would come back from JSON as replacement
	// characters.
	oneLine := strings.Repeat("é", maxSandboxOutputBytes)

	got := sandboxFailureOutput(&sandbox.SandboxResult{Stdout: oneLine}, nil)

	if !utf8.ValidString(got) {
		t.Error("truncated output is not valid UTF-8")
	}
	if !strings.Contains(got, sandboxTruncationNotice) {
		t.Error("expected the truncation notice")
	}
}

func TestSandboxFailureOutput_EmptyWhenNoOutput(t *testing.T) {
	t.Parallel()

	if got := sandboxFailureOutput(&sandbox.SandboxResult{}, nil); got != "" {
		t.Errorf("got %q, want empty string when the container produced nothing", got)
	}
}

func TestSandboxFailureOutput_CombinesStdoutAndStderr(t *testing.T) {
	t.Parallel()

	got := sandboxFailureOutput(&sandbox.SandboxResult{
		Stdout: "step 3/5: configure_make\n",
		Stderr: "make failed\n",
	}, nil)

	if !strings.Contains(got, "step 3/5: configure_make") || !strings.Contains(got, "make failed") {
		t.Errorf("got %q, want both streams", got)
	}
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}
