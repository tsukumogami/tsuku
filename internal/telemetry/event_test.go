package telemetry

import (
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

func TestNewLLMProviderFailoverEvent(t *testing.T) {
	e := NewLLMProviderFailoverEvent("claude", "gemini", "circuit_breaker_open")

	if e.Action != "llm_provider_failover" {
		t.Errorf("Action = %q, want %q", e.Action, "llm_provider_failover")
	}
	if e.FromProvider != "claude" {
		t.Errorf("FromProvider = %q, want %q", e.FromProvider, "claude")
	}
	if e.ToProvider != "gemini" {
		t.Errorf("ToProvider = %q, want %q", e.ToProvider, "gemini")
	}
	if e.Reason != "circuit_breaker_open" {
		t.Errorf("Reason = %q, want %q", e.Reason, "circuit_breaker_open")
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
