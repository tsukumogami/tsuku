package telemetry

import (
	"errors"
	"runtime"
	"testing"

	"github.com/tsukumogami/tsuku/internal/buildinfo"
)

func TestNewInstallEvent(t *testing.T) {
	e := NewInstallEvent("nodejs", "@LTS", "22.0.0", false)

	if e.Action != "install" {
		t.Errorf("Action = %q, want %q", e.Action, "install")
	}
	if e.Recipe != "nodejs" {
		t.Errorf("Recipe = %q, want %q", e.Recipe, "nodejs")
	}
	if e.VersionConstraint != "@LTS" {
		t.Errorf("VersionConstraint = %q, want %q", e.VersionConstraint, "@LTS")
	}
	if e.VersionResolved != "22.0.0" {
		t.Errorf("VersionResolved = %q, want %q", e.VersionResolved, "22.0.0")
	}
	if e.IsDependency != false {
		t.Errorf("IsDependency = %v, want %v", e.IsDependency, false)
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
	if e.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", e.Arch, runtime.GOARCH)
	}
	if e.TsukuVersion != buildinfo.Version() {
		t.Errorf("TsukuVersion = %q, want %q", e.TsukuVersion, buildinfo.Version())
	}
	if e.SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q, want %q", e.SchemaVersion, "1")
	}
}

func TestNewInstallEvent_Dependency(t *testing.T) {
	e := NewInstallEvent("openssl", "", "3.0.0", true)

	if e.IsDependency != true {
		t.Errorf("IsDependency = %v, want %v", e.IsDependency, true)
	}
	if e.VersionConstraint != "" {
		t.Errorf("VersionConstraint = %q, want empty", e.VersionConstraint)
	}
}

