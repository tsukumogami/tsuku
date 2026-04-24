package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

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

	out := buildSandboxJSONOutput("ruff", result)

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

	out := buildSandboxJSONOutput("ruff", result)

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

	out := buildSandboxJSONOutput("ruff", result)

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

	out := buildSandboxJSONOutput("ruff", result)

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

	out := buildSandboxJSONOutput("serve", result)

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
	if roundTripped.DurationMs != original.DurationMs {
		t.Errorf("DurationMs = %d, want %d", roundTripped.DurationMs, original.DurationMs)
	}
	if roundTripped.Error == nil || *roundTripped.Error != *original.Error {
		t.Errorf("Error = %v, want %v", roundTripped.Error, original.Error)
	}
}

// strPtr returns a pointer to the given string.
func strPtr(s string) *string {
	return &s
}