func TestNewUpdateEvent(t *testing.T) {
	e := NewUpdateEvent("nodejs", "20.0.0", "22.0.0")

	if e.Action != "update" {
		t.Errorf("Action = %q, want %q", e.Action, "update")
	}
	if e.Recipe != "nodejs" {
		t.Errorf("Recipe = %q, want %q", e.Recipe, "nodejs")
	}
	if e.VersionPrevious != "20.0.0" {
		t.Errorf("VersionPrevious = %q, want %q", e.VersionPrevious, "20.0.0")
	}
	if e.VersionResolved != "22.0.0" {
		t.Errorf("VersionResolved = %q, want %q", e.VersionResolved, "22.0.0")
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
}

func TestNewRemoveEvent(t *testing.T) {
	e := NewRemoveEvent("nodejs", "22.0.0")

	if e.Action != "remove" {
		t.Errorf("Action = %q, want %q", e.Action, "remove")
	}
	if e.Recipe != "nodejs" {
		t.Errorf("Recipe = %q, want %q", e.Recipe, "nodejs")
	}
	if e.VersionPrevious != "22.0.0" {
		t.Errorf("VersionPrevious = %q, want %q", e.VersionPrevious, "22.0.0")
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
}

func TestNewLLMGenerationStartedEvent(t *testing.T) {
	e := NewLLMGenerationStartedEvent("claude", "serve", "owner/repo")

	if e.Action != "llm_generation_started" {
		t.Errorf("Action = %q, want %q", e.Action, "llm_generation_started")
	}
	if e.Provider != "claude" {
		t.Errorf("Provider = %q, want %q", e.Provider, "claude")
	}
	if e.ToolName != "serve" {
		t.Errorf("ToolName = %q, want %q", e.ToolName, "serve")
	}
	if e.Repo != "owner/repo" {
		t.Errorf("Repo = %q, want %q", e.Repo, "owner/repo")
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
	if e.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", e.Arch, runtime.GOARCH)
	}
	if e.TsukuVersion != buildinfo.Version() {
		t.Errorf("TsukuVersion = %q, want %q", e.TsukuVersion, buildinfo.Version())
	}
	if e.SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q, want %q", e.SchemaVersion, "1")
	}
}

func TestNewLLMGenerationCompletedEvent(t *testing.T) {
	e := NewLLMGenerationCompletedEvent("gemini", "serve", true, 1500, 2)

	if e.Action != "llm_generation_completed" {
		t.Errorf("Action = %q, want %q", e.Action, "llm_generation_completed")
	}
	if e.Provider != "gemini" {
		t.Errorf("Provider = %q, want %q", e.Provider, "gemini")
	}
	if e.ToolName != "serve" {
		t.Errorf("ToolName = %q, want %q", e.ToolName, "serve")
	}
	if e.Success != true {
		t.Errorf("Success = %v, want %v", e.Success, true)
	}
	if e.DurationMs != 1500 {
		t.Errorf("DurationMs = %d, want %d", e.DurationMs, 1500)
	}
	if e.Attempts != 2 {
		t.Errorf("Attempts = %d, want %d", e.Attempts, 2)
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
}

func TestNewLLMRepairAttemptEvent(t *testing.T) {
	e := NewLLMRepairAttemptEvent("claude", 2, "validation_error")

	if e.Action != "llm_repair_attempt" {
		t.Errorf("Action = %q, want %q", e.Action, "llm_repair_attempt")
	}
	if e.Provider != "claude" {
		t.Errorf("Provider = %q, want %q", e.Provider, "claude")
	}
	if e.AttemptNumber != 2 {
		t.Errorf("AttemptNumber = %d, want %d", e.AttemptNumber, 2)
	}
	if e.ErrorCategory != "validation_error" {
		t.Errorf("ErrorCategory = %q, want %q", e.ErrorCategory, "validation_error")
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
}

func TestNewLLMValidationResultEvent(t *testing.T) {
	e := NewLLMValidationResultEvent(false, "toml_parse", 1)

	if e.Action != "llm_validation_result" {
		t.Errorf("Action = %q, want %q", e.Action, "llm_validation_result")
	}
	if e.Passed != false {
		t.Errorf("Passed = %v, want %v", e.Passed, false)
	}
	if e.ErrorCategory != "toml_parse" {
		t.Errorf("ErrorCategory = %q, want %q", e.ErrorCategory, "toml_parse")
	}
	if e.AttemptNumber != 1 {
		t.Errorf("AttemptNumber = %d, want %d", e.AttemptNumber, 1)
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
}

func TestNewLLMCircuitBreakerTripEvent(t *testing.T) {
	e := NewLLMCircuitBreakerTripEvent("claude", 3)

	if e.Action != "llm_circuit_breaker_trip" {
		t.Errorf("Action = %q, want %q", e.Action, "llm_circuit_breaker_trip")
	}
	if e.Provider != "claude" {
		t.Errorf("Provider = %q, want %q", e.Provider, "claude")
	}
	if e.Failures != 3 {
		t.Errorf("Failures = %d, want %d", e.Failures, 3)
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
}

func TestNewVerifySelfRepairEvent(t *testing.T) {
	e := NewVerifySelfRepairEvent("mytool", "output_detection", true)

	if e.Action != "verify_self_repair" {
		t.Errorf("Action = %q, want %q", e.Action, "verify_self_repair")
	}
	if e.ToolName != "mytool" {
		t.Errorf("ToolName = %q, want %q", e.ToolName, "mytool")
	}
	if e.Method != "output_detection" {
		t.Errorf("Method = %q, want %q", e.Method, "output_detection")
	}
	if e.Success != true {
		t.Errorf("Success = %v, want %v", e.Success, true)
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
	if e.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", e.Arch, runtime.GOARCH)
	}
	if e.TsukuVersion != buildinfo.Version() {
		t.Errorf("TsukuVersion = %q, want %q", e.TsukuVersion, buildinfo.Version())
	}
	if e.SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q, want %q", e.SchemaVersion, "1")
	}
}

func TestNewVerifySelfRepairEvent_FallbackHelp(t *testing.T) {
	e := NewVerifySelfRepairEvent("gh", "fallback_help", true)

	if e.Action != "verify_self_repair" {
		t.Errorf("Action = %q, want %q", e.Action, "verify_self_repair")
	}
	if e.ToolName != "gh" {
		t.Errorf("ToolName = %q, want %q", e.ToolName, "gh")
	}
	if e.Method != "fallback_help" {
		t.Errorf("Method = %q, want %q", e.Method, "fallback_help")
	}
	if e.Success != true {
		t.Errorf("Success = %v, want %v", e.Success, true)
	}
}

func TestNewBinaryNameRepairEvent(t *testing.T) {
	e := NewBinaryNameRepairEvent("sqlx-cli", "crates.io", true)

	if e.Action != "binary_name_repair" {
		t.Errorf("Action = %q, want %q", e.Action, "binary_name_repair")
	}
	if e.ToolName != "sqlx-cli" {
		t.Errorf("ToolName = %q, want %q", e.ToolName, "sqlx-cli")
	}
	if e.Builder != "crates.io" {
		t.Errorf("Builder = %q, want %q", e.Builder, "crates.io")
	}
	if e.Success != true {
		t.Errorf("Success = %v, want %v", e.Success, true)
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
	if e.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", e.Arch, runtime.GOARCH)
	}
	if e.TsukuVersion != buildinfo.Version() {
		t.Errorf("TsukuVersion = %q, want %q", e.TsukuVersion, buildinfo.Version())
	}
	if e.SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q, want %q", e.SchemaVersion, "1")
	}
}

func TestNewBinaryNameRepairEvent_NpmBuilder(t *testing.T) {
	e := NewBinaryNameRepairEvent("typescript", "npm", true)

	if e.Action != "binary_name_repair" {
		t.Errorf("Action = %q, want %q", e.Action, "binary_name_repair")
	}
	if e.ToolName != "typescript" {
		t.Errorf("ToolName = %q, want %q", e.ToolName, "typescript")
	}
	if e.Builder != "npm" {
		t.Errorf("Builder = %q, want %q", e.Builder, "npm")
	}
}

func TestNewUpdateOutcomeSuccessEvent(t *testing.T) {
	e := NewUpdateOutcomeSuccessEvent("kubectl", "1.28.0", "1.29.0", "manual")

	if e.Action != "update_outcome_success" {
		t.Errorf("Action = %q, want %q", e.Action, "update_outcome_success")
	}
	if e.Recipe != "kubectl" {
		t.Errorf("Recipe = %q, want %q", e.Recipe, "kubectl")
	}
	if e.VersionPrevious != "1.28.0" {
		t.Errorf("VersionPrevious = %q, want %q", e.VersionPrevious, "1.28.0")
	}
	if e.VersionTarget != "1.29.0" {
		t.Errorf("VersionTarget = %q, want %q", e.VersionTarget, "1.29.0")
	}
	if e.Trigger != "manual" {
		t.Errorf("Trigger = %q, want %q", e.Trigger, "manual")
	}
	if e.ErrorType != "" {
		t.Errorf("ErrorType = %q, want empty", e.ErrorType)
	}
	if e.OS != runtime.GOOS {
		t.Errorf("OS = %q, want %q", e.OS, runtime.GOOS)
	}
	if e.Arch != runtime.GOARCH {
		t.Errorf("Arch = %q, want %q", e.Arch, runtime.GOARCH)
	}
	if e.TsukuVersion != buildinfo.Version() {
		t.Errorf("TsukuVersion = %q, want %q", e.TsukuVersion, buildinfo.Version())
	}
	if e.SchemaVersion != "1" {
		t.Errorf("SchemaVersion = %q, want %q", e.SchemaVersion, "1")
	}
}

func TestNewUpdateOutcomeFailureEvent(t *testing.T) {
	e := NewUpdateOutcomeFailureEvent("terraform", "1.6.0", ErrorTypeDownloadFailed, "auto")

	if e.Action != "update_outcome_failure" {
		t.Errorf("Action = %q, want %q", e.Action, "update_outcome_failure")
	}
	if e.Recipe != "terraform" {
		t.Errorf("Recipe = %q, want %q", e.Recipe, "terraform")
	}
	if e.VersionTarget != "1.6.0" {
		t.Errorf("VersionTarget = %q, want %q", e.VersionTarget, "1.6.0")
	}
	if e.ErrorType != ErrorTypeDownloadFailed {
		t.Errorf("ErrorType = %q, want %q", e.ErrorType, ErrorTypeDownloadFailed)
	}
	if e.Trigger != "auto" {
		t.Errorf("Trigger = %q, want %q", e.Trigger, "auto")
	}
	if e.VersionPrevious != "" {
		t.Errorf("VersionPrevious = %q, want empty", e.VersionPrevious)
	}
}

func TestNewUpdateOutcomeRollbackEvent(t *testing.T) {
	e := NewUpdateOutcomeRollbackEvent("node", "18.0.0", "20.0.0", "auto")

	if e.Action != "update_outcome_rollback" {
		t.Errorf("Action = %q, want %q", e.Action, "update_outcome_rollback")
	}
	if e.Recipe != "node" {
		t.Errorf("Recipe = %q, want %q", e.Recipe, "node")
	}
	if e.VersionPrevious != "18.0.0" {
		t.Errorf("VersionPrevious = %q, want %q", e.VersionPrevious, "18.0.0")
	}
	if e.VersionTarget != "20.0.0" {
		t.Errorf("VersionTarget = %q, want %q", e.VersionTarget, "20.0.0")
	}
	if e.Trigger != "auto" {
		t.Errorf("Trigger = %q, want %q", e.Trigger, "auto")
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil error", nil, ""},
		{"checksum error", errors.New("checksum mismatch for file"), ErrorTypeChecksumFailed},
		{"sha256 error", errors.New("sha256 verification failed"), ErrorTypeChecksumFailed},
		{"signature error", errors.New("invalid signature"), ErrorTypeChecksumFailed},
		{"extract error", errors.New("failed to extract archive"), ErrorTypeExtractionFailed},
		{"untar error", errors.New("untar: unexpected EOF"), ErrorTypeExtractionFailed},
		{"unzip error", errors.New("unzip failed"), ErrorTypeExtractionFailed},
		{"decompress error", errors.New("decompress: invalid header"), ErrorTypeExtractionFailed},
		{"permission error", errors.New("permission denied"), ErrorTypePermissionFailed},
		{"chmod error", errors.New("chmod +x failed"), ErrorTypePermissionFailed},
		{"symlink error", errors.New("symlink creation failed"), ErrorTypeSymlinkFailed},
		{"verification error", errors.New("verification failed for tool"), ErrorTypeVerificationFailed},
		{"verify error", errors.New("could not verify binary"), ErrorTypeVerificationFailed},
		{"state.json error", errors.New("failed to write state.json"), ErrorTypeStateFailed},
		{"state file error", errors.New("corrupt state file"), ErrorTypeStateFailed},
		{"resolve error", errors.New("could not resolve version"), ErrorTypeVersionResolveFailed},
		{"version provider error", errors.New("version provider timeout"), ErrorTypeVersionResolveFailed},
		{"no matching version", errors.New("no matching version found"), ErrorTypeVersionResolveFailed},
		{"download error", errors.New("download failed: 404"), ErrorTypeDownloadFailed},
		{"HTTP error", errors.New("HTTP 503 from server"), ErrorTypeDownloadFailed},
		{"timeout error", errors.New("connection timeout"), ErrorTypeDownloadFailed},
		{"connection error", errors.New("connection refused"), ErrorTypeDownloadFailed},
		{"unknown error", errors.New("something unexpected happened"), ErrorTypeUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ClassifyError(tt.err)
			if got != tt.want {
				t.Errorf("ClassifyError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
